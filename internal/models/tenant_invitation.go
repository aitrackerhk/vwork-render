package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TenantInvitation represents an invitation for someone to join a tenant.
// The invited person may or may not already have a vWork account.
type TenantInvitation struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant         *Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	InviterID      uuid.UUID  `gorm:"type:uuid;not null" json:"inviter_id"`
	Inviter        *User      `gorm:"foreignKey:InviterID" json:"inviter,omitempty"`
	Email          string     `gorm:"type:varchar(255);not null;index" json:"email"`
	TokenHash      []byte     `gorm:"type:bytea;not null;index" json:"-"`
	Status         string     `gorm:"type:varchar(50);not null;default:'pending'" json:"status"` // pending, accepted, expired, cancelled
	ExpiresAt      time.Time  `gorm:"not null" json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	AcceptedUserID *uuid.UUID `gorm:"type:uuid" json:"accepted_user_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (t *TenantInvitation) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

func (TenantInvitation) TableName() string {
	return "tenant_invitations"
}
