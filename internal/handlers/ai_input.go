package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nwork/config"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ProcessOCR 處理圖片 OCR
func ProcessOCR(c *fiber.Ctx) error {
	cfg := config.Load()

	if cfg.Vision.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "Vision API Key 未配置"})
	}

	// 獲取上傳的圖片
	file, err := c.FormFile("image")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "請上傳圖片文件"})
	}

	// 讀取文件內容
	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取圖片文件"})
	}
	defer src.Close()

	imageData, err := io.ReadAll(src)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取圖片數據"})
	}

	// 轉換為 base64
	imageBase64 := base64.StdEncoding.EncodeToString(imageData)

	// 構建 Vision API 請求
	apiURL := fmt.Sprintf("https://vision.googleapis.com/v1/images:annotate?key=%s", cfg.Vision.APIKey)

	requestBody := map[string]interface{}{
		"requests": []map[string]interface{}{
			{
				"image": map[string]interface{}{
					"content": imageBase64,
				},
				"features": []map[string]interface{}{
					{
						"type":       "TEXT_DETECTION",
						"maxResults": 10,
					},
					{
						"type":       "LOGO_DETECTION",
						"maxResults": 5,
					},
					{
						"type":       "LABEL_DETECTION",
						"maxResults": 5,
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法構建請求"})
	}

	// 調用 Vision API
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "調用 Vision API 失敗: " + err.Error()})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取 API 響應"})
	}

	if resp.StatusCode != 200 {
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": string(body)})
	}

	// 解析響應
	var visionResponse map[string]interface{}
	if err := json.Unmarshal(body, &visionResponse); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法解析 API 響應"})
	}

	// 提取文字與品牌/標籤
	var extractedText strings.Builder
	logoNames := []string{}
	labelNames := []string{}
	if responses, ok := visionResponse["responses"].([]interface{}); ok && len(responses) > 0 {
		if response, ok := responses[0].(map[string]interface{}); ok {
			if textAnnotations, ok := response["textAnnotations"].([]interface{}); ok && len(textAnnotations) > 0 {
				// 第一個是完整文字，其他是單詞
				if fullText, ok := textAnnotations[0].(map[string]interface{}); ok {
					if description, ok := fullText["description"].(string); ok {
						extractedText.WriteString(description)
					}
				}
			}
			if logos, ok := response["logoAnnotations"].([]interface{}); ok {
				for _, item := range logos {
					if logo, ok := item.(map[string]interface{}); ok {
						if desc, ok := logo["description"].(string); ok && desc != "" {
							logoNames = append(logoNames, desc)
						}
					}
				}
			}
			if labels, ok := response["labelAnnotations"].([]interface{}); ok {
				for _, item := range labels {
					if label, ok := item.(map[string]interface{}); ok {
						if desc, ok := label["description"].(string); ok && desc != "" {
							labelNames = append(labelNames, desc)
						}
					}
				}
			}
		}
	}

	if len(logoNames) > 0 {
		extractedText.WriteString("\n\n[Logo]\n")
		extractedText.WriteString(strings.Join(logoNames, ", "))
	}
	if len(labelNames) > 0 {
		extractedText.WriteString("\n\n[Labels]\n")
		extractedText.WriteString(strings.Join(labelNames, ", "))
	}

	text := extractedText.String()
	if text == "" {
		return c.Status(404).JSON(fiber.Map{"error": "圖片中未檢測到文字或品牌資訊"})
	}

	return c.JSON(fiber.Map{
		"text": text,
	})
}

