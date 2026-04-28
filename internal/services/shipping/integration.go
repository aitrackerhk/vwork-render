package shipping

import (
	"fmt"
	"log"

	"github.com/google/uuid"
)

// ShippingIntegrationService 配送整合服務
// 用於統一處理各種配送 API 的調用
type ShippingIntegrationService struct {
	TenantID uuid.UUID
	Config   map[string]interface{}
}

// CreateOrderRequest 創建運單的統一請求格式
type CreateOrderRequest struct {
	IntegrationType string // sfexpress, lalamove, dhl, ups, fedex, amazon

	// 寄件人信息
	SenderName    string
	SenderPhone   string
	SenderAddress string

	// 收件人信息
	RecipientName    string
	RecipientPhone   string
	RecipientAddress string

	// 配送信息
	Weight      float64
	ItemCount   int
	Description string
	Notes       string
}

// CreateOrderResult 創建運單的統一響應格式
type CreateOrderResult struct {
	Success        bool
	Message        string
	TrackingNumber string  // 物流單號
	OrderRef       string  // 平台訂單號
	EstimatedFee   float64 // 預估運費
	ShareLink      string  // 追蹤連結（如 Lalamove）
}

// NewShippingIntegrationService 創建配送整合服務
func NewShippingIntegrationService(tenantID uuid.UUID, config map[string]interface{}) *ShippingIntegrationService {
	return &ShippingIntegrationService{
		TenantID: tenantID,
		Config:   config,
	}
}

// CreateOrder 根據整合類型創建運單
func (s *ShippingIntegrationService) CreateOrder(req CreateOrderRequest) (*CreateOrderResult, error) {
	switch req.IntegrationType {
	case "sfexpress":
		return s.createSFExpressOrder(req)
	case "lalamove":
		return s.createLalamoveOrder(req)
	case "dhl":
		return s.createDHLOrder(req)
	case "ups":
		return s.createUPSOrder(req)
	case "fedex":
		return s.createFedExOrder(req)
	case "amazon":
		return s.createAmazonOrder(req)
	default:
		return &CreateOrderResult{
			Success: false,
			Message: "不支援的配送連接類型",
		}, nil
	}
}

// createSFExpressOrder 創建順豐運單
func (s *ShippingIntegrationService) createSFExpressOrder(req CreateOrderRequest) (*CreateOrderResult, error) {
	sfConfig := ParseSFExpressConfigFromJSON(s.Config)
	if sfConfig == nil || !sfConfig.Enabled {
		return &CreateOrderResult{
			Success: false,
			Message: "SF Express 未啟用或配置不正確",
		}, nil
	}

	if !sfConfig.AutoCreateOrder {
		return &CreateOrderResult{
			Success: false,
			Message: "SF Express 自動創建運單功能未開啟",
		}, nil
	}

	service := NewSFExpressService(*sfConfig)

	sfReq := SFCreateOrderRequest{
		SenderName:       req.SenderName,
		SenderPhone:      req.SenderPhone,
		SenderAddress:    req.SenderAddress,
		RecipientName:    req.RecipientName,
		RecipientPhone:   req.RecipientPhone,
		RecipientAddress: req.RecipientAddress,
		ExpressType:      sfConfig.ExpressType,
		PayMethod:        sfConfig.PayMethod,
		Weight:           req.Weight,
		ItemCount:        req.ItemCount,
		CustomerNote:     req.Notes,
	}

	// 默認值
	if sfReq.ExpressType == "" {
		sfReq.ExpressType = "1" // 順豐標快
	}
	if sfReq.PayMethod == "" {
		sfReq.PayMethod = "1" // 寄方付
	}
	if sfReq.ItemCount < 1 {
		sfReq.ItemCount = 1
	}

	resp, err := service.CreateOrder(sfReq)
	if err != nil {
		log.Printf("[ShippingIntegration] SF Express 創建運單失敗: %v", err)
		return &CreateOrderResult{
			Success: false,
			Message: fmt.Sprintf("SF Express API 錯誤: %v", err),
		}, nil
	}

	if !resp.Success {
		return &CreateOrderResult{
			Success: false,
			Message: resp.Message,
		}, nil
	}

	return &CreateOrderResult{
		Success:        true,
		Message:        "順豐運單創建成功",
		TrackingNumber: resp.WaybillNo,
		OrderRef:       resp.OrderID,
		EstimatedFee:   resp.EstimatedPrice,
	}, nil
}

