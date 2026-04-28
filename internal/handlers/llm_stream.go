package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"nwork/config"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
)

type streamRequest struct {
	Model       string       `json:"model"`
	Messages    []Message    `json:"messages"`
	Temperature float64      `json:"temperature"`
	MaxTokens   int          `json:"max_tokens"`
	Tools       []ToolDef    `json:"tools,omitempty"`       // Client tool schemas for native Gemini FC (desktop only)
	Attachments []Attachment `json:"attachments,omitempty"` // Image/file attachments (base64 inlineData for Gemini)
	WebSearch   bool         `json:"web_search,omitempty"`  // Enable Google Search grounding via Gemini
}

// ToolDef represents a tool definition sent by the desktop client.
// Maps to TMCPItem on the frontend side.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ChatWithLLMStream 串流聊天端點（需認證 + AI Coins，僅支援 Gemini）
// POST /api/v1/llm/chat/stream
func ChatWithLLMStream(c *fiber.Ctx) error {
	cfg := config.Load()
	if cfg.LLM.Provider != "gemini" {
		return c.Status(501).JSON(fiber.Map{"error": "串流模式暫不支援此 AI 供應商"})
	}
	if cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "AI API Key 未配置"})
	}

	var req streamRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	model := req.Model
	if model == "" {
		model = cfg.LLM.Model
	}
	if model == "gemini-pro" || model == "gemini-1.5-flash" || model == "gemini-1.5-pro" || model == "gemini-2.0-flash-exp" {
		model = "gemini-2.5-flash"
	}
	model = strings.TrimSuffix(model, "-latest")

	defaultPrompt := "你是 vAi，由 V-sys 開發的專業 AI 助手。你能回答任何問題——不論是業務數據查詢、一般知識、技術問題、日常生活、創意寫作或任何其他主題。你不會以「隱私」或「無法回答」為由拒絕合理的問題。\n" +
		"重要：你的名字是「vAi」，由 V-sys 開發。絕對不要提及 Google、Gemini、Bard 或任何其他 AI 供應商的名稱。如果用戶問你是誰或用什麼技術，只回答「我是 vAi，由 V-sys 開發的 AI 助手」。\n" +
		"你具有網路搜尋能力（透過 google_search 工具），可以搜尋最新的即時資訊、新聞、市場趨勢等。當用戶詢問需要即時資訊的問題（如新聞、最新消息、天氣、市場動態、時事等），請善用搜尋功能為用戶提供最新資訊。不要拒絕這類請求。"
	basePrompt := models.GetSystemSetting("ai_system_prompt", defaultPrompt)

	contents := []map[string]interface{}{
		{
			"role": "user",
			"parts": []map[string]interface{}{
				{"text": basePrompt},
			},
		},
		{
			"role": "model",
			"parts": []map[string]interface{}{
				{"text": "明白了。"},
			},
		},
	}

	for idx, msg := range req.Messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}

		parts := []map[string]interface{}{
			{"text": msg.Content},
		}
		// Attach files/audio to the last user message as inlineData for Gemini.
		if role == "user" && idx == len(req.Messages)-1 && len(req.Attachments) > 0 {
			for _, att := range req.Attachments {
				if strings.TrimSpace(att.Data) == "" {
					continue
				}
				parts = append(parts, map[string]interface{}{
					"inlineData": map[string]interface{}{
						"mimeType": att.MimeType,
						"data":     att.Data,
					},
				})
			}
		}
		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": parts,
		})
	}

	geminiReq := map[string]interface{}{
		"contents": contents,
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
	// Always enable Google Search grounding — the model decides when to use it.
	geminiReq["tools"] = []interface{}{
		map[string]interface{}{
			"google_search": map[string]interface{}{},
		},
	}
	log.Printf("[ChatWithLLMStream] google_search tool registered")
	// Ensure generationConfig exists
	if geminiReq["generationConfig"] == nil {
		geminiReq["generationConfig"] = map[string]interface{}{}
	}
	genCfgWeb := geminiReq["generationConfig"].(map[string]interface{})
	if req.Temperature > 0 {
		genCfgWeb["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		genCfgWeb["maxOutputTokens"] = req.MaxTokens
	}
	// Gemini 2.5 models have thinking enabled by default, which consumes maxOutputTokens
	// and can cause empty responses. Disable thinking by default for reliability.
	if strings.HasPrefix(model, "gemini-2.5") {
		genCfgWeb["thinkingConfig"] = map[string]interface{}{
			"thinkingBudget": 0,
		}
	}

	reqBody, err := json.Marshal(geminiReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to build AI request"})
	}

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?key=%s&alt=sse", model, cfg.LLM.APIKey)
	log.Printf("[ChatWithLLMStream] Calling Gemini API: model=%s, messages=%d", model, len(req.Messages))
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create request"})
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[ChatWithLLMStream] Gemini HTTP call FAILED: %s", err.Error())
		return c.Status(500).JSON(fiber.Map{"error": "Failed to call LLM API: " + err.Error()})
	}
	log.Printf("[ChatWithLLMStream] Gemini API response status: %d", resp.StatusCode)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("[ChatWithLLMStream] Gemini API ERROR %d: %s", resp.StatusCode, string(body))
		// Gemini returned an error — fall back to non-SSE URL and try again
		log.Printf("[ChatWithLLMStream] Retrying WITHOUT alt=sse...")
		apiURL2 := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?key=%s", model, cfg.LLM.APIKey)
		httpReq2, err2 := http.NewRequest("POST", apiURL2, bytes.NewBuffer(reqBody))
		if err2 != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create fallback request"})
		}
		httpReq2.Header.Set("Content-Type", "application/json")
		resp2, err2 := client.Do(httpReq2)
		if err2 != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to call LLM API (fallback): " + err2.Error()})
		}
		if resp2.StatusCode != 200 {
			body2, _ := io.ReadAll(resp2.Body)
			resp2.Body.Close()
			log.Printf("[ChatWithLLMStream] Fallback also failed %d: %s", resp2.StatusCode, string(body2))
			return c.Status(resp2.StatusCode).JSON(fiber.Map{"error": string(body2)})
		}
		// Fallback succeeded — use JSON array parsing
		log.Printf("[ChatWithLLMStream] Fallback succeeded — using JSON decoder for raw array")
		resp = resp2
		// Use JSON decoder for non-SSE response below
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			defer resp.Body.Close()
			decoder := json.NewDecoder(resp.Body)
			// Read the opening '[' of the array
			if t, err := decoder.Token(); err != nil {
				log.Printf("[ChatWithLLMStream] JSON decoder: failed to read opening token: %v", err)
			} else {
				log.Printf("[ChatWithLLMStream] JSON decoder: opening token: %v", t)
			}
			chunkCount := 0
			for decoder.More() {
				var chunk map[string]interface{}
				if err := decoder.Decode(&chunk); err != nil {
					log.Printf("[ChatWithLLMStream] JSON decoder error: %v", err)
					break
				}
				chunkCount++
				textChunk := extractGeminiChunkText(chunk)
				if textChunk == "" {
					continue
				}
				textChunk = sanitizeBrandNames(textChunk)
				fmt.Fprintf(w, "data: %s\n\n", escapeSSE(textChunk))
				_ = w.Flush()
			}
			log.Printf("[ChatWithLLMStream] JSON decoder stream ended — chunkCount=%d", chunkCount)
			fmt.Fprint(w, "event: done\ndata: [DONE]\n\n")
			_ = w.Flush()
		})
		return nil
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		// Collect grounding metadata across chunks (only present in final chunk)
		var lastGroundingMeta map[string]interface{}
		chunkCount := 0
		sentAny := false

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "data:") {
				line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				log.Printf("[ChatWithLLMStream] SSE chunk parse error: %v, line prefix: %s", err, line[:min(len(line), 100)])
				continue
			}

			chunkCount++

			// Check for grounding metadata in this chunk (usually in the last chunk).
			// Always check — google_search is always registered as a tool.
			if candidates, ok := chunk["candidates"].([]interface{}); ok && len(candidates) > 0 {
				if candidate, ok := candidates[0].(map[string]interface{}); ok {
					if gm, ok := candidate["groundingMetadata"].(map[string]interface{}); ok {
						lastGroundingMeta = gm
					}
				}
			}

			textChunk := extractGeminiChunkText(chunk)
			if textChunk == "" {
				continue
			}

			textChunk = sanitizeBrandNames(textChunk)
			fmt.Fprintf(w, "data: %s\n\n", escapeSSE(textChunk))
			_ = w.Flush()
			sentAny = true
		}

		if scanErr := scanner.Err(); scanErr != nil {
			log.Printf("[ChatWithLLMStream] scanner error: %s", scanErr.Error())
		}

		log.Printf("[ChatWithLLMStream] SSE stream ended — chunkCount=%d, sentAny=%v", chunkCount, sentAny)

		// Send grounding metadata as a separate SSE event before done
		if lastGroundingMeta != nil {
			groundingData := extractGroundingMetadata(lastGroundingMeta)
			if len(groundingData) > 0 {
				groundingJSON, err := json.Marshal(groundingData)
				if err == nil {
					fmt.Fprintf(w, "event: grounding\ndata: %s\n\n", string(groundingJSON))
					_ = w.Flush()
				}
			}
		}

		fmt.Fprint(w, "event: done\ndata: [DONE]\n\n")
		_ = w.Flush()
	})

	return nil
}

