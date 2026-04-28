package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// GenerateEmployeeNumber 生成員工編號（基於 user_tenants 表）
// 格式：EMP-YYYYMMDD-001
// 每個租戶獨立編號，確保唯一性
func GenerateEmployeeNumber(tenantID uuid.UUID) string {
	today := time.Now().Format("20060102")
	datePrefix := "EMP-" + today + "-"

	// 查詢今天該租戶已有的員工編號數量（從 user_tenants 表）
	var count int64
	database.DB.Model(&models.UserTenant{}).Where("tenant_id = ? AND employee_number LIKE ?", tenantID, datePrefix+"%").Count(&count)

	// 查詢今天已預留的編號
	var reservedCount int64
	database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
		tenantID, "employee_number", datePrefix+"%").Count(&reservedCount)

	sequence := count + reservedCount + 1
	employeeNumber := datePrefix + fmt.Sprintf("%03d", sequence)

	// 確保唯一性：如果編號已存在於 user_tenants，遞增序號
	for {
		var existingUT models.UserTenant
		if err := database.DB.Where("tenant_id = ? AND employee_number = ?", tenantID, employeeNumber).First(&existingUT).Error; err != nil {
			// 編號不存在，可以使用
			break
		}
		// 編號已存在，遞增序號
		sequence++
		employeeNumber = datePrefix + fmt.Sprintf("%03d", sequence)
	}

	return employeeNumber
}

