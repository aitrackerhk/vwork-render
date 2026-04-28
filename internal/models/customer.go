package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Customer 客戶模型
type Customer struct {
	ID               uuid.UUID         `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID         `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Code             string            `gorm:"type:varchar(100)" json:"code"`
	Name             string            `gorm:"type:varchar(255);not null" json:"name"`
	LastName         string            `gorm:"type:varchar(100)" json:"last_name"` // 姓氏（可選）
	Email            string            `gorm:"type:varchar(255)" json:"email"`
	Phone            string            `gorm:"type:varchar(50)" json:"phone"`
	PhoneCountryCode string            `gorm:"type:varchar(10)" json:"phone_country_code"`
	BirthDate        *time.Time        `gorm:"type:date" json:"birth_date,omitempty"`
	Gender           string            `gorm:"type:varchar(20)" json:"gender,omitempty"` // 性別：male, female, unknown
	Address          string            `gorm:"type:text" json:"address"`
	Status           string            `gorm:"type:varchar(50);not null;default:'active'" json:"status"`
	MemberLevelID    *uuid.UUID        `gorm:"type:uuid" json:"member_level_id,omitempty"`
	MemberLevel      *MemberLevel      `gorm:"foreignKey:MemberLevelID" json:"member_level,omitempty"`
	TotalPoints      int               `gorm:"default:0" json:"total_points"`
	FranchiseID      *uuid.UUID        `gorm:"type:uuid" json:"franchise_id,omitempty"`
	Franchise        *Franchise        `gorm:"foreignKey:FranchiseID" json:"franchise,omitempty"`
	ReferralCode     string            `gorm:"type:varchar(50)" json:"referral_code,omitempty"`
	ProfilePic       string            `gorm:"type:varchar(500)" json:"profile_pic,omitempty"`            // 客戶頭像
	PasswordHash     string            `gorm:"type:varchar(255)" json:"-"`                                // 密碼哈希（不返回給前端）
	TrashedAt        *time.Time        `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	CreatedBy        *uuid.UUID        `gorm:"type:uuid" json:"created_by"`
	UpdatedBy        *uuid.UUID        `gorm:"type:uuid" json:"updated_by"`
	ExtraFields      JSONB             `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	Addresses        []CustomerAddress `gorm:"foreignKey:CustomerID" json:"addresses,omitempty"`
	Labels           []CustomerLabel   `gorm:"many2many:customer_label_relations;foreignKey:ID;joinForeignKey:CustomerID;References:ID;joinReferences:LabelID" json:"labels,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (c *Customer) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Customer) TableName() string {
	return "customers"
}
