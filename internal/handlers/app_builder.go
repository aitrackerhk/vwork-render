package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AppConfig 租戶 App 配置
type AppConfig struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TenantID  uuid.UUID `json:"tenant_id" gorm:"type:uuid;index"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 基本資訊
	AppName        string `json:"app_name" gorm:"size:100"`        // App 名稱
	AppDescription string `json:"app_description" gorm:"size:500"` // App 描述
	PackageName    string `json:"package_name" gorm:"size:100"`    // com.example.app
	BundleID       string `json:"bundle_id" gorm:"size:100"`       // iOS Bundle ID

	// 品牌設定
	PrimaryColor   string `json:"primary_color" gorm:"size:20"`   // #FF5722
	SecondaryColor string `json:"secondary_color" gorm:"size:20"` // #FFC107
	LogoURL        string `json:"logo_url" gorm:"size:500"`       // Logo 圖片 URL
	SplashURL      string `json:"splash_url" gorm:"size:500"`     // 啟動畫面 URL

	// API 設定
	APIBaseURL string `json:"api_base_url" gorm:"size:200"` // 後端 API 地址

	// 功能開關
	EnableOffline       bool `json:"enable_offline" gorm:"default:true"`       // 離線模式
	EnableNotifications bool `json:"enable_notifications" gorm:"default:true"` // 推送通知
	EnableAnalytics     bool `json:"enable_analytics" gorm:"default:false"`    // 數據分析

	// 構建狀態
	BuildStatus   string    `json:"build_status" gorm:"size:20;default:'pending'"` // pending, building, success, failed
	LastBuildAt   time.Time `json:"last_build_at"`
	BuildErrorMsg string    `json:"build_error_msg" gorm:"size:1000"`
	AndroidAPKURL string    `json:"android_apk_url" gorm:"size:500"`                // 構建完成的 APK 下載鏈接
	AndroidAABURL string    `json:"android_aab_url" gorm:"size:500"`                // AAB 下載鏈接
	IOSIPAUrl     string    `json:"ios_ipa_url" gorm:"column:ios_ipa_url;size:500"` // IPA 下載鏈接

	// 上架資訊
	GooglePlayURL string `json:"google_play_url" gorm:"size:500"` // Google Play 商店鏈接
	AppStoreURL   string `json:"app_store_url" gorm:"size:500"`   // App Store 鏈接
	PublishStatus string `json:"publish_status" gorm:"size:20"`   // draft, review, published
}

// AppBuildRequest 構建請求
type AppBuildRequest struct {
	Platform string `json:"platform"` // android, ios, both
}

// AppBuildHandler App 構建處理器
type AppBuildHandler struct {
	DB *gorm.DB
}

// NewAppBuildHandler 創建處理器
func NewAppBuildHandler(db *gorm.DB) *AppBuildHandler {
	return &AppBuildHandler{DB: db}
}

// GetAppConfig 獲取租戶 App 配置
func (h *AppBuildHandler) GetAppConfig(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	var config AppConfig
	result := h.DB.Where("tenant_id = ?", tenantID).First(&config)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// 返回默認配置
			return c.JSON(fiber.Map{
				"success": true,
				"data":    nil,
				"message": "尚未配置 App",
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": result.Error.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    config,
	})
}

// SaveAppConfig 保存 App 配置
func (h *AppBuildHandler) SaveAppConfig(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	var input AppConfig
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求數據"})
	}

	// 驗證必填欄位
	if input.AppName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "App 名稱為必填"})
	}

	// 自動生成 Package Name（如果未提供）
	if input.PackageName == "" {
		input.PackageName = generatePackageName(input.AppName, tenantID)
	}
	if input.BundleID == "" {
		input.BundleID = input.PackageName
	}

	// 查找現有配置
	var existing AppConfig
	result := h.DB.Where("tenant_id = ?", tenantID).First(&existing)

	if result.Error == gorm.ErrRecordNotFound {
		// 創建新配置
		input.TenantID = tenantID
		input.BuildStatus = "pending"
		if err := h.DB.Create(&input).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"success": true,
			"data":    input,
			"message": "App 配置已創建",
		})
	}

	// 更新現有配置
	input.ID = existing.ID
	input.TenantID = tenantID
	input.CreatedAt = existing.CreatedAt
	if err := h.DB.Save(&input).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    input,
		"message": "App 配置已更新",
	})
}

// TriggerBuild 觸發 App 構建
func (h *AppBuildHandler) TriggerBuild(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	var req AppBuildRequest
	if err := c.BodyParser(&req); err != nil {
		req.Platform = "both"
	}

	// 獲取配置
	var config AppConfig
	if err := h.DB.Where("tenant_id = ?", tenantID).First(&config).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "請先配置 App 資訊"})
	}

	// 更新構建狀態
	config.BuildStatus = "building"
	config.LastBuildAt = time.Now()
	config.BuildErrorMsg = ""
	h.DB.Save(&config)

	// 生成構建配置文件
	buildConfig, err := h.generateBuildConfig(&config, req.Platform)
	if err != nil {
		config.BuildStatus = "failed"
		config.BuildErrorMsg = err.Error()
		h.DB.Save(&config)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 觸發 GitHub Actions 構建（或本地構建）
	buildID, err := h.triggerGitHubBuild(buildConfig)
	if err != nil {
		config.BuildStatus = "failed"
		config.BuildErrorMsg = err.Error()
		h.DB.Save(&config)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":  true,
		"build_id": buildID,
		"message":  "構建已開始，請稍後查看狀態",
		"status":   "building",
	})
}

// GetBuildStatus 獲取構建狀態
func (h *AppBuildHandler) GetBuildStatus(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	var config AppConfig
	if err := h.DB.Where("tenant_id = ?", tenantID).First(&config).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "未找到 App 配置"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data": fiber.Map{
			"status":      config.BuildStatus,
			"last_build":  config.LastBuildAt,
			"error":       config.BuildErrorMsg,
			"android_apk": config.AndroidAPKURL,
			"android_aab": config.AndroidAABURL,
			"ios_ipa":     config.IOSIPAUrl,
			"google_play": config.GooglePlayURL,
			"app_store":   config.AppStoreURL,
		},
	})
}

// BuildWebhook 構建完成回調
func (h *AppBuildHandler) BuildWebhook(c *fiber.Ctx) error {
	// 驗證 webhook 簽名
	signature := c.Get("X-Build-Signature")
	if !verifyWebhookSignature(signature, c.Body()) {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid signature"})
	}

	var payload struct {
		TenantID   uint   `json:"tenant_id"`
		Status     string `json:"status"`
		Error      string `json:"error"`
		AndroidAPK string `json:"android_apk"`
		AndroidAAB string `json:"android_aab"`
		IOSIPA     string `json:"ios_ipa"`
	}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var config AppConfig
	if err := h.DB.Where("tenant_id = ?", payload.TenantID).First(&config).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Config not found"})
	}

	config.BuildStatus = payload.Status
	config.BuildErrorMsg = payload.Error
	config.AndroidAPKURL = payload.AndroidAPK
	config.AndroidAABURL = payload.AndroidAAB
	config.IOSIPAUrl = payload.IOSIPA
	h.DB.Save(&config)

	return c.JSON(fiber.Map{"success": true})
}

// 生成構建配置
func (h *AppBuildHandler) generateBuildConfig(config *AppConfig, platform string) (map[string]interface{}, error) {
	buildConfig := map[string]interface{}{
		"tenant_id":    config.TenantID,
		"app_name":     config.AppName,
		"package_name": config.PackageName,
		"bundle_id":    config.BundleID,
		"api_base_url": config.APIBaseURL,
		"platform":     platform,
		"branding": map[string]string{
			"primary_color":   config.PrimaryColor,
			"secondary_color": config.SecondaryColor,
			"logo_url":        config.LogoURL,
			"splash_url":      config.SplashURL,
		},
		"features": map[string]bool{
			"offline":       config.EnableOffline,
			"notifications": config.EnableNotifications,
			"analytics":     config.EnableAnalytics,
		},
		"build_time": time.Now().UTC().Format(time.RFC3339),
	}

	return buildConfig, nil
}

// 觸發 GitHub Actions 構建
func (h *AppBuildHandler) triggerGitHubBuild(config map[string]interface{}) (string, error) {
	// 生成唯一構建 ID
	buildID := generateBuildID()

	// 轉換配置為 JSON
	configJSON, _ := json.MarshalIndent(config, "", "  ")

	// 獲取全局配置
	appCfg := mustAppConfig()

	// 檢查 GitHub 配置是否存在
	if appCfg.GitHub.Token == "" || appCfg.GitHub.Owner == "" || appCfg.GitHub.Repo == "" {
		// 如果沒有配置，僅在開發環境下返回成功（模擬）
		// 但為了確保任務完成，這裡我們嚴格一點，如果沒配置就報錯，或者根據環境變量決定
		// 考慮到這是 "正式調用"，我們應該嘗試調用
		return "", errors.New("GitHub configuration (Token, Owner, Repo) is missing")
	}

	// 構建請求 URL
	// workflow_id 可以是文件名（如 build.yml）或 ID
	workflowID := appCfg.GitHub.WorkflowID
	if workflowID == "" {
		workflowID = "build_app.yml"
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/workflows/%s/dispatches",
		appCfg.GitHub.Owner, appCfg.GitHub.Repo, workflowID)

	// 準備請求體
	// GitHub workflow_dispatch payload
	payload := map[string]interface{}{
		"ref": "main", // 默認使用 main 分支
		"inputs": map[string]string{
			"build_config": base64.StdEncoding.EncodeToString(configJSON),
			"build_id":     buildID,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// 創建請求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+appCfg.GitHub.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	// 發送請求
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 檢查響應狀態
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API failed with status %d: %s", resp.StatusCode, string(body))
	}

	return buildID, nil
}

// 生成 Package Name
func generatePackageName(appName string, tenantID uuid.UUID) string {
	// 轉換為小寫，移除特殊字符
	name := strings.ToLower(appName)
	name = strings.ReplaceAll(name, " ", "")
	name = strings.ReplaceAll(name, "-", "")

	// 只保留字母數字
	var cleaned strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cleaned.WriteRune(r)
		}
	}

	tenantPart := strings.ReplaceAll(strings.ToLower(tenantID.String()), "-", "")
	return fmt.Sprintf("com.vwork.tenant%s.%s", tenantPart, cleaned.String())
}

// 生成構建 ID
func generateBuildID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// 驗證 Webhook 簽名
func verifyWebhookSignature(signature string, body []byte) bool {
	// TODO: 實現 HMAC-SHA256 簽名驗證
	// 從環境變量獲取 webhook secret
	return true // 暫時跳過驗證
}