// GetUsers 獲取用戶列表（同一租戶下的所有用戶）
func GetUsers(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var users []models.User
	query := database.DB.Where("tenant_id = ?", tenantID)

	// 搜索
	search := c.Query("search")
	if search != "" {
		query = query.Where("name ILIKE ? OR email ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 狀態過濾
	status := c.Query("status")
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 角色過濾（通過 role_id 關聯查詢）
	roleID := c.Query("role_id")
	if roleID != "" {
		if roleUUID, err := uuid.Parse(roleID); err == nil {
			query = query.Where("role_id = ?", roleUUID)
		}
	}

	var total int64
	query.Model(&models.User{}).Count(&total)

	if err := query.Preload("Level").Preload("Department").Preload("Role").
		Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 移除密碼哈希
	for i := range users {
		users[i].PasswordHash = ""
	}

	return c.JSON(fiber.Map{
		"data":  users,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetUser 獲取單個用戶
func GetUser(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var user models.User
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("Level").Preload("Department").Preload("Role").First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// 從 ExtraFields 或 Phone 中提取 phone_country_code
	if user.ExtraFields != nil {
		if phoneCC, ok := user.ExtraFields["phone_country_code"].(string); ok && phoneCC != "" {
			// 已經在 ExtraFields 中，直接使用
		} else if user.Phone != "" {
			// 嘗試從 Phone 字段解析
			parts := strings.Split(user.Phone, " ")
			if len(parts) > 0 && strings.HasPrefix(parts[0], "+") {
				if user.ExtraFields == nil {
					user.ExtraFields = make(models.JSONB)
				}
				user.ExtraFields["phone_country_code"] = parts[0]
			}
		}
	} else if user.Phone != "" {
		// 嘗試從 Phone 字段解析
		parts := strings.Split(user.Phone, " ")
		if len(parts) > 0 && strings.HasPrefix(parts[0], "+") {
			if user.ExtraFields == nil {
				user.ExtraFields = make(models.JSONB)
			}
			user.ExtraFields["phone_country_code"] = parts[0]
		}
	}

	user.PasswordHash = ""
	return c.JSON(user)
}

// CreateUser 創建用戶（同一租戶下）
func CreateUser(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Email           string                 `json:"email"`
		Password        string                 `json:"password"`
		Name            string                 `json:"name"`
		EmployeeNumber  string                 `json:"employee_number"`
		ProfilePic      string                 `json:"profile_pic"`
		BirthDate       *string                `json:"birth_date"`
		Phone           string                 `json:"phone"`
		RoleID          *uuid.UUID             `json:"role_id"`
		Status          string                 `json:"status"`
		LevelID         *uuid.UUID             `json:"level_id"`
		DepartmentID    *uuid.UUID             `json:"department_id"`
		Salary          *float64               `json:"salary"`
		CommissionMode  *string                `json:"commission_mode"`
		CommissionRate  *float64               `json:"commission_rate"`
		CommissionFixed *float64               `json:"commission_fixed"`
		ExtraFields     map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 驗證必填字段
	if req.Email == "" || req.Password == "" || req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email, password, and name are required"})
	}

	// 驗證用戶名稱：最多 50 位
	if utf8.RuneCountInString(req.Name) > 50 {
		return c.Status(400).JSON(fiber.Map{"error": "Name must be at most 50 characters"})
	}

	// 驗證密碼：最小 6 位，最多 20 位
	if len(req.Password) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": "Password must be at least 6 characters"})
	}
	if len(req.Password) > 20 {
		return c.Status(400).JSON(fiber.Map{"error": "Password must be at most 20 characters"})
	}

	// 檢查郵箱是否已存在（同一租戶下）
	var existingUser models.User
	if err := database.DB.Where("tenant_id = ? AND email = ?", tenantID, req.Email).First(&existingUser).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Email already exists in this tenant"})
	}

	// 哈希密碼
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	// 設置默認值
	if req.Status == "" {
		req.Status = "active"
	}

	// 驗證角色（如果提供了 role_id）
	if req.RoleID != nil && *req.RoleID != uuid.Nil {
		var role models.Role
		if err := database.DB.Where("id = ? AND tenant_id = ?", *req.RoleID, tenantID).First(&role).Error; err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid role ID: role not found or does not belong to this tenant"})
		}
	}

	// 解析出生日期
	var birthDate *time.Time
	if req.BirthDate != nil && *req.BirthDate != "" {
		if parsedDate, err := time.Parse("2006-01-02", *req.BirthDate); err == nil {
			birthDate = &parsedDate
		}
	}

	// 如果沒有提供員工編號，自動生成
	employeeNumber := req.EmployeeNumber
	if employeeNumber == "" {
		employeeNumber = GenerateEmployeeNumber(tenantID)
	}

	now := time.Now()
	user := models.User{
		TenantID:       &tenantID,
		Email:          req.Email,
		PasswordHash:   string(hashedPassword),
		Name:           req.Name,
		EmployeeNumber: employeeNumber,
		ProfilePic:     req.ProfilePic,
		BirthDate:      birthDate,
		Phone:          req.Phone,
		RoleID:         req.RoleID,
		Status:         req.Status,
		LevelID:        req.LevelID,
		DepartmentID:   req.DepartmentID,
		Salary:         0,
		CommissionRate: 0,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExtraFields:    make(models.JSONB),
	}
	if req.Salary != nil {
		user.Salary = *req.Salary
	}

	// 佣金設定（訂單和服務單分開）
	// 處理訂單佣金設定
	if req.ExtraFields != nil {
		if orderMode, ok := req.ExtraFields["order_commission_mode"].(string); ok && orderMode != "" {
			if orderMode != "fixed" && orderMode != "percent" {
				orderMode = "percent"
			}
			user.ExtraFields["order_commission_mode"] = orderMode
			if orderMode == "fixed" {
				if orderFixed, ok := req.ExtraFields["order_commission_fixed"].(float64); ok {
					user.ExtraFields["order_commission_fixed"] = orderFixed
				} else {
					user.ExtraFields["order_commission_fixed"] = 0
				}
				user.ExtraFields["order_commission_rate"] = 0
			} else {
				if orderRate, ok := req.ExtraFields["order_commission_rate"].(float64); ok {
					user.ExtraFields["order_commission_rate"] = orderRate
				} else {
					user.ExtraFields["order_commission_rate"] = 0
				}
				user.ExtraFields["order_commission_fixed"] = 0
			}
		}
		// 處理服務單佣金設定
		if serviceMode, ok := req.ExtraFields["service_order_commission_mode"].(string); ok && serviceMode != "" {
			if serviceMode != "fixed" && serviceMode != "percent" {
				serviceMode = "percent"
			}
			user.ExtraFields["service_order_commission_mode"] = serviceMode
			if serviceMode == "fixed" {
				if serviceFixed, ok := req.ExtraFields["service_order_commission_fixed"].(float64); ok {
					user.ExtraFields["service_order_commission_fixed"] = serviceFixed
				} else {
					user.ExtraFields["service_order_commission_fixed"] = 0
				}
				user.ExtraFields["service_order_commission_rate"] = 0
			} else {
				if serviceRate, ok := req.ExtraFields["service_order_commission_rate"].(float64); ok {
					user.ExtraFields["service_order_commission_rate"] = serviceRate
				} else {
					user.ExtraFields["service_order_commission_rate"] = 0
				}
				user.ExtraFields["service_order_commission_fixed"] = 0
			}
		}
	}

	// 向後兼容：如果沒有設置新的字段，使用舊的 commission_mode/rate/fixed 作為默認值
	if req.CommissionMode != nil && *req.CommissionMode != "" {
		commissionMode := *req.CommissionMode
		if commissionMode != "fixed" && commissionMode != "percent" {
			commissionMode = "percent"
		}
		// 如果沒有設置訂單佣金，使用舊值
		if _, ok := user.ExtraFields["order_commission_mode"]; !ok {
			user.ExtraFields["order_commission_mode"] = commissionMode
			if commissionMode == "fixed" {
				if req.CommissionFixed != nil {
					user.ExtraFields["order_commission_fixed"] = *req.CommissionFixed
				} else {
					user.ExtraFields["order_commission_fixed"] = 0
				}
				user.ExtraFields["order_commission_rate"] = 0
			} else {
				if req.CommissionRate != nil {
					user.ExtraFields["order_commission_rate"] = *req.CommissionRate
					user.CommissionRate = *req.CommissionRate
				} else {
					user.ExtraFields["order_commission_rate"] = 0
				}
				user.ExtraFields["order_commission_fixed"] = 0
			}
		}
		// 如果沒有設置服務單佣金，使用舊值
		if _, ok := user.ExtraFields["service_order_commission_mode"]; !ok {
			user.ExtraFields["service_order_commission_mode"] = commissionMode
			if commissionMode == "fixed" {
				if req.CommissionFixed != nil {
					user.ExtraFields["service_order_commission_fixed"] = *req.CommissionFixed
				} else {
					user.ExtraFields["service_order_commission_fixed"] = 0
				}
				user.ExtraFields["service_order_commission_rate"] = 0
			} else {
				if req.CommissionRate != nil {
					user.ExtraFields["service_order_commission_rate"] = *req.CommissionRate
				} else {
					user.ExtraFields["service_order_commission_rate"] = 0
				}
				user.ExtraFields["service_order_commission_fixed"] = 0
			}
		}
		// 保留舊字段用於向後兼容
		user.ExtraFields["commission_mode"] = commissionMode
		if commissionMode == "fixed" {
			if req.CommissionFixed != nil {
				user.ExtraFields["commission_fixed"] = *req.CommissionFixed
			} else {
				user.ExtraFields["commission_fixed"] = 0
			}
			user.CommissionRate = 0
		} else {
			if req.CommissionRate != nil {
				user.CommissionRate = *req.CommissionRate
			}
			user.ExtraFields["commission_fixed"] = 0
		}
	}

	if req.ExtraFields != nil {
		// merge
		for k, v := range req.ExtraFields {
			user.ExtraFields[k] = v
		}
	}

	// 使用 Omit 排除不存在的 user_role 字段
	if err := database.DB.Omit("user_role").Create(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create user"})
	}

	// 創建 user_tenants 記錄並設定員工編號
	nowUT := time.Now()
	userTenant := models.UserTenant{
		UserID:         user.ID,
		TenantID:       tenantID,
		EmployeeNumber: employeeNumber,
		IsDefault:      true,
		LastUsedAt:     &nowUT,
		CreatedAt:      nowUT,
		UpdatedAt:      nowUT,
	}
	if err := database.DB.Create(&userTenant).Error; err != nil {
		log.Printf("⚠️  create user_tenants failed: user_id=%s tenant_id=%s err=%v", user.ID, tenantID, err)
	}

	// 同時更新 users 表的 employee_number（向後兼容）
	if employeeNumber != "" {
		database.DB.Model(&user).Update("employee_number", employeeNumber)
		user.EmployeeNumber = employeeNumber
	}

	// 獲取當前登錄用戶（創建者）
	currentUserID := middleware.GetUserID(c)

	// 記錄活動：用戶加入租戶（新用戶被創建並加入到租戶，使用同步方式確保記錄成功）
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
		changes := map[string]interface{}{
			"tenant_id":   tenant.ID.String(),
			"tenant_name": tenant.Name,
			"user_id":     user.ID.String(),
			"user_name":   user.Name,
			"user_email":  user.Email,
			"created_by":  currentUserID.String(),
		}
		if err := utils.LogActivitySync(tenantID, currentUserID, "join", "tenant", &tenant.ID,
			fmt.Sprintf(`{"key":"tenant.join","params":{"name":%q,"email":%q,"tenant_name":%q}}`, user.Name, user.Email, tenant.Name),
			changes, c); err != nil {
			log.Printf("⚠️  Failed to log user join tenant activity: %v", err)
			// 不中斷主流程，但記錄錯誤
		}
	}

	user.PasswordHash = ""
	return c.Status(201).JSON(user)
}