// createLalamoveOrder 創建 Lalamove 訂單
func (s *ShippingIntegrationService) createLalamoveOrder(req CreateOrderRequest) (*CreateOrderResult, error) {
	llmConfig := ParseLalamoveConfigFromJSON(s.Config)
	if llmConfig == nil || !llmConfig.Enabled {
		return &CreateOrderResult{
			Success: false,
			Message: "Lalamove 未啟用或配置不正確",
		}, nil
	}

	service := NewLalamoveService(*llmConfig)

	// 構建報價請求
	// 注意：Lalamove 需要坐標，這裡使用簡化版本
	// 實際應用需要通過地理編碼 API 將地址轉換為坐標
	quoteReq := LalamoveQuoteRequest{
		ServiceType: llmConfig.ServiceType,
		Stops: []LalamoveStop{
			{
				Address: req.SenderAddress,
				Coordinates: LalamoveCoordinates{
					Lat: "22.3193", // 香港默認坐標，實際需要地理編碼
					Lng: "114.1694",
				},
			},
			{
				Address: req.RecipientAddress,
				Coordinates: LalamoveCoordinates{
					Lat: "22.2799",
					Lng: "114.1735",
				},
			},
		},
		Deliveries: []LalamoveDelivery{
			{
				ToStop: 1,
				ToContact: LalamoveContact{
					Name:  req.RecipientName,
					Phone: req.RecipientPhone,
				},
				Remarks: req.Notes,
			},
		},
	}

	if quoteReq.ServiceType == "" {
		quoteReq.ServiceType = "MOTORCYCLE"
	}

	// 獲取報價
	quoteResp, err := service.GetQuotation(quoteReq)
	if err != nil {
		log.Printf("[ShippingIntegration] Lalamove 報價失敗: %v", err)
		return &CreateOrderResult{
			Success: false,
			Message: fmt.Sprintf("Lalamove 報價失敗: %v", err),
		}, nil
	}

	if !quoteResp.Success {
		return &CreateOrderResult{
			Success: false,
			Message: quoteResp.Message,
		}, nil
	}

	// 創建訂單
	orderReq := LalamoveOrderRequest{
		QuotationID: quoteResp.QuotationID,
		Sender: LalamoveContact{
			Name:  req.SenderName,
			Phone: req.SenderPhone,
		},
		Stops:        quoteReq.Stops,
		Deliveries:   quoteReq.Deliveries,
		IsPODEnabled: llmConfig.RequireSignature,
	}

	orderResp, err := service.CreateOrder(orderReq)
	if err != nil {
		log.Printf("[ShippingIntegration] Lalamove 創建訂單失敗: %v", err)
		return &CreateOrderResult{
			Success: false,
			Message: fmt.Sprintf("Lalamove 創建訂單失敗: %v", err),
		}, nil
	}

	if !orderResp.Success {
		return &CreateOrderResult{
			Success: false,
			Message: orderResp.Message,
		}, nil
	}

	return &CreateOrderResult{
		Success:        true,
		Message:        "Lalamove 訂單創建成功",
		TrackingNumber: orderResp.OrderRef,
		OrderRef:       orderResp.OrderRef,
		EstimatedFee:   quoteResp.TotalFee,
		ShareLink:      orderResp.ShareLink,
	}, nil
}

