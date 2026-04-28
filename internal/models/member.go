package models

import (
	"time"

	"github.com/google/uuid"
)

// MemberLevel 會員等級
type MemberLevel struct {
	ID                uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID          uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant            Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name              string     `gorm:"type:varchar(255);not null" json:"name"`
	Code              *string    `gorm:"type:varchar(50)" json:"code,omitempty"`
	LevelOrder        int        `gorm:"default:0" json:"level_order"`
	MinPoints         int        `gorm:"default:0" json:"min_points"`
	MinPurchaseAmount float64    `gorm:"type:decimal(10,2);default:0.00" json:"min_purchase_amount"`
	DiscountRate      float64    `gorm:"type:decimal(5,2);default:0.00" json:"discount_rate"`
	IsDefault         bool       `gorm:"default:false" json:"is_default"` // 是否為系統預設會員等級
	AutoUpgrade       bool       `gorm:"default:false" json:"auto_upgrade"`
	Description       string     `gorm:"type:text" json:"description"`
	Benefits          JSONB      `gorm:"type:jsonb;default:'{}'" json:"benefits,omitempty"`
	Status            string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields       JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	TrashedAt         *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (MemberLevel) TableName() string {
	return "member_levels"
}

// Point 積分
type Point struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	CustomerID  uuid.UUID  `gorm:"type:uuid;not null" json:"customer_id"`
	Customer    Customer   `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Points      int        `gorm:"not null" json:"points"`
	PointsType  string     `gorm:"type:varchar(50);not null" json:"points_type"`
	SourceType  string     `gorm:"type:varchar(100)" json:"source_type"`
	SourceID    *uuid.UUID `gorm:"type:uuid" json:"source_id,omitempty"`
	Description string     `gorm:"type:text" json:"description"`
	ExpiresAt   *time.Time `gorm:"type:timestamp" json:"expires_at,omitempty"`
	Status      string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (Point) TableName() string {
	return "points"
}

// Franchise 加盟
type Franchise struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant         Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name           string     `gorm:"type:varchar(255);not null" json:"name"`
	Code           *string    `gorm:"type:varchar(50)" json:"code,omitempty"`
	ContactPerson  string     `gorm:"type:varchar(255)" json:"contact_person"`
	Phone          string     `gorm:"type:varchar(50)" json:"phone"`
	Email          string     `gorm:"type:varchar(255)" json:"email"`
	Address        string     `gorm:"type:text" json:"address"`
	RegionID       *uuid.UUID `gorm:"type:uuid" json:"region_id,omitempty"`
	Region         *Region    `gorm:"foreignKey:RegionID" json:"region,omitempty"`
	AgreementStart *time.Time `gorm:"type:date" json:"agreement_start,omitempty"`
	AgreementEnd   *time.Time `gorm:"type:date" json:"agreement_end,omitempty"`
	CommissionRate float64    `gorm:"type:decimal(5,2);default:0.00" json:"commission_rate"`
	Status         string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields    JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (Franchise) TableName() string {
	return "franchises"
}
