package models

import (
	"time"
)

// OformTemplate represents a document template for vOffice desktop app
// Served via /dashboard/api/oforms API (compatible with OnlyOffice oforms format)
type OformTemplate struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Locale       string    `gorm:"type:varchar(10);not null;default:'en'" json:"locale"`
	NameForm     string    `gorm:"type:varchar(500);not null;column:name_form" json:"name_form"`
	TemplateDesc string    `gorm:"type:text;default:'';column:template_desc" json:"template_desc"`
	FileExt      string    `gorm:"type:varchar(20);not null;default:'pdf';column:file_ext" json:"file_ext"`
	FileURL      string    `gorm:"type:varchar(1000);not null;column:file_url" json:"file_url"`
	FileSize     int64     `gorm:"default:0;column:file_size" json:"file_size"`
	ThumbnailURL string    `gorm:"type:varchar(1000);default:'';column:thumbnail_url" json:"thumbnail_url"`
	PreviewURL   string    `gorm:"type:varchar(1000);default:'';column:preview_url" json:"preview_url"`
	Category     string    `gorm:"type:varchar(200);default:''" json:"category"`
	SortOrder    int       `gorm:"default:0;column:sort_order" json:"sort_order"`
	IsActive     bool      `gorm:"default:true;column:is_active" json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName specifies the table name
func (OformTemplate) TableName() string {
	return "oform_templates"
}
