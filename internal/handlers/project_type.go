package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetProjectTypes 獲取項目類型列表
func GetProjectTypes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var types []models.ProjectType
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ProjectType{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&types).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch project types: %v", err)})
	}

	return c.JSON(fiber.Map{"data": types, "total": total, "page": page, "limit": limit})
}

// GetProjectType 獲取單個項目類型
func GetProjectType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project type ID"})
	}

	var t models.ProjectType
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&t).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project type not found"})
	}
	return c.JSON(t)
}

// CreateProjectType 創建項目類型
func CreateProjectType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Name        string                  `json:"name"`
		Color       string                  `json:"color"`
		Status      string                  `json:"status"`
		ExtraFields map[string]interface{}  `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	now := utils.NowInTenantTimezone(tenantID)
	t := models.ProjectType{
		TenantID:  tenantID,
		Name:      req.Name,
		Color:     req.Color,
		Status:    req.Status,
		CreatedBy: &userID,
		UpdatedBy: &userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if t.Status == "" {
		t.Status = "active"
	}
	if t.Color == "" {
		t.Color = "#6366f1"
	}
	if req.ExtraFields != nil {
		t.ExtraFields = models.JSONB(req.ExtraFields)
	}

	if err := database.DB.Create(&t).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create project type: %v", err)})
	}
	return c.Status(201).JSON(t)
}

// UpdateProjectType 更新項目類型
func UpdateProjectType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project type ID"})
	}

	var t models.ProjectType
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&t).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project type not found"})
	}

	var req struct {
		Name        *string                 `json:"name"`
		Color       *string                 `json:"color"`
		Status      *string                 `json:"status"`
		ExtraFields map[string]interface{}  `json:"extra_fields"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Name != nil {
		if *req.Name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Name cannot be empty"})
		}
		t.Name = *req.Name
	}
	if req.Color != nil {
		t.Color = *req.Color
	}
	if req.Status != nil {
		t.Status = *req.Status
	}
	if req.ExtraFields != nil {
		t.ExtraFields = models.JSONB(req.ExtraFields)
	}

	t.UpdatedBy = &userID
	t.UpdatedAt = utils.NowInTenantTimezone(tenantID)

	if err := database.DB.Save(&t).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update project type: %v", err)})
	}
	return c.JSON(t)
}

// DeleteProjectType 刪除項目類型
func DeleteProjectType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project type ID"})
	}

	var t models.ProjectType
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&t).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Project type not found"})
	}
	if err := database.DB.Delete(&t).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete project type: %v", err)})
	}
	return c.JSON(fiber.Map{"message": "Project type deleted"})
}


