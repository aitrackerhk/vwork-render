package handlers

import (
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// POS銷售 (POSSale) CRUD
// ============================================

func GetPOSSales(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID := c.Query("customer_id")
	staffID := c.Query("staff_id")

	var sales []models.POSSale
	query := database.DB.Where("tenant_id = ?", tenantID)

	if customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}
	if staffID != "" {
		query = query.Where("staff_id = ?", staffID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if startDate := c.Query("start_date"); startDate != "" {
		query = query.Where("sale_date >= ?", startDate)
	}
	if endDate := c.Query("end_date"); endDate != "" {
		query = query.Where("sale_date <= ?", endDate)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.POSSale{}).Count(&total)

	if err := query.Preload("Customer").Preload("Staff").
		Offset(offset).Limit(limit).Order("sale_date DESC").
		Find(&sales).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  sales,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetPOSSale(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var sale models.POSSale

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Customer").Preload("Staff").Preload("POSSaleItems").
		Preload("POSSaleItems.Product").Preload("POSSaleItems.Service").
		Preload("POSPayments").Preload("POSPayments.Currency").
		First(&sale).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "POS sale not found"})
	}

	return c.JSON(sale)
}

func CreatePOSSale(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	staffID := middleware.GetUserID(c)

	var req struct {
		CustomerID     *uuid.UUID `json:"customer_id"`
		StaffID        *uuid.UUID `json:"staff_id"`
		SaleNumber     string     `json:"sale_number"`
		SaleDate       string     `json:"sale_date"`
		Subtotal       float64    `json:"subtotal"`
		DiscountAmount float64    `json:"discount_amount"`
		TaxAmount      float64    `json:"tax_amount"`
		TotalAmount    float64    `json:"total_amount"`
		Status         string     `json:"status"`
		Notes          string     `json:"notes"`
		POSSaleItems   []struct {
			ProductID      *uuid.UUID `json:"product_id"`
			ServiceID      *uuid.UUID `json:"service_id"`
			ItemName       string     `json:"item_name"`
			Quantity       int        `json:"quantity"`
			UnitPrice      float64    `json:"unit_price"`
			DiscountAmount float64    `json:"discount_amount"`
			TaxRate        float64    `json:"tax_rate"`
			TotalAmount    float64    `json:"total_amount"`
		} `json:"pos_sale_items"`
		ExtraFields map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 解析日期（使用租戶時區）
	var saleDate time.Time
	if req.SaleDate != "" {
		if parsed, err := utils.ParseDateTimeInTenantTimezone(tenantID, req.SaleDate); err == nil {
			saleDate = parsed
		} else {
			saleDate = utils.NowInTenantTimezone(tenantID)
		}
	} else {
		saleDate = utils.NowInTenantTimezone(tenantID)
	}

	// 生成銷售號
	if req.SaleNumber == "" {
		req.SaleNumber = "POS-" + time.Now().Format("20060102150405")
	}

	// 檢查銷售號是否已存在
	var existingSale models.POSSale
	if err := database.DB.Where("tenant_id = ? AND sale_number = ?", tenantID, req.SaleNumber).First(&existingSale).Error; err == nil {
		// 如果已存在，重新生成
		req.SaleNumber = "POS-" + time.Now().Format("20060102150405")
	}

	if req.Status == "" {
		// POS 建單預設應為「已確認」，不是「已完成」
		req.Status = "confirmed"
	}

	sale := models.POSSale{
		TenantID:       tenantID,
		CustomerID:     req.CustomerID,
		StaffID:        req.StaffID,
		SaleNumber:     req.SaleNumber,
		SaleDate:       saleDate,
		Subtotal:       req.Subtotal,
		DiscountAmount: req.DiscountAmount,
		TaxAmount:      req.TaxAmount,
		TotalAmount:    req.TotalAmount,
		Status:         req.Status,
		Notes:          req.Notes,
		ExtraFields:    models.JSONB(req.ExtraFields),
	}

	if sale.StaffID == nil {
		sale.StaffID = &staffID
	}

	// 計算積分（如果有客戶且積分設置存在）
	var pointsEarned int = 0
	if req.CustomerID != nil {
		var pointSetting models.PointSetting
		if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
			// 檢查是否開啟訂單消費獲得積分（POS 視為訂單）
			if pointSetting.EnableOrderEarnPoints && pointSetting.PointsPerDollar > 0 {
				// 計算實際支付金額（總金額 - 折扣）
				actualAmount := req.TotalAmount - req.DiscountAmount
				if actualAmount > 0 {
					pointsEarned = int(actualAmount * pointSetting.PointsPerDollar)
				}
			}
		}
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(&sale).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create POS sale"})
	}

	// 創建銷售明細
	for _, itemReq := range req.POSSaleItems {
		saleItem := models.POSSaleItem{
			POSSaleID:      sale.ID,
			ProductID:      itemReq.ProductID,
			ServiceID:      itemReq.ServiceID,
			ItemName:       itemReq.ItemName,
			Quantity:       itemReq.Quantity,
			UnitPrice:      itemReq.UnitPrice,
			DiscountAmount: itemReq.DiscountAmount,
			TaxRate:        itemReq.TaxRate,
			TotalAmount:    itemReq.TotalAmount,
		}
		if err := tx.Create(&saleItem).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create POS sale item"})
		}
	}

	// 如果客戶存在且獲得了積分，創建積分記錄並更新客戶總積分
	if req.CustomerID != nil && pointsEarned > 0 {
		// 創建積分記錄
		point := models.Point{
			TenantID:    tenantID,
			CustomerID:  *req.CustomerID,
			Points:      pointsEarned,
			PointsType:  "earned",
			SourceType:  "pos_sale",
			SourceID:    &sale.ID,
			Description: fmt.Sprintf("POS銷售 %s 消費獲得積分", sale.SaleNumber),
			Status:      "active",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := tx.Create(&point).Error; err != nil {
			// 記錄錯誤但不影響POS銷售創建
			log.Printf("Failed to create point record: %v", err)
		} else {
			// 更新客戶總積分
			var customer models.Customer
			if err := tx.Where("id = ?", *req.CustomerID).First(&customer).Error; err == nil {
				customer.TotalPoints += pointsEarned
				tx.Save(&customer)
			}
		}
	}

	tx.Commit()

	// 處理介紹人積分獎勵（僅在 POS 完成時）
	if sale.CustomerID != nil && sale.Status == "completed" {
		var customer models.Customer
		if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, *sale.CustomerID).First(&customer).Error; err == nil {
			if customer.ReferralCode != "" {
				// 檢查是否開啟訂單介紹人獎勵積分（POS 視為訂單）
				var pointSetting models.PointSetting
				if err := database.DB.Where("tenant_id = ?", tenantID).First(&pointSetting).Error; err == nil {
					if pointSetting.EnableOrderReferralBonus {
						processReferralBonus(tenantID, customer.ReferralCode, customer.ID, "POS", sale.ID, sale.SaleNumber, pointsEarned)
					}
				} else {
					// 如果沒有設定，預設開啟（向後兼容）
					processReferralBonus(tenantID, customer.ReferralCode, customer.ID, "POS", sale.ID, sale.SaleNumber, pointsEarned)
				}
			}
		}
	}

	// 處理會員等級自動升級（檢查所有啟用自動升級的等級，按順序檢查直到不滿足條件）
	if sale.CustomerID != nil {
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			var hasAutoUpgrade bool
			database.DB.Model(&models.MemberLevel{}).
				Where("tenant_id = ? AND auto_upgrade = ? AND status = ?", tenantID, true, "active").
				Select("COUNT(*) > 0").
				Scan(&hasAutoUpgrade)

			if hasAutoUpgrade {
				checkAndUpgradeMemberLevel(tenantID, *sale.CustomerID, sale.TotalAmount)
			}
		}
	}

	// 發送 POS 銷售確認 email（如果銷售完成且有客戶 email）
	if sale.CustomerID != nil && sale.Status == "completed" {
		var customer models.Customer
		if err := database.DB.Where("id = ?", *sale.CustomerID).First(&customer).Error; err == nil {
			if customer.Email != "" {
				var tenant models.Tenant
				if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
					saleDate := sale.SaleDate.Format("2006-01-02")
					if err := email.EnqueueOrderConfirmationEmail(
						tenantID,
						tenant.Subdomain,
						customer.ID,
						customer.Email,
						customer.Name,
						sale.SaleNumber,
						saleDate,
						sale.TotalAmount,
						"pos_sale",
					); err != nil {
						log.Printf("Failed to enqueue POS sale confirmation email: %v", err)
					}
				}
			}
		}
	}

	// 重新載入關聯數據
	database.DB.Where("id = ?", sale.ID).
		Preload("Customer").Preload("Staff").Preload("POSSaleItems").
		Preload("POSSaleItems.Product").Preload("POSSaleItems.Service").
		First(&sale)

	return c.Status(201).JSON(sale)
}

