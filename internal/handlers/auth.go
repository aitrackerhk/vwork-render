package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// isSecureCookie returns true when the server is running in production
// (APP_ENV=prod/production) or when PUBLIC_SCHEME=https.
// In development (HTTP / localhost), cookies must NOT set Secure=true
// or the browser will refuse to send them.
func isSecureCookie() bool {
	scheme := strings.ToLower(strings.TrimSpace(os.Getenv("PUBLIC_SCHEME")))
	if scheme == "https" {
		return true
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	return env == "prod" || env == "production"
}

// setAuthCookie sets the auth_token HTTPOnly cookie with the correct
// Secure flag based on the current environment.
func setAuthCookie(c *fiber.Ctx, token string, maxAge time.Duration) {
	c.Cookie(&fiber.Cookie{
		Name:     "auth_token",
		Value:    token,
		Expires:  time.Now().Add(maxAge),
		HTTPOnly: true,
		Secure:   isSecureCookie(),
		SameSite: "Lax",
		Path:     "/",
	})
}

// clearAuthCookie expires the auth_token cookie immediately.
func clearAuthCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     "auth_token",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		HTTPOnly: true,
		Secure:   isSecureCookie(),
		SameSite: "Lax",
		Path:     "/",
	})
}

// RegisterRequest 註冊請求
type RegisterRequest struct {
	Email            string  `json:"email"`
	Phone            string  `json:"phone"`
	PhoneCountryCode string  `json:"phone_country_code"`
	BirthDate        *string `json:"birth_date"`
	Password         string  `json:"password"`
	Name             string  `json:"name"`
}

// LoginRequest 登錄請求
type LoginRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	Subdomain string `json:"subdomain"`
	TenantID  string `json:"tenant_id"`
}

func ensureUserTenantLink(user *models.User) {
	if user == nil || user.ID == uuid.Nil || user.TenantID == nil || *user.TenantID == uuid.Nil {
		return
	}
	var existing models.UserTenant
	if err := database.DB.Where("user_id = ? AND tenant_id = ?", user.ID, *user.TenantID).First(&existing).Error; err == nil {
		return
	}
	now := time.Now()
	link := models.UserTenant{
		UserID:     user.ID,
		TenantID:   *user.TenantID,
		Role:       user.UserRole,
		IsDefault:  true,
		LastUsedAt: &now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_ = database.DB.Create(&link).Error
}

func fetchUserTenants(userID uuid.UUID) ([]models.UserTenant, error) {
	var links []models.UserTenant
	if userID == uuid.Nil {
		return links, nil
	}
	err := database.DB.Where("user_id = ?", userID).
		Preload("Tenant").
		Order("last_used_at DESC NULLS LAST, created_at DESC").
		Find(&links).Error
	return links, err
}

func normalizeTenantStatus(tenant *models.Tenant) error {
	if tenant == nil || tenant.ID == uuid.Nil {
		return fmt.Errorf("tenant not found")
	}
	if tenant.Status == "" {
		tenant.Status = "active"
		database.DB.Save(tenant)
		log.Printf("Login: tenant '%s' status was empty, set to 'active'", tenant.ID)
		return nil
	}
	status := strings.TrimSpace(strings.ToLower(tenant.Status))
	if status != "" && status != "active" && status != "suspended" && status != "trial_expired" {
		return fmt.Errorf("tenant inactive")
	}
	return nil
}

func buildTenantList(links []models.UserTenant) []fiber.Map {
	list := make([]fiber.Map, 0, len(links))
	for _, link := range links {
		tenant := link.Tenant
		if tenant == nil || tenant.ID == uuid.Nil {
			continue
		}
		list = append(list, fiber.Map{
			"id":           tenant.ID,
			"name":         tenant.Name,
			"subdomain":    tenant.Subdomain,
			"plan":         tenant.Plan,
			"status":       tenant.Status,
			"last_used_at": link.LastUsedAt,
		})
	}
	return list
}

// Register 租戶註冊
func Register(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// 正規化電話（註冊表單是「區號 + 電話」分開）
	phoneCC := strings.TrimSpace(req.PhoneCountryCode)
	phoneNum := strings.TrimSpace(req.Phone)
	var fullPhone string
	if phoneNum != "" {
		if phoneCC != "" {
			// 若使用者已把區號打在電話欄，避免重複拼接
			if strings.HasPrefix(phoneNum, phoneCC) {
				fullPhone = phoneNum
			} else {
				fullPhone = phoneCC + " " + phoneNum
			}
		} else {
			fullPhone = phoneNum
		}
	}

	// 驗證用戶名稱：最多 50 位
	if len(req.Name) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Name is required",
		})
	}
	if utf8.RuneCountInString(req.Name) > 50 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Name must be at most 50 characters",
		})
	}

	// 驗證密碼：最小 6 位，最多 20 位
	if len(req.Password) < 6 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Password must be at least 6 characters",
		})
	}
	if len(req.Password) > 20 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Password must be at most 20 characters",
		})
	}

	// 檢查 email 是否已在任何租戶中註冊過（全局唯一性檢查）
	var existingUser models.User
	if err := database.DB.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email already registered",
		})
	}

	// 創建用戶（不創建租戶，租戶將在後續設置頁面創建）
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	now := time.Now()

	// 解析出生日期
	var birthDate *time.Time
	if req.BirthDate != nil && *req.BirthDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", *req.BirthDate); err == nil {
			birthDate = &parsedDate
		}
	}

	user := models.User{
		TenantID:     nil, // 暫時沒有租戶
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Name:         req.Name,
		Phone:        fullPhone,
		BirthDate:    birthDate,
		UserRole:     "admin",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
		ExtraFields:  models.JSONB{},
	}

	// 保存電話區號到 ExtraFields（如果提供）
	if phoneCC != "" {
		if user.ExtraFields == nil {
			user.ExtraFields = make(models.JSONB)
		}
		user.ExtraFields["phone_country_code"] = phoneCC
	}

	if err := database.DB.Create(&user).Error; err != nil {
		log.Printf("❌ Failed to create user: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create user: " + err.Error(),
		})
	}

	// 註冊成功後自動創建登錄 session（生成 token）
	token, err := utils.GenerateToken(user.ID, uuid.Nil, user.Email, user.UserRole, "web")
	if err != nil {
		log.Printf("⚠️  Failed to generate token after registration: %v", err)
		// 即使生成 token 失敗，仍然返回成功，但記錄警告
	} else {
		// 設置 cookie（用於網頁訪問）
		setAuthCookie(c, token, 30*24*time.Hour)
		log.Printf("✅ Created session token after registration for user: %s", user.ID)
	}

	// 檢查個人資料完整性
	isProfileComplete := strings.TrimSpace(user.Name) != "" && strings.TrimSpace(user.Email) != ""
	hasPhone := strings.TrimSpace(user.Phone) != ""

	// 返回用戶信息和 token（前端可直接使用）
	return c.Status(201).JSON(fiber.Map{
		"message": "User registered successfully. Please complete tenant setup.",
		"token":   token, // 返回 token，前端可以直接使用
		"user": fiber.Map{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
		},
		"requires_setup": true, // 標記需要設置租戶
		"profile": fiber.Map{
			"complete": isProfileComplete,
			"hasPhone": hasPhone,
		},
	})
}

