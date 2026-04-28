package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func normalizeCategory(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return "other"
	}
	return v
}

func getAccountIDByCode(tx *gorm.DB, tenantID uuid.UUID, code string) (uuid.UUID, error) {
	var account models.Account
	if err := tx.Where("tenant_id = ? AND code = ? AND is_active = ?", tenantID, code, true).First(&account).Error; err != nil {
		return uuid.Nil, err
	}
	return account.ID, nil
}

func findPostingRule(tx *gorm.DB, tenantID uuid.UUID, sourceType, category string) (*models.PostingRule, error) {
	sourceType = strings.TrimSpace(strings.ToLower(sourceType))
	category = normalizeCategory(category)

	var exact models.PostingRule
	if err := tx.Where("tenant_id = ? AND source_type = ? AND category = ? AND is_active = ?", tenantID, sourceType, category, true).
		Order("sort_order ASC, created_at ASC").
		First(&exact).Error; err == nil {
		return &exact, nil
	}

	var wildcard models.PostingRule
	if err := tx.Where("tenant_id = ? AND source_type = ? AND category = ? AND is_active = ?", tenantID, sourceType, "*", true).
		Order("sort_order ASC, created_at ASC").
		First(&wildcard).Error; err == nil {
		return &wildcard, nil
	}

	return nil, gorm.ErrRecordNotFound
}

func fallbackAccountCodes(sourceType, category string) (string, string) {
	category = normalizeCategory(category)

	if sourceType == "income" {
		debit := "1110"
		credit := "4190"
		switch category {
		case "order":
			credit = "4110"
		case "service_order", "service":
			credit = "4120"
		case "commission":
			credit = "4130"
		}
		return debit, credit
	}

	debit := "6900"
	credit := "1110"
	switch category {
	case "purchase":
		debit = "5100"
	case "salary":
		debit = "6100"
	case "rent":
		debit = "6200"
	case "utility":
		debit = "6300"
	case "order_commission", "service_order_commission", "commission":
		debit = "6600"
	case "product_tax", "service_tax":
		debit = "1150"
	case "refund":
		debit = "6900"
	}

	return debit, credit
}

func resolvePostingAccounts(tx *gorm.DB, tenantID uuid.UUID, sourceType, category string) (uuid.UUID, uuid.UUID, string, error) {
	rule, err := findPostingRule(tx, tenantID, sourceType, category)
	if err == nil {
		return rule.DebitAccountID, rule.CreditAccountID, "rule", nil
	}

	debitCode, creditCode := fallbackAccountCodes(sourceType, category)
	debitID, err := getAccountIDByCode(tx, tenantID, debitCode)
	if err != nil {
		return uuid.Nil, uuid.Nil, "fallback", fmt.Errorf("debit account %s not found", debitCode)
	}
	creditID, err := getAccountIDByCode(tx, tenantID, creditCode)
	if err != nil {
		return uuid.Nil, uuid.Nil, "fallback", fmt.Errorf("credit account %s not found", creditCode)
	}

	return debitID, creditID, "fallback", nil
}

func upsertJournalEntryForSource(tx *gorm.DB, tenantID uuid.UUID, userID *uuid.UUID, sourceType string, sourceID uuid.UUID, entryDate time.Time, description string, amount float64, debitAccountID, creditAccountID uuid.UUID) (uuid.UUID, error) {
	if amount <= 0 {
		return uuid.Nil, fmt.Errorf("amount must be greater than zero")
	}

	var entry models.JournalEntry
	err := tx.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, sourceType, sourceID).First(&entry).Error
	now := utils.NowInTenantTimezone(tenantID)

	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return uuid.Nil, err
		}
		entry = models.JournalEntry{
			TenantID:      tenantID,
			EntryNumber:   fmt.Sprintf("JE-%s-%s", entryDate.Format("20060102"), uuid.New().String()[:8]),
			EntryDate:     entryDate,
			Description:   description,
			ReferenceType: sourceType,
			ReferenceID:   &sourceID,
			Status:        "posted",
			TotalDebit:    amount,
			TotalCredit:   amount,
			CreatedBy:     userID,
			UpdatedBy:     userID,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return uuid.Nil, err
		}
	} else {
		entry.EntryDate = entryDate
		entry.Description = description
		entry.Status = "posted"
		entry.TotalDebit = amount
		entry.TotalCredit = amount
		entry.UpdatedBy = userID
		entry.UpdatedAt = now
		if err := tx.Save(&entry).Error; err != nil {
			return uuid.Nil, err
		}
		if err := tx.Where("tenant_id = ? AND journal_entry_id = ?", tenantID, entry.ID).Delete(&models.JournalEntryLine{}).Error; err != nil {
			return uuid.Nil, err
		}
	}

	lines := []models.JournalEntryLine{
		{
			TenantID:       tenantID,
			JournalEntryID: entry.ID,
			AccountID:      debitAccountID,
			Description:    description,
			DebitAmount:    amount,
			CreditAmount:   0,
			CreatedAt:      now,
		},
		{
			TenantID:       tenantID,
			JournalEntryID: entry.ID,
			AccountID:      creditAccountID,
			Description:    description,
			DebitAmount:    0,
			CreditAmount:   amount,
			CreatedAt:      now,
		},
	}

	if err := tx.Create(&lines).Error; err != nil {
		return uuid.Nil, err
	}

	return entry.ID, nil
}

