package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ─── Request / Response Types (Kling 3.0 Omni) ───────────────────

// videoGenerateRequest is the request body from frontend for multi-shot video generation.
// Frontend sends all storyboard shots in a single request; backend builds one Kling
// omni-video request with all shots (max 15s total duration, sound: "on").
type videoGenerateRequest struct {
	AspectRatio string             `json:"aspect_ratio,omitempty"` // "16:9", "9:16", "1:1", etc.
	Duration    string             `json:"duration,omitempty"`     // total duration: "5", "10", "15"
	Shots       []videoShotRequest `json:"shots"`                  // multi-shot prompts (max 6, total <= 15s)
	Model       string             `json:"model,omitempty"`        // model override (default: kling-v3-omni)
}

// videoShotRequest is a single shot in the multi-shot request.
type videoShotRequest struct {
	Index    int    `json:"index"`           // 1-based shot index
	Prompt   string `json:"prompt"`          // scene description (user's language; translated server-side)
	Duration string `json:"duration"`        // "5", "10", "15" (seconds, string)
	Image    string `json:"image,omitempty"` // base64 data URL of scene image (Gemini-generated)
}

// --- Kling Omni API response types ---

// klingCreateResponse is the response from POST /v1/videos/omni-video
type klingCreateResponse struct {
	Code    int    `json:"code"`    // 0 = success
	Message string `json:"message"` // "success" or error message
	Data    *struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"` // "submitted", "processing", etc.
	} `json:"data,omitempty"`
}

// klingPollResponse is the response from GET /v1/videos/{task_id}
type klingPollResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		TaskID        string `json:"task_id"`
		TaskStatus    string `json:"task_status"` // "submitted", "processing", "succeed", "failed"
		TaskStatusMsg string `json:"task_status_msg,omitempty"`
		TaskResult    *struct {
			Videos []struct {
				ID       string `json:"id"`
				URL      string `json:"url"`
				Duration string `json:"duration"`
			} `json:"videos,omitempty"`
		} `json:"task_result,omitempty"`
	} `json:"data,omitempty"`
}

// --- Video-Extend chaining state ---

// extendInfo tracks remaining prompts to be appended via video-extend after
// the initial omni-video (which is limited to 15s total) completes.
type extendInfo struct {
	Prompts      []string // remaining translated prompts to extend, in order
	BaseURL      string   // Kling API base URL
	AccessKey    string   // for JWT generation
	SecretKey    string   // for JWT generation
	Model        string   // model name
	IsExtendTask bool     // true if this task_id is from video-extend (not omni-video)
}

// extendQueue maps an active Kling task_id to the extend info needed after it
// completes. When a task succeeds and has an entry here, PollVideoOperation
// automatically submits a video-extend request and replaces the entry with
// the new extend task_id.
var (
	extendQueue   = make(map[string]*extendInfo)
	extendQueueMu sync.Mutex
)

// Note: Kling video-extend only supports V1.0/V1.5/V1.6 models, NOT v3-omni.
// All shots must fit within a single omni-video request (max 15s total).

// --- Kling JWT Helper ---

// generateKlingJWT creates an HS256 JWT for Kling API authentication.
// iss = access_key, signed with secret_key, valid for 1 hour.
func generateKlingJWT(accessKey, secretKey string) (string, error) {
	now := time.Now()
	payload := jwt.MapClaims{
		"iss": accessKey,
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(-5 * time.Second).Unix(),
		"iat": now.Add(-10 * time.Second).Unix(),
		"jti": fmt.Sprintf("idt%d", now.UnixMilli()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)
	return token.SignedString([]byte(secretKey))
}

// --- Image Save Helper ---

// saveBase64ImageForKling decodes a base64 data URL image and saves it to local storage.
// Returns the public URL that Kling can access (full domain URL, not relative path).
func saveBase64ImageForKling(base64DataURL string, c *fiber.Ctx) (string, error) {
	if base64DataURL == "" {
		return "", fmt.Errorf("empty image data")
	}

	// Strip data URL prefix: "data:image/png;base64,..."
	imageData := base64DataURL
	ext := ".png"
	if strings.HasPrefix(imageData, "data:") {
		if idx := strings.Index(imageData, ";base64,"); idx >= 0 {
			mime := imageData[5:idx]
			imageData = imageData[idx+8:]
			// Determine extension from mime type
			switch mime {
			case "image/jpeg":
				ext = ".jpg"
			case "image/webp":
				ext = ".webp"
			default:
				ext = ".png"
			}
		} else if idx := strings.Index(imageData, ","); idx >= 0 {
			imageData = imageData[idx+1:]
		}
	}

	// Decode base64
	imgBytes, err := base64.StdEncoding.DecodeString(imageData)
	if err != nil {
		// Try raw encoding (no padding)
		imgBytes, err = base64.RawStdEncoding.DecodeString(imageData)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 image: %w", err)
		}
	}

	// Get tenant ID
	tenantUUID := middleware.GetTenantID(c)
	tenantID := tenantUUID.String()
	if tenantID == "" || tenantID == "00000000-0000-0000-0000-000000000000" {
		tenantID = "default"
	}

	// Create upload directory
	dateStr := time.Now().Format("2006-01-02")
	uploadDir := filepath.Join("web", "uploads", tenantID, "video-frames", dateStr)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Save file
	filename := uuid.New().String() + ext
	filePath := filepath.Join(uploadDir, filename)
	if err := os.WriteFile(filePath, imgBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to write image: %w", err)
	}

	// Build public URL (full domain, not relative — Kling needs to fetch it)
	cfg := config.Load()
	scheme := cfg.Domain.Scheme
	if scheme == "" {
		scheme = "https"
	}
	baseDomain := cfg.Domain.BaseDomain

	relativePath := fmt.Sprintf("/uploads/%s/video-frames/%s/%s", tenantID, dateStr, filename)

	// Try to get tenant subdomain for full URL
	var tenantSubdomain string
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantUUID).First(&tenant).Error; err == nil {
		tenantSubdomain = tenant.Subdomain
	}

	var publicURL string
	if tenantSubdomain != "" && baseDomain != "" {
		publicURL = fmt.Sprintf("%s://%s.%s%s", scheme, tenantSubdomain, baseDomain, relativePath)
	} else {
		// Fallback: use request host
		host := c.Get("Host")
		if host != "" {
			publicURL = fmt.Sprintf("%s://%s%s", scheme, host, relativePath)
		} else {
			// Last resort: relative URL (may not work for Kling)
			publicURL = relativePath
		}
	}

	log.Printf("[VideoGen] Image saved for Kling: %s (%d bytes)", publicURL, len(imgBytes))
	return publicURL, nil
}

// ─── GenerateVideo: Submit multi-shot video generation to Kling 3.0 Omni ─────

