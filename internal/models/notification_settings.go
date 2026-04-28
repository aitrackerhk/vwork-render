package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NotificationSettings 系統提示設定
type NotificationSettings struct {
	ID                                uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	TenantID                          uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"tenant_id"`
	AttendanceNotificationsEnabled   bool      `gorm:"default:true" json:"attendance_notifications_enabled"`   // 打卡提示資訊
	OrderNotificationsEnabled        bool      `gorm:"default:true" json:"order_notifications_enabled"`        // 訂單提示資訊
	ServiceOrderNotificationsEnabled  bool      `gorm:"default:true" json:"service_order_notifications_enabled"`  // 服務單提示資訊
	AppointmentNotificationsEnabled   bool      `gorm:"default:true" json:"appointment_notifications_enabled"`   // 預約提示資訊
	ProjectDueNotificationsEnabled    bool      `gorm:"default:true" json:"project_due_notifications_enabled"`    // 項目/task 到期提示資訊
	CreatedAt                         time.Time `json:"created_at"`
	UpdatedAt                         time.Time `json:"updated_at"`

	// Relations
	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

func (NotificationSettings) TableName() string {
	return "notification_settings"
}

func (ns *NotificationSettings) BeforeCreate(tx *gorm.DB) error {
	if ns.ID == uuid.Nil {
		ns.ID = uuid.New()
	}
	return nil
}