// UpdateUser 更新用戶
func UpdateUser(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	var user models.User
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	var req struct {
		Email           *string                 `json:"email"`
		Password        *string                 `json:"password"`
		Name            *string                 `json:"name"`
		EmployeeNumber  *string                 `json:"employee_number"`
		ProfilePic      *string                 `json:"profile_pic"`
		BirthDate       *string                 `json:"birth_date"`
		Phone           *string                 `json:"phone"`
		RoleID          *uuid.UUID              `json:"role_id"`
		Status          *string                 `json:"status"`
		LevelID         *uuid.UUID              `json:"level_id"`
		DepartmentID    *uuid.UUID              `json:"department_id"`
		ShiftID         *uuid.UUID              `json:"shift_id"`
		Salary          *float64                `json:"salary"`
		CommissionRate  *float64                `json:"commission_rate"`
		CommissionMode  *string                 `json:"commission_mode"`
		CommissionFixed *float64                `json:"commission_fixed"`
		ExtraFields     *map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 更新字段
	if req.Email != nil {
		// 檢查郵箱是否已被其他用戶使用
		var existingUser models.User
		if err := database.DB.Where("tenant_id = ? AND email = ? AND id != ?", tenantID, *req.Email, id).First(&existingUser).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Email already exists"})
		}
		user.Email = *req.Email
	}

	if req.Name != nil {
		if utf8.RuneCountInString(*req.Name) > 50 {
			return c.Status(400).JSON(fiber.Map{"error": "Name must be at most 50 characters"})
		}
		user.Name = *req.Name
	}

	if req.Password != nil {
		if len(*req.Password) < 6 {
			return c.Status(400).JSON(fiber.Map{"error": "Password must be at least 6 characters"})
		}
		if len(*req.Password) > 20 {
			return c.Status(400).JSON(fiber.Map{"error": "Password must be at most 20 characters"})
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
		}
		user.PasswordHash = string(hashedPassword)
	}

	if req.RoleID != nil {
		// 如果 RoleID 是空 UUID，設置為 nil 以清空關聯
		if *req.RoleID == uuid.Nil {
			user.RoleID = nil
		} else {
			// 驗證角色是否存在且屬於當前租戶
			var role models.Role
			if err := database.DB.Where("id = ? AND tenant_id = ?", *req.RoleID, tenantID).First(&role).Error; err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Invalid role ID: role not found or does not belong to this tenant"})
			}
			user.RoleID = req.RoleID
		}
	}

	if req.Status != nil {
		user.Status = *req.Status
	}

	if req.LevelID != nil {
		user.LevelID = req.LevelID
	}

	if req.DepartmentID != nil {
		user.DepartmentID = req.DepartmentID
	}
	if req.ShiftID != nil {
		user.ShiftID = req.ShiftID
	}
	if req.ProfilePic != nil {
		user.ProfilePic = *req.ProfilePic // 允許空字符串，用於清空頭像
	}
	if req.BirthDate != nil {
		if *req.BirthDate == "" {
			user.BirthDate = nil // 清空出生日期
		} else {
			if birthDate, err := time.Parse("2006-01-02", *req.BirthDate); err == nil {
				user.BirthDate = &birthDate
			}
		}
	}
	if req.Phone != nil {
		user.Phone = *req.Phone // 允許空字串清空
	}

	// 處理 phone_country_code：如果 ExtraFields 中包含，提取並保存
	if req.ExtraFields != nil {
		if phoneCC, ok := (*req.ExtraFields)["phone_country_code"].(string); ok && phoneCC != "" {
			if user.ExtraFields == nil {
				user.ExtraFields = make(models.JSONB)
			}
			user.ExtraFields["phone_country_code"] = phoneCC
		}
	}
	if req.Salary != nil {
		user.Salary = *req.Salary
	}

	// ExtraFields：若有傳則整體替換；否則在需要時進行 merge
	if req.ExtraFields != nil {
		user.ExtraFields = models.JSONB(*req.ExtraFields)
	} else if user.ExtraFields == nil {
		user.ExtraFields = make(models.JSONB)
	}

	// 佣金設定（訂單和服務單分開）
	// 處理訂單佣金設定
	if req.ExtraFields != nil {
		if orderMode, ok := (*req.ExtraFields)["order_commission_mode"].(string); ok && orderMode != "" {
			if orderMode != "fixed" && orderMode != "percent" {
				orderMode = "percent"
			}
			user.ExtraFields["order_commission_mode"] = orderMode
			if orderMode == "fixed" {
				if orderFixed, ok := (*req.ExtraFields)["order_commission_fixed"].(float64); ok {
					user.ExtraFields["order_commission_fixed"] = orderFixed
				} else {
					user.ExtraFields["order_commission_fixed"] = 0
				}
				user.ExtraFields["order_commission_rate"] = 0
			} else {
				if orderRate, ok := (*req.ExtraFields)["order_commission_rate"].(float64); ok {
					user.ExtraFields["order_commission_rate"] = orderRate
				} else {
					user.ExtraFields["order_commission_rate"] = 0
				}
				user.ExtraFields["order_commission_fixed"] = 0
			}
		}
		// 處理服務單佣金設定
		if serviceMode, ok := (*req.ExtraFields)["service_order_commission_mode"].(string); ok && serviceMode != "" {
			if serviceMode != "fixed" && serviceMode != "percent" {
				serviceMode = "percent"
			}
			user.ExtraFields["service_order_commission_mode"] = serviceMode
			if serviceMode == "fixed" {
				if serviceFixed, ok := (*req.ExtraFields)["service_order_commission_fixed"].(float64); ok {
					user.ExtraFields["service_order_commission_fixed"] = serviceFixed
				} else {
					user.ExtraFields["service_order_commission_fixed"] = 0
				}
				user.ExtraFields["service_order_commission_rate"] = 0
			} else {
				if serviceRate, ok := (*req.ExtraFields)["service_order_commission_rate"].(float64); ok {
					user.ExtraFields["service_order_commission_rate"] = serviceRate
				} else {
					user.ExtraFields["service_order_commission_rate"] = 0
				}
				user.ExtraFields["service_order_commission_fixed"] = 0
			}
		}
	}

	// 向後兼容：如果沒有設置新的字段，使用舊的 commission_mode/rate/fixed 作為默認值
	if req.CommissionMode != nil && *req.CommissionMode != "" {
		mode := *req.CommissionMode
		if mode != "fixed" && mode != "percent" {
			mode = "percent"
		}
		// 如果沒有設置訂單佣金，使用舊值
		if _, ok := user.ExtraFields["order_commission_mode"]; !ok {
			user.ExtraFields["order_commission_mode"] = mode
			if mode == "fixed" {
				if req.CommissionFixed != nil {
					user.ExtraFields["order_commission_fixed"] = *req.CommissionFixed
				} else {
					user.ExtraFields["order_commission_fixed"] = 0
				}
				user.ExtraFields["order_commission_rate"] = 0
			} else {
				if req.CommissionRate != nil {
					user.ExtraFields["order_commission_rate"] = *req.CommissionRate
					user.CommissionRate = *req.CommissionRate
				} else {
					user.ExtraFields["order_commission_rate"] = 0
				}
				user.ExtraFields["order_commission_fixed"] = 0
			}
		}
		// 如果沒有設置服務單佣金，使用舊值
		if _, ok := user.ExtraFields["service_order_commission_mode"]; !ok {
			user.ExtraFields["service_order_commission_mode"] = mode
			if mode == "fixed" {
				if req.CommissionFixed != nil {
					user.ExtraFields["service_order_commission_fixed"] = *req.CommissionFixed
				} else {
					user.ExtraFields["service_order_commission_fixed"] = 0
				}
				user.ExtraFields["service_order_commission_rate"] = 0
			} else {
				if req.CommissionRate != nil {
					user.ExtraFields["service_order_commission_rate"] = *req.CommissionRate
				} else {
					user.ExtraFields["service_order_commission_rate"] = 0
				}
				user.ExtraFields["service_order_commission_fixed"] = 0
			}
		}
		// 保留舊字段用於向後兼容
		user.ExtraFields["commission_mode"] = mode
		if mode == "fixed" {
			if req.CommissionFixed != nil {
				user.ExtraFields["commission_fixed"] = *req.CommissionFixed
			}
			user.CommissionRate = 0
		} else {
			if req.CommissionRate != nil {
				user.CommissionRate = *req.CommissionRate
			}
			user.ExtraFields["commission_fixed"] = 0
		}
	} else {
		// fallback: update commission_rate only if provided and no mode change
		if req.CommissionRate != nil {
			user.CommissionRate = *req.CommissionRate
			// keep mode percent by default if not set
			if _, ok := user.ExtraFields["commission_mode"]; !ok {
				user.ExtraFields["commission_mode"] = "percent"
			}
		}
		if req.CommissionFixed != nil {
			user.ExtraFields["commission_fixed"] = *req.CommissionFixed
			user.ExtraFields["commission_mode"] = "fixed"
			user.CommissionRate = 0
		}
	}

	user.UpdatedAt = time.Now()

	// 使用 Select 明確指定要更新的字段，避免更新不存在的 user_role 字段
	// 注意：employee_number 是唯讀字段（在創建時自動生成），不應該在更新時被覆蓋
	if err := database.DB.Model(&user).Select(
		"email", "password_hash", "name", "role_id", "phone",
		"status", "level_id", "department_id", "shift_id", "salary", "commission_rate", "profile_pic", "birth_date", "extra_fields", "updated_at",
	).Updates(&user).Error; err != nil {
		log.Printf("Failed to update user: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update user: " + err.Error()})
	}

	user.PasswordHash = ""
	return c.JSON(user)
}