// ProcessSTT 處理語音轉文字
func ProcessSTT(c *fiber.Ctx) error {
	cfg := config.Load()

	if cfg.Speech.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "Speech API Key 未配置"})
	}

	// 獲取上傳的音頻文件
	file, err := c.FormFile("audio")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "請上傳音頻文件"})
	}

	// 讀取文件內容
	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取音頻文件"})
	}
	defer src.Close()

	audioData, err := io.ReadAll(src)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取音頻數據"})
	}

	// 轉換為 base64
	audioBase64 := base64.StdEncoding.EncodeToString(audioData)

	// 檢測音頻格式（從文件名）
	audioFormat := "LINEAR16"
	needsSampleRate := true // 是否需要指定 sampleRateHertz
	if strings.HasSuffix(strings.ToLower(file.Filename), ".mp3") {
		audioFormat = "MP3"
	} else if strings.HasSuffix(strings.ToLower(file.Filename), ".wav") {
		audioFormat = "LINEAR16"
	} else if strings.HasSuffix(strings.ToLower(file.Filename), ".flac") {
		audioFormat = "FLAC"
	} else if strings.HasSuffix(strings.ToLower(file.Filename), ".ogg") {
		audioFormat = "OGG_OPUS"
		needsSampleRate = false // OGG OPUS 讓 API 自動檢測
	} else if strings.HasSuffix(strings.ToLower(file.Filename), ".webm") {
		audioFormat = "WEBM_OPUS"
		needsSampleRate = false // WEBM OPUS 讓 API 自動檢測（通常為 48000 Hz）
	}

	// 構建 Speech-to-Text API 請求
	apiURL := fmt.Sprintf("https://speech.googleapis.com/v1/speech:recognize?key=%s", cfg.Speech.APIKey)

	config := map[string]interface{}{
		"encoding":                 audioFormat,
		"languageCode":             "zh-HK", // 默認使用繁體中文（香港），可以根據需要調整
		"alternativeLanguageCodes": []string{"zh-TW", "zh-CN", "en-US"},
	}

	// 只有當需要時才指定 sampleRateHertz（對於 OPUS 格式，讓 API 自動檢測）
	if needsSampleRate {
		config["sampleRateHertz"] = 16000
	}

	requestBody := map[string]interface{}{
		"config": config,
		"audio": map[string]interface{}{
			"content": audioBase64,
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法構建請求"})
	}

	// 調用 Speech-to-Text API
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "調用 Speech-to-Text API 失敗: " + err.Error()})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取 API 響應"})
	}

	if resp.StatusCode != 200 {
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": string(body)})
	}

	// 解析響應
	var speechResponse map[string]interface{}
	if err := json.Unmarshal(body, &speechResponse); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法解析 API 響應"})
	}

	// 提取文字
	var extractedText strings.Builder
	if results, ok := speechResponse["results"].([]interface{}); ok && len(results) > 0 {
		for _, result := range results {
			if resultMap, ok := result.(map[string]interface{}); ok {
				if alternatives, ok := resultMap["alternatives"].([]interface{}); ok && len(alternatives) > 0 {
					if alt, ok := alternatives[0].(map[string]interface{}); ok {
						if transcript, ok := alt["transcript"].(string); ok {
							extractedText.WriteString(transcript)
							extractedText.WriteString(" ")
						}
					}
				}
			}
		}
	}

	text := strings.TrimSpace(extractedText.String())
	if text == "" {
		return c.Status(404).JSON(fiber.Map{"error": "音頻中未檢測到語音"})
	}

	return c.JSON(fiber.Map{
		"text": text,
	})
}

