package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type reportPeriod struct {
	Start time.Time
	End   time.Time
}

func parseReportPeriod(c *fiber.Ctx, tenantID uuid.UUID) reportPeriod {
	loc := utils.GetTenantLocation(tenantID)
	now := utils.NowInTenantTimezone(tenantID)

	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	end := now

	if s := strings.TrimSpace(c.Query("start_date")); s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			start = t
		}
	}
	if s := strings.TrimSpace(c.Query("end_date")); s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			end = t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		}
	}

	return reportPeriod{Start: start, End: end}
}

func toDateString(t time.Time) string {
	return t.Format("2006-01-02")
}

func boolParam(c *fiber.Ctx, key string) bool {
	v := strings.ToLower(strings.TrimSpace(c.Query(key)))
	return v == "1" || v == "true" || v == "yes"
}

// ============================================
// 會計科目表 (Chart of Accounts)
// ============================================

func GetAccounts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var accounts []models.Account
	query := database.DB.Where("tenant_id = ?", tenantID)

	if t := strings.TrimSpace(c.Query("account_type")); t != "" {
		query = query.Where("account_type = ?", t)
	}
	if active := strings.TrimSpace(c.Query("is_active")); active != "" {
		if active == "true" || active == "1" {
			query = query.Where("is_active = ?", true)
		} else if active == "false" || active == "0" {
			query = query.Where("is_active = ?", false)
		}
	}
	if q := strings.TrimSpace(c.Query("search")); q != "" {
		query = query.Where("code ILIKE ? OR name ILIKE ?", "%"+q+"%", "%"+q+"%")
	}

	var total int64
	query.Model(&models.Account{}).Count(&total)

	if boolParam(c, "all") {
		if err := query.Order("sort_order ASC, code ASC").Find(&accounts).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch accounts"})
		}
		return c.JSON(fiber.Map{"data": accounts, "total": total})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	if err := query.Order("sort_order ASC, code ASC").Offset(offset).Limit(limit).Find(&accounts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch accounts"})
	}

	return c.JSON(fiber.Map{
		"data":  accounts,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetAccount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid account ID"})
	}

	var account models.Account
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&account).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Account not found"})
	}

	return c.JSON(account)
}

func CreateAccount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		ParentID    *uuid.UUID `json:"parent_id"`
		Code        string     `json:"code"`
		Name        string     `json:"name"`
		AccountType string     `json:"account_type"`
		SubType     string     `json:"sub_type"`
		Description string     `json:"description"`
		Currency    string     `json:"currency"`
		IsActive    *bool      `json:"is_active"`
		TaxRate     float64    `json:"tax_rate"`
		SortOrder   int        `json:"sort_order"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.Code = strings.TrimSpace(req.Code)
	req.Name = strings.TrimSpace(req.Name)
	req.AccountType = strings.TrimSpace(req.AccountType)
	if req.Code == "" || req.Name == "" || req.AccountType == "" {
		return c.Status(400).JSON(fiber.Map{"error": "code, name, account_type are required"})
	}

	var exists int64
	database.DB.Model(&models.Account{}).Where("tenant_id = ? AND code = ?", tenantID, req.Code).Count(&exists)
	if exists > 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Account code already exists"})
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	account := models.Account{
		TenantID:    tenantID,
		ParentID:    req.ParentID,
		Code:        req.Code,
		Name:        req.Name,
		AccountType: req.AccountType,
		SubType:     req.SubType,
		Description: req.Description,
		Currency:    req.Currency,
		IsActive:    isActive,
		TaxRate:     req.TaxRate,
		SortOrder:   req.SortOrder,
	}

	if err := database.DB.Create(&account).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create account"})
	}

	return c.Status(201).JSON(account)
}

func UpdateAccount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid account ID"})
	}

	var account models.Account
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&account).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Account not found"})
	}

	var req struct {
		ParentID    *uuid.UUID `json:"parent_id"`
		Code        string     `json:"code"`
		Name        string     `json:"name"`
		AccountType string     `json:"account_type"`
		SubType     string     `json:"sub_type"`
		Description string     `json:"description"`
		Currency    string     `json:"currency"`
		IsActive    *bool      `json:"is_active"`
		TaxRate     float64    `json:"tax_rate"`
		SortOrder   *int       `json:"sort_order"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if account.IsSystem && req.Code != "" && strings.TrimSpace(req.Code) != account.Code {
		return c.Status(400).JSON(fiber.Map{"error": "System account code cannot be changed"})
	}

	if req.Code != "" {
		newCode := strings.TrimSpace(req.Code)
		if newCode != account.Code {
			var exists int64
			database.DB.Model(&models.Account{}).Where("tenant_id = ? AND code = ? AND id <> ?", tenantID, newCode, account.ID).Count(&exists)
			if exists > 0 {
				return c.Status(400).JSON(fiber.Map{"error": "Account code already exists"})
			}
			account.Code = newCode
		}
	}
	if req.Name != "" {
		account.Name = strings.TrimSpace(req.Name)
	}
	if req.AccountType != "" {
		account.AccountType = strings.TrimSpace(req.AccountType)
	}
	account.ParentID = req.ParentID
	account.SubType = req.SubType
	account.Description = req.Description
	account.Currency = req.Currency
	account.TaxRate = req.TaxRate
	if req.IsActive != nil {
		account.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		account.SortOrder = *req.SortOrder
	}

	if err := database.DB.Save(&account).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update account"})
	}

	return c.JSON(account)
}

func DeleteAccount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid account ID"})
	}

	var account models.Account
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&account).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Account not found"})
	}
	if account.IsSystem {
		return c.Status(400).JSON(fiber.Map{"error": "System account cannot be deleted"})
	}

	var childCount int64
	database.DB.Model(&models.Account{}).Where("tenant_id = ? AND parent_id = ?", tenantID, account.ID).Count(&childCount)
	if childCount > 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Please delete child accounts first"})
	}

	var lineCount int64
	database.DB.Model(&models.JournalEntryLine{}).Where("tenant_id = ? AND account_id = ?", tenantID, account.ID).Count(&lineCount)
	if lineCount > 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Account has journal entries and cannot be deleted"})
	}

	if err := database.DB.Delete(&account).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete account"})
	}

	return c.JSON(fiber.Map{"message": "Account deleted successfully"})
}

