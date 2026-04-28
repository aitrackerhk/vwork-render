package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ShippingIntegrationsModuleCode 配送整合模塊代碼
const ShippingIntegrationsModuleCode = "shipping_integrations"

// GetShippingIntegrations 獲取配送整合設定
func GetShippingIntegrations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var module models.TenantModule
	if err := database.DB.Where("tenant_id = ? AND module_code = ?", tenantID, ShippingIntegrationsModuleCode).
		First(&module).Error; err != nil {
		// 不存在，返回預設空配置
		return c.JSON(fiber.Map{
			"data": fiber.Map{
				"sfexpress": fiber.Map{
					"enabled": false,
				},
			},
		})
	}

	return c.JSON(fiber.Map{
		"data": module.Config,
	})
}

// UpdateShippingIntegrations 更新配送整合設定
func UpdateShippingIntegrations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var req map[string]interface{}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// 查找或創建模塊配置
	var module models.TenantModule
	if err := database.DB.Where("tenant_id = ? AND module_code = ?", tenantID, ShippingIntegrationsModuleCode).
		First(&module).Error; err != nil {
		// 創建新的
		module = models.TenantModule{
			TenantID:   tenantID,
			ModuleCode: ShippingIntegrationsModuleCode,
			IsEnabled:  true,
			Config:     models.JSONB(req),
		}
		if err := database.DB.Create(&module).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create shipping integrations config"})
		}
	} else {
		// 更新現有的
		if err := database.DB.Model(&module).Update("config", models.JSONB(req)).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update shipping integrations config"})
		}
	}

	return c.JSON(fiber.Map{
		"message": "Shipping integrations updated successfully",
		"data":    module.Config,
	})
}

// TestSFExpressConnection 測試 SF Express 連接
// 注意：這只是一個佔位實現，實際 API 調用需要根據順豐 API 文檔實現
func TestSFExpressConnection(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var req struct {
		PartnerID   string `json:"partner_id"`
		Checkword   string `json:"checkword"`
		Environment string `json:"environment"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.PartnerID == "" || req.Checkword == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "客戶編碼和校驗碼不能為空",
		})
	}

	// TODO: 實際調用 SF Express API 進行驗證
	// 目前只做基本驗證，實際生產環境需要實現 SF Express API 調用
	// SF Express API 文檔: https://open.sf-express.com/

	// 模擬測試連接成功（實際應該調用 SF Express 的驗證接口）
	// 返回成功表示配置格式正確，實際連接狀態需要在發貨時確認
	return c.JSON(fiber.Map{
		"success":     true,
		"message":     "配置格式正確，請在實際發貨時確認連接狀態",
		"environment": req.Environment,
	})
}

// TestLalamoveConnection 測試 Lalamove 連接
// Lalamove API 文檔: https://developers.lalamove.com/
func TestLalamoveConnection(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var req struct {
		APIKey      string `json:"api_key"`
		APISecret   string `json:"api_secret"`
		Environment string `json:"environment"`
		Market      string `json:"market"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.APIKey == "" || req.APISecret == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "API Key 和 API Secret 不能為空",
		})
	}

	if req.Market == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "請選擇服務區域",
		})
	}

	// 驗證市場代碼
	validMarkets := map[string]bool{
		"HK": true, "SG": true, "TW": true, "TH": true,
		"VN": true, "PH": true, "MY": true, "ID": true,
	}
	if !validMarkets[req.Market] {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "不支援的服務區域",
		})
	}

	// TODO: 實際調用 Lalamove API 進行驗證
	// Lalamove 使用 HMAC-SHA256 簽名驗證
	// Sandbox: https://rest.sandbox.lalamove.com
	// Production: https://rest.lalamove.com
	//
	// 驗證步驟：
	// 1. 構建簽名: HMAC-SHA256(timestamp + method + path + body, api_secret)
	// 2. 調用 GET /v3/cities 獲取可用城市列表
	// 3. 如果返回成功，表示 API 憑證有效

	// 模擬測試連接成功
	return c.JSON(fiber.Map{
		"success":     true,
		"message":     "配置格式正確，請在實際下單時確認連接狀態",
		"environment": req.Environment,
		"market":      req.Market,
	})
}

