package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Block 區塊模型（可重用的頁面元件區塊）
type Block struct {
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID      uuid.UUID      `json:"tenant_id" gorm:"type:uuid;not null;index"`
	Name          string         `json:"name" gorm:"type:varchar(255);not null"`
	ComponentType string         `json:"component_type" gorm:"type:varchar(100);not null"`
	ComponentData JSONB          `json:"component_data" gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	CreatedBy     *uuid.UUID     `json:"created_by" gorm:"type:uuid"`
	UpdatedBy     *uuid.UUID     `json:"updated_by" gorm:"type:uuid"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	ExtraFields   JSONB          `json:"extra_fields" gorm:"type:jsonb;default:'{}'::jsonb"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Block) TableName() string {
	return "blocks"
}

// BeforeCreate 創建前設置 UUID
func (b *Block) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.ComponentData == nil {
		b.ComponentData = make(JSONB)
	}
	if b.ExtraFields == nil {
		b.ExtraFields = make(JSONB)
	}
	return nil
}
