package handlers

import (
	"encoding/json"
	"log"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 日曆 (Calendar) CRUD
// ============================================

func GetCalendars(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var calendars []models.Calendar
	query := database.DB.Where("tenant_id = ?", tenantID)

	if userID != uuid.Nil {
		query = query.Where("user_id = ?", userID)
	}

	if startDate := c.Query("start_date"); startDate != "" {
		query = query.Where("start_time >= ?", startDate)
	}
	if endDate := c.Query("end_date"); endDate != "" {
		startDate := c.Query("start_date")
		// 查詢結束日期當天及之前的所有事件
		if startDate != "" {
			query = query.Where("(start_time <= ? OR (end_time IS NOT NULL AND end_time >= ?))", endDate, startDate)
		} else {
			query = query.Where("start_time <= ?", endDate)
		}
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Calendar{}).Count(&total)

	if err := query.Preload("User").Offset(offset).Limit(limit).Order("start_time ASC").Find(&calendars).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 額外：加入預約與請假事件
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	// 預約
	var appts []models.Appointment
	apptQuery := database.DB.Where("tenant_id = ?", tenantID)
	if startDate != "" && endDate != "" {
		apptQuery = apptQuery.Where("(start_time BETWEEN ? AND ?) OR (end_time BETWEEN ? AND ?)", startDate, endDate, startDate, endDate)
	}
	apptQuery.Preload("Customer").Preload("Rooms").Preload("Equipments").Find(&appts)
	for _, a := range appts {
		title := "預約"
		if a.Customer.Name != "" {
			title = a.Customer.Name + " 預約"
		}
		endTime := a.EndTime
		calendars = append(calendars, models.Calendar{
			ID:        uuid.New(),
			TenantID:  tenantID,
			Title:     title,
			StartTime: a.StartTime,
			EndTime:   &endTime,
			UserID:    a.CustomerID, // optional
			ExtraFields: models.JSONB{
				"appointment_id": a.ID.String(),
			},
		})
	}

	// 請假
	var leaves []models.LeaveRequest
	leaveQuery := database.DB.Where("tenant_id = ?", tenantID)
	if startDate != "" && endDate != "" {
		leaveQuery = leaveQuery.Where("(start_date BETWEEN ? AND ?) OR (end_date BETWEEN ? AND ?)", startDate, endDate, startDate, endDate)
	}
	leaveQuery.Preload("User").Find(&leaves)
	for _, l := range leaves {
		endDate := l.EndDate
		calendars = append(calendars, models.Calendar{
			ID:        uuid.New(),
			TenantID:  tenantID,
			Title:     "請假-" + l.User.Name,
			StartTime: l.StartDate,
			EndTime:   &endDate,
			AllDay:    true,
			UserID:    l.UserID,
			ExtraFields: models.JSONB{
				"leave_request_id": l.ID.String(),
			},
		})
	}

	// 假期
	var holidays []models.Holiday
	holidayQuery := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active")
	if startDate != "" && endDate != "" {
		// 查詢在日期範圍內的假期，或每年重複的假期
		holidayQuery = holidayQuery.Where("(start_date <= ? AND end_date >= ?) OR (is_recurring = true)", endDate, startDate)
	}
	holidayQuery.Find(&holidays)
	for _, h := range holidays {
		// 如果是每年重複的假期，需要為查詢範圍內的每一年生成事件
		if h.IsRecurring {
			// 解析查詢的開始和結束年份
			var queryStartYear, queryEndYear int
			if startDate != "" {
				if startTime, err := utils.ParseDateInTenantTimezone(tenantID, startDate); err == nil {
					queryStartYear = startTime.Year()
				}
			} else {
				queryStartYear = utils.NowInTenantTimezone(tenantID).Year()
			}
			if endDate != "" {
				if endTime, err := utils.ParseDateInTenantTimezone(tenantID, endDate); err == nil {
					queryEndYear = endTime.Year()
				} else {
					queryEndYear = queryStartYear
				}
			} else {
				queryEndYear = queryStartYear
			}

			// 為每一年生成假期事件
			for year := queryStartYear; year <= queryEndYear; year++ {
				// 計算當年的開始和結束日期
				originalStart := h.StartDate
				originalEnd := h.EndDate
				startMonth := int(originalStart.Month())
				startDay := originalStart.Day()
				endMonth := int(originalEnd.Month())
				endDay := originalEnd.Day()

				yearlyStart := time.Date(year, time.Month(startMonth), startDay, 0, 0, 0, 0, time.UTC)
				yearlyEnd := time.Date(year, time.Month(endMonth), endDay, 0, 0, 0, 0, time.UTC)

				// 檢查是否在查詢範圍內
				if startDate != "" && endDate != "" {
					queryStart, _ := utils.ParseDateInTenantTimezone(tenantID, startDate)
					queryEnd, _ := utils.ParseDateInTenantTimezone(tenantID, endDate)
					if yearlyEnd.Before(queryStart) || yearlyStart.After(queryEnd) {
						continue
					}
				}

				yearlyEndPtr := yearlyEnd
				calendars = append(calendars, models.Calendar{
					ID:        uuid.New(),
					TenantID:  tenantID,
					Title:     "假期-" + h.Name,
					StartTime: yearlyStart,
					EndTime:   &yearlyEndPtr,
					AllDay:    true,
					EventType: "holiday",
					ExtraFields: models.JSONB{
						"holiday_id": h.ID.String(),
					},
				})
			}
		} else {
			// 非重複假期，直接添加
			endDate := h.EndDate
			calendars = append(calendars, models.Calendar{
				ID:        uuid.New(),
				TenantID:  tenantID,
				Title:     "假期-" + h.Name,
				StartTime: h.StartDate,
				EndTime:   &endDate,
				AllDay:    true,
				EventType: "holiday",
				ExtraFields: models.JSONB{
					"holiday_id": h.ID.String(),
				},
			})
		}
	}

	// 專案任務（Project Tasks）
	type projectTaskRow struct {
		ID             uuid.UUID  `gorm:"column:id"`
		ProjectID      uuid.UUID  `gorm:"column:project_id"`
		Title          string     `gorm:"column:title"`
		DueDate        *time.Time `gorm:"column:due_date"`
		AssigneeUserID *uuid.UUID `gorm:"column:assignee_user_id"`
		ProjectName    string     `gorm:"column:project_name"`
	}

	var projectTasks []projectTaskRow
	projectTaskQuery := database.DB.
		Table("project_tasks").
		Select("project_tasks.id, project_tasks.project_id, project_tasks.title, project_tasks.due_date, project_tasks.assignee_user_id, projects.name AS project_name").
		Joins("JOIN projects ON projects.id = project_tasks.project_id AND projects.tenant_id = project_tasks.tenant_id").
		Where("project_tasks.tenant_id = ?", tenantID).
		Where("project_tasks.due_date IS NOT NULL")

	if startDate != "" && endDate != "" {
		projectTaskQuery = projectTaskQuery.Where("project_tasks.due_date BETWEEN ? AND ?", startDate, endDate)
	}
	projectTaskQuery.Order("project_tasks.due_date ASC").Find(&projectTasks)

	for _, t := range projectTasks {
		if t.DueDate == nil {
			continue
		}
		title := "專案任務"
		if t.ProjectName != "" {
			title = "專案任務-" + t.ProjectName
		}
		if t.Title != "" {
			title = title + ": " + t.Title
		}

		eventUserID := userID
		if t.AssigneeUserID != nil && *t.AssigneeUserID != uuid.Nil {
			eventUserID = *t.AssigneeUserID
		}

		calendars = append(calendars, models.Calendar{
			ID:        uuid.New(),
			TenantID:  tenantID,
			UserID:    eventUserID,
			Title:     title,
			StartTime: *t.DueDate,
			AllDay:    true,
			EventType: "project_task",
			ExtraFields: models.JSONB{
				"project_task_id": t.ID.String(),
				"project_id":      t.ProjectID.String(),
			},
		})
	}

	return c.JSON(fiber.Map{
		"data":  calendars,
		"total": total + int64(len(appts)) + int64(len(leaves)) + int64(len(holidays)) + int64(len(projectTasks)),
		"page":  page,
		"limit": limit,
	})
}

func GetCalendar(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var calendar models.Calendar

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("User").First(&calendar).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Calendar not found"})
	}

	return c.JSON(calendar)
}

