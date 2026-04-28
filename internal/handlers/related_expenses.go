package handlers

import (
	"fmt"
	"log"
	"math"
	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"
	"time"

	"github.com/google/uuid"
)

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func parseUUIDList(v interface{}) []uuid.UUID {
	if v == nil {
		return nil
	}
	out := make([]uuid.UUID, 0)
	switch vv := v.(type) {
	case []interface{}:
		for _, it := range vv {
			switch s := it.(type) {
			case string:
				if id, err := uuid.Parse(s); err == nil {
					out = append(out, id)
				}
			}
		}
	case []string:
		for _, s := range vv {
			if id, err := uuid.Parse(s); err == nil {
				out = append(out, id)
			}
		}
	}
	return out
}

func defaultPaymentMethodCode(tenantID uuid.UUID) string {
	var pm models.PaymentMethod
	// 優先：支出預設
	if err := database.DB.
		Where("tenant_id = ? AND status = ? AND is_default_expense = ?", tenantID, "active", true).
		First(&pm).Error; err == nil {
		return pm.Code
	}
	// fallback：舊版只有 is_default（客戶付款預設）
	if err := database.DB.
		Where("tenant_id = ? AND status = ? AND is_default = ?", tenantID, "active", true).
		First(&pm).Error; err == nil {
		return pm.Code
	}
	// fallback：任何 active
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenantID, "active").
		First(&pm).Error; err == nil {
		return pm.Code
	}
	return ""
}

func defaultPaymentBankAccountID(tenantID uuid.UUID) *uuid.UUID {
	var ba models.BankAccount
	if err := database.DB.
		Where("tenant_id = ? AND status = ? AND is_default_payment = ?", tenantID, "active", true).
		First(&ba).Error; err == nil && ba.ID != uuid.Nil {
		id := ba.ID
		return &id
	}
	// fallback：兼容舊資料 is_default
	if err := database.DB.
		Where("tenant_id = ? AND status = ? AND is_default = ?", tenantID, "active", true).
		First(&ba).Error; err == nil && ba.ID != uuid.Nil {
		id := ba.ID
		return &id
	}
	return nil
}

func defaultReceivingBankAccountID(tenantID uuid.UUID) *uuid.UUID {
	var ba models.BankAccount
	if err := database.DB.
		Where("tenant_id = ? AND status = ? AND is_default_receiving = ?", tenantID, "active", true).
		First(&ba).Error; err == nil && ba.ID != uuid.Nil {
		id := ba.ID
		return &id
	}
	// fallback：兼容舊資料 is_default
	if err := database.DB.
		Where("tenant_id = ? AND status = ? AND is_default = ?", tenantID, "active", true).
		First(&ba).Error; err == nil && ba.ID != uuid.Nil {
		id := ba.ID
		return &id
	}
	return nil
}

func defaultReceivingPaymentMethodCode(tenantID uuid.UUID) string {
	var pm models.PaymentMethod
	// 優先：客戶付款預設（is_default）
	if err := database.DB.
		Where("tenant_id = ? AND status = ? AND is_default = ?", tenantID, "active", true).
		First(&pm).Error; err == nil {
		return pm.Code
	}
	// fallback：任何 active
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenantID, "active").
		First(&pm).Error; err == nil {
		return pm.Code
	}
	return ""
}

type orderTaxBaseRow struct {
	TaxID      uuid.UUID `gorm:"column:tax_id"`
	Name       string    `gorm:"column:name"`
	TaxMode    string    `gorm:"column:tax_mode"`
	TaxValue   float64   `gorm:"column:tax_value"`
	BaseAmount float64   `gorm:"column:base_amount"`
}