func InitializeDefaultAccounts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var count int64
	database.DB.Model(&models.Account{}).Where("tenant_id = ?", tenantID).Count(&count)

	insertedAccounts := 0
	insertedRules := 0
	postedIncomes := 0
	postedExpenses := 0
	initializedAccounts := false

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if count == 0 {
			type accountNode struct {
				Data   models.DefaultAccount
				Parent *uuid.UUID
			}

			queue := make([]accountNode, 0)
			for _, root := range models.GetDefaultChartOfAccounts() {
				queue = append(queue, accountNode{Data: root})
			}

			for len(queue) > 0 {
				node := queue[0]
				queue = queue[1:]

				account := models.Account{
					TenantID:    tenantID,
					ParentID:    node.Parent,
					Code:        node.Data.Code,
					Name:        node.Data.Name,
					AccountType: node.Data.AccountType,
					SubType:     node.Data.SubType,
					SortOrder:   node.Data.SortOrder,
					TaxRate:     node.Data.TaxRate,
					IsSystem:    true,
					IsActive:    true,
				}
				if err := tx.Create(&account).Error; err != nil {
					return err
				}
				insertedAccounts++

				for _, ch := range node.Data.Children {
					parentID := account.ID
					queue = append(queue, accountNode{Data: ch, Parent: &parentID})
				}
			}
			initializedAccounts = true
		}

		var err error
		insertedRules, err = ensureDefaultPostingRulesTx(tx, tenantID)
		if err != nil {
			return err
		}

		postedIncomes, postedExpenses, err = backfillJournalEntriesTx(tx, tenantID)
		if err != nil {
			return err
		}

		if userID != uuid.Nil {
			_ = userID
		}
		return nil
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to initialize accounting setup"})
	}

	return c.JSON(fiber.Map{
		"message":                "Accounting setup completed",
		"initialized_accounts":   initializedAccounts,
		"accounts_inserted":      insertedAccounts,
		"posting_rules_inserted": insertedRules,
		"income_posted":          postedIncomes,
		"expense_posted":         postedExpenses,
		"setup_completed":        true,
	})
}

// ============================================
// 日記帳 (Journal Entries)
// ============================================

func GetJournalEntries(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var entries []models.JournalEntry
	query := database.DB.Where("tenant_id = ?", tenantID)

	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if startDate := strings.TrimSpace(c.Query("start_date")); startDate != "" {
		query = query.Where("entry_date >= ?", startDate)
	}
	if endDate := strings.TrimSpace(c.Query("end_date")); endDate != "" {
		query = query.Where("entry_date <= ?", endDate)
	}

	var total int64
	query.Model(&models.JournalEntry{}).Count(&total)

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	if err := query.Order("entry_date DESC, created_at DESC").Offset(offset).Limit(limit).Find(&entries).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch journal entries"})
	}

	return c.JSON(fiber.Map{
		"data":  entries,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetJournalEntry(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid journal entry ID"})
	}

	var entry models.JournalEntry
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("Lines").
		Preload("Lines.Account").
		First(&entry).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Journal entry not found"})
	}

	resp := fiber.Map{
		"id":             entry.ID,
		"tenant_id":      entry.TenantID,
		"entry_number":   entry.EntryNumber,
		"entry_date":     entry.EntryDate,
		"description":    entry.Description,
		"reference_type": entry.ReferenceType,
		"reference_id":   entry.ReferenceID,
		"status":         entry.Status,
		"total_debit":    entry.TotalDebit,
		"total_credit":   entry.TotalCredit,
		"lines":          entry.Lines,
	}

	if len(entry.Lines) >= 2 {
		for _, l := range entry.Lines {
			if l.DebitAmount > 0 && resp["debit_account_id"] == nil {
				resp["debit_account_id"] = l.AccountID
				resp["debit_description"] = l.Description
				resp["amount"] = l.DebitAmount
			}
			if l.CreditAmount > 0 && resp["credit_account_id"] == nil {
				resp["credit_account_id"] = l.AccountID
				resp["credit_description"] = l.Description
				if resp["amount"] == nil {
					resp["amount"] = l.CreditAmount
				}
			}
		}
	}

	return c.JSON(resp)
}

