package models

import "time"

// AdminSetting stores platform-level key-value settings managed by vworkadmin.
// Table: admin_settings
type AdminSetting struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Key         string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"key"`
	Value       string    `gorm:"type:text;not null;default:''" json:"value"`
	Description string    `gorm:"type:varchar(255);default:''" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AdminSetting) TableName() string { return "admin_settings" }
