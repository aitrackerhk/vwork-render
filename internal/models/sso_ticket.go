package models

import (
	"time"

	"github.com/google/uuid"
)

// SSOTicket 跨域單點登入票據
// 用於在不同產品域名之間安全傳遞認證狀態
type SSOTicket struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	TenantID  *uuid.UUID `gorm:"type:uuid;index" json:"tenant_id,omitempty"`
	Ticket    string     `gorm:"type:varchar(128);not null;uniqueIndex" json:"ticket"`
	Used      bool       `gorm:"not null;default:false" json:"used"`
	ExpiresAt time.Time  `gorm:"type:timestamptz;not null" json:"expires_at"`
	CreatedAt time.Time  `gorm:"type:timestamptz;not null;default:now()" json:"created_at"`
}

func (SSOTicket) TableName() string {
	return "sso_tickets"
}
