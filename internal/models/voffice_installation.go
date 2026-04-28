package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VOfficeInstallation vOffice 安裝記錄
type VOfficeInstallation struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	MachineID string     `gorm:"type:varchar(255);not null;uniqueIndex" json:"machine_id"`
	TenantID  *uuid.UUID `gorm:"type:uuid;index" json:"tenant_id,omitempty"`
	UserID    *uuid.UUID `gorm:"type:uuid;index" json:"user_id,omitempty"`

	// Software info
	AppVersion    string `gorm:"type:varchar(50);not null;default:''" json:"app_version"`
	BuildNumber   string `gorm:"type:varchar(50);default:''" json:"build_number"`
	UpdateChannel string `gorm:"type:varchar(20);default:'stable'" json:"update_channel"`

	// OS info
	OSType    string `gorm:"type:varchar(20);not null;default:'';index" json:"os_type"`
	OSVersion string `gorm:"type:varchar(100);default:''" json:"os_version"`
	OSArch    string `gorm:"type:varchar(20);default:''" json:"os_arch"`

	// Hardware info
	CPUModel         string `gorm:"type:varchar(200);default:''" json:"cpu_model"`
	CPUCores         int    `gorm:"default:0" json:"cpu_cores"`
	RAMGB            int    `gorm:"default:0" json:"ram_gb"`
	ScreenResolution string `gorm:"type:varchar(50);default:''" json:"screen_resolution"`
	DisplayCount     int    `gorm:"default:1" json:"display_count"`

	// Network/Geo
	IPAddress string `gorm:"type:varchar(50);default:''" json:"ip_address"`
	Country   string `gorm:"type:varchar(10);default:''" json:"country"`
	City      string `gorm:"type:varchar(100);default:''" json:"city"`
	Language  string `gorm:"type:varchar(20);default:''" json:"language"`

	// Status
	IsActive    bool       `gorm:"default:true;index" json:"is_active"`
	FirstSeenAt time.Time  `gorm:"not null;default:now()" json:"first_seen_at"`
	LastSeenAt  time.Time  `gorm:"not null;default:now();index" json:"last_seen_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`

	// Metadata
	ExtraData JSONB `gorm:"type:jsonb;default:'{}'" json:"extra_data,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (v *VOfficeInstallation) BeforeCreate(tx *gorm.DB) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	if v.ExtraData == nil {
		v.ExtraData = make(JSONB)
	}
	return nil
}

func (VOfficeInstallation) TableName() string {
	return "voffice_installations"
}

// VOfficeRelease vOffice 版本發佈記錄
type VOfficeRelease struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Version     string    `gorm:"type:varchar(50);not null;index" json:"version"`
	BuildNumber string    `gorm:"type:varchar(50);default:''" json:"build_number"`
	Channel     string    `gorm:"type:varchar(20);not null;default:'stable';index" json:"channel"`
	Platform    string    `gorm:"type:varchar(20);not null;index" json:"platform"` // windows / macos / linux

	// Download info
	DownloadURL string `gorm:"type:text;not null" json:"download_url"`
	FileSize    int64  `gorm:"default:0" json:"file_size"`
	Checksum    string `gorm:"type:varchar(128);default:''" json:"checksum"`

	// Version info
	ReleaseNotes string `gorm:"type:text;default:''" json:"release_notes"`
	MinOSVersion string `gorm:"type:varchar(50);default:''" json:"min_os_version"`
	IsMandatory  bool   `gorm:"default:false" json:"is_mandatory"`
	IsLatest     bool   `gorm:"default:false;index" json:"is_latest"`

	PublishedAt time.Time `gorm:"default:now()" json:"published_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (v *VOfficeRelease) BeforeCreate(tx *gorm.DB) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return nil
}

func (VOfficeRelease) TableName() string {
	return "voffice_releases"
}
