package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// APIToken represents a system-level API token for external services (e.g. v01, vOffice, third-party integrations).
// Holders of a valid token can call vWork API without user login.
type APIToken struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string     `gorm:"type:varchar(255);not null" json:"name"`                   // Human-readable label, e.g. "v01 Production"
	Product     string     `gorm:"type:varchar(20);not null" json:"product"`                 // Product this token belongs to: vai, vwork, vmarket, voffice
	TokenHash   string     `gorm:"type:varchar(128);not null;uniqueIndex" json:"-"`          // SHA-256 hash of the plain token (never store raw)
	TokenPrefix string     `gorm:"type:varchar(12);not null" json:"token_prefix"`            // First 8 chars of token for identification (e.g. "vwk_abc1")
	Scopes      string     `gorm:"type:text;default:'*'" json:"scopes"`                      // Comma-separated scopes, "*" = full access
	Status      string     `gorm:"type:varchar(20);not null;default:'active'" json:"status"` // active, revoked
	CreatedByID uuid.UUID  `gorm:"type:uuid;not null" json:"created_by_id"`                  // The user who created this token
	LastUsedAt  *time.Time `gorm:"type:timestamptz" json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `gorm:"type:timestamptz" json:"expires_at,omitempty"` // nil = never expires
	RevokedAt   *time.Time `gorm:"type:timestamptz" json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `gorm:"type:timestamptz;not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"type:timestamptz;not null;default:now()" json:"updated_at"`

	// Relations (for preloading)
	Tenant    Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	CreatedBy User   `gorm:"foreignKey:CreatedByID" json:"created_by,omitempty"`
}

func (APIToken) TableName() string {
	return "api_tokens"
}

// BeforeCreate sets UUID if not provided.
func (t *APIToken) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}
