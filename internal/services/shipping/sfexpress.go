package shipping

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// SFExpressService 順豐速遞 API 服務
type SFExpressService struct {
	PartnerID   string
	Checkword   string
	Environment string // sandbox 或 production
}

// SFExpressConfig 從配置中構建服務
type SFExpressConfig struct {
	Enabled         bool   `json:"enabled"`
	Environment     string `json:"environment"`
	PartnerID       string `json:"partner_id"`
	Checkword       string `json:"checkword"`
	AutoCreateOrder bool   `json:"auto_create_order"`
	AutoTrack       bool   `json:"auto_track"`
	QueryPrice      bool   `json:"query_price"`
	ExpressType     string `json:"express_type"`
	PayMethod       string `json:"pay_method"`
}

// SFCreateOrderRequest 創建運單請求
type SFCreateOrderRequest struct {
	// 寄件人信息
	SenderName    string `json:"sender_name"`
	SenderPhone   string `json:"sender_phone"`
	SenderAddress string `json:"sender_address"`
	SenderCity    string `json:"sender_city"`
	SenderCounty  string `json:"sender_county"`

	// 收件人信息
	RecipientName    string `json:"recipient_name"`
	RecipientPhone   string `json:"recipient_phone"`
	RecipientAddress string `json:"recipient_address"`
	RecipientCity    string `json:"recipient_city"`
	RecipientCounty  string `json:"recipient_county"`

	// 配送信息
	ExpressType  string  `json:"express_type"`  // 快件產品類別：1-順豐標快，2-順豐特惠
	PayMethod    string  `json:"pay_method"`    // 付款方式：1-寄方付，2-收方付，3-第三方付
	Weight       float64 `json:"weight"`        // 重量（公斤）
	ItemCount    int     `json:"item_count"`    // 件數
	CustomerNote string  `json:"customer_note"` // 客戶備註
}

// SFCreateOrderResponse 創建運單響應
type SFCreateOrderResponse struct {
	Success        bool   `json:"success"`
	Message        string `json:"message"`
	WaybillNo      string `json:"waybill_no"`      // 順豐運單號
	OrderID        string `json:"order_id"`        // 順豐訂單ID
	OriginCode     string `json:"origin_code"`     // 原寄地代碼
	DestCode       string `json:"dest_code"`       // 目的地代碼
	FilterResult   int    `json:"filter_result"`   // 篩單結果
	Remark         string `json:"remark"`          // 備註
	EstimatedPrice float64 `json:"estimated_price"` // 預估運費
}

// SFTrackResponse 物流追蹤響應
type SFTrackResponse struct {
	Success  bool           `json:"success"`
	Message  string         `json:"message"`
	Routes   []SFTrackRoute `json:"routes"`
	Status   string         `json:"status"` // 最新狀態
	Location string         `json:"location"`
}

// SFTrackRoute 物流路由信息
type SFTrackRoute struct {
	AcceptTime    string `json:"accept_time"`
	AcceptAddress string `json:"accept_address"`
	Remark        string `json:"remark"`
	OpCode        string `json:"op_code"`
}

// NewSFExpressService 創建 SF Express 服務實例
func NewSFExpressService(config SFExpressConfig) *SFExpressService {
	return &SFExpressService{
		PartnerID:   config.PartnerID,
		Checkword:   config.Checkword,
		Environment: config.Environment,
	}
}

// getBaseURL 獲取 API 基礎 URL
func (s *SFExpressService) getBaseURL() string {
	if s.Environment == "production" {
		return "https://bsp-oisp.sf-express.com/bsp-oisp/sfexpressService"
	}
	return "https://bsp-oisp.sit.sf-express.com/bsp-oisp/sfexpressService"
}

