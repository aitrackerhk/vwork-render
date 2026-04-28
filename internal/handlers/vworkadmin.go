package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/models"
	"nwork/internal/utils"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// getAdminCredentials returns admin username/password from env vars with fallback defaults.
func getAdminCredentials() (string, string) {
	user := os.Getenv("VWORK_ADMIN_USER")
	if user == "" {
		user = "vadmin"
	}
	pass := os.Getenv("VWORK_ADMIN_PASS")
	if pass == "" {
		pass = "Vvhk_9634"
	}
	return user, pass
}

// getAdminCookieSecret returns the HMAC key used to sign the vworkadmin_auth cookie.
// Falls back to JWT_SECRET if VWORK_ADMIN_COOKIE_SECRET is not set.
func getAdminCookieSecret() []byte {
	secret := os.Getenv("VWORK_ADMIN_COOKIE_SECRET")
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	if secret == "" {
		secret = "vworkadmin-default-secret-change-in-production"
	}
	return []byte(secret)
}

// signAdminCookie generates an HMAC-SHA256 signature for the admin cookie value.
func signAdminCookie(value string) string {
	mac := hmac.New(sha256.New, getAdminCookieSecret())
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyAdminCookie checks if the cookie value matches its HMAC signature.
// Expected format: "payload|signature"
func verifyAdminCookie(cookieValue string) bool {
	parts := strings.SplitN(cookieValue, "|", 2)
	if len(parts) != 2 {
		return false
	}
	payload, sig := parts[0], parts[1]
	expected := signAdminCookie(payload)
	return hmac.Equal([]byte(sig), []byte(expected))
}

// isVWorkAdmin validates the vworkadmin_auth cookie using HMAC signature verification.
func isVWorkAdmin(c *fiber.Ctx) bool {
	cookie := c.Cookies("vworkadmin_auth")
	if cookie == "" {
		return false
	}
	return verifyAdminCookie(cookie)
}

func VWorkAdminPage(c *fiber.Ctx) error {
	return c.Render("pages/vworkadmin", fiber.Map{
		"Authed": isVWorkAdmin(c),
	}, "layouts/blank") // 使用簡單空白布局
}

func VWorkAdminLogin(c *fiber.Ctx) error {
	type req struct {
		Username string `json:"username" form:"username"`
		Password string `json:"password" form:"password"`
	}
	var r req
	if err := c.BodyParser(&r); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}
	adminUser, adminPass := getAdminCredentials()
	if r.Username != adminUser || r.Password != adminPass {
		return c.Status(401).JSON(fiber.Map{"error": "帳號或密碼錯誤"})
	}

	// Generate a signed cookie value: "timestamp|hmac"
	payload := fmt.Sprintf("vworkadmin:%d", time.Now().Unix())
	sig := signAdminCookie(payload)
	cookieValue := payload + "|" + sig

	c.Cookie(&fiber.Cookie{
		Name:     "vworkadmin_auth",
		Value:    cookieValue,
		Path:     "/",
		HTTPOnly: true,
		Secure:   false, // set to true in production for HTTPS
		SameSite: "Lax",
		Expires:  time.Now().Add(24 * time.Hour),
	})
	return c.JSON(fiber.Map{"success": true})
}

func VWorkAdminLogout(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "vworkadmin_auth",
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		Expires:  time.Now().Add(-1 * time.Hour),
	})
	return c.Redirect("/vworkadmin")
}

type tenantOverview struct {
	ID                 uuid.UUID  `json:"id"`
	Name               string     `json:"name"`
	Subdomain          string     `json:"subdomain"`
	Plan               string     `json:"plan"`
	Status             string     `json:"status"`
	TrialExpiresAt     *time.Time `json:"trial_expires_at"`
	SubscriptionID     *string    `json:"subscription_id"`
	StripeCustomerID   *string    `json:"stripe_customer_id"`
	CurrentPlan        string     `json:"current_plan"`
	CurrentInterval    string     `json:"current_interval"`
	CurrentPeriodStart *time.Time `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end"`
	CurrentPeriodAmt   float64    `json:"current_period_amount"`
	TotalPaid          float64    `json:"total_paid_estimate"`
	SalesPartnerCode   string     `json:"sales_partner_code"`
	SalesPartnerTrial  bool       `json:"sales_partner_trial"`
	CreatedByName      string     `json:"created_by_name"`
	CreatedByEmail     string     `json:"created_by_email"`
}

