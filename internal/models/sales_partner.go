package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SalesPartnerApplication 銷售商加盟申請
// 狀態：pending / approved / rejected
// Code 於審核通過時生成
// NOTE: Code 可供多個租戶使用（非一次性）
type SalesPartnerApplication struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name                 string     `gorm:"type:varchar(100);not null" json:"name"`
	ContactName          string     `gorm:"type:varchar(100);not null" json:"contact_name"`
	Company              string     `gorm:"type:varchar(200);not null" json:"company"`
	Email                string     `gorm:"type:varchar(255);not null" json:"email"`
	Phone                string     `gorm:"type:varchar(50);not null" json:"phone"`
	Region               *string    `gorm:"type:varchar(100)" json:"region,omitempty"`
	Message              *string    `gorm:"type:text" json:"message,omitempty"`
	Status               string     `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Code                 *string    `gorm:"type:varchar(50);uniqueIndex:idx_sales_partner_applications_code" json:"code,omitempty"`
	MonthlyPrice         *float64   `gorm:"type:decimal(10,2)" json:"monthly_price,omitempty"`
	YearlyPrice          *float64   `gorm:"type:decimal(10,2)" json:"yearly_price,omitempty"`
	Currency             *string    `gorm:"type:varchar(10)" json:"currency,omitempty"`
	StripePriceIDMonthly *string    `gorm:"type:varchar(255)" json:"stripe_price_id_monthly,omitempty"`
	StripePriceIDYearly  *string    `gorm:"type:varchar(255)" json:"stripe_price_id_yearly,omitempty"`
	TrialMonths          *int       `gorm:"type:integer" json:"trial_months,omitempty"`     // 銷售商設定的試用月數，最多 2 個月
	MonthlyAICoins       *int       `gorm:"type:integer" json:"monthly_ai_coins,omitempty"` // AI Coins 月配額 (200-500)，不設定=300
	ApprovedAt           *time.Time `gorm:"type:timestamp" json:"approved_at,omitempty"`
	RejectedAt           *time.Time `gorm:"type:timestamp" json:"rejected_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (SalesPartnerApplication) TableName() string {
	return "sales_partner_applications"
}

func (a *SalesPartnerApplication) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Status == "" {
		a.Status = "pending"
	}
	return nil
}
