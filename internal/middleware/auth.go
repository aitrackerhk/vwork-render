package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/url"
	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// TenantMiddleware 從請求中提取租戶信息
func TenantMiddleware(c *fiber.Ctx) error {
	// /api/v1/public/* 是公開 API，handler 自行透過 :subdomain 參數查詢租戶，不需要此中間件
	if strings.HasPrefix(c.Path(), "/api/v1/public/") {
		return c.Next()
	}
	// /api/v1/vworkadmin/* 是 vWork 管理後台 API，不需要租戶上下文
	if strings.HasPrefix(c.Path(), "/api/v1/vworkadmin/") {
		return c.Next()
	}
	// 如果 AuthMiddleware 已經設置了 tenant_id（從 JWT token），直接使用
	tenantID := GetTenantID(c)
	if tenantID != uuid.Nil {
		// 已經有 tenant_id，查詢租戶信息（如果需要）
		var tenant models.Tenant
		if err := database.DB.Where("id = ? AND status = ?", tenantID, "active").First(&tenant).Error; err == nil {
			c.Locals("tenant", tenant)
		}
		return c.Next()
	}

	// 如果沒有從 JWT 獲取到，嘗試從子域名或 header 獲取租戶
	subdomain := c.Get("X-Tenant-Subdomain")
	if subdomain == "" {
		host := c.Hostname()
		parts := strings.Split(host, ".")
		if len(parts) > 0 && parts[0] != "localhost" && parts[0] != "127" {
			subdomain = parts[0]
		}
	}

	if subdomain == "" {
		// 嘗試從已登入用戶補齊 tenant（避免前端未帶子域名 header 時 400）
		if userID, ok := c.Locals("user_id").(uuid.UUID); ok && userID != uuid.Nil {
			var user models.User
			if err := database.DB.Select("tenant_id").Where("id = ?", userID).First(&user).Error; err == nil {
				if user.TenantID != nil && *user.TenantID != uuid.Nil {
					var tenant models.Tenant
					if err := database.DB.Where("id = ? AND status = ?", *user.TenantID, "active").First(&tenant).Error; err == nil {
						c.Locals("tenant_id", tenant.ID)
						c.Locals("tenant", tenant)
						return c.Next()
					}
				}
			}
		}

		// 允許租戶設置 API 通過（用戶還沒有租戶時必須能訪問）
		path := c.Path()
		if path == "/api/v1/tenant/setup" {
			log.Printf("🔵 TenantMiddleware: Allowing /api/v1/tenant/setup to pass (no subdomain)")
			return c.Next()
		}
		// 對於 API 請求，如果沒有租戶信息，返回錯誤
		accept := c.Get("Accept")
		if strings.Contains(accept, "application/json") || strings.HasPrefix(path, "/api") {
			return c.Status(400).JSON(fiber.Map{
				"error": "Tenant subdomain is required",
			})
		}
		// 對於網頁請求，允許繼續（可能不需要租戶）
		return c.Next()
	}

	// 允許租戶設置 API 通過（即使有 subdomain 但查詢不到租戶，用戶還沒有租戶時必須能訪問）
	path := c.Path()
	if path == "/api/v1/tenant/setup" {
		log.Printf("🔵 TenantMiddleware: Allowing /api/v1/tenant/setup to pass (before tenant lookup)")
		return c.Next()
	}

	// 查詢租戶
	var tenant models.Tenant
	if err := database.DB.Where("subdomain = ? AND status = ?", subdomain, "active").First(&tenant).Error; err != nil {
		// 對於某些公開 API（如獲取行業模板列表），即使找不到租戶也應該允許訪問
		// 因為用戶可能在創建租戶之前需要查看模板
		publicPaths := []string{
			"/api/v1/industry-templates",
			"/api/v1/sales-partner/apply",
		}
		for _, publicPath := range publicPaths {
			if strings.HasPrefix(path, publicPath) {
				log.Printf("🔵 TenantMiddleware: Allowing public API %s to pass (tenant not found)", path)
				return c.Next()
			}
		}

		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 將租戶信息存儲到 locals
	c.Locals("tenant_id", tenant.ID)
	c.Locals("tenant", tenant)

	return c.Next()
}

// AuthMiddleware JWT 認證中間件
func AuthMiddleware(c *fiber.Ctx) error {
	// 跳過公開路徑
	path := c.Path()
	// /api/v1/public/* 全部是公開 API（public chat、customer auth、checkout 等），
	// 這些路由直接在 app 上註冊，不需要認證；但 api Group("/api/v1") 的中間件
	// 會先攔截所有 /api/v1/* 請求，所以必須在此明確跳過。
	if strings.HasPrefix(path, "/api/v1/public/") {
		return c.Next()
	}
	if path == "/sales-partner" || strings.HasPrefix(path, "/api/v1/sales-partner/") || path == "/vworkadmin" || strings.HasPrefix(path, "/api/v1/vworkadmin/") {
		return c.Next()
	}
	// 公開 Landing / 內容頁面 — 不需要認證
	// vWork 平台 Blog、vMarket、租戶公開頁面、條款/隱私、多語系前綴、平台 Blog API
	if strings.HasPrefix(path, "/vwork-blog") ||
		strings.HasPrefix(path, "/vwork-events") ||
		strings.HasPrefix(path, "/vmarket") ||
		strings.HasPrefix(path, "/co/") ||
		strings.HasPrefix(path, "/industry/") ||
		strings.HasPrefix(path, "/custom/") ||
		path == "/enterprise-custom" ||
		path == "/terms" || path == "/privacy" ||
		strings.HasPrefix(path, "/en/") ||
		strings.HasPrefix(path, "/zh-cn/") ||
		path == "/api/v1/platform-blogs" ||
		path == "/api/v1/platform-events" {
		return c.Next()
	}
	// vOffice check-update — exact match only (NOT prefix), so /check-update/auth still requires auth
	if path == "/api/v1/voffice/check-update" {
		return c.Next()
	}
	// vOffice latest-release — public API for landing page and download page
	if path == "/api/v1/voffice/latest-release" {
		return c.Next()
	}
	// VMarket domain (vmarketai.com) serves /products, /services, /companies,
	// /map and /join as public marketplace pages — no auth required.  We inline
	// the hostname check here instead of importing handlers to avoid a circular
	// dependency.
	if path == "/products" || path == "/services" || path == "/companies" || path == "/map" || path == "/join" {
		host := strings.ToLower(c.Hostname())
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		if host == "vmarketai.com" || host == "www.vmarketai.com" {
			return c.Next()
		}
	}

	publicPaths := []string{
		"/", "/login", "/contact", "/help", "/sales-partner", "/vworkadmin",
		"/reset-password", "/accept-invite",
		"/api/v1/auth/login", "/api/v1/auth/register", "/api/v1/auth/forgot-password", "/api/v1/auth/reset-password",
		"/api/v1/auth/logout", // Logout must be reachable even with an invalid/expired token
		"/api/v1/auth/google", // Google OAuth 相關端點（包括 /api/v1/auth/google/config 和 /api/v1/auth/google）
		"/api/v1/auth/validate-invite",
		"/api/v1/contact",
		"/api/v1/sso/validate",
		"/api/v1/sales-partner/apply",
		"/api/v1/vworkadmin",
		"/api/v1/countries", "/api/v1/country-regions",
		"/api/v1/ad-config",
		"/api/v1/carousel", // carousel update check (public)
		// /api/v1/phone-country-codes 必須走 Auth + Tenant（CMS 表單/Select2 會用到，依 JWT 的 tenant_id 取資料）
		// 前台公開請改用：/api/v1/public/:subdomain/phone-country-codes
	}
	for _, publicPath := range publicPaths {
		if path == publicPath || strings.HasPrefix(path, publicPath+"/") {
			return c.Next()
		}
	}

	unauthorized := func(jsonMsg string) error {
		accept := c.Get("Accept")
		// 對於 API 路徑（以 /api 開頭），始終返回 JSON，不論 Accept header
		if strings.HasPrefix(path, "/api") {
			return c.Status(401).JSON(fiber.Map{
				"error": jsonMsg,
			})
		}
		// 瀏覽器頁面請求：導去登入頁，並清掉舊 token（避免反覆帶著壞 cookie）
		if strings.Contains(accept, "text/html") {
			c.ClearCookie("auth_token")
			if path != "/login" {
				// Preserve query parameters (e.g. ?product=vai) when redirecting to login
				loginURL := "/login"
				if rawQuery := string(c.Request().URI().QueryString()); rawQuery != "" {
					loginURL += "?" + rawQuery
				} else {
					// Infer product from path prefix so the login page knows
					// which product the user was trying to access and can
					// redirect back after authentication.
					switch {
					case strings.HasPrefix(path, "/vai-"):
						loginURL += "?product=vai"
					case strings.HasPrefix(path, "/voffice-"):
						loginURL += "?product=voffice"
					case strings.HasPrefix(path, "/vmarket-"):
						loginURL += "?product=vmarket"
					}
				}
				return c.Redirect(loginURL)
			}
			return c.Next()
		}
		// 其他情況：回 401 JSON
		return c.Status(401).JSON(fiber.Map{
			"error": jsonMsg,
		})
	}

	var tokenString string

	// 優先從 Authorization header 獲取 token（API 請求）
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			// 不是 Bearer 格式，嘗試直接使用
			tokenString = authHeader
		}
	} else {
		// 如果沒有 header，嘗試從 cookie 獲取（網頁請求）
		tokenString = c.Cookies("auth_token")
	}

	// ── Server-side SSO ticket handling ──
	// When a user arrives via app-switcher with ?sso_ticket=xxx but no auth
	// cookie (first visit to this domain), validate the ticket server-side,
	// set the auth cookie, and redirect to the same URL without the ticket
	// parameter.  This prevents AuthMiddleware from redirecting to /login
	// before sso.js ever gets a chance to run.
	if tokenString == "" {
		if ssoTicket := c.Query("sso_ticket"); ssoTicket != "" {
			_, err := validateSSOTicketServerSide(c, ssoTicket)
			if err != nil {
				log.Printf("[SSO-MW] Server-side ticket validation failed: %v", err)
				// Fall through to normal unauthorized handling — sso.js may
				// still attempt client-side validation on the login page.
			} else {
				// Cookie is already set by validateSSOTicketServerSide.
				// Redirect to the same URL without sso_ticket so the browser
				// re-sends the request with the new cookie.
				cleanURL := stripQueryParam(c, "sso_ticket")
				return c.Redirect(cleanURL)
			}
		}
	}

	// 如果還是沒有 token，檢查是否是網頁請求
	if tokenString == "" {
		return unauthorized("Authorization header or cookie is required")
	}

	// ── System API Token 分支 ──
	// 以 "vwk_" 前綴識別系統級 API token（用於 v01、vOffice、第三方整合）
	if strings.HasPrefix(tokenString, "vwk_") {
		return authenticateSystemToken(c, tokenString, unauthorized)
	}

	// ── JWT Token 分支（用戶登入）──
	// 驗證 token
	claims, err := utils.ValidateToken(tokenString)
	if err != nil {
		// Token 無效或過期
		return unauthorized("Invalid or expired token")
	}

	// 驗證 claims 數據完整性（允許 TenantID 為 Nil，用於尚未設置租戶的用戶）
	if claims.UserID == uuid.Nil || claims.Email == "" {
		return unauthorized("Incomplete session data")
	}

	// 將用戶信息存儲到 locals（先存儲，以便 TenantMiddleware 使用）
	c.Locals("user_id", claims.UserID)
	c.Locals("user_email", claims.Email)
	c.Locals("user_role", claims.Role)

	// 如果 TenantID 不是 Nil，存儲它
	if claims.TenantID != uuid.Nil {
		c.Locals("tenant_id", claims.TenantID)
	}

	// 重要：JWT 只代表「曾經登入過」，仍需要每次請求確認 user 仍存在且未停用/未刪除
	var user models.User
	// 如果 TenantID 是 Nil，只查詢用戶（不檢查 tenant_id）
	if claims.TenantID == uuid.Nil {
		if err := database.DB.Select("id", "tenant_id", "status", "logged_out_at", "web_logged_out_at", "desktop_logged_out_at").
			Where("id = ?", claims.UserID).
			First(&user).Error; err != nil {
			return unauthorized("User not found")
		}
		// 如果用戶沒有租戶，且訪問的不是設置頁面，允許繼續（中間件會處理重定向）
		if user.TenantID == nil || *user.TenantID == uuid.Nil {
			// 允許訪問 /setup-tenant 頁面
			if path == "/setup-tenant" {
				return c.Next()
			}
			// 其他頁面會在後續中間件中重定向
			return c.Next()
		}
		// 用戶有 tenant_id，但 JWT 中沒有：設置到 locals（修復 400 錯誤）
		c.Locals("tenant_id", *user.TenantID)
	} else {
		// 有 TenantID，正常檢查
		if err := database.DB.Select("id", "tenant_id", "status", "logged_out_at", "web_logged_out_at", "desktop_logged_out_at").
			Where("id = ? AND tenant_id = ?", claims.UserID, claims.TenantID).
			First(&user).Error; err != nil {
			return unauthorized("User not found")
		}
	}

	if strings.TrimSpace(strings.ToLower(user.Status)) != "active" {
		return unauthorized("Account is inactive")
	}

	// ── SSO Platform-Aware Logout check ──
	// Check logout timestamp based on the token's platform claim.
	// "desktop" tokens are checked against DesktopLoggedOutAt;
	// "web" (or empty/legacy) tokens are checked against WebLoggedOutAt,
	// then fallback to the legacy LoggedOutAt field.
	if claims.IssuedAt != nil {
		var loggedOutAt *time.Time
		if claims.Platform == "desktop" {
			loggedOutAt = user.DesktopLoggedOutAt
		} else {
			// web or legacy token: prefer WebLoggedOutAt, fallback to LoggedOutAt
			loggedOutAt = user.WebLoggedOutAt
			if loggedOutAt == nil {
				loggedOutAt = user.LoggedOutAt
			}
		}
		if loggedOutAt != nil && claims.IssuedAt.Time.Before(*loggedOutAt) {
			// Clear the stale cookie so the browser stops sending it
			c.ClearCookie("auth_token")
			return unauthorized("Session expired (logged out)")
		}
	}

	// 如果有 TenantID，檢查租戶
	if claims.TenantID != uuid.Nil {
		var tenant models.Tenant
		if err := database.DB.Select("id", "status").
			Where("id = ?", claims.TenantID).
			First(&tenant).Error; err != nil {
			return unauthorized("Tenant not found")
		}
		tenantStatus := strings.TrimSpace(strings.ToLower(tenant.Status))
		// 允許 suspended 和 trial_expired 的租戶仍維持登入狀態（例如試用過期/需付款時）
		// 真正的功能鎖定交由 TrialExpiredMiddleware 處理（會把使用者導到 /subscription-required，並放行 billing API）
		if tenantStatus != "" && tenantStatus != "active" && tenantStatus != "suspended" && tenantStatus != "trial_expired" {
			return unauthorized("Tenant is inactive")
		}

		// 驗證租戶 ID 是否匹配（如果 TenantMiddleware 已經設置了租戶）
		tenantID := GetTenantID(c)
		if tenantID != uuid.Nil && claims.TenantID != tenantID {
			// 租戶不匹配：回 403 Forbidden（而非 401），因為 session 本身是有效的，
			// 只是 token 中的 tenant 與請求的 tenant 不一致。
			// 使用 401 會導致前端誤判為 session 失效而強制登出。
			accept := c.Get("Accept")
			if strings.HasPrefix(path, "/api") || strings.Contains(accept, "application/json") {
				return c.Status(403).JSON(fiber.Map{
					"error": "Tenant mismatch",
				})
			}
			return c.Status(403).JSON(fiber.Map{
				"error": "Tenant mismatch",
			})
		}
	}

	return c.Next()
}

