package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
)

// GetCouponConditions 獲取優惠券條件列表
func GetCouponConditions(c *fiber.Ctx) error {
	couponID := c.Params("coupon_id")

	var conditions []models.CouponCondition
	if err := database.DB.Where("coupon_id = ?", couponID).
		Preload("Product").Preload("MemberLevel").Preload("Customer").
		Find(&conditions).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(conditions)
}

// CreateCouponCondition 創建優惠券條件
func CreateCouponCondition(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	couponID := c.Params("coupon_id")

	// 驗證 coupon 是否存在且屬於該租戶
	var coupon models.Coupon
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, couponID).First(&coupon).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Coupon not found"})
	}

	var condition models.CouponCondition
	if err := c.BodyParser(&condition); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	condition.CouponID = coupon.ID
	if condition.MatchType == "" {
		condition.MatchType = "and"
	}

	if err := database.DB.Create(&condition).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 重新載入關聯數據
	database.DB.Preload("Product").Preload("MemberLevel").Preload("Customer").First(&condition, condition.ID)

	return c.Status(201).JSON(condition)
}

// UpdateCouponCondition 更新優惠券條件
func UpdateCouponCondition(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	couponID := c.Params("coupon_id")
	conditionID := c.Params("id")

	// 驗證 coupon 是否存在且屬於該租戶
	var coupon models.Coupon
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, couponID).First(&coupon).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Coupon not found"})
	}

	var condition models.CouponCondition
	if err := database.DB.Where("coupon_id = ? AND id = ?", couponID, conditionID).First(&condition).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Condition not found"})
	}

	var req models.CouponCondition
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 更新字段
	if req.ConditionType != "" {
		condition.ConditionType = req.ConditionType
	}
	if req.ProductID != nil {
		condition.ProductID = req.ProductID
	}
	if req.Quantity != nil {
		condition.Quantity = req.Quantity
	}
	if req.Amount != nil {
		condition.Amount = req.Amount
	}
	if req.MemberLevelID != nil {
		condition.MemberLevelID = req.MemberLevelID
	}
	if req.CustomerID != nil {
		condition.CustomerID = req.CustomerID
	}
	if req.MatchType != "" {
		condition.MatchType = req.MatchType
	}

	if err := database.DB.Save(&condition).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 重新載入關聯數據
	database.DB.Preload("Product").Preload("MemberLevel").Preload("Customer").First(&condition, condition.ID)

	return c.JSON(condition)
}

// DeleteCouponCondition 刪除優惠券條件
func DeleteCouponCondition(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	couponID := c.Params("coupon_id")
	conditionID := c.Params("id")

	// 驗證 coupon 是否存在且屬於該租戶
	var coupon models.Coupon
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, couponID).First(&coupon).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Coupon not found"})
	}

	if err := database.DB.Where("coupon_id = ? AND id = ?", couponID, conditionID).Delete(&models.CouponCondition{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Condition deleted"})
}

