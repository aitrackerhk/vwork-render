package hktvmall

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
	mockResponse := APIResponse{
		Code:    0,
		Message: "Success",
		Data:    json.RawMessage(`{"shop_id": "shop123", "shop_name": "My Shop", "status": "active"}`),
	}
	jsonBytes, _ := json.Marshal(mockResponse)

	// Create client with mock transport
	client := NewClient("merchant", "shop", "app", "secret", "https://api.hktvmall.com")
	client.HTTPClient.Transport = &MockRoundTripper{
		RoundTripFunc: func(req *http.Request) *http.Response {
			// Verify request
			if req.URL.Path != "/api/v1/shop/info" {
				t.Errorf("Expected path /api/v1/shop/info, got %s", req.URL.Path)
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
	if info.ShopID != "shop123" {
		t.Errorf("Expected shop_id shop123, got %s", info.ShopID)
	}
	if info.Status != "active" {
		t.Errorf("Expected status active, got %s", info.Status)
	}
}
