package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/phpdave11/gofpdf"
	"github.com/xuri/excelize/v2"
)

// --- Request / Response types ---

type docGenerateRequest struct {
	Prompt  string `json:"prompt"`
	DocType string `json:"doc_type"` // docx, xlsx, pptx
	Title   string `json:"title,omitempty"`
	Model   string `json:"model,omitempty"`
}

// --- PPTX Theme System (referencing vOffice/OnlyOffice ColorSchemes) ---

// pptxThemePreset defines colors, fonts, and background for a PPTX theme.
// Color order follows OOXML: dk1, lt1, dk2, lt2, accent1-6, hlink, folHlink.
type pptxThemePreset struct {
	Name      string // Display name
	Dk1       string // Dark 1 (primary text)
	Lt1       string // Light 1 (primary background)
	Dk2       string // Dark 2 (secondary text)
	Lt2       string // Light 2 (secondary background)
	Accent1   string
	Accent2   string
	Accent3   string
	Accent4   string
	Accent5   string
	Accent6   string
	Hlink     string // Hyperlink
	FolHlink  string // Followed hyperlink
	TitleFont string // Major (title) Latin font
	BodyFont  string // Minor (body) Latin font
	EaFont    string // East-Asian font
	SlideBg   string // Slide background color
}

// pptxThemePresets maps theme keys to preset definitions.
// Derived from vOffice/OnlyOffice built-in color schemes (ColorSchemes.js).
var pptxThemePresets = map[string]pptxThemePreset{
	// Default / fallback — clean modern look
	"default": {
		Name: "Office 2013-2022", Dk1: "000000", Lt1: "FFFFFF", Dk2: "44546A", Lt2: "E7E6E6",
		Accent1: "4472C4", Accent2: "ED7D31", Accent3: "A5A5A5", Accent4: "FFC000",
		Accent5: "5B9BD5", Accent6: "70AD47", Hlink: "0563C1", FolHlink: "954F72",
		TitleFont: "Calibri Light", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Professional / corporate — Office 2007-2010 classic
	"professional": {
		Name: "Office 2007-2010", Dk1: "000000", Lt1: "FFFFFF", Dk2: "1F497D", Lt2: "EEECE1",
		Accent1: "4F81BD", Accent2: "C0504D", Accent3: "9BBB59", Accent4: "8064A2",
		Accent5: "4BACC6", Accent6: "F79646", Hlink: "0000FF", FolHlink: "800080",
		TitleFont: "Cambria", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Blue — cool tech/science
	"blue": {
		Name: "Blue", Dk1: "000000", Lt1: "FFFFFF", Dk2: "17406D", Lt2: "DBEFF9",
		Accent1: "0F6FC6", Accent2: "009DD9", Accent3: "0BD0D9", Accent4: "10CF9B",
		Accent5: "7CCA62", Accent6: "A5C249", Hlink: "F49100", FolHlink: "85DFD0",
		TitleFont: "Calibri", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Green — nature/environment/sustainability
	"green": {
		Name: "Green", Dk1: "000000", Lt1: "FFFFFF", Dk2: "455F51", Lt2: "E3DED1",
		Accent1: "549E39", Accent2: "8AB833", Accent3: "C0CF3A", Accent4: "029676",
		Accent5: "4AB5C4", Accent6: "0989B1", Hlink: "6B9F25", FolHlink: "BA6906",
		TitleFont: "Calibri", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Red — energy/passion/marketing
	"red": {
		Name: "Red", Dk1: "000000", Lt1: "FFFFFF", Dk2: "323232", Lt2: "E5C243",
		Accent1: "A5300F", Accent2: "D55816", Accent3: "E19825", Accent4: "B19C7D",
		Accent5: "7F5F52", Accent6: "B27D49", Hlink: "6B9F25", FolHlink: "B26B02",
		TitleFont: "Calibri", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Violet — creative/design/art
	"violet": {
		Name: "Violet", Dk1: "000000", Lt1: "FFFFFF", Dk2: "373545", Lt2: "DCD8DC",
		Accent1: "AD84C6", Accent2: "8784C7", Accent3: "5D739A", Accent4: "6997AF",
		Accent5: "84ACB6", Accent6: "6F8183", Hlink: "69A020", FolHlink: "8C8C8C",
		TitleFont: "Calibri", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Orange — warm/friendly/food/lifestyle
	"orange": {
		Name: "Orange", Dk1: "000000", Lt1: "FFFFFF", Dk2: "637052", Lt2: "CCDDEA",
		Accent1: "E48312", Accent2: "BD582C", Accent3: "865640", Accent4: "9B8357",
		Accent5: "C2BC80", Accent6: "94A088", Hlink: "2998E3", FolHlink: "8C8C8C",
		TitleFont: "Calibri", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Grayscale — minimal/monochrome/formal
	"grayscale": {
		Name: "Grayscale", Dk1: "000000", Lt1: "FFFFFF", Dk2: "000000", Lt2: "F8F8F8",
		Accent1: "DDDDDD", Accent2: "B2B2B2", Accent3: "969696", Accent4: "808080",
		Accent5: "5F5F5F", Accent6: "4D4D4D", Hlink: "5F5F5F", FolHlink: "919191",
		TitleFont: "Calibri", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Slipstream — vibrant/modern/startup
	"slipstream": {
		Name: "Slipstream", Dk1: "000000", Lt1: "FFFFFF", Dk2: "212745", Lt2: "B4DCFA",
		Accent1: "4E67C8", Accent2: "5ECCF3", Accent3: "A7EA52", Accent4: "5DCEAF",
		Accent5: "FF8021", Accent6: "F14124", Hlink: "56C7AA", FolHlink: "59A8D1",
		TitleFont: "Calibri", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "FFFFFF",
	},
	// Dark — dark background, light text
	"dark": {
		Name: "Dark", Dk1: "FFFFFF", Lt1: "1A1A2E", Dk2: "E0E0E0", Lt2: "2D2D44",
		Accent1: "4E67C8", Accent2: "5ECCF3", Accent3: "A7EA52", Accent4: "FFD166",
		Accent5: "FF8021", Accent6: "EF476F", Hlink: "56C7AA", FolHlink: "59A8D1",
		TitleFont: "Calibri", BodyFont: "Calibri", EaFont: "Microsoft JhengHei",
		SlideBg: "1A1A2E",
	},
}

// getPptxTheme returns the theme preset for the given key, falling back to "default".
func getPptxTheme(key string) pptxThemePreset {
	key = strings.ToLower(strings.TrimSpace(key))
	if t, ok := pptxThemePresets[key]; ok {
		return t
	}
	return pptxThemePresets["default"]
}

// --- CRUD Handlers ---

// GetAiDocuments lists all documents for the current user
// GET /api/v1/ai/documents
func GetAiDocuments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	query := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID)

	// Optional source filter: ?source=docs or ?source=chat
	if source := c.Query("source"); source != "" {
		query = query.Where("COALESCE(source, 'docs') = ?", source)
	}

	var docs []models.AiDocument
	if err := query.Order("created_at DESC").Find(&docs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	items := make([]fiber.Map, 0, len(docs))
	for _, d := range docs {
		items = append(items, fiber.Map{
			"id":            d.ID,
			"title":         d.Title,
			"prompt":        d.Prompt,
			"doc_type":      d.DocType,
			"file_url":      d.FileURL,
			"file_size":     d.FileSize,
			"model":         d.Model,
			"status":        d.Status,
			"source":        d.Source,
			"error_message": d.ErrorMessage,
			"created_at":    d.CreatedAt,
			"completed_at":  d.CompletedAt,
		})
	}

	return c.JSON(fiber.Map{"data": items})
}

// GetAiDocument returns a single document
// GET /api/v1/ai/documents/:id
func GetAiDocument(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var doc models.AiDocument
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&doc).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Document not found"})
	}

	return c.JSON(fiber.Map{
		"id":            doc.ID,
		"title":         doc.Title,
		"prompt":        doc.Prompt,
		"doc_type":      doc.DocType,
		"file_url":      doc.FileURL,
		"file_size":     doc.FileSize,
		"model":         doc.Model,
		"status":        doc.Status,
		"error_message": doc.ErrorMessage,
		"created_at":    doc.CreatedAt,
		"completed_at":  doc.CompletedAt,
	})
}

// DeleteAiDocument deletes a document record and its file
// DELETE /api/v1/ai/documents/:id
func DeleteAiDocument(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var doc models.AiDocument
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&doc).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Document not found"})
	}

	// Delete file from disk if exists
	if doc.FilePath != "" {
		if err := os.Remove(doc.FilePath); err != nil && !os.IsNotExist(err) {
			log.Printf("[WARN] Failed to delete document file %s: %v", doc.FilePath, err)
		}
	}

	if err := database.DB.Delete(&doc).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Mark related chat messages' doc_info as deleted so the chat UI shows
	// "相關記錄已刪除" instead of a download card.
	go markChatDocInfoDeleted(tenantID, id)

	return c.JSON(fiber.Map{"message": "Document deleted"})
}

// markChatDocInfoDeleted finds chat messages referencing the given document ID
// and sets extra_fields->'doc_info'->'deleted' = true. This runs in a goroutine
// so the delete response is not delayed.
func markChatDocInfoDeleted(tenantID uuid.UUID, docID string) {
	var messages []models.Message
	err := database.DB.
		Where("tenant_id = ? AND message_type = ? AND extra_fields->'doc_info'->>'id' = ?",
			tenantID, "ai_chat", docID).
		Find(&messages).Error
	if err != nil {
		log.Printf("[DOC-DELETE] Failed to find chat messages for doc %s: %v", docID, err)
		return
	}

	for _, msg := range messages {
		extra := map[string]interface{}(msg.ExtraFields)
		if extra == nil {
			continue
		}
		docInfo, ok := extra["doc_info"].(map[string]interface{})
		if !ok {
			continue
		}
		docInfo["deleted"] = true
		extra["doc_info"] = docInfo
		if err := database.DB.Model(&msg).Update("extra_fields", models.JSONB(extra)).Error; err != nil {
			log.Printf("[DOC-DELETE] Failed to mark chat message %s as deleted: %v", msg.ID.String(), err)
		} else {
			log.Printf("[DOC-DELETE] Marked chat message %s doc_info as deleted (doc=%s)", msg.ID.String(), docID)
		}
	}
}

// DownloadAiDocument serves the generated document file for download
// GET /api/v1/ai/documents/:id/download
func DownloadAiDocument(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var doc models.AiDocument
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&doc).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Document not found"})
	}

	if doc.Status != "completed" || doc.FilePath == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Document is not ready for download"})
	}

	// Check file exists
	if _, err := os.Stat(doc.FilePath); os.IsNotExist(err) {
		return c.Status(404).JSON(fiber.Map{"error": "Document file not found on server"})
	}

	// Set content type based on doc type
	contentType := "application/octet-stream"
	switch doc.DocType {
	case "docx":
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xlsx":
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "pptx":
		contentType = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case "pdf":
		contentType = "application/pdf"
	}

	filename := fmt.Sprintf("%s.%s", doc.Title, doc.DocType)
	c.Set("Content-Type", contentType)
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	return c.SendFile(doc.FilePath)
}