type userOverview struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Status      string     `json:"status"`
	UserRole    string     `json:"user_role"`
	RoleName    string     `json:"role_name"`
	TenantID    *uuid.UUID `json:"tenant_id"`
	TenantName  string     `json:"tenant_name"`
	Phone       string     `json:"phone"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func VWorkAdminOverview(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var tenants []models.Tenant
	if err := database.DB.Order("created_at DESC").Find(&tenants).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to load tenants"})
	}

	// Build a map of tenant_id -> creator (earliest user_tenants record per tenant)
	type creatorInfo struct {
		Name  string
		Email string
	}
	creatorMap := map[uuid.UUID]creatorInfo{}
	if len(tenants) > 0 {
		tenantIDs := make([]uuid.UUID, 0, len(tenants))
		for _, t := range tenants {
			tenantIDs = append(tenantIDs, t.ID)
		}
		// Query earliest user_tenant per tenant using a subquery for min created_at
		type creatorRow struct {
			TenantID uuid.UUID
			UserName string
			Email    string
		}
		var rows []creatorRow
		database.DB.Raw(`
			SELECT ut.tenant_id, u.name AS user_name, u.email
			FROM user_tenants ut
			JOIN users u ON u.id = ut.user_id
			WHERE ut.tenant_id IN ?
			  AND ut.created_at = (
			    SELECT MIN(ut2.created_at)
			    FROM user_tenants ut2
			    WHERE ut2.tenant_id = ut.tenant_id
			  )
		`, tenantIDs).Scan(&rows)
		for _, r := range rows {
			creatorMap[r.TenantID] = creatorInfo{Name: r.UserName, Email: r.Email}
		}
	}

	overviews := make([]tenantOverview, 0, len(tenants))
	for _, t := range tenants {
		var sub models.Subscription
		_ = database.DB.
			Where("tenant_id = ?", t.ID).
			Preload("Plan").
			Order("current_period_end DESC NULLS LAST, created_at DESC").
			First(&sub).Error

		currentPlan := ""
		currentInterval := ""
		currentAmt := 0.0
		if sub.ID != uuid.Nil && sub.Plan.ID != uuid.Nil {
			currentPlan = sub.Plan.DisplayName
			currentInterval = sub.Plan.Interval
			if sub.Plan.Interval == "year" && sub.Plan.YearlyPrice > 0 {
				currentAmt = sub.Plan.YearlyPrice
			} else {
				currentAmt = sub.Plan.Price
			}
		}

		// Extract sales partner info from ExtraFields
		spCode := ""
		spTrial := false
		if t.ExtraFields != nil {
			if v, ok := t.ExtraFields["sales_partner_code"]; ok {
				if s, ok := v.(string); ok {
					spCode = s
				}
			}
			if v, ok := t.ExtraFields["sales_partner_trial"]; ok {
				if b, ok := v.(bool); ok {
					spTrial = b
				}
			}
		}

		overviews = append(overviews, tenantOverview{
			ID:                 t.ID,
			Name:               t.Name,
			Subdomain:          t.Subdomain,
			Plan:               t.Plan,
			Status:             t.Status,
			TrialExpiresAt:     t.TrialExpiresAt,
			SubscriptionID:     t.SubscriptionID,
			StripeCustomerID:   t.StripeCustomerID,
			CurrentPlan:        currentPlan,
			CurrentInterval:    currentInterval,
			CurrentPeriodStart: sub.CurrentPeriodStart,
			CurrentPeriodEnd:   sub.CurrentPeriodEnd,
			CurrentPeriodAmt:   currentAmt,
			TotalPaid:          currentAmt, // 簡化估算（可後續接 Stripe 發票累計）
			SalesPartnerCode:   spCode,
			SalesPartnerTrial:  spTrial,
			CreatedByName:      creatorMap[t.ID].Name,
			CreatedByEmail:     creatorMap[t.ID].Email,
		})
	}

	var emailJobs []models.EmailJob
	_ = database.DB.Order("created_at DESC").Limit(200).Find(&emailJobs).Error

	var pendingJobs []models.EmailJob
	_ = database.DB.
		Where("status IN ?", []string{"queued", "sending"}).
		Order("run_at ASC, created_at ASC").
		Limit(200).
		Find(&pendingJobs).Error

	var salesPartners []models.SalesPartnerApplication
	_ = database.DB.Order("created_at DESC").Find(&salesPartners).Error

	var users []models.User
	_ = database.DB.
		Preload("Role").
		Preload("Tenant").
		Order("created_at DESC").
		Limit(1000).
		Find(&users).Error

	userOverviews := make([]userOverview, 0, len(users))
	for _, u := range users {
		roleName := u.UserRole
		if u.Role != nil && strings.TrimSpace(u.Role.Name) != "" {
			roleName = u.Role.Name
		}
		tenantName := ""
		if u.Tenant.ID != uuid.Nil {
			tenantName = u.Tenant.Name
		}
		userOverviews = append(userOverviews, userOverview{
			ID:          u.ID,
			Name:        u.Name,
			Email:       u.Email,
			Status:      u.Status,
			UserRole:    u.UserRole,
			RoleName:    roleName,
			TenantID:    u.TenantID,
			TenantName:  tenantName,
			Phone:       u.Phone,
			LastLoginAt: u.LastLoginAt,
			CreatedAt:   u.CreatedAt,
		})
	}

	// 彙整 template（以 kind + subject 為參考）
	type tmpl struct {
		Kind    string `json:"kind"`
		Subject string `json:"subject"`
	}
	templatesMap := map[string]tmpl{}
	for _, job := range emailJobs {
		if _, ok := templatesMap[job.Kind]; !ok {
			templatesMap[job.Kind] = tmpl{Kind: job.Kind, Subject: job.Subject}
		}
	}
	templates := make([]tmpl, 0, len(templatesMap))
	for _, v := range templatesMap {
		templates = append(templates, v)
	}

	templatePreviews, _ := email.ListTemplatePreviews()

	return c.JSON(fiber.Map{
		"tenants":         overviews,
		"email_jobs":      emailJobs,
		"pending_jobs":    pendingJobs,
		"email_templates": templates,
		"email_previews":  templatePreviews,
		"sales_partners":  salesPartners,
		"users":           userOverviews,
	})
}

// VWorkAdminLoginAsUser 以指定用戶登入（設定 auth_token）
// Uses GenerateImpersonateToken to mark the token as admin-created.
// Does NOT update the target user's last_login_at to avoid side effects.
func VWorkAdminLoginAsUser(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		UserID   string `json:"user_id"`
		TenantID string `json:"tenant_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	userID, err := uuid.Parse(strings.TrimSpace(req.UserID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user_id"})
	}

	var user models.User
	if err := database.DB.Preload("Role").First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "user not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "failed to load user"})
	}

	var tenant models.Tenant
	if strings.TrimSpace(req.TenantID) != "" {
		tenantUUID, err := uuid.Parse(strings.TrimSpace(req.TenantID))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid tenant_id"})
		}
		var link models.UserTenant
		if err := database.DB.Preload("Tenant").Where("user_id = ? AND tenant_id = ?", user.ID, tenantUUID).First(&link).Error; err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "user not in tenant"})
		}
		if link.Tenant != nil {
			tenant = *link.Tenant
		} else {
			_ = database.DB.First(&tenant, "id = ?", tenantUUID).Error
		}
	} else if user.TenantID != nil && *user.TenantID != uuid.Nil {
		_ = database.DB.First(&tenant, "id = ?", *user.TenantID).Error
	} else {
		var link models.UserTenant
		if err := database.DB.Preload("Tenant").Where("user_id = ?", user.ID).
			Order("is_default DESC, last_used_at DESC NULLS LAST, created_at DESC").
			First(&link).Error; err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "tenant not found"})
		}
		if link.Tenant != nil {
			tenant = *link.Tenant
		} else {
			_ = database.DB.First(&tenant, "id = ?", link.TenantID).Error
		}
	}

	if tenant.ID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "tenant not found"})
	}

	roleName := user.UserRole
	if user.Role != nil && strings.TrimSpace(user.Role.Name) != "" {
		roleName = user.Role.Name
	}

	// Generate an impersonate token (marked with "vworkadmin" as the impersonator).
	// NOTE: We intentionally do NOT update last_login_at or tenant_id on the user
	// to avoid polluting the user's real login history.
	token, err := utils.GenerateImpersonateToken(user.ID, tenant.ID, user.Email, roleName, "web", "vworkadmin")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to generate token"})
	}

	setAuthCookie(c, token, 30*24*time.Hour)

	// Enhanced audit log — includes admin IP address
	clientIP := c.IP()
	utils.LogActivity(tenant.ID, user.ID, "login_as", "vworkadmin", nil,
		fmt.Sprintf(`{"key":"vworkadmin.login_as","params":{"name":%q,"email":%q,"admin_ip":%q}}`, user.Name, user.Email, clientIP), nil, c)

	return c.JSON(fiber.Map{
		"success":      true,
		"token":        token,
		"user_id":      user.ID,
		"tenant_id":    tenant.ID,
		"impersonated": true,
	})
}

