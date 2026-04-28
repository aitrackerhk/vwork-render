package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func containsStr(list []string, v string) bool {
	for _, it := range list {
		if it == v {
			return true
		}
	}
	return false
}

func normalizeDefaultInclude(input []string, allowed string) []string {
	// 只允許單一值（產品稅=order、服務稅=service_order）
	for _, it := range input {
		if strings.TrimSpace(it) == allowed {
			return []string{allowed}
		}
	}
	return []string{}
}

// ============================================
// 產品稅 (ProductTax) CRUD
// ============================================

func GetProductTaxes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var taxes []models.ProductTax
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ProductTax{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&taxes).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": taxes, "total": total, "page": page, "limit": limit})
}

func GetProductTax(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax ID"})
	}
	var tax models.ProductTax
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&tax).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tax not found"})
	}
	return c.JSON(tax)
}

func CreateProductTax(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var tax models.ProductTax
	if err := c.BodyParser(&tax); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if tax.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if tax.TaxMode == "" {
		tax.TaxMode = "percent"
	}
	if tax.Status == "" {
		tax.Status = "active"
	}
	tax.DefaultInclude = models.StringArrayJSONB(normalizeDefaultInclude([]string(tax.DefaultInclude), "order"))
	now := time.Now()
	tax.TenantID = tenantID
	tax.CreatedAt = now
	tax.UpdatedAt = now

	if err := database.DB.Create(&tax).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(tax)
}

func UpdateProductTax(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax ID"})
	}
	var existing models.ProductTax
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&existing).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tax not found"})
	}
	var req struct {
		Name           *string      `json:"name"`
		Code           *string      `json:"code"`
		TaxMode        *string      `json:"tax_mode"`
		TaxValue       *float64     `json:"tax_value"`
		DefaultInclude *[]string    `json:"default_include"`
		Status         *string      `json:"status"`
		ExtraFields    models.JSONB `json:"extra_fields"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Code != nil {
		existing.Code = req.Code
	}
	if req.TaxMode != nil && *req.TaxMode != "" {
		existing.TaxMode = *req.TaxMode
	}
	if req.TaxValue != nil {
		existing.TaxValue = *req.TaxValue
	}
	// 若前端有送 default_include（即使是 []），就更新（並 sanitize 成只允許 "order"）
	if req.DefaultInclude != nil {
		existing.DefaultInclude = models.StringArrayJSONB(normalizeDefaultInclude(*req.DefaultInclude, "order"))
	}
	if req.Status != nil && *req.Status != "" {
		existing.Status = *req.Status
	}
	if req.ExtraFields != nil {
		existing.ExtraFields = req.ExtraFields
	}
	existing.UpdatedAt = time.Now()

	if err := database.DB.Save(&existing).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(existing)
}

func DeleteProductTax(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax ID"})
	}
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.ProductTax{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Tax deleted"})
}

// ============================================
// 服務稅 (ServiceTax) CRUD
// ============================================

func GetServiceTaxes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var taxes []models.ServiceTax
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ServiceTax{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&taxes).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"data": taxes, "total": total, "page": page, "limit": limit})
}

func GetServiceTax(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax ID"})
	}
	var tax models.ServiceTax
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&tax).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tax not found"})
	}
	return c.JSON(tax)
}

func CreateServiceTax(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var tax models.ServiceTax
	if err := c.BodyParser(&tax); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if tax.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if tax.TaxMode == "" {
		tax.TaxMode = "percent"
	}
	if tax.Status == "" {
		tax.Status = "active"
	}
	tax.DefaultInclude = models.StringArrayJSONB(normalizeDefaultInclude([]string(tax.DefaultInclude), "service_order"))
	now := time.Now()
	tax.TenantID = tenantID
	tax.CreatedAt = now
	tax.UpdatedAt = now

	if err := database.DB.Create(&tax).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(tax)
}

func UpdateServiceTax(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax ID"})
	}
	var existing models.ServiceTax
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&existing).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tax not found"})
	}
	var req struct {
		Name           *string      `json:"name"`
		Code           *string      `json:"code"`
		TaxMode        *string      `json:"tax_mode"`
		TaxValue       *float64     `json:"tax_value"`
		DefaultInclude *[]string    `json:"default_include"`
		Status         *string      `json:"status"`
		ExtraFields    models.JSONB `json:"extra_fields"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Code != nil {
		existing.Code = req.Code
	}
	if req.TaxMode != nil && *req.TaxMode != "" {
		existing.TaxMode = *req.TaxMode
	}
	if req.TaxValue != nil {
		existing.TaxValue = *req.TaxValue
	}
	if req.DefaultInclude != nil {
		existing.DefaultInclude = models.StringArrayJSONB(normalizeDefaultInclude(*req.DefaultInclude, "service_order"))
	}
	if req.Status != nil && *req.Status != "" {
		existing.Status = *req.Status
	}
	if req.ExtraFields != nil {
		existing.ExtraFields = req.ExtraFields
	}
	existing.UpdatedAt = time.Now()

	if err := database.DB.Save(&existing).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(existing)
}

func DeleteServiceTax(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tax ID"})
	}
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.ServiceTax{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Tax deleted"})
}