// --- AI Document Generation ---

// GenerateAiDocument generates a document using Gemini AI
// POST /api/v1/ai/doc-generate
func GenerateAiDocument(c *fiber.Ctx) error {
	cfg := config.Load()
	if cfg.LLM.Provider != "gemini" {
		return c.Status(501).JSON(fiber.Map{"error": "此功能暫不支援目前的 AI 供應商"})
	}
	if cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "AI API Key 未配置"})
	}

	var req docGenerateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Prompt 為必填"})
	}

	docType := strings.ToLower(strings.TrimSpace(req.DocType))
	if docType == "" {
		docType = "docx"
	}
	if docType != "docx" && docType != "xlsx" && docType != "pptx" && docType != "pdf" {
		return c.Status(400).JSON(fiber.Map{"error": "Unsupported doc_type. Must be docx, xlsx, pptx, or pdf"})
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		// Use first 50 chars of prompt as title
		title = prompt
		if len(title) > 50 {
			title = title[:50]
		}
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = cfg.LLM.Model
	}
	if model == "" {
		model = "gemini-2.0-flash"
	}

	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	// Create document record in DB with status "generating"
	doc := models.AiDocument{
		TenantID: tenantID,
		UserID:   userID,
		Title:    title,
		Prompt:   prompt,
		DocType:  docType,
		Model:    model,
		Status:   "generating",
	}
	if err := database.DB.Create(&doc).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create document record"})
	}

	// Call Gemini to generate structured content
	content, err := callGeminiForDocument(cfg, prompt, docType, model)
	if err != nil {
		doc.Status = "failed"
		doc.ErrorMessage = err.Error()
		database.DB.Save(&doc)
		return c.Status(500).JSON(fiber.Map{
			"error":       "AI generation failed: " + err.Error(),
			"document_id": doc.ID,
		})
	}

	// Generate the actual document file (standalone API uses default theme)
	filePath, fileSize, err := generateDocumentFile(tenantID, doc.ID, title, docType, "", content)
	if err != nil {
		doc.Status = "failed"
		doc.ErrorMessage = err.Error()
		database.DB.Save(&doc)
		return c.Status(500).JSON(fiber.Map{
			"error":       "File generation failed: " + err.Error(),
			"document_id": doc.ID,
		})
	}

	// Update document record
	now := time.Now()
	doc.Status = "completed"
	doc.FilePath = filePath
	doc.FileURL = fmt.Sprintf("/api/v1/ai/documents/%s/download", doc.ID.String())
	doc.FileSize = fileSize
	doc.CompletedAt = &now
	if err := database.DB.Save(&doc).Error; err != nil {
		log.Printf("[WARN] Failed to update document record: %v", err)
	}

	return c.JSON(fiber.Map{
		"id":           doc.ID,
		"title":        doc.Title,
		"doc_type":     doc.DocType,
		"file_url":     doc.FileURL,
		"file_size":    doc.FileSize,
		"status":       doc.Status,
		"model":        doc.Model,
		"created_at":   doc.CreatedAt,
		"completed_at": doc.CompletedAt,
	})
}

// --- Gemini API call ---

type geminiDocResponse struct {
	Title    string             `json:"title"`
	Sections []geminiDocSection `json:"sections,omitempty"`
	Rows     [][]string         `json:"rows,omitempty"`
	Headers  []string           `json:"headers,omitempty"`
	Slides   []geminiDocSlide   `json:"slides,omitempty"`
}

type geminiDocSection struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

type geminiDocSlide struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// callGeminiForDocument generates structured document content using multi-batch Gemini API calls.
// For PPTX: 4 batches x 3 slides = up to 12 slides (following vOffice 3-slide-per-batch pattern).
// For DOCX/PDF: 3 batches x 3 sections = up to 9 sections.
// For XLSX: single call (tabular data is best generated as one coherent dataset).
func callGeminiForDocument(cfg *config.Config, prompt, docType, model string) (*geminiDocResponse, error) {
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)
	client := &http.Client{Timeout: 60 * time.Second}

	// XLSX uses single-call (tabular data benefits from being generated as one coherent set)
	if docType == "xlsx" {
		return callGeminiSingle(client, apiURL, prompt, docType)
	}

	// PPTX / DOCX / PDF use multi-batch calls
	return callGeminiMultiBatch(client, apiURL, prompt, docType)
}

// callGeminiSingle makes a single Gemini API call for XLSX generation.
func callGeminiSingle(client *http.Client, apiURL, prompt, docType string) (*geminiDocResponse, error) {
	systemPrompt := `You are an expert data analyst who creates professional spreadsheets with realistic data.
You MUST respond with ONLY valid JSON (no markdown code fences, no extra text). Use this exact format:
{"title": "Sheet Title", "headers": ["Column1", "Column2", "Column3"], "rows": [["data1", "data2", "data3"]]}

STRICT REQUIREMENTS:
- Generate 4-8 meaningful column headers (never fewer than 4, never more than 8)
- Generate 15-30 rows of realistic, diverse data (never fewer than 15, never more than 30)
- Data must be realistic and internally consistent (e.g., dates in order, numbers that make sense)
- Include a mix of text, numbers, dates, and percentages where appropriate
- If the topic involves financial data, include currency amounts with proper formatting
- Write in the same language as the user's prompt`

	result, err := callGeminiOnce(client, apiURL, systemPrompt, prompt)
	if err != nil {
		return nil, err
	}

	// Enforce limits
	if len(result.Headers) > 8 {
		result.Headers = result.Headers[:8]
		for i, row := range result.Rows {
			if len(row) > 8 {
				result.Rows[i] = row[:8]
			}
		}
	}
	if len(result.Rows) > 30 {
		result.Rows = result.Rows[:30]
	}

	return result, nil
}

