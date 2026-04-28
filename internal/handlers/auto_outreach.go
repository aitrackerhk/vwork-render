package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================================
// Auto Outreach Campaign CRUD
// ============================================================

// GetAutoOutreachCampaigns lists all auto-outreach campaigns for the tenant
func GetAutoOutreachCampaigns(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var campaigns []models.AutoOutreachCampaign
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.AutoOutreachCampaign{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&campaigns).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  campaigns,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetAutoOutreachCampaign returns a single campaign
func GetAutoOutreachCampaign(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var campaign models.AutoOutreachCampaign
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&campaign).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	return c.JSON(campaign)
}

// CreateAutoOutreachCampaign creates a new auto-outreach campaign
func CreateAutoOutreachCampaign(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		SearchKeywords  string `json:"search_keywords"`
		SearchRegion    string `json:"search_region"`
		ProductID       string `json:"product_id"`
		ContactType     string `json:"contact_type"`
		ResultLimit     int    `json:"result_limit"`
		Channel         string `json:"channel"`
		EmailSubject    string `json:"email_subject"`
		MessageContent  string `json:"message_content"`
		IntervalMinutes int    `json:"interval_minutes"`
		MaxLeadsPerRun  int    `json:"max_leads_per_run"`
		MaxSendsPerRun  int    `json:"max_sends_per_run"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if strings.TrimSpace(req.Name) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Campaign name is required"})
	}
	if strings.TrimSpace(req.SearchKeywords) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Search keywords are required"})
	}
	if strings.TrimSpace(req.MessageContent) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Message content is required"})
	}

	channel := strings.TrimSpace(req.Channel)
	if channel == "" {
		channel = "email"
	}
	validChannels := map[string]bool{"email": true, "whatsapp": true, "both": true}
	if !validChannels[channel] {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid channel. Must be: email, whatsapp, both"})
	}

	contactType := strings.TrimSpace(req.ContactType)
	validContactTypes := map[string]bool{"": true, "email": true, "phone": true, "both": true}
	if !validContactTypes[contactType] {
		contactType = ""
	}

	resultLimit := req.ResultLimit
	if resultLimit <= 0 {
		resultLimit = 50
	}

	intervalMinutes := req.IntervalMinutes
	if intervalMinutes < 60 {
		intervalMinutes = 60
	}
	maxLeadsPerRun := req.MaxLeadsPerRun
	if maxLeadsPerRun <= 0 {
		maxLeadsPerRun = 10
	}
	maxSendsPerRun := req.MaxSendsPerRun
	if maxSendsPerRun <= 0 {
		maxSendsPerRun = 10
	}

	var productID *uuid.UUID
	if req.ProductID != "" {
		if pid, err := uuid.Parse(req.ProductID); err == nil {
			productID = &pid
		}
	}

	now := time.Now()
	nextRunAt := now.Add(time.Duration(intervalMinutes) * time.Minute)

	campaign := models.AutoOutreachCampaign{
		TenantID:        tenantID,
		CreatedByID:     userID,
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		SearchKeywords:  strings.TrimSpace(req.SearchKeywords),
		SearchRegion:    strings.TrimSpace(req.SearchRegion),
		ProductID:       productID,
		ContactType:     contactType,
		ResultLimit:     resultLimit,
		Channel:         channel,
		EmailSubject:    strings.TrimSpace(req.EmailSubject),
		MessageContent:  strings.TrimSpace(req.MessageContent),
		IsActive:        true,
		IntervalMinutes: intervalMinutes,
		MaxLeadsPerRun:  maxLeadsPerRun,
		MaxSendsPerRun:  maxSendsPerRun,
		NextRunAt:       &nextRunAt,
		Status:          "active",
	}

	if err := database.DB.Create(&campaign).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create campaign: " + err.Error()})
	}

	return c.Status(201).JSON(campaign)
}

// UpdateAutoOutreachCampaign updates an existing campaign
func UpdateAutoOutreachCampaign(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var campaign models.AutoOutreachCampaign
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&campaign).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	var req struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		SearchKeywords  string `json:"search_keywords"`
		SearchRegion    string `json:"search_region"`
		ProductID       string `json:"product_id"`
		ContactType     string `json:"contact_type"`
		ResultLimit     int    `json:"result_limit"`
		Channel         string `json:"channel"`
		EmailSubject    string `json:"email_subject"`
		MessageContent  string `json:"message_content"`
		IntervalMinutes int    `json:"interval_minutes"`
		MaxLeadsPerRun  int    `json:"max_leads_per_run"`
		MaxSendsPerRun  int    `json:"max_sends_per_run"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if strings.TrimSpace(req.Name) != "" {
		campaign.Name = strings.TrimSpace(req.Name)
	}
	if req.Description != "" {
		campaign.Description = strings.TrimSpace(req.Description)
	}
	if strings.TrimSpace(req.SearchKeywords) != "" {
		campaign.SearchKeywords = strings.TrimSpace(req.SearchKeywords)
	}
	campaign.SearchRegion = strings.TrimSpace(req.SearchRegion)

	if req.ProductID != "" {
		if pid, err := uuid.Parse(req.ProductID); err == nil {
			campaign.ProductID = &pid
		}
	}

	// Always update contact_type (can be empty = "不限")
	validContactTypes := map[string]bool{"": true, "email": true, "phone": true, "both": true}
	if validContactTypes[req.ContactType] {
		campaign.ContactType = req.ContactType
	}
	if req.ResultLimit > 0 {
		campaign.ResultLimit = req.ResultLimit
	}

	if req.Channel != "" {
		validChannels := map[string]bool{"email": true, "whatsapp": true, "both": true}
		if validChannels[req.Channel] {
			campaign.Channel = req.Channel
		}
	}

	if req.EmailSubject != "" {
		campaign.EmailSubject = strings.TrimSpace(req.EmailSubject)
	}
	if req.MessageContent != "" {
		campaign.MessageContent = strings.TrimSpace(req.MessageContent)
	}

	if req.IntervalMinutes >= 60 {
		campaign.IntervalMinutes = req.IntervalMinutes
	}
	if req.MaxLeadsPerRun > 0 {
		campaign.MaxLeadsPerRun = req.MaxLeadsPerRun
	}
	if req.MaxSendsPerRun > 0 {
		campaign.MaxSendsPerRun = req.MaxSendsPerRun
	}

	if err := database.DB.Save(&campaign).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(campaign)
}

