package middleware

import (
	"nwork/internal/database"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// TrialExpiredMiddleware 檢查試用期是否過期，如果過期則顯示訂閱彈窗
func TrialExpiredMiddleware(c *fiber.Ctx) error {
	tenantID := GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Next()
	}

	// 付款/訂閱相關端點必須放行：否則試用過期用戶永遠無法完成付款解鎖
	// （即使用戶被鎖在 /subscription-required，也需要能呼叫 create-checkout-session）
	p := c.Path()
	if strings.HasPrefix(p, "/api/v1/billing/") {
		return c.Next()
	}
	// 訂閱頁面必須放行：允許試用過期的用戶訪問訂閱頁面
	if p == "/subscription-required" {
		return c.Next()
	}
	// 用戶信息 API 必須放行：首頁和其他頁面需要獲取用戶信息以顯示登入狀態
	if p == "/api/v1/user/me" {
		return c.Next()
	}
	// 租戶行業模板 API 必須放行：登錄後需要檢查行業模板以決定重定向
	if p == "/api/v1/tenant/industry-template" {
		return c.Next()
	}
	// 租戶設置 API 必須放行：用戶還沒有租戶時必須能設置租戶
	if p == "/api/v1/tenant/setup" {
		return c.Next()
	}
	// 公開頁面永遠放行（即使已過期也不應被導走）
	if p == "/" || p == "/home" || p == "/contact" || p == "/sales-partner" || strings.HasPrefix(p, "/help") {
		return c.Next()
	}
	if p == "/api/v1/sales-partner/apply" {
		return c.Next()
	}

	// 獲取租戶信息
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Next()
	}

	// 如果有訂閱，跳過試用期檢查
	if tenant.SubscriptionID != nil && *tenant.SubscriptionID != "" {
		return c.Next()
	}

	// 只檢查試用期/免費計劃
	if tenant.Plan != "trial" && tenant.Plan != "free" {
		return c.Next()
	}

	// 如果是銷售商試用模式，檢查銷售商試用到期時間
	if isSalesPartnerTrial(&tenant) {
		// 銷售商試用期內，允許繼續（無配額限制）
		return c.Next()
	}

	// 檢查銷售商試用是否過期（曾經是銷售商試用但已過期）
	if tenant.ExtraFields != nil {
		if v, ok := tenant.ExtraFields["sales_partner_trial"]; ok {
			if b, ok := v.(bool); ok && b {
				// 曾經是銷售商試用，但 isSalesPartnerTrial 返回 false 表示已過期
				if tenant.TrialExpiresAt != nil && tenant.TrialExpiresAt.Before(time.Now()) {
					// 銷售商試用已過期，需要訂閱
					accept := c.Get("Accept")
					if strings.Contains(accept, "application/json") || strings.HasPrefix(c.Path(), "/api") {
						return c.Status(403).JSON(fiber.Map{
							"error":    "subscription_required",
							"message":  "銷售商試用期已結束，請訂閱以繼續使用",
							"redirect": "/subscription-required",
						})
					}
					return c.Redirect("/subscription-required")
				}
			}
		}
	}

	// 僅在「試用配額已超額」且一週寬限期到期後鎖定
	graceUntil, hasGrace := getTrialGraceUntil(&tenant)
	if !hasGrace {
		return c.Next()
	}

	now := time.Now()
	if graceUntil.After(now) {
		// 寬限期內，繼續
		return c.Next()
	}

	// 試用期已過期，檢查是否是 API 請求
	accept := c.Get("Accept")
	if strings.Contains(accept, "application/json") || strings.HasPrefix(c.Path(), "/api") {
		// API 請求：返回「需要訂閱」訊號（前端可依 redirect 進入訂閱/付款流程）
		return c.Status(403).JSON(fiber.Map{
			"error":    "subscription_required",
			"message":  "Subscription required",
			"redirect": "/subscription-required",
		})
	}

	// 網頁請求，重定向到訂閱頁面
	return c.Redirect("/subscription-required")
}