// callGeminiMultiBatch implements the multi-batch pattern (inspired by vOffice's 3-per-batch approach).
// Batch 1: generates title + first 3 items + outline of remaining items.
// Batch 2-N: generates next 3 items each, continuing the outline.
// All batches are merged into a single geminiDocResponse.
func callGeminiMultiBatch(client *http.Client, apiURL, prompt, docType string) (*geminiDocResponse, error) {
	// Determine batch configuration
	var itemsPerBatch, totalBatches int
	var itemName string

	switch docType {
	case "pptx":
		itemsPerBatch = 3
		totalBatches = 4 // 4 x 3 = 12 slides
		itemName = "slides"
	case "docx", "pdf":
		itemsPerBatch = 3
		totalBatches = 3 // 3 x 3 = 9 sections
		itemName = "sections"
	default:
		return nil, fmt.Errorf("unsupported doc type for multi-batch: %s", docType)
	}

	merged := &geminiDocResponse{}
	var outlinePlan string // Outline from batch 1, fed to subsequent batches for coherence

	for batch := 1; batch <= totalBatches; batch++ {
		systemPrompt := buildBatchSystemPrompt(docType, itemName, batch, totalBatches, itemsPerBatch, outlinePlan)

		var userPrompt string
		if batch == 1 {
			userPrompt = prompt
		} else {
			// Subsequent batches get the original prompt + context of what was already generated
			userPrompt = fmt.Sprintf("Original request: %s\n\nOutline plan: %s\n\nYou already generated %s 1-%d. Now generate %s %d-%d.",
				prompt, outlinePlan, itemName, (batch-1)*itemsPerBatch,
				itemName, (batch-1)*itemsPerBatch+1, batch*itemsPerBatch)
		}

		result, err := callGeminiOnce(client, apiURL, systemPrompt, userPrompt)
		if err != nil {
			// If a later batch fails, return what we have so far (at least batch 1 succeeded)
			if batch > 1 && (len(merged.Slides) > 0 || len(merged.Sections) > 0) {
				log.Printf("[WARN] Gemini batch %d/%d failed, using %d %s from previous batches: %v",
					batch, totalBatches, len(merged.Slides)+len(merged.Sections), itemName, err)
				break
			}
			return nil, fmt.Errorf("batch %d failed: %w", batch, err)
		}

		// Extract title from batch 1
		if batch == 1 {
			merged.Title = result.Title
			// If batch 1 returned an outline field, use it for subsequent batches
			// We encode the generated items as the implicit outline
			var items []string
			switch docType {
			case "pptx":
				for _, s := range result.Slides {
					items = append(items, s.Title)
				}
			case "docx", "pdf":
				for _, s := range result.Sections {
					items = append(items, s.Heading)
				}
			}
			outlinePlan = strings.Join(items, " | ")
		}

		// Merge items
		switch docType {
		case "pptx":
			merged.Slides = append(merged.Slides, result.Slides...)
		case "docx", "pdf":
			merged.Sections = append(merged.Sections, result.Sections...)
		}

		log.Printf("[INFO] Gemini batch %d/%d complete: +%d %s (total: %d slides, %d sections)",
			batch, totalBatches,
			len(result.Slides)+len(result.Sections), itemName,
			len(merged.Slides), len(merged.Sections))
	}

	// Enforce final limits
	switch docType {
	case "pptx":
		if len(merged.Slides) > 12 {
			merged.Slides = merged.Slides[:12]
		}
	case "docx", "pdf":
		if len(merged.Sections) > 9 {
			merged.Sections = merged.Sections[:9]
		}
	}

	return merged, nil
}

// buildBatchSystemPrompt creates the system prompt for a specific batch number.
func buildBatchSystemPrompt(docType, itemName string, batch, totalBatches, itemsPerBatch int, outlinePlan string) string {
	startIdx := (batch-1)*itemsPerBatch + 1
	endIdx := batch * itemsPerBatch

	switch docType {
	case "pptx":
		if batch == 1 {
			return fmt.Sprintf(`You are an expert presentation designer creating a professional slide deck.
You MUST respond with ONLY valid JSON (no markdown, no extra text). Use this exact format:
{"title": "Presentation Title", "slides": [{"title": "Slide Title", "content": "Point one.\nPoint two.\nPoint three."}]}

This is BATCH %d of %d. Generate exactly %d slides (%s %d-%d).
These first slides should cover:
- Slide 1: Introduction/Overview of the topic (subtitle-style, 1-2 lines)
- Slide 2-3: Opening main content with detailed bullet points

CONTENT RULES (per slide):
- Each slide MUST have 4-6 bullet points separated by \n
- Each bullet point MUST be 1-2 complete sentences with real substance (15-25 words each)
- Use professional, concise language suitable for business presentations
- Write in the same language as the user's prompt`, batch, totalBatches, itemsPerBatch, itemName, startIdx, endIdx)
		}
		if batch == totalBatches {
			return fmt.Sprintf(`You are an expert presentation designer continuing a slide deck.
You MUST respond with ONLY valid JSON. Use this exact format:
{"title": "", "slides": [{"title": "Slide Title", "content": "Point one.\nPoint two.\nPoint three."}]}

This is BATCH %d of %d (FINAL). Generate exactly %d slides (%s %d-%d).
Previously generated outline: %s
These final slides should cover:
- Summary/Key takeaways
- Conclusion or Call to Action
- Thank You / Q&A slide

CONTENT RULES (per slide):
- Each slide MUST have 4-6 bullet points separated by \n
- Each bullet point MUST be 1-2 complete sentences with real substance (15-25 words each)
- Maintain consistency with the previous slides in tone and style
- Write in the same language as the user's prompt`, batch, totalBatches, itemsPerBatch, itemName, startIdx, endIdx, outlinePlan)
		}
		return fmt.Sprintf(`You are an expert presentation designer continuing a slide deck.
You MUST respond with ONLY valid JSON. Use this exact format:
{"title": "", "slides": [{"title": "Slide Title", "content": "Point one.\nPoint two.\nPoint three."}]}

This is BATCH %d of %d. Generate exactly %d slides (%s %d-%d).
Previously generated outline: %s
Continue with the next main content slides, diving deeper into the topic.

CONTENT RULES (per slide):
- Each slide MUST have 4-6 bullet points separated by \n
- Each bullet point MUST be 1-2 complete sentences with real substance (15-25 words each)
- Maintain consistency with previous slides in tone and style
- Write in the same language as the user's prompt`, batch, totalBatches, itemsPerBatch, itemName, startIdx, endIdx, outlinePlan)

	case "docx", "pdf":
		formatName := "document"
		if docType == "pdf" {
			formatName = "PDF document"
		}
		if batch == 1 {
			return fmt.Sprintf(`You are an expert writer creating a professional %s.
You MUST respond with ONLY valid JSON (no markdown, no extra text). Use this exact format:
{"title": "Document Title", "sections": [{"heading": "Section Heading", "body": "Paragraph text.\nAnother paragraph."}]}

This is BATCH %d of %d. Generate exactly %d sections (sections %d-%d).
These first sections should include an Introduction/Overview and initial main content.

CONTENT RULES (per section):
- Each section body MUST have 3-6 paragraphs separated by \n
- Each paragraph MUST be 3-5 sentences with substantive, detailed content
- Use professional, formal tone appropriate for business documents
- Write in the same language as the user's prompt
- Each section should be 150-250 words`, formatName, batch, totalBatches, itemsPerBatch, startIdx, endIdx)
		}
		if batch == totalBatches {
			return fmt.Sprintf(`You are an expert writer continuing a professional %s.
You MUST respond with ONLY valid JSON. Use this exact format:
{"title": "", "sections": [{"heading": "Section Heading", "body": "Paragraph text.\nAnother paragraph."}]}

This is BATCH %d of %d (FINAL). Generate exactly %d sections (sections %d-%d).
Previously generated outline: %s
These final sections should include concluding content and a Summary/Conclusion.

CONTENT RULES (per section):
- Each section body MUST have 3-6 paragraphs separated by \n
- Each paragraph MUST be 3-5 sentences with substantive, detailed content
- Maintain consistency with previous sections in tone and terminology
- Write in the same language as the user's prompt
- Each section should be 150-250 words`, formatName, batch, totalBatches, itemsPerBatch, startIdx, endIdx, outlinePlan)
		}
		return fmt.Sprintf(`You are an expert writer continuing a professional %s.
You MUST respond with ONLY valid JSON. Use this exact format:
{"title": "", "sections": [{"heading": "Section Heading", "body": "Paragraph text.\nAnother paragraph."}]}

This is BATCH %d of %d. Generate exactly %d sections (sections %d-%d).
Previously generated outline: %s
Continue with the next main content sections, expanding on the topic.

CONTENT RULES (per section):
- Each section body MUST have 3-6 paragraphs separated by \n
- Each paragraph MUST be 3-5 sentences with substantive, detailed content
- Maintain consistency with previous sections in tone and terminology
- Write in the same language as the user's prompt
- Each section should be 150-250 words`, formatName, batch, totalBatches, itemsPerBatch, startIdx, endIdx, outlinePlan)
	}

	return ""
}

