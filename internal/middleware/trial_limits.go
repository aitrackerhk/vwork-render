package middleware

import (
	"fmt"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/models"
	"nwork/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	trialOrderLimit        = 5
	trialServiceOrderLimit = 5
	trialTaskLimit         = 5
	trialAIDailyLimit      = 5
	trialGraceHours        = 168 // 1 week
)

func isTrialLimitedTenant(tenant *models.Tenant) bool {
	if tenant == nil {
		return false
	}
	if tenant.SubscriptionID != nil && strings.TrimSpace(*tenant.SubscriptionID) != "" {
		return false
	}
	// 如果是銷售商試用模式，則不受配額限制
	if isSalesPartnerTrial(tenant) {
		return false
	}
	return tenant.Plan == "trial" || tenant.Plan == "free"
}

// isSalesPartnerTrial 檢查租戶是否為銷售商試用模式（無配額限制）
func isSalesPartnerTrial(tenant *models.Tenant) bool {
	if tenant == nil || tenant.ExtraFields == nil {
		return false
	}
	// 檢查 sales_partner_trial 標記
	if v, ok := tenant.ExtraFields["sales_partner_trial"]; ok {
		if b, ok := v.(bool); ok && b {
			// 檢查是否在試用期內
			if tenant.TrialExpiresAt != nil && tenant.TrialExpiresAt.After(time.Now()) {
				return true
			}
		}
	}
	return false
}

func parseExtraTime(fields models.JSONB, key string) (*time.Time, bool) {
	if fields == nil {
		return nil, false
	}
	raw, ok := fields[key]
	if !ok || raw == nil {
		return nil, false
	}
	switch v := raw.(type) {
	case string:
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
		if err == nil {
			return &t, true
		}
	case time.Time:
		return &v, true
	}
	return nil, false
}

func getTrialGraceUntil(tenant *models.Tenant) (*time.Time, bool) {
	if tenant == nil {
		return nil, false
	}
	if t, ok := parseExtraTime(tenant.ExtraFields, "trial_limit_grace_until"); ok {
		return t, true
	}
	return nil, false
}

func ensureTrialLimitGrace(tenant *models.Tenant, userID uuid.UUID) (time.Time, error) {
	now := utils.NowInTenantTimezone(tenant.ID)

	if tenant.ExtraFields == nil {
		tenant.ExtraFields = make(models.JSONB)
	}

	if existing, ok := getTrialGraceUntil(tenant); ok && existing.After(now) {
		return *existing, nil
	}

	graceUntil := now.Add(time.Duration(trialGraceHours) * time.Hour)
	tenant.ExtraFields["trial_limit_exceeded_at"] = now.Format(time.RFC3339)
	tenant.ExtraFields["trial_limit_grace_until"] = graceUntil.Format(time.RFC3339)

	if err := database.DB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Updates(map[string]interface{}{
		"extra_fields":     tenant.ExtraFields,
		"trial_expires_at": graceUntil,
	}).Error; err != nil {
		return graceUntil, err
	}

	sendTrialLimitNotice(tenant, userID, graceUntil)
	return graceUntil, nil
}

func sendTrialLimitNotice(tenant *models.Tenant, userID uuid.UUID, graceUntil time.Time) {
	if tenant == nil {
		return
	}
	if tenant.ExtraFields != nil {
		if _, ok := tenant.ExtraFields["trial_limit_notice_sent_at"]; ok {
			return
		}
	}

	if tenant.ExtraFields == nil {
		tenant.ExtraFields = make(models.JSONB)
	}
	tenant.ExtraFields["trial_limit_notice_sent_at"] = utils.NowInTenantTimezone(tenant.ID).Format(time.RFC3339)
	_ = database.DB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("extra_fields", tenant.ExtraFields).Error

	deadlineText := graceUntil.Format("2006-01-02 15:04")
	title := "免費試用已達上限"
	message := fmt.Sprintf("您的免費試用已達上限，請於 %s 前完成訂閱以繼續使用。", deadlineText)
	link := "/subscription-required"
	go utils.CreateNotificationAlertForAllUsers(tenant.ID, "trial_limit_exceeded", title, message, link, nil)

	var users []models.User
	if err := database.DB.Where("tenant_id = ? AND status = ?", tenant.ID, "active").Find(&users).Error; err != nil {
		return
	}
	for _, u := range users {
		if strings.TrimSpace(u.Email) == "" {
			continue
		}
		_ = email.EnqueueTrialLimitExceededEmail(tenant.ID, tenant.Subdomain, u.ID, u.Email, u.Name, graceUntil)
	}
}