// VWorkAdminHardwarePurchases 取得硬件購買記錄
func VWorkAdminHardwarePurchases(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var purchases []models.HardwarePurchase
	if err := database.DB.Order("created_at DESC").Limit(500).Find(&purchases).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to load purchases"})
	}

	// tenant name map
	var tenants []models.Tenant
	_ = database.DB.Select("id", "name", "subdomain").Find(&tenants).Error
	tenantMap := map[uuid.UUID]models.Tenant{}
	for _, t := range tenants {
		tenantMap[t.ID] = t
	}

	res := make([]fiber.Map, 0, len(purchases))
	for _, p := range purchases {
		items := []interface{}{}
		if raw, ok := p.Items["items"]; ok {
			items, _ = raw.([]interface{})
		}
		if len(items) == 0 {
			if raw, ok := p.Items["_data"]; ok {
				items, _ = raw.([]interface{})
			}
		}

		name := ""
		subdomain := ""
		if t, ok := tenantMap[p.TenantID]; ok {
			name = t.Name
			subdomain = t.Subdomain
		}

		res = append(res, fiber.Map{
			"id":                  p.ID,
			"tenant_id":           p.TenantID,
			"tenant_name":         name,
			"tenant_subdomain":    subdomain,
			"user_id":             p.UserID,
			"status":              p.Status,
			"checkout_session_id": p.CheckoutSessionID,
			"payment_intent_id":   p.PaymentIntentID,
			"currency":            p.Currency,
			"amount_total":        p.AmountTotal,
			"items":               items,
			"company_info":        p.CompanyInfo,
			"created_at":          p.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{"data": res})
}

// VWorkAdminGetPrompt 獲取 AI prompt
func VWorkAdminGetPrompt(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	defaultPrompt := "你是 Vai 助手，專門處理業務相關的事務。你只回答與業務、工作、商業相關的問題，不處理私人事務。如果用戶詢問私人問題，請禮貌地告知你只處理業務相關的事務。"
	prompt := models.GetSystemSetting("ai_system_prompt", defaultPrompt)

	return c.JSON(fiber.Map{
		"prompt": prompt,
	})
}

// VWorkAdminUpdatePrompt 更新 AI prompt
func VWorkAdminUpdatePrompt(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	if req.Prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "prompt cannot be empty"})
	}

	desc := "AI 助手的系統提示詞"
	if err := models.SetSystemSetting("ai_system_prompt", req.Prompt, &desc); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update prompt: " + err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "prompt": req.Prompt})
}

// ── Ad Sidebar Config ──────────────────────────────────────────────

// VWorkAdminGetAdConfig returns the current ad sidebar configuration.
func VWorkAdminGetAdConfig(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	enabled := models.GetSystemSetting("ad_sidebar_enabled", "true")
	mode := models.GetSystemSetting("ad_sidebar_mode", "both")                // both | adsense_only | vsuite_only
	vsuiteWeight := models.GetSystemSetting("ad_sidebar_vsuite_weight", "50") // 0-100 percentage

	return c.JSON(fiber.Map{
		"enabled":       enabled == "true",
		"mode":          mode,
		"vsuite_weight": vsuiteWeight,
	})
}

