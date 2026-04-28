package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ServiceType 服務種類
type ServiceType struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant          Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name            string    `gorm:"type:varchar(255);not null" json:"name"`
	Code            *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	Description     string    `gorm:"type:text" json:"description"`
	DurationMinutes *int      `gorm:"type:integer" json:"duration_minutes,omitempty"`
	Status          string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields     JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (ServiceType) TableName() string {
	return "service_types"
}

// Service 服務
type Service struct {
	ID              uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID        uuid.UUID    `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant          Tenant       `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	ServiceTypeID   *uuid.UUID   `gorm:"type:uuid" json:"service_type_id,omitempty"`
	ServiceType     *ServiceType `gorm:"foreignKey:ServiceTypeID" json:"service_type,omitempty"`
	Name            string       `gorm:"type:varchar(255);not null" json:"name"`
	Code            *string      `gorm:"type:varchar(50)" json:"code,omitempty"`
	Description     string       `gorm:"type:text" json:"description"`
	Price           float64      `gorm:"type:decimal(18,2);default:0.00" json:"price"`
	DurationMinutes *int         `gorm:"type:integer" json:"duration_minutes,omitempty"`
	Status          string       `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields     JSONB        `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	TrashedAt       *time.Time   `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間

	// 關聯：服務稅
	ServiceTaxes []ServiceTax `gorm:"many2many:service_tax_relations;foreignKey:ID;joinForeignKey:ServiceID;References:ID;joinReferences:TaxID" json:"service_taxes,omitempty"`
}

func (Service) TableName() string {
	return "services"
}

// Appointment 預約
type Appointment struct {
	ID         uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID   uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant     Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	CustomerID uuid.UUID  `gorm:"type:uuid;not null" json:"customer_id"`
	Customer   Customer   `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	ServiceID  *uuid.UUID `gorm:"type:uuid" json:"service_id,omitempty"`
	Service    *Service   `gorm:"foreignKey:ServiceID" json:"service,omitempty"`
	StaffID    *uuid.UUID `gorm:"type:uuid" json:"staff_id,omitempty"`
	// staff_id 現在對應 service_staff（見 migrations/070_fix_appointments_staff_id_fkey.sql）
	Staff           *ServiceStaff `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
	StartTime       time.Time     `gorm:"type:timestamp;not null" json:"start_time"`     // 開始時間（日期+時間）
	EndTime         time.Time     `gorm:"type:timestamp;not null" json:"end_time"`       // 結束時間（日期+時間）
	ReminderTime    *time.Time    `gorm:"type:timestamp" json:"reminder_time,omitempty"` // 提醒時間（日期+時間）
	AppointmentDate time.Time     `gorm:"type:date;not null" json:"appointment_date"`    // 保留用於向後兼容
	AppointmentTime SQLTime       `gorm:"type:time;not null" json:"appointment_time"`    // 保留用於向後兼容（PostgreSQL TIME）
	DurationMinutes *int          `gorm:"type:integer" json:"duration_minutes,omitempty"`
	ServiceOrderID  *uuid.UUID    `gorm:"type:uuid" json:"service_order_id,omitempty"` // 關聯的服務單ID
	ServiceOrder    *ServiceOrder `gorm:"foreignKey:ServiceOrderID" json:"service_order,omitempty"`
	RentalOrderID   *uuid.UUID    `gorm:"type:uuid" json:"rental_order_id,omitempty"` // 關聯的出租單ID
	RentalOrder     *RentalOrder  `gorm:"foreignKey:RentalOrderID" json:"rental_order,omitempty"`
	Rooms           []Room        `gorm:"many2many:appointment_rooms;" json:"rooms,omitempty"`
	Equipments      []Equipment   `gorm:"many2many:appointment_equipments;" json:"equipments,omitempty"`
	Vehicles        []Vehicle     `gorm:"many2many:appointment_vehicles;" json:"vehicles,omitempty"`
	Notes           string        `gorm:"type:text" json:"notes"`
	Status          string        `gorm:"type:varchar(50);default:'pending'" json:"status"`
	ExtraFields     JSONB         `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
	TrashedAt       *time.Time    `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Appointment) TableName() string {
	return "appointments"
}

// ServiceOrder 服務單（類似訂單結構）
type ServiceOrder struct {
	ID               uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID  `gorm:"type:uuid;not null;index;uniqueIndex:idx_service_orders_tenant_order_number" json:"tenant_id"`
	Tenant           Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	OrderNumber      string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_service_orders_tenant_order_number" json:"order_number"`
	CustomerID       *uuid.UUID `gorm:"type:uuid" json:"customer_id"`
	Customer         Customer   `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	ServiceDate      time.Time  `gorm:"type:date;not null" json:"service_date"`
	Status           string     `gorm:"type:varchar(50);not null;default:'draft'" json:"status"`
	TotalAmount      float64    `gorm:"type:decimal(15,2);default:0" json:"total_amount"`
	CouponID         *uuid.UUID `gorm:"type:uuid" json:"coupon_id,omitempty"`
	Coupon           *Coupon    `gorm:"foreignKey:CouponID" json:"coupon,omitempty"`
	PointsUsed       int        `gorm:"default:0" json:"points_used"`
	PointsEarned     int        `gorm:"default:0" json:"points_earned"`
	PointsDiscount   float64    `gorm:"type:decimal(18,2);default:0.00" json:"points_discount"`
	CouponDiscount   float64    `gorm:"type:decimal(18,2);default:0.00" json:"coupon_discount"`
	ReferralCode     string     `gorm:"type:varchar(50)" json:"referral_code,omitempty"`
	ContactName      string     `gorm:"type:varchar(255)" json:"contact_name,omitempty"`
	ContactEmail     string     `gorm:"type:varchar(255)" json:"contact_email,omitempty"`
	ContactPhone     string     `gorm:"type:varchar(50)" json:"contact_phone,omitempty"`
	ContactAddress   string     `gorm:"type:text" json:"contact_address,omitempty"`
	SalespersonID    *uuid.UUID `gorm:"type:uuid" json:"salesperson_id,omitempty"` // 銷售員（員工）ID
	Salesperson      *User      `gorm:"foreignKey:SalespersonID" json:"salesperson,omitempty"`
	StoreID          *uuid.UUID `gorm:"type:uuid" json:"store_id,omitempty"` // 所屬店舖ID
	Store            *Store     `gorm:"foreignKey:StoreID" json:"store,omitempty"`
	OrderID          *uuid.UUID `gorm:"type:uuid" json:"order_id,omitempty"` // 關聯的訂單ID（服務套票轉化）
	Order            *Order     `gorm:"foreignKey:OrderID" json:"order,omitempty"`
	CommissionAmount float64    `gorm:"type:decimal(15,2);default:0" json:"commission_amount"` // 佣金金額
	Notes            string     `gorm:"type:text" json:"notes"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CreatedBy        *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy        *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	TrashedAt        *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields      JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`

	// 關聯
	ServiceOrderItems []ServiceOrderItem  `gorm:"foreignKey:ServiceOrderID" json:"service_order_items,omitempty"`
	Labels            []ServiceOrderLabel `gorm:"many2many:service_order_label_relations;foreignKey:ID;joinForeignKey:ServiceOrderID;References:ID;joinReferences:LabelID" json:"labels,omitempty"`
	Appointments      []Appointment       `gorm:"foreignKey:ServiceOrderID" json:"appointments,omitempty"` // 關聯的預約列表
}

// BeforeCreate 創建前設置 UUID
func (so *ServiceOrder) BeforeCreate(tx *gorm.DB) error {
	if so.ID == uuid.Nil {
		so.ID = uuid.New()
	}
	return nil
}

func (ServiceOrder) TableName() string {
	return "service_orders"
}

// ServiceOrderItem 服務單明細（類似訂單明細，但關聯服務而非產品）
type ServiceOrderItem struct {
	ID             uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID    `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ServiceOrderID uuid.UUID    `gorm:"type:uuid;not null;index" json:"service_order_id"`
	ServiceOrder   ServiceOrder `gorm:"foreignKey:ServiceOrderID" json:"service_order,omitempty"`
	ServiceID      *uuid.UUID   `gorm:"type:uuid" json:"service_id"`
	Service        *Service     `gorm:"foreignKey:ServiceID" json:"service,omitempty"`
	StaffID        *uuid.UUID   `gorm:"type:uuid" json:"staff_id,omitempty"`
	Staff          *User        `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
	Quantity       float64      `gorm:"type:decimal(10,2);not null" json:"quantity"`
	UnitPrice      float64      `gorm:"type:decimal(15,2);not null" json:"unit_price"`
	TotalPrice     float64      `gorm:"type:decimal(15,2);not null" json:"total_price"`
	Notes          string       `gorm:"type:text" json:"notes"`
	CreatedAt      time.Time    `json:"created_at"`
	TrashedAt      *time.Time   `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields    JSONB        `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
}

// BeforeCreate 創建前設置 UUID
func (soi *ServiceOrderItem) BeforeCreate(tx *gorm.DB) error {
	if soi.ID == uuid.Nil {
		soi.ID = uuid.New()
	}
	return nil
}

func (ServiceOrderItem) TableName() string {
	return "service_order_items"
}

// ServiceStaff 服務員
type ServiceStaff struct {
	ID             uuid.UUID    `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID    `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant         Tenant       `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name           string       `gorm:"type:varchar(255);not null" json:"name"`
	Phone          string       `gorm:"type:varchar(50)" json:"phone"`
	UserID         *uuid.UUID   `gorm:"type:uuid" json:"user_id,omitempty"` // 可選，如果關聯用戶
	User           *User        `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ServiceTypeID  *uuid.UUID   `gorm:"type:uuid" json:"service_type_id,omitempty"` // 所屬服務類別
	ServiceType    *ServiceType `gorm:"foreignKey:ServiceTypeID" json:"service_type,omitempty"`
	EmployeeNumber *string      `gorm:"type:varchar(50)" json:"employee_number,omitempty"`
	Specialization string       `gorm:"type:text" json:"specialization"`
	HourlyRate     float64      `gorm:"type:decimal(18,2)" json:"hourly_rate"`
	Status         string       `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields    JSONB        `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

func (ServiceStaff) TableName() string {
	return "service_staff"
}

// Room 房間
type Room struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant   Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name     string    `gorm:"type:varchar(255);not null" json:"name"`
	Code     *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	// ImageURL 不落 DB 欄位，存於 ExtraFields.image_url，方便不跑遷移也能支援圖片上傳
	ImageURL     *string   `gorm:"-" json:"image_url,omitempty"`
	Description  string    `gorm:"type:text" json:"description"`
	Capacity     *int      `gorm:"type:integer" json:"capacity,omitempty"`
	Status       string    `gorm:"type:varchar(50);default:'available'" json:"status"`
	AllowOverlap bool      `gorm:"default:false" json:"allow_overlap"` // 允許重複使用
	Notes        string    `gorm:"type:text" json:"notes"`
	ExtraFields  JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Room) TableName() string {
	return "rooms"
}

// Equipment 設備
type Equipment struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant   Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name     string    `gorm:"type:varchar(255);not null" json:"name"`
	Code     *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	// ImageURL 不落 DB 欄位，存於 ExtraFields.image_url
	ImageURL      *string   `gorm:"-" json:"image_url,omitempty"`
	EquipmentType string    `gorm:"type:varchar(100)" json:"equipment_type"`
	Status        string    `gorm:"type:varchar(50);default:'available'" json:"status"`
	AllowOverlap  bool      `gorm:"default:false" json:"allow_overlap"` // 允許重複使用
	Notes         string    `gorm:"type:text" json:"notes"`
	ExtraFields   JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (Equipment) TableName() string {
	return "equipments"
}

// Vehicle 車輛
type Vehicle struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant   Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name     string    `gorm:"type:varchar(255);not null" json:"name"`
	Code     *string   `gorm:"type:varchar(50)" json:"code,omitempty"`
	// ImageURL 不落 DB 欄位，存於 ExtraFields.image_url
	ImageURL     *string    `gorm:"-" json:"image_url,omitempty"`
	VehicleType  string     `gorm:"type:varchar(100)" json:"vehicle_type"`
	LicensePlate string     `gorm:"type:varchar(50)" json:"license_plate"`
	Status       string     `gorm:"type:varchar(50);default:'available'" json:"status"`
	AllowOverlap bool       `gorm:"default:false" json:"allow_overlap"` // 允許重複使用
	Notes        string     `gorm:"type:text" json:"notes"`
	ExtraFields  JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	TrashedAt    *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Vehicle) TableName() string {
	return "vehicles"
}

// AppointmentVehicle 預約車輛關聯表
type AppointmentVehicle struct {
	AppointmentID uuid.UUID   `gorm:"type:uuid;primary_key" json:"appointment_id"`
	VehicleID     uuid.UUID   `gorm:"type:uuid;primary_key" json:"vehicle_id"`
	Appointment   Appointment `gorm:"foreignKey:AppointmentID" json:"appointment,omitempty"`
	Vehicle       Vehicle     `gorm:"foreignKey:VehicleID" json:"vehicle,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
}

