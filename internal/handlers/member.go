package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ============================================
// 會員等級 (MemberLevel) CRUD
// ============================================

func GetMemberLevels(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var memberLevels []models.MemberLevel
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.MemberLevel{}).Count(&total)

	// 按 is_default DESC 排序，確保默認值在第一個，然後按 level_order ASC 排序
	if err := query.Offset(offset).Limit(limit).Order("is_default DESC, level_order ASC").Find(&memberLevels).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  memberLevels,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetMemberLevel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var memberLevel models.MemberLevel

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&memberLevel).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Member level not found"})
	}

	return c.JSON(memberLevel)
}

func CreateMemberLevel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var memberLevel models.MemberLevel
	if err := c.BodyParser(&memberLevel); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	memberLevel.TenantID = tenantID

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置
	if memberLevel.IsDefault {
		if err := tx.Model(&models.MemberLevel{}).
			Where("tenant_id = ? AND is_default = ?", tenantID, true).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other member levels: " + err.Error()})
		}
	}

	if err := tx.Create(&memberLevel).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	return c.Status(201).JSON(memberLevel)
}

func UpdateMemberLevel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var memberLevel models.MemberLevel

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&memberLevel).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Member level not found"})
	}

	var req struct {
		Name              string   `json:"name"`
		Code              *string  `json:"code"`
		LevelOrder        int      `json:"level_order"`
		MinPoints         int      `json:"min_points"`
		MinPurchaseAmount float64  `json:"min_purchase_amount"`
		DiscountRate      float64  `json:"discount_rate"`
		IsDefault         bool     `json:"is_default"`
		AutoUpgrade       bool     `json:"auto_upgrade"`
		Description       string   `json:"description"`
		Status            string   `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault {
		if err := tx.Model(&models.MemberLevel{}).
			Where("tenant_id = ? AND is_default = ? AND id != ?", tenantID, true, id).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other member levels: " + err.Error()})
		}
	}

	memberLevel.Name = req.Name
	memberLevel.Code = req.Code
	memberLevel.LevelOrder = req.LevelOrder
	memberLevel.MinPoints = req.MinPoints
	memberLevel.MinPurchaseAmount = req.MinPurchaseAmount
	memberLevel.DiscountRate = req.DiscountRate
	memberLevel.IsDefault = req.IsDefault
	memberLevel.AutoUpgrade = req.AutoUpgrade
	memberLevel.Description = req.Description
	memberLevel.Status = req.Status
	memberLevel.TenantID = tenantID

	if err := tx.Save(&memberLevel).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	// 重新查詢以確保返回最新的數據（包括 is_default）
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&memberLevel).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload member level"})
	}

	return c.JSON(memberLevel)
}

func DeleteMemberLevel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.MemberLevel{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Member level deleted"})
}

// ============================================
// 積分 (Point) CRUD
// ============================================

func GetPoints(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID := c.Query("customer_id")
	
	var points []models.Point
	query := database.DB.Where("tenant_id = ?", tenantID)

	if customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}

	if pointsType := c.Query("points_type"); pointsType != "" {
		query = query.Where("points_type = ?", pointsType)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Point{}).Count(&total)

	if err := query.Preload("Customer").Offset(offset).Limit(limit).Order("created_at DESC").Find(&points).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  points,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetPoint(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var point models.Point

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("Customer").First(&point).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Point not found"})
	}

	return c.JSON(point)
}

func CreatePoint(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var point models.Point
	if err := c.BodyParser(&point); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	point.TenantID = tenantID

	// 更新客戶總積分
	if point.PointsType == "earned" {
		var customer models.Customer
		if err := database.DB.First(&customer, "id = ?", point.CustomerID).Error; err == nil {
			customer.TotalPoints += point.Points
			database.DB.Save(&customer)
		}
	} else if point.PointsType == "redeemed" {
		var customer models.Customer
		if err := database.DB.First(&customer, "id = ?", point.CustomerID).Error; err == nil {
			customer.TotalPoints -= point.Points
			if customer.TotalPoints < 0 {
				customer.TotalPoints = 0
			}
			database.DB.Save(&customer)
		}
	}

	if err := database.DB.Create(&point).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(point)
}

func DeletePoint(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	
	var point models.Point
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&point).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Point not found"})
	}

	// 回滾客戶積分
	if point.PointsType == "earned" {
		var customer models.Customer
		if err := database.DB.First(&customer, "id = ?", point.CustomerID).Error; err == nil {
			customer.TotalPoints -= point.Points
			if customer.TotalPoints < 0 {
				customer.TotalPoints = 0
			}
			database.DB.Save(&customer)
		}
	} else if point.PointsType == "redeemed" {
		var customer models.Customer
		if err := database.DB.First(&customer, "id = ?", point.CustomerID).Error; err == nil {
			customer.TotalPoints += point.Points
			database.DB.Save(&customer)
		}
	}

	if err := database.DB.Delete(&point).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Point deleted"})
}

// ============================================
// 介紹記錄 (Referral Records) CRUD
// ============================================

// GetReferrals 獲取介紹記錄列表（查詢所有有介紹人的客戶）
func GetReferrals(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	
	// 查詢有referral_code的客戶（即有介紹人的客戶）
	var customers []models.Customer
	query := database.DB.Where("tenant_id = ? AND referral_code != '' AND referral_code IS NOT NULL", tenantID)

	// 搜索過濾
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ? OR referral_code ILIKE ?", 
			"%"+search+"%", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// 分頁
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Customer{}).Count(&total)

	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&customers).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 構建返回數據
	type ReferralRecord struct {
		ID           string    `json:"id"`
		CustomerName string    `json:"customer_name"`
		ReferralCode string    `json:"referral_code"`
		ReferrerName string    `json:"referrer_name"`
		CreatedAt    time.Time `json:"created_at"`
	}

	var records []ReferralRecord
	for _, customer := range customers {
		// 查找介紹人（通過 referral_code 匹配介紹人的 code）
		var referrer models.Customer
		referrerName := ""
		if customer.ReferralCode != "" {
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, customer.ReferralCode).
				First(&referrer).Error; err == nil {
				referrerName = referrer.Name
			}
		}

		records = append(records, ReferralRecord{
			ID:           customer.ID.String(),
			CustomerName: customer.Name,
			ReferralCode: customer.ReferralCode,
			ReferrerName: referrerName,
			CreatedAt:    customer.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  records,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

