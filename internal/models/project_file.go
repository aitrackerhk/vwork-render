package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProjectFile struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index" json:"project_id"`

	FileURL  string `gorm:"type:varchar(500);not null" json:"file_url"`
	FileName string `gorm:"type:varchar(255);not null" json:"file_name"`
	MimeType string `gorm:"type:varchar(120)" json:"mime_type"`
	FileSize int64  `gorm:"type:bigint;default:0" json:"file_size"`

	Description string `gorm:"type:text" json:"description"`

	UploadedBy *uuid.UUID `gorm:"type:uuid" json:"uploaded_by,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ExtraFields JSONB `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
}

func (ProjectFile) TableName() string { return "project_files" }

func (f *ProjectFile) BeforeCreate(tx *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	if f.ExtraFields == nil {
		f.ExtraFields = make(JSONB)
	}
	return nil
}