// VWorkAdminUpdateAdConfig updates the ad sidebar configuration.
func VWorkAdminUpdateAdConfig(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Enabled      *bool  `json:"enabled"`
		Mode         string `json:"mode"`
		VsuiteWeight *int   `json:"vsuite_weight"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	if req.Enabled != nil {
		val := "false"
		if *req.Enabled {
			val = "true"
		}
		desc := "Ad sidebar global on/off switch"
		if err := models.SetSystemSetting("ad_sidebar_enabled", val, &desc); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to update ad_sidebar_enabled"})
		}
	}

	if req.Mode != "" {
		validModes := map[string]bool{"both": true, "adsense_only": true, "vsuite_only": true}
		if !validModes[req.Mode] {
			return c.Status(400).JSON(fiber.Map{"error": "mode must be: both, adsense_only, or vsuite_only"})
		}
		desc := "Ad sidebar display mode (both/adsense_only/vsuite_only)"
		if err := models.SetSystemSetting("ad_sidebar_mode", req.Mode, &desc); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to update ad_sidebar_mode"})
		}
	}

	if req.VsuiteWeight != nil {
		w := *req.VsuiteWeight
		if w < 0 || w > 100 {
			return c.Status(400).JSON(fiber.Map{"error": "vsuite_weight must be between 0 and 100"})
		}
		val := fmt.Sprintf("%d", w)
		desc := "vSuite ad weight percentage (0-100) when mode=both"
		if err := models.SetSystemSetting("ad_sidebar_vsuite_weight", val, &desc); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to update ad_sidebar_vsuite_weight"})
		}
	}

	return c.JSON(fiber.Map{"success": true})
}

// PublicGetAdConfig returns ad config for the frontend (no auth required, minimal info).
func PublicGetAdConfig(c *fiber.Ctx) error {
	enabled := models.GetSystemSetting("ad_sidebar_enabled", "true")
	mode := models.GetSystemSetting("ad_sidebar_mode", "both")
	vsuiteWeight := models.GetSystemSetting("ad_sidebar_vsuite_weight", "50")

	return c.JSON(fiber.Map{
		"enabled":       enabled == "true",
		"mode":          mode,
		"vsuite_weight": vsuiteWeight,
	})
}

// VWorkAdminGetFieldMatchPrompt 獲取字段匹配 prompt
func VWorkAdminGetFieldMatchPrompt(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	defaultPrompt := `你是一個智能數據提取助手。請從以下文字中提取信息，並盡可能匹配到對應的字段。

文字內容：
%s

可用字段：
%s

匹配規則：
1. 使用模糊匹配，字段名稱和文字內容不需要完全一致
2. 支持同義詞、相似詞匹配（例如："姓名"可以匹配"名字"、"名稱"等）
3. 如果文字中包含字段相關的信息，即使不完全匹配也要嘗試提取
4. 盡可能匹配更多字段，不要過於嚴格
5. 如果字段名稱是英文，文字中的中文描述也可以匹配（例如：name 可以匹配"姓名"、"名字"等）
6. 對於數值、日期、電話等格式，盡可能識別並提取

請以 JSON 格式返回結果，格式如下：
{
  "matches": [
    {
      "field_name": "字段名稱（使用可用字段列表中的實際字段名稱）",
      "value": "提取的值",
      "confidence": 0.7
    }
  ]
}

請盡可能匹配所有相關字段，即使置信度較低也可以包含。confidence 是置信度（0-1之間的小數），建議設置在 0.5-1.0 之間。`
	prompt := models.GetSystemSetting("ai_field_match_prompt", defaultPrompt)

	return c.JSON(fiber.Map{
		"prompt": prompt,
	})
}

// VWorkAdminUpdateFieldMatchPrompt 更新字段匹配 prompt
func VWorkAdminUpdateFieldMatchPrompt(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	if req.Prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "prompt cannot be empty"})
	}

	desc := "AI 字段匹配的提示詞（用於 OCR/STT 表單輸入功能）"
	if err := models.SetSystemSetting("ai_field_match_prompt", req.Prompt, &desc); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update prompt: " + err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "prompt": req.Prompt})
}

type seoSettingItem struct {
	Key          string `json:"key"`
	Value        string `json:"value"`
	Description  string `json:"description"`
	DefaultValue string `json:"default_value"`
}

type seoSection struct {
	ID       string           `json:"id"`
	Label    string           `json:"label"`
	PageKey  string           `json:"page_key"`
	Settings []seoSettingItem `json:"settings"`
}

func seoSectionsDefinition() []seoSection {
	langs := []struct {
		Code  string
		Label string
	}{
		{Code: "zh", Label: "繁中"},
		{Code: "en", Label: "EN"},
		{Code: "zh-CN", Label: "簡中"},
	}

	// Default SEO values per page per language — mirrors the hardcoded values in NewLandingSEO / NewVMarketSEO
	type langDefaults struct {
		Title       string
		Description string
		Keywords    string
		Image       string
	}
	type pageDefaults struct {
		ID       string
		Label    string
		PageKey  string
		Defaults map[string]langDefaults // keyed by lang code
	}

	pages := []pageDefaults{
		{ID: "landing-vwork", Label: "Landing - vWork", PageKey: "landing.vwork", Defaults: map[string]langDefaults{
			"zh":    {Title: "vWork - AI 智能企業管理平台", Description: "vWork 提供 POS、庫存、訂單、客戶管理的一站式企業管理方案，協助企業提升營運效率。", Keywords: "vWork, ERP, POS, 庫存管理, 訂單管理, 企業管理系統", Image: "/static/vworkicon.png"},
			"en":    {Title: "vWork - AI Business Management Platform", Description: "vWork provides POS, inventory, order, and customer management in one platform to improve business efficiency.", Keywords: "vWork, ERP, POS, inventory management, order management, business software", Image: "/static/vworkicon.png"},
			"zh-CN": {Title: "vWork - AI 智能企业管理平台", Description: "vWork 提供 POS、库存、订单、客户管理的一站式企业管理方案，帮助企业提升运营效率。", Keywords: "vWork, ERP, POS, 库存管理, 订单管理, 企业管理系统", Image: "/static/vworkicon.png"},
		}},
		{ID: "landing-vsys", Label: "Landing - V-sys", PageKey: "landing.vsys", Defaults: map[string]langDefaults{
			"zh":    {Title: "V-sys - 企業數位化解決方案", Description: "V-sys 透過 AI 技術提供企業數位化解決方案，包含 vWork、vAI、vOffice 與 vMarket。", Keywords: "V-sys, AI, 數位化, 企業方案, vWork, vAI, vOffice, vMarket", Image: "/static/vicon.png"},
			"en":    {Title: "V-sys - Digital Solutions for Enterprises", Description: "V-sys delivers AI-powered digital solutions including vWork, vAI, vOffice, and vMarket.", Keywords: "V-sys, AI, digital transformation, enterprise solutions", Image: "/static/vicon.png"},
			"zh-CN": {Title: "V-sys - 企业数字化解决方案", Description: "V-sys 通过 AI 技术提供企业数字化解决方案，涵盖 vWork、vAI、vOffice 和 vMarket。", Keywords: "V-sys, AI, 数字化, 企业方案, vWork, vAI, vOffice, vMarket", Image: "/static/vicon.png"},
		}},
		{ID: "landing-vai", Label: "Landing - vAI", PageKey: "landing.vai", Defaults: map[string]langDefaults{
			"zh":    {Title: "vAI - 智能 AI 助手", Description: "vAI 協助企業快速完成對話、文件、圖片與影片工作流程，提升日常營運效率。", Keywords: "vAI, AI 助手, 智能客服, AI 文件, AI 圖片, AI 影片", Image: "/static/vaiicon.png"},
			"en":    {Title: "vAI - Intelligent AI Assistant", Description: "vAI helps teams handle chat, documents, images, and videos with faster AI workflows.", Keywords: "vAI, AI assistant, AI chat, AI documents, AI image, AI video", Image: "/static/vaiicon.png"},
			"zh-CN": {Title: "vAI - 智能 AI 助手", Description: "vAI 帮助企业更快完成对话、文档、图片和视频工作流程，提升日常效率。", Keywords: "vAI, AI 助手, 智能客服, AI 文档, AI 图片, AI 视频", Image: "/static/vaiicon.png"},
		}},
		{ID: "landing-voffice", Label: "Landing - vOffice", PageKey: "landing.voffice", Defaults: map[string]langDefaults{
			"zh":    {Title: "vOffice - 智能辦公套件", Description: "vOffice 提供文件、表格與簡報的辦公能力，並整合 AI 助手，提升團隊協作效率。", Keywords: "vOffice, 辦公套件, 文件編輯, 表格, 簡報, AI 辦公", Image: "/static/vofficeicon.png"},
			"en":    {Title: "vOffice - Smart Office Suite", Description: "vOffice offers documents, spreadsheets, and presentations with built-in AI for better team productivity.", Keywords: "vOffice, office suite, documents, spreadsheets, presentations, AI office", Image: "/static/vofficeicon.png"},
			"zh-CN": {Title: "vOffice - 智能办公套件", Description: "vOffice 提供文档、表格与演示文稿能力，并整合 AI 助手，提升团队协作效率。", Keywords: "vOffice, 办公套件, 文档编辑, 表格, 演示, AI 办公", Image: "/static/vofficeicon.png"},
		}},
		{ID: "vmarket-home", Label: "vMarket - Home", PageKey: "vmarket.home", Defaults: map[string]langDefaults{
			"zh":    {Title: "vMarket - 商家與服務平台", Description: "vMarket 匯集優質商家與商品服務，協助使用者快速搜尋、比較與找到合適的合作夥伴。", Keywords: "vMarket, 商家平台, 商品搜尋, 服務搜尋, 企業配對", Image: "/static/vmarketicon.png"},
			"en":    {Title: "vMarket - Business & Services Platform", Description: "vMarket aggregates quality merchants and services, helping users search, compare, and find the right partners.", Keywords: "vMarket, business platform, product search, service search, enterprise matching", Image: "/static/vmarketicon.png"},
			"zh-CN": {Title: "vMarket - 商家与服务平台", Description: "vMarket 汇集优质商家与商品服务，帮助用户快速搜索、比较与找到合适的合作伙伴。", Keywords: "vMarket, 商家平台, 商品搜索, 服务搜索, 企业配对", Image: "/static/vmarketicon.png"},
		}},
		{ID: "vmarket-products", Label: "vMarket - Products", PageKey: "vmarket.products", Defaults: map[string]langDefaults{
			"zh":    {Title: "vMarket 商品", Description: "瀏覽 vMarket 上的商品，搜尋比較最適合的產品。", Keywords: "vMarket, 商品, 產品搜尋", Image: "/static/vmarketicon.png"},
			"en":    {Title: "vMarket Products", Description: "Browse products on vMarket and find the best fit.", Keywords: "vMarket, products, product search", Image: "/static/vmarketicon.png"},
			"zh-CN": {Title: "vMarket 商品", Description: "浏览 vMarket 上的商品，搜索比较最适合的产品。", Keywords: "vMarket, 商品, 产品搜索", Image: "/static/vmarketicon.png"},
		}},
		{ID: "vmarket-services", Label: "vMarket - Services", PageKey: "vmarket.services", Defaults: map[string]langDefaults{
			"zh":    {Title: "vMarket 服務", Description: "瀏覽 vMarket 上的服務項目，快速找到合適的服務提供者。", Keywords: "vMarket, 服務, 服務搜尋", Image: "/static/vmarketicon.png"},
			"en":    {Title: "vMarket Services", Description: "Browse services on vMarket and find the right service providers.", Keywords: "vMarket, services, service search", Image: "/static/vmarketicon.png"},
			"zh-CN": {Title: "vMarket 服务", Description: "浏览 vMarket 上的服务项目，快速找到合适的服务提供者。", Keywords: "vMarket, 服务, 服务搜索", Image: "/static/vmarketicon.png"},
		}},
		{ID: "vmarket-companies", Label: "vMarket - Companies", PageKey: "vmarket.companies", Defaults: map[string]langDefaults{
			"zh":    {Title: "vMarket 商家", Description: "瀏覽 vMarket 上的商家，尋找值得信賴的合作夥伴。", Keywords: "vMarket, 商家, 企業搜尋", Image: "/static/vmarketicon.png"},
			"en":    {Title: "vMarket Companies", Description: "Browse companies on vMarket and find trusted business partners.", Keywords: "vMarket, companies, business search", Image: "/static/vmarketicon.png"},
			"zh-CN": {Title: "vMarket 商家", Description: "浏览 vMarket 上的商家，寻找值得信赖的合作伙伴。", Keywords: "vMarket, 商家, 企业搜索", Image: "/static/vmarketicon.png"},
		}},
		{ID: "vmarket-map", Label: "vMarket - Map", PageKey: "vmarket.map", Defaults: map[string]langDefaults{
			"zh":    {Title: "vMarket 地圖", Description: "在地圖上探索附近的商家與服務。", Keywords: "vMarket, 地圖, 附近商家", Image: "/static/vmarketicon.png"},
			"en":    {Title: "vMarket Map", Description: "Explore nearby businesses and services on the map.", Keywords: "vMarket, map, nearby businesses", Image: "/static/vmarketicon.png"},
			"zh-CN": {Title: "vMarket 地图", Description: "在地图上探索附近的商家与服务。", Keywords: "vMarket, 地图, 附近商家", Image: "/static/vmarketicon.png"},
		}},
		{ID: "vmarket-join", Label: "vMarket - Join", PageKey: "vmarket.join", Defaults: map[string]langDefaults{
			"zh":    {Title: "加入 vMarket", Description: "免費加入 vMarket，展示你的商家和服務給更多潛在客戶。", Keywords: "vMarket, 加入, 免費上架", Image: "/static/vmarketicon.png"},
			"en":    {Title: "Join vMarket", Description: "Join vMarket for free and showcase your business to potential customers.", Keywords: "vMarket, join, free listing", Image: "/static/vmarketicon.png"},
			"zh-CN": {Title: "加入 vMarket", Description: "免费加入 vMarket，展示你的商家和服务给更多潜在客户。", Keywords: "vMarket, 加入, 免费上架", Image: "/static/vmarketicon.png"},
		}},
	}

	sections := make([]seoSection, 0, len(pages))
	for _, p := range pages {
		items := make([]seoSettingItem, 0, len(langs)*4)
		for _, l := range langs {
			prefix := fmt.Sprintf("seo.%s.%s", p.PageKey, l.Code)
			def := p.Defaults[l.Code]
			items = append(items,
				seoSettingItem{Key: prefix + ".title", Description: l.Label + " title", DefaultValue: def.Title},
				seoSettingItem{Key: prefix + ".description", Description: l.Label + " description", DefaultValue: def.Description},
				seoSettingItem{Key: prefix + ".keywords", Description: l.Label + " keywords", DefaultValue: def.Keywords},
				seoSettingItem{Key: prefix + ".image", Description: l.Label + " image URL/path", DefaultValue: def.Image},
			)
		}
		sections = append(sections, seoSection{ID: p.ID, Label: p.Label, PageKey: p.PageKey, Settings: items})
	}

	return sections
}

func VWorkAdminGetSEOSettings(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	sections := seoSectionsDefinition()
	for si := range sections {
		for ii := range sections[si].Settings {
			item := &sections[si].Settings[ii]
			v := models.GetSystemSetting(item.Key, "")
			if v == "" {
				v = item.DefaultValue
			}
			item.Value = v
		}
	}

	return c.JSON(fiber.Map{"sections": sections})
}

func VWorkAdminUpdateSEOSettings(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		Settings map[string]string `json:"settings"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}
	if len(req.Settings) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "settings cannot be empty"})
	}

	allowed := map[string]bool{}
	for _, sec := range seoSectionsDefinition() {
		for _, item := range sec.Settings {
			allowed[item.Key] = true
		}
	}

	for key, val := range req.Settings {
		if !allowed[key] {
			return c.Status(400).JSON(fiber.Map{"error": "invalid key: " + key})
		}
		value := strings.TrimSpace(val)
		desc := "SEO setting managed via vworkadmin"
		if err := models.SetSystemSetting(key, value, &desc); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to save: " + key})
		}
	}

	return c.JSON(fiber.Map{"success": true, "updated": len(req.Settings)})
}