func CreateCalendar(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var calendar models.Calendar
	if err := c.BodyParser(&calendar); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	calendar.TenantID = tenantID
	if calendar.UserID == uuid.Nil {
		calendar.UserID = userID
	}

	if err := database.DB.Create(&calendar).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(calendar)
}

func UpdateCalendar(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var calendar models.Calendar

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&calendar).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Calendar not found"})
	}

	if err := c.BodyParser(&calendar); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	calendar.TenantID = tenantID // 確保不能修改 tenant_id

	if err := database.DB.Save(&calendar).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(calendar)
}

func DeleteCalendar(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Calendar{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Calendar deleted"})
}

// ============================================
// 提醒 (Reminder) CRUD
// ============================================

func GetReminders(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var reminders []models.Reminder
	query := database.DB.Where("tenant_id = ?", tenantID)

	if userID != uuid.Nil {
		query = query.Where("user_id = ?", userID)
	}

	if isCompleted := c.Query("is_completed"); isCompleted != "" {
		if isCompleted == "true" {
			query = query.Where("is_completed = ?", true)
		} else {
			query = query.Where("is_completed = ?", false)
		}
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Reminder{}).Count(&total)

	if err := query.Preload("User").Offset(offset).Limit(limit).Order("remind_time ASC").Find(&reminders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  reminders,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetReminder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var reminder models.Reminder

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("User").First(&reminder).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Reminder not found"})
	}

	return c.JSON(reminder)
}

func CreateReminder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var reminder models.Reminder
	if err := c.BodyParser(&reminder); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	reminder.TenantID = tenantID
	if reminder.UserID == uuid.Nil {
		reminder.UserID = userID
	}

	if err := database.DB.Create(&reminder).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(reminder)
}

func UpdateReminder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var reminder models.Reminder

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&reminder).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Reminder not found"})
	}

	if err := c.BodyParser(&reminder); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	reminder.TenantID = tenantID

	if err := database.DB.Save(&reminder).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(reminder)
}

func CompleteReminder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var reminder models.Reminder

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&reminder).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Reminder not found"})
	}

	now := time.Now()
	reminder.IsCompleted = true
	reminder.CompletedAt = &now

	if err := database.DB.Save(&reminder).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(reminder)
}

func DeleteReminder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Reminder{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Reminder deleted"})
}

// ============================================
// 訊息 (Message) CRUD
// ============================================