// Login 用戶登錄
func Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// 驗證輸入
	if req.Email == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email and password are required",
		})
	}

	// 通過電子郵件直接查找用戶（電子郵件是唯一的）
	var user models.User
	if err := database.DB.Where("LOWER(email) = LOWER(?)", req.Email).First(&user).Error; err != nil {
		log.Printf("Login failed: email '%s' not found, error: %v", req.Email, err)
		return c.Status(401).JSON(fiber.Map{
			"error": "電子郵件或密碼錯誤",
		})
	}

	// 檢查用戶狀態
	if user.Status != "active" {
		log.Printf("Login failed: user '%s' account is inactive (status: %s)", req.Email, user.Status)
		return c.Status(401).JSON(fiber.Map{
			"error": "帳戶已被停用，請聯繫管理員",
		})
	}

	// 驗證密碼
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		log.Printf("Login failed: password mismatch for user '%s'", req.Email)
		return c.Status(401).JSON(fiber.Map{
			"error": "電子郵件或密碼錯誤",
		})
	}

	// 建立/補齊 user_tenants 關聯（向後兼容）
	ensureUserTenantLink(&user)

	links, err := fetchUserTenants(user.ID)
	if err != nil {
		log.Printf("Login failed: fetch user tenants error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch user tenants"})
	}

	// 沒有任何租戶
	if len(links) == 0 {
		log.Printf("Login: user '%s' has no tenant, redirecting to setup", req.Email)
		// 仍然生成 token，但標記需要設置租戶
		token, err := utils.GenerateToken(user.ID, uuid.Nil, user.Email, user.UserRole, "web")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to generate token",
			})
		}
		setAuthCookie(c, token, 30*24*time.Hour)
		// 檢查個人資料完整性
		isProfileComplete := strings.TrimSpace(user.Name) != "" && strings.TrimSpace(user.Email) != ""
		hasPhone := strings.TrimSpace(user.Phone) != ""
		profileGuideSkipped := false
		if user.ExtraFields != nil {
			if skipped, ok := user.ExtraFields["profile_guide_skipped"].(bool); ok {
				profileGuideSkipped = skipped
			}
		}

		return c.JSON(fiber.Map{
			"message":        "Login successful",
			"token":          token,
			"requires_setup": true,
			"user": fiber.Map{
				"id":    user.ID,
				"email": user.Email,
				"name":  user.Name,
				"role":  user.UserRole,
			},
			"profile": fiber.Map{
				"complete": isProfileComplete,
				"hasPhone": hasPhone,
				"skipped":  profileGuideSkipped,
			},
		})
	}

	// 選擇租戶
	var selectedTenant *models.Tenant
	if req.TenantID != "" {
		if tid, err := uuid.Parse(req.TenantID); err == nil {
			for _, link := range links {
				if link.Tenant != nil && link.Tenant.ID == tid {
					selectedTenant = link.Tenant
					break
				}
			}
		}
	} else if strings.TrimSpace(req.Subdomain) != "" {
		sub := strings.TrimSpace(req.Subdomain)
		for _, link := range links {
			if link.Tenant != nil && link.Tenant.Subdomain == sub {
				selectedTenant = link.Tenant
				break
			}
		}
	} else if len(links) == 1 {
		selectedTenant = links[0].Tenant
	}

	// 多租戶需要選擇
	if selectedTenant == nil {
		token, err := utils.GenerateToken(user.ID, uuid.Nil, user.Email, user.UserRole, "web")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
		}
		setAuthCookie(c, token, 30*24*time.Hour)

		isProfileComplete := strings.TrimSpace(user.Name) != "" && strings.TrimSpace(user.Email) != ""
		hasPhone := strings.TrimSpace(user.Phone) != ""
		profileGuideSkipped := false
		if user.ExtraFields != nil {
			if skipped, ok := user.ExtraFields["profile_guide_skipped"].(bool); ok {
				profileGuideSkipped = skipped
			}
		}

		return c.JSON(fiber.Map{
			"message":                   "Login successful",
			"token":                     token,
			"requires_tenant_selection": true,
			"tenants":                   buildTenantList(links),
			"last_tenant_id":            user.TenantID,
			"user": fiber.Map{
				"id":    user.ID,
				"email": user.Email,
				"name":  user.Name,
				"role":  user.UserRole,
			},
			"profile": fiber.Map{
				"complete": isProfileComplete,
				"hasPhone": hasPhone,
				"skipped":  profileGuideSkipped,
			},
		})
	}

	// 檢查租戶狀態
	if err := normalizeTenantStatus(selectedTenant); err != nil {
		log.Printf("Login failed: tenant '%s' is inactive for user '%s'", selectedTenant.ID, req.Email)
		return c.Status(401).JSON(fiber.Map{
			"error": "帳戶所屬企業未激活，請聯繫管理員",
		})
	}

	log.Printf("Login attempt: email='%s', tenant_id='%s', tenant_subdomain='%s'", req.Email, selectedTenant.ID, selectedTenant.Subdomain)

	// 更新最後登錄時間
	now := time.Now()
	user.LastLoginAt = &now
	database.DB.Save(&user)
	// 更新使用者預設租戶（用於下次自動選擇）
	if user.TenantID == nil || *user.TenantID != selectedTenant.ID {
		_ = database.DB.Model(&user).Update("tenant_id", selectedTenant.ID).Error
	}
	// 更新 user_tenants 的最後使用時間
	_ = database.DB.Model(&models.UserTenant{}).
		Where("user_id = ? AND tenant_id = ?", user.ID, selectedTenant.ID).
		Updates(map[string]interface{}{"last_used_at": now, "updated_at": now, "is_default": true}).Error

	// 生成 JWT Token
	roleName := user.UserRole
	if user.Role != nil {
		roleName = user.Role.Name
	}
	token, err := utils.GenerateToken(user.ID, selectedTenant.ID, user.Email, roleName, "web")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to generate token",
		})
	}

	// 設置 cookie（用於網頁訪問）
	setAuthCookie(c, token, 30*24*time.Hour)

	// 檢查個人資料完整性
	isProfileComplete := strings.TrimSpace(user.Name) != "" && strings.TrimSpace(user.Email) != ""
	hasPhone := strings.TrimSpace(user.Phone) != ""
	profileGuideSkipped := false
	if user.ExtraFields != nil {
		if skipped, ok := user.ExtraFields["profile_guide_skipped"].(bool); ok {
			profileGuideSkipped = skipped
		}
	}

	// 記錄登入活動
	utils.LogActivity(selectedTenant.ID, user.ID, "login", "auth", nil,
		fmt.Sprintf(`{"key":"auth.login","params":{"name":%q,"email":%q}}`, user.Name, user.Email), nil, c)

	return c.JSON(fiber.Map{
		"message": "Login successful",
		"token":   token,
		"user": fiber.Map{
			"id":    user.ID,
			"email": user.Email,
			"name":  user.Name,
			"role":  user.UserRole,
		},
		"tenant": fiber.Map{
			"id":        selectedTenant.ID,
			"name":      selectedTenant.Name,
			"subdomain": selectedTenant.Subdomain,
			"plan":      selectedTenant.Plan,
		},
		"profile": fiber.Map{
			"complete": isProfileComplete,
			"hasPhone": hasPhone,
			"skipped":  profileGuideSkipped,
		},
	})
}

