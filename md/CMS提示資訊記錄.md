# CMS 提示資訊系統記錄

## 概述
CMS 系統中的提示資訊（Notifications）功能用於向用戶發送系統自動生成的提醒和通知。

## 提示資訊頁面
- **路由**: `/notifications`
- **頁面標題**: 提示資訊
- **模板文件**: `nwork/web/templates/pages/notifications.html`
- **API 路徑**: `/api/v1/notifications`

## 功能特性

### 1. 顯示功能
- 顯示所有提示資訊列表
- 支持過濾：全部 / 未讀 / 已讀
- 每30秒自動刷新
- 分頁顯示（每頁20條）

### 2. 操作功能
- **標記為已讀**: 單個提示資訊可標記為已讀
- **全部標記為已讀**: 一鍵標記所有提示為已讀
- **刷新**: 手動刷新提示資訊列表
- **點擊跳轉**: 點擊提示資訊可跳轉到相關頁面（如果有連結）

### 3. 未讀數量顯示
- 在頂部導航欄顯示未讀提示資訊數量（Badge）
- 在側邊欄顯示未讀提示資訊數量（Badge）
- 每30秒自動更新未讀數量
- 超過99條顯示為 "99+"

## 提示資訊類型（Type）

### 1. `appointment_today` - 今日有預約
- **生成時間**: 每天早上8:00
- **觸發條件**: 當日有預約記錄
- **標題**: "今日有預約"
- **訊息內容**:
  - 單個預約: "您今日有 1 個預約：[客戶名稱] 的 [服務名稱]（[時間]）"
  - 多個預約: "您今日有 [數量] 個預約，請查看預約管理頁面"
- **連結**: `/appointments?appointment_date=[今日日期]`
- **防重複**: 每天每個用戶只生成一次

### 2. `reminder` - 提醒通知
- **生成時間**: 每小時掃描一次
- **觸發條件**: 提醒時間在未來1小時內或已過期（最多過期1小時）
- **標題和訊息**:
  - **已過期**: 
    - 標題: "提醒已到期"
    - 訊息: "提醒「[提醒標題]」已到期：[描述]"
  - **15分鐘內到期**:
    - 標題: "提醒即將到期"
    - 訊息: "提醒「[提醒標題]」將在 [分鐘數] 分鐘後到期：[描述]"
  - **1小時內到期**:
    - 標題: "提醒通知"
    - 訊息: "提醒「[提醒標題]」將在 [分鐘數] 分鐘後到期：[描述]"
- **連結**: `/reminders`
- **防重複**: 每個提醒在提醒時間前後1小時內只生成一次

### 3. `project_due_today` - 項目今日到期
- **生成時間**: 每小時掃描一次
- **觸發條件**: 項目的 `end_date` 等於今日
- **標題**: "項目今日到期"
- **訊息**: "項目「[項目名稱]」今日到期，請確認是否需要調整日期或標記完成。"
- **連結**: `/projects/[項目ID]/edit`
- **防重複**: 使用 `dedupe_key` 防止重複生成
- **接收者**: 項目負責人（OwnerUserID）

### 4. `task_due_today` - 任務今日到期
- **生成時間**: 每小時掃描一次
- **觸發條件**: 任務的 `due_date` 等於今日
- **標題**: "任務今日到期"
- **訊息**: "任務「[任務標題]」今日到期，請確認進度。"
- **連結**: `/projects/[項目ID]/edit`
- **防重複**: 使用 `dedupe_key` 防止重複生成
- **接收者**: 任務負責人（AssigneeUserID）

## 數據模型

### NotificationAlert 結構
```go
type NotificationAlert struct {
    ID          uuid.UUID  // 主鍵
    TenantID    uuid.UUID  // 租戶ID
    UserID      uuid.UUID  // 用戶ID（接收者）
    Type        string     // 類型（appointment_today, reminder, project_due_today, task_due_today）
    Title       string     // 標題
    Message     string     // 訊息內容
    Link        string     // 可選的連結
    DedupeKey   string     // 防重複鍵（可選）
    IsRead      bool       // 是否已讀
    ReadAt      *time.Time // 已讀時間
    GeneratedAt time.Time  // 生成時間
    CreatedAt   time.Time  // 創建時間
    UpdatedAt   time.Time  // 更新時間
}
```

