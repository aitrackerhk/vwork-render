package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type googleAdsOAuthState struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	Ts       int64  `json:"ts"`
}

func buildGoogleAdsOAuthConfig(baseURL string, cfg *config.Config) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.GoogleOAuth.ClientID,
		ClientSecret: cfg.GoogleOAuth.ClientSecret,
		RedirectURL:  strings.TrimRight(baseURL, "/") + "/google-ads/oauth/callback",
		Scopes: []string{
			"https://www.googleapis.com/auth/adwords",
			"openid",
			"email",
			"profile",
		},
		Endpoint: google.Endpoint,
	}
}

func buildOAuthBaseURL(c *fiber.Ctx, cfg *config.Config) string {
	scheme := c.Protocol()
	host := c.Hostname()
	port := c.Port()

	if host == "" {
		host = strings.TrimSpace(cfg.Domain.BaseDomain)
	}

	base := scheme + "://" + host
	if port != "" && port != "80" && port != "443" && !strings.Contains(host, ":") {
		base += ":" + port
	}
	return base
}

func signOAuthState(payload string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sig)
}

func encodeOAuthState(state googleAdsOAuthState, secret string) (string, error) {
	bytes, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(bytes)
	sig := signOAuthState(payload, secret)
	return payload + "." + sig, nil
}

func decodeOAuthState(raw string, secret string) (*googleAdsOAuthState, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid state")
	}
	payload := parts[0]
	sig := parts[1]
	if signOAuthState(payload, secret) != sig {
		return nil, fmt.Errorf("invalid signature")
	}
	bytes, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload")
	}
	var state googleAdsOAuthState
	if err := json.Unmarshal(bytes, &state); err != nil {
		return nil, fmt.Errorf("invalid payload")
	}
	return &state, nil
}

func getEnterpriseForTenant(tenantID uuid.UUID) (*models.Enterprise, error) {
	var enterprise models.Enterprise
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&enterprise).Error; err != nil {
		return nil, err
	}
	return &enterprise, nil
}

func getGoogleAdsAccount(enterprise *models.Enterprise) map[string]interface{} {
	if enterprise.ExtraFields == nil {
		enterprise.ExtraFields = make(map[string]interface{})
	}
	googleAds, _ := enterprise.ExtraFields["google_ads"].(map[string]interface{})
	if googleAds == nil {
		googleAds = make(map[string]interface{})
	}
	account, _ := googleAds["account"].(map[string]interface{})
	if account == nil {
		account = make(map[string]interface{})
	}
	googleAds["account"] = account
	enterprise.ExtraFields["google_ads"] = googleAds
	return account
}

// GetGoogleAdsConnectURL 返回 OAuth 授權連結
func GetGoogleAdsConnectURL(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	cfg := config.Load()
	if !cfg.GoogleOAuth.Enabled || cfg.GoogleOAuth.ClientID == "" || cfg.GoogleOAuth.ClientSecret == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Google OAuth is not configured"})
	}

	enterprise, err := getEnterpriseForTenant(tenantID)
	if err == nil {
		account := getGoogleAdsAccount(enterprise)
		if connected, ok := account["connected"].(bool); ok && connected {
			if token, ok := account["refresh_token"].(string); ok && strings.TrimSpace(token) != "" {
				return c.Status(409).JSON(fiber.Map{"error": "Google Ads already connected"})
			}
		}
	}

	baseURL := buildOAuthBaseURL(c, cfg)
	oauthConf := buildGoogleAdsOAuthConfig(baseURL, cfg)

	state, err := encodeOAuthState(googleAdsOAuthState{
		TenantID: tenantID.String(),
		UserID:   userID.String(),
		Ts:       time.Now().Unix(),
	}, cfg.JWT.Secret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to build state"})
	}

	authURL := oauthConf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	return c.JSON(fiber.Map{"url": authURL})
}

