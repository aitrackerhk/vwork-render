package models

import (
	"time"

	"github.com/google/uuid"
)

// SupportCommunication 客服通訊
type SupportCommunication struct {
	ID                uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant            Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	CustomerID        uuid.UUID  `gorm:"type:uuid;not null" json:"customer_id"`
	Customer          Customer   `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	StaffID           *uuid.UUID `gorm:"type:uuid" json:"staff_id,omitempty"`
	Staff             *User      `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
	CommunicationType string     `gorm:"type:varchar(50);not null" json:"communication_type"`
	Subject           string     `gorm:"type:varchar(255)" json:"subject"`
	Content           string     `gorm:"type:text;not null" json:"content"`
	Direction         string     `gorm:"type:varchar(20);not null" json:"direction"`
	Status            string     `gorm:"type:varchar(50);default:'open'" json:"status"`
	Priority          string     `gorm:"type:varchar(20);default:'normal'" json:"priority"`
	ResolvedAt        *time.Time `gorm:"type:timestamp" json:"resolved_at,omitempty"`
	ExtraFields       JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (SupportCommunication) TableName() string {
	return "support_communications"
}

// Promotion 推廣發送
type Promotion struct {
	ID              uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant          Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Title           string     `gorm:"type:varchar(255);not null" json:"title"`
	EmailSubject    string     `gorm:"type:varchar(255);default:''" json:"email_subject"`
	Content         string     `gorm:"type:text;not null" json:"content"`
	PromotionType   string     `gorm:"type:varchar(50);not null" json:"promotion_type"`
	TargetAudience  JSONB      `gorm:"type:jsonb;default:'{}'" json:"target_audience,omitempty"`
	ScheduledAt     *time.Time `gorm:"type:timestamp" json:"scheduled_at,omitempty"`
	SentAt          *time.Time `gorm:"type:timestamp" json:"sent_at,omitempty"`
	TotalRecipients int        `gorm:"default:0" json:"total_recipients"`
	SuccessCount    int        `gorm:"default:0" json:"success_count"`
	FailCount       int        `gorm:"default:0" json:"fail_count"`
	Status          string     `gorm:"type:varchar(50);default:'draft'" json:"status"`
	ExtraFields     JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (Promotion) TableName() string {
	return "promotions"
}
