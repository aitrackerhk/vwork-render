package handlers

import (
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/services/delivery"
)

// ============================================
// 外賣平台整合設定 API
// ============================================

// GetDeliveryIntegrations 獲取外賣平台整合列表
func GetDeliveryIntegrations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var integrations []models.DeliveryIntegration
	query := database.DB.Where("tenant_id = ?", tenantID).Order("platform ASC")

	// 可選：按平台過濾
	if platform := c.Query("platform"); platform != "" {
		query = query.Where("platform = ?", platform)
	}

	if err := query.Find(&integrations).Error; err != nil {
		log.Printf("[DeliveryIntegration] 獲取整合列表失敗: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "獲取失敗"})
	}

	// 構建響應（隱藏敏感信息）
	result := make([]fiber.Map, 0, len(integrations))
	for _, intg := range integrations {
		result = append(result, buildIntegrationResponse(&intg))
	}

	return c.JSON(result)
}

// GetDeliveryIntegration 獲取單個外賣平台整合
func GetDeliveryIntegration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var integration models.DeliveryIntegration
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&integration).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "整合不存在"})
	}

	return c.JSON(buildIntegrationResponse(&integration))
}

// buildIntegrationResponse constructs a response map for a DeliveryIntegration,
// including masked API credentials and extracted settings for frontend consumption.
func buildIntegrationResponse(intg *models.DeliveryIntegration) fiber.Map {
	// Extract environment from settings (default: sandbox)
	environment := "sandbox"
	if intg.Settings != nil {
		if env, ok := intg.Settings["environment"].(string); ok && env != "" {
			environment = env
		}
	}

	return fiber.Map{
		"id":            intg.ID,
		"platform":      intg.Platform,
		"merchant_id":   intg.MerchantID,
		"merchant_name": intg.MerchantName,
		"is_enabled":    intg.IsEnabled,
		"is_connected":  intg.IsConnected,
		"last_sync_at":  intg.LastSyncAt,
		"last_error":    intg.LastError,
		"webhook_url":   intg.WebhookURL,
		"settings":      intg.Settings,
		"environment":   environment,
		"created_at":    intg.CreatedAt,
		"updated_at":    intg.UpdatedAt,
		// Masked API credential indicators
		"has_api_key":    intg.APIKey != "",
		"has_api_secret": intg.APISecret != "",
	}
}

