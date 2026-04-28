package handlers

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ===== QFPay Client: FPS, PayMe, Alipay HK, WeChat Pay HK, BoC Pay, Octopus =====
//
// QFPay API docs: https://sdk.qfapi.com/
// Flow:
//   1. Server creates a payment order via QFPay API → returns QR code URL or redirect URL
//   2. Customer scans QR / redirects to wallet app
//   3. QFPay sends webhook notification on payment completion
//   4. Server verifies signature and marks order completed
//
// Credential sources (in priority order):
//   a. Tenant's payment_methods.extra_fields (tenant-level QFPay account)
//   b. Platform config.QFPay (platform-level shared account)

// QFPayCredentials holds the API credentials for a QFPay request.
type QFPayCredentials struct {
	AppCode   string
	ClientKey string
	BaseURL   string
}

// QFPayOrderResponse is the response from QFPay /trade/v1/payment endpoint.
type QFPayOrderResponse struct {
	RespCode string `json:"respcd"`
	RespMsg  string `json:"resperr"`
	SysTime  string `json:"sysdtm"`
	// Payment data
	OrderID    string `json:"syssn"`        // QFPay system order number
	QRCode     string `json:"qrcode"`       // QR code content (for scan-to-pay)
	PayURL     string `json:"pay_url"`      // Redirect URL (for online redirect)
	OutTradeNo string `json:"out_trade_no"` // Our order number
}

// QFPayQueryResponse is the response from QFPay query endpoint.
type QFPayQueryResponse struct {
	RespCode string `json:"respcd"`
	RespMsg  string `json:"resperr"`
	Data     []struct {
		OrderID    string `json:"syssn"`
		OutTradeNo string `json:"out_trade_no"`
		Status     string `json:"order_status"` // "0"=pending, "1"=success, ...
		PayType    string `json:"pay_type"`
		Amount     string `json:"txamt"`
		Currency   string `json:"txcurrcd"`
	} `json:"data"`
}

// QFPayNotification is the webhook notification from QFPay.
type QFPayNotification struct {
	Status     string `json:"status"` // "1" = success
	PayType    string `json:"pay_type"`
	SysSN      string `json:"syssn"`        // QFPay system order number
	OutTradeNo string `json:"out_trade_no"` // Our order ID
	Amount     string `json:"txamt"`        // Amount in cents
	Currency   string `json:"txcurrcd"`
	RespCode   string `json:"respcd"`
	Sign       string `json:"signature"` // For verification
}

// resolveQFPayCredentials gets QFPay credentials from tenant's payment method ExtraFields,
// falling back to platform-level config.
func resolveQFPayCredentials(pmCode string, tenantExtraFields map[string]interface{}) *QFPayCredentials {
	// Try tenant-level first
	appCode := extractString(tenantExtraFields, "qfpay_app_code")
	clientKey := extractString(tenantExtraFields, "qfpay_client_key")
	baseURL := extractString(tenantExtraFields, "qfpay_base_url")

	// Fall back to platform config
	cfg := mustAppConfig()
	if appCode == "" {
		appCode = cfg.QFPay.AppCode
	}
	if clientKey == "" {
		clientKey = cfg.QFPay.ClientKey
	}
	if baseURL == "" {
		baseURL = cfg.QFPay.BaseURL
	}
	if baseURL == "" {
		baseURL = "https://openapi-hk.qfapi.com"
	}

	if appCode == "" || clientKey == "" {
		return nil
	}

	return &QFPayCredentials{
		AppCode:   appCode,
		ClientKey: clientKey,
		BaseURL:   baseURL,
	}
}

// extractString safely extracts a string from a map.
func extractString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

// qfpaySign generates the HMAC-SHA256 signature for QFPay API requests.
// QFPay requires: sort params by key → URL encode → HMAC-SHA256 with client_key
func qfpaySign(params map[string]string, clientKey string) string {
	// Sort keys
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build query string
	var parts []string
	for _, k := range keys {
		v := params[k]
		if v == "" {
			continue
		}
		parts = append(parts, k+"="+url.QueryEscape(v))
	}
	data := strings.Join(parts, "&")

	// HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(clientKey))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// qfpayMD5Sign generates MD5 signature (used by some older QFPay endpoints).
func qfpayMD5Sign(params map[string]string, clientKey string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		v := params[k]
		if v == "" {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	data := strings.Join(parts, "&") + clientKey

	h := md5.New()
	h.Write([]byte(data))
	return strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
}

// qfpayCreatePayment calls QFPay API to create a payment order.
// Returns the QFPay response including QR code / redirect URL.
func qfpayCreatePayment(creds *QFPayCredentials, payType string, amountCents int64, currency string, outTradeNo string, notifyURL string, goodsName string) (*QFPayOrderResponse, error) {
	endpoint := creds.BaseURL + "/trade/v1/payment"

	amountStr := fmt.Sprintf("%d", amountCents)
	now := time.Now().Format("2006-01-02 15:04:05")

	params := map[string]string{
		"pay_type":     payType,
		"out_trade_no": outTradeNo,
		"txamt":        amountStr,
		"txcurrcd":     strings.ToUpper(currency),
		"txdtm":        now,
		"goods_name":   goodsName,
	}

	if notifyURL != "" {
		params["notify_url"] = notifyURL
	}

	// Add return URL for redirect-based flows (FPS, PayMe, etc.)
	// QFPay will redirect user back after payment
	if notifyURL != "" {
		// Use same base as notify URL but with /return path
		params["return_url"] = strings.Replace(notifyURL, "/notify", "/return", 1)
	}

	sign := qfpaySign(params, creds.ClientKey)

	// Build form body
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("qfpay create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-QF-APPCODE", creds.AppCode)
	req.Header.Set("X-QF-SIGN", sign)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qfpay request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qfpay read response failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("qfpay HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result QFPayOrderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("qfpay parse response failed: %w", err)
	}

	if result.RespCode != "0000" {
		return &result, fmt.Errorf("qfpay error %s: %s", result.RespCode, result.RespMsg)
	}

	return &result, nil
}

// qfpayQueryOrder queries QFPay for the status of a payment order.
func qfpayQueryOrder(creds *QFPayCredentials, syssn string) (*QFPayQueryResponse, error) {
	endpoint := creds.BaseURL + "/trade/v1/query"

	params := map[string]string{
		"syssn": syssn,
	}

	sign := qfpaySign(params, creds.ClientKey)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("qfpay query request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-QF-APPCODE", creds.AppCode)
	req.Header.Set("X-QF-SIGN", sign)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qfpay query failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qfpay read query response failed: %w", err)
	}

	var result QFPayQueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("qfpay parse query response failed: %w", err)
	}

	return &result, nil
}

// qfpayVerifyNotification verifies the signature of a QFPay webhook notification.
func qfpayVerifyNotification(notification map[string]string, clientKey string) bool {
	sig := notification["signature"]
	if sig == "" {
		return false
	}

	// Remove signature from params before verifying
	params := make(map[string]string)
	for k, v := range notification {
		if k != "signature" {
			params[k] = v
		}
	}

	expected := qfpaySign(params, clientKey)
	return hmac.Equal([]byte(sig), []byte(expected))
}