// GetCurrentUser 獲取當前登錄用戶信息
func GetCurrentUser(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{
			"error": "User ID not found",
		})
	}
	tenantID := middleware.GetTenantID(c)
	// 注意：tenantID 為 Nil 是合法的（新註冊用戶尚未設置租戶）

	var user models.User
	// 如果 tenantID 存在，先嘗試通過 tenant_id 查找
	if tenantID != uuid.Nil {
		if err := database.DB.Where("id = ? AND tenant_id = ?", userID, tenantID).
			Preload("Level").Preload("Department").Preload("Role").First(&user).Error; err == nil {
			// 找到用戶，繼續處理
		} else {
			// 如果找不到，嘗試只通過 user_id 查找（可能是舊數據）
			if err2 := database.DB.Where("id = ?", userID).
				Preload("Level").Preload("Department").Preload("Role").First(&user).Error; err2 != nil {
				return c.Status(404).JSON(fiber.Map{
					"error": "User not found",
				})
			}
			// 如果找到用戶但 tenant_id 不匹配，仍然返回用戶（因為這是 /user/me，應該允許）
		}
	} else {
		// 如果沒有 tenantID，只通過 user_id 查找（新註冊用戶尚未設置租戶）
		if err := database.DB.Where("id = ?", userID).
			Preload("Level").Preload("Department").Preload("Role").First(&user).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "User not found",
			})
		}
	}

	// 如果用戶沒有租戶（新註冊用戶），返回基本用戶信息
	if tenantID == uuid.Nil {
		var phoneCountryCode string
		if user.ExtraFields != nil {
			if phoneCC, ok := user.ExtraFields["phone_country_code"].(string); ok && phoneCC != "" {
				phoneCountryCode = phoneCC
			}
		}
		if phoneCountryCode == "" && user.Phone != "" {
			parts := strings.Split(user.Phone, " ")
			if len(parts) > 0 && strings.HasPrefix(parts[0], "+") {
				phoneCountryCode = parts[0]
			}
		}
		hasPassword := user.PasswordHash != "" && len(user.PasswordHash) > 0
		response := fiber.Map{
			"id":                 user.ID,
			"name":               user.Name,
			"email":              user.Email,
			"phone":              user.Phone,
			"phone_country_code": phoneCountryCode,
			"birth_date":         user.BirthDate,
			"profile_pic":        user.ProfilePic,
			"extra_fields":       user.ExtraFields,
			"has_password":       hasPassword,
			"status":             user.Status,
			"last_login_at":      user.LastLoginAt,
			"created_at":         user.CreatedAt,
			"tenant_id":          user.TenantID,
			"requires_setup":     true,
		}
		return c.JSON(response)
	}

	// 獲取租戶信息（包含 subdomain）
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
		// 將租戶信息添加到響應中
		// 從 ExtraFields 或 Phone 中提取 phone_country_code
		var phoneCountryCode string
		if user.ExtraFields != nil {
			if phoneCC, ok := user.ExtraFields["phone_country_code"].(string); ok && phoneCC != "" {
				phoneCountryCode = phoneCC
			}
		}
		// 如果 ExtraFields 中沒有，嘗試從 Phone 字段解析
		if phoneCountryCode == "" && user.Phone != "" {
			parts := strings.Split(user.Phone, " ")
			if len(parts) > 0 && strings.HasPrefix(parts[0], "+") {
				phoneCountryCode = parts[0]
				// 保存到 ExtraFields 以便下次使用
				if user.ExtraFields == nil {
					user.ExtraFields = make(models.JSONB)
				}
				user.ExtraFields["phone_country_code"] = phoneCountryCode
				// 更新數據庫（異步，不阻塞響應）
				go func() {
					database.DB.Model(&user).Update("extra_fields", user.ExtraFields)
				}()
			}
		}

		// 檢查用戶是否有密碼
		hasPassword := user.PasswordHash != "" && len(user.PasswordHash) > 0

		response := fiber.Map{
			"id":                 user.ID,
			"name":               user.Name,
			"email":              user.Email,
			"phone":              user.Phone,
			"phone_country_code": phoneCountryCode,
			"birth_date":         user.BirthDate,
			"profile_pic":        user.ProfilePic,
			"extra_fields":       user.ExtraFields,
			"has_password":       hasPassword,
			"level_id":           user.LevelID,
			"department_id":      user.DepartmentID,
			"role_id":            user.RoleID,
			"status":             user.Status,
			"last_login_at":      user.LastLoginAt,
			"created_at":         user.CreatedAt,
			"tenant_id":          user.TenantID,
			"tenant": fiber.Map{
				"id":        tenant.ID,
				"name":      tenant.Name,
				"subdomain": tenant.Subdomain,
			},
		}

		// 如果有 role，添加 role 信息
		if user.Role != nil {
			response["role"] = fiber.Map{
				"id":          user.Role.ID,
				"name":        user.Role.Name,
				"description": user.Role.Description,
			}
		}

		// 如果有 level，添加 level 信息（向後兼容）
		if user.Level != nil {
			response["level"] = fiber.Map{
				"id":          user.Level.ID,
				"name":        user.Level.Name,
				"permissions": user.Level.Permissions,
			}
		}

		// 如果有 department，添加 department 信息
		if user.Department != nil {
			response["department"] = fiber.Map{
				"id":   user.Department.ID,
				"name": user.Department.Name,
			}
		}

		return c.JSON(response)
	}

	// 不返回密碼哈希
	user.PasswordHash = ""
	return c.JSON(user)
}

// GetUserTenants 取得使用者可選租戶列表
func GetUserTenants(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "User ID not found"})
	}
	links, err := fetchUserTenants(userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch user tenants"})
	}
	list := buildTenantList(links)

	// 確保當前 JWT token 中的租戶也在列表中（防止遺漏）
	currentTenantID := middleware.GetTenantID(c)
	if currentTenantID != uuid.Nil {
		found := false
		for _, item := range list {
			if id, ok := item["id"].(uuid.UUID); ok && id == currentTenantID {
				found = true
				break
			}
		}
		if !found {
			// 查詢當前租戶信息並添加到列表
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", currentTenantID).First(&tenant).Error; err == nil {
				list = append([]fiber.Map{{
					"id":           tenant.ID,
					"name":         tenant.Name,
					"subdomain":    tenant.Subdomain,
					"plan":         tenant.Plan,
					"status":       tenant.Status,
					"last_used_at": nil,
				}}, list...)
				// 同時補建 user_tenants 關聯
				now := time.Now()
				link := models.UserTenant{
					UserID:     userID,
					TenantID:   currentTenantID,
					Role:       "member",
					IsDefault:  false,
					LastUsedAt: &now,
					CreatedAt:  now,
					UpdatedAt:  now,
				}
				_ = database.DB.Create(&link).Error
			}
		}
	}

	return c.JSON(fiber.Map{
		"data":  list,
		"total": len(list),
	})
}

