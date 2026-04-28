package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetBusinessGoals 獲取業務目標列表
func GetBusinessGoals(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	status := c.Query("status")
	metricType := c.Query("metric_type")
	search := c.Query("search")
	priority := c.Query("priority")

	query := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if metricType != "" {
		query = query.Where("metric_type = ?", metricType)
	}
	if priority != "" {
		query = query.Where("priority = ?", priority)
	}
	if search != "" {
		query = query.Where("(title ILIKE ? OR description ILIKE ?)", "%"+search+"%", "%"+search+"%")
	}

	var total int64
	query.Model(&models.BusinessGoal{}).Count(&total)

	var goals []models.BusinessGoal
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&goals).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch business goals: %v", err)})
	}

	// Auto-refresh current_value for non-custom active goals
	for i := range goals {
		g := &goals[i]
		if g.MetricType != "custom" && g.Status == "active" {
			newVal := calculateCurrentValue(tenantID, g.MetricType, g.TargetEntityID, g.StartDate, g.EndDate)
			if newVal != g.CurrentValue {
				g.CurrentValue = newVal
				// Check if goal is completed
				if newVal >= g.TargetValue {
					g.Status = "completed"
				}
				// Check if goal has expired
				if time.Now().After(g.EndDate) && newVal < g.TargetValue {
					g.Status = "failed"
				}
				database.DB.Model(g).Updates(map[string]interface{}{
					"current_value": g.CurrentValue,
					"status":        g.Status,
				})
			}
		}
	}

	return c.JSON(fiber.Map{
		"data":  goals,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetBusinessGoal 獲取單個業務目標（含追蹤記錄）
func GetBusinessGoal(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	var goal models.BusinessGoal
	if err := database.DB.
		Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).
		Preload("Trackings", func(db *gorm.DB) *gorm.DB {
			return db.Order("tracked_at DESC").Limit(50)
		}).
		First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	// Auto-refresh current_value for non-custom active goals
	if goal.MetricType != "custom" && goal.Status == "active" {
		newVal := calculateCurrentValue(tenantID, goal.MetricType, goal.TargetEntityID, goal.StartDate, goal.EndDate)
		if newVal != goal.CurrentValue {
			goal.CurrentValue = newVal
			// Check if goal is completed
			if newVal >= goal.TargetValue {
				goal.Status = "completed"
			}
			// Check if goal has expired
			if time.Now().After(goal.EndDate) && newVal < goal.TargetValue {
				goal.Status = "failed"
			}
			database.DB.Model(&goal).Updates(map[string]interface{}{
				"current_value": goal.CurrentValue,
				"status":        goal.Status,
			})
		}
	}

	return c.JSON(goal)
}

// CreateBusinessGoal 新增業務目標
func CreateBusinessGoal(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		Title          string     `json:"title"`
		Description    string     `json:"description"`
		MetricType     string     `json:"metric_type"`
		TargetEntityID *uuid.UUID `json:"target_entity_id"`
		TargetValue    float64    `json:"target_value"`
		Unit           string     `json:"unit"`
		StartDate      string     `json:"start_date"`
		EndDate        string     `json:"end_date"`
		Priority       string     `json:"priority"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.TargetValue <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Target value must be greater than 0"})
	}
	if req.StartDate == "" || req.EndDate == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Start date and end date are required"})
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid start date format (use YYYY-MM-DD)"})
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid end date format (use YYYY-MM-DD)"})
	}
	if endDate.Before(startDate) {
		return c.Status(400).JSON(fiber.Map{"error": "End date must be after start date"})
	}

	metricType := req.MetricType
	if metricType == "" {
		metricType = "custom"
	}
	priority := req.Priority
	if priority == "" {
		priority = "medium"
	}

	// Auto-generate title if empty
	title := req.Title
	if title == "" {
		title = generateGoalTitle(metricType, req.TargetValue, req.Unit)
	}

	// Calculate current value based on metric type
	currentValue := calculateCurrentValue(tenantID, metricType, req.TargetEntityID, startDate, endDate)

	goal := models.BusinessGoal{
		TenantID:       tenantID,
		Title:          title,
		Description:    req.Description,
		MetricType:     metricType,
		TargetEntityID: req.TargetEntityID,
		TargetValue:    req.TargetValue,
		CurrentValue:   currentValue,
		Unit:           req.Unit,
		StartDate:      startDate,
		EndDate:        endDate,
		Status:         "active",
		Priority:       priority,
		CreatedBy:      &userID,
		UpdatedBy:      &userID,
	}

	if err := database.DB.Create(&goal).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create business goal: %v", err)})
	}

	// Create initial tracking record
	pct := goal.ProgressPercent()
	tracking := models.BusinessGoalTracking{
		TenantID:     tenantID,
		GoalID:       goal.ID,
		TrackedValue: currentValue,
		DeltaValue:   0,
		ProgressPct:  pct,
		Source:       "auto",
		TrackedAt:    time.Now(),
	}
	database.DB.Create(&tracking)

	// Log activity
	_ = utils.LogActivity(tenantID, userID, "create", "business_goal", &goal.ID, fmt.Sprintf(`{"key":"business_goal.create","params":{"title":%q}}`, goal.Title), nil, c)

	return c.Status(201).JSON(goal)
}

// UpdateBusinessGoal 更新業務目標
func UpdateBusinessGoal(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	var goal models.BusinessGoal
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	var req struct {
		Title          *string    `json:"title"`
		Description    *string    `json:"description"`
		MetricType     *string    `json:"metric_type"`
		TargetEntityID *uuid.UUID `json:"target_entity_id"`
		TargetValue    *float64   `json:"target_value"`
		CurrentValue   *float64   `json:"current_value"`
		Unit           *string    `json:"unit"`
		StartDate      *string    `json:"start_date"`
		EndDate        *string    `json:"end_date"`
		Status         *string    `json:"status"`
		Priority       *string    `json:"priority"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	changes := map[string]interface{}{}

	if req.Title != nil && *req.Title != goal.Title {
		changes["title"] = map[string]interface{}{"old": goal.Title, "new": *req.Title}
		goal.Title = *req.Title
	}
	if req.Description != nil {
		goal.Description = *req.Description
	}
	if req.MetricType != nil {
		goal.MetricType = *req.MetricType
	}
	if req.TargetEntityID != nil {
		goal.TargetEntityID = req.TargetEntityID
	}
	if req.TargetValue != nil && *req.TargetValue != goal.TargetValue {
		changes["target_value"] = map[string]interface{}{"old": goal.TargetValue, "new": *req.TargetValue}
		goal.TargetValue = *req.TargetValue
	}
	if req.CurrentValue != nil {
		goal.CurrentValue = *req.CurrentValue
	}
	if req.Unit != nil {
		goal.Unit = *req.Unit
	}
	if req.StartDate != nil {
		if sd, err := time.Parse("2006-01-02", *req.StartDate); err == nil {
			goal.StartDate = sd
		}
	}
	if req.EndDate != nil {
		if ed, err := time.Parse("2006-01-02", *req.EndDate); err == nil {
			goal.EndDate = ed
		}
	}
	if req.Status != nil {
		changes["status"] = map[string]interface{}{"old": goal.Status, "new": *req.Status}
		goal.Status = *req.Status
	}
	if req.Priority != nil {
		goal.Priority = *req.Priority
	}

	goal.UpdatedBy = &userID

	if err := database.DB.Save(&goal).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update business goal: %v", err)})
	}

	_ = utils.LogActivity(tenantID, userID, "update", "business_goal", &goal.ID, fmt.Sprintf(`{"key":"business_goal.update","params":{"title":%q}}`, goal.Title), changes, c)

	return c.JSON(goal)
}

