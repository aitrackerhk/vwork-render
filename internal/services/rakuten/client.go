package rakuten

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// API Endpoints for Rakuten Ichiba (Japan)
const (
	BaseURL     = "https://api.rms.rakuten.co.jp/es/2.0"
	AuthURL     = "https://api.rms.rakuten.co.jp/es/2.0/auth/token"
	InventoryURL = "https://api.rms.rakuten.co.jp/es/2.0/inventory"
	ItemURL     = "https://api.rms.rakuten.co.jp/es/2.0/items"
	OrderURL    = "https://api.rms.rakuten.co.jp/es/2.0/order"
)

// Client represents a Rakuten RMS API client
type Client struct {
	ServiceSecret string
	LicenseKey    string
	ShopURL       string
	AccessToken   string
	RefreshToken  string
	HTTPClient    *http.Client
}

// NewClient creates a new Rakuten API client
func NewClient(serviceSecret, licenseKey, shopURL, accessToken, refreshToken string) *Client {
	return &Client{
		ServiceSecret: serviceSecret,
		LicenseKey:    licenseKey,
		ShopURL:       shopURL,
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
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
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
}

// APIResponse represents a generic API response
type APIResponse struct {
	Status    string          `json:"status"`
	Message   string          `json:"message"`
	RequestID string          `json:"requestId"`
	Data      json.RawMessage `json:"data,omitempty"`
	Errors    []APIError      `json:"errors,omitempty"`
}

// APIError represents an API error
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// generateSignature generates HMAC-SHA256 signature for API request
func (c *Client) generateSignature(method, path string, timestamp string, params map[string]string) string {
	// Sort parameters by key
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build sign string
	var signBuilder strings.Builder
	signBuilder.WriteString(method)
	signBuilder.WriteString("\n")
	signBuilder.WriteString(path)
	signBuilder.WriteString("\n")
	signBuilder.WriteString(timestamp)
	signBuilder.WriteString("\n")

	for i, k := range keys {
		if i > 0 {
			signBuilder.WriteString("&")
		}
		signBuilder.WriteString(k)
		signBuilder.WriteString("=")
		signBuilder.WriteString(url.QueryEscape(params[k]))
	}

	// HMAC-SHA256
	h := hmac.New(sha256.New, []byte(c.ServiceSecret))
	h.Write([]byte(signBuilder.String()))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// buildAuthHeader builds the authorization header
func (c *Client) buildAuthHeader() string {
	// Rakuten uses Base64 encoded serviceSecret:licenseKey
	credentials := c.ServiceSecret + ":" + c.LicenseKey
	return "ESA " + base64.StdEncoding.EncodeToString([]byte(credentials))
}

// DoRequest executes an API request
func (c *Client) DoRequest(method, apiPath string, params map[string]string, body interface{}) (*APIResponse, error) {
	reqURL := BaseURL + apiPath

	// Build query string for GET requests
	if method == "GET" && len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		reqURL += "?" + values.Encode()
	}

	var req *http.Request
	var err error

	if method == "POST" || method == "PUT" || method == "PATCH" {
		var bodyReader io.Reader
		if body != nil {
			jsonBody, _ := json.Marshal(body)
			bodyReader = strings.NewReader(string(jsonBody))
		}
		req, err = http.NewRequest(method, reqURL, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	} else {
		req, err = http.NewRequest(method, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
	}

	// Set headers
	req.Header.Set("Authorization", c.buildAuthHeader())
	req.Header.Set("Accept", "application/json")

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
		// Try to return raw response if parsing fails
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(respBody))
	}

	if resp.StatusCode >= 400 || apiResp.Status == "error" {
		errMsg := apiResp.Message
		if len(apiResp.Errors) > 0 {
			errMsg = apiResp.Errors[0].Message
		}
		return &apiResp, fmt.Errorf("API error: %s", errMsg)
	}

	return &apiResp, nil
}

// Get performs a GET request
func (c *Client) Get(apiPath string, params map[string]string) (*APIResponse, error) {
	return c.DoRequest("GET", apiPath, params, nil)
}

// Post performs a POST request
func (c *Client) Post(apiPath string, body interface{}) (*APIResponse, error) {
	return c.DoRequest("POST", apiPath, nil, body)
}

// Put performs a PUT request
func (c *Client) Put(apiPath string, body interface{}) (*APIResponse, error) {
	return c.DoRequest("PUT", apiPath, nil, body)
}

// Patch performs a PATCH request
func (c *Client) Patch(apiPath string, body interface{}) (*APIResponse, error) {
	return c.DoRequest("PATCH", apiPath, nil, body)
}

// Delete performs a DELETE request
func (c *Client) Delete(apiPath string) (*APIResponse, error) {
	return c.DoRequest("DELETE", apiPath, nil, nil)
}

// RefreshAccessToken refreshes the access token
func (c *Client) RefreshAccessToken() (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.RefreshToken)

	req, err := http.NewRequest("POST", AuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", c.buildAuthHeader())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Update client tokens
	c.AccessToken = tokenResp.AccessToken
	c.RefreshToken = tokenResp.RefreshToken

	return &tokenResp, nil
}

// ExchangeAuthCode exchanges authorization code for tokens
func (c *Client) ExchangeAuthCode(code, redirectURI string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", AuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", c.buildAuthHeader())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.AccessToken = tokenResp.AccessToken
	c.RefreshToken = tokenResp.RefreshToken

	return &tokenResp, nil
}

// GenerateAuthURL generates the OAuth authorization URL
func (c *Client) GenerateAuthURL(redirectURI, state string) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", c.ServiceSecret)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", "rakuten_ichiba")
	if state != "" {
		params.Set("state", state)
	}

	return "https://login.rms.rakuten.co.jp/oauth/authorize?" + params.Encode()
}

// GetShopInfo gets shop information
func (c *Client) GetShopInfo() (map[string]interface{}, error) {
	resp, err := c.Get("/shop/info", nil)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to parse shop info: %w", err)
	}

	return data, nil
}

// GetAuthURL returns the OAuth authorization URL
func (c *Client) GetAuthURL(state string) string {
	return c.GenerateAuthURL("", state)
}

// ExchangeCode exchanges authorization code for tokens
func (c *Client) ExchangeCode(code string) error {
	tokenResp, err := c.ExchangeAuthCode(code, "")
	if err != nil {
		return err
	}
	c.AccessToken = tokenResp.AccessToken
	c.RefreshToken = tokenResp.RefreshToken
	return nil
}

// GetAccessToken returns the current access token
func (c *Client) GetAccessToken() string {
	return c.AccessToken
}

// GetRefreshToken returns the current refresh token
func (c *Client) GetRefreshToken() string {
	return c.RefreshToken
}

// GetTokenExpiry returns a default expiry time (1 hour from now)
func (c *Client) GetTokenExpiry() time.Time {
	return time.Now().Add(time.Hour)
}
