package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// ============================================================
// Request / Response types
// ============================================================

type leadFinderAnalyzeRequest struct {
	ProductID string `json:"product_id,omitempty"` // optional product filter
	Region    string `json:"region,omitempty"`     // optional region filter
}

type leadFinderSearchRequest struct {
	Keywords             string `json:"keywords"`                  // AI-generated or user-edited keywords
	ProductID            string `json:"product_id,omitempty"`      // optional product filter
	Region               string `json:"region,omitempty"`          // optional region filter
	TargetIndustry       string `json:"target_industry,omitempty"` // AI-inferred target industry
	ContactType          string `json:"contact_type,omitempty"`    // "email", "phone", "both", or "" (all)
	ResultLimit          int    `json:"result_limit,omitempty"`
	ExcludeInstitutional bool   `json:"exclude_institutional,omitempty"` // exclude .gov/.org/.edu domains
	SkipDuplicates       bool   `json:"skip_duplicates,omitempty"`       // skip leads that already exist in tenant's DB
}

const (
	leadFinderDefaultResultLimit = 50
	leadFinderMaxResultLimit     = 500
	serperDefaultPageSize        = 100 // Serper.dev charges per API call, not per result — always request max
	serperMaxPageSize            = 100 // Serper.dev max results per request
)

// contactEnrichmentSuffixes are appended to original keywords to generate
// variant queries that are more likely to surface pages containing contact info.
var contactEnrichmentSuffixes = []string{
	"contact email",
	"phone email directory",
	"company list contact",
}

// institutionalTLDs are top-level domain suffixes that indicate government,
// educational, or non-commercial institutional websites. These are never
// real B2B leads but often rank high in SEO results.
var institutionalTLDs = []string{
	".gov", ".edu", ".mil", ".int", ".org",
	".gov.hk", ".gov.uk", ".gov.au", ".gov.sg", ".gov.tw", ".gov.cn", ".gov.my",
	".gov.ph", ".gov.in", ".gov.za", ".gov.br", ".gov.ca", ".gov.nz", ".gov.jp",
	".edu.hk", ".edu.au", ".edu.sg", ".edu.tw", ".edu.cn", ".edu.my",
	".edu.ph", ".edu.in", ".edu.za", ".edu.br", ".edu.uk",
	".ac.uk", ".ac.jp", ".ac.kr", ".ac.nz",
	".org.hk", ".org.tw", ".org.uk", ".org.au", ".org.sg", ".org.cn", ".org.my",
	".org.nz", ".org.in", ".org.za", ".org.br", ".org.ph", ".org.jp",
}

// NOTE on Serper.dev query-level domain exclusion:
// Serper supports Google -site: operators in the query string, BUT imposes a
// hard limit: when -site: operators are present, num must be <= 20 (otherwise
// HTTP 400 "Query not allowed"). Since num=20 returns at most ~10 results and
// num=100 (without -site:) returns the same ~10 results, query-level exclusion
// offers no real benefit — it only restricts our page size for no gain.
// Therefore we rely entirely on the post-filter (isInstitutionalDomain) which
// runs after results are returned with the full num=100 page size.

// isInstitutionalDomain checks if a domain belongs to an institutional TLD.
// This serves as a post-filter to catch country-specific variants (e.g.
// .gov.hk, .edu.au) that the Serper query-level exclusion may miss.
func isInstitutionalDomain(domain string) bool {
	domain = strings.ToLower(domain)
	for _, tld := range institutionalTLDs {
		if strings.HasSuffix(domain, tld) {
			return true
		}
	}
	return false
}

func normalizeLeadFinderResultLimit(limit int) int {
	if limit <= 0 {
		return leadFinderDefaultResultLimit
	}
	if limit > leadFinderMaxResultLimit {
		return leadFinderMaxResultLimit
	}
	return limit
}