// callGeminiOnce makes a single Gemini API call and returns the parsed response.
func callGeminiOnce(client *http.Client, apiURL, systemPrompt, userPrompt string) (*geminiDocResponse, error) {
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

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to build Gemini request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Gemini API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse Gemini response
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
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("Gemini returned empty response")
	}

	// Extract JSON text from response
	jsonText := geminiResp.Candidates[0].Content.Parts[0].Text

	// Clean up — sometimes Gemini wraps in ```json ... ```
	jsonText = strings.TrimSpace(jsonText)
	if strings.HasPrefix(jsonText, "```") {
		lines := strings.Split(jsonText, "\n")
		if len(lines) > 2 {
			jsonText = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var docContent geminiDocResponse
	if err := json.Unmarshal([]byte(jsonText), &docContent); err != nil {
		return nil, fmt.Errorf("failed to parse AI content as JSON: %w (raw: %s)", err, jsonText[:min(200, len(jsonText))])
	}

	return &docContent, nil
}

// --- Document file generators ---

func generateDocumentFile(tenantID uuid.UUID, docID uuid.UUID, title, docType, theme string, content *geminiDocResponse) (string, int64, error) {
	// Create upload directory
	uploadDir := filepath.Join("web", "uploads", tenantID.String(), "documents")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create upload directory: %w", err)
	}

	filename := fmt.Sprintf("%s.%s", docID.String(), docType)
	filePath := filepath.Join(uploadDir, filename)

	var err error
	switch docType {
	case "docx":
		err = generateDocx(filePath, title, content)
	case "xlsx":
		err = generateXlsx(filePath, title, content)
	case "pptx":
		err = generatePptx(filePath, title, theme, content)
	case "pdf":
		err = generatePdf(filePath, title, content)
	default:
		return "", 0, fmt.Errorf("unsupported doc type: %s", docType)
	}

	if err != nil {
		return "", 0, err
	}

	// Get file size
	info, err := os.Stat(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to stat generated file: %w", err)
	}

	return filePath, info.Size(), nil
}

// generateDocx creates a proper .docx file with theme, styles, settings, and fontTable.
// References vOffice OOXML patterns: styles use themeColor attributes, theme1.xml provides color scheme.
func generateDocx(filePath, title string, content *geminiDocResponse) error {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// [Content_Types].xml
	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
  <Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>
  <Override PartName="/word/fontTable.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.fontTable+xml"/>
  <Override PartName="/word/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.ms-office.activeX+xml"/>
</Types>`
	writeZipFile(w, "[Content_Types].xml", contentTypes)

	// _rels/.rels
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`
	writeZipFile(w, "_rels/.rels", rels)

	// word/_rels/document.xml.rels — reference styles, settings, fontTable, theme
	docRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/fontTable" Target="fontTable.xml"/>
  <Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>
</Relationships>`
	writeZipFile(w, "word/_rels/document.xml.rels", docRels)

	// docProps/core.xml
	now := time.Now().Format("2006-01-02T15:04:05Z")
	coreXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
                   xmlns:dc="http://purl.org/dc/elements/1.1/"
                   xmlns:dcterms="http://purl.org/dc/terms/"
                   xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:title>%s</dc:title>
  <dc:creator>vAI</dc:creator>
  <cp:lastModifiedBy>vAI</cp:lastModifiedBy>
  <cp:revision>1</cp:revision>
  <dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
  <dcterms:modified xsi:type="dcterms:W3CDTF">%s</dcterms:modified>
</cp:coreProperties>`, escapeXML(title), now, now)
	writeZipFile(w, "docProps/core.xml", coreXML)

	// docProps/app.xml
	appXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
  <Application>vAI Document Generator</Application>
  <AppVersion>1.0</AppVersion>
</Properties>`
	writeZipFile(w, "docProps/app.xml", appXML)

	// word/settings.xml
	settingsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
            xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math">
  <w:zoom w:percent="100"/>
  <w:defaultTabStop w:val="720"/>
  <w:characterSpacingControl w:val="doNotCompress"/>
  <w:compat>
    <w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/>
  </w:compat>
  <w:themeFontLang w:val="en-US" w:eastAsia="zh-TW"/>
</w:settings>`
	writeZipFile(w, "word/settings.xml", settingsXML)

	// word/fontTable.xml
	fontTableXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:fonts xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:font w:name="Calibri">
    <w:panose1 w:val="020F0502020204030204"/>
    <w:charset w:val="00"/>
    <w:family w:val="swiss"/>
    <w:pitch w:val="variable"/>
  </w:font>
  <w:font w:name="Calibri Light">
    <w:panose1 w:val="020F0302020204030204"/>
    <w:charset w:val="00"/>
    <w:family w:val="swiss"/>
    <w:pitch w:val="variable"/>
  </w:font>
  <w:font w:name="Microsoft JhengHei">
    <w:charset w:val="88"/>
    <w:family w:val="swiss"/>
    <w:pitch w:val="variable"/>
  </w:font>
</w:fonts>`
	writeZipFile(w, "word/fontTable.xml", fontTableXML)

	// word/theme/theme1.xml — Office default theme (matching vOffice default color scheme)
	themeXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Office Theme">
  <a:themeElements>
    <a:clrScheme name="Office">
      <a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1>
      <a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>
      <a:dk2><a:srgbClr val="44546A"/></a:dk2>
      <a:lt2><a:srgbClr val="E7E6E6"/></a:lt2>
      <a:accent1><a:srgbClr val="4472C4"/></a:accent1>
      <a:accent2><a:srgbClr val="ED7D31"/></a:accent2>
      <a:accent3><a:srgbClr val="A5A5A5"/></a:accent3>
      <a:accent4><a:srgbClr val="FFC000"/></a:accent4>
      <a:accent5><a:srgbClr val="5B9BD5"/></a:accent5>
      <a:accent6><a:srgbClr val="70AD47"/></a:accent6>
      <a:hlink><a:srgbClr val="0563C1"/></a:hlink>
      <a:folHlink><a:srgbClr val="954F72"/></a:folHlink>
    </a:clrScheme>
    <a:fontScheme name="Office">
      <a:majorFont>
        <a:latin typeface="Calibri Light"/>
        <a:ea typeface="Microsoft JhengHei"/>
        <a:cs typeface=""/>
      </a:majorFont>
      <a:minorFont>
        <a:latin typeface="Calibri"/>
        <a:ea typeface="Microsoft JhengHei"/>
        <a:cs typeface=""/>
      </a:minorFont>
    </a:fontScheme>
    <a:fmtScheme name="Office">
      <a:fillStyleLst>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
      </a:fillStyleLst>
      <a:lnStyleLst>
        <a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>
        <a:ln w="25400"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>
        <a:ln w="38100"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>
      </a:lnStyleLst>
      <a:effectStyleLst>
        <a:effectStyle><a:effectLst/></a:effectStyle>
        <a:effectStyle><a:effectLst/></a:effectStyle>
        <a:effectStyle><a:effectLst/></a:effectStyle>
      </a:effectStyleLst>
      <a:bgFillStyleLst>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
      </a:bgFillStyleLst>
    </a:fmtScheme>
  </a:themeElements>
</a:theme>`
	writeZipFile(w, "word/theme/theme1.xml", themeXML)

	// word/styles.xml — styles with themeColor references (following vOffice default-styles.js patterns)
	stylesXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
          xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"
          mc:Ignorable="w14">
  <w:docDefaults>
    <w:rPrDefault>
      <w:rPr>
        <w:rFonts w:asciiTheme="minorHAnsi" w:hAnsiTheme="minorHAnsi" w:eastAsiaTheme="minorEastAsia" w:cstheme="minorBidi"/>
        <w:sz w:val="22"/>
        <w:szCs w:val="22"/>
        <w:lang w:val="en-US" w:eastAsia="zh-TW"/>
      </w:rPr>
    </w:rPrDefault>
    <w:pPrDefault>
      <w:pPr>
        <w:spacing w:after="160" w:line="259" w:lineRule="auto"/>
      </w:pPr>
    </w:pPrDefault>
  </w:docDefaults>
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal">
    <w:name w:val="Normal"/>
    <w:qFormat/>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Title">
    <w:name w:val="Title"/>
    <w:basedOn w:val="Normal"/>
    <w:qFormat/>
    <w:pPr>
      <w:spacing w:after="300" w:line="240" w:lineRule="auto"/>
      <w:jc w:val="center"/>
    </w:pPr>
    <w:rPr>
      <w:rFonts w:asciiTheme="majorHAnsi" w:hAnsiTheme="majorHAnsi" w:eastAsiaTheme="majorEastAsia"/>
      <w:b/>
      <w:sz w:val="56"/>
      <w:szCs w:val="56"/>
      <w:color w:val="262626" w:themeColor="text1" w:themeTint="D9"/>
    </w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="heading 1"/>
    <w:basedOn w:val="Normal"/>
    <w:qFormat/>
    <w:pPr>
      <w:keepNext/>
      <w:keepLines/>
      <w:spacing w:before="360" w:after="120" w:line="240" w:lineRule="auto"/>
      <w:outlineLvl w:val="0"/>
    </w:pPr>
    <w:rPr>
      <w:rFonts w:asciiTheme="majorHAnsi" w:hAnsiTheme="majorHAnsi" w:eastAsiaTheme="majorEastAsia"/>
      <w:b/>
      <w:sz w:val="32"/>
      <w:szCs w:val="32"/>
      <w:color w:val="2F5496" w:themeColor="accent1" w:themeShade="BF"/>
    </w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading2">
    <w:name w:val="heading 2"/>
    <w:basedOn w:val="Normal"/>
    <w:qFormat/>
    <w:pPr>
      <w:keepNext/>
      <w:keepLines/>
      <w:spacing w:before="240" w:after="80"/>
      <w:outlineLvl w:val="1"/>
    </w:pPr>
    <w:rPr>
      <w:rFonts w:asciiTheme="majorHAnsi" w:hAnsiTheme="majorHAnsi" w:eastAsiaTheme="majorEastAsia"/>
      <w:b/>
      <w:sz w:val="26"/>
      <w:szCs w:val="26"/>
      <w:color w:val="2F5496" w:themeColor="accent1" w:themeShade="BF"/>
    </w:rPr>
  </w:style>
  <w:style w:type="character" w:default="1" w:styleId="DefaultParagraphFont">
    <w:name w:val="Default Paragraph Font"/>
  </w:style>