// GenerateVideo submits a single multi-shot video generation request to Kling 3.0 Omni API.
// All storyboard shots are sent in one request (max 15s total, sound: "on").
// POST /api/v1/llm/video
func GenerateVideo(c *fiber.Ctx) error {
	cfg := config.Load()

	// Kling credentials
	accessKey := cfg.Kling.AccessKey
	secretKey := cfg.Kling.SecretKey
	if accessKey == "" || secretKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "Kling API 金鑰未配置 (KLING_ACCESS_KEY / KLING_SECRET_KEY)"})
	}

	model := cfg.Kling.Model
	if model == "" {
		model = "kling-v3-omni"
	}
	baseURL := cfg.Kling.BaseURL
	if baseURL == "" {
		baseURL = "https://api-singapore.klingai.com"
	}

	var req videoGenerateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if len(req.Shots) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "至少需要一個分鏡 (shots)"})
	}
	if len(req.Shots) > 6 {
		return c.Status(400).JSON(fiber.Map{"error": "最多支援 6 個分鏡"})
	}

	// Model override from request
	if m := strings.TrimSpace(req.Model); m != "" {
		model = m
	}

	// Aspect ratio (default 9:16 for vertical video)
	aspectRatio := strings.TrimSpace(req.AspectRatio)
	if aspectRatio == "" {
		aspectRatio = "9:16"
	}
	// Validate Kling-supported ratios
	validRatios := map[string]bool{"16:9": true, "9:16": true, "1:1": true, "4:3": true, "3:4": true, "2.39:1": true, "21:9": true}
	if !validRatios[aspectRatio] {
		aspectRatio = "9:16"
	}

	// Calculate and clamp each shot duration to Kling-valid range: 3-15 seconds.
	// Total duration across all shots must be <= 15s (single request, no batching).
	totalDuration := 0
	for i := range req.Shots {
		d := parseDurationSeconds(req.Shots[i].Duration)
		if d < 3 {
			d = 5
		}
		if d > 15 {
			d = 15
		}
		req.Shots[i].Duration = strconv.Itoa(d)
		totalDuration += d
	}
	// Clamp total duration to max 15s
	if totalDuration > 15 {
		log.Printf("[VideoGen] Total duration %ds exceeds 15s, clamping shot durations", totalDuration)
		totalDuration = 0
		for i := range req.Shots {
			d, _ := strconv.Atoi(req.Shots[i].Duration)
			if totalDuration+d > 15 {
				d = 15 - totalDuration
				if d < 3 {
					d = 3
				}
			}
			req.Shots[i].Duration = strconv.Itoa(d)
			totalDuration += d
			if totalDuration >= 15 {
				// Truncate remaining shots
				req.Shots = req.Shots[:i+1]
				break
			}
		}
	}

	// Generate JWT for auth
	jwtToken, err := generateKlingJWT(accessKey, secretKey)
	if err != nil {
		log.Printf("[VideoGen] Failed to generate Kling JWT: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate auth token"})
	}

	apiURL := baseURL + "/v1/videos/omni-video"

	// Translate prompts and save images for all shots
	var imageList []map[string]interface{}
	var multiPrompt []map[string]interface{}
	imgIdx := 0
	for i, shot := range req.Shots {
		prompt := strings.TrimSpace(shot.Prompt)
		if prompt == "" {
			prompt = "繼續場景"
		}

		var imageURL string
		if shot.Image != "" {
			publicURL, err := saveBase64ImageForKling(shot.Image, c)
			if err != nil {
				log.Printf("[VideoGen] Failed to save image for shot %d: %v", i+1, err)
				return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("分鏡圖片儲存失敗: %v", err)})
			}
			imageURL = publicURL
		}

		tp := prompt
		if imageURL != "" {
			imgIdx++
			tp = fmt.Sprintf("<<<image_%d>>> %s", imgIdx, tp)
			imageList = append(imageList, map[string]interface{}{
				"image_url": imageURL,
			})
		}

		// Kling API enforces max 512 chars per multi_prompt[].prompt
		if len([]rune(tp)) > 512 {
			runes := []rune(tp)
			tp = string(runes[:512])
			log.Printf("[VideoGen] Shot %d prompt truncated from %d to 512 chars", i+1, len(runes))
		}

		shotDuration := shot.Duration
		if shotDuration == "" {
			shotDuration = "5"
		}
		multiPrompt = append(multiPrompt, map[string]interface{}{
			"index":    i + 1,
			"prompt":   tp,
			"duration": shotDuration,
		})
	}

	if len(multiPrompt) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "沒有有效的分鏡 prompt"})
	}

	// Build single Kling omni-video request (all shots, max 15s total)
	isMultiShot := len(multiPrompt) > 1
	durationStr := strconv.Itoa(totalDuration)

	klingReq := map[string]interface{}{
		"model_name":   model,
		"multi_shot":   isMultiShot,
		"shot_type":    "customize",
		"prompt":       "",
		"multi_prompt": multiPrompt,
		"mode":         "pro",
		"sound":        "on",
		"duration":     durationStr,
		"aspect_ratio": aspectRatio,
	}
	if len(imageList) > 0 {
		klingReq["image_list"] = imageList
	}

	payload, err := json.Marshal(klingReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to build Kling request"})
	}

	log.Printf("[VideoGen] Submitting single request: shots=%d, duration=%ss, images=%d",
		len(multiPrompt), durationStr, len(imageList))
	{
		debugStr := string(payload)
		if len(debugStr) > 2000 {
			debugStr = debugStr[:2000] + "... (truncated)"
		}
		log.Printf("[VideoGen] Request body: %s", debugStr)
	}

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("[VideoGen] Failed to build request: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create request"})
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+jwtToken)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[VideoGen] Kling API call failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Kling API 呼叫失敗: " + err.Error()})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		log.Printf("[VideoGen] Kling API error %d: %s", resp.StatusCode, string(body))
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": "Kling API error: " + string(body)})
	}

	var klingResp klingCreateResponse
	if err := json.Unmarshal(body, &klingResp); err != nil {
		log.Printf("[VideoGen] Failed to parse response: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse Kling response"})
	}

	if klingResp.Code != 0 {
		log.Printf("[VideoGen] Kling error code %d: %s", klingResp.Code, klingResp.Message)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Kling API 錯誤: %s (code: %d)", klingResp.Message, klingResp.Code)})
	}

	if klingResp.Data == nil || klingResp.Data.TaskID == "" {
		log.Printf("[VideoGen] No task_id in response: %s", string(body))
		return c.Status(500).JSON(fiber.Map{"error": "Kling API 未返回 task_id"})
	}

	log.Printf("[VideoGen] task_id=%s, status=%s", klingResp.Data.TaskID, klingResp.Data.TaskStatus)

	return c.JSON(fiber.Map{
		"task_id":      klingResp.Data.TaskID,
		"done":         false,
		"model":        model,
		"status":       "submitted",
		"total_shots":  len(req.Shots),
		"aspect_ratio": aspectRatio,
	})
}

// PollVideoOperation checks the status of a Kling video generation task.
// GET /api/v1/llm/video/*
// The wildcard captures the task_id.
func PollVideoOperation(c *fiber.Ctx) error {
	cfg := config.Load()

	accessKey := cfg.Kling.AccessKey
	secretKey := cfg.Kling.SecretKey
	if accessKey == "" || secretKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "Kling API 金鑰未配置"})
	}

	baseURL := cfg.Kling.BaseURL
	if baseURL == "" {
		baseURL = "https://api-singapore.klingai.com"
	}

	// Extract task_id from URL
	taskID := c.Params("*")
	if taskID == "" {
		// Fallback: extract from raw URL path
		rawPath := c.OriginalURL()
		prefix := "/llm/video/"
		if idx := strings.Index(rawPath, prefix); idx >= 0 {
			candidate := rawPath[idx+len(prefix):]
			if qIdx := strings.Index(candidate, "?"); qIdx >= 0 {
				candidate = candidate[:qIdx]
			}
			taskID = candidate
		}
	}
	if taskID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Task ID is required"})
	}

	// Generate JWT for auth
	jwtToken, err := generateKlingJWT(accessKey, secretKey)
	if err != nil {
		log.Printf("[VideoPoll] Failed to generate Kling JWT: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate auth token"})
	}

	apiURL := fmt.Sprintf("%s/v1/videos/omni-video/%s", baseURL, taskID)
	log.Printf("[VideoPoll] Polling Kling task: task_id=%s", taskID)

	httpReq, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create request"})
	}
	httpReq.Header.Set("Authorization", "Bearer "+jwtToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Kling poll 失敗: " + err.Error()})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		bodyStr := strings.TrimSpace(string(body))
		if bodyStr == "" {
			bodyStr = fmt.Sprintf("HTTP %d (no response body)", resp.StatusCode)
		}
		log.Printf("[VideoPoll] Kling API error %d: %s", resp.StatusCode, bodyStr)
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": "Kling poll error: " + bodyStr})
	}

	var pollResp klingPollResponse
	if err := json.Unmarshal(body, &pollResp); err != nil {
		log.Printf("[VideoPoll] Failed to parse Kling poll response: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse poll response"})
	}

	if pollResp.Code != 0 {
		log.Printf("[VideoPoll] Kling poll error code %d: %s", pollResp.Code, pollResp.Message)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Kling poll 錯誤: %s", pollResp.Message)})
	}

	response := fiber.Map{
		"task_id": taskID,
	}

	if pollResp.Data == nil {
		response["done"] = false
		response["status"] = "unknown"
		return c.JSON(response)
	}

	status := pollResp.Data.TaskStatus
	log.Printf("[VideoPoll] task=%s status=%s", taskID, status)

	switch status {
	case "succeed":
		videoURL := ""
		if pollResp.Data.TaskResult != nil && len(pollResp.Data.TaskResult.Videos) > 0 {
			videoURL = pollResp.Data.TaskResult.Videos[0].URL
		}

		response["done"] = true
		if videoURL != "" {
			log.Printf("[VideoPoll] Kling video ready: %s", truncateStr(videoURL, 120))
			localURL := downloadVideoFromURL(videoURL, c)
			if localURL != "" {
				videoURL = localURL
			}
		}
		response["result"] = fiber.Map{
			"video_url": videoURL,
		}

	case "failed":
		response["done"] = true
		errMsg := "影片生成失敗"
		if pollResp.Data.TaskStatusMsg != "" {
			errMsg = pollResp.Data.TaskStatusMsg
		}
		log.Printf("[VideoPoll] task=%s FAILED: %s", taskID, errMsg)
		response["error"] = fiber.Map{
			"message": errMsg,
		}

	default: // "submitted", "processing"
		response["done"] = false
		response["status"] = status
		if pollResp.Data.TaskStatusMsg != "" {
			response["status_msg"] = pollResp.Data.TaskStatusMsg
		}
	}

	return c.JSON(response)
}

// ─── Helper functions ─────────────────────────────────────────────