func GetMessages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var messages []models.Message
	query := database.DB.Where("tenant_id = ?", tenantID)

	if userID != uuid.Nil {
		query = query.Where("to_user_id = ?", userID)
	}

	if isRead := c.Query("is_read"); isRead != "" {
		if isRead == "true" {
			query = query.Where("is_read = ?", true)
		} else {
			query = query.Where("is_read = ?", false)
		}
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Message{}).Count(&total)

	if err := query.Preload("FromUser").Preload("ToUser").Offset(offset).Limit(limit).Order("created_at DESC").Find(&messages).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  messages,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetUnreadMessageCount 獲取未讀消息數
func GetUnreadMessageCount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.JSON(fiber.Map{"count": 0})
	}

	var count int64
	// 個人未讀（to_user_id = me）
	if err := database.DB.Model(&models.Message{}).
		Where("tenant_id = ? AND to_user_id = ? AND is_read = ?", tenantID, userID, false).
		Count(&count).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 公開網站訪客未讀（shared inbox）：只計算「訪客發來」的未讀（from_user_id IS NULL）
	var publicUnread int64
	if err := database.DB.Model(&models.Message{}).
		Where("tenant_id = ? AND status = ? AND is_read = ?", tenantID, "active", false).
		Where("(message_type = ? OR extra_fields->>'public_chat' = 'true')", "public_chat").
		Where("from_user_id IS NULL").
		Count(&publicUnread).Error; err == nil {
		count += publicUnread
	}

	return c.JSON(fiber.Map{"count": count})
}

