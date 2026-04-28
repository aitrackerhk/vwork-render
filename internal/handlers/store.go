package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetStores 獲取店舖列表
func GetStores(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var stores []models.Store
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 搜索過濾
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Store{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&stores).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch stores: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  stores,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetStore 獲取單個店舖
func GetStore(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid store ID"})
	}

	var store models.Store
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&store).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Store not found"})
	}

	return c.JSON(store)
}

// CreateStore 創建店舖
func CreateStore(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Name             string                 `json:"name"`
		Code             string                 `json:"code"`
		Address          string                 `json:"address"`
		ImageURL         string                 `json:"image_url"`
		ContactPerson    string                 `json:"contact_person"`
		PhoneCountryCode string                 `json:"phone_country_code"`
		Phone            string                 `json:"phone"`
		Email            string                 `json:"email"`
		Status           string                 `json:"status"`
		ExtraFields      map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	// 自動生成店舖編號（如果未提供）
	autoCode, err := generateAutoCode(tenantID, req.Code, autoCodeConfig{
		Prefix:     "STORE-",
		FieldName:  "code",
		PageName:   "stores",
		Format:     codeFormatSeq,
		TableModel: &models.Store{},
		Column:     "code",
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate store code: " + err.Error()})
	}

	// 檢查 code 是否已存在
	var existingStore models.Store
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, autoCode).First(&existingStore).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Store code already exists"})
	}

	store := models.Store{
		TenantID:         tenantID,
		Name:             req.Name,
		Code:             autoCode,
		Address:          req.Address,
		ImageURL:         req.ImageURL,
		ContactPerson:    req.ContactPerson,
		PhoneCountryCode: req.PhoneCountryCode,
		Phone:            req.Phone,
		Email:            req.Email,
		Status:           req.Status,
		CreatedBy:        &userID,
		UpdatedBy:        &userID,
	}

	if req.Status == "" {
		store.Status = "active"
	}

	if req.ExtraFields != nil {
		store.ExtraFields = models.JSONB(req.ExtraFields)
	}

	if err := database.DB.Create(&store).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create store: %v", err)})
	}

	// 成功建立後釋放預留編號
	releaseReservedCode(tenantID, "code", store.Code)

	return c.Status(201).JSON(store)
}

// UpdateStore 更新店舖
func UpdateStore(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid store ID"})
	}

	var store models.Store
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&store).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Store not found"})
	}

	var req struct {
		Name             *string                `json:"name"`
		Code             *string                `json:"code"`
		Address          *string                `json:"address"`
		ImageURL         *string                `json:"image_url"`
		ContactPerson    *string                `json:"contact_person"`
		PhoneCountryCode *string                `json:"phone_country_code"`
		Phone            *string                `json:"phone"`
		Email            *string                `json:"email"`
		Status           *string                `json:"status"`
		ExtraFields      map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	if req.Name != nil {
		store.Name = *req.Name
	}
	if req.Code != nil {
		// 檢查 code 是否已被其他店舖使用
		var existingStore models.Store
		if err := database.DB.Where("tenant_id = ? AND code = ? AND id != ?", tenantID, *req.Code, id).First(&existingStore).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Store code already exists"})
		}
		store.Code = *req.Code
	}
	if req.Address != nil {
		store.Address = *req.Address
	}
	if req.ImageURL != nil {
		store.ImageURL = *req.ImageURL
	}
	if req.ContactPerson != nil {
		store.ContactPerson = *req.ContactPerson
	}
	if req.PhoneCountryCode != nil {
		store.PhoneCountryCode = *req.PhoneCountryCode
	}
	if req.Phone != nil {
		store.Phone = *req.Phone
	}
	if req.Email != nil {
		store.Email = *req.Email
	}
	if req.Status != nil {
		store.Status = *req.Status
	}
	if req.ExtraFields != nil {
		store.ExtraFields = models.JSONB(req.ExtraFields)
	}

	store.UpdatedBy = &userID

	if err := database.DB.Save(&store).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update store: %v", err)})
	}

	return c.JSON(store)
}

// DeleteStore 刪除店舖
func DeleteStore(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid store ID"})
	}

	var store models.Store
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&store).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Store not found"})
	}

	if err := database.DB.Delete(&store).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete store: %v", err)})
	}

	return c.JSON(fiber.Map{"message": "Store deleted successfully"})
}
