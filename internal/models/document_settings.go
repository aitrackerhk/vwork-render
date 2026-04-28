package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DocumentSettings 文件設定（發票/發貨單/合約 的條款/備註/樣式）
type DocumentSettings struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	DocumentType string    `gorm:"type:varchar(50);not null" json:"document_type"` // invoice, shipping_note, contract, style (全域文件樣式)
	Terms        string    `gorm:"type:text" json:"terms"`                         // 條款
	Notes        string    `gorm:"type:text" json:"notes"`                         // 備註
	// 文件樣式欄位（document_type = "style" 時使用，全域共用）
	LogoURL       *string   `gorm:"column:logo_url;type:varchar(500)" json:"logo_url,omitempty"`               // 文件專屬 Logo，未設定時 fallback 企業 Logo
	LogoWidth     float64   `gorm:"column:logo_width;type:decimal(8,2);default:0" json:"logo_width"`           // Logo 寬度 pt（0 = 自動）
	LogoHeight    float64   `gorm:"column:logo_height;type:decimal(8,2);default:0" json:"logo_height"`         // Logo 高度 pt（0 = 自動）
	TitleFontSize float64   `gorm:"column:title_font_size;type:decimal(5,2);default:0" json:"title_font_size"` // 標題字型大小 pt（0 = 預設 22）
	BodyFontSize  float64   `gorm:"column:body_font_size;type:decimal(5,2);default:0" json:"body_font_size"`   // 內文字型大小 pt（0 = 預設 11）
	NotesFontSize float64   `gorm:"column:notes_font_size;type:decimal(5,2);default:0" json:"notes_font_size"` // 條款/備註字型大小 pt（0 = 預設 10）
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// DocumentStyleDefaults 文件樣式預設值
const (
	DefaultTitleFontSize    = 22.0
	DefaultSubtitleFontSize = 16.0
	DefaultSectionFontSize  = 13.0
	DefaultBodyFontSize     = 11.0
	DefaultNotesFontSize    = 10.0
)

// GetTitleFontSize 取得標題字型大小（含 fallback）
func (ds *DocumentSettings) GetTitleFontSize() float64 {
	if ds != nil && ds.TitleFontSize > 0 {
		return ds.TitleFontSize
	}
	return DefaultTitleFontSize
}

// GetBodyFontSize 取得內文字型大小（含 fallback）
func (ds *DocumentSettings) GetBodyFontSize() float64 {
	if ds != nil && ds.BodyFontSize > 0 {
		return ds.BodyFontSize
	}
	return DefaultBodyFontSize
}

// GetNotesFontSize 取得備註字型大小（含 fallback）
func (ds *DocumentSettings) GetNotesFontSize() float64 {
	if ds != nil && ds.NotesFontSize > 0 {
		return ds.NotesFontSize
	}
	return DefaultNotesFontSize
}

// BeforeCreate 創建前設置 UUID
func (ds *DocumentSettings) BeforeCreate(tx *gorm.DB) error {
	if ds.ID == uuid.Nil {
		ds.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (DocumentSettings) TableName() string {
	return "document_settings"
}