</w:styles>`
	writeZipFile(w, "word/styles.xml", stylesXML)

	// word/document.xml — the main content
	var bodyXML strings.Builder

	// Title paragraph
	bodyXML.WriteString(fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p>`, escapeXML(title)))

	// Sections
	if content != nil {
		for _, sec := range content.Sections {
			// Heading
			bodyXML.WriteString(fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>%s</w:t></w:r></w:p>`, escapeXML(sec.Heading)))

			// Body — split by newlines into paragraphs
			paragraphs := strings.Split(sec.Body, "\n")
			for _, p := range paragraphs {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				bodyXML.WriteString(fmt.Sprintf(`<w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, escapeXML(p)))
			}
		}
	}

	// Section properties (A4 page size, margins)
	bodyXML.WriteString(`<w:sectPr>
      <w:pgSz w:w="11906" w:h="16838"/>
      <w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="708" w:footer="708" w:gutter="0"/>
      <w:cols w:space="708"/>
    </w:sectPr>`)

	documentXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
            xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
            xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">
  <w:body>
    %s
  </w:body>
</w:document>`, bodyXML.String())

	writeZipFile(w, "word/document.xml", documentXML)

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to finalize docx: %w", err)
	}

	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

// generateXlsx creates a well-styled xlsx file using excelize.
// Uses theme-aware styling and professional formatting (alternating row colors, borders).
func generateXlsx(filePath, title string, content *geminiDocResponse) error {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "Sheet1"
	if content != nil && content.Title != "" {
		// Sanitize sheet name (max 31 chars, no special chars)
		sn := content.Title
		if len(sn) > 31 {
			sn = sn[:31]
		}
		// Remove invalid characters for sheet names
		for _, ch := range []string{"/", "\\", "?", "*", "[", "]", ":"} {
			sn = strings.ReplaceAll(sn, ch, "")
		}
		if sn != "" {
			f.SetSheetName("Sheet1", sn)
			sheetName = sn
		}
	}

	if content != nil {
		// --- Header style: Bold white text on accent1 blue background with thin borders ---
		headerStyle, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true, Size: 11, Color: "#FFFFFF"},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"#4472C4"}, Pattern: 1},
			Alignment: &excelize.Alignment{
				Horizontal: "center",
				Vertical:   "center",
			},
			Border: []excelize.Border{
				{Type: "left", Color: "#B4C6E7", Style: 1},
				{Type: "top", Color: "#B4C6E7", Style: 1},
				{Type: "bottom", Color: "#B4C6E7", Style: 1},
				{Type: "right", Color: "#B4C6E7", Style: 1},
			},
		})

		// --- Even row style: light blue background with borders ---
		evenRowStyle, _ := f.NewStyle(&excelize.Style{
			Fill: excelize.Fill{Type: "pattern", Color: []string{"#D9E2F3"}, Pattern: 1},
			Border: []excelize.Border{
				{Type: "left", Color: "#B4C6E7", Style: 1},
				{Type: "top", Color: "#B4C6E7", Style: 1},
				{Type: "bottom", Color: "#B4C6E7", Style: 1},
				{Type: "right", Color: "#B4C6E7", Style: 1},
			},
		})

		// --- Odd row style: white background with borders ---
		oddRowStyle, _ := f.NewStyle(&excelize.Style{
			Border: []excelize.Border{
				{Type: "left", Color: "#B4C6E7", Style: 1},
				{Type: "top", Color: "#B4C6E7", Style: 1},
				{Type: "bottom", Color: "#B4C6E7", Style: 1},
				{Type: "right", Color: "#B4C6E7", Style: 1},
			},
		})

		// Write headers
		for i, h := range content.Headers {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(sheetName, cell, h)
			f.SetCellStyle(sheetName, cell, cell, headerStyle)
			// Auto-width estimation
			colName, _ := excelize.ColumnNumberToName(i + 1)
			width := float64(len(h)*2 + 4)
			if width < 12 {
				width = 12
			}
			if width > 40 {
				width = 40
			}
			f.SetColWidth(sheetName, colName, colName, width)
		}

		// Set header row height
		f.SetRowHeight(sheetName, 1, 24)

		// Write data rows with alternating colors
		for rowIdx, row := range content.Rows {
			rowNum := rowIdx + 2
			style := oddRowStyle
			if rowIdx%2 == 0 {
				style = evenRowStyle
			}
			for colIdx, val := range row {
				cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowNum)
				f.SetCellValue(sheetName, cell, val)
				f.SetCellStyle(sheetName, cell, cell, style)
			}
		}

		// Auto-filter on the header row
		if len(content.Headers) > 0 {
			lastCol, _ := excelize.ColumnNumberToName(len(content.Headers))
			lastRow := len(content.Rows) + 1
			filterRange := fmt.Sprintf("%s!A1:%s%d", sheetName, lastCol, lastRow)
			f.AutoFilter(sheetName, filterRange, nil)
		}

		// Freeze the header row
		f.SetPanes(sheetName, &excelize.Panes{
			Freeze:      true,
			Split:       false,
			XSplit:      0,
			YSplit:      1,
			TopLeftCell: "A2",
			ActivePane:  "bottomLeft",
		})
	}

	// Set document properties
	f.SetDocProps(&excelize.DocProperties{
		Creator:     "vAI",
		Title:       title,
		Description: "Generated by vAI Document Generator",
	})

	return f.SaveAs(filePath)
}

// generatePdf creates a well-formatted PDF with CJK font support, header/footer, and proper layout.
func generatePdf(filePath, title string, content *geminiDocResponse) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(25, 25, 25)
	pdf.SetAutoPageBreak(true, 25)

	// Use a Unicode-capable font for CJK support
	fontPath := filepath.Join("web", "static", "fonts")
	cjkFont := false

	// Try to load NotoSansCJK or any CJK font if present
	fontFiles := []string{"NotoSansCJKtc-Regular.ttf", "NotoSansTC-Regular.ttf", "msjh.ttf", "msyh.ttf", "kaiu.ttf"}
	for _, ff := range fontFiles {
		fullPath := filepath.Join(fontPath, ff)
		if _, err := os.Stat(fullPath); err == nil {
			pdf.AddUTF8Font("CJK", "", fullPath)
			pdf.AddUTF8Font("CJK", "B", fullPath) // bold fallback to regular
			cjkFont = true
			break
		}
	}

	// --- Footer with page numbers ---
	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		if cjkFont {
			pdf.SetFont("CJK", "", 8)
		} else {
			pdf.SetFont("Arial", "", 8)
		}
		pdf.SetTextColor(150, 150, 150)
		pageStr := fmt.Sprintf("- %d -", pdf.PageNo())
		pdf.CellFormat(0, 10, pageStr, "", 0, "C", false, 0, "")
	})

	// --- Title Page ---
	pdf.AddPage()

	// Add some vertical spacing before title
	pdf.Ln(40)

	// Title
	if cjkFont {
		pdf.SetFont("CJK", "B", 28)
	} else {
		pdf.SetFont("Arial", "B", 28)
	}
	pdf.SetTextColor(44, 62, 80)
	if cjkFont {
		pdf.MultiCell(0, 14, title, "", "C", false)
	} else {
		pdf.CellFormat(0, 14, title, "", 1, "C", false, 0, "")
	}

	// Decorative line under title
	pdf.Ln(6)
	pdf.SetDrawColor(68, 114, 196) // accent1 color
	pdf.SetLineWidth(0.8)
	centerX := 105.0
	pdf.Line(centerX-40, pdf.GetY(), centerX+40, pdf.GetY())
	pdf.Ln(6)

	// Subtitle: "Generated by vAI"
	if cjkFont {
		pdf.SetFont("CJK", "", 11)
	} else {
		pdf.SetFont("Arial", "", 11)
	}
	pdf.SetTextColor(120, 120, 120)
	genDate := time.Now().Format("2006-01-02")
	if cjkFont {
		pdf.MultiCell(0, 6, fmt.Sprintf("Generated by vAI | %s", genDate), "", "C", false)
	} else {
		pdf.CellFormat(0, 6, fmt.Sprintf("Generated by vAI | %s", genDate), "", 1, "C", false, 0, "")
	}

	// --- Content Pages ---
	if content != nil {
		for _, sec := range content.Sections {
			// Section heading
			if cjkFont {
				pdf.SetFont("CJK", "B", 16)
			} else {
				pdf.SetFont("Arial", "B", 16)
			}
			pdf.SetTextColor(44, 62, 80)

			// Check if we need a new page (heading needs at least 30mm)
			if pdf.GetY() > 250 {
				pdf.AddPage()
			}

			pdf.Ln(6)
			if cjkFont {
				pdf.MultiCell(0, 9, sec.Heading, "", "L", false)
			} else {
				pdf.CellFormat(0, 9, sec.Heading, "", 1, "L", false, 0, "")
			}

			// Accent line under heading
			pdf.SetDrawColor(68, 114, 196)
			pdf.SetLineWidth(0.5)
			pdf.Line(25, pdf.GetY()+1, 185, pdf.GetY()+1)
			pdf.Ln(5)

			// Section body
			if cjkFont {
				pdf.SetFont("CJK", "", 11)
			} else {
				pdf.SetFont("Arial", "", 11)
			}
			pdf.SetTextColor(55, 65, 81)

			paragraphs := strings.Split(sec.Body, "\n")
			for _, p := range paragraphs {
				p = strings.TrimSpace(p)
				if p == "" {
					pdf.Ln(3)
					continue
				}
				pdf.MultiCell(0, 6, p, "", "L", false)
				pdf.Ln(2)
			}

			pdf.Ln(3)
		}
	}

	return pdf.OutputFileAndClose(filePath)
}

// generatePptx creates a proper .pptx file following OOXML spec.
// Uses proper slide master/layout inheritance with placeholders and schemeClr references.
// Two layouts: slideLayout1 (title slide with ctrTitle+subTitle), slideLayout2 (content with title+body).
// All boilerplate files included: presProps, viewProps, tableStyles, docProps.
func generatePptx(filePath, title, themeName string, content *geminiDocResponse) error {
	theme := getPptxTheme(themeName)

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Calculate slide count: 1 title slide + N content slides
	slideCount := 1
	if content != nil && len(content.Slides) > 0 {
		slideCount = len(content.Slides) + 1
	}

	// Relationship IDs for presentation.xml.rels:
	// rId1..rIdN = slides, then slideMaster, presProps, viewProps, theme, tableStyles
	smRId := fmt.Sprintf("rId%d", slideCount+1)
	presPropsRId := fmt.Sprintf("rId%d", slideCount+2)
	viewPropsRId := fmt.Sprintf("rId%d", slideCount+3)
	themeRId := fmt.Sprintf("rId%d", slideCount+4)
	tableStylesRId := fmt.Sprintf("rId%d", slideCount+5)

	// --- [Content_Types].xml ---
	var slideOverrides strings.Builder
	for i := 1; i <= slideCount; i++ {
		slideOverrides.WriteString(fmt.Sprintf(`  <Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