// DeleteBusinessGoal 刪除業務目標（軟刪除）
func DeleteBusinessGoal(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	var goal models.BusinessGoal
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	now := time.Now()
	goal.TrashedAt = &now
	if err := database.DB.Save(&goal).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete business goal: %v", err)})
	}

	_ = utils.LogActivity(tenantID, userID, "delete", "business_goal", &goal.ID, fmt.Sprintf(`{"key":"business_goal.delete","params":{"title":%q}}`, goal.Title), nil, c)

	return c.JSON(fiber.Map{"message": "Business goal deleted successfully. It will be permanently removed after 7 days."})
}

// RefreshBusinessGoalProgress 刷新目標進度（重新計算 current_value）
func RefreshBusinessGoalProgress(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	var goal models.BusinessGoal
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	if goal.MetricType == "custom" {
		return c.Status(400).JSON(fiber.Map{"error": "Custom goals must be updated manually"})
	}

	oldValue := goal.CurrentValue
	newValue := calculateCurrentValue(tenantID, goal.MetricType, goal.TargetEntityID, goal.StartDate, goal.EndDate)
	goal.CurrentValue = newValue

	// Check if goal is completed
	if newValue >= goal.TargetValue && goal.Status == "active" {
		goal.Status = "completed"
	}

	// Check if goal has expired
	if time.Now().After(goal.EndDate) && goal.Status == "active" && newValue < goal.TargetValue {
		goal.Status = "failed"
	}

	if err := database.DB.Save(&goal).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update goal progress: %v", err)})
	}

	// Create tracking record
	delta := newValue - oldValue
	pct := goal.ProgressPercent()
	tracking := models.BusinessGoalTracking{
		TenantID:     tenantID,
		GoalID:       goal.ID,
		TrackedValue: newValue,
		DeltaValue:   delta,
		ProgressPct:  pct,
		Source:       "auto",
		TrackedAt:    time.Now(),
	}
	database.DB.Create(&tracking)

	return c.JSON(goal)
}

