package handlers

import (
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/models"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// reserveSpecificNumber inserts into reserved_numbers to prevent re-use later.
// It returns true if inserted; false if already reserved; error for other failures.
func reserveSpecificNumber(tenantID uuid.UUID, fieldName, fieldValue, pageName string) (bool, error) {
	if tenantID == uuid.Nil || fieldName == "" || fieldValue == "" {
		return false, fmt.Errorf("invalid reserve params")
	}
	r := models.ReservedNumber{
		TenantID:   tenantID,
		FieldName:  fieldName,
		FieldValue: fieldValue,
		PageName:   pageName,
		CreatedAt:  time.Now(),
	}
	err := database.DB.Create(&r).Error
	if err == nil {
		return true, nil
	}
	// unique violation => already reserved
	if strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(err.Error(), "duplicate key value") {
		return false, nil
	}
	return false, err
}

func parseSuffixSeq(datePrefix, s string) int {
	if !strings.HasPrefix(s, datePrefix) || len(s) <= len(datePrefix) {
		return 0
	}
	rest := s[len(datePrefix):]
	// allow 001 style
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0
	}
	return n
}

func maxReservedSeq(tenantID uuid.UUID, fieldName, datePrefix string) int {
	var rows []models.ReservedNumber
	database.DB.
		Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?", tenantID, fieldName, datePrefix+"%").
		Select("field_value").
		Find(&rows)
	max := 0
	for _, r := range rows {
		n := parseSuffixSeq(datePrefix, r.FieldValue)
		if n > max {
			max = n
		}
	}
	return max
}

func maxInvoiceSeqUsed(tenantID uuid.UUID, datePrefix string) int {
	max := 0

	// 1) invoices table
	{
		var invoices []models.Invoice
		database.DB.
			Where("tenant_id = ? AND invoice_number LIKE ?", tenantID, datePrefix+"%").
			Select("invoice_number").
			Find(&invoices)
		for _, inv := range invoices {
			n := parseSuffixSeq(datePrefix, inv.InvoiceNumber)
			if n > max {
				max = n
			}
		}
	}

	// 2) orders payment_records
	{
		var orders []models.Order
		database.DB.
			Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").
			Select("id, extra_fields").
			Find(&orders)
		for _, o := range orders {
			if o.ExtraFields == nil {
				continue
			}
			fields := map[string]interface{}(o.ExtraFields)
			if records, ok := fields["payment_records"].([]interface{}); ok {
				for _, rAny := range records {
					r, ok := rAny.(map[string]interface{})
					if !ok {
						continue
					}
					if s, ok := r["invoice_number"].(string); ok {
						n := parseSuffixSeq(datePrefix, s)
						if n > max {
							max = n
						}
					}
				}
			}
		}
	}

	// 3) service_orders payment_records
	{
		var serviceOrders []models.ServiceOrder
		database.DB.
			Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").
			Select("id, extra_fields").
			Find(&serviceOrders)
		for _, so := range serviceOrders {
			if so.ExtraFields == nil {
				continue
			}
			fields := map[string]interface{}(so.ExtraFields)
			if records, ok := fields["payment_records"].([]interface{}); ok {
				for _, rAny := range records {
					r, ok := rAny.(map[string]interface{})
					if !ok {
						continue
					}
					if s, ok := r["invoice_number"].(string); ok {
						n := parseSuffixSeq(datePrefix, s)
						if n > max {
							max = n
						}
					}
				}
			}
		}
	}

	return max
}

func maxExpenseSeqUsed(tenantID uuid.UUID, datePrefix string) int {
	max := 0
	var expenses []models.Expense
	database.DB.
		Where("tenant_id = ? AND (description LIKE ? OR extra_fields::text LIKE ?)", tenantID, "%"+datePrefix+"%", "%"+datePrefix+"%").
		Select("id, description, extra_fields").
		Find(&expenses)
	for _, exp := range expenses {
		if strings.HasPrefix(exp.Description, datePrefix) {
			n := parseSuffixSeq(datePrefix, exp.Description)
			if n > max {
				max = n
			}
		}
		if exp.ExtraFields != nil {
			fields := map[string]interface{}(exp.ExtraFields)
			for _, k := range []string{"expense_number", "invoice_number"} {
				if s, ok := fields[k].(string); ok {
					n := parseSuffixSeq(datePrefix, s)
					if n > max {
						max = n
					}
				}
			}
		}
	}
	return max
}

