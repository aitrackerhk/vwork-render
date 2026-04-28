package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ProjectResourceReservation struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index" json:"project_id"`

	ResourceType string    `gorm:"type:varchar(50);not null;index" json:"resource_type"` // room, equipment, vehicle
	ResourceID   uuid.UUID `gorm:"type:uuid;not null;index" json:"resource_id"`

	StartTime time.Time `gorm:"type:timestamp;not null" json:"start_time"`
	EndTime   time.Time `gorm:"type:timestamp;not null" json:"end_time"`

	Status string `gorm:"type:varchar(50);default:'active';index" json:"status"` // active, cancelled
	Notes  string `gorm:"type:text" json:"notes"`

	UserID *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"`

	CreatedBy *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy *uuid.UUID `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	ExtraFields JSONB `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
}

func (ProjectResourceReservation) TableName() string { return "project_resource_reservations" }

func (r *ProjectResourceReservation) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.ExtraFields == nil {
		r.ExtraFields = make(JSONB)
	}
	return nil
}


