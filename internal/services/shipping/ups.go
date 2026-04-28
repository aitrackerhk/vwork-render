package shipping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// UPSService UPS API service
type UPSService struct {
	ClientID     string
	ClientSecret string
	AccountNo    string
	Environment  string // sandbox or production
	accessToken  string
	tokenExpiry  time.Time
}

// UPSConfig configuration for UPS
type UPSConfig struct {
	Enabled         bool   `json:"enabled"`
	Environment     string `json:"environment"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
	AccountNo       string `json:"account_no"`
	AutoCreateOrder bool   `json:"auto_create_order"`
	AutoTrack       bool   `json:"auto_track"`
	QueryPrice      bool   `json:"query_price"`
	ServiceCode     string `json:"service_code"`
}

// UPSCreateOrderRequest create shipment request
type UPSCreateOrderRequest struct {
	// Sender info
	SenderName    string `json:"sender_name"`
	SenderPhone   string `json:"sender_phone"`
	SenderAddress string `json:"sender_address"`
	SenderCity    string `json:"sender_city"`
	SenderState   string `json:"sender_state"`
	SenderPostal  string `json:"sender_postal"`
	SenderCountry string `json:"sender_country"` // ISO 2-letter

	// Recipient info
	RecipientName    string `json:"recipient_name"`
	RecipientPhone   string `json:"recipient_phone"`
	RecipientAddress string `json:"recipient_address"`
	RecipientCity    string `json:"recipient_city"`
	RecipientState   string `json:"recipient_state"`
	RecipientPostal  string `json:"recipient_postal"`
	RecipientCountry string `json:"recipient_country"`

	// Shipment info
	ServiceCode  string  `json:"service_code"` // 01: Next Day Air, 02: 2nd Day Air, 03: Ground, etc.
	Weight       float64 `json:"weight"`       // lbs or kg
	Length       float64 `json:"length"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	Description  string  `json:"description"`
	CustomerNote string  `json:"customer_note"`
}

// UPSCreateOrderResponse create shipment response
type UPSCreateOrderResponse struct {
	Success        bool    `json:"success"`
	Message        string  `json:"message"`
	ShipmentID     string  `json:"shipment_id"`
	TrackingNumber string  `json:"tracking_number"`
	EstimatedPrice float64 `json:"estimated_price"`
	Currency       string  `json:"currency"`
}

// UPSTrackResponse tracking response
type UPSTrackResponse struct {
	Success  bool            `json:"success"`
	Message  string          `json:"message"`
	Events   []UPSTrackEvent `json:"events"`
	Status   string          `json:"status"`
	Location string          `json:"location"`
}

// UPSTrackEvent individual tracking event
type UPSTrackEvent struct {
	Timestamp   string `json:"timestamp"`
	Location    string `json:"location"`
	Description string `json:"description"`
	StatusCode  string `json:"status_code"`
}

// UPSRateResponse rate query response
type UPSRateResponse struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message"`
	TotalPrice   float64 `json:"total_price"`
	Currency     string  `json:"currency"`
	ServiceName  string  `json:"service_name"`
	DeliveryDays int     `json:"delivery_days"`
}

// NewUPSService create UPS service instance
func NewUPSService(config UPSConfig) *UPSService {
	return &UPSService{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		AccountNo:    config.AccountNo,
		Environment:  config.Environment,
	}
}

// getBaseURL get API base URL
func (s *UPSService) getBaseURL() string {
	if s.Environment == "production" {
		return "https://onlinetools.ups.com"
	}
	return "https://wwwcie.ups.com"
}

// getAccessToken obtain OAuth 2.0 access token
func (s *UPSService) getAccessToken() (string, error) {
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		return s.accessToken, nil
	}

	tokenURL := s.getBaseURL() + "/security/v1/oauth/token"
	body := "grant_type=client_credentials"

	req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("create token request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.ClientID, s.ClientSecret)

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
	// UPS tokens typically expire in 4 hours, refresh at 3.5h
	s.tokenExpiry = time.Now().Add(210 * time.Minute)

	return token, nil
}