// submitVideoExtend sends a POST to Kling's video-extend API to continue
// a completed video with a new prompt. Returns the new task_id.
func submitVideoExtend(baseURL, accessKey, secretKey, videoID, prompt string) (string, error) {
	jwtToken, err := generateKlingJWT(accessKey, secretKey)
	if err != nil {
		return "", fmt.Errorf("JWT generation failed: %w", err)
	}

	reqBody := map[string]interface{}{
		"prompt":          prompt,
		"video_id":        videoID,
		"negative_prompt": "",
		"callback_url":    "",
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	apiURL := baseURL + "/v1/videos/video-extend"
	log.Printf("[VideoExtend] Submitting video-extend: video_id=%s, prompt=%s", videoID, truncateStr(prompt, 80))

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		return "", fmt.Errorf("request creation failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+jwtToken)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var klingResp klingCreateResponse
	if err := json.Unmarshal(body, &klingResp); err != nil {
		return "", fmt.Errorf("parse response failed: %w, body: %s", err, string(body))
	}

	if klingResp.Code != 0 {
		return "", fmt.Errorf("Kling error code %d: %s", klingResp.Code, klingResp.Message)
	}

	if klingResp.Data == nil || klingResp.Data.TaskID == "" {
		return "", fmt.Errorf("no task_id in response: %s", string(body))
	}

	log.Printf("[VideoExtend] video-extend task submitted: task_id=%s", klingResp.Data.TaskID)
	return klingResp.Data.TaskID, nil
}

func parseDurationSeconds(d string) int {
	d = strings.TrimSpace(strings.ToLower(d))
	d = strings.TrimSuffix(d, "s")
	var sec int
	if _, err := fmt.Sscanf(d, "%d", &sec); err != nil {
		return 8 // default 8 seconds for Veo 3.1
	}
	if sec < 1 {
		sec = 8
	}
	return sec
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// downloadVideoFromURL downloads a video from a public URL to local storage.
// Returns the local URL on success, or empty string on failure.
func downloadVideoFromURL(videoURL string, c *fiber.Ctx) string {
	if videoURL == "" {
		return ""
	}

	log.Printf("[VideoPoll] Downloading video from URL: %s", truncateStr(videoURL, 120))

	client := &http.Client{Timeout: 120 * time.Second}
	dlReq, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		log.Printf("[VideoPoll] Failed to create download request: %v", err)
		return ""
	}

	dlResp, err := client.Do(dlReq)
	if err != nil {
		log.Printf("[VideoPoll] Failed to download video: %v", err)
		return ""
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != 200 {
		errBody, _ := io.ReadAll(dlResp.Body)
		log.Printf("[VideoPoll] Video download failed with status %d: %s", dlResp.StatusCode, string(errBody))
		return ""
	}

	// Get tenant ID from context
	tenantUUID := middleware.GetTenantID(c)
	tenantID := tenantUUID.String()
	if tenantID == "" || tenantID == "00000000-0000-0000-0000-000000000000" {
		log.Printf("[VideoPoll] No tenant ID in context, using 'default'")
		tenantID = "default"
	}

	// Create upload directory
	dateStr := time.Now().Format("2006-01-02")
	uploadDir := filepath.Join("web", "uploads", tenantID, "videos", dateStr)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("[VideoPoll] Failed to create upload directory: %v", err)
		return ""
	}

	// Save to file
	filename := uuid.New().String() + ".mp4"
	filePath := filepath.Join(uploadDir, filename)

	outFile, err := os.Create(filePath)
	if err != nil {
		log.Printf("[VideoPoll] Failed to create video file: %v", err)
		return ""
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, dlResp.Body)
	if err != nil {
		log.Printf("[VideoPoll] Failed to write video file: %v", err)
		os.Remove(filePath)
		return ""
	}

	localURL := fmt.Sprintf("/uploads/%s/videos/%s/%s", tenantID, dateStr, filename)
	log.Printf("[VideoPoll] Video saved locally: %s (%d bytes)", localURL, written)

	return localURL
}

// base64Decode decodes a base64 string (standard or URL-safe encoding).
func base64Decode(s string) ([]byte, error) {
	// Try standard encoding first
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	// Try URL-safe encoding
	if decoded, err := base64.URLEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	// Fallback: try raw (no padding) standard
	if decoded, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	// Last resort: raw URL-safe
	return base64.RawURLEncoding.DecodeString(s)
}

// ─── Video History ────────────────────────────────────────────────

// GetVideoHistory returns all messages that contain video_info in extra_fields.
// This covers videos generated from both vai-video page and vai-chat.
// GET /api/v1/llm/video/history
func GetVideoHistory(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	query := database.DB.Model(&models.Message{}).
		Where("tenant_id = ?", tenantID).
		Where("(from_user_id = ? OR to_user_id = ?)", userID, userID).
		Where("extra_fields->'video_info' IS NOT NULL").
		Where("trashed_at IS NULL").
		Where("COALESCE((extra_fields->'video_info'->>'deleted')::boolean, false) = false")

	// Optional per-project filter
	if projectID := c.Query("project_id"); projectID != "" {
		query = query.Where("id = ?", projectID)
	}

	var total int64
	query.Count(&total)

	var messages []models.Message
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&messages).Error; err != nil {
		log.Printf("[VideoHistory] Query failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  messages,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// SaveVideoRecord saves/creates a video project as a message.
// A video project contains multiple shots stored in extra_fields.shots[].
// POST /api/v1/llm/video/history
func SaveVideoRecord(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var body struct {
		Title  string                   `json:"title"`  // project title
		Shots  []map[string]interface{} `json:"shots"`  // shots array
		Status string                   `json:"status"` // "active", "done"
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = "影片專案"
	}

	status := body.Status
	if status == "" {
		status = "active"
	}

	// Build video_info at project level (so GetVideoHistory query works)
	videoInfo := map[string]interface{}{
		"status": status,
	}

	extraFields := map[string]interface{}{
		"video_info": videoInfo,
		"source":     "vai-video",
		"shots":      body.Shots,
	}

	msg := models.Message{
		TenantID:    tenantID,
		FromUserID:  &userID,
		Subject:     title,
		Content:     title,
		MessageType: "ai_video",
		ExtraFields: models.JSONB(extraFields),
	}

	if err := database.DB.Create(&msg).Error; err != nil {
		log.Printf("[VideoHistory] Failed to save video project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save video project"})
	}

	log.Printf("[VideoHistory] Created video project id=%s title=%s", msg.ID.String(), truncateStr(title, 60))

	return c.Status(201).JSON(fiber.Map{
		"id":   msg.ID,
		"data": msg,
	})
}

// UpdateVideoRecord updates an existing video project (shots, status, title).
// PATCH /api/v1/llm/video/history/:id
func UpdateVideoRecord(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	var msg models.Message
	if err := database.DB.Where("id = ? AND tenant_id = ? AND message_type = ?", id, tenantID, "ai_video").First(&msg).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Video project not found"})
	}

	var body struct {
		Title       *string                  `json:"title,omitempty"`
		Shots       []map[string]interface{} `json:"shots,omitempty"`
		Status      *string                  `json:"status,omitempty"`
		ChatHistory []map[string]interface{} `json:"chat_history,omitempty"`
		Storyboard  map[string]interface{}   `json:"storyboard,omitempty"`
	}
	if err := c.BodyParser(&body); err != nil {
		log.Printf("[VideoHistory] PATCH body parse error: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	log.Printf("[VideoHistory] PATCH id=%s: chatHistory=%d items, storyboard=%v, shots=%d",
		id, len(body.ChatHistory), body.Storyboard != nil, len(body.Shots))

	// Parse existing extra_fields
	existing := map[string]interface{}(msg.ExtraFields)

	if body.Shots != nil {
		existing["shots"] = body.Shots
	}

	if body.ChatHistory != nil {
		existing["chat_history"] = body.ChatHistory
	}

	if body.Storyboard != nil {
		existing["storyboard"] = body.Storyboard
	}

	if body.Status != nil {
		videoInfo, ok := existing["video_info"].(map[string]interface{})
		if !ok {
			videoInfo = map[string]interface{}{}
		}
		videoInfo["status"] = *body.Status
		existing["video_info"] = videoInfo
	}

	updates := map[string]interface{}{
		"extra_fields": models.JSONB(existing),
	}

	if body.Title != nil {
		updates["subject"] = *body.Title
		updates["content"] = *body.Title
	}

	if err := database.DB.Model(&msg).Updates(updates).Error; err != nil {
		log.Printf("[VideoHistory] Failed to update video project: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update video project"})
	}

	// Reload
	database.DB.Where("id = ?", msg.ID).First(&msg)

	return c.JSON(fiber.Map{
		"id":   msg.ID,
		"data": msg,
	})
}

// DeleteVideoRecord soft-deletes a video project by setting trashed_at.
// DELETE /api/v1/llm/video/history/:id
func DeleteVideoRecord(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	now := time.Now()
	result := database.DB.Model(&models.Message{}).
		Where("id = ? AND tenant_id = ? AND message_type = ?", id, tenantID, "ai_video").
		Update("trashed_at", now)

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": result.Error.Error()})
	}
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Video project not found"})
	}

	return c.JSON(fiber.Map{"message": "deleted"})
}

// ─── Video Chat (Multi-turn Conversation → Storyboard) ──────────