// SelectTenant 切換使用者的目前租戶
func SelectTenant(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "User ID not found"})
	}
	var req struct {
		TenantID string `json:"tenant_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if strings.TrimSpace(req.TenantID) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "tenant_id is required"})
	}
	tenantUUID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid tenant_id"})
	}

	// 確認使用者有權限
	var link models.UserTenant
	if err := database.DB.Where("user_id = ? AND tenant_id = ?", userID, tenantUUID).
		Preload("Tenant").First(&link).Error; err != nil {
		return c.Status(403).JSON(fiber.Map{"error": "Permission denied"})
	}
	if link.Tenant == nil || link.Tenant.ID == uuid.Nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	if err := normalizeTenantStatus(link.Tenant); err != nil {
		return c.Status(403).JSON(fiber.Map{"error": "Tenant is inactive"})
	}

	// 更新使用者預設租戶與 last_used_at
	now := time.Now()
	_ = database.DB.Model(&models.User{}).Where("id = ?", userID).Update("tenant_id", tenantUUID).Error
	_ = database.DB.Model(&models.UserTenant{}).
		Where("user_id = ? AND tenant_id = ?", userID, tenantUUID).
		Updates(map[string]interface{}{"last_used_at": now, "updated_at": now, "is_default": true}).Error

	// 生成新 token
	var user models.User
	if err := database.DB.Where("id = ?", userID).Preload("Role").First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}
	roleName := user.UserRole
	if user.Role != nil && user.Role.Name != "" {
		roleName = user.Role.Name
	}
	token, err := utils.GenerateToken(user.ID, tenantUUID, user.Email, roleName, "web")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}
	// 更新 cookie
	setAuthCookie(c, token, 30*24*time.Hour)

	return c.JSON(fiber.Map{
		"message": "Tenant selected",
		"token":   token,
		"tenant": fiber.Map{
			"id":        link.Tenant.ID,
			"name":      link.Tenant.Name,
			"subdomain": link.Tenant.Subdomain,
			"plan":      link.Tenant.Plan,
		},
	})
}

// VerifyPasswordRequest 驗證密碼請求
type VerifyPasswordRequest struct {
	Password string `json:"password"`
}

// VerifyPassword 驗證當前用戶密碼
func VerifyPassword(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "User ID not found",
		})
	}
	tenantID := middleware.GetTenantID(c)
	// tenantID 為 Nil 是合法的（新註冊用戶尚未設置租戶）

	var user models.User
	if tenantID != uuid.Nil {
		if err := database.DB.Where("id = ? AND tenant_id = ?", userID, tenantID).First(&user).Error; err != nil {
			// Fallback: try by userID only
			if err2 := database.DB.Where("id = ?", userID).First(&user).Error; err2 != nil {
				return c.Status(404).JSON(fiber.Map{
					"error": "User not found",
				})
			}
		}
	} else {
		if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "User not found",
			})
		}
	}

	// 檢查用戶是否有密碼
	if user.PasswordHash == "" || len(user.PasswordHash) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error":        "User has no password set",
			"has_password": false,
		})
	}

	var req VerifyPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// 驗證密碼
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{
			"error": "Password incorrect",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Password verified",
	})
}

// UpdateCurrentUserRequest 更新當前用戶請求
type UpdateCurrentUserRequest struct {
	Name             *string `json:"name"`
	Email            *string `json:"email"`
	OldPassword      *string `json:"old_password"`
	Password         *string `json:"password"`
	ProfilePic       *string `json:"profile_pic"`
	BirthDate        *string `json:"birth_date"`
	Phone            *string `json:"phone"`
	PhoneCountryCode *string `json:"phone_country_code"`
}

// UpdateCurrentUser 更新當前登錄用戶信息
func UpdateCurrentUser(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "User ID not found",
		})
	}
	tenantID := middleware.GetTenantID(c)
	// 注意：tenantID 為 Nil 是合法的（新註冊用戶尚未設置租戶）

	var user models.User
	if tenantID != uuid.Nil {
		if err := database.DB.Where("id = ? AND tenant_id = ?", userID, tenantID).First(&user).Error; err != nil {
			// fallback: 嘗試只用 userID 查找
			if err2 := database.DB.Where("id = ?", userID).First(&user).Error; err2 != nil {
				return c.Status(404).JSON(fiber.Map{
					"error": "User not found",
				})
			}
		}
	} else {
		if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "User not found",
			})
		}
	}

	var req UpdateCurrentUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// 更新名稱
	if req.Name != nil {
		if len(*req.Name) == 0 {
			return c.Status(400).JSON(fiber.Map{
				"error": "Name cannot be empty",
			})
		}
		if utf8.RuneCountInString(*req.Name) > 50 {
			return c.Status(400).JSON(fiber.Map{
				"error": "Name must be at most 50 characters",
			})
		}
		user.Name = *req.Name
	}

	// 更新郵箱
	if req.Email != nil {
		// 檢查郵箱是否已被其他用戶使用
		var existingUser models.User
		if tenantID != uuid.Nil {
			if err := database.DB.Where("tenant_id = ? AND email = ? AND id != ?", tenantID, *req.Email, userID).First(&existingUser).Error; err == nil {
				return c.Status(400).JSON(fiber.Map{
					"error": "Email already in use",
				})
			}
		} else {
			if err := database.DB.Where("email = ? AND id != ? AND tenant_id IS NULL", *req.Email, userID).First(&existingUser).Error; err == nil {
				return c.Status(400).JSON(fiber.Map{
					"error": "Email already in use",
				})
			}
		}
		user.Email = *req.Email
	}

	// 更新密碼
	if req.Password != nil {
		if req.OldPassword == nil || strings.TrimSpace(*req.OldPassword) == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "Old password is required",
			})
		}
		if user.PasswordHash == "" || len(user.PasswordHash) == 0 {
			return c.Status(400).JSON(fiber.Map{
				"error":        "User has no password set",
				"has_password": false,
			})
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(*req.OldPassword)); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Old password incorrect",
			})
		}
		if len(*req.Password) < 6 {
			return c.Status(400).JSON(fiber.Map{
				"error": "Password must be at least 6 characters",
			})
		}
		if len(*req.Password) > 20 {
			return c.Status(400).JSON(fiber.Map{
				"error": "Password must be at most 20 characters",
			})
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to hash password",
			})
		}
		user.PasswordHash = string(hashedPassword)
	}

	// 更新頭像
	if req.ProfilePic != nil {
		user.ProfilePic = *req.ProfilePic // 允許空字串清空
	}

	// 更新出生日期
	if req.BirthDate != nil {
		if *req.BirthDate == "" {
			user.BirthDate = nil // 清空出生日期
		} else {
			if birthDate, err := time.Parse("2006-01-02", *req.BirthDate); err == nil {
				user.BirthDate = &birthDate
			}
		}
	}

	// 更新電話和電話區號
	if req.Phone != nil || req.PhoneCountryCode != nil {
		// 組合電話：如果有區號和號碼，組合它們
		var phoneCountryCode string
		var phoneNumber string

		// 獲取電話區號
		if req.PhoneCountryCode != nil {
			phoneCountryCode = strings.TrimSpace(*req.PhoneCountryCode)
		} else if user.ExtraFields != nil {
			if phoneCC, ok := user.ExtraFields["phone_country_code"].(string); ok {
				phoneCountryCode = phoneCC
			}
		}

		// 獲取電話號碼
		if req.Phone != nil {
			phoneNumber = strings.TrimSpace(*req.Phone)
		}

		// 組合完整電話
		if phoneNumber != "" {
			if phoneCountryCode != "" {
				// 若使用者已把區號打在電話欄，避免重複拼接
				if strings.HasPrefix(phoneNumber, phoneCountryCode) {
					user.Phone = phoneNumber
				} else {
					user.Phone = phoneCountryCode + " " + phoneNumber
				}
			} else {
				user.Phone = phoneNumber
			}
		} else {
			user.Phone = ""
		}

		// 保存電話區號到 ExtraFields
		if phoneCountryCode != "" {
			if user.ExtraFields == nil {
				user.ExtraFields = make(models.JSONB)
			}
			user.ExtraFields["phone_country_code"] = phoneCountryCode
		} else if req.PhoneCountryCode != nil && *req.PhoneCountryCode == "" {
			// 如果明確傳入空字符串，清除區號
			if user.ExtraFields != nil {
				delete(user.ExtraFields, "phone_country_code")
			}
		}
	}

	user.UpdatedAt = time.Now()

	// 只更新會變動的欄位，避免 Save() 觸碰到 DB 尚未存在/不同名的欄位（例如 user_role/role）
	updates := map[string]interface{}{
		"updated_at": user.UpdatedAt,
	}
	if req.Name != nil {
		updates["name"] = user.Name
	}
	if req.Email != nil {
		updates["email"] = user.Email
	}
	if req.Password != nil {
		updates["password_hash"] = user.PasswordHash
	}
	if req.ProfilePic != nil {
		updates["profile_pic"] = user.ProfilePic
	}
	if req.BirthDate != nil {
		updates["birth_date"] = user.BirthDate
	}
	if req.Phone != nil || req.PhoneCountryCode != nil {
		updates["phone"] = user.Phone
		if user.ExtraFields != nil && user.ExtraFields["phone_country_code"] != nil {
			updates["extra_fields"] = user.ExtraFields
		}
	}

	if tenantID != uuid.Nil {
		if err := database.DB.Model(&models.User{}).
			Where("id = ? AND tenant_id = ?", userID, tenantID).
			Updates(updates).Error; err != nil {
			log.Printf("Failed to update user(me): %v", err)
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update user: " + err.Error(),
			})
		}
	} else {
		if err := database.DB.Model(&models.User{}).
			Where("id = ?", userID).
			Updates(updates).Error; err != nil {
			log.Printf("Failed to update user(me): %v", err)
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update user: " + err.Error(),
			})
		}
	}

	// 不返回密碼哈希
	user.PasswordHash = ""
	return c.JSON(user)
}

// ChangePasswordRequest 變更密碼請求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword 變更當前用戶密碼
func ChangePassword(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "User ID not found",
		})
	}
	tenantID := middleware.GetTenantID(c)
	// tenantID 為 Nil 是合法的（新註冊用戶尚未設置租戶）

	log.Printf("ChangePassword request: user_id=%s tenant_id=%s", userID, tenantID)

	var user models.User
	if tenantID != uuid.Nil {
		if err := database.DB.Where("id = ? AND tenant_id = ?", userID, tenantID).First(&user).Error; err != nil {
			// Fallback: try by userID only
			if err2 := database.DB.Where("id = ?", userID).First(&user).Error; err2 != nil {
				return c.Status(404).JSON(fiber.Map{
					"error": "User not found",
				})
			}
		}
	} else {
		if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "User not found",
			})
		}
	}

	var req ChangePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	if strings.TrimSpace(req.OldPassword) == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Old password is required",
		})
	}
	if strings.TrimSpace(req.NewPassword) == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "New password is required",
		})
	}
	if len(req.NewPassword) < 6 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Password must be at least 6 characters",
		})
	}
	if len(req.NewPassword) > 20 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Password must be at most 20 characters",
		})
	}
	if user.PasswordHash == "" || len(user.PasswordHash) == 0 {
		return c.Status(400).JSON(fiber.Map{
			"error":        "User has no password set",
			"has_password": false,
		})
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Old password incorrect",
		})
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	// Update password - tenant-optional query
	updateQuery := database.DB.Model(&models.User{})
	if tenantID != uuid.Nil {
		updateQuery = updateQuery.Where("id = ? AND tenant_id = ?", userID, tenantID)
	} else {
		updateQuery = updateQuery.Where("id = ?", userID)
	}
	if err := updateQuery.Updates(map[string]interface{}{
		"password_hash": string(hashedPassword),
		"updated_at":    time.Now(),
	}).Error; err != nil {
		log.Printf("Failed to change password: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update password: " + err.Error(),
		})
	}

	log.Printf("ChangePassword success: user_id=%s tenant_id=%s", userID, tenantID)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Password updated",
	})
}

// ForgotPassword 忘記密碼（發送重置連結）
func ForgotPassword(c *fiber.Ctx) error {
	var req struct {
		Email string `json:"email"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	if req.Email == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email is required",
		})
	}

	// 查找用戶
	var user models.User
	if err := database.DB.Where("LOWER(email) = LOWER(?)", req.Email).First(&user).Error; err != nil {
		// 為了安全，即使用戶不存在也返回成功消息
		return c.JSON(fiber.Map{
			"message": "If the email exists, a password reset link has been sent",
		})
	}

	// 查找租戶 subdomain（用於生成連結）
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", user.TenantID).First(&tenant).Error; err != nil {
		// 不透露內部錯誤給前端
		log.Printf("⚠️  ForgotPassword tenant not found: email=%s user_id=%s tenant_id=%s err=%v", req.Email, user.ID, user.TenantID, err)
		return c.JSON(fiber.Map{
			"message": "If the email exists, a password reset link has been sent",
		})
	}

	// 生成 reset token（存 hash，寄出明文 token）
	token, tokenHash, err := newOpaqueToken()
	if err != nil {
		log.Printf("⚠️  ForgotPassword new token failed: email=%s user_id=%s err=%v", req.Email, user.ID, err)
		return c.JSON(fiber.Map{
			"message": "If the email exists, a password reset link has been sent",
		})
	}

	resetToken := models.PasswordResetToken{
		ID:        uuid.New(),
		TenantID:  tenant.ID,
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(30 * time.Minute),
		CreatedAt: time.Now(),
	}

	if err := database.DB.Create(&resetToken).Error; err != nil {
		log.Printf("⚠️  ForgotPassword save token failed: email=%s user_id=%s err=%v", req.Email, user.ID, err)
		return c.JSON(fiber.Map{
			"message": "If the email exists, a password reset link has been sent",
		})
	}

	resetURL, err := email.ResetPasswordURL(tenant.Subdomain, token)
	if err != nil {
		log.Printf("⚠️  ResetPasswordURL failed: %v", err)
		return c.JSON(fiber.Map{
			"message": "If the email exists, a password reset link has been sent",
		})
	}
	idempotencyKey := "password_reset:" + resetToken.ID.String()

	if err := email.EnqueuePasswordResetEmail(tenant.ID, user.ID, tenant.Subdomain, user.Email, user.Name, resetURL, idempotencyKey); err != nil {
		log.Printf("⚠️  enqueue password reset email failed: email=%s user_id=%s err=%v", req.Email, user.ID, err)
	}
	log.Printf("✅ Password reset requested for email: %s (User ID: %s)", req.Email, user.ID)

	return c.JSON(fiber.Map{
		"message": "If the email exists, a password reset link has been sent",
	})
}

