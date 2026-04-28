package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AdPosition 廣告位置模型
type AdPosition struct {
	ID            uuid.UUID      `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID      uuid.UUID      `json:"tenant_id" gorm:"type:uuid;not null;index"`
	Name          string         `json:"name" gorm:"type:varchar(255);not null"`
	Code          string         `json:"code" gorm:"type:varchar(100);not null"`
	Description   string         `json:"description" gorm:"type:text"`
	Width         int            `json:"width" gorm:"default:1920"`
	Height        int            `json:"height" gorm:"default:1080"`
	SlideInterval int            `json:"slide_interval" gorm:"default:5"` // 輪播間隔（秒）
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`

	// 關聯
	Ads []Ad `json:"ads,omitempty" gorm:"foreignKey:AdPositionID"`
}

// TableName 指定表名
func (AdPosition) TableName() string {
	return "ad_positions"
}

// BeforeCreate 創建前設置 UUID
func (a *AdPosition) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.Width == 0 {
		a.Width = 1920
	}
	if a.Height == 0 {
		a.Height = 1080
	}
	if a.SlideInterval == 0 {
		a.SlideInterval = 5
	}
	return nil
}

// Ad 廣告模型
type Ad struct {
	ID           uuid.UUID      `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID     uuid.UUID      `json:"tenant_id" gorm:"type:uuid;not null;index"`
	AdPositionID uuid.UUID      `json:"ad_position_id" gorm:"type:uuid;not null;index"`
	Name         string         `json:"name" gorm:"type:varchar(255);not null"`
	MediaType    string         `json:"media_type" gorm:"type:varchar(50);not null;default:'image'"` // image, video
	MediaURL     string         `json:"media_url" gorm:"type:text;not null"`
	MediaPath    string         `json:"media_path" gorm:"type:text"`
	Duration     int            `json:"duration" gorm:"default:5"` // 顯示時長（秒）
	SortOrder    int            `json:"sort_order" gorm:"default:0"`
	IsActive     bool           `json:"is_active" gorm:"default:true"`
	StartDate    *time.Time     `json:"start_date"`
	EndDate      *time.Time     `json:"end_date"`
	ExtraFields  JSONB          `json:"extra_fields" gorm:"type:jsonb;default:'{}'::jsonb"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`

	// 關聯
	AdPosition *AdPosition `json:"ad_position,omitempty" gorm:"foreignKey:AdPositionID"`
}

// TableName 指定表名
func (Ad) TableName() string {
	return "ads"
}

// BeforeCreate 創建前設置 UUID
func (a *Ad) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.MediaType == "" {
		a.MediaType = "image"
	}
	if a.Duration == 0 {
		a.Duration = 5
	}
	if a.ExtraFields == nil {
		a.ExtraFields = make(JSONB)
	}
	return nil
}

// CarouselSettings 輪播設定模型
type CarouselSettings struct {
	ID                 uuid.UUID  `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID           uuid.UUID  `json:"tenant_id" gorm:"type:uuid;not null;index"`
	AdPositionID       uuid.UUID  `json:"ad_position_id" gorm:"type:uuid;not null;index"`
	SlideInterval      int        `json:"slide_interval" gorm:"default:5"`      // 圖片輪播秒數
	TransitionDuration int        `json:"transition_duration" gorm:"default:500"` // 轉場時間（毫秒）
	AutoUpdate         bool       `json:"auto_update" gorm:"default:true"`
	UpdateInterval     int        `json:"update_interval" gorm:"default:3600"` // 更新檢查間隔（秒）
	Version            int        `json:"version" gorm:"default:1"`
	LastGeneratedAt    *time.Time `json:"last_generated_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	// 關聯
	AdPosition *AdPosition `json:"ad_position,omitempty" gorm:"foreignKey:AdPositionID"`
}

// TableName 指定表名
func (CarouselSettings) TableName() string {
	return "carousel_settings"
}

// BeforeCreate 創建前設置 UUID
func (c *CarouselSettings) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.SlideInterval == 0 {
		c.SlideInterval = 5
	}
	if c.TransitionDuration == 0 {
		c.TransitionDuration = 500
	}
	if c.UpdateInterval == 0 {
		c.UpdateInterval = 3600
	}
	if c.Version == 0 {
		c.Version = 1
	}
	return nil
}