// VWorkAdminAssignTenantToSalesPartner 將租戶指派到加盟商
// 如果租戶沒有訂閱（還在試用），自動啟用加盟商試用 plan
// 同時贈送加盟商設定的 vCoin（200-500）
func VWorkAdminAssignTenantToSalesPartner(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		TenantID       string `json:"tenant_id"`
		SalesPartnerID string `json:"sales_partner_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	tenantID, err := uuid.Parse(strings.TrimSpace(req.TenantID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid tenant_id"})
	}

	salesPartnerID, err := uuid.Parse(strings.TrimSpace(req.SalesPartnerID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid sales_partner_id"})
	}

	// Load tenant
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "tenant not found"})
	}

	// Load sales partner
	var sp models.SalesPartnerApplication
	if err := database.DB.Where("id = ? AND status = ?", salesPartnerID, "approved").First(&sp).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "sales partner not found or not approved"})
	}

	if sp.Code == nil || strings.TrimSpace(*sp.Code) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "sales partner has no code"})
	}

	// Update tenant ExtraFields with sales partner info
	if tenant.ExtraFields == nil {
		tenant.ExtraFields = make(models.JSONB)
	}
	tenant.ExtraFields["sales_partner_code"] = *sp.Code
	tenant.ExtraFields["sales_partner_application_id"] = sp.ID.String()

	updates := map[string]interface{}{
		"extra_fields": tenant.ExtraFields,
	}

	// If tenant has no subscription (still on trial), activate sales partner trial plan
	hasSubscription := tenant.SubscriptionID != nil && strings.TrimSpace(*tenant.SubscriptionID) != ""
	isTrial := tenant.Plan == "trial" || tenant.Plan == "free" || tenant.Plan == ""

	if !hasSubscription && isTrial {
		// Determine trial months from sales partner setting
		trialMonths := 1 // default 1 month
		if sp.TrialMonths != nil && *sp.TrialMonths > 0 {
			trialMonths = *sp.TrialMonths
		}

		tenant.ExtraFields["sales_partner_trial"] = true
		tenant.ExtraFields["sales_partner_trial_months"] = float64(trialMonths)
		updates["extra_fields"] = tenant.ExtraFields

		// Set trial expiration
		trialExpires := time.Now().AddDate(0, trialMonths, 0)
		updates["trial_expires_at"] = trialExpires
		updates["plan"] = "trial"
		updates["status"] = "active"
	}

	if err := database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to assign tenant to sales partner"})
	}

	// Grant vCoin bonus to tenant
	bonusCoins := models.SalesPartnerDefaultCoins // default 300
	if sp.MonthlyAICoins != nil {
		bonusCoins = *sp.MonthlyAICoins
		if bonusCoins < models.SalesPartnerMinCoins {
			bonusCoins = models.SalesPartnerMinCoins
		}
		if bonusCoins > models.SalesPartnerMaxCoins {
			bonusCoins = models.SalesPartnerMaxCoins
		}
	}

	// Add vCoin bonus to tenant's AI coins account
	// When creating a new account, also include the 50 welcome bonus on top of the partner bonus
	const welcomeBonus = 50
	var account models.TenantAICoins
	isNewAccount := false
	err = database.DB.Where("tenant_id = ?", tenantID).First(&account).Error
	if err == gorm.ErrRecordNotFound {
		isNewAccount = true
		// Create new account with welcome bonus + partner bonus
		totalInitial := bonusCoins + welcomeBonus
		now := time.Now()
		nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
		nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		account = models.TenantAICoins{
			TenantID:         tenantID,
			Balance:          totalInitial,
			MonthlyAllotment: 0,
			MonthlyUsed:      0,
			MonthlyResetAt:   nextMonth,
			DailyFreeUsed:    0,
			DailyFreeResetAt: nextDay,
			PurchasedBalance: totalInitial,
			TotalPurchased:   totalInitial,
			TotalUsed:        0,
		}
		database.DB.Create(&account)
	} else if err == nil {
		// Add bonus to existing account
		account.PurchasedBalance += bonusCoins
		account.Balance += bonusCoins
		account.TotalPurchased += bonusCoins
		database.DB.Model(&account).Updates(map[string]interface{}{
			"purchased_balance": account.PurchasedBalance,
			"balance":           account.Balance,
			"total_purchased":   account.TotalPurchased,
		})
	}

	// Record welcome bonus transaction if new account was created
	if isNewAccount {
		welcomeTx := models.AICoinsTransaction{
			TenantID:     tenantID,
			UserID:       uuid.Nil,
			Type:         models.AICoinsTypeBonus,
			Amount:       welcomeBonus,
			BalanceAfter: welcomeBonus,
			Description:  "歡迎禮 vCoin",
			Metadata:     models.JSONB{"reason": "welcome_bonus"},
		}
		database.DB.Create(&welcomeTx)
	}

	// Record partner bonus transaction
	tx := models.AICoinsTransaction{
		TenantID:     tenantID,
		UserID:       uuid.Nil, // system operation
		Type:         models.AICoinsTypeBonus,
		Amount:       bonusCoins,
		BalanceAfter: account.PurchasedBalance,
		Description:  fmt.Sprintf("加盟商 %s 贈送 vCoin", sp.Name),
		Metadata: models.JSONB{
			"reason":           "sales_partner_assign",
			"sales_partner_id": sp.ID.String(),
			"sales_partner":    sp.Name,
		},
	}
	database.DB.Create(&tx)

	return c.JSON(fiber.Map{
		"success":      true,
		"tenant_id":    tenantID,
		"partner":      sp.Name,
		"partner_code": *sp.Code,
		"bonus_coins":  bonusCoins,
		"trial_active": !hasSubscription && isTrial,
	})
}

// VWorkAdminUnassignTenantFromSalesPartner 將租戶從加盟商移除
func VWorkAdminUnassignTenantFromSalesPartner(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		TenantID string `json:"tenant_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid payload"})
	}

	tenantID, err := uuid.Parse(strings.TrimSpace(req.TenantID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid tenant_id"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "tenant not found"})
	}

	if tenant.ExtraFields == nil {
		return c.JSON(fiber.Map{"success": true, "message": "tenant has no sales partner"})
	}

	delete(tenant.ExtraFields, "sales_partner_code")
	delete(tenant.ExtraFields, "sales_partner_application_id")
	delete(tenant.ExtraFields, "sales_partner_trial")
	delete(tenant.ExtraFields, "sales_partner_trial_months")

	if err := database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to unassign"})
	}

	return c.JSON(fiber.Map{"success": true})
}