### 數據表
- **表名**: `notification_alerts`
- **主鍵**: `id` (UUID)
- **索引**: `tenant_id`, `user_id`

## API 接口

### 1. 獲取提示資訊列表
- **方法**: `GET`
- **路徑**: `/api/v1/notifications`
- **查詢參數**:
  - `page`: 頁碼（默認: 1）
  - `limit`: 每頁數量（默認: 50）
  - `is_read`: 過濾已讀/未讀（true/false）
  - `type`: 過濾類型

### 2. 獲取未讀數量
- **方法**: `GET`
- **路徑**: `/api/v1/notifications/unread-count`
- **返回**: `{"count": 數量}`

### 3. 獲取單個提示資訊
- **方法**: `GET`
- **路徑**: `/api/v1/notifications/:id`

### 4. 標記為已讀
- **方法**: `PUT`
- **路徑**: `/api/v1/notifications/:id/read`

### 5. 全部標記為已讀
- **方法**: `PUT`
- **路徑**: `/api/v1/notifications/read-all`
- **返回**: `{"success": true, "count": 標記數量}`

## 定時任務（Cronjob）

### 1. 預約提示生成
- **執行時間**: 每天早上 8:00
- **函數**: `generateAppointmentNotifications()`
- **功能**: 為所有活躍租戶的用戶生成今日預約提示

### 2. 提醒掃描
- **執行時間**: 每小時執行一次
- **函數**: `generateReminderNotifications()`
- **功能**: 掃描即將到期或已過期的提醒，生成提示資訊

### 3. 項目/任務到期掃描
- **執行時間**: 每小時執行一次
- **函數**: `generateProjectDueNotifications()`
- **功能**: 掃描今日到期的項目和任務，生成提示資訊

## 相關文件

### 後端文件
- **模型**: `nwork/internal/models/notification_alert.go`
- **處理器**: `nwork/internal/handlers/notification.go`
- **定時任務**: `nwork/cmd/cronjob/main.go`
- **路由配置**: `nwork/cmd/api/main.go` (第1821-1826行)

### 前端文件
- **頁面模板**: `nwork/web/templates/pages/notifications.html`
- **佈局文件**: `nwork/web/templates/layouts/cms_layout.html` (第362-404行)
- **應用工具**: `nwork/web/static/js/app.js` (提示訊息顯示功能)

## 使用說明

### 查看提示資訊
1. 點擊頂部導航欄或側邊欄的「提示資訊」圖標
2. 使用過濾按鈕查看全部/未讀/已讀提示
3. 點擊提示卡片可跳轉到相關頁面並自動標記為已讀

### 管理提示資訊
- 單個標記為已讀：點擊提示卡片右側的「✓」按鈕
- 全部標記為已讀：點擊頁面右上角的「全部標記為已讀」按鈕
- 手動刷新：點擊「刷新」按鈕

## 注意事項

1. **防重複機制**:
   - 預約提示：每天每個用戶只生成一次（基於 `type` 和 `generated_at`）
   - 提醒提示：每個提醒在提醒時間前後1小時內只生成一次
   - 項目/任務到期：使用 `dedupe_key` 防止重複生成

2. **自動刷新**:
   - 提示資訊列表每30秒自動刷新
   - 未讀數量每30秒自動更新

3. **權限**:
   - 每個用戶只能看到自己的提示資訊
   - 基於 `tenant_id` 和 `user_id` 進行過濾

4. **數據清理**:
   - 目前沒有自動清理機制
   - 建議定期清理舊的已讀提示資訊

## 未來擴展建議

1. 添加更多提示類型（如庫存不足、付款到期等）
2. 支持自定義提示規則
3. 添加郵件/短信通知功能
4. 支持提示資訊分類和優先級
5. 添加批量刪除功能
6. 實現自動清理舊提示資訊的機制

