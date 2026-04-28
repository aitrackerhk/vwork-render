package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AutoOutreachCampaign represents a fully automated outreach campaign
// that periodically finds leads and sends email/WhatsApp to them.
// Execution frequency: at most once per hour (enforced by cronjob).
type AutoOutreachCampaign struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	CreatedByID uuid.UUID `gorm:"type:uuid;not null" json:"created_by_id"`

	// Campaign settings
	Name        string `gorm:"type:varchar(255);not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`

	// Lead search parameters (reuse lead finder logic)
	SearchKeywords string     `gorm:"type:text" json:"search_keywords"`                // Keywords for Google search
	SearchRegion   string     `gorm:"type:varchar(255)" json:"search_region"`          // Target region
	ProductID      *uuid.UUID `gorm:"type:uuid" json:"product_id,omitempty"`           // Optional product focus
	ContactType    string     `gorm:"type:varchar(50);default:''" json:"contact_type"` // "", "email", "phone", "both"
	ResultLimit    int        `gorm:"not null;default:50" json:"result_limit"`         // 50, 100, 200, 500

	// Outreach channel: "email", "whatsapp", "both"
	Channel string `gorm:"type:varchar(50);not null;default:'email'" json:"channel"`

	// Message content (can be AI-generated)
	EmailSubject   string `gorm:"type:varchar(500)" json:"email_subject"`
	MessageContent string `gorm:"type:text" json:"message_content"` // HTML for email, text for WhatsApp

	// Schedule control
	IsActive         bool       `gorm:"not null;default:true" json:"is_active"`             // Master on/off switch
	IntervalMinutes  int        `gorm:"not null;default:60" json:"interval_minutes"`        // Min 60 (1 hour)
	MaxLeadsPerRun   int        `gorm:"not null;default:10" json:"max_leads_per_run"`       // How many new leads to find per run
	MaxSendsPerRun   int        `gorm:"not null;default:10" json:"max_sends_per_run"`       // How many messages to send per run
	TotalSentCount   int        `gorm:"not null;default:0" json:"total_sent_count"`         // Lifetime sent count
	TotalLeadsFound  int        `gorm:"not null;default:0" json:"total_leads_found"`        // Lifetime leads found
	LastRunAt        *time.Time `gorm:"type:timestamptz" json:"last_run_at,omitempty"`      // Last execution time
	NextRunAt        *time.Time `gorm:"type:timestamptz" json:"next_run_at,omitempty"`      // Next scheduled run
	LastRunStatus    string     `gorm:"type:varchar(50);default:''" json:"last_run_status"` // success, partial, failed, quota_exceeded
	LastRunMessage   string     `gorm:"type:text" json:"last_run_message"`                  // Details of last run
	ConsecutiveFails int        `gorm:"not null;default:0" json:"consecutive_fails"`        // Auto-pause after 3 consecutive fails

	// Status: active, paused, completed, quota_exceeded
	Status      string    `gorm:"type:varchar(50);not null;default:'active'" json:"status"`
	ExtraFields JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AutoOutreachCampaign) TableName() string {
	return "auto_outreach_campaigns"
}

func (c *AutoOutreachCampaign) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.ExtraFields == nil {
		c.ExtraFields = make(JSONB)
	}
	if c.IntervalMinutes < 60 {
		c.IntervalMinutes = 60
	}
	return nil
}

// AutoOutreachLog records each execution run of an auto-outreach campaign.
type AutoOutreachLog struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	CampaignID uuid.UUID `gorm:"type:uuid;not null;index" json:"campaign_id"`

	LeadsFound     int    `gorm:"not null;default:0" json:"leads_found"`
	EmailsSent     int    `gorm:"not null;default:0" json:"emails_sent"`
	WhatsAppSent   int    `gorm:"not null;default:0" json:"whatsapp_sent"`
	FailCount      int    `gorm:"not null;default:0" json:"fail_count"`
	Status         string `gorm:"type:varchar(50);not null" json:"status"` // success, partial, failed, quota_exceeded
	Message        string `gorm:"type:text" json:"message"`
	QuotaUsed      int64  `gorm:"not null;default:0" json:"quota_used"`
	QuotaRemaining int64  `gorm:"not null;default:0" json:"quota_remaining"`

	ExtraFields JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (AutoOutreachLog) TableName() string {
	return "auto_outreach_logs"
}

func (l *AutoOutreachLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	if l.ExtraFields == nil {
		l.ExtraFields = make(JSONB)
	}
	return nil
}
