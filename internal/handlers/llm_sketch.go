package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"nwork/config"
	"nwork/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// maxImageBase64Size is the max allowed base64 size per image (~4MB base64 ≈ 3MB binary)
const maxImageBase64Size = 4 * 1024 * 1024

type sketchGenerateRequest struct {
	Prompt          string   `json:"prompt"`
	SketchImage     string   `json:"sketch_image,omitempty"`     // base64 data URL of current canvas
	ReferenceImage  string   `json:"reference_image,omitempty"`  // DEPRECATED: single reference image (kept for backward compat)
	ReferenceImages []string `json:"reference_images,omitempty"` // base64 data URLs of attached reference images (max 2)
	SketchID        string   `json:"sketch_id,omitempty"`        // optional: current sketch ID for linking
	Model           string   `json:"model,omitempty"`
}

// GenerateLLMSketch 透過 Gemini 根據草圖和 prompt 生成圖片
// POST /api/v1/ai/sketch-generate
func GenerateLLMSketch(c *fiber.Ctx) error {
	cfg := config.Load()
	if cfg.LLM.Provider != "gemini" {
		return c.Status(501).JSON(fiber.Map{"error": "此功能暫不支援目前的 AI 供應商"})
	}
	if cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "AI API Key 未配置"})
	}

	var req sketchGenerateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Prompt 為必填"})
	}

	model := strings.TrimSpace(req.Model)
	if model == "" || model == "gemini" {
		model = cfg.LLM.ImageModel
	}
	if model == "" {
		model = "gemini-3-pro-image-preview"
	}

	// Build request parts
	parts := make([]map[string]interface{}, 0, 4)

	// If a sketch image is provided, include it as inline_data
	if req.SketchImage != "" {
		imageData, mimeType := parseDataURL(req.SketchImage)
		if imageData != "" {
			if len(imageData) > maxImageBase64Size {
				log.Printf("[WARN] Sketch image too large (%d bytes base64), skipping", len(imageData))
			} else {
				parts = append(parts, map[string]interface{}{
					"inlineData": map[string]interface{}{
						"mimeType": mimeType,
						"data":     imageData,
					},
				})
			}
		}
	}

	// Collect reference images: prefer new array field, fall back to legacy single field
	refImages := req.ReferenceImages
	if len(refImages) == 0 && req.ReferenceImage != "" {
		refImages = []string{req.ReferenceImage}
	}
	// Limit to 2 reference images (Gemini 2.5 Flash Image allows max 3 parts total)
	if len(refImages) > 2 {
		refImages = refImages[:2]
	}
	for _, refImg := range refImages {
		imageData, mimeType := parseDataURL(refImg)
		if imageData != "" {
			if len(imageData) > maxImageBase64Size {
				log.Printf("[WARN] Reference image too large (%d bytes base64), skipping", len(imageData))
				continue
			}
			parts = append(parts, map[string]interface{}{
				"inlineData": map[string]interface{}{
					"mimeType": mimeType,
					"data":     imageData,
				},
			})
		}
	}

	// Build the text prompt based on what images are provided
	hasSketch := req.SketchImage != ""
	refCount := len(refImages)
	var fullPrompt string
	switch {
	case hasSketch && refCount >= 2:
		fullPrompt = fmt.Sprintf("I've attached three images: the first is my current sketch/canvas, and the next two are reference images for style and content guidance. Based on all of them, generate a refined, high-quality image. User instruction: %s", prompt)
	case hasSketch && refCount == 1:
		fullPrompt = fmt.Sprintf("I've attached two images: the first is my current sketch/canvas, and the second is a reference image. Based on both, generate a refined, high-quality image. User instruction: %s", prompt)
	case hasSketch:
		fullPrompt = fmt.Sprintf("Based on the attached sketch drawing, generate a refined, high-quality image. User instruction: %s", prompt)
	case refCount >= 2:
		fullPrompt = fmt.Sprintf("I've attached two reference images. Using them as visual context, generate a new image. User instruction: %s", prompt)
	case refCount == 1:
		fullPrompt = fmt.Sprintf("I've attached a reference image. Using it as visual context, generate a new image. User instruction: %s", prompt)
	default:
		fullPrompt = fmt.Sprintf("Generate an image based on this description: %s", prompt)
	}
	parts = append(parts, map[string]interface{}{
		"text": fullPrompt,
	})

	geminiReq := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": parts,
			},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"IMAGE", "TEXT"},
		},
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to build AI request"})
	}

	// Fallback model chain: if primary model returns 503/429, try the next one
	imageModels := buildImageModelChain(model)

	payloadSize := len(payload)
	log.Printf("[INFO] Sketch generate: primary_model=%s, payload_size=%d bytes, parts=%d, fallback_chain=%v", model, payloadSize, len(parts), imageModels)

	client := &http.Client{Timeout: 180 * time.Second}

	var geminiResp geminiGenerateContentResponse
	var usedModel string

	for i, tryModel := range imageModels {
		apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", tryModel, cfg.LLM.APIKey)

		resp, body, err := callGeminiWithRetry(client, apiURL, payload, tryModel)
		if err != nil {
			// Network-level error after retries exhausted
			if i < len(imageModels)-1 {
				log.Printf("[WARN] Sketch generate: model %s network error, trying fallback: %v", tryModel, err)
				continue
			}
			return c.Status(500).JSON(fiber.Map{"error": "Failed to call AI API: " + err.Error()})
		}

		if resp.StatusCode == 503 || resp.StatusCode == 429 || resp.StatusCode == 404 {
			resp.Body.Close()
			if i < len(imageModels)-1 {
				log.Printf("[WARN] Sketch generate: model %s returned %d, trying fallback model", tryModel, resp.StatusCode)
				continue
			}
			// Last model also failed
			log.Printf("[ERROR] Sketch generate: all models exhausted, last error %d: %s", resp.StatusCode, string(body))
			return c.Status(resp.StatusCode).JSON(fiber.Map{"error": string(body)})
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			log.Printf("[ERROR] Sketch generate API (%s) returned %d: %s", tryModel, resp.StatusCode, string(body))
			return c.Status(resp.StatusCode).JSON(fiber.Map{"error": string(body)})
		}

		// Decode successful response
		if err := json.Unmarshal(body, &geminiResp); err != nil {
			resp.Body.Close()
			log.Printf("[ERROR] Sketch generate: failed to decode response from %s: %v", tryModel, err)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to parse AI response: " + err.Error()})
		}
		resp.Body.Close()
		usedModel = tryModel
		break
	}

	if usedModel == "" {
		return c.Status(500).JSON(fiber.Map{"error": "No AI model available"})
	}

	// Extract image and text from response
	var imageData string
	var imageURL string
	var textResponse string

	for _, candidate := range geminiResp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && part.InlineData.Data != "" {
				imageData = part.InlineData.Data
				mimeType := part.InlineData.MimeType
				if mimeType == "" {
					mimeType = "image/png"
				}
				imageURL = fmt.Sprintf("data:%s;base64,%s", mimeType, part.InlineData.Data)
			}
			if part.Text != "" {
				textResponse = part.Text
			}
		}
	}

	// Save generation record to DB
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	var sketchIDPtr *uuid.UUID
	if req.SketchID != "" {
		if parsed, err := uuid.Parse(req.SketchID); err == nil {
			sketchIDPtr = &parsed
		}
	}
	if err := SaveSketchGeneration(tenantID, userID, sketchIDPtr, prompt, imageURL, usedModel, "sketch"); err != nil {
		log.Printf("[WARN] Failed to save sketch generation history: %v", err)
	}

	return c.JSON(fiber.Map{
		"image_data": imageData,
		"image_url":  imageURL,
		"text":       textResponse,
		"model":      usedModel,
		"prompt":     prompt,
	})
}

