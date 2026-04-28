package handlers

import (
	"encoding/json"
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 優惠券 (Coupon) CRUD
// ============================================

func GetCoupons(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var coupons []models.Coupon
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("LOWER(code) LIKE ? OR LOWER(code) LIKE ?", "%"+strings.ToLower(search)+"%", "%"+strings.ToLower(search)+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if couponType := c.Query("coupon_type"); couponType != "" {
		query = query.Where("coupon_type = ?", couponType)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Coupon{}).Count(&total)

	if err := query.Preload("Conditions").Preload("Conditions.Product").Preload("Conditions.MemberLevel").Preload("Conditions.Customer").
		Offset(offset).Limit(limit).Order("created_at DESC").Find(&coupons).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  coupons,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetCoupon(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	// 防止路由衝突：如果 id 是 "validate"，說明路由匹配錯誤
	if id == "validate" || id == "usage" {
		return c.Status(404).JSON(fiber.Map{"error": "Invalid coupon ID"})
	}

	// 驗證 id 是否為有效的 UUID
	if _, err := uuid.Parse(id); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid coupon ID format"})
	}

	var coupon models.Coupon
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&coupon).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Coupon not found"})
	}

	return c.JSON(coupon)
}

func CreateCoupon(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var coupon models.Coupon
	if err := c.BodyParser(&coupon); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	coupon.TenantID = tenantID

	// 檢查優惠券代碼是否已存在
	var existing models.Coupon
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, coupon.Code).First(&existing).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Coupon code already exists"})
	}

	if err := database.DB.Create(&coupon).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(coupon)
}

func UpdateCoupon(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var coupon models.Coupon

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&coupon).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Coupon not found"})
	}

	var req models.Coupon
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 如果更新了代碼，檢查是否與其他優惠券衝突
	if req.Code != "" && req.Code != coupon.Code {
		var existing models.Coupon
		if err := database.DB.Where("tenant_id = ? AND code = ? AND id != ?", tenantID, req.Code, id).First(&existing).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Coupon code already exists"})
		}
		coupon.Code = req.Code
	}

	coupon.Name = req.Name
	coupon.Description = req.Description
	coupon.CouponType = req.CouponType
	coupon.DiscountValue = req.DiscountValue
	coupon.MinPurchase = req.MinPurchase
	coupon.MaxDiscount = req.MaxDiscount
	coupon.ValidFrom = req.ValidFrom
	// ValidTo 為 nil 或空時表示一直有效
	coupon.ValidTo = req.ValidTo
	coupon.UsageLimit = req.UsageLimit
	coupon.CustomerLimit = req.CustomerLimit
	coupon.MemberLevelID = req.MemberLevelID
	coupon.MinProductQuantity = req.MinProductQuantity
	coupon.MinProductAmount = req.MinProductAmount
	coupon.Status = req.Status
	coupon.ExtraFields = req.ExtraFields

	if err := database.DB.Save(&coupon).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(coupon)
}