// ResetPassword 重置密碼（使用 email token）
func ResetPassword(c *fiber.Ctx) error {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.Token == "" || req.NewPassword == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Token and new_password are required"})
	}
	if len(req.NewPassword) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": "Password must be at least 6 characters"})
	}
	if len(req.NewPassword) > 20 {
		return c.Status(400).JSON(fiber.Map{"error": "Password must be at most 20 characters"})
	}

	// token 可以是 query string 轉進來（前端有時會 encode）
	tokenStr, _ := url.QueryUnescape(req.Token)
	if tokenStr == "" {
		tokenStr = req.Token
	}

	hash := sha256.Sum256([]byte(tokenStr))

	var t models.PasswordResetToken
	if err := database.DB.Where("token_hash = ? AND used_at IS NULL AND expires_at > NOW()", hash[:]).First(&t).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid or expired token"})
	}

	// 更新密碼
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	if err := database.DB.Model(&models.User{}).Where("id = ?", t.UserID).Update("password_hash", string(hashedPassword)).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update password"})
	}

	now := time.Now()
	if err := database.DB.Model(&models.PasswordResetToken{}).Where("id = ?", t.ID).Update("used_at", now).Error; err != nil {
		// 密碼已改，但 token 沒標 used：仍然回成功，並記錄
		log.Printf("⚠️  ResetPassword mark token used failed: token_id=%s user_id=%s err=%v", t.ID, t.UserID, err)
	}

	return c.JSON(fiber.Map{"message": "Password reset successful"})
}

