package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// ===== Payment Gateway Constants & Registry =====
// 支援的線上支付方式 Code 常數（存入 payment_methods.code）
//
// 每個 gateway code 對應一組獨立的 handler 和 ExtraFields schema。
// 租戶在 CMS 建立 payment method 時選擇 code，系統自動路由到正確的 handler。

const (
	// ── 原有 ──
	GatewayStripe        = "stripe"         // Stripe Direct（租戶自有 API key）
	GatewayStripeConnect = "stripe_connect" // Stripe Connect（平台 key + destination charges）
	GatewayPayPal        = "paypal"         // PayPal Buttons

	// ── Stripe 原生支付方式（透過 Stripe PaymentIntent + payment_method_types）──
	GatewayAlipay    = "alipay"     // 支付寶（中國）— Stripe 原生
	GatewayWeChatPay = "wechat_pay" // 微信支付 — Stripe 原生
	GatewayApplePay  = "apple_pay"  // Apple Pay — 透過 Stripe Payment Request Button
	GatewayGooglePay = "google_pay" // Google Pay — 透過 Stripe Payment Request Button

	// ── QFPay 聚合支付 ──
	GatewayFPS      = "fps"       // 轉數快 (FPS) — QFPay
	GatewayPayMe    = "payme"     // PayMe — QFPay
	GatewayAlipayHK = "alipay_hk" // Alipay HK — QFPay
	GatewayWeChatHK = "wechat_hk" // WeChat Pay HK — QFPay
	GatewayBoCPay   = "boc_pay"   // BoC Pay — QFPay
	GatewayOctopus  = "octopus"   // 八達通 — QFPay

	// ── Stripe 原生支付方式（東南亞）──
	GatewayPayNow  = "paynow"  // PayNow（新加坡即時轉帳）— Stripe 原生
	GatewayGrabPay = "grabpay" // GrabPay（東南亞超級 App）— Stripe 原生

	// ── UnionPay ──
	GatewayUnionPay = "unionpay" // 銀聯/雲閃付

	// ── 非 gateway（保留原有）──
	GatewayNormal       = "normal"        // 一般（現金/轉帳等）
	GatewayCardTerminal = "card_terminal" // 實體卡機
)

// GatewayProvider maps gateway code to the upstream provider for routing.
type GatewayProvider string

const (
	ProviderStripe   GatewayProvider = "stripe"
	ProviderPayPal   GatewayProvider = "paypal"
	ProviderQFPay    GatewayProvider = "qfpay"
	ProviderUnionPay GatewayProvider = "unionpay"
	ProviderNone     GatewayProvider = "none"
)

// GatewayInfo describes a payment gateway for UI/routing.
type GatewayInfo struct {
	Code          string          `json:"code"`
	DisplayName   string          `json:"display_name"`
	DisplayNameZH string          `json:"display_name_zh"`
	Provider      GatewayProvider `json:"provider"`
	IsOnline      bool            `json:"is_online"`
	// SupportedCurrencies — empty means all
	SupportedCurrencies []string `json:"supported_currencies,omitempty"`
}

