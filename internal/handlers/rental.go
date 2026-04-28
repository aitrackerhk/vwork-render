package handlers

import (
	"errors"
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func GetRentalOrders(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID := c.Query("customer_id")

	var rentalOrders []models.RentalOrder
	query := database.DB.Where("tenant_id = ?", tenantID)

	if customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 搜索過濾（出租單號 + 客戶名 + 資源名）
	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"(order_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = rental_orders.customer_id AND customers.name ILIKE ?) OR EXISTS (SELECT 1 FROM rental_order_items roi WHERE roi.rental_order_id = rental_orders.id AND roi.trashed_at IS NULL AND roi.resource_name ILIKE ?))",
			like, like, like,
		)
	}

	// 標籤過濾（支援多選）
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN rental_order_label_relations ON rental_orders.id = rental_order_label_relations.rental_order_id").
			Where("rental_order_label_relations.label_id IN ?", labelIDs).
			Group("rental_orders.id")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.RentalOrder{}).Count(&total)

	if err := query.Preload("Customer").Preload("RentalOrderItems.Staff").
		Preload("Salesperson").Preload("Store").Preload("Labels").Preload("Appointments").
		Offset(offset).Limit(limit).Order("rental_date DESC").
		Find(&rentalOrders).Error; err != nil {
		log.Printf("Error loading rental orders: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load rental orders: " + err.Error()})
	}

	// 為每張出租單計算不重複的資源名稱列表
	type rentalOrderWithResources struct {
		models.RentalOrder
		Resources []string `json:"resources"`
	}
	results := make([]rentalOrderWithResources, len(rentalOrders))
	for i, ro := range rentalOrders {
		results[i].RentalOrder = ro
		seen := map[string]bool{}
		for _, item := range ro.RentalOrderItems {
			name := item.ResourceName
			if name != "" && !seen[name] {
				seen[name] = true
				results[i].Resources = append(results[i].Resources, name)
			}
		}
		if results[i].Resources == nil {
			results[i].Resources = []string{}
		}
	}

	return c.JSON(fiber.Map{
		"data":  results,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetRentalOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var rentalOrder models.RentalOrder

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Customer").Preload("RentalOrderItems.Staff").
		Preload("Salesperson").Preload("Store").Preload("Labels").Preload("Appointments").
		First(&rentalOrder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "Rental order not found"})
		}
		log.Printf("Error loading rental order %s: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load rental order: " + err.Error()})
	}

	// 從 income 表獲取付款記錄
	var incomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "rental_order", rentalOrder.ID).Find(&incomes)

	// 構建結果，包含付款記錄
	result := make(map[string]interface{})
	result["id"] = rentalOrder.ID
	result["tenant_id"] = rentalOrder.TenantID
	result["order_number"] = rentalOrder.OrderNumber
	result["customer_id"] = rentalOrder.CustomerID
	result["customer"] = rentalOrder.Customer
	result["contact_name"] = rentalOrder.ContactName
	result["contact_email"] = rentalOrder.ContactEmail
	result["contact_phone"] = rentalOrder.ContactPhone
	result["contact_address"] = rentalOrder.ContactAddress
	result["salesperson_id"] = rentalOrder.SalespersonID
	result["salesperson"] = rentalOrder.Salesperson
	result["rental_date"] = rentalOrder.RentalDate
	result["status"] = rentalOrder.Status
	result["notes"] = rentalOrder.Notes
	result["rental_order_items"] = rentalOrder.RentalOrderItems
	result["labels"] = rentalOrder.Labels
	result["appointments"] = rentalOrder.Appointments
	result["created_at"] = rentalOrder.CreatedAt
	result["updated_at"] = rentalOrder.UpdatedAt
	if rentalOrder.ExtraFields != nil {
		fields := map[string]interface{}(rentalOrder.ExtraFields)
		if refundNotes, exists := fields["refund_notes"]; exists {
			result["refund_notes"] = refundNotes
		}
	}
	result["created_by"] = rentalOrder.CreatedBy
	result["updated_by"] = rentalOrder.UpdatedBy
	result["extra_fields"] = rentalOrder.ExtraFields

	// 從 income 表獲取付款記錄
	if len(incomes) > 0 {
		paymentRecords := make([]map[string]interface{}, len(incomes))
		for i, inc := range incomes {
			ef := map[string]interface{}{}
			if inc.ExtraFields != nil {
				ef = map[string]interface{}(inc.ExtraFields)
			}
			paymentRecords[i] = map[string]interface{}{
				"income_id":         inc.ID.String(),
				"payment_date":      inc.IncomeDate.Format("2006-01-02"),
				"payment_method":    inc.PaymentMethod,
				"payment_method_id": ef["payment_method_id"],
				"bank_account_id":   inc.BankAccountID,
				"amount":            inc.Amount,
				"invoice_number":    ef["invoice_number"],
				"reference_number":  ef["reference_number"],
				"notes":             inc.Notes,
			}
		}
		result["payment_records"] = paymentRecords
	} else {
		result["payment_records"] = []interface{}{}
	}

	return c.JSON(result)
}

func CreateRentalOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		OrderNumber         string      `json:"order_number"`
		CustomerID          *uuid.UUID  `json:"customer_id"`
		StoreID             *uuid.UUID  `json:"store_id"`
		ContactName         string      `json:"contact_name"`
		ContactEmail        string      `json:"contact_email"`
		ContactPhone        string      `json:"contact_phone"`
		ContactAddress      string      `json:"contact_address"`
		SalespersonID       *uuid.UUID  `json:"salesperson_id"`
		CommissionAmount    float64     `json:"commission_amount"`
		RentalDate          string      `json:"rental_date"`
		Status              string      `json:"status"`
		Notes               string      `json:"notes"`
		ReferralCode        string      `json:"referral_code"`
		OrderDiscount       float64     `json:"order_discount"`
		OrderDiscountAmount float64     `json:"order_discount_amount"`
		LabelIDs            []uuid.UUID `json:"label_ids"`
		RentalOrderItems    []struct {
			ResourceType string     `json:"resource_type"`
			ResourceID   *uuid.UUID `json:"resource_id"`
			ResourceName string     `json:"resource_name"`
			StaffID      *uuid.UUID `json:"staff_id"`
			Quantity     float64    `json:"quantity"`
			UnitPrice    float64    `json:"unit_price"`
			Notes        string     `json:"notes"`
		} `json:"rental_order_items"`
		Appointments []struct {
			ID         *uuid.UUID `json:"id"`
			CustomerID *uuid.UUID `json:"customer_id"`
			ServiceID  *uuid.UUID `json:"service_id"`
			StaffID    *uuid.UUID `json:"staff_id"`
			StartTime  string     `json:"start_time"`
			EndTime    string     `json:"end_time"`
			Status     string     `json:"status"`
			Notes      string     `json:"notes"`
		} `json:"appointments"`
		PaymentRecords []map[string]interface{} `json:"payment_records"`
		ExtraFields    map[string]interface{}   `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 如果沒有提供出租單號，自動生成
	if req.OrderNumber == "" {
		req.OrderNumber = "RO-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
	}

	// 檢查出租單號是否已存在
	var existingRentalOrder models.RentalOrder
	if err := database.DB.Where("tenant_id = ? AND order_number = ?", tenantID, req.OrderNumber).First(&existingRentalOrder).Error; err == nil {
		req.OrderNumber = "RO-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
	}

	// 解析日期（使用租戶時區）
	rentalDate, err := utils.ParseDateInTenantTimezone(tenantID, req.RentalDate)
	if err != nil {
		rentalDate = utils.NowInTenantTimezone(tenantID)
	}

	if req.Status == "" {
		req.Status = "confirmed"
	}

	// 如果沒有提供referral_code，嘗試從客戶信息中獲取
	if req.ReferralCode == "" && req.CustomerID != nil {
		var customer models.Customer
		if err := database.DB.Where("id = ? AND tenant_id = ?", req.CustomerID, tenantID).First(&customer).Error; err == nil {
			req.ReferralCode = customer.ReferralCode
		}
	}

	now := time.Now()

	// 處理 ExtraFields
	extraFields := make(map[string]interface{})
	if req.ExtraFields != nil {
		for k, v := range req.ExtraFields {
			if k != "payment_records" {
				extraFields[k] = v
			}
		}
	}

	rentalOrder := models.RentalOrder{
		TenantID:         tenantID,
		OrderNumber:      req.OrderNumber,
		CustomerID:       req.CustomerID,
		StoreID:          req.StoreID,
		ContactName:      req.ContactName,
		ContactEmail:     req.ContactEmail,
		ContactPhone:     req.ContactPhone,
		ContactAddress:   req.ContactAddress,
		SalespersonID:    req.SalespersonID,
		CommissionAmount: req.CommissionAmount,
		RentalDate:       rentalDate,
		Status:           req.Status,
		Notes:            req.Notes,
		ReferralCode:     req.ReferralCode,
		CreatedBy:        &userID,
		UpdatedBy:        &userID,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExtraFields:      models.JSONB(extraFields),
	}

	// 計算總金額
	var subtotal float64
	for _, item := range req.RentalOrderItems {
		itemTotal := item.Quantity * item.UnitPrice
		subtotal += itemTotal
	}

	// 應用折扣
	orderDiscountAmount := req.OrderDiscountAmount
	if req.OrderDiscount > 0 && orderDiscountAmount == 0 {
		orderDiscountAmount = subtotal * req.OrderDiscount / 100
	}

	rentalOrder.TotalAmount = subtotal - orderDiscountAmount
	if rentalOrder.TotalAmount < 0 {
		rentalOrder.TotalAmount = 0
	}

	// 計算積分（如果有客戶且積分設置存在）
	var pointsEarned int = 0
	if req.CustomerID != nil {
		var pointSetting models.PointSetting
		if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
			if pointSetting.EnableServiceOrderEarnPoints && pointSetting.PointsPerDollar > 0 {
				actualAmount := rentalOrder.TotalAmount
				if actualAmount > 0 {
					pointsEarned = int(actualAmount * pointSetting.PointsPerDollar)
				}
			}
		}
	}
	rentalOrder.PointsEarned = pointsEarned

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&rentalOrder).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create rental order: " + err.Error()})
	}

	// 創建出租單明細
	for _, itemReq := range req.RentalOrderItems {
		itemTotal := itemReq.Quantity * itemReq.UnitPrice
		rentalOrderItem := models.RentalOrderItem{
			TenantID:      tenantID,
			RentalOrderID: rentalOrder.ID,
			ResourceType:  itemReq.ResourceType,
			ResourceID:    itemReq.ResourceID,
			ResourceName:  itemReq.ResourceName,
			StaffID:       itemReq.StaffID,
			Quantity:      itemReq.Quantity,
			UnitPrice:     itemReq.UnitPrice,
			TotalPrice:    itemTotal,
			Notes:         itemReq.Notes,
			CreatedAt:     now,
		}

		if err := tx.Create(&rentalOrderItem).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create rental order item: " + err.Error()})
		}
	}

	// 處理標籤關聯
	if len(req.LabelIDs) > 0 {
		for _, labelID := range req.LabelIDs {
			var label models.RentalOrderLabel
			if err := tx.Where("id = ? AND tenant_id = ?", labelID, tenantID).First(&label).Error; err == nil {
				relation := map[string]interface{}{
					"rental_order_id": rentalOrder.ID,
					"label_id":        labelID,
				}
				if err := tx.Table("rental_order_label_relations").Create(relation).Error; err != nil {
					continue
				}
			}
		}
	}

	// 處理預約記錄
	if len(req.Appointments) > 0 {
		for _, aptReq := range req.Appointments {
			startTime, err := utils.ParseDateTimeInTenantTimezone(tenantID, aptReq.StartTime)
			if err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid start_time format: " + err.Error()})
			}

			endTime, err := utils.ParseDateTimeInTenantTimezone(tenantID, aptReq.EndTime)
			if err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid end_time format: " + err.Error()})
			}

			if endTime.Before(startTime) || endTime.Equal(startTime) {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "結束時間必須晚於開始時間"})
			}

			customerID := aptReq.CustomerID
			if customerID == nil {
				customerID = req.CustomerID
			}
			if customerID == nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "預約必須有客戶ID"})
			}

			var customer models.Customer
			if err := tx.Where("id = ? AND tenant_id = ?", *customerID, tenantID).First(&customer).Error; err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID: customer not found or does not belong to this tenant"})
			}

			if aptReq.StaffID != nil {
				var staff models.ServiceStaff
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.StaffID, tenantID).First(&staff).Error; err != nil {
					tx.Rollback()
					log.Printf("[CreateRentalOrder] Invalid staff ID: %v, error: %v", *aptReq.StaffID, err)
					return c.Status(400).JSON(fiber.Map{"error": "Invalid staff ID: service staff not found or does not belong to this tenant"})
				}
			}

			if aptReq.StaffID != nil {
				var userCheck models.User
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.StaffID, tenantID).First(&userCheck).Error; err != nil {
					tx.Rollback()
					return c.Status(400).JSON(fiber.Map{"error": "Invalid staff ID: user not found or does not belong to this tenant"})
				}
			}

			appointmentDate := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
			appointmentTime := time.Date(2000, 1, 1, startTime.Hour(), startTime.Minute(), startTime.Second(), 0, startTime.Location())

			appointment := models.Appointment{
				TenantID:        tenantID,
				CustomerID:      *customerID,
				ServiceID:       aptReq.ServiceID,
				StaffID:         aptReq.StaffID,
				StartTime:       startTime,
				EndTime:         endTime,
				AppointmentDate: appointmentDate,
				AppointmentTime: models.NewSQLTime(appointmentTime),
				RentalOrderID:   &rentalOrder.ID,
				Status:          aptReq.Status,
				Notes:           aptReq.Notes,
			}

			if aptReq.Status == "" {
				appointment.Status = "pending"
			}

			if err := tx.Create(&appointment).Error; err != nil {
				tx.Rollback()
				log.Printf("[CreateRentalOrder] Failed to create appointment: %v", err)
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create appointment: " + err.Error()})
			}
		}
	}

	// 為每個付款記錄創建 income 記錄
	if len(req.PaymentRecords) > 0 {
		for _, record := range req.PaymentRecords {
			amount, _ := record["amount"].(float64)
			if amount > 0 {
				paymentDateStr, _ := record["payment_date"].(string)
				paymentMethod, _ := record["payment_method"].(string)
				paymentMethodID, _ := record["payment_method_id"].(string)
				bankAccountIDStr, _ := record["bank_account_id"].(string)
				notes, _ := record["notes"].(string)
				invoiceNumber, _ := record["invoice_number"].(string)
				referenceNumber, _ := record["reference_number"].(string)

				if invoiceNumber == "" {
					if n, err := reserveNextNumber(tenantID, "invoice_number", "rental-orders"); err == nil {
						invoiceNumber = n
					}
				} else {
					_, _ = reserveSpecificNumber(tenantID, "invoice_number", invoiceNumber, "rental-orders")
				}

				var incomeDate time.Time
				if paymentDateStr != "" {
					if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
						incomeDate = parsedDate
					} else {
						incomeDate = rentalOrder.RentalDate
					}
				} else {
					incomeDate = rentalOrder.RentalDate
				}

				var bankAccountID *uuid.UUID
				if bankAccountIDStr != "" {
					if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
						bankAccountID = &parsedID
					}
				}

				description := fmt.Sprintf("出租單 %s 的付款", rentalOrder.OrderNumber)
				if invoiceNumber != "" {
					description = invoiceNumber
				}

				relatedUserID := &userID
				if rentalOrder.SalespersonID != nil {
					relatedUserID = rentalOrder.SalespersonID
				}
				income := models.Income{
					TenantID:      tenantID,
					RelatedUserID: relatedUserID,
					IncomeType:    "rental_order",
					ReferenceID:   &rentalOrder.ID,
					ReferenceType: "rental_order",
					Category:      "rental_order",
					Description:   description,
					Amount:        amount,
					IncomeDate:    incomeDate,
					PaymentMethod: paymentMethod,
					BankAccountID: bankAccountID,
					Status:        "confirmed",
					Notes:         notes,
					CreatedBy:     &userID,
					UpdatedBy:     &userID,
					CreatedAt:     now,
					UpdatedAt:     now,
					ExtraFields: models.JSONB(map[string]interface{}{
						"payment_method_id": paymentMethodID,
						"invoice_number":    invoiceNumber,
						"reference_number":  referenceNumber,
						"rental_order_id":   rentalOrder.ID.String(),
					}),
				}

				if err := tx.Create(&income).Error; err != nil {
					// 創建失敗，不中斷出租單創建
				}
			}
		}
	} else {
		// 檢查是否自動生成付款記錄
		auto := getDocumentAutoSettingsForTenant(tenantID)
		shouldAutoGeneratePayment := auto.AutoGenerateServiceOrderPayment && rentalOrder.TotalAmount > 0

		if shouldAutoGeneratePayment {
			defPayMethod := defaultReceivingPaymentMethodCode(tenantID)
			defBankAccID := defaultReceivingBankAccountID(tenantID)

			invoiceNumber := ""
			if n, err := reserveNextNumber(tenantID, "invoice_number", "rental-orders"); err == nil {
				invoiceNumber = n
			}

			description := fmt.Sprintf("出租單 %s 的付款", rentalOrder.OrderNumber)
			if invoiceNumber != "" {
				description = invoiceNumber
			}

			relatedUserID := &userID
			if rentalOrder.SalespersonID != nil {
				relatedUserID = rentalOrder.SalespersonID
			}
			income := models.Income{
				TenantID:      tenantID,
				RelatedUserID: relatedUserID,
				IncomeType:    "rental_order",
				ReferenceID:   &rentalOrder.ID,
				ReferenceType: "rental_order",
				Category:      "rental_order",
				Description:   description,
				Amount:        rentalOrder.TotalAmount,
				IncomeDate:    rentalOrder.RentalDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defBankAccID,
				Status:        "confirmed",
				Notes:         "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExtraFields: models.JSONB(map[string]interface{}{
					"invoice_number":  invoiceNumber,
					"rental_order_id": rentalOrder.ID.String(),
				}),
			}

			if err := tx.Create(&income).Error; err != nil {
				log.Printf("Failed to auto-generate payment record: %v", err)
			}
		}
	}

	// 如果客戶存在且獲得了積分，創建積分記錄並更新客戶總積分
	if req.CustomerID != nil && pointsEarned > 0 {
		var existingPoint models.Point
		err := tx.Where("tenant_id = ? AND customer_id = ? AND source_type = ? AND source_id = ? AND points_type = ?",
			tenantID,
			*req.CustomerID,
			"rental_order",
			rentalOrder.ID,
			"earned",
		).First(&existingPoint).Error
		if err == nil {
			// 已存在積分記錄，跳過避免重複累加
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Failed to check existing point record: %v", err)
		} else {
			point := models.Point{
				TenantID:    tenantID,
				CustomerID:  *req.CustomerID,
				Points:      pointsEarned,
				PointsType:  "earned",
				SourceType:  "rental_order",
				SourceID:    &rentalOrder.ID,
				Description: fmt.Sprintf("出租單 %s 消費獲得積分", rentalOrder.OrderNumber),
				Status:      "active",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Create(&point).Error; err != nil {
				log.Printf("Failed to create point record: %v", err)
			} else {
				var customer models.Customer
				if err := tx.Where("id = ?", *req.CustomerID).First(&customer).Error; err == nil {
					customer.TotalPoints += pointsEarned
					tx.Save(&customer)
				}
			}
		}
	}

	tx.Commit()

	// 同步 Invoice
	syncOrderInvoice(database.DB, tenantID, "rental_order", rentalOrder.ID, &userID)

	// 處理介紹人積分獎勵（僅在出租單完成時）
	if rentalOrder.CustomerID != nil && rentalOrder.Status == "completed" {
		refCode := rentalOrder.ReferralCode
		if refCode == "" {
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *rentalOrder.CustomerID).First(&customer).Error; err == nil {
				refCode = customer.ReferralCode
			}
		}
		if refCode != "" {
			var pointSetting models.PointSetting
			if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
				if pointSetting.EnableServiceOrderReferralBonus {
					processReferralBonus(tenantID, refCode, *rentalOrder.CustomerID, "出租單", rentalOrder.ID, rentalOrder.OrderNumber, pointsEarned)
				}
			} else {
				processReferralBonus(tenantID, refCode, *rentalOrder.CustomerID, "出租單", rentalOrder.ID, rentalOrder.OrderNumber, pointsEarned)
			}
		}
	}

	// 處理會員等級自動升級
	if rentalOrder.CustomerID != nil {
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			var hasAutoUpgrade bool
			database.DB.Model(&models.MemberLevel{}).
				Where("tenant_id = ? AND auto_upgrade = ? AND status = ?", tenantID, true, "active").
				Select("COUNT(*) > 0").
				Scan(&hasAutoUpgrade)

			if hasAutoUpgrade {
				checkAndUpgradeMemberLevel(tenantID, *rentalOrder.CustomerID, rentalOrder.TotalAmount)
			}
		}
	}

	// 發送出租單確認 email
	if rentalOrder.Status == "confirmed" {
		emailTo := strings.TrimSpace(rentalOrder.ContactEmail)
		customerName := strings.TrimSpace(rentalOrder.ContactName)
		customerID := uuid.Nil
		if rentalOrder.CustomerID != nil {
			customerID = *rentalOrder.CustomerID
			var customer models.Customer
			if err := database.DB.Where("id = ?", *rentalOrder.CustomerID).First(&customer).Error; err == nil {
				if emailTo == "" {
					emailTo = strings.TrimSpace(customer.Email)
				}
				if customerName == "" {
					customerName = strings.TrimSpace(customer.Name)
				}
			}
		}
		if emailTo != "" {
			if customerName == "" {
				customerName = "客戶"
			}
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				rentalDateStr := rentalOrder.RentalDate.Format("2006-01-02")
				if err := email.EnqueueOrderConfirmationEmail(
					tenantID,
					tenant.Subdomain,
					customerID,
					emailTo,
					customerName,
					rentalOrder.OrderNumber,
					rentalDateStr,
					rentalOrder.TotalAmount,
					"rental_order",
				); err != nil {
					log.Printf("Failed to enqueue rental order confirmation email: %v", err)
				}
			}
		}
	}

	// 重新載入關聯數據
	database.DB.Where("id = ?", rentalOrder.ID).
		Preload("Customer").Preload("RentalOrderItems.Staff").
		Preload("Salesperson").Preload("Store").Preload("Labels").Preload("Appointments").
		First(&rentalOrder)

	// 通知整個 domain 的所有用戶（檢查通知設置）
	var settings models.NotificationSettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error; err != nil {
		settings.ServiceOrderNotificationsEnabled = true
	}

	if settings.ServiceOrderNotificationsEnabled {
		customerName := "散客"
		if rentalOrder.CustomerID != nil {
			if rentalOrder.Customer.ID != uuid.Nil {
				customerName = rentalOrder.Customer.Name
			}
		}
		title := fmt.Sprintf("新出租單：%s", rentalOrder.OrderNumber)
		message := fmt.Sprintf("出租單編號：%s\n客戶：%s\n總金額：$%.2f\n狀態：%s",
			rentalOrder.OrderNumber, customerName, rentalOrder.TotalAmount, rentalOrder.Status)
		link := fmt.Sprintf("/rental-orders?rental_order_id=%s", rentalOrder.ID.String())
		go utils.CreateNotificationAlertForAllUsers(tenantID, "rental_order_created", title, message, link, &userID)
	}

	// Auto-refresh business goals related to rental orders
	// go RefreshActiveBusinessGoals(tenantID, []string{"rental_order_count"})

	return c.Status(201).JSON(rentalOrder)
}

func UpdateRentalOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")
	var rentalOrder models.RentalOrder

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&rentalOrder).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Rental order not found"})
	}
	oldStatus := rentalOrder.Status

	// 保存原始積分值（用於後續計算差值）
	oldPointsEarned := rentalOrder.PointsEarned
	oldCustomerID := rentalOrder.CustomerID

	var req struct {
		OrderNumber      string      `json:"order_number"`
		CustomerID       *uuid.UUID  `json:"customer_id"`
		StoreID          *uuid.UUID  `json:"store_id"`
		ContactName      string      `json:"contact_name"`
		ContactEmail     string      `json:"contact_email"`
		ContactPhone     string      `json:"contact_phone"`
		ContactAddress   string      `json:"contact_address"`
		SalespersonID    *uuid.UUID  `json:"salesperson_id"`
		RentalDate       string      `json:"rental_date"`
		Status           string      `json:"status"`
		Notes            string      `json:"notes"`
		LabelIDs         []uuid.UUID `json:"label_ids"`
		RentalOrderItems []struct {
			ResourceType string     `json:"resource_type"`
			ResourceID   *uuid.UUID `json:"resource_id"`
			ResourceName string     `json:"resource_name"`
			StaffID      *uuid.UUID `json:"staff_id"`
			Quantity     float64    `json:"quantity"`
			UnitPrice    float64    `json:"unit_price"`
			Notes        string     `json:"notes"`
		} `json:"rental_order_items"`
		Appointments []struct {
			ID         *uuid.UUID `json:"id"`
			CustomerID *uuid.UUID `json:"customer_id"`
			ServiceID  *uuid.UUID `json:"service_id"`
			StaffID    *uuid.UUID `json:"staff_id"`
			StartTime  string     `json:"start_time"`
			EndTime    string     `json:"end_time"`
			Status     string     `json:"status"`
			Notes      string     `json:"notes"`
		} `json:"appointments"`
		PaymentRecords []map[string]interface{} `json:"payment_records"`
		ExtraFields    map[string]interface{}   `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 更新出租單基本信息
	if req.OrderNumber != "" {
		rentalOrder.OrderNumber = req.OrderNumber
	}
	if req.CustomerID != nil {
		rentalOrder.CustomerID = req.CustomerID
	}
	rentalOrder.StoreID = req.StoreID
	rentalOrder.ContactName = req.ContactName
	rentalOrder.ContactEmail = req.ContactEmail
	rentalOrder.ContactPhone = req.ContactPhone
	rentalOrder.ContactAddress = req.ContactAddress
	rentalOrder.SalespersonID = req.SalespersonID
	if req.RentalDate != "" {
		if rentalDate, err := utils.ParseDateInTenantTimezone(tenantID, req.RentalDate); err == nil {
			rentalOrder.RentalDate = rentalDate
		}
	}
	if req.Status != "" {
		rentalOrder.Status = req.Status
	}
	rentalOrder.Notes = req.Notes
	rentalOrder.UpdatedBy = &userID
	rentalOrder.UpdatedAt = time.Now()

	if req.ExtraFields != nil {
		rentalOrder.ExtraFields = models.JSONB(req.ExtraFields)
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Save(&rentalOrder).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update rental order: " + err.Error()})
	}

	// 刪除現有的出租單明細
	tx.Where("rental_order_id = ?", rentalOrder.ID).Delete(&models.RentalOrderItem{})

	// 重新創建出租單明細
	now := time.Now()
	for i, itemReq := range req.RentalOrderItems {
		if itemReq.Quantity <= 0 {
			tx.Rollback()
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("出租項 %d 的數量必須大於 0", i+1)})
		}
		if itemReq.UnitPrice < 0 {
			tx.Rollback()
			return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("出租項 %d 的單價不能為負數", i+1)})
		}

		itemTotal := itemReq.Quantity * itemReq.UnitPrice
		rentalOrderItem := models.RentalOrderItem{
			TenantID:      tenantID,
			RentalOrderID: rentalOrder.ID,
			ResourceType:  itemReq.ResourceType,
			ResourceID:    itemReq.ResourceID,
			ResourceName:  itemReq.ResourceName,
			StaffID:       itemReq.StaffID,
			Quantity:      itemReq.Quantity,
			UnitPrice:     itemReq.UnitPrice,
			TotalPrice:    itemTotal,
			Notes:         itemReq.Notes,
			CreatedAt:     now,
		}

		if err := tx.Create(&rentalOrderItem).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to create rental order item: %v, item: %+v", err, rentalOrderItem)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update rental order item: " + err.Error()})
		}
	}

	// 處理標籤關聯
	if req.LabelIDs != nil {
		tx.Table("rental_order_label_relations").Where("rental_order_id = ?", rentalOrder.ID).Delete(nil)

		for _, labelID := range req.LabelIDs {
			var label models.RentalOrderLabel
			if err := tx.Where("id = ? AND tenant_id = ?", labelID, tenantID).First(&label).Error; err == nil {
				relation := map[string]interface{}{
					"rental_order_id": rentalOrder.ID,
					"label_id":        labelID,
				}
				if err := tx.Table("rental_order_label_relations").Create(relation).Error; err != nil {
					continue
				}
			}
		}
	}

	// 處理預約記錄
	if len(req.Appointments) > 0 {
		keepIDs := make(map[uuid.UUID]bool)
		for _, aptReq := range req.Appointments {
			if aptReq.ID != nil {
				keepIDs[*aptReq.ID] = true
			}
		}

		var existingAppointments []models.Appointment
		tx.Where("rental_order_id = ?", rentalOrder.ID).Find(&existingAppointments)
		for _, apt := range existingAppointments {
			if !keepIDs[apt.ID] {
				tx.Delete(&apt)
			}
		}

		for _, aptReq := range req.Appointments {
			startTime, err := utils.ParseDateTimeInTenantTimezone(tenantID, aptReq.StartTime)
			if err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid start_time format: " + err.Error()})
			}

			endTime, err := utils.ParseDateTimeInTenantTimezone(tenantID, aptReq.EndTime)
			if err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid end_time format: " + err.Error()})
			}

			if endTime.Before(startTime) || endTime.Equal(startTime) {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "結束時間必須晚於開始時間"})
			}

			customerID := aptReq.CustomerID
			if customerID == nil {
				customerID = req.CustomerID
			}
			if customerID == nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "預約必須有客戶ID"})
			}

			var customer models.Customer
			if err := tx.Where("id = ? AND tenant_id = ?", *customerID, tenantID).First(&customer).Error; err != nil {
				tx.Rollback()
				return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID: customer not found or does not belong to this tenant"})
			}

			if aptReq.ServiceID != nil {
				var service models.Service
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.ServiceID, tenantID).First(&service).Error; err != nil {
					tx.Rollback()
					return c.Status(400).JSON(fiber.Map{"error": "Invalid service ID: service not found or does not belong to this tenant"})
				}
			}

			if aptReq.StaffID != nil {
				var staff models.ServiceStaff
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.StaffID, tenantID).First(&staff).Error; err != nil {
					tx.Rollback()
					return c.Status(400).JSON(fiber.Map{"error": "Invalid staff ID: service staff not found or does not belong to this tenant"})
				}
			}

			if aptReq.ID != nil {
				var appointment models.Appointment
				if err := tx.Where("id = ? AND tenant_id = ?", *aptReq.ID, tenantID).First(&appointment).Error; err == nil {
					appointmentDate := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
					appointmentTime := time.Date(2000, 1, 1, startTime.Hour(), startTime.Minute(), startTime.Second(), 0, startTime.Location())

					appointment.CustomerID = *customerID
					appointment.ServiceID = aptReq.ServiceID
					appointment.StaffID = aptReq.StaffID
					appointment.StartTime = startTime
					appointment.EndTime = endTime
					appointment.AppointmentDate = appointmentDate
					appointment.AppointmentTime = models.NewSQLTime(appointmentTime)
					appointment.Status = aptReq.Status
					appointment.Notes = aptReq.Notes
					if aptReq.Status == "" {
						appointment.Status = "pending"
					}
					if err := tx.Save(&appointment).Error; err != nil {
						tx.Rollback()
						return c.Status(500).JSON(fiber.Map{"error": "Failed to update appointment: " + err.Error()})
					}
				}
			} else {
				appointmentDate := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, startTime.Location())
				appointmentTime := time.Date(2000, 1, 1, startTime.Hour(), startTime.Minute(), startTime.Second(), 0, startTime.Location())

				appointment := models.Appointment{
					TenantID:        tenantID,
					CustomerID:      *customerID,
					ServiceID:       aptReq.ServiceID,
					StaffID:         aptReq.StaffID,
					StartTime:       startTime,
					EndTime:         endTime,
					AppointmentDate: appointmentDate,
					AppointmentTime: models.NewSQLTime(appointmentTime),
					RentalOrderID:   &rentalOrder.ID,
					Status:          aptReq.Status,
					Notes:           aptReq.Notes,
				}

				if aptReq.Status == "" {
					appointment.Status = "confirmed"
				}

				if appointment.StartTime.IsZero() {
					tx.Rollback()
					return c.Status(400).JSON(fiber.Map{"error": "StartTime cannot be zero"})
				}
				if appointment.EndTime.IsZero() {
					tx.Rollback()
					return c.Status(400).JSON(fiber.Map{"error": "EndTime cannot be zero"})
				}
				if appointment.CustomerID == uuid.Nil {
					tx.Rollback()
					return c.Status(400).JSON(fiber.Map{"error": "CustomerID is required"})
				}

				if err := tx.Create(&appointment).Error; err != nil {
					tx.Rollback()
					return c.Status(500).JSON(fiber.Map{"error": "Failed to create appointment: " + err.Error()})
				}
			}
		}
	}

	// 處理付款記錄（只保存到 income 表）
	var existingIncomes []models.Income
	database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "rental_order", rentalOrder.ID).Find(&existingIncomes)
	existingIncomeIDs := make(map[uuid.UUID]bool)
	for _, inc := range existingIncomes {
		existingIncomeIDs[inc.ID] = true
	}

	if len(req.PaymentRecords) > 0 {
		keptIncomeIDs := make(map[uuid.UUID]bool)

		for _, record := range req.PaymentRecords {
			if record == nil {
				continue
			}

			var amount float64
			amountRaw, exists := record["amount"]
			if !exists {
				continue
			}

			switch v := amountRaw.(type) {
			case float64:
				amount = v
			case float32:
				amount = float64(v)
			case int:
				amount = float64(v)
			case int32:
				amount = float64(v)
			case int64:
				amount = float64(v)
			case string:
				if parsed, err := strconv.ParseFloat(v, 64); err == nil {
					amount = parsed
				}
			default:
				if strVal := fmt.Sprintf("%v", v); strVal != "" {
					if parsed, err := strconv.ParseFloat(strVal, 64); err == nil {
						amount = parsed
					}
				}
			}

			if amount > 0 {
				var incomeIDStr string
				if idVal, exists := record["income_id"]; exists {
					if idVal != nil {
						if idStr, ok := idVal.(string); ok {
							incomeIDStr = idStr
						}
					}
				}

				if incomeIDStr != "" {
					if incomeID, err := uuid.Parse(incomeIDStr); err == nil {
						keptIncomeIDs[incomeID] = true
						var existingIncome models.Income
						if err := database.DB.Where("id = ? AND tenant_id = ?", incomeID, tenantID).First(&existingIncome).Error; err == nil {
							paymentDateStr, _ := record["payment_date"].(string)
							paymentMethod, _ := record["payment_method"].(string)
							paymentMethodID, _ := record["payment_method_id"].(string)
							bankAccountIDStr, _ := record["bank_account_id"].(string)
							notes, _ := record["notes"].(string)
							invoiceNumber, _ := record["invoice_number"].(string)
							referenceNumber, _ := record["reference_number"].(string)

							var incomeDate time.Time
							if paymentDateStr != "" {
								if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
									incomeDate = parsedDate
								} else {
									incomeDate = rentalOrder.RentalDate
								}
							} else {
								incomeDate = rentalOrder.RentalDate
							}

							var bankAccountID *uuid.UUID
							if bankAccountIDStr != "" {
								if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
									bankAccountID = &parsedID
								}
							}

							incomeExtraFields := make(map[string]interface{})
							if existingIncome.ExtraFields != nil {
								incomeExtraFields = map[string]interface{}(existingIncome.ExtraFields)
							}
							if paymentMethodID != "" {
								incomeExtraFields["payment_method_id"] = paymentMethodID
							}
							if invoiceNumber != "" {
								incomeExtraFields["invoice_number"] = invoiceNumber
							}
							if referenceNumber != "" {
								incomeExtraFields["reference_number"] = referenceNumber
							}
							incomeExtraFields["rental_order_id"] = rentalOrder.ID.String()

							existingIncome.Amount = amount
							existingIncome.IncomeDate = incomeDate
							existingIncome.PaymentMethod = paymentMethod
							existingIncome.BankAccountID = bankAccountID
							existingIncome.Notes = notes
							existingIncome.ExtraFields = models.JSONB(incomeExtraFields)
							existingIncome.UpdatedBy = &userID
							existingIncome.UpdatedAt = time.Now()

							database.DB.Save(&existingIncome)
						}
					}
				} else {
					paymentDateStr, _ := record["payment_date"].(string)
					paymentMethod, _ := record["payment_method"].(string)
					paymentMethodID, _ := record["payment_method_id"].(string)
					bankAccountIDStr, _ := record["bank_account_id"].(string)
					notes, _ := record["notes"].(string)
					invoiceNumber, _ := record["invoice_number"].(string)
					referenceNumber, _ := record["reference_number"].(string)

					if invoiceNumber == "" {
						if n, err := reserveNextNumber(tenantID, "invoice_number", "rental-orders"); err == nil {
							invoiceNumber = n
						}
					} else {
						_, _ = reserveSpecificNumber(tenantID, "invoice_number", invoiceNumber, "rental-orders")
					}

					var incomeDate time.Time
					if paymentDateStr != "" {
						if parsedDate, err := utils.ParseDateInTenantTimezone(tenantID, paymentDateStr); err == nil {
							incomeDate = parsedDate
						} else {
							incomeDate = rentalOrder.RentalDate
						}
					} else {
						incomeDate = rentalOrder.RentalDate
					}

					var bankAccountID *uuid.UUID
					if bankAccountIDStr != "" {
						if parsedID, err := uuid.Parse(bankAccountIDStr); err == nil {
							bankAccountID = &parsedID
						}
					}

					description := fmt.Sprintf("出租單 %s 的付款", rentalOrder.OrderNumber)
					if invoiceNumber != "" {
						description = invoiceNumber
					}

					relatedUserID := &userID
					if rentalOrder.SalespersonID != nil {
						relatedUserID = rentalOrder.SalespersonID
					}
					income := models.Income{
						TenantID:      tenantID,
						RelatedUserID: relatedUserID,
						IncomeType:    "rental_order",
						ReferenceID:   &rentalOrder.ID,
						ReferenceType: "rental_order",
						Category:      "rental_order",
						Description:   description,
						Amount:        amount,
						IncomeDate:    incomeDate,
						PaymentMethod: paymentMethod,
						BankAccountID: bankAccountID,
						Status:        "confirmed",
						Notes:         notes,
						CreatedBy:     &userID,
						UpdatedBy:     &userID,
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
						ExtraFields: models.JSONB(map[string]interface{}{
							"payment_method_id": paymentMethodID,
							"invoice_number":    invoiceNumber,
							"reference_number":  referenceNumber,
							"rental_order_id":   rentalOrder.ID.String(),
						}),
					}

					if err := database.DB.Create(&income).Error; err == nil {
						keptIncomeIDs[income.ID] = true
					}
				}
			}
		}

		// 刪除不在新付款記錄中的 income 記錄
		for incomeID := range existingIncomeIDs {
			if !keptIncomeIDs[incomeID] {
				database.DB.Where("id = ? AND tenant_id = ?", incomeID, tenantID).Delete(&models.Income{})
			}
		}
	} else {
		auto := getDocumentAutoSettingsForTenant(tenantID)
		shouldAutoGeneratePayment := auto.AutoGenerateServiceOrderPayment && rentalOrder.TotalAmount > 0

		if shouldAutoGeneratePayment {
			defPayMethod := defaultReceivingPaymentMethodCode(tenantID)
			defBankAccID := defaultReceivingBankAccountID(tenantID)

			invoiceNumber := ""
			if n, err := reserveNextNumber(tenantID, "invoice_number", "rental-orders"); err == nil {
				invoiceNumber = n
			}

			description := fmt.Sprintf("出租單 %s 的付款", rentalOrder.OrderNumber)
			if invoiceNumber != "" {
				description = invoiceNumber
			}

			relatedUserID := &userID
			if rentalOrder.SalespersonID != nil {
				relatedUserID = rentalOrder.SalespersonID
			}
			income := models.Income{
				TenantID:      tenantID,
				RelatedUserID: relatedUserID,
				IncomeType:    "rental_order",
				ReferenceID:   &rentalOrder.ID,
				ReferenceType: "rental_order",
				Category:      "rental_order",
				Description:   description,
				Amount:        rentalOrder.TotalAmount,
				IncomeDate:    rentalOrder.RentalDate,
				PaymentMethod: defPayMethod,
				BankAccountID: defBankAccID,
				Status:        "confirmed",
				Notes:         "",
				CreatedBy:     &userID,
				UpdatedBy:     &userID,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				ExtraFields: models.JSONB(map[string]interface{}{
					"invoice_number":  invoiceNumber,
					"rental_order_id": rentalOrder.ID.String(),
				}),
			}

			if err := database.DB.Create(&income).Error; err != nil {
				log.Printf("Failed to auto-generate payment record: %v", err)
			}
		} else {
			database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "rental_order", rentalOrder.ID).Delete(&models.Income{})
		}
	}

	// 重新計算總金額
	var newSubtotal float64
	for _, itemReq := range req.RentalOrderItems {
		newSubtotal += itemReq.Quantity * itemReq.UnitPrice
	}
	rentalOrder.TotalAmount = newSubtotal
	if rentalOrder.TotalAmount < 0 {
		rentalOrder.TotalAmount = 0
	}
	tx.Model(&rentalOrder).Update("total_amount", rentalOrder.TotalAmount)

	tx.Commit()

	// 同步 Invoice
	syncOrderInvoice(database.DB, tenantID, "rental_order", rentalOrder.ID, &userID)

	// 重新計算積分
	var newPointsEarned int = 0
	var pointSetting models.PointSetting
	hasPointSetting := false
	if rentalOrder.CustomerID != nil {
		if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
			hasPointSetting = true
			if pointSetting.EnableServiceOrderEarnPoints && pointSetting.PointsPerDollar > 0 {
				actualAmount := rentalOrder.TotalAmount - rentalOrder.CouponDiscount - rentalOrder.PointsDiscount
				if actualAmount > 0 {
					newPointsEarned = int(actualAmount * pointSetting.PointsPerDollar)
				}
			}
		}
	}

	if newPointsEarned != rentalOrder.PointsEarned {
		rentalOrder.PointsEarned = newPointsEarned
		database.DB.Model(&rentalOrder).Update("points_earned", newPointsEarned)
	}

	// 積分調整：如果積分增加，補發差值
	if hasPointSetting && pointSetting.EnablePointsAdjustmentOnEdit && rentalOrder.CustomerID != nil {
		pointsDiff := newPointsEarned - oldPointsEarned
		if pointsDiff > 0 {
			sameCustomer := oldCustomerID != nil && *oldCustomerID == *rentalOrder.CustomerID
			if sameCustomer {
				pointNow := time.Now()
				point := models.Point{
					TenantID:    tenantID,
					CustomerID:  *rentalOrder.CustomerID,
					Points:      pointsDiff,
					PointsType:  "adjustment",
					SourceType:  "rental_order_edit",
					SourceID:    &rentalOrder.ID,
					Description: fmt.Sprintf("編輯出租單 %s 補發積分差值 (+%d)", rentalOrder.OrderNumber, pointsDiff),
					Status:      "active",
					CreatedAt:   pointNow,
					UpdatedAt:   pointNow,
				}
				if err := database.DB.Create(&point).Error; err != nil {
					log.Printf("Failed to create point adjustment record for rental order edit: %v", err)
				} else {
					var customer models.Customer
					if err := database.DB.Where("id = ?", *rentalOrder.CustomerID).First(&customer).Error; err == nil {
						customer.TotalPoints += pointsDiff
						database.DB.Save(&customer)
					}
				}
			}
		}
	}

	// 處理退款單（refund_notes）
	if req.ExtraFields != nil {
		if refundNotes, exists := req.ExtraFields["refund_notes"]; exists {
			if notesList, ok := refundNotes.([]interface{}); ok && len(notesList) > 0 {
				// 資源數量 map（用於限制：退的總量不能超過）
				orderQtyByResource := map[string]float64{}
				for _, it := range req.RentalOrderItems {
					key := it.ResourceType + ":" + it.ResourceName
					orderQtyByResource[key] += it.Quantity
				}

				vendor := ""
				if rentalOrder.CustomerID != nil {
					var customer models.Customer
					if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *rentalOrder.CustomerID).First(&customer).Error; err == nil {
						vendor = customer.Name
					}
				}
				if vendor == "" {
					vendor = "客戶"
				}

				for noteIndex, n := range notesList {
					note, ok := n.(map[string]interface{})
					if !ok {
						continue
					}

					refundNumber, _ := note["refund_number"].(string)
					refundDateStr, _ := note["refund_date"].(string)
					if refundDateStr == "" {
						refundDateStr = rentalOrder.RentalDate.Format("2006-01-02")
					}
					refundDate, err := utils.ParseDateInTenantTimezone(tenantID, refundDateStr)
					if err != nil {
						refundDate = utils.NowInTenantTimezone(tenantID)
					}

					// items total
					itemsTotal := 0.0
					itemsRaw, _ := note["items"].([]interface{})
					for _, ir := range itemsRaw {
						im, ok := ir.(map[string]interface{})
						if !ok {
							continue
						}
						var qty float64
						switch v := im["quantity"].(type) {
						case float64:
							qty = v
						case float32:
							qty = float64(v)
						case int:
							qty = float64(v)
						case int64:
							qty = float64(v)
						case string:
							if parsed, err := strconv.ParseFloat(v, 64); err == nil {
								qty = parsed
							}
						}
						if qty < 0 {
							qty = 0
						}

						var unit float64
						switch v := im["unit_price"].(type) {
						case float64:
							unit = v
						case float32:
							unit = float64(v)
						case int:
							unit = float64(v)
						case int64:
							unit = float64(v)
						case string:
							if parsed, err := strconv.ParseFloat(v, 64); err == nil {
								unit = parsed
							}
						}
						if unit < 0 {
							unit = 0
						}
						itemsTotal += qty * unit
					}

					extraAmount := 0.0
					switch v := note["extra_amount"].(type) {
					case float64:
						extraAmount = v
					case float32:
						extraAmount = float64(v)
					case int:
						extraAmount = float64(v)
					case int64:
						extraAmount = float64(v)
					case string:
						if parsed, err := strconv.ParseFloat(v, 64); err == nil {
							extraAmount = parsed
						}
					}
					if extraAmount < 0 {
						extraAmount = 0
					}

					totalRefund := itemsTotal + extraAmount
					if totalRefund < 0 {
						totalRefund = 0
					}

					// Expense（退款金額 -> 支出）
					expenseIDStr, _ := note["expense_id"].(string)
					if totalRefund > 0 && expenseIDStr == "" {
						paymentMethod := "cash"
						var pm models.PaymentMethod
						if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default_expense = ?", tenantID, "active", true).First(&pm).Error; err == nil && pm.Code != "" {
							paymentMethod = pm.Code
						} else if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default = ?", tenantID, "active", true).First(&pm).Error; err == nil && pm.Code != "" {
							paymentMethod = pm.Code
						}
						var bankAccountID *uuid.UUID = nil
						{
							var ba models.BankAccount
							if err := database.DB.Where("tenant_id = ? AND status = ? AND is_default_payment = ?", tenantID, "active", true).First(&ba).Error; err == nil && ba.ID != uuid.Nil {
								id := ba.ID
								bankAccountID = &id
							}
						}

						description := fmt.Sprintf("退款單 %s（出租單 %s）", refundNumber, rentalOrder.OrderNumber)
						exp := models.Expense{
							TenantID:      tenantID,
							RelatedUserID: rentalOrder.SalespersonID,
							ExpenseType:   "other",
							ReferenceID:   &rentalOrder.ID,
							ReferenceType: "rental_order_refund",
							Category:      "refund",
							Description:   description,
							Amount:        totalRefund,
							ExpenseDate:   refundDate,
							PaymentMethod: paymentMethod,
							BankAccountID: bankAccountID,
							Vendor:        vendor,
							Status:        "confirmed",
							Notes:         fmt.Sprintf("refund_note_index=%d", noteIndex),
							CreatedBy:     &userID,
							UpdatedBy:     &userID,
							CreatedAt:     time.Now(),
							UpdatedAt:     time.Now(),
							ExtraFields: models.JSONB(map[string]interface{}{
								"refund_number":   refundNumber,
								"rental_order_id": rentalOrder.ID.String(),
								"note_index":      noteIndex,
							}),
						}
						if exp.RelatedUserID == nil {
							exp.RelatedUserID = &userID
						}
						if err := database.DB.Create(&exp).Error; err == nil {
							note["expense_id"] = exp.ID.String()
						}
					}
				}

				req.ExtraFields["refund_notes"] = notesList
				rentalOrder.ExtraFields = models.JSONB(req.ExtraFields)
				database.DB.Model(&rentalOrder).Update("extra_fields", rentalOrder.ExtraFields)
			}
		}
	}

	// 狀態轉 completed 時，發放介紹人積分
	if oldStatus != "completed" && rentalOrder.Status == "completed" && rentalOrder.CustomerID != nil {
		refCode := rentalOrder.ReferralCode
		if refCode == "" {
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *rentalOrder.CustomerID).First(&customer).Error; err == nil {
				refCode = customer.ReferralCode
			}
		}
		if refCode != "" {
			var ps models.PointSetting
			if err := database.DB.Where("tenant_id = ?", tenantID).First(&ps).Error; err == nil {
				if ps.EnableServiceOrderReferralBonus {
					processReferralBonus(tenantID, refCode, *rentalOrder.CustomerID, "出租單", rentalOrder.ID, rentalOrder.OrderNumber, rentalOrder.PointsEarned)
				}
			} else {
				processReferralBonus(tenantID, refCode, *rentalOrder.CustomerID, "出租單", rentalOrder.ID, rentalOrder.OrderNumber, rentalOrder.PointsEarned)
			}
		}
	}

	// 狀態轉 confirmed 時寄送出租單確認 email
	if oldStatus != "confirmed" && rentalOrder.Status == "confirmed" {
		emailTo := strings.TrimSpace(rentalOrder.ContactEmail)
		customerName := strings.TrimSpace(rentalOrder.ContactName)
		customerID := uuid.Nil
		if rentalOrder.CustomerID != nil {
			customerID = *rentalOrder.CustomerID
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *rentalOrder.CustomerID).First(&customer).Error; err == nil {
				if emailTo == "" {
					emailTo = strings.TrimSpace(customer.Email)
				}
				if customerName == "" {
					customerName = strings.TrimSpace(customer.Name)
				}
			}
		}
		if emailTo != "" {
			if customerName == "" {
				customerName = "客戶"
			}
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				rentalDateStr := rentalOrder.RentalDate.Format("2006-01-02")
				if err := email.EnqueueOrderConfirmationEmail(
					tenantID,
					tenant.Subdomain,
					customerID,
					emailTo,
					customerName,
					rentalOrder.OrderNumber,
					rentalDateStr,
					rentalOrder.TotalAmount,
					"rental_order",
				); err != nil {
					log.Printf("Failed to enqueue rental order confirmation email: %v", err)
				}
			}
		}
	}

	// 重新載入關聯數據
	database.DB.Where("id = ?", rentalOrder.ID).
		Preload("Customer").Preload("RentalOrderItems.Staff").
		Preload("Salesperson").Preload("Store").Preload("Labels").Preload("Appointments").
		First(&rentalOrder)

	// Auto-refresh business goals related to rental orders
	// go RefreshActiveBusinessGoals(tenantID, []string{"rental_order_count"})

	return c.JSON(rentalOrder)
}

func DeleteRentalOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.RentalOrder{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Auto-refresh business goals related to rental orders
	// go RefreshActiveBusinessGoals(tenantID, []string{"rental_order_count"})

	return c.JSON(fiber.Map{"message": "Rental order deleted"})
}
