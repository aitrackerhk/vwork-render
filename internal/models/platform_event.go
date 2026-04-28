package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PlatformEvent is a platform-level event managed via vworkadmin.
// Independent of tenants — used for the vWork official website events section.
type PlatformEvent struct {
	ID            uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Title         string     `gorm:"type:varchar(500);not null" json:"title"`
	Slug          string     `gorm:"type:varchar(500);not null;uniqueIndex:idx_platform_events_lang_slug" json:"slug"`
	Content       *string    `gorm:"type:text" json:"content,omitempty"`
	Excerpt       *string    `gorm:"type:text" json:"excerpt,omitempty"`
	FeaturedImage *string    `gorm:"type:varchar(1000)" json:"featured_image,omitempty"`
	Location      *string    `gorm:"type:varchar(500)" json:"location,omitempty"`
	EventDate     *time.Time `gorm:"type:timestamp with time zone" json:"event_date,omitempty"`
	EventEndDate  *time.Time `gorm:"type:timestamp with time zone" json:"event_end_date,omitempty"`
	MaxAttendees  int        `gorm:"default:0" json:"max_attendees"`                 // 0 = unlimited
	Status        string     `gorm:"type:varchar(50);default:'draft'" json:"status"` // draft, published, archived, cancelled
	PublishedAt   *time.Time `gorm:"type:timestamp with time zone" json:"published_at,omitempty"`
	Category      *string    `gorm:"type:varchar(200)" json:"category,omitempty"`
	Lang          string     `gorm:"type:varchar(10);default:'zh';uniqueIndex:idx_platform_events_lang_slug" json:"lang"`
	SortOrder     int        `gorm:"default:0" json:"sort_order"`
	ViewCount     int        `gorm:"default:0" json:"view_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Virtual field: count of registrations (not stored in DB)
	RegistrationCount int `gorm:"-" json:"registration_count,omitempty"`
}

func (PlatformEvent) TableName() string {
	return "platform_events"
}

func (e *PlatformEvent) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// PlatformEventRegistration records a user's registration for an event.
type PlatformEventRegistration struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	EventID   uuid.UUID `gorm:"type:uuid;not null;index:idx_event_registrations_event_id" json:"event_id"`
	Name      string    `gorm:"type:varchar(255);not null" json:"name"`
	Phone     string    `gorm:"type:varchar(50);not null" json:"phone"`
	Email     *string   `gorm:"type:varchar(255)" json:"email,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (PlatformEventRegistration) TableName() string {
	return "platform_event_registrations"
}

func (r *PlatformEventRegistration) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}
