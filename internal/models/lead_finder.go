package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LeadFinderSearch represents an AI-powered lead search session
type LeadFinderSearch struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	CreatedByID uuid.UUID `gorm:"type:uuid;not null" json:"created_by_id"`
	// AI-generated search parameters
	Keywords       string `gorm:"type:text" json:"keywords"`                // AI-generated keywords
	TargetIndustry string `gorm:"type:varchar(255)" json:"target_industry"` // AI-inferred target industry
	AIAnalysis     string `gorm:"type:text" json:"ai_analysis"`             // Full AI analysis text
	// User filters
	ProductID   *uuid.UUID `gorm:"type:uuid" json:"product_id,omitempty"`                     // Targeted product
	Region      string     `gorm:"type:varchar(255)" json:"region"`                           // Target region/area
	Status      string     `gorm:"type:varchar(50);not null;default:'pending'" json:"status"` // pending, searching, completed, failed
	ResultCount int        `gorm:"default:0" json:"result_count"`
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	TrashedAt   *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (LeadFinderSearch) TableName() string {
	return "lead_finder_searches"
}

func (s *LeadFinderSearch) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.ExtraFields == nil {
		s.ExtraFields = make(JSONB)
	}
	return nil
}

// LeadFinderResult represents a single lead found from web search
type LeadFinderResult struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	SearchID uuid.UUID `gorm:"type:uuid;not null;index" json:"search_id"`
	// Extracted data
	CompanyName     string `gorm:"type:varchar(500)" json:"company_name"`
	Website         string `gorm:"type:varchar(1000)" json:"website"`
	WebsiteDomain   string `gorm:"type:varchar(255)" json:"website_domain"` // Extracted domain for dedup
	Phone           string `gorm:"type:varchar(255)" json:"phone"`
	NormalizedPhone string `gorm:"type:varchar(50)" json:"normalized_phone"` // Digits-only for dedup
	Email           string `gorm:"type:varchar(500)" json:"email"`
	Address         string `gorm:"type:text" json:"address"`
	Description     string `gorm:"type:text" json:"description"`           // Snippet / summary
	SourceURL       string `gorm:"type:varchar(2000)" json:"source_url"`   // Original search result URL
	SourceTitle     string `gorm:"type:varchar(1000)" json:"source_title"` // Original search result title
	Relevance       int    `gorm:"default:0" json:"relevance"`             // 0-100 relevance score
	// Status tracking
	Status                string     `gorm:"type:varchar(50);not null;default:'new'" json:"status"` // new, contacted, converted, dismissed
	ConvertedToCustomerID *uuid.UUID `gorm:"type:uuid" json:"converted_to_customer_id,omitempty"`
	Notes                 string     `gorm:"type:text" json:"notes"`
	ExtraFields           JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (LeadFinderResult) TableName() string {
	return "lead_finder_results"
}

func (r *LeadFinderResult) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.ExtraFields == nil {
		r.ExtraFields = make(JSONB)
	}
	return nil
}