func removeJournalEntryForSource(tx *gorm.DB, tenantID uuid.UUID, sourceType string, sourceID uuid.UUID) error {
	var entry models.JournalEntry
	if err := tx.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, sourceType, sourceID).First(&entry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}

	if err := tx.Where("tenant_id = ? AND journal_entry_id = ?", tenantID, entry.ID).Delete(&models.JournalEntryLine{}).Error; err != nil {
		return err
	}
	if err := tx.Where("tenant_id = ? AND id = ?", tenantID, entry.ID).Delete(&models.JournalEntry{}).Error; err != nil {
		return err
	}
	return nil
}

func syncIncomeJournalEntryTx(tx *gorm.DB, income *models.Income, userID *uuid.UUID) error {
	if income == nil {
		return nil
	}
	status := strings.TrimSpace(strings.ToLower(income.Status))
	if status != "confirmed" || income.Amount <= 0 {
		if err := removeJournalEntryForSource(tx, income.TenantID, "income", income.ID); err != nil {
			return err
		}
		if income.JournalEntryID != nil {
			if err := tx.Model(&models.Income{}).
				Where("tenant_id = ? AND id = ?", income.TenantID, income.ID).
				Update("journal_entry_id", nil).Error; err != nil {
				return err
			}
			income.JournalEntryID = nil
		}
		return nil
	}

	debitID, creditID, _, err := resolvePostingAccounts(tx, income.TenantID, "income", income.Category)
	if err != nil {
		return err
	}

	desc := strings.TrimSpace(income.Description)
	if desc == "" {
		desc = fmt.Sprintf("Income %s", income.ID.String())
	}

	jeID, err := upsertJournalEntryForSource(tx, income.TenantID, userID, "income", income.ID, income.IncomeDate, desc, income.Amount, debitID, creditID)
	if err != nil {
		return err
	}

	if err := tx.Model(&models.Income{}).
		Where("tenant_id = ? AND id = ?", income.TenantID, income.ID).
		Update("journal_entry_id", jeID).Error; err != nil {
		return err
	}
	income.JournalEntryID = &jeID

	return nil
}

func syncExpenseJournalEntryTx(tx *gorm.DB, expense *models.Expense, userID *uuid.UUID) error {
	if expense == nil {
		return nil
	}
	status := strings.TrimSpace(strings.ToLower(expense.Status))
	if status != "confirmed" || expense.Amount <= 0 {
		if err := removeJournalEntryForSource(tx, expense.TenantID, "expense", expense.ID); err != nil {
			return err
		}
		if expense.JournalEntryID != nil {
			if err := tx.Model(&models.Expense{}).
				Where("tenant_id = ? AND id = ?", expense.TenantID, expense.ID).
				Update("journal_entry_id", nil).Error; err != nil {
				return err
			}
			expense.JournalEntryID = nil
		}
		return nil
	}

	debitID, creditID, _, err := resolvePostingAccounts(tx, expense.TenantID, "expense", expense.Category)
	if err != nil {
		return err
	}

	desc := strings.TrimSpace(expense.Description)
	if desc == "" {
		desc = fmt.Sprintf("Expense %s", expense.ID.String())
	}

	jeID, err := upsertJournalEntryForSource(tx, expense.TenantID, userID, "expense", expense.ID, expense.ExpenseDate, desc, expense.Amount, debitID, creditID)
	if err != nil {
		return err
	}

	if err := tx.Model(&models.Expense{}).
		Where("tenant_id = ? AND id = ?", expense.TenantID, expense.ID).
		Update("journal_entry_id", jeID).Error; err != nil {
		return err
	}
	expense.JournalEntryID = &jeID

	return nil
}

