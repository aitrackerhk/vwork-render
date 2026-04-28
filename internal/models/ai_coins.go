package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 每日免費查詢次數（所有方案相同，可用於交談及圖片生成，不含影片/文件生成）
const DailyFreeQueries = 5

// TenantAICoins vCoin 帳戶
type TenantAICoins struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	TenantID         uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"tenant_id"`
	Balance          int       `gorm:"default:0" json:"balance"`           // 當前可用餘額（購買的 + 剩餘月配額）
	MonthlyAllotment int       `gorm:"default:0" json:"monthly_allotment"` // 每月配額（根據訂閱方案）
	MonthlyUsed      int       `gorm:"default:0" json:"monthly_used"`      // 本月已用配額
	MonthlyResetAt   time.Time `json:"monthly_reset_at"`                   // 配額重置時間
	DailyFreeUsed    int       `gorm:"default:0" json:"daily_free_used"`   // 今日已用免費次數
	DailyFreeResetAt time.Time `json:"daily_free_reset_at"`                // 每日免費重置時間
	PurchasedBalance int       `gorm:"default:0" json:"purchased_balance"` // 購買的 coins 餘額（不過期）
	TotalPurchased   int       `gorm:"default:0" json:"total_purchased"`   // 歷史購買總量
	TotalUsed        int       `gorm:"default:0" json:"total_used"`        // 歷史消耗總量
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (t *TenantAICoins) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

func (TenantAICoins) TableName() string {
	return "tenant_ai_coins"
}

// GetAvailableCoins 獲取可用 coins 數量（月配額剩餘 + 購買的）
func (t *TenantAICoins) GetAvailableCoins() int {
	monthlyRemaining := t.MonthlyAllotment - t.MonthlyUsed
	if monthlyRemaining < 0 {
		monthlyRemaining = 0
	}
	return monthlyRemaining + t.PurchasedBalance
}

// GetDailyFreeRemaining 獲取今日剩餘免費次數
func (t *TenantAICoins) GetDailyFreeRemaining() int {
	remaining := DailyFreeQueries - t.DailyFreeUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// CanUseDailyFree 檢查是否可以使用每日免費
// 允許：交談(1)、圖片生成(3)、草圖(3)、OCR(1)、STT(1)、欄位匹配(1)、AI分析(1)
// 禁止：文件生成(5+)、影片生成(50)、影片延伸(50)
func (t *TenantAICoins) CanUseDailyFree(coinsRequired int) bool {
	// 禁止文件生成和影片生成/延伸（成本太高）
	if coinsRequired >= AICoinsDocGenBase {
		return false
	}
	return t.GetDailyFreeRemaining() > 0
}

// AICoinsTransaction vCoin 交易記錄
type AICoinsTransaction struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid;index" json:"tenant_id"`
	UserID       uuid.UUID `gorm:"type:uuid" json:"user_id"`
	Type         string    `gorm:"type:varchar(50);index" json:"type"` // purchase, monthly_reset, consume, refund, bonus
	Amount       int       `json:"amount"`                             // 正數=增加, 負數=消耗
	BalanceAfter int       `json:"balance_after"`                      // 交易後餘額
	Description  string    `gorm:"type:varchar(500)" json:"description"`
	Metadata     JSONB     `gorm:"type:jsonb" json:"metadata"` // 額外資訊
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

func (t *AICoinsTransaction) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

func (AICoinsTransaction) TableName() string {
	return "ai_coins_transactions"
}

// Transaction types
const (
	AICoinsTypePurchase     = "purchase"      // 購買
	AICoinsTypeMonthlyReset = "monthly_reset" // 月配額重置
	AICoinsTypeConsume      = "consume"       // 消耗
	AICoinsTypeRefund       = "refund"        // 退款
	AICoinsTypeBonus        = "bonus"         // 贈送/獎勵
	AICoinsTypeExpire       = "expire"        // 過期（月配額）
)

