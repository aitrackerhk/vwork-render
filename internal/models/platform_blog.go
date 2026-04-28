package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PlatformBlog is a platform-level blog post managed via vworkadmin.
// Independent of tenants — used for the vWork official website blog.
type PlatformBlog struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Title          string     `gorm:"type:varchar(500);not null" json:"title"`
	Slug           string     `gorm:"type:varchar(500);not null;uniqueIndex:idx_platform_blogs_lang_slug" json:"slug"`
	Content        *string    `gorm:"type:text" json:"content,omitempty"`
	Excerpt        *string    `gorm:"type:text" json:"excerpt,omitempty"`
	FeaturedImage  *string    `gorm:"type:varchar(1000)" json:"featured_image,omitempty"`
	Author         string     `gorm:"type:varchar(255)" json:"author"`
	Status         string     `gorm:"type:varchar(50);default:'draft'" json:"status"` // draft, published, archived
	PublishedAt    *time.Time `gorm:"type:timestamp with time zone" json:"published_at,omitempty"`
	Category       *string    `gorm:"type:varchar(200)" json:"category,omitempty"`
	Tags           JSONB      `gorm:"type:jsonb;default:'[]'" json:"tags,omitempty"`
	ViewCount      int        `gorm:"default:0" json:"view_count"`
	SEOTitle       *string    `gorm:"type:varchar(500)" json:"seo_title,omitempty"`
	SEODescription *string    `gorm:"type:text" json:"seo_description,omitempty"`
	SEOKeywords    *string    `gorm:"type:varchar(500)" json:"seo_keywords,omitempty"`
	Lang           string     `gorm:"type:varchar(10);default:'zh';uniqueIndex:idx_platform_blogs_lang_slug" json:"lang"`
	SortOrder      int        `gorm:"default:0" json:"sort_order"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (PlatformBlog) TableName() string {
	return "platform_blogs"
}

func (b *PlatformBlog) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}
