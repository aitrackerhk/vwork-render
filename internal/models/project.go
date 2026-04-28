package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Project 項目（多租戶）
type Project struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Code        string    `gorm:"type:varchar(100)" json:"code"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	CoverURL    string    `gorm:"type:varchar(500)" json:"cover_url"`

	Status string `gorm:"type:varchar(50);default:'active';index" json:"status"` // active, on_hold, completed, cancelled

	StartDate *time.Time `gorm:"type:date" json:"start_date,omitempty"`
	EndDate   *time.Time `gorm:"type:date" json:"end_date,omitempty"`

	Budget float64 `gorm:"type:decimal(15,2);default:0" json:"budget"`

	ProjectTypeID *uuid.UUID   `gorm:"type:uuid" json:"project_type_id,omitempty"`
	ProjectType   *ProjectType `gorm:"foreignKey:ProjectTypeID" json:"project_type,omitempty"`

	OwnerUserID *uuid.UUID `gorm:"type:uuid" json:"owner_user_id,omitempty"`
	Owner       *User      `gorm:"foreignKey:OwnerUserID" json:"owner,omitempty"`

	Tasks []ProjectTask `gorm:"foreignKey:ProjectID" json:"tasks,omitempty"`

	CreatedBy *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy *uuid.UUID `gorm:"type:uuid" json:"updated_by,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間

	ExtraFields JSONB `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
}

func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.ExtraFields == nil {
		p.ExtraFields = make(JSONB)
	}
	return nil
}