// videoChatMessage represents a single message in the video chat conversation.
type videoChatMessage struct {
	Role        string                   `json:"role"`                  // "user" or "assistant"
	Content     string                   `json:"content"`               // text content
	Attachments []map[string]interface{} `json:"attachments,omitempty"` // [{filename, mime_type, data (base64), file_url}]
}

// videoChatRequest is the request body for the multi-turn video chat.
type videoChatRequest struct {
	Message           string                   `json:"message"`               // Current user message
	History           []videoChatMessage       `json:"history,omitempty"`     // Previous conversation turns
	Attachments       []map[string]interface{} `json:"attachments,omitempty"` // Attachments for current message
	AspectRatio       string                   `json:"aspect_ratio,omitempty"`
	Duration          string                   `json:"duration,omitempty"`
	Language          string                   `json:"language,omitempty"`
	CurrentShots      []map[string]interface{} `json:"current_shots,omitempty"`      // Current project shots state
	CurrentStoryboard map[string]interface{}   `json:"current_storyboard,omitempty"` // Current storyboard (if any)
}

// langCodeToName maps BCP-47 locale codes to human-readable language names
// for use in LLM system prompts.
func langCodeToName(code string) string {
	m := map[string]string{
		"yue-HK": "廣東話 (Cantonese)",
		"zh-HK":  "繁體中文 (Traditional Chinese)",
		"zh-TW":  "繁體中文 (Traditional Chinese)",
		"zh-CN":  "简体中文 (Simplified Chinese)",
		"zh":     "中文 (Chinese)",
		"en-US":  "English",
		"en-GB":  "English",
		"en":     "English",
		"ja-JP":  "日本語 (Japanese)",
		"ja":     "日本語 (Japanese)",
		"ko-KR":  "한국어 (Korean)",
		"ko":     "한국어 (Korean)",
		"fr-FR":  "Français (French)",
		"fr":     "Français (French)",
		"de-DE":  "Deutsch (German)",
		"de":     "Deutsch (German)",
		"es-ES":  "Español (Spanish)",
		"es":     "Español (Spanish)",
		"pt-BR":  "Português (Portuguese)",
		"pt":     "Português (Portuguese)",
		"th-TH":  "ภาษาไทย (Thai)",
		"th":     "ภาษาไทย (Thai)",
		"vi-VN":  "Tiếng Việt (Vietnamese)",
		"vi":     "Tiếng Việt (Vietnamese)",
		"id-ID":  "Bahasa Indonesia (Indonesian)",
		"ms-MY":  "Bahasa Melayu (Malay)",
	}
	if name, ok := m[code]; ok {
		return name
	}
	// Try base language (e.g. "fr-CA" → "fr")
	if idx := strings.Index(code, "-"); idx > 0 {
		if name, ok := m[code[:idx]]; ok {
			return name
		}
	}
	return code // fallback: return the code itself
}

// langCodeToVoiceTag returns the voice/speech instruction tag for Kling prompts.
// Kling needs to know what language to generate audio in, so instead of generic
// "對話：" or "旁白：", we use language-specific tags like "用廣東話說：".
func langCodeToVoiceTag(code string) string {
	m := map[string]string{
		"yue-HK": "用廣東話說",
		"zh-HK":  "用廣東話說",
		"zh-TW":  "用國語說",
		"zh-CN":  "用普通话说",
		"zh":     "用中文說",
		"en-US":  "Speaking in English",
		"en-GB":  "Speaking in English",
		"en":     "Speaking in English",
		"ja-JP":  "日本語で話す",
		"ja":     "日本語で話す",
		"ko-KR":  "한국어로 말하기",
		"ko":     "한국어로 말하기",
		"fr-FR":  "Parlant en français",
		"fr":     "Parlant en français",
		"de-DE":  "Auf Deutsch sprechen",
		"de":     "Auf Deutsch sprechen",
		"es-ES":  "Hablando en español",
		"es":     "Hablando en español",
		"pt-BR":  "Falando em português",
		"pt":     "Falando em português",
		"th-TH":  "พูดเป็นภาษาไทย",
		"th":     "พูดเป็นภาษาไทย",
		"vi-VN":  "Nói bằng tiếng Việt",
		"vi":     "Nói bằng tiếng Việt",
		"id-ID":  "Berbicara dalam Bahasa Indonesia",
		"ms-MY":  "Bercakap dalam Bahasa Melayu",
	}
	if tag, ok := m[code]; ok {
		return tag
	}
	if idx := strings.Index(code, "-"); idx > 0 {
		if tag, ok := m[code[:idx]]; ok {
			return tag
		}
	}
	return "Speaking in " + code
}

// buildLangReinforcement returns a final prompt block written IN the target language
// to reinforce that the AI should respond in that language. Gemini tends to follow
// the language of the last text it reads in the system prompt, so adding a block
// in the user's language dramatically improves compliance.
func buildLangReinforcement(langName, langCode string) string {
	// Chinese-family reinforcement (Traditional Chinese, Simplified Chinese, Cantonese)
	if strings.HasPrefix(langCode, "zh") || strings.HasPrefix(langCode, "yue") {
		return fmt.Sprintf(`

【最終語言提醒 — 極度重要】
你必須使用%s回覆用戶。storyboard 的所有欄位（title、summary、narration、dialogue、description）全部必須用%s撰寫。
不要用英文寫任何欄位。系統會在送去影片生成引擎前自動翻譯 description 為英文。
錯誤示範：「"description": "Close-up of a man in his car"」← 這是錯的！
正確示範：「"description": "車內近景，一名男子正在看後視鏡，自然光線"」← 這才是對的！
錯誤示範：「"title": "Quick Hair Styling On-The-Go"」← 這是錯的！
正確示範：「"title": "車內快速吹頭造型示範"」← 這才是對的！`, langName, langName)
	}

	// Japanese
	if strings.HasPrefix(langCode, "ja") {
		return fmt.Sprintf(`

【最終言語リマインダー — 非常に重要】
ユーザーには必ず%sで返答してください。storyboardの全フィールド（title、summary、narration、dialogue、description）はすべて%sで記述してください。
英語を使わないでください。システムがdescriptionを自動的に英語に翻訳します。`, langName, langName)
	}

	// Korean
	if strings.HasPrefix(langCode, "ko") {
		return fmt.Sprintf(`

【최종 언어 알림 — 매우 중요】
사용자에게 반드시 %s로 응답하세요. storyboard의 모든 필드(title, summary, narration, dialogue, description)는 모두 %s로 작성해야 합니다.
영어를 사용하지 마세요. 시스템이 description을 자동으로 영어로 번역합니다.`, langName, langName)
	}

	// For non-CJK languages, add a generic English reinforcement
	// (the original prompt already covers this, but repeat for safety)
	if langCode != "en-US" && langCode != "en-GB" && langCode != "en" {
		return fmt.Sprintf(`

FINAL LANGUAGE REMINDER (EXTREMELY IMPORTANT):
You MUST respond in %s. ALL storyboard fields including title, summary, narration, dialogue, AND description MUST be in %s.
Do NOT use English for any field. The system will translate description to English automatically before video generation.`, langName, langName)
	}

	return "" // English — no extra reinforcement needed
}