func UpdatePOSSale(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var sale models.POSSale

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&sale).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "POS sale not found"})
	}

	var req struct {
		CustomerID     *uuid.UUID `json:"customer_id"`
		StaffID        *uuid.UUID `json:"staff_id"`
		SaleNumber     *string    `json:"sale_number"`
		SaleDate       *string    `json:"sale_date"`
		Subtotal       *float64   `json:"subtotal"`
		DiscountAmount *float64   `json:"discount_amount"`
		TaxAmount      *float64   `json:"tax_amount"`
		TotalAmount    *float64   `json:"total_amount"`
		Status         *string    `json:"status"`
		Notes          *string    `json:"notes"`
		POSSaleItems   []struct {
			ProductID      *uuid.UUID `json:"product_id"`
			ServiceID      *uuid.UUID `json:"service_id"`
			ItemName       string     `json:"item_name"`
			Quantity       int        `json:"quantity"`
			UnitPrice      float64    `json:"unit_price"`
			DiscountAmount float64    `json:"discount_amount"`
			TaxRate        float64    `json:"tax_rate"`
			TotalAmount    float64    `json:"total_amount"`
		} `json:"pos_sale_items"`
		ExtraFields *map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.CustomerID != nil {
		sale.CustomerID = req.CustomerID
	}
	if req.StaffID != nil {
		sale.StaffID = req.StaffID
	}
	if req.SaleNumber != nil {
		sale.SaleNumber = *req.SaleNumber
	}
	if req.SaleDate != nil {
		if parsed, err := utils.ParseDateTimeInTenantTimezone(tenantID, *req.SaleDate); err == nil {
			sale.SaleDate = parsed
		}
	}
	if req.Subtotal != nil {
		sale.Subtotal = *req.Subtotal
	}
	if req.DiscountAmount != nil {
		sale.DiscountAmount = *req.DiscountAmount
	}
	if req.TaxAmount != nil {
		sale.TaxAmount = *req.TaxAmount
	}
	if req.TotalAmount != nil {
		sale.TotalAmount = *req.TotalAmount
	}
	if req.Status != nil {
		sale.Status = *req.Status
	}
	if req.Notes != nil {
		sale.Notes = *req.Notes
	}
	if req.ExtraFields != nil {
		sale.ExtraFields = models.JSONB(*req.ExtraFields)
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 更新銷售明細
	if req.POSSaleItems != nil {
		// 刪除現有明細
		tx.Where("pos_sale_id = ?", sale.ID).Delete(&models.POSSaleItem{})

		// 創建新明細
		for _, itemReq := range req.POSSaleItems {
			saleItem := models.POSSaleItem{
				POSSaleID:      sale.ID,
				ProductID:      itemReq.ProductID,
				ServiceID:      itemReq.ServiceID,
				ItemName:       itemReq.ItemName,
				Quantity:       itemReq.Quantity,
				UnitPrice:      itemReq.UnitPrice,
				DiscountAmount: itemReq.DiscountAmount,
				TaxRate:        itemReq.TaxRate,
				TotalAmount:    itemReq.TotalAmount,
			}
			if err := tx.Create(&saleItem).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": "Failed to update POS sale item"})
			}
		}
	}

	sale.TenantID = tenantID

	if err := tx.Save(&sale).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	tx.Commit()

	// 重新載入關聯數據
	database.DB.Where("id = ?", sale.ID).
		Preload("Customer").Preload("Staff").Preload("POSSaleItems").
		Preload("POSSaleItems.Product").Preload("POSSaleItems.Service").
		First(&sale)

	return c.JSON(sale)
}

