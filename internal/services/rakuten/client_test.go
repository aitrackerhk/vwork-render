package rakuten

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// MockRoundTripper is a custom RoundTripper for testing
type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) *http.Response
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req), nil
}

func TestClient_GetShopInfo(t *testing.T) {
	// Setup mock response
	mockResponse := map[string]interface{}{
		"status":  "OK",
		"message": "Success",
		"data": map[string]interface{}{
			"shopUrl":  "https://www.rakuten.co.jp/myshop",
			"shopName": "My Shop",
		},
	}
	jsonBytes, _ := json.Marshal(mockResponse)

	// Create client with mock transport
	client := NewClient("secret", "key", "shop", "access", "refresh")
	client.HTTPClient.Transport = &MockRoundTripper{
		RoundTripFunc: func(req *http.Request) *http.Response {
			// Verify request
			if req.URL.Path != "/es/2.0/shop/info" {
				t.Errorf("Expected path /es/2.0/shop/info, got %s", req.URL.Path)
			}
			if req.Method != "GET" {
				t.Errorf("Expected method GET, got %s", req.Method)
			}

			// Return mock response
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
				Header:     make(http.Header),
			}
		},
	}

	// Execute test
	info, err := client.GetShopInfo()
	if err != nil {
		t.Fatalf("GetShopInfo failed: %v", err)
	}

	// Verify result
	if info["shopUrl"] != "https://www.rakuten.co.jp/myshop" {
		t.Errorf("Expected shopUrl https://www.rakuten.co.jp/myshop, got %v", info["shopUrl"])
	}
}

func TestClient_RefreshAccessToken(t *testing.T) {
	// Setup mock response
	mockResponse := TokenResponse{
		AccessToken:  "new_access",
		RefreshToken: "new_refresh",
		ExpiresIn:    3600,
	}
	jsonBytes, _ := json.Marshal(mockResponse)

	client := NewClient("secret", "key", "shop", "old_access", "old_refresh")
	client.HTTPClient.Transport = &MockRoundTripper{
		RoundTripFunc: func(req *http.Request) *http.Response {
			if req.URL.String() != AuthURL {
				t.Errorf("Expected URL %s, got %s", AuthURL, req.URL.String())
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(jsonBytes)),
				Header:     make(http.Header),
			}
		},
	}

	token, err := client.RefreshAccessToken()
	if err != nil {
		t.Fatalf("RefreshAccessToken failed: %v", err)
	}

	if token.AccessToken != "new_access" {
		t.Errorf("Expected access token new_access, got %s", token.AccessToken)
	}
	if client.AccessToken != "new_access" {
		t.Errorf("Client access token not updated")
	}
}
