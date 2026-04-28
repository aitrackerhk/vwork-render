package models

import (
	"time"

	"github.com/google/uuid"
)

// PaymentLink represents a shareable checkout link for an existing order.
type PaymentLink struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	OrderID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"order_id"`
	Token       string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"token"`
	Status      string     `gorm:"type:varchar(20);not null;default:'active'" json:"status"` // active, paid, expired, cancelled
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Notes       string     `gorm:"type:text;default:''" json:"notes"`
	CreatedBy   *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// Relations
	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Order  Order  `gorm:"foreignKey:OrderID" json:"order,omitempty"`
}

func (PaymentLink) TableName() string { return "payment_links" }
