package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Blog 博客文章
type Blog struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant         Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Title          string     `gorm:"type:varchar(255);not null" json:"title"`
	Slug           string     `gorm:"type:varchar(255);not null" json:"slug"`
	Content        *string    `gorm:"type:text" json:"content,omitempty"` // HTML 內容
	Excerpt        *string    `gorm:"type:text" json:"excerpt,omitempty"` // 摘要
	FeaturedImage  *string    `gorm:"type:varchar(500)" json:"featured_image,omitempty"`
	AuthorID       *uuid.UUID `gorm:"type:uuid" json:"author_id,omitempty"`
	Author         User       `gorm:"foreignKey:AuthorID" json:"author,omitempty"`
	Status         string     `gorm:"type:varchar(50);default:'draft'" json:"status"` // draft, published, archived
	PublishedAt    *time.Time `gorm:"type:timestamp with time zone" json:"published_at,omitempty"`
	Category       *string    `gorm:"type:varchar(100)" json:"category,omitempty"`
	Tags           JSONB      `gorm:"type:jsonb;default:'[]'" json:"tags,omitempty"`
	ViewCount      int        `gorm:"default:0" json:"view_count"`
	SEOTitle       *string    `gorm:"type:varchar(255)" json:"seo_title,omitempty"`
	SEODescription *string    `gorm:"type:text" json:"seo_description,omitempty"`
	SEOKeywords    *string    `gorm:"type:varchar(500)" json:"seo_keywords,omitempty"`
	CreatedBy      *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy      *uuid.UUID `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	TrashedAt      *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields    JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
}

func (Blog) TableName() string {
	return "blogs"
}

func (b *Blog) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.Tags == nil {
		b.Tags = make(JSONB)
	}
	if b.ExtraFields == nil {
		b.ExtraFields = make(JSONB)
	}
	return nil
}