// MatchFieldsWithLLM 使用 LLM 匹配字段
func MatchFieldsWithLLM(c *fiber.Ctx) error {
	cfg := config.Load()

	if cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "LLM API Key 未配置"})
	}

	var req struct {
		Text      string                   `json:"text"`
		FieldList []map[string]interface{} `json:"field_list"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	if req.Text == "" {
		return c.Status(400).JSON(fiber.Map{"error": "文字內容不能為空"})
	}

	if len(req.FieldList) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "字段列表不能為空"})
	}

	// 構建字段列表字符串
	fieldListStr := ""
	for i, field := range req.FieldList {
		fieldName := ""
		fieldLabel := ""
		fieldType := ""
		if name, ok := field["name"].(string); ok {
			fieldName = name
		}
		if label, ok := field["label"].(string); ok {
			fieldLabel = label
		}
		if fieldLabel == "" {
			fieldLabel = fieldName
		}
		if ftype, ok := field["type"].(string); ok {
			fieldType = ftype
		}

		// 構建字段描述
		fieldDesc := fmt.Sprintf("%d. %s (%s)", i+1, fieldLabel, fieldName)

		// 檢查是否是 select2 或 relation-select（動態加載選項）
		if relationApi, ok := field["relationApi"].(string); ok && relationApi != "" {
			fieldDesc += " [類型: " + fieldType + ", 動態選項字段]"
			fieldDesc += " - 注意：此字段的選項是從 API 動態加載的，請根據文字內容匹配選項的文本描述（如名稱、標題等），而不是匹配 ID。返回的值應該是選項的文本描述，前端會自動匹配對應的選項。"
		} else if options, ok := field["options"].([]interface{}); ok && len(options) > 0 {
			// 如果字段有固定選項（yes/no, radio, checkbox），列出可選值
			fieldDesc += " [類型: " + fieldType + ", 可選值: "
			optionValues := []string{}
			for _, opt := range options {
				if optMap, ok := opt.(map[string]interface{}); ok {
					if value, ok := optMap["value"].(string); ok {
						label := value
						if lbl, ok := optMap["label"].(string); ok && lbl != "" {
							label = lbl
						}
						optionValues = append(optionValues, fmt.Sprintf("%s(%s)", label, value))
					}
				}
			}
			fieldDesc += strings.Join(optionValues, ", ") + "]"
			fieldDesc += " - 注意：此字段只能選擇上述預定義的值，不能輸入任意文本"
		} else if fieldType != "" {
			fieldDesc += " [類型: " + fieldType + "]"
		}

		fieldListStr += fieldDesc + "\n"
	}

	// 從數據庫讀取字段匹配提示詞，如果沒有則使用默認值
	defaultPrompt := `你是一個智能數據提取助手。請從以下文字中提取信息，並盡可能匹配到對應的字段。

文字內容：
%s

可用字段：
%s

匹配規則：
1. 使用模糊匹配，字段名稱和文字內容不需要完全一致
2. 支持同義詞、相似詞匹配（例如："姓名"可以匹配"名字"、"名稱"等）
3. 如果文字中包含字段相關的信息，即使不完全匹配也要嘗試提取
4. 盡可能匹配更多字段，不要過於嚴格
5. 如果字段名稱是英文，文字中的中文描述也可以匹配（例如：name 可以匹配"姓名"、"名字"等）
6. 如果文字包含品牌/Logo 名稱，優先用於匹配名稱、公司名稱、品牌名稱等字段
7. 對於數值、日期、電話等格式，盡可能識別並提取
8. 對於 yes/no、radio、checkbox 類型的字段，value 必須是字段選項中預定義的值之一，不能輸入任意文本。如果字段有 options 屬性，只能從這些選項中選擇匹配的值

請以 JSON 格式返回結果，格式如下：
{
  "matches": [
    {
      "field_name": "字段名稱（使用可用字段列表中的實際字段名稱）",
      "value": "提取的值",
      "confidence": 0.7
    }
  ]
}

請盡可能匹配所有相關字段，即使置信度較低也可以包含。confidence 是置信度（0-1之間的小數），建議設置在 0.5-1.0 之間。`

	promptTemplate := models.GetSystemSetting("ai_field_match_prompt", defaultPrompt)

	// 使用 fmt.Sprintf 替換模板中的 %s（文字內容和字段列表）
	prompt := fmt.Sprintf(promptTemplate, req.Text, fieldListStr)

	// 調用 LLM API
	var apiURL string
	var reqBody []byte
	var err error

	if cfg.LLM.Provider == "gemini" {
		model := cfg.LLM.Model
		if model == "" {
			model = "gemini-2.5-flash"
		}
		model = strings.TrimSuffix(model, "-latest")
		apiURL = fmt.Sprintf("https://generativelanguage.googleapis.com/v1/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

		geminiReq := map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"role": "user",
					"parts": []map[string]interface{}{
						{"text": prompt},
					},
				},
			},
			"generationConfig": map[string]interface{}{
				"temperature":     0.3,
				"maxOutputTokens": 2048,
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

		reqBody, err = json.Marshal(geminiReq)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "無法構建請求"})
		}
	} else {
		// OpenAI 格式
		apiURL = cfg.LLM.Endpoint
		llmReq := map[string]interface{}{
			"model": cfg.LLM.Model,
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": prompt,
				},
			},
			"temperature": 0.3,
			"max_tokens":  2000,
		}

		reqBody, err = json.Marshal(llmReq)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "無法構建請求"})
		}
	}

	// 發送請求
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法創建請求"})
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.LLM.Provider != "gemini" && cfg.LLM.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.LLM.APIKey)
	}

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "調用 LLM API 失敗: " + err.Error()})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取 API 響應"})
	}

	if resp.StatusCode != 200 {
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": string(body)})
	}

	// 解析 LLM 響應
	var llmResponse map[string]interface{}
	if err := json.Unmarshal(body, &llmResponse); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法解析 LLM 響應"})
	}

	// 提取 LLM 返回的文字
	var llmText string
	if cfg.LLM.Provider == "gemini" {
		if candidates, ok := llmResponse["candidates"].([]interface{}); ok && len(candidates) > 0 {
			if candidate, ok := candidates[0].(map[string]interface{}); ok {
				if content, ok := candidate["content"].(map[string]interface{}); ok {
					if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
						if part, ok := parts[0].(map[string]interface{}); ok {
							if text, ok := part["text"].(string); ok {
								llmText = text
							}
						}
					}
				}
			}
		}
	} else {
		// OpenAI 格式
		if choices, ok := llmResponse["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok {
						llmText = content
					}
				}
			}
		}
	}

	if llmText == "" {
		return c.Status(500).JSON(fiber.Map{"error": "LLM 未返回有效內容"})
	}

	// 嘗試從 LLM 返回的文字中提取 JSON
	// 移除可能的 markdown 代碼塊標記
	llmText = strings.TrimSpace(llmText)
	if strings.HasPrefix(llmText, "```json") {
		llmText = strings.TrimPrefix(llmText, "```json")
		llmText = strings.TrimSuffix(llmText, "```")
	} else if strings.HasPrefix(llmText, "```") {
		llmText = strings.TrimPrefix(llmText, "```")
		llmText = strings.TrimSuffix(llmText, "```")
	}
	llmText = strings.TrimSpace(llmText)

	// 解析 JSON
	var matchResult map[string]interface{}
	if err := json.Unmarshal([]byte(llmText), &matchResult); err != nil {
		// 如果解析失敗，嘗試查找 JSON 對象
		startIdx := strings.Index(llmText, "{")
		endIdx := strings.LastIndex(llmText, "}")
		if startIdx >= 0 && endIdx > startIdx {
			jsonStr := llmText[startIdx : endIdx+1]
			if err := json.Unmarshal([]byte(jsonStr), &matchResult); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "無法解析 LLM 返回的 JSON: " + err.Error()})
			}
		} else {
			return c.Status(500).JSON(fiber.Map{"error": "LLM 返回的內容不包含有效的 JSON"})
		}
	}

	return c.JSON(matchResult)
}

