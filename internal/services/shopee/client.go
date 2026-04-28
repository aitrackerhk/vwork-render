package shopee

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
	"strconv"
	"strings"
	"time"
)

// API Endpoints by region
var RegionEndpoints = map[string]string{
	"TW": "https://partner.shopeemobile.com",
	"SG": "https://partner.shopeemobile.com",
	"MY": "https://partner.shopeemobile.com",
	"TH": "https://partner.shopeemobile.com",
	"PH": "https://partner.shopeemobile.com",
	"VN": "https://partner.shopeemobile.com",
	"ID": "https://partner.shopeemobile.com",
	"BR": "https://partner.shopeemobile.com",
}

// Client represents a Shopee API client
type Client struct {
	PartnerID    int64
	PartnerKey   string
	ShopID       int64
	AccessToken  string
	RefreshToken string
	Region       string
	BaseURL      string
	HTTPClient   *http.Client
}

// NewClient creates a new Shopee API client
func NewClient(partnerID int64, partnerKey string, shopID int64, accessToken, refreshToken, region string) *Client {
	baseURL := RegionEndpoints[region]
	if baseURL == "" {
		baseURL = RegionEndpoints["SG"] // default
	}

	return &Client{
		PartnerID:    partnerID,
		PartnerKey:   partnerKey,
		ShopID:       shopID,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Region:       region,
		BaseURL:      baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GenerateSign generates the HMAC-SHA256 signature for API requests
func (c *Client) GenerateSign(path string, timestamp int64) string {
	baseString := fmt.Sprintf("%d%s%d", c.PartnerID, path, timestamp)
	if c.AccessToken != "" {
		baseString = fmt.Sprintf("%d%s%d%s%d", c.PartnerID, path, timestamp, c.AccessToken, c.ShopID)
	}

	h := hmac.New(sha256.New, []byte(c.PartnerKey))
	h.Write([]byte(baseString))
	return hex.EncodeToString(h.Sum(nil))
}

// BuildURL builds the full API URL with authentication parameters
func (c *Client) BuildURL(path string, params map[string]string) string {
	timestamp := time.Now().Unix()
	sign := c.GenerateSign(path, timestamp)

	u, _ := url.Parse(c.BaseURL + path)
	q := u.Query()
	q.Set("partner_id", strconv.FormatInt(c.PartnerID, 10))
	q.Set("timestamp", strconv.FormatInt(timestamp, 10))
	q.Set("sign", sign)

	if c.AccessToken != "" {
		q.Set("access_token", c.AccessToken)
		q.Set("shop_id", strconv.FormatInt(c.ShopID, 10))
	}

	for k, v := range params {
		q.Set(k, v)
	}

	u.RawQuery = q.Encode()
	return u.String()
}

// APIResponse represents a generic Shopee API response
type APIResponse struct {
	Error    string          `json:"error"`
	Message  string          `json:"message"`
	Response json.RawMessage `json:"response"`
	Warning  string          `json:"warning"`
}

// DoRequest performs an HTTP request to the Shopee API
func (c *Client) DoRequest(method, path string, params map[string]string, body interface{}) (*APIResponse, error) {
	fullURL := c.BuildURL(path, params)

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = strings.NewReader(string(jsonBody))
	}

	req, err := http.NewRequest(method, fullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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

	if apiResp.Error != "" {
		return &apiResp, fmt.Errorf("API error: %s - %s", apiResp.Error, apiResp.Message)
	}

	return &apiResp, nil
}

// Get performs a GET request
func (c *Client) Get(path string, params map[string]string) (*APIResponse, error) {
	return c.DoRequest("GET", path, params, nil)
}

// Post performs a POST request
func (c *Client) Post(path string, params map[string]string, body interface{}) (*APIResponse, error) {
	return c.DoRequest("POST", path, params, body)
}

// GenerateAuthURL generates the OAuth authorization URL
func (c *Client) GenerateAuthURL(redirectURL string) string {
	timestamp := time.Now().Unix()
	path := "/api/v2/shop/auth_partner"

	baseString := fmt.Sprintf("%d%s%d", c.PartnerID, path, timestamp)
	h := hmac.New(sha256.New, []byte(c.PartnerKey))
	h.Write([]byte(baseString))
	sign := hex.EncodeToString(h.Sum(nil))

	params := url.Values{}
	params.Set("partner_id", strconv.FormatInt(c.PartnerID, 10))
	params.Set("timestamp", strconv.FormatInt(timestamp, 10))
	params.Set("sign", sign)
	params.Set("redirect", redirectURL)

	return fmt.Sprintf("%s%s?%s", c.BaseURL, path, params.Encode())
}

// SortMapKeys returns sorted keys of a map
func SortMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