// authenticateSystemToken validates a system-level API token (prefix "vwk_").
// On success it populates c.Locals with tenant_id, user_id (creator), user_role="system",
// and sets "is_system_token"=true so downstream handlers can distinguish.
func authenticateSystemToken(c *fiber.Ctx, rawToken string, unauthorized func(string) error) error {
	// Hash the raw token and look it up
	h := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(h[:])

	var apiToken models.APIToken
	if err := database.DB.
		Where("token_hash = ?", tokenHash).
		First(&apiToken).Error; err != nil {
		return unauthorized("Invalid API token")
	}

	// Check status
	if apiToken.Status != "active" {
		return unauthorized("API token has been revoked")
	}

	// Check expiry
	if apiToken.ExpiresAt != nil && time.Now().After(*apiToken.ExpiresAt) {
		return unauthorized("API token has expired")
	}

	// Verify tenant is still active
	var tenant models.Tenant
	if err := database.DB.Select("id", "status").
		Where("id = ?", apiToken.TenantID).
		First(&tenant).Error; err != nil {
		return unauthorized("Tenant not found")
	}
	tenantStatus := strings.TrimSpace(strings.ToLower(tenant.Status))
	if tenantStatus != "" && tenantStatus != "active" && tenantStatus != "suspended" && tenantStatus != "trial_expired" {
		return unauthorized("Tenant is inactive")
	}

	// Populate context — downstream handlers work the same as with JWT
	c.Locals("tenant_id", apiToken.TenantID)
	c.Locals("user_id", apiToken.CreatedByID) // attribute actions to the token creator
	c.Locals("user_email", "system")          // placeholder for system token
	c.Locals("user_role", "admin")            // system tokens get admin-level access
	c.Locals("is_system_token", true)
	c.Locals("api_token_id", apiToken.ID)
	c.Locals("api_token_scopes", apiToken.Scopes)

	// Update last_used_at asynchronously (fire-and-forget to avoid slowing down requests)
	go func(id uuid.UUID) {
		now := time.Now()
		database.DB.Model(&models.APIToken{}).Where("id = ?", id).Update("last_used_at", now)
	}(apiToken.ID)

	return c.Next()
}