// CreateOrUpdateDeliveryIntegration 創建或更新外賣平台整合
func CreateOrUpdateDeliveryIntegration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Platform      string                 `json:"platform"` // foodpanda, keeta, deliveroo
		MerchantID    string                 `json:"merchant_id"`
		MerchantName  string                 `json:"merchant_name"`
		APIKey        string                 `json:"api_key"`
		APISecret     string                 `json:"api_secret"`
		AccessToken   string                 `json:"access_token"`
		RefreshToken  string                 `json:"refresh_token"`
		WebhookSecret string                 `json:"webhook_secret"`
		IsEnabled     *bool                  `json:"is_enabled"`
		Enabled       *bool                  `json:"enabled"`     // Frontend uses this name
		Environment   string                 `json:"environment"` // sandbox / production
		Config        map[string]interface{} `json:"config"`      // Platform-specific config (vendor_id, merchant_id, restaurant_id, etc.)
		Settings      map[string]interface{} `json:"settings"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	// 驗證平台類型
	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform != "foodpanda" && platform != "keeta" && platform != "deliveroo" {
		return c.Status(400).JSON(fiber.Map{"error": "不支援的平台類型"})
	}

	// 查找是否已存在
	var integration models.DeliveryIntegration
	err := database.DB.Where("tenant_id = ? AND platform = ?", tenantID, platform).First(&integration).Error
	isNew := err != nil

	if isNew {
		integration = models.DeliveryIntegration{
			TenantID:  tenantID,
			Platform:  models.DeliveryPlatform(platform),
			IsEnabled: true,
			CreatedBy: &userID,
		}
	}

	// 更新字段
	if req.MerchantID != "" {
		integration.MerchantID = req.MerchantID
	}
	if req.MerchantName != "" {
		integration.MerchantName = req.MerchantName
	}
	if req.APIKey != "" {
		integration.APIKey = req.APIKey
	}
	if req.APISecret != "" {
		integration.APISecret = req.APISecret
	}
	if req.AccessToken != "" {
		integration.AccessToken = req.AccessToken
	}
	if req.RefreshToken != "" {
		integration.RefreshToken = req.RefreshToken
	}
	if req.WebhookSecret != "" {
		integration.WebhookSecret = req.WebhookSecret
	}
	if req.IsEnabled != nil {
		integration.IsEnabled = *req.IsEnabled
	} else if req.Enabled != nil {
		// Frontend sends "enabled" instead of "is_enabled"
		integration.IsEnabled = *req.Enabled
	}

	// Extract merchant_id from platform-specific config fields
	if req.Config != nil {
		switch platform {
		case "foodpanda":
			if vid, ok := req.Config["vendor_id"].(string); ok && vid != "" {
				integration.MerchantID = vid
			}
		case "keeta":
			if mid, ok := req.Config["merchant_id"].(string); ok && mid != "" {
				integration.MerchantID = mid
			}
		case "deliveroo":
			if rid, ok := req.Config["restaurant_id"].(string); ok && rid != "" {
				integration.MerchantID = rid
			}
		}
	}

	// Merge settings: frontend settings + environment + config
	if req.Settings == nil {
		req.Settings = make(map[string]interface{})
	}
	if req.Environment != "" {
		req.Settings["environment"] = req.Environment
	}
	if req.Config != nil {
		req.Settings["config"] = req.Config
	}
	// Merge with existing settings (preserve fields not sent)
	if integration.Settings != nil {
		for k, v := range req.Settings {
			integration.Settings[k] = v
		}
	} else {
		integration.Settings = models.JSONB(req.Settings)
	}

	integration.UpdatedBy = &userID
	integration.UpdatedAt = time.Now()

	// 生成 Webhook URL
	if integration.WebhookURL == "" {
		// TODO: 從配置讀取基礎 URL
		baseURL := "https://api.vwork.ai"
		integration.WebhookURL = baseURL + "/api/v1/delivery/webhook/" + string(integration.Platform)
	}

	if isNew {
		if err := database.DB.Create(&integration).Error; err != nil {
			log.Printf("[DeliveryIntegration] 創建整合失敗: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "創建失敗"})
		}
	} else {
		if err := database.DB.Save(&integration).Error; err != nil {
			log.Printf("[DeliveryIntegration] 更新整合失敗: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "更新失敗"})
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"id":      integration.ID,
		"message": "保存成功",
	})
}

// DeleteDeliveryIntegration 刪除外賣平台整合
func DeleteDeliveryIntegration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	result := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.DeliveryIntegration{})
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "刪除失敗"})
	}
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "整合不存在"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// TestDeliveryIntegration 測試外賣平台連接（by ID）
func TestDeliveryIntegration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var integration models.DeliveryIntegration
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&integration).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "整合不存在"})
	}

	return testIntegrationConnection(c, &integration)
}

// TestDeliveryIntegrationDirect 測試外賣平台連接（by platform data, no ID needed）
// This is used by the frontend before saving when no integration ID exists yet.
func TestDeliveryIntegrationDirect(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Platform    string                 `json:"platform"`
		APIKey      string                 `json:"api_key"`
		APISecret   string                 `json:"api_secret"`
		Environment string                 `json:"environment"`
		Config      map[string]interface{} `json:"config"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	platform := strings.ToLower(strings.TrimSpace(req.Platform))
	if platform != "foodpanda" && platform != "keeta" && platform != "deliveroo" {
		return c.Status(400).JSON(fiber.Map{"error": "不支援的平台類型"})
	}

	// Try to find existing integration to get stored credentials
	var integration models.DeliveryIntegration
	err := database.DB.Where("tenant_id = ? AND platform = ?", tenantID, platform).First(&integration).Error
	if err != nil {
		// No existing integration, build from request data
		integration = models.DeliveryIntegration{
			TenantID: tenantID,
			Platform: models.DeliveryPlatform(platform),
		}
	}

	// Override with request data if provided
	if req.APIKey != "" {
		integration.APIKey = req.APIKey
	}
	if req.APISecret != "" {
		integration.APISecret = req.APISecret
	}
	// Extract merchant_id from config
	if req.Config != nil {
		switch platform {
		case "foodpanda":
			if vid, ok := req.Config["vendor_id"].(string); ok && vid != "" {
				integration.MerchantID = vid
			}
		case "keeta":
			if mid, ok := req.Config["merchant_id"].(string); ok && mid != "" {
				integration.MerchantID = mid
			}
		case "deliveroo":
			if rid, ok := req.Config["restaurant_id"].(string); ok && rid != "" {
				integration.MerchantID = rid
			}
		}
	}

	return testIntegrationConnection(c, &integration)
}

// testIntegrationConnection shared logic for testing a platform connection
func testIntegrationConnection(c *fiber.Ctx, integration *models.DeliveryIntegration) error {

	// Determine if sandbox
	isSandbox := false
	if integration.Settings != nil {
		if env, ok := integration.Settings["environment"].(string); ok && env == "sandbox" {
			isSandbox = true
		}
	}

	// 創建平台服務
	config := delivery.IntegrationConfig{
		Platform:      delivery.Platform(integration.Platform),
		MerchantID:    integration.MerchantID,
		APIKey:        integration.APIKey,
		APISecret:     integration.APISecret,
		AccessToken:   integration.AccessToken,
		RefreshToken:  integration.RefreshToken,
		WebhookSecret: integration.WebhookSecret,
		Sandbox:       isSandbox,
		ExtraSettings: integration.Settings,
	}

	service := delivery.NewIntegrationService(integration.TenantID, config)
	platformService, err := service.GetPlatformService()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// 測試連接
	if err := platformService.TestConnection(); err != nil {
		// Only update DB if integration is already persisted
		if integration.ID != uuid.Nil {
			integration.IsConnected = false
			integration.LastError = err.Error()
			database.DB.Save(&integration)
		}

		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error":   err.Error(),
		})
	}

	// Only update DB if integration is already persisted
	if integration.ID != uuid.Nil {
		integration.IsConnected = true
		integration.LastError = ""
		integration.LastSyncAt = timePtr(time.Now())
		database.DB.Save(&integration)
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "連接成功",
	})
}

// 輔助函數
func timePtr(t time.Time) *time.Time {
	return &t
}