// GetConversations 獲取對話列表（與當前用戶有對話的所有用戶和客戶）
func GetConversations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// 獲取所有與當前用戶有對話的用戶和客戶
	type ConversationResult struct {
		OtherUserID      *uuid.UUID `json:"other_user_id,omitempty"`     // 如果是與用戶對話
		OtherCustomerID  *uuid.UUID `json:"other_customer_id,omitempty"` // 如果是與客戶對話
		VisitorID        string     `json:"visitor_id,omitempty"`        // 如果是與網站訪客對話
		OtherUserName    string     `json:"other_user_name"`
		ConversationType string     `json:"conversation_type"` // "user", "customer", "ai", 或 "visitor"
		LastMessage      string     `json:"last_message"`
		LastMessageTime  time.Time  `json:"last_message_time"`
		UnreadCount      int64      `json:"unread_count"`
	}

	// 查詢所有與當前用戶有對話的消息（包括用戶、客戶、AI、以及 public chat）
	var allMessages []models.Message

	// 先嘗試查詢包含 to_customer_id 的條件
	// 包括：1) 用戶間對話 2) 用戶與客戶對話（全租戶共享） 3) AI 對話 4) 訪客對話（全租戶共享）
	// AI 對話：message_type = 'ai_chat' 或 extra_fields->>'ai_chat' = 'true'，且 from_user_id = userID（所有 AI 對話消息都是當前用戶發起的）
	// 客戶對話：所有 to_customer_id 不為空的消息，全租戶用戶都可見
	// 訪客對話：message_type = 'public_chat'，全租戶用戶都可見
	// 注意：需要先檢查是否是 AI 消息，如果是 AI 消息則只需要 from_user_id = userID；如果是普通消息則需要 from_user_id = userID OR to_user_id = userID
	query := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").
		Where(
			// 1) AI：只看自己發起的 AI 對話
			"((message_type = ? OR extra_fields->>'ai_chat' = 'true') AND from_user_id = ?) OR "+
				// 2) Public chat：tenant 共享收件匣（所有登入使用者都看得到）
				"(message_type = ? OR extra_fields->>'public_chat' = 'true') OR "+
				// 3) 客戶對話：所有 to_customer_id 不為空的消息，全租戶用戶都可見
				"(to_customer_id IS NOT NULL) OR "+
				// 4) 一般訊息：自己是 sender 或 receiver
				"((message_type != ? AND (extra_fields->>'ai_chat')::text != 'true' AND (extra_fields->>'public_chat')::text != 'true' AND to_customer_id IS NULL) AND (from_user_id = ? OR to_user_id = ?))",
			"ai_chat", userID, "public_chat", "ai_chat", userID, userID,
		)

	// 嘗試預載入所有關聯
	err := query.Preload("FromUser").Preload("ToUser").Preload("ToCustomer").
		Order("created_at DESC").
		Find(&allMessages).Error

	// 如果預載入 ToCustomer 失敗（可能是列不存在），嘗試不預載入 ToCustomer
	if err != nil {
		err = database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").
			Where(
				"((message_type = ? OR extra_fields->>'ai_chat' = 'true') AND from_user_id = ?) OR "+
					"(message_type = ? OR extra_fields->>'public_chat' = 'true') OR "+
					"(to_customer_id IS NOT NULL) OR "+
					"((message_type != ? AND (extra_fields->>'ai_chat')::text != 'true' AND (extra_fields->>'public_chat')::text != 'true' AND to_customer_id IS NULL) AND (from_user_id = ? OR to_user_id = ?))",
				"ai_chat", userID, "public_chat", "ai_chat", userID, userID,
			).
			Preload("FromUser").Preload("ToUser").
			Order("created_at DESC").
			Find(&allMessages).Error

		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		// 手動載入 ToCustomer（如果列存在）
		for i := range allMessages {
			if allMessages[i].ToCustomerID != nil {
				var customer models.Customer
				if err := database.DB.First(&customer, "id = ?", allMessages[i].ToCustomerID).Error; err == nil {
					allMessages[i].ToCustomer = &customer
				}
			}
		}
	}

	// 構建對話映射（使用組合鍵：type + id/string）
	conversationMap := make(map[string]*ConversationResult)

	for _, msg := range allMessages {
		var key string
		var otherUserName string
		var visitorID string

		// 檢查是否為 AI 對話
		isAIChat := false
		if msg.MessageType == "ai_chat" {
			isAIChat = true
		} else if msg.ExtraFields != nil {
			extraFieldsMap := map[string]interface{}(msg.ExtraFields)
			if aiChatVal, ok := extraFieldsMap["ai_chat"]; ok {
				if aiChatBool, ok := aiChatVal.(bool); ok && aiChatBool {
					isAIChat = true
				} else if aiChatStr, ok := aiChatVal.(string); ok && aiChatStr == "true" {
					isAIChat = true
				}
			}
		}

		// 檢查是否為 Public chat（網站訪客）
		isPublicChat := false
		if msg.MessageType == "public_chat" {
			isPublicChat = true
		} else if msg.ExtraFields != nil {
			extraFieldsMap := map[string]interface{}(msg.ExtraFields)
			if v, ok := extraFieldsMap["public_chat"]; ok {
				if vb, ok := v.(bool); ok && vb {
					isPublicChat = true
				} else if vs, ok := v.(string); ok && strings.ToLower(strings.TrimSpace(vs)) == "true" {
					isPublicChat = true
				}
			}
		}

		if isAIChat {
			// AI 對話：使用固定的 key
			key = "ai"
			otherUserName = "AI 助手"
		} else if isPublicChat {
			// Public chat：用 visitor_id 分組
			if msg.ExtraFields != nil {
				extraFieldsMap := map[string]interface{}(msg.ExtraFields)
				if v, ok := extraFieldsMap["visitor_id"]; ok {
					if vs, ok := v.(string); ok {
						visitorID = strings.TrimSpace(vs)
					}
				}
				if v, ok := extraFieldsMap["visitor_name"]; ok {
					if vs, ok := v.(string); ok {
						otherUserName = strings.TrimSpace(vs)
					}
				}
			}
			if visitorID == "" {
				// 沒有 visitor_id 無法分組，跳過
				continue
			}
			if otherUserName == "" {
				// fallback：匿名訪客 + 尾碼
				suffix := visitorID
				if len(suffix) > 6 {
					suffix = suffix[len(suffix)-6:]
				}
				otherUserName = "訪客 " + suffix
			}
			key = "visitor:" + visitorID
		} else if msg.ToCustomerID != nil {
			// 客戶對話：全租戶共享，無論當前用戶是否為發送者或接收者
			key = "customer:" + msg.ToCustomerID.String()
			if msg.ToCustomer != nil && msg.ToCustomer.Name != "" {
				otherUserName = msg.ToCustomer.Name
			} else {
				otherUserName = "未知客戶"
			}
		} else if msg.FromUserID != nil && *msg.FromUserID == userID {
			// 當前用戶是發送者
			if msg.ToUserID != nil {
				// 發送給用戶
				key = "user:" + msg.ToUserID.String()
				if msg.ToUser != nil && msg.ToUser.Name != "" {
					otherUserName = msg.ToUser.Name
				} else {
					otherUserName = "未知用戶"
				}
			} else {
				continue
			}
		} else {
			// 當前用戶是接收者
			if msg.ToUserID != nil && *msg.ToUserID == userID {
				// 從用戶接收
				if msg.FromUserID != nil {
					key = "user:" + msg.FromUserID.String()
					if msg.FromUser != nil && msg.FromUser.Name != "" {
						otherUserName = msg.FromUser.Name
					} else {
						otherUserName = "未知用戶"
					}
				} else {
					continue
				}
			} else {
				continue
			}
		}

		if conv, exists := conversationMap[key]; exists {
			// 如果這個對話有更新的消息，更新它
			if msg.CreatedAt.After(conv.LastMessageTime) {
				conv.LastMessage = msg.Content
				conv.LastMessageTime = msg.CreatedAt
			}
			// 累加未讀消息數：
			// - AI：不計
			// - visitor：只計算訪客發來（from_user_id IS NULL）的未讀
			// - user/customer：原邏輯
			if strings.HasPrefix(key, "visitor:") {
				if msg.FromUserID == nil && !msg.IsRead {
					conv.UnreadCount++
				}
			} else if key != "ai" && ((msg.ToUserID != nil && *msg.ToUserID == userID) || (msg.ToCustomerID != nil)) && !msg.IsRead {
				conv.UnreadCount++
			}
		} else {
			unreadCount := int64(0)
			if strings.HasPrefix(key, "visitor:") {
				if msg.FromUserID == nil && !msg.IsRead {
					unreadCount = 1
				}
			} else if key != "ai" && ((msg.ToUserID != nil && *msg.ToUserID == userID) || (msg.ToCustomerID != nil)) && !msg.IsRead {
				// AI 對話不計算未讀
				unreadCount = 1
			}
			conv := &ConversationResult{
				OtherUserName:    otherUserName,
				ConversationType: "",
				LastMessage:      msg.Content,
				LastMessageTime:  msg.CreatedAt,
				UnreadCount:      unreadCount,
			}
			if key == "ai" {
				conv.ConversationType = "ai"
			} else if strings.HasPrefix(key, "visitor:") {
				conv.ConversationType = "visitor"
				conv.VisitorID = visitorID
			} else if strings.HasPrefix(key, "user:") {
				conv.ConversationType = "user"
				if id, err := uuid.Parse(strings.TrimPrefix(key, "user:")); err == nil {
					conv.OtherUserID = &id
				}
			} else if strings.HasPrefix(key, "customer:") {
				conv.ConversationType = "customer"
				if id, err := uuid.Parse(strings.TrimPrefix(key, "customer:")); err == nil {
					conv.OtherCustomerID = &id
				}
			}
			conversationMap[key] = conv
		}
	}

	// 轉換為切片並排序
	var conversations []ConversationResult
	for _, conv := range conversationMap {
		conversations = append(conversations, *conv)
	}

	// 按最後消息時間排序
	for i := 0; i < len(conversations)-1; i++ {
		for j := i + 1; j < len(conversations); j++ {
			if conversations[i].LastMessageTime.Before(conversations[j].LastMessageTime) {
				conversations[i], conversations[j] = conversations[j], conversations[i]
			}
		}
	}

	return c.JSON(fiber.Map{"data": conversations})
}

