package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CountryRegion 國家地區模型
type CountryRegion struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CountryCode string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_country_region" json:"country_code"`
	RegionCode  string    `gorm:"type:varchar(50);not null;uniqueIndex:idx_country_region" json:"region_code"`
	// RegionName stores a JSON string like {"en":"Beijing","zh":"北京市"} (no DB migration needed)
	RegionName string    `gorm:"type:varchar(255);not null" json:"region_name"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BeforeCreate 創建前設置 UUID
func (c *CountryRegion) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (CountryRegion) TableName() string {
	return "country_regions"
}
