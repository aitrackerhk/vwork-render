package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetPurchaseOrderLabels 獲取採購標籤列表
func GetPurchaseOrderLabels(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var labels []models.PurchaseOrderLabel
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
	if err := query.Model(&models.PurchaseOrderLabel{}).Count(&total).Error; err != nil {
		// 如果表不存在，返回空列表而不是错误
		if err.Error() == "relation \"purchase_order_labels\" does not exist" {
			return c.JSON(fiber.Map{
				"data":  []models.PurchaseOrderLabel{},
				"total": 0,
				"page":  page,
				"limit": limit,
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to count purchase order labels: " + err.Error()})
	}

	if err := query.Offset(offset).Limit(limit).Order("name ASC").Find(&labels).Error; err != nil {
		// 如果表不存在，返回空列表而不是错误
		if err.Error() == "relation \"purchase_order_labels\" does not exist" {
			return c.JSON(fiber.Map{
				"data":  []models.PurchaseOrderLabel{},
				"total": 0,
				"page":  page,
				"limit": limit,
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch purchase order labels: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  labels,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetPurchaseOrderLabel 獲取單個採購標籤
func GetPurchaseOrderLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var label models.PurchaseOrderLabel
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order label not found"})
	}

	return c.JSON(label)
}

// CreatePurchaseOrderLabel 創建採購標籤
func CreatePurchaseOrderLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		Name   string `json:"name"`
		Color  string `json:"color"`
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
	var existingLabel models.PurchaseOrderLabel
	if err := database.DB.Where("tenant_id = ? AND name = ?", tenantID, req.Name).First(&existingLabel).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Label name already exists"})
	}

	label := models.PurchaseOrderLabel{
		TenantID: tenantID,
		Name:     req.Name,
		Color:    req.Color,
		Status:   status,
	}

	if err := database.DB.Create(&label).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create purchase order label"})
	}

	return c.Status(201).JSON(label)
}

// UpdatePurchaseOrderLabel 更新採購標籤
func UpdatePurchaseOrderLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var req struct {
		Name   string `json:"name"`
		Color  string `json:"color"`
		Status string `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	var label models.PurchaseOrderLabel
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order label not found"})
	}

	// 如果名稱改變，檢查新名稱是否已存在
	if req.Name != "" && req.Name != label.Name {
		var existingLabel models.PurchaseOrderLabel
		if err := database.DB.Where("tenant_id = ? AND name = ? AND id != ?", tenantID, req.Name, id).First(&existingLabel).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Label name already exists"})
		}
		label.Name = req.Name
	}

	if req.Color != "" {
		label.Color = req.Color
	}

	if req.Status != "" {
		label.Status = req.Status
	}

	if err := database.DB.Save(&label).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update purchase order label"})
	}

	return c.JSON(label)
}

// DeletePurchaseOrderLabel 刪除採購標籤
func DeletePurchaseOrderLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var label models.PurchaseOrderLabel
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Purchase order label not found"})
	}

	// 刪除標籤關聯
	database.DB.Where("label_id = ?", id).Delete(&models.PurchaseOrderLabelRelation{})

	// 刪除標籤
	if err := database.DB.Delete(&label).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete purchase order label"})
	}

	return c.JSON(fiber.Map{"message": "Purchase order label deleted successfully"})
}
