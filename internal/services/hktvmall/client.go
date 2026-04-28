package hktvmall

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultAPIBase is the default HKTVmall API endpoint
	// 商家需要向 HKTVmall 申請獲取正確的 API 端點
	DefaultAPIBase = "https://api.hktvmall.com"
)

// Client represents a HKTVmall API client
type Client struct {
	MerchantID string
	ShopID     string
	AppID      string
	AppSecret  string
	APIBase    string
	HTTPClient *http.Client
}

// NewClient creates a new HKTVmall API client
func NewClient(merchantID, shopID, appID, appSecret, apiBase string) *Client {
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}

	return &Client{
		MerchantID: merchantID,
		ShopID:     shopID,
		AppID:      appID,
		AppSecret:  appSecret,
		APIBase:    strings.TrimSuffix(apiBase, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GenerateSign generates the HMAC-SHA256 signature for API requests
func (c *Client) GenerateSign(timestamp int64, path string, body string) string {
	// 簽名格式: HMAC-SHA256(appId + timestamp + path + body, appSecret)
	baseString := fmt.Sprintf("%s%d%s%s", c.AppID, timestamp, path, body)
	h := hmac.New(sha256.New, []byte(c.AppSecret))
	h.Write([]byte(baseString))
	return hex.EncodeToString(h.Sum(nil))
}

// APIResponse represents a generic HKTVmall API response
type APIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// IsSuccess checks if the API call was successful
func (r *APIResponse) IsSuccess() bool {
	return r.Code == 0 || r.Code == 200
}

// DoRequest performs an HTTP request to the HKTVmall API
func (c *Client) DoRequest(method, path string, params map[string]string, body interface{}) (*APIResponse, error) {
	timestamp := time.Now().Unix()

	var bodyBytes []byte
	var reqBody io.Reader
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = strings.NewReader(string(bodyBytes))
	}

	sign := c.GenerateSign(timestamp, path, string(bodyBytes))

	// Build URL with query params
	u, err := url.Parse(c.APIBase + path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(method, u.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Id", c.AppID)
	req.Header.Set("X-Merchant-Id", c.MerchantID)
	req.Header.Set("X-Shop-Id", c.ShopID)
	req.Header.Set("X-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Sign", sign)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(respBody))
	}

	if !apiResp.IsSuccess() {
		return &apiResp, fmt.Errorf("API error: code=%d, message=%s", apiResp.Code, apiResp.Message)
	}

	return &apiResp, nil
}

// Get performs a GET request
func (c *Client) Get(path string, params map[string]string) (*APIResponse, error) {
	return c.DoRequest("GET", path, params, nil)
}

// Post performs a POST request
func (c *Client) Post(path string, body interface{}) (*APIResponse, error) {
	return c.DoRequest("POST", path, nil, body)
}

// Put performs a PUT request
func (c *Client) Put(path string, body interface{}) (*APIResponse, error) {
	return c.DoRequest("PUT", path, nil, body)
}

// Delete performs a DELETE request
func (c *Client) Delete(path string, params map[string]string) (*APIResponse, error) {
	return c.DoRequest("DELETE", path, params, nil)
}

// TestConnection tests if the API credentials are valid
func (c *Client) TestConnection() error {
	// 嘗試獲取商店信息來驗證連接
	_, err := c.GetShopInfo()
	return err
}

// ShopInfo represents shop information
type ShopInfo struct {
	ShopID     string `json:"shop_id"`
	ShopName   string `json:"shop_name"`
	MerchantID string `json:"merchant_id"`
	Status     string `json:"status"`
}

// GetShopInfo retrieves the shop information
func (c *Client) GetShopInfo() (*ShopInfo, error) {
	resp, err := c.Get("/api/v1/shop/info", nil)
	if err != nil {
		return nil, err
	}

	var info ShopInfo
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse shop info: %w", err)
	}

	return &info, nil
}