// AI 功能消耗定義（精準定價，根據實際 API 成本）
//
// 定價對照表（1 vCoin ≈ HK$0.2 ≈ US$0.026）：
//
//	簡單查詢       1 vCoin    — Gemini Flash 文字（成本 < $0.005）
//	圖片生成       3 vCoin    — Gemini Flash Image Gen（成本 ~$0.04）
//	草圖生成       3 vCoin    — Gemini Flash Image Gen（成本 ~$0.04）
//	文件生成       5+ vCoin   — 每 15 頁 5 vCoin，最低 5（多次 API call）
//	影片生成       20 vCoin   — BytePlus Seedance 1.5 Pro 每 5s 為單位（成本 ~US$0.08）
//	影片延伸       20 vCoin   — 已棄用（前端自動擷取最後一幀 + 新分鏡）
//	OCR            1 vCoin    — Google Vision API（成本 ~$0.0015）
//	STT            1 vCoin    — Google Speech API（成本 ~$0.006）
//	欄位匹配       1 vCoin    — Gemini Flash 文字
//	AI 建議/分析   1 vCoin    — Gemini Flash 文字
//	WhatsApp 推廣  3 vCoin/則 — Meta Cloud API 行銷對話（成本 ~US$0.073）
//	Email 推廣     1 vCoin/10封 — Brevo SMTP（成本 ~US$0.001/封）
const (
	AICoinsQuerySimple     = 1  // 簡單查詢 / 聊天
	AICoinsQueryOCR        = 1  // OCR 文字辨識
	AICoinsQuerySTT        = 1  // 語音轉文字
	AICoinsQueryMatch      = 1  // 欄位匹配
	AICoinsQueryAnalysis   = 1  // AI 建議 / 分析
	AICoinsImageGen        = 3  // 圖片生成
	AICoinsSketchGen       = 3  // 草圖生成
	AICoinsDocGenBase      = 5  // 文件生成 — 最低消耗
	AICoinsDocGenPer15     = 5  // 文件生成 — 每 15 頁額外消耗
	AICoinsVideoGen        = 30 // 影片生成（每 5s 為單位，BytePlus Seedance 1.5 Pro）
	AICoinsVideoExtend     = 30 // 影片延伸（已棄用，保留常數供向後相容）
	AICoinsMusicGen        = 5  // BGM 音樂生成（Lyria WebSocket bidiGenerateMusic）
	AICoinsWhatsApp        = 3  // WhatsApp 推廣（每則 3 vCoin ≈ US$0.078，成本 ~US$0.073）
	AICoinsEmailPer10      = 1  // Email 推廣（每 10 封 1 vCoin ≈ US$0.0026/封，成本 ~US$0.001/封）
	AICoinsLeadAnalyze     = 1  // Lead Finder AI 分析（Gemini 呼叫）
	AICoinsLeadSearchPer50 = 1  // Lead Finder 搜尋（每 50 筆結果 1 vCoin）
)

// CalcEmailPromoCoins 根據 email 收件人數計算消耗 vCoin
// 規則：每 10 封 1 vCoin，最低 1 vCoin，無條件進位
func CalcEmailPromoCoins(recipients int) int {
	if recipients <= 0 {
		return 0
	}
	// 每 10 封一個單位，無條件進位
	units := (recipients + 9) / 10
	cost := units * AICoinsEmailPer10
	if cost < 1 {
		cost = 1
	}
	return cost
}

// CalcWhatsAppPromoCoins 根據 WhatsApp 收件人數計算消耗 vCoin
func CalcWhatsAppPromoCoins(recipients int) int {
	return recipients * AICoinsWhatsApp
}

// CalcLeadSearchCoins 根據搜尋結果數計算消耗 vCoin
// 規則：每 50 筆結果 1 vCoin，最低 1 vCoin，無條件進位
func CalcLeadSearchCoins(resultCount int) int {
	if resultCount <= 0 {
		return 0
	}
	units := (resultCount + 49) / 50
	cost := units * AICoinsLeadSearchPer50
	if cost < 1 {
		cost = 1
	}
	return cost
}

// CalcDocGenCoins 根據文件頁數計算消耗 vCoin
// 規則：每 15 頁 5 vCoin，最低 5 vCoin
func CalcDocGenCoins(pages int) int {
	if pages <= 0 {
		return AICoinsDocGenBase
	}
	// 每 15 頁一個單位，無條件進位
	units := (pages + 14) / 15
	cost := units * AICoinsDocGenPer15
	if cost < AICoinsDocGenBase {
		cost = AICoinsDocGenBase
	}
	return cost
}

// AICoinsPlan AI Coins 購買套餐
type AICoinsPlan struct {
	ID        string    `gorm:"primaryKey" json:"id"` // small, medium, large, mega
	Name      string    `gorm:"type:varchar(100)" json:"name"`
	NameEn    string    `gorm:"type:varchar(100)" json:"name_en"`
	Coins     int       `json:"coins"`
	PriceHKD  int       `json:"price_hkd"`
	PriceUSD  int       `json:"price_usd"`
	Discount  int       `json:"discount"` // 折扣百分比
	Popular   bool      `json:"popular"`  // 是否熱門
	Active    bool      `gorm:"default:true" json:"active"`
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AICoinsPlan) TableName() string {
	return "ai_coins_plans"
}

// 預設套餐
var DefaultAICoinsPlans = []AICoinsPlan{
	{
		ID:        "small",
		Name:      "小包",
		NameEn:    "Small Pack",
		Coins:     100,
		PriceHKD:  20,
		PriceUSD:  3,
		Discount:  0,
		Popular:   false,
		Active:    true,
		SortOrder: 1,
	},
	{
		ID:        "medium",
		Name:      "中包",
		NameEn:    "Medium Pack",
		Coins:     300,
		PriceHKD:  50,
		PriceUSD:  7,
		Discount:  15,
		Popular:   true,
		Active:    true,
		SortOrder: 2,
	},
	{
		ID:        "large",
		Name:      "大包",
		NameEn:    "Large Pack",
		Coins:     700,
		PriceHKD:  100,
		PriceUSD:  13,
		Discount:  30,
		Popular:   false,
		Active:    true,
		SortOrder: 3,
	},
	{
		ID:        "mega",
		Name:      "超值包",
		NameEn:    "Mega Pack",
		Coins:     1600,
		PriceHKD:  200,
		PriceUSD:  26,
		Discount:  37,
		Popular:   false,
		Active:    true,
		SortOrder: 4,
	},
}

