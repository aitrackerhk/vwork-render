package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DiningArea 餐桌區域
// 例：A 區 1-2 人、B 區 3-4 人
// code 建議使用 A/B
// name 可用 A區/B區
// min_seats/max_seats 用於容量描述
// store_id 可選（多店舖）
type DiningArea struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	StoreID   *uuid.UUID `gorm:"type:uuid" json:"store_id"`
	Store     *Store     `gorm:"foreignKey:StoreID" json:"store,omitempty"`
	Code      string     `gorm:"type:varchar(50);not null" json:"code"`
	Name      string     `gorm:"type:varchar(100);not null" json:"name"`
	MinSeats  int        `gorm:"default:1" json:"min_seats"`
	MaxSeats  int        `gorm:"default:1" json:"max_seats"`
	SortOrder int        `gorm:"default:0" json:"sort_order"`
	IsActive  bool       `gorm:"default:true" json:"is_active"`
	Notes     string     `gorm:"type:text" json:"notes"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	CreatedBy *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
}

func (d *DiningArea) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

func (DiningArea) TableName() string {
	return "dining_areas"
}

// DiningTable 餐桌
// code 例：A-1
// seats 可手動設定
// status: available/occupied/cleaning/reserved
// store_id 可選（多店舖）
type DiningTable struct {
	ID        uuid.UUID   `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID   `gorm:"type:uuid;not null;index" json:"tenant_id"`
	StoreID   *uuid.UUID  `gorm:"type:uuid" json:"store_id"`
	Store     *Store      `gorm:"foreignKey:StoreID" json:"store,omitempty"`
	AreaID    *uuid.UUID  `gorm:"type:uuid" json:"area_id"`
	Area      *DiningArea `gorm:"foreignKey:AreaID" json:"area,omitempty"`
	Code      string      `gorm:"type:varchar(50);not null" json:"code"`
	Name      string      `gorm:"type:varchar(100)" json:"name"`
	Seats     int         `gorm:"default:1" json:"seats"`
	Status    string      `gorm:"type:varchar(20);default:'available'" json:"status"`
	IsActive  bool        `gorm:"default:true" json:"is_active"`
	Notes     string      `gorm:"type:text" json:"notes"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	CreatedBy *uuid.UUID  `gorm:"type:uuid" json:"created_by"`
	UpdatedBy *uuid.UUID  `gorm:"type:uuid" json:"updated_by"`
}

func (d *DiningTable) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

func (DiningTable) TableName() string {
	return "dining_tables"
}

// DiningQueue 排隊候位
// status: waiting/seated/cancelled
// table_id 可選（入座後）
type DiningQueue struct {
	ID           uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	StoreID      *uuid.UUID   `gorm:"type:uuid" json:"store_id"`
	Store        *Store       `gorm:"foreignKey:StoreID" json:"store,omitempty"`
	AreaID       *uuid.UUID   `gorm:"type:uuid" json:"area_id"`
	Area         *DiningArea  `gorm:"foreignKey:AreaID" json:"area,omitempty"`
	Name         string       `gorm:"type:varchar(100);not null" json:"name"`
	Phone        string       `gorm:"type:varchar(50)" json:"phone"`
	PartySize    int          `gorm:"default:1" json:"party_size"`
	ReservationAt *time.Time  `gorm:"type:timestamp" json:"reservation_at"`
	Status       string       `gorm:"type:varchar(20);default:'waiting'" json:"status"`
	TableID      *uuid.UUID   `gorm:"type:uuid" json:"table_id"`
	Table        *DiningTable `gorm:"foreignKey:TableID" json:"table,omitempty"`
	TableCode    string       `gorm:"type:varchar(50)" json:"table_code"`
	TicketNumber string       `gorm:"type:varchar(50);index" json:"ticket_number"`
	TicketSeq    int          `gorm:"index" json:"ticket_seq"`
	Notes        string       `gorm:"type:text" json:"notes"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	SeatedAt     *time.Time   `json:"seated_at"`
	CancelledAt  *time.Time   `json:"cancelled_at"`
	CreatedBy    *uuid.UUID   `gorm:"type:uuid" json:"created_by"`
	UpdatedBy    *uuid.UUID   `gorm:"type:uuid" json:"updated_by"`
}

func (d *DiningQueue) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

func (DiningQueue) TableName() string {
	return "dining_queues"
}