// GetBusinessGoalTrackings 獲取目標追蹤記錄
func GetBusinessGoalTrackings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	goalID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	// Verify goal belongs to tenant
	var goal models.BusinessGoal
	if err := database.DB.Where("id = ? AND tenant_id = ?", goalID, tenantID).First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	var trackings []models.BusinessGoalTracking
	if err := database.DB.Where("goal_id = ? AND tenant_id = ?", goalID, tenantID).
		Order("tracked_at DESC").
		Limit(100).
		Find(&trackings).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch trackings: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data": trackings,
		"goal": goal,
	})
}

// AddManualTracking 手動添加追蹤記錄（用於 custom 類型目標）
func AddManualTracking(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	goalID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	var goal models.BusinessGoal
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", goalID, tenantID).First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	var req struct {
		Value float64 `json:"value"`
		Note  string  `json:"note"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	oldValue := goal.CurrentValue
	goal.CurrentValue = req.Value

	if goal.CurrentValue >= goal.TargetValue && goal.Status == "active" {
		goal.Status = "completed"
	}

	if err := database.DB.Save(&goal).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update goal: %v", err)})
	}

	delta := req.Value - oldValue
	pct := goal.ProgressPercent()
	tracking := models.BusinessGoalTracking{
		TenantID:     tenantID,
		GoalID:       goal.ID,
		TrackedValue: req.Value,
		DeltaValue:   delta,
		ProgressPct:  pct,
		AiNote:       req.Note,
		Source:       "manual",
		TrackedAt:    time.Now(),
	}
	database.DB.Create(&tracking)

	return c.JSON(fiber.Map{
		"goal":     goal,
		"tracking": tracking,
	})
}

// GetBusinessGoalAISuggestion 使用 AI 分析目標並給出建議
func GetBusinessGoalAISuggestion(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	var goal models.BusinessGoal
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	// Get recent trackings for trend analysis
	var trackings []models.BusinessGoalTracking
	database.DB.Where("goal_id = ? AND tenant_id = ?", id, tenantID).
		Order("tracked_at DESC").
		Limit(20).
		Find(&trackings)

	// Build AI prompt (legacy endpoint: read locale from query param, default to zh)
	locale := c.Query("locale", "zh")
	suggestion, err := generateGoalAISuggestion(tenantID, &goal, trackings, locale)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to generate AI suggestion: %v", err)})
	}

	// Save suggestion to goal
	now := time.Now()
	newSuggestion := map[string]interface{}{
		"content":    suggestion,
		"created_at": now.Format(time.RFC3339),
	}

	// Build suggestions list: prepend new, keep max 10
	var items []interface{}
	if goal.AiSuggestions != nil {
		if existingItems, ok := goal.AiSuggestions["items"]; ok {
			if arr, ok := existingItems.([]interface{}); ok {
				items = arr
			}
		}
	}
	items = append([]interface{}{newSuggestion}, items...)
	if len(items) > 10 {
		items = items[:10]
	}

	goal.AiSuggestions = models.JSONB{"items": items}
	goal.AiLastAnalyzedAt = &now

	database.DB.Save(&goal)

	return c.JSON(fiber.Map{
		"suggestion":  suggestion,
		"goal":        goal,
		"analyzed_at": now,
	})
}

// GetBusinessGoalsDashboard 獲取業務目標儀表板數據
func GetBusinessGoalsDashboard(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	// Active goals
	var activeGoals []models.BusinessGoal
	database.DB.Where("tenant_id = ? AND status = 'active' AND trashed_at IS NULL", tenantID).
		Order("priority DESC, end_date ASC").
		Find(&activeGoals)

	// Refresh current values for auto-trackable goals
	for i := range activeGoals {
		g := &activeGoals[i]
		if g.MetricType != "custom" {
			newVal := calculateCurrentValue(tenantID, g.MetricType, g.TargetEntityID, g.StartDate, g.EndDate)
			if newVal != g.CurrentValue {
				g.CurrentValue = newVal
				database.DB.Model(g).Update("current_value", newVal)
			}
		}
	}

	// Summary stats
	var totalActive int64
	var totalCompleted int64
	var totalFailed int64
	database.DB.Model(&models.BusinessGoal{}).Where("tenant_id = ? AND status = 'active' AND trashed_at IS NULL", tenantID).Count(&totalActive)
	database.DB.Model(&models.BusinessGoal{}).Where("tenant_id = ? AND status = 'completed' AND trashed_at IS NULL", tenantID).Count(&totalCompleted)
	database.DB.Model(&models.BusinessGoal{}).Where("tenant_id = ? AND status = 'failed' AND trashed_at IS NULL", tenantID).Count(&totalFailed)

	// Goals expiring soon (within 7 days)
	sevenDaysLater := time.Now().AddDate(0, 0, 7)
	var expiringSoon []models.BusinessGoal
	database.DB.Where("tenant_id = ? AND status = 'active' AND end_date <= ? AND trashed_at IS NULL", tenantID, sevenDaysLater).
		Order("end_date ASC").
		Find(&expiringSoon)

	return c.JSON(fiber.Map{
		"active_goals":    activeGoals,
		"total_active":    totalActive,
		"total_completed": totalCompleted,
		"total_failed":    totalFailed,
		"expiring_soon":   expiringSoon,
	})
}

// calculateCurrentValue 根據指標類型從業務數據計算當前值
func calculateCurrentValue(tenantID uuid.UUID, metricType string, targetEntityID *uuid.UUID, startDate, endDate time.Time) float64 {
	var value float64

	// endDate from DB is date-only (00:00:00), so we need to extend it to end of day
	// by adding 1 day and using < instead of <=
	endDateExclusive := endDate.AddDate(0, 0, 1)

	switch metricType {
	case "order_count":
		var count int64
		query := database.DB.Model(&models.Order{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND status != ? AND trashed_at IS NULL",
				tenantID, startDate, endDateExclusive, "cancelled")
		query.Count(&count)
		value = float64(count)

	case "revenue":
		database.DB.Model(&models.Order{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND status != ? AND trashed_at IS NULL",
				tenantID, startDate, endDateExclusive, "cancelled").
			Select("COALESCE(SUM(total_amount), 0)").
			Scan(&value)

	case "customer_count":
		var count int64
		database.DB.Model(&models.Customer{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND trashed_at IS NULL",
				tenantID, startDate, endDateExclusive).
			Count(&count)
		value = float64(count)

	case "product_sales_qty":
		query := database.DB.Model(&models.OrderItem{}).
			Joins("JOIN orders ON orders.id = order_items.order_id").
			Where("order_items.tenant_id = ? AND orders.created_at >= ? AND orders.created_at < ? AND orders.status != ? AND orders.trashed_at IS NULL",
				tenantID, startDate, endDateExclusive, "cancelled")
		if targetEntityID != nil {
			query = query.Where("order_items.product_id = ?", *targetEntityID)
		}
		query.Select("COALESCE(SUM(order_items.quantity), 0)").Scan(&value)

	case "service_order_count":
		var count int64
		database.DB.Model(&models.ServiceOrder{}).
			Where("tenant_id = ? AND created_at >= ? AND created_at < ? AND trashed_at IS NULL",
				tenantID, startDate, endDateExclusive).
			Count(&count)
		value = float64(count)

	default:
		// custom: return 0, user manages manually
		value = 0
	}

	return math.Round(value*100) / 100
}

// generateGoalAISuggestion 使用 LLM 生成目標建議
// goalPromptByLocale returns locale-aware prompt fragments for business goal AI analysis.
// Returns (roleDesc, langInstruction, promptTemplate, labels) based on the user's UI locale.
func goalPromptByLocale(locale string) (roleDesc, langInstr string, labels map[string]string) {
	switch {
	case strings.HasPrefix(locale, "en"):
		return "You are a professional business consultant AI.",
			"Please answer in English, concise and practical, no more than 500 words.",
			map[string]string{
				"enterprise":     "Enterprise",
				"industry":       "Industry",
				"address":        "Address",
				"phone":          "Phone",
				"email":          "Email",
				"timezone":       "Timezone",
				"enterpriseInfo": "Enterprise Profile",
				"goalTitle":      "Goal Title",
				"goalDesc":       "Goal Description",
				"metricType":     "Metric Type",
				"targetValue":    "Target Value",
				"currentValue":   "Current Value",
				"progress":       "Progress",
				"expectedProg":   "Expected Progress (by time ratio)",
				"daysLeft":       "Days Remaining",
				"totalDays":      "Total Days",
				"status":         "Status",
				"priority":       "Priority",
				"trendUp":        "Trend: Rising (+%.2f)",
				"trendDown":      "Trend: Declining (%.2f)",
				"trendFlat":      "Trend: Flat",
				"provide":        "Please provide:",
				"item1":          "1. Current progress assessment (whether it meets expectations)",
				"item2":          "2. Specific action suggestions (3-5 items)",
				"item3":          "3. Risk warnings if any",
				"item4":          "4. Prediction of whether the goal can be achieved on time",
				"userQuestion":   "User's question",
				"chatInstr":      "Based on the above data, answer the user's question. Be specific, practical, and constructive. Please answer in English, no more than 500 words.",
			}
	case locale == "zh-CN":
		return "你是一位专业的业务顾问 AI。",
			"请用简体中文回答，回答要简洁实用，不超过 500 字。",
			map[string]string{
				"enterprise":     "企业",
				"industry":       "行业",
				"address":        "地址",
				"phone":          "电话",
				"email":          "电邮",
				"timezone":       "时区",
				"enterpriseInfo": "企业概况",
				"goalTitle":      "目标名称",
				"goalDesc":       "目标描述",
				"metricType":     "指标类型",
				"targetValue":    "目标值",
				"currentValue":   "当前值",
				"progress":       "进度",
				"expectedProg":   "预期进度（按时间比例）",
				"daysLeft":       "剩余天数",
				"totalDays":      "总天数",
				"status":         "状态",
				"priority":       "优先级",
				"trendUp":        "趋势：上升中（+%.2f）",
				"trendDown":      "趋势：下降中（%.2f）",
				"trendFlat":      "趋势：持平",
				"provide":        "请提供：",
				"item1":          "1. 目前进度评估（是否符合预期）",
				"item2":          "2. 具体的行动建议（3-5 条）",
				"item3":          "3. 如有风险，给出预警",
				"item4":          "4. 预测能否在期限内达成",
				"userQuestion":   "用户的问题",
				"chatInstr":      "请根据以上资料，回答用户的问题。回答要具体、实用、有建设性。请用简体中文回答，不超过 500 字。",
			}
	default: // zh (Traditional Chinese)
		return "你是一位專業的業務顧問 AI。",
			"請用繁體中文回答，回答要簡潔實用，不超過 500 字。",
			map[string]string{
				"enterprise":     "企業",
				"industry":       "行業",
				"address":        "地址",
				"phone":          "電話",
				"email":          "電郵",
				"timezone":       "時區",
				"enterpriseInfo": "企業概況",
				"goalTitle":      "目標名稱",
				"goalDesc":       "目標描述",
				"metricType":     "指標類型",
				"targetValue":    "目標值",
				"currentValue":   "當前值",
				"progress":       "進度",
				"expectedProg":   "預期進度（按時間比例）",
				"daysLeft":       "剩餘天數",
				"totalDays":      "總天數",
				"status":         "狀態",
				"priority":       "優先級",
				"trendUp":        "趨勢：上升中（+%.2f）",
				"trendDown":      "趨勢：下降中（%.2f）",
				"trendFlat":      "趨勢：持平",
				"provide":        "請提供：",
				"item1":          "1. 目前進度評估（是否符合預期）",
				"item2":          "2. 具體的行動建議（3-5 條）",
				"item3":          "3. 如有風險，給出預警",
				"item4":          "4. 預測能否在期限內達成",
				"userQuestion":   "用戶的問題",
				"chatInstr":      "請根據以上資料，回答用戶的問題。回答要具體、實用、有建設性。請用繁體中文回答，不超過 500 字。",
			}
	}
}

// buildEnterpriseInfo builds a formatted enterprise profile block for AI prompts.
// It reads enterprise data (including ExtraFields like industry, phone, email) and
// returns a multi-line string section to embed in the prompt.
func buildEnterpriseInfo(tenantID uuid.UUID, labels map[string]string) string {
	var enterprise models.Enterprise
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&enterprise).Error; err != nil {
		return ""
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("--- %s ---", labels["enterpriseInfo"]))
	lines = append(lines, fmt.Sprintf("%s：%s", labels["enterprise"], enterprise.Name))

	// Industry from extra_fields
	if enterprise.ExtraFields != nil {
		if industry, ok := enterprise.ExtraFields["industry"]; ok {
			if s, ok := industry.(string); ok && s != "" {
				lines = append(lines, fmt.Sprintf("%s：%s", labels["industry"], s))
			}
		}
		if phone, ok := enterprise.ExtraFields["phone"]; ok {
			if s, ok := phone.(string); ok && s != "" {
				lines = append(lines, fmt.Sprintf("%s：%s", labels["phone"], s))
			}
		}
		if email, ok := enterprise.ExtraFields["email"]; ok {
			if s, ok := email.(string); ok && s != "" {
				lines = append(lines, fmt.Sprintf("%s：%s", labels["email"], s))
			}
		}
	}

	if enterprise.Address != nil && *enterprise.Address != "" {
		lines = append(lines, fmt.Sprintf("%s：%s", labels["address"], *enterprise.Address))
	}
	if enterprise.Timezone != "" {
		lines = append(lines, fmt.Sprintf("%s：%s", labels["timezone"], enterprise.Timezone))
	}

	return strings.Join(lines, "\n")
}

func generateGoalAISuggestion(tenantID uuid.UUID, goal *models.BusinessGoal, trackings []models.BusinessGoalTracking, locale string) (string, error) {
	cfg := config.Load()
	if cfg.LLM.APIKey == "" {
		return "", fmt.Errorf("LLM API Key not configured")
	}

	// Build context
	progressPct := goal.ProgressPercent()
	daysLeft := int(time.Until(goal.EndDate).Hours() / 24)
	if daysLeft < 0 {
		daysLeft = 0
	}
	totalDays := int(goal.EndDate.Sub(goal.StartDate).Hours() / 24)
	if totalDays < 1 {
		totalDays = 1
	}
	daysElapsed := totalDays - daysLeft
	expectedPct := float64(daysElapsed) / float64(totalDays) * 100

	// Get locale-aware prompt fragments
	roleDesc, langInstr, labels := goalPromptByLocale(locale)

	// Build trend info from trackings (locale-aware)
	trendInfo := ""
	if len(trackings) >= 2 {
		latest := trackings[0]
		oldest := trackings[len(trackings)-1]
		totalDelta := latest.TrackedValue - oldest.TrackedValue
		if totalDelta > 0 {
			trendInfo = fmt.Sprintf(labels["trendUp"], totalDelta)
		} else if totalDelta < 0 {
			trendInfo = fmt.Sprintf(labels["trendDown"], totalDelta)
		} else {
			trendInfo = labels["trendFlat"]
		}
	}

	// Get enterprise info (full profile)
	enterpriseInfo := buildEnterpriseInfo(tenantID, labels)

	prompt := fmt.Sprintf(`%s

%s

%s：%s
%s：%s
%s：%s
%s：%.2f %s
%s：%.2f %s
%s：%.1f%%
%s：%.1f%%
%s：%d / %s %d
%s：%s
%s：%s
%s

%s
%s
%s
%s
%s

%s`,
		roleDesc,
		enterpriseInfo,
		labels["goalTitle"], goal.Title,
		labels["goalDesc"], goal.Description,
		labels["metricType"], goal.MetricType,
		labels["targetValue"], goal.TargetValue, goal.Unit,
		labels["currentValue"], goal.CurrentValue, goal.Unit,
		labels["progress"], progressPct,
		labels["expectedProg"], expectedPct,
		labels["daysLeft"], daysLeft, labels["totalDays"], totalDays,
		labels["status"], goal.Status,
		labels["priority"], goal.Priority,
		trendInfo,
		labels["provide"],
		labels["item1"],
		labels["item2"],
		labels["item3"],
		labels["item4"],
		langInstr,
	)

	if cfg.LLM.Provider == "gemini" {
		return callGeminiForSuggestion(cfg, prompt)
	}
	return callOpenAIForSuggestion(cfg, prompt, roleDesc)
}

func callGeminiForSuggestion(cfg *config.Config, prompt string) (string, error) {
	model := cfg.LLM.Model
	if model == "" || model == "gemini-pro" || model == "gemini-1.5-flash" || model == "gemini-1.5-pro" || model == "gemini-2.0-flash-exp" {
		model = "gemini-2.5-flash"
	}
	model = strings.TrimSuffix(model, "-latest")

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 4096,
		},
		"safetySettings": []map[string]interface{}{
			{
				"category":  "HARM_CATEGORY_HARASSMENT",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_HATE_SPEECH",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_SEXUALLY_EXPLICIT",
				"threshold": "BLOCK_NONE",
			},
			{
				"category":  "HARM_CATEGORY_DANGEROUS_CONTENT",
				"threshold": "BLOCK_NONE",
			},
		},
	}

	body, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Gemini API error: %s", string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	// Extract text from Gemini response
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", fmt.Errorf("no candidates in response")
	}
	candidate := candidates[0].(map[string]interface{})

	// Check for finish reason
	if reason, ok := candidate["finishReason"].(string); ok && reason != "STOP" && reason != "" {
		log.Printf("[vAI] Gemini finish reason: %s", reason)
		// If it's SAFETY or OTHER, it might be why it's truncated
	}

	content, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no content in candidate")
	}
	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("no parts in content")
	}

	var fullText strings.Builder
	for _, p := range parts {
		if part, ok := p.(map[string]interface{}); ok {
			if text, ok := part["text"].(string); ok {
				fullText.WriteString(text)
			}
		}
	}

	return fullText.String(), nil
}

func callOpenAIForSuggestion(cfg *config.Config, prompt string, systemMsg string) (string, error) {
	model := cfg.LLM.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	apiURL := cfg.LLM.Endpoint
	if apiURL == "" {
		apiURL = "https://api.openai.com/v1/chat/completions"
	}

	if systemMsg == "" {
		systemMsg = "你是一位專業的業務顧問 AI，專門幫助分析業務目標並提供建議。"
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemMsg},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  1024,
	}

	body, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.LLM.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OpenAI API error: %s", string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	choice := choices[0].(map[string]interface{})
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no message in choice")
	}
	text, ok := message["content"].(string)
	if !ok {
		return "", fmt.Errorf("no content in message")
	}

	return text, nil
}

// GetBusinessGoalAIAnalyses 獲取目標的 AI 分析歷史記錄
func GetBusinessGoalAIAnalyses(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	goalID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	// Verify goal belongs to tenant
	var goal models.BusinessGoal
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", goalID, tenantID).First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	limit := c.QueryInt("limit", 50)
	if limit < 1 || limit > 200 {
		limit = 50
	}

	var analyses []models.BusinessGoalAIAnalysis
	if err := database.DB.Where("goal_id = ? AND tenant_id = ?", goalID, tenantID).
		Order("created_at DESC").
		Limit(limit).
		Find(&analyses).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch AI analyses: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data": analyses,
		"goal": goal,
	})
}

// CreateBusinessGoalAIAnalysis 建立新的 AI 分析（支持 suggestion 和 chat 類型）
func CreateBusinessGoalAIAnalysis(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	goalID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	var goal models.BusinessGoal
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", goalID, tenantID).First(&goal).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Business goal not found"})
	}

	var req struct {
		AnalysisType string `json:"analysis_type"` // suggestion or chat
		UserPrompt   string `json:"user_prompt"`   // user's question (for chat type)
		Locale       string `json:"locale"`        // UI language (en, zh, zh-CN)
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	analysisType := req.AnalysisType
	if analysisType == "" {
		analysisType = "suggestion"
	}
	if analysisType != "suggestion" && analysisType != "chat" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid analysis type. Use 'suggestion' or 'chat'"})
	}

	// For chat type, user_prompt is required
	if analysisType == "chat" && strings.TrimSpace(req.UserPrompt) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "User prompt is required for chat analysis"})
	}

	// Get recent trackings for trend analysis
	var trackings []models.BusinessGoalTracking
	database.DB.Where("goal_id = ? AND tenant_id = ?", goalID, tenantID).
		Order("tracked_at DESC").
		Limit(20).
		Find(&trackings)

	// Generate AI response based on type
	locale := req.Locale
	if locale == "" {
		locale = "zh" // default to Traditional Chinese
	}
	var aiResponse string
	if analysisType == "chat" {
		aiResponse, err = generateGoalAIChatResponse(tenantID, &goal, trackings, req.UserPrompt, locale)
	} else {
		aiResponse, err = generateGoalAISuggestion(tenantID, &goal, trackings, locale)
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to generate AI analysis: %v", err)})
	}

	// Build goal snapshot
	goalSnapshot := models.JSONB{
		"title":         goal.Title,
		"metric_type":   goal.MetricType,
		"target_value":  goal.TargetValue,
		"current_value": goal.CurrentValue,
		"unit":          goal.Unit,
		"status":        goal.Status,
		"priority":      goal.Priority,
		"progress_pct":  goal.ProgressPercent(),
	}

	// Save analysis record
	analysis := models.BusinessGoalAIAnalysis{
		TenantID:     tenantID,
		GoalID:       goalID,
		AnalysisType: analysisType,
		UserPrompt:   req.UserPrompt,
		AIResponse:   aiResponse,
		GoalSnapshot: goalSnapshot,
		CreatedBy:    &userID,
	}

	if err := database.DB.Create(&analysis).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to save AI analysis: %v", err)})
	}

	// Also update the goal's ai_last_analyzed_at
	now := time.Now()
	goal.AiLastAnalyzedAt = &now
	database.DB.Model(&goal).Update("ai_last_analyzed_at", now)

	return c.Status(201).JSON(analysis)
}

// DeleteBusinessGoalAIAnalysis 刪除 AI 分析記錄
func DeleteBusinessGoalAIAnalysis(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	goalID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid goal ID"})
	}

	analysisID, err := uuid.Parse(c.Params("analysisId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid analysis ID"})
	}

	var analysis models.BusinessGoalAIAnalysis
	if err := database.DB.Where("id = ? AND goal_id = ? AND tenant_id = ?", analysisID, goalID, tenantID).First(&analysis).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "AI analysis not found"})
	}

	if err := database.DB.Delete(&analysis).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete AI analysis: %v", err)})
	}

	return c.JSON(fiber.Map{"message": "AI analysis deleted successfully"})
}

// generateGoalAIChatResponse 使用 LLM 回答用戶關於目標的問題
func generateGoalAIChatResponse(tenantID uuid.UUID, goal *models.BusinessGoal, trackings []models.BusinessGoalTracking, userQuestion string, locale string) (string, error) {
	cfg := config.Load()
	if cfg.LLM.APIKey == "" {
		return "", fmt.Errorf("LLM API Key not configured")
	}

	// Build context
	progressPct := goal.ProgressPercent()
	daysLeft := int(time.Until(goal.EndDate).Hours() / 24)
	if daysLeft < 0 {
		daysLeft = 0
	}
	totalDays := int(goal.EndDate.Sub(goal.StartDate).Hours() / 24)
	if totalDays < 1 {
		totalDays = 1
	}
	daysElapsed := totalDays - daysLeft
	expectedPct := float64(daysElapsed) / float64(totalDays) * 100

	// Get locale-aware prompt fragments
	roleDesc, _, labels := goalPromptByLocale(locale)

	// Build trend info from trackings (locale-aware)
	trendInfo := ""
	if len(trackings) >= 2 {
		latest := trackings[0]
		oldest := trackings[len(trackings)-1]
		totalDelta := latest.TrackedValue - oldest.TrackedValue
		if totalDelta > 0 {
			trendInfo = fmt.Sprintf(labels["trendUp"], totalDelta)
		} else if totalDelta < 0 {
			trendInfo = fmt.Sprintf(labels["trendDown"], totalDelta)
		} else {
			trendInfo = labels["trendFlat"]
		}
	}

	// Get enterprise info (full profile)
	enterpriseInfo := buildEnterpriseInfo(tenantID, labels)

	prompt := fmt.Sprintf(`%s

%s

%s：%s
%s：%s
%s：%s
%s：%.2f %s
%s：%.2f %s
%s：%.1f%%
%s：%.1f%%
%s：%d / %s %d
%s：%s
%s：%s
%s

%s：%s

%s`,
		roleDesc,
		enterpriseInfo,
		labels["goalTitle"], goal.Title,
		labels["goalDesc"], goal.Description,
		labels["metricType"], goal.MetricType,
		labels["targetValue"], goal.TargetValue, goal.Unit,
		labels["currentValue"], goal.CurrentValue, goal.Unit,
		labels["progress"], progressPct,
		labels["expectedProg"], expectedPct,
		labels["daysLeft"], daysLeft, labels["totalDays"], totalDays,
		labels["status"], goal.Status,
		labels["priority"], goal.Priority,
		trendInfo,
		labels["userQuestion"], userQuestion,
		labels["chatInstr"],
	)

	if cfg.LLM.Provider == "gemini" {
		return callGeminiForSuggestion(cfg, prompt)
	}
	return callOpenAIForSuggestion(cfg, prompt, roleDesc)
}

// RefreshActiveBusinessGoals refreshes current_value for all active non-custom business goals
// that match the given metric types for a tenant. Call this asynchronously (via goroutine)
// after creating/updating orders, customers, service orders, etc.
func RefreshActiveBusinessGoals(tenantID uuid.UUID, metricTypes []string) {
	if len(metricTypes) == 0 {
		return
	}

	var goals []models.BusinessGoal
	database.DB.Where("tenant_id = ? AND status = 'active' AND metric_type IN ? AND trashed_at IS NULL", tenantID, metricTypes).
		Find(&goals)

	for i := range goals {
		g := &goals[i]
		newVal := calculateCurrentValue(tenantID, g.MetricType, g.TargetEntityID, g.StartDate, g.EndDate)
		if newVal != g.CurrentValue {
			oldVal := g.CurrentValue
			g.CurrentValue = newVal

			// Check if goal is completed
			if newVal >= g.TargetValue {
				g.Status = "completed"
			}
			// Check if goal has expired
			if time.Now().After(g.EndDate) && newVal < g.TargetValue {
				g.Status = "failed"
			}

			database.DB.Model(g).Updates(map[string]interface{}{
				"current_value": g.CurrentValue,
				"status":        g.Status,
			})

			// Create tracking record
			delta := newVal - oldVal
			pct := g.ProgressPercent()
			tracking := models.BusinessGoalTracking{
				TenantID:     tenantID,
				GoalID:       g.ID,
				TrackedValue: newVal,
				DeltaValue:   delta,
				ProgressPct:  pct,
				Source:       "auto",
				TrackedAt:    time.Now(),
			}
			database.DB.Create(&tracking)
		}
	}
}

// generateGoalTitle generates a title from metric type and target value when user leaves title empty
func generateGoalTitle(metricType string, targetValue float64, unit string) string {
	metricNames := map[string]string{
		"order_count":         "訂單數量",
		"revenue":             "營業額",
		"customer_count":      "客戶數量",
		"product_sales_qty":   "產品銷量",
		"service_order_count": "服務訂單數",
		"custom":              "自定義目標",
	}

	name, ok := metricNames[metricType]
	if !ok {
		name = "目標"
	}

	// Format target value — remove trailing zeros
	valueStr := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", targetValue), "0"), ".")

	if unit != "" {
		return fmt.Sprintf("%s 達到 %s %s", name, valueStr, unit)
	}
	return fmt.Sprintf("%s 達到 %s", name, valueStr)
}
