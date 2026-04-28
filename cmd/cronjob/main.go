package main

import (
	"context"
	"fmt"
	"log"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

func main() {
	loadDotEnv()

	cfg := config.Load()
	if err := database.Connect(cfg); err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	log.Println("✅ Database connected successfully")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 創建調度器
	s, err := gocron.NewScheduler()
	if err != nil {
		log.Fatalf("❌ Failed to create scheduler: %v", err)
	}

	// 可以添加更多定時任務
	// 例如：每天凌晨執行數據清理任務
	dailyCleanupJob := func() {
		log.Println("🧹 Running daily cleanup job at", time.Now().Format("2006-01-02 15:04:05"))

		// 1. 清理過期的 SSOTicket (ExpireAt < Now)
		if err := database.DB.Unscoped().Where("expires_at < ?", time.Now()).Delete(&models.SSOTicket{}).Error; err != nil {
			log.Printf("❌ Failed to clean expired SSO tickets: %v", err)
		} else {
			log.Println("✅ Cleaned expired SSO tickets")
		}

		// 2. 清理過期的 PasswordResetToken (ExpireAt < Now, assuming table exists based on context, need verify)
		// Checking if PasswordResetToken model exists first. I saw it in file list.
		if err := database.DB.Unscoped().Where("expires_at < ?", time.Now()).Delete(&models.PasswordResetToken{}).Error; err != nil {
			log.Printf("❌ Failed to clean expired password reset tokens: %v", err)
		} else {
			log.Println("✅ Cleaned expired password reset tokens")
		}
	}

	// 每天凌晨2點執行清理任務
	_, err = s.NewJob(
		gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(2, 0, 0))),
		gocron.NewTask(dailyCleanupJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule daily cleanup job: %v", err)
	}
	log.Println("✅ Scheduled daily cleanup job to run at 02:00")

	// 生成提示資訊任務（例如今日有預約）
	generateNotificationsJob := func() {
		log.Println("🔔 Running notification generation job at", time.Now().Format("2006-01-02 15:04:05"))
		generateAppointmentNotifications()
		generateServiceOrderNotifications()
		generateReminderNotifications()
		generateProjectDueNotifications()
	}

	// 每天早上8點執行提示資訊生成任務
	_, err = s.NewJob(
		gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(8, 0, 0))),
		gocron.NewTask(generateNotificationsJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule notification generation job: %v", err)
	}
	log.Println("✅ Scheduled notification generation job to run at 08:00")

	// 每小時掃描一次 reminders（檢查即將到來的提醒）
	scanRemindersJob := func() {
		log.Println("⏰ Running reminder scan job at", time.Now().Format("2006-01-02 15:04:05"))
		generateReminderNotifications()
	}

	// 每小時執行一次 reminders 掃描任務
	_, err = s.NewJob(
		gocron.DurationJob(1*time.Hour),
		gocron.NewTask(scanRemindersJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule reminder scan job: %v", err)
	}
	log.Println("✅ Scheduled reminder scan job to run every hour")

	// 每小時掃描一次 projects/tasks 到期（當日）
	scanProjectsJob := func() {
		log.Println("⏰ Running project/task due scan job at", time.Now().Format("2006-01-02 15:04:05"))
		generateProjectDueNotifications()
	}
	_, err = s.NewJob(
		gocron.DurationJob(1*time.Hour),
		gocron.NewTask(scanProjectsJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule project scan job: %v", err)
	}
	log.Println("✅ Scheduled project/task due scan job to run every hour")

	// HR：打卡提示（每小時掃描一次：未打卡/未下班打卡）
	scanAttendanceJob := func() {
		log.Println("⏰ Running attendance scan job at", time.Now().Format("2006-01-02 15:04:05"))
		generateAttendanceNotifications()
	}
	_, err = s.NewJob(
		gocron.DurationJob(1*time.Hour),
		gocron.NewTask(scanAttendanceJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule attendance scan job: %v", err)
	}
	log.Println("✅ Scheduled attendance scan job to run every hour")

	// 自訂網域：Cloudflare SSL for SaaS 背景自動同步（DNS OK → 建立/同步 custom hostname）
	cloudflareSyncJob := func() {
		// Quiet log to reduce noise; only log on errors inside sync
		syncCloudflareCustomDomains()
	}
	_, err = s.NewJob(
		gocron.DurationJob(getCFDomainSyncInterval()),
		gocron.NewTask(cloudflareSyncJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Cloudflare custom domain sync job: %v", err)
	}
	log.Println("✅ Scheduled Cloudflare custom domain sync job")

	// Shopee 訂單同步（每 15 分鐘從 Shopee 拉取新訂單）
	shopeeOrderSyncJob := func() {
		syncShopeeOrdersForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(15*time.Minute),
		gocron.NewTask(shopeeOrderSyncJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Shopee order sync job: %v", err)
	}
	log.Println("✅ Scheduled Shopee order sync job to run every 15 minutes")

	// Shopee Token 刷新（每 3 小時檢查一次）
	shopeeTokenRefreshJob := func() {
		refreshShopeeTokensForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(3*time.Hour),
		gocron.NewTask(shopeeTokenRefreshJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Shopee token refresh job: %v", err)
	}
	log.Println("✅ Scheduled Shopee token refresh job to run every 3 hours")

	// Amazon 訂單同步（每 15 分鐘從 Amazon 拉取新訂單）
	amazonOrderSyncJob := func() {
		syncAmazonOrdersForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(15*time.Minute),
		gocron.NewTask(amazonOrderSyncJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Amazon order sync job: %v", err)
	}
	log.Println("✅ Scheduled Amazon order sync job to run every 15 minutes")

	// Amazon Token 刷新（每 3 小時檢查一次）
	amazonTokenRefreshJob := func() {
		refreshAmazonTokensForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(3*time.Hour),
		gocron.NewTask(amazonTokenRefreshJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Amazon token refresh job: %v", err)
	}
	log.Println("✅ Scheduled Amazon token refresh job to run every 3 hours")

	// Lazada 訂單同步（每 15 分鐘從 Lazada 拉取新訂單）
	lazadaOrderSyncJob := func() {
		syncLazadaOrdersForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(15*time.Minute),
		gocron.NewTask(lazadaOrderSyncJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Lazada order sync job: %v", err)
	}
	log.Println("✅ Scheduled Lazada order sync job to run every 15 minutes")

	// Lazada Token 刷新（每 3 小時檢查一次）
	lazadaTokenRefreshJob := func() {
		refreshLazadaTokensForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(3*time.Hour),
		gocron.NewTask(lazadaTokenRefreshJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Lazada token refresh job: %v", err)
	}
	log.Println("✅ Scheduled Lazada token refresh job to run every 3 hours")

	// Rakuten 訂單同步（每 15 分鐘從樂天拉取新訂單）
	rakutenOrderSyncJob := func() {
		syncRakutenOrdersForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(15*time.Minute),
		gocron.NewTask(rakutenOrderSyncJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Rakuten order sync job: %v", err)
	}
	log.Println("✅ Scheduled Rakuten order sync job to run every 15 minutes")

	// Rakuten Token 刷新（每 3 小時檢查一次）
	rakutenTokenRefreshJob := func() {
		refreshRakutenTokensForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(3*time.Hour),
		gocron.NewTask(rakutenTokenRefreshJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule Rakuten token refresh job: %v", err)
	}
	log.Println("✅ Scheduled Rakuten token refresh job to run every 3 hours")

	// HKTVmall 訂單同步（每 15 分鐘從 HKTVmall 拉取新訂單）
	hktvmallOrderSyncJob := func() {
		syncHKTVmallOrdersForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(15*time.Minute),
		gocron.NewTask(hktvmallOrderSyncJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule HKTVmall order sync job: %v", err)
	}
	log.Println("✅ Scheduled HKTVmall order sync job to run every 15 minutes")

	// HKTVmall 產品同步（每小時從 HKTVmall 拉取產品）
	hktvmallProductSyncJob := func() {
		syncHKTVmallProductsForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(1*time.Hour),
		gocron.NewTask(hktvmallProductSyncJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule HKTVmall product sync job: %v", err)
	}
	log.Println("✅ Scheduled HKTVmall product sync job to run every hour")

	// 外賣平台訂單同步（每 15 分鐘從 Foodpanda/Keeta/Deliveroo 拉取新訂單）
	deliveryOrderSyncJob := func() {
		syncDeliveryOrdersForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(15*time.Minute),
		gocron.NewTask(deliveryOrderSyncJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule delivery order sync job: %v", err)
	}
	log.Println("✅ Scheduled delivery order sync job to run every 15 minutes")

	// 外賣平台 Token 刷新（每 3 小時檢查一次）
	deliveryTokenRefreshJob := func() {
		refreshDeliveryTokensForAllTenants()
	}
	_, err = s.NewJob(
		gocron.DurationJob(3*time.Hour),
		gocron.NewTask(deliveryTokenRefreshJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule delivery token refresh job: %v", err)
	}
	log.Println("✅ Scheduled delivery token refresh job to run every 3 hours")

	// 自動外展（每小時掃描一次）
	autoOutreachJob := func() {
		runAutoOutreachJob()
	}
	_, err = s.NewJob(
		gocron.DurationJob(1*time.Hour),
		gocron.NewTask(autoOutreachJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule auto-outreach job: %v", err)
	}
	log.Println("✅ Scheduled auto-outreach job to run every hour")

	// SEO Sitemap：每天凌晨3點寫入 DB flag，通知 API server 重新生成 sitemap
	// API server 的 SitemapXML handler 會檢查此 flag，發現比 cache 更新就重新生成
	sitemapInvalidateJob := func() {
		log.Println("🗺️ Running daily sitemap invalidation at", time.Now().Format("2006-01-02 15:04:05"))
		desc := "Cronjob daily sitemap cache invalidation timestamp"
		if err := models.SetSystemSetting("sitemap_invalidated_at", time.Now().UTC().Format(time.RFC3339), &desc); err != nil {
			log.Printf("❌ Failed to set sitemap_invalidated_at: %v", err)
		} else {
			log.Println("✅ Sitemap invalidation flag written to DB")
		}
	}
	_, err = s.NewJob(
		gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(3, 0, 0))),
		gocron.NewTask(sitemapInvalidateJob),
	)
	if err != nil {
		log.Fatalf("❌ Failed to schedule sitemap invalidation job: %v", err)
	}
	log.Println("✅ Scheduled sitemap invalidation job to run daily at 03:00")

	// 啟動調度器
	s.Start()
	log.Println("🚀 Cronjob scheduler started. Press Ctrl+C to stop.")

	// Start email queue worker (moved from cmd/email_worker)
	go startEmailQueueWorkers(ctx, cfg)

	<-ctx.Done()
	log.Println("🛑 Cronjob shutting down...")
	_ = s.Shutdown()
	log.Println("✅ Cronjob stopped")
}

// tenantHasAnyModule 判斷租戶是否啟用任一 module_code
func tenantHasAnyModule(tenantID uuid.UUID, codes ...string) bool {
	if tenantID == uuid.Nil || len(codes) == 0 {
		return false
	}
	var cnt int64
	if err := database.DB.Model(&models.TenantModule{}).
		Where("tenant_id = ? AND is_enabled = ? AND module_code IN ?", tenantID, true, codes).
		Count(&cnt).Error; err != nil {
		return false
	}
	return cnt > 0
}

// tenantHasModule 判斷租戶是否啟用指定 module_code
func tenantHasModule(tenantID uuid.UUID, code string) bool {
	return tenantHasAnyModule(tenantID, code)
}

// getNotificationSettings 獲取租戶的通知設定（如果不存在則返回默認值：全部啟用）
func getNotificationSettings(tenantID uuid.UUID) models.NotificationSettings {
	var settings models.NotificationSettings
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&settings).Error; err != nil {
		// 如果不存在，返回默認值（全部啟用）
		return models.NotificationSettings{
			TenantID:                         tenantID,
			AttendanceNotificationsEnabled:   true,
			OrderNotificationsEnabled:        true,
			ServiceOrderNotificationsEnabled: true,
			AppointmentNotificationsEnabled:  true,
			ProjectDueNotificationsEnabled:   true,
		}
	}
	return settings
}

// generateAppointmentNotifications 生成今日有預約的提示資訊
func generateAppointmentNotifications() {
	// 獲取所有活躍的租戶
	var tenants []models.Tenant
	if err := database.DB.Where("status = ?", "active").Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to fetch tenants: %v", err)
		return
	}

	for _, tenant := range tenants {
		// 模組 gating：模板包含服務單時也要有預約提示（所以 appointments OR service_orders 都算）
		if !tenantHasAnyModule(tenant.ID, "appointments", "service_orders") {
			continue
		}

		// 檢查通知設定是否啟用
		settings := getNotificationSettings(tenant.ID)
		if !settings.AppointmentNotificationsEnabled {
			continue
		}

		today := utils.TodayInTenantTimezone(tenant.ID)

		// 查找今日的預約
		var appointments []models.Appointment
		if err := database.DB.Where("tenant_id = ? AND DATE(start_time) = ?", tenant.ID, today).
			Preload("Customer").Preload("Staff").Preload("Service").
			Find(&appointments).Error; err != nil {
			log.Printf("❌ Failed to fetch appointments for tenant %s: %v", tenant.ID, err)
			continue
		}

		if len(appointments) == 0 {
			continue
		}

		// 獲取該租戶的所有用戶
		var users []models.User
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenant.ID, "active").Find(&users).Error; err != nil {
			log.Printf("❌ Failed to fetch users for tenant %s: %v", tenant.ID, err)
			continue
		}

		// 為每個用戶生成提示資訊（如果還沒有生成過）
		for _, user := range users {
			// 生成提示資訊
			appointmentCount := len(appointments)
			title := "今日有預約"
			message := ""
			if appointmentCount == 1 {
				appt := appointments[0]
				customerName := "客戶"
				if appt.Customer.Name != "" {
					customerName = appt.Customer.Name
				}
				serviceName := "服務"
				if appt.Service != nil && appt.Service.Name != "" {
					serviceName = appt.Service.Name
				}
				startTime := appt.StartTime.Format("15:04")
				message = "您今日有 1 個預約：" + customerName + " 的 " + serviceName + "（" + startTime + "）"
			} else {
				message = "您今日有 " + strconv.Itoa(appointmentCount) + " 個預約，請查看預約管理頁面"
			}

			dedupeKey := "appointment_today:" + tenant.ID.String() + ":" + user.ID.String() + ":" + today
			alert := models.NotificationAlert{
				ID:          uuid.New(),
				TenantID:    tenant.ID,
				UserID:      user.ID,
				Type:        "appointment_today",
				Title:       title,
				Message:     message,
				Link:        "/appointments?appointment_date=" + today,
				DedupeKey:   dedupeKey,
				IsRead:      false,
				GeneratedAt: time.Now(),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			if err := database.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&alert).Error; err != nil {
				log.Printf("❌ Failed to create notification alert for user %s: %v", user.ID, err)
			} else if alert.ID != uuid.Nil {
				log.Printf("✅ Created appointment notification for user %s (tenant %s)", user.ID, tenant.ID)
			}
		}
	}
}

// generateServiceOrderNotifications 生成今日有服務單的提示資訊（服務單模組）
func generateServiceOrderNotifications() {
	var tenants []models.Tenant
	if err := database.DB.Where("status = ?", "active").Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to fetch tenants: %v", err)
		return
	}

	for _, tenant := range tenants {
		if !tenantHasModule(tenant.ID, "service_orders") {
			continue
		}

		// 檢查通知設定是否啟用
		settings := getNotificationSettings(tenant.ID)
		if !settings.ServiceOrderNotificationsEnabled {
			continue
		}

		today := utils.TodayInTenantTimezone(tenant.ID)

		var orders []models.ServiceOrder
		if err := database.DB.
			Where("tenant_id = ? AND service_date = ? AND status <> ?", tenant.ID, today, "completed").
			Preload("Customer").
			Find(&orders).Error; err != nil {
			log.Printf("❌ Failed to fetch service orders for tenant %s: %v", tenant.ID, err)
			continue
		}
		if len(orders) == 0 {
			continue
		}

		var users []models.User
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenant.ID, "active").Find(&users).Error; err != nil {
			log.Printf("❌ Failed to fetch users for tenant %s: %v", tenant.ID, err)
			continue
		}

		for _, user := range users {
			count := len(orders)
			title := "今日有服務單"
			message := ""
			if count == 1 {
				customerName := "客戶"
				if orders[0].Customer.Name != "" {
					customerName = orders[0].Customer.Name
				}
				message = "您今日有 1 筆服務單：" + customerName + "（單號 " + orders[0].OrderNumber + "）"
			} else {
				message = "您今日有 " + strconv.Itoa(count) + " 筆服務單，請查看服務單管理頁面"
			}

			key := "service_order_today:" + tenant.ID.String() + ":" + user.ID.String() + ":" + today
			alert := models.NotificationAlert{
				ID:          uuid.New(),
				TenantID:    tenant.ID,
				UserID:      user.ID,
				Type:        "service_order_today",
				Title:       title,
				Message:     message,
				Link:        "/service-orders",
				DedupeKey:   key,
				IsRead:      false,
				GeneratedAt: time.Now(),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if err := database.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&alert).Error; err != nil {
				log.Printf("❌ Failed to create service order notification for user %s: %v", user.ID, err)
			}
		}
	}
}

// generateReminderNotifications 掃描 reminders 並生成提示資訊
func generateReminderNotifications() {
	// 獲取所有活躍的租戶
	var tenants []models.Tenant
	if err := database.DB.Where("status = ?", "active").Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to fetch tenants: %v", err)
		return
	}

	now := time.Now()
	// 檢查未來1小時內到期的提醒，以及已過期但未完成的提醒（最多過期1小時）
	oneHourLater := now.Add(1 * time.Hour)
	oneHourAgo := now.Add(-1 * time.Hour)

	for _, tenant := range tenants {
		// 查找即將到來或剛過期的提醒（未完成且狀態為 active）
		var reminders []models.Reminder
		if err := database.DB.Where(
			"tenant_id = ? AND status = ? AND is_completed = ? AND remind_time >= ? AND remind_time <= ?",
			tenant.ID, "active", false, oneHourAgo, oneHourLater,
		).Find(&reminders).Error; err != nil {
			log.Printf("❌ Failed to fetch reminders for tenant %s: %v", tenant.ID, err)
			continue
		}

		if len(reminders) == 0 {
			continue
		}

		// 為每個提醒生成提示資訊
		for _, reminder := range reminders {
			// 檢查是否已經為此提醒生成過提示資訊（在提醒時間前後1小時內）
			reminderTimeStart := reminder.RemindTime.Add(-1 * time.Hour)
			reminderTimeEnd := reminder.RemindTime.Add(1 * time.Hour)

			var existingAlert models.NotificationAlert
			err := database.DB.Where(
				"tenant_id = ? AND user_id = ? AND type = ? AND generated_at >= ? AND generated_at <= ?",
				tenant.ID, reminder.UserID, "reminder", reminderTimeStart, reminderTimeEnd,
			).First(&existingAlert).Error

			if err == nil {
				// 已經生成過，跳過
				continue
			}

			// 判斷提醒狀態
			timeDiff := reminder.RemindTime.Sub(now)
			title := ""
			message := ""
			link := "/reminders"

			if timeDiff <= 0 {
				// 已過期
				title = "提醒已到期"
				message = "提醒「" + reminder.Title + "」已到期"
				if reminder.Description != "" {
					message += "：" + reminder.Description
				}
			} else if timeDiff <= 15*time.Minute {
				// 15分鐘內到期
				title = "提醒即將到期"
				minutes := int(timeDiff.Minutes())
				message = "提醒「" + reminder.Title + "」將在 " + strconv.Itoa(minutes) + " 分鐘後到期"
				if reminder.Description != "" {
					message += "：" + reminder.Description
				}
			} else {
				// 1小時內到期
				title = "提醒通知"
				minutes := int(timeDiff.Minutes())
				message = "提醒「" + reminder.Title + "」將在 " + strconv.Itoa(minutes) + " 分鐘後到期"
				if reminder.Description != "" {
					message += "：" + reminder.Description
				}
			}

			alert := models.NotificationAlert{
				ID:          uuid.New(),
				TenantID:    tenant.ID,
				UserID:      reminder.UserID,
				Type:        "reminder",
				Title:       title,
				Message:     message,
				Link:        link,
				IsRead:      false,
				GeneratedAt: now,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			if err := database.DB.Create(&alert).Error; err != nil {
				log.Printf("❌ Failed to create reminder notification alert for user %s: %v", reminder.UserID, err)
			} else {
				log.Printf("✅ Created reminder notification for user %s (tenant %s, reminder: %s)", reminder.UserID, tenant.ID, reminder.Title)
			}
		}
	}
}

// generateProjectDueNotifications：項目到期 / 任務到期（當日）生成提示資訊（不重覆）
func generateProjectDueNotifications() {
	var tenants []models.Tenant
	if err := database.DB.Where("status = ?", "active").Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to fetch tenants for project due notifications: %v", err)
		return
	}

	for _, tenant := range tenants {
		// 模組 gating：projects（歷史上曾出現 project/projects 兩種代碼）
		if !tenantHasAnyModule(tenant.ID, "projects", "project") {
			continue
		}

		// 檢查通知設定是否啟用
		settings := getNotificationSettings(tenant.ID)
		if !settings.ProjectDueNotificationsEnabled {
			continue
		}

		today := utils.TodayInTenantTimezone(tenant.ID)

		// 1) 項目到期（end_date = today）
		var projects []models.Project
		if err := database.DB.Where("tenant_id = ? AND status IN ? AND end_date IS NOT NULL AND end_date = ?", tenant.ID, []string{"active", "on_hold"}, today).
			Find(&projects).Error; err != nil {
			log.Printf("❌ Failed to fetch due projects for tenant %s: %v", tenant.ID, err)
		} else {
			for _, p := range projects {
				if p.OwnerUserID == nil {
					continue
				}
				key := "project_due_today:" + tenant.ID.String() + ":" + p.ID.String() + ":" + today + ":" + p.OwnerUserID.String()
				alert := models.NotificationAlert{
					ID:          uuid.New(),
					TenantID:    tenant.ID,
					UserID:      *p.OwnerUserID,
					Type:        "project_due_today",
					Title:       "項目今日到期",
					Message:     "項目「" + p.Name + "」今日到期，請確認是否需要調整日期或標記完成。",
					Link:        "/projects/" + p.ID.String() + "/edit",
					DedupeKey:   key,
					IsRead:      false,
					GeneratedAt: time.Now(),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				if err := database.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&alert).Error; err != nil {
					log.Printf("❌ Failed to create project due alert: %v", err)
				}
			}
		}

		// 2) 任務到期（due_date = today）
		var tasks []models.ProjectTask
		if err := database.DB.Where("tenant_id = ? AND status IN ? AND due_date IS NOT NULL AND due_date = ?", tenant.ID, []string{"todo", "in_progress", "blocked"}, today).
			Find(&tasks).Error; err != nil {
			log.Printf("❌ Failed to fetch due tasks for tenant %s: %v", tenant.ID, err)
			continue
		}
		for _, t := range tasks {
			if t.AssigneeUserID == nil {
				continue
			}
			key := "task_due_today:" + tenant.ID.String() + ":" + t.ID.String() + ":" + today + ":" + t.AssigneeUserID.String()
			alert := models.NotificationAlert{
				ID:          uuid.New(),
				TenantID:    tenant.ID,
				UserID:      *t.AssigneeUserID,
				Type:        "task_due_today",
				Title:       "任務今日到期",
				Message:     "任務「" + t.Title + "」今日到期，請確認進度。",
				Link:        "/projects/" + t.ProjectID.String() + "/edit",
				DedupeKey:   key,
				IsRead:      false,
				GeneratedAt: time.Now(),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			if err := database.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&alert).Error; err != nil {
				log.Printf("❌ Failed to create task due alert: %v", err)
			}
		}
	}
}

// generateAttendanceNotifications：打卡提示（未打卡 / 未下班打卡）
func generateAttendanceNotifications() {
	var tenants []models.Tenant
	if err := database.DB.Where("status = ?", "active").Find(&tenants).Error; err != nil {
		log.Printf("❌ Failed to fetch tenants for attendance notifications: %v", err)
		return
	}

	for _, tenant := range tenants {
		if !tenantHasModule(tenant.ID, "hr") {
			continue
		}

		// 檢查通知設定是否啟用
		settings := getNotificationSettings(tenant.ID)
		if !settings.AttendanceNotificationsEnabled {
			continue
		}

		now := utils.NowInTenantTimezone(tenant.ID)
		today := now.Format("2006-01-02")
		currentHour := now.Hour()

		var users []models.User
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenant.ID, "active").Preload("Shift").Find(&users).Error; err != nil {
			log.Printf("❌ Failed to fetch users for tenant %s: %v", tenant.ID, err)
			continue
		}

		for _, u := range users {
			// 如果用戶沒有設定 shift，使用預設時間（09:00-12:00 檢查上班，18:00-21:00 檢查下班）
			var shiftStartHour, shiftStartMinute, shiftEndHour, shiftEndMinute int
			if u.Shift != nil {
				shiftStartHour = u.Shift.StartTime.Hour()
				shiftStartMinute = u.Shift.StartTime.Minute()
				shiftEndHour = u.Shift.EndTime.Hour()
				shiftEndMinute = u.Shift.EndTime.Minute()
			} else {
				// 預設：09:00 上班，18:00 下班
				shiftStartHour = 9
				shiftStartMinute = 0
				shiftEndHour = 18
				shiftEndMinute = 0
			}

			// 計算檢查時間範圍（上班時間前後 3 小時，下班時間前後 3 小時）
			clockInCheckStart := shiftStartHour - 1
			clockInCheckEnd := shiftStartHour + 3
			clockOutCheckStart := shiftEndHour - 1
			clockOutCheckEnd := shiftEndHour + 3

			// 檢查是否在打卡檢查時間範圍內
			shouldClockInCheck := currentHour >= clockInCheckStart && currentHour <= clockInCheckEnd
			shouldClockOutCheck := currentHour >= clockOutCheckStart && currentHour <= clockOutCheckEnd

			if !shouldClockInCheck && !shouldClockOutCheck {
				continue
			}

			var att models.Attendance
			err := database.DB.Where("tenant_id = ? AND user_id = ? AND date = ?", tenant.ID, u.ID, today).First(&att).Error
			hasClockIn := false
			hasClockOut := false
			if err == nil {
				hasClockIn = att.ClockIn != nil
				hasClockOut = att.ClockOut != nil
			}

			// 上班打卡檢查：在上班時間前後 3 小時內，且尚未打卡
			if shouldClockInCheck && !hasClockIn {
				shiftName := "預設時段"
				if u.Shift != nil {
					shiftName = u.Shift.Name
				}
				key := "attendance_clock_in_missing:" + tenant.ID.String() + ":" + u.ID.String() + ":" + today
				alert := models.NotificationAlert{
					ID:          uuid.New(),
					TenantID:    tenant.ID,
					UserID:      u.ID,
					Type:        "attendance_clock_in_missing",
					Title:       "今日尚未打卡",
					Message:     "您今日尚未打卡上班（" + shiftName + " " + fmt.Sprintf("%02d:%02d", shiftStartHour, shiftStartMinute) + "），請到打卡頁完成打卡。",
					Link:        "/attendance/clock",
					DedupeKey:   key,
					IsRead:      false,
					GeneratedAt: time.Now(),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				_ = database.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&alert).Error
			}

			// 下班打卡檢查：在下班時間前後 3 小時內，且已打卡上班但尚未打卡下班
			if shouldClockOutCheck && hasClockIn && !hasClockOut {
				shiftName := "預設時段"
				if u.Shift != nil {
					shiftName = u.Shift.Name
				}
				key := "attendance_clock_out_missing:" + tenant.ID.String() + ":" + u.ID.String() + ":" + today
				alert := models.NotificationAlert{
					ID:          uuid.New(),
					TenantID:    tenant.ID,
					UserID:      u.ID,
					Type:        "attendance_clock_out_missing",
					Title:       "今日尚未下班打卡",
					Message:     "您今日尚未打卡下班（" + shiftName + " " + fmt.Sprintf("%02d:%02d", shiftEndHour, shiftEndMinute) + "），請到打卡頁完成下班打卡。",
					Link:        "/attendance/clock",
					DedupeKey:   key,
					IsRead:      false,
					GeneratedAt: time.Now(),
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				_ = database.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(&alert).Error
			}
		}
	}
}