func DeleteCoupon(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Coupon{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Coupon deleted"})
}

// ValidateCoupon 驗證優惠券是否可用
func ValidateCoupon(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	code := c.Query("code")
	amount := c.Query("amount")          // 訂單金額
	customerID := c.Query("customer_id") // 可選，用於檢查客戶使用次數

	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Coupon code is required"})
	}

	var coupon models.Coupon
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, code).Preload("Conditions").Preload("Conditions.Product").Preload("Conditions.MemberLevel").Preload("Conditions.Customer").First(&coupon).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Coupon not found"})
	}

	// 檢查狀態
	if coupon.Status != "active" {
		return c.Status(400).JSON(fiber.Map{"error": "Coupon is not active"})
	}

	// 檢查有效期
	now := time.Now()
	if coupon.ValidFrom.After(now) {
		return c.Status(400).JSON(fiber.Map{"error": "Coupon is not valid at this time"})
	}
	// 如果 ValidTo 為空（nil），表示一直有效
	if coupon.ValidTo != nil && coupon.ValidTo.Before(now) {
		return c.Status(400).JSON(fiber.Map{"error": "Coupon is not valid at this time"})
	}

	// 檢查最低消費
	if amount != "" {
		orderAmount, _ := strconv.ParseFloat(amount, 64)
		if orderAmount < coupon.MinPurchase {
			return c.Status(400).JSON(fiber.Map{"error": "Order amount is less than minimum purchase"})
		}
	}

	// 檢查使用次數限制
	if coupon.UsageLimit != nil && coupon.UsedCount >= *coupon.UsageLimit {
		return c.Status(400).JSON(fiber.Map{"error": "Coupon usage limit reached"})
	}

	// 檢查客戶使用次數限制
	if customerID != "" && coupon.CustomerLimit != nil {
		var usedCount int64
		database.DB.Model(&models.Order{}).Where("tenant_id = ? AND customer_id = ? AND coupon_id = ?", tenantID, customerID, coupon.ID).Count(&usedCount)
		if int(usedCount) >= *coupon.CustomerLimit {
			return c.Status(400).JSON(fiber.Map{"error": "Customer usage limit reached"})
		}
	}

	// 檢查會員等級限制
	if coupon.MemberLevelID != nil && customerID != "" {
		var customer models.Customer
		if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, customerID).First(&customer).Error; err == nil {
			if customer.MemberLevelID == nil || *customer.MemberLevelID != *coupon.MemberLevelID {
				return c.Status(400).JSON(fiber.Map{"error": "Coupon is not available for this member level"})
			}
		}
	}

	// 解析訂單項目（如果提供）
	var orderItems []map[string]interface{}
	if itemsJSON := c.Query("items"); itemsJSON != "" {
		// 從 JSON 字符串解析訂單項目
		if err := json.Unmarshal([]byte(itemsJSON), &orderItems); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid order items format"})
		}
	}

	// 驗證購物車產品數量和金額限制
	if err := validateCartRequirements(&coupon, orderItems); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// 驗證多條件匹配
	if err := validateCouponConditions(&coupon, orderItems, customerID, tenantID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// 計算折扣金額
	var discountAmount float64
	orderAmount, _ := strconv.ParseFloat(amount, 64)
	if coupon.CouponType == "percentage" {
		discountAmount = orderAmount * coupon.DiscountValue / 100
		if coupon.MaxDiscount != nil && discountAmount > *coupon.MaxDiscount {
			discountAmount = *coupon.MaxDiscount
		}
	} else if coupon.CouponType == "fixed_amount" {
		discountAmount = coupon.DiscountValue
		if discountAmount > orderAmount {
			discountAmount = orderAmount
		}
	} else if coupon.CouponType == "free_shipping" {
		// 免運費，這裡假設運費為0或從其他地方獲取
		discountAmount = 0
	}

	return c.JSON(fiber.Map{
		"valid":           true,
		"coupon":          coupon,
		"discount_amount": discountAmount,
	})
}

// ============================================
// 積分設置 (PointSetting) CRUD
// ============================================

