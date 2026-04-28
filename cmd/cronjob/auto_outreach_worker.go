package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nwork/config"
	"nwork/internal/database"
	emailpkg "nwork/internal/email"
	"nwork/internal/models"
	"nwork/internal/whatsapp"

	"github.com/google/uuid"
)

// runAutoOutreachJob scans all active campaigns with next_run_at <= NOW()
// and executes the outreach workflow for each:
//  1. Serper.dev search to find new leads
//  2. Send email / WhatsApp to found leads
//  3. Check email quota; if insufficient, notify admin
//  4. Auto-pause after 3 consecutive failures
func runAutoOutreachJob() {
	log.Println("[AutoOutreach] Running auto-outreach job at", time.Now().Format("2006-01-02 15:04:05"))

	now := time.Now()
	var campaigns []models.AutoOutreachCampaign
	if err := database.DB.
		Where("is_active = ? AND status = ? AND (next_run_at IS NULL OR next_run_at <= ?)", true, "active", now).
		Find(&campaigns).Error; err != nil {
		log.Printf("[AutoOutreach] Failed to fetch campaigns: %v", err)
		return
	}

	if len(campaigns) == 0 {
		return
	}

	cfg := config.Load()
	log.Printf("[AutoOutreach] Found %d campaign(s) to run", len(campaigns))

	for _, campaign := range campaigns {
		runSingleCampaign(cfg, campaign)
	}
}