func ensureDefaultPostingRulesTx(tx *gorm.DB, tenantID uuid.UUID) (int, error) {
	var count int64
	if err := tx.Model(&models.PostingRule{}).Where("tenant_id = ?", tenantID).Count(&count).Error; err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, nil
	}

	type def struct {
		SourceType string
		Category   string
		DebitCode  string
		CreditCode string
		Desc       string
		Sort       int
	}
	defs := []def{
		{SourceType: "income", Category: "order", DebitCode: "1110", CreditCode: "4110", Desc: "訂單收入", Sort: 10},
		{SourceType: "income", Category: "service_order", DebitCode: "1110", CreditCode: "4120", Desc: "服務收入", Sort: 20},
		{SourceType: "income", Category: "*", DebitCode: "1110", CreditCode: "4190", Desc: "一般收入", Sort: 999},
		{SourceType: "expense", Category: "purchase", DebitCode: "5100", CreditCode: "1110", Desc: "採購支出", Sort: 10},
		{SourceType: "expense", Category: "rent", DebitCode: "6200", CreditCode: "1110", Desc: "租金支出", Sort: 20},
		{SourceType: "expense", Category: "utility", DebitCode: "6300", CreditCode: "1110", Desc: "水電支出", Sort: 30},
		{SourceType: "expense", Category: "salary", DebitCode: "6100", CreditCode: "1110", Desc: "薪資支出", Sort: 40},
		{SourceType: "expense", Category: "order_commission", DebitCode: "6600", CreditCode: "1110", Desc: "訂單佣金", Sort: 50},
		{SourceType: "expense", Category: "service_order_commission", DebitCode: "6600", CreditCode: "1110", Desc: "服務單佣金", Sort: 60},
		{SourceType: "expense", Category: "product_tax", DebitCode: "1150", CreditCode: "1110", Desc: "產品進項稅", Sort: 70},
		{SourceType: "expense", Category: "service_tax", DebitCode: "1150", CreditCode: "1110", Desc: "服務進項稅", Sort: 80},
		{SourceType: "expense", Category: "*", DebitCode: "6900", CreditCode: "1110", Desc: "一般支出", Sort: 999},
	}

	inserted := 0
	for _, d := range defs {
		debitID, err := getAccountIDByCode(tx, tenantID, d.DebitCode)
		if err != nil {
			continue
		}
		creditID, err := getAccountIDByCode(tx, tenantID, d.CreditCode)
		if err != nil {
			continue
		}

		row := models.PostingRule{
			TenantID:        tenantID,
			SourceType:      d.SourceType,
			Category:        d.Category,
			DebitAccountID:  debitID,
			CreditAccountID: creditID,
			Description:     d.Desc,
			IsSystem:        true,
			IsActive:        true,
			SortOrder:       d.Sort,
		}
		if err := tx.Create(&row).Error; err != nil {
			return inserted, err
		}
		inserted++
	}

	return inserted, nil
}

func backfillJournalEntriesTx(tx *gorm.DB, tenantID uuid.UUID) (int, int, error) {
	var incomes []models.Income
	if err := tx.Where("tenant_id = ? AND status = ?", tenantID, "confirmed").Find(&incomes).Error; err != nil {
		return 0, 0, err
	}

	incomeCount := 0
	for i := range incomes {
		if err := syncIncomeJournalEntryTx(tx, &incomes[i], nil); err == nil {
			incomeCount++
		}
	}

	var expenses []models.Expense
	if err := tx.Where("tenant_id = ? AND status = ?", tenantID, "confirmed").Find(&expenses).Error; err != nil {
		return incomeCount, 0, err
	}

	expenseCount := 0
	for i := range expenses {
		if err := syncExpenseJournalEntryTx(tx, &expenses[i], nil); err == nil {
			expenseCount++
		}
	}

	return incomeCount, expenseCount, nil
}

func syncInvoiceJournalEntryTx(tx *gorm.DB, invoice *models.Invoice, userID *uuid.UUID) error {
	if invoice == nil {
		return nil
	}
	status := strings.TrimSpace(strings.ToLower(invoice.Status))
	if status == "draft" || status == "cancelled" || status == "void" || invoice.TotalAmount <= 0 {
		return removeJournalEntryForSource(tx, invoice.TenantID, "invoice", invoice.ID)
	}

	debitCode := "1120"
	if invoice.PaidAmount >= invoice.TotalAmount {
		debitCode = "1110"
	}

	debitID, err := getAccountIDByCode(tx, invoice.TenantID, debitCode)
	if err != nil {
		return fmt.Errorf("debit account %s not found", debitCode)
	}
	creditID, err := getAccountIDByCode(tx, invoice.TenantID, "4110")
	if err != nil {
		creditID, err = getAccountIDByCode(tx, invoice.TenantID, "4190")
		if err != nil {
			return fmt.Errorf("credit revenue account not found")
		}
	}

	desc := strings.TrimSpace(invoice.Notes)
	if desc == "" {
		desc = fmt.Sprintf("Invoice %s", invoice.InvoiceNumber)
	}

	_, err = upsertJournalEntryForSource(
		tx,
		invoice.TenantID,
		userID,
		"invoice",
		invoice.ID,
		invoice.InvoiceDate,
		desc,
		invoice.TotalAmount,
		debitID,
		creditID,
	)
	return err
}

func GetAccountingSetupStatus(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var accountCount int64
	var ruleCount int64
	var incomePosted int64
	var expensePosted int64

	database.DB.Model(&models.Account{}).Where("tenant_id = ?", tenantID).Count(&accountCount)
	database.DB.Model(&models.PostingRule{}).Where("tenant_id = ?", tenantID).Count(&ruleCount)
	database.DB.Model(&models.Income{}).Where("tenant_id = ? AND journal_entry_id IS NOT NULL", tenantID).Count(&incomePosted)
	database.DB.Model(&models.Expense{}).Where("tenant_id = ? AND journal_entry_id IS NOT NULL", tenantID).Count(&expensePosted)

	setupCompleted := accountCount > 0 && ruleCount > 0

	return c.JSON(fiber.Map{
		"account_count":        accountCount,
		"posting_rule_count":   ruleCount,
		"income_posted_count":  incomePosted,
		"expense_posted_count": expensePosted,
		"setup_completed":      setupCompleted,
	})
}