// 訂閱方案的月配額
// free/trial: 0 coins（只有每日 5 次免費）
// personal: 100 coins（預留）
// vsuite_monthly/vsuite_yearly: 500 coins（直營），訂閱用戶無每日免費，全部扣 coin
// vsuite_pro_monthly/vsuite_pro_yearly: 1000 coins（直營），含 vAi Video
// vsuite_pro_plus_monthly/vsuite_pro_plus_yearly: 3000 coins，含 vAi Video + 自動搵客
// 加盟商客戶: 預設 3000，可設定
// Legacy: monthly/yearly mapped to vsuite equivalent
var PlanMonthlyCoins = map[string]int{
	"free":                    0,
	"trial":                   0,
	"personal":                100,
	"monthly":                 500, // legacy → vsuite
	"yearly":                  500, // legacy → vsuite
	"vsuite_monthly":          500,
	"vsuite_yearly":           500,
	"vsuite_pro_monthly":      1000,
	"vsuite_pro_yearly":       1000,
	"vsuite_pro_plus_monthly": 3000,
	"vsuite_pro_plus_yearly":  3000,
}

// IsPaidPlan 檢查是否為付費方案（訂閱用戶無每日免費查詢）
func IsPaidPlan(plan string) bool {
	switch plan {
	case "monthly", "yearly",
		"vsuite_monthly", "vsuite_yearly",
		"vsuite_pro_monthly", "vsuite_pro_yearly",
		"vsuite_pro_plus_monthly", "vsuite_pro_plus_yearly":
		return true
	}
	return false
}

// 加盟商配額常數
const (
	SalesPartnerDefaultCoins = 2000 // 加盟商預設配額
	SalesPartnerMinCoins     = 2000 // 加盟商最低配額
	SalesPartnerMaxCoins     = 2200 // 加盟商最高配額
)

// GetPlanMonthlyCoins 獲取方案的月配額（直營用戶）
func GetPlanMonthlyCoins(plan string) int {
	if coins, ok := PlanMonthlyCoins[plan]; ok {
		return coins
	}
	return 0 // 預設（只有每日免費）
}

// GetPlanMonthlyCoinsWithPartner 獲取方案的月配額（考慮加盟商設定）
// salesPartnerCoins: 加盟商設定的配額，nil 表示使用預設
func GetPlanMonthlyCoinsWithPartner(plan string, salesPartnerCoins *int) int {
	// 只有付費方案才考慮加盟商配額
	if !IsPaidPlan(plan) {
		return GetPlanMonthlyCoins(plan)
	}

	// 有加盟商設定
	if salesPartnerCoins != nil {
		coins := *salesPartnerCoins
		// 確保在範圍內
		if coins < SalesPartnerMinCoins {
			coins = SalesPartnerMinCoins
		}
		if coins > SalesPartnerMaxCoins {
			coins = SalesPartnerMaxCoins
		}
		return coins
	}

	// 無設定 = 直營標準配額
	return GetPlanMonthlyCoins(plan)
}

// ---------------------------------------------------------------------------
// Feature gating helpers
// ---------------------------------------------------------------------------
// 不同方案可使用的功能：
//   vSuite:     基本功能，無 vAi Video、無自動搵客
//   vSuite Pro: 含 vAi Video，無自動搵客
//   vSuite Pro+: 含 vAi Video + 自動搵客
// ---------------------------------------------------------------------------

// PlanHasVideoGen 檢查方案是否包含 vAi 影片生成功能（vSuite Pro 及以上）
func PlanHasVideoGen(plan string) bool {
	switch plan {
	case "vsuite_pro_monthly", "vsuite_pro_yearly",
		"vsuite_pro_plus_monthly", "vsuite_pro_plus_yearly":
		return true
	}
	return false
}

// PlanHasLeadFinder 檢查方案是否包含自動搵客功能（僅 vSuite Pro+）
func PlanHasLeadFinder(plan string) bool {
	switch plan {
	case "vsuite_pro_plus_monthly", "vsuite_pro_plus_yearly":
		return true
	}
	return false
}

// RequiredPlanForVideo 返回使用 vAi Video 所需的最低方案名稱
func RequiredPlanForVideo() string {
	return "vSuite Pro"
}

// RequiredPlanForLeadFinder 返回使用自動搵客所需的最低方案名稱
func RequiredPlanForLeadFinder() string {
	return "vSuite Pro+"
}