func runSingleCampaign(cfg *config.Config, campaign models.AutoOutreachCampaign) {
	log.Printf("[AutoOutreach] Running campaign %s (%s) for tenant %s", campaign.Name, campaign.ID, campaign.TenantID)

	now := time.Now()
	logEntry := models.AutoOutreachLog{
		TenantID:   campaign.TenantID,
		CampaignID: campaign.ID,
		Status:     "running",
	}

	// Defer: always save the log entry and update campaign
	defer func() {
		logEntry.CreatedAt = time.Now()
		if err := database.DB.Create(&logEntry).Error; err != nil {
			log.Printf("[AutoOutreach] Failed to save log entry: %v", err)
		}

		// Schedule next run
		nextRun := time.Now().Add(time.Duration(campaign.IntervalMinutes) * time.Minute)
		campaign.NextRunAt = &nextRun
		campaign.LastRunAt = &now
		campaign.LastRunStatus = logEntry.Status
		campaign.LastRunMessage = logEntry.Message

		if err := database.DB.Save(&campaign).Error; err != nil {
			log.Printf("[AutoOutreach] Failed to update campaign: %v", err)
		}
	}()

	// Step 1: Check email quota (if channel includes email)
	if campaign.Channel == "email" || campaign.Channel == "both" {
		dailyUsage, err := emailpkg.GetDailyEmailUsage()
		if err != nil {
			log.Printf("[AutoOutreach] Failed to get daily email usage: %v", err)
		}
		dailyLimit := int64(emailpkg.GetBrevoFreeDailyLimit())
		remaining := dailyLimit - dailyUsage

		logEntry.QuotaUsed = dailyUsage
		logEntry.QuotaRemaining = remaining

		if remaining < int64(campaign.MaxSendsPerRun) {
			// Quota insufficient — notify admin and skip
			logEntry.Status = "quota_exceeded"
			logEntry.Message = fmt.Sprintf("Email quota insufficient: used %d / limit %d, remaining %d, need %d",
				dailyUsage, dailyLimit, remaining, campaign.MaxSendsPerRun)
			campaign.Status = "quota_exceeded"
			campaign.IsActive = false // pause until admin checks
			log.Printf("[AutoOutreach] %s", logEntry.Message)

			// Notify admin
			emailpkg.EnqueueAdminNotification("auto_outreach_quota_exceeded", map[string]string{
				"campaign_name": campaign.Name,
				"campaign_id":   campaign.ID.String(),
				"daily_usage":   fmt.Sprintf("%d", dailyUsage),
				"daily_limit":   fmt.Sprintf("%d", dailyLimit),
				"remaining":     fmt.Sprintf("%d", remaining),
			})
			return
		}
	}

	// Step 2: Find new leads via Google Custom Search
	leads, err := findLeadsForCampaign(cfg, campaign)
	if err != nil {
		logEntry.Status = "failed"
		logEntry.Message = "Lead search failed: " + err.Error()
		campaign.ConsecutiveFails++
		if campaign.ConsecutiveFails >= 3 {
			campaign.IsActive = false
			campaign.Status = "paused"
			logEntry.Message += " (auto-paused after 3 consecutive failures)"
		}
		log.Printf("[AutoOutreach] %s", logEntry.Message)
		return
	}

	logEntry.LeadsFound = len(leads)
	campaign.TotalLeadsFound += len(leads)

	if len(leads) == 0 {
		logEntry.Status = "success"
		logEntry.Message = "No new leads found for this run"
		campaign.ConsecutiveFails = 0
		log.Printf("[AutoOutreach] No new leads found for campaign %s", campaign.ID)
		return
	}

	// Step 3: Send outreach messages
	emailsSent := 0
	whatsAppSent := 0
	failCount := 0

	maxSends := campaign.MaxSendsPerRun
	sentCount := 0

	emailSubject := strings.TrimSpace(campaign.EmailSubject)
	if emailSubject == "" {
		emailSubject = campaign.Name
	}

	// Initialize WhatsApp client if needed
	var waClient *whatsapp.Client
	if campaign.Channel == "whatsapp" || campaign.Channel == "both" {
		waClient = whatsapp.NewClient(cfg)
	}

	for _, lead := range leads {
		if sentCount >= maxSends {
			break
		}

		sentAny := false

		// Send email
		if (campaign.Channel == "email" || campaign.Channel == "both") && strings.TrimSpace(lead.Email) != "" {
			err := emailpkg.EnqueuePromotionEmail(
				campaign.TenantID,
				campaign.ID, // use campaign ID as promotion ID for tracking
				lead.Email,
				lead.CompanyName,
				emailSubject,
				campaign.MessageContent,
				"", // no unsubscribe URL for now
			)
			if err != nil {
				log.Printf("[AutoOutreach] Email enqueue failed for %s: %v", lead.Email, err)
				failCount++
			} else {
				emailsSent++
				sentAny = true
			}
		}

		// Send WhatsApp
		if (campaign.Channel == "whatsapp" || campaign.Channel == "both") && strings.TrimSpace(lead.Phone) != "" {
			if waClient != nil && waClient.IsConfigured() {
				if err := waClient.SendTextMessage(lead.Phone, campaign.MessageContent); err != nil {
					log.Printf("[AutoOutreach] WhatsApp send failed for %s: %v", lead.Phone, err)
					failCount++
				} else {
					whatsAppSent++
					sentAny = true
				}
			}
		}

		if sentAny {
			sentCount++
			// Mark lead as contacted
			database.DB.Model(&lead).Update("status", "contacted")
		}
	}

	logEntry.EmailsSent = emailsSent
	logEntry.WhatsAppSent = whatsAppSent
	logEntry.FailCount = failCount

	campaign.TotalSentCount += emailsSent + whatsAppSent

	if failCount > 0 && emailsSent == 0 && whatsAppSent == 0 {
		logEntry.Status = "failed"
		logEntry.Message = fmt.Sprintf("All sends failed: %d failures", failCount)
		campaign.ConsecutiveFails++
		if campaign.ConsecutiveFails >= 3 {
			campaign.IsActive = false
			campaign.Status = "paused"
			logEntry.Message += " (auto-paused after 3 consecutive failures)"
		}
	} else if failCount > 0 {
		logEntry.Status = "partial"
		logEntry.Message = fmt.Sprintf("Partial: %d emails, %d WhatsApp sent, %d failed", emailsSent, whatsAppSent, failCount)
		campaign.ConsecutiveFails = 0
	} else {
		logEntry.Status = "success"
		logEntry.Message = fmt.Sprintf("Success: %d emails, %d WhatsApp sent", emailsSent, whatsAppSent)
		campaign.ConsecutiveFails = 0
	}

	log.Printf("[AutoOutreach] Campaign %s result: %s", campaign.ID, logEntry.Message)
}