`, i))
	}

	contentTypes := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
%s  <Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>
  <Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>
  <Override PartName="/ppt/slideLayouts/slideLayout2.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>
  <Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
  <Override PartName="/ppt/presProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presProps+xml"/>
  <Override PartName="/ppt/viewProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.viewProps+xml"/>
  <Override PartName="/ppt/tableStyles.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.ms-office.activeX+xml"/>
</Types>`, slideOverrides.String())
	writeZipFile(w, "[Content_Types].xml", contentTypes)

	// --- _rels/.rels ---
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`
	writeZipFile(w, "_rels/.rels", rels)

	// --- docProps/core.xml ---
	now := time.Now().Format("2006-01-02T15:04:05Z")
	coreXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
                   xmlns:dc="http://purl.org/dc/elements/1.1/"
                   xmlns:dcterms="http://purl.org/dc/terms/"
                   xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:title>%s</dc:title>
  <dc:creator>vAI</dc:creator>
  <cp:lastModifiedBy>vAI</cp:lastModifiedBy>
  <cp:revision>1</cp:revision>
  <dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
  <dcterms:modified xsi:type="dcterms:W3CDTF">%s</dcterms:modified>
</cp:coreProperties>`, escapeXML(title), now, now)
	writeZipFile(w, "docProps/core.xml", coreXML)

	// --- docProps/app.xml ---
	appXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
  <Application>vAI Document Generator</Application>
  <PresentationFormat>On-screen Show (16:9)</PresentationFormat>
  <Slides>%d</Slides>
  <AppVersion>1.0</AppVersion>
</Properties>`, slideCount)
	writeZipFile(w, "docProps/app.xml", appXML)

	// --- ppt/_rels/presentation.xml.rels ---
	var slideRels strings.Builder
	for i := 1; i <= slideCount; i++ {
		slideRels.WriteString(fmt.Sprintf(`  <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>
`, i, i))
	}
	presRels := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
%s  <Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>
  <Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/presProps" Target="presProps.xml"/>
  <Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/viewProps" Target="viewProps.xml"/>
  <Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>
  <Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/tableStyles" Target="tableStyles.xml"/>
</Relationships>`, slideRels.String(), smRId, presPropsRId, viewPropsRId, themeRId, tableStylesRId)
	writeZipFile(w, "ppt/_rels/presentation.xml.rels", presRels)

	// --- ppt/presentation.xml ---
	var slideList strings.Builder
	for i := 1; i <= slideCount; i++ {
		slideList.WriteString(fmt.Sprintf(`    <p:sldId id="%d" r:id="rId%d"/>
`, 255+i, i))
	}
	presentationXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
                xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
                xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:sldMasterIdLst>
    <p:sldMasterId id="2147483648" r:id="%s"/>
  </p:sldMasterIdLst>
  <p:sldIdLst>
%s  </p:sldIdLst>
  <p:sldSz cx="12192000" cy="6858000"/>
  <p:notesSz cx="6858000" cy="9144000"/>
  <p:defaultTextStyle>
    <a:defPPr>
      <a:defRPr lang="zh-TW"/>
    </a:defPPr>
    <a:lvl1pPr marL="0" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" hangingPunct="1">
      <a:defRPr sz="1800" kern="1200">
        <a:solidFill><a:schemeClr val="tx1"/></a:solidFill>
        <a:latin typeface="+mn-lt"/>
        <a:ea typeface="+mn-ea"/>
        <a:cs typeface="+mn-cs"/>
      </a:defRPr>
    </a:lvl1pPr>
  </p:defaultTextStyle>
</p:presentation>`, smRId, slideList.String())
	writeZipFile(w, "ppt/presentation.xml", presentationXML)

	// --- ppt/presProps.xml ---
	writeZipFile(w, "ppt/presProps.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentationPr xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
                  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
                  xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`)

	// --- ppt/viewProps.xml ---
	writeZipFile(w, "ppt/viewProps.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:viewPr xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
          xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
          xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
          lastView="sldThumbnailView">
  <p:slideViewPr>
    <p:cSldViewPr>
      <p:cViewPr>
        <p:scale><a:sx n="100" d="100"/><a:sy n="100" d="100"/></p:scale>
        <p:origin x="0" y="0"/>
      </p:cViewPr>
      <p:guideLst/>
    </p:cSldViewPr>
  </p:slideViewPr>
</p:viewPr>`)

	// --- ppt/tableStyles.xml ---
	writeZipFile(w, "ppt/tableStyles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<a:tblStyleLst xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"/>`)

	// --- ppt/theme/theme1.xml — dynamic theme from preset ---
	// Only place where srgbClr is used (to define the color scheme values)
	themeXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="%s">
  <a:themeElements>
    <a:clrScheme name="%s">
      <a:dk1><a:srgbClr val="%s"/></a:dk1>
      <a:lt1><a:srgbClr val="%s"/></a:lt1>
      <a:dk2><a:srgbClr val="%s"/></a:dk2>
      <a:lt2><a:srgbClr val="%s"/></a:lt2>
      <a:accent1><a:srgbClr val="%s"/></a:accent1>
      <a:accent2><a:srgbClr val="%s"/></a:accent2>
      <a:accent3><a:srgbClr val="%s"/></a:accent3>
      <a:accent4><a:srgbClr val="%s"/></a:accent4>
      <a:accent5><a:srgbClr val="%s"/></a:accent5>
      <a:accent6><a:srgbClr val="%s"/></a:accent6>
      <a:hlink><a:srgbClr val="%s"/></a:hlink>
      <a:folHlink><a:srgbClr val="%s"/></a:folHlink>
    </a:clrScheme>
    <a:fontScheme name="%s">
      <a:majorFont>
        <a:latin typeface="%s"/>
        <a:ea typeface="%s"/>
        <a:cs typeface=""/>
      </a:majorFont>
      <a:minorFont>
        <a:latin typeface="%s"/>
        <a:ea typeface="%s"/>
        <a:cs typeface=""/>
      </a:minorFont>
    </a:fontScheme>
    <a:fmtScheme name="%s">
      <a:fillStyleLst>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
      </a:fillStyleLst>
      <a:lnStyleLst>
        <a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>
        <a:ln w="25400"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>
        <a:ln w="38100"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln>
      </a:lnStyleLst>
      <a:effectStyleLst>
        <a:effectStyle><a:effectLst/></a:effectStyle>
        <a:effectStyle><a:effectLst/></a:effectStyle>
        <a:effectStyle><a:effectLst/></a:effectStyle>
      </a:effectStyleLst>
      <a:bgFillStyleLst>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
        <a:solidFill><a:schemeClr val="phClr"/></a:solidFill>
      </a:bgFillStyleLst>
    </a:fmtScheme>
  </a:themeElements>
