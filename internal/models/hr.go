package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Attendance 打卡記錄
type Attendance struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Date          time.Time `gorm:"type:date;not null;index" json:"date"`
	ClockIn       *time.Time `gorm:"type:timestamp" json:"clock_in,omitempty"`
	ClockOut      *time.Time `gorm:"type:timestamp" json:"clock_out,omitempty"`
	BreakDuration int       `gorm:"default:0" json:"break_duration"` // 休息時間（分鐘）
	WorkDuration  int       `gorm:"default:0" json:"work_duration"` // 工作時長（分鐘）
	OTDuration    int       `gorm:"default:0" json:"ot_duration"` // 加班時長（分鐘）
	Status        string    `gorm:"type:varchar(50);default:'normal'" json:"status"` // normal, late, early_leave, absent
	Notes         string    `gorm:"type:text" json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	
	// 關聯
	User          *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// LeaveRequest 請假申請
type LeaveRequest struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	LeaveType     string    `gorm:"type:varchar(50);not null" json:"leave_type"` // annual, sick, personal, unpaid
	StartDate     time.Time `gorm:"type:date;not null" json:"start_date"`
	EndDate       time.Time `gorm:"type:date;not null" json:"end_date"`
	Days          float64   `gorm:"type:decimal(5,2);not null" json:"days"`
	Reason        string    `gorm:"type:text" json:"reason"`
	Status        string    `gorm:"type:varchar(50);default:'pending'" json:"status"` // pending, approved, rejected, cancelled
	ApprovedBy    *uuid.UUID `gorm:"type:uuid" json:"approved_by,omitempty"`
	ApprovedAt    *time.Time `gorm:"type:timestamp" json:"approved_at,omitempty"`
	RejectReason  string    `gorm:"type:text" json:"reject_reason"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	
	// 關聯
	User          *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Approver      *User     `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`
}

// Payroll 薪資記錄
type Payroll struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	UserID        uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	PayPeriod     string    `gorm:"type:varchar(50);not null" json:"pay_period"` // YYYY-MM
	BaseSalary    float64   `gorm:"type:decimal(15,2);default:0" json:"base_salary"`
	OTHours       float64   `gorm:"type:decimal(10,2);default:0" json:"ot_hours"`
	OTAmount      float64   `gorm:"type:decimal(15,2);default:0" json:"ot_amount"`
	MPFEmployee   float64   `gorm:"type:decimal(15,2);default:0" json:"mpf_employee"` // 員工強制供款金額
	MPFEmployer   float64   `gorm:"type:decimal(15,2);default:0" json:"mpf_employer"` // 雇主強制供款金額
	MPFTotal      float64   `gorm:"type:decimal(15,2);default:0" json:"mpf_total"`
	EmployeeMandatoryRate float64 `gorm:"type:decimal(6,4);default:0.05" json:"employee_mandatory_rate"` // 員工強制供款 %
	EmployerMandatoryRate float64 `gorm:"type:decimal(6,4);default:0.05" json:"employer_mandatory_rate"` // 雇主強制供款 %
	Allowances    float64   `gorm:"type:decimal(15,2);default:0" json:"allowances"` // 津貼
	Deductions    float64   `gorm:"type:decimal(15,2);default:0" json:"deductions"` // 扣除
	GrossSalary   float64   `gorm:"type:decimal(15,2);default:0" json:"gross_salary"` // 總薪金
	NetSalary     float64   `gorm:"type:decimal(15,2);default:0" json:"net_salary"` // 淨薪金
	Status        string    `gorm:"type:varchar(50);default:'draft'" json:"status"` // draft, confirmed, paid
	Notes         string    `gorm:"type:text" json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	
	// 關聯
	User          *User     `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Contributions []PayrollContribution `gorm:"foreignKey:PayrollID" json:"contributions,omitempty"`
	Adjustments   []PayrollAdjustment   `gorm:"foreignKey:PayrollID" json:"adjustments,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (a *Attendance) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

func (lr *LeaveRequest) BeforeCreate(tx *gorm.DB) error {
	if lr.ID == uuid.Nil {
		lr.ID = uuid.New()
	}
	return nil
}

func (p *Payroll) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (Attendance) TableName() string {
	return "attendances"
}

func (LeaveRequest) TableName() string {
	return "leave_requests"
}

func (Payroll) TableName() string {
	return "payrolls"
}

// PayrollContribution 供款明細
type PayrollContribution struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	PayrollID uuid.UUID `gorm:"type:uuid;not null;index" json:"payroll_id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"` // 款項名稱
	Payer     string    `gorm:"type:varchar(20);not null" json:"payer"`  // employee, employer
	Mode      string    `gorm:"type:varchar(20);not null" json:"mode"`   // percent, fixed
	Rate      float64   `gorm:"type:decimal(10,4);default:0" json:"rate"` // 0.05 = 5%
	Amount    float64   `gorm:"type:decimal(15,2);default:0" json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

func (PayrollContribution) TableName() string {
	return "payroll_contributions"
}

// PayrollAdjustment 薪資附加項目（多 row）
type PayrollAdjustment struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	PayrollID uuid.UUID `gorm:"type:uuid;not null;index" json:"payroll_id"`

	Name      string  `gorm:"type:varchar(255);not null;default:''" json:"name"`
	Direction string  `gorm:"type:varchar(20);not null;default:'add'" json:"direction"` // add/subtract
	Mode      string  `gorm:"type:varchar(20);not null;default:'fixed'" json:"mode"`    // percent/fixed
	Rate      float64 `gorm:"type:decimal(10,4);default:0" json:"rate"`                 // 0.05=5%
	Amount    float64 `gorm:"type:decimal(15,2);default:0" json:"amount"`               // fixed
	CreatedAt time.Time `json:"created_at"`
}

func (PayrollAdjustment) TableName() string { return "payroll_adjustments" }

// Holiday 假期
type Holiday struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Name        string    `gorm:"type:varchar(255);not null" json:"name"` // 假期名稱
	Description string    `gorm:"type:text" json:"description"`            // 假期描述
	StartDate   time.Time `gorm:"type:date;not null" json:"start_date"`   // 開始日期
	EndDate     time.Time `gorm:"type:date;not null" json:"end_date"`     // 結束日期
	IsRecurring bool      `gorm:"default:false" json:"is_recurring"`       // 是否每年重複
	RecurringRule string  `gorm:"type:varchar(100)" json:"recurring_rule"` // 重複規則（如：每年1月1日）
	Status      string    `gorm:"type:varchar(50);default:'active'" json:"status"` // active, inactive
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy   *uuid.UUID `gorm:"type:uuid" json:"updated_by,omitempty"`
}

func (h *Holiday) BeforeCreate(tx *gorm.DB) error {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	return nil
}

func (Holiday) TableName() string {
	return "holidays"
}