// buildStoryboardExampleJSON returns a JSON example block with values written
// in the target language. This is the most effective way to get Gemini to output
// non-English content — showing, not just telling.
func buildStoryboardExampleJSON(langCode, duration, voiceLocale string) string {
	vTag := langCodeToVoiceTag(voiceLocale)

	// Chinese (Traditional/Simplified/Cantonese)
	if strings.HasPrefix(langCode, "zh") || strings.HasPrefix(langCode, "yue") {
		return fmt.Sprintf(`{"type": "storyboard", "storyboard": {
  "title": "車內快速造型示範",
  "summary": "示範如何在車裡用噴髮膠快速做出時尚髮型，適合趕時間的都市人。",
  "characters": [
    {"name": "阿傑", "description": "28歲亞洲男性，短黑髮側分，淺棕色皮膚，中等身材，穿白色圓領T恤搭配深藍牛仔褲，五官端正，下巴線條分明"}
  ],
  "shots": [
    {
      "shot_number": 1,
      "type": "scene",
      "description": "車內近景，一名28歲亞洲男性短黑髮側分穿白色圓領T恤的男子坐在駕駛座，手持噴髮膠，看著後視鏡整理頭髮，自然日光透過車窗照進來。輕快節奏的背景音樂。%s：今天教大家如何在車裡快速打理頭髮。",
      "duration": "%s"
    },
    {
      "shot_number": 2,
      "type": "scene",
      "description": "中景，同一名28歲亞洲男性短黑髮側分穿白色圓領T恤的男子噴髮膠後用手指撥弄頭髮造型，動態鏡頭運動，柔和光線。輕鬆愉快的背景音樂。%s：只需要三十秒，輕鬆搞定出門造型。",
      "duration": "%s"
    }
  ],
  "total_length": "10s"
}}`, vTag, duration, vTag, duration)
	}

	// Japanese
	if strings.HasPrefix(langCode, "ja") {
		return fmt.Sprintf(`{"type": "storyboard", "storyboard": {
  "title": "車内クイックヘアスタイリング",
  "summary": "車の中でヘアスプレーを使って素早くスタイリングする方法のデモンストレーション。",
  "characters": [
    {"name": "太郎", "description": "28歳の日本人男性、短い黒髪を横分け、色白、中肉中背、白いクルーネックTシャツにダークブルーのジーンズ、すっきりとした顔立ち"}
  ],
  "shots": [
    {
      "shot_number": 1,
      "type": "scene",
      "description": "車内のクローズアップ、28歳の短い黒髪横分けの白いTシャツを着た日本人男性がヘアスプレーを持ちながらミラーを調整、自然光。軽快なBGM。%s：今日は車内で素早く髪を整える方法をお見せします。",
      "duration": "%s"
    }
  ],
  "total_length": "%s"
}}`, vTag, duration, duration)
	}

	// Default / English
	return fmt.Sprintf(`{"type": "storyboard", "storyboard": {
  "title": "Quick Hair Styling On-The-Go",
  "summary": "A demonstration of how to quickly style your hair in the car using hairspray.",
  "characters": [
    {"name": "Jake", "description": "28-year-old Caucasian male, short brown hair parted to the side, light skin, medium build, wearing a white crew-neck t-shirt and dark blue jeans, clean-shaven with a defined jawline"}
  ],
  "shots": [
    {
      "shot_number": 1,
      "type": "scene",
      "description": "Close-up of a 28-year-old Caucasian male with short brown hair parted to the side wearing a white crew-neck t-shirt sitting in a car, holding a hairspray can, adjusting rearview mirror, natural daylight. Upbeat cheerful background music. %s: Today we show you how to quickly style your hair on the go.",
      "duration": "%s"
    }
  ],
  "total_length": "%s"
}}`, vTag, duration, duration)
}

// translatePromptToEnglish uses Gemini to translate a video generation prompt
// to English for Kling 3.0 Omni. If the prompt is already in English or translation
// fails, returns the original prompt unchanged.
//
// IMPORTANT: Voice/dialogue lines (e.g. "用廣東話說：歡迎嚟到我哋店") must NOT be translated —
// Kling needs the original language text to generate correct speech audio. This function
// extracts dialogue lines, translates only the visual description, then re-appends the
// original dialogue text unchanged.
func translatePromptToEnglish(apiKey, prompt string) string {
	// Known voice tag prefixes (from langCodeToVoiceTag + legacy tags)
	voicePrefixes := []string{
		"用廣東話說", "用國語說", "用普通话说", "用中文說",
		"Speaking in English", "Speaking in Cantonese", "Speaking in Mandarin",
		"日本語で話す", "한국어로 말하기",
		"Parlant en français", "Auf Deutsch sprechen", "Hablando en español",
		"Falando em português", "พูดเป็นภาษาไทย", "Nói bằng tiếng Việt",
		"Berbicara dalam Bahasa Indonesia", "Bercakap dalam Bahasa Melayu",
		"Speaking in",
		// Legacy tags
		"旁白：", "旁白:", "對話：", "對話:", "Narration:", "Dialogue:",
	}

	// Split prompt into visual description and voice/dialogue parts.
	// Voice lines typically appear after the visual description, separated by ". " or newline.
	// Pattern: "visual description. 用廣東話說：dialogue text"
	var visualPart string
	var voiceLines []string

	// Split by periods followed by voice tags, or by newlines
	lines := splitPromptLines(prompt)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isVoice := false
		for _, prefix := range voicePrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				isVoice = true
				break
			}
		}
		if isVoice {
			voiceLines = append(voiceLines, trimmed)
		} else {
			if visualPart != "" {
				visualPart += ". "
			}
			visualPart += trimmed
		}
	}

	// If there's no visual part to translate (only voice lines), return as-is
	if strings.TrimSpace(visualPart) == "" {
		return prompt
	}

	// Quick check: if the visual part appears to be mostly ASCII/English, skip translation
	nonASCII := 0
	for _, r := range visualPart {
		if r > 127 {
			nonASCII++
		}
	}
	// If less than 10% non-ASCII characters, assume English — no translation needed
	if len(visualPart) > 0 && float64(nonASCII)/float64(len([]rune(visualPart))) < 0.1 {
		// Visual part is already English; just re-combine with voice lines
		if len(voiceLines) > 0 {
			return visualPart + ". " + strings.Join(voiceLines, ". ")
		}
		return visualPart
	}

	if apiKey == "" {
		return prompt
	}

	model := "gemini-2.0-flash"
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

	geminiReq := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": fmt.Sprintf(`Translate the following video generation prompt to English. This will be used as input for an AI video generation model (Kling 3.0 Omni), so keep it as a vivid, detailed visual description with camera angles, lighting, and composition details.

Output ONLY the translated text, nothing else.

Prompt to translate:
%s`, visualPart)},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.2,
		},
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		log.Printf("[TranslatePrompt] Failed to marshal request: %v", err)
		return prompt
	}

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("[TranslatePrompt] Failed to create request: %v", err)
		return prompt
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[TranslatePrompt] API call failed: %v", err)
		return prompt
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		log.Printf("[TranslatePrompt] API error %d: %s", resp.StatusCode, truncateStr(string(body), 200))
		return prompt
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		log.Printf("[TranslatePrompt] Failed to parse response: %v", err)
		return prompt
	}

	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		translated := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
		if translated != "" {
			// Re-combine translated visual description with original voice lines
			if len(voiceLines) > 0 {
				translated = translated + ". " + strings.Join(voiceLines, ". ")
			}
			log.Printf("[TranslatePrompt] Translated (visual only): %s → %s", truncateStr(prompt, 80), truncateStr(translated, 80))
			return translated
		}
	}

	return prompt
}

// splitPromptLines splits a prompt into segments by newlines and by ". " before voice tags.
// This handles cases like: "視覺描述. 用廣東話說：對話文字" as well as multi-line prompts.
func splitPromptLines(prompt string) []string {
	// Known voice tag starts (must match voicePrefixes in translatePromptToEnglish)
	voiceStarts := []string{
		"用廣東話說", "用國語說", "用普通话说", "用中文說",
		"Speaking in", "日本語で話す", "한국어로 말하기",
		"Parlant en", "Auf Deutsch", "Hablando en",
		"Falando em", "พูดเป็นภาษาไทย", "Nói bằng",
		"Berbicara dalam", "Bercakap dalam",
		"旁白：", "旁白:", "對話：", "對話:", "Narration:", "Dialogue:",
	}

	var result []string

	// First split by newlines
	rawLines := strings.Split(prompt, "\n")
	for _, rawLine := range rawLines {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}
		// Within each line, look for ". VoiceTag" pattern and split there
		split := false
		for _, vs := range voiceStarts {
			// Look for ". VoiceTag" or ", VoiceTag"
			for _, sep := range []string{". ", ", "} {
				idx := strings.Index(rawLine, sep+vs)
				if idx >= 0 {
					before := strings.TrimSpace(rawLine[:idx])
					after := strings.TrimSpace(rawLine[idx+len(sep):])
					if before != "" {
						result = append(result, before)
					}
					if after != "" {
						result = append(result, after)
					}
					split = true
					break
				}
			}
			if split {
				break
			}
		}
		if !split {
			result = append(result, rawLine)
		}
	}

	return result
}

// resolveAttachmentBase64 resolves the base64 data for an attachment.
// If "data" is already base64, it is returned as-is (stripping any data-URL prefix).
// If "data" is empty but "file_url" is a local path (e.g. /uploads/... or /static/...),
// the file is read from disk under "web/" and base64-encoded.
func resolveAttachmentBase64(att map[string]interface{}) string {
	data, _ := att["data"].(string)

	// Strip data-URL prefix if present (e.g. "data:image/jpeg;base64,/9j/4AA...")
	if strings.HasPrefix(data, "data:") {
		if idx := strings.Index(data, ","); idx >= 0 {
			data = data[idx+1:]
		}
	}

	// If data looks like a URL path instead of base64, it's a bug — treat as empty
	if strings.HasPrefix(data, "/") {
		// This was the old bug: frontend sent file path as data
		// Try to resolve it from disk instead
		fileURL := data
		data = ""
		if att["file_url"] == nil || att["file_url"] == "" {
			att["file_url"] = fileURL
		}
	}

	// If we have valid base64 data, return it
	if data != "" {
		return data
	}

	// Fall back: read from file_url on disk
	fileURL, _ := att["file_url"].(string)
	if fileURL == "" {
		return ""
	}

	// Resolve to disk path: /uploads/... → web/uploads/..., /static/... → web/static/...
	diskPath := filepath.Join("web", filepath.FromSlash(fileURL))
	fileBytes, err := os.ReadFile(diskPath)
	if err != nil {
		log.Printf("[WARN] resolveAttachmentBase64: cannot read %s: %v", diskPath, err)
		return ""
	}
	return base64.StdEncoding.EncodeToString(fileBytes)
}

// VideoChatMessage handles multi-turn conversation for video planning.
// The AI will ask follow-up questions if it needs more information (e.g. product images,
// style preferences, target audience) before generating the final storyboard.
// POST /api/v1/llm/video/chat
func VideoChatMessage(c *fiber.Ctx) error {
	log.Println("[VideoChat] v4.1 handler invoked (no JSON mode, temp=0.4)")
	cfg := config.Load()
	if cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "LLM API Key 未配置"})
	}

	var req videoChatRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		return c.Status(400).JSON(fiber.Map{"error": "訊息為必填"})
	}

	aspectRatio := strings.TrimSpace(req.AspectRatio)
	if aspectRatio == "" {
		aspectRatio = "9:16"
	}

	duration := strings.TrimSpace(req.Duration)
	if duration == "" {
		duration = "10s"
	}

	lang := strings.TrimSpace(req.Language)
	if lang == "" {
		lang = "zh-TW"
	}

	// Map BCP-47 locale codes to human-readable language names for the prompt
	langName := langCodeToName(lang)
	voiceTag := langCodeToVoiceTag(lang)

	// Build system prompt for multi-turn video planning
	systemPrompt := fmt.Sprintf(`你是 vAi，一個專業的影片製作助手。你的唯一職責是幫助用戶規劃和製作影片。

