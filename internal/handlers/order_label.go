package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetOrderLabels 獲取訂單標籤列表
func GetOrderLabels(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var labels []models.OrderLabel
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 搜索過濾
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 100)
	offset := (page - 1) * limit

	var total int64
	if err := query.Model(&models.OrderLabel{}).Count(&total).Error; err != nil {
		// 如果表不存在，返回空列表而不是错误
		if err.Error() == "relation \"order_labels\" does not exist" {
			return c.JSON(fiber.Map{
				"data":  []models.OrderLabel{},
				"total": 0,
				"page":  page,
				"limit": limit,
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to count order labels: " + err.Error()})
	}

	if err := query.Offset(offset).Limit(limit).Order("name ASC").Find(&labels).Error; err != nil {
		// 如果表不存在，返回空列表而不是错误
		if err.Error() == "relation \"order_labels\" does not exist" {
			return c.JSON(fiber.Map{
				"data":  []models.OrderLabel{},
				"total": 0,
				"page":  page,
				"limit": limit,
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch order labels: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  labels,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetOrderLabel 獲取單個訂單標籤
func GetOrderLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var label models.OrderLabel
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order label not found"})
	}

	return c.JSON(label)
}

// CreateOrderLabel 創建訂單標籤
func CreateOrderLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		Status string `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Label name is required"})
	}

	if req.Color == "" {
		req.Color = "#007bff"
	}

	status := req.Status
	if status == "" {
		status = "active"
	}

	// 檢查名稱是否已存在
	var existingLabel models.OrderLabel
	if err := database.DB.Where("tenant_id = ? AND name = ?", tenantID, req.Name).First(&existingLabel).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Label name already exists"})
	}

	label := models.OrderLabel{
		TenantID: tenantID,
		Name:     req.Name,
		Color:    req.Color,
		Status:   status,
	}

	if err := database.DB.Create(&label).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create order label"})
	}

	return c.Status(201).JSON(label)
}

// UpdateOrderLabel 更新訂單標籤
func UpdateOrderLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var label models.OrderLabel
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order label not found"})
	}

	var req struct {
		Name   *string `json:"name"`
		Color  *string `json:"color"`
		Status *string `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name != nil {
		// 檢查名稱是否已被其他標籤使用
		var existingLabel models.OrderLabel
		if err := database.DB.Where("tenant_id = ? AND name = ? AND id != ?", tenantID, *req.Name, id).First(&existingLabel).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Label name already exists"})
		}
		label.Name = *req.Name
	}

	if req.Color != nil {
		label.Color = *req.Color
	}

	if req.Status != nil {
		label.Status = *req.Status
	}

	if err := database.DB.Save(&label).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update order label"})
	}

	return c.JSON(label)
}

// DeleteOrderLabel 刪除訂單標籤
func DeleteOrderLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var label models.OrderLabel
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order label not found"})
	}

	// 刪除所有關聯
	database.DB.Where("label_id = ?", id).Delete(&models.OrderLabelRelation{})

	// 刪除標籤
	if err := database.DB.Delete(&label).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete order label"})
	}

	return c.JSON(fiber.Map{"message": "Order label deleted successfully"})
}