func (AppointmentVehicle) TableName() string {
	return "appointment_vehicles"
}

// RoomEquipment 房間設備（保留用於向後兼容，但建議使用 Room 和 Equipment）
type RoomEquipment struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant        Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	RoomName      string    `gorm:"type:varchar(255);not null" json:"room_name"`
	EquipmentName string    `gorm:"type:varchar(255);not null" json:"equipment_name"`
	EquipmentType string    `gorm:"type:varchar(100)" json:"equipment_type"`
	Status        string    `gorm:"type:varchar(50);default:'available'" json:"status"`
	Notes         string    `gorm:"type:text" json:"notes"`
	ExtraFields   JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (RoomEquipment) TableName() string {
	return "room_equipment"
}

// AppointmentRoom 預約房間關聯表
type AppointmentRoom struct {
	AppointmentID uuid.UUID   `gorm:"type:uuid;primary_key" json:"appointment_id"`
	RoomID        uuid.UUID   `gorm:"type:uuid;primary_key" json:"room_id"`
	Appointment   Appointment `gorm:"foreignKey:AppointmentID" json:"appointment,omitempty"`
	Room          Room        `gorm:"foreignKey:RoomID" json:"room,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
}

func (AppointmentRoom) TableName() string {
	return "appointment_rooms"
}

// AppointmentEquipment 預約設備關聯表
type AppointmentEquipment struct {
	AppointmentID uuid.UUID   `gorm:"type:uuid;primary_key" json:"appointment_id"`
	EquipmentID   uuid.UUID   `gorm:"type:uuid;primary_key" json:"equipment_id"`
	Appointment   Appointment `gorm:"foreignKey:AppointmentID" json:"appointment,omitempty"`
	Equipment     Equipment   `gorm:"foreignKey:EquipmentID" json:"equipment,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
}

func (AppointmentEquipment) TableName() string {
	return "appointment_equipments"
}

// ServiceOrderLabel 服務單標籤
type ServiceOrderLabel struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Tenant    Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Name      string     `gorm:"type:varchar(255);not null" json:"name"`
	Color     string     `gorm:"type:varchar(7);not null;default:'#007bff'" json:"color"`
	Status    string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (ServiceOrderLabel) TableName() string {
	return "service_order_labels"
}

// BeforeCreate 創建前設置 UUID
func (sol *ServiceOrderLabel) BeforeCreate(tx *gorm.DB) error {
	if sol.ID == uuid.Nil {
		sol.ID = uuid.New()
	}
	return nil
}