語言規則：
- 用戶說中文，你就用中文回覆。用戶說英文，你就用英文回覆。永遠跟隨用戶的語言。
- 分鏡表（storyboard）的所有欄位都必須用 %s 撰寫，包括 title、summary、description。
- description 欄位用 %s 撰寫即可，系統會自動翻譯成英文再送去影片生成引擎。

你只做三件事：
1. 用戶描述影片想法 → 先判斷是否有足夠的關鍵資訊來生成分鏡表：
   - 關鍵資訊（缺少任何一項就必須先問）：影片的具體主題或主角是什麼（品牌名稱、產品名稱、公司名稱、人物、具體事物）
   - 如果用戶的描述缺少關鍵資訊（例如只說「做一個品牌宣傳片」但沒說是什麼品牌），用 {"type": "message"} 格式問一個簡短的問題來釐清
   - 如果關鍵資訊已經足夠明確，直接生成分鏡表。非關鍵的細節（鏡頭角度、燈光、轉場等）可以用合理的創意預設值填補，不需要問
   - 絕對不要自己編造品牌名稱、產品名稱、公司名稱、人名等具體名詞
2. 用戶對已有的分鏡表提出修改意見 → 根據意見重新生成一份完整的分鏡表
3. 用戶的訊息跟影片製作無關（閒聊、問問題、離題） → 禮貌提醒這裡是影片生成工具，請描述想要製作的影片內容

回覆格式 — 只輸出 JSON，不要加 markdown：

提醒用戶時：
{"type": "message", "content": "這裡是 vAi 影片製作助手，請告訴我你想製作什麼樣的影片，我會幫你規劃分鏡 🎬"}

需要問用戶問題時（最多問一個問題）：
{"type": "message", "content": "你的問題..."}

生成分鏡表時，完整範例：
%s

影片引擎：Kling 3.0 Omni（原生音訊，支援旁白、對話、環境音效，無需額外 TTS 或配樂）

分鏡規則：
- 每個分鏡的 "type" 一律為 "scene"
- "description"：用 %s 詳細描述鏡頭畫面，包括鏡頭角度、光線、運動
- 絕對不要在 description 中寫畫面比例（如 9:16、16:9、直式構圖、橫式構圖等），比例已由系統自動帶入參數
- 如果鏡頭需要語音（旁白或對話），在 description 末尾加上「%s：xxx」，Kling 會自動用該語言生成語音。絕對不要用「旁白：」或「對話：」，一律用「%s：」
- 時長："5s"、"10s" 或 "15s"
- 轉場："cut"、"fade" 或 "dissolve"
- 最多 6 個分鏡，總時長不超過 15 秒。
- 每個分鏡從前一個鏡頭的最後一幀延續，描述要自然銜接