// ToggleAutoOutreachCampaign toggles a campaign active/paused
func ToggleAutoOutreachCampaign(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var campaign models.AutoOutreachCampaign
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&campaign).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Campaign not found"})
	}

	if campaign.IsActive {
		// Pause
		campaign.IsActive = false
		campaign.Status = "paused"
	} else {
		// Resume
		campaign.IsActive = true
		campaign.Status = "active"
		campaign.ConsecutiveFails = 0
		now := time.Now()
		nextRun := now.Add(time.Duration(campaign.IntervalMinutes) * time.Minute)
		campaign.NextRunAt = &nextRun
	}

	if err := database.DB.Save(&campaign).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(campaign)
}

// DeleteAutoOutreachCampaign deletes a campaign
func DeleteAutoOutreachCampaign(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Delete logs first
	database.DB.Where("tenant_id = ? AND campaign_id = ?", tenantID, id).Delete(&models.AutoOutreachLog{})

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.AutoOutreachCampaign{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Campaign deleted"})
}

// GetAutoOutreachLogs returns execution logs for a campaign
func GetAutoOutreachLogs(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	campaignID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var logs []models.AutoOutreachLog
	var total int64

	query := database.DB.Where("tenant_id = ? AND campaign_id = ?", tenantID, campaignID)
	query.Model(&models.AutoOutreachLog{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  logs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// ============================================================
// POST /api/v1/auto-outreach/generate-content
// AI generates outreach message content
// ============================================================

func AutoOutreachGenerateContent(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req struct {
		Channel   string `json:"channel"`    // email, whatsapp, both
		ProductID string `json:"product_id"` // optional
		Keywords  string `json:"keywords"`   // target keywords
		Region    string `json:"region"`     // target region
		Tone      string `json:"tone"`       // professional, casual, friendly
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	cfg := config.Load()
	if cfg.LLM.APIKey == "" {
		return c.Status(500).JSON(fiber.Map{"error": "AI API key not configured"})
	}

	// Gather company context
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	companyName := enterprise.Name
	companyDesc := ""
	if enterprise.ExtraFields != nil {
		if desc, ok := enterprise.ExtraFields["description"].(string); ok {
			companyDesc = desc
		}
	}

	// Get products info
	var products []models.Product
	database.DB.Where("tenant_id = ?", tenantID).Order("RANDOM()").Limit(5).Find(&products)
	productNames := make([]string, 0, len(products))
	for _, prod := range products {
		desc := prod.Name
		if prod.Category != "" {
			desc += " (" + prod.Category + ")"
		}
		productNames = append(productNames, desc)
	}

	// Selected product
	selectedProduct := ""
	if req.ProductID != "" {
		prodID, err := uuid.Parse(req.ProductID)
		if err == nil {
			var prod models.Product
			if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, prodID).First(&prod).Error; err == nil {
				selectedProduct = fmt.Sprintf("重點產品: %s - %s (分類: %s, 價格: %.2f)", prod.Name, prod.Description, prod.Category, prod.Price)
			}
		}
	}

	channel := req.Channel
	if channel == "" {
		channel = "email"
	}

	tone := req.Tone
	if tone == "" {
		tone = "professional"
	}

	// Build AI prompt
	systemPrompt := `你是一個專業的B2B銷售文案撰寫專家。根據用戶提供的公司資料和目標客戶資訊，撰寫高轉換率的外展訊息。

你必須返回嚴格的JSON格式（不要加markdown格式標記），包含以下欄位：
- email_subject: 電郵主旨（如適用，吸引人的標題，30字內）
- email_content: HTML 格式的電郵內容（專業排版，包含公司介紹、產品亮點、行動呼籲）
- whatsapp_content: WhatsApp 純文字版本（簡短精煉，200字內，包含emoji）
- reasoning: 文案策略說明（簡述為何如此撰寫）`

	contextParts := []string{
		fmt.Sprintf("公司名稱: %s", companyName),
	}
	if companyDesc != "" {
		contextParts = append(contextParts, fmt.Sprintf("公司簡介: %s", companyDesc))
	}
	if len(productNames) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("產品/服務: %s", strings.Join(productNames, ", ")))
	}
	if selectedProduct != "" {
		contextParts = append(contextParts, selectedProduct)
	}
	if req.Keywords != "" {
		contextParts = append(contextParts, fmt.Sprintf("目標關鍵字: %s", req.Keywords))
	}
	if req.Region != "" {
		contextParts = append(contextParts, fmt.Sprintf("目標地區: %s", req.Region))
	}
	contextParts = append(contextParts, fmt.Sprintf("發送管道: %s", channel))
	contextParts = append(contextParts, fmt.Sprintf("語氣風格: %s", tone))

	userPrompt := fmt.Sprintf("以下是我的公司資料，請撰寫一封外展訊息：\n\n%s", strings.Join(contextParts, "\n"))

	// Call Gemini API
	model := cfg.LLM.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	geminiReq := map[string]interface{}{
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemPrompt},
			},
		},
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": userPrompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseMimeType": "application/json",
			"temperature":      0.8,
		},
	}

	payload, _ := json.Marshal(geminiReq)
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	httpReq, _ := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[AutoOutreach] Gemini API error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "AI service unavailable"})
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[AutoOutreach] Gemini API error status %d: %s", resp.StatusCode, string(body))
		return c.Status(500).JSON(fiber.Map{"error": "AI service error"})
	}

	// Parse response
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		log.Printf("[AutoOutreach] Failed to parse Gemini response: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse AI response"})
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return c.Status(500).JSON(fiber.Map{"error": "Empty AI response"})
	}

	jsonText := geminiResp.Candidates[0].Content.Parts[0].Text
	jsonText = strings.TrimSpace(jsonText)
	// Clean markdown fences if present
	if strings.HasPrefix(jsonText, "```") {
		lines := strings.Split(jsonText, "\n")
		if len(lines) > 2 {
			jsonText = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var result struct {
		EmailSubject    string `json:"email_subject"`
		EmailContent    string `json:"email_content"`
		WhatsAppContent string `json:"whatsapp_content"`
		Reasoning       string `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		log.Printf("[AutoOutreach] Failed to parse AI JSON: %v, raw: %s", err, jsonText)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse AI content", "raw": jsonText})
	}

	return c.JSON(fiber.Map{
		"email_subject":    result.EmailSubject,
		"email_content":    result.EmailContent,
		"whatsapp_content": result.WhatsAppContent,
		"reasoning":        result.Reasoning,
	})
}