// ensureAndSyncOrderRelatedExpenses
// - 若開啟自動生成：會建立缺少的佣金/稅項支出，並同步金額
// - 若未開啟：僅同步「已存在」的支出金額（由外部呼叫 syncOrderRelatedExpenses）
func ensureAndSyncOrderRelatedExpenses(tenantID uuid.UUID, orderID uuid.UUID, userID uuid.UUID, auto models.DocumentAutoSettings) {
	var order models.Order
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, orderID).
		First(&order).Error; err != nil {
		return
	}

	now := utils.NowInTenantTimezone(tenantID)
	defPayMethod := defaultPaymentMethodCode(tenantID)
	defPayAccID := defaultPaymentBankAccountID(tenantID)

	// ===== 佣金支出（訂單） =====
	if auto.AutoGenerateOrderCommission && order.SalespersonID != nil && order.CommissionAmount > 0 {
		var commissionExpense models.Expense
		err := database.DB.Where("tenant_id = ? AND category = ? AND reference_type = ? AND reference_id = ?",
			tenantID, "order_commission", "order", orderID).First(&commissionExpense).Error
		if err != nil {
			// 新建
			var salesperson models.User
			_ = database.DB.Select("id,name").Where("tenant_id = ? AND id = ?", tenantID, *order.SalespersonID).First(&salesperson).Error
			exp := models.Expense{
				TenantID:      tenantID,
				RelatedUserID: order.SalespersonID,
				ExpenseType:   "order_commission",
				Category:      "order_commission",
				ReferenceID:   &orderID,
				ReferenceType: "order",
				Description:   fmt.Sprintf("訂單佣金 - 銷售員: %s", salesperson.Name),
				Amount:        round2(order.CommissionAmount),
				ExpenseDate:   order.OrderDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defPayAccID,
				Status:        "confirmed",
				Vendor:        "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExtraFields: models.JSONB(map[string]interface{}{
					"order_id":        orderID.String(),
					"salesperson_id":  order.SalespersonID.String(),
					"salesperson_name": salesperson.Name,
				}),
			}
			if err := database.DB.Create(&exp).Error; err != nil {
				log.Printf("Failed to create order commission expense: %v", err)
			}
		} else {
			// 更新已存在的佣金支出
			commissionExpense.Amount = round2(order.CommissionAmount)
			commissionExpense.UpdatedAt = now
			commissionExpense.UpdatedBy = &userID
			if commissionExpense.ReferenceID == nil {
				commissionExpense.ReferenceID = &orderID
			}
			if commissionExpense.ReferenceType == "" {
				commissionExpense.ReferenceType = "order"
			}
			if err := database.DB.Save(&commissionExpense).Error; err != nil {
				log.Printf("Failed to update order commission expense: %v", err)
			}
		}
	}

	// ===== 稅項支出 =====
	if !auto.AutoGenerateOrderTaxes {
		return
	}

	// tax ids 來自 order.extra_fields.product_tax_ids
	var taxIDs []uuid.UUID
	if order.ExtraFields != nil {
		ef := map[string]interface{}(order.ExtraFields)
		taxIDs = parseUUIDList(ef["product_tax_ids"])
	}
	if len(taxIDs) == 0 {
		return
	}

	// 載入已生成的 tax 支出（每個 tax_id 一筆）
	var existingTaxExpenses []models.Expense
	_ = database.DB.Where("tenant_id = ? AND category = ? AND reference_type = ? AND reference_id = ?",
		tenantID, "product_tax", "order", orderID).Find(&existingTaxExpenses).Error
	existingByTaxID := make(map[string]*models.Expense)
	for i := range existingTaxExpenses {
		e := &existingTaxExpenses[i]
		if e.ExtraFields == nil {
			continue
		}
		fields := map[string]interface{}(e.ExtraFields)
		// 支持多種格式：string UUID、uuid.UUID、或其他可轉換格式
		var tidStr string
		if tid, ok := fields["tax_id"].(string); ok && tid != "" {
			tidStr = tid
		} else if tidUUID, ok := fields["tax_id"].(uuid.UUID); ok {
			tidStr = tidUUID.String()
		} else if tidAny, ok := fields["tax_id"]; ok {
			// 嘗試轉換為字串
			tidStr = fmt.Sprintf("%v", tidAny)
		}
		if tidStr != "" {
			// 標準化為小寫 UUID 字串格式（去除連字符）
			if parsedUUID, err := uuid.Parse(tidStr); err == nil {
				existingByTaxID[parsedUUID.String()] = e
			} else {
				// 如果無法解析，直接使用原始字串
				existingByTaxID[tidStr] = e
			}
		}
	}

	// 計算折扣比例（只分攤 coupon / points，與前端一致）
	subtotal := order.TotalAmount
	actual := subtotal - order.CouponDiscount - order.PointsDiscount
	if actual < 0 {
		actual = 0
	}
	ratio := 0.0
	if subtotal > 0 {
		ratio = actual / subtotal
	}

	// 查詢每個稅的 base（只包含該稅關聯到的產品）
	var rows []orderTaxBaseRow
	err := database.DB.Raw(`
		SELECT
			t.id AS tax_id,
			t.name AS name,
			t.tax_mode AS tax_mode,
			t.tax_value AS tax_value,
			COALESCE(SUM(oi.total_price), 0) AS base_amount
		FROM order_items oi
		JOIN product_tax_relations ptr ON ptr.product_id = oi.product_id
		JOIN product_taxes t ON t.id = ptr.tax_id
		WHERE oi.tenant_id = ? AND oi.order_id = ? AND t.id IN ?
		GROUP BY t.id, t.name, t.tax_mode, t.tax_value
	`, tenantID, orderID, taxIDs).Scan(&rows).Error
	if err != nil {
		return
	}

	// rows 只會包含「有綁 product_tax_relations」的稅；default_include / 全單稅會漏掉
	seenTax := make(map[string]bool)
	for _, r := range rows {
		seenTax[r.TaxID.String()] = true
	}

	for _, r := range rows {
		baseAdj := r.BaseAmount * ratio
		amt := 0.0
		if r.TaxMode == "fixed" {
			if r.BaseAmount > 0 {
				amt = r.TaxValue
			}
		} else {
			amt = baseAdj * (r.TaxValue / 100.0)
		}
		amt = round2(amt)

		ef := map[string]interface{}{
			"order_id":        orderID.String(),
			"tax_id":          r.TaxID.String(),
			"tax_name":        r.Name,
			"tax_mode":        r.TaxMode,
			"tax_value":       r.TaxValue,
			"base_amount":     round2(r.BaseAmount),
			"discount_ratio":  ratio,
		}

		ex := existingByTaxID[r.TaxID.String()]
		if ex == nil {
			// 建立新支出
			newExp := models.Expense{
				TenantID:      tenantID,
				ExpenseType:   "product_tax",
				Category:      "product_tax",
				ReferenceID:   &orderID,
				ReferenceType: "order",
				Description:   fmt.Sprintf("訂單稅項 - %s", r.Name),
				Amount:        amt,
				ExpenseDate:   order.OrderDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defPayAccID,
				Status:        "confirmed",
				Vendor:        "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExtraFields:   models.JSONB(ef),
			}
			if err := database.DB.Create(&newExp).Error; err != nil {
				log.Printf("Failed to create order tax expense: %v", err)
			} else {
				existingByTaxID[r.TaxID.String()] = &newExp
			}
			continue
		}

		// 更新既有
		ex.Amount = amt
		ex.Description = fmt.Sprintf("訂單稅項 - %s", r.Name)
		ex.UpdatedAt = now
		if ex.ReferenceID == nil {
			ex.ReferenceID = &orderID
		}
		if ex.ReferenceType == "" {
			ex.ReferenceType = "order"
		}
		ex.ExtraFields = models.JSONB(ef)
		if err := database.DB.Save(ex).Error; err != nil {
			log.Printf("Failed to update order tax expense: %v", err)
		}
	}

	// 追加：沒有 product_tax_relations 的稅（使用 order.TotalAmount 作為 base，仍然套用 ratio）
	missing := make([]uuid.UUID, 0)
	for _, tid := range taxIDs {
		if !seenTax[tid.String()] {
			missing = append(missing, tid)
		}
	}
	if len(missing) > 0 {
		var taxes []models.ProductTax
		_ = database.DB.Where("tenant_id = ? AND id IN ?", tenantID, missing).Find(&taxes).Error
		for _, t := range taxes {
			base := subtotal
			baseAdj := base * ratio
			amt := 0.0
			if t.TaxMode == "fixed" {
				if base > 0 {
					amt = t.TaxValue
				}
			} else {
				amt = baseAdj * (t.TaxValue / 100.0)
			}
			amt = round2(amt)

			ef := map[string]interface{}{
				"order_id":        orderID.String(),
				"tax_id":          t.ID.String(),
				"tax_name":        t.Name,
				"tax_mode":        t.TaxMode,
				"tax_value":       t.TaxValue,
				"base_amount":     round2(base),
				"discount_ratio":  ratio,
			}

			ex := existingByTaxID[t.ID.String()]
			if ex == nil {
				newExp := models.Expense{
					TenantID:      tenantID,
					ExpenseType:   "product_tax",
					Category:      "product_tax",
					ReferenceID:   &orderID,
					ReferenceType: "order",
					Description:   fmt.Sprintf("訂單稅項 - %s", t.Name),
					Amount:        amt,
					ExpenseDate:   order.OrderDate,
					PaymentMethod: defPayMethod,
					BankAccountID: defPayAccID,
					Status:        "confirmed",
					Vendor:        "",
					CreatedBy:     &userID,
					UpdatedBy:     &userID,
					CreatedAt:     now,
					UpdatedAt:     now,
					ExtraFields:   models.JSONB(ef),
				}
				if err := database.DB.Create(&newExp).Error; err != nil {
					log.Printf("Failed to create order tax expense (missing relation): %v", err)
				}
				continue
			}

			ex.Amount = amt
			ex.Description = fmt.Sprintf("訂單稅項 - %s", t.Name)
			ex.UpdatedAt = now
			ex.ExtraFields = models.JSONB(ef)
			if ex.PaymentMethod == "" && defPayMethod != "" {
				ex.PaymentMethod = defPayMethod
			}
			if ex.BankAccountID == nil && defPayAccID != nil {
				ex.BankAccountID = defPayAccID
			}
			if err := database.DB.Save(ex).Error; err != nil {
				log.Printf("Failed to update order tax expense (missing relation): %v", err)
			}
		}
	}
}

