package shipping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FedExService FedEx API service
type FedExService struct {
	ClientID     string
	ClientSecret string
	AccountNo    string
	Environment  string // sandbox or production
	accessToken  string
	tokenExpiry  time.Time
}

// FedExConfig configuration for FedEx
type FedExConfig struct {
	Enabled         bool   `json:"enabled"`
	Environment     string `json:"environment"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
	AccountNo       string `json:"account_no"`
	AutoCreateOrder bool   `json:"auto_create_order"`
	AutoTrack       bool   `json:"auto_track"`
	QueryPrice      bool   `json:"query_price"`
	ServiceType     string `json:"service_type"`
}

// FedExCreateOrderRequest create shipment request
type FedExCreateOrderRequest struct {
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
	ServiceType  string  `json:"service_type"` // FEDEX_INTERNATIONAL_PRIORITY, FEDEX_GROUND, etc.
	Weight       float64 `json:"weight"`
	Length       float64 `json:"length"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	Description  string  `json:"description"`
	CustomerNote string  `json:"customer_note"`
}

// FedExCreateOrderResponse create shipment response
type FedExCreateOrderResponse struct {
	Success        bool    `json:"success"`
	Message        string  `json:"message"`
	ShipmentID     string  `json:"shipment_id"`
	TrackingNumber string  `json:"tracking_number"`
	EstimatedPrice float64 `json:"estimated_price"`
	Currency       string  `json:"currency"`
}

// FedExTrackResponse tracking response
type FedExTrackResponse struct {
	Success  bool              `json:"success"`
	Message  string            `json:"message"`
	Events   []FedExTrackEvent `json:"events"`
	Status   string            `json:"status"`
	Location string            `json:"location"`
}

// FedExTrackEvent individual tracking event
type FedExTrackEvent struct {
	Timestamp   string `json:"timestamp"`
	Location    string `json:"location"`
	Description string `json:"description"`
	StatusCode  string `json:"status_code"`
}

// FedExRateResponse rate query response
type FedExRateResponse struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message"`
	TotalPrice   float64 `json:"total_price"`
	Currency     string  `json:"currency"`
	ServiceName  string  `json:"service_name"`
	DeliveryDays int     `json:"delivery_days"`
}

// NewFedExService create FedEx service instance
func NewFedExService(config FedExConfig) *FedExService {
	return &FedExService{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		AccountNo:    config.AccountNo,
		Environment:  config.Environment,
	}
}

// getBaseURL get API base URL
func (s *FedExService) getBaseURL() string {
	if s.Environment == "production" {
		return "https://apis.fedex.com"
	}
	return "https://apis-sandbox.fedex.com"
}

// getAccessToken obtain OAuth 2.0 access token
func (s *FedExService) getAccessToken() (string, error) {
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		return s.accessToken, nil
	}

	tokenURL := s.getBaseURL() + "/oauth/token"
	body := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s", s.ClientID, s.ClientSecret)

	req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(body))
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
		return "", fmt.Errorf("OAuth token error %d: %s", resp.StatusCode, string(respBody))
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
	// FedEx tokens expire in 1 hour, refresh at 55 min
	s.tokenExpiry = time.Now().Add(55 * time.Minute)

	return token, nil
}

