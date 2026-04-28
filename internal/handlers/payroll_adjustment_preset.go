package handlers

import (
	"fmt"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetPayrollAdjustmentPresets 列表
func GetPayrollAdjustmentPresets(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if limit <= 0 {
		limit = 20
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := (page - 1) * limit

	q := strings.TrimSpace(c.Query("search"))
	status := strings.TrimSpace(c.Query("status"))

	query := database.DB.Model(&models.PayrollAdjustmentPreset{}).Where("tenant_id = ?", tenantID)
	if q != "" {
		query = query.Where("name ILIKE ?", "%"+q+"%")
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Count(&total)

	var items []models.PayrollAdjustmentPreset
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch payroll adjustment presets: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetPayrollAdjustmentPreset(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var item models.PayrollAdjustmentPreset
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&item).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Preset not found"})
	}
	return c.JSON(item)
}

func CreatePayrollAdjustmentPreset(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		Name        string  `json:"name"`
		Direction   string  `json:"direction"`
		Mode        string  `json:"mode"`
		RatePercent float64 `json:"rate_percent"`
		Amount      float64 `json:"amount"`
		Status      string  `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if req.Direction != "add" && req.Direction != "subtract" {
		req.Direction = "add"
	}
	if req.Mode != "fixed" && req.Mode != "percent" {
		req.Mode = "fixed"
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Status != "active" && req.Status != "inactive" {
		req.Status = "active"
	}

	item := models.PayrollAdjustmentPreset{
		TenantID:    tenantID,
		Name:        req.Name,
		Direction:   req.Direction,
		Mode:        req.Mode,
		RatePercent: req.RatePercent,
		Amount:      req.Amount,
		Status:      req.Status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := database.DB.Create(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create preset"})
	}
	return c.Status(201).JSON(item)
}

func UpdatePayrollAdjustmentPreset(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var item models.PayrollAdjustmentPreset
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&item).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Preset not found"})
	}

	var req struct {
		Name        string  `json:"name"`
		Direction   string  `json:"direction"`
		Mode        string  `json:"mode"`
		RatePercent float64 `json:"rate_percent"`
		Amount      float64 `json:"amount"`
		Status      string  `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name != "" {
		item.Name = req.Name
	}
	if req.Direction == "add" || req.Direction == "subtract" {
		item.Direction = req.Direction
	}
	if req.Mode == "fixed" || req.Mode == "percent" {
		item.Mode = req.Mode
	}
	item.RatePercent = req.RatePercent
	item.Amount = req.Amount
	if req.Status == "active" || req.Status == "inactive" {
		item.Status = req.Status
	}
	item.UpdatedAt = time.Now()

	if err := database.DB.Save(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update preset"})
	}
	return c.JSON(item)
}

func DeletePayrollAdjustmentPreset(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.PayrollAdjustmentPreset{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete preset"})
	}
	return c.JSON(fiber.Map{"message": "Deleted"})
}