// GatewayRegistry is the canonical list of all supported gateway codes.
var GatewayRegistry = []GatewayInfo{
	{Code: GatewayStripe, DisplayName: "Credit Card (Stripe)", DisplayNameZH: "信用卡 (Stripe)", Provider: ProviderStripe, IsOnline: true, SupportedCurrencies: nil},
	{Code: GatewayStripeConnect, DisplayName: "Credit Card (Stripe Connect)", DisplayNameZH: "信用卡 (Stripe Connect)", Provider: ProviderStripe, IsOnline: true, SupportedCurrencies: nil},
	{Code: GatewayPayPal, DisplayName: "PayPal", DisplayNameZH: "PayPal", Provider: ProviderPayPal, IsOnline: true, SupportedCurrencies: nil},
	{Code: GatewayAlipay, DisplayName: "Alipay", DisplayNameZH: "支付寶", Provider: ProviderStripe, IsOnline: true, SupportedCurrencies: []string{"hkd", "cny", "usd", "sgd"}},
	{Code: GatewayWeChatPay, DisplayName: "WeChat Pay", DisplayNameZH: "微信支付", Provider: ProviderStripe, IsOnline: true, SupportedCurrencies: []string{"hkd", "cny", "usd", "sgd"}},
	{Code: GatewayApplePay, DisplayName: "Apple Pay", DisplayNameZH: "Apple Pay", Provider: ProviderStripe, IsOnline: true, SupportedCurrencies: nil},
	{Code: GatewayGooglePay, DisplayName: "Google Pay", DisplayNameZH: "Google Pay", Provider: ProviderStripe, IsOnline: true, SupportedCurrencies: nil},
	{Code: GatewayFPS, DisplayName: "FPS", DisplayNameZH: "轉數快", Provider: ProviderQFPay, IsOnline: true, SupportedCurrencies: []string{"hkd"}},
	{Code: GatewayPayMe, DisplayName: "PayMe", DisplayNameZH: "PayMe", Provider: ProviderQFPay, IsOnline: true, SupportedCurrencies: []string{"hkd"}},
	{Code: GatewayAlipayHK, DisplayName: "Alipay HK", DisplayNameZH: "支付寶HK", Provider: ProviderQFPay, IsOnline: true, SupportedCurrencies: []string{"hkd"}},
	{Code: GatewayWeChatHK, DisplayName: "WeChat Pay HK", DisplayNameZH: "微信支付HK", Provider: ProviderQFPay, IsOnline: true, SupportedCurrencies: []string{"hkd"}},
	{Code: GatewayBoCPay, DisplayName: "BoC Pay", DisplayNameZH: "中銀支付", Provider: ProviderQFPay, IsOnline: true, SupportedCurrencies: []string{"hkd"}},
	{Code: GatewayOctopus, DisplayName: "Octopus", DisplayNameZH: "八達通", Provider: ProviderQFPay, IsOnline: true, SupportedCurrencies: []string{"hkd"}},
	{Code: GatewayPayNow, DisplayName: "PayNow", DisplayNameZH: "PayNow (新加坡)", Provider: ProviderStripe, IsOnline: true, SupportedCurrencies: []string{"sgd"}},
	{Code: GatewayGrabPay, DisplayName: "GrabPay", DisplayNameZH: "GrabPay", Provider: ProviderStripe, IsOnline: true, SupportedCurrencies: []string{"sgd", "myr"}},
	{Code: GatewayUnionPay, DisplayName: "UnionPay", DisplayNameZH: "銀聯/雲閃付", Provider: ProviderUnionPay, IsOnline: true, SupportedCurrencies: []string{"hkd", "cny"}},
	{Code: GatewayNormal, DisplayName: "Normal", DisplayNameZH: "一般", Provider: ProviderNone, IsOnline: false},
	{Code: GatewayCardTerminal, DisplayName: "Card Terminal", DisplayNameZH: "卡機", Provider: ProviderNone, IsOnline: false},
}

// GatewayByCode returns the GatewayInfo for a given code, or nil if not found.
func GatewayByCode(code string) *GatewayInfo {
	for i := range GatewayRegistry {
		if GatewayRegistry[i].Code == code {
			return &GatewayRegistry[i]
		}
	}
	return nil
}

// ProviderForCode returns the upstream provider for a gateway code.
func ProviderForCode(code string) GatewayProvider {
	if g := GatewayByCode(code); g != nil {
		return g.Provider
	}
	return ProviderNone
}

// IsStripeNativeMethod returns true if this gateway code uses Stripe PaymentIntent
// with a specific payment_method_type (not the default card).
func IsStripeNativeMethod(code string) bool {
	switch code {
	case GatewayAlipay, GatewayWeChatPay, GatewayPayNow, GatewayGrabPay:
		return true
	}
	return false
}

// IsStripePaymentRequestMethod returns true if this gateway code uses
// Stripe Payment Request Button (Apple Pay / Google Pay).
func IsStripePaymentRequestMethod(code string) bool {
	switch code {
	case GatewayApplePay, GatewayGooglePay:
		return true
	}
	return false
}

// IsQFPayMethod returns true if this gateway code uses QFPay API.
func IsQFPayMethod(code string) bool {
	switch code {
	case GatewayFPS, GatewayPayMe, GatewayAlipayHK, GatewayWeChatHK, GatewayBoCPay, GatewayOctopus:
		return true
	}
	return false
}

// StripePaymentMethodType returns the Stripe payment_method_type string for a gateway code.
// Returns empty string if not a Stripe native method.
func StripePaymentMethodType(code string) string {
	switch code {
	case GatewayAlipay:
		return "alipay"
	case GatewayWeChatPay:
		return "wechat_pay"
	case GatewayPayNow:
		return "paynow"
	case GatewayGrabPay:
		return "grabpay"
	default:
		return ""
	}
}

// QFPayPayType returns the QFPay pay_type string for a gateway code.
// Reference: https://sdk.qfapi.com/
func QFPayPayType(code string) string {
	switch code {
	case GatewayFPS:
		return "801107" // FPS Online
	case GatewayPayMe:
		return "805814" // PayMe Online
	case GatewayAlipayHK:
		return "801510" // Alipay HK Online
	case GatewayWeChatHK:
		return "800213" // WeChat Pay HK Online
	case GatewayBoCPay:
		return "805020" // BoC Pay Online
	case GatewayOctopus:
		return "805120" // Octopus Online
	default:
		return ""
	}
}

// GetPaymentGatewayRegistry returns the full list of supported payment gateway types.
// GET /api/v1/public/payment-gateways
func GetPaymentGatewayRegistry(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"gateways": GatewayRegistry,
	})
}