func GetPointSetting(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var setting models.PointSetting

	if err := database.DB.Where("tenant_id = ?", tenantID).First(&setting).Error; err != nil {
		// 如果不存在，創建默認設置
		setting = models.PointSetting{
			TenantID:            tenantID,
			EarnPointsEnabled:   true,
			PointsPerDollar:     1.00,
			DollarPerPoint:      0.01,
			MinPointsToUse:      0,
			ReferralBonusMode:   "fixed",
			ReferralBonusValue:  0,
			ReferralCountPolicy: "all",
			EnableOrderEarnPoints: true,
			EnableServiceOrderEarnPoints: true,
			EnableOrderReferralBonus: true,
			EnableServiceOrderReferralBonus: true,
			Status:              "active",
			ExtraFields:         make(models.JSONB),
		}
		if err := database.DB.Create(&setting).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 向後兼容：若新欄位還沒回填，優先用舊欄位補一份
	if setting.ReferralBonusMode == "" {
		setting.ReferralBonusMode = "fixed"
	}
	if setting.ReferralBonusValue == 0 && setting.ReferralBonusPoints > 0 && setting.ReferralBonusMode == "fixed" {
		setting.ReferralBonusValue = float64(setting.ReferralBonusPoints)
	}

	return c.JSON(setting)
}

func UpdatePointSetting(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var req models.PointSetting
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	var setting models.PointSetting
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&setting).Error; err != nil {
		// 如果不存在，創建新設置
		setting = models.PointSetting{
			TenantID:            tenantID,
			ReferralBonusMode:   "fixed",
			ReferralCountPolicy: "all",
			EnableOrderEarnPoints: true,
			EnableServiceOrderEarnPoints: true,
			EnableOrderReferralBonus: true,
			EnableServiceOrderReferralBonus: true,
			EnablePointsAdjustmentOnEdit: true,
		}
	}

	setting.PointsPerDollar = req.PointsPerDollar
	setting.DollarPerPoint = req.DollarPerPoint
	setting.MinPointsToUse = req.MinPointsToUse
	setting.MaxPointsPercent = req.MaxPointsPercent
	setting.EarnPointsEnabled = req.EarnPointsEnabled
	// 介紹人獎勵：新欄位為主，舊欄位向後兼容
	if req.ReferralBonusMode != "" {
		setting.ReferralBonusMode = req.ReferralBonusMode
	}
	setting.ReferralBonusValue = req.ReferralBonusValue
	setting.ReferralBonusPoints = req.ReferralBonusPoints
	if setting.ReferralBonusMode == "fixed" {
		// fixed 模式下，確保舊欄位仍同步（避免舊前端/舊邏輯讀不到）
		if setting.ReferralBonusValue > 0 {
			setting.ReferralBonusPoints = int(setting.ReferralBonusValue + 0.5)
		}
	}
	setting.ReferralCountPolicy = req.ReferralCountPolicy
	setting.EnableOrderEarnPoints = req.EnableOrderEarnPoints
	setting.EnableServiceOrderEarnPoints = req.EnableServiceOrderEarnPoints
	setting.EnableOrderReferralBonus = req.EnableOrderReferralBonus
	setting.EnableServiceOrderReferralBonus = req.EnableServiceOrderReferralBonus
	setting.EnablePointsAdjustmentOnEdit = req.EnablePointsAdjustmentOnEdit
	setting.Status = req.Status
	setting.ExtraFields = req.ExtraFields

	if setting.ID == uuid.Nil {
		if err := database.DB.Create(&setting).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	} else {
		if err := database.DB.Save(&setting).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}

	return c.JSON(setting)
}

// GetCouponUsage 獲取優惠券使用記錄
func GetCouponUsage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	couponID := c.Params("id")

	var orders []models.Order
	query := database.DB.Where("tenant_id = ? AND coupon_id = ?", tenantID, couponID)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Order{}).Count(&total)

	if err := query.Preload("Customer").Offset(offset).Limit(limit).Order("created_at DESC").Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  orders,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// validateCartRequirements 驗證購物車產品數量和金額要求
func validateCartRequirements(coupon *models.Coupon, orderItems []map[string]interface{}) error {
	if len(orderItems) == 0 {
		// 如果沒有提供訂單項目，跳過驗證
		return nil
	}

	// 計算購物車總產品數量和金額
	var totalQuantity int
	var totalAmount float64

	for _, item := range orderItems {
		if qty, ok := item["quantity"].(float64); ok {
			totalQuantity += int(qty)
		}
		if price, ok := item["unit_price"].(float64); ok {
			if qty, ok := item["quantity"].(float64); ok {
				totalAmount += price * qty
			}
		}
	}

	// 檢查最低產品數量
	if coupon.MinProductQuantity != nil && totalQuantity < *coupon.MinProductQuantity {
		return fmt.Errorf("購物車產品數量不足，需要至少 %d 件", *coupon.MinProductQuantity)
	}

	// 檢查最低產品金額
	if coupon.MinProductAmount != nil && totalAmount < *coupon.MinProductAmount {
		return fmt.Errorf("購物車產品金額不足，需要至少 $%.2f", *coupon.MinProductAmount)
	}

	return nil
}