func CreateJournalEntry(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		EntryNumber       string     `json:"entry_number"`
		EntryDate         string     `json:"entry_date"`
		Description       string     `json:"description"`
		ReferenceType     string     `json:"reference_type"`
		ReferenceID       *uuid.UUID `json:"reference_id"`
		Status            string     `json:"status"`
		DebitAccountID    *uuid.UUID `json:"debit_account_id"`
		CreditAccountID   *uuid.UUID `json:"credit_account_id"`
		Amount            float64    `json:"amount"`
		DebitDescription  string     `json:"debit_description"`
		CreditDescription string     `json:"credit_description"`
		Lines             []struct {
			AccountID    uuid.UUID `json:"account_id"`
			Description  string    `json:"description"`
			DebitAmount  float64   `json:"debit_amount"`
			CreditAmount float64   `json:"credit_amount"`
		} `json:"lines"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if len(req.Lines) < 2 && req.DebitAccountID != nil && req.CreditAccountID != nil && req.Amount > 0 {
		req.Lines = []struct {
			AccountID    uuid.UUID `json:"account_id"`
			Description  string    `json:"description"`
			DebitAmount  float64   `json:"debit_amount"`
			CreditAmount float64   `json:"credit_amount"`
		}{
			{AccountID: *req.DebitAccountID, Description: req.DebitDescription, DebitAmount: req.Amount, CreditAmount: 0},
			{AccountID: *req.CreditAccountID, Description: req.CreditDescription, DebitAmount: 0, CreditAmount: req.Amount},
		}
	}
	if len(req.Lines) < 2 {
		return c.Status(400).JSON(fiber.Map{"error": "Journal entry requires at least two lines (or debit/credit account + amount)"})
	}

	entryDate := utils.NowInTenantTimezone(tenantID)
	if req.EntryDate != "" {
		if t, err := utils.ParseDateInTenantTimezone(tenantID, req.EntryDate); err == nil {
			entryDate = t
		}
	}

	if strings.TrimSpace(req.EntryNumber) == "" {
		req.EntryNumber = fmt.Sprintf("JE-%s-%s", entryDate.Format("20060102"), uuid.New().String()[:8])
	}
	if req.Status == "" {
		req.Status = "posted"
	}

	var totalDebit float64
	var totalCredit float64
	for _, l := range req.Lines {
		totalDebit += l.DebitAmount
		totalCredit += l.CreditAmount
	}
	if totalDebit <= 0 || totalCredit <= 0 || absFloat(totalDebit-totalCredit) > 0.0001 {
		return c.Status(400).JSON(fiber.Map{"error": "Total debit must equal total credit and be greater than zero"})
	}

	entry := models.JournalEntry{
		TenantID:      tenantID,
		EntryNumber:   req.EntryNumber,
		EntryDate:     entryDate,
		Description:   req.Description,
		ReferenceType: req.ReferenceType,
		ReferenceID:   req.ReferenceID,
		Status:        req.Status,
		TotalDebit:    totalDebit,
		TotalCredit:   totalCredit,
		CreatedBy:     &userID,
		UpdatedBy:     &userID,
		CreatedAt:     utils.NowInTenantTimezone(tenantID),
		UpdatedAt:     utils.NowInTenantTimezone(tenantID),
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		for _, l := range req.Lines {
			line := models.JournalEntryLine{
				TenantID:       tenantID,
				JournalEntryID: entry.ID,
				AccountID:      l.AccountID,
				Description:    l.Description,
				DebitAmount:    l.DebitAmount,
				CreditAmount:   l.CreditAmount,
				CreatedAt:      utils.NowInTenantTimezone(tenantID),
			}
			if err := tx.Create(&line).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create journal entry"})
	}

	var created models.JournalEntry
	database.DB.Where("id = ?", entry.ID).Preload("Lines").Preload("Lines.Account").First(&created)
	return c.Status(201).JSON(created)
}

func UpdateJournalEntry(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid journal entry ID"})
	}

	var entry models.JournalEntry
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&entry).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Journal entry not found"})
	}

	var req struct {
		EntryDate         *string    `json:"entry_date"`
		Description       *string    `json:"description"`
		ReferenceType     *string    `json:"reference_type"`
		ReferenceID       *uuid.UUID `json:"reference_id"`
		Status            *string    `json:"status"`
		DebitAccountID    *uuid.UUID `json:"debit_account_id"`
		CreditAccountID   *uuid.UUID `json:"credit_account_id"`
		Amount            *float64   `json:"amount"`
		DebitDescription  *string    `json:"debit_description"`
		CreditDescription *string    `json:"credit_description"`
		Lines             []struct {
			AccountID    uuid.UUID `json:"account_id"`
			Description  string    `json:"description"`
			DebitAmount  float64   `json:"debit_amount"`
			CreditAmount float64   `json:"credit_amount"`
		} `json:"lines"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.EntryDate != nil {
		if t, err := utils.ParseDateInTenantTimezone(tenantID, *req.EntryDate); err == nil {
			entry.EntryDate = t
		}
	}
	if req.Description != nil {
		entry.Description = *req.Description
	}
	if req.ReferenceType != nil {
		entry.ReferenceType = *req.ReferenceType
	}
	if req.ReferenceID != nil {
		entry.ReferenceID = req.ReferenceID
	}
	if req.Status != nil {
		entry.Status = *req.Status
	}

	entry.UpdatedBy = &userID
	entry.UpdatedAt = utils.NowInTenantTimezone(tenantID)

	if len(req.Lines) == 0 && req.DebitAccountID != nil && req.CreditAccountID != nil && req.Amount != nil && *req.Amount > 0 {
		debitDesc := ""
		creditDesc := ""
		if req.DebitDescription != nil {
			debitDesc = *req.DebitDescription
		}
		if req.CreditDescription != nil {
			creditDesc = *req.CreditDescription
		}
		req.Lines = []struct {
			AccountID    uuid.UUID `json:"account_id"`
			Description  string    `json:"description"`
			DebitAmount  float64   `json:"debit_amount"`
			CreditAmount float64   `json:"credit_amount"`
		}{
			{AccountID: *req.DebitAccountID, Description: debitDesc, DebitAmount: *req.Amount, CreditAmount: 0},
			{AccountID: *req.CreditAccountID, Description: creditDesc, DebitAmount: 0, CreditAmount: *req.Amount},
		}
	}

	if len(req.Lines) > 0 {
		var totalDebit float64
		var totalCredit float64
		for _, l := range req.Lines {
			totalDebit += l.DebitAmount
			totalCredit += l.CreditAmount
		}
		if totalDebit <= 0 || totalCredit <= 0 || absFloat(totalDebit-totalCredit) > 0.0001 {
			return c.Status(400).JSON(fiber.Map{"error": "Total debit must equal total credit and be greater than zero"})
		}

		entry.TotalDebit = totalDebit
		entry.TotalCredit = totalCredit

		if err := database.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(&entry).Error; err != nil {
				return err
			}
			if err := tx.Where("tenant_id = ? AND journal_entry_id = ?", tenantID, entry.ID).Delete(&models.JournalEntryLine{}).Error; err != nil {
				return err
			}
			for _, l := range req.Lines {
				line := models.JournalEntryLine{
					TenantID:       tenantID,
					JournalEntryID: entry.ID,
					AccountID:      l.AccountID,
					Description:    l.Description,
					DebitAmount:    l.DebitAmount,
					CreditAmount:   l.CreditAmount,
					CreatedAt:      utils.NowInTenantTimezone(tenantID),
				}
				if err := tx.Create(&line).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update journal entry"})
		}
	} else {
		if err := database.DB.Save(&entry).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update journal entry"})
		}
	}

	var updated models.JournalEntry
	database.DB.Where("id = ?", entry.ID).Preload("Lines").Preload("Lines.Account").First(&updated)
	return c.JSON(updated)
}

func DeleteJournalEntry(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid journal entry ID"})
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND journal_entry_id = ?", tenantID, id).Delete(&models.JournalEntryLine{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.JournalEntry{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete journal entry"})
	}

	return c.JSON(fiber.Map{"message": "Journal entry deleted successfully"})
}

// ============================================
// 稅務配置 (Tax Config)
// ============================================

func GetTaxConfigs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var rows []models.TaxConfig
	query := database.DB.Where("tenant_id = ?", tenantID)

	if region := strings.TrimSpace(c.Query("region")); region != "" {
		query = query.Where("region = ?", region)
	}
	if taxType := strings.TrimSpace(c.Query("tax_type")); taxType != "" {
		query = query.Where("tax_type = ?", taxType)
	}

	var total int64
	query.Model(&models.TaxConfig{}).Count(&total)

	if boolParam(c, "all") {
		if err := query.Order("tax_type ASC, name ASC").Find(&rows).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch tax configs"})
		}
		return c.JSON(fiber.Map{"data": rows, "total": total})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	if err := query.Order("tax_type ASC, name ASC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch tax configs"})
	}

	return c.JSON(fiber.Map{"data": rows, "total": total, "page": page, "limit": limit})
}

func GetTaxConfig(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax config ID"})
	}

	var row models.TaxConfig
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tax config not found"})
	}

	return c.JSON(row)
}

func GetPostingRules(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var rows []models.PostingRule
	query := database.DB.Where("tenant_id = ?", tenantID)

	if sourceType := strings.TrimSpace(c.Query("source_type")); sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if category := strings.TrimSpace(c.Query("category")); category != "" {
		query = query.Where("category = ?", category)
	}

	var total int64
	query.Model(&models.PostingRule{}).Count(&total)

	if boolParam(c, "all") {
		if err := query.Order("source_type ASC, sort_order ASC, category ASC").Preload("DebitAccount").Preload("CreditAccount").Find(&rows).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch posting rules"})
		}
		return c.JSON(fiber.Map{"data": rows, "total": total})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	if err := query.Order("source_type ASC, sort_order ASC, category ASC").Offset(offset).Limit(limit).
		Preload("DebitAccount").Preload("CreditAccount").
		Find(&rows).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch posting rules"})
	}

	return c.JSON(fiber.Map{"data": rows, "total": total, "page": page, "limit": limit})
}

