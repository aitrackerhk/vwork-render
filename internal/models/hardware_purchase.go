package models

import (
	"time"

	"github.com/google/uuid"
)

type HardwarePurchase struct {
	ID                uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;index" json:"tenant_id"`
	UserID            *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"`
	Status            string     `gorm:"type:varchar(50);default:'created'" json:"status"`
	CheckoutSessionID string     `gorm:"type:varchar(255)" json:"checkout_session_id"`
	PaymentIntentID   *string    `gorm:"type:varchar(255)" json:"payment_intent_id,omitempty"`
	StripeCustomerID  *string    `gorm:"type:varchar(255)" json:"stripe_customer_id,omitempty"`
	Currency          string     `gorm:"type:varchar(20)" json:"currency"`
	AmountTotal       float64    `gorm:"type:numeric(12,2);default:0" json:"amount_total"`
	Items             JSONB      `gorm:"type:jsonb;default:'[]'" json:"items"`
	CompanyInfo       JSONB      `gorm:"type:jsonb" json:"company_info,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (HardwarePurchase) TableName() string {
	return "hardware_purchases"
}
