package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetCustomerLabels 獲取客戶標籤列表
func GetCustomerLabels(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var labels []models.CustomerLabel
	query := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 100)
	offset := (page - 1) * limit

	var total int64
	if err := query.Model(&models.CustomerLabel{}).Count(&total).Error; err != nil {
		if err.Error() == "relation \"customer_labels\" does not exist" {
			return c.JSON(fiber.Map{
				"data":  []models.CustomerLabel{},
				"total": 0,
				"page":  page,
				"limit": limit,
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to count customer labels: " + err.Error()})
	}

	if err := query.Offset(offset).Limit(limit).Order("name ASC").Find(&labels).Error; err != nil {
		if err.Error() == "relation \"customer_labels\" does not exist" {
			return c.JSON(fiber.Map{
				"data":  []models.CustomerLabel{},
				"total": 0,
				"page":  page,
				"limit": limit,
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch customer labels: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  labels,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetCustomerLabel 獲取單個客戶標籤
func GetCustomerLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var label models.CustomerLabel
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Customer label not found"})
	}

	return c.JSON(label)
}

// CreateCustomerLabel 創建客戶標籤
func CreateCustomerLabel(c *fiber.Ctx) error {
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

	var existingLabel models.CustomerLabel
	if err := database.DB.Where("tenant_id = ? AND name = ? AND trashed_at IS NULL", tenantID, req.Name).First(&existingLabel).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Label name already exists"})
	}

	label := models.CustomerLabel{
		TenantID: tenantID,
		Name:     req.Name,
		Color:    req.Color,
		Status:   status,
	}

	if err := database.DB.Create(&label).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create customer label"})
	}

	return c.Status(201).JSON(label)
}

// UpdateCustomerLabel 更新客戶標籤
func UpdateCustomerLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var label models.CustomerLabel
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Customer label not found"})
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
		var existingLabel models.CustomerLabel
		if err := database.DB.Where("tenant_id = ? AND name = ? AND id != ? AND trashed_at IS NULL", tenantID, *req.Name, id).First(&existingLabel).Error; err == nil {
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
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update customer label"})
	}

	return c.JSON(label)
}

// DeleteCustomerLabel 刪除客戶標籤
func DeleteCustomerLabel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid label ID"})
	}

	var label models.CustomerLabel
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).First(&label).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Customer label not found"})
	}

	database.DB.Where("label_id = ?", id).Delete(&models.CustomerLabelRelation{})

	if err := database.DB.Delete(&label).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete customer label"})
	}

	return c.JSON(fiber.Map{"message": "Customer label deleted successfully"})
}
