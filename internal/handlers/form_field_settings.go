package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 用戶表單欄位設定 API
// ============================================

// GetUserFormFieldSettings 獲取租戶的表單欄位設定
// GET /user/form-field-settings/:pageName
func GetUserFormFieldSettings(c *fiber.Ctx) error {
	fmt.Fprintf(os.Stderr, "[FieldSettings][GET] === HANDLER CALLED === path=%s\n", c.Path())
	tenantID := middleware.GetTenantID(c)
	pageName := c.Params("pageName")
	fmt.Fprintf(os.Stderr, "[FieldSettings][GET] tenant=%s page=%s\n", tenantID, pageName)

	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	if pageName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Page name is required"})
	}

	var settings models.UserFormFieldSettings
	err := database.DB.Where("tenant_id = ? AND page_name = ?", tenantID, pageName).First(&settings).Error
	if err != nil {
		// 如果不存在，返回空設定
		return c.JSON(fiber.Map{
			"field_config": fiber.Map{
				"fields":      []interface{}{},
				"extraFields": []interface{}{},
			},
		})
	}

	// Debug: log extraFields count when loading
	if ef, ok := settings.FieldConfig["extraFields"]; ok {
		if arr, ok := ef.([]interface{}); ok {
			log.Printf("[FieldSettings][GET] page=%s tenant=%s extraFields count=%d", pageName, tenantID, len(arr))
		}
	}

	return c.JSON(fiber.Map{
		"id":           settings.ID,
		"page_name":    settings.PageName,
		"field_config": settings.FieldConfig,
	})
}

// SaveUserFormFieldSettings 保存租戶表單欄位設定
// POST /user/form-field-settings/:pageName
func SaveUserFormFieldSettings(c *fiber.Ctx) error {
	fmt.Fprintf(os.Stderr, "[FieldSettings][SAVE] === HANDLER CALLED === path=%s method=%s\n", c.Path(), c.Method())
	tenantID := middleware.GetTenantID(c)
	pageName := c.Params("pageName")
	fmt.Fprintf(os.Stderr, "[FieldSettings][SAVE] tenant=%s page=%s bodyLen=%d\n", tenantID, pageName, len(c.Body()))
	fmt.Fprintf(os.Stderr, "[FieldSettings][SAVE] FULL BODY: %s\n", string(c.Body()))

	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	if pageName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Page name is required"})
	}

	// 解析請求體
	var input struct {
		FieldConfig json.RawMessage `json:"field_config"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// 解析 field_config
	var fieldConfig map[string]interface{}
	if err := json.Unmarshal(input.FieldConfig, &fieldConfig); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid field_config format"})
	}

	// 驗證必要欄位
	if _, ok := fieldConfig["fields"]; !ok {
		fieldConfig["fields"] = []interface{}{}
	}
	if _, ok := fieldConfig["extraFields"]; !ok {
		fieldConfig["extraFields"] = []interface{}{}
	}

	// Debug: log incoming extraFields
	if ef, ok := fieldConfig["extraFields"]; ok {
		if arr, ok := ef.([]interface{}); ok {
			log.Printf("[FieldSettings][SAVE] page=%s tenant=%s incoming extraFields count=%d", pageName, tenantID, len(arr))
			for i, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					log.Printf("[FieldSettings][SAVE]   extraField[%d]: key=%v label=%v type=%v", i, m["key"], m["label"], m["type"])
				}
			}
		}
	}

	// 轉換為 JSONB
	fieldConfigJSONB := models.JSONB(fieldConfig)

	// 序列化為 JSON string（用於 UPDATE 時的顯式 ::jsonb 轉型）
	fieldConfigJSON, jsonErr := json.Marshal(fieldConfig)
	if jsonErr != nil {
		log.Printf("[FieldSettings][SAVE] JSON marshal failed: %v", jsonErr)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to serialize field_config"})
	}

	// 查找或創建設定
	var settings models.UserFormFieldSettings
	err := database.DB.Where("tenant_id = ? AND page_name = ?", tenantID, pageName).First(&settings).Error
	if err != nil {
		// 創建新記錄 — 使用原始 SQL 確保 jsonb 類型正確
		newID := uuid.New()
		if err := database.DB.Exec(
			`INSERT INTO user_form_field_settings (id, tenant_id, page_name, field_config, created_at, updated_at)
			 VALUES (?, ?, ?, ?::jsonb, NOW(), NOW())`,
			newID, tenantID, pageName, string(fieldConfigJSON),
		).Error; err != nil {
			log.Printf("[FieldSettings][SAVE] Create FAILED: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save form field settings"})
		}
		settings.ID = newID
		settings.TenantID = tenantID
		settings.PageName = pageName
		settings.FieldConfig = fieldConfigJSONB
		log.Printf("[FieldSettings][SAVE] Created new record id=%s", newID)
	} else {
		// 更新現有記錄 — 使用原始 SQL 確保 jsonb 類型正確
		// pgx v5 中 []byte 會被當成 bytea，導致 GORM 的 JSONB.Value() 寫入失敗
		// 用 string + ::jsonb 顯式轉型繞過此問題
		if err := database.DB.Exec(
			"UPDATE user_form_field_settings SET field_config = ?::jsonb, updated_at = NOW() WHERE id = ?",
			string(fieldConfigJSON), settings.ID,
		).Error; err != nil {
			log.Printf("[FieldSettings][SAVE] Update FAILED: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update form field settings"})
		}
		// 更新本地 copy 以返回正確的值
		settings.FieldConfig = fieldConfigJSONB
		log.Printf("[FieldSettings][SAVE] Updated existing record id=%s", settings.ID)
	}

	// 驗證：從 DB 重新讀取，確認 extraFields 已正確保存
	// 同時用 DB 的資料作為 response（而不是本地 copy），確保前端拿到的是 DB 實際狀態
	var verify models.UserFormFieldSettings
	if verifyErr := database.DB.Where("id = ?", settings.ID).First(&verify).Error; verifyErr == nil {
		if ef, ok := verify.FieldConfig["extraFields"]; ok {
			if arr, ok := ef.([]interface{}); ok {
				log.Printf("[FieldSettings][VERIFY] After save, DB has extraFields count=%d", len(arr))
			} else {
				log.Printf("[FieldSettings][VERIFY] After save, extraFields exists but not array: %T", ef)
			}
		} else {
			log.Printf("[FieldSettings][VERIFY] After save, DB has NO extraFields key!")
		}
		// 使用 DB 重新讀取的資料返回
		settings = verify
	} else {
		log.Printf("[FieldSettings][VERIFY] Re-read failed: %v", verifyErr)
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"id":           settings.ID,
		"page_name":    settings.PageName,
		"field_config": settings.FieldConfig,
	})
}

// DeleteUserFormFieldSettings 刪除租戶表單欄位設定（重置為預設）
// DELETE /user/form-field-settings/:pageName
func DeleteUserFormFieldSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	pageName := c.Params("pageName")

	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	if pageName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Page name is required"})
	}

	result := database.DB.Where("tenant_id = ? AND page_name = ?", tenantID, pageName).Delete(&models.UserFormFieldSettings{})
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete form field settings"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Form field settings reset to default",
	})
}
