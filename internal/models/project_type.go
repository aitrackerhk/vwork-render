package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectType 項目類型（多租戶）
type ProjectType struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`

	Name  string `gorm:"type:varchar(255);not null" json:"name"`
	Color string `gorm:"type:varchar(50);default:'#6366f1'" json:"color"`
	Status string `gorm:"type:varchar(50);default:'active';index" json:"status"`

	CreatedBy *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy *uuid.UUID `gorm:"type:uuid" json:"updated_by,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ExtraFields JSONB `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
}

func (ProjectType) TableName() string {
	return "project_types"
}

func (t *ProjectType) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.ExtraFields == nil {
		t.ExtraFields = make(JSONB)
	}
	return nil
}