func extractGeminiChunkText(chunk map[string]interface{}) string {
	candidates, ok := chunk["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return ""
	}
	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return ""
	}
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return ""
	}
	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		part, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		// Skip thinking parts from gemini-2.5 models (thought: true)
		if thought, ok := part["thought"].(bool); ok && thought {
			continue
		}
		if text, ok := part["text"].(string); ok {
			sb.WriteString(text)
		}
	}
	return sb.String()
}

// extractGeminiFunctionCall extracts a functionCall from a Gemini SSE chunk.
// Returns (functionName, args map, true) if found, ("", nil, false) otherwise.
func extractGeminiFunctionCall(chunk map[string]interface{}) (string, map[string]interface{}, bool) {
	candidates, ok := chunk["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", nil, false
	}
	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return "", nil, false
	}
	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return "", nil, false
	}
	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", nil, false
	}
	for _, p := range parts {
		part, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		fc, ok := part["functionCall"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fc["name"].(string)
		args, _ := fc["args"].(map[string]interface{})
		if name != "" {
			return name, args, true
		}
	}
	return "", nil, false
}

func escapeSSE(s string) string {
	return strings.ReplaceAll(s, "\n", "\\n")
}

// sanitizeBrandNames replaces references to the upstream LLM provider
// (Google, Gemini, etc.) so end-users see only "vAi" branding.
// Applied to every AI response before it reaches the client.
func sanitizeBrandNames(s string) string {
	// Order matters: replace longer / more specific phrases first.
	replacements := []struct{ old, new string }{
		// English phrases (case-sensitive)
		{"Google Gemini", "vAi"},
		{"Google AI", "vAi"},
		{"Gemini Pro", "vAi"},
		{"Gemini Flash", "vAi"},
		{"Gemini Advanced", "vAi"},
		{"Gemini Ultra", "vAi"},
		{"Gemini", "vAi"},
		{"Google Bard", "vAi"},
		// Chinese variants
		{"谷歌", "vAi"},
		// "by Google" / "from Google" — only when clearly referring to the AI
		{"by Google", "by V-sys"},
		{"from Google", "from V-sys"},
		// Standalone "Google" as AI self-reference (use word-boundary heuristic)
		// We intentionally keep Google in URLs and common product names untouched.
	}
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.old, r.new)
	}

	// Case-insensitive pass for stray "gemini" / "GEMINI" variants
	lower := strings.ToLower(s)
	idx := 0
	var sb strings.Builder
	sb.Grow(len(s))
	for {
		pos := strings.Index(lower[idx:], "gemini")
		if pos == -1 {
			sb.WriteString(s[idx:])
			break
		}
		sb.WriteString(s[idx : idx+pos])
		sb.WriteString("vAi")
		idx += pos + len("gemini")
	}
	return sb.String()
}