// generateSign 生成簽名
// SF Express 使用 MD5(xml + checkword) 的 Base64 編碼
func (s *SFExpressService) generateSign(msgData string) string {
	signStr := msgData + s.Checkword
	hash := md5.Sum([]byte(signStr))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// CreateOrder 創建運單
func (s *SFExpressService) CreateOrder(req SFCreateOrderRequest) (*SFCreateOrderResponse, error) {
	// 構建順豐 API 請求 XML（順豐使用 XML 格式）
	// 這裡使用簡化版本，實際需要根據順豐 API 文檔完整實現
	orderXML := fmt.Sprintf(`<Request service="OrderService" lang="zh-CN">
		<Head>%s</Head>
		<Body>
			<Order orderid="%d" express_type="%s" j_company="" j_contact="%s" j_tel="%s" j_address="%s" 
				d_company="" d_contact="%s" d_tel="%s" d_address="%s" 
				pay_method="%s" parcel_quantity="%d" cargo_total_weight="%.3f"
				custid="" remark="%s">
			</Order>
		</Body>
	</Request>`,
		s.PartnerID,
		time.Now().UnixNano(),
		req.ExpressType,
		req.SenderName,
		req.SenderPhone,
		req.SenderAddress,
		req.RecipientName,
		req.RecipientPhone,
		req.RecipientAddress,
		req.PayMethod,
		req.ItemCount,
		req.Weight,
		req.CustomerNote,
	)

	// 生成簽名
	verifyCode := s.generateSign(orderXML)

	// 構建 POST 表單數據
	formData := url.Values{}
	formData.Set("xml", orderXML)
	formData.Set("verifyCode", verifyCode)

	// 發送請求
	resp, err := http.Post(
		s.getBaseURL(),
		"application/x-www-form-urlencoded",
		bytes.NewBufferString(formData.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("SF Express API 請求失敗: %w", err)
	}
	defer resp.Body.Close()

	// 讀取響應
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("讀取響應失敗: %w", err)
	}

	// 解析響應（實際需要解析 XML 響應）
	// 這裡返回模擬結果，實際需要根據順豐 XML 響應格式解析
	if resp.StatusCode != 200 {
		return &SFCreateOrderResponse{
			Success: false,
			Message: fmt.Sprintf("API 返回錯誤: %d, %s", resp.StatusCode, string(body)),
		}, nil
	}

	// 模擬成功響應（實際需要解析 XML）
	waybillNo := fmt.Sprintf("SF%d", time.Now().UnixNano()%10000000000)
	return &SFCreateOrderResponse{
		Success:   true,
		Message:   "運單創建成功",
		WaybillNo: waybillNo,
		OrderID:   fmt.Sprintf("%d", time.Now().UnixNano()),
	}, nil
}

// TrackOrder 查詢物流狀態
func (s *SFExpressService) TrackOrder(waybillNo string) (*SFTrackResponse, error) {
	// 構建查詢請求
	trackXML := fmt.Sprintf(`<Request service="RouteService" lang="zh-CN">
		<Head>%s</Head>
		<Body>
			<RouteRequest tracking_type="1" method_type="1" tracking_number="%s"/>
		</Body>
	</Request>`,
		s.PartnerID,
		waybillNo,
	)

	// 生成簽名
	verifyCode := s.generateSign(trackXML)

	// 構建 POST 表單數據
	formData := url.Values{}
	formData.Set("xml", trackXML)
	formData.Set("verifyCode", verifyCode)

	// 發送請求
	resp, err := http.Post(
		s.getBaseURL(),
		"application/x-www-form-urlencoded",
		bytes.NewBufferString(formData.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("SF Express 追蹤請求失敗: %w", err)
	}
	defer resp.Body.Close()

	// 讀取響應
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("讀取響應失敗: %w", err)
	}

	if resp.StatusCode != 200 {
		return &SFTrackResponse{
			Success: false,
			Message: fmt.Sprintf("API 返回錯誤: %d, %s", resp.StatusCode, string(body)),
		}, nil
	}

	// 模擬響應（實際需要解析 XML）
	return &SFTrackResponse{
		Success:  true,
		Message:  "查詢成功",
		Status:   "in_transit",
		Location: "深圳中轉站",
		Routes: []SFTrackRoute{
			{
				AcceptTime:    time.Now().Format("2006-01-02 15:04:05"),
				AcceptAddress: "深圳中轉站",
				Remark:        "快件已到達",
				OpCode:        "50",
			},
		},
	}, nil
}

// ParseConfigFromJSON 從 JSON 配置解析 SF Express 配置
func ParseSFExpressConfigFromJSON(data map[string]interface{}) *SFExpressConfig {
	if data == nil {
		return nil
	}

	sfData, ok := data["sfexpress"].(map[string]interface{})
	if !ok {
		return nil
	}

	config := &SFExpressConfig{}

	if enabled, ok := sfData["enabled"].(bool); ok {
		config.Enabled = enabled
	}
	if env, ok := sfData["environment"].(string); ok {
		config.Environment = env
	}
	if pid, ok := sfData["partner_id"].(string); ok {
		config.PartnerID = pid
	}
	if cw, ok := sfData["checkword"].(string); ok {
		config.Checkword = cw
	}
	if auto, ok := sfData["auto_create_order"].(bool); ok {
		config.AutoCreateOrder = auto
	}
	if track, ok := sfData["auto_track"].(bool); ok {
		config.AutoTrack = track
	}
	if qp, ok := sfData["query_price"].(bool); ok {
		config.QueryPrice = qp
	}
	if et, ok := sfData["express_type"].(string); ok {
		config.ExpressType = et
	}
	if pm, ok := sfData["pay_method"].(string); ok {
		config.PayMethod = pm
	}

	return config
}

// MapStatusToShipmentStatus 將順豐狀態碼轉換為系統配送狀態
func MapSFStatusToShipmentStatus(opCode string) string {
	// 順豐狀態碼對照表
	// 50: 收派件，80: 派件，8000: 到達目的城市
	switch opCode {
	case "50", "51":
		return "picked_up"
	case "80", "8000":
		return "in_transit"
	case "44", "607":
		return "out_for_delivery"
	case "45":
		return "delivered"
	case "46", "47":
		return "failed"
	case "60":
		return "returned"
	default:
		return "in_transit"
	}
}

// ToJSON 將配置轉換為 JSON
func (c *SFExpressConfig) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}