func buildLeadFinderQueries(keywords string, region string) []string {
	parts := strings.FieldsFunc(keywords, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '\n', '|':
			return true
		default:
			return false
		}
	})

	queries := make([]string, 0, len(parts)*3)
	seen := make(map[string]struct{})
	appendQuery := func(q string) {
		q = strings.TrimSpace(q)
		if q == "" {
			return
		}
		if region != "" && !strings.Contains(strings.ToLower(q), strings.ToLower(region)) {
			q += " " + region
		}
		key := strings.ToLower(q)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		queries = append(queries, q)
	}

	// First pass: add original keywords
	originals := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p != "" {
			originals = append(originals, p)
			appendQuery(p)
		}
	}

	if len(originals) == 0 {
		originals = append(originals, keywords)
		appendQuery(keywords)
	}

	// Second pass: generate contact-enrichment variants for each original keyword.
	// These are appended AFTER all originals so they serve as fallback queries
	// that are more likely to surface pages with email/phone info.
	for _, orig := range originals {
		for _, suffix := range contactEnrichmentSuffixes {
			// Skip if the original keyword already contains the enrichment words
			lowerOrig := strings.ToLower(orig)
			if strings.Contains(lowerOrig, "contact") || strings.Contains(lowerOrig, "email") || strings.Contains(lowerOrig, "directory") {
				continue
			}
			appendQuery(orig + " " + suffix)
		}
	}

	return queries
}

type leadFinderResultUpdateRequest struct {
	Status string `json:"status"` // new, contacted, converted, dismissed
	Notes  string `json:"notes,omitempty"`
}

// AI analysis response from Gemini
type leadFinderAIAnalysis struct {
	TargetIndustry string   `json:"target_industry"`
	TargetProfile  string   `json:"target_profile"`
	SearchKeywords []string `json:"search_keywords"`
	ReasoningNotes string   `json:"reasoning_notes"`
}

// Serper.dev API response (replaces Google Custom Search)
type serperSearchResponse struct {
	Organic []serperSearchItem `json:"organic"`
}

type serperSearchItem struct {
	Title    string `json:"title"`
	Link     string `json:"link"`
	Snippet  string `json:"snippet"`
	Position int    `json:"position"`
}

// ============================================================
// POST /api/v1/lead-finder/analyze
// AI analyzes company context → generates keywords & target profile
// ============================================================

func LeadFinderAnalyze(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req leadFinderAnalyzeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	cfg := config.Load()
	if cfg.LLM.APIKey == "" {
		return c.Status(500).JSON(fiber.Map{"error": "AI API key not configured"})
	}

	// 1. Gather company context
	var enterprise models.Enterprise
	database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

	// Company info from ExtraFields
	companyName := enterprise.Name
	companyDesc := ""
	companyCategory := ""
	if enterprise.ExtraFields != nil {
		if desc, ok := enterprise.ExtraFields["description"].(string); ok {
			companyDesc = desc
		}
		if cat, ok := enterprise.ExtraFields["category"].(string); ok {
			companyCategory = cat
		}
	}

	// Last 5 customers
	var customers []models.Customer
	database.DB.Where("tenant_id = ?", tenantID).Order("created_at DESC").Limit(5).Find(&customers)
	customerNames := make([]string, 0, len(customers))
	for _, cust := range customers {
		name := cust.Name
		if cust.LastName != "" {
			name += " " + cust.LastName
		}
		customerNames = append(customerNames, name)
	}

	// Random 5 products
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

	// Optional: if user selected a specific product
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

	// 2. Build AI prompt
	systemPrompt := `你是一個專業的B2B銷售顧問和市場分析師。根據用戶提供的公司資料，分析其業務特徵，找出最有可能成為其客戶的目標對象。

**重要**：你生成的搜尋條件的核心目標是找到有公開聯絡方式（電郵、電話）的潛在客戶。搜尋策略：
1. 至少一半的搜尋條件應包含 "contact"、"email"、"directory"、"supplier list"、"company list" 等能引導搜尋到聯絡頁面或企業名錄的詞語
2. 優先搜尋行業目錄、商會名錄、B2B平台（如 Alibaba、Kompass、Yellow Pages、ThomasNet 等）上的企業列表
3. 搜尋條件示例格式："[行業] companies [地區] contact email"、"[行業] directory [地區]"、"[行業] supplier list [地區] phone"
4. 避免過於通用的條件（如只有行業名稱），要具體到能搜出帶聯絡資訊的頁面

你必須返回嚴格的JSON格式（不要加markdown格式標記），包含以下欄位：
- target_industry: 目標行業（字串）
- target_profile: 目標客戶畫像描述（字串，100字內）
- search_keywords: 用於搜尋潛在客戶的條件列表（字串陣列，8-15個組合，要包含地區/行業/需求等組合詞，至少一半要包含 contact/email/directory 等詞）
- reasoning_notes: 分析推理說明（字串，簡述為何推薦這些目標，不要提及「關鍵字」或「搜尋條件」等技術細節，只從業務角度說明）`

	contextParts := []string{
		fmt.Sprintf("公司名稱: %s", companyName),
	}
	if companyDesc != "" {
		contextParts = append(contextParts, fmt.Sprintf("公司簡介: %s", companyDesc))
	}
	if companyCategory != "" {
		contextParts = append(contextParts, fmt.Sprintf("業務類別: %s", companyCategory))
	}
	if len(customerNames) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("最近客戶: %s", strings.Join(customerNames, ", ")))
	}
	if len(productNames) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("產品/服務: %s", strings.Join(productNames, ", ")))
	}
	if selectedProduct != "" {
		contextParts = append(contextParts, selectedProduct)
	}
	if req.Region != "" {
		contextParts = append(contextParts, fmt.Sprintf("目標地區: %s", req.Region))
	}

	userPrompt := fmt.Sprintf("以下是我的公司資料，請分析並生成搜尋潛在客戶的關鍵字：\n\n%s", strings.Join(contextParts, "\n"))

	// 3. Call Gemini API
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
			"temperature":      0.7,
		},
	}

	payload, _ := json.Marshal(geminiReq)
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	httpReq, _ := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[LeadFinder] Gemini API error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "AI service unavailable"})
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[LeadFinder] Gemini API error status %d: %s", resp.StatusCode, string(body))
		return c.Status(500).JSON(fiber.Map{"error": "AI service error"})
	}

	// 4. Parse response
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
		log.Printf("[LeadFinder] Failed to parse Gemini response: %v", err)
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

	var analysis leadFinderAIAnalysis
	if err := json.Unmarshal([]byte(jsonText), &analysis); err != nil {
		log.Printf("[LeadFinder] Failed to parse AI JSON: %v, raw: %s", err, jsonText)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse AI analysis", "raw": jsonText})
	}

	return c.JSON(fiber.Map{
		"analysis": analysis,
		"context": fiber.Map{
			"company_name":     companyName,
			"company_desc":     companyDesc,
			"company_category": companyCategory,
			"customers":        customerNames,
			"products":         productNames,
		},
	})
}

