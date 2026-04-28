package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID                 uuid.UUID   `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID           *uuid.UUID  `gorm:"type:uuid;index" json:"tenant_id,omitempty"`
	Email              string      `gorm:"type:varchar(255);not null;uniqueIndex:idx_tenant_email" json:"email"`
	PasswordHash       string      `gorm:"type:varchar(255);not null" json:"-"`
	Name               string      `gorm:"type:varchar(255);not null" json:"name"`
	EmployeeNumber     string      `gorm:"type:varchar(100)" json:"employee_number"`         // 員工編號
	UserRole           string      `gorm:"type:varchar(50);default:'user'" json:"user_role"` // admin, manager, user (保留用於向後兼容)
	Status             string      `gorm:"type:varchar(50);default:'active'" json:"status"`  // active, inactive
	LevelID            *uuid.UUID  `gorm:"type:uuid" json:"level_id,omitempty"`              // 保留用於向後兼容
	Level              *Level      `gorm:"foreignKey:LevelID" json:"level,omitempty"`        // 保留用於向後兼容
	RoleID             *uuid.UUID  `gorm:"type:uuid" json:"role_id,omitempty"`
	Role               *Role       `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	DepartmentID       *uuid.UUID  `gorm:"type:uuid" json:"department_id,omitempty"`
	Department         *Department `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
	ShiftID            *uuid.UUID  `gorm:"type:uuid" json:"shift_id,omitempty"`
	Shift              *Shift      `gorm:"foreignKey:ShiftID" json:"shift,omitempty"`
	Salary             float64     `gorm:"type:decimal(15,2);default:0" json:"salary"`
	SalaryMode         string      `gorm:"type:varchar(20);default:'monthly'" json:"salary_mode"` // 薪資方式: monthly, hourly
	CommissionRate     float64     `gorm:"type:decimal(5,2);default:0" json:"commission_rate"`    // 佣金率（百分比）
	ProfilePic         string      `gorm:"type:varchar(500)" json:"profile_pic,omitempty"`        // 用戶頭像
	BirthDate          *time.Time  `gorm:"type:date" json:"birth_date,omitempty"`                 // 出生日期
	Phone              string      `gorm:"type:varchar(50)" json:"phone,omitempty"`               // 電話（格式：+852 12345678）
	LastLoginAt        *time.Time  `json:"last_login_at"`
	LoggedOutAt        *time.Time  `gorm:"type:timestamp with time zone" json:"logged_out_at,omitempty"`         // SSO global logout (legacy, treated as web)
	WebLoggedOutAt     *time.Time  `gorm:"type:timestamp with time zone" json:"web_logged_out_at,omitempty"`     // Web platform logout: web tokens issued before this time are invalid
	DesktopLoggedOutAt *time.Time  `gorm:"type:timestamp with time zone" json:"desktop_logged_out_at,omitempty"` // Desktop platform logout: desktop tokens issued before this time are invalid
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
	TrashedAt          *time.Time  `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
	ExtraFields        JSONB       `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
	Tenant             Tenant      `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	if u.ExtraFields == nil {
		u.ExtraFields = make(JSONB)
	}
	return nil
}
