package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Store 店舖
type Store struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant           Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name             string     `gorm:"type:varchar(255);not null" json:"name"`
	Code             string     `gorm:"type:varchar(100);not null" json:"code"`
	Address          string     `gorm:"type:text" json:"address"`
	ImageURL         string     `gorm:"type:varchar(500);column:image_url" json:"image_url,omitempty"`
	ContactPerson    string     `gorm:"type:varchar(255)" json:"contact_person"`
	PhoneCountryCode string     `gorm:"type:varchar(10)" json:"phone_country_code"`
	Phone            string     `gorm:"type:varchar(50)" json:"phone"`
	Email            string     `gorm:"type:varchar(255)" json:"email"`
	Status           string     `gorm:"type:varchar(50);default:'active'" json:"status"`
	ExtraFields      JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CreatedBy        *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy        *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	TrashedAt        *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

// BeforeCreate 創建前設置 UUID
func (s *Store) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Store) TableName() string {
	return "stores"
}