// GetConversationMessages 獲取與特定用戶或客戶的對話消息
func GetConversationMessages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	otherUserIDStr := c.Query("other_user_id")
	otherCustomerIDStr := c.Query("other_customer_id")
	visitorID := strings.TrimSpace(c.Query("visitor_id"))

	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var messages []models.Message
	query := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active")

	// 檢查是否為 AI 聊天查詢
	aiChat := c.Query("ai_chat") == "true"
	conversationID := c.Query("conversation_id")

	if aiChat {
		// AI 聊天：查詢 AI 消息
		query = query.Where("(message_type = ? OR extra_fields->>'ai_chat' = 'true')", "ai_chat").
			Where("from_user_id = ?", userID) // 只查詢當前用戶的 AI 對話

		// 如果指定了 conversation_id，只查詢該對話的消息
		if conversationID != "" {
			convUUID, err := uuid.Parse(conversationID)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid conversation_id"})
			}
			query = query.Where("conversation_id = ?", convUUID)
		}
		// 未指定 conversation_id 時，查詢所有 AI 對話消息（包括有/無 conversation_id 的）
	} else if otherUserIDStr != "" {
		// 與用戶的對話
		otherUserID, err := uuid.Parse(otherUserIDStr)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid other_user_id"})
		}
		query = query.Where("((from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?))",
			userID, otherUserID, otherUserID, userID)
	} else if otherCustomerIDStr != "" {
		// 與客戶的對話
		otherCustomerID, err := uuid.Parse(otherCustomerIDStr)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid other_customer_id"})
		}
		query = query.Where("((from_user_id = ? AND to_customer_id = ?) OR (to_customer_id = ?))",
			userID, otherCustomerID, otherCustomerID)
	} else if visitorID != "" {
		// 與網站訪客的對話（public_chat）
		query = query.
			Where("(message_type = ? OR extra_fields->>'public_chat' = 'true')", "public_chat").
			Where("extra_fields->>'visitor_id' = ?", visitorID)
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "other_user_id, other_customer_id, visitor_id, or ai_chat=true is required"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Message{}).Count(&total)

	if err := query.Preload("FromUser").Preload("ToUser").Preload("ToCustomer").
		Offset(offset).Limit(limit).
		Order("created_at ASC").Find(&messages).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 標記消息為已讀（如果當前用戶是接收者）
	for i := range messages {
		// visitor 對話：只標記「訪客發來」為已讀（from_user_id IS NULL）
		if visitorID != "" {
			if messages[i].FromUserID == nil && !messages[i].IsRead {
				now := time.Now()
				messages[i].IsRead = true
				messages[i].ReadAt = &now
				database.DB.Save(&messages[i])
			}
			continue
		}
		if ((messages[i].ToUserID != nil && *messages[i].ToUserID == userID) || messages[i].ToCustomerID != nil) && !messages[i].IsRead {
			now := time.Now()
			messages[i].IsRead = true
			messages[i].ReadAt = &now
			database.DB.Save(&messages[i])
		}
	}

	return c.JSON(fiber.Map{
		"data":  messages,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetMessage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var message models.Message

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("FromUser").Preload("ToUser").Preload("ToCustomer").First(&message).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Message not found"})
	}

	return c.JSON(message)
}