func GetPostingRule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid posting rule ID"})
	}

	var row models.PostingRule
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("DebitAccount").Preload("CreditAccount").
		First(&row).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Posting rule not found"})
	}

	return c.JSON(row)
}

func CreatePostingRule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		SourceType      string    `json:"source_type"`
		Category        string    `json:"category"`
		DebitAccountID  uuid.UUID `json:"debit_account_id"`
		CreditAccountID uuid.UUID `json:"credit_account_id"`
		Description     string    `json:"description"`
		IsActive        *bool     `json:"is_active"`
		SortOrder       int       `json:"sort_order"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.SourceType = strings.TrimSpace(strings.ToLower(req.SourceType))
	req.Category = normalizeCategory(req.Category)
	if req.SourceType == "" || req.DebitAccountID == uuid.Nil || req.CreditAccountID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "source_type, debit_account_id, credit_account_id are required"})
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	row := models.PostingRule{
		TenantID:        tenantID,
		SourceType:      req.SourceType,
		Category:        req.Category,
		DebitAccountID:  req.DebitAccountID,
		CreditAccountID: req.CreditAccountID,
		Description:     req.Description,
		IsSystem:        false,
		IsActive:        isActive,
		SortOrder:       req.SortOrder,
	}

	if err := database.DB.Create(&row).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create posting rule"})
	}

	return c.Status(201).JSON(row)
}

func UpdatePostingRule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid posting rule ID"})
	}

	var row models.PostingRule
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Posting rule not found"})
	}

	var req struct {
		SourceType      *string    `json:"source_type"`
		Category        *string    `json:"category"`
		DebitAccountID  *uuid.UUID `json:"debit_account_id"`
		CreditAccountID *uuid.UUID `json:"credit_account_id"`
		Description     *string    `json:"description"`
		IsActive        *bool      `json:"is_active"`
		SortOrder       *int       `json:"sort_order"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if row.IsSystem && req.Category != nil {
		if normalizeCategory(*req.Category) != row.Category {
			return c.Status(400).JSON(fiber.Map{"error": "System posting rule category cannot be changed"})
		}
	}

	if req.SourceType != nil {
		row.SourceType = strings.TrimSpace(strings.ToLower(*req.SourceType))
	}
	if req.Category != nil {
		row.Category = normalizeCategory(*req.Category)
	}
	if req.DebitAccountID != nil {
		row.DebitAccountID = *req.DebitAccountID
	}
	if req.CreditAccountID != nil {
		row.CreditAccountID = *req.CreditAccountID
	}
	if req.Description != nil {
		row.Description = *req.Description
	}
	if req.IsActive != nil {
		row.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		row.SortOrder = *req.SortOrder
	}

	if err := database.DB.Save(&row).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update posting rule"})
	}

	return c.JSON(row)
}

func DeletePostingRule(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid posting rule ID"})
	}

	var row models.PostingRule
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Posting rule not found"})
	}
	if row.IsSystem {
		return c.Status(400).JSON(fiber.Map{"error": "System posting rule cannot be deleted"})
	}

	if err := database.DB.Delete(&row).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete posting rule"})
	}

	return c.JSON(fiber.Map{"message": "Posting rule deleted successfully"})
}

func BackfillAccountingEntries(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var incomeCount int
	var expenseCount int

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		incomeCount, expenseCount, err = backfillJournalEntriesTx(tx, tenantID)
		return err
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to backfill accounting entries"})
	}

	return c.JSON(fiber.Map{
		"message":        "Backfill completed",
		"income_posted":  incomeCount,
		"expense_posted": expenseCount,
	})
}

func CreateTaxConfig(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Name        string     `json:"name"`
		Code        string     `json:"code"`
		Region      string     `json:"region"`
		TaxType     string     `json:"tax_type"`
		Rate        float64    `json:"rate"`
		IsDefault   bool       `json:"is_default"`
		IsActive    *bool      `json:"is_active"`
		AccountID   *uuid.UUID `json:"account_id"`
		Description string     `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.TaxType) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and tax_type are required"})
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	row := models.TaxConfig{
		TenantID:    tenantID,
		Name:        strings.TrimSpace(req.Name),
		Code:        strings.TrimSpace(req.Code),
		Region:      strings.TrimSpace(req.Region),
		TaxType:     strings.TrimSpace(req.TaxType),
		Rate:        req.Rate,
		IsDefault:   req.IsDefault,
		IsActive:    isActive,
		AccountID:   req.AccountID,
		Description: req.Description,
	}

	if req.IsDefault {
		database.DB.Model(&models.TaxConfig{}).Where("tenant_id = ? AND tax_type = ?", tenantID, row.TaxType).Update("is_default", false)
	}

	if err := database.DB.Create(&row).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create tax config"})
	}

	return c.Status(201).JSON(row)
}

func UpdateTaxConfig(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax config ID"})
	}

	var row models.TaxConfig
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&row).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tax config not found"})
	}

	var req struct {
		Name        *string    `json:"name"`
		Code        *string    `json:"code"`
		Region      *string    `json:"region"`
		TaxType     *string    `json:"tax_type"`
		Rate        *float64   `json:"rate"`
		IsDefault   *bool      `json:"is_default"`
		IsActive    *bool      `json:"is_active"`
		AccountID   *uuid.UUID `json:"account_id"`
		Description *string    `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name != nil {
		row.Name = strings.TrimSpace(*req.Name)
	}
	if req.Code != nil {
		row.Code = strings.TrimSpace(*req.Code)
	}
	if req.Region != nil {
		row.Region = strings.TrimSpace(*req.Region)
	}
	if req.TaxType != nil {
		row.TaxType = strings.TrimSpace(*req.TaxType)
	}
	if req.Rate != nil {
		row.Rate = *req.Rate
	}
	if req.IsActive != nil {
		row.IsActive = *req.IsActive
	}
	if req.AccountID != nil {
		row.AccountID = req.AccountID
	}
	if req.Description != nil {
		row.Description = *req.Description
	}
	if req.IsDefault != nil {
		row.IsDefault = *req.IsDefault
		if row.IsDefault {
			database.DB.Model(&models.TaxConfig{}).Where("tenant_id = ? AND tax_type = ? AND id <> ?", tenantID, row.TaxType, row.ID).Update("is_default", false)
		}
	}

	if err := database.DB.Save(&row).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update tax config"})
	}

	return c.JSON(row)
}

