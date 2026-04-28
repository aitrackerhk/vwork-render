package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Tenant struct {
	ID                 uuid.UUID         `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name               string            `gorm:"type:varchar(255);not null" json:"name"`
	Subdomain          string            `gorm:"type:varchar(100);uniqueIndex;not null" json:"subdomain"`
	Plan               string            `gorm:"type:varchar(50);default:'trial'" json:"plan"` // trial, monthly, yearly
	Status             string            `gorm:"type:varchar(50);default:'active'" json:"status"` // active, suspended, cancelled, trial_expired
	TrialExpiresAt     *time.Time        `gorm:"type:timestamp" json:"trial_expires_at,omitempty"`
	SubscriptionID     *string           `gorm:"type:varchar(255)" json:"subscription_id,omitempty"` // Stripe subscription ID
	StripeCustomerID              *string           `gorm:"type:varchar(255)" json:"stripe_customer_id,omitempty"`
	StripeConnectAccountID        *string           `gorm:"type:varchar(255)" json:"stripe_connect_account_id,omitempty"`
	StripeConnectOnboardingComplete bool             `gorm:"default:false" json:"stripe_connect_onboarding_complete"`
	IndustryTemplateID *uuid.UUID        `gorm:"type:uuid" json:"industry_template_id,omitempty"`
	IndustryTemplate   *IndustryTemplate `gorm:"foreignKey:IndustryTemplateID" json:"industry_template,omitempty"`
	WebsiteTheme       *string           `gorm:"type:varchar(50)" json:"website_theme,omitempty"`
	WebsiteType        *string           `gorm:"type:varchar(50)" json:"website_type,omitempty"` // 'ecommerce', 'general'
	WebsiteEnabled     bool              `gorm:"default:false" json:"website_enabled"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	ExtraFields        JSONB             `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
}

type JSONB map[string]interface{}

// Value 實現 driver.Valuer 接口
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(j)
}

// Scan 實現 sql.Scanner 接口
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = make(JSONB)
		return nil
	}
	
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return nil
	}
	
	if len(bytes) == 0 {
		*j = make(JSONB)
		return nil
	}
	
	// 嘗試解析為 map
	var m map[string]interface{}
	if err := json.Unmarshal(bytes, &m); err == nil {
		*j = JSONB(m)
		return nil
	}
	
	// 如果解析為 map 失敗，嘗試解析為數組或其他類型
	var anyValue interface{}
	if err := json.Unmarshal(bytes, &anyValue); err == nil {
		// 如果是數組或其他類型，包裝在 map 中
		*j = JSONB{"_data": anyValue}
		return nil
	}
	
	return json.Unmarshal(bytes, (*map[string]interface{})(j))
}

// BeforeCreate 創建前設置 UUID
func (t *Tenant) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.ExtraFields == nil {
		t.ExtraFields = make(JSONB)
	}
	return nil
}

