package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectTask 項目任務（多租戶）
type ProjectTask struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	ProjectID uuid.UUID `gorm:"type:uuid;not null;index" json:"project_id"`

	Title       string `gorm:"type:varchar(255);not null" json:"title"`
	Description string `gorm:"type:text" json:"description"`

	Status   string `gorm:"type:varchar(50);default:'todo';index" json:"status"`     // todo, in_progress, done, blocked
	Priority string `gorm:"type:varchar(50);default:'medium';index" json:"priority"` // low, medium, high

	DueDate *time.Time `gorm:"type:date" json:"due_date,omitempty"`

	AssigneeUserID *uuid.UUID `gorm:"type:uuid" json:"assignee_user_id,omitempty"`
	Assignee       *User      `gorm:"foreignKey:AssigneeUserID" json:"assignee,omitempty"`

	SortOrder int `gorm:"type:int;default:0;index" json:"sort_order"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間

	ExtraFields JSONB `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`
}

func (t *ProjectTask) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.ExtraFields == nil {
		t.ExtraFields = make(JSONB)
	}
	return nil
}