func DeleteTaxConfig(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax config ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.TaxConfig{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete tax config"})
	}

	return c.JSON(fiber.Map{"message": "Tax config deleted successfully"})
}

// ============================================
// 專業會計報表
// ============================================

type reportResult struct {
	Title   string                   `json:"title"`
	Period  map[string]string        `json:"period"`
	Summary map[string]float64       `json:"summary"`
	Rows    []map[string]interface{} `json:"rows"`
}

type accountBalanceRow struct {
	Code        string
	Name        string
	AccountType string
	SubType     string
	Balance     float64
}

func hasPostedJournalEntries(tenantID uuid.UUID, start, end time.Time) bool {
	var c int64
	database.DB.Model(&models.JournalEntry{}).
		Where("tenant_id = ? AND status = 'posted' AND entry_date >= ? AND entry_date <= ?", tenantID, start, end).
		Count(&c)
	return c > 0
}

func buildProfitLossReport(tenantID uuid.UUID, period reportPeriod) (reportResult, error) {
	result := reportResult{
		Title: "損益表 (Profit & Loss)",
		Period: map[string]string{
			"start_date": toDateString(period.Start),
			"end_date":   toDateString(period.End),
		},
		Summary: map[string]float64{},
		Rows:    []map[string]interface{}{},
	}

	if hasPostedJournalEntries(tenantID, period.Start, period.End) {
		var rows []accountBalanceRow
		if err := database.DB.Raw(`
			SELECT
				a.code,
				a.name,
				a.account_type,
				COALESCE(a.sub_type, '') AS sub_type,
				COALESCE(SUM(
					CASE
						WHEN a.account_type = 'revenue' THEN (l.credit_amount - l.debit_amount)
						WHEN a.account_type = 'expense' THEN (l.debit_amount - l.credit_amount)
						ELSE 0
					END
				), 0) AS balance
			FROM journal_entry_lines l
			JOIN journal_entries e ON e.id = l.journal_entry_id
			JOIN accounts a ON a.id = l.account_id
			WHERE l.tenant_id = ?
			  AND e.status = 'posted'
			  AND e.entry_date >= ?
			  AND e.entry_date <= ?
			  AND a.account_type IN ('revenue', 'expense')
			GROUP BY a.code, a.name, a.account_type, a.sub_type
			HAVING ABS(COALESCE(SUM(
				CASE
					WHEN a.account_type = 'revenue' THEN (l.credit_amount - l.debit_amount)
					WHEN a.account_type = 'expense' THEN (l.debit_amount - l.credit_amount)
					ELSE 0
				END
			), 0)) > 0.0001
			ORDER BY a.code ASC
		`, tenantID, period.Start, period.End).Scan(&rows).Error; err != nil {
			return result, err
		}

		var totalRevenue float64
		var cogs float64
		var operatingExpense float64
		var otherIncome float64
		var otherExpense float64
		var taxExpense float64

		for _, r := range rows {
			result.Rows = append(result.Rows, map[string]interface{}{
				"section":      r.AccountType,
				"account_code": r.Code,
				"account_name": r.Name,
				"sub_type":     r.SubType,
				"amount":       r.Balance,
				"basis":        "journal_entry_lines",
			})

			if r.AccountType == "revenue" {
				totalRevenue += r.Balance
				if r.SubType == "other_income" {
					otherIncome += r.Balance
				}
				continue
			}

			switch r.SubType {
			case "cogs":
				cogs += r.Balance
			case "operating_expense":
				operatingExpense += r.Balance
			case "other_expense":
				otherExpense += r.Balance
			case "tax_expense":
				taxExpense += r.Balance
			default:
				operatingExpense += r.Balance
			}
		}

		grossProfit := totalRevenue - cogs
		netProfit := grossProfit - operatingExpense + otherIncome - otherExpense - taxExpense

		result.Summary["total_income"] = totalRevenue
		result.Summary["cost_of_goods_sold"] = cogs
		result.Summary["gross_profit"] = grossProfit
		result.Summary["operating_expense"] = operatingExpense
		result.Summary["other_income"] = otherIncome
		result.Summary["other_expense"] = otherExpense
		result.Summary["tax_expense"] = taxExpense
		result.Summary["net_profit"] = netProfit
		result.Summary["data_source_journal"] = 1
		return result, nil
	}

	// Fallback: legacy transactional aggregation when no journal entries exist
	type categoryTotal struct {
		Category string
		Total    float64
	}
	var incomeRows []categoryTotal
	if err := database.DB.Raw(`
		SELECT COALESCE(category, 'other') AS category, COALESCE(SUM(amount), 0) AS total
		FROM incomes
		WHERE tenant_id = ?
		  AND status = 'confirmed'
		  AND income_date >= ?
		  AND income_date <= ?
		GROUP BY category
		ORDER BY total DESC
	`, tenantID, period.Start, period.End).Scan(&incomeRows).Error; err != nil {
		return result, err
	}
	var expenseRows []categoryTotal
	if err := database.DB.Raw(`
		SELECT COALESCE(category, 'other') AS category, COALESCE(SUM(amount), 0) AS total
		FROM expenses
		WHERE tenant_id = ?
		  AND status = 'confirmed'
		  AND expense_date >= ?
		  AND expense_date <= ?
		GROUP BY category
		ORDER BY total DESC
	`, tenantID, period.Start, period.End).Scan(&expenseRows).Error; err != nil {
		return result, err
	}
	var totalIncome float64
	for _, r := range incomeRows {
		totalIncome += r.Total
		result.Rows = append(result.Rows, map[string]interface{}{"section": "income", "category": r.Category, "amount": r.Total, "basis": "legacy_transactions"})
	}
	var operatingExpense float64
	for _, r := range expenseRows {
		operatingExpense += r.Total
		result.Rows = append(result.Rows, map[string]interface{}{"section": "expense", "category": r.Category, "amount": r.Total, "basis": "legacy_transactions"})
	}
	result.Summary["total_income"] = totalIncome
	result.Summary["operating_expense"] = operatingExpense
	result.Summary["net_profit"] = totalIncome - operatingExpense
	result.Summary["data_source_journal"] = 0
	return result, nil
}