func syncOrderRelatedExpenses(tenantID uuid.UUID, orderID uuid.UUID) {
	// best-effort: 不要因為同步支出失敗而影響訂單更新
	var order models.Order
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, orderID).
		First(&order).Error; err != nil {
		return
	}

	// 佣金支出（若已生成則更新）
	var commissionExpense models.Expense
	if err := database.DB.Where("tenant_id = ? AND category = ? AND reference_type = ? AND reference_id = ?",
		tenantID, "order_commission", "order", orderID).First(&commissionExpense).Error; err == nil {
		if order.CommissionAmount != commissionExpense.Amount {
			commissionExpense.Amount = round2(order.CommissionAmount)
			commissionExpense.UpdatedAt = time.Now()
			if err := database.DB.Save(&commissionExpense).Error; err != nil {
				log.Printf("Failed to update order commission expense: %v", err)
			}
		}
	}

	// tax ids 來自 order.extra_fields.product_tax_ids
	var taxIDs []uuid.UUID
	if order.ExtraFields != nil {
		ef := map[string]interface{}(order.ExtraFields)
		taxIDs = parseUUIDList(ef["product_tax_ids"])
	}
	if len(taxIDs) == 0 {
		return
	}

	// 先載入已生成的 tax 支出（每個 tax_id 一筆）
	var existingTaxExpenses []models.Expense
	_ = database.DB.Where("tenant_id = ? AND category = ? AND reference_type = ? AND reference_id = ?",
		tenantID, "product_tax", "order", orderID).Find(&existingTaxExpenses).Error

	existingByTaxID := make(map[string]*models.Expense)
	for i := range existingTaxExpenses {
		e := &existingTaxExpenses[i]
		if e.ExtraFields == nil {
			continue
		}
		fields := map[string]interface{}(e.ExtraFields)
		// 支持多種格式：string UUID、uuid.UUID、或其他可轉換格式
		var tidStr string
		if tid, ok := fields["tax_id"].(string); ok && tid != "" {
			tidStr = tid
		} else if tidUUID, ok := fields["tax_id"].(uuid.UUID); ok {
			tidStr = tidUUID.String()
		} else if tidAny, ok := fields["tax_id"]; ok {
			// 嘗試轉換為字串
			tidStr = fmt.Sprintf("%v", tidAny)
		}
		if tidStr != "" {
			// 標準化為小寫 UUID 字串格式（去除連字符）
			if parsedUUID, err := uuid.Parse(tidStr); err == nil {
				existingByTaxID[parsedUUID.String()] = e
			} else {
				// 如果無法解析，直接使用原始字串
				existingByTaxID[tidStr] = e
			}
		}
	}

	// 計算折扣比例（只分攤 coupon / points，與前端一致）
	subtotal := order.TotalAmount
	actual := subtotal - order.CouponDiscount - order.PointsDiscount
	if actual < 0 {
		actual = 0
	}
	ratio := 0.0
	if subtotal > 0 {
		ratio = actual / subtotal
	}

	// 查詢每個稅的 base（只包含該稅關聯到的產品）
	var rows []orderTaxBaseRow
	err := database.DB.Raw(`
		SELECT
			t.id AS tax_id,
			t.name AS name,
			t.tax_mode AS tax_mode,
			t.tax_value AS tax_value,
			COALESCE(SUM(oi.total_price), 0) AS base_amount
		FROM order_items oi
		JOIN product_tax_relations ptr ON ptr.product_id = oi.product_id
		JOIN product_taxes t ON t.id = ptr.tax_id
		WHERE oi.tenant_id = ? AND oi.order_id = ? AND t.id IN ?
		GROUP BY t.id, t.name, t.tax_mode, t.tax_value
	`, tenantID, orderID, taxIDs).Scan(&rows).Error
	if err != nil {
		return
	}

	now := time.Now()
	seenTax := make(map[string]bool)
	for _, r := range rows {
		seenTax[r.TaxID.String()] = true
	}
	for _, r := range rows {
		baseAdj := r.BaseAmount * ratio
		amt := 0.0
		if r.TaxMode == "fixed" {
			if r.BaseAmount > 0 {
				amt = r.TaxValue
			}
		} else {
			amt = baseAdj * (r.TaxValue / 100.0)
		}
		amt = round2(amt)

		// 只更新已存在支出
		ex := existingByTaxID[r.TaxID.String()]
		if ex == nil {
			continue
		}

		ex.Amount = amt
		ex.Description = fmt.Sprintf("訂單稅項 - %s", r.Name)
		ex.UpdatedAt = now
		if ex.ReferenceID == nil {
			ex.ReferenceID = &orderID
		}
		if ex.ReferenceType == "" {
			ex.ReferenceType = "order"
		}

		// 更新 extra_fields 快照（方便前端顯示）
		ef := map[string]interface{}{}
		if ex.ExtraFields != nil {
			ef = map[string]interface{}(ex.ExtraFields)
		}
		ef["order_id"] = orderID.String()
		ef["tax_id"] = r.TaxID.String()
		ef["tax_name"] = r.Name
		ef["tax_mode"] = r.TaxMode
		ef["tax_value"] = r.TaxValue
		ef["base_amount"] = round2(r.BaseAmount)
		ef["discount_ratio"] = ratio
		ex.ExtraFields = models.JSONB(ef)

		if err := database.DB.Save(ex).Error; err != nil {
			log.Printf("Failed to save expense: %v", err)
		}
	}

	// 追加：沒有 product_tax_relations 的稅，只更新已存在支出
	missing := make([]uuid.UUID, 0)
	for _, tid := range taxIDs {
		if !seenTax[tid.String()] {
			missing = append(missing, tid)
		}
	}
	if len(missing) > 0 {
		var taxes []models.ProductTax
		_ = database.DB.Where("tenant_id = ? AND id IN ?", tenantID, missing).Find(&taxes).Error
		for _, t := range taxes {
			ex := existingByTaxID[t.ID.String()]
			if ex == nil {
				continue
			}
			base := subtotal
			baseAdj := base * ratio
			amt := 0.0
			if t.TaxMode == "fixed" {
				if base > 0 {
					amt = t.TaxValue
				}
			} else {
				amt = baseAdj * (t.TaxValue / 100.0)
			}
			amt = round2(amt)

			ef := map[string]interface{}{}
			if ex.ExtraFields != nil {
				ef = map[string]interface{}(ex.ExtraFields)
			}
			ef["order_id"] = orderID.String()
			ef["tax_id"] = t.ID.String()
			ef["tax_name"] = t.Name
			ef["tax_mode"] = t.TaxMode
			ef["tax_value"] = t.TaxValue
			ef["base_amount"] = round2(base)
			ef["discount_ratio"] = ratio

			ex.Amount = amt
			ex.Description = fmt.Sprintf("訂單稅項 - %s", t.Name)
			ex.UpdatedAt = now
			ex.ExtraFields = models.JSONB(ef)
			if err := database.DB.Save(ex).Error; err != nil {
			log.Printf("Failed to save expense: %v", err)
		}
		}
	}
}

