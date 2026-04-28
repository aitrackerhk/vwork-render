package amazon

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// MockRoundTripper is a custom RoundTripper for testing
type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) *http.Response
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req), nil
}

func TestClient_GetOrders(t *testing.T) {
	// Setup mock response
	mockResponse := struct {
		Payload struct {
			Orders    []Order `json:"Orders"`
			NextToken string  `json:"NextToken"`
		} `json:"payload"`
	}{
		Payload: struct {
			Orders    []Order `json:"Orders"`
			NextToken string  `json:"NextToken"`
		}{
			Orders: []Order{
				{AmazonOrderID: "123-1234567-1234567", OrderStatus: "Shipped"},
			},
			NextToken: "token123",
		},
	}
	jsonBytes, _ := json.Marshal(mockResponse)

	// Create client with mock transport
	client := NewClient("id", "secret", "refresh", "NA", "US", "awsAccess", "awsSecret", "role")
	// Pre-set access token to avoid refresh call
	client.AccessToken = "valid_access_token"
	client.AccessTokenExp = time.Now().Add(1 * time.Hour)
	client.HTTPClient.Transport = &MockRoundTripper{
		RoundTripFunc: func(req *http.Request) *http.Response {
			// Verify request
			if req.URL.Path != "/orders/v0/orders" {
				t.Errorf("Expected path /orders/v0/orders, got %s", req.URL.Path)
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
	orders, token, err := client.GetOrders(time.Now(), time.Time{}, nil, "")
	if err != nil {
		t.Fatalf("GetOrders failed: %v", err)
	}

	// Verify result
	if len(orders) != 1 {
		t.Errorf("Expected 1 order, got %d", len(orders))
	}
	if orders[0].AmazonOrderID != "123-1234567-1234567" {
		t.Errorf("Expected OrderID 123-1234567-1234567, got %s", orders[0].AmazonOrderID)
	}
	if token != "token123" {
		t.Errorf("Expected NextToken token123, got %s", token)
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

	client := NewClient("id", "secret", "old_refresh", "NA", "US", "awsAccess", "awsSecret", "role")
	client.HTTPClient.Transport = &MockRoundTripper{
		RoundTripFunc: func(req *http.Request) *http.Response {
			if req.URL.String() != LWAEndpoint {
				t.Errorf("Expected URL %s, got %s", LWAEndpoint, req.URL.String())
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
