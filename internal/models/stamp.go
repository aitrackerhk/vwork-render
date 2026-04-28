package models

import (
	"time"

	"github.com/google/uuid"
)

// StampSetting 印花設定
type StampSetting struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name        string    `gorm:"type:varchar(255);not null;default:'印花活動'" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Status      string    `gorm:"type:varchar(20);not null;default:'active'" json:"status"` // active, inactive

	// 購買特定產品獲得印花設定
	ProductStampEnabled    bool `gorm:"default:false" json:"product_stamp_enabled"`
	ProductStampCount      int  `gorm:"default:1" json:"product_stamp_count"`          // 每次獲得幾個印花
	ProductStampDailyLimit *int `gorm:"type:integer" json:"product_stamp_daily_limit"` // 每日每產品獲得上限 (NULL = 無上限)

	// 購買特定服務獲得印花設定
	ServiceStampEnabled    bool `gorm:"default:false" json:"service_stamp_enabled"`
	ServiceStampCount      int  `gorm:"default:1" json:"service_stamp_count"`          // 每次獲得幾個印花
	ServiceStampDailyLimit *int `gorm:"type:integer" json:"service_stamp_daily_limit"` // 每日每服務獲得上限 (NULL = 無上限)

	// 購買特定金額獲得印花設定
	AmountStampEnabled    bool    `gorm:"default:false" json:"amount_stamp_enabled"`
	AmountPerStamp        float64 `gorm:"type:decimal(18,2);default:100.00" json:"amount_per_stamp"` // 每消費多少金額獲得1印花
	AmountStampDailyLimit *int    `gorm:"type:integer" json:"amount_stamp_daily_limit"`              // 每日獲得上限 (NULL = 無上限)

	ValidFrom *time.Time `gorm:"type:timestamp with time zone" json:"valid_from,omitempty"`
	ValidTo   *time.Time `gorm:"type:timestamp with time zone" json:"valid_to,omitempty"`

	ExtraFields JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 關聯
	EarningProducts    []StampEarningProduct    `gorm:"foreignKey:StampSettingID" json:"earning_products,omitempty"`
	EarningServices    []StampEarningService    `gorm:"foreignKey:StampSettingID" json:"earning_services,omitempty"`
	RedeemableProducts []StampRedeemableProduct `gorm:"foreignKey:StampSettingID" json:"redeemable_products,omitempty"`
}

func (StampSetting) TableName() string {
	return "stamp_settings"
}

// StampEarningProduct 印花獲取產品設定
type StampEarningProduct struct {
	ID             uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID    `gorm:"type:uuid;not null" json:"tenant_id"`
	StampSettingID uuid.UUID    `gorm:"type:uuid;not null" json:"stamp_setting_id"`
	StampSetting   StampSetting `gorm:"foreignKey:StampSettingID" json:"stamp_setting,omitempty"`
	ProductID      uuid.UUID    `gorm:"type:uuid;not null" json:"product_id"`
	Product        Product      `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	StampCount     int          `gorm:"default:1" json:"stamp_count"` // 購買此產品獲得幾個印花
	CreatedAt      time.Time    `json:"created_at"`
}

func (StampEarningProduct) TableName() string {
	return "stamp_earning_products"
}

// StampEarningService 印花獲取服務設定
type StampEarningService struct {
	ID             uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID    `gorm:"type:uuid;not null" json:"tenant_id"`
	StampSettingID uuid.UUID    `gorm:"type:uuid;not null" json:"stamp_setting_id"`
	StampSetting   StampSetting `gorm:"foreignKey:StampSettingID" json:"stamp_setting,omitempty"`
	ServiceID      uuid.UUID    `gorm:"type:uuid;not null" json:"service_id"`
	Service        Service      `gorm:"foreignKey:ServiceID" json:"service,omitempty"`
	StampCount     int          `gorm:"default:1" json:"stamp_count"` // 購買此服務獲得幾個印花
	CreatedAt      time.Time    `json:"created_at"`
}

func (StampEarningService) TableName() string {
	return "stamp_earning_services"
}