// VWorkAdminDeleteUser 刪除用戶
// 如果相關租戶只有這一個用戶，連同租戶一起刪除
func VWorkAdminDeleteUser(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	idStr := c.Params("id")
	userID, err := uuid.Parse(idStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}

	// 1. 查找用戶
	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}

	// 2. 查找該用戶關聯的所有租戶
	var userTenants []models.UserTenant
	if err := database.DB.Where("user_id = ?", userID).Find(&userTenants).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to find user tenants"})
	}

	// 3. 找出需要刪除的租戶（只剩這一個用戶的租戶）
	tenantsToDelete := []uuid.UUID{}
	for _, ut := range userTenants {
		var count int64
		// 檢查該租戶下是否還有其他用戶
		database.DB.Model(&models.UserTenant{}).
			Where("tenant_id = ? AND user_id != ?", ut.TenantID, userID).
			Count(&count)

		if count == 0 {
			tenantsToDelete = append(tenantsToDelete, ut.TenantID)
		}
	}

	// 4. 執行刪除（事務處理）
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// 刪除 UserTenant 關聯
		if err := tx.Where("user_id = ?", userID).Delete(&models.UserTenant{}).Error; err != nil {
			return err
		}

		// 刪除 User
		if err := tx.Delete(&user).Error; err != nil {
			return err
		}

		// 刪除相關租戶
		if len(tenantsToDelete) > 0 {
			if err := tx.Where("id IN ?", tenantsToDelete).Delete(&models.Tenant{}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to delete user: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":         true,
		"deleted_tenants": tenantsToDelete,
	})
}