func newOpaqueToken() (token string, tokenHash []byte, err error) {
	// 32 bytes -> 43 chars base64url (no padding)
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(token))
	return token, sum[:], nil
}

// GoogleOAuthRequest Google OAuth 登錄請求
type GoogleOAuthRequest struct {
	Token string `json:"token"` // Google ID Token
}

// GoogleTokenInfo Google Token 驗證響應
type GoogleTokenInfo struct {
	Iss              string `json:"iss"`             // Issuer
	Sub              string `json:"sub"`             // Subject (Google user ID)
	Email            string `json:"email"`           // Email
	EmailVerified    bool   `json:"email_verified"`  // Email verified (can be string "true"/"false" or bool)
	Name             string `json:"name"`            // Full name
	Picture          string `json:"picture"`         // Profile picture URL
	GivenName        string `json:"given_name"`      // First name
	FamilyName       string `json:"family_name"`     // Last name
	Aud              string `json:"aud"`             // Audience (Client ID)
	Exp              int64  `json:"exp"`             // Expiration time
	Iat              int64  `json:"iat"`             // Issued at
	Error            string `json:"error,omitempty"` // Error if validation failed
	ErrorDescription string `json:"error_description,omitempty"`

	// 用於處理 email_verified 可能為字符串的情況
	EmailVerifiedStr string `json:"-"`
}

// UnmarshalJSON 自定義 JSON 解析，處理 email_verified 可能是字符串或布爾值
func (gti *GoogleTokenInfo) UnmarshalJSON(data []byte) error {
	// 先解析為 map 以便靈活處理
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// 提取基本字段
	if v, ok := raw["iss"].(string); ok {
		gti.Iss = v
	}
	if v, ok := raw["sub"].(string); ok {
		gti.Sub = v
	}
	if v, ok := raw["email"].(string); ok {
		gti.Email = v
	}
	if v, ok := raw["name"].(string); ok {
		gti.Name = v
	}
	if v, ok := raw["picture"].(string); ok {
		gti.Picture = v
	}
	if v, ok := raw["given_name"].(string); ok {
		gti.GivenName = v
	}
	if v, ok := raw["family_name"].(string); ok {
		gti.FamilyName = v
	}
	if v, ok := raw["aud"].(string); ok {
		gti.Aud = v
	}
	if v, ok := raw["exp"].(float64); ok {
		gti.Exp = int64(v)
	}
	if v, ok := raw["iat"].(float64); ok {
		gti.Iat = int64(v)
	}
	if v, ok := raw["error"].(string); ok {
		gti.Error = v
	}
	if v, ok := raw["error_description"].(string); ok {
		gti.ErrorDescription = v
	}

	// 處理 email_verified：可能是 bool 或 string
	if v, ok := raw["email_verified"].(bool); ok {
		gti.EmailVerified = v
	} else if v, ok := raw["email_verified"].(string); ok {
		gti.EmailVerified = (v == "true")
	} else if v, ok := raw["email_verified"].(float64); ok {
		// 有時可能是數字 1/0
		gti.EmailVerified = (v == 1)
	}

	return nil
}