func CreateMessage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	fromUserID := middleware.GetUserID(c)

	var message models.Message
	if err := c.BodyParser(&message); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	message.TenantID = tenantID
	if message.FromUserID == nil {
		message.FromUserID = &fromUserID
	}

	// AI 消息不需要 to_user_id 或 to_customer_id
	isAIChat := message.MessageType == "ai_chat" ||
		(message.ExtraFields != nil &&
			map[string]interface{}(message.ExtraFields)["ai_chat"] == true)

	// Public chat（網站訪客）不需要 to_user_id/to_customer_id，但必須有 visitor_id
	isPublicChat := message.MessageType == "public_chat"
	if !isPublicChat && message.ExtraFields != nil {
		if v, ok := map[string]interface{}(message.ExtraFields)["public_chat"]; ok {
			if vb, ok := v.(bool); ok && vb {
				isPublicChat = true
			} else if vs, ok := v.(string); ok && strings.ToLower(strings.TrimSpace(vs)) == "true" {
				isPublicChat = true
			}
		}
	}
	if isPublicChat {
		if message.ExtraFields == nil {
			message.ExtraFields = models.JSONB{}
		}
		if message.MessageType == "" || message.MessageType == "normal" {
			message.MessageType = "public_chat"
		}
		// ensure visitor_id exists
		vid := ""
		if v, ok := map[string]interface{}(message.ExtraFields)["visitor_id"]; ok {
			if vs, ok := v.(string); ok {
				vid = strings.TrimSpace(vs)
			}
		}
		if vid == "" {
			return c.Status(400).JSON(fiber.Map{"error": "visitor_id is required for public_chat"})
		}
		// public chat 不應填收件人
		message.ToUserID = nil
		message.ToCustomerID = nil
	}

	if !isAIChat && !isPublicChat {
		// 驗證：to_user_id 和 to_customer_id 至少有一個不為空
		if message.ToUserID == nil && message.ToCustomerID == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Either to_user_id or to_customer_id must be provided"})
		}

		// 驗證：不能同時指定 to_user_id 和 to_customer_id
		if message.ToUserID != nil && message.ToCustomerID != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Cannot specify both to_user_id and to_customer_id"})
		}
	}

	// 對於 AI 消息和 public chat，確保不包含 to_customer_id 和 to_user_id（如果數據庫中沒有該列）
	if isAIChat || isPublicChat {
		// 清除 to_user_id 和 to_customer_id，因為 AI 消息和 public chat 不需要這些字段
		message.ToUserID = nil
		message.ToCustomerID = nil
		// 使用 Omit 明確排除這些字段，避免 GORM 嘗試插入不存在的列
		if err := database.DB.Omit("ToCustomerID", "ToCustomer", "ToUserID", "ToUser", "FromUser", "Tenant").Create(&message).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		// 更新對話的 updated_at
		if isAIChat && message.ConversationID != nil {
			database.DB.Model(&models.AiConversation{}).
				Where("id = ?", *message.ConversationID).
				Update("updated_at", time.Now())
		}
	} else {
		if err := database.DB.Create(&message).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 重新載入關聯數據（嘗試預載入，如果列不存在則忽略錯誤）
	reloadQuery := database.DB.Preload("FromUser")
	// 只在非 AI 消息時嘗試預載入 ToUser 和 ToCustomer
	if !isAIChat {
		reloadQuery = reloadQuery.Preload("ToUser")
		// 嘗試預載入 ToCustomer，如果列不存在會失敗，但不影響結果
		if message.ToCustomerID != nil {
			reloadQuery = reloadQuery.Preload("ToCustomer")
		}
	}
	if err := reloadQuery.First(&message, message.ID).Error; err != nil {
		// 如果預載入失敗，嘗試不預載入 ToCustomer
		if err := database.DB.Preload("FromUser").Preload("ToUser").First(&message, message.ID).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to reload message"})
		}
	}

	return c.Status(201).JSON(message)
}

func MarkMessageRead(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var message models.Message

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&message).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Message not found"})
	}

	now := time.Now()
	message.IsRead = true
	message.ReadAt = &now

	if err := database.DB.Save(&message).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(message)
}