// VWorkAdminAddSubscription 管理員手動為租戶新增 subscription（免 Stripe 付款）
func VWorkAdminAddSubscription(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		TenantID string `json:"tenant_id"`
		Plan     string `json:"plan"`   // vsuite_monthly, vsuite_yearly, vsuite_pro_monthly, vsuite_pro_yearly
		Months   int    `json:"months"` // subscription duration in months (default: based on plan interval)
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	tenantID, err := uuid.Parse(strings.TrimSpace(req.TenantID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的租戶 ID"})
	}

	planName := strings.ToLower(strings.TrimSpace(req.Plan))
	planName = normalizePlanName(planName)
	if !isValidPaidPlan(planName) {
		return c.Status(400).JSON(fiber.Map{"error": "無效的方案名稱"})
	}

	// Load tenant
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "租戶不存在"})
	}

	// Load subscription plan
	var plan models.SubscriptionPlan
	if err := database.DB.Where("name = ? AND is_active = true", planName).First(&plan).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "訂閱方案不存在或未啟用"})
	}

	// Determine period
	now := time.Now()
	var periodEnd time.Time
	if req.Months > 0 {
		periodEnd = now.AddDate(0, req.Months, 0)
	} else if plan.Interval == "year" {
		periodEnd = now.AddDate(1, 0, 0)
	} else {
		periodEnd = now.AddDate(0, 1, 0)
	}

	// Generate a unique pseudo stripe ID for admin-created subscriptions
	adminSubID := fmt.Sprintf("admin_manual_%s_%d", tenantID.String()[:8], now.Unix())

	// Cancel any existing active subscription for this tenant
	database.DB.Model(&models.Subscription{}).
		Where("tenant_id = ? AND status = ?", tenantID, "active").
		Updates(map[string]interface{}{
			"status":     "canceled",
			"updated_at": now,
		})

	// Create subscription record
	sub := models.Subscription{
		TenantID:             tenantID,
		PlanID:               plan.ID,
		StripeSubscriptionID: adminSubID,
		Status:               "active",
		CurrentPeriodStart:   &now,
		CurrentPeriodEnd:     &periodEnd,
		CancelAtPeriodEnd:    false,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := database.DB.Create(&sub).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "建立訂閱失敗: " + err.Error()})
	}

	// Update tenant plan/status
	tenantUpdates := map[string]interface{}{
		"plan":             planName,
		"status":           "active",
		"subscription_id":  adminSubID,
		"trial_expires_at": nil,
	}
	if err := database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(tenantUpdates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "更新租戶失敗: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"success":         true,
		"subscription_id": sub.ID,
		"plan":            planName,
		"plan_display":    plan.DisplayName,
		"period_start":    now,
		"period_end":      periodEnd,
		"message":         fmt.Sprintf("已為租戶 %s 新增 %s 訂閱", tenant.Name, plan.DisplayName),
	})
}

// VWorkAdminGetSubscriptionPlans 取得所有可用的訂閱方案
func VWorkAdminGetSubscriptionPlans(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var plans []models.SubscriptionPlan
	if err := database.DB.Where("is_active = true").Order("name ASC").Find(&plans).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "讀取方案失敗"})
	}

	return c.JSON(fiber.Map{"plans": plans})
}