// findLeadsForCampaign performs Serper.dev search and saves new leads
func findLeadsForCampaign(cfg *config.Config, campaign models.AutoOutreachCampaign) ([]models.LeadFinderResult, error) {
	if cfg.Serper.APIKey == "" {
		return nil, fmt.Errorf("Serper API not configured")
	}

	query := campaign.SearchKeywords
	if campaign.SearchRegion != "" {
		query += " " + campaign.SearchRegion
	}

	// Serper.dev: POST https://google.serper.dev/search
	reqBody, _ := json.Marshal(map[string]interface{}{
		"q":   query,
		"num": 10,
	})

	serperReq, err := http.NewRequest("POST", "https://google.serper.dev/search", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create Serper request: %w", err)
	}
	serperReq.Header.Set("X-API-KEY", cfg.Serper.APIKey)
	serperReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(serperReq)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		// Detect Serper.dev credit exhaustion → notify admin
		if resp.StatusCode == 402 || resp.StatusCode == 429 ||
			strings.Contains(strings.ToLower(bodyStr), "credit") ||
			strings.Contains(strings.ToLower(bodyStr), "limit") ||
			strings.Contains(strings.ToLower(bodyStr), "quota") {
			emailpkg.EnqueueAdminNotification("serper_credit_exhausted", map[string]string{
				"status_code": fmt.Sprintf("%d", resp.StatusCode),
				"response":    bodyStr,
				"query":       query,
				"source":      "auto_outreach",
				"campaign_id": campaign.ID.String(),
				"timestamp":   time.Now().Format(time.RFC3339),
			})
		}

		return nil, fmt.Errorf("search API error %d: %s", resp.StatusCode, bodyStr)
	}

	var serperResp struct {
		Organic []struct {
			Title    string `json:"title"`
			Link     string `json:"link"`
			Snippet  string `json:"snippet"`
			Position int    `json:"position"`
		} `json:"organic"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&serperResp); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	// Create a search record for tracking
	search := models.LeadFinderSearch{
		TenantID:    campaign.TenantID,
		CreatedByID: campaign.CreatedByID,
		Keywords:    campaign.SearchKeywords,
		Region:      campaign.SearchRegion,
		ProductID:   campaign.ProductID,
		Status:      "completed",
		ExtraFields: models.JSONB{"source": "auto_outreach", "campaign_id": campaign.ID.String()},
	}
	database.DB.Create(&search)

	// Process results — extract contacts, dedup
	results := make([]models.LeadFinderResult, 0, len(serperResp.Organic))
	maxLeads := campaign.MaxLeadsPerRun

	for i, item := range serperResp.Organic {
		if len(results) >= maxLeads {
			break
		}

		result := models.LeadFinderResult{
			TenantID:    campaign.TenantID,
			SearchID:    search.ID,
			CompanyName: extractCompanyNameFromTitle(item.Title),
			Website:     item.Link,
			Description: item.Snippet,
			SourceURL:   item.Link,
			SourceTitle: item.Title,
			Relevance:   100 - (i * 10),
			Status:      "new",
		}

		// Extract phone and email from snippet only (Serper has no pagemap/metatags)
		result.Phone = extractPhoneFromSnippet(item.Snippet)
		result.Email = extractEmailFromSnippet(item.Snippet)

		// Filter by contact type preference
		switch campaign.ContactType {
		case "email":
			if result.Email == "" {
				continue
			}
		case "phone":
			if result.Phone == "" {
				continue
			}
		case "both":
			if result.Email == "" || result.Phone == "" {
				continue
			}
		default:
			// "" = no filter, but still need at least one contact method for sending
			if result.Email == "" && result.Phone == "" {
				continue
			}
		}

		// Compute normalized fields for dedup
		result.WebsiteDomain = extractDomain(result.Website)
		result.NormalizedPhone = normalizePhoneNumber(result.Phone)

		// Dedup check
		isDup := checkDuplicate(campaign.TenantID, result.WebsiteDomain, result.NormalizedPhone, result.Email)
		if isDup {
			continue // Skip duplicates entirely for auto-outreach
		}

		if err := database.DB.Create(&result).Error; err != nil {
			log.Printf("[AutoOutreach] Failed to save lead result: %v", err)
			continue
		}
		results = append(results, result)
	}

	// Update search record
	database.DB.Model(&search).Updates(map[string]interface{}{
		"result_count": len(results),
	})

	return results, nil
}

// ============================================================
// Helper functions (duplicated from lead_finder.go since they
// are in the handlers package and we're in the main package)
// ============================================================

func extractCompanyNameFromTitle(title string) string {
	separators := []string{" - ", " | ", " — ", " · ", " :: "}
	for _, sep := range separators {
		parts := strings.SplitN(title, sep, 2)
		if len(parts) > 0 && len(parts[0]) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	return strings.TrimSpace(title)
}

func extractPhoneFromSnippet(text string) string {
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		if runes[i] == '+' || (runes[i] >= '0' && runes[i] <= '9') || runes[i] == '(' {
			start := i
			digitCount := 0
			for i < len(runes) && (runes[i] >= '0' && runes[i] <= '9' || runes[i] == '+' || runes[i] == '-' || runes[i] == ' ' || runes[i] == '(' || runes[i] == ')') {
				if runes[i] >= '0' && runes[i] <= '9' {
					digitCount++
				}
				i++
			}
			if digitCount >= 8 && digitCount <= 15 {
				return strings.TrimSpace(string(runes[start:i]))
			}
		} else {
			i++
		}
	}
	return ""
}

func extractEmailFromSnippet(text string) string {
	words := strings.Fields(text)
	for _, word := range words {
		word = strings.Trim(word, ".,;:!?<>()[]{}\"'")
		if strings.Contains(word, "@") && strings.Contains(word, ".") {
			parts := strings.Split(word, "@")
			if len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 2 && strings.Contains(parts[1], ".") {
				return word
			}
		}
	}
	return ""
}

func extractDomain(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

func normalizePhoneNumber(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	var buf bytes.Buffer
	for _, r := range phone {
		if r >= '0' && r <= '9' || r == '+' {
			buf.WriteRune(r)
		}
	}
	normalized := buf.String()
	if strings.HasPrefix(normalized, "00") && len(normalized) > 4 {
		normalized = "+" + normalized[2:]
	}
	return normalized
}

func checkDuplicate(tenantID uuid.UUID, domain, normalizedPhone, email string) bool {
	db := database.DB
	emailLower := strings.ToLower(strings.TrimSpace(email))

	// Check against existing lead_finder_results
	if domain != "" {
		var count int64
		db.Model(&models.LeadFinderResult{}).Where("tenant_id = ? AND website_domain = ? AND website_domain != ''", tenantID, domain).Count(&count)
		if count > 0 {
			return true
		}
	}
	if normalizedPhone != "" {
		var count int64
		db.Model(&models.LeadFinderResult{}).Where("tenant_id = ? AND normalized_phone = ? AND normalized_phone != ''", tenantID, normalizedPhone).Count(&count)
		if count > 0 {
			return true
		}
	}
	if emailLower != "" {
		var count int64
		db.Model(&models.LeadFinderResult{}).Where("tenant_id = ? AND lower(email) = ? AND email != ''", tenantID, emailLower).Count(&count)
		if count > 0 {
			return true
		}
	}

	// Check against existing customers
	if normalizedPhone != "" {
		var count int64
		db.Model(&models.Customer{}).Where("tenant_id = ? AND regexp_replace(phone, '[^0-9+]', '', 'g') = ? AND phone != ''", tenantID, normalizedPhone).Count(&count)
		if count > 0 {
			return true
		}
	}
	if emailLower != "" {
		var count int64
		db.Model(&models.Customer{}).Where("tenant_id = ? AND lower(email) = ? AND email != ''", tenantID, emailLower).Count(&count)
		if count > 0 {
			return true
		}
	}

	return false
}