// sanitizeToolName replaces characters not allowed in Gemini FC function names.
// Gemini only allows [a-zA-Z0-9_] in function names.
// Common offender: "desktop-editor_generate_pptx" → "desktop_editor_generate_pptx"
func sanitizeToolName(name string) string {
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

// ChatWithLLMStreamDesktop 專門給 vOffice desktop 用的串流聊天端點。
// 支援原生 Gemini Function Calling — client 傳來 tool schemas，backend 全部
// 註冊為 Gemini native FC。Gemini 回傳 functionCall 時，backend 透過 SSE
// event "function_call" 轉發給前端。前端執行 tool 後，re-POST 帶 function
// response 繼續對話。
// POST /api/v1/llm/chat/stream/desktop
func ChatWithLLMStreamDesktop(c *fiber.Ctx) error {
	cfg := config.Load()
	if cfg.LLM.Provider != "gemini" {
		return c.Status(501).JSON(fiber.Map{"error": "串流模式暫不支援此 AI 供應商"})
	}
	if cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "AI API Key 未配置"})
	}

	var req streamRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	model := req.Model
	if model == "" {
		model = cfg.LLM.Model
	}
	if model == "gemini-pro" || model == "gemini-1.5-flash" || model == "gemini-1.5-pro" || model == "gemini-2.0-flash-exp" {
		model = "gemini-2.0-flash"
	}
	model = strings.TrimSuffix(model, "-latest")

	// ── System prompt: vai business prompt + client system messages ──
	defaultPrompt := "你是 vAi，由 V-sys 開發的專業 AI 助手。你能回答任何問題——不論是業務數據查詢、一般知識、技術問題、日常生活、創意寫作或任何其他主題。你不會以「隱私」或「無法回答」為由拒絕合理的問題。\n" +
		"重要：你的名字是「vAi」，由 V-sys 開發。絕對不要提及 Google、Gemini、Bard 或任何其他 AI 供應商的名稱。如果用戶問你是誰或用什麼技術，只回答「我是 vAi，由 V-sys 開發的 AI 助手」。\n" +
		"你具有網路搜尋能力（透過 google_search 工具），可以搜尋最新的即時資訊、新聞、市場趨勢等。當用戶詢問需要即時資訊的問題（如新聞、最新消息、天氣、市場動態、時事等），請善用搜尋功能為用戶提供最新資訊。不要拒絕這類請求。"
	fullSystemPrompt := models.GetSystemSetting("ai_system_prompt", defaultPrompt)

	var clientSystemParts []string
	for _, msg := range req.Messages {
		if msg.Role == "system" && strings.TrimSpace(msg.Content) != "" {
			clientSystemParts = append(clientSystemParts, msg.Content)
		}
	}
	if len(clientSystemParts) > 0 {
		fullSystemPrompt = fullSystemPrompt + "\n\n" + strings.Join(clientSystemParts, "\n\n")
	}

	// ── Build conversation contents ──
	// Supports normal text messages (user/assistant) plus special roles:
	//   role="function_call"     → Gemini model's functionCall part
	//   role="function_response" → User's functionResponse part
	var contents []map[string]interface{}
	var lastRole string
	for i, msg := range req.Messages {
		if msg.Role == "system" {
			continue
		}

		// Handle function_call messages (model wanted to call a tool)
		if msg.Role == "function_call" && msg.FunctionCallName != "" {
			fc := map[string]interface{}{
				"name": sanitizeToolName(msg.FunctionCallName),
				"args": msg.FunctionCallArgs,
			}
			if msg.ThoughtSignature != "" {
				fc["thought_signature"] = msg.ThoughtSignature
			}
			parts := []map[string]interface{}{
				{
					"functionCall": fc,
				},
			}
			// Include any text prefix (assistant often says "I will do X" before calling tool)
			if strings.TrimSpace(msg.Content) != "" {
				parts = append([]map[string]interface{}{{"text": msg.Content}}, parts...)
			}
			entry := map[string]interface{}{
				"role":  "model",
				"parts": parts,
			}
			contents = append(contents, entry)
			lastRole = "model"
			continue
		}

		// Handle function_response messages (tool execution result)
		if msg.Role == "function_response" && msg.FunctionRespName != "" {
			entry := map[string]interface{}{
				"role": "user",
				"parts": []map[string]interface{}{
					{
						"functionResponse": map[string]interface{}{
							"name": sanitizeToolName(msg.FunctionRespName),
							"response": map[string]interface{}{
								"result": msg.FunctionRespData,
							},
						},
					},
				},
			}
			contents = append(contents, entry)
			lastRole = "user"
			continue
		}

		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}

		// Build parts for this message
		var parts []map[string]interface{}
		if strings.TrimSpace(msg.Content) != "" {
			parts = append(parts, map[string]interface{}{"text": msg.Content})
		}

		// If this is the last user message and there are attachments, add inlineData parts
		// (images, PDFs, documents, etc. — anything Gemini supports as inline data)
		if msg.Role == "user" && len(req.Attachments) > 0 {
			// Find if this is the last user message in the list
			isLastUser := true
			for j := i + 1; j < len(req.Messages); j++ {
				if req.Messages[j].Role == "user" {
					isLastUser = false
					break
				}
			}
			if isLastUser {
				for _, att := range req.Attachments {
					if att.Data != "" && att.MimeType != "" {
						parts = append(parts, map[string]interface{}{
							"inlineData": map[string]interface{}{
								"mimeType": att.MimeType,
								"data":     att.Data,
							},
						})
					}
				}
				log.Printf("[Desktop FC] Added %d attachment(s) as inlineData to last user message", len(req.Attachments))
			}
		}

		if len(parts) == 0 {
			parts = append(parts, map[string]interface{}{"text": " "})
		}

		// Gemini requires alternating user/model roles — merge consecutive same roles
		if role == lastRole && len(contents) > 0 {
			prev := contents[len(contents)-1]
			prevParts, ok := prev["parts"].([]map[string]interface{})
			if !ok {
				// Handle potential []interface{} from unmarshal
				if rawParts, ok := prev["parts"].([]interface{}); ok {
					for _, rp := range rawParts {
						if pMap, ok := rp.(map[string]interface{}); ok {
							prevParts = append(prevParts, pMap)
						}
					}
				}
			}
			prevParts = append(prevParts, parts...)
			prev["parts"] = prevParts
		} else {
			contents = append(contents, map[string]interface{}{
				"role":  role,
				"parts": parts,
			})
			lastRole = role
		}
	}

	if len(contents) == 0 {
		contents = append(contents, map[string]interface{}{
			"role": "user",
			"parts": []map[string]interface{}{
				{"text": "Hello"},
			},
		})
	}

	// Ensure first message is user role (Gemini requirement)
	if first, ok := contents[0]["role"].(string); ok && first == "model" {
		contents = append([]map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "(conversation continues)"},
				},
			},
		}, contents...)
	}

	// ── Build Gemini native FC tool declarations from client-provided tools ──
	geminiReq := map[string]interface{}{
		"contents": contents,
		"systemInstruction": map[string]interface{}{
			"parts": []map[string]interface{}{
				{"text": fullSystemPrompt},
			},
		},
	}

	if len(req.Tools) > 0 {
		var fnDecls []map[string]interface{}
		for _, t := range req.Tools {
			// Gemini FC only allows [a-zA-Z0-9_] in function names.
			// Tool names like "desktop-editor_generate_pptx" contain hyphens
			// which cause "Malformed function call" errors.
			// Sanitize: replace all non-alphanumeric/underscore chars with underscore.
			sanitizedName := sanitizeToolName(t.Name)
			decl := map[string]interface{}{
				"name":        sanitizedName,
				"description": t.Description,
			}
			// Convert JSON Schema (inputSchema) to Gemini parameter format
			if t.InputSchema != nil {
				decl["parameters"] = convertJSONSchemaToGemini(t.InputSchema)
			}
			fnDecls = append(fnDecls, decl)
		}
		// Gemini does NOT allow google_search and functionDeclarations in the
		// same request. Use functionDeclarations only when client tools exist.
		geminiReq["tools"] = []interface{}{
			map[string]interface{}{
				"functionDeclarations": fnDecls,
			},
		}
		geminiReq["toolConfig"] = map[string]interface{}{
			"functionCallingConfig": map[string]interface{}{
				"mode": "AUTO",
			},
		}
		log.Printf("[Desktop FC] Registered %d tools as native Gemini FC (google_search disabled to avoid conflict)", len(fnDecls))
	} else {
		// No client tools — still register google_search so the model can search the web.
		geminiReq["tools"] = []interface{}{
			map[string]interface{}{
				"google_search": map[string]interface{}{},
			},
		}
		log.Printf("[Desktop FC] No client tools — google_search only")
	}

	// Build reverse mapping: sanitized name → original name
	// so we can restore the original tool name when Gemini returns a functionCall
	toolNameMap := make(map[string]string)
	for _, t := range req.Tools {
		sanitized := sanitizeToolName(t.Name)
		if sanitized != t.Name {
			toolNameMap[sanitized] = t.Name
			log.Printf("[Desktop FC] Tool name mapping: %s → %s", sanitized, t.Name)
		}
	}

	// Ensure generationConfig exists
	if geminiReq["generationConfig"] == nil {
		geminiReq["generationConfig"] = map[string]interface{}{}
	}
	genCfg := geminiReq["generationConfig"].(map[string]interface{})

	if req.Temperature > 0 {
		genCfg["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		genCfg["maxOutputTokens"] = req.MaxTokens
	}

	// Gemini 2.5 models have thinking enabled by default, which consumes maxOutputTokens
	// and can cause empty responses. Disable thinking by default for reliability.
	if strings.HasPrefix(model, "gemini-2.5") {
		genCfg["thinkingConfig"] = map[string]interface{}{
			"thinkingBudget": 0,
		}
		log.Printf("[Desktop FC] Disabled thinking for model %s (thinkingBudget: 0)", model)
	}

	// ── Debug: log the conversation contents for FC debugging ──
	if contentsJSON, err := json.MarshalIndent(contents, "", "  "); err == nil {
		contentsStr := string(contentsJSON)
		if len(contentsStr) > 3000 {
			contentsStr = contentsStr[:3000] + "...(truncated)"
		}
		log.Printf("[Desktop FC DEBUG] Contents sent to Gemini (%d entries):\n%s", len(contents), contentsStr)
	}

	reqBody, err := json.Marshal(geminiReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to build AI request"})
	}

	// Debug: log the full request size
	log.Printf("[Desktop FC DEBUG] Request body size: %d bytes, model: %s", len(reqBody), model)

	// Use SSE streaming for text output
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?key=%s&alt=sse", model, cfg.LLM.APIKey)
	log.Printf("[Desktop FC DEBUG] Calling Gemini API: model=%s, URL length=%d", model, len(apiURL))
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create request"})
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[Desktop FC DEBUG] Gemini HTTP call FAILED: %s", err.Error())
		return c.Status(500).JSON(fiber.Map{"error": "Failed to call LLM API: " + err.Error()})
	}
	log.Printf("[Desktop FC DEBUG] Gemini API response status: %d", resp.StatusCode)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		bodyStr := string(body)
		if len(bodyStr) > 1500 {
			bodyStr = bodyStr[:1500] + "...(truncated)"
		}
		log.Printf("[Desktop FC DEBUG] Gemini API ERROR %d: %s", resp.StatusCode, bodyStr)
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": string(body)})
	}
	log.Printf("[Desktop FC DEBUG] Gemini API returned 200 — starting SSE stream")

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		chunkCount := 0
		sentAny := false

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "data:") {
				line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				log.Printf("[Desktop FC DEBUG] SSE chunk parse error: %v, line: %s", err, line[:min(len(line), 200)])
				// Forward parse error to frontend for debugging
				errPayload, _ := json.Marshal(map[string]interface{}{
					"type":    "parse_error",
					"error":   err.Error(),
					"rawLine": line[:min(len(line), 500)],
				})
				fmt.Fprintf(w, "event: debug\ndata: %s\n\n", string(errPayload))
				_ = w.Flush()
				continue
			}

			chunkCount++

			// Debug: log raw chunk structure (first 500 chars)
			chunkJSON, _ := json.Marshal(chunk)
			chunkStr := string(chunkJSON)
			if len(chunkStr) > 500 {
				chunkStr = chunkStr[:500] + "..."
			}
			log.Printf("[Desktop FC DEBUG] SSE chunk #%d: %s", chunkCount, chunkStr)

			// Forward raw chunk to frontend for debugging (truncated)
			debugPayload, _ := json.Marshal(map[string]interface{}{
				"type":     "chunk",
				"chunkNum": chunkCount,
				"raw":      chunkStr,
			})
			fmt.Fprintf(w, "event: debug\ndata: %s\n\n", string(debugPayload))
			_ = w.Flush()

			// ── Check for functionCall (native Gemini FC) ──
			// Forward ALL function calls to frontend via SSE event.
			// Frontend executes the tool and re-POSTs with function_response.
			fnName, fnArgs, hasFnCall := extractGeminiFunctionCall(chunk)
			if hasFnCall && fnName != "" {
				// Restore original tool name (with hyphens) from sanitized name
				originalName := fnName
				if orig, ok := toolNameMap[fnName]; ok {
					originalName = orig
					log.Printf("[Desktop FC] Restored tool name: %s → %s", fnName, originalName)
				}

				// Extract thought signature if present
				var thoughtSig string
				if candidates, ok := chunk["candidates"].([]interface{}); ok && len(candidates) > 0 {
					if cand, ok := candidates[0].(map[string]interface{}); ok {
						if content, ok := cand["content"].(map[string]interface{}); ok {
							if parts, ok := content["parts"].([]interface{}); ok {
								for _, p := range parts {
									if part, ok := p.(map[string]interface{}); ok {
										if ts, ok := part["thoughtSignature"].(string); ok {
											thoughtSig = ts
										} else if ts, ok := part["thought_signature"].(string); ok {
											thoughtSig = ts
										}
									}
								}
							}
						}
					}
				}

				log.Printf("[Desktop FC] functionCall: %s args=%v, hasThoughtSig=%v", originalName, fnArgs, thoughtSig != "")

				payloadMap := map[string]interface{}{
					"name": originalName,
					"args": fnArgs,
				}
				if thoughtSig != "" {
					payloadMap["thought_signature"] = thoughtSig
				}

				fcPayload, _ := json.Marshal(payloadMap)
				fmt.Fprintf(w, "event: function_call\ndata: %s\n\n", string(fcPayload))
				_ = w.Flush()

				// End the stream — frontend will execute the tool and re-POST
				fmt.Fprint(w, "event: done\ndata: [DONE]\n\n")
				_ = w.Flush()
				sentAny = true
				return
			}

			// ── Normal text chunk ──
			textChunk := extractGeminiChunkText(chunk)
			if textChunk == "" {
				log.Printf("[Desktop FC DEBUG] chunk #%d has no text and no functionCall — skipping", chunkCount)
				continue
			}

			textChunk = sanitizeBrandNames(textChunk)
			fmt.Fprintf(w, "data: %s\n\n", escapeSSE(textChunk))
			_ = w.Flush()
			sentAny = true
		}

		if scanErr := scanner.Err(); scanErr != nil {
			log.Printf("[Desktop FC DEBUG] scanner error: %s", scanErr.Error())
			errPayload, _ := json.Marshal(map[string]interface{}{
				"type":  "scanner_error",
				"error": scanErr.Error(),
			})
			fmt.Fprintf(w, "event: debug\ndata: %s\n\n", string(errPayload))
			_ = w.Flush()
		}

		log.Printf("[Desktop FC DEBUG] SSE stream loop ended — chunkCount=%d, sentAny=%v — sending done event", chunkCount, sentAny)
		if !sentAny {
			log.Printf("[Desktop FC WARN] Stream ended with NO content sent to client")
		}
		fmt.Fprint(w, "event: done\ndata: [DONE]\n\n")
		_ = w.Flush()
	})

	return nil
}