人物一致性規則（非常重要）：
- 如果影片中有人物角色出現，在 storyboard 中加入 "characters" 陣列（和 "title"、"summary"、"shots" 同級）
- 每個人物包含 "name"（名字）和 "description"（極其詳細的外觀描述）
- description 必須包含：性別、年齡範圍、髮型髮色、膚色、體型、穿著服裝的詳細描述（顏色、款式、材質）、面部特徵
- 例如：{"name": "小明", "description": "25歲亞洲男性，短黑髮偏分，淺棕色皮膚，中等身材，穿白色圓領T恤搭配深藍牛仔褲，戴黑框眼鏡，五官端正"}
- 每個 shot 的 description 中提到人物時，必須完整重複該人物的外觀描述（不要只寫名字），這樣每個鏡頭才能保持人物一致
- 人物數量不限，但要確保 description 夠詳細`,
		langName, langName, // 語言規則
		buildStoryboardExampleJSON(lang, duration, lang), // 完整 JSON 範例
		langName,           // description language
		voiceTag, voiceTag) // voice/speech tags

	// Append a final language reinforcement block IN THE USER'S LANGUAGE.
	systemPrompt += buildLangReinforcement(langName, lang)

	// Inject current storyboard/shots context so AI can modify or re-plan
	if len(req.CurrentShots) > 0 || len(req.CurrentStoryboard) > 0 {
		systemPrompt += "\n\n--- 目前專案的分鏡狀態 ---\n"
		systemPrompt += "用戶目前已有以下分鏡（shots）。當用戶要求修改分鏡時，你應該基於這些現有分鏡進行修改，而不是完全重新規劃。\n"
		systemPrompt += "如果用戶明確要求「重新規劃」或「全部重做」，才生成全新的分鏡表。\n"

		if len(req.CurrentStoryboard) > 0 {
			sbJSON, _ := json.Marshal(req.CurrentStoryboard)
			systemPrompt += "\n目前的分鏡表（storyboard）：\n" + string(sbJSON) + "\n"
		}

		if len(req.CurrentShots) > 0 {
			shotsJSON, _ := json.Marshal(req.CurrentShots)
			systemPrompt += "\n目前的分鏡狀態：\n" + string(shotsJSON) + "\n"
			systemPrompt += "注意：status='done' 且 has_video=true 的分鏡已經生成了影片，修改時應盡量保留這些分鏡的 description。\n"
		}
	}

	// Build Gemini conversation from history
	var contents []map[string]interface{}

	for _, msg := range req.History {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}

		parts := []map[string]interface{}{
			{"text": msg.Content},
		}

		// Add image attachments as inlineData for Gemini
		for _, att := range msg.Attachments {
			mimeType, _ := att["mime_type"].(string)
			if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
				continue
			}
			b64 := resolveAttachmentBase64(att)
			if b64 == "" {
				continue
			}
			parts = append(parts, map[string]interface{}{
				"inlineData": map[string]interface{}{
					"mimeType": mimeType,
					"data":     b64,
				},
			})
		}

		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": parts,
		})
	}

	// Add current user message with inline language reminder for non-English
	// Gemini pays most attention to the last user turn, so embedding the reminder
	// here is more effective than only having it in the system prompt.
	userText := message
	if lang != "en-US" && lang != "en-GB" && lang != "en" {
		if strings.HasPrefix(lang, "zh") || strings.HasPrefix(lang, "yue") {
			userText = message + "\n\n（重要：你的回覆必須用中文。JSON 裡所有欄位都必須用中文，包括 description。不要用英文寫任何欄位。回覆時只輸出 JSON，不要加 markdown。）"
		} else if strings.HasPrefix(lang, "ja") {
			userText = message + "\n\n（重要：返答は日本語で。JSONの全フィールド（description含む）は日本語で記述。JSONのみ出力、markdownなし。）"
		} else {
			userText = message + fmt.Sprintf("\n\n(IMPORTANT: Respond in %s. ALL JSON fields including description must be in %s. Output JSON only, no markdown.)", langName, langName)
		}
	} else {
		userText = message + "\n\n(Output JSON only, no markdown fences.)"
	}
	currentParts := []map[string]interface{}{
		{"text": userText},
	}

	// Add current message attachments
	for _, att := range req.Attachments {
		mimeType, _ := att["mime_type"].(string)
		if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
			continue
		}
		b64 := resolveAttachmentBase64(att)
		if b64 == "" {
			continue
		}
		currentParts = append(currentParts, map[string]interface{}{
			"inlineData": map[string]interface{}{
				"mimeType": mimeType,
				"data":     b64,
			},
		})
	}

	contents = append(contents, map[string]interface{}{
		"role":  "user",
		"parts": currentParts,
	})

	// Call Gemini API
	model := cfg.LLM.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

	// NOTE: We intentionally do NOT set responseMimeType: "application/json" because
	// Gemini's JSON mode has a strong English bias — it ignores language instructions
	// and outputs English string values. Instead we ask for JSON in the prompt and
	// strip any markdown fences from the response (already handled below).
	geminiReq := map[string]interface{}{
		"contents": contents,
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemPrompt},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.4,
		},
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to build request"})
	}

	log.Printf("[VideoChat] Sending message: msg=%s, history=%d turns, attachments=%d, lang=%s (name=%s)",
		truncateStr(message, 80), len(req.History), len(req.Attachments), lang, langName)

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create request"})
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[VideoChat] Gemini API call failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "AI API 呼叫失敗: " + err.Error()})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		log.Printf("[VideoChat] Gemini API error %d: %s", resp.StatusCode, string(body))
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": "AI API 錯誤: " + string(body)})
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
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		log.Printf("[VideoChat] Failed to parse Gemini response: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse AI response"})
	}

	var rawText string
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		rawText = geminiResp.Candidates[0].Content.Parts[0].Text
	}
	if rawText == "" {
		return c.Status(500).JSON(fiber.Map{"error": "AI 未返回有效內容"})
	}

	log.Printf("[VideoChat] RAW Gemini response (first 500 chars): %s", truncateStr(rawText, 500))

	// Strip markdown fences if present
	rawText = strings.TrimSpace(rawText)
	if strings.HasPrefix(rawText, "```json") {
		rawText = strings.TrimPrefix(rawText, "```json")
		rawText = strings.TrimSuffix(rawText, "```")
		rawText = strings.TrimSpace(rawText)
	} else if strings.HasPrefix(rawText, "```") {
		rawText = strings.TrimPrefix(rawText, "```")
		rawText = strings.TrimSuffix(rawText, "```")
		rawText = strings.TrimSpace(rawText)
	}

	// Parse the response JSON to determine type
	var chatResp map[string]interface{}
	if err := json.Unmarshal([]byte(rawText), &chatResp); err != nil {
		log.Printf("[VideoChat] Failed to parse response JSON: %v, raw: %s", err, truncateStr(rawText, 500))
		// Fallback: treat as a plain message
		return c.JSON(fiber.Map{
			"type":    "message",
			"content": rawText,
		})
	}

	respType, _ := chatResp["type"].(string)

	if respType == "storyboard" {
		// Parse and validate the storyboard
		storyboardRaw, ok := chatResp["storyboard"]
		if !ok {
			return c.JSON(fiber.Map{
				"type":    "message",
				"content": rawText,
			})
		}

		// Re-marshal and parse storyboard for validation
		sbBytes, _ := json.Marshal(storyboardRaw)
		var storyboard storyboardResponse
		if err := json.Unmarshal(sbBytes, &storyboard); err != nil {
			log.Printf("[VideoChat] Failed to parse storyboard from chat: %v", err)
			return c.JSON(fiber.Map{
				"type":    "message",
				"content": rawText,
			})
		}

		// Validate and fix shots
		for i := range storyboard.Shots {
			storyboard.Shots[i].ShotNumber = i + 1
			if storyboard.Shots[i].Duration == "" {
				storyboard.Shots[i].Duration = duration
			}
			if storyboard.Shots[i].Type == "" {
				storyboard.Shots[i].Type = "scene" // default to scene
			}
		}

		// ── Generate character reference images via Gemini ──
		// For each character that the AI defined, we generate a portrait image so
		// Kling can use it as a reference image for visual consistency across shots.
		if len(storyboard.Characters) > 0 {
			imageModel := cfg.LLM.ImageModel
			if imageModel == "" {
				imageModel = "gemini-3-pro-image-preview"
			}
			log.Printf("[VideoChat] Generating reference images for %d character(s)...", len(storyboard.Characters))
			for i := range storyboard.Characters {
				ch := &storyboard.Characters[i]
				if ch.Description == "" {
					log.Printf("[VideoChat] Skipping character %q: no description", ch.Name)
					continue
				}
				portraitPrompt := fmt.Sprintf(
					"A single portrait photo of a character named %q. Full appearance: %s. "+
						"Clean background, centered composition, studio lighting, high detail, photorealistic.",
					ch.Name, ch.Description,
				)
				dataURLs, err := callGeminiImageGeneration(cfg.LLM.APIKey, imageModel, portraitPrompt, "1:1")
				if err != nil {
					log.Printf("[VideoChat] WARNING: Failed to generate image for character %q: %v", ch.Name, err)
					continue // graceful degradation — proceed without image
				}
				if len(dataURLs) > 0 {
					ch.ImageURL = dataURLs[0]
					log.Printf("[VideoChat] Generated reference image for character %q (%d bytes)", ch.Name, len(dataURLs[0]))
				}
			}
		}

		// Collect all image attachments from conversation as reference images for Veo
		// These will be used as image-to-video first frames so Veo knows what products look like
		var referenceImages []map[string]interface{}
		seen := map[string]bool{}
		collectRef := func(att map[string]interface{}) {
			mimeType, _ := att["mime_type"].(string)
			if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
				return
			}
			fileURL, _ := att["file_url"].(string)
			filename, _ := att["filename"].(string)
			// Need either file_url or base64 data
			b64 := resolveAttachmentBase64(att)
			if b64 == "" && fileURL == "" {
				return
			}
			key := fileURL
			if key == "" {
				key = filename
			}
			if key != "" && seen[key] {
				return
			}
			if key != "" {
				seen[key] = true
			}
			ref := map[string]interface{}{
				"mime_type": mimeType,
				"filename":  filename,
			}
			if fileURL != "" {
				ref["file_url"] = fileURL
			}
			if b64 != "" {
				ref["data"] = b64
			}
			referenceImages = append(referenceImages, ref)
		}
		for _, msg := range req.History {
			for _, att := range msg.Attachments {
				collectRef(att)
			}
		}
		for _, att := range req.Attachments {
			collectRef(att)
		}

		log.Printf("[VideoChat] Generated storyboard: title=%s, shots=%d, refImages=%d",
			truncateStr(storyboard.Title, 60), len(storyboard.Shots), len(referenceImages))

		// ── Generate scene reference image for the first shot ──
		// If user provided image attachments (e.g. a product photo), compose a scene
		// image using Gemini: place the user's subject into the first shot's described
		// scene. This gives Kling a strong first-frame reference for image-to-video.
		var firstShotRefImage string
		if len(referenceImages) > 0 && len(storyboard.Shots) > 0 {
			// Get the first user image's base64 data
			refImg := referenceImages[0]
			refB64 := ""
			refMime := ""
			if d, ok := refImg["data"].(string); ok && d != "" {
				refB64 = d
			}
			if m, ok := refImg["mime_type"].(string); ok {
				refMime = m
			}

			if refB64 != "" {
				// Strip data URL prefix if present (callGeminiImageEdit expects raw base64)
				rawB64 := refB64
				if idx := strings.Index(rawB64, ","); idx >= 0 {
					rawB64 = rawB64[idx+1:]
				}

				firstDesc := storyboard.Shots[0].Description
				scenePrompt := fmt.Sprintf(
					"Place the subject from the reference image into this scene. "+
						"Scene description: %s. "+
						"Keep the subject's appearance exactly as shown in the reference image. "+
						"Photorealistic, high quality, cinematic composition.",
					firstDesc,
				)

				imageModel := cfg.LLM.ImageModel
				if imageModel == "" {
					imageModel = "gemini-3-pro-image-preview"
				}

				log.Printf("[VideoChat] Generating scene reference image for first shot...")
				dataURLs, err := callGeminiImageEdit(cfg.LLM.APIKey, imageModel, scenePrompt, aspectRatio, rawB64, refMime)
				if err != nil {
					log.Printf("[VideoChat] WARNING: Failed to generate scene ref image: %v", err)
				} else if len(dataURLs) > 0 {
					firstShotRefImage = dataURLs[0]
					log.Printf("[VideoChat] Generated scene reference image for first shot (%d bytes)", len(firstShotRefImage))
				}
			}
		}

		return c.JSON(fiber.Map{
			"type":                 "storyboard",
			"storyboard":           storyboard,
			"aspect_ratio":         aspectRatio,
			"duration":             duration,
			"reference_images":     referenceImages,
			"first_shot_ref_image": firstShotRefImage,
		})
	}

	// Default: it's a follow-up message
	content, _ := chatResp["content"].(string)
	if content == "" {
		content = rawText
	}

	log.Printf("[VideoChat] AI follow-up: %s", truncateStr(content, 120))

	return c.JSON(fiber.Map{
		"type":    "message",
		"content": content,
	})
}

// ─── Storyboard Generation (Legacy one-shot, kept for backward compat) ──

// storyboardRequest is the request body for AI storyboard generation.
type storyboardRequest struct {
	Description string `json:"description"`            // User's natural-language video idea
	NumShots    int    `json:"num_shots,omitempty"`    // Desired number of shots (default 3-6)
	AspectRatio string `json:"aspect_ratio,omitempty"` // "16:9", "9:16", "1:1"
	Duration    string `json:"duration,omitempty"`     // Per-shot duration "5s" or "10s"
	Language    string `json:"language,omitempty"`     // "zh-TW", "en", etc. for prompt output
}

