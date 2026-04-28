package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CustomerAddress 客戶地址模型
type CustomerAddress struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	CustomerID   uuid.UUID  `gorm:"type:uuid;not null;index" json:"customer_id"`
	CountryCode  string     `gorm:"type:varchar(10);not null" json:"country_code"`
	CountryName  string     `gorm:"type:varchar(255);not null" json:"country_name"`
	RegionCode   string     `gorm:"type:varchar(50)" json:"region_code"`
	RegionName   string     `gorm:"type:varchar(255)" json:"region_name"`
	PostalCode   string     `gorm:"type:varchar(50)" json:"postal_code"`
	AddressLine1 string     `gorm:"type:text;not null" json:"address_line1"`
	AddressLine2 string     `gorm:"type:text" json:"address_line2"`
	IsDefault    bool       `gorm:"default:false" json:"is_default"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	TrashedAt    *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

// BeforeCreate 創建前設置 UUID
func (c *CustomerAddress) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (CustomerAddress) TableName() string {
	return "customer_addresses"
}

// FormatAddress 格式化地址（根據語言）
func (c *CustomerAddress) FormatAddress(lang string) string {
	if lang == "en" {
		// 英文格式：地址 -> 地區 -> 國家（用逗號分隔）
		parts := []string{c.AddressLine1}
		if c.AddressLine2 != "" {
			parts = append(parts, c.AddressLine2)
		}
		if c.RegionName != "" {
			parts = append(parts, c.RegionName)
		}
		if c.PostalCode != "" {
			parts = append(parts, c.PostalCode)
		}
		parts = append(parts, c.CountryName)
		return joinStrings(parts, ", ")
	} else {
		// 中文格式：國家 -> 地區 -> 地址
		parts := []string{c.CountryName}
		if c.RegionName != "" {
			parts = append(parts, c.RegionName)
		}
		if c.PostalCode != "" {
			parts = append(parts, c.PostalCode)
		}
		parts = append(parts, c.AddressLine1)
		if c.AddressLine2 != "" {
			parts = append(parts, c.AddressLine2)
		}
		return joinStrings(parts, "")
	}
}

func joinStrings(parts []string, separator string) string {
	result := ""
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i > 0 {
			result += separator
		}
		result += part
	}
	return result
}
