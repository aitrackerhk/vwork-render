package models

import (
	"time"

	"github.com/google/uuid"
)

// CouponCondition 優惠券條件（多條件匹配）
type CouponCondition struct {
	ID            uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	CouponID      uuid.UUID    `gorm:"type:uuid;not null;index" json:"coupon_id"`
	Coupon        Coupon       `gorm:"foreignKey:CouponID" json:"coupon,omitempty"`
	ConditionType string       `gorm:"type:varchar(50);not null" json:"condition_type"` // product_quantity, product_amount, member_level, customer
	ProductID     *uuid.UUID   `gorm:"type:uuid" json:"product_id,omitempty"`           // 特定產品ID（當 condition_type 為 product_quantity 或 product_amount 時）
	Product       *Product     `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Quantity      *int         `gorm:"type:integer" json:"quantity,omitempty"`     // 產品數量要求
	Amount        *float64     `gorm:"type:decimal(18,2)" json:"amount,omitempty"` // 產品金額要求
	MemberLevelID *uuid.UUID   `gorm:"type:uuid" json:"member_level_id,omitempty"` // 會員等級ID（當 condition_type 為 member_level 時）
	MemberLevel   *MemberLevel `gorm:"foreignKey:MemberLevelID" json:"member_level,omitempty"`
	CustomerID    *uuid.UUID   `gorm:"type:uuid" json:"customer_id,omitempty"` // 客戶ID（當 condition_type 為 customer 時）
	Customer      *Customer    `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	MatchType     string       `gorm:"type:varchar(20);default:'and'" json:"match_type"`          // and, or - 與其他條件的匹配方式
	TrashedAt     *time.Time   `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (CouponCondition) TableName() string {
	return "coupon_conditions"
}
