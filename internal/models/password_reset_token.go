package models

import (
	"time"

	"github.com/google/uuid"
)

// PasswordResetToken stores a one-time password reset token (hashed).
// Table: password_reset_tokens
type PasswordResetToken struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`

	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID   uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`

	TokenHash []byte     `gorm:"type:bytea;not null;uniqueIndex" json:"-"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

func (PasswordResetToken) TableName() string { return "password_reset_tokens" }