// convertJSONSchemaToGemini converts a JSON Schema object (from TMCPItem.inputSchema)
// to Gemini's parameter format. Gemini uses uppercase TYPE names.
func convertJSONSchemaToGemini(schema map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	if t, ok := schema["type"].(string); ok {
		result["type"] = strings.ToUpper(t)
	}

	if props, ok := schema["properties"].(map[string]interface{}); ok {
		geminiProps := make(map[string]interface{})
		for k, v := range props {
			if propMap, ok := v.(map[string]interface{}); ok {
				geminiProps[k] = convertJSONSchemaToGemini(propMap)
			}
		}
		result["properties"] = geminiProps
	}

	if req, ok := schema["required"].([]interface{}); ok {
		reqStrs := make([]string, 0, len(req))
		for _, r := range req {
			if s, ok := r.(string); ok {
				reqStrs = append(reqStrs, s)
			}
		}
		result["required"] = reqStrs
	}

	if desc, ok := schema["description"].(string); ok {
		result["description"] = desc
	}

	if enum, ok := schema["enum"].([]interface{}); ok {
		result["enum"] = enum
	}

	// Handle array items
	if items, ok := schema["items"].(map[string]interface{}); ok {
		result["items"] = convertJSONSchemaToGemini(items)
	}

	return result
}
