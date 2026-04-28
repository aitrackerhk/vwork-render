package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Page 公司網站頁面
type Page struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant         Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name           string     `gorm:"type:varchar(255);not null" json:"name"`
	Slug           string     `gorm:"type:varchar(255);not null" json:"slug"`
	Title          *string    `gorm:"type:varchar(255)" json:"title,omitempty"`
	Description    *string    `gorm:"type:text" json:"description,omitempty"`
	TopnavStyle    string     `gorm:"type:varchar(50);default:'default'" json:"topnav_style"` // default, light, dark, transparent
	Status         string     `gorm:"type:varchar(50);default:'draft'" json:"status"`         // draft, published
	IsHomepage     bool       `gorm:"default:false" json:"is_homepage"`
	SEOTitle       *string    `gorm:"type:varchar(255)" json:"seo_title,omitempty"`
	SEODescription *string    `gorm:"type:text" json:"seo_description,omitempty"`
	SEOKeywords    *string    `gorm:"type:varchar(500)" json:"seo_keywords,omitempty"`
	CreatedBy      *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy      *uuid.UUID `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	TrashedAt      *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields    JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`

	// Relations
	Components []PageComponent `gorm:"foreignKey:PageID;constraint:OnDelete:CASCADE" json:"components,omitempty"`
}

func (Page) TableName() string {
	return "pages"
}

func (p *Page) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.ExtraFields == nil {
		p.ExtraFields = make(JSONB)
	}
	return nil
}

// PageComponent 頁面元件
type PageComponent struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant        Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	PageID        uuid.UUID  `gorm:"type:uuid;not null" json:"page_id"`
	Page          Page       `gorm:"foreignKey:PageID" json:"page,omitempty"`
	BlockID       *uuid.UUID `gorm:"type:uuid" json:"block_id,omitempty"` // Reference to a linked block; when set, public rendering uses block's latest data
	Block         *Block     `gorm:"foreignKey:BlockID" json:"block,omitempty"`
	ComponentType string     `gorm:"type:varchar(100);not null" json:"component_type"`       // hero, text, image, button, section, etc.
	ComponentData JSONB      `gorm:"type:jsonb;not null;default:'{}'" json:"component_data"` // 元件配置數據
	SortOrder     int        `gorm:"default:0" json:"sort_order"`
	IsActive      bool       `gorm:"default:true" json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ExtraFields   JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
}

func (PageComponent) TableName() string {
	return "page_components"
}

func (pc *PageComponent) BeforeCreate(tx *gorm.DB) error {
	if pc.ID == uuid.Nil {
		pc.ID = uuid.New()
	}
	if pc.ComponentData == nil {
		pc.ComponentData = make(JSONB)
	}
	if pc.ExtraFields == nil {
		pc.ExtraFields = make(JSONB)
	}
	return nil
}
