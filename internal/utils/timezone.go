package utils

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/models"
	"time"

	"github.com/google/uuid"
)

// GetTenantTimezone 獲取租戶的時區設置
func GetTenantTimezone(tenantID uuid.UUID) string {
	var enterprise models.Enterprise
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&enterprise).Error; err == nil {
		if enterprise.Timezone != "" {
			return enterprise.Timezone
		}
	}
	return "Asia/Hong_Kong" // 默認時區
}

// GetTenantLocation 獲取租戶的時區 Location 對象
func GetTenantLocation(tenantID uuid.UUID) *time.Location {
	timezone := GetTenantTimezone(tenantID)
	loc, err := time.LoadLocation(timezone)
	if err != nil || loc == nil {
		// 如果時區無效，使用默認值
		loc, err = time.LoadLocation("Asia/Hong_Kong")
	}
	// 在某些部署環境（例如獨立 exe、系統缺少 zoneinfo）下 LoadLocation 仍可能失敗，
	// 為了避免 time.Time.In(nil) 直接 panic，最後一層保底回退到 UTC。
	if err != nil || loc == nil {
		return time.UTC
	}
	return loc
}

// NowInTenantTimezone 獲取租戶時區的當前時間
func NowInTenantTimezone(tenantID uuid.UUID) time.Time {
	loc := GetTenantLocation(tenantID)
	return time.Now().In(loc)
}

// ParseDateInTenantTimezone 在租戶時區中解析日期字符串
func ParseDateInTenantTimezone(tenantID uuid.UUID, dateStr string) (time.Time, error) {
	loc := GetTenantLocation(tenantID)
	return time.ParseInLocation("2006-01-02", dateStr, loc)
}

// FormatDateInTenantTimezone 在租戶時區中格式化日期
func FormatDateInTenantTimezone(tenantID uuid.UUID, t time.Time) string {
	loc := GetTenantLocation(tenantID)
	return t.In(loc).Format("2006-01-02")
}

// TodayInTenantTimezone 獲取租戶時區的今天日期字符串
func TodayInTenantTimezone(tenantID uuid.UUID) string {
	now := NowInTenantTimezone(tenantID)
	return now.Format("2006-01-02")
}

// ParseDateTimeInTenantTimezone 在租戶時區中解析日期時間字符串（支持多種格式）
func ParseDateTimeInTenantTimezone(tenantID uuid.UUID, dateTimeStr string) (time.Time, error) {
	loc := GetTenantLocation(tenantID)
	
	// 嘗試 RFC3339 格式（帶時區）
	if t, err := time.Parse(time.RFC3339, dateTimeStr); err == nil {
		// 轉換到租戶時區
		return t.In(loc), nil
	}
	
	// 嘗試 "2006-01-02T15:04" 格式（不帶時區，假設為租戶時區）
	if t, err := time.ParseInLocation("2006-01-02T15:04", dateTimeStr, loc); err == nil {
		return t, nil
	}
	
	// 嘗試 "2006-01-02 15:04:05" 格式
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", dateTimeStr, loc); err == nil {
		return t, nil
	}
	
	// 嘗試 "2006-01-02 15:04" 格式
	if t, err := time.ParseInLocation("2006-01-02 15:04", dateTimeStr, loc); err == nil {
		return t, nil
	}
	
	return time.Time{}, fmt.Errorf("unable to parse datetime: %s", dateTimeStr)
}

