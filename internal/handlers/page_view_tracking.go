package handlers

import (
	"time"

	"nwork/internal/database"

	"github.com/google/uuid"
)

// recordPageViewDaily 在 page_view_daily 以 (tenant_id, page_id, view_date) 累加瀏覽量。
// - 使用 UPSERT 避免高併發下產生重複 rows
// - 以 UTC 日期統計（避免伺服器時區/夏令時間造成混亂）
func recordPageViewDaily(tenantID, pageID uuid.UUID, at time.Time) {
	if tenantID == uuid.Nil || pageID == uuid.Nil {
		return
	}

	// 轉成 UTC 的 date
	utc := at.UTC()
	viewDate := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)

	// 不要阻塞頁面渲染：這裡用 goroutine，失敗就忽略
	go func() {
		_ = database.DB.Exec(`
			INSERT INTO page_view_daily (tenant_id, page_id, view_date, view_count, created_at, updated_at)
			VALUES (?, ?, ?, 1, now(), now())
			ON CONFLICT (tenant_id, page_id, view_date)
			DO UPDATE SET view_count = page_view_daily.view_count + 1, updated_at = now()
		`, tenantID, pageID, viewDate).Error
	}()
}