func buildBalanceSheetReport(tenantID uuid.UUID, period reportPeriod) (reportResult, error) {
	result := reportResult{
		Title: "資產負債表 (Balance Sheet)",
		Period: map[string]string{
			"as_of_date": toDateString(period.End),
		},
		Summary: map[string]float64{},
		Rows:    []map[string]interface{}{},
	}

	if hasPostedJournalEntries(tenantID, time.Date(1970, 1, 1, 0, 0, 0, 0, period.End.Location()), period.End) {
		var rows []accountBalanceRow
		if err := database.DB.Raw(`
			SELECT
				a.code,
				a.name,
				a.account_type,
				COALESCE(a.sub_type, '') AS sub_type,
				COALESCE(SUM(
					CASE
						WHEN a.account_type = 'asset' THEN (l.debit_amount - l.credit_amount)
						WHEN a.account_type IN ('liability', 'equity') THEN (l.credit_amount - l.debit_amount)
						ELSE 0
					END
				), 0) AS balance
			FROM journal_entry_lines l
			JOIN journal_entries e ON e.id = l.journal_entry_id
			JOIN accounts a ON a.id = l.account_id
			WHERE l.tenant_id = ?
			  AND e.status = 'posted'
			  AND e.entry_date <= ?
			  AND a.account_type IN ('asset', 'liability', 'equity')
			GROUP BY a.code, a.name, a.account_type, a.sub_type
			HAVING ABS(COALESCE(SUM(
				CASE
					WHEN a.account_type = 'asset' THEN (l.debit_amount - l.credit_amount)
					WHEN a.account_type IN ('liability', 'equity') THEN (l.credit_amount - l.debit_amount)
					ELSE 0
				END
			), 0)) > 0.0001
			ORDER BY a.code ASC
		`, tenantID, period.End).Scan(&rows).Error; err != nil {
			return result, err
		}

		var assetsTotal float64
		var liabilitiesTotal float64
		var equityTotal float64

		for _, r := range rows {
			section := r.AccountType
			result.Rows = append(result.Rows, map[string]interface{}{
				"section":      section,
				"account_code": r.Code,
				"account_name": r.Name,
				"sub_type":     r.SubType,
				"amount":       r.Balance,
				"basis":        "journal_entry_lines",
			})
			switch r.AccountType {
			case "asset":
				assetsTotal += r.Balance
			case "liability":
				liabilitiesTotal += r.Balance
			case "equity":
				equityTotal += r.Balance
			}
		}

		result.Summary["total_assets"] = assetsTotal
		result.Summary["total_liabilities"] = liabilitiesTotal
		result.Summary["total_equity"] = equityTotal
		result.Summary["balance_check"] = assetsTotal - liabilitiesTotal - equityTotal
		result.Summary["data_source_journal"] = 1
		return result, nil
	}

	// Fallback legacy when no journal entries exist
	var cashIn, cashOut, ar, ap float64
	_ = database.DB.Raw(`SELECT COALESCE(SUM(amount),0) FROM incomes WHERE tenant_id = ? AND status = 'confirmed' AND income_date <= ?`, tenantID, period.End).Scan(&cashIn).Error
	_ = database.DB.Raw(`SELECT COALESCE(SUM(amount),0) FROM expenses WHERE tenant_id = ? AND status = 'confirmed' AND expense_date <= ?`, tenantID, period.End).Scan(&cashOut).Error
	_ = database.DB.Raw(`SELECT COALESCE(SUM(total_amount - paid_amount),0) FROM invoices WHERE tenant_id = ? AND status IN ('pending','partial') AND invoice_date <= ?`, tenantID, period.End).Scan(&ar).Error
	_ = database.DB.Raw(`SELECT COALESCE(SUM(amount),0) FROM expenses WHERE tenant_id = ? AND status = 'pending' AND expense_date <= ?`, tenantID, period.End).Scan(&ap).Error
	assetsTotal := (cashIn - cashOut) + ar
	liabilitiesTotal := ap
	equityTotal := assetsTotal - liabilitiesTotal
	result.Summary["total_assets"] = assetsTotal
	result.Summary["total_liabilities"] = liabilitiesTotal
	result.Summary["total_equity"] = equityTotal
	result.Summary["data_source_journal"] = 0
	return result, nil
}

func buildCashFlowReport(tenantID uuid.UUID, period reportPeriod) (reportResult, error) {
	result := reportResult{
		Title: "現金流量表 (Cash Flow Statement)",
		Period: map[string]string{
			"start_date": toDateString(period.Start),
			"end_date":   toDateString(period.End),
		},
		Summary: map[string]float64{},
		Rows:    []map[string]interface{}{},
	}

	if hasPostedJournalEntries(tenantID, period.Start, period.End) {
		type sectionRow struct {
			Section string
			NetCash float64
		}
		var sections []sectionRow
		if err := database.DB.Raw(`
			WITH cash_accounts AS (
				SELECT id
				FROM accounts
				WHERE tenant_id = ?
				  AND (code LIKE '111%%' OR name ILIKE '%%cash%%' OR name ILIKE '%%bank%%' OR name LIKE '%%現金%%' OR name LIKE '%%銀行%%')
			),
			entry_cash AS (
				SELECT
					e.id AS journal_entry_id,
					l.id AS line_id,
					(l.debit_amount - l.credit_amount) AS cash_delta
				FROM journal_entries e
				JOIN journal_entry_lines l ON l.journal_entry_id = e.id
				WHERE l.tenant_id = ?
				  AND e.status = 'posted'
				  AND e.entry_date >= ?
				  AND e.entry_date <= ?
				  AND l.account_id IN (SELECT id FROM cash_accounts)
			),
			counterparty AS (
				SELECT
					ec.cash_delta,
					a.account_type,
					COALESCE(a.sub_type, '') AS sub_type
				FROM entry_cash ec
				JOIN journal_entry_lines ol ON ol.journal_entry_id = ec.journal_entry_id AND ol.id <> ec.line_id
				JOIN accounts a ON a.id = ol.account_id
			)
			SELECT
				CASE
					WHEN sub_type = 'fixed_asset' THEN 'investing'
					WHEN account_type IN ('liability', 'equity') THEN 'financing'
					ELSE 'operating'
				END AS section,
				COALESCE(SUM(cash_delta), 0) AS net_cash
			FROM counterparty
			GROUP BY
				CASE
					WHEN sub_type = 'fixed_asset' THEN 'investing'
					WHEN account_type IN ('liability', 'equity') THEN 'financing'
					ELSE 'operating'
				END
		`, tenantID, tenantID, period.Start, period.End).Scan(&sections).Error; err != nil {
			return result, err
		}

		netOperating := 0.0
		netInvesting := 0.0
		netFinancing := 0.0
		for _, s := range sections {
			switch s.Section {
			case "operating":
				netOperating = s.NetCash
			case "investing":
				netInvesting = s.NetCash
			case "financing":
				netFinancing = s.NetCash
			}
			result.Rows = append(result.Rows, map[string]interface{}{"section": s.Section, "net_cash": s.NetCash, "basis": "journal_entry_lines"})
		}

		result.Summary["net_operating_cash_flow"] = netOperating
		result.Summary["net_investing_cash_flow"] = netInvesting
		result.Summary["net_financing_cash_flow"] = netFinancing
		result.Summary["net_cash_flow"] = netOperating + netInvesting + netFinancing
		result.Summary["data_source_journal"] = 1
		return result, nil
	}

	// Fallback legacy when no journal entries exist
	var operatingInflow, operatingOutflow float64
	_ = database.DB.Raw(`SELECT COALESCE(SUM(amount), 0) FROM incomes WHERE tenant_id = ? AND status = 'confirmed' AND income_date >= ? AND income_date <= ?`, tenantID, period.Start, period.End).Scan(&operatingInflow).Error
	_ = database.DB.Raw(`SELECT COALESCE(SUM(amount), 0) FROM expenses WHERE tenant_id = ? AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?`, tenantID, period.Start, period.End).Scan(&operatingOutflow).Error
	result.Summary["net_operating_cash_flow"] = operatingInflow - operatingOutflow
	result.Summary["net_investing_cash_flow"] = 0
	result.Summary["net_financing_cash_flow"] = 0
	result.Summary["net_cash_flow"] = operatingInflow - operatingOutflow
	result.Summary["data_source_journal"] = 0
	return result, nil
}

