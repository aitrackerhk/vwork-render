package shipping

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DHLService DHL Express API service
type DHLService struct {
	APIKey      string
	APISecret   string
	Environment string // sandbox or production
	AccountNo   string
}

// DHLConfig configuration for DHL Express
type DHLConfig struct {
	Enabled         bool   `json:"enabled"`
	Environment     string `json:"environment"`
	APIKey          string `json:"api_key"`
	APISecret       string `json:"api_secret"`
	AccountNo       string `json:"account_no"`
	AutoCreateOrder bool   `json:"auto_create_order"`
	AutoTrack       bool   `json:"auto_track"`
	QueryPrice      bool   `json:"query_price"`
	ProductCode     string `json:"product_code"`
}

// DHLCreateOrderRequest create shipment request
type DHLCreateOrderRequest struct {
	// Sender info
	SenderName    string `json:"sender_name"`
	SenderPhone   string `json:"sender_phone"`
	SenderAddress string `json:"sender_address"`
	SenderCity    string `json:"sender_city"`
	SenderPostal  string `json:"sender_postal"`
	SenderCountry string `json:"sender_country"` // ISO 2-letter country code

	// Recipient info
	RecipientName    string `json:"recipient_name"`
	RecipientPhone   string `json:"recipient_phone"`
	RecipientAddress string `json:"recipient_address"`
	RecipientCity    string `json:"recipient_city"`
	RecipientPostal  string `json:"recipient_postal"`
	RecipientCountry string `json:"recipient_country"` // ISO 2-letter country code

	// Shipment info
	ProductCode   string  `json:"product_code"` // P: Express Worldwide, D: Express Worldwide Doc, etc.
	Weight        float64 `json:"weight"`       // kg
	Length        float64 `json:"length"`       // cm
	Width         float64 `json:"width"`        // cm
	Height        float64 `json:"height"`       // cm
	Description   string  `json:"description"`
	CustomerNote  string  `json:"customer_note"`
	DeclaredValue float64 `json:"declared_value"`
	Currency      string  `json:"currency"` // e.g. HKD, USD
}

// DHLCreateOrderResponse create shipment response
type DHLCreateOrderResponse struct {
	Success        bool    `json:"success"`
	Message        string  `json:"message"`
	ShipmentID     string  `json:"shipment_id"`
	TrackingNumber string  `json:"tracking_number"`
	DispatchNumber string  `json:"dispatch_number"`
	EstimatedPrice float64 `json:"estimated_price"`
	Currency       string  `json:"currency"`
}

// DHLTrackResponse tracking response
type DHLTrackResponse struct {
	Success  bool            `json:"success"`
	Message  string          `json:"message"`
	Events   []DHLTrackEvent `json:"events"`
	Status   string          `json:"status"`
	Location string          `json:"location"`
}

// DHLTrackEvent individual tracking event
type DHLTrackEvent struct {
	Timestamp   string `json:"timestamp"`
	Location    string `json:"location"`
	Description string `json:"description"`
	StatusCode  string `json:"status_code"`
}

// DHLRateResponse rate query response
type DHLRateResponse struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message"`
	TotalPrice   float64 `json:"total_price"`
	Currency     string  `json:"currency"`
	DeliveryDays int     `json:"delivery_days"`
	ProductName  string  `json:"product_name"`
}

// NewDHLService create DHL Express service instance
func NewDHLService(config DHLConfig) *DHLService {
	return &DHLService{
		APIKey:      config.APIKey,
		APISecret:   config.APISecret,
		Environment: config.Environment,
		AccountNo:   config.AccountNo,
	}
}

// getBaseURL get API base URL
func (s *DHLService) getBaseURL() string {
	if s.Environment == "production" {
		return "https://express.api.dhl.com"
	}
	return "https://express.api.dhl.com/mydhlapi/test"
}

// getAuthHeader generate Basic Auth header
func (s *DHLService) getAuthHeader() string {
	credentials := s.APIKey + ":" + s.APISecret
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
}