</a:theme>`,
		escapeXML(theme.Name), escapeXML(theme.Name),
		theme.Dk1, theme.Lt1, theme.Dk2, theme.Lt2,
		theme.Accent1, theme.Accent2, theme.Accent3, theme.Accent4,
		theme.Accent5, theme.Accent6, theme.Hlink, theme.FolHlink,
		escapeXML(theme.Name), theme.TitleFont, theme.EaFont,
		theme.BodyFont, theme.EaFont, escapeXML(theme.Name))
	writeZipFile(w, "ppt/theme/theme1.xml", themeXML)

	// --- ppt/slideMasters/slideMaster1.xml ---
	// Background via bgRef (inherits from theme fmtScheme), placeholders for title/body,
	// clrMap maps logical names to theme slots, txStyles define title/body/other text defaults.
	// Decorative shapes: bottom accent stripe visible on all slides inheriting master.
	slideMasterXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
             xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
             xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:bg>
      <p:bgRef idx="1001">
        <a:schemeClr val="bg1"/>
      </p:bgRef>
    </p:bg>
    <p:spTree>
      <p:nvGrpSpPr>
        <p:cNvPr id="1" name=""/>
        <p:cNvGrpSpPr/>
        <p:nvPr/>
      </p:nvGrpSpPr>
      <p:grpSpPr/>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="2" name="Title Placeholder 1"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="title"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="609600" y="274638"/>
            <a:ext cx="10972800" cy="1143000"/>
          </a:xfrm>
          <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
        </p:spPr>
        <p:txBody>
          <a:bodyPr vert="horz" lIns="91440" tIns="45720" rIns="91440" bIns="45720" rtlCol="0" anchor="ctr"/>
          <a:lstStyle/>
          <a:p><a:r><a:rPr lang="zh-TW"/><a:t>Title</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="3" name="Text Placeholder 2"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="body" idx="1"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="609600" y="1600200"/>
            <a:ext cx="10972800" cy="4525963"/>
          </a:xfrm>
          <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
        </p:spPr>
        <p:txBody>
          <a:bodyPr vert="horz" lIns="91440" tIns="45720" rIns="91440" bIns="45720" rtlCol="0" anchor="t"/>
          <a:lstStyle/>
          <a:p><a:r><a:rPr lang="zh-TW"/><a:t>Body</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
  <p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2"
            accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>
  <p:sldLayoutIdLst>
    <p:sldLayoutId id="2147483649" r:id="rId1"/>
    <p:sldLayoutId id="2147483650" r:id="rId2"/>
  </p:sldLayoutIdLst>
  <p:txStyles>
    <p:titleStyle>
      <a:lvl1pPr algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" hangingPunct="1">
        <a:spcBef><a:spcPct val="0"/></a:spcBef>
        <a:defRPr sz="3600" b="1" kern="1200">
          <a:solidFill><a:schemeClr val="accent1"/></a:solidFill>
          <a:latin typeface="+mj-lt"/>
          <a:ea typeface="+mj-ea"/>
          <a:cs typeface="+mj-cs"/>
        </a:defRPr>
      </a:lvl1pPr>
    </p:titleStyle>
    <p:bodyStyle>
      <a:lvl1pPr marL="342900" indent="-342900" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" hangingPunct="1">
        <a:spcBef><a:spcPct val="20000"/></a:spcBef>
        <a:buFont typeface="Arial"/>
        <a:buChar char="&#x2022;"/>
        <a:defRPr sz="2000" kern="1200">
          <a:solidFill><a:schemeClr val="tx1"/></a:solidFill>
          <a:latin typeface="+mn-lt"/>
          <a:ea typeface="+mn-ea"/>
          <a:cs typeface="+mn-cs"/>
        </a:defRPr>
      </a:lvl1pPr>
      <a:lvl2pPr marL="742950" indent="-285750" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" hangingPunct="1">
        <a:spcBef><a:spcPct val="20000"/></a:spcBef>
        <a:buFont typeface="Arial"/>
        <a:buChar char="&#x2013;"/>
        <a:defRPr sz="1800" kern="1200">
          <a:solidFill><a:schemeClr val="tx1"/></a:solidFill>
          <a:latin typeface="+mn-lt"/>
          <a:ea typeface="+mn-ea"/>
          <a:cs typeface="+mn-cs"/>
        </a:defRPr>
      </a:lvl2pPr>
    </p:bodyStyle>
    <p:otherStyle>
      <a:defPPr>
        <a:defRPr lang="zh-TW"/>
      </a:defPPr>
      <a:lvl1pPr marL="0" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" hangingPunct="1">
        <a:defRPr sz="1800" kern="1200">
          <a:solidFill><a:schemeClr val="tx1"/></a:solidFill>
          <a:latin typeface="+mn-lt"/>
          <a:ea typeface="+mn-ea"/>
          <a:cs typeface="+mn-cs"/>
        </a:defRPr>
      </a:lvl1pPr>
    </p:otherStyle>
  </p:txStyles>
</p:sldMaster>`
	writeZipFile(w, "ppt/slideMasters/slideMaster1.xml", slideMasterXML)

	// ppt/slideMasters/_rels/slideMaster1.xml.rels — references both slideLayouts and theme
	slideMasterRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout2.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="../theme/theme1.xml"/>