func buildARAgingReport(tenantID uuid.UUID, period reportPeriod) (reportResult, error) {
	result := reportResult{
		Title: "應收帳款帳齡分析 (A/R Aging)",
		Period: map[string]string{
			"as_of_date": toDateString(period.End),
		},
		Summary: map[string]float64{},
		Rows:    []map[string]interface{}{},
	}

	type arRow struct {
		InvoiceNumber string     `json:"invoice_number"`
		CustomerName  string     `json:"customer_name"`
		InvoiceDate   time.Time  `json:"invoice_date"`
		DueDate       *time.Time `json:"due_date"`
		Outstanding   float64    `json:"outstanding"`
	}

	var rows []arRow
	if err := database.DB.Raw(`
		SELECT
			i.invoice_number,
			COALESCE(c.name, '') AS customer_name,
			i.invoice_date,
			i.due_date,
			COALESCE(i.total_amount - i.paid_amount, 0) AS outstanding
		FROM invoices i
		LEFT JOIN customers c ON c.id = i.customer_id
		WHERE i.tenant_id = ?
		  AND i.status IN ('pending', 'partial')
		  AND i.invoice_date <= ?
		ORDER BY i.invoice_date ASC
	`, tenantID, period.End).Scan(&rows).Error; err != nil {
		return result, err
	}

	buckets := map[string]float64{
		"current": 0,
		"1_30":    0,
		"31_60":   0,
		"61_90":   0,
		"90_plus": 0,
	}
	var totalOutstanding float64

	for _, r := range rows {
		refDate := r.InvoiceDate
		if r.DueDate != nil {
			refDate = *r.DueDate
		}
		days := int(period.End.Sub(refDate).Hours() / 24)
		if days < 0 {
			days = 0
		}

		bucket := "current"
		switch {
		case days == 0:
			bucket = "current"
		case days <= 30:
			bucket = "1_30"
		case days <= 60:
			bucket = "31_60"
		case days <= 90:
			bucket = "61_90"
		default:
			bucket = "90_plus"
		}
		buckets[bucket] += r.Outstanding
		totalOutstanding += r.Outstanding

		result.Rows = append(result.Rows, map[string]interface{}{
			"invoice_number": r.InvoiceNumber,
			"customer_name":  r.CustomerName,
			"invoice_date":   toDateString(r.InvoiceDate),
			"due_date": func() string {
				if r.DueDate == nil {
					return ""
				}
				return toDateString(*r.DueDate)
			}(),
			"days_outstanding": days,
			"bucket":           bucket,
			"outstanding":      r.Outstanding,
		})
	}

	result.Summary["total_outstanding"] = totalOutstanding
	result.Summary["bucket_current"] = buckets["current"]
	result.Summary["bucket_1_30"] = buckets["1_30"]
	result.Summary["bucket_31_60"] = buckets["31_60"]
	result.Summary["bucket_61_90"] = buckets["61_90"]
	result.Summary["bucket_90_plus"] = buckets["90_plus"]

	return result, nil
}

func buildAPAgingReport(tenantID uuid.UUID, period reportPeriod) (reportResult, error) {
	result := reportResult{
		Title:   "應付帳款帳齡分析 (A/P Aging)",
		Period:  map[string]string{"as_of_date": toDateString(period.End)},
		Summary: map[string]float64{},
		Rows:    []map[string]interface{}{},
	}

	type apRow struct {
		Vendor      string    `json:"vendor"`
		ExpenseDate time.Time `json:"expense_date"`
		Amount      float64   `json:"amount"`
		Description string    `json:"description"`
	}

	var rows []apRow
	if err := database.DB.Raw(`
		SELECT COALESCE(vendor, '') AS vendor, expense_date, amount, COALESCE(description, '') AS description
		FROM expenses
		WHERE tenant_id = ?
		  AND status = 'pending'
		  AND expense_date <= ?
		ORDER BY expense_date ASC
	`, tenantID, period.End).Scan(&rows).Error; err != nil {
		return result, err
	}

	buckets := map[string]float64{"current": 0, "1_30": 0, "31_60": 0, "61_90": 0, "90_plus": 0}
	var totalPayable float64

	for _, r := range rows {
		days := int(period.End.Sub(r.ExpenseDate).Hours() / 24)
		if days < 0 {
			days = 0
		}
		bucket := "current"
		switch {
		case days == 0:
			bucket = "current"
		case days <= 30:
			bucket = "1_30"
		case days <= 60:
			bucket = "31_60"
		case days <= 90:
			bucket = "61_90"
		default:
			bucket = "90_plus"
		}

		buckets[bucket] += r.Amount
		totalPayable += r.Amount

		result.Rows = append(result.Rows, map[string]interface{}{
			"vendor":           r.Vendor,
			"expense_date":     toDateString(r.ExpenseDate),
			"description":      r.Description,
			"days_outstanding": days,
			"bucket":           bucket,
			"amount":           r.Amount,
		})
	}

	result.Summary["total_payable"] = totalPayable
	result.Summary["bucket_current"] = buckets["current"]
	result.Summary["bucket_1_30"] = buckets["1_30"]
	result.Summary["bucket_31_60"] = buckets["31_60"]
	result.Summary["bucket_61_90"] = buckets["61_90"]
	result.Summary["bucket_90_plus"] = buckets["90_plus"]

	return result, nil
}