// StampRedeemableProduct 印花可換購產品設定
type StampRedeemableProduct struct {
	ID             uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID    `gorm:"type:uuid;not null" json:"tenant_id"`
	StampSettingID uuid.UUID    `gorm:"type:uuid;not null" json:"stamp_setting_id"`
	StampSetting   StampSetting `gorm:"foreignKey:StampSettingID" json:"stamp_setting,omitempty"`
	ProductID      uuid.UUID    `gorm:"type:uuid;not null" json:"product_id"`
	Product        Product      `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	StampsRequired int          `gorm:"not null;default:10" json:"stamps_required"` // 需要多少印花換購
	QuantityLimit  *int         `gorm:"type:integer" json:"quantity_limit"`         // 每次可換數量上限
	DailyLimit     *int         `gorm:"type:integer" json:"daily_limit"`            // 每日可換上限
	Status         string       `gorm:"type:varchar(20);not null;default:'active'" json:"status"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

func (StampRedeemableProduct) TableName() string {
	return "stamp_redeemable_products"
}

// StampRecord 客戶印花記錄
type StampRecord struct {
	ID             uuid.UUID     `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID     `gorm:"type:uuid;not null" json:"tenant_id"`
	CustomerID     uuid.UUID     `gorm:"type:uuid;not null" json:"customer_id"`
	Customer       Customer      `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	StampSettingID *uuid.UUID    `gorm:"type:uuid" json:"stamp_setting_id"`
	StampSetting   *StampSetting `gorm:"foreignKey:StampSettingID" json:"stamp_setting,omitempty"`
	RecordType     string        `gorm:"type:varchar(20);not null" json:"record_type"` // earn (獲得), redeem (兌換)
	StampCount     int           `gorm:"not null" json:"stamp_count"`                  // 正數=獲得, 負數=使用
	BalanceAfter   int           `gorm:"not null;default:0" json:"balance_after"`      // 操作後餘額

	// 來源記錄
	SourceType string     `gorm:"type:varchar(50)" json:"source_type"` // order, service_order, manual, expired
	SourceID   *uuid.UUID `gorm:"type:uuid" json:"source_id"`          // 關聯的訂單/服務單 ID
	ProductID  *uuid.UUID `gorm:"type:uuid" json:"product_id"`
	Product    *Product   `gorm:"foreignKey:ProductID" json:"product,omitempty"`

	Notes       string     `gorm:"type:text" json:"notes"`
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   *uuid.UUID `gorm:"type:uuid" json:"created_by"`
}

func (StampRecord) TableName() string {
	return "stamp_records"
}

// CustomerStampBalance 客戶印花餘額
type CustomerStampBalance struct {
	ID             uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID    `gorm:"type:uuid;not null" json:"tenant_id"`
	CustomerID     uuid.UUID    `gorm:"type:uuid;not null" json:"customer_id"`
	Customer       Customer     `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	StampSettingID uuid.UUID    `gorm:"type:uuid;not null" json:"stamp_setting_id"`
	StampSetting   StampSetting `gorm:"foreignKey:StampSettingID" json:"stamp_setting,omitempty"`
	Balance        int          `gorm:"not null;default:0" json:"balance"`
	TotalEarned    int          `gorm:"not null;default:0" json:"total_earned"`
	TotalRedeemed  int          `gorm:"not null;default:0" json:"total_redeemed"`
	LastEarnedAt   *time.Time   `gorm:"type:timestamp with time zone" json:"last_earned_at"`
	LastRedeemedAt *time.Time   `gorm:"type:timestamp with time zone" json:"last_redeemed_at"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

func (CustomerStampBalance) TableName() string {
	return "customer_stamp_balances"
}

// StampDailyRecord 每日印花記錄
type StampDailyRecord struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	CustomerID     uuid.UUID `gorm:"type:uuid;not null" json:"customer_id"`
	StampSettingID uuid.UUID `gorm:"type:uuid;not null" json:"stamp_setting_id"`
	RecordDate     time.Time `gorm:"type:date;not null" json:"record_date"`

	// 產品印花
	ProductID           *uuid.UUID `gorm:"type:uuid" json:"product_id"`
	ProductStampsEarned int        `gorm:"default:0" json:"product_stamps_earned"`

	// 金額印花
	AmountStampsEarned int `gorm:"default:0" json:"amount_stamps_earned"`

	// 兌換記錄
	RedeemedProductID *uuid.UUID `gorm:"type:uuid" json:"redeemed_product_id"`
	RedeemedCount     int        `gorm:"default:0" json:"redeemed_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (StampDailyRecord) TableName() string {
	return "stamp_daily_records"
}