// ============================================================
// POST /api/v1/lead-finder/search
// Execute Serper.dev search + extract contacts → save to DB
// ============================================================

func LeadFinderSearch(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req leadFinderSearchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if strings.TrimSpace(req.Keywords) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Keywords are required"})
	}

	req.ResultLimit = normalizeLeadFinderResultLimit(req.ResultLimit)

	cfg := config.Load()
	if cfg.Serper.APIKey == "" {
		return c.Status(500).JSON(fiber.Map{"error": "Search API not configured"})
	}

	// 1. Create search record
	var productID *uuid.UUID
	if req.ProductID != "" {
		if pid, err := uuid.Parse(req.ProductID); err == nil {
			productID = &pid
		}
	}

	search := models.LeadFinderSearch{
		TenantID:       tenantID,
		CreatedByID:    userID,
		Keywords:       req.Keywords,
		Region:         req.Region,
		TargetIndustry: req.TargetIndustry,
		ProductID:      productID,
		Status:         "searching",
		ExtraFields: models.JSONB{
			"contact_type":          req.ContactType,
			"requested_limit":       req.ResultLimit,
			"exclude_institutional": req.ExcludeInstitutional,
			"skip_duplicates":       req.SkipDuplicates,
		},
	}

	if err := database.DB.Create(&search).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create search record"})
	}

	// 2. Execute Serper.dev search across keyword groups until requested limit is reached.
	// Strategy: iterate over all queries (original + contact-enrichment variants).
	// For each query, paginate up to serperMaxPages pages so we maximise the chance
	// of finding leads that contain email/phone in their snippet.
	queries := buildLeadFinderQueries(req.Keywords, req.Region)
	search.ExtraFields["query_count"] = len(queries)

	client := &http.Client{Timeout: 30 * time.Second}

	const serperMaxPages = 3 // max pages per query (page size * 3 = up to 90 results per keyword)

	// 3. Process results — extract phone/email, dedup check
	results := make([]models.LeadFinderResult, 0, req.ResultLimit)
	dupCount := 0
	totalSerperCalls := 0
	seenURLs := make(map[string]struct{}) // track URLs across pages to avoid processing duplicates

	for _, query := range queries {
		if len(results) >= req.ResultLimit {
			break
		}

		// Paginate within this query
		for page := 0; page < serperMaxPages; page++ {
			if len(results) >= req.ResultLimit {
				break
			}

			serperPayload := map[string]interface{}{
				"q":   query,
				"num": serperDefaultPageSize,
			}
			if page > 0 {
				serperPayload["start"] = page * serperDefaultPageSize
			}

			reqBody, _ := json.Marshal(serperPayload)

			serperReq, err := http.NewRequest("POST", "https://google.serper.dev/search", bytes.NewReader(reqBody))
			if err != nil {
				log.Printf("[LeadFinder] Failed to create Serper request: %v", err)
				break
			}
			serperReq.Header.Set("X-API-KEY", cfg.Serper.APIKey)
			serperReq.Header.Set("Content-Type", "application/json")

			searchResp, err := client.Do(serperReq)
			if err != nil {
				log.Printf("[LeadFinder] Serper request error: %v", err)
				break
			}
			totalSerperCalls++

			var serperResp serperSearchResponse
			func() {
				defer searchResp.Body.Close()

				if searchResp.StatusCode != 200 {
					body, _ := io.ReadAll(searchResp.Body)
					bodyStr := string(body)
					log.Printf("[LeadFinder] Serper error status %d: %s", searchResp.StatusCode, bodyStr)

					// Detect Serper.dev credit exhaustion (402/429 or credit-related message)
					if searchResp.StatusCode == 402 || searchResp.StatusCode == 429 ||
						strings.Contains(strings.ToLower(bodyStr), "credit") ||
						strings.Contains(strings.ToLower(bodyStr), "limit") ||
						strings.Contains(strings.ToLower(bodyStr), "quota") {
						email.EnqueueAdminNotification("serper_credit_exhausted", map[string]string{
							"status_code": fmt.Sprintf("%d", searchResp.StatusCode),
							"response":    bodyStr,
							"query":       query,
							"timestamp":   time.Now().Format(time.RFC3339),
						})
					}

					err = fmt.Errorf("Serper API %d: %s", searchResp.StatusCode, bodyStr)
					return
				}

				if decodeErr := json.NewDecoder(searchResp.Body).Decode(&serperResp); decodeErr != nil {
					log.Printf("[LeadFinder] Failed to parse Serper response: %v", decodeErr)
					err = fmt.Errorf("failed to parse search results")
				}
			}()
			if err != nil {
				// On API-level failure (e.g. credit exhausted), stop entirely
				database.DB.Model(&search).Updates(map[string]interface{}{"status": "failed"})
				return c.Status(500).JSON(fiber.Map{"error": err.Error()})
			}

			if len(serperResp.Organic) == 0 {
				break // No more results for this query, move to next keyword
			}

			contactHitsThisPage := 0
			for i, item := range serperResp.Organic {
				// Dedup by URL within this search session
				if _, seen := seenURLs[item.Link]; seen {
					continue
				}
				seenURLs[item.Link] = struct{}{}

				// Post-filter: skip institutional domains (.gov, .org, .edu, etc.)
				// This catches country-specific variants (e.g. .gov.hk, .edu.au)
				// that the Serper query-level -site: exclusion may miss.
				if req.ExcludeInstitutional {
					domain := extractDomainFromURL(item.Link)
					if isInstitutionalDomain(domain) {
						continue
					}
				}

				absoluteIndex := (totalSerperCalls-1)*serperDefaultPageSize + i
				result := models.LeadFinderResult{
					TenantID:    tenantID,
					SearchID:    search.ID,
					CompanyName: extractCompanyName(item.Title),
					Website:     item.Link,
					Description: item.Snippet,
					SourceURL:   item.Link,
					SourceTitle: item.Title,
					Relevance:   max(10, 100-(absoluteIndex%30)*3),
					Status:      "new",
				}

				result.Phone = extractPhoneFromText(item.Snippet)
				result.Email = extractEmailFromText(item.Snippet)

				// Contact-type filter: enforce that leads always have usable contact info.
				// Even when user picks "all" (empty contact_type), we require at least
				// one contact method — paying customers expect actionable leads.
				switch req.ContactType {
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
					// "any" — require at least one contact method
					if result.Email == "" && result.Phone == "" {
						continue
					}
				}

				contactHitsThisPage++
				result.WebsiteDomain = extractDomainFromURL(result.Website)
				result.NormalizedPhone = normalizePhone(result.Phone)

				isDup, dupType, dupInfo := checkLeadDuplicate(tenantID, result.WebsiteDomain, result.NormalizedPhone, result.Email)
				if isDup {
					dupCount++
					// When skip_duplicates is enabled, completely skip leads that
					// already exist in the tenant's DB — don't save or return them.
					if req.SkipDuplicates {
						continue
					}
					if result.ExtraFields == nil {
						result.ExtraFields = make(models.JSONB)
					}
					result.ExtraFields["is_duplicate"] = true
					result.ExtraFields["duplicate_type"] = dupType
					result.ExtraFields["duplicate_match"] = dupInfo
				}

				if err := database.DB.Create(&result).Error; err != nil {
					log.Printf("[LeadFinder] Failed to save result: %v", err)
					continue
				}
				results = append(results, result)
				if len(results) >= req.ResultLimit {
					break
				}
			}

			// If this page yielded zero contact-bearing results, don't bother
			// fetching the next page for this query — move on to the next keyword.
			if contactHitsThisPage == 0 {
				break
			}
		}
	}

	// 4. vCoin billing — charge per 50 results
	resultCount := len(results)
	log.Printf("[LeadFinder] Search %s completed: %d/%d results (fill rate %.0f%%), %d Serper API calls, %d duplicates",
		search.ID, resultCount, req.ResultLimit,
		float64(resultCount)/float64(req.ResultLimit)*100,
		totalSerperCalls, dupCount)

	searchCoins := 0
	if resultCount > 0 {
		searchCoins = models.CalcLeadSearchCoins(resultCount)
		ok, remaining, err := ConsumeAICoins(tenantID, userID, searchCoins,
			fmt.Sprintf("Lead Finder 搜尋 — %d 筆結果", resultCount),
			models.JSONB{"search_id": search.ID.String(), "result_count": resultCount})
		if !ok || err != nil {
			log.Printf("[LeadFinder] vCoin billing failed (tenant=%s, coins=%d, remaining=%d): %v",
				tenantID, searchCoins, remaining, err)
			// Results already saved — billing failure is non-blocking but logged
		}
	}

	// 5. Update search status
	database.DB.Model(&search).Updates(map[string]interface{}{
		"status":       "completed",
		"result_count": resultCount,
		"extra_fields": models.JSONB{
			"contact_type":          req.ContactType,
			"requested_limit":       req.ResultLimit,
			"exclude_institutional": req.ExcludeInstitutional,
			"skip_duplicates":       req.SkipDuplicates,
			"query_count":           len(queries),
			"serper_api_calls":      totalSerperCalls,
			"duplicate_count":       dupCount,
			"fill_rate_percent":     int(float64(resultCount) / float64(req.ResultLimit) * 100),
			"vcoin_cost":            searchCoins,
		},
	})
	search.Status = "completed"
	search.ResultCount = resultCount

	return c.JSON(fiber.Map{
		"search":          search,
		"results":         results,
		"duplicate_count": dupCount,
	})
}

