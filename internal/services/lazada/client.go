package lazada

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Regional API endpoints for Lazada Open Platform
var RegionEndpoints = map[string]string{
	"SG": "https://api.lazada.sg/rest",      // Singapore
	"MY": "https://api.lazada.com.my/rest",  // Malaysia
	"TH": "https://api.lazada.co.th/rest",   // Thailand
	"PH": "https://api.lazada.com.ph/rest",  // Philippines
	"ID": "https://api.lazada.co.id/rest",   // Indonesia
	"VN": "https://api.lazada.vn/rest",      // Vietnam
}

// Auth endpoint (same for all regions)
const AuthEndpoint = "https://auth.lazada.com/rest"

// Client represents a Lazada Open Platform API client
type Client struct {
	AppKey       string
	AppSecret    string
	AccessToken  string
	RefreshToken string
	Region       string
	BaseURL      string
	HTTPClient   *http.Client
}

// NewClient creates a new Lazada API client
func NewClient(appKey, appSecret, accessToken, refreshToken, region string) *Client {
	baseURL := RegionEndpoints["SG"] // default
	if endpoint, ok := RegionEndpoints[region]; ok {
		baseURL = endpoint
	}

	return &Client{
		AppKey:       appKey,
		AppSecret:    appSecret,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Region:       region,
		BaseURL:      baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TokenResponse represents OAuth token response
type TokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	AccountID        string `json:"account_id"`
	AccountPlatform  string `json:"account_platform"`
	Country          string `json:"country"`
	UserID           string `json:"user_id"`
	SellerID         string `json:"seller_id"`
	ShortCode        string `json:"short_code"`
}

// APIResponse represents a generic API response
type APIResponse struct {
	Code      string          `json:"code"`
	Type      string          `json:"type"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

// GenerateSign generates HMAC-SHA256 signature for API request
func (c *Client) GenerateSign(params map[string]string, apiPath string) string {
	// Sort parameters by key
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build sign string: api_path + sorted params
	var signBuilder strings.Builder
	signBuilder.WriteString(apiPath)
	for _, k := range keys {
		signBuilder.WriteString(k)
		signBuilder.WriteString(params[k])
	}

	// HMAC-SHA256
	h := hmac.New(sha256.New, []byte(c.AppSecret))
	h.Write([]byte(signBuilder.String()))
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
}

// buildRequest builds an API request with signature
func (c *Client) buildRequest(method, apiPath string, params map[string]string, body interface{}) (*http.Request, error) {
	// Common parameters
	if params == nil {
		params = make(map[string]string)
	}
	params["app_key"] = c.AppKey
	params["timestamp"] = fmt.Sprintf("%d", time.Now().UnixMilli())
	params["sign_method"] = "sha256"

	if c.AccessToken != "" {
		params["access_token"] = c.AccessToken
	}

	// Generate signature
	params["sign"] = c.GenerateSign(params, apiPath)

	// Build URL
	reqURL := c.BaseURL + apiPath
	if method == "GET" {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		reqURL += "?" + values.Encode()
	}

	var req *http.Request
	var err error

	if method == "POST" && body != nil {
		jsonBody, _ := json.Marshal(body)
		req, err = http.NewRequest(method, reqURL, strings.NewReader(string(jsonBody)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		// Add params to URL for POST
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		req.URL.RawQuery = values.Encode()
	} else if method == "POST" {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		req, err = http.NewRequest(method, reqURL, strings.NewReader(values.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest(method, reqURL, nil)
		if err != nil {
			return nil, err
		}
	}

	return req, nil
}

// DoRequest executes an API request
func (c *Client) DoRequest(method, apiPath string, params map[string]string, body interface{}) (*APIResponse, error) {
	req, err := c.buildRequest(method, apiPath, params, body)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(respBody))
	}

	if apiResp.Code != "0" {
		return &apiResp, fmt.Errorf("API error: %s - %s", apiResp.Code, apiResp.Message)
	}

	return &apiResp, nil
}

// Get performs a GET request
func (c *Client) Get(apiPath string, params map[string]string) (*APIResponse, error) {
	return c.DoRequest("GET", apiPath, params, nil)
}

// Post performs a POST request
func (c *Client) Post(apiPath string, params map[string]string, body interface{}) (*APIResponse, error) {
	return c.DoRequest("POST", apiPath, params, body)
}

// RefreshAccessToken refreshes the access token
func (c *Client) RefreshAccessToken() (*TokenResponse, error) {
	params := map[string]string{
		"app_key":       c.AppKey,
		"timestamp":     fmt.Sprintf("%d", time.Now().UnixMilli()),
		"sign_method":   "sha256",
		"refresh_token": c.RefreshToken,
	}
	params["sign"] = c.GenerateSign(params, "/auth/token/refresh")

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	reqURL := AuthEndpoint + "/auth/token/refresh?" + values.Encode()
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Code      string        `json:"code"`
		Message   string        `json:"message"`
		RequestID string        `json:"request_id"`
		Data      TokenResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != "0" {
		return nil, fmt.Errorf("token refresh failed: %s - %s", result.Code, result.Message)
	}

	// Update client tokens
	c.AccessToken = result.Data.AccessToken
	c.RefreshToken = result.Data.RefreshToken

	return &result.Data, nil
}

// ExchangeAuthCode exchanges authorization code for tokens
func (c *Client) ExchangeAuthCode(code string) (*TokenResponse, error) {
	params := map[string]string{
		"app_key":     c.AppKey,
		"timestamp":   fmt.Sprintf("%d", time.Now().UnixMilli()),
		"sign_method": "sha256",
		"code":        code,
	}
	params["sign"] = c.GenerateSign(params, "/auth/token/create")

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	reqURL := AuthEndpoint + "/auth/token/create?" + values.Encode()
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Code      string        `json:"code"`
		Message   string        `json:"message"`
		RequestID string        `json:"request_id"`
		Data      TokenResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Code != "0" {
		return nil, fmt.Errorf("auth code exchange failed: %s - %s", result.Code, result.Message)
	}

	return &result.Data, nil
}

// GenerateAuthURL generates the OAuth authorization URL
func (c *Client) GenerateAuthURL(redirectURI, state string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("force_auth", "true")
	params.Set("redirect_uri", redirectURI)
	params.Set("client_id", c.AppKey)
	if state != "" {
		params.Set("state", state)
	}

	return "https://auth.lazada.com/oauth/authorize?" + params.Encode()
}

// GetSeller gets seller information
func (c *Client) GetSeller() (map[string]interface{}, error) {
	resp, err := c.Get("/seller/get", nil)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse seller data: %w", err)
	}

	return data, nil
}