// IsSystemToken returns true if the current request was authenticated via a system API token.
func IsSystemToken(c *fiber.Ctx) bool {
	v, ok := c.Locals("is_system_token").(bool)
	return ok && v
}

// RequireRole 檢查用戶角色
func RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole, ok := c.Locals("user_role").(string)
		if !ok {
			return c.Status(403).JSON(fiber.Map{
				"error": "Unauthorized",
			})
		}

		hasRole := false
		for _, role := range roles {
			if userRole == role {
				hasRole = true
				break
			}
		}

		if !hasRole {
			return c.Status(403).JSON(fiber.Map{
				"error": "Insufficient permissions",
			})
		}

		return c.Next()
	}
}

// GetUserID 從 context 獲取用戶 ID
func GetUserID(c *fiber.Ctx) uuid.UUID {
	if userID, ok := c.Locals("user_id").(uuid.UUID); ok {
		return userID
	}
	return uuid.Nil
}

// GetTenantID 從 context 獲取租戶 ID
func GetTenantID(c *fiber.Ctx) uuid.UUID {
	if tenantID, ok := c.Locals("tenant_id").(uuid.UUID); ok {
		return tenantID
	}
	return uuid.Nil
}

// ──────────────────────────────────────────────────────────
// SSO server-side helpers (used by AuthMiddleware)
// ──────────────────────────────────────────────────────────

