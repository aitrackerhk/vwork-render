package models

import (
	"time"

	"github.com/google/uuid"
)

// EmailJob is a DB-backed queue item for sending outbound emails.
// Table: email_jobs
type EmailJob struct {
	ID uint64 `gorm:"primaryKey;autoIncrement" json:"id"`

	TenantID *uuid.UUID `gorm:"type:uuid" json:"tenant_id,omitempty"`
	UserID   *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"`

	Kind           string  `gorm:"type:varchar(50);not null" json:"kind"`
	IdempotencyKey *string `gorm:"type:varchar(200);uniqueIndex" json:"idempotency_key,omitempty"`

	ToEmail     string                `gorm:"type:varchar(255);not null" json:"to_email"`
	Subject     string                `gorm:"type:varchar(255);not null" json:"subject"`
	BodyText    string                `gorm:"type:text" json:"body_text"`
	BodyHTML    string                `gorm:"type:text" json:"body_html"`
	Attachments EmailAttachmentsJSONB `gorm:"type:jsonb" json:"attachments,omitempty"`

	Status   string    `gorm:"type:varchar(20);not null;default:'queued'" json:"status"`
	Attempts int       `gorm:"not null;default:0" json:"attempts"`
	RunAt    time.Time `gorm:"not null" json:"run_at"`

	LockedAt  *time.Time `json:"locked_at,omitempty"`
	LockedBy  *string    `gorm:"type:varchar(100)" json:"locked_by,omitempty"`
	LastError *string    `gorm:"type:text" json:"last_error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (EmailJob) TableName() string { return "email_jobs" }