func DeletePOSSale(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.POSSale{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "POS sale deleted"})
}

// ============================================
// POS銷售明細 (POSSaleItem) CRUD
// ============================================

func GetPOSSaleItems(c *fiber.Ctx) error {
	posSaleID := c.Query("pos_sale_id")

	var items []models.POSSaleItem
	query := database.DB

	if posSaleID != "" {
		query = query.Where("pos_sale_id = ?", posSaleID)
	}

	if err := query.Preload("POSSale").Preload("Product").Preload("Service").Find(&items).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": items})
}

func CreatePOSSaleItem(c *fiber.Ctx) error {
	var item models.POSSaleItem
	if err := c.BodyParser(&item); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 計算總金額
	item.TotalAmount = float64(item.Quantity)*item.UnitPrice - item.DiscountAmount

	if err := database.DB.Create(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新POS銷售總金額
	updatePOSSaleTotal(item.POSSaleID)

	return c.Status(201).JSON(item)
}

func UpdatePOSSaleItem(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.POSSaleItem

	if err := database.DB.First(&item, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "POS sale item not found"})
	}

	oldPOSSaleID := item.POSSaleID

	if err := c.BodyParser(&item); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 計算總金額
	item.TotalAmount = float64(item.Quantity)*item.UnitPrice - item.DiscountAmount

	if err := database.DB.Save(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新POS銷售總金額
	updatePOSSaleTotal(item.POSSaleID)
	if oldPOSSaleID != item.POSSaleID {
		updatePOSSaleTotal(oldPOSSaleID)
	}

	return c.JSON(item)
}