// DeleteUser 刪除用戶
func DeleteUser(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	// 檢查用戶是否存在
	var user models.User
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// 不允許刪除自己
	currentUserID := middleware.GetUserID(c)
	if user.ID == currentUserID {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot delete yourself"})
	}

	// 檢查是否為租戶主帳號（該租戶下最早創建的用戶）
	var firstUser models.User
	if err := database.DB.Where("tenant_id = ?", tenantID).Order("created_at ASC").First(&firstUser).Error; err == nil {
		if user.ID == firstUser.ID {
			return c.Status(400).JSON(fiber.Map{"error": "Cannot delete tenant owner account"})
		}
	}

	if err := database.DB.Delete(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete user"})
	}

	return c.JSON(fiber.Map{"message": "User deleted successfully"})
}

// InviteUser 邀請用戶加入租戶（如果用戶不存在則創建）
func InviteUser(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	currentUserID := middleware.GetUserID(c)

	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 驗證必填字段
	if req.Email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Email is required"})
	}

	// 清理 email
	email := strings.TrimSpace(strings.ToLower(req.Email))

	// 獲取租戶信息
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// 檢查用戶是否已存在於系統中
	var existingUser models.User
	userExists := database.DB.Where("LOWER(email) = ?", email).First(&existingUser).Error == nil

	var user models.User
	var isNewUser bool

	if userExists {
		// 用戶已存在，檢查是否已在此租戶
		var userTenant models.UserTenant
		if err := database.DB.Where("user_id = ? AND tenant_id = ?", existingUser.ID, tenantID).First(&userTenant).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "User is already a member of this tenant"})
		}

		// 用戶存在但不在此租戶，將其添加到租戶
		userTenant = models.UserTenant{
			ID:        uuid.New(),
			UserID:    existingUser.ID,
			TenantID:  tenantID,
			Role:      "user",
			IsDefault: false,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := database.DB.Create(&userTenant).Error; err != nil {
			log.Printf("⚠️  InviteUser: failed to add user to tenant: user_id=%s tenant_id=%s err=%v", existingUser.ID, tenantID, err)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to add user to tenant"})
		}

		user = existingUser
		isNewUser = false
	} else {
		// 用戶不存在，創建新用戶（狀態為 pending，無密碼）
		userName := strings.TrimSpace(req.Name)
		if userName == "" {
			// 從 email 中提取名稱
			parts := strings.Split(email, "@")
			userName = parts[0]
		}

		now := time.Now()
		user = models.User{
			ID:           uuid.New(),
			TenantID:     &tenantID,
			Email:        email,
			PasswordHash: "", // 無密碼，待用戶設置
			Name:         userName,
			Status:       "pending", // 待激活
			CreatedAt:    now,
			UpdatedAt:    now,
			ExtraFields:  make(models.JSONB),
		}

		if err := database.DB.Omit("user_role").Create(&user).Error; err != nil {
			log.Printf("⚠️  InviteUser: failed to create user: email=%s err=%v", email, err)
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create user"})
		}

		// 創建 user_tenant 關聯
		userTenant := models.UserTenant{
			ID:        uuid.New(),
			UserID:    user.ID,
			TenantID:  tenantID,
			Role:      "user",
			IsDefault: true, // 第一個租戶為默認
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := database.DB.Create(&userTenant).Error; err != nil {
			log.Printf("⚠️  InviteUser: failed to create user_tenant: user_id=%s tenant_id=%s err=%v", user.ID, tenantID, err)
			// 不回滾用戶創建，因為後續可以修復
		}

		isNewUser = true
	}

	// 記錄活動日誌
	changes := map[string]interface{}{
		"invited_user_id":    user.ID.String(),
		"invited_user_email": user.Email,
		"invited_user_name":  user.Name,
		"is_new_user":        isNewUser,
	}
	utils.LogActivity(tenantID, currentUserID, "invite", "user", &user.ID,
		fmt.Sprintf(`{"key":"user.invite","params":{"name":%q,"email":%q}}`, user.Name, user.Email),
		changes, c)

	// 返回用戶信息，前端可以繼續調用 send-invite 發送郵件
	user.PasswordHash = ""
	return c.Status(201).JSON(fiber.Map{
		"user":        user,
		"is_new_user": isNewUser,
		"message":     "User invited successfully. Please send invite email to complete the process.",
	})
}