func trialLimitExceededResponse(c *fiber.Ctx, graceUntil time.Time) error {
	return c.Status(403).JSON(fiber.Map{
		"error":       "subscription_required",
		"message":     "試用配額已達上限，請於一週內完成訂閱以繼續使用。",
		"redirect":    "/subscription-required",
		"grace_until": graceUntil.Format(time.RFC3339),
	})
}

// EnforceTrialOrderLimit 檢查訂單配額（訂單上限 5 個）
func EnforceTrialOrderLimit(c *fiber.Ctx, tenantID uuid.UUID, userID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return nil
	}
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil
	}
	if !isTrialLimitedTenant(&tenant) {
		return nil
	}

	var orderCount int64
	database.DB.Model(&models.Order{}).Where("tenant_id = ?", tenantID).Count(&orderCount)

	if orderCount >= trialOrderLimit {
		graceUntil, _ := ensureTrialLimitGrace(&tenant, userID)
		if graceUntil.After(utils.NowInTenantTimezone(tenant.ID)) {
			return nil
		}
		return trialLimitExceededResponse(c, graceUntil)
	}
	return nil
}

// EnforceTrialServiceOrderLimit 檢查服務單配額（服務單上限 5 個）
func EnforceTrialServiceOrderLimit(c *fiber.Ctx, tenantID uuid.UUID, userID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return nil
	}
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil
	}
	if !isTrialLimitedTenant(&tenant) {
		return nil
	}

	var serviceOrderCount int64
	database.DB.Model(&models.ServiceOrder{}).Where("tenant_id = ?", tenantID).Count(&serviceOrderCount)

	if serviceOrderCount >= trialServiceOrderLimit {
		graceUntil, _ := ensureTrialLimitGrace(&tenant, userID)
		if graceUntil.After(utils.NowInTenantTimezone(tenant.ID)) {
			return nil
		}
		return trialLimitExceededResponse(c, graceUntil)
	}
	return nil
}

// EnforceTrialOrderServiceLimit 舊函數，為了向後兼容保留（現在分別檢查訂單和服務單）
// Deprecated: 請使用 EnforceTrialOrderLimit 或 EnforceTrialServiceOrderLimit
func EnforceTrialOrderServiceLimit(c *fiber.Ctx, tenantID uuid.UUID, userID uuid.UUID) error {
	// 這個函數現在不再使用，但保留以防有其他地方調用
	// 實際上現在應該分別調用 EnforceTrialOrderLimit 和 EnforceTrialServiceOrderLimit
	return nil
}

func EnforceTrialTaskLimit(c *fiber.Ctx, tenantID uuid.UUID, userID uuid.UUID, newTasks int, excludeProjectID *uuid.UUID) error {
	if tenantID == uuid.Nil || newTasks <= 0 {
		return nil
	}
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil
	}
	if !isTrialLimitedTenant(&tenant) {
		return nil
	}

	var existingCount int64
	query := database.DB.Model(&models.ProjectTask{}).Where("tenant_id = ?", tenantID)
	if excludeProjectID != nil && *excludeProjectID != uuid.Nil {
		query = query.Where("project_id <> ?", *excludeProjectID)
	}
	query.Count(&existingCount)

	if existingCount+int64(newTasks) > trialTaskLimit {
		graceUntil, _ := ensureTrialLimitGrace(&tenant, userID)
		if graceUntil.After(utils.NowInTenantTimezone(tenant.ID)) {
			return nil
		}
		return trialLimitExceededResponse(c, graceUntil)
	}
	return nil
}

func EnforceTrialAIDailyLimit(c *fiber.Ctx, tenantID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return nil
	}
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil
	}
	if !isTrialLimitedTenant(&tenant) {
		return nil
	}

	now := utils.NowInTenantTimezone(tenant.ID)
	today := now.Format("2006-01-02")
	count := 0
	storedDate := ""

	if tenant.ExtraFields != nil {
		if v, ok := tenant.ExtraFields["trial_ai_daily_date"].(string); ok {
			storedDate = v
		}
		if v, ok := tenant.ExtraFields["trial_ai_daily_count"].(float64); ok {
			count = int(v)
		}
	}

	if storedDate != today {
		count = 0
		storedDate = today
	}

	if count >= trialAIDailyLimit {
		return c.Status(403).JSON(fiber.Map{
			"error":   "trial_ai_limit_exceeded",
			"message": "試用方案每日 AI 詢問上限為 5 條，請於明日再試或升級訂閱。",
			"limit":   trialAIDailyLimit,
		})
	}

	if tenant.ExtraFields == nil {
		tenant.ExtraFields = make(models.JSONB)
	}
	tenant.ExtraFields["trial_ai_daily_date"] = storedDate
	tenant.ExtraFields["trial_ai_daily_count"] = float64(count + 1)
	_ = database.DB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("extra_fields", tenant.ExtraFields).Error

	return nil
}