</Relationships>`
	writeZipFile(w, "ppt/slideMasters/_rels/slideMaster1.xml.rels", slideMasterRels)

	// --- ppt/slideLayouts/slideLayout1.xml — Title Slide layout (ctrTitle + subTitle) ---
	// Background: solid accent1 fill. Title text: lt1 (white). Subtitle: lt1 with 75% opacity.
	// This makes the title slide visually distinct per theme.
	slideLayout1XML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
             xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
             xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
             showMasterSp="0" type="title" preserve="1">
  <p:cSld name="Title Slide">
    <p:bg>
      <p:bgPr>
        <a:solidFill><a:schemeClr val="accent1"/></a:solidFill>
        <a:effectLst/>
      </p:bgPr>
    </p:bg>
    <p:spTree>
      <p:nvGrpSpPr>
        <p:cNvPr id="1" name=""/>
        <p:cNvGrpSpPr/>
        <p:nvPr/>
      </p:nvGrpSpPr>
      <p:grpSpPr/>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="10" name="Bottom Stripe"/>
          <p:cNvSpPr/>
          <p:nvPr/>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="0" y="6400800"/>
            <a:ext cx="12192000" cy="457200"/>
          </a:xfrm>
          <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
          <a:solidFill><a:schemeClr val="dk2"/></a:solidFill>
          <a:ln><a:noFill/></a:ln>
        </p:spPr>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="2" name="Title 1"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="ctrTitle"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="1524000" y="1122363"/>
            <a:ext cx="9144000" cy="2387600"/>
          </a:xfrm>
        </p:spPr>
        <p:txBody>
          <a:bodyPr anchor="b"/>
          <a:lstStyle>
            <a:lvl1pPr algn="ctr">
              <a:defRPr sz="4400" b="1">
                <a:solidFill><a:schemeClr val="lt1"/></a:solidFill>
                <a:latin typeface="+mj-lt"/>
                <a:ea typeface="+mj-ea"/>
                <a:cs typeface="+mj-cs"/>
              </a:defRPr>
            </a:lvl1pPr>
          </a:lstStyle>
          <a:p><a:r><a:rPr lang="zh-TW"/><a:t>Title</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="3" name="Subtitle 2"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="subTitle" idx="1"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="1524000" y="3602038"/>
            <a:ext cx="9144000" cy="1655762"/>
          </a:xfrm>
        </p:spPr>
        <p:txBody>
          <a:bodyPr/>
          <a:lstStyle>
            <a:lvl1pPr marL="0" indent="0" algn="ctr">
              <a:buNone/>
              <a:defRPr sz="2400">
                <a:solidFill><a:schemeClr val="lt1"><a:alpha val="75000"/></a:schemeClr></a:solidFill>
                <a:latin typeface="+mn-lt"/>
                <a:ea typeface="+mn-ea"/>
                <a:cs typeface="+mn-cs"/>
              </a:defRPr>
            </a:lvl1pPr>
          </a:lstStyle>
          <a:p><a:r><a:rPr lang="zh-TW"/><a:t>Subtitle</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
  <p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>
</p:sldLayout>`
	writeZipFile(w, "ppt/slideLayouts/slideLayout1.xml", slideLayout1XML)

	// slideLayout1 rels — references slideMaster1
	writeZipFile(w, "ppt/slideLayouts/_rels/slideLayout1.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/>
</Relationships>`)

	// --- ppt/slideLayouts/slideLayout2.xml — Content layout (title + body placeholder) ---
	// Title area: accent1 background bar behind the title text (white text on accent1 bar).
	// Body area: standard text on white background (inherits from master bg).
	// Bottom accent stripe for visual consistency.
	slideLayout2XML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
             xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
             xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
             type="obj" preserve="1">
  <p:cSld name="Title, Content">
    <p:spTree>
      <p:nvGrpSpPr>
        <p:cNvPr id="1" name=""/>
        <p:cNvGrpSpPr/>
        <p:nvPr/>
      </p:nvGrpSpPr>
      <p:grpSpPr/>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="10" name="Title Bar Background"/>
          <p:cNvSpPr/>
          <p:nvPr/>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="0" y="0"/>
            <a:ext cx="12192000" cy="1524000"/>
          </a:xfrm>
          <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
          <a:solidFill><a:schemeClr val="accent1"/></a:solidFill>
          <a:ln><a:noFill/></a:ln>
        </p:spPr>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="11" name="Bottom Stripe"/>
          <p:cNvSpPr/>
          <p:nvPr/>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="0" y="6705600"/>
            <a:ext cx="12192000" cy="152400"/>
          </a:xfrm>
          <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
          <a:solidFill><a:schemeClr val="accent1"/></a:solidFill>
          <a:ln><a:noFill/></a:ln>
        </p:spPr>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="2" name="Title 1"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="title"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="609600" y="274638"/>
            <a:ext cx="10972800" cy="1143000"/>
          </a:xfrm>
        </p:spPr>
        <p:txBody>
          <a:bodyPr anchor="ctr"/>
          <a:lstStyle>
            <a:lvl1pPr>
              <a:defRPr sz="3600" b="1">
                <a:solidFill><a:schemeClr val="lt1"/></a:solidFill>
                <a:latin typeface="+mj-lt"/>
                <a:ea typeface="+mj-ea"/>
                <a:cs typeface="+mj-cs"/>
              </a:defRPr>
            </a:lvl1pPr>
          </a:lstStyle>
          <a:p><a:r><a:rPr lang="zh-TW"/><a:t>Title</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="3" name="Content Placeholder 2"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph idx="1"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr>
          <a:xfrm>
            <a:off x="609600" y="1700213"/>
            <a:ext cx="10972800" cy="4800600"/>
          </a:xfrm>
        </p:spPr>
        <p:txBody>
          <a:bodyPr anchor="t"/>
          <a:lstStyle/>
          <a:p><a:r><a:rPr lang="zh-TW"/><a:t>Content</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
  <p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>
</p:sldLayout>`
	writeZipFile(w, "ppt/slideLayouts/slideLayout2.xml", slideLayout2XML)

	// slideLayout2 rels — references slideMaster1
	writeZipFile(w, "ppt/slideLayouts/_rels/slideLayout2.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/>
</Relationships>`)

	// --- Generate slides ---

	// Slide 1: Title slide (uses slideLayout1 — ctrTitle + subTitle placeholders)
	writePptxTitleSlide(w, 1, title, content)

	// Content slides (use slideLayout2 — title + body placeholders)
	if content != nil {
		for i, slide := range content.Slides {
			writePptxContentSlide(w, i+2, slide.Title, slide.Content)
		}
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to finalize pptx: %w", err)
	}

	return os.WriteFile(filePath, buf.Bytes(), 0644)
}

// writePptxTitleSlide writes slide 1 using title layout placeholders (ctrTitle + subTitle).
// Slides are minimal — they only supply text content and inherit position/style from layout.
func writePptxTitleSlide(w *zip.Writer, slideNum int, title string, content *geminiDocResponse) {
	// Slide rels — references slideLayout1 (title layout)
	writeZipFile(w, fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", slideNum), `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`)

	// Subtitle text: use the first slide's content if available, else a default
	subtitle := "AI Generated Presentation"
	if content != nil && len(content.Slides) > 0 {
		firstContent := strings.TrimSpace(content.Slides[0].Content)
		if firstContent != "" {
			// Use first line only as subtitle
			if idx := strings.Index(firstContent, "\n"); idx > 0 {
				subtitle = strings.TrimSpace(firstContent[:idx])
			} else {
				subtitle = firstContent
			}
		}
	}

	slideXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
       xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
       xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:spTree>
      <p:nvGrpSpPr>
        <p:cNvPr id="1" name=""/>
        <p:cNvGrpSpPr/>
        <p:nvPr/>
      </p:nvGrpSpPr>
      <p:grpSpPr/>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="2" name="Title 1"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="ctrTitle"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr/>
        <p:txBody>
          <a:bodyPr/>
          <a:lstStyle/>
          <a:p><a:r><a:rPr lang="zh-TW" dirty="0"/><a:t>%s</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="3" name="Subtitle 2"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="subTitle" idx="1"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr/>
        <p:txBody>
          <a:bodyPr/>
          <a:lstStyle/>
          <a:p><a:r><a:rPr lang="zh-TW" dirty="0"/><a:t>%s</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
  <p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>
</p:sld>`, escapeXML(title), escapeXML(subtitle))

	writeZipFile(w, fmt.Sprintf("ppt/slides/slide%d.xml", slideNum), slideXML)
}

// writePptxContentSlide writes a content slide using content layout placeholders (title + body idx=1).
// All styling (fonts, colors, bullets) is inherited from slideMaster txStyles.
func writePptxContentSlide(w *zip.Writer, slideNum int, slideTitle, slideContent string) {
	// Slide rels — references slideLayout2 (content layout)
	writeZipFile(w, fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", slideNum), `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout2.xml"/>
</Relationships>`)

	// Build body paragraphs — each line becomes a bullet paragraph (inherits from bodyStyle)
	lines := strings.Split(slideContent, "\n")
	var bodyParagraphs strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		bodyParagraphs.WriteString(fmt.Sprintf(`<a:p><a:r><a:rPr lang="zh-TW" dirty="0"/><a:t>%s</a:t></a:r></a:p>`, escapeXML(line)))
	}

	// If no body paragraphs, add an empty one
	bodyContent := bodyParagraphs.String()
	if bodyContent == "" {
		bodyContent = `<a:p><a:endParaRPr lang="zh-TW"/></a:p>`
	}

	slideXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
       xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
       xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld>
    <p:spTree>
      <p:nvGrpSpPr>
        <p:cNvPr id="1" name=""/>
        <p:cNvGrpSpPr/>
        <p:nvPr/>
      </p:nvGrpSpPr>
      <p:grpSpPr/>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="2" name="Title 1"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph type="title"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr/>
        <p:txBody>
          <a:bodyPr/>
          <a:lstStyle/>
          <a:p><a:r><a:rPr lang="zh-TW" dirty="0"/><a:t>%s</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
      <p:sp>
        <p:nvSpPr>
          <p:cNvPr id="3" name="Content Placeholder 2"/>
          <p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr>
          <p:nvPr><p:ph idx="1"/></p:nvPr>
        </p:nvSpPr>
        <p:spPr/>
        <p:txBody>
          <a:bodyPr/>
          <a:lstStyle/>
          %s
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
  <p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>
</p:sld>`, escapeXML(slideTitle), bodyContent)

	writeZipFile(w, fmt.Sprintf("ppt/slides/slide%d.xml", slideNum), slideXML)
}

// --- Helpers ---

func writeZipFile(w *zip.Writer, name, content string) {
	f, err := w.Create(name)
	if err != nil {
		log.Printf("[WARN] Failed to create zip entry %s: %v", name, err)
		return
	}
	if _, err := f.Write([]byte(content)); err != nil {
		log.Printf("[WARN] Failed to write zip entry %s: %v", name, err)
	}
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
