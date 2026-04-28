package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// SetupTenantRequest 租戶設置請求
type SetupTenantRequest struct {
	CompanyName      string  `json:"company_name"`
	CompanyEmail     *string `json:"company_email"`
	Phone            *string `json:"phone"`
	PhoneCountryCode *string `json:"phone_country_code"`
	Address          *string `json:"address"`
	CountryCode      *string `json:"country_code"`
	Industry         *string `json:"industry"`
	LogoURL          *string `json:"logo_url"`
	SalesPartnerCode *string `json:"sales_partner_code"`
}

// SetupTenant 創建租戶和企業
func SetupTenant(c *fiber.Ctx) error {
	// 確保返回 JSON Content-Type
	c.Set("Content-Type", "application/json")

	log.Printf("🔵 SetupTenant handler called - Method: %s, Path: %s", c.Method(), c.Path())

	userID := middleware.GetUserID(c)
	log.Printf("🔵 UserID from middleware: %v", userID)
	if userID == uuid.Nil {
		log.Printf("❌ User ID not found in context")
		return c.Status(400).JSON(fiber.Map{
			"error": "User ID not found",
		})
	}

	// 檢查用戶是否已有租戶
	var user models.User
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Allow multiple tenants (max 5)
	var tenantCount int64
	if err := database.DB.Model(&models.UserTenant{}).Where("user_id = ?", userID).Count(&tenantCount).Error; err != nil {
		log.Printf("⚠️ Failed to count user tenants: %v", err)
	}

	if tenantCount >= 5 {
		return c.Status(400).JSON(fiber.Map{
			"error": "User has reached the maximum number of tenants (5)",
		})
	}

	var req SetupTenantRequest
	contentType := strings.ToLower(strings.TrimSpace(c.Get("Content-Type")))
	if strings.Contains(contentType, "multipart/form-data") {
		// Multipart: supports uploading company logo before tenant exists.
		req.CompanyName = strings.TrimSpace(c.FormValue("company_name"))
		if v := strings.TrimSpace(c.FormValue("company_email")); v != "" {
			req.CompanyEmail = &v
		}
		if v := strings.TrimSpace(c.FormValue("phone")); v != "" {
			req.Phone = &v
		}
		if v := strings.TrimSpace(c.FormValue("phone_country_code")); v != "" {
			req.PhoneCountryCode = &v
		}
		if v := strings.TrimSpace(c.FormValue("address")); v != "" {
			req.Address = &v
		}
		if v := strings.TrimSpace(c.FormValue("country_code")); v != "" {
			req.CountryCode = &v
		}
		if v := strings.TrimSpace(c.FormValue("industry")); v != "" {
			req.Industry = &v
		}
		if v := strings.TrimSpace(c.FormValue("logo_url")); v != "" {
			req.LogoURL = &v
		}
		if v := strings.TrimSpace(c.FormValue("sales_partner_code")); v != "" {
			req.SalesPartnerCode = &v
		}
	} else {
		// JSON
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid request",
			})
		}
	}

	// 驗證公司名稱
	if strings.TrimSpace(req.CompanyName) == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Company name is required",
		})
	}
	if req.CompanyEmail == nil || strings.TrimSpace(*req.CompanyEmail) == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Company email is required",
		})
	}

	var salesPartnerCode *string
	var salesPartnerAppID *uuid.UUID
	var salesPartnerTrialMonths *int // 銷售商設定的試用月數
	if req.SalesPartnerCode != nil {
		code := strings.ToUpper(strings.TrimSpace(*req.SalesPartnerCode))
		if code != "" {
			var app models.SalesPartnerApplication
			if err := database.DB.Where("code = ? AND status = ?", code, "approved").First(&app).Error; err != nil {
				return c.Status(400).JSON(fiber.Map{
					"error": "Invalid sales partner code",
				})
			}
			salesPartnerCode = &code
			salesPartnerAppID = &app.ID
			salesPartnerTrialMonths = app.TrialMonths // 可能為 nil
		}
	}

	// 生成 subdomain
	subdomain := ""
	for _, char := range req.CompanyName {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			subdomain += string(char)
		} else if char == ' ' || char == '-' || char == '_' {
			subdomain += "-"
		}
	}
	subdomain = strings.ToLower(subdomain)
	for strings.Contains(subdomain, "--") {
		subdomain = strings.ReplaceAll(subdomain, "--", "-")
	}
	subdomain = strings.Trim(subdomain, "-")
	if subdomain == "" {
		b := make([]byte, 4)
		rand.Read(b)
		subdomain = "tenant-" + base64.URLEncoding.EncodeToString(b)[:8]
	}

	// 檢查子域名是否已存在
	var existingTenant models.Tenant
	if err := database.DB.Where("subdomain = ?", subdomain).First(&existingTenant).Error; err == nil {
		b := make([]byte, 2)
		rand.Read(b)
		subdomain = subdomain + "-" + base64.URLEncoding.EncodeToString(b)[:4]
		for {
			if err := database.DB.Where("subdomain = ?", subdomain).First(&existingTenant).Error; err != nil {
				break
			}
			b := make([]byte, 2)
			rand.Read(b)
			subdomain = subdomain + "-" + base64.URLEncoding.EncodeToString(b)[:4]
		}
	}

	// 獲取配置
	trialDays := 0
	trialHours := 0

	// 創建租戶
	tenant := models.Tenant{
		Name:           req.CompanyName,
		Subdomain:      subdomain,
		Plan:           "trial",
		Status:         "active",
		WebsiteTheme:   nil,
		WebsiteType:    nil,
		WebsiteEnabled: false,
		ExtraFields: models.JSONB{
			"trial_days":  float64(trialDays),
			"trial_hours": float64(trialHours),
		},
	}
	if req.Phone != nil && *req.Phone != "" {
		tenant.ExtraFields["phone"] = *req.Phone
	}
	if req.PhoneCountryCode != nil && *req.PhoneCountryCode != "" {
		tenant.ExtraFields["phone_country_code"] = *req.PhoneCountryCode
	}
	if req.CountryCode != nil && strings.TrimSpace(*req.CountryCode) != "" {
		tenant.ExtraFields["country_code"] = strings.ToUpper(strings.TrimSpace(*req.CountryCode))
	}
	if req.Industry != nil && strings.TrimSpace(*req.Industry) != "" {
		tenant.ExtraFields["industry"] = strings.TrimSpace(*req.Industry)
	}
	tenant.ExtraFields["email"] = strings.TrimSpace(*req.CompanyEmail)
	if salesPartnerCode != nil {
		tenant.ExtraFields["sales_partner_code"] = *salesPartnerCode
	}
	if salesPartnerAppID != nil {
		tenant.ExtraFields["sales_partner_application_id"] = salesPartnerAppID.String()
	}
	// 若銷售商有設定試用月數，則標記為銷售商試用（無配額限制）
	if salesPartnerTrialMonths != nil && *salesPartnerTrialMonths > 0 {
		tenant.ExtraFields["sales_partner_trial_months"] = float64(*salesPartnerTrialMonths)
		tenant.ExtraFields["sales_partner_trial"] = true // 標記為銷售商試用模式，無配額限制
		// 設定試用到期時間為 N 個月後
		trialExpires := time.Now().AddDate(0, *salesPartnerTrialMonths, 0)
		tenant.TrialExpiresAt = &trialExpires
	}

	if err := database.DB.Create(&tenant).Error; err != nil {
		log.Printf("❌ Failed to create tenant: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create tenant: " + err.Error(),
		})
	}

	// 若上傳公司 Logo（multipart/form-data），tenant 建立後才能落盤到 tenant 目錄
	var uploadedLogoURL *string
	if strings.Contains(contentType, "multipart/form-data") {
		if logoFile, err := c.FormFile("company_logo"); err == nil && logoFile != nil {
			if url, _, _, _, _, err2 := saveUploadedImageForTenant(tenant.ID, logoFile); err2 == nil {
				u := strings.TrimSpace(url)
				if u != "" {
					uploadedLogoURL = &u
				}
			} else {
				log.Printf("⚠️  Upload company_logo failed: %v", err2)
			}
		}
	}
	if uploadedLogoURL == nil && req.LogoURL != nil {
		u := strings.TrimSpace(*req.LogoURL)
		if u != "" {
			uploadedLogoURL = &u
		}
	}

	// 創建企業
	var domain *string
	if subdomain != "" {
		d := subdomain
		domain = &d
	}

	phoneNum := ""
	if req.Phone != nil {
		phoneNum = *req.Phone
	}

	enterprise := models.Enterprise{
		TenantID:    tenant.ID,
		Name:        req.CompanyName,
		Domain:      domain,
		LogoURL:     uploadedLogoURL,
		Status:      "active",
		ExtraFields: models.JSONB{},
	}
	if strings.TrimSpace(phoneNum) != "" {
		enterprise.ExtraFields["phone"] = phoneNum
	}
	if req.PhoneCountryCode != nil && *req.PhoneCountryCode != "" {
		enterprise.ExtraFields["phone_country_code"] = *req.PhoneCountryCode
	}
	if req.CountryCode != nil && strings.TrimSpace(*req.CountryCode) != "" {
		enterprise.ExtraFields["country_code"] = strings.ToUpper(strings.TrimSpace(*req.CountryCode))
	}
	if req.Industry != nil && strings.TrimSpace(*req.Industry) != "" {
		enterprise.ExtraFields["industry"] = strings.TrimSpace(*req.Industry)
	}
	if req.Address != nil && *req.Address != "" {
		enterprise.Address = req.Address
	}
	if strings.TrimSpace(phoneNum) != "" {
		p := strings.TrimSpace(phoneNum)
		enterprise.Phone = &p
	}

	if err := database.DB.Create(&enterprise).Error; err != nil {
		log.Printf("❌ Failed to create enterprise: %v", err)
		database.DB.Delete(&tenant)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create enterprise: " + err.Error(),
		})
	}

	// 更新用戶的租戶 ID
	user.TenantID = &tenant.ID
	if err := database.DB.Model(&user).Update("tenant_id", tenant.ID).Error; err != nil {
		log.Printf("❌ Failed to update user tenant_id: %v", err)
		database.DB.Delete(&enterprise)
		database.DB.Delete(&tenant)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update user: " + err.Error(),
		})
	}

	// 初始化租戶的預設數據（包括創建 Admin Role）
	defaultPhoneCode := "+852"
	if req.PhoneCountryCode != nil && *req.PhoneCountryCode != "" {
		defaultPhoneCode = *req.PhoneCountryCode
	}
	if err := utils.InitTenantData(tenant.ID, defaultPhoneCode); err != nil {
		log.Printf("⚠️  初始化租戶預設數據失敗: %v", err)
		// 如果初始化失败，尝试清理已创建的数据
		database.DB.Delete(&enterprise)
		database.DB.Delete(&tenant)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to initialize tenant data: " + err.Error(),
		})
	}

	// 將用戶關聯到默認工作時段（如果存在）
	var defaultShift models.Shift
	if err := database.DB.Where("tenant_id = ? AND is_default = ?", tenant.ID, true).First(&defaultShift).Error; err == nil {
		if err := database.DB.Model(&user).Update("shift_id", defaultShift.ID).Error; err != nil {
			log.Printf("⚠️  關聯默認工作時段失敗: %v", err)
		}
	}

	// 設置 Admin Role（InitTenantData 已經創建了，這裡只需要查找並關聯給用戶）
	var adminRole models.Role
	if err := database.DB.Where("tenant_id = ? AND LOWER(name) = ?", tenant.ID, "admin").First(&adminRole).Error; err != nil {
		log.Printf("⚠️  Admin role not found after InitTenantData: %v", err)
		// 如果找不到，嘗試創建（作為備用）
		now2 := time.Now()
		adminRole = models.Role{
			ID:          uuid.New(),
			TenantID:    tenant.ID,
			Name:        "Admin",
			Description: "System administrator with full permissions",
			Permissions: models.StringArrayJSONB{},
			Status:      "active",
			CreatedAt:   now2,
			UpdatedAt:   now2,
		}
		if err2 := database.DB.Create(&adminRole).Error; err2 != nil {
			log.Printf("⚠️  create admin role failed: tenant_id=%s err=%v", tenant.ID, err2)
		}
	}
	if adminRole.ID != uuid.Nil {
		user.RoleID = &adminRole.ID
		if err := database.DB.Model(&user).Update("role_id", adminRole.ID).Error; err != nil {
			log.Printf("⚠️  assign admin role failed: user_id=%s role_id=%s err=%v", user.ID, adminRole.ID, err)
		}
	}

	// 為租戶擁有者生成員工編號（如果沒有）
	if user.EmployeeNumber == "" {
		employeeNumber := GenerateEmployeeNumber(tenant.ID)
		if err := database.DB.Model(&user).Update("employee_number", employeeNumber).Error; err != nil {
			log.Printf("⚠️  assign employee number failed: user_id=%s err=%v", user.ID, err)
		} else {
			user.EmployeeNumber = employeeNumber
			log.Printf("✅ Generated employee number %s for tenant owner: user_id=%s", employeeNumber, user.ID)
		}
	}

	// 若有設定試用到期（目前為無日數上限，除非配額超額觸發）才寫入
	if trialDays > 0 || trialHours > 0 {
		trialExpiresAt := user.CreatedAt.AddDate(0, 0, trialDays).Add(time.Duration(trialHours) * time.Hour)
		tenant.TrialExpiresAt = &trialExpiresAt
		database.DB.Model(&tenant).Update("trial_expires_at", trialExpiresAt)
	}

	// 重新獲取用戶信息以獲取最新的 role 信息
	var updatedUser models.User
	if err := database.DB.Where("id = ?", user.ID).Preload("Role").First(&updatedUser).Error; err != nil {
		log.Printf("⚠️  Failed to reload user after tenant setup: %v", err)
		updatedUser = user
	}

	// 確定 role 名稱（優先使用 Role.Name，否則使用 UserRole）
	roleName := updatedUser.UserRole
	if updatedUser.Role != nil && updatedUser.Role.Name != "" {
		roleName = updatedUser.Role.Name
	}
	if roleName == "" {
		roleName = "Admin" // 默認值
	}

	// 重新生成 JWT Token（包含新的 tenant_id）
	newToken, err := utils.GenerateToken(updatedUser.ID, tenant.ID, updatedUser.Email, roleName, "web")
	if err != nil {
		log.Printf("⚠️  Failed to generate new token after tenant setup: %v", err)
		// 即使生成 token 失敗，仍然返回成功，但記錄警告
	} else {
		// 更新 cookie 中的 auth_token
		setAuthCookie(c, newToken, 30*24*time.Hour)
		log.Printf("✅ Updated session token after tenant setup for user: %s, tenant: %s", updatedUser.ID, tenant.ID)
	}

	// 建立 user_tenants 關聯（支援多租戶）
	if updatedUser.ID != uuid.Nil && tenant.ID != uuid.Nil {
		var existing models.UserTenant
		if err := database.DB.Where("user_id = ? AND tenant_id = ?", updatedUser.ID, tenant.ID).First(&existing).Error; err != nil {
			nowLink := time.Now()
			link := models.UserTenant{
				UserID:     updatedUser.ID,
				TenantID:   tenant.ID,
				Role:       roleName,
				IsDefault:  true,
				LastUsedAt: &nowLink,
				CreatedAt:  nowLink,
				UpdatedAt:  nowLink,
			}
			if err := database.DB.Create(&link).Error; err != nil {
				log.Printf("⚠️  create user_tenants failed: user_id=%s tenant_id=%s err=%v", updatedUser.ID, tenant.ID, err)
			}
		} else {
			nowLink := time.Now()
			existing.IsDefault = true
			existing.LastUsedAt = &nowLink
			existing.UpdatedAt = nowLink
			if err := database.DB.Save(&existing).Error; err != nil {
				log.Printf("⚠️  update user_tenants failed: user_id=%s tenant_id=%s err=%v", updatedUser.ID, tenant.ID, err)
			}
		}
	}

	// 記錄活動：租戶建立（使用同步方式確保記錄成功）
	changes := map[string]interface{}{
		"tenant_id":       tenant.ID.String(),
		"tenant_name":     tenant.Name,
		"subdomain":       tenant.Subdomain,
		"plan":            tenant.Plan,
		"enterprise_id":   enterprise.ID.String(),
		"enterprise_name": enterprise.Name,
	}
	if err := utils.LogActivitySync(tenant.ID, updatedUser.ID, "create", "tenant", &tenant.ID,
		fmt.Sprintf(`{"key":"tenant.create","params":{"name":%q,"email":%q,"tenant_name":%q}}`, updatedUser.Name, updatedUser.Email, tenant.Name),
		changes, c); err != nil {
		log.Printf("⚠️  Failed to log tenant creation activity: %v", err)
		// 不中斷主流程，但記錄錯誤
	}

	// 記錄活動：用戶加入租戶（使用同步方式確保記錄成功）
	userJoinChanges := map[string]interface{}{
		"tenant_id":   tenant.ID.String(),
		"tenant_name": tenant.Name,
		"user_id":     updatedUser.ID.String(),
		"user_name":   updatedUser.Name,
		"user_email":  updatedUser.Email,
	}
	if err := utils.LogActivitySync(tenant.ID, updatedUser.ID, "join", "tenant", &tenant.ID,
		fmt.Sprintf(`{"key":"tenant.join","params":{"name":%q,"email":%q,"tenant_name":%q}}`, updatedUser.Name, updatedUser.Email, tenant.Name),
		userJoinChanges, c); err != nil {
		log.Printf("⚠️  Failed to log user join tenant activity: %v", err)
		// 不中斷主流程，但記錄錯誤
	}

	// 入列 welcome email（不影響主流程）
	if strings.TrimSpace(updatedUser.Email) != "" {
		if err := email.EnqueueWelcomeEmail(tenant.ID, updatedUser.ID, tenant.Subdomain, updatedUser.Email, updatedUser.Name); err != nil {
			log.Printf("⚠️  enqueue welcome email failed: tenant_id=%s user_id=%s err=%v", tenant.ID, updatedUser.ID, err)
		}
	}

	// 通知管理員：新租戶註冊
	go email.EnqueueAdminNotification("new_registration", map[string]string{
		"tenant_name": tenant.Name,
		"subdomain":   tenant.Subdomain,
		"user_name":   updatedUser.Name,
		"user_email":  updatedUser.Email,
		"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
	})

	// 確保返回正確的 Content-Type 和 JSON 格式
	c.Set("Content-Type", "application/json")
	return c.Status(201).JSON(fiber.Map{
		"message": "Tenant setup successful",
		"token":   newToken, // 返回新 token，前端可以選擇使用
		"tenant": fiber.Map{
			"id":        tenant.ID,
			"name":      tenant.Name,
			"subdomain": tenant.Subdomain,
		},
		"enterprise": fiber.Map{
			"id":   enterprise.ID,
			"name": enterprise.Name,
		},
	})
}
