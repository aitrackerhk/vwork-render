package models

import (
	"nwork/internal/database"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SystemSetting struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Key         string    `gorm:"type:varchar(255);uniqueIndex;not null" json:"key"`
	Value       string    `gorm:"type:text;not null" json:"value"`
	Description *string   `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (SystemSetting) TableName() string {
	return "system_settings"
}

// GetSystemSetting 獲取系統設定
func GetSystemSetting(key string, defaultValue string) string {
	var setting SystemSetting
	if err := database.DB.Where("key = ?", key).First(&setting).Error; err != nil {
		return defaultValue
	}
	return setting.Value
}

// SetSystemSetting 設置系統設定
func SetSystemSetting(key string, value string, description *string) error {
	var setting SystemSetting
	err := database.DB.Where("key = ?", key).First(&setting).Error
	
	if err == gorm.ErrRecordNotFound {
		// 創建新設定
		setting = SystemSetting{
			Key:         key,
			Value:       value,
			Description: description,
		}
		return database.DB.Create(&setting).Error
	} else if err != nil {
		return err
	}
	
	// 更新現有設定
	setting.Value = value
	if description != nil {
		setting.Description = description
	}
	return database.DB.Save(&setting).Error
}

