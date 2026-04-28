package models

import (
	"time"

	"github.com/google/uuid"
)

// ReservedNumber 預留編號
type ReservedNumber struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	FieldName  string    `gorm:"type:varchar(100);not null" json:"field_name"`
	FieldValue string    `gorm:"type:varchar(255);not null" json:"field_value"`
	PageName   string    `gorm:"type:varchar(100)" json:"page_name"`
	CreatedAt  time.Time `json:"created_at"`
}

func (ReservedNumber) TableName() string {
	return "reserved_numbers"
}