// makeRequest send API request with OAuth token
func (s *FedExService) makeRequest(method, path string, body interface{}) ([]byte, error) {
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
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-locale", "en_US")

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

// CreateOrder create a shipment
func (s *FedExService) CreateOrder(req FedExCreateOrderRequest) (*FedExCreateOrderResponse, error) {
	shipmentReq := map[string]interface{}{
		"labelResponseOptions": "URL_ONLY",
		"requestedShipment": map[string]interface{}{
			"shipper": map[string]interface{}{
				"contact": map[string]interface{}{
					"personName":  req.SenderName,
					"phoneNumber": req.SenderPhone,
				},
				"address": map[string]interface{}{
					"streetLines":         []string{req.SenderAddress},
					"city":                req.SenderCity,
					"stateOrProvinceCode": req.SenderState,
					"postalCode":          req.SenderPostal,
					"countryCode":         req.SenderCountry,
				},
			},
			"recipients": []map[string]interface{}{
				{
					"contact": map[string]interface{}{
						"personName":  req.RecipientName,
						"phoneNumber": req.RecipientPhone,
					},
					"address": map[string]interface{}{
						"streetLines":         []string{req.RecipientAddress},
						"city":                req.RecipientCity,
						"stateOrProvinceCode": req.RecipientState,
						"postalCode":          req.RecipientPostal,
						"countryCode":         req.RecipientCountry,
					},
				},
			},
			"serviceType":   req.ServiceType,
			"packagingType": "YOUR_PACKAGING",
			"pickupType":    "USE_SCHEDULED_PICKUP",
			"shippingChargesPayment": map[string]interface{}{
				"paymentType": "SENDER",
				"payor": map[string]interface{}{
					"responsibleParty": map[string]interface{}{
						"accountNumber": map[string]interface{}{
							"value": s.AccountNo,
						},
					},
				},
			},
			"requestedPackageLineItems": []map[string]interface{}{
				{
					"weight": map[string]interface{}{
						"units": "KG",
						"value": req.Weight,
					},
					"dimensions": map[string]interface{}{
						"length": req.Length,
						"width":  req.Width,
						"height": req.Height,
						"units":  "CM",
					},
				},
			},
		},
		"accountNumber": map[string]interface{}{
			"value": s.AccountNo,
		},
	}

	respBody, err := s.makeRequest("POST", "/ship/v1/shipments", shipmentReq)
	if err != nil {
		return &FedExCreateOrderResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &FedExCreateOrderResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	trackingNumber := ""
	shipmentID := ""
	var estimatedPrice float64

	if output, ok := result["output"].(map[string]interface{}); ok {
		if txnShipments, ok := output["transactionShipments"].([]interface{}); ok && len(txnShipments) > 0 {
			if shipment, ok := txnShipments[0].(map[string]interface{}); ok {
				if sid, ok := shipment["masterTrackingNumber"].(string); ok {
					trackingNumber = sid
					shipmentID = sid
				}
				if pieces, ok := shipment["pieceResponses"].([]interface{}); ok && len(pieces) > 0 {
					if piece, ok := pieces[0].(map[string]interface{}); ok {
						if tn, ok := piece["trackingNumber"].(string); ok {
							trackingNumber = tn
						}
					}
				}
				if charges, ok := shipment["shipmentRateDetail"].(map[string]interface{}); ok {
					if total, ok := charges["totalNetCharge"].(float64); ok {
						estimatedPrice = total
					}
				}
			}
		}
	}

	return &FedExCreateOrderResponse{
		Success:        true,
		Message:        "FedEx shipment created successfully",
		ShipmentID:     shipmentID,
		TrackingNumber: trackingNumber,
		EstimatedPrice: estimatedPrice,
	}, nil
}

// TrackOrder track a shipment
func (s *FedExService) TrackOrder(trackingNumber string) (*FedExTrackResponse, error) {
	trackReq := map[string]interface{}{
		"includeDetailedScans": true,
		"trackingInfo": []map[string]interface{}{
			{
				"trackingNumberInfo": map[string]interface{}{
					"trackingNumber": trackingNumber,
				},
			},
		},
	}

	respBody, err := s.makeRequest("POST", "/track/v1/trackingnumbers", trackReq)
	if err != nil {
		return &FedExTrackResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &FedExTrackResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	var events []FedExTrackEvent
	var latestStatus, latestLocation string

	if output, ok := result["output"].(map[string]interface{}); ok {
		if completeResults, ok := output["completeTrackResults"].([]interface{}); ok && len(completeResults) > 0 {
			if cr, ok := completeResults[0].(map[string]interface{}); ok {
				if trackResults, ok := cr["trackResults"].([]interface{}); ok && len(trackResults) > 0 {
					if tr, ok := trackResults[0].(map[string]interface{}); ok {
						if scanEvents, ok := tr["scanEvents"].([]interface{}); ok {
							for _, se := range scanEvents {
								if evt, ok := se.(map[string]interface{}); ok {
									event := FedExTrackEvent{}
									if ts, ok := evt["date"].(string); ok {
										event.Timestamp = ts
									}
									if desc, ok := evt["eventDescription"].(string); ok {
										event.Description = desc
									}
									if sc, ok := evt["derivedStatusCode"].(string); ok {
										event.StatusCode = sc
									}
									if loc, ok := evt["scanLocation"].(map[string]interface{}); ok {
										city, _ := loc["city"].(string)
										country, _ := loc["countryCode"].(string)
										event.Location = city + ", " + country
									}
									events = append(events, event)
								}
							}
						}
						if latestEvent, ok := tr["latestStatusDetail"].(map[string]interface{}); ok {
							if sc, ok := latestEvent["code"].(string); ok {
								latestStatus = sc
							}
						}
					}
				}
			}
		}
	}

	if len(events) > 0 {
		latestLocation = events[0].Location
	}

	return &FedExTrackResponse{
		Success:  true,
		Message:  "tracking query successful",
		Events:   events,
		Status:   latestStatus,
		Location: latestLocation,
	}, nil
}

// ParseFedExConfigFromJSON parse FedEx config from JSON data
func ParseFedExConfigFromJSON(data map[string]interface{}) *FedExConfig {
	if data == nil {
		return nil
	}

	fedexData, ok := data["fedex"].(map[string]interface{})
	if !ok {
		return nil
	}

	config := &FedExConfig{}

	if enabled, ok := fedexData["enabled"].(bool); ok {
		config.Enabled = enabled
	}
	if env, ok := fedexData["environment"].(string); ok {
		config.Environment = env
	}
	if cid, ok := fedexData["client_id"].(string); ok {
		config.ClientID = cid
	}
	if cs, ok := fedexData["client_secret"].(string); ok {
		config.ClientSecret = cs
	}
	if acc, ok := fedexData["account_no"].(string); ok {
		config.AccountNo = acc
	}
	if auto, ok := fedexData["auto_create_order"].(bool); ok {
		config.AutoCreateOrder = auto
	}
	if track, ok := fedexData["auto_track"].(bool); ok {
		config.AutoTrack = track
	}
	if qp, ok := fedexData["query_price"].(bool); ok {
		config.QueryPrice = qp
	}
	if st, ok := fedexData["service_type"].(string); ok {
		config.ServiceType = st
	}

	return config
}

// MapFedExStatusToShipmentStatus convert FedEx status code to system shipment status
func MapFedExStatusToShipmentStatus(statusCode string) string {
	switch statusCode {
	case "PU": // Picked Up
		return "picked_up"
	case "IT": // In Transit
		return "in_transit"
	case "OD": // Out for Delivery
		return "out_for_delivery"
	case "DL": // Delivered
		return "delivered"
	case "DE", "SE": // Delivery Exception, Shipment Exception
		return "failed"
	case "CA": // Cancelled
		return "cancelled"
	case "RS": // Return to Shipper
		return "returned"
	default:
		return "in_transit"
	}
}

// ToJSON convert config to JSON
func (c *FedExConfig) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}