// ============================================================
// GET /api/v1/lead-finder/searches
// List past searches for this tenant
// ============================================================

func LeadFinderGetSearches(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var searches []models.LeadFinderSearch
	if err := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).Order("created_at DESC").Limit(50).Find(&searches).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": searches})
}

// ============================================================
// GET /api/v1/lead-finder/searches/:id/results
// Get results for a specific search
// ============================================================

func LeadFinderGetResults(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	searchID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Verify search belongs to tenant
	var search models.LeadFinderSearch
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, searchID).First(&search).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Search not found"})
	}

	var results []models.LeadFinderResult
	if err := database.DB.Where("tenant_id = ? AND search_id = ?", tenantID, searchID).Order("relevance DESC").Find(&results).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"search":  search,
		"results": results,
	})
}

// ============================================================
// GET /api/v1/lead-finder/results
// List ALL lead results across all searches (with filters)
// ============================================================

func LeadFinderGetAllResults(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	query := database.DB.Where("lead_finder_results.tenant_id = ?", tenantID)

	// Status filter
	if status := c.Query("status"); status != "" {
		query = query.Where("lead_finder_results.status = ?", status)
	}

	// Contact type filter
	switch c.Query("contact_type") {
	case "email":
		query = query.Where("lead_finder_results.email != ''")
	case "phone":
		query = query.Where("lead_finder_results.phone != ''")
	case "both":
		query = query.Where("lead_finder_results.email != '' AND lead_finder_results.phone != ''")
	}

	// Duplicate filter
	if c.Query("hide_duplicates") == "true" {
		query = query.Where("(lead_finder_results.extra_fields->>'is_duplicate')::boolean IS NOT TRUE")
	}

	// Search text filter (company name, email, phone)
	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		query = query.Where("(lead_finder_results.company_name ILIKE ? OR lead_finder_results.email ILIKE ? OR lead_finder_results.phone ILIKE ?)", like, like, like)
	}

	// Count total
	var total int64
	query.Model(&models.LeadFinderResult{}).Count(&total)

	// Pagination
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

	var results []models.LeadFinderResult
	if err := query.Order("lead_finder_results.created_at DESC").Offset(offset).Limit(limit).Find(&results).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  results,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// ============================================================