// validateSSOTicketServerSide validates an SSO ticket in the middleware layer,
// sets the auth_token cookie on success, and returns the JWT token string.
// This mirrors the logic in handlers.ValidateSSOTicket but runs inside
// AuthMiddleware so the user doesn't get redirected to /login first.
func validateSSOTicketServerSide(c *fiber.Ctx, ticket string) (string, error) {
	// 1. Look up the ticket
	var ssoTicket models.SSOTicket
	if err := database.DB.Where("ticket = ?", ticket).First(&ssoTicket).Error; err != nil {
		return "", err
	}

	// 2. Check if already used
	if ssoTicket.Used {
		return "", fiber.NewError(401, "Ticket already used")
	}

	// 3. Check expiry
	if time.Now().After(ssoTicket.ExpiresAt) {
		database.DB.Model(&ssoTicket).Update("used", true)
		return "", fiber.NewError(401, "Ticket expired")
	}

	// 4. Mark as used (one-time)
	database.DB.Model(&ssoTicket).Update("used", true)

	// 5. Look up the user
	var user models.User
	if err := database.DB.Preload("Role").Where("id = ? AND status = ?", ssoTicket.UserID, "active").First(&user).Error; err != nil {
		return "", fiber.NewError(401, "User not found or inactive")
	}

	// 6. Determine tenant
	tenantID := uuid.Nil
	if ssoTicket.TenantID != nil {
		tenantID = *ssoTicket.TenantID
	} else if user.TenantID != nil {
		tenantID = *user.TenantID
	}

	// 7. Generate JWT
	roleName := user.UserRole
	if user.Role != nil {
		roleName = user.Role.Name
	}
	token, err := utils.GenerateToken(user.ID, tenantID, user.Email, roleName, "web")
	if err != nil {
		return "", err
	}

	// 8. Set auth cookie on the response
	secure := false
	scheme := strings.ToLower(strings.TrimSpace(os.Getenv("PUBLIC_SCHEME")))
	if scheme == "https" {
		secure = true
	} else {
		env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
		secure = env == "prod" || env == "production"
	}
	c.Cookie(&fiber.Cookie{
		Name:     "auth_token",
		Value:    token,
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: "Lax",
		Path:     "/",
	})

	log.Printf("[SSO-MW] Server-side SSO login successful for user %s (tenant=%s)", user.Email, tenantID)
	return token, nil
}

// stripQueryParam removes the named query parameter from the current request
// URL and returns the resulting URL string (path + remaining query).
func stripQueryParam(c *fiber.Ctx, param string) string {
	originalURL := string(c.Request().URI().RequestURI())
	u, err := url.Parse(originalURL)
	if err != nil {
		// Fallback: just return the path
		return c.Path()
	}
	q := u.Query()
	q.Del(param)
	u.RawQuery = q.Encode()
	return u.String()
}
