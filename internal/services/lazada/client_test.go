package lazada

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

func TestClient_GetSeller(t *testing.T) {
	// Setup mock response
	mockResponse := APIResponse{
		Code:    "0",
		Message: "Success",
		Data:    json.RawMessage(`{"seller_id": "12345", "name": "Lazada Shop"}`),
	}
	jsonBytes, _ := json.Marshal(mockResponse)

	// Create client with mock transport
	client := NewClient("appKey", "appSecret", "access", "refresh", "SG")
	client.HTTPClient.Transport = &MockRoundTripper{
		RoundTripFunc: func(req *http.Request) *http.Response {
			// Verify request
			if req.URL.Path != "/rest/seller/get" {
				t.Errorf("Expected path /rest/seller/get, got %s", req.URL.Path)
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
	seller, err := client.GetSeller()
	if err != nil {
		t.Fatalf("GetSeller failed: %v", err)
	}

	// Verify result
	if seller["seller_id"] != "12345" {
		t.Errorf("Expected seller_id 12345, got %v", seller["seller_id"])
	}
}

func TestClient_RefreshAccessToken(t *testing.T) {
	// Setup mock response
	mockData := TokenResponse{
		AccessToken:  "new_access",
		RefreshToken: "new_refresh",
		ExpiresIn:    3600,
	}

	mockResponse := struct {
		Code      string        `json:"code"`
		Message   string        `json:"message"`
		RequestID string        `json:"request_id"`
		Data      TokenResponse `json:"data"`
	}{
		Code:    "0",
		Message: "Success",
		Data:    mockData,
	}
	jsonBytes, _ := json.Marshal(mockResponse)

	client := NewClient("appKey", "appSecret", "old_access", "old_refresh", "SG")
	client.HTTPClient.Transport = &MockRoundTripper{
		RoundTripFunc: func(req *http.Request) *http.Response {
			if req.URL.Path != "/rest/auth/token/refresh" {
				t.Errorf("Expected path /rest/auth/token/refresh, got %s", req.URL.Path)
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
