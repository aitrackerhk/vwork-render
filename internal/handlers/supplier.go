package handlers

import (
	"log"
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetSuppliers 獲取供應商列表
func GetSuppliers(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		log.Printf("GetSuppliers: Tenant ID is nil")
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var suppliers []models.Supplier
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 構建查詢條件
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	// 計算總數
	var total int64
	if err := query.Model(&models.Supplier{}).Count(&total).Error; err != nil {
		log.Printf("GetSuppliers: Count error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to count suppliers", "details": err.Error()})
	}

	// 查詢數據
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&suppliers).Error; err != nil {
		log.Printf("GetSuppliers: Find error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch suppliers", "details": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  suppliers,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetSupplier 獲取單個供應商
func GetSupplier(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier ID"})
	}

	var supplier models.Supplier
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&supplier).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Supplier not found"})
	}

	return c.JSON(supplier)
}

// CreateSupplier 創建供應商
func CreateSupplier(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Code             string                 `json:"code"`
		Name             string                 `json:"name"`
		LastName         string                 `json:"last_name"`
		Email            string                 `json:"email"`
		Phone            string                 `json:"phone"`
		PhoneCountryCode string                 `json:"phone_country_code"`
		Address          string                 `json:"address"`
		Status           string                 `json:"status"`
		ExtraFields      map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if req.Status == "" {
		req.Status = "active"
	}

	now := time.Now()

	// 自動生成供應商編號（如果未提供）
	autoCode, err := generateAutoCode(tenantID, req.Code, autoCodeConfig{
		Prefix:     "SUP-",
		FieldName:  "code",
		PageName:   "suppliers",
		Format:     codeFormatDate,
		TableModel: &models.Supplier{},
		Column:     "code",
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate supplier code: " + err.Error()})
	}

	supplier := models.Supplier{
		TenantID:         tenantID,
		Code:             autoCode,
		Name:             req.Name,
		LastName:         req.LastName,
		Email:            req.Email,
		Phone:            req.Phone,
		PhoneCountryCode: req.PhoneCountryCode,
		Address:          req.Address,
		Status:           req.Status,
		CreatedBy:        &userID,
		UpdatedBy:        &userID,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExtraFields:      models.JSONB(req.ExtraFields),
	}

	if err := database.DB.Create(&supplier).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create supplier"})
	}

	// 成功建立後釋放預留編號
	releaseReservedCode(tenantID, "code", supplier.Code)

	return c.Status(201).JSON(supplier)
}

// UpdateSupplier 更新供應商
func UpdateSupplier(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier ID"})
	}

	var supplier models.Supplier
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&supplier).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Supplier not found"})
	}

	var req struct {
		Code             *string                 `json:"code"`
		Name             *string                 `json:"name"`
		LastName         *string                 `json:"last_name"`
		Email            *string                 `json:"email"`
		Phone            *string                 `json:"phone"`
		PhoneCountryCode *string                 `json:"phone_country_code"`
		Address          *string                 `json:"address"`
		Status           *string                 `json:"status"`
		ExtraFields      *map[string]interface{} `json:"extra_fields"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Code != nil {
		supplier.Code = *req.Code
	}
	if req.Name != nil {
		supplier.Name = *req.Name
	}
	if req.LastName != nil {
		supplier.LastName = *req.LastName
	}
	if req.Email != nil {
		supplier.Email = *req.Email
	}
	if req.Phone != nil {
		supplier.Phone = *req.Phone
	}
	if req.PhoneCountryCode != nil {
		supplier.PhoneCountryCode = *req.PhoneCountryCode
	}
	if req.Address != nil {
		supplier.Address = *req.Address
	}
	if req.Status != nil {
		supplier.Status = *req.Status
	}
	if req.ExtraFields != nil {
		supplier.ExtraFields = models.JSONB(*req.ExtraFields)
	}

	supplier.UpdatedBy = &userID
	supplier.UpdatedAt = time.Now()

	if err := database.DB.Save(&supplier).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update supplier"})
	}
	return c.JSON(supplier)
}

// DeleteSupplier 刪除供應商
func DeleteSupplier(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid supplier ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.Supplier{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete supplier"})
	}
	return c.JSON(fiber.Map{"message": "Supplier deleted successfully"})
}