// PUT /api/v1/lead-finder/results/:id
// Update result status / notes
// ============================================================

func LeadFinderUpdateResult(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	resultID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req leadFinderResultUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	validStatuses := map[string]bool{"new": true, "contacted": true, "converted": true, "dismissed": true}
	if !validStatuses[req.Status] {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid status. Must be: new, contacted, converted, dismissed"})
	}

	var result models.LeadFinderResult
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, resultID).First(&result).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Result not found"})
	}

	updates := map[string]interface{}{
		"status": req.Status,
	}
	if req.Notes != "" {
		updates["notes"] = req.Notes
	}

	if err := database.DB.Model(&result).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	result.Status = req.Status
	if req.Notes != "" {
		result.Notes = req.Notes
	}

	return c.JSON(fiber.Map{"data": result})
}

// ============================================================
// DELETE /api/v1/lead-finder/searches/:id
// Delete a search and all its associated results
// ============================================================

func LeadFinderDeleteSearch(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	searchID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Soft delete: set trashed_at = NOW() (moves to trash, 7-day retention)
	now := time.Now()
	if err := database.DB.Model(&models.LeadFinderSearch{}).
		Where("tenant_id = ? AND id = ? AND trashed_at IS NULL", tenantID, searchID).
		Update("trashed_at", now).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Search moved to trash"})
}

