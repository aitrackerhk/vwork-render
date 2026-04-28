package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BusinessGoal 業務目標模型
type BusinessGoal struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	Title          string     `gorm:"type:varchar(255);not null" json:"title"`
	Description    string     `gorm:"type:text" json:"description"`
	MetricType     string     `gorm:"type:varchar(50);not null;default:'custom'" json:"metric_type"` // order_count, revenue, customer_count, product_sales_qty, service_order_count, custom
	TargetEntityID *uuid.UUID `gorm:"type:uuid" json:"target_entity_id,omitempty"`                   // for product/service specific goals
	TargetValue    float64    `gorm:"type:decimal(18,2);not null;default:0" json:"target_value"`
	CurrentValue   float64    `gorm:"type:decimal(18,2);not null;default:0" json:"current_value"`
	Unit           string     `gorm:"type:varchar(50);not null;default:''" json:"unit"`
	StartDate      time.Time  `gorm:"type:date;not null" json:"start_date"`
	EndDate        time.Time  `gorm:"type:date;not null" json:"end_date"`
	Status         string     `gorm:"type:varchar(20);not null;default:'active'" json:"status"`   // active, completed, failed, paused
	Priority       string     `gorm:"type:varchar(20);not null;default:'medium'" json:"priority"` // low, medium, high

	// AI fields
	AiSuggestions    JSONB      `gorm:"type:jsonb;default:'{}'" json:"ai_suggestions"`
	AiLastAnalyzedAt *time.Time `gorm:"type:timestamp with time zone" json:"ai_last_analyzed_at,omitempty"`

	// Ownership
	CreatedBy *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	TrashedAt *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	ExtraFields JSONB `gorm:"type:jsonb;default:'{}'" json:"extra_fields"`

	// Relations
	Trackings  []BusinessGoalTracking   `gorm:"foreignKey:GoalID" json:"trackings,omitempty"`
	AIAnalyses []BusinessGoalAIAnalysis `gorm:"foreignKey:GoalID" json:"ai_analyses,omitempty"`
}

// BeforeCreate 創建前設置 UUID
func (bg *BusinessGoal) BeforeCreate(tx *gorm.DB) error {
	if bg.ID == uuid.Nil {
		bg.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (BusinessGoal) TableName() string {
	return "business_goals"
}

// ProgressPercent 計算進度百分比
func (bg *BusinessGoal) ProgressPercent() float64 {
	if bg.TargetValue == 0 {
		return 0
	}
	pct := (bg.CurrentValue / bg.TargetValue) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

// BusinessGoalTracking 業務目標追蹤記錄
type BusinessGoalTracking struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null;index" json:"tenant_id"`
	GoalID       uuid.UUID `gorm:"type:uuid;not null;index" json:"goal_id"`
	TrackedValue float64   `gorm:"type:decimal(18,2);not null;default:0" json:"tracked_value"`
	DeltaValue   float64   `gorm:"type:decimal(18,2);not null;default:0" json:"delta_value"`
	ProgressPct  float64   `gorm:"type:decimal(5,2);not null;default:0" json:"progress_pct"`
	AiNote       string    `gorm:"type:text" json:"ai_note"`
	Source       string    `gorm:"type:varchar(20);not null;default:'auto'" json:"source"` // auto, manual
	TrackedAt    time.Time `gorm:"type:timestamp with time zone;not null;default:NOW()" json:"tracked_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// BeforeCreate 創建前設置 UUID
func (t *BusinessGoalTracking) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (BusinessGoalTracking) TableName() string {
	return "business_goal_trackings"
}

// BusinessGoalAIAnalysis AI 分析歷史記錄
type BusinessGoalAIAnalysis struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_id"`
	GoalID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"goal_id"`
	AnalysisType string     `gorm:"type:varchar(20);not null;default:'suggestion'" json:"analysis_type"` // suggestion, chat
	UserPrompt   string     `gorm:"type:text" json:"user_prompt,omitempty"`
	AIResponse   string     `gorm:"type:text;not null" json:"ai_response"`
	GoalSnapshot JSONB      `gorm:"type:jsonb;default:'{}'" json:"goal_snapshot"`
	CreatedBy    *uuid.UUID `gorm:"type:uuid" json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
}

// BeforeCreate 創建前設置 UUID
func (a *BusinessGoalAIAnalysis) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// TableName 指定表名
func (BusinessGoalAIAnalysis) TableName() string {
	return "business_goal_ai_analyses"
}