// TestDHLConnection 測試 DHL Express 連接
// DHL Express API 文檔: https://developer.dhl.com/api-reference/dhl-express-mydhl-api
func TestDHLConnection(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var req struct {
		APIKey      string `json:"api_key"`
		APISecret   string `json:"api_secret"`
		AccountNo   string `json:"account_no"`
		Environment string `json:"environment"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.APIKey == "" || req.APISecret == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "API Key 和 API Secret 不能為空",
		})
	}

	if req.AccountNo == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "DHL 帳號不能為空",
		})
	}

	// TODO: 實際調用 DHL Express API 進行驗證
	// DHL Express 使用 Basic Auth (API Key + API Secret)
	// Sandbox + Production 都使用: https://express.api.dhl.com/mydhlapi
	// 可以調用 GET /mydhlapi/rates 驗證憑證是否有效

	// 模擬測試連接成功
	return c.JSON(fiber.Map{
		"success":     true,
		"message":     "配置格式正確，請在實際發貨時確認連接狀態",
		"environment": req.Environment,
	})
}

// TestUPSConnection 測試 UPS 連接
// UPS API 文檔: https://developer.ups.com/
func TestUPSConnection(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		AccountNo    string `json:"account_no"`
		Environment  string `json:"environment"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.ClientID == "" || req.ClientSecret == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Client ID 和 Client Secret 不能為空",
		})
	}

	if req.AccountNo == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "UPS 帳號不能為空",
		})
	}

	// TODO: 實際調用 UPS API 進行驗證
	// UPS 使用 OAuth 2.0 Client Credentials 流程
	// Sandbox: https://wwwcie.ups.com
	// Production: https://onlinetools.ups.com
	// 可以調用 POST /security/v1/oauth/token 驗證憑證是否有效

	// 模擬測試連接成功
	return c.JSON(fiber.Map{
		"success":     true,
		"message":     "配置格式正確，請在實際發貨時確認連接狀態",
		"environment": req.Environment,
	})
}

// TestFedExConnection 測試 FedEx 連接
// FedEx API 文檔: https://developer.fedex.com/
func TestFedExConnection(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		AccountNo    string `json:"account_no"`
		Environment  string `json:"environment"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.ClientID == "" || req.ClientSecret == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Client ID 和 Client Secret 不能為空",
		})
	}

	if req.AccountNo == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "FedEx 帳號不能為空",
		})
	}

	// TODO: 實際調用 FedEx API 進行驗證
	// FedEx 使用 OAuth 2.0 Client Credentials 流程 (form post)
	// Sandbox: https://apis-sandbox.fedex.com
	// Production: https://apis.fedex.com
	// 可以調用 POST /oauth/token 驗證憑證是否有效

	// 模擬測試連接成功
	return c.JSON(fiber.Map{
		"success":     true,
		"message":     "配置格式正確，請在實際發貨時確認連接狀態",
		"environment": req.Environment,
	})
}

// TestAmazonShippingConnection 測試 Amazon Shipping 連接
// Amazon SP-API 文檔: https://developer-docs.amazon.com/sp-api/
func TestAmazonShippingConnection(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RefreshToken string `json:"refresh_token"`
		Region       string `json:"region"`
		Environment  string `json:"environment"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.ClientID == "" || req.ClientSecret == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Client ID 和 Client Secret 不能為空",
		})
	}

	if req.RefreshToken == "" {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "Refresh Token 不能為空",
		})
	}

	// 驗證區域代碼
	validRegions := map[string]bool{
		"NA": true, "EU": true, "FE": true,
	}
	if req.Region != "" && !validRegions[req.Region] {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"message": "不支援的區域，請選擇 NA、EU 或 FE",
		})
	}

	// TODO: 實際調用 Amazon SP-API 進行驗證
	// Amazon 使用 LWA (Login with Amazon) refresh token 流程
	// Auth endpoint: https://api.amazon.com/auth/o2/token
	// NA: https://sellingpartnerapi-na.amazon.com
	// EU: https://sellingpartnerapi-eu.amazon.com
	// FE: https://sellingpartnerapi-fe.amazon.com
	// 可以調用 POST /auth/o2/token 獲取 access token 驗證憑證是否有效

	// 模擬測試連接成功
	return c.JSON(fiber.Map{
		"success":     true,
		"message":     "配置格式正確，請在實際發貨時確認連接狀態",
		"environment": req.Environment,
		"region":      req.Region,
	})
}
