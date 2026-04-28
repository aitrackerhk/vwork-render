package models

import (
	"time"

	"github.com/google/uuid"
)

// JobVacancy 空缺
type JobVacancy struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Title        string     `gorm:"type:varchar(255);not null" json:"title"`
	DepartmentID *uuid.UUID `gorm:"type:uuid" json:"department_id,omitempty"`
	Headcount    int        `gorm:"not null;default:1" json:"headcount"`
	Status       string     `gorm:"type:varchar(20);not null;default:'open'" json:"status"` // open/closed
	Description  string     `gorm:"type:text" json:"description,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	// 手動填充（避免跨 company/tenant 的 join 複雜度）
	Department *Department `gorm:"-" json:"department,omitempty"`
}

func (JobVacancy) TableName() string {
	return "job_vacancies"
}


