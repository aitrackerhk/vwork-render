package models

import (
	"time"

	"github.com/google/uuid"
)

// Calendar 日曆
type Calendar struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	User        User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	EventType   string     `gorm:"type:varchar(50)" json:"event_type"`
	StartTime   time.Time  `gorm:"type:timestamp;not null" json:"start_time"`
	EndTime     *time.Time `gorm:"type:timestamp" json:"end_time,omitempty"`
	AllDay      bool       `gorm:"default:false" json:"all_day"`
	Recurrence  JSONB      `gorm:"type:jsonb" json:"recurrence,omitempty"`
	Status      string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (Calendar) TableName() string {
	return "calendars"
}

// Reminder 提醒
type Reminder struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	User        User       `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	RemindTime  time.Time  `gorm:"type:timestamp;not null" json:"remind_time"`
	IsCompleted bool       `gorm:"default:false" json:"is_completed"`
	CompletedAt *time.Time `gorm:"type:timestamp" json:"completed_at,omitempty"`
	Status      string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (Reminder) TableName() string {
	return "reminders"
}

// AiConversation AI 對話會話
type AiConversation struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	Title     string    `gorm:"type:varchar(255);not null;default:'新對話'" json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AiConversation) TableName() string {
	return "ai_conversations"
}

// AiSketch AI 草圖
type AiSketch struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	Title        string    `gorm:"type:varchar(255);not null;default:'未命名草圖'" json:"title"`
	Objects      string    `gorm:"type:text;default:'[]'" json:"objects"`
	CanvasWidth  int       `gorm:"type:int;default:800" json:"canvas_width"`
	CanvasHeight int       `gorm:"type:int;default:600" json:"canvas_height"`
	AspectRatio  string    `gorm:"type:varchar(10);default:'free'" json:"aspect_ratio"`
	Thumbnail    string    `gorm:"type:text" json:"thumbnail,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (AiSketch) TableName() string {
	return "ai_sketches"
}

// AiDocument stores AI-generated documents (docx, xlsx, pptx) for download
type AiDocument struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID     uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	Title        string     `gorm:"type:varchar(255);not null" json:"title"`
	Prompt       string     `gorm:"type:text;not null" json:"prompt"`
	DocType      string     `gorm:"type:varchar(20);not null" json:"doc_type"`     // docx, xlsx, pptx
	FilePath     string     `gorm:"type:varchar(500);default:''" json:"file_path"` // server file path
	FileURL      string     `gorm:"type:varchar(500);default:''" json:"file_url"`  // download URL
	FileSize     int64      `gorm:"type:bigint;default:0" json:"file_size"`        // file size in bytes
	Model        string     `gorm:"type:varchar(100)" json:"model,omitempty"`
	Status       string     `gorm:"type:varchar(20);default:'pending'" json:"status"` // pending, generating, completed, failed
	Source       string     `gorm:"type:varchar(20);default:'docs'" json:"source"`    // docs, chat
	ErrorMessage string     `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `gorm:"type:timestamp" json:"completed_at,omitempty"`
}

func (AiDocument) TableName() string {
	return "ai_documents"
}

// AiSketchGeneration stores AI generation history (prompt + result image)
// Source: "sketch" = generated from vai-sketch tool, "chat" = generated from vai-chat
type AiSketchGeneration struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	SketchID    *uuid.UUID `gorm:"type:uuid" json:"sketch_id,omitempty"`
	Prompt      string     `gorm:"type:text;not null" json:"prompt"`
	ResultImage string     `gorm:"type:text" json:"result_image,omitempty"`
	Model       string     `gorm:"type:varchar(100)" json:"model,omitempty"`
	Source      string     `gorm:"type:varchar(20);default:'sketch'" json:"source"`
	Status      string     `gorm:"type:varchar(20);default:'completed'" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (AiSketchGeneration) TableName() string {
	return "ai_sketch_generations"
}

// Message 訊息
type Message struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID       uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant         Tenant     `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	FromUserID     *uuid.UUID `gorm:"type:uuid" json:"from_user_id,omitempty"`
	FromUser       *User      `gorm:"foreignKey:FromUserID" json:"from_user,omitempty"`
	ToUserID       *uuid.UUID `gorm:"type:uuid" json:"to_user_id,omitempty"` // 改为可选，因为可能是发给客户
	ToUser         *User      `gorm:"foreignKey:ToUserID" json:"to_user,omitempty"`
	ToCustomerID   *uuid.UUID `gorm:"type:uuid" json:"to_customer_id,omitempty"` // 新增：发给客户的ID
	ToCustomer     *Customer  `gorm:"foreignKey:ToCustomerID" json:"to_customer,omitempty"`
	ConversationID *uuid.UUID `gorm:"type:uuid" json:"conversation_id,omitempty"` // AI 對話會話 ID
	Subject        string     `gorm:"type:varchar(255)" json:"subject"`
	Content        string     `gorm:"type:text;not null" json:"content"`
	IsRead         bool       `gorm:"default:false" json:"is_read"`
	ReadAt         *time.Time `gorm:"type:timestamp" json:"read_at,omitempty"`
	MessageType    string     `gorm:"type:varchar(50);default:'normal'" json:"message_type"`
	Status         string     `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields    JSONB      `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	TrashedAt      *time.Time `gorm:"type:timestamp with time zone" json:"trashed_at,omitempty"` // 軟刪除時間
}

func (Message) TableName() string {
	return "messages"
}

// Note 備忘
type Note struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	UserID      uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Title       string    `gorm:"type:varchar(255);not null" json:"title"`
	Content     string    `gorm:"type:text" json:"content"`
	Category    string    `gorm:"type:varchar(100)" json:"category"`
	Tags        JSONB     `gorm:"type:jsonb;default:'[]'" json:"tags,omitempty"`
	IsPinned    bool      `gorm:"default:false" json:"is_pinned"`
	Status      string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Note) TableName() string {
	return "notes"
}

// PersonalData 個人資料
type PersonalData struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID    uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	Tenant      Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	UserID      uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	User        User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	DataType    string    `gorm:"type:varchar(100);not null" json:"data_type"`
	KeyName     string    `gorm:"type:varchar(255);not null" json:"key_name"`
	Value       string    `gorm:"type:text" json:"value"`
	IsEncrypted bool      `gorm:"default:false" json:"is_encrypted"`
	Status      string    `gorm:"type:varchar(20);default:'active'" json:"status"`
	ExtraFields JSONB     `gorm:"type:jsonb;default:'{}'" json:"extra_fields,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (PersonalData) TableName() string {
	return "personal_data"
}