// parseDataURL extracts base64 data and mime type from a data URL
// e.g. "data:image/png;base64,iVBOR..." -> ("iVBOR...", "image/png")
func parseDataURL(dataURL string) (string, string) {
	if !strings.HasPrefix(dataURL, "data:") {
		return dataURL, "image/png"
	}

	// Split "data:image/png;base64,..." into parts
	commaIdx := strings.Index(dataURL, ",")
	if commaIdx < 0 {
		return "", ""
	}

	header := dataURL[5:commaIdx] // Remove "data:" prefix
	data := dataURL[commaIdx+1:]

	// Extract mime type from "image/png;base64"
	mimeType := "image/png"
	semicolonIdx := strings.Index(header, ";")
	if semicolonIdx > 0 {
		mimeType = header[:semicolonIdx]
	} else {
		mimeType = header
	}

	return data, mimeType
}

// buildImageModelChain returns a list of models to try, starting with the
// requested model and appending fallback alternatives for image generation.
func buildImageModelChain(primary string) []string {
	// Gemini models that support image generation (responseModalities: IMAGE),
	// ordered by quality/preference. Only models confirmed via ListModels API.
	allImageModels := []string{
		"gemini-3-pro-image-preview",
		"gemini-3.1-flash-image-preview",
		"gemini-2.5-flash-image",
		"gemini-2.0-flash",
	}

	chain := []string{primary}
	for _, m := range allImageModels {
		if m != primary {
			chain = append(chain, m)
		}
	}
	return chain
}

// callGeminiWithRetry calls the Gemini API with retry on transient network
// errors (EOF, connection reset). Returns the response, body bytes, and error.
func callGeminiWithRetry(client *http.Client, apiURL string, payload []byte, model string) (*http.Response, []byte, error) {
	maxRetries := 2
	for attempt := 1; attempt <= maxRetries; attempt++ {
		httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("[ERROR] Gemini call (%s) attempt %d/%d failed: %v", model, attempt, maxRetries, err)
			if attempt < maxRetries && (strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "connection reset")) {
				time.Sleep(2 * time.Second)
				continue
			}
			return nil, nil, err
		}

		body, _ := io.ReadAll(resp.Body)
		return resp, body, nil
	}
	return nil, nil, fmt.Errorf("all retries exhausted for model %s", model)
}
