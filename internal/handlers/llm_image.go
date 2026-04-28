package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"nwork/config"

	"github.com/gofiber/fiber/v2"
)

type imageGenerateRequest struct {
	Prompt      string `json:"prompt"`
	Model       string `json:"model,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Number      int    `json:"number,omitempty"`
}

// Gemini 圖像生成 API 回應格式
type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData,omitempty"`
				Text string `json:"text,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// callGeminiImageGeneration is the core image generation logic, extracted so it
// can be reused by both the HTTP handler and the tool-calling flow in ChatWithLLM.
// It calls the Gemini image model and returns data URLs (base64) of generated images.
// On 503/429 it automatically falls back to alternative image-capable models.
func callGeminiImageGeneration(apiKey, model, prompt, aspect string) (dataURLs []string, err error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if model == "" {
		model = "gemini-3-pro-image-preview"
	}
	if aspect == "" {
		aspect = "1:1"
	}

	fullPrompt := fmt.Sprintf("Generate image with %s aspect ratio: %s", aspect, prompt)
	geminiReq := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": fullPrompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"IMAGE", "TEXT"},
		},
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to build Gemini request: %w", err)
	}

	imageModels := buildImageModelChain(model)
	client := &http.Client{Timeout: 180 * time.Second}

	log.Printf("[INFO] Image generate: primary_model=%s, fallback_chain=%v", model, imageModels)

	for i, tryModel := range imageModels {
		apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", tryModel, apiKey)

		resp, body, callErr := callGeminiWithRetry(client, apiURL, payload, tryModel)
		if callErr != nil {
			if i < len(imageModels)-1 {
				log.Printf("[WARN] Image generate: model %s network error, trying fallback: %v", tryModel, callErr)
				continue
			}
			return nil, fmt.Errorf("failed to call Gemini API: %w", callErr)
		}

		if resp.StatusCode == 503 || resp.StatusCode == 429 || resp.StatusCode == 404 {
			resp.Body.Close()
			if i < len(imageModels)-1 {
				log.Printf("[WARN] Image generate: model %s returned %d, trying fallback", tryModel, resp.StatusCode)
				continue
			}
			return nil, fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(body))
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(body))
		}

		resp.Body.Close()

		var geminiResp geminiGenerateContentResponse
		if err := json.Unmarshal(body, &geminiResp); err != nil {
			return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
		}

		for _, candidate := range geminiResp.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.InlineData != nil && part.InlineData.Data != "" {
					mimeType := part.InlineData.MimeType
					if mimeType == "" {
						mimeType = "image/png"
					}
					dataURLs = append(dataURLs, fmt.Sprintf("data:%s;base64,%s", mimeType, part.InlineData.Data))
				}
			}
		}

		if len(dataURLs) > 0 {
			log.Printf("[INFO] Image generate: success with model %s", tryModel)
		}
		return dataURLs, nil
	}

	return nil, fmt.Errorf("all image models exhausted")
}

// callGeminiImageEdit generates an image using Gemini with a reference image as input.
// This is used for scene reference generation: given a user's photo (e.g. a car) and
// a scene description (e.g. "this car at a car wash centre"), Gemini produces a composed
// scene image that Kling can use as the first frame for image-to-video generation.
// refImageB64 should be raw base64 (no "data:" prefix) and refMimeType is e.g. "image/jpeg".
func callGeminiImageEdit(apiKey, model, prompt, aspect, refImageB64, refMimeType string) (dataURLs []string, err error) {
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if refImageB64 == "" {
		return nil, fmt.Errorf("reference image is required")
	}
	if model == "" {
		model = "gemini-3-pro-image-preview"
	}
	if aspect == "" {
		aspect = "16:9"
	}
	if refMimeType == "" {
		refMimeType = "image/jpeg"
	}

	fullPrompt := fmt.Sprintf("Generate image with %s aspect ratio: %s", aspect, prompt)
	geminiReq := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"inlineData": map[string]interface{}{
							"mimeType": refMimeType,
							"data":     refImageB64,
						},
					},
					{"text": fullPrompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"IMAGE", "TEXT"},
		},
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to build Gemini request: %w", err)
	}

	imageModels := buildImageModelChain(model)
	client := &http.Client{Timeout: 180 * time.Second}

	log.Printf("[INFO] Image edit: primary_model=%s, aspect=%s, prompt=%s", model, aspect, truncateStr(prompt, 80))

	for i, tryModel := range imageModels {
		apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", tryModel, apiKey)

		resp, body, callErr := callGeminiWithRetry(client, apiURL, payload, tryModel)
		if callErr != nil {
			if i < len(imageModels)-1 {
				log.Printf("[WARN] Image edit: model %s network error, trying fallback: %v", tryModel, callErr)
				continue
			}
			return nil, fmt.Errorf("failed to call Gemini API: %w", callErr)
		}

		if resp.StatusCode == 503 || resp.StatusCode == 429 || resp.StatusCode == 404 {
			resp.Body.Close()
			if i < len(imageModels)-1 {
				log.Printf("[WARN] Image edit: model %s returned %d, trying fallback", tryModel, resp.StatusCode)
				continue
			}
			return nil, fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(body))
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(body))
		}

		resp.Body.Close()

		var geminiResp geminiGenerateContentResponse
		if err := json.Unmarshal(body, &geminiResp); err != nil {
			return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
		}

		for _, candidate := range geminiResp.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.InlineData != nil && part.InlineData.Data != "" {
					mimeType := part.InlineData.MimeType
					if mimeType == "" {
						mimeType = "image/png"
					}
					dataURLs = append(dataURLs, fmt.Sprintf("data:%s;base64,%s", mimeType, part.InlineData.Data))
				}
			}
		}

		if len(dataURLs) > 0 {
			log.Printf("[INFO] Image edit: success with model %s", tryModel)
		}
		return dataURLs, nil
	}

	return nil, fmt.Errorf("all image models exhausted")
}

// GenerateLLMImage 透過 Gemini 生成圖片
// POST /api/v1/llm/image
func GenerateLLMImage(c *fiber.Ctx) error {
	cfg := config.Load()
	if cfg.LLM.Provider != "gemini" {
		return c.Status(501).JSON(fiber.Map{"error": "此功能暫不支援目前的 AI 供應商"})
	}
	if cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "AI API Key 未配置"})
	}

	var req imageGenerateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Prompt 為必填"})
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = cfg.LLM.ImageModel
	}

	aspect := strings.TrimSpace(req.AspectRatio)

	dataURLs, err := callGeminiImageGeneration(cfg.LLM.APIKey, model, prompt, aspect)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Extract raw base64 images (without data: prefix) for backward compatibility
	images := make([]string, 0, len(dataURLs))
	for _, du := range dataURLs {
		// "data:image/png;base64,AAAA..." -> "AAAA..."
		if idx := strings.Index(du, ","); idx >= 0 {
			images = append(images, du[idx+1:])
		}
	}

	return c.JSON(fiber.Map{
		"model":        model,
		"prompt":       prompt,
		"images":       images,
		"data_urls":    dataURLs,
		"count":        len(dataURLs),
		"aspect_ratio": aspect,
	})
}