type serviceOrderTaxBaseRow struct {
	TaxID      uuid.UUID `gorm:"column:tax_id"`
	Name       string    `gorm:"column:name"`
	TaxMode    string    `gorm:"column:tax_mode"`
	TaxValue   float64   `gorm:"column:tax_value"`
	BaseAmount float64   `gorm:"column:base_amount"`
}

func ensureAndSyncServiceOrderRelatedExpenses(tenantID uuid.UUID, serviceOrderID uuid.UUID, userID uuid.UUID, auto models.DocumentAutoSettings) {
	var so models.ServiceOrder
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, serviceOrderID).
		First(&so).Error; err != nil {
		return
	}

	now := utils.NowInTenantTimezone(tenantID)
	defPayMethod := defaultPaymentMethodCode(tenantID)
	defPayAccID := defaultPaymentBankAccountID(tenantID)

	// 佣金支出（服務單）
	if auto.AutoGenerateServiceOrderCommission && so.SalespersonID != nil && so.CommissionAmount > 0 {
		var commissionExpense models.Expense
		err := database.DB.Where("tenant_id = ? AND category = ? AND reference_type = ? AND reference_id = ?",
			tenantID, "service_order_commission", "service_order", serviceOrderID).First(&commissionExpense).Error
		if err != nil {
			var salesperson models.User
			_ = database.DB.Select("id,name").Where("tenant_id = ? AND id = ?", tenantID, *so.SalespersonID).First(&salesperson).Error
			exp := models.Expense{
				TenantID:      tenantID,
				RelatedUserID: so.SalespersonID,
				ExpenseType:   "service_order_commission",
				Category:      "service_order_commission",
				ReferenceID:   &serviceOrderID,
				ReferenceType: "service_order",
				Description:   fmt.Sprintf("服務單佣金 - 銷售員: %s", salesperson.Name),
				Amount:        round2(so.CommissionAmount),
				ExpenseDate:   so.ServiceDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defPayAccID,
				Status:        "confirmed",
				Vendor:        "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExtraFields: models.JSONB(map[string]interface{}{
					"service_order_id":  serviceOrderID.String(),
					"salesperson_id":    so.SalespersonID.String(),
					"salesperson_name":  salesperson.Name,
				}),
			}
			if err := database.DB.Create(&exp).Error; err != nil {
				log.Printf("Failed to create order commission expense: %v", err)
			}
		} else {
			// 更新已存在的佣金支出
			commissionExpense.Amount = round2(so.CommissionAmount)
			commissionExpense.UpdatedAt = now
			commissionExpense.UpdatedBy = &userID
			if commissionExpense.ReferenceID == nil {
				commissionExpense.ReferenceID = &serviceOrderID
			}
			if commissionExpense.ReferenceType == "" {
				commissionExpense.ReferenceType = "service_order"
			}
			if err := database.DB.Save(&commissionExpense).Error; err != nil {
				log.Printf("Failed to update order commission expense: %v", err)
			}
		}
	}

	if !auto.AutoGenerateServiceTaxes {
		return
	}

	var taxIDs []uuid.UUID
	if so.ExtraFields != nil {
		ef := map[string]interface{}(so.ExtraFields)
		taxIDs = parseUUIDList(ef["service_tax_ids"])
	}
	if len(taxIDs) == 0 {
		return
	}

	var existingTaxExpenses []models.Expense
	_ = database.DB.Where("tenant_id = ? AND category = ? AND reference_type = ? AND reference_id = ?",
		tenantID, "service_tax", "service_order", serviceOrderID).Find(&existingTaxExpenses).Error
	existingByTaxID := make(map[string]*models.Expense)
	for i := range existingTaxExpenses {
		e := &existingTaxExpenses[i]
		if e.ExtraFields == nil {
			continue
		}
		fields := map[string]interface{}(e.ExtraFields)
		// 支持多種格式：string UUID、uuid.UUID、或其他可轉換格式
		var tidStr string
		if tid, ok := fields["tax_id"].(string); ok && tid != "" {
			tidStr = tid
		} else if tidUUID, ok := fields["tax_id"].(uuid.UUID); ok {
			tidStr = tidUUID.String()
		} else if tidAny, ok := fields["tax_id"]; ok {
			// 嘗試轉換為字串
			tidStr = fmt.Sprintf("%v", tidAny)
		}
		if tidStr != "" {
			// 標準化為小寫 UUID 字串格式（去除連字符）
			if parsedUUID, err := uuid.Parse(tidStr); err == nil {
				existingByTaxID[parsedUUID.String()] = e
			} else {
				// 如果無法解析，直接使用原始字串
				existingByTaxID[tidStr] = e
			}
		}
	}

	subtotal := so.TotalAmount
	actual := subtotal - so.CouponDiscount - so.PointsDiscount
	if actual < 0 {
		actual = 0
	}
	ratio := 0.0
	if subtotal > 0 {
		ratio = actual / subtotal
	}

	var rows []serviceOrderTaxBaseRow
	err := database.DB.Raw(`
		SELECT
			t.id AS tax_id,
			t.name AS name,
			t.tax_mode AS tax_mode,
			t.tax_value AS tax_value,
			COALESCE(SUM(soi.total_price), 0) AS base_amount
		FROM service_order_items soi
		JOIN service_tax_relations str ON str.service_id = soi.service_id
		JOIN service_taxes t ON t.id = str.tax_id
		WHERE soi.tenant_id = ? AND soi.service_order_id = ? AND t.id IN ?
		GROUP BY t.id, t.name, t.tax_mode, t.tax_value
	`, tenantID, serviceOrderID, taxIDs).Scan(&rows).Error
	if err != nil {
		return
	}

	// rows 只會包含「有綁 service_tax_relations」的稅；default_include / 全單稅會漏掉
	seenTax := make(map[string]bool)
	for _, r := range rows {
		seenTax[r.TaxID.String()] = true
	}

	for _, r := range rows {
		baseAdj := r.BaseAmount * ratio
		amt := 0.0
		if r.TaxMode == "fixed" {
			if r.BaseAmount > 0 {
				amt = r.TaxValue
			}
		} else {
			amt = baseAdj * (r.TaxValue / 100.0)
		}
		amt = round2(amt)

		ef := map[string]interface{}{
			"service_order_id":  serviceOrderID.String(),
			"tax_id":            r.TaxID.String(),
			"tax_name":          r.Name,
			"tax_mode":          r.TaxMode,
			"tax_value":         r.TaxValue,
			"base_amount":       round2(r.BaseAmount),
			"discount_ratio":    ratio,
		}

		ex := existingByTaxID[r.TaxID.String()]
		if ex == nil {
			newExp := models.Expense{
				TenantID:      tenantID,
				ExpenseType:   "service_tax",
				Category:      "service_tax",
				ReferenceID:   &serviceOrderID,
				ReferenceType: "service_order",
				Description:   fmt.Sprintf("服務單稅項 - %s", r.Name),
				Amount:        amt,
				ExpenseDate:   so.ServiceDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defPayAccID,
				Status:        "confirmed",
				Vendor:        "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExtraFields:   models.JSONB(ef),
			}
			if err := database.DB.Create(&newExp).Error; err != nil {
				log.Printf("Failed to create order tax expense: %v", err)
			} else {
				existingByTaxID[r.TaxID.String()] = &newExp
			}
			continue
		}

		ex.Amount = amt
		ex.Description = fmt.Sprintf("服務單稅項 - %s", r.Name)
		ex.UpdatedAt = now
		if ex.ReferenceID == nil {
			ex.ReferenceID = &serviceOrderID
		}
		if ex.ReferenceType == "" {
			ex.ReferenceType = "service_order"
		}
		ex.ExtraFields = models.JSONB(ef)
		if err := database.DB.Save(ex).Error; err != nil {
			log.Printf("Failed to update order tax expense: %v", err)
		}
	}

	// 追加：沒有 service_tax_relations 的稅（使用 so.TotalAmount 作為 base，仍然套用 ratio）
	missing := make([]uuid.UUID, 0)
	for _, tid := range taxIDs {
		if !seenTax[tid.String()] {
			missing = append(missing, tid)
		}
	}
	if len(missing) > 0 {
		var taxes []models.ServiceTax
		_ = database.DB.Where("tenant_id = ? AND id IN ?", tenantID, missing).Find(&taxes).Error
		for _, t := range taxes {
			base := subtotal
			baseAdj := base * ratio
			amt := 0.0
			if t.TaxMode == "fixed" {
				if base > 0 {
					amt = t.TaxValue
				}
			} else {
				amt = baseAdj * (t.TaxValue / 100.0)
			}
			amt = round2(amt)

			ef := map[string]interface{}{
				"service_order_id":  serviceOrderID.String(),
				"tax_id":            t.ID.String(),
				"tax_name":          t.Name,
				"tax_mode":          t.TaxMode,
				"tax_value":         t.TaxValue,
				"base_amount":       round2(base),
				"discount_ratio":    ratio,
			}

			ex := existingByTaxID[t.ID.String()]
			if ex == nil {
				newExp := models.Expense{
					TenantID:      tenantID,
					ExpenseType:   "service_tax",
					Category:      "service_tax",
					ReferenceID:   &serviceOrderID,
					ReferenceType: "service_order",
					Description:   fmt.Sprintf("服務單稅項 - %s", t.Name),
					Amount:        amt,
					ExpenseDate:   so.ServiceDate,
					PaymentMethod: defPayMethod,
					BankAccountID: defPayAccID,
					Status:        "confirmed",
					Vendor:        "",
					CreatedBy:     &userID,
					UpdatedBy:     &userID,
					CreatedAt:     now,
					UpdatedAt:     now,
					ExtraFields:   models.JSONB(ef),
				}
				if err := database.DB.Create(&newExp).Error; err != nil {
					log.Printf("Failed to create service order tax expense (missing relation): %v", err)
				}
				continue
			}

			ex.Amount = amt
			ex.Description = fmt.Sprintf("服務單稅項 - %s", t.Name)
			ex.UpdatedAt = now
			ex.ExtraFields = models.JSONB(ef)
			if ex.PaymentMethod == "" && defPayMethod != "" {
				ex.PaymentMethod = defPayMethod
			}
			if ex.BankAccountID == nil && defPayAccID != nil {
				ex.BankAccountID = defPayAccID
			}
			if err := database.DB.Save(ex).Error; err != nil {
			log.Printf("Failed to save expense: %v", err)
		}
		}
	}
}