// validateCouponConditions 驗證優惠券多條件匹配
func validateCouponConditions(coupon *models.Coupon, orderItems []map[string]interface{}, customerID string, tenantID uuid.UUID) error {
	if len(coupon.Conditions) == 0 {
		return nil // 沒有條件，直接通過
	}

	// 檢查客戶信息（如果需要）
	var customer *models.Customer
	if customerID != "" {
		var c models.Customer
		if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, customerID).First(&c).Error; err == nil {
			customer = &c
		}
	}

	// 檢查是否有 OR 條件（任一滿足即可）
	hasOrCondition := false
	for _, condition := range coupon.Conditions {
		if condition.MatchType == "or" {
			hasOrCondition = true
			break
		}
	}

	if hasOrCondition {
		// OR 模式：至少一個條件滿足即可
		anyMatched := false
		for _, condition := range coupon.Conditions {
			if checkCondition(&condition, orderItems, customer) {
				anyMatched = true
				break
			}
		}
		if !anyMatched {
			return fmt.Errorf("優惠券條件不滿足")
		}
	} else {
		// AND 模式：所有條件都必須滿足
		for _, condition := range coupon.Conditions {
			if !checkCondition(&condition, orderItems, customer) {
				return fmt.Errorf("優惠券條件不滿足：%s", getConditionDescription(&condition))
			}
		}
	}

	return nil
}

// checkCondition 檢查單個條件是否滿足
func checkCondition(condition *models.CouponCondition, orderItems []map[string]interface{}, customer *models.Customer) bool {
	switch condition.ConditionType {
	case "product_quantity":
		// 檢查特定產品數量
		if condition.ProductID == nil || condition.Quantity == nil {
			return false
		}
		productID := condition.ProductID.String()
		var totalQty int
		for _, item := range orderItems {
			if itemProductID, ok := item["product_id"].(string); ok && itemProductID == productID {
				if qty, ok := item["quantity"].(float64); ok {
					totalQty += int(qty)
				}
			}
		}
		return totalQty >= *condition.Quantity

	case "product_amount":
		// 檢查特定產品金額
		if condition.ProductID == nil || condition.Amount == nil {
			return false
		}
		productID := condition.ProductID.String()
		var totalAmount float64
		for _, item := range orderItems {
			if itemProductID, ok := item["product_id"].(string); ok && itemProductID == productID {
				if price, ok := item["unit_price"].(float64); ok {
					if qty, ok := item["quantity"].(float64); ok {
						totalAmount += price * qty
					}
				}
			}
		}
		return totalAmount >= *condition.Amount

	case "member_level":
		// 檢查會員等級
		if condition.MemberLevelID == nil || customer == nil {
			return false
		}
		return customer.MemberLevelID != nil && *customer.MemberLevelID == *condition.MemberLevelID

	case "customer":
		// 檢查特定客戶
		if condition.CustomerID == nil || customer == nil {
			return false
		}
		return customer.ID == *condition.CustomerID

	default:
		return false
	}
}

// getConditionDescription 獲取條件描述
func getConditionDescription(condition *models.CouponCondition) string {
	switch condition.ConditionType {
	case "product_quantity":
		if condition.Product != nil && condition.Quantity != nil {
			return fmt.Sprintf("需要購買 %s 至少 %d 件", condition.Product.Name, *condition.Quantity)
		}
	case "product_amount":
		if condition.Product != nil && condition.Amount != nil {
			return fmt.Sprintf("需要購買 %s 至少 $%.2f", condition.Product.Name, *condition.Amount)
		}
	case "member_level":
		if condition.MemberLevel != nil {
			return fmt.Sprintf("需要會員等級：%s", condition.MemberLevel.Name)
		}
	case "customer":
		if condition.Customer != nil {
			return fmt.Sprintf("僅限客戶：%s", condition.Customer.Name)
		}
	}
	return "條件不滿足"
}