// GoogleAdsOAuthCallback Google OAuth 回調
func GoogleAdsOAuthCallback(c *fiber.Ctx) error {
	cfg := config.Load()
	rawState := c.Query("state")
	code := c.Query("code")
	if rawState == "" || code == "" {
		return c.Status(400).SendString("Missing state or code")
	}

	state, err := decodeOAuthState(rawState, cfg.JWT.Secret)
	if err != nil {
		return c.Status(400).SendString("Invalid state")
	}

	tenantID, err := uuid.Parse(state.TenantID)
	if err != nil {
		return c.Status(400).SendString("Invalid tenant")
	}

	baseURL := buildOAuthBaseURL(c, cfg)
	oauthConf := buildGoogleAdsOAuthConfig(baseURL, cfg)

	token, err := oauthConf.Exchange(context.Background(), code)
	if err != nil {
		return c.Status(400).SendString("OAuth exchange failed")
	}

	email := ""
	if token.AccessToken != "" {
		req, _ := http.NewRequest("GET", "https://openidconnect.googleapis.com/v1/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var userinfo struct {
				Email string `json:"email"`
			}
			_ = json.Unmarshal(body, &userinfo)
			email = strings.TrimSpace(userinfo.Email)
		}
	}

	enterprise, err := getEnterpriseForTenant(tenantID)
	if err != nil {
		return c.Status(404).SendString("Enterprise not found")
	}

	account := getGoogleAdsAccount(enterprise)
	account["connected"] = true
	if email != "" {
		account["email"] = email
	}
	if token.RefreshToken != "" {
		account["refresh_token"] = token.RefreshToken
	}
	account["token_expires_at"] = token.Expiry.Format(time.RFC3339)
	account["connected_at"] = time.Now().Format(time.RFC3339)

	if err := database.DB.Save(enterprise).Error; err != nil {
		return c.Status(500).SendString("Failed to save connection")
	}

	return c.Redirect("/google-ads?google_ads=connected")
}

// DisconnectGoogleAds 解除 Google Ads 連接
func DisconnectGoogleAds(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	enterprise, err := getEnterpriseForTenant(tenantID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Enterprise not found"})
	}

	account := getGoogleAdsAccount(enterprise)
	account["connected"] = false
	account["email"] = ""
	delete(account, "refresh_token")
	delete(account, "token_expires_at")

	if err := database.DB.Save(enterprise).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to disconnect"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// GenerateGoogleAd 根據租戶資料使用 LLM 生成廣告內容
func GenerateGoogleAd(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	cfg := config.Load()
	if cfg.LLM.APIKey == "" {
		return c.Status(400).JSON(fiber.Map{"error": "LLM API Key not configured"})
	}

	// Collect tenant business data
	var enterprise models.Enterprise
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&enterprise).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Enterprise not found"})
	}

	var tenant models.Tenant
	if err := database.DB.Preload("IndustryTemplate").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Get top products (up to 10, active only)
	var products []models.Product
	database.DB.Where("tenant_id = ? AND status = 'active' AND trashed_at IS NULL", tenantID).
		Order("updated_at DESC").Limit(10).Find(&products)

	// Get services (up to 10, active only)
	var services []models.Service
	database.DB.Where("tenant_id = ? AND status = 'active'", tenantID).
		Order("updated_at DESC").Limit(10).Find(&services)

	// Build context for LLM
	var sb strings.Builder

	// Enterprise info
	sb.WriteString(fmt.Sprintf("企業名稱：%s\n", enterprise.Name))
	if enterprise.Address != nil && *enterprise.Address != "" {
		sb.WriteString(fmt.Sprintf("地址：%s\n", *enterprise.Address))
	}
	if enterprise.Domain != nil && *enterprise.Domain != "" {
		sb.WriteString(fmt.Sprintf("網站：%s\n", *enterprise.Domain))
	}
	// Extract phone and email from ExtraFields
	if enterprise.ExtraFields != nil {
		if phone, ok := enterprise.ExtraFields["phone"].(string); ok && phone != "" {
			sb.WriteString(fmt.Sprintf("電話：%s\n", phone))
		}
		if email, ok := enterprise.ExtraFields["email"].(string); ok && email != "" {
			sb.WriteString(fmt.Sprintf("電郵：%s\n", email))
		}
	}

	// Industry info
	if tenant.IndustryTemplate != nil {
		sb.WriteString(fmt.Sprintf("行業：%s\n", tenant.IndustryTemplate.Name))
		if tenant.IndustryTemplate.Description != nil && *tenant.IndustryTemplate.Description != "" {
			sb.WriteString(fmt.Sprintf("行業描述：%s\n", *tenant.IndustryTemplate.Description))
		}
	}

	// Website type
	if tenant.WebsiteType != nil && *tenant.WebsiteType != "" {
		sb.WriteString(fmt.Sprintf("網站類型：%s\n", *tenant.WebsiteType))
	}

	// Products
	if len(products) > 0 {
		sb.WriteString("\n主要產品/商品：\n")
		for i, p := range products {
			line := fmt.Sprintf("%d. %s", i+1, p.Name)
			if p.Price > 0 {
				line += fmt.Sprintf("（價格：%.2f）", p.Price)
			}
			if p.Description != "" {
				desc := p.Description
				if len(desc) > 80 {
					desc = desc[:80] + "..."
				}
				line += fmt.Sprintf(" - %s", desc)
			}
			sb.WriteString(line + "\n")
		}
	}

	// Services
	if len(services) > 0 {
		sb.WriteString("\n主要服務：\n")
		for i, s := range services {
			line := fmt.Sprintf("%d. %s", i+1, s.Name)
			if s.Price > 0 {
				line += fmt.Sprintf("（價格：%.2f）", s.Price)
			}
			if s.Description != "" {
				desc := s.Description
				if len(desc) > 80 {
					desc = desc[:80] + "..."
				}
				line += fmt.Sprintf(" - %s", desc)
			}
			sb.WriteString(line + "\n")
		}
	}

	businessContext := sb.String()

	// Build target URL
	targetURL := ""
	if enterprise.Domain != nil && *enterprise.Domain != "" {
		targetURL = *enterprise.Domain
		if !strings.HasPrefix(targetURL, "http") {
			targetURL = "https://" + targetURL
		}
	}

	prompt := fmt.Sprintf(`你是一位專業的 Google Ads 廣告文案專家。請根據以下企業資料，生成一則適合投放的 Google 廣告。

%s

請嚴格按照以下 JSON 格式回覆（不要加任何其他文字或 markdown 標記）：
{
  "name": "廣告活動名稱（簡短描述性名稱）",
  "budget": 建議每日預算數字（純數字，單位 HKD），
  "target_url": "%s",
  "content": "廣告文案（包含標題和描述，突出賣點和行動呼籲，200字以內）"
}

要求：
1. 廣告名稱要簡潔，能反映業務重點
2. 預算建議根據企業規模合理設定（小型 50-100，中型 100-300，大型 300-500）
3. 廣告文案要吸引人，包含關鍵賣點、優勢和行動呼籲（CTA）
4. 如有產品/服務價格信息，可以適當融入文案
5. 使用繁體中文撰寫`, businessContext, targetURL)

	// Call LLM
	var llmResult string
	var llmErr error
	if cfg.LLM.Provider == "gemini" {
		llmResult, llmErr = callGeminiForAdGeneration(cfg, prompt)
	} else {
		llmResult, llmErr = callOpenAIForAdGeneration(cfg, prompt)
	}
	if llmErr != nil {
		return c.Status(500).JSON(fiber.Map{"error": "AI 生成失敗: " + llmErr.Error()})
	}

	// Parse JSON from LLM response
	llmResult = strings.TrimSpace(llmResult)
	// Strip markdown code block if present
	if strings.HasPrefix(llmResult, "```") {
		lines := strings.SplitN(llmResult, "\n", 2)
		if len(lines) > 1 {
			llmResult = lines[1]
		}
		if idx := strings.LastIndex(llmResult, "```"); idx >= 0 {
			llmResult = llmResult[:idx]
		}
		llmResult = strings.TrimSpace(llmResult)
	}

	var adData map[string]interface{}
	if err := json.Unmarshal([]byte(llmResult), &adData); err != nil {
		// If JSON parse fails, return raw text as content
		adData = map[string]interface{}{
			"name":       enterprise.Name + " 廣告推廣",
			"budget":     100,
			"target_url": targetURL,
			"content":    llmResult,
		}
	}

	// Ensure target_url is set
	if adData["target_url"] == nil || adData["target_url"] == "" {
		adData["target_url"] = targetURL
	}

	return c.JSON(adData)
}

