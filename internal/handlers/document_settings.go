package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetDocumentSettings 獲取文件設定
func GetDocumentSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	docType := c.Query("document_type") // invoice, shipping_note, contract, style

	var settings models.DocumentSettings
	query := database.DB.Where("tenant_id = ?", tenantID)
	if docType != "" {
		query = query.Where("document_type = ?", docType)
	}

	if err := query.First(&settings).Error; err != nil {
		// 如果不存在，返回默認值
		return c.JSON(models.DocumentSettings{
			TenantID:     tenantID,
			DocumentType: docType,
			Terms:        "",
			Notes:        "",
		})
	}

	return c.JSON(settings)
}

// CreateOrUpdateDocumentSettings 創建或更新文件設定
func CreateOrUpdateDocumentSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		DocumentType  string  `json:"document_type"`
		Terms         string  `json:"terms"`
		Notes         string  `json:"notes"`
		LogoURL       *string `json:"logo_url,omitempty"`
		LogoWidth     float64 `json:"logo_width"`
		LogoHeight    float64 `json:"logo_height"`
		TitleFontSize float64 `json:"title_font_size"`
		BodyFontSize  float64 `json:"body_font_size"`
		NotesFontSize float64 `json:"notes_font_size"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	validTypes := map[string]bool{
		"invoice": true, "shipping_note": true, "contract": true, "style": true,
	}
	if !validTypes[req.DocumentType] {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid document_type. Must be 'invoice', 'shipping_note', 'contract', or 'style'"})
	}

	var settings models.DocumentSettings
	err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, req.DocumentType).First(&settings).Error

	if err != nil {
		// 創建新設定
		settings = models.DocumentSettings{
			TenantID:      tenantID,
			DocumentType:  req.DocumentType,
			Terms:         req.Terms,
			Notes:         req.Notes,
			LogoURL:       req.LogoURL,
			LogoWidth:     req.LogoWidth,
			LogoHeight:    req.LogoHeight,
			TitleFontSize: req.TitleFontSize,
			BodyFontSize:  req.BodyFontSize,
			NotesFontSize: req.NotesFontSize,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		if err := database.DB.Create(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create document settings"})
		}
	} else {
		// 更新現有設定
		settings.Terms = req.Terms
		settings.Notes = req.Notes
		settings.UpdatedAt = time.Now()
		// 樣式欄位（所有 document_type 都可設定，但主要用於 style）
		if req.DocumentType == "style" {
			settings.LogoURL = req.LogoURL
			settings.LogoWidth = req.LogoWidth
			settings.LogoHeight = req.LogoHeight
			settings.TitleFontSize = req.TitleFontSize
			settings.BodyFontSize = req.BodyFontSize
			settings.NotesFontSize = req.NotesFontSize
		}
		if err := database.DB.Save(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update document settings"})
		}
	}

	return c.JSON(settings)
}

// GetDocumentStyleSettings 獲取全域文件樣式設定（for PDF 生成使用）
func GetDocumentStyleSettings(tenantID uuid.UUID) *models.DocumentSettings {
	var settings models.DocumentSettings
	if err := database.DB.Where("tenant_id = ? AND document_type = ?", tenantID, "style").First(&settings).Error; err != nil {
		return nil
	}
	return &settings
}