// makeRequest send API request with OAuth token
func (s *UPSService) makeRequest(method, path string, body interface{}) ([]byte, error) {
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
	req.Header.Set("transId", fmt.Sprintf("%d", time.Now().UnixNano()))
	req.Header.Set("transactionSrc", "vwork")

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
func (s *UPSService) CreateOrder(req UPSCreateOrderRequest) (*UPSCreateOrderResponse, error) {
	shipmentReq := map[string]interface{}{
		"ShipmentRequest": map[string]interface{}{
			"Request": map[string]interface{}{
				"SubVersion":    "1801",
				"RequestOption": "nonvalidate",
				"TransactionReference": map[string]interface{}{
					"CustomerContext": "vwork shipment",
				},
			},
			"Shipment": map[string]interface{}{
				"Description": req.Description,
				"Shipper": map[string]interface{}{
					"Name":          req.SenderName,
					"ShipperNumber": s.AccountNo,
					"Phone": map[string]interface{}{
						"Number": req.SenderPhone,
					},
					"Address": map[string]interface{}{
						"AddressLine":       []string{req.SenderAddress},
						"City":              req.SenderCity,
						"StateProvinceCode": req.SenderState,
						"PostalCode":        req.SenderPostal,
						"CountryCode":       req.SenderCountry,
					},
				},
				"ShipTo": map[string]interface{}{
					"Name": req.RecipientName,
					"Phone": map[string]interface{}{
						"Number": req.RecipientPhone,
					},
					"Address": map[string]interface{}{
						"AddressLine":       []string{req.RecipientAddress},
						"City":              req.RecipientCity,
						"StateProvinceCode": req.RecipientState,
						"PostalCode":        req.RecipientPostal,
						"CountryCode":       req.RecipientCountry,
					},
				},
				"PaymentInformation": map[string]interface{}{
					"ShipmentCharge": []map[string]interface{}{
						{
							"Type": "01", // Transportation charges
							"BillShipper": map[string]interface{}{
								"AccountNumber": s.AccountNo,
							},
						},
					},
				},
				"Service": map[string]interface{}{
					"Code":        req.ServiceCode,
					"Description": "UPS Service",
				},
				"Package": []map[string]interface{}{
					{
						"PackagingType": map[string]interface{}{
							"Code":        "02", // Customer Supplied Package
							"Description": "Package",
						},
						"Dimensions": map[string]interface{}{
							"UnitOfMeasurement": map[string]interface{}{
								"Code":        "CM",
								"Description": "Centimeters",
							},
							"Length": fmt.Sprintf("%.0f", req.Length),
							"Width":  fmt.Sprintf("%.0f", req.Width),
							"Height": fmt.Sprintf("%.0f", req.Height),
						},
						"PackageWeight": map[string]interface{}{
							"UnitOfMeasurement": map[string]interface{}{
								"Code":        "KGS",
								"Description": "Kilograms",
							},
							"Weight": fmt.Sprintf("%.1f", req.Weight),
						},
					},
				},
			},
			"LabelSpecification": map[string]interface{}{
				"LabelImageFormat": map[string]interface{}{
					"Code":        "PNG",
					"Description": "PNG",
				},
				"HTTPUserAgent": "Mozilla/5.0",
			},
		},
	}

	respBody, err := s.makeRequest("POST", "/api/shipments/v1801/ship", shipmentReq)
	if err != nil {
		return &UPSCreateOrderResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &UPSCreateOrderResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	// Extract tracking number
	trackingNumber := ""
	shipmentID := ""
	var estimatedPrice float64

	if shipResp, ok := result["ShipmentResponse"].(map[string]interface{}); ok {
		if shipResult, ok := shipResp["ShipmentResults"].(map[string]interface{}); ok {
			if sid, ok := shipResult["ShipmentIdentificationNumber"].(string); ok {
				shipmentID = sid
				trackingNumber = sid
			}
			if packages, ok := shipResult["PackageResults"].([]interface{}); ok && len(packages) > 0 {
				if pkg, ok := packages[0].(map[string]interface{}); ok {
					if tn, ok := pkg["TrackingNumber"].(string); ok {
						trackingNumber = tn
					}
				}
			}
			if charges, ok := shipResult["ShipmentCharges"].(map[string]interface{}); ok {
				if total, ok := charges["TotalCharges"].(map[string]interface{}); ok {
					if val, ok := total["MonetaryValue"].(string); ok {
						fmt.Sscanf(val, "%f", &estimatedPrice)
					}
				}
			}
		}
	}

	return &UPSCreateOrderResponse{
		Success:        true,
		Message:        "UPS shipment created successfully",
		ShipmentID:     shipmentID,
		TrackingNumber: trackingNumber,
		EstimatedPrice: estimatedPrice,
	}, nil
}

// TrackOrder track a shipment
func (s *UPSService) TrackOrder(trackingNumber string) (*UPSTrackResponse, error) {
	path := fmt.Sprintf("/api/track/v1/details/%s", trackingNumber)
	respBody, err := s.makeRequest("GET", path, nil)
	if err != nil {
		return &UPSTrackResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &UPSTrackResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	var events []UPSTrackEvent
	var latestStatus, latestLocation string

	if trackResp, ok := result["trackResponse"].(map[string]interface{}); ok {
		if shipments, ok := trackResp["shipment"].([]interface{}); ok && len(shipments) > 0 {
			if shipment, ok := shipments[0].(map[string]interface{}); ok {
				if packages, ok := shipment["package"].([]interface{}); ok && len(packages) > 0 {
					if pkg, ok := packages[0].(map[string]interface{}); ok {
						if activity, ok := pkg["activity"].([]interface{}); ok {
							for _, a := range activity {
								if act, ok := a.(map[string]interface{}); ok {
									event := UPSTrackEvent{}
									if status, ok := act["status"].(map[string]interface{}); ok {
										if desc, ok := status["description"].(string); ok {
											event.Description = desc
										}
										if sc, ok := status["code"].(string); ok {
											event.StatusCode = sc
										}
									}
									if loc, ok := act["location"].(map[string]interface{}); ok {
										if addr, ok := loc["address"].(map[string]interface{}); ok {
											city, _ := addr["city"].(string)
											country, _ := addr["country"].(string)
											event.Location = city + ", " + country
										}
									}
									if ts, ok := act["date"].(string); ok {
										event.Timestamp = ts
									}
									events = append(events, event)
								}
							}
						}
						if currentStatus, ok := pkg["currentStatus"].(map[string]interface{}); ok {
							if sc, ok := currentStatus["code"].(string); ok {
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

	return &UPSTrackResponse{
		Success:  true,
		Message:  "tracking query successful",
		Events:   events,
		Status:   latestStatus,
		Location: latestLocation,
	}, nil
}

// ParseUPSConfigFromJSON parse UPS config from JSON data
func ParseUPSConfigFromJSON(data map[string]interface{}) *UPSConfig {
	if data == nil {
		return nil
	}

	upsData, ok := data["ups"].(map[string]interface{})
	if !ok {
		return nil
	}

	config := &UPSConfig{}

	if enabled, ok := upsData["enabled"].(bool); ok {
		config.Enabled = enabled
	}
	if env, ok := upsData["environment"].(string); ok {
		config.Environment = env
	}
	if cid, ok := upsData["client_id"].(string); ok {
		config.ClientID = cid
	}
	if cs, ok := upsData["client_secret"].(string); ok {
		config.ClientSecret = cs
	}
	if acc, ok := upsData["account_no"].(string); ok {
		config.AccountNo = acc
	}
	if auto, ok := upsData["auto_create_order"].(bool); ok {
		config.AutoCreateOrder = auto
	}
	if track, ok := upsData["auto_track"].(bool); ok {
		config.AutoTrack = track
	}
	if qp, ok := upsData["query_price"].(bool); ok {
		config.QueryPrice = qp
	}
	if sc, ok := upsData["service_code"].(string); ok {
		config.ServiceCode = sc
	}

	return config
}

// MapUPSStatusToShipmentStatus convert UPS status code to system shipment status
func MapUPSStatusToShipmentStatus(statusCode string) string {
	switch statusCode {
	case "M": // Manifest Pickup
		return "pending"
	case "P": // Pickup
		return "picked_up"
	case "I": // In Transit
		return "in_transit"
	case "O": // Out for Delivery
		return "out_for_delivery"
	case "D": // Delivered
		return "delivered"
	case "X": // Exception
		return "failed"
	case "RS": // Returned to Shipper
		return "returned"
	default:
		return "in_transit"
	}
}

// ToJSON convert config to JSON
func (c *UPSConfig) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}
