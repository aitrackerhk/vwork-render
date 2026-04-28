package models

import (
	"time"

	"github.com/google/uuid"
)

// Region 地區
type Region struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Name        string     `gorm:"type:varchar(255);not null" json:"name"`
	Code        *string    `gorm:"type:varchar(50);unique" json:"code,omitempty"`
	ParentID    *uuid.UUID `gorm:"type:uuid" json:"parent_id,omitempty"`
	Parent      *Region    `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Status      string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (Region) TableName() string {
	return "regions"
}

