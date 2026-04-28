package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// ============================================
// 角色 (Role) CRUD（原級別）
// ============================================

func GetRoles(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var roles []models.Role
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Role{}).Count(&total)

	// 使用 Select 明確指定要查詢的字段，避免 GORM 嘗試查詢不存在的 role_order 字段
	if err := query.Select("id", "tenant_id", "name", "description", "permissions", "status", "created_at", "updated_at").
		Offset(offset).Limit(limit).Order("name ASC").Find(&roles).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  roles,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetRole(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)
	var role models.Role

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&role).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Role not found"})
	}

	return c.JSON(role)
}

func CreateRole(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
		Status      string   `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// admin role 為系統保留角色：不能透過 API 建立（避免重名/覆蓋）
	if req.Name == "admin" || req.Name == "Admin" || req.Name == "ADMIN" {
		return c.Status(400).JSON(fiber.Map{"error": "Admin role is reserved and cannot be created"})
	}

	role := models.Role{
		TenantID:    tenantID,
		Name:        req.Name,
		Description: req.Description,
		Permissions: models.StringArrayJSONB(req.Permissions),
		Status:      req.Status,
	}

	if err := database.DB.Create(&role).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(role)
}

func UpdateRole(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)
	var role models.Role

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&role).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Role not found"})
	}

	// admin role 為預設角色，不能更改（避免被清空/覆蓋權限）
	if role.Name == "admin" || role.Name == "Admin" || role.Name == "ADMIN" {
		return c.Status(400).JSON(fiber.Map{"error": "Admin role is a default role and cannot be modified"})
	}

	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
		Status      string   `json:"status"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 避免把其他角色改名成 admin
	if req.Name == "admin" || req.Name == "Admin" || req.Name == "ADMIN" {
		return c.Status(400).JSON(fiber.Map{"error": "Admin role name is reserved and cannot be used"})
	}

	role.Name = req.Name
	role.Description = req.Description
	role.Permissions = models.StringArrayJSONB(req.Permissions)
	role.Status = req.Status

	if err := database.DB.Save(&role).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(role)
}

func DeleteRole(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)

	// 檢查是否為 admin role（預設角色，不能刪除）
	var role models.Role
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&role).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Role not found"})
	}

	// admin role 是預設角色，不能刪除
	if role.Name == "admin" || role.Name == "Admin" || role.Name == "ADMIN" {
		return c.Status(400).JSON(fiber.Map{"error": "Admin role is a default role and cannot be deleted"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.Role{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Role deleted"})
}