func buildTaxSummaryReport(tenantID uuid.UUID, period reportPeriod) (reportResult, error) {
	result := reportResult{
		Title:   "稅務摘要報告 (Tax Summary)",
		Period:  map[string]string{"start_date": toDateString(period.Start), "end_date": toDateString(period.End)},
		Summary: map[string]float64{},
		Rows:    []map[string]interface{}{},
	}

	outputTax := 0.0
	inputTax := 0.0
	if hasPostedJournalEntries(tenantID, period.Start, period.End) {
		_ = database.DB.Raw(`
			SELECT COALESCE(SUM(l.credit_amount - l.debit_amount), 0)
			FROM journal_entry_lines l
			JOIN journal_entries e ON e.id = l.journal_entry_id
			JOIN accounts a ON a.id = l.account_id
			WHERE l.tenant_id = ?
			  AND e.status = 'posted'
			  AND e.entry_date >= ?
			  AND e.entry_date <= ?
			  AND (a.code = '2130' OR a.sub_type = 'tax_payable')
		`, tenantID, period.Start, period.End).Scan(&outputTax).Error

		_ = database.DB.Raw(`
			SELECT COALESCE(SUM(l.debit_amount - l.credit_amount), 0)
			FROM journal_entry_lines l
			JOIN journal_entries e ON e.id = l.journal_entry_id
			JOIN accounts a ON a.id = l.account_id
			WHERE l.tenant_id = ?
			  AND e.status = 'posted'
			  AND e.entry_date >= ?
			  AND e.entry_date <= ?
			  AND (a.code = '1150' OR a.name LIKE '%%進項稅%%')
		`, tenantID, period.Start, period.End).Scan(&inputTax).Error
	} else {
		_ = database.DB.Raw(`
			SELECT COALESCE(SUM(tax_amount), 0)
			FROM invoices
			WHERE tenant_id = ?
			  AND invoice_date >= ?
			  AND invoice_date <= ?
			  AND status <> 'draft'
		`, tenantID, period.Start, period.End).Scan(&outputTax).Error
		_ = database.DB.Raw(`
			SELECT COALESCE(SUM(amount), 0)
			FROM expenses
			WHERE tenant_id = ?
			  AND category IN ('product_tax', 'service_tax')
			  AND status = 'confirmed'
			  AND expense_date >= ?
			  AND expense_date <= ?
		`, tenantID, period.Start, period.End).Scan(&inputTax).Error
	}

	netTaxPayable := outputTax - inputTax

	var taxConfigs []models.TaxConfig
	_ = database.DB.Where("tenant_id = ? AND is_active = ?", tenantID, true).Order("tax_type ASC, name ASC").Find(&taxConfigs).Error
	for _, tc := range taxConfigs {
		result.Rows = append(result.Rows, map[string]interface{}{
			"name":       tc.Name,
			"code":       tc.Code,
			"region":     tc.Region,
			"tax_type":   tc.TaxType,
			"rate":       tc.Rate,
			"is_default": tc.IsDefault,
		})
	}

	result.Summary["output_tax"] = outputTax
	result.Summary["input_tax"] = inputTax
	result.Summary["net_tax_payable"] = netTaxPayable
	if hasPostedJournalEntries(tenantID, period.Start, period.End) {
		result.Summary["data_source_journal"] = 1
	} else {
		result.Summary["data_source_journal"] = 0
	}

	return result, nil
}

func getReportByType(reportType string, tenantID uuid.UUID, period reportPeriod) (reportResult, error) {
	switch reportType {
	case "profit-loss", "pl":
		return buildProfitLossReport(tenantID, period)
	case "balance-sheet", "bs":
		return buildBalanceSheetReport(tenantID, period)
	case "cash-flow", "cf":
		return buildCashFlowReport(tenantID, period)
	case "ar-aging", "ar":
		return buildARAgingReport(tenantID, period)
	case "ap-aging", "ap":
		return buildAPAgingReport(tenantID, period)
	case "tax-summary", "tax":
		return buildTaxSummaryReport(tenantID, period)
	default:
		return reportResult{}, fmt.Errorf("unsupported report type")
	}
}

func GetAccountingReport(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	reportType := strings.TrimSpace(c.Params("type"))
	period := parseReportPeriod(c, tenantID)

	report, err := getReportByType(reportType, tenantID, period)
	if err != nil {
		if err.Error() == "unsupported report type" {
			return c.Status(400).JSON(fiber.Map{"error": "Unsupported report type"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate report"})
	}

	return c.JSON(report)
}

func ExportAccountingReportExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	reportType := strings.TrimSpace(c.Params("type"))
	period := parseReportPeriod(c, tenantID)

	report, err := getReportByType(reportType, tenantID, period)
	if err != nil {
		if err.Error() == "unsupported report type" {
			return c.Status(400).JSON(fiber.Map{"error": "Unsupported report type"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate report"})
	}

	f := excelize.NewFile()
	sheet := "Report"
	f.SetSheetName("Sheet1", sheet)

	_ = f.SetCellValue(sheet, "A1", report.Title)
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf("Period: %s ~ %s", report.Period["start_date"], report.Period["end_date"]))

	row := 4
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Summary Item")
	_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Amount")
	row++

	keys := make([]string, 0, len(report.Summary))
	for k := range report.Summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), k)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), report.Summary[k])
		row++
	}

	row += 1
	_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Details")
	row++

	headers := []string{}
	if len(report.Rows) > 0 {
		for k := range report.Rows[0] {
			headers = append(headers, k)
		}
		sort.Strings(headers)
	}

	for i, h := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, row), h)
	}
	row++

	for _, data := range report.Rows {
		for i, h := range headers {
			col, _ := excelize.ColumnNumberToName(i + 1)
			_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", col, row), data[h])
		}
		row++
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to export Excel"})
	}

	filename := fmt.Sprintf("%s_%s.xlsx", reportType, time.Now().Format("20060102150405"))
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	return c.Send(buf.Bytes())
}

func ExportAccountingReportPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	reportType := strings.TrimSpace(c.Params("type"))
	period := parseReportPeriod(c, tenantID)

	report, err := getReportByType(reportType, tenantID, period)
	if err != nil {
		if err.Error() == "unsupported report type" {
			return c.Status(400).JSON(fiber.Map{"error": "Unsupported report type"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate report"})
	}

	headers := []string{"Item", "Value"}
	rows := make([][]string, 0)

	keys := make([]string, 0, len(report.Summary))
	for k := range report.Summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rows = append(rows, []string{k, fmt.Sprintf("%.2f", report.Summary[k])})
	}

	if len(report.Rows) > 0 {
		rows = append(rows, []string{"", ""})
		rows = append(rows, []string{"Details", ""})
		for _, r := range report.Rows {
			parts := make([]string, 0)
			for k, v := range r {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
			sort.Strings(parts)
			rows = append(rows, []string{strings.Join(parts, "; "), ""})
		}
	}

	pdfBytes, err := utils.BuildTablePDFBytes(report.Title, headers, rows)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to export PDF"})
	}

	filename := fmt.Sprintf("%s_%s.pdf", reportType, time.Now().Format("20060102150405"))
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	return c.Send(pdfBytes)
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