// UpdateMessage updates a message's content and/or extra_fields
// PATCH /api/v1/messages/:id
func UpdateMessage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var existing models.Message
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&existing).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Message not found"})
	}

	// Only the sender can update the message
	if existing.FromUserID != nil && *existing.FromUserID != userID {
		return c.Status(403).JSON(fiber.Map{"error": "Cannot update messages from other users"})
	}

	var body struct {
		Content     *string      `json:"content"`
		ExtraFields models.JSONB `json:"extra_fields"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	updates := map[string]interface{}{}
	if body.Content != nil {
		updates["content"] = *body.Content
	}
	if body.ExtraFields != nil {
		updates["extra_fields"] = body.ExtraFields
	}

	if len(updates) == 0 {
		return c.JSON(existing)
	}

	if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(existing)
}

func DeleteMessage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	// Load the message first so we can clean up cross-tool references
	var msg models.Message
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&msg).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Message not found"})
	}

	// Clean up related ai_sketch_generations if message has image_urls
	go cleanupSketchGenerationsForMessage(tenantID, userID, msg)

	// Clean up related ai_documents if message has doc_info
	go cleanupDocumentForMessage(tenantID, userID, msg)

	// Delete the message
	if err := database.DB.Delete(&msg).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Message deleted"})
}

// cleanupSketchGenerationsForMessage removes ai_sketch_generations records
// whose result_image matches one of the message's extra_fields.image_urls.
// This runs in a goroutine to avoid blocking the delete response.
func cleanupSketchGenerationsForMessage(tenantID, userID uuid.UUID, msg models.Message) {
	if msg.ExtraFields == nil {
		return
	}

	extra := map[string]interface{}(msg.ExtraFields)

	// Check for image_urls
	imageURLsRaw, ok := extra["image_urls"]
	if !ok || imageURLsRaw == nil {
		return
	}

	// Convert to []string
	var imageURLs []string
	rawBytes, err := json.Marshal(imageURLsRaw)
	if err != nil {
		log.Printf("[DeleteMessage] Failed to marshal image_urls: %v", err)
		return
	}
	if err := json.Unmarshal(rawBytes, &imageURLs); err != nil {
		log.Printf("[DeleteMessage] Failed to unmarshal image_urls: %v", err)
		return
	}

	if len(imageURLs) == 0 {
		return
	}

	// Delete matching ai_sketch_generations (source='chat')
	result := database.DB.Where(
		"tenant_id = ? AND user_id = ? AND source = ? AND result_image IN ?",
		tenantID, userID, "chat", imageURLs,
	).Delete(&models.AiSketchGeneration{})

	if result.Error != nil {
		log.Printf("[DeleteMessage] Failed to cleanup sketch generations: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("[DeleteMessage] Cleaned up %d sketch generation(s) for deleted message %s", result.RowsAffected, msg.ID.String())
	}
}

// cleanupDocumentForMessage deletes the ai_documents record referenced by
// the message's extra_fields.doc_info.id, including the file on disk.
// This runs in a goroutine to avoid blocking the delete response.
func cleanupDocumentForMessage(tenantID, userID uuid.UUID, msg models.Message) {
	if msg.ExtraFields == nil {
		return
	}

	extra := map[string]interface{}(msg.ExtraFields)

	docInfoRaw, ok := extra["doc_info"]
	if !ok || docInfoRaw == nil {
		return
	}

	docInfo, ok := docInfoRaw.(map[string]interface{})
	if !ok {
		return
	}

	docID, ok := docInfo["id"].(string)
	if !ok || docID == "" {
		return
	}

	// Find and delete the document
	var doc models.AiDocument
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, docID).
		First(&doc).Error; err != nil {
		log.Printf("[DeleteMessage] Document %s not found for cleanup (may already be deleted): %v", docID, err)
		return
	}

	// Delete file from disk if exists
	if doc.FilePath != "" {
		if err := os.Remove(doc.FilePath); err != nil && !os.IsNotExist(err) {
			log.Printf("[DeleteMessage] Failed to delete document file %s: %v", doc.FilePath, err)
		}
	}

	if err := database.DB.Delete(&doc).Error; err != nil {
		log.Printf("[DeleteMessage] Failed to cleanup document %s: %v", docID, err)
	} else {
		log.Printf("[DeleteMessage] Cleaned up document %s for deleted message %s", docID, msg.ID.String())
	}
}

// ============================================
// AI 對話會話 (AiConversation) CRUD
// ============================================

// GetAiConversations 取得當前用戶的所有 AI 對話會話
func GetAiConversations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var conversations []models.AiConversation
	if err := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("updated_at DESC").
		Find(&conversations).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 為每個對話計算消息數量和最後一條消息的預覽
	type ConversationItem struct {
		models.AiConversation
		MessageCount int    `json:"message_count"`
		LastMessage  string `json:"last_message"`
	}

	items := make([]ConversationItem, 0, len(conversations))
	for _, conv := range conversations {
		item := ConversationItem{AiConversation: conv}

		// 計算消息數
		var count int64
		database.DB.Model(&models.Message{}).
			Where("conversation_id = ? AND status = ?", conv.ID, "active").
			Count(&count)
		item.MessageCount = int(count)

		// 最後一條消息預覽
		var lastMsg models.Message
		if err := database.DB.Where("conversation_id = ? AND status = ?", conv.ID, "active").
			Order("created_at DESC").First(&lastMsg).Error; err == nil {
			preview := lastMsg.Content
			if len([]rune(preview)) > 50 {
				preview = string([]rune(preview)[:50]) + "..."
			}
			item.LastMessage = preview
		}

		items = append(items, item)
	}

	return c.JSON(fiber.Map{"data": items})
}

// CreateAiConversation 建立新的 AI 對話會話
func CreateAiConversation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&body); err != nil {
		body.Title = ""
	}
	if body.Title == "" {
		body.Title = "新對話"
	}

	conv := models.AiConversation{
		TenantID: tenantID,
		UserID:   userID,
		Title:    body.Title,
	}

	if err := database.DB.Create(&conv).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(conv)
}

// UpdateAiConversation 更新 AI 對話會話（重命名）
func UpdateAiConversation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var conv models.AiConversation
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&conv).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Conversation not found"})
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if body.Title != "" {
		conv.Title = body.Title
	}

	if err := database.DB.Save(&conv).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(conv)
}

// DeleteAiConversation 刪除 AI 對話會話及其所有消息
func DeleteAiConversation(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var conv models.AiConversation
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&conv).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Conversation not found"})
	}

	// 刪除該對話下的所有消息
	database.DB.Where("conversation_id = ?", conv.ID).Delete(&models.Message{})

	// 刪除對話本身
	if err := database.DB.Delete(&conv).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Conversation deleted"})
}

// GetAiConversationMessages 取得特定 AI 對話的消息
func GetAiConversationMessages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// 驗證對話屬於當前用戶
	var conv models.AiConversation
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&conv).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Conversation not found"})
	}

	var messages []models.Message
	if err := database.DB.Where("conversation_id = ? AND status = ?", conv.ID, "active").
		Order("created_at ASC").
		Find(&messages).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":         messages,
		"conversation": conv,
	})
}

// SearchAiConversations searches conversations by title and message content
func SearchAiConversations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.JSON(fiber.Map{"data": []interface{}{}})
	}

	like := "%" + strings.ToLower(q) + "%"
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit > 50 {
		limit = 50
	}

	type SearchResult struct {
		ConversationID    uuid.UUID `json:"conversation_id"`
		ConversationTitle string    `json:"conversation_title"`
		MatchType         string    `json:"match_type"` // "title" or "message"
		MessagePreview    string    `json:"message_preview,omitempty"`
		MessageRole       string    `json:"message_role,omitempty"`
		UpdatedAt         time.Time `json:"updated_at"`
	}

	var results []SearchResult

	// 1) Search by conversation title
	var titleConvs []models.AiConversation
	database.DB.Where("tenant_id = ? AND user_id = ? AND LOWER(title) LIKE ?", tenantID, userID, like).
		Order("updated_at DESC").
		Limit(limit).
		Find(&titleConvs)

	titleConvIDs := make(map[uuid.UUID]bool)
	for _, conv := range titleConvs {
		titleConvIDs[conv.ID] = true
		results = append(results, SearchResult{
			ConversationID:    conv.ID,
			ConversationTitle: conv.Title,
			MatchType:         "title",
			UpdatedAt:         conv.UpdatedAt,
		})
	}

	// 2) Search by message content
	type msgRow struct {
		ConversationID uuid.UUID `gorm:"column:conversation_id"`
		Content        string    `gorm:"column:content"`
		ExtraFields    string    `gorm:"column:extra_fields"`
		ConvTitle      string    `gorm:"column:conv_title"`
		ConvUpdatedAt  time.Time `gorm:"column:conv_updated_at"`
	}

	var msgRows []msgRow
	database.DB.Raw(`
		SELECT DISTINCT ON (m.conversation_id)
			m.conversation_id,
			m.content,
			m.extra_fields::text AS extra_fields,
			c.title AS conv_title,
			c.updated_at AS conv_updated_at
		FROM messages m
		JOIN ai_conversations c ON c.id = m.conversation_id
		WHERE c.tenant_id = ? AND c.user_id = ?
			AND m.message_type = 'ai_chat' AND m.status = 'active'
			AND LOWER(m.content) LIKE ?
		ORDER BY m.conversation_id, m.created_at DESC
		LIMIT ?
	`, tenantID, userID, like, limit).Scan(&msgRows)

	for _, row := range msgRows {
		if titleConvIDs[row.ConversationID] {
			continue // already matched by title
		}
		preview := row.Content
		if len([]rune(preview)) > 80 {
			preview = string([]rune(preview)[:80]) + "..."
		}
		role := ""
		if strings.Contains(row.ExtraFields, `"assistant"`) {
			role = "assistant"
		} else if strings.Contains(row.ExtraFields, `"user"`) {
			role = "user"
		}
		results = append(results, SearchResult{
			ConversationID:    row.ConversationID,
			ConversationTitle: row.ConvTitle,
			MatchType:         "message",
			MessagePreview:    preview,
			MessageRole:       role,
			UpdatedAt:         row.ConvUpdatedAt,
		})
	}

	return c.JSON(fiber.Map{"data": results})
}

// ============================================
// 備忘 (Note) CRUD
// ============================================

func GetNotes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var notes []models.Note
	query := database.DB.Where("tenant_id = ?", tenantID)

	if userID != uuid.Nil {
		query = query.Where("user_id = ?", userID)
	}

	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}

	if isPinned := c.Query("is_pinned"); isPinned == "true" {
		query = query.Where("is_pinned = ?", true)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Note{}).Count(&total)

	if err := query.Preload("User").Offset(offset).Limit(limit).Order("is_pinned DESC, created_at DESC").Find(&notes).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  notes,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetNote(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var note models.Note

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("User").First(&note).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Note not found"})
	}

	return c.JSON(note)
}

func CreateNote(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var note models.Note
	if err := c.BodyParser(&note); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	note.TenantID = tenantID
	if note.UserID == uuid.Nil {
		note.UserID = userID
	}

	if err := database.DB.Create(&note).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(note)
}

func UpdateNote(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var note models.Note

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&note).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Note not found"})
	}

	if err := c.BodyParser(&note); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	note.TenantID = tenantID

	if err := database.DB.Save(&note).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(note)
}

func DeleteNote(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Note{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Note deleted"})
}

// ============================================
// 個人資料 (PersonalData) CRUD
// ============================================

func GetPersonalData(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var personalData []models.PersonalData
	query := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID)

	if dataType := c.Query("data_type"); dataType != "" {
		query = query.Where("data_type = ?", dataType)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.PersonalData{}).Count(&total)

	if err := query.Preload("User").Offset(offset).Limit(limit).Find(&personalData).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  personalData,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetPersonalDataItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")
	var personalData models.PersonalData

	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).Preload("User").First(&personalData).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Personal data not found"})
	}

	return c.JSON(personalData)
}

func CreatePersonalData(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var personalData models.PersonalData
	if err := c.BodyParser(&personalData); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	personalData.TenantID = tenantID
	personalData.UserID = userID

	if err := database.DB.Create(&personalData).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(personalData)
}

func UpdatePersonalData(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")
	var personalData models.PersonalData

	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).First(&personalData).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Personal data not found"})
	}

	if err := c.BodyParser(&personalData); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	personalData.TenantID = tenantID
	personalData.UserID = userID

	if err := database.DB.Save(&personalData).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(personalData)
}

func DeletePersonalData(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).Delete(&models.PersonalData{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Personal data deleted"})
}