func callGeminiForAdGeneration(cfg *config.Config, prompt string) (string, error) {
	model := cfg.LLM.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}
	model = strings.TrimSuffix(model, "-latest")

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 4096,
		},
		"safetySettings": []map[string]interface{}{
			{
				"category":  "HARM_CATEGORY_HARASSMENT",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_HATE_SPEECH",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_SEXUALLY_EXPLICIT",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_DANGEROUS_CONTENT",
				"threshold": "BLOCK_NONE",
			},
		},
	}

	body, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Gemini API error: %s", string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", fmt.Errorf("no candidates in response")
	}
	candidate := candidates[0].(map[string]interface{})
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no content in candidate")
	}
	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("no parts in content")
	}
	part := parts[0].(map[string]interface{})
	text, ok := part["text"].(string)
	if !ok {
		return "", fmt.Errorf("no text in part")
	}

	return text, nil
}

func callOpenAIForAdGeneration(cfg *config.Config, prompt string) (string, error) {
	model := cfg.LLM.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	apiURL := cfg.LLM.Endpoint
	if apiURL == "" {
		apiURL = "https://api.openai.com/v1/chat/completions"
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一位專業的 Google Ads 廣告文案專家，專門根據企業資料生成高效的廣告文案。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  1024,
	}

	body, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.LLM.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OpenAI API error: %s", string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	choice := choices[0].(map[string]interface{})
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no message in choice")
	}
	text, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("no content in message")
	}

	return text, nil
}