// MIME type mapping for AI file upload
var aiAllowedMimeTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".pdf":  "application/pdf",
	".txt":  "text/plain",
	".webm": "audio/webm",
	".wav":  "audio/wav",
	".mp3":  "audio/mpeg",
	".m4a":  "audio/mp4",
	".ogg":  "audio/ogg",
	".doc":  "application/msword",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls":  "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".ppt":  "application/vnd.ms-powerpoint",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
}

// UploadAIFile handles file upload for AI chat, returns base64 encoded file data
// and also saves the file to disk so it can be downloaded/previewed later.
func UploadAIFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "請上傳文件"})
	}

	// Check file size (max 20MB)
	if file.Size > 20*1024*1024 {
		return c.Status(400).JSON(fiber.Map{"error": "文件大小不能超過 20MB"})
	}

	// Get file extension and validate
	ext := strings.ToLower(filepath.Ext(file.Filename))
	mimeType, ok := aiAllowedMimeTypes[ext]
	if !ok {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("不支援的文件格式: %s。支援格式: xls, xlsx, doc, docx, ppt, pptx, pdf, txt, jpg, png, webm, wav, mp3, m4a, ogg", ext)})
	}

	// Read file content
	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取文件"})
	}
	defer src.Close()

	fileData, err := io.ReadAll(src)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法讀取文件數據"})
	}

	// Convert to base64
	fileBase64 := base64.StdEncoding.EncodeToString(fileData)

	// Save file to disk for later download/preview
	tenantID := middleware.GetTenantID(c)
	dateStr := time.Now().Format("2006-01-02")
	uploadDir := filepath.Join("web", "uploads", tenantID.String(), "ai-files", dateStr)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		// File save failed — still return base64 data without file_url
		return c.JSON(fiber.Map{
			"data":      fileBase64,
			"mime_type": mimeType,
			"filename":  file.Filename,
			"size":      file.Size,
		})
	}

	savedFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	savedPath := filepath.Join(uploadDir, savedFilename)
	if err := os.WriteFile(savedPath, fileData, 0644); err != nil {
		// File save failed — still return base64 data without file_url
		return c.JSON(fiber.Map{
			"data":      fileBase64,
			"mime_type": mimeType,
			"filename":  file.Filename,
			"size":      file.Size,
		})
	}

	fileURL := fmt.Sprintf("/uploads/%s/ai-files/%s/%s", tenantID.String(), dateStr, savedFilename)

	return c.JSON(fiber.Map{
		"data":      fileBase64,
		"mime_type": mimeType,
		"filename":  file.Filename,
		"size":      file.Size,
		"file_url":  fileURL,
	})
}