func DeletePOSSaleItem(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.POSSaleItem

	if err := database.DB.First(&item, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "POS sale item not found"})
	}

	posSaleID := item.POSSaleID

	if err := database.DB.Delete(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 更新POS銷售總金額
	updatePOSSaleTotal(posSaleID)

	return c.JSON(fiber.Map{"message": "POS sale item deleted"})
}

// updatePOSSaleTotal 更新POS銷售總金額
func updatePOSSaleTotal(posSaleID uuid.UUID) {
	var subtotal float64
	database.DB.Model(&models.POSSaleItem{}).
		Where("pos_sale_id = ?", posSaleID).
		Select("COALESCE(SUM(total_amount), 0)").
		Scan(&subtotal)

	var sale models.POSSale
	if err := database.DB.First(&sale, "id = ?", posSaleID).Error; err != nil {
		return
	}

	totalAmount := subtotal - sale.DiscountAmount + sale.TaxAmount

	database.DB.Model(&models.POSSale{}).
		Where("id = ?", posSaleID).
		Updates(map[string]interface{}{
			"subtotal":     subtotal,
			"total_amount": totalAmount,
		})
}

// ============================================
// POS支付 (POSPayment) CRUD
// ============================================

func GetPOSPayments(c *fiber.Ctx) error {
	posSaleID := c.Query("pos_sale_id")

	var payments []models.POSPayment
	query := database.DB

	if posSaleID != "" {
		query = query.Where("pos_sale_id = ?", posSaleID)
	}

	if err := query.Preload("POSSale").Preload("Currency").Find(&payments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": payments})
}

func GetPOSPayment(c *fiber.Ctx) error {
	id := c.Params("id")
	var payment models.POSPayment

	if err := database.DB.Preload("POSSale").Preload("Currency").First(&payment, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "POS payment not found"})
	}

	return c.JSON(payment)
}

func CreatePOSPayment(c *fiber.Ctx) error {
	var payment models.POSPayment
	if err := c.BodyParser(&payment); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Create(&payment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(payment)
}

func UpdatePOSPayment(c *fiber.Ctx) error {
	id := c.Params("id")
	var payment models.POSPayment

	if err := database.DB.First(&payment, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "POS payment not found"})
	}

	if err := c.BodyParser(&payment); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Save(&payment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(payment)
}

func DeletePOSPayment(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := database.DB.Delete(&models.POSPayment{}, "id = ?", id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "POS payment deleted"})
}

// ============================================
// 訂單報表 (OrderReport) CRUD
// ============================================

func GetOrderReports(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	reportType := c.Query("report_type")
	reportDate := c.Query("report_date")

	var reports []models.OrderReport
	query := database.DB.Where("tenant_id = ?", tenantID)

	if reportType != "" {
		query = query.Where("report_type = ?", reportType)
	}
	if reportDate != "" {
		query = query.Where("report_date = ?", reportDate)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.OrderReport{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("report_date DESC").Find(&reports).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  reports,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetOrderReport(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var report models.OrderReport

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&report).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order report not found"})
	}

	return c.JSON(report)
}

func CreateOrderReport(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var report models.OrderReport
	if err := c.BodyParser(&report); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	report.TenantID = tenantID

	if err := database.DB.Create(&report).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(report)
}

func UpdateOrderReport(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var report models.OrderReport

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&report).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order report not found"})
	}

	if err := c.BodyParser(&report); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	report.TenantID = tenantID

	if err := database.DB.Save(&report).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(report)
}

func DeleteOrderReport(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.OrderReport{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Order report deleted"})
}