// GoogleLogin Google OAuth 登錄
func GoogleLogin(c *fiber.Ctx) error {
	cfg := config.Load()

	// 檢查是否啟用 Google OAuth
	if !cfg.GoogleOAuth.Enabled {
		return c.Status(503).JSON(fiber.Map{
			"error": "Google OAuth is not enabled",
		})
	}

	if cfg.GoogleOAuth.ClientID == "" {
		return c.Status(500).JSON(fiber.Map{
			"error": "Google OAuth client ID is not configured",
		})
	}

	var req GoogleOAuthRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	if req.Token == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Google token is required",
		})
	}

	// 驗證 Google ID Token
	log.Printf("Verifying Google token, length: %d", len(req.Token))
	tokenInfo, err := verifyGoogleToken(req.Token, cfg.GoogleOAuth.ClientID)
	if err != nil {
		log.Printf("Google token verification failed: %v", err)
		return c.Status(401).JSON(fiber.Map{
			"error": fmt.Sprintf("Invalid Google token: %v", err),
		})
	}

	// 檢查 email 是否已驗證
	if !tokenInfo.EmailVerified {
		return c.Status(400).JSON(fiber.Map{
			"error": "Email not verified by Google",
		})
	}

	// 檢查 token 的 audience 是否匹配
	if tokenInfo.Aud != cfg.GoogleOAuth.ClientID {
		log.Printf("Google token audience mismatch: expected %s, got %s", cfg.GoogleOAuth.ClientID, tokenInfo.Aud)
		return c.Status(401).JSON(fiber.Map{
			"error": "Invalid token audience",
		})
	}

	// 查找或創建用戶
	var user models.User
	if err := database.DB.Where("LOWER(email) = LOWER(?)", tokenInfo.Email).First(&user).Error; err != nil {
		// 用戶不存在，創建新用戶
		log.Printf("Creating new user from Google OAuth: %s", tokenInfo.Email)

		now := time.Now()
		userName := tokenInfo.Name
		if userName == "" {
			userName = tokenInfo.GivenName
			if tokenInfo.FamilyName != "" {
				if userName != "" {
					userName += " " + tokenInfo.FamilyName
				} else {
					userName = tokenInfo.FamilyName
				}
			}
		}
		// 限制名稱長度
		if len(userName) > 12 {
			userName = userName[:12]
		}
		if userName == "" {
			// 如果還是沒有名稱，使用 email 的前綴
			emailParts := strings.Split(tokenInfo.Email, "@")
			userName = emailParts[0]
			if len(userName) > 12 {
				userName = userName[:12]
			}
		}

		user = models.User{
			TenantID:     nil, // 暫時沒有租戶
			Email:        strings.ToLower(tokenInfo.Email),
			PasswordHash: "", // Google OAuth 用戶沒有密碼
			Name:         userName,
			Phone:        "",
			BirthDate:    nil,
			UserRole:     "admin",
			Status:       "active",
			CreatedAt:    now,
			UpdatedAt:    now,
			ExtraFields: models.JSONB{
				"google_oauth": true,
				"google_id":    tokenInfo.Sub,
			},
		}

		if err := database.DB.Create(&user).Error; err != nil {
			log.Printf("Failed to create user from Google OAuth: %v", err)
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to create user",
			})
		}
	} else {
		// 用戶已存在，更新最後登錄時間
		now := time.Now()
		user.LastLoginAt = &now

		// 更新 ExtraFields 標記為 Google OAuth 用戶
		if user.ExtraFields == nil {
			user.ExtraFields = make(models.JSONB)
		}
		user.ExtraFields["google_oauth"] = true
		user.ExtraFields["google_id"] = tokenInfo.Sub

		// 如果用戶名稱是空的，嘗試更新
		if strings.TrimSpace(user.Name) == "" && tokenInfo.Name != "" {
			userName := tokenInfo.Name
			if len(userName) > 12 {
				userName = userName[:12]
			}
			user.Name = userName
		}

		database.DB.Save(&user)

		// 檢查用戶狀態
		if user.Status != "active" {
			return c.Status(403).JSON(fiber.Map{
				"error": "帳戶已被停用，請聯繫管理員",
			})
		}
	}

	// 處理租戶邏輯（與 Login 函數一致：使用 user_tenants 表查詢）
	// 建立/補齊 user_tenants 關聯（向後兼容：將 users.tenant_id 同步到 user_tenants）
	ensureUserTenantLink(&user)

	links, err := fetchUserTenants(user.ID)
	if err != nil {
		log.Printf("GoogleLogin failed: fetch user tenants error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch user tenants"})
	}

	// 共用的個人資料完整性檢查
	isProfileComplete := strings.TrimSpace(user.Name) != "" && strings.TrimSpace(user.Email) != ""
	hasPhone := strings.TrimSpace(user.Phone) != ""
	profileGuideSkipped := false
	if user.ExtraFields != nil {
		if skipped, ok := user.ExtraFields["profile_guide_skipped"].(bool); ok {
			profileGuideSkipped = skipped
		}
	}
	profileMap := fiber.Map{
		"complete": isProfileComplete,
		"hasPhone": hasPhone,
		"skipped":  profileGuideSkipped,
	}
	userMap := fiber.Map{
		"id":    user.ID,
		"email": user.Email,
		"name":  user.Name,
		"role":  user.UserRole,
	}

	// 沒有任何租戶
	if len(links) == 0 {
		log.Printf("GoogleLogin: user '%s' has no tenant, redirecting to setup", tokenInfo.Email)
		token, err := utils.GenerateToken(user.ID, uuid.Nil, user.Email, user.UserRole, "web")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
		}
		setAuthCookie(c, token, 30*24*time.Hour)
		return c.JSON(fiber.Map{
			"message":        "Login successful",
			"token":          token,
			"requires_setup": true,
			"user":           userMap,
			"profile":        profileMap,
		})
	}

	// 選擇租戶：單租戶自動選；多租戶需要前端選擇
	var selectedTenant *models.Tenant
	if len(links) == 1 {
		selectedTenant = links[0].Tenant
	}

	// 多租戶但未選擇
	if selectedTenant == nil {
		token, err := utils.GenerateToken(user.ID, uuid.Nil, user.Email, user.UserRole, "web")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
		}
		setAuthCookie(c, token, 30*24*time.Hour)
		return c.JSON(fiber.Map{
			"message":                   "Login successful",
			"token":                     token,
			"requires_tenant_selection": true,
			"tenants":                   buildTenantList(links),
			"last_tenant_id":            user.TenantID,
			"user":                      userMap,
			"profile":                   profileMap,
		})
	}

	// 檢查租戶狀態
	if err := normalizeTenantStatus(selectedTenant); err != nil {
		log.Printf("GoogleLogin failed: tenant '%s' is inactive for user '%s'", selectedTenant.ID, tokenInfo.Email)
		return c.Status(403).JSON(fiber.Map{
			"error": "帳戶所屬企業未激活，請聯繫管理員",
		})
	}

	// 更新最後登錄時間
	now := time.Now()
	user.LastLoginAt = &now
	database.DB.Save(&user)
	// 更新使用者預設租戶（用於下次自動選擇）
	if user.TenantID == nil || *user.TenantID != selectedTenant.ID {
		_ = database.DB.Model(&user).Update("tenant_id", selectedTenant.ID).Error
	}
	// 更新 user_tenants 的最後使用時間
	_ = database.DB.Model(&models.UserTenant{}).
		Where("user_id = ? AND tenant_id = ?", user.ID, selectedTenant.ID).
		Updates(map[string]interface{}{"last_used_at": now, "updated_at": now, "is_default": true}).Error

	// 生成 JWT Token
	roleName := user.UserRole
	if user.Role != nil {
		roleName = user.Role.Name
	}
	token, err := utils.GenerateToken(user.ID, selectedTenant.ID, user.Email, roleName, "web")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	setAuthCookie(c, token, 30*24*time.Hour)

	// 記錄登入活動
	utils.LogActivity(selectedTenant.ID, user.ID, "login", "auth", nil,
		fmt.Sprintf(`{"key":"auth.google_login","params":{"name":%q,"email":%q}}`, user.Name, user.Email), nil, c)

	return c.JSON(fiber.Map{
		"message": "Login successful",
		"token":   token,
		"user":    userMap,
		"tenant": fiber.Map{
			"id":        selectedTenant.ID,
			"name":      selectedTenant.Name,
			"subdomain": selectedTenant.Subdomain,
			"plan":      selectedTenant.Plan,
		},
		"profile": profileMap,
	})
}

