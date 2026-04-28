package models

import (
	"time"

	"github.com/google/uuid"
)

// Coupon 優惠券
type Coupon struct {
	ID                 uuid.UUID         `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID           uuid.UUID         `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant             Tenant            `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Code               string            `gorm:"type:varchar(100);not null;uniqueIndex:idx_tenant_coupon_code" json:"code"`
	Name               string            `gorm:"type:varchar(255);not null" json:"name"`
	Description        string            `gorm:"type:text" json:"description"`
	CouponType         string            `gorm:"type:varchar(50);not null" json:"coupon_type"`          // percentage, fixed_amount, free_shipping
	DiscountValue      float64           `gorm:"type:decimal(18,2);default:0.00" json:"discount_value"` // 折扣值（百分比或固定金額）
	MinPurchase        float64           `gorm:"type:decimal(18,2);default:0.00" json:"min_purchase"`   // 最低消費金額
	MaxDiscount        *float64          `gorm:"type:decimal(18,2)" json:"max_discount,omitempty"`      // 最大折扣金額（僅百分比時有效）
	ValidFrom          time.Time         `gorm:"type:timestamp with time zone;not null" json:"valid_from"`
	ValidTo            *time.Time        `gorm:"type:timestamp with time zone" json:"valid_to,omitempty"` // 可選，為空表示一直有效
	UsageLimit         *int              `gorm:"type:integer" json:"usage_limit,omitempty"`               // 使用次數限制
	UsedCount          int               `gorm:"default:0" json:"used_count"`                             // 已使用次數
	CustomerLimit      *int              `gorm:"type:integer" json:"customer_limit,omitempty"`            // 每個客戶使用次數限制
	MemberLevelID      *uuid.UUID        `gorm:"type:uuid" json:"member_level_id,omitempty"`              // 限制特定會員等級
	MemberLevel        *MemberLevel      `gorm:"foreignKey:MemberLevelID" json:"member_level,omitempty"`
	MinProductQuantity *int              `gorm:"type:integer" json:"min_product_quantity,omitempty"`                          // 最低購物車產品數量要求
	MinProductAmount   *float64          `gorm:"type:decimal(18,2)" json:"min_product_amount,omitempty"`                      // 最低購物車產品金額要求
	Conditions         []CouponCondition `gorm:"foreignKey:CouponID;constraint:OnDelete:CASCADE" json:"conditions,omitempty"` // 多條件匹配
	Status             string            `gorm:"type:varchar(20);default:'active'" json:"status"`                             // active, inactive, expired
	ExtraFields        JSONB             `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	TrashedAt          *time.Time        `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Coupon) TableName() string {
	return "coupons"
}

// PointSetting 積分設置
type PointSetting struct {
	ID                              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID                        uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_tenant_point_setting" json:"tenant_id"`
	Tenant                          Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	EarnPointsEnabled               bool      `gorm:"default:true" json:"earn_points_enabled"`                     // 消費獲得積分 開關
	PointsPerDollar                 float64   `gorm:"type:decimal(10,2);default:1.00" json:"points_per_dollar"`    // 每消費1元獲得多少積分
	DollarPerPoint                  float64   `gorm:"type:decimal(10,4);default:0.01" json:"dollar_per_point"`     // 每1積分等於多少現金
	MinPointsToUse                  int       `gorm:"default:0" json:"min_points_to_use"`                          // 最低使用積分
	MaxPointsPercent                *float64  `gorm:"type:decimal(5,2)" json:"max_points_percent,omitempty"`       // 積分最多可抵扣訂單金額的百分比
	ReferralBonusMode               string    `gorm:"type:varchar(20);default:'fixed'" json:"referral_bonus_mode"` // fixed, percent
	ReferralBonusValue              float64   `gorm:"type:decimal(10,2);default:0" json:"referral_bonus_value"`    // percent(0-100) 或 fixed points
	ReferralBonusPoints             int       `gorm:"default:0" json:"referral_bonus_points"`                      // 舊欄位：介绍人奖励积分（向後兼容）
	ReferralCountPolicy             string    `gorm:"type:varchar(20);default:'all'" json:"referral_count_policy"` // 介绍人积分计算策略: first_only 或 all
	EnableOrderEarnPoints           bool      `gorm:"default:true" json:"enable_order_earn_points"`                // 開啟訂單消費獲得積分
	EnableServiceOrderEarnPoints    bool      `gorm:"default:true" json:"enable_service_order_earn_points"`        // 開啟服務單消費獲得積分
	EnableOrderReferralBonus        bool      `gorm:"default:true" json:"enable_order_referral_bonus"`             // 開啟訂單介紹人獎勵積分
	EnableServiceOrderReferralBonus bool      `gorm:"default:true" json:"enable_service_order_referral_bonus"`     // 開啟服務單介紹人獎勵積分
	EnablePointsAdjustmentOnEdit    bool      `gorm:"default:true" json:"enable_points_adjustment_on_edit"`        // 編輯訂單/服務單時補發積分差值
	Status                          string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields                     JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt                       time.Time `json:"created_at"`
	UpdatedAt                       time.Time `json:"updated_at"`
}

func (PointSetting) TableName() string {
	return "point_settings"
}