// makeRequest send API request
func (s *DHLService) makeRequest(method, path string, body interface{}) ([]byte, error) {
	var bodyBytes []byte
	var err error
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
	req.Header.Set("Authorization", s.getAuthHeader())

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
func (s *DHLService) CreateOrder(req DHLCreateOrderRequest) (*DHLCreateOrderResponse, error) {
	// Build DHL Express shipment request body
	shipmentReq := map[string]interface{}{
		"plannedShippingDateAndTime": time.Now().Add(24*time.Hour).Format("2006-01-02T15:04:05") + " GMT+08:00",
		"pickup": map[string]interface{}{
			"isRequested": false,
		},
		"productCode": req.ProductCode,
		"accounts": []map[string]interface{}{
			{
				"typeCode": "shipper",
				"number":   s.AccountNo,
			},
		},
		"customerDetails": map[string]interface{}{
			"shipperDetails": map[string]interface{}{
				"postalAddress": map[string]interface{}{
					"addressLine1": req.SenderAddress,
					"cityName":     req.SenderCity,
					"postalCode":   req.SenderPostal,
					"countryCode":  req.SenderCountry,
				},
				"contactInformation": map[string]interface{}{
					"phone":       req.SenderPhone,
					"companyName": req.SenderName,
					"fullName":    req.SenderName,
				},
			},
			"receiverDetails": map[string]interface{}{
				"postalAddress": map[string]interface{}{
					"addressLine1": req.RecipientAddress,
					"cityName":     req.RecipientCity,
					"postalCode":   req.RecipientPostal,
					"countryCode":  req.RecipientCountry,
				},
				"contactInformation": map[string]interface{}{
					"phone":       req.RecipientPhone,
					"companyName": req.RecipientName,
					"fullName":    req.RecipientName,
				},
			},
		},
		"content": map[string]interface{}{
			"packages": []map[string]interface{}{
				{
					"weight":      req.Weight,
					"description": req.Description,
					"dimensions": map[string]interface{}{
						"length": req.Length,
						"width":  req.Width,
						"height": req.Height,
					},
				},
			},
			"isCustomsDeclarable": req.RecipientCountry != req.SenderCountry,
			"description":         req.Description,
			"unitOfMeasurement":   "metric",
		},
	}

	// Set defaults
	if req.ProductCode == "" {
		shipmentReq["productCode"] = "P" // Express Worldwide
	}

	respBody, err := s.makeRequest("POST", "/mydhlapi/shipments", shipmentReq)
	if err != nil {
		return &DHLCreateOrderResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &DHLCreateOrderResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	// Extract shipment tracking number
	trackingNumber := ""
	shipmentID := ""
	dispatchNumber := ""

	if packages, ok := result["packages"].([]interface{}); ok && len(packages) > 0 {
		if pkg, ok := packages[0].(map[string]interface{}); ok {
			if tn, ok := pkg["trackingNumber"].(string); ok {
				trackingNumber = tn
			}
		}
	}
	if sid, ok := result["shipmentTrackingNumber"].(string); ok {
		trackingNumber = sid
		shipmentID = sid
	}
	if dn, ok := result["dispatchConfirmationNumber"].(string); ok {
		dispatchNumber = dn
	}

	return &DHLCreateOrderResponse{
		Success:        true,
		Message:        "DHL shipment created successfully",
		ShipmentID:     shipmentID,
		TrackingNumber: trackingNumber,
		DispatchNumber: dispatchNumber,
	}, nil
}

// TrackOrder track a shipment
func (s *DHLService) TrackOrder(trackingNumber string) (*DHLTrackResponse, error) {
	respBody, err := s.makeRequest("GET", "/mydhlapi/shipments/"+trackingNumber+"/tracking", nil)
	if err != nil {
		return &DHLTrackResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &DHLTrackResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	var events []DHLTrackEvent
	var latestStatus, latestLocation string

	if shipments, ok := result["shipments"].([]interface{}); ok && len(shipments) > 0 {
		if shipment, ok := shipments[0].(map[string]interface{}); ok {
			if evts, ok := shipment["events"].([]interface{}); ok {
				for _, e := range evts {
					if evt, ok := e.(map[string]interface{}); ok {
						event := DHLTrackEvent{}
						if ts, ok := evt["timestamp"].(string); ok {
							event.Timestamp = ts
						}
						if loc, ok := evt["location"].(map[string]interface{}); ok {
							if addr, ok := loc["address"].(map[string]interface{}); ok {
								if city, ok := addr["addressLocality"].(string); ok {
									event.Location = city
								}
							}
						}
						if desc, ok := evt["description"].(string); ok {
							event.Description = desc
						}
						if sc, ok := evt["statusCode"].(string); ok {
							event.StatusCode = sc
						}
						events = append(events, event)
					}
				}
			}
			if status, ok := shipment["status"].(map[string]interface{}); ok {
				if sc, ok := status["statusCode"].(string); ok {
					latestStatus = sc
				}
				if loc, ok := status["location"].(map[string]interface{}); ok {
					if addr, ok := loc["address"].(map[string]interface{}); ok {
						if city, ok := addr["addressLocality"].(string); ok {
							latestLocation = city
						}
					}
				}
			}
		}
	}

	return &DHLTrackResponse{
		Success:  true,
		Message:  "tracking query successful",
		Events:   events,
		Status:   latestStatus,
		Location: latestLocation,
	}, nil
}

// GetRates query shipping rates
func (s *DHLService) GetRates(originCountry, originCity, originPostal, destCountry, destCity, destPostal string, weight float64) (*DHLRateResponse, error) {
	path := fmt.Sprintf("/mydhlapi/rates?accountNumber=%s&originCountryCode=%s&originCityName=%s&originPostalCode=%s&destinationCountryCode=%s&destinationCityName=%s&destinationPostalCode=%s&weight=%g&length=20&width=15&height=10&plannedShippingDate=%s&isCustomsDeclarable=%t&unitOfMeasurement=metric",
		s.AccountNo,
		originCountry, originCity, originPostal,
		destCountry, destCity, destPostal,
		weight,
		time.Now().Add(24*time.Hour).Format("2006-01-02"),
		originCountry != destCountry,
	)

	respBody, err := s.makeRequest("GET", path, nil)
	if err != nil {
		return &DHLRateResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return &DHLRateResponse{
			Success: false,
			Message: "parse response failed: " + err.Error(),
		}, nil
	}

	// Extract first product rate
	if products, ok := result["products"].([]interface{}); ok && len(products) > 0 {
		if product, ok := products[0].(map[string]interface{}); ok {
			var totalPrice float64
			var currency, productName string
			var deliveryDays int

			if name, ok := product["productName"].(string); ok {
				productName = name
			}
			if pricing, ok := product["totalPrice"].([]interface{}); ok && len(pricing) > 0 {
				if p, ok := pricing[0].(map[string]interface{}); ok {
					if price, ok := p["price"].(float64); ok {
						totalPrice = price
					}
					if cur, ok := p["currencyType"].(string); ok {
						currency = cur
					}
				}
			}
			if dt, ok := product["deliveryCapabilities"].(map[string]interface{}); ok {
				if days, ok := dt["totalTransitDays"].(float64); ok {
					deliveryDays = int(days)
				}
			}

			return &DHLRateResponse{
				Success:      true,
				Message:      "rate query successful",
				TotalPrice:   totalPrice,
				Currency:     currency,
				DeliveryDays: deliveryDays,
				ProductName:  productName,
			}, nil
		}
	}

	return &DHLRateResponse{
		Success: false,
		Message: "no available products found",
	}, nil
}

// ParseDHLConfigFromJSON parse DHL config from JSON data
func ParseDHLConfigFromJSON(data map[string]interface{}) *DHLConfig {
	if data == nil {
		return nil
	}

	dhlData, ok := data["dhl"].(map[string]interface{})
	if !ok {
		return nil
	}

	config := &DHLConfig{}

	if enabled, ok := dhlData["enabled"].(bool); ok {
		config.Enabled = enabled
	}
	if env, ok := dhlData["environment"].(string); ok {
		config.Environment = env
	}
	if key, ok := dhlData["api_key"].(string); ok {
		config.APIKey = key
	}
	if secret, ok := dhlData["api_secret"].(string); ok {
		config.APISecret = secret
	}
	if acc, ok := dhlData["account_no"].(string); ok {
		config.AccountNo = acc
	}
	if auto, ok := dhlData["auto_create_order"].(bool); ok {
		config.AutoCreateOrder = auto
	}
	if track, ok := dhlData["auto_track"].(bool); ok {
		config.AutoTrack = track
	}
	if qp, ok := dhlData["query_price"].(bool); ok {
		config.QueryPrice = qp
	}
	if pc, ok := dhlData["product_code"].(string); ok {
		config.ProductCode = pc
	}

	return config
}

// MapDHLStatusToShipmentStatus convert DHL status code to system shipment status
func MapDHLStatusToShipmentStatus(statusCode string) string {
	// DHL status codes:
	// pre-transit, transit, delivered, failure, unknown
	switch statusCode {
	case "pre-transit":
		return "pending"
	case "transit":
		return "in_transit"
	case "out-for-delivery":
		return "out_for_delivery"
	case "delivered":
		return "delivered"
	case "failure", "exception":
		return "failed"
	case "returned":
		return "returned"
	default:
		return "in_transit"
	}
}

// ToJSON convert config to JSON
func (c *DHLConfig) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}