// verifyGoogleToken 驗證 Google ID Token
func verifyGoogleToken(idToken, clientID string) (*GoogleTokenInfo, error) {
	// 使用 Google 的 tokeninfo 端點驗證 token
	tokenInfoURL := fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?id_token=%s", url.QueryEscape(idToken))
	log.Printf("Verifying Google token with URL: %s", tokenInfoURL)

	resp, err := http.Get(tokenInfoURL)
	if err != nil {
		log.Printf("Failed to request tokeninfo: %v", err)
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read tokeninfo response: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("Tokeninfo response status: %d, body: %s", resp.StatusCode, string(body))

	// 檢查 HTTP 狀態碼
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tokeninfo endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenInfo GoogleTokenInfo
	if err := json.Unmarshal(body, &tokenInfo); err != nil {
		log.Printf("Failed to parse tokeninfo JSON: %v, body: %s", err, string(body))
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// 檢查是否有錯誤
	if tokenInfo.Error != "" {
		log.Printf("Tokeninfo error: %s - %s", tokenInfo.Error, tokenInfo.ErrorDescription)
		return nil, fmt.Errorf("token verification failed: %s - %s", tokenInfo.Error, tokenInfo.ErrorDescription)
	}

	// 檢查必要的字段
	if tokenInfo.Email == "" {
		return nil, fmt.Errorf("token missing email field")
	}

	// 檢查 token 是否過期
	if tokenInfo.Exp > 0 {
		expTime := time.Unix(tokenInfo.Exp, 0)
		if time.Now().After(expTime) {
			return nil, fmt.Errorf("token has expired")
		}
	}

	log.Printf("Token verified successfully for email: %s", tokenInfo.Email)
	return &tokenInfo, nil
}

// CheckProfileCompleteRequest 檢查個人資料完整性請求
type CheckProfileCompleteRequest struct {
	Skip bool `json:"skip"` // 是否跳過（暫時標記為已查看）
}

// CheckProfileComplete 檢查個人資料是否完整
func CheckProfileComplete(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "User ID not found",
		})
	}

	var user models.User
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// 檢查個人資料是否完整
	// 必須字段：name, email
	// 可選但建議：phone
	isComplete := true
	missingFields := []string{}

	if strings.TrimSpace(user.Name) == "" {
		isComplete = false
		missingFields = append(missingFields, "name")
	}

	if strings.TrimSpace(user.Email) == "" {
		isComplete = false
		missingFields = append(missingFields, "email")
	}

	// 電話是可選的，但如果沒有則標記為建議填寫
	hasPhone := strings.TrimSpace(user.Phone) != ""

	return c.JSON(fiber.Map{
		"complete":      isComplete,
		"missingFields": missingFields,
		"hasPhone":      hasPhone,
		"user": fiber.Map{
			"name":  user.Name,
			"email": user.Email,
			"phone": user.Phone,
		},
	})
}

// UpdateProfileGuideRequest 更新個人資料（引導頁面）
type UpdateProfileGuideRequest struct {
	Name             *string `json:"name"`
	Phone            *string `json:"phone"`
	PhoneCountryCode *string `json:"phone_country_code"`
	BirthDate        *string `json:"birth_date"`
	Skip             bool    `json:"skip"` // 是否跳過（暫時標記）
}

// UpdateProfileGuide 更新個人資料（引導頁面）
func UpdateProfileGuide(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "User ID not found",
		})
	}

	var user models.User
	if err := database.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var req UpdateProfileGuideRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// 更新名稱
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name != "" && len(name) <= 12 {
			user.Name = name
		}
	}

	// 更新電話和電話區號
	if req.Phone != nil || req.PhoneCountryCode != nil {
		var phoneCountryCode string
		var phoneNumber string

		if req.PhoneCountryCode != nil {
			phoneCountryCode = strings.TrimSpace(*req.PhoneCountryCode)
		} else if user.ExtraFields != nil {
			if phoneCC, ok := user.ExtraFields["phone_country_code"].(string); ok {
				phoneCountryCode = phoneCC
			}
		}

		if req.Phone != nil {
			phoneNumber = strings.TrimSpace(*req.Phone)
		}

		if phoneNumber != "" {
			if phoneCountryCode != "" {
				if strings.HasPrefix(phoneNumber, phoneCountryCode) {
					user.Phone = phoneNumber
				} else {
					user.Phone = phoneCountryCode + " " + phoneNumber
				}
			} else {
				user.Phone = phoneNumber
			}
		}

		if phoneCountryCode != "" {
			if user.ExtraFields == nil {
				user.ExtraFields = make(models.JSONB)
			}
			user.ExtraFields["phone_country_code"] = phoneCountryCode
		}
	}

	// 更新出生日期
	if req.BirthDate != nil {
		if *req.BirthDate == "" {
			user.BirthDate = nil
		} else {
			if birthDate, err := time.Parse("2006-01-02", *req.BirthDate); err == nil {
				user.BirthDate = &birthDate
			}
		}
	}

	// 如果跳過，在 ExtraFields 中標記
	if req.Skip {
		if user.ExtraFields == nil {
			user.ExtraFields = make(models.JSONB)
		}
		user.ExtraFields["profile_guide_skipped"] = true
		user.ExtraFields["profile_guide_skipped_at"] = time.Now().Format(time.RFC3339)
	}

	user.UpdatedAt = time.Now()

	if err := database.DB.Save(&user).Error; err != nil {
		log.Printf("❌ Failed to update user profile: %v", err)
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update profile",
		})
	}

	// 重新檢查資料完整性
	isComplete := strings.TrimSpace(user.Name) != "" && strings.TrimSpace(user.Email) != ""
	hasPhone := strings.TrimSpace(user.Phone) != ""

	return c.JSON(fiber.Map{
		"message":  "Profile updated successfully",
		"complete": isComplete,
		"hasPhone": hasPhone,
		"skipped":  req.Skip,
	})
}

// GetGoogleOAuthConfig 獲取 Google OAuth 配置（公開 API，用於前端初始化）
func GetGoogleOAuthConfig(c *fiber.Ctx) error {
	cfg := config.Load()

	// 只返回客戶端 ID 和是否啟用，不返回 Client Secret（安全考慮）
	return c.JSON(fiber.Map{
		"client_id": cfg.GoogleOAuth.ClientID,
		"enabled":   cfg.GoogleOAuth.Enabled,
	})
}

// Logout 清除 auth_token HTTPOnly cookie 並記錄全域登出時間（SSO global logout）
// 所有在此時間之前簽發的 JWT token 都會被 auth middleware 拒絕，
// 實現跨域（跨產品）單點登出。
func Logout(c *fiber.Ctx) error {
	// Clear the HTTPOnly cookie
	clearAuthCookie(c)

	// Try to identify the user from the token (best-effort: token may already be invalid)
	// We parse the JWT directly instead of relying on middleware, because this endpoint
	// may be called with an already-invalidated token.
	var userID uuid.UUID
	tokenString := c.Cookies("auth_token")
	if tokenString == "" {
		if auth := c.Get("Authorization"); auth != "" {
			tokenString = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	// Determine platform from the token's claims (web logout only invalidates web tokens, etc.)
	platform := "web" // default for legacy tokens without platform claim
	if tokenString != "" {
		if claims, err := utils.ValidateToken(tokenString); err == nil {
			userID = claims.UserID
			if claims.Platform != "" {
				platform = claims.Platform
			}
		}
	}
	// Fallback: if middleware already set user_id (normal flow)
	if userID == uuid.Nil {
		userID = middleware.GetUserID(c)
	}

	// Update the platform-specific logged_out_at so only tokens for this platform become invalid.
	// Web logout does NOT invalidate desktop tokens, and vice versa.
	if userID != uuid.Nil {
		now := time.Now()
		var column string
		switch platform {
		case "desktop":
			column = "desktop_logged_out_at"
		default:
			column = "web_logged_out_at"
		}
		if err := database.DB.Model(&models.User{}).Where("id = ?", userID).
			Update(column, now).Error; err != nil {
			log.Printf("[Logout] Failed to update %s for user %s: %v", column, userID, err)
		} else {
			log.Printf("[Logout] Platform logout: user=%s platform=%s %s=%s", userID, platform, column, now.Format(time.RFC3339))
		}
	}

	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}