func syncServiceOrderRelatedExpenses(tenantID uuid.UUID, serviceOrderID uuid.UUID) {
	var so models.ServiceOrder
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, serviceOrderID).
		First(&so).Error; err != nil {
		return
	}

	// 佣金支出（若已生成則更新）
	var commissionExpense models.Expense
	if err := database.DB.Where("tenant_id = ? AND category = ? AND reference_type = ? AND reference_id = ?",
		tenantID, "service_order_commission", "service_order", serviceOrderID).First(&commissionExpense).Error; err == nil {
		if so.CommissionAmount != commissionExpense.Amount {
			commissionExpense.Amount = round2(so.CommissionAmount)
			commissionExpense.UpdatedAt = time.Now()
			if err := database.DB.Save(&commissionExpense).Error; err != nil {
				log.Printf("Failed to update order commission expense: %v", err)
			}
		}
	}

	var taxIDs []uuid.UUID
	if so.ExtraFields != nil {
		ef := map[string]interface{}(so.ExtraFields)
		taxIDs = parseUUIDList(ef["service_tax_ids"])
	}
	if len(taxIDs) == 0 {
		return
	}

	// 已生成 tax 支出
	var existingTaxExpenses []models.Expense
	_ = database.DB.Where("tenant_id = ? AND category = ? AND reference_type = ? AND reference_id = ?",
		tenantID, "service_tax", "service_order", serviceOrderID).Find(&existingTaxExpenses).Error
	existingByTaxID := make(map[string]*models.Expense)
	for i := range existingTaxExpenses {
		e := &existingTaxExpenses[i]
		if e.ExtraFields == nil {
			continue
		}
		fields := map[string]interface{}(e.ExtraFields)
		// 支持多種格式：string UUID、uuid.UUID、或其他可轉換格式
		var tidStr string
		if tid, ok := fields["tax_id"].(string); ok && tid != "" {
			tidStr = tid
		} else if tidUUID, ok := fields["tax_id"].(uuid.UUID); ok {
			tidStr = tidUUID.String()
		} else if tidAny, ok := fields["tax_id"]; ok {
			// 嘗試轉換為字串
			tidStr = fmt.Sprintf("%v", tidAny)
		}
		if tidStr != "" {
			// 標準化為小寫 UUID 字串格式（去除連字符）
			if parsedUUID, err := uuid.Parse(tidStr); err == nil {
				existingByTaxID[parsedUUID.String()] = e
			} else {
				// 如果無法解析，直接使用原始字串
				existingByTaxID[tidStr] = e
			}
		}
	}

	// 服務單折扣：同訂單，僅分攤 coupon/points
	subtotal := so.TotalAmount
	actual := subtotal - so.CouponDiscount - so.PointsDiscount
	if actual < 0 {
		actual = 0
	}
	ratio := 0.0
	if subtotal > 0 {
		ratio = actual / subtotal
	}

	var rows []serviceOrderTaxBaseRow
	err := database.DB.Raw(`
		SELECT
			t.id AS tax_id,
			t.name AS name,
			t.tax_mode AS tax_mode,
			t.tax_value AS tax_value,
			COALESCE(SUM(soi.total_price), 0) AS base_amount
		FROM service_order_items soi
		JOIN service_tax_relations str ON str.service_id = soi.service_id
		JOIN service_taxes t ON t.id = str.tax_id
		WHERE soi.tenant_id = ? AND soi.service_order_id = ? AND t.id IN ?
		GROUP BY t.id, t.name, t.tax_mode, t.tax_value
	`, tenantID, serviceOrderID, taxIDs).Scan(&rows).Error
	if err != nil {
		return
	}

	now := time.Now()
	seenTax := make(map[string]bool)
	for _, r := range rows {
		seenTax[r.TaxID.String()] = true
	}
	for _, r := range rows {
		baseAdj := r.BaseAmount * ratio
		amt := 0.0
		if r.TaxMode == "fixed" {
			if r.BaseAmount > 0 {
				amt = r.TaxValue
			}
		} else {
			amt = baseAdj * (r.TaxValue / 100.0)
		}
		amt = round2(amt)

		ex := existingByTaxID[r.TaxID.String()]
		if ex == nil {
			continue
		}

		ex.Amount = amt
		ex.Description = fmt.Sprintf("服務單稅項 - %s", r.Name)
		ex.UpdatedAt = now
		if ex.ReferenceID == nil {
			ex.ReferenceID = &serviceOrderID
		}
		if ex.ReferenceType == "" {
			ex.ReferenceType = "service_order"
		}

		ef := map[string]interface{}{}
		if ex.ExtraFields != nil {
			ef = map[string]interface{}(ex.ExtraFields)
		}
		ef["service_order_id"] = serviceOrderID.String()
		ef["tax_id"] = r.TaxID.String()
		ef["tax_name"] = r.Name
		ef["tax_mode"] = r.TaxMode
		ef["tax_value"] = r.TaxValue
		ef["base_amount"] = round2(r.BaseAmount)
		ef["discount_ratio"] = ratio
		ex.ExtraFields = models.JSONB(ef)

		if err := database.DB.Save(ex).Error; err != nil {
			log.Printf("Failed to save expense: %v", err)
		}
	}

	// 追加：沒有 service_tax_relations 的稅，只更新已存在支出
	missing := make([]uuid.UUID, 0)
	for _, tid := range taxIDs {
		if !seenTax[tid.String()] {
			missing = append(missing, tid)
		}
	}
	if len(missing) > 0 {
		var taxes []models.ServiceTax
		_ = database.DB.Where("tenant_id = ? AND id IN ?", tenantID, missing).Find(&taxes).Error
		for _, t := range taxes {
			ex := existingByTaxID[t.ID.String()]
			if ex == nil {
				continue
			}
			base := subtotal
			baseAdj := base * ratio
			amt := 0.0
			if t.TaxMode == "fixed" {
				if base > 0 {
					amt = t.TaxValue
				}
			} else {
				amt = baseAdj * (t.TaxValue / 100.0)
			}
			amt = round2(amt)

			ef := map[string]interface{}{}
			if ex.ExtraFields != nil {
				ef = map[string]interface{}(ex.ExtraFields)
			}
			ef["service_order_id"] = serviceOrderID.String()
			ef["tax_id"] = t.ID.String()
			ef["tax_name"] = t.Name
			ef["tax_mode"] = t.TaxMode
			ef["tax_value"] = t.TaxValue
			ef["base_amount"] = round2(base)
			ef["discount_ratio"] = ratio

			ex.Amount = amt
			ex.Description = fmt.Sprintf("服務單稅項 - %s", t.Name)
			ex.UpdatedAt = now
			ex.ExtraFields = models.JSONB(ef)
			if err := database.DB.Save(ex).Error; err != nil {
			log.Printf("Failed to save expense: %v", err)
		}
		}
	}
}
