package amazon

import (
	"bytes"
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

// Regional endpoints for Amazon SP-API
var RegionEndpoints = map[string]string{
	"NA": "https://sellingpartnerapi-na.amazon.com", // North America (US, CA, MX, BR)
	"EU": "https://sellingpartnerapi-eu.amazon.com", // Europe (UK, DE, FR, IT, ES, NL, SE, PL, TR, AE, IN)
	"FE": "https://sellingpartnerapi-fe.amazon.com", // Far East (JP, AU, SG)
}

// LWA (Login with Amazon) endpoints
const (
	LWAEndpoint = "https://api.amazon.com/auth/o2/token"
)

// Marketplace IDs
var MarketplaceIDs = map[string]string{
	// North America
	"US": "ATVPDKIKX0DER",
	"CA": "A2EUQ1WTGCTBG2",
	"MX": "A1AM78C64UM0Y8",
	"BR": "A2Q3Y263D00KWC",
	// Europe
	"UK": "A1F83G8C2ARO7P",
	"DE": "A1PA6795UKMFR9",
	"FR": "A13V1IB3VIYBER",
	"IT": "APJ6JRA9NG5V4",
	"ES": "A1RKKUPIHCS9HS",
	"NL": "A1805IZSGTT6HS",
	"SE": "A2NODRKZP88ZB9",
	"PL": "A1C3SOZRARQ6R3",
	"TR": "A33AVAJ2PDY3EV",
	"AE": "A2VIGQ35RCS4UG",
	"IN": "A21TJRUUN4KGV",
	// Far East
	"JP": "A1VC38T7YXB528",
	"AU": "A39IBJ37TRP1C6",
	"SG": "A19VAU5U5O7RUS",
	// Taiwan (using SG marketplace)
	"TW": "A19VAU5U5O7RUS",
}

// Client represents an Amazon SP-API client
type Client struct {
	ClientID       string
	ClientSecret   string
	RefreshToken   string
	AccessToken    string
	AccessTokenExp time.Time
	AWSAccessKey   string
	AWSSecretKey   string
	RoleARN        string
	Region         string
	MarketplaceID  string
	BaseURL        string
	HTTPClient     *http.Client
}

// NewClient creates a new Amazon SP-API client
func NewClient(clientID, clientSecret, refreshToken, region, marketplaceID, awsAccessKey, awsSecretKey, roleARN string) *Client {
	baseURL := RegionEndpoints["NA"] // default
	if endpoint, ok := RegionEndpoints[region]; ok {
		baseURL = endpoint
	} else {
		// Map country code to region
		switch region {
		case "US", "CA", "MX", "BR":
			baseURL = RegionEndpoints["NA"]
		case "UK", "DE", "FR", "IT", "ES", "NL", "SE", "PL", "TR", "AE", "IN":
			baseURL = RegionEndpoints["EU"]
		case "JP", "AU", "SG", "TW":
			baseURL = RegionEndpoints["FE"]
		}
	}

	// Use provided marketplace ID or default based on region
	if marketplaceID == "" {
		marketplaceID = MarketplaceIDs[region]
		if marketplaceID == "" {
			marketplaceID = MarketplaceIDs["US"] // default
		}
	}

	return &Client{
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		RefreshToken:  refreshToken,
		AWSAccessKey:  awsAccessKey,
		AWSSecretKey:  awsSecretKey,
		RoleARN:       roleARN,
		Region:        region,
		MarketplaceID: marketplaceID,
		BaseURL:       baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TokenResponse represents LWA token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// RefreshAccessToken refreshes the LWA access token
func (c *Client) RefreshAccessToken() (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", c.RefreshToken)
	data.Set("client_id", c.ClientID)
	data.Set("client_secret", c.ClientSecret)

	req, err := http.NewRequest("POST", LWAEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	c.AccessToken = tokenResp.AccessToken
	c.AccessTokenExp = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return &tokenResp, nil
}

// EnsureAccessToken ensures we have a valid access token
func (c *Client) EnsureAccessToken() error {
	if c.AccessToken != "" && time.Now().Before(c.AccessTokenExp.Add(-5*time.Minute)) {
		return nil // Token still valid
	}

	_, err := c.RefreshAccessToken()
	return err
}

// signRequest signs the request with AWS Signature Version 4
func (c *Client) signRequest(req *http.Request, payload []byte) error {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	// Get AWS region from endpoint
	awsRegion := "us-east-1"
	if strings.Contains(c.BaseURL, "-eu.") {
		awsRegion = "eu-west-1"
	} else if strings.Contains(c.BaseURL, "-fe.") {
		awsRegion = "us-west-2"
	}

	service := "execute-api"

	// Create canonical request
	method := req.Method
	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	// Canonical query string
	canonicalQueryString := req.URL.RawQuery

	// Set required headers
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("host", req.URL.Host)

	// Build signed headers
	signedHeaders := make([]string, 0)
	headerMap := make(map[string]string)
	for key := range req.Header {
		lowerKey := strings.ToLower(key)
		signedHeaders = append(signedHeaders, lowerKey)
		headerMap[lowerKey] = strings.TrimSpace(req.Header.Get(key))
	}
	sort.Strings(signedHeaders)

	canonicalHeaders := ""
	for _, key := range signedHeaders {
		canonicalHeaders += key + ":" + headerMap[key] + "\n"
	}

	signedHeadersStr := strings.Join(signedHeaders, ";")

	// Payload hash
	payloadHash := sha256Hash(payload)

	// Canonical request
	canonicalRequest := method + "\n" +
		canonicalURI + "\n" +
		canonicalQueryString + "\n" +
		canonicalHeaders + "\n" +
		signedHeadersStr + "\n" +
		payloadHash

	// String to sign
	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := dateStamp + "/" + awsRegion + "/" + service + "/aws4_request"
	stringToSign := algorithm + "\n" +
		amzDate + "\n" +
		credentialScope + "\n" +
		sha256Hash([]byte(canonicalRequest))

	// Calculate signature
	signingKey := getSignatureKey(c.AWSSecretKey, dateStamp, awsRegion, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// Authorization header
	authHeader := algorithm + " " +
		"Credential=" + c.AWSAccessKey + "/" + credentialScope + ", " +
		"SignedHeaders=" + signedHeadersStr + ", " +
		"Signature=" + signature

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("x-amz-access-token", c.AccessToken)

	return nil
}

// DoRequest performs an HTTP request to the SP-API
func (c *Client) DoRequest(method, path string, params map[string]string, body interface{}) ([]byte, error) {
	if err := c.EnsureAccessToken(); err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	fullURL, _ := url.Parse(c.BaseURL + path)
	q := fullURL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	fullURL.RawQuery = q.Encode()

	var reqBody []byte
	var bodyReader io.Reader
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(reqBody)
	}

	req, err := http.NewRequest(method, fullURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if err := c.signRequest(req, reqBody); err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
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

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Get performs a GET request
func (c *Client) Get(path string, params map[string]string) ([]byte, error) {
	return c.DoRequest("GET", path, params, nil)
}

// Post performs a POST request
func (c *Client) Post(path string, params map[string]string, body interface{}) ([]byte, error) {
	return c.DoRequest("POST", path, params, body)
}

// Patch performs a PATCH request
func (c *Client) Patch(path string, params map[string]string, body interface{}) ([]byte, error) {
	return c.DoRequest("PATCH", path, params, body)
}

// Helper functions
func sha256Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func getSignatureKey(key, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+key), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

// GenerateAuthURL generates the LWA authorization URL for OAuth
func (c *Client) GenerateAuthURL(redirectURI, state string) string {
	params := url.Values{}
	params.Set("application_id", c.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)

	return "https://sellercentral.amazon.com/apps/authorize/consent?" + params.Encode()
}

// ExchangeAuthCode exchanges authorization code for tokens
func (c *Client) ExchangeAuthCode(code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", c.ClientID)
	data.Set("client_secret", c.ClientSecret)

	req, err := http.NewRequest("POST", LWAEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	c.AccessToken = tokenResp.AccessToken
	c.RefreshToken = tokenResp.RefreshToken
	c.AccessTokenExp = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return &tokenResp, nil
}