// ============================================================
// DELETE /api/v1/lead-finder/results/:id
// Delete a lead result
// ============================================================

func LeadFinderDeleteResult(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	resultID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, resultID).Delete(&models.LeadFinderResult{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Result deleted"})
}

// ============================================================
// POST /api/v1/lead-finder/results/:id/convert
// Convert a lead to a customer
// ============================================================

func LeadFinderConvertToCustomer(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	resultID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var result models.LeadFinderResult
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, resultID).First(&result).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Result not found"})
	}

	if result.Status == "converted" && result.ConvertedToCustomerID != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Already converted to customer"})
	}

	// Create customer from lead
	customer := models.Customer{
		TenantID: tenantID,
		Name:     result.CompanyName,
		Email:    result.Email,
		Phone:    result.Phone,
		Address:  result.Address,
	}

	if err := database.DB.Create(&customer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create customer: " + err.Error()})
	}

	// Update lead result
	database.DB.Model(&result).Updates(map[string]interface{}{
		"status":                   "converted",
		"converted_to_customer_id": customer.ID,
	})

	return c.JSON(fiber.Map{
		"message":  "Lead converted to customer",
		"customer": customer,
	})
}

// ============================================================
// GET /api/v1/lead-finder/searches/:id/export/excel
// Export results to Excel
// ============================================================

func LeadFinderExportExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	searchID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Verify search belongs to tenant
	var search models.LeadFinderSearch
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, searchID).First(&search).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Search not found"})
	}

	var results []models.LeadFinderResult
	if err := database.DB.Where("tenant_id = ? AND search_id = ?", tenantID, searchID).Order("relevance DESC").Find(&results).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Build Excel file
	f := excelize.NewFile()
	defer f.Close()
	sheetName := "Lead Results"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	// Header style
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "#B4C6E7", Style: 1},
			{Type: "right", Color: "#B4C6E7", Style: 1},
			{Type: "top", Color: "#B4C6E7", Style: 1},
			{Type: "bottom", Color: "#B4C6E7", Style: 1},
		},
	})

	// Even row style
	evenRowStyle, _ := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#D9E2F3"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "#D9E2F3", Style: 1},
			{Type: "right", Color: "#D9E2F3", Style: 1},
			{Type: "bottom", Color: "#D9E2F3", Style: 1},
		},
	})

	oddRowStyle, _ := f.NewStyle(&excelize.Style{
		Border: []excelize.Border{
			{Type: "left", Color: "#D9E2F3", Style: 1},
			{Type: "right", Color: "#D9E2F3", Style: 1},
			{Type: "bottom", Color: "#D9E2F3", Style: 1},
		},
	})

	headers := []string{"#", "公司名稱", "電話", "電郵", "網站", "描述", "相關度", "狀態", "備註", "建立時間"}
	colWidths := []float64{5, 25, 18, 28, 35, 40, 10, 12, 25, 20}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, h)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
		colName, _ := excelize.ColumnNumberToName(i + 1)
		f.SetColWidth(sheetName, colName, colName, colWidths[i])
	}
	f.SetRowHeight(sheetName, 1, 24)

	// Data rows
	for rowIdx, result := range results {
		row := rowIdx + 2
		style := oddRowStyle
		if rowIdx%2 == 0 {
			style = evenRowStyle
		}

		statusMap := map[string]string{
			"new": "新發現", "contacted": "已聯繫", "converted": "已轉客戶", "dismissed": "已排除",
		}
		statusText := statusMap[result.Status]
		if statusText == "" {
			statusText = result.Status
		}

		values := []interface{}{
			rowIdx + 1,
			result.CompanyName,
			result.Phone,
			result.Email,
			result.Website,
			result.Description,
			result.Relevance,
			statusText,
			result.Notes,
			result.CreatedAt.Format("2006-01-02 15:04"),
		}

		for colIdx, val := range values {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, row)
			f.SetCellValue(sheetName, cell, val)
			f.SetCellStyle(sheetName, cell, cell, style)
		}
	}

	// Auto-filter
	if len(results) > 0 {
		lastCol, _ := excelize.ColumnNumberToName(len(headers))
		lastRow := len(results) + 1
		f.AutoFilter(sheetName, fmt.Sprintf("A1:%s%d", lastCol, lastRow), nil)
	}

	// Freeze header
	f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	// Document properties
	f.SetDocProps(&excelize.DocProperties{
		Creator: "vWork - Lead Finder",
		Title:   fmt.Sprintf("Lead Search Results - %s", search.Keywords),
	})

	filename := fmt.Sprintf("leads_%s.xlsx", time.Now().Format("20060102150405"))
	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate Excel file"})
	}
	return nil
}

// ============================================================
// GET /api/v1/lead-finder/searches/:id/export/pdf
// Export results to PDF
// ============================================================

func LeadFinderExportPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	searchID := c.Params("id")

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Verify search belongs to tenant
	var search models.LeadFinderSearch
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, searchID).First(&search).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Search not found"})
	}

	var results []models.LeadFinderResult
	if err := database.DB.Where("tenant_id = ? AND search_id = ?", tenantID, searchID).Order("relevance DESC").Find(&results).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Build table data for PDF
	headers := []string{"#", "公司名稱", "電話", "電郵", "網站", "相關度", "狀態"}
	rows := make([][]string, 0, len(results))

	for i, result := range results {
		statusMap := map[string]string{
			"new": "新發現", "contacted": "已聯繫", "converted": "已轉客戶", "dismissed": "已排除",
		}
		statusText := statusMap[result.Status]
		if statusText == "" {
			statusText = result.Status
		}

		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			result.CompanyName,
			result.Phone,
			result.Email,
			result.Website,
			fmt.Sprintf("%d", result.Relevance),
			statusText,
		})
	}

	title := fmt.Sprintf("Lead 搜尋結果 - %s", search.Keywords)
	pdfBytes, err := utils.BuildTablePDFBytes(title, headers, rows)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate PDF"})
	}

	filename := fmt.Sprintf("leads_%s.pdf", time.Now().Format("20060102150405"))
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	return c.Send(pdfBytes)
}

// ============================================================
// Helper: extract domain from URL for dedup
// ============================================================

func extractDomainFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	// Ensure scheme
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	// Strip www. prefix
	host = strings.TrimPrefix(host, "www.")
	return host
}

// ============================================================
// Helper: normalize phone for dedup (digits only, strip formatting)
// ============================================================

var phoneStripRe = regexp.MustCompile(`[^0-9+]`)

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	// Keep only digits and leading +
	normalized := phoneStripRe.ReplaceAllString(phone, "")
	// If it starts with 00 (international), convert to +
	if strings.HasPrefix(normalized, "00") && len(normalized) > 4 {
		normalized = "+" + normalized[2:]
	}
	return normalized
}

// ============================================================
// Helper: check if a lead result is a duplicate
// Returns: (isDuplicate bool, duplicateType string, duplicateInfo string)
// duplicateType: "lead" or "customer"
// duplicateInfo: human-readable description of what matched
// ============================================================

func checkLeadDuplicate(tenantID uuid.UUID, domain, normalizedPhone, email string) (bool, string, string) {
	db := database.DB
	emailLower := strings.ToLower(strings.TrimSpace(email))

	// 1. Check against existing lead_finder_results
	if domain != "" {
		var count int64
		db.Model(&models.LeadFinderResult{}).Where("tenant_id = ? AND website_domain = ? AND website_domain != ''", tenantID, domain).Count(&count)
		if count > 0 {
			return true, "lead", "website_domain:" + domain
		}
	}
	if normalizedPhone != "" {
		var count int64
		db.Model(&models.LeadFinderResult{}).Where("tenant_id = ? AND normalized_phone = ? AND normalized_phone != ''", tenantID, normalizedPhone).Count(&count)
		if count > 0 {
			return true, "lead", "phone:" + normalizedPhone
		}
	}
	if emailLower != "" {
		var count int64
		db.Model(&models.LeadFinderResult{}).Where("tenant_id = ? AND lower(email) = ? AND email != ''", tenantID, emailLower).Count(&count)
		if count > 0 {
			return true, "lead", "email:" + emailLower
		}
	}

	// 2. Check against existing customers
	if normalizedPhone != "" {
		var count int64
		// Customer.Phone may have formatting, so we strip it in SQL
		db.Model(&models.Customer{}).Where("tenant_id = ? AND regexp_replace(phone, '[^0-9+]', '', 'g') = ? AND phone != ''", tenantID, normalizedPhone).Count(&count)
		if count > 0 {
			return true, "customer", "phone:" + normalizedPhone
		}
	}
	if emailLower != "" {
		var count int64
		db.Model(&models.Customer{}).Where("tenant_id = ? AND lower(email) = ? AND email != ''", tenantID, emailLower).Count(&count)
		if count > 0 {
			return true, "customer", "email:" + emailLower
		}
	}

	return false, "", ""
}

// ============================================================
// Helper: extract company name from search title
// ============================================================

func extractCompanyName(title string) string {
	// Clean common separators from search titles
	separators := []string{" - ", " | ", " — ", " · ", " :: "}
	for _, sep := range separators {
		parts := strings.SplitN(title, sep, 2)
		if len(parts) > 0 && len(parts[0]) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	return strings.TrimSpace(title)
}

// ============================================================
// Helper: extract phone numbers from text
// ============================================================

func extractPhoneFromText(text string) string {
	// Simple patterns for phone numbers (HK, international)
	// Look for sequences of digits with optional separators
	phones := []string{}

	// Common patterns: +852 XXXX XXXX, (852) XXXX-XXXX, XXX-XXXX-XXXX, XXXX XXXX
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
				phone := strings.TrimSpace(string(runes[start:i]))
				phones = append(phones, phone)
			}
		} else {
			i++
		}
	}

	if len(phones) > 0 {
		return phones[0] // Return first found phone
	}
	return ""
}

// ============================================================
// Helper: extract email from text
// ============================================================

func extractEmailFromText(text string) string {
	// Simple email extraction
	words := strings.Fields(text)
	for _, word := range words {
		word = strings.Trim(word, ".,;:!?<>()[]{}\"'")
		if strings.Contains(word, "@") && strings.Contains(word, ".") {
			// Basic validation
			parts := strings.Split(word, "@")
			if len(parts) == 2 && len(parts[0]) > 0 && len(parts[1]) > 2 && strings.Contains(parts[1], ".") {
				return word
			}
		}
	}
	return ""
}