// reserveNextNumber generates and reserves a tenant-scoped number.
// Supported: invoice_number (INV-YYYYMMDD-###), expense_number (EXP-YYYYMMDD-###).
func reserveNextNumber(tenantID uuid.UUID, fieldName, pageName string) (string, error) {
	today := time.Now().Format("20060102")
	var datePrefix string
	switch fieldName {
	case "invoice_number":
		datePrefix = "INV-" + today + "-"
	case "expense_number":
		datePrefix = "EXP-" + today + "-"
	default:
		return "", fmt.Errorf("unsupported field_name: %s", fieldName)
	}

	maxUsed := 0
	maxRes := maxReservedSeq(tenantID, fieldName, datePrefix)
	if fieldName == "invoice_number" {
		maxUsed = maxInvoiceSeqUsed(tenantID, datePrefix)
	} else {
		maxUsed = maxExpenseSeqUsed(tenantID, datePrefix)
	}
	base := maxUsed
	if maxRes > base {
		base = maxRes
	}

	for offset := 1; offset <= 200; offset++ {
		candidate := fmt.Sprintf("%s%03d", datePrefix, base+offset)
		inserted, err := reserveSpecificNumber(tenantID, fieldName, candidate, pageName)
		if err != nil {
			return "", err
		}
		if inserted {
			return candidate, nil
		}
		// already reserved -> try next
	}

	log.Printf("reserveNextNumber failed: tenant=%s field=%s prefix=%s", tenantID, fieldName, datePrefix)
	return "", fmt.Errorf("failed to reserve number")
}

// codeFormatType defines the numbering format.
type codeFormatType int

const (
	// codeFormatDate uses PREFIX + YYYYMMDD + "-" + 3-digit seq  (e.g. PROD-20260308-001)
	codeFormatDate codeFormatType = iota
	// codeFormatSeq uses PREFIX + 5-digit seq  (e.g. STORE-00001)
	codeFormatSeq
)

// autoCodeConfig describes how to auto-generate a code for a given entity.
type autoCodeConfig struct {
	Prefix     string         // e.g. "PROD-", "CUST-", "STORE-"
	FieldName  string         // reserved_numbers.field_name, typically "code"
	PageName   string         // reserved_numbers.page_name, e.g. "products"
	Format     codeFormatType // date-based or pure sequential
	TableModel interface{}    // GORM model pointer, e.g. &models.Product{}
	Column     string         // column name in table, e.g. "code"
}

// maxCodeSeqUsed scans the actual data table and returns the highest sequence number
// already in use that matches the given prefix.
func maxCodeSeqUsed(tenantID uuid.UUID, cfg autoCodeConfig, searchPrefix string) int {
	max := 0
	var values []string

	database.DB.
		Model(cfg.TableModel).
		Where("tenant_id = ? AND "+cfg.Column+" LIKE ?", tenantID, searchPrefix+"%").
		Pluck(cfg.Column, &values)

	for _, v := range values {
		n := parseSuffixSeq(searchPrefix, v)
		if n > max {
			max = n
		}
	}
	return max
}

// generateAutoCode generates and reserves a unique code for an entity.
// If the code is already provided (e.g. from a form with pre-reserved number), it returns it as-is.
// Otherwise, it auto-generates using the reserved_numbers mechanism to prevent collisions.
func generateAutoCode(tenantID uuid.UUID, existingCode string, cfg autoCodeConfig) (string, error) {
	// If a code is already provided (e.g. from form pre-reservation), use it as-is
	if strings.TrimSpace(existingCode) != "" {
		return existingCode, nil
	}

	today := time.Now().Format("20060102")
	var searchPrefix string
	var fmtFunc func(seq int) string

	switch cfg.Format {
	case codeFormatDate:
		searchPrefix = cfg.Prefix + today + "-"
		fmtFunc = func(seq int) string {
			return fmt.Sprintf("%s%03d", searchPrefix, seq)
		}
	case codeFormatSeq:
		searchPrefix = cfg.Prefix
		fmtFunc = func(seq int) string {
			return fmt.Sprintf("%s%05d", searchPrefix, seq)
		}
	default:
		return "", fmt.Errorf("unsupported code format type")
	}

	maxUsed := maxCodeSeqUsed(tenantID, cfg, searchPrefix)
	maxRes := maxReservedSeq(tenantID, cfg.FieldName, searchPrefix)

	base := maxUsed
	if maxRes > base {
		base = maxRes
	}

	for offset := 1; offset <= 200; offset++ {
		candidate := fmtFunc(base + offset)
		inserted, err := reserveSpecificNumber(tenantID, cfg.FieldName, candidate, cfg.PageName)
		if err != nil {
			return "", err
		}
		if inserted {
			return candidate, nil
		}
		// already reserved -> try next
	}

	log.Printf("generateAutoCode failed: tenant=%s page=%s prefix=%s", tenantID, cfg.PageName, searchPrefix)
	return "", fmt.Errorf("failed to generate unique code")
}

// releaseReservedCode releases a previously reserved code after the entity is successfully created.
// This is safe to call even if the code was not reserved (no-op).
func releaseReservedCode(tenantID uuid.UUID, fieldName, fieldValue string) {
	if fieldValue == "" {
		return
	}
	database.DB.Where("tenant_id = ? AND field_name = ? AND field_value = ?",
		tenantID, fieldName, fieldValue).Delete(&models.ReservedNumber{})
}
