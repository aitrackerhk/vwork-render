package shipping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AmazonShippingService Amazon Shipping API service
type AmazonShippingService struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	Region       string // NA, EU, FE
	Environment  string // sandbox or production
	accessToken  string
	tokenExpiry  time.Time
}

// AmazonShippingConfig configuration for Amazon Shipping
type AmazonShippingConfig struct {
	Enabled         bool   `json:"enabled"`
	Environment     string `json:"environment"`
	Region          string `json:"region"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
	RefreshToken    string `json:"refresh_token"`
	AutoCreateOrder bool   `json:"auto_create_order"`
	AutoTrack       bool   `json:"auto_track"`
	QueryPrice      bool   `json:"query_price"`
	ServiceType     string `json:"service_type"`
}

// AmazonCreateOrderRequest create shipment request
type AmazonCreateOrderRequest struct {
	// Sender info
	SenderName    string `json:"sender_name"`
	SenderPhone   string `json:"sender_phone"`
	SenderAddress string `json:"sender_address"`
	SenderCity    string `json:"sender_city"`
	SenderState   string `json:"sender_state"`
	SenderPostal  string `json:"sender_postal"`
	SenderCountry string `json:"sender_country"`

	// Recipient info
	RecipientName    string `json:"recipient_name"`
	RecipientPhone   string `json:"recipient_phone"`
	RecipientAddress string `json:"recipient_address"`
	RecipientCity    string `json:"recipient_city"`
	RecipientState   string `json:"recipient_state"`
	RecipientPostal  string `json:"recipient_postal"`
	RecipientCountry string `json:"recipient_country"`

	// Shipment info
	ServiceType  string  `json:"service_type"`
	Weight       float64 `json:"weight"` // grams
	Length       float64 `json:"length"` // cm
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	Description  string  `json:"description"`
	CustomerNote string  `json:"customer_note"`
}

// AmazonCreateOrderResponse create shipment response
type AmazonCreateOrderResponse struct {
	Success        bool    `json:"success"`
	Message        string  `json:"message"`
	ShipmentID     string  `json:"shipment_id"`
	TrackingNumber string  `json:"tracking_number"`
	EstimatedPrice float64 `json:"estimated_price"`
	Currency       string  `json:"currency"`
}

// AmazonTrackResponse tracking response
type AmazonTrackResponse struct {
	Success  bool               `json:"success"`
	Message  string             `json:"message"`
	Events   []AmazonTrackEvent `json:"events"`
	Status   string             `json:"status"`
	Location string             `json:"location"`
}

// AmazonTrackEvent individual tracking event
type AmazonTrackEvent struct {
	Timestamp   string `json:"timestamp"`
	Location    string `json:"location"`
	Description string `json:"description"`
	StatusCode  string `json:"status_code"`
}

// NewAmazonShippingService create Amazon Shipping service instance
func NewAmazonShippingService(config AmazonShippingConfig) *AmazonShippingService {
	return &AmazonShippingService{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RefreshToken: config.RefreshToken,
		Region:       config.Region,
		Environment:  config.Environment,
	}
}

// getBaseURL get API base URL based on region
func (s *AmazonShippingService) getBaseURL() string {
	if s.Environment == "sandbox" {
		return "https://sandbox.sellingpartnerapi-na.amazon.com"
	}
	switch s.Region {
	case "EU":
		return "https://sellingpartnerapi-eu.amazon.com"
	case "FE":
		return "https://sellingpartnerapi-fe.amazon.com"
	default: // NA
		return "https://sellingpartnerapi-na.amazon.com"
	}
}

// getTokenURL get LWA token URL
func (s *AmazonShippingService) getTokenURL() string {
	return "https://api.amazon.com/auth/o2/token"
}

// getAccessToken obtain LWA access token via refresh token
func (s *AmazonShippingService) getAccessToken() (string, error) {
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		return s.accessToken, nil
	}

	body := fmt.Sprintf("grant_type=refresh_token&client_id=%s&client_secret=%s&refresh_token=%s",
		s.ClientID, s.ClientSecret, s.RefreshToken)

	req, err := http.NewRequest("POST", s.getTokenURL(), bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("create token request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("LWA token error %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp map[string]interface{}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("parse token response failed: %w", err)
	}

	token, _ := tokenResp["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("no access token in response")
	}

	s.accessToken = token
	// Amazon LWA tokens expire in 1 hour, refresh at 55 min
	s.tokenExpiry = time.Now().Add(55 * time.Minute)

	return token, nil
}

// makeRequest send API request with access token
func (s *AmazonShippingService) makeRequest(method, path string, body interface{}) ([]byte, error) {
	token, err := s.getAccessToken()
	if err != nil {
		return nil, fmt.Errorf("get access token failed: %w", err)
	}

	var bodyBytes []byte
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("serialize request failed: %w", err)
		}
	}

	fullURL := s.getBaseURL() + path
	req, err := http.NewRequest(method, fullURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-amz-access-token", token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// CreateOrder create a shipment via Amazon Shipping V2
func (s *AmazonShippingService) CreateOrder(req AmazonCreateOrderRequest) (*AmazonCreateOrderResponse, error) {
	// Convert weight from kg to grams for Amazon API
	weightGrams := req.Weight * 1000

	shipmentReq := map[string]interface{}{
		"clientReferenceDetails": map[string]interface{}{
			"clientReferenceType": "IntegratorShipperId",
			"clientReferenceId":   fmt.Sprintf("vwork-%d", time.Now().UnixNano()),
		},
		"shipFrom": map[string]interface{}{
			"name":          req.SenderName,
			"phoneNumber":   req.SenderPhone,
			"addressLine1":  req.SenderAddress,
			"city":          req.SenderCity,
			"stateOrRegion": req.SenderState,
			"postalCode":    req.SenderPostal,
			"countryCode":   req.SenderCountry,
		},
		"shipTo": map[string]interface{}{
			"name":          req.RecipientName,
			"phoneNumber":   req.RecipientPhone,
			"addressLine1":  req.RecipientAddress,
			"city":          req.RecipientCity,
			"stateOrRegion": req.RecipientState,
			"postalCode":    req.RecipientPostal,
			"countryCode":   req.RecipientCountry,
		},
		"packages": []map[string]interface{}{
			{
				"dimensions": map[string]interface{}{
					"length": req.Length,
					"width":  req.Width,
					"height": req.Height,
					"unit":   "CM",
				},
				"weight": map[string]interface{}{
					"value": weightGrams,
					"unit":  "GRAM",
				},
				"items": []map[string]interface{}{
					{
						"description": req.Description,
						"quantity":    1,
					},
				},
			},
		},
	}

	if req.ServiceType != "" {
		shipmentReq["channelDetails"] = map[string]interface{}{
			"channelType": req.ServiceType,
		}
	}

	respBody, err := s.makeRequest("POST", "/shipping/v2/shipments", shipmentReq)
	if err != nil {
		return &AmazonCreateOrderResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &AmazonCreateOrderResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	trackingNumber := ""
	shipmentID := ""
	var estimatedPrice float64

	if payload, ok := result["payload"].(map[string]interface{}); ok {
		if sid, ok := payload["shipmentId"].(string); ok {
			shipmentID = sid
		}
		if tn, ok := payload["trackingId"].(string); ok {
			trackingNumber = tn
		}
		if promise, ok := payload["promise"].(map[string]interface{}); ok {
			if totalCharge, ok := promise["totalCharge"].(map[string]interface{}); ok {
				if val, ok := totalCharge["value"].(float64); ok {
					estimatedPrice = val
				}
			}
		}
	}

	return &AmazonCreateOrderResponse{
		Success:        true,
		Message:        "Amazon Shipping shipment created successfully",
		ShipmentID:     shipmentID,
		TrackingNumber: trackingNumber,
		EstimatedPrice: estimatedPrice,
	}, nil
}

// TrackOrder track a shipment
func (s *AmazonShippingService) TrackOrder(trackingID string) (*AmazonTrackResponse, error) {
	path := fmt.Sprintf("/shipping/v2/tracking?trackingId=%s", trackingID)
	respBody, err := s.makeRequest("GET", path, nil)
	if err != nil {
		return &AmazonTrackResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &AmazonTrackResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	var events []AmazonTrackEvent
	var latestStatus, latestLocation string

	if payload, ok := result["payload"].(map[string]interface{}); ok {
		if summary, ok := payload["summary"].(map[string]interface{}); ok {
			if sc, ok := summary["status"].(string); ok {
				latestStatus = sc
			}
		}
		if eventHistory, ok := payload["eventHistory"].([]interface{}); ok {
			for _, e := range eventHistory {
				if evt, ok := e.(map[string]interface{}); ok {
					event := AmazonTrackEvent{}
					if ts, ok := evt["eventTime"].(string); ok {
						event.Timestamp = ts
					}
					if desc, ok := evt["eventCode"].(string); ok {
						event.Description = desc
						event.StatusCode = desc
					}
					if loc, ok := evt["location"].(map[string]interface{}); ok {
						city, _ := loc["city"].(string)
						country, _ := loc["countryCode"].(string)
						event.Location = city + ", " + country
					}
					events = append(events, event)
				}
			}
		}
	}

	if len(events) > 0 {
		latestLocation = events[0].Location
	}

	return &AmazonTrackResponse{
		Success:  true,
		Message:  "tracking query successful",
		Events:   events,
		Status:   latestStatus,
		Location: latestLocation,
	}, nil
}

// ParseAmazonShippingConfigFromJSON parse Amazon Shipping config from JSON data
func ParseAmazonShippingConfigFromJSON(data map[string]interface{}) *AmazonShippingConfig {
	if data == nil {
		return nil
	}

	amazonData, ok := data["amazon"].(map[string]interface{})
	if !ok {
		return nil
	}

	config := &AmazonShippingConfig{}

	if enabled, ok := amazonData["enabled"].(bool); ok {
		config.Enabled = enabled
	}
	if env, ok := amazonData["environment"].(string); ok {
		config.Environment = env
	}
	if region, ok := amazonData["region"].(string); ok {
		config.Region = region
	}
	if cid, ok := amazonData["client_id"].(string); ok {
		config.ClientID = cid
	}
	if cs, ok := amazonData["client_secret"].(string); ok {
		config.ClientSecret = cs
	}
	if rt, ok := amazonData["refresh_token"].(string); ok {
		config.RefreshToken = rt
	}
	if auto, ok := amazonData["auto_create_order"].(bool); ok {
		config.AutoCreateOrder = auto
	}
	if track, ok := amazonData["auto_track"].(bool); ok {
		config.AutoTrack = track
	}
	if qp, ok := amazonData["query_price"].(bool); ok {
		config.QueryPrice = qp
	}
	if st, ok := amazonData["service_type"].(string); ok {
		config.ServiceType = st
	}

	return config
}

// MapAmazonStatusToShipmentStatus convert Amazon Shipping status to system shipment status
func MapAmazonStatusToShipmentStatus(status string) string {
	switch status {
	case "PreTransit":
		return "pending"
	case "InTransit":
		return "in_transit"
	case "OutForDelivery":
		return "out_for_delivery"
	case "Delivered":
		return "delivered"
	case "AttemptFail":
		return "failed"
	case "Cancelled":
		return "cancelled"
	case "Returning", "Returned":
		return "returned"
	default:
		return "in_transit"
	}
}

// ToJSON convert config to JSON
func (c *AmazonShippingConfig) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}