// createDHLOrder 創建 DHL 運單
func (s *ShippingIntegrationService) createDHLOrder(req CreateOrderRequest) (*CreateOrderResult, error) {
	dhlConfig := ParseDHLConfigFromJSON(s.Config)
	if dhlConfig == nil || !dhlConfig.Enabled {
		return &CreateOrderResult{
			Success: false,
			Message: "DHL Express 未啟用或配置不正確",
		}, nil
	}

	if !dhlConfig.AutoCreateOrder {
		return &CreateOrderResult{
			Success: false,
			Message: "DHL Express 自動創建運單功能未開啟",
		}, nil
	}

	service := NewDHLService(*dhlConfig)

	dhlReq := DHLCreateOrderRequest{
		SenderName:       req.SenderName,
		SenderPhone:      req.SenderPhone,
		SenderAddress:    req.SenderAddress,
		RecipientName:    req.RecipientName,
		RecipientPhone:   req.RecipientPhone,
		RecipientAddress: req.RecipientAddress,
		Weight:           req.Weight,
		Description:      req.Description,
		CustomerNote:     req.Notes,
		ProductCode:      dhlConfig.ProductCode,
	}

	// Defaults
	if dhlReq.ProductCode == "" {
		dhlReq.ProductCode = "P" // Express Worldwide
	}
	if dhlReq.Weight <= 0 {
		dhlReq.Weight = 0.5
	}

	resp, err := service.CreateOrder(dhlReq)
	if err != nil {
		log.Printf("[ShippingIntegration] DHL Express 創建運單失敗: %v", err)
		return &CreateOrderResult{
			Success: false,
			Message: fmt.Sprintf("DHL Express API 錯誤: %v", err),
		}, nil
	}

	if !resp.Success {
		return &CreateOrderResult{
			Success: false,
			Message: resp.Message,
		}, nil
	}

	return &CreateOrderResult{
		Success:        true,
		Message:        "DHL Express 運單創建成功",
		TrackingNumber: resp.TrackingNumber,
		OrderRef:       resp.ShipmentID,
		EstimatedFee:   resp.EstimatedPrice,
	}, nil
}

// createUPSOrder 創建 UPS 運單
func (s *ShippingIntegrationService) createUPSOrder(req CreateOrderRequest) (*CreateOrderResult, error) {
	upsConfig := ParseUPSConfigFromJSON(s.Config)
	if upsConfig == nil || !upsConfig.Enabled {
		return &CreateOrderResult{
			Success: false,
			Message: "UPS 未啟用或配置不正確",
		}, nil
	}

	if !upsConfig.AutoCreateOrder {
		return &CreateOrderResult{
			Success: false,
			Message: "UPS 自動創建運單功能未開啟",
		}, nil
	}

	service := NewUPSService(*upsConfig)

	upsReq := UPSCreateOrderRequest{
		SenderName:       req.SenderName,
		SenderPhone:      req.SenderPhone,
		SenderAddress:    req.SenderAddress,
		RecipientName:    req.RecipientName,
		RecipientPhone:   req.RecipientPhone,
		RecipientAddress: req.RecipientAddress,
		Weight:           req.Weight,
		Description:      req.Description,
		CustomerNote:     req.Notes,
		ServiceCode:      upsConfig.ServiceCode,
	}

	if upsReq.ServiceCode == "" {
		upsReq.ServiceCode = "03" // UPS Ground
	}
	if upsReq.Weight <= 0 {
		upsReq.Weight = 0.5
	}

	resp, err := service.CreateOrder(upsReq)
	if err != nil {
		log.Printf("[ShippingIntegration] UPS 創建運單失敗: %v", err)
		return &CreateOrderResult{
			Success: false,
			Message: fmt.Sprintf("UPS API 錯誤: %v", err),
		}, nil
	}

	if !resp.Success {
		return &CreateOrderResult{
			Success: false,
			Message: resp.Message,
		}, nil
	}

	return &CreateOrderResult{
		Success:        true,
		Message:        "UPS 運單創建成功",
		TrackingNumber: resp.TrackingNumber,
		OrderRef:       resp.ShipmentID,
		EstimatedFee:   resp.EstimatedPrice,
	}, nil
}

