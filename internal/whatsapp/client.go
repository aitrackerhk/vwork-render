package whatsapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"nwork/config"
	"time"
)

var ErrWhatsAppNotConfigured = errors.New("whatsapp api not configured")
var ErrWhatsAppNotEnabled = errors.New("whatsapp api not enabled")

type Client struct {
	cfg           *config.Config
	httpClient    *http.Client
	baseURL       string
	phoneNumberID string
	accessToken   string
}

// NewClient 創建新的 WhatsApp API 客戶端
func NewClient(cfg *config.Config) *Client {
	baseURL := fmt.Sprintf("https://graph.facebook.com/%s", cfg.WhatsApp.APIVersion)
	
	return &Client{
		cfg:           cfg,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		baseURL:       baseURL,
		phoneNumberID: cfg.WhatsApp.PhoneNumberID,
		accessToken:   cfg.WhatsApp.APIToken,
	}
}

// IsConfigured 檢查 WhatsApp API 是否已配置
func (c *Client) IsConfigured() bool {
	return c.cfg.WhatsApp.Enabled &&
		c.cfg.WhatsApp.APIToken != "" &&
		c.cfg.WhatsApp.PhoneNumberID != ""
}

// SendTextMessage 發送文本消息
func (c *Client) SendTextMessage(to, message string) error {
	if !c.IsConfigured() {
		return ErrWhatsAppNotConfigured
	}

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text": map[string]string{
			"body": message,
		},
	}

	return c.sendMessage(payload)
}

// SendTemplateMessage 發送模板消息（用於 24 小時窗口外的消息）
func (c *Client) SendTemplateMessage(to, templateName, languageCode string, parameters []map[string]string) error {
	if !c.IsConfigured() {
		return ErrWhatsAppNotConfigured
	}

	template := map[string]interface{}{
		"name": templateName,
		"language": map[string]string{
			"code": languageCode,
		},
	}

	// 如果有參數，添加到模板中
	if len(parameters) > 0 {
		components := []map[string]interface{}{
			{
				"type":       "body",
				"parameters": parameters,
			},
		}
		template["components"] = components
	}

	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "template",
		"template":          template,
	}

	return c.sendMessage(payload)
}

// sendMessage 發送消息的內部方法
func (c *Client) sendMessage(payload map[string]interface{}) error {
	url := fmt.Sprintf("%s/%s/messages", c.baseURL, c.phoneNumberID)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error struct {
				Message   string `json:"message"`
				Type      string `json:"type"`
				Code      int    `json:"code"`
				ErrorData struct {
					MessagingProduct string `json:"messaging_product"`
					Details          string `json:"details"`
				} `json:"error_data"`
			} `json:"error"`
		}
		
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return fmt.Errorf("whatsapp api error [%d]: %s", errorResp.Error.Code, errorResp.Error.Message)
		}
		
		return fmt.Errorf("whatsapp api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		MessagingProduct string `json:"messaging_product"`
		Contacts         []struct {
			Input string `json:"input"`
			WAID  string `json:"wa_id"`
		} `json:"contacts"`
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		// 即使解析失敗，如果狀態碼是 200，也認為成功
		return nil
	}

	return nil
}

// UploadMedia 上傳媒體文件
func (c *Client) UploadMedia(fileData []byte, mimeType string) (string, error) {
	if !c.IsConfigured() {
		return "", ErrWhatsAppNotConfigured
	}

	url := fmt.Sprintf("%s/%s/media", c.baseURL, c.phoneNumberID)

	// 創建 multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	
	// 添加文件
	part, err := writer.CreateFormFile("file", "media")
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return "", fmt.Errorf("write file data: %w", err)
	}
	
	// 添加 mime type
	if err := writer.WriteField("messaging_product", "whatsapp"); err != nil {
		return "", fmt.Errorf("write messaging_product: %w", err)
	}
	if err := writer.WriteField("type", mimeType); err != nil {
		return "", fmt.Errorf("write type: %w", err)
	}
	
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close writer: %w", err)
	}

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whatsapp api error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	return result.ID, nil
}

