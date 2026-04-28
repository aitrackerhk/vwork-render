package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetBlocks 獲取區塊列表
func GetBlocks(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var blocks []models.Block

	query := database.DB.Where("tenant_id = ?", tenantID)

	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	// 元件類型過濾
	if componentType := c.Query("component_type"); componentType != "" {
		query = query.Where("component_type = ?", componentType)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 100)
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.Block{}).Where("tenant_id = ?", tenantID).Count(&total)

	if err := query.
		Offset(offset).Limit(limit).
		Order("created_at DESC").
		Find(&blocks).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch blocks: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  blocks,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetBlock 獲取單個區塊
func GetBlock(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	blockID := c.Params("id")

	var block models.Block
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, blockID).
		First(&block).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Block not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch block: %v", err)})
	}

	return c.JSON(block)
}

// CreateBlock 創建區塊
func CreateBlock(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Name          string                 `json:"name"`
		ComponentType string                 `json:"component_type"`
		ComponentData map[string]interface{} `json:"component_data"`
		ExtraFields   map[string]interface{} `json:"extra_fields,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// 驗證必填字段
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if req.ComponentType == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Component type is required"})
	}

	now := time.Now()
	block := models.Block{
		TenantID:      tenantID,
		Name:          req.Name,
		ComponentType: req.ComponentType,
		CreatedBy:     &userID,
		UpdatedBy:     &userID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if req.ComponentData != nil {
		block.ComponentData = models.JSONB(req.ComponentData)
	} else {
		block.ComponentData = make(models.JSONB)
	}

	if req.ExtraFields != nil {
		block.ExtraFields = models.JSONB(req.ExtraFields)
	} else {
		block.ExtraFields = make(models.JSONB)
	}

	if err := database.DB.Create(&block).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create block: %v", err)})
	}

	return c.Status(201).JSON(block)
}

// UpdateBlock 更新區塊
func UpdateBlock(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	blockID := c.Params("id")

	var req struct {
		Name          string                 `json:"name,omitempty"`
		ComponentType string                 `json:"component_type,omitempty"`
		ComponentData map[string]interface{} `json:"component_data,omitempty"`
		ExtraFields   map[string]interface{} `json:"extra_fields,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	var block models.Block
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, blockID).First(&block).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Block not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch block: %v", err)})
	}

	// 更新字段
	if req.Name != "" {
		block.Name = req.Name
	}
	if req.ComponentType != "" {
		block.ComponentType = req.ComponentType
	}
	if req.ComponentData != nil {
		block.ComponentData = models.JSONB(req.ComponentData)
	}
	if req.ExtraFields != nil {
		block.ExtraFields = models.JSONB(req.ExtraFields)
	}

	block.UpdatedBy = &userID
	block.UpdatedAt = time.Now()

	if err := database.DB.Save(&block).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update block: %v", err)})
	}

	return c.JSON(block)
}

// DeleteBlock 刪除區塊
func DeleteBlock(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	blockID := c.Params("id")

	var block models.Block
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, blockID).First(&block).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Block not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch block: %v", err)})
	}

	if err := database.DB.Delete(&block).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete block: %v", err)})
	}

	return c.JSON(fiber.Map{"message": "Block deleted successfully"})
}

