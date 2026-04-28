package models

import (
	"time"

	"github.com/google/uuid"
)

// NotificationAlert represents a system-generated alert for a user
// Table: notification_alerts
type NotificationAlert struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant    Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`

	Type    string `gorm:"type:varchar(50);not null" json:"type"`    // e.g. 'appointment_today', 'low_stock', 'payment_due'
	Title   string `gorm:"type:varchar(255);not null" json:"title"`
	Message string `gorm:"type:text;not null" json:"message"`
	Link    string `gorm:"type:varchar(500)" json:"link,omitempty"` // optional link to related page

	// 用於 cron/worker 防止重覆生成（可空）
	DedupeKey string `gorm:"type:varchar(255)" json:"dedupe_key,omitempty"`

	IsRead  bool       `gorm:"default:false" json:"is_read"`
	ReadAt  *time.Time `gorm:"type:timestamp" json:"read_at,omitempty"`

	GeneratedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"generated_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (NotificationAlert) TableName() string {
	return "notification_alerts"
}

