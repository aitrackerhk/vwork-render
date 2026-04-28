package handlers

import (
	"log"
	"net/http"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
)

// GetPhoneCountryCodes 取得區號清單（按租戶過濾）
func GetPhoneCountryCodes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var codes []models.PhoneCountryCode
	query := database.DB.Model(&models.PhoneCountryCode{}).Where("tenant_id = ?", tenantID)

	// 搜索過濾
	if search := c.Query("search"); search != "" {
		query = query.Where("code ILIKE ? OR name ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	// 計算總數
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// 查詢數據（按 is_default DESC 排序，確保默認值在第一個，然後按 code ASC 排序）
	if err := query.Order("is_default DESC, code ASC").Offset(offset).Limit(limit).Find(&codes).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  codes,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetPhoneCountryCode 取得單個區號
func GetPhoneCountryCode(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)
	var code models.PhoneCountryCode
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&code).Error; err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(code)
}

// SetDefaultPhoneCountryCode 設置預設區號（按租戶）
func SetDefaultPhoneCountryCode(c *fiber.Ctx) error {
	code := c.Params("code")
	tenantID := middleware.GetTenantID(c)
	if code == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "code is required"})
	}

	tx := database.DB.Begin()
	if err := tx.Model(&models.PhoneCountryCode{}).Where("tenant_id = ? AND is_default = TRUE", tenantID).Update("is_default", false).Error; err != nil {
		tx.Rollback()
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if err := tx.Model(&models.PhoneCountryCode{}).Where("tenant_id = ? AND code = ?", tenantID, code).Update("is_default", true).Error; err != nil {
		tx.Rollback()
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if err := tx.Commit().Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "default updated"})
}

// CreatePhoneCountryCode 新增區號（可供 CMS 後台使用）
func CreatePhoneCountryCode(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var body struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		IsDefault bool   `json:"is_default"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.Code == "" || body.Name == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "code and name required"})
	}

	// 檢查區碼是否已存在（同一租戶下）
	var existingCode models.PhoneCountryCode
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, body.Code).First(&existingCode).Error; err == nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "區碼已存在"})
	}

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置（同一租戶下）
	if body.IsDefault {
		if err := tx.Model(&models.PhoneCountryCode{}).
			Where("tenant_id = ? AND is_default = ?", tenantID, true).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update other phone codes: " + err.Error()})
		}
	}

	rec := models.PhoneCountryCode{
		TenantID:  tenantID,
		Code:      body.Code,
		Name:      body.Name,
		IsDefault: body.IsDefault,
	}
	if err := tx.Create(&rec).Error; err != nil {
		tx.Rollback()
		// 檢查是否為重複區碼錯誤
		if err.Error() == "pq: duplicate key value violates unique constraint \"phone_country_codes_code_key\"" ||
			err.Error() == "UNIQUE constraint failed: phone_country_codes.code" {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "區碼已存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	return c.Status(http.StatusCreated).JSON(rec)
}

// UpdatePhoneCountryCode 更新名稱和 is_default（不改 code）
func UpdatePhoneCountryCode(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)
	var code models.PhoneCountryCode
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&code).Error; err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}

	var body struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		IsDefault bool   `json:"is_default"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	// 如果 code 有變更，檢查是否與其他記錄重複（同一租戶下）
	if body.Code != "" && body.Code != code.Code {
		var existingCode models.PhoneCountryCode
		if err := database.DB.Where("tenant_id = ? AND code = ? AND id != ?", tenantID, body.Code, id).First(&existingCode).Error; err == nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "區碼已存在"})
		}
		code.Code = body.Code
	}

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置（同一租戶下）
	if body.IsDefault {
		if err := tx.Model(&models.PhoneCountryCode{}).
			Where("tenant_id = ? AND is_default = ? AND id != ?", tenantID, true, id).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update other phone codes: " + err.Error()})
		}
	}

	// 更新字段
	code.Name = body.Name
	code.IsDefault = body.IsDefault

	if err := tx.Save(&code).Error; err != nil {
		tx.Rollback()
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	// 重新查詢以確保返回最新的數據（包括 is_default）
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&code).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reload phone code"})
	}

	return c.JSON(code)
}

// DeletePhoneCountryCode 刪除區號（防呆：預設不可刪）
func DeletePhoneCountryCode(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)
	var rec models.PhoneCountryCode
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&rec).Error; err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	if rec.IsDefault {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "cannot delete default code"})
	}
	if err := database.DB.Delete(&rec).Error; err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