// SendUserInviteEmail 發送用戶邀請郵件（用於設定密碼加入租戶）
func SendUserInviteEmail(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	// 查找用戶（可能不在當前租戶，但要確認有權限）
	var user models.User
	if err := database.DB.Where("id = ?", id).First(&user).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// 驗證用戶是否在此租戶（通過 user_tenants）
	var userTenant models.UserTenant
	if err := database.DB.Where("user_id = ? AND tenant_id = ?", user.ID, tenantID).First(&userTenant).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(403).JSON(fiber.Map{"error": "User is not a member of this tenant"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to verify user membership"})
	}

	if user.Email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "User email is required"})
	}

	// 獲取租戶信息
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// 生成 invite token
	token, tokenHash, err := newUserInviteToken()
	if err != nil {
		log.Printf("⚠️  SendUserInviteEmail: new token failed: user_id=%s err=%v", user.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate invite token"})
	}

	// 創建 invite token（使用 password_reset_tokens 表）
	inviteToken := models.PasswordResetToken{
		ID:        uuid.New(),
		TenantID:  tenant.ID,
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // 7天有效期
		CreatedAt: time.Now(),
	}

	if err := database.DB.Create(&inviteToken).Error; err != nil {
		log.Printf("⚠️  SendUserInviteEmail: save token failed: user_id=%s err=%v", user.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save invite token"})
	}

	inviteURL, err := email.UserInviteURL(tenant.Subdomain, token)
	if err != nil {
		log.Printf("⚠️  UserInviteURL failed: user_id=%s err=%v", user.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate invite URL"})
	}

	if err := email.EnqueueUserInviteEmail(tenant.ID, tenant.Subdomain, tenant.Name, user.ID, user.Email, user.Name, inviteURL); err != nil {
		log.Printf("⚠️  enqueue user invite email failed: user_id=%s err=%v", user.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to enqueue invite email"})
	}

	log.Printf("✅ User invite email sent: user_id=%s email=%s tenant=%s", user.ID, user.Email, tenant.Name)

	return c.JSON(fiber.Map{
		"message": "Invite email sent successfully",
	})
}

// newUserInviteToken 生成用戶邀請 token
func newUserInviteToken() (plainToken string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	plainToken = hex.EncodeToString(b)
	hashBytes := sha256.Sum256([]byte(plainToken))
	return plainToken, hashBytes[:], nil
}