// storyboardShot is a single shot in the AI-generated storyboard.
type storyboardShot struct {
	ShotNumber  int    `json:"shot_number"`
	Type        string `json:"type"`        // Always "scene" (Kling handles all audio natively)
	Description string `json:"description"` // Visual description + optional narration (e.g. "...旁白：xxx")
	Duration    string `json:"duration"`    // "5s", "10s", or "15s" (Kling 3.0 Omni)
}

// storyboardCharacter describes a main character for visual consistency across shots.
type storyboardCharacter struct {
	Name        string `json:"name"`                // Character name/label
	Description string `json:"description"`         // Detailed visual appearance description
	ImageURL    string `json:"image_url,omitempty"` // Generated reference image data URL (filled by backend)
}

// storyboardResponse is the AI-generated storyboard.
type storyboardResponse struct {
	Title       string                `json:"title"`
	Summary     string                `json:"summary"`
	Characters  []storyboardCharacter `json:"characters,omitempty"`
	Shots       []storyboardShot      `json:"shots"`
	TotalLength string                `json:"total_length"` // e.g. "15s"
}

// GenerateStoryboard uses Gemini to break down a user's video idea into shots.
// POST /api/v1/llm/video/storyboard
func GenerateStoryboard(c *fiber.Ctx) error {
	cfg := config.Load()
	if cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "LLM API Key 未配置"})
	}

	var req storyboardRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	description := strings.TrimSpace(req.Description)
	if description == "" {
		return c.Status(400).JSON(fiber.Map{"error": "描述為必填"})
	}

	// Defaults
	numShots := req.NumShots
	if numShots < 2 {
		numShots = 3
	}
	if numShots > 6 {
		numShots = 6 // Kling 3.0 Omni max 6 shots per request
	}

	aspectRatio := strings.TrimSpace(req.AspectRatio)
	if aspectRatio == "" {
		aspectRatio = "9:16"
	}

	duration := strings.TrimSpace(req.Duration)
	if duration == "" {
		duration = "10s"
	}

	lang := strings.TrimSpace(req.Language)
	if lang == "" {
		lang = "zh-TW"
	}
	langName := langCodeToName(lang)
	voiceTag := langCodeToVoiceTag(lang)

	// Build Gemini prompt for storyboard generation
	systemPrompt := fmt.Sprintf(`You are a professional video storyboard creator. Given a user's video concept, you break it down into individual shots suitable for AI video generation (Kling 3.0 Omni with native audio).

CRITICAL LANGUAGE RULE:
- ALL storyboard fields MUST be in %s — including title, summary, AND description.
- The "description" field is the visual description of the shot. Write it in %s so the user can read and edit it. The system will translate it to English automatically before sending to the video generation engine.
- If the user's concept is in Chinese, ALL fields MUST be Chinese. Do NOT write any field in English.

RULES:
1. Output ONLY valid JSON, no markdown fences, no extra text.
2. Each shot "description": vivid visual description in %s (camera angle, lighting, motion, subject details).
3. If a shot needs narration or dialogue, embed it at the end of "description" as "%s: xxx" — this tells Kling which language to generate audio in. Do NOT use generic "旁白：" or "對話：" or "Narration:" — always use "%s: xxx".
4. Duration: "5s", "10s", or "15s". Do NOT include a "transition" field — Kling handles transitions natively.
5. Every shot "type" is always "scene".
6. Max 6 shots, total duration max 15 seconds.
7. BACKGROUND MUSIC: If the user's concept mentions background music, soundtrack, or any music style, then EVERY shot description MUST include a matching background music/sound description (e.g. "輕鬆愉快的背景音樂", "柔和鋼琴旋律", "energetic upbeat music"). Place the music cue before any voice tag line. This ensures the AI video engine generates consistent background music across all shots.

COMPLETE EXAMPLE (notice ALL fields are in %s):
%s`,
		langName, langName,
		langName,
		voiceTag, voiceTag,
		langName,
		buildStoryboardExampleJSON(lang, duration, lang),
	)

	// Add language reinforcement in the user's own language
	systemPrompt += buildLangReinforcement(langName, lang)

	userPrompt := fmt.Sprintf(`Create a video storyboard with %d shots.

Video concept: %s

Aspect ratio: %s
Per-shot duration preference: %s

REMEMBER: ALL fields (title, summary, description) must be in %s.
Output JSON only, no markdown fences.`, numShots, description, aspectRatio, duration, langName)

	// For Chinese, add explicit inline language reminder in the user prompt
	if strings.HasPrefix(lang, "zh") || strings.HasPrefix(lang, "yue") {
		userPrompt += "\n\n（重要提醒：所有欄位都必須用中文撰寫，包括 description。不要用英文寫任何欄位。）"
	}

	// Call Gemini API
	model := cfg.LLM.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

	// NOTE: We intentionally do NOT set responseMimeType: "application/json" because
	// Gemini's JSON mode has a strong English bias — it outputs English string values
	// even when asked for Chinese. We handle JSON parsing and markdown fence stripping below.
	geminiReq := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": userPrompt},
				},
			},
		},
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": systemPrompt},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature": 0.4,
		},
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to build request"})
	}

	log.Printf("[Storyboard] Generating storyboard: desc=%s, shots=%d, ratio=%s, lang=%s",
		truncateStr(description, 80), numShots, aspectRatio, lang)

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create request"})
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[Storyboard] Gemini API call failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "AI API 呼叫失敗: " + err.Error()})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		log.Printf("[Storyboard] Gemini API error %d: %s", resp.StatusCode, string(body))
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": "AI API 錯誤: " + string(body)})
	}

	// Parse Gemini response → extract text → parse as storyboard JSON
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		log.Printf("[Storyboard] Failed to parse Gemini response: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse AI response"})
	}

	// Extract text from first candidate
	var rawText string
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		rawText = geminiResp.Candidates[0].Content.Parts[0].Text
	}
	if rawText == "" {
		log.Printf("[Storyboard] Empty response from Gemini")
		return c.Status(500).JSON(fiber.Map{"error": "AI 未返回有效內容"})
	}

	// Strip markdown code fences if present
	rawText = strings.TrimSpace(rawText)
	if strings.HasPrefix(rawText, "```json") {
		rawText = strings.TrimPrefix(rawText, "```json")
		rawText = strings.TrimSuffix(rawText, "```")
		rawText = strings.TrimSpace(rawText)
	} else if strings.HasPrefix(rawText, "```") {
		rawText = strings.TrimPrefix(rawText, "```")
		rawText = strings.TrimSuffix(rawText, "```")
		rawText = strings.TrimSpace(rawText)
	}

	// Parse storyboard JSON
	var storyboard storyboardResponse
	if err := json.Unmarshal([]byte(rawText), &storyboard); err != nil {
		log.Printf("[Storyboard] Failed to parse storyboard JSON: %v, raw: %s", err, truncateStr(rawText, 500))
		return c.Status(500).JSON(fiber.Map{
			"error":    "AI 回應格式錯誤",
			"raw_text": rawText,
		})
	}

	// Validate and fix shots
	for i := range storyboard.Shots {
		storyboard.Shots[i].ShotNumber = i + 1
		if storyboard.Shots[i].Duration == "" {
			storyboard.Shots[i].Duration = duration
		}
		if storyboard.Shots[i].Type == "" {
			storyboard.Shots[i].Type = "scene"
		}
	}

	log.Printf("[Storyboard] Generated storyboard: title=%s, shots=%d",
		truncateStr(storyboard.Title, 60), len(storyboard.Shots))

	return c.JSON(fiber.Map{
		"storyboard":   storyboard,
		"aspect_ratio": aspectRatio,
		"duration":     duration,
	})
}

// MarkChatVideoDeleted marks a chat-generated video as deleted by setting
// extra_fields->'video_info'->'deleted' = true on the chat message.
// This does NOT soft-delete the message itself (so it still appears in chat
// with a "相關記錄已刪除" indicator).
// PATCH /api/v1/llm/video/history/:id/mark-deleted
func MarkChatVideoDeleted(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	// Load the message (must be ai_chat type with video_info)
	var msg models.Message
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&msg).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Message not found"})
	}

	// Verify it has video_info
	existing := map[string]interface{}(msg.ExtraFields)
	videoInfoRaw, ok := existing["video_info"]
	if !ok || videoInfoRaw == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Message has no video_info"})
	}

	videoInfo, ok := videoInfoRaw.(map[string]interface{})
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid video_info format"})
	}

	// Set deleted flag
	videoInfo["deleted"] = true
	existing["video_info"] = videoInfo

	if err := database.DB.Model(&msg).Update("extra_fields", models.JSONB(existing)).Error; err != nil {
		log.Printf("[VideoHistory] Failed to mark chat video deleted: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to mark video deleted"})
	}

	log.Printf("[VideoHistory] Marked chat video deleted: message_id=%s", id)

	return c.JSON(fiber.Map{"message": "deleted"})
}
