package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Plan name constants
const (
	PlanFree                 = "free"
	PlanTrial                = "trial"
	PlanVSuiteMonthly        = "vsuite_monthly"
	PlanVSuiteYearly         = "vsuite_yearly"
	PlanVSuiteProMonthly     = "vsuite_pro_monthly"
	PlanVSuiteProYearly      = "vsuite_pro_yearly"
	PlanVSuiteProPlusMonthly = "vsuite_pro_plus_monthly"
	PlanVSuiteProPlusYearly  = "vsuite_pro_plus_yearly"
	// Legacy plan names (kept for backward compatibility)
	PlanMonthly = "monthly"
	PlanYearly  = "yearly"
)

// Payment provider constants
const (
	PaymentProviderStripe = "stripe"
	PaymentProviderGoogle = "google"
	PaymentProviderApple  = "apple"
)

// IAP purchase status constants
const (
	IAPStatusPurchased = "purchased"
	IAPStatusPending   = "pending"
	IAPStatusRefunded  = "refunded"
	IAPStatusExpired   = "expired"
	IAPStatusCancelled = "cancelled"
)

// IAP purchase type constants
const (
	IAPTypeSubscription = "subscription"
	IAPTypeConsumable   = "consumable"
)

// SubscriptionPlan 訂閱計劃
type SubscriptionPlan struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name            string    `gorm:"type:varchar(100);not null" json:"name"`         // vsuite_monthly, vsuite_yearly, vsuite_pro_monthly, vsuite_pro_yearly
	DisplayName     string    `gorm:"type:varchar(255);not null" json:"display_name"` // 月付方案, 年付方案
	Price           float64   `gorm:"type:decimal(10,2);not null" json:"price"`       // 每月價格
	YearlyPrice     float64   `gorm:"type:decimal(10,2)" json:"yearly_price"`         // 年付總價（僅年付計劃）
	Interval        string    `gorm:"type:varchar(20);not null" json:"interval"`      // month, year
	StripePriceID   string    `gorm:"type:varchar(255)" json:"stripe_price_id"`       // Stripe Price ID
	GoogleProductID string    `gorm:"type:varchar(255)" json:"google_product_id"`     // Google Play product ID
	AppleProductID  string    `gorm:"type:varchar(255)" json:"apple_product_id"`      // App Store product ID
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Subscription 訂閱記錄
type Subscription struct {
	ID                         uuid.UUID        `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID                   uuid.UUID        `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant                     Tenant           `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	PlanID                     uuid.UUID        `gorm:"type:uuid;not null" json:"plan_id"`
	Plan                       SubscriptionPlan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
	PaymentProvider            string           `gorm:"type:varchar(20);default:'stripe'" json:"payment_provider"` // stripe, google, apple
	StripeSubscriptionID       string           `gorm:"type:varchar(255);uniqueIndex" json:"stripe_subscription_id"`
	GooglePurchaseToken        string           `gorm:"type:text" json:"google_purchase_token,omitempty"`
	GoogleOrderID              string           `gorm:"type:varchar(255)" json:"google_order_id,omitempty"`
	AppleOriginalTransactionID string           `gorm:"type:varchar(255)" json:"apple_original_transaction_id,omitempty"`
	AppleTransactionID         string           `gorm:"type:varchar(255)" json:"apple_transaction_id,omitempty"`
	Status                     string           `gorm:"type:varchar(50);not null" json:"status"` // active, canceled, past_due, trialing
	CurrentPeriodStart         *time.Time       `gorm:"type:timestamp" json:"current_period_start"`
	CurrentPeriodEnd           *time.Time       `gorm:"type:timestamp" json:"current_period_end"`
	CancelAtPeriodEnd          bool             `gorm:"default:false" json:"cancel_at_period_end"`
	CreatedAt                  time.Time        `json:"created_at"`
	UpdatedAt                  time.Time        `json:"updated_at"`
}

// IAPPurchase IAP 購買記錄（審計追蹤）
type IAPPurchase struct {
	ID                         uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID                   uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID                     uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	Platform                   string     `gorm:"type:varchar(20);not null" json:"platform"`      // google, apple
	ProductID                  string     `gorm:"type:varchar(255);not null" json:"product_id"`   // IAP product ID
	PurchaseType               string     `gorm:"type:varchar(20);not null" json:"purchase_type"` // subscription, consumable
	GooglePurchaseToken        string     `gorm:"type:text" json:"google_purchase_token,omitempty"`
	GoogleOrderID              string     `gorm:"type:varchar(255)" json:"google_order_id,omitempty"`
	AppleTransactionID         string     `gorm:"type:varchar(255)" json:"apple_transaction_id,omitempty"`
	AppleOriginalTransactionID string     `gorm:"type:varchar(255)" json:"apple_original_transaction_id,omitempty"`
	AppleEnvironment           string     `gorm:"type:varchar(20)" json:"apple_environment,omitempty"` // Production, Sandbox
	Status                     string     `gorm:"type:varchar(50);not null" json:"status"`             // purchased, pending, refunded, expired, cancelled
	Amount                     float64    `gorm:"type:decimal(10,2)" json:"amount"`
	Currency                   string     `gorm:"type:varchar(10)" json:"currency"`
	VerifiedAt                 *time.Time `gorm:"type:timestamp" json:"verified_at"`
	ExpiresAt                  *time.Time `gorm:"type:timestamp" json:"expires_at"`
	RawReceipt                 string     `gorm:"type:jsonb" json:"raw_receipt,omitempty"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
}

func (SubscriptionPlan) TableName() string {
	return "subscription_plans"
}

func (Subscription) TableName() string {
	return "subscriptions"
}

func (IAPPurchase) TableName() string {
	return "iap_purchases"
}

// BeforeCreate 創建前設置 UUID
func (s *SubscriptionPlan) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (s *Subscription) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (p *IAPPurchase) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