// createFedExOrder 創建 FedEx 運單
func (s *ShippingIntegrationService) createFedExOrder(req CreateOrderRequest) (*CreateOrderResult, error) {
	fedexConfig := ParseFedExConfigFromJSON(s.Config)
	if fedexConfig == nil || !fedexConfig.Enabled {
		return &CreateOrderResult{
			Success: false,
			Message: "FedEx 未啟用或配置不正確",
		}, nil
	}

	if !fedexConfig.AutoCreateOrder {
		return &CreateOrderResult{
			Success: false,
			Message: "FedEx 自動創建運單功能未開啟",
		}, nil
	}

	service := NewFedExService(*fedexConfig)

	fedexReq := FedExCreateOrderRequest{
		SenderName:       req.SenderName,
		SenderPhone:      req.SenderPhone,
		SenderAddress:    req.SenderAddress,
		RecipientName:    req.RecipientName,
		RecipientPhone:   req.RecipientPhone,
		RecipientAddress: req.RecipientAddress,
		Weight:           req.Weight,
		Description:      req.Description,
		CustomerNote:     req.Notes,
		ServiceType:      fedexConfig.ServiceType,
	}

	if fedexReq.ServiceType == "" {
		fedexReq.ServiceType = "FEDEX_INTERNATIONAL_PRIORITY"
	}
	if fedexReq.Weight <= 0 {
		fedexReq.Weight = 0.5
	}

	resp, err := service.CreateOrder(fedexReq)
	if err != nil {
		log.Printf("[ShippingIntegration] FedEx 創建運單失敗: %v", err)
		return &CreateOrderResult{
			Success: false,
			Message: fmt.Sprintf("FedEx API 錯誤: %v", err),
		}, nil
	}

	if !resp.Success {
		return &CreateOrderResult{
			Success: false,
			Message: resp.Message,
		}, nil
	}

	return &CreateOrderResult{
		Success:        true,
		Message:        "FedEx 運單創建成功",
		TrackingNumber: resp.TrackingNumber,
		OrderRef:       resp.ShipmentID,
		EstimatedFee:   resp.EstimatedPrice,
	}, nil
}

// createAmazonOrder 創建 Amazon Shipping 運單
func (s *ShippingIntegrationService) createAmazonOrder(req CreateOrderRequest) (*CreateOrderResult, error) {
	amazonConfig := ParseAmazonShippingConfigFromJSON(s.Config)
	if amazonConfig == nil || !amazonConfig.Enabled {
		return &CreateOrderResult{
			Success: false,
			Message: "Amazon Logistics 未啟用或配置不正確",
		}, nil
	}

	if !amazonConfig.AutoCreateOrder {
		return &CreateOrderResult{
			Success: false,
			Message: "Amazon Logistics 自動創建運單功能未開啟",
		}, nil
	}

	service := NewAmazonShippingService(*amazonConfig)

	amazonReq := AmazonCreateOrderRequest{
		SenderName:       req.SenderName,
		SenderPhone:      req.SenderPhone,
		SenderAddress:    req.SenderAddress,
		RecipientName:    req.RecipientName,
		RecipientPhone:   req.RecipientPhone,
		RecipientAddress: req.RecipientAddress,
		Weight:           req.Weight,
		Description:      req.Description,
		CustomerNote:     req.Notes,
		ServiceType:      amazonConfig.ServiceType,
	}

	if amazonReq.Weight <= 0 {
		amazonReq.Weight = 0.5
	}

	resp, err := service.CreateOrder(amazonReq)
	if err != nil {
		log.Printf("[ShippingIntegration] Amazon Logistics 創建運單失敗: %v", err)
		return &CreateOrderResult{
			Success: false,
			Message: fmt.Sprintf("Amazon Shipping API 錯誤: %v", err),
		}, nil
	}

	if !resp.Success {
		return &CreateOrderResult{
			Success: false,
			Message: resp.Message,
		}, nil
	}

	return &CreateOrderResult{
		Success:        true,
		Message:        "Amazon Logistics 運單創建成功",
		TrackingNumber: resp.TrackingNumber,
		OrderRef:       resp.ShipmentID,
		EstimatedFee:   resp.EstimatedPrice,
	}, nil
}

// IsIntegrationEnabled 檢查指定類型的整合是否啟用
func (s *ShippingIntegrationService) IsIntegrationEnabled(integrationType string) bool {
	switch integrationType {
	case "sfexpress":
		config := ParseSFExpressConfigFromJSON(s.Config)
		return config != nil && config.Enabled && config.AutoCreateOrder
	case "lalamove":
		config := ParseLalamoveConfigFromJSON(s.Config)
		return config != nil && config.Enabled
	case "dhl":
		config := ParseDHLConfigFromJSON(s.Config)
		return config != nil && config.Enabled && config.AutoCreateOrder
	case "ups":
		config := ParseUPSConfigFromJSON(s.Config)
		return config != nil && config.Enabled && config.AutoCreateOrder
	case "fedex":
		config := ParseFedExConfigFromJSON(s.Config)
		return config != nil && config.Enabled && config.AutoCreateOrder
	case "amazon":
		config := ParseAmazonShippingConfigFromJSON(s.Config)
		return config != nil && config.Enabled && config.AutoCreateOrder
	default:
		return false
	}
}