// VWorkAdminAddBonusCoins 管理員手動加 vCoin 給租戶
func VWorkAdminAddBonusCoins(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		TenantID    string `json:"tenant_id"`
		Amount      int    `json:"amount"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	targetTenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的租戶 ID"})
	}

	if req.Amount <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "數量必須大於 0"})
	}

	if req.Description == "" {
		req.Description = "管理員手動加幣"
	}

	adminUserID := uuid.Nil // vworkadmin 沒有真實 user context
	err = AddAICoins(targetTenantID, adminUserID, req.Amount, models.AICoinsTypeBonus, req.Description, models.JSONB{
		"added_by": "vworkadmin",
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "添加失敗：" + err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("成功添加 %d vCoin 給租戶", req.Amount),
	})
}

// VWorkAdminGetTenantCoins 查詢租戶的 vCoin 餘額
func VWorkAdminGetTenantCoins(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	tenantID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的租戶 ID"})
	}

	var account models.TenantAICoins
	result := database.DB.Where("tenant_id = ?", tenantID).First(&account)
	if result.Error != nil {
		// No account yet — return zero balance
		return c.JSON(fiber.Map{
			"balance":           0,
			"monthly_allotment": 0,
			"monthly_used":      0,
			"purchased_balance": 0,
		})
	}

	return c.JSON(fiber.Map{
		"balance":           account.GetAvailableCoins(),
		"monthly_allotment": account.MonthlyAllotment,
		"monthly_used":      account.MonthlyUsed,
		"purchased_balance": account.PurchasedBalance,
	})
}

// VWorkAdminConvertPassword 管理員手動轉換用戶密碼
func VWorkAdminConvertPassword(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		UserID   string `json:"user_id"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	userID, err := uuid.Parse(strings.TrimSpace(req.UserID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的用戶 ID"})
	}

	if strings.TrimSpace(req.Password) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "密碼不能為空"})
	}

	if len(req.Password) < 6 || len(req.Password) > 20 {
		return c.Status(400).JSON(fiber.Map{"error": "密碼長度必須在 6-20 個字符之間"})
	}

	// 查找用戶
	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "用戶不存在"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "查找用戶失敗"})
	}

	// 生成新的密碼哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "密碼哈希失敗"})
	}

	// 更新用戶密碼
	if err := database.DB.Model(&user).Update("password_hash", string(hashedPassword)).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "更新密碼失敗"})
	}

	// 記錄操作日誌
	clientIP := c.IP()
	tenantID := uuid.Nil
	if user.TenantID != nil {
		tenantID = *user.TenantID
	}
	utils.LogActivity(tenantID, user.ID, "password_convert", "vworkadmin", nil,
		fmt.Sprintf(`{"key":"vworkadmin.password_convert","params":{"admin_ip":%q}}`, clientIP), nil, c)

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("已成功為用戶 %s 轉換密碼", user.Name),
	})
}

// VWorkAdminUpdateUserEmail 管理員修改用戶 Email
func VWorkAdminUpdateUserEmail(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	userID, err := uuid.Parse(strings.TrimSpace(req.UserID))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的用戶 ID"})
	}

	newEmail := strings.TrimSpace(req.Email)
	if newEmail == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email 不能為空"})
	}

	// 查找用戶
	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "用戶不存在"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "查找用戶失敗"})
	}

	// 檢查同租戶下 email 是否重複
	if user.TenantID != nil {
		var existingUser models.User
		if err := database.DB.Where("tenant_id = ? AND email = ? AND id != ?", *user.TenantID, newEmail, userID).First(&existingUser).Error; err == nil {
			return c.Status(409).JSON(fiber.Map{"error": "該 Email 已被同租戶下的其他用戶使用"})
		}
	}

	oldEmail := user.Email

	// 更新 Email
	if err := database.DB.Model(&user).Update("email", newEmail).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "更新 Email 失敗"})
	}

	// 記錄操作日誌
	clientIP := c.IP()
	tenantID := uuid.Nil
	if user.TenantID != nil {
		tenantID = *user.TenantID
	}
	utils.LogActivity(tenantID, user.ID, "email_update", "vworkadmin", nil,
		fmt.Sprintf(`{"key":"vworkadmin.email_update","params":{"admin_ip":%q,"old_email":%q,"new_email":%q}}`, clientIP, oldEmail, newEmail), nil, c)

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("已成功將用戶 %s 的 Email 從 %s 更新為 %s", user.Name, oldEmail, newEmail),
	})
}

// VWorkAdminRegenerateSitemap invalidates the sitemap cache and triggers regeneration.
func VWorkAdminRegenerateSitemap(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	InvalidateSitemapCache()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Sitemap 快取已清除，下次訪問 /sitemap.xml 時將自動重新產生。",
	})
}

// VWorkAdminGetAdminEmails returns the admin notification email addresses stored in admin_settings.
func VWorkAdminGetAdminEmails(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var setting models.AdminSetting
	if err := database.DB.Where("key = ?", "admin_emails").First(&setting).Error; err != nil {
		return c.JSON(fiber.Map{
			"admin_emails": "",
		})
	}
	return c.JSON(fiber.Map{
		"admin_emails": setting.Value,
	})
}

// VWorkAdminUpdateAdminEmails updates the admin notification email addresses in admin_settings.
func VWorkAdminUpdateAdminEmails(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var req struct {
		AdminEmails string `json:"admin_emails"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	// Validate email format (basic check)
	emails := strings.TrimSpace(req.AdminEmails)
	if emails != "" {
		for _, e := range strings.Split(emails, ",") {
			e = strings.TrimSpace(e)
			if e != "" && !strings.Contains(e, "@") {
				return c.Status(400).JSON(fiber.Map{"error": "無效的 Email 格式: " + e})
			}
		}
	}

	// Upsert into admin_settings
	result := database.DB.Where("key = ?", "admin_emails").First(&models.AdminSetting{})
	if result.Error != nil {
		// Create
		setting := models.AdminSetting{
			Key:         "admin_emails",
			Value:       emails,
			Description: "接收管理通知的 Email 地址（逗號分隔）",
		}
		if err := database.DB.Create(&setting).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "儲存失敗: " + err.Error()})
		}
	} else {
		// Update
		if err := database.DB.Model(&models.AdminSetting{}).Where("key = ?", "admin_emails").
			Updates(map[string]interface{}{
				"value":      emails,
				"updated_at": time.Now(),
			}).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "更新失敗: " + err.Error()})
		}
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "管理員通知 Email 已更新",
	})
}

// VWorkAdminGetEmailQuota returns the current daily email usage vs Brevo free limit.
// This is accessible to vworkadmin for monitoring purposes.
func VWorkAdminGetEmailQuota(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	usage, err := email.GetDailyEmailUsage()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "查詢失敗: " + err.Error()})
	}
	limit := email.GetBrevoFreeDailyLimit()

	return c.JSON(fiber.Map{
		"daily_usage": usage,
		"daily_limit": limit,
		"remaining":   int64(limit) - usage,
		"exceeded":    usage >= int64(limit),
	})
}

// GetEmailQuota returns the current daily email usage vs Brevo free limit.
// This endpoint is accessible by authenticated CMS users to check before sending promotions.
func GetEmailQuota(c *fiber.Ctx) error {
	usage, err := email.GetDailyEmailUsage()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "查詢失敗: " + err.Error()})
	}
	limit := email.GetBrevoFreeDailyLimit()
	cfg := config.Load()
	whatsappPhone := cfg.WhatsApp.PhoneNumber
	if whatsappPhone == "" {
		whatsappPhone = "85246237234"
	}

	return c.JSON(fiber.Map{
		"daily_usage":    usage,
		"daily_limit":    limit,
		"remaining":      int64(limit) - usage,
		"exceeded":       usage >= int64(limit),
		"whatsapp_phone": whatsappPhone,
	})
}
