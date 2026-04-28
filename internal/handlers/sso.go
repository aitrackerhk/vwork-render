package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────
// SSO 跨域單點登入
//
// 流程：
//  1. 用戶已在 Domain-A 登入
//  2. 前端呼叫 POST /api/v1/sso/ticket 取得一次性 ticket (60 秒有效)
//  3. 前端把 ticket 附加到目標 Domain-B 的 URL：https://domain-b/?sso_ticket=xxx
//  4. Domain-B 的前端偵測到 sso_ticket 參數，呼叫 POST /api/v1/sso/validate
//  5. 後端驗證 ticket、設定 cookie、回傳 user/tenant 資料
//  6. 前端存入 localStorage，完成跨域登入
// ──────────────────────────────────────────────────────────

// GenerateSSOTicket 產生一次性跨域認證票據
// POST /api/v1/sso/ticket
// 需要已登入（帶 auth_token cookie 或 Authorization header）
func GenerateSSOTicket(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Not authenticated"})
	}
	tenantID := middleware.GetTenantID(c)

	// 產生安全隨機 ticket（32 bytes = 64 hex chars）
	ticketBytes := make([]byte, 32)
	if _, err := rand.Read(ticketBytes); err != nil {
		log.Printf("[SSO] Failed to generate random ticket: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate ticket"})
	}
	ticket := hex.EncodeToString(ticketBytes)

	var tenantIDPtr *uuid.UUID
	if tenantID != uuid.Nil {
		tenantIDPtr = &tenantID
	}

	ssoTicket := models.SSOTicket{
		UserID:    userID,
		TenantID:  tenantIDPtr,
		Ticket:    ticket,
		Used:      false,
		ExpiresAt: time.Now().Add(60 * time.Second), // 60 秒有效
	}

	if err := database.DB.Create(&ssoTicket).Error; err != nil {
		log.Printf("[SSO] Failed to create ticket: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create ticket"})
	}

	return c.JSON(fiber.Map{
		"ticket":     ticket,
		"expires_in": 60,
	})
}

// ValidateSSOTicket 驗證跨域認證票據並設定 cookie
// POST /api/v1/sso/validate
// Body: { "ticket": "xxx" }
// 公開端點（不需要已登入）
func ValidateSSOTicket(c *fiber.Ctx) error {
	type validateRequest struct {
		Ticket string `json:"ticket"`
	}
	var req validateRequest
	if err := c.BodyParser(&req); err != nil || req.Ticket == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: ticket is required"})
	}

	// 查詢 ticket
	var ssoTicket models.SSOTicket
	if err := database.DB.Where("ticket = ?", req.Ticket).First(&ssoTicket).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid or expired ticket"})
	}

	// 檢查是否已使用
	if ssoTicket.Used {
		return c.Status(401).JSON(fiber.Map{"error": "Ticket already used"})
	}

	// 檢查是否過期
	if time.Now().After(ssoTicket.ExpiresAt) {
		// 標記為已使用（清理）
		database.DB.Model(&ssoTicket).Update("used", true)
		return c.Status(401).JSON(fiber.Map{"error": "Ticket expired"})
	}

	// 標記為已使用（一次性）
	database.DB.Model(&ssoTicket).Update("used", true)

	// 查詢用戶
	var user models.User
	if err := database.DB.Preload("Role").Where("id = ? AND status = ?", ssoTicket.UserID, "active").First(&user).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "User not found or inactive"})
	}

	// 確定 tenant
	tenantID := uuid.Nil
	if ssoTicket.TenantID != nil {
		tenantID = *ssoTicket.TenantID
	} else if user.TenantID != nil {
		tenantID = *user.TenantID
	}

	// 查詢 tenant 資訊
	var tenant models.Tenant
	var tenantData fiber.Map
	if tenantID != uuid.Nil {
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			tenantData = fiber.Map{
				"id":        tenant.ID,
				"name":      tenant.Name,
				"subdomain": tenant.Subdomain,
			}
		}
	}

	// 生成 JWT Token
	// 這裡是瀏覽器跨網域 SSO（例如 app switcher），
	// 目標站點應取得 web token，才能與 web logout 同步失效。
	roleName := user.UserRole
	if user.Role != nil {
		roleName = user.Role.Name
	}
	token, err := utils.GenerateToken(user.ID, tenantID, user.Email, roleName, "web")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// 設定 cookie（在目標域名上）
	setAuthCookie(c, token, 30*24*time.Hour)

	// 回傳用戶與租戶資料（前端存入 localStorage）
	userData := fiber.Map{
		"id":    user.ID,
		"email": user.Email,
		"name":  user.Name,
	}
	if user.ProfilePic != "" {
		userData["profile_pic"] = user.ProfilePic
	}

	result := fiber.Map{
		"token": token,
		"user":  userData,
	}
	if tenantData != nil {
		result["tenant"] = tenantData
	}

	return c.JSON(result)
}

// CleanupExpiredSSOTickets 清理過期的 SSO 票據
// 可以在定時任務中呼叫
func CleanupExpiredSSOTickets() {
	result := database.DB.Where("expires_at < ? OR used = ?", time.Now().Add(-24*time.Hour), true).
		Delete(&models.SSOTicket{})
	if result.Error != nil {
		log.Printf("[SSO] Failed to cleanup tickets: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("[SSO] Cleaned up %d expired/used tickets", result.RowsAffected)
	}
}
