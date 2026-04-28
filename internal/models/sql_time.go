package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SQLTime 表示「只有時間」的欄位（PostgreSQL TIME）。
// 這個專案目前的 driver 會把 TIME 掃描成 string/[]byte，
// 所以我們用 string 作為底層型別，並自行處理 Scan/Value。
//
// 存的格式固定為 "HH:MM:SS"。
type SQLTime string

func NewSQLTime(t time.Time) SQLTime {
	if t.IsZero() {
		return SQLTime("")
	}
	return SQLTime(t.Format("15:04:05"))
}

func (t SQLTime) Value() (driver.Value, error) {
	if strings.TrimSpace(string(t)) == "" {
		return nil, nil
	}
	return string(t), nil
}

func (t *SQLTime) Scan(value interface{}) error {
	if value == nil {
		*t = SQLTime("")
		return nil
	}

	switch v := value.(type) {
	case []byte:
		*t = SQLTime(strings.TrimSpace(string(v)))
		return nil
	case string:
		*t = SQLTime(strings.TrimSpace(v))
		return nil
	case time.Time:
		*t = NewSQLTime(v)
		return nil
	default:
		return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type *models.SQLTime", value)
	}
}

func (t SQLTime) MarshalJSON() ([]byte, error) {
	if strings.TrimSpace(string(t)) == "" {
		return []byte("null"), nil
	}
	return json.Marshal(string(t))
}

func (t *SQLTime) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*t = SQLTime("")
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*t = SQLTime(strings.TrimSpace(s))
	return nil
}

// ToTime 將 SQLTime 轉換為 time.Time（使用今天的日期）
func (t SQLTime) ToTime() (time.Time, error) {
	s := strings.TrimSpace(string(t))
	if s == "" {
		return time.Time{}, fmt.Errorf("empty SQLTime")
	}
	// 支持 HH:MM 或 HH:MM:SS 格式
	layout := "15:04:05"
	if len(s) == 5 {
		layout = "15:04"
	}
	// 使用今天的日期，時間部分從 SQLTime 解析
	today := time.Now().Format("2006-01-02")
	return time.Parse("2006-01-02 "+layout, today+" "+s)
}

// Hour 返回小時數（0-23）
func (t SQLTime) Hour() int {
	s := strings.TrimSpace(string(t))
	if s == "" {
		return 0
	}
	// 解析 HH:MM 或 HH:MM:SS 格式
	parts := strings.Split(s, ":")
	if len(parts) > 0 {
		var hour int
		fmt.Sscanf(parts[0], "%d", &hour)
		return hour
	}
	return 0
}

// Minute 返回分鐘數（0-59）
func (t SQLTime) Minute() int {
	s := strings.TrimSpace(string(t))
	if s == "" {
		return 0
	}
	// 解析 HH:MM 或 HH:MM:SS 格式
	parts := strings.Split(s, ":")
	if len(parts) > 1 {
		var minute int
		fmt.Sscanf(parts[1], "%d", &minute)
		return minute
	}
	return 0
}

// Second 返回秒數（0-59）
func (t SQLTime) Second() int {
	s := strings.TrimSpace(string(t))
	if s == "" {
		return 0
	}
	// 解析 HH:MM:SS 格式
	parts := strings.Split(s, ":")
	if len(parts) > 2 {
		var second int
		fmt.Sscanf(parts[2], "%d", &second)
		return second
	}
	return 0
}
