package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 企業 (Enterprise) CRUD
// ============================================

// GetCurrentEnterprise 獲取當前租戶的企業（一個租戶只有一個企業）
func GetCurrentEnterprise(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	var enterprise models.Enterprise
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&enterprise).Error; err != nil {
		// 如果不存在，嘗試從租戶自動創建
		var tenant models.Tenant
		if err := database.DB.First(&tenant, "id = ?", tenantID).Error; err != nil {
			// 返回空模板
			return c.JSON(fiber.Map{
				"id":           nil,
				"tenant_id":    tenantID,
				"name":         "",
				"code":         nil,
				"domain":       "",
				"status":       "active",
				"extra_fields": make(map[string]interface{}),
			})
		}

		// 自動創建
		enterprise = models.Enterprise{
			TenantID: tenantID,
			Name:     tenant.Name,
			Status:   "active",
			Timezone: "Asia/Hong_Kong",
		}
		if strings.TrimSpace(tenant.Subdomain) != "" {
			d := tenant.Subdomain
			enterprise.Domain = &d
		}
		if err := database.DB.Create(&enterprise).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 企業電話存放於 extra_fields.phone，但前端期望 top-level 的 phone 欄位
	if enterprise.ExtraFields != nil {
		if v, ok := enterprise.ExtraFields["phone"]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				ss := strings.TrimSpace(s)
				enterprise.Phone = &ss
			}
		}
		// 兼容：若有人用 phone_country_code + phone_number（或 phone）分開存
		if enterprise.Phone == nil {
			cc, _ := enterprise.ExtraFields["phone_country_code"].(string)
			num, _ := enterprise.ExtraFields["phone_number"].(string)
			cc = strings.TrimSpace(cc)
			num = strings.TrimSpace(num)
			if cc != "" && num != "" {
				combined := cc + " " + num
				enterprise.Phone = &combined
			} else if num != "" {
				enterprise.Phone = &num
			}
		}
		// 企業電郵存放於 extra_fields.email
		if v, ok := enterprise.ExtraFields["email"]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				ss := strings.TrimSpace(s)
				enterprise.Email = &ss
			}
		}
	}

	return c.JSON(enterprise)
}

// UpdateCurrentEnterprise 更新當前租戶的企業（如果不存在則創建）
func UpdateCurrentEnterprise(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID not found"})
	}

	// 以 raw body 為準抽取 phone（避免某些情況 BodyParser/指標欄位解析不到）
	var rawBody map[string]interface{}
	_ = json.Unmarshal(c.Body(), &rawBody)
	var rawPhoneProvided bool
	var rawPhonePtr *string
	var rawEmailProvided bool
	var rawEmailPtr *string
	if rawBody != nil {
		if v, ok := rawBody["phone"]; ok {
			rawPhoneProvided = true
			switch vv := v.(type) {
			case string:
				s := strings.TrimSpace(vv)
				rawPhonePtr = &s
			case nil:
				rawPhonePtr = nil
			default:
				// 非字串型別，忽略但視為「有提供」避免誤判
				rawPhonePtr = nil
			}
		}
		if v, ok := rawBody["email"]; ok {
			rawEmailProvided = true
			switch vv := v.(type) {
			case string:
				s := strings.TrimSpace(vv)
				rawEmailPtr = &s
			case nil:
				rawEmailPtr = nil
			default:
				rawEmailPtr = nil
			}
		}
	}

	var req struct {
		Name        string                 `json:"name"`
		Code        *string                `json:"code"`
		Phone       *string                `json:"phone"`
		Email       *string                `json:"email"`
		LogoURL     *string                `json:"logo_url"`
		Address     *string                `json:"address"`
		Timezone    string                 `json:"timezone"`
		Status      string                 `json:"status"`
		Subdomain   string                 `json:"subdomain"`
		ExtraFields map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := middleware.EnforceTrialAIDailyLimit(c, tenantID); err != nil {
		return err
	}

	// 若 raw body 有帶 phone，優先使用 raw 解析結果（避免 req.Phone = nil 造成不更新）
	if rawPhoneProvided {
		req.Phone = rawPhonePtr
	}
	if rawEmailProvided {
		req.Email = rawEmailPtr
	}

	// 查找現有企業
	var enterprise models.Enterprise
	err := database.DB.Where("tenant_id = ?", tenantID).First(&enterprise).Error

	if err != nil {
		// 不存在，創建新企業
		ef := req.ExtraFields
		if ef == nil {
			ef = make(map[string]interface{})
		}
		// phone 存放於 extra_fields.phone，避免改 schema
		if req.Phone != nil {
			p := strings.TrimSpace(*req.Phone)
			if p == "" {
				delete(ef, "phone")
			} else {
				ef["phone"] = p
			}
		}
		// email 存放於 extra_fields.email
		if req.Email != nil {
			e := strings.TrimSpace(*req.Email)
			if e == "" {
				delete(ef, "email")
			} else {
				ef["email"] = e
			}
		}

		enterprise = models.Enterprise{
			TenantID:    tenantID,
			Name:        req.Name,
			Code:        req.Code,
			Phone:       req.Phone, // 僅用於回傳；實際落庫在 ExtraFields
			Email:       req.Email, // 僅用於回傳；實際落庫在 ExtraFields
			LogoURL:     req.LogoURL,
			Address:     req.Address,
			Timezone:    req.Timezone,
			Status:      req.Status,
			ExtraFields: models.JSONB(ef),
		}
		if enterprise.Status == "" {
			enterprise.Status = "active"
		}
		if enterprise.Timezone == "" {
			enterprise.Timezone = "Asia/Hong_Kong"
		}
		// domain 不允許從前端修改，如無則嘗試從租戶補上
		if enterprise.Domain == nil {
			var tenant models.Tenant
			if err := database.DB.First(&tenant, "id = ?", tenantID).Error; err == nil && strings.TrimSpace(tenant.Subdomain) != "" {
				d := tenant.Subdomain
				enterprise.Domain = &d
			}
		}
		if err := database.DB.Create(&enterprise).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		// 同步更新 tenants.name
		if req.Name != "" {
			database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("name", req.Name)
		}
		return c.Status(201).JSON(enterprise)
	}

	// 更新現有企業
	enterprise.Name = req.Name
	if req.Code != nil {
		enterprise.Code = req.Code
	}
	if req.LogoURL != nil {
		enterprise.LogoURL = req.LogoURL
	}
	if req.Address != nil {
		enterprise.Address = req.Address
	}
	if req.Timezone != "" {
		enterprise.Timezone = req.Timezone
	}
	if req.Status != "" {
		enterprise.Status = req.Status
	}
	// 合併 extra_fields：避免前端只送部分欄位時覆蓋掉既有資料
	if req.ExtraFields != nil {
		if enterprise.ExtraFields == nil {
			enterprise.ExtraFields = make(models.JSONB)
		}
		for k, v := range req.ExtraFields {
			// 送 null 代表刪除
			if v == nil {
				delete(enterprise.ExtraFields, k)
				continue
			}
			enterprise.ExtraFields[k] = v
		}
	}
	// phone 存放於 extra_fields.phone（放在 ExtraFields 覆蓋之後，避免被覆蓋掉）
	if enterprise.ExtraFields == nil {
		enterprise.ExtraFields = make(models.JSONB)
	}
	if req.Phone != nil {
		p := strings.TrimSpace(*req.Phone)
		if p == "" {
			delete(enterprise.ExtraFields, "phone")
		} else {
			enterprise.ExtraFields["phone"] = p
		}
		enterprise.Phone = req.Phone
	}
	// email 存放於 extra_fields.email
	if req.Email != nil {
		e := strings.TrimSpace(*req.Email)
		if e == "" {
			delete(enterprise.ExtraFields, "email")
		} else {
			enterprise.ExtraFields["email"] = e
		}
		enterprise.Email = req.Email
	}

	// 如果提供了 subdomain，更新租戶的 subdomain
	if req.Subdomain != "" {
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			// 檢查 subdomain 是否已被其他租戶使用
			var existingTenant models.Tenant
			if err := database.DB.Where("subdomain = ? AND id != ?", req.Subdomain, tenantID).First(&existingTenant).Error; err == nil {
				return c.Status(400).JSON(fiber.Map{"error": "子域名已被使用"})
			}
			tenant.Subdomain = req.Subdomain
			if err := database.DB.Save(&tenant).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "更新子域名失敗: " + err.Error()})
			}
			// 同步更新企業的 domain
			if enterprise.Domain == nil || *enterprise.Domain != req.Subdomain {
				enterprise.Domain = &req.Subdomain
				database.DB.Save(&enterprise)
			}
		}
	}

	// 用 Updates 明確寫入（特別是 jsonb/map 欄位），避免 Save 在不同 DB/driver 下沒更新到 extra_fields
	updates := map[string]interface{}{
		"name":         enterprise.Name,
		"code":         enterprise.Code,
		"logo_url":     enterprise.LogoURL,
		"address":      enterprise.Address,
		"timezone":     enterprise.Timezone,
		"status":       enterprise.Status,
		"extra_fields": enterprise.ExtraFields,
	}
	if err := database.DB.Model(&enterprise).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 針對部分環境/driver：Updates(map) 對 jsonb 可能沒落庫，這裡再強制寫一次 extra_fields
	// （只有在本次請求有送 phone、email 或 extra_fields 時才執行，避免不必要的寫入）
	if rawPhoneProvided || rawEmailProvided || req.ExtraFields != nil {
		if err := database.DB.Model(&enterprise).Update("extra_fields", enterprise.ExtraFields).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 同步更新 tenants.name，確保 tenantSwitchBtn 和選擇企業 popup 顯示最新名稱
	if req.Name != "" {
		database.DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("name", req.Name)
	}

	// 重新從 DB 讀一次，確保回傳的是最新資料（以及確保 extra_fields 已 scan 回來）
	if err := database.DB.Where("id = ?", enterprise.ID).First(&enterprise).Error; err == nil {
		// 重新計算 top-level phone（前端用來拆區號顯示）
		if enterprise.ExtraFields != nil {
			if v, ok := enterprise.ExtraFields["phone"]; ok {
				if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
					ss := strings.TrimSpace(s)
					enterprise.Phone = &ss
				}
			}
			if enterprise.Phone == nil {
				cc, _ := enterprise.ExtraFields["phone_country_code"].(string)
				num, _ := enterprise.ExtraFields["phone_number"].(string)
				cc = strings.TrimSpace(cc)
				num = strings.TrimSpace(num)
				if cc != "" && num != "" {
					combined := cc + " " + num
					enterprise.Phone = &combined
				} else if num != "" {
					enterprise.Phone = &num
				}
			}
			// 重新計算 top-level email
			if v, ok := enterprise.ExtraFields["email"]; ok {
				if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
					ss := strings.TrimSpace(s)
					enterprise.Email = &ss
				}
			}
		}
	}

	return c.JSON(enterprise)
}

// ============================================
// 公司 (Company) CRUD
// ============================================

func GetCompanies(c *fiber.Ctx) error {
	enterpriseID := c.Query("enterprise_id")
	var companies []models.Company
	query := database.DB

	if enterpriseID != "" {
		query = query.Where("enterprise_id = ?", enterpriseID)
	}

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Company{}).Count(&total)

	if err := query.Preload("Enterprise").Offset(offset).Limit(limit).Find(&companies).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  companies,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetCompany(c *fiber.Ctx) error {
	id := c.Params("id")
	var company models.Company

	if err := database.DB.Preload("Enterprise").First(&company, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Company not found"})
	}

	return c.JSON(company)
}

func CreateCompany(c *fiber.Ctx) error {
	var company models.Company
	if err := c.BodyParser(&company); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Create(&company).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(company)
}

func UpdateCompany(c *fiber.Ctx) error {
	id := c.Params("id")
	var company models.Company

	if err := database.DB.First(&company, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Company not found"})
	}

	if err := c.BodyParser(&company); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Save(&company).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(company)
}

func DeleteCompany(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := database.DB.Delete(&models.Company{}, "id = ?", id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Company deleted"})
}

// ============================================
// 部門 (Department) CRUD
// ============================================

func GetDepartments(c *fiber.Ctx) error {
	companyID := c.Query("company_id")
	var departments []models.Department
	query := database.DB

	if companyID != "" {
		query = query.Where("company_id = ?", companyID)
	}

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Department{}).Count(&total)

	if err := query.Preload("Company").Preload("Parent").Offset(offset).Limit(limit).Find(&departments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  departments,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetDepartment(c *fiber.Ctx) error {
	id := c.Params("id")
	var department models.Department

	if err := database.DB.Preload("Company").Preload("Parent").First(&department, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Department not found"})
	}

	return c.JSON(department)
}

func CreateDepartment(c *fiber.Ctx) error {
	var department models.Department
	if err := c.BodyParser(&department); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Create(&department).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(department)
}

func UpdateDepartment(c *fiber.Ctx) error {
	id := c.Params("id")
	var department models.Department

	if err := database.DB.First(&department, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Department not found"})
	}

	if err := c.BodyParser(&department); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Save(&department).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(department)
}

func DeleteDepartment(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := database.DB.Delete(&models.Department{}, "id = ?", id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Department deleted"})
}

// ============================================
// 級別 (Level) CRUD
// ============================================

func GetLevels(c *fiber.Ctx) error {
	departmentID := c.Query("department_id")
	var levels []models.Level
	query := database.DB

	if departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Level{}).Count(&total)

	if err := query.Preload("Department").Offset(offset).Limit(limit).Order("level_order ASC").Find(&levels).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  levels,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetLevel(c *fiber.Ctx) error {
	id := c.Params("id")
	var level models.Level

	if err := database.DB.Preload("Department").First(&level, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Level not found"})
	}

	return c.JSON(level)
}

func CreateLevel(c *fiber.Ctx) error {
	var level models.Level
	if err := c.BodyParser(&level); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Create(&level).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(level)
}

func UpdateLevel(c *fiber.Ctx) error {
	id := c.Params("id")
	var level models.Level

	if err := database.DB.First(&level, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Level not found"})
	}

	if err := c.BodyParser(&level); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Save(&level).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(level)
}

func DeleteLevel(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := database.DB.Delete(&models.Level{}, "id = ?", id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Level deleted"})
}

// ============================================
// 地區 (Region) CRUD
// ============================================

func GetRegions(c *fiber.Ctx) error {
	parentID := c.Query("parent_id")
	var regions []models.Region
	query := database.DB

	if parentID != "" {
		query = query.Where("parent_id = ?", parentID)
	} else if c.Query("root_only") == "true" {
		query = query.Where("parent_id IS NULL")
	}

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Region{}).Count(&total)

	if err := query.Preload("Parent").Offset(offset).Limit(limit).Find(&regions).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  regions,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetRegion(c *fiber.Ctx) error {
	id := c.Params("id")
	var region models.Region

	if err := database.DB.Preload("Parent").First(&region, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Region not found"})
	}

	return c.JSON(region)
}

func CreateRegion(c *fiber.Ctx) error {
	var region models.Region
	if err := c.BodyParser(&region); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Create(&region).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(region)
}

func UpdateRegion(c *fiber.Ctx) error {
	id := c.Params("id")
	var region models.Region

	if err := database.DB.First(&region, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Region not found"})
	}

	if err := c.BodyParser(&region); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Save(&region).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(region)
}

func DeleteRegion(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := database.DB.Delete(&models.Region{}, "id = ?", id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Region deleted"})
}

// ============================================
// 貨幣 (Currency) CRUD
// ============================================

func GetCurrencies(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var currencies []models.Currency
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Currency{}).Count(&total)

	// 按 is_default DESC 排序，確保默認值在第一個，然後按 code ASC 排序
	if err := query.Order("is_default DESC, code ASC").Offset(offset).Limit(limit).Find(&currencies).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  currencies,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetCurrency(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)
	var currency models.Currency

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&currency).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Currency not found"})
	}

	return c.JSON(currency)
}

func CreateCurrency(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var currency models.Currency
	if err := c.BodyParser(&currency); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	currency.TenantID = tenantID

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置（同一租戶下）
	if currency.IsDefault {
		if err := tx.Model(&models.Currency{}).
			Where("tenant_id = ? AND is_default = ?", tenantID, true).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other currencies: " + err.Error()})
		}
	}

	if err := tx.Create(&currency).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	return c.Status(201).JSON(currency)
}

func UpdateCurrency(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)
	var currency models.Currency

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&currency).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Currency not found"})
	}

	var req struct {
		Code         string  `json:"code"`
		Name         string  `json:"name"`
		Symbol       *string `json:"symbol"`
		ExchangeRate float64 `json:"exchange_rate"`
		IsDefault    bool    `json:"is_default"`
		Status       string  `json:"status"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置（同一租戶下）
	if req.IsDefault {
		result := tx.Model(&models.Currency{}).
			Where("tenant_id = ? AND is_default = ? AND id != ?", tenantID, true, id).
			Update("is_default", false)
		if result.Error != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other currencies: " + result.Error.Error()})
		}
		// 檢查是否有記錄被更新
		if result.RowsAffected == 0 {
			// 沒有其他默認貨幣，這是正常的，繼續執行
		}
	}

	// 更新貨幣字段
	currency.Code = req.Code
	currency.Name = req.Name
	currency.Symbol = req.Symbol
	currency.ExchangeRate = req.ExchangeRate
	currency.IsDefault = req.IsDefault
	currency.Status = req.Status

	// 保存更新
	if err := tx.Save(&currency).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save currency: " + err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	// 重新查詢以確保返回最新的數據（包括 is_default）
	var updatedCurrency models.Currency
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&updatedCurrency).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload currency"})
	}

	return c.JSON(updatedCurrency)
}

func DeleteCurrency(c *fiber.Ctx) error {
	id := c.Params("id")
	tenantID := middleware.GetTenantID(c)

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.Currency{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Currency deleted"})
}

// UpdateCurrencyRates 自動更新所有貨幣的匯率
func UpdateCurrencyRates(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	// 獲取所有貨幣（按租戶過濾）
	var currencies []models.Currency
	if err := database.DB.Where("tenant_id = ?", tenantID).Find(&currencies).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch currencies"})
	}

	if len(currencies) == 0 {
		return c.JSON(fiber.Map{"message": "No currencies found", "updated": 0})
	}

	// 找到基礎貨幣（通常是 HKD，匯率為 1.0）
	var baseCurrency *models.Currency
	for i := range currencies {
		if currencies[i].ExchangeRate == 1.0 || currencies[i].Code == "HKD" {
			baseCurrency = &currencies[i]
			break
		}
	}
	if baseCurrency == nil {
		// 如果沒有找到，使用第一個貨幣作為基礎
		baseCurrency = &currencies[0]
	}

	// 使用免費的匯率 API (exchangerate-api.com 免費版本)
	// 注意：免費版本需要 API key，但我們可以使用公開的 API
	// 這裡使用 fixer.io 的免費替代方案或 exchangerate-api.com
	baseCode := baseCurrency.Code

	// 構建 API URL - 使用 exchangerate-api.com 的免費端點（不需要 API key，但有速率限制）
	apiURL := fmt.Sprintf("https://api.exchangerate-api.com/v4/latest/%s", baseCode)

	// 發送 HTTP 請求
	resp, err := http.Get(apiURL)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch exchange rates: " + err.Error()})
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return c.Status(500).JSON(fiber.Map{"error": "Exchange rate API returned error"})
	}

	var rateData struct {
		Rates map[string]float64 `json:"rates"`
		Base  string             `json:"base"`
		Date  string             `json:"date"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rateData); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse exchange rate data: " + err.Error()})
	}

	// 更新每個貨幣的匯率
	updatedCount := 0
	for i := range currencies {
		if currencies[i].Code == baseCode {
			// 基礎貨幣保持 1.0
			currencies[i].ExchangeRate = 1.0
		} else if rate, exists := rateData.Rates[currencies[i].Code]; exists {
			currencies[i].ExchangeRate = rate
		} else {
			// 如果 API 中沒有該貨幣，跳過
			continue
		}

		if err := database.DB.Save(&currencies[i]).Error; err != nil {
			log.Printf("Failed to update currency %s: %v", currencies[i].Code, err)
			continue
		}
		updatedCount++
	}

	return c.JSON(fiber.Map{
		"message": "Exchange rates updated successfully",
		"updated": updatedCount,
		"base":    baseCode,
		"date":    rateData.Date,
	})
}

// ============================================
// LLM 配置 API
// ============================================

// GetLLMConfig 獲取 LLM 配置（不包含敏感信息）
func GetLLMConfig(c *fiber.Ctx) error {
	cfg := config.Load()

	// 對於 Gemini，不需要 endpoint，只需要 API key
	enabled := false
	if cfg.LLM.Provider == "gemini" {
		enabled = cfg.LLM.APIKey != ""
	} else {
		enabled = cfg.LLM.Endpoint != ""
	}

	// 只返回 endpoint 和 model，不返回 API key
	return c.JSON(fiber.Map{
		"endpoint":    cfg.LLM.Endpoint,
		"model":       cfg.LLM.Model,
		"image_model": cfg.LLM.ImageModel,
		"provider":    cfg.LLM.Provider,
		"enabled":     enabled,
	})
}

// QueryIntent 查詢意圖結構
// Intent 可選值：
//
//	客戶相關: latest_customers, top_spending_customers, search_customer
//	訂單相關: largest_order, latest_orders, recent_orders
//	產品相關: latest_products, low_stock_products
//	預約相關: latest_appointments, upcoming_appointments
//	資源相關: available_rooms, available_equipments, available_vehicles
//	服務相關: latest_services, search_service
//	服務單相關: latest_service_orders
//	HR/員工相關: latest_users, latest_service_staffs, departments, staff_shifts
//	假期相關: holidays
//	項目相關: latest_projects
//	教學/幫助相關: help
//	統計數據: total_statistics, dashboard_stats
//	快捷動作: create_customer, create_order, create_service_order, create_product
type QueryIntent struct {
	NeedData bool                   `json:"need_data"`
	Intent   string                 `json:"intent,omitempty"`
	Limit    int                    `json:"limit,omitempty"`  // 最大限制為 50
	Query    string                 `json:"query,omitempty"`  // 用戶查詢關鍵詞（help/search 意圖用）
	Params   map[string]interface{} `json:"params,omitempty"` // 快捷動作參數（create 意圖用）
}

// getBaseURL 從請求中獲取基礎 URL
func getBaseURL(c *fiber.Ctx) string {
	scheme := "https"
	if c.Protocol() == "http" {
		scheme = "http"
	}
	host := c.Hostname()
	if host == "" {
		// 如果無法從請求獲取，嘗試從配置獲取
		cfg := config.Load()
		if cfg.Domain.BaseDomain != "" {
			host = cfg.Domain.BaseDomain
			if cfg.Domain.Scheme != "" {
				scheme = cfg.Domain.Scheme
			}
		} else {
			host = "localhost:3001"
			scheme = "http"
		}
	}
	return scheme + "://" + host
}

// getCMSBaseURL 回傳 vWork CMS 的 base URL（用於管理鏈接）。
// 與 getBaseURL 不同，此函數固定使用 config 中的 BaseDomain（而非請求來源域名），
// 確保從 vAi 等其他產品發出的請求也能產生正確的 vWork CMS 連結。
func getCMSBaseURL() string {
	cfg := config.Load()
	scheme := "https"
	if cfg.Domain.Scheme != "" {
		scheme = cfg.Domain.Scheme
	}
	host := strings.TrimSpace(cfg.Domain.BaseDomain)
	if host == "" {
		return "http://localhost:3001"
	}
	// 確保使用 www 子域名（例如 vworkai.com → www.vworkai.com）
	if !strings.HasPrefix(host, "www.") && !strings.Contains(host, "localhost") {
		host = "www." + host
	}
	return scheme + "://" + host
}

// isActionIntent 判斷是否為快捷動作意圖
func isActionIntent(intent string) bool {
	switch intent {
	case "create_customer", "create_order", "create_service_order", "create_product",
		"update_order_status", "create_appointment", "create_invoice",
		"create_supplier", "create_purchase_order", "update_customer", "update_product",
		"export_customers", "export_orders", "export_products",
		"quotation_to_order", "send_payment_link", "get_payment_link",
		"create_reminder", "create_note", "approve_leave",
		"create_shipment", "create_business_goal":
		return true
	}
	return false
}

// buildGeminiFunctionDeclarations returns all function declarations for Gemini Function Calling.
// Each business intent is exposed as a separate function the model can invoke.
func buildGeminiFunctionDeclarations() []map[string]interface{} {
	// Helper to build a simple string property
	strProp := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "STRING", "description": desc}
	}
	intProp := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "INTEGER", "description": desc}
	}
	numProp := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "NUMBER", "description": desc}
	}

	// Shared "limit" property for query intents
	limitProp := intProp("Number of results to return (max 50, default 5)")

	// --- Query intents (no params or just limit/query) ---
	decls := []map[string]interface{}{
		{
			"name":        "latest_customers",
			"description": "Get customer list. Use when the user wants to view, check, query, list, or browse customers without specifying a particular name or phone. Covers queries like '查詢客戶', '客戶列表', '我想看客戶', 'show customers'. Returns customers sorted by newest first.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "top_spending_customers",
			"description": "Get customers ranked by total spending amount",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "search_customer",
			"description": "Search for a specific customer by name, phone number, or email. Use ONLY when the user provides a specific name, phone, or email to search. Do NOT use this for general customer browsing — use latest_customers instead.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Customer name, phone number, or email to search for"),
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "largest_order",
			"description": "Get orders with the largest total amount",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "latest_orders",
			"description": "Get orders list. Use when the user wants to view, check, query, list, or browse orders without specifying a particular order number or customer name. Covers queries like '查詢訂單', '我想看訂單', '訂單列表', 'show me orders', 'check orders'. Returns orders sorted by newest first.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "recent_orders",
			"description": "Get recent orders (alias for latest_orders). Use this when the user asks about recent or latest orders.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "latest_products",
			"description": "Get product list. Use when the user wants to view, check, query, list, or browse products. Covers queries like '查詢產品', '商品列表', '我想看產品', 'show products'. Returns products sorted by newest first.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "low_stock_products",
			"description": "Get products with low stock (quantity < 10)",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "latest_appointments",
			"description": "Get appointment list. Use when the user wants to view, check, query, or browse appointments/bookings. Covers queries like '查詢預約', '預約列表', 'show appointments'. Returns appointments sorted by newest first.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "upcoming_appointments",
			"description": "Get upcoming (future) appointments that are not cancelled",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "available_rooms",
			"description": "Get all currently available rooms",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "available_equipments",
			"description": "Get all currently available equipment",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "available_vehicles",
			"description": "Get all currently available vehicles",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "latest_services",
			"description": "Get the service catalog — list of services offered (e.g. haircut, massage, consultation). Use when user asks about available services, service menu, or what services the business provides.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "search_service",
			"description": "Search services by name or code. Use when the user wants to find a specific service or check its price/duration.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Service name or code to search"),
					"limit": intProp("Max results to return (default 10)"),
				},
			},
		},
		{
			"name":        "latest_service_orders",
			"description": "Get the most recently created service orders",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "search_service_order",
			"description": "Search for service orders by service order number, customer name, or service name. Use when the user wants to find specific service orders related to a customer or service. Do NOT use this for general browsing — use latest_service_orders instead.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Service order number, customer name, or service name to search for"),
					"limit": intProp("Max results to return (default 10)"),
				},
			},
		},
		{
			"name":        "latest_users",
			"description": "Get the most recently created staff/employees",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "latest_service_staffs",
			"description": "Get the most recently created service staff members",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "departments",
			"description": "Get the list of departments in the organization",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "staff_shifts",
			"description": "Get staff shift/schedule information",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "holidays",
			"description": "Get upcoming holidays within the next 30 days",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "latest_projects",
			"description": "Get the most recently created projects",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "latest_transactions",
			"description": "Get the most recent financial transactions",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": limitProp,
				},
			},
		},
		{
			"name":        "income_summary",
			"description": "Get a summary of income/revenue",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "expense_summary",
			"description": "Get a summary of expenses",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "accounts_receivable",
			"description": "Get accounts receivable data",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "accounts_payable",
			"description": "Get accounts payable data",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "profit_loss",
			"description": "Get profit and loss statement data",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "balance_sheet",
			"description": "Get balance sheet data",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "cash_flow",
			"description": "Get cash flow statement data",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "total_statistics",
			"description": "Get overall business statistics (customers, products, orders, revenue)",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "dashboard_stats",
			"description": "Get dashboard statistics overview (alias for total_statistics)",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "help",
			"description": "Find help documentation / tutorials about how to use the system. Use when user asks how to do something.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Keyword describing what the user needs help with (e.g. 訂單, 客戶, POS)"),
				},
			},
		},
		// --- Action intents ---
		{
			"name":        "create_customer",
			"description": "Create a new customer record. Use when the user asks to add/create a customer.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name":               strProp("Customer first name (required)"),
					"last_name":          strProp("Customer last name"),
					"phone":              strProp("Phone number"),
					"phone_country_code": strProp("Phone country code (e.g. +852, +886)"),
					"email":              strProp("Email address"),
					"gender":             strProp("Gender (male/female/unknown)"),
					"address":            strProp("Address"),
				},
			},
		},
		{
			"name":        "create_order",
			"description": "Create a new sales order. Use when the user asks to create an order.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"customer_name":  strProp("Customer name to identify the customer"),
					"customer_phone": strProp("Customer phone to identify the customer"),
					"notes":          strProp("Order notes"),
					"items": map[string]interface{}{
						"type":        "ARRAY",
						"description": "List of order items",
						"items": map[string]interface{}{
							"type": "OBJECT",
							"properties": map[string]interface{}{
								"product_name": strProp("Product name"),
								"quantity":     numProp("Quantity (default 1)"),
								"unit_price":   numProp("Unit price (if not specified, uses product's default price)"),
							},
						},
					},
				},
			},
		},
		{
			"name":        "create_service_order",
			"description": "Create a new service order. Use when the user asks to create a service order.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"customer_name":  strProp("Customer name to identify the customer"),
					"customer_phone": strProp("Customer phone to identify the customer"),
					"service_date":   strProp("Service date in YYYY-MM-DD format"),
					"notes":          strProp("Service order notes"),
					"items": map[string]interface{}{
						"type":        "ARRAY",
						"description": "List of service items",
						"items": map[string]interface{}{
							"type": "OBJECT",
							"properties": map[string]interface{}{
								"service_name": strProp("Service name"),
								"quantity":     numProp("Quantity (default 1)"),
								"unit_price":   numProp("Unit price (if not specified, uses service's default price)"),
							},
						},
					},
				},
			},
		},
		{
			"name":        "create_product",
			"description": "Create a new product. Use when the user asks to add/create a product.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name":        strProp("Product name (required)"),
					"sku":         strProp("SKU code"),
					"barcode":     strProp("Barcode"),
					"price":       numProp("Selling price"),
					"cost":        numProp("Cost price"),
					"unit":        strProp("Unit of measurement (e.g. 個, 箱, kg)"),
					"category":    strProp("Product category"),
					"description": strProp("Product description"),
					"status":      strProp("Status: active or inactive (default active)"),
				},
			},
		},

		// ======================== Tier 1 — High frequency core ========================

		{
			"name":        "search_order",
			"description": "Search for a specific order by order number, customer name, product name, or keyword. Use ONLY when the user provides a specific order number, customer name, product name, or search keyword. Do NOT use this for general order browsing — use latest_orders instead.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Order number, customer name, product name, or keyword to search for"),
					"limit": intProp("Max results to return (default 10)"),
				},
			},
		},
		{
			"name":        "search_product",
			"description": "Search for a specific product by name, SKU, or barcode. Use ONLY when the user provides a specific product name, SKU, or barcode to search. Do NOT use this for general product browsing — use latest_products instead.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Product name, SKU, or barcode to search"),
					"limit": intProp("Max results to return (default 10)"),
				},
			},
		},
		{
			"name":        "update_order_status",
			"description": "Update an order's status. Valid transitions: draft→confirmed→processing→completed, or any→cancelled. Use when the user asks to change/update an order status.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"order_number", "status"},
				"properties": map[string]interface{}{
					"order_number": strProp("The order number to update"),
					"status":       strProp("New status: draft, confirmed, processing, completed, or cancelled"),
				},
			},
		},
		{
			"name":        "create_appointment",
			"description": "Create a new appointment/booking. Use when the user asks to book or schedule an appointment.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"customer_name", "start_time"},
				"properties": map[string]interface{}{
					"customer_name":  strProp("Customer name"),
					"customer_phone": strProp("Customer phone (optional, helps identify customer)"),
					"service_name":   strProp("Service name (optional)"),
					"staff_name":     strProp("Staff/technician name (optional)"),
					"start_time":     strProp("Appointment start time in YYYY-MM-DD HH:MM format"),
					"end_time":       strProp("Appointment end time in YYYY-MM-DD HH:MM format (optional)"),
					"duration":       intProp("Duration in minutes (default 60, used if end_time not given)"),
					"notes":          strProp("Appointment notes"),
				},
			},
		},
		{
			"name":        "create_invoice",
			"description": "Create a new invoice, optionally linked to an order. Use when the user asks to create/generate an invoice.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"order_number":  strProp("Link to an existing order by its order number (optional)"),
					"customer_name": strProp("Customer name (optional if order_number provided)"),
					"subtotal":      numProp("Invoice subtotal amount"),
					"tax_amount":    numProp("Tax amount (default 0)"),
					"due_date":      strProp("Due date in YYYY-MM-DD format (optional)"),
					"notes":         strProp("Invoice notes"),
				},
			},
		},
		{
			"name":        "latest_invoices",
			"description": "Get the most recent invoices. Use when the user asks about recent/latest invoices.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": intProp("Number of invoices to return (default 5, max 50)"),
				},
			},
		},
		{
			"name":        "search_invoice",
			"description": "Search invoices by invoice number or customer name. Use when the user wants to find a specific invoice.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Invoice number or customer name to search"),
					"limit": intProp("Max results to return (default 10)"),
				},
			},
		},
		{
			"name":        "get_order_invoices",
			"description": "Get invoices linked to a specific order. Use when the user asks about invoices for an order, or wants to see the invoice(s) of a particular order.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"order_number"},
				"properties": map[string]interface{}{
					"order_number": strProp("The order number to look up invoices for"),
				},
			},
		},
		{
			"name":        "global_search",
			"description": "Search across all entity types: customers, orders, products, projects, users, service orders, purchase orders. Use when the user wants a broad/global search.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Search keyword"),
					"limit": intProp("Max results to return (default 20)"),
				},
			},
		},

		// ======================== Tier 2 — Business management ========================

		{
			"name":        "create_supplier",
			"description": "Create a new supplier. Use when the user asks to add/create a supplier.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"name"},
				"properties": map[string]interface{}{
					"name":    strProp("Supplier name (required)"),
					"phone":   strProp("Phone number"),
					"email":   strProp("Email address"),
					"address": strProp("Address"),
				},
			},
		},
		{
			"name":        "search_supplier",
			"description": "Search suppliers by name, phone, or email. Use when the user wants to find a supplier.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("Supplier name, phone, or email to search"),
					"limit": intProp("Max results to return (default 10)"),
				},
			},
		},
		{
			"name":        "create_purchase_order",
			"description": "Create a new purchase order. Use when the user asks to create a purchase order.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"supplier_name"},
				"properties": map[string]interface{}{
					"supplier_name": strProp("Supplier name to identify the supplier"),
					"order_date":    strProp("Order date in YYYY-MM-DD format (default today)"),
					"notes":         strProp("Purchase order notes"),
					"items": map[string]interface{}{
						"type":        "ARRAY",
						"description": "List of purchase items",
						"items": map[string]interface{}{
							"type": "OBJECT",
							"properties": map[string]interface{}{
								"product_name": strProp("Product name"),
								"quantity":     numProp("Quantity"),
								"unit_price":   numProp("Unit price"),
							},
						},
					},
				},
			},
		},
		{
			"name":        "latest_purchase_orders",
			"description": "Get the most recent purchase orders. Use when the user asks about recent purchase orders.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": intProp("Number of purchase orders to return (default 5, max 50)"),
				},
			},
		},
		{
			"name":        "update_customer",
			"description": "Update an existing customer's information. Use when the user asks to update/change customer details.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"customer_name"},
				"properties": map[string]interface{}{
					"customer_name":  strProp("Customer name to identify the customer (required)"),
					"customer_phone": strProp("Customer phone to identify the customer (optional)"),
					"new_name":       strProp("New name"),
					"new_phone":      strProp("New phone number"),
					"new_email":      strProp("New email address"),
					"new_address":    strProp("New address"),
					"new_gender":     strProp("New gender: male, female, or unknown"),
					"new_status":     strProp("New status: active or inactive"),
				},
			},
		},
		{
			"name":        "update_product",
			"description": "Update an existing product's information. Use when the user asks to update/change product details.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"product_name"},
				"properties": map[string]interface{}{
					"product_name": strProp("Product name to identify the product (required)"),
					"product_sku":  strProp("Product SKU to identify the product (optional)"),
					"new_name":     strProp("New product name"),
					"new_price":    numProp("New selling price"),
					"new_cost":     numProp("New cost price"),
					"new_stock":    intProp("New stock quantity"),
					"new_sku":      strProp("New SKU"),
					"new_status":   strProp("New status: active or inactive"),
				},
			},
		},
		{
			"name":        "export_customers",
			"description": "Export customer list to Excel. Returns a download link. Use when the user asks to export/download customers.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"status": strProp("Filter by status: active or inactive (optional, default all)"),
				},
			},
		},
		{
			"name":        "export_orders",
			"description": "Export order list to Excel. Returns a download link. Use when the user asks to export/download orders.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"status": strProp("Filter by status: draft, confirmed, processing, completed, cancelled (optional, default all)"),
				},
			},
		},
		{
			"name":        "export_products",
			"description": "Export product list to Excel. Returns a download link. Use when the user asks to export/download products.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"status": strProp("Filter by status: active or inactive (optional, default all)"),
				},
			},
		},
		{
			"name":        "inventory_summary",
			"description": "Get inventory overview: total products, stock levels, low stock count, out of stock count. Use when the user asks about inventory/stock status.",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "quotation_to_order",
			"description": "Convert a quotation to a confirmed order. Use when the user asks to convert/confirm a quotation.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"order_number"},
				"properties": map[string]interface{}{
					"order_number": strProp("The quotation/order number to convert"),
				},
			},
		},
		{
			"name":        "send_payment_link",
			"description": "Generate a payment link for an order so the customer can pay online. Use when the user asks to send a payment link.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"order_number"},
				"properties": map[string]interface{}{
					"order_number":     strProp("The order number to generate payment link for"),
					"expires_in_hours": intProp("Link expiry in hours (default 72, 0 = no expiry)"),
				},
			},
		},
		{
			"name":        "get_payment_link",
			"description": "Get the existing payment link for an order. Use when the user asks to check, view, or retrieve the payment link for an order.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"order_number"},
				"properties": map[string]interface{}{
					"order_number": strProp("The order number to look up the payment link for"),
				},
			},
		},

		// ======================== Tier 3 — Advanced modules ========================

		{
			"name":        "create_reminder",
			"description": "Create a personal reminder. Use when the user asks to set/create a reminder.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"title"},
				"properties": map[string]interface{}{
					"title":       strProp("Reminder title (required)"),
					"description": strProp("Reminder description"),
					"remind_time": strProp("Reminder time in YYYY-MM-DD HH:MM format"),
				},
			},
		},
		{
			"name":        "upcoming_reminders",
			"description": "Get upcoming/pending reminders for the current user. Use when the user asks about their reminders.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": intProp("Number of reminders to return (default 10)"),
				},
			},
		},
		{
			"name":        "create_note",
			"description": "Create a personal note/memo. Use when the user asks to create/save a note.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"title"},
				"properties": map[string]interface{}{
					"title":    strProp("Note title (required)"),
					"content":  strProp("Note content"),
					"category": strProp("Note category (optional)"),
				},
			},
		},
		{
			"name":        "daily_report",
			"description": "Get today's business report: orders count, revenue, new customers, appointments. Use when the user asks about today's report/summary.",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "monthly_report",
			"description": "Get this month's business report: total orders, revenue, new customers, top products. Use when the user asks about this month's report/summary.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"month": strProp("Month in YYYY-MM format (default current month)"),
				},
			},
		},
		{
			"name":        "leave_requests",
			"description": "Get pending leave requests. Use when the user asks about leave requests.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"status": strProp("Filter by status: pending, approved, rejected (default pending)"),
					"limit":  intProp("Max results to return (default 10)"),
				},
			},
		},
		{
			"name":        "approve_leave",
			"description": "Approve a pending leave request. Use when the user asks to approve a leave request.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"employee_name"},
				"properties": map[string]interface{}{
					"employee_name": strProp("Employee name whose leave to approve"),
				},
			},
		},
		{
			"name":        "latest_shipments",
			"description": "Get the most recent shipments. Use when the user asks about recent shipments.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": intProp("Number of shipments to return (default 5, max 50)"),
				},
			},
		},
		{
			"name":        "create_shipment",
			"description": "Create a new shipment. Use when the user asks to create/arrange a shipment.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"order_number":      strProp("Link to an order by its order number (optional)"),
					"recipient_name":    strProp("Recipient name"),
					"recipient_phone":   strProp("Recipient phone"),
					"recipient_address": strProp("Recipient address"),
					"sender_name":       strProp("Sender name (optional)"),
					"sender_phone":      strProp("Sender phone (optional)"),
					"sender_address":    strProp("Sender address (optional)"),
					"tracking_number":   strProp("Tracking number (optional)"),
					"notes":             strProp("Shipment notes"),
				},
			},
		},
		{
			"name":        "dining_queue",
			"description": "Get current dining queue status: waiting customers, seated count. Use when the user asks about the dining queue.",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "pos_daily_summary",
			"description": "Get POS daily sales summary: total sales, transaction count, average order value. Use when the user asks about today's POS/sales summary.",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "customer_history",
			"description": "Get all orders and service records for a specific customer. Use when the user asks about a customer's history/records.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"customer_name"},
				"properties": map[string]interface{}{
					"customer_name":  strProp("Customer name to look up"),
					"customer_phone": strProp("Customer phone (optional, helps identify customer)"),
					"limit":          intProp("Max results per category (default 10)"),
				},
			},
		},
		{
			"name":        "product_sales_analysis",
			"description": "Get product sales analysis: best sellers, slow movers, revenue by product. Use when the user asks about product sales performance.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit": intProp("Number of top/bottom products to show (default 10)"),
				},
			},
		},
		// --- Business goal intents ---
		{
			"name":        "business_goals",
			"description": "Get business goals with progress tracking. Use when the user asks about their business goals, targets, objectives, KPIs, or performance targets. Shows goal title, target vs current value, progress percentage, deadline, and status.",
			"parameters": map[string]interface{}{
				"type": "OBJECT",
				"properties": map[string]interface{}{
					"limit":  intProp("Number of goals to return (default 10)"),
					"status": strProp("Filter by status: active, completed, failed, paused (default: all active goals)"),
				},
			},
		},
		{
			"name":        "analyze_business_goal",
			"description": "Analyze a specific business goal in depth: calculate real-time progress from actual business data (orders, revenue, customers), compare against target, estimate if the goal is on track, and provide AI suggestions. Use when the user asks to analyze, review, check progress, or get insights on a specific business goal.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": strProp("The business goal title or keyword to search for and analyze"),
				},
			},
		},
		{
			"name":        "create_business_goal",
			"description": "Create a new business goal/target/KPI. Use when the user wants to set a business target, create a goal, or define a KPI. Examples: '設定本月營收目標10萬', 'set a goal for 100 new customers this quarter', '建立訂單目標', 'I want to reach 50 orders by end of month'.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"title", "metric_type", "target_value", "start_date", "end_date"},
				"properties": map[string]interface{}{
					"title":        strProp("Goal title (e.g. '本月營收目標', 'Q1 Customer Acquisition')"),
					"description":  strProp("Detailed description of the goal"),
					"metric_type":  strProp("Type of metric to track. Must be one of: 'revenue' (total order revenue), 'order_count' (number of orders), 'customer_count' (number of new customers), 'product_sales_qty' (product sales quantity), 'service_order_count' (number of service orders), 'custom' (user-defined metric)"),
					"target_value": numProp("The numeric target value to achieve (e.g. 100000 for $100k revenue, 50 for 50 orders)"),
					"unit":         strProp("Unit of measurement (e.g. '元', '$', '筆', '位', 'units'). Infer from context."),
					"start_date":   strProp("Goal start date in YYYY-MM-DD format"),
					"end_date":     strProp("Goal end date/deadline in YYYY-MM-DD format"),
					"priority":     strProp("Priority level: low, medium, high (default: medium)"),
				},
			},
		},
		// --- Company / tenant info intent ---
		{
			"name":        "company_info",
			"description": "Get the current company/tenant information including company name, plan, website status, public site link, and custom domain. Use when the user asks about their company info, website link, public site URL, storefront URL, online store link, or company profile.",
			"parameters": map[string]interface{}{
				"type":       "OBJECT",
				"properties": map[string]interface{}{},
			},
		},
		// --- Navigation intent (frontend-handled) ---
		{
			"name":        "navigate_to_page",
			"description": "Navigate the browser to a specific vWork page. Use when the user asks to go to, open, or view a specific page (e.g. 'go to customers', 'open dashboard', 'show orders'). Valid pages: dashboard, customers, orders, service-orders, purchase-orders, products, projects, suppliers, invoices, accounting, pos, inventory, appointments, staff-shifts, messages, notifications, vai-chat, billing, personal-data, vcoins",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"page"},
				"properties": map[string]interface{}{
					"page": strProp("The page path to navigate to, e.g. 'customers', 'orders', 'dashboard', 'vai-chat'. Use the short name without leading slash."),
				},
			},
		},
		// --- Image generation intent ---
		{
			"name":        "generate_image",
			"description": "Generate an image using AI. Use when the user asks to create, draw, generate, design, or make an image, picture, illustration, photo, icon, logo, or artwork. Examples: '畫一張貓咪', 'generate a sunset photo', '幫我設計一個 logo', 'create an illustration of a dragon'.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"prompt"},
				"properties": map[string]interface{}{
					"prompt":       strProp("A detailed English description of the image to generate. Translate the user's request to English if needed and add detail for better results."),
					"aspect_ratio": strProp("The aspect ratio of the image. Options: '1:1' (square, default), '16:9' (landscape), '9:16' (portrait), '4:3', '3:4'. Infer from context or default to '1:1'."),
				},
			},
		},
		// --- Document generation intent ---
		{
			"name":        "generate_document",
			"description": "Generate a document file (Word, Excel, PowerPoint, or PDF) using AI. Use when the user asks to create, generate, produce, write, or make a document, report, spreadsheet, presentation, file, or PDF. Examples: '生成一份報告', '幫我做一份簡報', '建立一個Excel表格', 'generate a sales report', 'create a presentation about SEO', '生成關於SEO的pptx', '做一份PDF文件', '寫一份企劃書'.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"prompt", "doc_type"},
				"properties": map[string]interface{}{
					"prompt":   strProp("A clear description of the document content to generate. Keep the user's original language and intent. Include topic, structure, and any specific requirements mentioned."),
					"doc_type": strProp("The document format to generate. Must be one of: 'docx' (Word document — for reports, proposals, articles, letters), 'xlsx' (Excel spreadsheet — for data tables, lists, financial data), 'pptx' (PowerPoint presentation — for slides, presentations, pitch decks), 'pdf' (PDF document — for formal reports, printable documents). Infer from context: if user says '簡報/presentation/slides/ppt' use 'pptx'; if '表格/spreadsheet/excel' use 'xlsx'; if 'pdf' use 'pdf'; otherwise default to 'docx'."),
					"title":    strProp("A concise title for the document. Infer from the user's request. Use the same language as the user's prompt."),
					"theme":    strProp("The visual theme/color scheme for the document (mainly affects PPTX presentations). Choose the most appropriate theme based on the content topic and tone. Available themes: 'default' (Office 2013-2022, clean modern — general purpose), 'professional' (Office 2007-2010 classic — corporate, formal), 'blue' (cool tech — technology, science, IT), 'green' (nature — environment, sustainability, health), 'red' (energetic — marketing, passion, sales), 'violet' (creative — design, art, culture), 'orange' (warm — food, lifestyle, social), 'grayscale' (minimal — formal, monochrome, elegant), 'slipstream' (vibrant — startup, modern, dynamic), 'dark' (dark background — tech talks, night mode, dramatic). If user specifies a theme preference, use it. Otherwise infer from the topic."),
				},
			},
		},
		// --- Video generation intent ---
		{
			"name":        "generate_video",
			"description": "Generate a short video (up to 8 seconds) using AI. Use when the user asks to create, generate, produce, or make a video, clip, animation, or motion content. Examples: '生成一段影片', '幫我做一個影片', 'generate a video of a cat walking', '製作一個產品展示影片', '生成一段日落的影片', 'create a short video', '拍一段影片', '做一個動畫'.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"prompt"},
				"properties": map[string]interface{}{
					"prompt":       strProp("A detailed English description of the video to generate. Translate the user's request to English if needed and add cinematic details for better results (camera movement, lighting, atmosphere, etc.)."),
					"aspect_ratio": strProp("The aspect ratio of the video. Options: '16:9' (landscape, default), '9:16' (portrait/vertical). Only these two are supported. Infer from context or default to '16:9'."),
					"duration":     strProp("The duration of the video. Options: '4s', '6s', '8s'. Only these three are supported. Default to '8s' for maximum length unless user specifies shorter."),
				},
			},
		},
	}

	// Mark unused helpers to avoid compile errors
	_ = numProp
	_ = intProp
	_ = strProp

	return decls
}

// buildNavigationOnlyDeclarations returns a minimal set of function
// declarations containing only navigate_to_page and generate_image.
// This is used when no tenant is available so the model can still handle
// navigation and image generation requests without exposing data-query
// functions that would fail.
func buildNavigationOnlyDeclarations() []map[string]interface{} {
	strProp := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "STRING", "description": desc}
	}
	return []map[string]interface{}{
		{
			"name":        "navigate_to_page",
			"description": "Navigate the browser to a specific vWork page. Use when the user asks to go to, open, or view a specific page (e.g. 'go to customers', 'open dashboard', 'show orders'). Valid pages: dashboard, customers, orders, service-orders, purchase-orders, products, projects, suppliers, invoices, accounting, pos, inventory, appointments, staff-shifts, messages, notifications, vai-chat, billing, personal-data, vcoins",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"page"},
				"properties": map[string]interface{}{
					"page": strProp("The page path to navigate to, e.g. 'customers', 'orders', 'dashboard', 'vai-chat'. Use the short name without leading slash."),
				},
			},
		},
		{
			"name":        "generate_image",
			"description": "Generate an image using AI. Use when the user asks to create, draw, generate, design, or make an image, picture, illustration, photo, icon, logo, or artwork. Examples: '畫一張貓咪', 'generate a sunset photo', '幫我設計一個 logo', 'create an illustration of a dragon'.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"prompt"},
				"properties": map[string]interface{}{
					"prompt":       strProp("A detailed English description of the image to generate. Translate the user's request to English if needed and add detail for better results."),
					"aspect_ratio": strProp("The aspect ratio of the image. Options: '1:1' (square, default), '16:9' (landscape), '9:16' (portrait), '4:3', '3:4'. Infer from context or default to '1:1'."),
				},
			},
		},
		{
			"name":        "generate_document",
			"description": "Generate a document file (Word, Excel, PowerPoint, or PDF) using AI. Use when the user asks to create, generate, produce, write, or make a document, report, spreadsheet, presentation, file, or PDF. Examples: '生成一份報告', '幫我做一份簡報', '建立一個Excel表格', 'generate a sales report', 'create a presentation about SEO', '生成關於SEO的pptx', '做一份PDF文件', '寫一份企劃書'.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"prompt", "doc_type"},
				"properties": map[string]interface{}{
					"prompt":   strProp("A clear description of the document content to generate. Keep the user's original language and intent. Include topic, structure, and any specific requirements mentioned."),
					"doc_type": strProp("The document format to generate. Must be one of: 'docx' (Word document), 'xlsx' (Excel spreadsheet), 'pptx' (PowerPoint presentation), 'pdf' (PDF document). Infer from context: if user says '簡報/presentation/slides/ppt' use 'pptx'; if '表格/spreadsheet/excel' use 'xlsx'; if 'pdf' use 'pdf'; otherwise default to 'docx'."),
					"title":    strProp("A concise title for the document. Infer from the user's request. Use the same language as the user's prompt."),
					"theme":    strProp("The visual theme for the document (mainly affects PPTX). Available: 'default', 'professional', 'blue', 'green', 'red', 'violet', 'orange', 'grayscale', 'slipstream', 'dark'. Infer from the topic."),
				},
			},
		},
		{
			"name":        "generate_video",
			"description": "Generate a short video (up to 8 seconds) using AI. Use when the user asks to create, generate, produce, or make a video, clip, animation, or motion content. Examples: '生成一段影片', '幫我做一個影片', 'generate a video of a cat walking', '製作一個產品展示影片', '生成一段日落的影片', 'create a short video', '拍一段影片', '做一個動畫'.",
			"parameters": map[string]interface{}{
				"type":     "OBJECT",
				"required": []string{"prompt"},
				"properties": map[string]interface{}{
					"prompt":       strProp("A detailed English description of the video to generate. Translate the user's request to English if needed and add cinematic details for better results (camera movement, lighting, atmosphere, etc.)."),
					"aspect_ratio": strProp("The aspect ratio of the video. Options: '16:9' (landscape, default), '9:16' (portrait/vertical). Only these two are supported. Infer from context or default to '16:9'."),
					"duration":     strProp("The duration of the video. Options: '4s', '6s', '8s'. Only these three are supported. Default to '8s' for maximum length unless user specifies shorter."),
				},
			},
		},
	}
}

// executeFunctionCall executes a Gemini function call and returns the result string.
// It bridges the functionCall name+args to the existing executeAction / getDataByIntent logic.
func executeFunctionCall(c *fiber.Ctx, tenantID uuid.UUID, fnName string, fnArgs map[string]interface{}) string {
	// Handle navigate_to_page: store the target URL in Fiber locals so the
	// response handler can inject it into the JSON payload for the frontend.
	if fnName == "navigate_to_page" {
		page := getParamString(fnArgs, "page")
		if page != "" {
			// Normalize: strip leading slash if present
			page = strings.TrimPrefix(page, "/")
			c.Locals("vai_navigate_url", "/"+page)
		}
		// Return different function result based on page context:
		// - CMS: the frontend will perform SPA navigation, so tell the model to say "taking you there"
		// - vai-chat: the frontend will display a clickable link, so tell the model to mention the link
		pageCtx, _ := c.Locals("vai_page_context").(string)
		if pageCtx == "vai-chat" {
			return fmt.Sprintf(`Navigation requested to page: /%s. The user is on the standalone vAi chat page. A clickable button to the /%s page has already been automatically added below your message by the system. Simply tell the user you have prepared the link and they can click the button below. Do NOT include any URL, link text, or placeholder like [link] in your response — the button is already there.`, page, page)
		}
		return fmt.Sprintf(`Navigation requested to page: /%s. Tell the user you are taking them there now.`, page)
	}

	// Handle generate_image: call the Gemini image generation API and store
	// the resulting data URLs in Fiber locals for the response handler.
	if fnName == "generate_image" {
		prompt := getParamString(fnArgs, "prompt")
		aspect := getParamString(fnArgs, "aspect_ratio")
		if prompt == "" {
			return "Error: image prompt is required."
		}
		cfg := config.Load()
		model := cfg.LLM.ImageModel
		dataURLs, err := callGeminiImageGeneration(cfg.LLM.APIKey, model, prompt, aspect)
		if err != nil {
			return fmt.Sprintf("Image generation failed: %s", err.Error())
		}
		if len(dataURLs) == 0 {
			return "Image generation completed but no images were returned."
		}
		// Store image data URLs in Fiber locals so the response handler can
		// inject them into the JSON payload (similar to navigate_url pattern).
		c.Locals("vai_image_urls", dataURLs)

		// Also save each generated image to ai_sketch_generations with source="chat"
		// so they appear in vai-sketch generation history.
		userID := middleware.GetUserID(c)
		if userID != uuid.Nil {
			for _, dataURL := range dataURLs {
				go func(imgURL string) {
					if err := SaveSketchGeneration(tenantID, userID, nil, prompt, imgURL, model, "chat"); err != nil {
						log.Printf("[WARN] Failed to save chat image to sketch generations: %v", err)
					}
				}(dataURL)
			}
		}

		return fmt.Sprintf("Successfully generated %d image(s) for prompt: %s. The images are being displayed to the user. Describe what you generated briefly.", len(dataURLs), prompt)
	}

	// Handle generate_video: submit a Google Veo 3.1 video generation request (async task).
	// Returns the operation_id so the frontend can poll for completion.
	if fnName == "generate_video" {
		// Video generation costs 20 coins on top of the 1 coin already charged
		// by the chat middleware (total = 1 + 20 = 21 for vCoin users).
		// If Daily Free was used for the chat 1-coin, no real coin was deducted,
		// but the video fee is still 20 coins from vCoin.
		extraCoins := models.AICoinsVideoGen // 20
		usedFree, _ := c.Locals("used_daily_free").(bool)
		userID := middleware.GetUserID(c)
		ok, available, err := ConsumeAICoins(tenantID, userID, extraCoins, "AI 影片生成（交談觸發）", models.JSONB{"source": "chat_tool_call", "tool": "generate_video", "used_daily_free": usedFree})
		if err != nil || !ok {
			return fmt.Sprintf("Video generation requires %d vCoin but you only have %d available. Please purchase more vCoin. 影片生成需要 %d vCoin，目前餘額 %d，請購買更多 vCoin。",
				extraCoins, available, extraCoins, available)
		}

		prompt := getParamString(fnArgs, "prompt")
		if prompt == "" {
			return "Error: video prompt is required."
		}
		aspectRatio := getParamString(fnArgs, "aspect_ratio")
		if aspectRatio == "" {
			aspectRatio = "9:16"
		}
		// Validate Kling-supported ratios
		validRatios := map[string]bool{"16:9": true, "9:16": true, "1:1": true, "4:3": true, "3:4": true, "2.39:1": true, "21:9": true}
		if !validRatios[aspectRatio] {
			aspectRatio = "9:16"
		}
		duration := getParamString(fnArgs, "duration")
		if duration == "" {
			duration = "5"
		}

		cfg := config.Load()
		accessKey := cfg.Kling.AccessKey
		secretKey := cfg.Kling.SecretKey
		if accessKey == "" || secretKey == "" {
			return "Error: Kling API credentials not configured (KLING_ACCESS_KEY / KLING_SECRET_KEY)."
		}

		klingModel := cfg.Kling.Model
		if klingModel == "" {
			klingModel = "kling-v3-omni"
		}
		klingBaseURL := cfg.Kling.BaseURL
		if klingBaseURL == "" {
			klingBaseURL = "https://api-singapore.klingai.com"
		}

		// Validate duration (Kling: "5", "10", "15")
		durationSec := parseDurationSeconds(duration)
		if durationSec <= 5 {
			durationSec = 5
		} else if durationSec <= 10 {
			durationSec = 10
		} else {
			durationSec = 15
		}
		durationStr := strconv.Itoa(durationSec)

		// Truncate prompt to Kling 512 char limit
		promptRunes := []rune(prompt)
		if len(promptRunes) > 512 {
			prompt = string(promptRunes[:512])
		}

		// Build Kling Omni request (single-shot from chat tool call)
		klingReq := map[string]interface{}{
			"model_name": klingModel,
			"multi_shot": false,
			"shot_type":  "customize",
			"prompt":     "",
			"multi_prompt": []map[string]interface{}{
				{
					"index":    1,
					"prompt":   prompt,
					"duration": durationStr,
				},
			},
			"mode":         "std",
			"sound":        "on",
			"duration":     durationStr,
			"aspect_ratio": aspectRatio,
		}
		payload, err := json.Marshal(klingReq)
		if err != nil {
			return "Error: failed to build Kling request."
		}

		// Generate JWT
		jwtToken, err := generateKlingJWT(accessKey, secretKey)
		if err != nil {
			return "Error: failed to generate Kling auth token."
		}

		apiURL := klingBaseURL + "/v1/videos/omni-video"

		log.Printf("[VideoGen-ToolCall] Submitting Kling Omni: model=%s, prompt=%s, ratio=%s, duration=%s",
			klingModel, truncateStr(prompt, 60), aspectRatio, durationStr)

		httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payload))
		if err != nil {
			return "Error: failed to create Kling API request."
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+jwtToken)

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("[VideoGen-ToolCall] Kling API call failed: %v", err)
			return fmt.Sprintf("Video generation request failed: %s", err.Error())
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			log.Printf("[VideoGen-ToolCall] Kling API error %d: %s", resp.StatusCode, string(body))
			return fmt.Sprintf("Video generation API error: %s", string(body))
		}

		var klingResp klingCreateResponse
		if err := json.Unmarshal(body, &klingResp); err != nil {
			log.Printf("[VideoGen-ToolCall] Failed to parse Kling response: %v", err)
			return "Error: failed to parse Kling API response."
		}

		if klingResp.Code != 0 || klingResp.Data == nil || klingResp.Data.TaskID == "" {
			errMsg := klingResp.Message
			if errMsg == "" {
				errMsg = "no task_id returned"
			}
			return fmt.Sprintf("Error: Kling API error: %s", errMsg)
		}

		taskID := klingResp.Data.TaskID
		log.Printf("[VideoGen-ToolCall] Kling task submitted: task_id=%s", taskID)

		// Store video_info in Fiber locals for response injection
		videoInfo := map[string]interface{}{
			"task_id":      taskID,
			"status":       "generating",
			"prompt":       prompt,
			"aspect_ratio": aspectRatio,
			"duration":     duration,
		}
		c.Locals("vai_video_info", videoInfo)

		return fmt.Sprintf("Video generation has been submitted successfully. The video is being generated with prompt: %s (duration: %ss, aspect ratio: %s). Tell the user the video is being generated and they will see it when ready.", prompt, durationStr, aspectRatio)
	}

	// Handle generate_document: create DB record, spawn async goroutine for
	// heavy Gemini calls + file generation, return immediately with status.
	if fnName == "generate_document" {
		// Document generation costs 5 coins on top of the 1 coin already charged
		// by the chat middleware (total = 1 + 5 = 6 for vCoin users).
		// If Daily Free was used for the chat 1-coin, no real coin was deducted,
		// but the document fee is still 5 coins from vCoin.
		extraCoins := models.AICoinsDocGenBase // 5
		usedFree, _ := c.Locals("used_daily_free").(bool)
		userID := middleware.GetUserID(c)
		ok, available, err := ConsumeAICoins(tenantID, userID, extraCoins, "AI 文件生成（交談觸發）", models.JSONB{"source": "chat_tool_call", "tool": "generate_document", "used_daily_free": usedFree})
		if err != nil || !ok {
			return fmt.Sprintf("Document generation requires %d vCoin but you only have %d available. Please purchase more vCoin. 文件生成需要 %d vCoin，目前餘額 %d，請購買更多 vCoin。",
				extraCoins, available, extraCoins, available)
		}

		prompt := getParamString(fnArgs, "prompt")
		docType := strings.ToLower(getParamString(fnArgs, "doc_type"))
		title := getParamString(fnArgs, "title")
		theme := strings.ToLower(getParamString(fnArgs, "theme"))
		if prompt == "" {
			return "Error: document prompt is required."
		}
		if docType == "" {
			docType = "docx"
		}
		if docType != "docx" && docType != "xlsx" && docType != "pptx" && docType != "pdf" {
			docType = "docx"
		}
		if title == "" {
			title = prompt
			if len(title) > 50 {
				title = title[:50]
			}
		}

		cfg := config.Load()
		model := cfg.LLM.Model
		if model == "" {
			model = "gemini-2.0-flash"
		}

		// userID already obtained above (line 2178) for coin deduction

		// Parse conversation_id from the request body (sent by frontend)
		var convID *uuid.UUID
		if rawBody := c.Body(); len(rawBody) > 0 {
			var bodyMap map[string]interface{}
			if json.Unmarshal(rawBody, &bodyMap) == nil {
				if cidStr, ok := bodyMap["conversation_id"].(string); ok && cidStr != "" {
					if parsed, err := uuid.Parse(cidStr); err == nil {
						convID = &parsed
					}
				}
			}
		}

		// Create document record in DB with status "generating"
		doc := models.AiDocument{
			TenantID: tenantID,
			UserID:   userID,
			Title:    title,
			Prompt:   prompt,
			DocType:  docType,
			Model:    model,
			Status:   "generating",
			Source:   "chat",
		}
		if err := database.DB.Create(&doc).Error; err != nil {
			return fmt.Sprintf("Failed to create document record: %s", err.Error())
		}

		log.Printf("[DOC-GEN] Created document record id=%s, spawning async goroutine", doc.ID.String())

		// Spawn goroutine for the heavy work (Gemini multi-batch + file gen + DB update + message save)
		go func(docID uuid.UUID, tID uuid.UUID, uID uuid.UUID, conversationID *uuid.UUID,
			cfg *config.Config, prompt, docType, title, theme, model string) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[DOC-GEN-ASYNC] panic recovered for doc %s: %v", docID.String(), r)
					database.DB.Model(&models.AiDocument{}).Where("id = ?", docID).Updates(map[string]interface{}{
						"status":        "failed",
						"error_message": fmt.Sprintf("panic: %v", r),
					})
				}
			}()

			// Call Gemini to generate structured content
			content, err := callGeminiForDocument(cfg, prompt, docType, model)
			if err != nil {
				log.Printf("[DOC-GEN-ASYNC] Gemini call failed for doc %s: %s", docID.String(), err.Error())
				database.DB.Model(&models.AiDocument{}).Where("id = ?", docID).Updates(map[string]interface{}{
					"status":        "failed",
					"error_message": err.Error(),
				})
				// Save failure message to DB (double insurance)
				saveDocGenMessage(tID, uID, conversationID, docID, title, docType, 0, "failed",
					fmt.Sprintf("文件生成失敗：%s", err.Error()))
				return
			}

			// Generate the actual document file
			filePath, fileSize, err := generateDocumentFile(tID, docID, title, docType, theme, content)
			if err != nil {
				log.Printf("[DOC-GEN-ASYNC] File generation failed for doc %s: %s", docID.String(), err.Error())
				database.DB.Model(&models.AiDocument{}).Where("id = ?", docID).Updates(map[string]interface{}{
					"status":        "failed",
					"error_message": err.Error(),
				})
				saveDocGenMessage(tID, uID, conversationID, docID, title, docType, 0, "failed",
					fmt.Sprintf("文件建立失敗：%s", err.Error()))
				return
			}

			// Update document record to completed
			now := time.Now()
			fileURL := fmt.Sprintf("/api/v1/ai/documents/%s/download", docID.String())
			database.DB.Model(&models.AiDocument{}).Where("id = ?", docID).Updates(map[string]interface{}{
				"status":       "completed",
				"file_path":    filePath,
				"file_url":     fileURL,
				"file_size":    fileSize,
				"completed_at": now,
			})

			log.Printf("[DOC-GEN-ASYNC] Document completed: id=%s, type=%s, title=%s, size=%d", docID.String(), docType, title, fileSize)

			// Save AI message to DB (double insurance — persists even if frontend disconnected)
			typeLabels := map[string]string{"docx": "Word", "xlsx": "Excel", "pptx": "PowerPoint", "pdf": "PDF"}
			typeLabel := typeLabels[docType]
			if typeLabel == "" {
				typeLabel = docType
			}
			msgContent := fmt.Sprintf("已為您生成 %s 文件「%s」，請點擊下方卡片下載。", typeLabel, title)
			saveDocGenMessage(tID, uID, conversationID, docID, title, docType, fileSize, "completed", msgContent)

		}(doc.ID, tenantID, userID, convID, cfg, prompt, docType, title, theme, model)

		// Store doc info in Fiber locals for immediate response (status: generating)
		docInfo := map[string]interface{}{
			"id":       doc.ID.String(),
			"title":    doc.Title,
			"doc_type": doc.DocType,
			"status":   "generating",
		}
		c.Locals("vai_doc_info", docInfo)

		typeLabels := map[string]string{"docx": "Word", "xlsx": "Excel", "pptx": "PowerPoint", "pdf": "PDF"}
		typeLabel := typeLabels[docType]
		if typeLabel == "" {
			typeLabel = docType
		}
		return fmt.Sprintf("I am now generating a %s document titled '%s'. The document is being created in the background and will be ready shortly. Tell the user the document is being generated and they will see a download card when it's ready.", typeLabel, title)
	}

	// Build a QueryIntent from the function call args for action intents
	if isActionIntent(fnName) {
		intent := &QueryIntent{
			NeedData: true,
			Intent:   fnName,
			Params:   fnArgs,
		}
		return executeAction(c, tenantID, intent)
	}

	// For query intents, extract limit and query from args
	limit := 5
	if v, ok := fnArgs["limit"]; ok {
		switch n := v.(type) {
		case float64:
			limit = int(n)
		case int:
			limit = n
		}
	}

	query := ""
	if v, ok := fnArgs["query"]; ok {
		if s, ok := v.(string); ok {
			query = s
		}
	}
	// Fallback: some tools use specific param names (e.g. order_number) instead of "query"
	if query == "" {
		for _, key := range []string{"order_number", "keyword", "search"} {
			if v, ok := fnArgs[key]; ok {
				if s, ok := v.(string); ok && s != "" {
					query = s
					break
				}
			}
		}
	}

	return getDataByIntent(c, tenantID, fnName, limit, query)
}

// getParamString 從 params map 中安全取字串
// saveDocGenMessage saves an AI assistant message to the messages DB table
// after async document generation completes (or fails). This acts as "double
// insurance" so the message persists even if the frontend navigated away.
func saveDocGenMessage(tenantID, userID uuid.UUID, conversationID *uuid.UUID,
	docID uuid.UUID, title, docType string, fileSize int64, status, content string) {

	extraFields := map[string]interface{}{
		"ai_chat": true,
		"role":    "assistant",
	}

	docInfo := map[string]interface{}{
		"id":       docID.String(),
		"title":    title,
		"doc_type": docType,
		"status":   status,
	}
	if status == "completed" {
		docInfo["file_url"] = fmt.Sprintf("/api/v1/ai/documents/%s/download", docID.String())
		docInfo["file_size"] = fileSize
	}
	extraFields["doc_info"] = docInfo

	// Check if the frontend already saved a message for this doc (to avoid duplicates).
	// Search for an existing message with this doc_id in extra_fields.
	docIDStr := docID.String()
	var existingMsg models.Message
	searchQuery := database.DB.Where("tenant_id = ? AND message_type = ? AND extra_fields->>'role' = ?",
		tenantID, "ai_chat", "assistant")
	if conversationID != nil {
		searchQuery = searchQuery.Where("conversation_id = ?", *conversationID)
	}
	// Use JSONB path query to find message with matching doc_info.id
	searchQuery = searchQuery.Where("extra_fields->'doc_info'->>'id' = ?", docIDStr)

	if searchQuery.First(&existingMsg).Error == nil {
		// Message already exists (saved by frontend) — update it in place
		log.Printf("[DOC-GEN-ASYNC] Found existing message %s for doc %s, updating", existingMsg.ID.String(), docIDStr)
		database.DB.Model(&existingMsg).Updates(map[string]interface{}{
			"content":      content,
			"extra_fields": models.JSONB(extraFields),
		})
		return
	}

	// No existing message — create a new one (frontend disconnected before saving)
	msg := models.Message{
		TenantID:       tenantID,
		FromUserID:     &userID,
		ConversationID: conversationID,
		Subject:        "AI Chat",
		Content:        content,
		MessageType:    "ai_chat",
		ExtraFields:    models.JSONB(extraFields),
	}
	if err := database.DB.Create(&msg).Error; err != nil {
		log.Printf("[DOC-GEN-ASYNC] Failed to save AI message for doc %s: %s", docIDStr, err.Error())
	} else {
		log.Printf("[DOC-GEN-ASYNC] Created new AI message for doc %s (conversation=%v)", docIDStr, conversationID)
	}
}

// saveAIChatMessage saves an AI assistant message to the messages DB table
// as "double insurance" — if the frontend navigates away before saving,
// the message is still persisted by the backend.
// extraData may contain "doc_info", "image_urls", "navigate_url".
// It waits a short period then checks if the frontend already saved the
// message to avoid duplicates.
func saveAIChatMessage(tenantID, userID uuid.UUID, conversationID *uuid.UUID,
	content string, extraData map[string]interface{}) {
	if conversationID == nil {
		// No conversation_id means we can't save meaningfully
		return
	}
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return
	}

	// For doc_info messages, skip here — saveDocGenMessage handles those
	if _, hasDocInfo := extraData["doc_info"]; hasDocInfo {
		return
	}

	// Wait a few seconds to give the frontend time to save the message first.
	// This avoids duplicates in the normal case while still acting as a safety
	// net when the user navigates away before the frontend saves.
	time.Sleep(5 * time.Second)

	// Check if a matching assistant message already exists in this conversation
	// (created within the last 60 seconds to avoid false positives with older
	// messages that happen to have the same content).
	var existingCount int64
	database.DB.Model(&models.Message{}).
		Where("conversation_id = ? AND message_type = ? AND extra_fields->>'role' = ? AND content = ? AND created_at > ?",
			*conversationID, "ai_chat", "assistant", content, time.Now().Add(-60*time.Second)).
		Count(&existingCount)

	if existingCount > 0 {
		log.Printf("[AI-CHAT-SAVE] Skipped duplicate: conv=%s (frontend already saved)", conversationID.String())
		return
	}

	extraFields := map[string]interface{}{
		"ai_chat": true,
		"role":    "assistant",
	}
	// Merge any extra data (image_urls, navigate_url, video_info)
	for k, v := range extraData {
		extraFields[k] = v
	}

	msg := models.Message{
		TenantID:       tenantID,
		FromUserID:     &userID,
		ConversationID: conversationID,
		Subject:        "AI Chat",
		Content:        content,
		MessageType:    "ai_chat",
		ExtraFields:    models.JSONB(extraFields),
	}
	if err := database.DB.Create(&msg).Error; err != nil {
		log.Printf("[AI-CHAT-SAVE] Failed to save AI message: %s", err.Error())
	} else {
		log.Printf("[AI-CHAT-SAVE] Saved AI message id=%s conv=%s (frontend did not save)", msg.ID.String(), conversationID.String())
	}
}

// saveAIChatUserMessage saves a user message to the messages DB table as backend
// safety net. Mobile clients (vai-mobile) call /llm/chat directly without first
// calling POST /messages, so the user message would be lost without this.
// Same dedup logic as saveAIChatMessage: wait briefly, then check for duplicates.
func saveAIChatUserMessage(tenantID, userID uuid.UUID, conversationID *uuid.UUID, content string) {
	if conversationID == nil || tenantID == uuid.Nil || userID == uuid.Nil || content == "" {
		return
	}

	// Wait a short time to let the web frontend save first (avoid duplicates)
	time.Sleep(3 * time.Second)

	// Check if this user message already exists (frontend may have saved it)
	var existingCount int64
	database.DB.Model(&models.Message{}).
		Where("conversation_id = ? AND message_type = ? AND extra_fields->>'role' = ? AND content = ? AND created_at > ?",
			*conversationID, "ai_chat", "user", content, time.Now().Add(-60*time.Second)).
		Count(&existingCount)

	if existingCount > 0 {
		log.Printf("[AI-CHAT-SAVE] Skipped duplicate user msg: conv=%s (frontend already saved)", conversationID.String())
		return
	}

	msg := models.Message{
		TenantID:       tenantID,
		FromUserID:     &userID,
		ConversationID: conversationID,
		Subject:        "AI Chat",
		Content:        content,
		MessageType:    "ai_chat",
		ExtraFields:    models.JSONB(map[string]interface{}{"ai_chat": true, "role": "user"}),
	}
	if err := database.DB.Create(&msg).Error; err != nil {
		log.Printf("[AI-CHAT-SAVE] Failed to save user message: %s", err.Error())
	} else {
		log.Printf("[AI-CHAT-SAVE] Saved user message id=%s conv=%s", msg.ID.String(), conversationID.String())
	}
}

func getParamString(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// getParamFloat 從 params map 中安全取數字
func getParamFloat(params map[string]interface{}, key string) float64 {
	if params == nil {
		return 0
	}
	if v, ok := params[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case string:
			if f, err := strconv.ParseFloat(n, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

// getParamInt 從 params map 中安全取整數
func getParamInt(params map[string]interface{}, key string) int {
	if params == nil {
		return 0
	}
	if v, ok := params[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				return i
			}
		}
	}
	return 0
}

// getParamItems 從 params map 中安全取 items 陣列
func getParamItems(params map[string]interface{}, key string) []map[string]interface{} {
	if params == nil {
		return nil
	}
	if v, ok := params[key]; ok {
		if items, ok := v.([]interface{}); ok {
			result := make([]map[string]interface{}, 0, len(items))
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					result = append(result, m)
				}
			}
			return result
		}
	}
	return nil
}

// executeAction 執行快捷動作（建立客戶、訂單、服務單等）
func executeAction(c *fiber.Ctx, tenantID uuid.UUID, intent *QueryIntent) string {
	// 防止執行動作時 panic 導致系統崩潰
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[executeAction] panic recovered: %v", r)
		}
	}()

	if tenantID == uuid.Nil || intent == nil {
		return ""
	}

	baseURL := getCMSBaseURL()
	userID := middleware.GetUserID(c)
	params := intent.Params

	switch intent.Intent {
	case "create_customer":
		name := getParamString(params, "name")
		if name == "" {
			return "\n\n【快捷動作失敗】\n建立客戶需要提供名字（name），請告訴我客戶姓名。"
		}

		customer := models.Customer{
			TenantID: tenantID,
			Name:     name,
			Status:   "active",
		}
		if v := getParamString(params, "last_name"); v != "" {
			customer.LastName = v
		}
		if v := getParamString(params, "phone"); v != "" {
			customer.Phone = v
		}
		if v := getParamString(params, "phone_country_code"); v != "" {
			customer.PhoneCountryCode = v
		}
		if v := getParamString(params, "email"); v != "" {
			customer.Email = v
		}
		if v := getParamString(params, "gender"); v != "" {
			customer.Gender = v
		}
		if v := getParamString(params, "address"); v != "" {
			customer.Address = v
		}
		if userID != uuid.Nil {
			customer.CreatedBy = &userID
			customer.UpdatedBy = &userID
		}

		if err := database.DB.Create(&customer).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立客戶失敗: %s", err.Error())
		}

		var result strings.Builder
		result.WriteString("\n\n【快捷動作成功 - 客戶已建立】\n")
		result.WriteString(fmt.Sprintf("- 客戶名稱: %s\n", customer.Name))
		if customer.Phone != "" {
			result.WriteString(fmt.Sprintf("- 電話: %s\n", customer.Phone))
		}
		if customer.Email != "" {
			result.WriteString(fmt.Sprintf("- 電郵: %s\n", customer.Email))
		}
		result.WriteString(fmt.Sprintf("- 客戶 ID: %s\n", customer.ID.String()))
		result.WriteString(fmt.Sprintf("- 管理鏈接: %s/customers/%s/edit\n", baseURL, customer.ID.String()))
		return result.String()

	case "create_order":
		// 1. 解析客戶（通過 name 或 phone 查找）
		var customerID *uuid.UUID
		var customerName string
		custName := getParamString(params, "customer_name")
		custPhone := getParamString(params, "customer_phone")

		if custName != "" || custPhone != "" {
			var customer models.Customer
			db := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
			if custName != "" {
				db = db.Where("name ILIKE ?", "%"+custName+"%")
			}
			if custPhone != "" {
				db = db.Where("phone ILIKE ?", "%"+custPhone+"%")
			}
			if err := db.First(&customer).Error; err == nil {
				customerID = &customer.ID
				customerName = customer.Name
			}
		}

		// 2. 解析訂單項
		var orderItems []models.OrderItem
		if itemsRaw, ok := params["items"]; ok {
			if itemsArr, ok := itemsRaw.([]interface{}); ok {
				for _, itemRaw := range itemsArr {
					if itemMap, ok := itemRaw.(map[string]interface{}); ok {
						productName := getParamString(itemMap, "product_name")
						quantity := getParamFloat(itemMap, "quantity")
						unitPrice := getParamFloat(itemMap, "unit_price")

						if quantity <= 0 {
							quantity = 1
						}

						item := models.OrderItem{
							TenantID: tenantID,
							Quantity: quantity,
						}

						// 查找產品
						if productName != "" {
							var product models.Product
							if err := database.DB.Where("tenant_id = ? AND name ILIKE ? AND status = ?",
								tenantID, "%"+productName+"%", "active").
								First(&product).Error; err == nil {
								item.ProductID = &product.ID
								if unitPrice <= 0 {
									unitPrice = product.Price
								}
							}
						}

						item.UnitPrice = unitPrice
						item.TotalPrice = unitPrice * quantity
						orderItems = append(orderItems, item)
					}
				}
			}
		}

		// 3. 生成訂單編號
		now := time.Now()
		var maxOrderNum string
		prefix := "ORD-" + now.Format("20060102") + "-"
		database.DB.Model(&models.Order{}).
			Where("tenant_id = ? AND order_number LIKE ?", tenantID, prefix+"%").
			Select("MAX(order_number)").
			Scan(&maxOrderNum)

		seq := 1
		if maxOrderNum != "" {
			parts := strings.Split(maxOrderNum, "-")
			if len(parts) == 3 {
				if n, err := strconv.Atoi(parts[2]); err == nil {
					seq = n + 1
				}
			}
		}
		orderNumber := fmt.Sprintf("%s%04d", prefix, seq)

		// 4. 計算總金額
		var totalAmount float64
		for _, item := range orderItems {
			totalAmount += item.TotalPrice
		}

		// 5. 建立訂單
		order := models.Order{
			TenantID:    tenantID,
			OrderNumber: orderNumber,
			CustomerID:  customerID,
			OrderDate:   now,
			Status:      "confirmed",
			TotalAmount: totalAmount,
			SourceType:  "erp",
		}

		if v := getParamString(params, "notes"); v != "" {
			order.Notes = v
		}

		if userID != uuid.Nil {
			order.SalespersonID = &userID
			order.CreatedBy = &userID
			order.UpdatedBy = &userID
		}

		if err := database.DB.Create(&order).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立訂單失敗: %s", err.Error())
		}

		// 6. 建立訂單項
		for i := range orderItems {
			orderItems[i].OrderID = order.ID
			database.DB.Create(&orderItems[i])
		}

		// 7. 格式化結果
		var result strings.Builder
		result.WriteString("\n\n【快捷動作成功 - 訂單已建立】\n")
		result.WriteString(fmt.Sprintf("- 訂單編號: %s\n", orderNumber))
		if customerName != "" {
			result.WriteString(fmt.Sprintf("- 客戶: %s\n", customerName))
		}
		result.WriteString(fmt.Sprintf("- 訂單日期: %s\n", now.Format("2006-01-02")))
		result.WriteString(fmt.Sprintf("- 狀態: confirmed\n"))
		if len(orderItems) > 0 {
			result.WriteString("- 訂單項目:\n")
			for i, item := range orderItems {
				itemDesc := fmt.Sprintf("  %d. 數量: %.0f, 單價: %.2f, 小計: %.2f", i+1, item.Quantity, item.UnitPrice, item.TotalPrice)
				if item.ProductID != nil {
					var p models.Product
					if err := database.DB.Where("id = ?", item.ProductID).First(&p).Error; err == nil {
						itemDesc = fmt.Sprintf("  %d. %s - 數量: %.0f, 單價: %.2f, 小計: %.2f", i+1, p.Name, item.Quantity, item.UnitPrice, item.TotalPrice)
					}
				}
				result.WriteString(itemDesc + "\n")
			}
		}
		result.WriteString(fmt.Sprintf("- 總金額: %.2f\n", totalAmount))
		result.WriteString(fmt.Sprintf("- 管理鏈接: %s/orders/%s/edit\n", baseURL, order.ID.String()))
		return result.String()

	case "create_service_order":
		// 1. 解析客戶
		var customerID *uuid.UUID
		var customerName string
		custName := getParamString(params, "customer_name")
		custPhone := getParamString(params, "customer_phone")

		if custName != "" || custPhone != "" {
			var customer models.Customer
			db := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
			if custName != "" {
				db = db.Where("name ILIKE ?", "%"+custName+"%")
			}
			if custPhone != "" {
				db = db.Where("phone ILIKE ?", "%"+custPhone+"%")
			}
			if err := db.First(&customer).Error; err == nil {
				customerID = &customer.ID
				customerName = customer.Name
			}
		}

		// 2. 解析服務單項
		var serviceOrderItems []models.ServiceOrderItem
		if itemsRaw, ok := params["items"]; ok {
			if itemsArr, ok := itemsRaw.([]interface{}); ok {
				for _, itemRaw := range itemsArr {
					if itemMap, ok := itemRaw.(map[string]interface{}); ok {
						serviceName := getParamString(itemMap, "service_name")
						quantity := getParamFloat(itemMap, "quantity")
						unitPrice := getParamFloat(itemMap, "unit_price")

						if quantity <= 0 {
							quantity = 1
						}

						item := models.ServiceOrderItem{
							Quantity: quantity,
						}

						// 查找服務
						if serviceName != "" {
							var service models.Service
							if err := database.DB.Where("tenant_id = ? AND name ILIKE ? AND status = ?",
								tenantID, "%"+serviceName+"%", "active").
								First(&service).Error; err == nil {
								item.ServiceID = &service.ID
								if unitPrice <= 0 {
									unitPrice = service.Price
								}
							}
						}

						item.UnitPrice = unitPrice
						item.TotalPrice = unitPrice * quantity
						serviceOrderItems = append(serviceOrderItems, item)
					}
				}
			}
		}

		// 3. 生成服務單編號
		now := time.Now()
		var maxOrderNum string
		prefix := "SVC-" + now.Format("20060102") + "-"
		database.DB.Model(&models.ServiceOrder{}).
			Where("tenant_id = ? AND order_number LIKE ?", tenantID, prefix+"%").
			Select("MAX(order_number)").
			Scan(&maxOrderNum)

		seq := 1
		if maxOrderNum != "" {
			parts := strings.Split(maxOrderNum, "-")
			if len(parts) == 3 {
				if n, err := strconv.Atoi(parts[2]); err == nil {
					seq = n + 1
				}
			}
		}
		orderNumber := fmt.Sprintf("%s%04d", prefix, seq)

		// 4. 解析服務日期
		serviceDate := now
		if v := getParamString(params, "service_date"); v != "" {
			if parsed, err := time.Parse("2006-01-02", v); err == nil {
				serviceDate = parsed
			}
		}

		// 5. 計算總金額
		var totalAmount float64
		for _, item := range serviceOrderItems {
			totalAmount += item.TotalPrice
		}

		// 6. 建立服務單
		serviceOrder := models.ServiceOrder{
			TenantID:    tenantID,
			OrderNumber: orderNumber,
			CustomerID:  customerID,
			ServiceDate: serviceDate,
			Status:      "confirmed",
			TotalAmount: totalAmount,
		}

		if v := getParamString(params, "notes"); v != "" {
			serviceOrder.Notes = v
		}

		if userID != uuid.Nil {
			serviceOrder.SalespersonID = &userID
			serviceOrder.CreatedBy = &userID
			serviceOrder.UpdatedBy = &userID
		}

		if err := database.DB.Create(&serviceOrder).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立服務單失敗: %s", err.Error())
		}

		// 7. 建立服務單項
		for i := range serviceOrderItems {
			serviceOrderItems[i].ServiceOrderID = serviceOrder.ID
			database.DB.Create(&serviceOrderItems[i])
		}

		// 8. 格式化結果
		var result strings.Builder
		result.WriteString("\n\n【快捷動作成功 - 服務單已建立】\n")
		result.WriteString(fmt.Sprintf("- 服務單編號: %s\n", orderNumber))
		if customerName != "" {
			result.WriteString(fmt.Sprintf("- 客戶: %s\n", customerName))
		}
		result.WriteString(fmt.Sprintf("- 服務日期: %s\n", serviceDate.Format("2006-01-02")))
		result.WriteString(fmt.Sprintf("- 狀態: confirmed\n"))
		if len(serviceOrderItems) > 0 {
			result.WriteString("- 服務項目:\n")
			for i, item := range serviceOrderItems {
				itemDesc := fmt.Sprintf("  %d. 數量: %.0f, 單價: %.2f, 小計: %.2f", i+1, item.Quantity, item.UnitPrice, item.TotalPrice)
				if item.ServiceID != nil {
					var s models.Service
					if err := database.DB.Where("id = ?", item.ServiceID).First(&s).Error; err == nil {
						itemDesc = fmt.Sprintf("  %d. %s - 數量: %.0f, 單價: %.2f, 小計: %.2f", i+1, s.Name, item.Quantity, item.UnitPrice, item.TotalPrice)
					}
				}
				result.WriteString(itemDesc + "\n")
			}
		}
		result.WriteString(fmt.Sprintf("- 總金額: %.2f\n", totalAmount))
		result.WriteString(fmt.Sprintf("- 管理鏈接: %s/service-orders/%s/edit\n", baseURL, serviceOrder.ID.String()))
		return result.String()

	case "create_product":
		name := getParamString(params, "name")
		if name == "" {
			return "\n\n【快捷動作失敗】\n建立商品需要提供名稱（name），請告訴我商品名稱。"
		}

		product := models.Product{
			TenantID: tenantID,
			Name:     name,
			Status:   "active",
		}
		if v := getParamString(params, "sku"); v != "" {
			product.SKU = v
		}
		if v := getParamString(params, "barcode"); v != "" {
			product.Barcode = v
		}
		if v := getParamString(params, "category"); v != "" {
			product.Category = v
		}
		if v := getParamString(params, "description"); v != "" {
			product.Description = v
		}
		if v := getParamString(params, "unit"); v != "" {
			product.Unit = v
		}
		if v := getParamString(params, "status"); v != "" {
			product.Status = v
		}
		if v := getParamFloat(params, "price"); v > 0 {
			product.Price = v
		}
		if v := getParamFloat(params, "cost"); v > 0 {
			product.Cost = v
		}
		if userID != uuid.Nil {
			product.CreatedBy = &userID
			product.UpdatedBy = &userID
		}

		// Check if VMarket is joined, auto-enable show_on_vmarket
		extraFields := map[string]interface{}{}
		if joined, err := getTenantVMarketJoined(tenantID); err == nil && joined {
			extraFields["show_on_vmarket"] = true
		}
		if len(extraFields) > 0 {
			product.ExtraFields = models.JSONB(extraFields)
		}

		if err := database.DB.Create(&product).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立商品失敗: %s", err.Error())
		}

		var result strings.Builder
		result.WriteString("\n\n【快捷動作成功 - 商品已建立】\n")
		result.WriteString(fmt.Sprintf("- 商品名稱: %s\n", product.Name))
		if product.SKU != "" {
			result.WriteString(fmt.Sprintf("- SKU: %s\n", product.SKU))
		}
		if product.Barcode != "" {
			result.WriteString(fmt.Sprintf("- 條碼: %s\n", product.Barcode))
		}
		if product.Price > 0 {
			result.WriteString(fmt.Sprintf("- 價格: %.2f\n", product.Price))
		}
		if product.Cost > 0 {
			result.WriteString(fmt.Sprintf("- 成本: %.2f\n", product.Cost))
		}
		if product.Unit != "" {
			result.WriteString(fmt.Sprintf("- 單位: %s\n", product.Unit))
		}
		if product.Category != "" {
			result.WriteString(fmt.Sprintf("- 分類: %s\n", product.Category))
		}
		result.WriteString(fmt.Sprintf("- 狀態: %s\n", product.Status))
		result.WriteString(fmt.Sprintf("- 商品 ID: %s\n", product.ID.String()))
		result.WriteString(fmt.Sprintf("- 管理鏈接: %s/products/%s/edit\n", baseURL, product.ID.String()))
		return result.String()

	// ======================== Tier 1 — update_order_status ========================
	case "update_order_status":
		orderNumber := getParamString(params, "order_number")
		newStatus := getParamString(params, "status")
		if orderNumber == "" || newStatus == "" {
			return "\n\n【快捷動作失敗】\n需要提供訂單編號（order_number）和新狀態（status）。"
		}
		validStatuses := map[string]bool{"draft": true, "confirmed": true, "processing": true, "completed": true, "cancelled": true}
		if !validStatuses[newStatus] {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n無效的狀態「%s」。有效狀態：draft, confirmed, processing, completed, cancelled。", newStatus)
		}
		var order models.Order
		if err := database.DB.Where("tenant_id = ? AND order_number = ? AND trashed_at IS NULL", tenantID, orderNumber).First(&order).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到訂單「%s」。", orderNumber)
		}
		oldStatus := order.Status
		now := utils.NowInTenantTimezone(tenantID)
		if err := database.DB.Model(&order).Updates(map[string]interface{}{
			"status":     newStatus,
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n更新訂單狀態失敗: %s", err.Error())
		}
		var result strings.Builder
		result.WriteString("\n\n【快捷動作成功 - 訂單狀態已更新】\n")
		result.WriteString(fmt.Sprintf("- 訂單編號: %s\n", orderNumber))
		result.WriteString(fmt.Sprintf("- 狀態變更: %s → %s\n", oldStatus, newStatus))
		result.WriteString(fmt.Sprintf("- 管理鏈接: %s/orders/%s/edit\n", baseURL, order.ID.String()))
		return result.String()

	// ======================== Tier 1 — create_appointment ========================
	case "create_appointment":
		customerName := getParamString(params, "customer_name")
		startTimeStr := getParamString(params, "start_time")
		if customerName == "" || startTimeStr == "" {
			return "\n\n【快捷動作失敗】\n建立預約需要提供客戶名稱（customer_name）和開始時間（start_time）。"
		}
		// Find customer
		customerQuery := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
		if phone := getParamString(params, "customer_phone"); phone != "" {
			customerQuery = customerQuery.Where("phone ILIKE ?", "%"+phone+"%")
		} else {
			customerQuery = customerQuery.Where("name ILIKE ?", "%"+customerName+"%")
		}
		var customer models.Customer
		if err := customerQuery.First(&customer).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到客戶「%s」，請先建立客戶。", customerName)
		}
		// Parse start time
		loc := utils.GetTenantLocation(tenantID)
		startTime, err := time.ParseInLocation("2006-01-02 15:04", startTimeStr, loc)
		if err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n無法解析開始時間「%s」，請使用 YYYY-MM-DD HH:MM 格式。", startTimeStr)
		}
		// Calculate end time
		duration := getParamInt(params, "duration")
		if duration <= 0 {
			duration = 60
		}
		var endTime time.Time
		if endTimeStr := getParamString(params, "end_time"); endTimeStr != "" {
			if et, err := time.ParseInLocation("2006-01-02 15:04", endTimeStr, loc); err == nil {
				endTime = et
			} else {
				endTime = startTime.Add(time.Duration(duration) * time.Minute)
			}
		} else {
			endTime = startTime.Add(time.Duration(duration) * time.Minute)
		}
		appointment := models.Appointment{
			TenantID:        tenantID,
			CustomerID:      customer.ID,
			StartTime:       startTime,
			EndTime:         endTime,
			AppointmentDate: startTime,
			Status:          "pending",
			Notes:           getParamString(params, "notes"),
		}
		// Optionally find service
		if serviceName := getParamString(params, "service_name"); serviceName != "" {
			var service models.Service
			if err := database.DB.Where("tenant_id = ? AND name ILIKE ?", tenantID, "%"+serviceName+"%").First(&service).Error; err == nil {
				appointment.ServiceID = &service.ID
			}
		}
		// Optionally find staff
		if staffName := getParamString(params, "staff_name"); staffName != "" {
			var staff models.User
			if err := database.DB.Where("tenant_id = ? AND name ILIKE ?", tenantID, "%"+staffName+"%").First(&staff).Error; err == nil {
				appointment.StaffID = &staff.ID
			}
		}
		if err := database.DB.Create(&appointment).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立預約失敗: %s", err.Error())
		}
		var apptResult strings.Builder
		apptResult.WriteString("\n\n【快捷動作成功 - 預約已建立】\n")
		apptResult.WriteString(fmt.Sprintf("- 客戶: %s\n", customer.Name))
		apptResult.WriteString(fmt.Sprintf("- 開始時間: %s\n", startTime.Format("2006-01-02 15:04")))
		apptResult.WriteString(fmt.Sprintf("- 結束時間: %s\n", endTime.Format("2006-01-02 15:04")))
		apptResult.WriteString(fmt.Sprintf("- 狀態: %s\n", appointment.Status))
		apptResult.WriteString(fmt.Sprintf("- 預約 ID: %s\n", appointment.ID.String()))
		apptResult.WriteString(fmt.Sprintf("- 管理鏈接: %s/appointments/%s/edit\n", baseURL, appointment.ID.String()))
		return apptResult.String()

	// ======================== Tier 1 — create_invoice ========================
	case "create_invoice":
		invoiceNumber := "INV-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
		// Check dup
		var existingInv models.Invoice
		if err := database.DB.Where("tenant_id = ? AND invoice_number = ?", tenantID, invoiceNumber).First(&existingInv).Error; err == nil {
			invoiceNumber = "INV-" + time.Now().Format("20060102") + "-" + uuid.New().String()[:8]
		}
		now := utils.NowInTenantTimezone(tenantID)
		invoice := models.Invoice{
			TenantID:      tenantID,
			InvoiceNumber: invoiceNumber,
			InvoiceDate:   now,
			Status:        "draft",
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if userID != uuid.Nil {
			invoice.CreatedBy = &userID
			invoice.UpdatedBy = &userID
		}
		// Link to order if provided
		if orderNum := getParamString(params, "order_number"); orderNum != "" {
			var order models.Order
			if err := database.DB.Where("tenant_id = ? AND order_number = ? AND trashed_at IS NULL", tenantID, orderNum).First(&order).Error; err == nil {
				invoice.OrderID = &order.ID
				invoice.CustomerID = order.CustomerID
				invoice.Subtotal = order.TotalAmount
				invoice.TotalAmount = order.TotalAmount
			}
		}
		// Override with customer name if no order
		if invoice.CustomerID == nil {
			if custName := getParamString(params, "customer_name"); custName != "" {
				var cust models.Customer
				if err := database.DB.Where("tenant_id = ? AND name ILIKE ? AND trashed_at IS NULL", tenantID, "%"+custName+"%").First(&cust).Error; err == nil {
					invoice.CustomerID = &cust.ID
				}
			}
		}
		if v := getParamFloat(params, "subtotal"); v > 0 {
			invoice.Subtotal = v
			invoice.TotalAmount = v + getParamFloat(params, "tax_amount")
		}
		if v := getParamFloat(params, "tax_amount"); v > 0 {
			invoice.TaxAmount = v
			invoice.TotalAmount = invoice.Subtotal + v
		}
		if invoice.TotalAmount == 0 && invoice.Subtotal > 0 {
			invoice.TotalAmount = invoice.Subtotal
		}
		if dueDateStr := getParamString(params, "due_date"); dueDateStr != "" {
			if d, err := utils.ParseDateInTenantTimezone(tenantID, dueDateStr); err == nil {
				invoice.DueDate = &d
			}
		}
		invoice.Notes = getParamString(params, "notes")
		if err := database.DB.Create(&invoice).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立發票失敗: %s", err.Error())
		}
		var invResult strings.Builder
		invResult.WriteString("\n\n【快捷動作成功 - 發票已建立】\n")
		invResult.WriteString(fmt.Sprintf("- 發票編號: %s\n", invoice.InvoiceNumber))
		invResult.WriteString(fmt.Sprintf("- 金額: %.2f\n", invoice.TotalAmount))
		invResult.WriteString(fmt.Sprintf("- 狀態: %s\n", invoice.Status))
		invResult.WriteString(fmt.Sprintf("- 發票 ID: %s\n", invoice.ID.String()))
		invResult.WriteString(fmt.Sprintf("- 管理鏈接: %s/invoices/%s/edit\n", baseURL, invoice.ID.String()))
		return invResult.String()

	// ======================== Tier 2 — create_supplier ========================
	case "create_supplier":
		name := getParamString(params, "name")
		if name == "" {
			return "\n\n【快捷動作失敗】\n建立供應商需要提供名稱（name）。"
		}
		supplier := models.Supplier{
			TenantID: tenantID,
			Name:     name,
			Status:   "active",
		}
		if v := getParamString(params, "phone"); v != "" {
			supplier.Phone = v
		}
		if v := getParamString(params, "email"); v != "" {
			supplier.Email = v
		}
		if v := getParamString(params, "address"); v != "" {
			supplier.Address = v
		}
		if userID != uuid.Nil {
			supplier.CreatedBy = &userID
			supplier.UpdatedBy = &userID
		}
		if err := database.DB.Create(&supplier).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立供應商失敗: %s", err.Error())
		}
		var supResult strings.Builder
		supResult.WriteString("\n\n【快捷動作成功 - 供應商已建立】\n")
		supResult.WriteString(fmt.Sprintf("- 名稱: %s\n", supplier.Name))
		if supplier.Phone != "" {
			supResult.WriteString(fmt.Sprintf("- 電話: %s\n", supplier.Phone))
		}
		if supplier.Email != "" {
			supResult.WriteString(fmt.Sprintf("- 電郵: %s\n", supplier.Email))
		}
		supResult.WriteString(fmt.Sprintf("- 供應商 ID: %s\n", supplier.ID.String()))
		supResult.WriteString(fmt.Sprintf("- 管理鏈接: %s/suppliers/%s/edit\n", baseURL, supplier.ID.String()))
		return supResult.String()

	// ======================== Tier 2 — create_purchase_order ========================
	case "create_purchase_order":
		supplierName := getParamString(params, "supplier_name")
		if supplierName == "" {
			return "\n\n【快捷動作失敗】\n建立採購單需要提供供應商名稱（supplier_name）。"
		}
		var supplier models.Supplier
		if err := database.DB.Where("tenant_id = ? AND name ILIKE ?", tenantID, "%"+supplierName+"%").First(&supplier).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到供應商「%s」，請先建立供應商。", supplierName)
		}
		// Auto-generate order number
		poNumber := "PO-" + time.Now().Format("20060102150405")
		for i := 0; i < 5; i++ {
			var existing models.PurchaseOrder
			if err := database.DB.Where("tenant_id = ? AND order_number = ?", tenantID, poNumber).First(&existing).Error; err != nil {
				break
			}
			poNumber = "PO-" + time.Now().Format("20060102150405") + "-" + uuid.New().String()[:4]
		}
		now := utils.NowInTenantTimezone(tenantID)
		orderDate := now
		if v := getParamString(params, "order_date"); v != "" {
			if d, err := utils.ParseDateInTenantTimezone(tenantID, v); err == nil {
				orderDate = d
			}
		}
		po := models.PurchaseOrder{
			TenantID:    tenantID,
			SupplierID:  &supplier.ID,
			OrderNumber: poNumber,
			OrderDate:   orderDate,
			Status:      "draft",
			Notes:       getParamString(params, "notes"),
		}
		// Calculate total from items
		items := getParamItems(params, "items")
		var totalAmount float64
		for _, item := range items {
			qty := getParamFloat(item, "quantity")
			price := getParamFloat(item, "unit_price")
			if qty <= 0 {
				qty = 1
			}
			totalAmount += qty * price
		}
		po.TotalAmount = totalAmount
		po.FinalAmount = totalAmount
		if err := database.DB.Create(&po).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立採購單失敗: %s", err.Error())
		}
		// Create items
		for _, item := range items {
			productName := getParamString(item, "product_name")
			qty := getParamFloat(item, "quantity")
			unitPrice := getParamFloat(item, "unit_price")
			if qty <= 0 {
				qty = 1
			}
			poItem := models.PurchaseOrderItem{
				PurchaseOrderID: po.ID,
				Quantity:        int(qty),
				UnitPrice:       unitPrice,
				TotalAmount:     qty * unitPrice,
			}
			// Try to find product
			if productName != "" {
				var product models.Product
				if err := database.DB.Where("tenant_id = ? AND name ILIKE ?", tenantID, "%"+productName+"%").First(&product).Error; err == nil {
					poItem.ProductID = &product.ID
					if unitPrice <= 0 {
						poItem.UnitPrice = product.Cost
						poItem.TotalAmount = qty * product.Cost
					}
				}
			}
			database.DB.Create(&poItem)
		}
		var poResult strings.Builder
		poResult.WriteString("\n\n【快捷動作成功 - 採購單已建立】\n")
		poResult.WriteString(fmt.Sprintf("- 採購單編號: %s\n", po.OrderNumber))
		poResult.WriteString(fmt.Sprintf("- 供應商: %s\n", supplier.Name))
		poResult.WriteString(fmt.Sprintf("- 總金額: %.2f\n", po.TotalAmount))
		poResult.WriteString(fmt.Sprintf("- 狀態: %s\n", po.Status))
		poResult.WriteString(fmt.Sprintf("- 採購單 ID: %s\n", po.ID.String()))
		poResult.WriteString(fmt.Sprintf("- 管理鏈接: %s/purchase-orders/%s/edit\n", baseURL, po.ID.String()))
		return poResult.String()

	// ======================== Tier 2 — update_customer ========================
	case "update_customer":
		customerName := getParamString(params, "customer_name")
		if customerName == "" {
			return "\n\n【快捷動作失敗】\n需要提供客戶名稱（customer_name）以識別要更新的客戶。"
		}
		custQuery := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
		if phone := getParamString(params, "customer_phone"); phone != "" {
			custQuery = custQuery.Where("phone ILIKE ?", "%"+phone+"%")
		} else {
			custQuery = custQuery.Where("name ILIKE ?", "%"+customerName+"%")
		}
		var customer models.Customer
		if err := custQuery.First(&customer).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到客戶「%s」。", customerName)
		}
		updates := map[string]interface{}{}
		if v := getParamString(params, "new_name"); v != "" {
			updates["name"] = v
		}
		if v := getParamString(params, "new_phone"); v != "" {
			updates["phone"] = v
		}
		if v := getParamString(params, "new_email"); v != "" {
			updates["email"] = v
		}
		if v := getParamString(params, "new_address"); v != "" {
			updates["address"] = v
		}
		if v := getParamString(params, "new_gender"); v != "" {
			updates["gender"] = v
		}
		if v := getParamString(params, "new_status"); v != "" {
			updates["status"] = v
		}
		if len(updates) == 0 {
			return "\n\n【快捷動作失敗】\n請提供要更新的欄位（如 new_name, new_phone, new_email 等）。"
		}
		updates["updated_at"] = utils.NowInTenantTimezone(tenantID)
		if err := database.DB.Model(&customer).Updates(updates).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n更新客戶失敗: %s", err.Error())
		}
		var custResult strings.Builder
		custResult.WriteString("\n\n【快捷動作成功 - 客戶已更新】\n")
		custResult.WriteString(fmt.Sprintf("- 客戶: %s\n", customer.Name))
		for k, v := range updates {
			if k != "updated_at" {
				custResult.WriteString(fmt.Sprintf("- %s → %v\n", k, v))
			}
		}
		custResult.WriteString(fmt.Sprintf("- 管理鏈接: %s/customers/%s/edit\n", baseURL, customer.ID.String()))
		return custResult.String()

	// ======================== Tier 2 — update_product ========================
	case "update_product":
		productName := getParamString(params, "product_name")
		if productName == "" {
			return "\n\n【快捷動作失敗】\n需要提供商品名稱（product_name）以識別要更新的商品。"
		}
		prodQuery := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
		if sku := getParamString(params, "product_sku"); sku != "" {
			prodQuery = prodQuery.Where("sku ILIKE ?", "%"+sku+"%")
		} else {
			prodQuery = prodQuery.Where("name ILIKE ?", "%"+productName+"%")
		}
		var product models.Product
		if err := prodQuery.First(&product).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到商品「%s」。", productName)
		}
		prodUpdates := map[string]interface{}{}
		if v := getParamString(params, "new_name"); v != "" {
			prodUpdates["name"] = v
		}
		if v := getParamFloat(params, "new_price"); v > 0 {
			prodUpdates["price"] = v
		}
		if v := getParamFloat(params, "new_cost"); v > 0 {
			prodUpdates["cost"] = v
		}
		if v := getParamInt(params, "new_stock"); v > 0 {
			prodUpdates["stock_quantity"] = v
		}
		if v := getParamString(params, "new_sku"); v != "" {
			prodUpdates["sku"] = v
		}
		if v := getParamString(params, "new_status"); v != "" {
			prodUpdates["status"] = v
		}
		if len(prodUpdates) == 0 {
			return "\n\n【快捷動作失敗】\n請提供要更新的欄位（如 new_name, new_price, new_stock 等）。"
		}
		prodUpdates["updated_at"] = utils.NowInTenantTimezone(tenantID)
		if err := database.DB.Model(&product).Updates(prodUpdates).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n更新商品失敗: %s", err.Error())
		}
		var prodResult strings.Builder
		prodResult.WriteString("\n\n【快捷動作成功 - 商品已更新】\n")
		prodResult.WriteString(fmt.Sprintf("- 商品: %s\n", product.Name))
		for k, v := range prodUpdates {
			if k != "updated_at" {
				prodResult.WriteString(fmt.Sprintf("- %s → %v\n", k, v))
			}
		}
		prodResult.WriteString(fmt.Sprintf("- 管理鏈接: %s/products/%s/edit\n", baseURL, product.ID.String()))
		return prodResult.String()

	// ======================== Tier 2 — export_customers ========================
	case "export_customers":
		statusFilter := getParamString(params, "status")
		q := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
		if statusFilter != "" {
			q = q.Where("status = ?", statusFilter)
		}
		var count int64
		q.Model(&models.Customer{}).Count(&count)
		// Return a message with the export link instead of streaming the file
		exportURL := fmt.Sprintf("%s/api/v1/customers/export?format=excel", baseURL)
		if statusFilter != "" {
			exportURL += "&status=" + statusFilter
		}
		var expResult strings.Builder
		expResult.WriteString("\n\n【匯出客戶】\n")
		expResult.WriteString(fmt.Sprintf("- 共 %d 筆客戶資料可匯出\n", count))
		expResult.WriteString(fmt.Sprintf("- 匯出鏈接: %s\n", exportURL))
		expResult.WriteString("- 請點擊上方鏈接下載 Excel 檔案\n")
		return expResult.String()

	// ======================== Tier 2 — export_orders ========================
	case "export_orders":
		statusFilter := getParamString(params, "status")
		q := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
		if statusFilter != "" {
			q = q.Where("status = ?", statusFilter)
		}
		var count int64
		q.Model(&models.Order{}).Count(&count)
		exportURL := fmt.Sprintf("%s/api/v1/orders/export?format=excel", baseURL)
		if statusFilter != "" {
			exportURL += "&status=" + statusFilter
		}
		var expOrdResult strings.Builder
		expOrdResult.WriteString("\n\n【匯出訂單】\n")
		expOrdResult.WriteString(fmt.Sprintf("- 共 %d 筆訂單資料可匯出\n", count))
		expOrdResult.WriteString(fmt.Sprintf("- 匯出鏈接: %s\n", exportURL))
		expOrdResult.WriteString("- 請點擊上方鏈接下載 Excel 檔案\n")
		return expOrdResult.String()

	// ======================== Tier 2 — export_products ========================
	case "export_products":
		statusFilter := getParamString(params, "status")
		q := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
		if statusFilter != "" {
			q = q.Where("status = ?", statusFilter)
		}
		var count int64
		q.Model(&models.Product{}).Count(&count)
		exportURL := fmt.Sprintf("%s/api/v1/products/export?format=excel", baseURL)
		if statusFilter != "" {
			exportURL += "&status=" + statusFilter
		}
		var expProdResult strings.Builder
		expProdResult.WriteString("\n\n【匯出商品】\n")
		expProdResult.WriteString(fmt.Sprintf("- 共 %d 筆商品資料可匯出\n", count))
		expProdResult.WriteString(fmt.Sprintf("- 匯出鏈接: %s\n", exportURL))
		expProdResult.WriteString("- 請點擊上方鏈接下載 Excel 檔案\n")
		return expProdResult.String()

	// ======================== Tier 2 — quotation_to_order ========================
	case "quotation_to_order":
		orderNumber := getParamString(params, "order_number")
		if orderNumber == "" {
			return "\n\n【快捷動作失敗】\n需要提供報價單/訂單編號（order_number）。"
		}
		var order models.Order
		if err := database.DB.Where("tenant_id = ? AND order_number = ? AND trashed_at IS NULL", tenantID, orderNumber).First(&order).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到訂單「%s」。", orderNumber)
		}
		if order.Status != "quotation" && order.Status != "draft" {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n訂單「%s」狀態為「%s」，只有報價單（quotation）或草稿（draft）才能轉為確認訂單。", orderNumber, order.Status)
		}
		now := utils.NowInTenantTimezone(tenantID)
		if err := database.DB.Model(&order).Updates(map[string]interface{}{
			"status":     "confirmed",
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n轉換訂單失敗: %s", err.Error())
		}
		var qtoResult strings.Builder
		qtoResult.WriteString("\n\n【快捷動作成功 - 報價單已轉為確認訂單】\n")
		qtoResult.WriteString(fmt.Sprintf("- 訂單編號: %s\n", orderNumber))
		qtoResult.WriteString(fmt.Sprintf("- 金額: %.2f\n", order.TotalAmount))
		qtoResult.WriteString(fmt.Sprintf("- 狀態: confirmed\n"))
		qtoResult.WriteString(fmt.Sprintf("- 管理鏈接: %s/orders/%s/edit\n", baseURL, order.ID.String()))
		return qtoResult.String()

	// ======================== Tier 2 — send_payment_link ========================
	case "send_payment_link":
		orderNumber := getParamString(params, "order_number")
		if orderNumber == "" {
			return "\n\n【快捷動作失敗】\n需要提供訂單編號（order_number）。"
		}
		var order models.Order
		if err := database.DB.Where("tenant_id = ? AND order_number = ? AND trashed_at IS NULL", tenantID, orderNumber).First(&order).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到訂單「%s」。", orderNumber)
		}
		if order.Status == "completed" {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n訂單「%s」已完成，無需付款鏈接。", orderNumber)
		}
		// Check for existing active link
		var existingLink models.PaymentLink
		if err := database.DB.Where("order_id = ? AND tenant_id = ? AND status = ?", order.ID, tenantID, "active").First(&existingLink).Error; err == nil {
			payURL := fmt.Sprintf("%s/pay/%s", baseURL, existingLink.Token)
			c.Locals("vai_payment_link_url", payURL)
			var existResult strings.Builder
			existResult.WriteString("\n\n【付款鏈接】（已存在）\n")
			existResult.WriteString(fmt.Sprintf("- 訂單編號: %s\n", orderNumber))
			existResult.WriteString(fmt.Sprintf("- 金額: %.2f\n", order.TotalAmount))
			existResult.WriteString(fmt.Sprintf("- 付款鏈接: %s\n", payURL))
			return existResult.String()
		}
		// Generate new token
		tokenBytes := make([]byte, 24)
		if _, err := rand.Read(tokenBytes); err != nil {
			return "\n\n【快捷動作失敗】\n生成付款鏈接失敗。"
		}
		token := hex.EncodeToString(tokenBytes)
		now := utils.NowInTenantTimezone(tenantID)
		link := models.PaymentLink{
			TenantID:  tenantID,
			OrderID:   order.ID,
			Token:     token,
			Status:    "active",
			CreatedBy: &userID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		expiresHours := getParamInt(params, "expires_in_hours")
		if expiresHours <= 0 {
			expiresHours = 72
		}
		if expiresHours > 0 {
			exp := now.Add(time.Duration(expiresHours) * time.Hour)
			link.ExpiresAt = &exp
		}
		if err := database.DB.Create(&link).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立付款鏈接失敗: %s", err.Error())
		}
		payURL := fmt.Sprintf("%s/pay/%s", baseURL, token)
		c.Locals("vai_payment_link_url", payURL)
		var payResult strings.Builder
		payResult.WriteString("\n\n【快捷動作成功 - 付款鏈接已生成】\n")
		payResult.WriteString(fmt.Sprintf("- 訂單編號: %s\n", orderNumber))
		payResult.WriteString(fmt.Sprintf("- 金額: %.2f\n", order.TotalAmount))
		payResult.WriteString(fmt.Sprintf("- 付款鏈接: %s\n", payURL))
		if link.ExpiresAt != nil {
			payResult.WriteString(fmt.Sprintf("- 有效期至: %s\n", link.ExpiresAt.Format("2006-01-02 15:04")))
		}
		return payResult.String()

	// ======================== Tier 2 — get_payment_link ========================
	case "get_payment_link":
		orderNumber := getParamString(params, "order_number")
		if orderNumber == "" {
			return "\n\n【查詢失敗】\n需要提供訂單編號（order_number）。"
		}
		var order models.Order
		if err := database.DB.Where("tenant_id = ? AND order_number = ? AND trashed_at IS NULL", tenantID, orderNumber).First(&order).Error; err != nil {
			return fmt.Sprintf("\n\n【查詢失敗】\n找不到訂單「%s」。", orderNumber)
		}
		var activeLink models.PaymentLink
		if err := database.DB.Where("order_id = ? AND tenant_id = ? AND status = ?", order.ID, tenantID, "active").First(&activeLink).Error; err != nil {
			return fmt.Sprintf("\n\n【查詢結果】\n訂單「%s」目前沒有有效的付款鏈接。可以使用 send_payment_link 來生成一個。", orderNumber)
		}
		payURL := fmt.Sprintf("%s/pay/%s", baseURL, activeLink.Token)
		c.Locals("vai_payment_link_url", payURL)
		var getResult strings.Builder
		getResult.WriteString("\n\n【付款鏈接查詢結果】\n")
		getResult.WriteString(fmt.Sprintf("- 訂單編號: %s\n", orderNumber))
		getResult.WriteString(fmt.Sprintf("- 金額: %.2f\n", order.TotalAmount))
		getResult.WriteString(fmt.Sprintf("- 付款鏈接: %s\n", payURL))
		getResult.WriteString(fmt.Sprintf("- 狀態: %s\n", activeLink.Status))
		if activeLink.ExpiresAt != nil {
			getResult.WriteString(fmt.Sprintf("- 有效期至: %s\n", activeLink.ExpiresAt.Format("2006-01-02 15:04")))
		}
		return getResult.String()

	// ======================== Tier 3 — create_reminder ========================
	case "create_reminder":
		title := getParamString(params, "title")
		if title == "" {
			return "\n\n【快捷動作失敗】\n建立提醒需要提供標題（title）。"
		}
		reminder := models.Reminder{
			TenantID: tenantID,
			UserID:   userID,
			Title:    title,
		}
		if v := getParamString(params, "description"); v != "" {
			reminder.Description = v
		}
		if v := getParamString(params, "remind_time"); v != "" {
			loc := utils.GetTenantLocation(tenantID)
			if t, err := time.ParseInLocation("2006-01-02 15:04", v, loc); err == nil {
				reminder.RemindTime = t
			}
		}
		if err := database.DB.Create(&reminder).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立提醒失敗: %s", err.Error())
		}
		var remResult strings.Builder
		remResult.WriteString("\n\n【快捷動作成功 - 提醒已建立】\n")
		remResult.WriteString(fmt.Sprintf("- 標題: %s\n", reminder.Title))
		if !reminder.RemindTime.IsZero() {
			remResult.WriteString(fmt.Sprintf("- 提醒時間: %s\n", reminder.RemindTime.Format("2006-01-02 15:04")))
		}
		remResult.WriteString(fmt.Sprintf("- 提醒 ID: %s\n", reminder.ID.String()))
		return remResult.String()

	// ======================== Tier 3 — create_note ========================
	case "create_note":
		title := getParamString(params, "title")
		if title == "" {
			return "\n\n【快捷動作失敗】\n建立筆記需要提供標題（title）。"
		}
		note := models.Note{
			TenantID: tenantID,
			UserID:   userID,
			Title:    title,
			Content:  getParamString(params, "content"),
			Category: getParamString(params, "category"),
		}
		if err := database.DB.Create(&note).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立筆記失敗: %s", err.Error())
		}
		var noteResult strings.Builder
		noteResult.WriteString("\n\n【快捷動作成功 - 筆記已建立】\n")
		noteResult.WriteString(fmt.Sprintf("- 標題: %s\n", note.Title))
		if note.Category != "" {
			noteResult.WriteString(fmt.Sprintf("- 分類: %s\n", note.Category))
		}
		noteResult.WriteString(fmt.Sprintf("- 筆記 ID: %s\n", note.ID.String()))
		return noteResult.String()

	// ======================== Tier 3 — approve_leave ========================
	case "approve_leave":
		employeeName := getParamString(params, "employee_name")
		if employeeName == "" {
			return "\n\n【快捷動作失敗】\n需要提供員工名稱（employee_name）。"
		}
		// Find the employee
		var employee models.User
		if err := database.DB.Where("tenant_id = ? AND name ILIKE ?", tenantID, "%"+employeeName+"%").First(&employee).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到員工「%s」。", employeeName)
		}
		// Find their pending leave request
		var leaveReq models.LeaveRequest
		if err := database.DB.Where("tenant_id = ? AND user_id = ? AND status = ?", tenantID, employee.ID, "pending").
			Order("created_at DESC").First(&leaveReq).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n找不到員工「%s」的待審批假期申請。", employeeName)
		}
		now := utils.NowInTenantTimezone(tenantID)
		if err := database.DB.Model(&leaveReq).Updates(map[string]interface{}{
			"status":      "approved",
			"approved_by": &userID,
			"approved_at": &now,
			"updated_at":  now,
		}).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n審批假期申請失敗: %s", err.Error())
		}
		var leaveResult strings.Builder
		leaveResult.WriteString("\n\n【快捷動作成功 - 假期申請已批准】\n")
		leaveResult.WriteString(fmt.Sprintf("- 員工: %s\n", employee.Name))
		leaveResult.WriteString(fmt.Sprintf("- 假期類型: %s\n", leaveReq.LeaveType))
		leaveResult.WriteString(fmt.Sprintf("- 開始日期: %s\n", leaveReq.StartDate.Format("2006-01-02")))
		leaveResult.WriteString(fmt.Sprintf("- 結束日期: %s\n", leaveReq.EndDate.Format("2006-01-02")))
		leaveResult.WriteString(fmt.Sprintf("- 天數: %.1f\n", leaveReq.Days))
		return leaveResult.String()

	// ======================== Tier 3 — create_shipment ========================
	case "create_shipment":
		// Auto-generate shipment number
		today := time.Now().Format("20060102")
		var shipCount int64
		database.DB.Model(&models.Shipment{}).Where("tenant_id = ? AND DATE(created_at) = CURRENT_DATE", tenantID).Count(&shipCount)
		shipNumber := fmt.Sprintf("SHP%s%04d", today, shipCount+1)
		now := utils.NowInTenantTimezone(tenantID)
		shipment := models.Shipment{
			TenantID:         tenantID,
			ShipmentNumber:   shipNumber,
			Status:           "pending",
			RecipientName:    getParamString(params, "recipient_name"),
			RecipientPhone:   getParamString(params, "recipient_phone"),
			RecipientAddress: getParamString(params, "recipient_address"),
			SenderName:       getParamString(params, "sender_name"),
			SenderPhone:      getParamString(params, "sender_phone"),
			SenderAddress:    getParamString(params, "sender_address"),
			TrackingNumber:   getParamString(params, "tracking_number"),
			Notes:            getParamString(params, "notes"),
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if userID != uuid.Nil {
			shipment.CreatedBy = &userID
		}
		// Link to order if provided
		if orderNum := getParamString(params, "order_number"); orderNum != "" {
			var order models.Order
			if err := database.DB.Where("tenant_id = ? AND order_number = ? AND trashed_at IS NULL", tenantID, orderNum).First(&order).Error; err == nil {
				shipment.OrderID = &order.ID
			}
		}
		if err := database.DB.Create(&shipment).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立出貨單失敗: %s", err.Error())
		}
		var shipResult strings.Builder
		shipResult.WriteString("\n\n【快捷動作成功 - 出貨單已建立】\n")
		shipResult.WriteString(fmt.Sprintf("- 出貨單編號: %s\n", shipment.ShipmentNumber))
		if shipment.RecipientName != "" {
			shipResult.WriteString(fmt.Sprintf("- 收件人: %s\n", shipment.RecipientName))
		}
		if shipment.TrackingNumber != "" {
			shipResult.WriteString(fmt.Sprintf("- 追蹤號碼: %s\n", shipment.TrackingNumber))
		}
		shipResult.WriteString(fmt.Sprintf("- 狀態: %s\n", shipment.Status))
		shipResult.WriteString(fmt.Sprintf("- 出貨單 ID: %s\n", shipment.ID.String()))
		shipResult.WriteString(fmt.Sprintf("- 管理鏈接: %s/shipments/%s/edit\n", baseURL, shipment.ID.String()))
		return shipResult.String()

	// ======================== Business Goal — create_business_goal ========================
	case "create_business_goal":
		title := getParamString(params, "title")
		if title == "" {
			return "\n\n【快捷動作失敗】\n建立業務目標需要提供標題（title）。"
		}
		metricType := getParamString(params, "metric_type")
		if metricType == "" {
			metricType = "custom"
		}
		targetValue := getParamFloat(params, "target_value")
		if targetValue <= 0 {
			return "\n\n【快捷動作失敗】\n業務目標需要提供大於 0 的目標值（target_value）。"
		}

		// Parse dates
		loc := utils.GetTenantLocation(tenantID)
		startDateStr := getParamString(params, "start_date")
		endDateStr := getParamString(params, "end_date")
		var startDate, endDate time.Time
		var err error
		if startDateStr != "" {
			startDate, err = time.ParseInLocation("2006-01-02", startDateStr, loc)
			if err != nil {
				return fmt.Sprintf("\n\n【快捷動作失敗】\n開始日期格式錯誤: %s，請使用 YYYY-MM-DD 格式。", startDateStr)
			}
		} else {
			startDate = time.Now().In(loc)
		}
		if endDateStr != "" {
			endDate, err = time.ParseInLocation("2006-01-02", endDateStr, loc)
			if err != nil {
				return fmt.Sprintf("\n\n【快捷動作失敗】\n結束日期格式錯誤: %s，請使用 YYYY-MM-DD 格式。", endDateStr)
			}
		} else {
			// Default: end of current month
			now := time.Now().In(loc)
			endDate = time.Date(now.Year(), now.Month()+1, 0, 23, 59, 59, 0, loc)
		}

		goal := models.BusinessGoal{
			TenantID:    tenantID,
			Title:       title,
			Description: getParamString(params, "description"),
			MetricType:  metricType,
			TargetValue: targetValue,
			Unit:        getParamString(params, "unit"),
			StartDate:   startDate,
			EndDate:     endDate,
			Status:      "active",
			Priority:    getParamString(params, "priority"),
		}
		if goal.Priority == "" {
			goal.Priority = "medium"
		}
		if userID != uuid.Nil {
			goal.CreatedBy = &userID
			goal.UpdatedBy = &userID
		}

		// Calculate current value from real data if metric is trackable
		goal.CurrentValue = calculateGoalCurrentValue(tenantID, metricType, nil, startDate, endDate)

		if err := database.DB.Create(&goal).Error; err != nil {
			return fmt.Sprintf("\n\n【快捷動作失敗】\n建立業務目標失敗: %s", err.Error())
		}

		var goalResult strings.Builder
		goalResult.WriteString("\n\n【快捷動作成功 - 業務目標已建立】\n")
		goalResult.WriteString(fmt.Sprintf("- 目標名稱: %s\n", goal.Title))
		goalResult.WriteString(fmt.Sprintf("- 指標類型: %s\n", translateMetricType(goal.MetricType)))
		goalResult.WriteString(fmt.Sprintf("- 目標值: %.0f %s\n", goal.TargetValue, goal.Unit))
		goalResult.WriteString(fmt.Sprintf("- 目前進度: %.0f %s (%.1f%%)\n", goal.CurrentValue, goal.Unit, goal.ProgressPercent()))
		goalResult.WriteString(fmt.Sprintf("- 期間: %s ~ %s\n", goal.StartDate.Format("2006-01-02"), goal.EndDate.Format("2006-01-02")))
		goalResult.WriteString(fmt.Sprintf("- 優先級: %s\n", goal.Priority))
		goalResult.WriteString(fmt.Sprintf("- 目標 ID: %s\n", goal.ID.String()))
		return goalResult.String()
	}

	return ""
}

// translateMetricType returns the Chinese label for a business goal metric type.
func translateMetricType(mt string) string {
	switch mt {
	case "revenue":
		return "營收"
	case "order_count":
		return "訂單數"
	case "customer_count":
		return "客戶數"
	case "product_sales_qty":
		return "產品銷量"
	case "service_order_count":
		return "服務訂單數"
	case "custom":
		return "自定義"
	default:
		return mt
	}
}

// calculateGoalCurrentValue computes the real-time current value for a business
// goal based on its metric_type by querying actual business data.
// For "custom" metrics it returns the stored current_value (no auto-calculation).
func calculateGoalCurrentValue(tenantID uuid.UUID, metricType string, targetEntityID *uuid.UUID, startDate, endDate time.Time) float64 {
	switch metricType {
	case "revenue":
		var total float64
		database.DB.Model(&models.Order{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status != ? AND created_at >= ? AND created_at <= ?",
				tenantID, "quotation", startDate, endDate).
			Select("COALESCE(SUM(total_amount), 0)").Scan(&total)
		return total

	case "order_count":
		var count int64
		database.DB.Model(&models.Order{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status != ? AND created_at >= ? AND created_at <= ?",
				tenantID, "quotation", startDate, endDate).
			Count(&count)
		return float64(count)

	case "customer_count":
		var count int64
		database.DB.Model(&models.Customer{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at <= ?",
				tenantID, startDate, endDate).
			Count(&count)
		return float64(count)

	case "product_sales_qty":
		var total float64
		db := database.DB.Model(&models.OrderItem{}).
			Joins("JOIN orders ON orders.id = order_items.order_id").
			Where("orders.tenant_id = ? AND orders.trashed_at IS NULL AND orders.status != ? AND orders.created_at >= ? AND orders.created_at <= ?",
				tenantID, "quotation", startDate, endDate)
		if targetEntityID != nil {
			db = db.Where("order_items.product_id = ?", *targetEntityID)
		}
		db.Select("COALESCE(SUM(order_items.quantity), 0)").Scan(&total)
		return total

	case "service_order_count":
		var count int64
		database.DB.Model(&models.ServiceOrder{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at <= ?",
				tenantID, startDate, endDate).
			Count(&count)
		return float64(count)

	default:
		// custom — no auto-calculation, return 0 (caller should use stored value)
		return 0
	}
}

// getDataByIntent 根據意圖類型獲取數據並返回格式化字符串
func getDataByIntent(c *fiber.Ctx, tenantID uuid.UUID, intent string, limit int, query string) string {
	// 防止查詢數據時 panic 導致系統崩潰
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[getDataByIntent] panic recovered (intent=%s): %v", intent, r)
		}
	}()

	if tenantID == uuid.Nil {
		return ""
	}

	baseURL := getCMSBaseURL()

	if limit <= 0 {
		limit = 5
	}
	// 最大限制為 50
	if limit > 50 {
		limit = 50
	}

	var dataSummary strings.Builder

	switch intent {
	case "latest_customers":
		var customers []models.Customer
		if err := database.DB.Where("tenant_id = ?", tenantID).
			Preload("MemberLevel").
			Order("created_at DESC").
			Limit(limit).
			Find(&customers).Error; err == nil && len(customers) > 0 {
			dataSummary.WriteString("\n\n【最新客人數據】\n")
			for i, c := range customers {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, c.Name))
				if c.Email != "" {
					dataSummary.WriteString(fmt.Sprintf(" (%s)", c.Email))
				}
				if c.Phone != "" {
					dataSummary.WriteString(fmt.Sprintf(" - %s", c.Phone))
				}
				dataSummary.WriteString(fmt.Sprintf(" (創建於: %s)\n", c.CreatedAt.Format("2006-01-02")))
			}
		}

	case "top_spending_customers":
		rows, err := database.DB.Model(&models.Order{}).
			Select("customer_id, SUM(total_amount) as total_spending, COUNT(*) as order_count").
			Where("tenant_id = ? AND customer_id IS NOT NULL AND status != ?", tenantID, "quotation").
			Group("customer_id").
			Order("total_spending DESC").
			Limit(limit).
			Rows()

		if err == nil {
			defer rows.Close()
			customerIDs := []uuid.UUID{}
			spendingMap := make(map[uuid.UUID]float64)
			orderCountMap := make(map[uuid.UUID]int64)

			for rows.Next() {
				var customerID uuid.UUID
				var totalSpending float64
				var orderCount int64
				if err := rows.Scan(&customerID, &totalSpending, &orderCount); err == nil {
					customerIDs = append(customerIDs, customerID)
					spendingMap[customerID] = totalSpending
					orderCountMap[customerID] = orderCount
				}
			}

			if len(customerIDs) > 0 {
				var customers []models.Customer
				database.DB.Where("tenant_id = ? AND id IN ?", tenantID, customerIDs).
					Preload("MemberLevel").
					Find(&customers)

				customerMap := make(map[uuid.UUID]models.Customer)
				for _, customer := range customers {
					customerMap[customer.ID] = customer
				}

				dataSummary.WriteString("\n\n【購物最多的客人數據】\n")
				for i, customerID := range customerIDs {
					if customer, exists := customerMap[customerID]; exists {
						dataSummary.WriteString(fmt.Sprintf("%d. %s - 總消費: %.2f (訂單數: %d)\n",
							i+1, customer.Name, spendingMap[customerID], orderCountMap[customerID]))
					}
				}
			}
		}

	case "largest_order":
		if limit == 0 {
			limit = 1
		}
		var orders []models.Order
		if err := database.DB.Where("tenant_id = ? AND status != ?", tenantID, "quotation").
			Preload("Customer").
			Preload("OrderItems.Product").
			Order("total_amount DESC").
			Limit(limit).
			Find(&orders).Error; err == nil && len(orders) > 0 {
			dataSummary.WriteString("\n\n【金額最大的訂單數據】\n")
			for i, order := range orders {
				if i > 0 {
					dataSummary.WriteString("\n")
				}
				dataSummary.WriteString(fmt.Sprintf("%d. 訂單編號: %s\n", i+1, order.OrderNumber))
				if order.Customer != nil {
					dataSummary.WriteString(fmt.Sprintf("   客戶: %s\n", order.Customer.Name))
				}
				dataSummary.WriteString(fmt.Sprintf("   訂單金額: %.2f\n", order.TotalAmount))
				dataSummary.WriteString(fmt.Sprintf("   訂單日期: %s\n", order.OrderDate.Format("2006-01-02")))
				dataSummary.WriteString(fmt.Sprintf("   狀態: %s\n", order.Status))
				if len(order.OrderItems) > 0 {
					dataSummary.WriteString("   商品明細:\n")
					for j, item := range order.OrderItems {
						name := "未知商品"
						if item.Product != nil && item.Product.Name != "" {
							name = item.Product.Name
						} else if item.ItemName != nil && *item.ItemName != "" {
							name = *item.ItemName
						}
						dataSummary.WriteString(fmt.Sprintf("     %d) %s × %.0f @ %.2f = %.2f\n", j+1, name, item.Quantity, item.UnitPrice, item.TotalPrice))
					}
				}
			}
		}

	case "latest_orders", "recent_orders":
		if limit == 0 {
			limit = 5
		}
		var orders []models.Order
		if err := database.DB.Where("tenant_id = ? AND status != ?", tenantID, "quotation").
			Preload("Customer").
			Preload("OrderItems.Product").
			Order("created_at DESC").
			Limit(limit).
			Find(&orders).Error; err == nil && len(orders) > 0 {
			dataSummary.WriteString("\n\n【最新訂單數據】\n")
			for i, order := range orders {
				if i > 0 {
					dataSummary.WriteString("\n")
				}
				dataSummary.WriteString(fmt.Sprintf("%d. 訂單編號: %s\n", i+1, order.OrderNumber))
				if order.Customer != nil {
					dataSummary.WriteString(fmt.Sprintf("   客戶: %s\n", order.Customer.Name))
				}
				dataSummary.WriteString(fmt.Sprintf("   金額: %.2f\n", order.TotalAmount))
				dataSummary.WriteString(fmt.Sprintf("   日期: %s\n", order.OrderDate.Format("2006-01-02")))
				dataSummary.WriteString(fmt.Sprintf("   狀態: %s\n", order.Status))
				if len(order.OrderItems) > 0 {
					dataSummary.WriteString("   商品明細:\n")
					for j, item := range order.OrderItems {
						name := "未知商品"
						if item.Product != nil && item.Product.Name != "" {
							name = item.Product.Name
						} else if item.ItemName != nil && *item.ItemName != "" {
							name = *item.ItemName
						}
						dataSummary.WriteString(fmt.Sprintf("     %d) %s × %.0f @ %.2f = %.2f\n", j+1, name, item.Quantity, item.UnitPrice, item.TotalPrice))
					}
				}
			}
		}

	case "latest_products":
		if limit == 0 {
			limit = 5
		}
		var products []models.Product
		if err := database.DB.Where("tenant_id = ?", tenantID).
			Preload("ProductType").
			Preload("Brand").
			Order("created_at DESC").
			Limit(limit).
			Find(&products).Error; err == nil && len(products) > 0 {
			dataSummary.WriteString("\n\n【最新產品數據】\n")
			for i, p := range products {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, p.Name))
				if p.Code != "" {
					dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", p.Code))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 價格: %.2f", p.Price))
				dataSummary.WriteString(fmt.Sprintf(" - 庫存: %d", p.StockQuantity))
				if p.Unit != "" {
					dataSummary.WriteString(fmt.Sprintf(" %s", p.Unit))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s", p.Status))
				dataSummary.WriteString(fmt.Sprintf(" (創建於: %s)\n", p.CreatedAt.Format("2006-01-02")))
			}
		}

	case "low_stock_products":
		if limit == 0 {
			limit = 10
		}
		var products []models.Product
		if err := database.DB.Where("tenant_id = ? AND stock_quantity < 10 AND stock_quantity >= 0", tenantID).
			Order("stock_quantity ASC").
			Limit(limit).
			Find(&products).Error; err == nil && len(products) > 0 {
			dataSummary.WriteString("\n\n【低庫存產品數據】\n")
			for i, p := range products {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, p.Name))
				if p.Code != "" {
					dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", p.Code))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 庫存: %d", p.StockQuantity))
				if p.Unit != "" {
					dataSummary.WriteString(fmt.Sprintf(" %s", p.Unit))
				}
				dataSummary.WriteString("\n")
			}
		}

	case "latest_appointments":
		if limit == 0 {
			limit = 5
		}
		var appointments []models.Appointment
		if err := database.DB.Where("tenant_id = ?", tenantID).
			Preload("Customer").
			Preload("Service").
			Preload("Staff").
			Order("created_at DESC").
			Limit(limit).
			Find(&appointments).Error; err == nil && len(appointments) > 0 {
			dataSummary.WriteString("\n\n【最新預約數據】\n")
			for i, apt := range appointments {
				dataSummary.WriteString(fmt.Sprintf("%d. ", i+1))
				if apt.CustomerID != uuid.Nil {
					if apt.Customer.Name != "" {
						dataSummary.WriteString(fmt.Sprintf("客戶: %s", apt.Customer.Name))
					}
				}
				if apt.ServiceID != nil && apt.Service != nil {
					dataSummary.WriteString(fmt.Sprintf(" - 服務: %s", apt.Service.Name))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 時間: %s", apt.StartTime.Format("2006-01-02 15:04")))
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", apt.Status))
			}
		}

	case "upcoming_appointments":
		if limit == 0 {
			limit = 5
		}
		now := time.Now()
		var appointments []models.Appointment
		if err := database.DB.Where("tenant_id = ? AND start_time >= ? AND status != ?", tenantID, now, "cancelled").
			Preload("Customer").
			Preload("Service").
			Preload("Staff").
			Order("start_time ASC").
			Limit(limit).
			Find(&appointments).Error; err == nil && len(appointments) > 0 {
			dataSummary.WriteString("\n\n【即將到來的預約數據】\n")
			for i, apt := range appointments {
				dataSummary.WriteString(fmt.Sprintf("%d. ", i+1))
				if apt.CustomerID != uuid.Nil {
					if apt.Customer.Name != "" {
						dataSummary.WriteString(fmt.Sprintf("客戶: %s", apt.Customer.Name))
					}
				}
				if apt.ServiceID != nil && apt.Service != nil {
					dataSummary.WriteString(fmt.Sprintf(" - 服務: %s", apt.Service.Name))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 時間: %s", apt.StartTime.Format("2006-01-02 15:04")))
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", apt.Status))
			}
		}

	case "holidays":
		if limit == 0 {
			limit = 10
		}
		start := time.Now()
		end := start.AddDate(0, 0, 30)
		var holidays []models.Holiday
		if err := database.DB.Where("tenant_id = ? AND status = ? AND start_date <= ? AND end_date >= ?", tenantID, "active", end, start).
			Order("start_date ASC").
			Limit(limit).
			Find(&holidays).Error; err == nil && len(holidays) > 0 {
			dataSummary.WriteString("\n\n【近期假期】\n")
			for i, h := range holidays {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, h.Name))
				dataSummary.WriteString(fmt.Sprintf(" - %s", h.StartDate.Format("2006-01-02")))
				if !h.EndDate.IsZero() && !h.EndDate.Equal(h.StartDate) {
					dataSummary.WriteString(fmt.Sprintf(" ~ %s", h.EndDate.Format("2006-01-02")))
				}
				if h.Description != "" {
					dataSummary.WriteString(fmt.Sprintf(" - %s", h.Description))
				}
				dataSummary.WriteString("\n")
			}
		}

	case "staff_shifts":
		if limit == 0 {
			limit = 20
		}
		var users []models.User
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").
			Preload("Department").
			Preload("Shift").
			Order("created_at DESC").
			Limit(limit).
			Find(&users).Error; err == nil && len(users) > 0 {
			dataSummary.WriteString("\n\n【員工排工/班表】\n")
			for i, u := range users {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, u.Name))
				if u.Department != nil && u.Department.Name != "" {
					dataSummary.WriteString(fmt.Sprintf(" - 部門: %s", u.Department.Name))
				}
				if u.Shift != nil {
					dataSummary.WriteString(fmt.Sprintf(" - 班別: %s", u.Shift.Name))
					if u.Shift.StartTime != "" || u.Shift.EndTime != "" {
						dataSummary.WriteString(fmt.Sprintf(" (%s-%s)", u.Shift.StartTime, u.Shift.EndTime))
					}
				}
				dataSummary.WriteString("\n")
			}
		}

	case "available_rooms":
		var rooms []models.Room
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "available").
			Order("name ASC").
			Find(&rooms).Error; err == nil && len(rooms) > 0 {
			dataSummary.WriteString("\n\n【可用房間數據】\n")
			for i, r := range rooms {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, r.Name))
				if r.Code != nil && *r.Code != "" {
					dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", *r.Code))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", r.Status))
			}
		}

	case "available_equipments":
		var equipments []models.Equipment
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "available").
			Order("name ASC").
			Find(&equipments).Error; err == nil && len(equipments) > 0 {
			dataSummary.WriteString("\n\n【可用設備數據】\n")
			for i, e := range equipments {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, e.Name))
				if e.Code != nil && *e.Code != "" {
					dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", *e.Code))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", e.Status))
			}
		}

	case "available_vehicles":
		var vehicles []models.Vehicle
		if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "available").
			Order("name ASC").
			Find(&vehicles).Error; err == nil && len(vehicles) > 0 {
			dataSummary.WriteString("\n\n【可用車輛數據】\n")
			for i, v := range vehicles {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, v.Name))
				if v.Code != nil && *v.Code != "" {
					dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", *v.Code))
				}
				if v.LicensePlate != "" {
					dataSummary.WriteString(fmt.Sprintf(" - 車牌: %s", v.LicensePlate))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", v.Status))
			}
		}

	case "latest_service_orders":
		if limit == 0 {
			limit = 5
		}
		var serviceOrders []models.ServiceOrder
		if err := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).
			Preload("Customer").
			Preload("ServiceOrderItems.Service").
			Order("created_at DESC").
			Limit(limit).
			Find(&serviceOrders).Error; err == nil && len(serviceOrders) > 0 {
			dataSummary.WriteString("\n\n【最新服務單數據】\n")
			for i, so := range serviceOrders {
				if i > 0 {
					dataSummary.WriteString("\n")
				}
				dataSummary.WriteString(fmt.Sprintf("%d. 服務單編號: %s\n", i+1, so.OrderNumber))
				if so.CustomerID != nil && so.Customer.Name != "" {
					dataSummary.WriteString(fmt.Sprintf("   客戶: %s\n", so.Customer.Name))
				}
				dataSummary.WriteString(fmt.Sprintf("   金額: %.2f\n", so.TotalAmount))
				dataSummary.WriteString(fmt.Sprintf("   服務日期: %s\n", so.ServiceDate.Format("2006-01-02")))
				dataSummary.WriteString(fmt.Sprintf("   狀態: %s\n", so.Status))
				if len(so.ServiceOrderItems) > 0 {
					dataSummary.WriteString("   服務項目:\n")
					for j, item := range so.ServiceOrderItems {
						name := "未知服務"
						if item.Service != nil && item.Service.Name != "" {
							name = item.Service.Name
						}
						dataSummary.WriteString(fmt.Sprintf("     %d) %s × %.0f @ %.2f = %.2f\n", j+1, name, item.Quantity, item.UnitPrice, item.TotalPrice))
					}
				}
			}
		}

	case "search_service_order":
		if query == "" {
			dataSummary.WriteString("\n\n【搜尋服務單】\n請提供搜尋關鍵詞（服務單號、客戶名稱或服務名稱）。")
			break
		}
		if limit == 0 {
			limit = 10
		}
		var svcOrders []models.ServiceOrder
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (order_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = service_orders.customer_id AND customers.name ILIKE ?) OR EXISTS (SELECT 1 FROM service_order_items soi JOIN services s ON soi.service_id = s.id WHERE soi.service_order_id = service_orders.id AND soi.trashed_at IS NULL AND s.name ILIKE ?))",
			tenantID, "%"+query+"%", "%"+query+"%", "%"+query+"%").
			Preload("Customer").
			Preload("ServiceOrderItems.Service").
			Order("created_at DESC").Limit(limit).Find(&svcOrders)
		if len(svcOrders) == 0 {
			dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋服務單】\n未找到符合「%s」的服務單。", query))
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋服務單 — 符合「%s」的結果（共 %d 筆）】\n", query, len(svcOrders)))
		for i, so := range svcOrders {
			if i > 0 {
				dataSummary.WriteString("\n")
			}
			dataSummary.WriteString(fmt.Sprintf("%d. 服務單編號: %s\n", i+1, so.OrderNumber))
			if so.CustomerID != nil && so.Customer.Name != "" {
				dataSummary.WriteString(fmt.Sprintf("   客戶: %s\n", so.Customer.Name))
			}
			dataSummary.WriteString(fmt.Sprintf("   金額: %.2f\n", so.TotalAmount))
			dataSummary.WriteString(fmt.Sprintf("   服務日期: %s\n", so.ServiceDate.Format("2006-01-02")))
			dataSummary.WriteString(fmt.Sprintf("   狀態: %s\n", so.Status))
			if len(so.ServiceOrderItems) > 0 {
				dataSummary.WriteString("   服務項目:\n")
				for j, item := range so.ServiceOrderItems {
					name := "未知服務"
					if item.Service != nil && item.Service.Name != "" {
						name = item.Service.Name
					}
					dataSummary.WriteString(fmt.Sprintf("     %d) %s × %.0f @ %.2f = %.2f\n", j+1, name, item.Quantity, item.UnitPrice, item.TotalPrice))
				}
			}
		}

	case "latest_users":
		if limit == 0 {
			limit = 5
		}
		var users []models.User
		if err := database.DB.Where("tenant_id = ?", tenantID).
			Preload("Department").
			Preload("Role").
			Order("created_at DESC").
			Limit(limit).
			Find(&users).Error; err == nil && len(users) > 0 {
			dataSummary.WriteString("\n\n【最新員工數據】\n")
			for i, u := range users {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, u.Name))
				if u.EmployeeNumber != "" {
					dataSummary.WriteString(fmt.Sprintf(" (員工編號: %s)", u.EmployeeNumber))
				}
				if u.Email != "" {
					dataSummary.WriteString(fmt.Sprintf(" - %s", u.Email))
				}
				if u.Department != nil {
					dataSummary.WriteString(fmt.Sprintf(" - 部門: %s", u.Department.Name))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", u.Status))
			}
		}

	case "latest_service_staffs":
		if limit == 0 {
			limit = 5
		}
		var staffs []models.ServiceStaff
		if err := database.DB.Where("tenant_id = ?", tenantID).
			Preload("User").
			Order("created_at DESC").
			Limit(limit).
			Find(&staffs).Error; err == nil && len(staffs) > 0 {
			dataSummary.WriteString("\n\n【最新服務員數據】\n")
			for i, s := range staffs {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, s.Name))
				if s.EmployeeNumber != nil && *s.EmployeeNumber != "" {
					dataSummary.WriteString(fmt.Sprintf(" (員工編號: %s)", *s.EmployeeNumber))
				}
				if s.Phone != "" {
					dataSummary.WriteString(fmt.Sprintf(" - %s", s.Phone))
				}
				if s.Specialization != "" {
					dataSummary.WriteString(fmt.Sprintf(" - 專長: %s", s.Specialization))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", s.Status))
			}
		}

	case "latest_services":
		if limit == 0 {
			limit = 10
		}
		var services []models.Service
		if err := database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND status = 'active'", tenantID).
			Preload("ServiceType").
			Order("created_at DESC").
			Limit(limit).
			Find(&services).Error; err == nil && len(services) > 0 {
			dataSummary.WriteString("\n\n【服務項目目錄】\n")
			for i, s := range services {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, s.Name))
				if s.Code != nil && *s.Code != "" {
					dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", *s.Code))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 價格: %.2f", s.Price))
				if s.DurationMinutes != nil && *s.DurationMinutes > 0 {
					dataSummary.WriteString(fmt.Sprintf(" - 時長: %d分鐘", *s.DurationMinutes))
				}
				if s.ServiceType != nil {
					dataSummary.WriteString(fmt.Sprintf(" - 類型: %s", s.ServiceType.Name))
				}
				if s.Description != "" {
					dataSummary.WriteString(fmt.Sprintf(" - %s", s.Description))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", s.Status))
			}
		} else {
			dataSummary.WriteString("\n\n【服務項目目錄】\n目前沒有服務項目。")
		}

	case "search_service":
		if query == "" {
			dataSummary.WriteString("\n\n【搜尋服務】\n請提供搜尋關鍵詞（服務名稱或編號）。")
			break
		}
		var services []models.Service
		if limit == 0 {
			limit = 10
		}
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (name ILIKE ? OR code ILIKE ?)",
			tenantID, "%"+query+"%", "%"+query+"%").
			Preload("ServiceType").
			Order("created_at DESC").Limit(limit).Find(&services)
		if len(services) == 0 {
			dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋服務】\n未找到符合「%s」的服務項目。", query))
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋服務 — 符合「%s」的結果（共 %d 筆）】\n", query, len(services)))
		for i, s := range services {
			dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, s.Name))
			if s.Code != nil && *s.Code != "" {
				dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", *s.Code))
			}
			dataSummary.WriteString(fmt.Sprintf(" | 價格: %.2f", s.Price))
			if s.DurationMinutes != nil && *s.DurationMinutes > 0 {
				dataSummary.WriteString(fmt.Sprintf(" | 時長: %d分鐘", *s.DurationMinutes))
			}
			if s.ServiceType != nil {
				dataSummary.WriteString(fmt.Sprintf(" | 類型: %s", s.ServiceType.Name))
			}
			dataSummary.WriteString(fmt.Sprintf(" | 狀態: %s\n", s.Status))
		}

	case "departments":
		if limit == 0 {
			limit = 20
		}
		// Department 通過 Company -> Enterprise 關聯到 TenantID
		var departments []models.Department
		query := database.DB.Table("departments").
			Joins("INNER JOIN companies ON companies.id = departments.company_id").
			Joins("INNER JOIN enterprises ON enterprises.id = companies.enterprise_id").
			Where("enterprises.tenant_id = ?", tenantID)

		var deptIDs []uuid.UUID
		if err := query.Select("departments.id").Find(&deptIDs).Error; err == nil && len(deptIDs) > 0 {
			if err := database.DB.Where("id IN ?", deptIDs).
				Preload("Company").
				Preload("Parent").
				Order("name ASC").
				Limit(limit).
				Find(&departments).Error; err == nil && len(departments) > 0 {
				dataSummary.WriteString("\n\n【部門數據】\n")
				for i, d := range departments {
					dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, d.Name))
					if d.Company.Name != "" {
						dataSummary.WriteString(fmt.Sprintf(" - 公司: %s", d.Company.Name))
					}
					if d.Parent != nil {
						dataSummary.WriteString(fmt.Sprintf(" - 上級部門: %s", d.Parent.Name))
					}
					dataSummary.WriteString("\n")
				}
			}
		}

	case "latest_projects":
		if limit == 0 {
			limit = 5
		}
		var projects []models.Project
		if err := database.DB.Where("tenant_id = ?", tenantID).
			Preload("ProjectType").
			Preload("Owner").
			Order("created_at DESC").
			Limit(limit).
			Find(&projects).Error; err == nil && len(projects) > 0 {
			dataSummary.WriteString("\n\n【最新項目數據】\n")
			for i, p := range projects {
				dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, p.Name))
				if p.Code != "" {
					dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", p.Code))
				}
				if p.Owner != nil {
					dataSummary.WriteString(fmt.Sprintf(" - 負責人: %s", p.Owner.Name))
				}
				if p.ProjectType != nil {
					dataSummary.WriteString(fmt.Sprintf(" - 類型: %s", p.ProjectType.Name))
				}
				dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s\n", p.Status))
			}
		}

	case "search_customer":
		// 根據用戶查詢搜索客戶（名字、電話、電郵）
		searchTerm := strings.TrimSpace(query)
		if searchTerm == "" {
			dataSummary.WriteString("\n\n【客戶搜索結果】\n請提供客戶姓名、電話或電郵進行搜索。\n")
		} else {
			if limit <= 0 {
				limit = 10
			}
			searchPattern := "%" + searchTerm + "%"
			var customers []models.Customer
			if err := database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (name ILIKE ? OR last_name ILIKE ? OR phone ILIKE ? OR email ILIKE ? OR code ILIKE ?)",
				tenantID, searchPattern, searchPattern, searchPattern, searchPattern, searchPattern).
				Preload("MemberLevel").
				Order("created_at DESC").
				Limit(limit).
				Find(&customers).Error; err == nil && len(customers) > 0 {
				dataSummary.WriteString(fmt.Sprintf("\n\n【客戶搜索結果】(搜索: %s，共 %d 筆)\n", searchTerm, len(customers)))
				for i, cust := range customers {
					dataSummary.WriteString(fmt.Sprintf("%d. %s", i+1, cust.Name))
					if cust.LastName != "" {
						dataSummary.WriteString(fmt.Sprintf(" %s", cust.LastName))
					}
					if cust.Code != "" {
						dataSummary.WriteString(fmt.Sprintf(" (編號: %s)", cust.Code))
					}
					if cust.Phone != "" {
						dataSummary.WriteString(fmt.Sprintf(" - 電話: %s", cust.Phone))
					}
					if cust.Email != "" {
						dataSummary.WriteString(fmt.Sprintf(" - 電郵: %s", cust.Email))
					}
					if cust.Gender != "" && cust.Gender != "unknown" {
						dataSummary.WriteString(fmt.Sprintf(" - 性別: %s", cust.Gender))
					}
					if cust.Address != "" {
						dataSummary.WriteString(fmt.Sprintf(" - 地址: %s", cust.Address))
					}
					if cust.MemberLevel != nil {
						dataSummary.WriteString(fmt.Sprintf(" - 會員等級: %s", cust.MemberLevel.Name))
					}
					dataSummary.WriteString(fmt.Sprintf(" - 狀態: %s", cust.Status))
					// 附加管理鏈接
					dataSummary.WriteString(fmt.Sprintf("\n   管理鏈接: %s/customers/%s/edit\n", baseURL, cust.ID.String()))
				}
			} else {
				dataSummary.WriteString(fmt.Sprintf("\n\n【客戶搜索結果】\n找不到匹配「%s」的客戶。\n", searchTerm))
			}
		}

	case "help":
		// 服務端關鍵詞匹配幫助分類，避免 LLM 幻覺生成不存在的 URL
		type helpCategory struct {
			Slug     string
			Title    string
			Keywords []string // 用於匹配用戶查詢
		}
		helpCategories := []helpCategory{
			{"getting-started", "快速入門指南", []string{"入門", "開始", "新手", "快速", "start", "begin", "getting started", "setup"}},
			{"account", "帳戶管理", []string{"帳戶", "賬戶", "帳號", "賬號", "登入", "登錄", "密碼", "account", "login", "password"}},
			{"product", "商品管理", []string{"商品", "產品", "product", "商品管理"}},
			{"order", "訂單管理", []string{"訂單", "下單", "order", "落單"}},
			{"customer", "客戶管理", []string{"客戶", "客人", "會員", "customer", "client", "member"}},
			{"service", "服務管理", []string{"服務", "service", "服務單"}},
			{"pos", "POS 收銀系統", []string{"pos", "收銀", "結帳", "結賬", "收款", "cashier"}},
			{"pos-self-service", "POS 自助", []string{"自助", "self-service", "自助點餐"}},
			{"vbuilder", "vBuilder 網站", []string{"vbuilder", "網站建設", "建站", "website builder"}},
			{"import-export", "匯入 / 匯出", []string{"匯入", "匯出", "導入", "導出", "import", "export", "csv", "excel"}},
			{"inventory", "庫存管理", []string{"庫存", "存貨", "盤點", "inventory", "stock"}},
			{"accounting", "會計管理", []string{"會計", "記帳", "記賬", "財務", "accounting", "finance"}},
			{"hr", "HR 人力資源", []string{"hr", "人力", "人事", "員工", "staff", "employee", "薪資", "考勤"}},
			{"dynamic-fields", "動態字段功能", []string{"動態字段", "自定義字段", "custom field", "dynamic field"}},
			{"reports", "報表功能", []string{"報表", "報告", "report", "統計", "分析"}},
			{"ai", "vAi 智能助手", []string{"ai", "vai", "智能", "助手"}},
			{"purchase", "採購管理", []string{"採購", "進貨", "purchase"}},
			{"supplier", "供應商管理", []string{"供應商", "供貨商", "supplier", "vendor"}},
			{"warehouse", "倉庫管理", []string{"倉庫", "倉儲", "warehouse"}},
			{"project", "項目管理", []string{"項目", "專案", "project"}},
			{"store", "店舖管理", []string{"店舖", "店鋪", "門店", "store", "shop"}},
			{"website", "網站管理", []string{"網站", "網頁", "website", "webpage"}},
			{"promotion", "優惠管理", []string{"優惠", "折扣", "促銷", "coupon", "promotion", "discount"}},
			{"resource", "資源管理", []string{"資源", "房間", "設備", "車輛", "resource", "room", "equipment"}},
			{"personal-tools", "個人工具", []string{"個人", "工具", "personal", "tool"}},
			{"dns-setup", "DNS 設定", []string{"dns", "域名", "domain"}},
		}

		queryLower := strings.ToLower(strings.TrimSpace(query))
		var matched []helpCategory

		if queryLower != "" {
			// 按關鍵詞匹配
			for _, cat := range helpCategories {
				for _, kw := range cat.Keywords {
					if strings.Contains(queryLower, strings.ToLower(kw)) || strings.Contains(strings.ToLower(kw), queryLower) {
						matched = append(matched, cat)
						break
					}
				}
			}
		}

		if len(matched) == 0 {
			// 沒有匹配到具體分類，返回前 5 個推薦分類
			topCategories := []string{"getting-started", "order", "product", "customer", "pos"}
			for _, slug := range topCategories {
				for _, cat := range helpCategories {
					if cat.Slug == slug {
						matched = append(matched, cat)
						break
					}
				}
			}
		}

		dataSummary.WriteString("\n\n【相關教學文檔】\n")
		dataSummary.WriteString("以下是匹配到的教學頁面，請直接使用這些鏈接回覆用戶，不要修改或創造新的 URL：\n\n")
		for _, cat := range matched {
			helpURL := fmt.Sprintf("%s/help/%s", baseURL, cat.Slug)
			dataSummary.WriteString(fmt.Sprintf("- %s: %s\n", cat.Title, helpURL))
		}
		dataSummary.WriteString("\n注意：只使用上面列出的鏈接，不要自行編造或修改任何 URL。")

	case "total_statistics", "dashboard_stats":
		// 客戶統計
		var totalCustomers int64
		var activeCustomers int64
		database.DB.Model(&models.Customer{}).Where("tenant_id = ?", tenantID).Count(&totalCustomers)
		database.DB.Model(&models.Customer{}).Where("tenant_id = ? AND status = ?", tenantID, "active").Count(&activeCustomers)

		// 產品統計
		var totalProducts int64
		var activeProducts int64
		var lowStockProducts int64
		database.DB.Model(&models.Product{}).Where("tenant_id = ?", tenantID).Count(&totalProducts)
		database.DB.Model(&models.Product{}).Where("tenant_id = ? AND status = ?", tenantID, "active").Count(&activeProducts)
		database.DB.Model(&models.Product{}).Where("tenant_id = ? AND stock_quantity < 10", tenantID).Count(&lowStockProducts)

		// 訂單統計
		var totalOrders int64
		var pendingOrders int64
		database.DB.Model(&models.Order{}).Where("tenant_id = ?", tenantID).Count(&totalOrders)
		database.DB.Model(&models.Order{}).Where("tenant_id = ? AND status IN ?", tenantID, []string{"draft", "confirmed", "processing"}).Count(&pendingOrders)

		// 收入統計
		var totalRevenue float64
		database.DB.Model(&models.Order{}).
			Where("tenant_id = ? AND status != ?", tenantID, "quotation").
			Select("COALESCE(SUM(total_amount), 0)").
			Scan(&totalRevenue)

		dataSummary.WriteString("\n\n【總體統計數據】\n")
		dataSummary.WriteString("客戶統計：\n")
		dataSummary.WriteString(fmt.Sprintf("  - 總客戶數: %d\n", totalCustomers))
		dataSummary.WriteString(fmt.Sprintf("  - 活躍客戶數: %d\n", activeCustomers))
		dataSummary.WriteString("\n產品統計：\n")
		dataSummary.WriteString(fmt.Sprintf("  - 總產品數: %d\n", totalProducts))
		dataSummary.WriteString(fmt.Sprintf("  - 活躍產品數: %d\n", activeProducts))
		dataSummary.WriteString(fmt.Sprintf("  - 低庫存產品數: %d\n", lowStockProducts))
		dataSummary.WriteString("\n訂單統計：\n")
		dataSummary.WriteString(fmt.Sprintf("  - 總訂單數: %d\n", totalOrders))
		dataSummary.WriteString(fmt.Sprintf("  - 待處理訂單數: %d\n", pendingOrders))
		dataSummary.WriteString("\n收入統計：\n")
		dataSummary.WriteString(fmt.Sprintf("  - 總收入: %.2f\n", totalRevenue))

	// ======================== Tier 1 — search_order ========================
	case "search_order":
		if query == "" {
			dataSummary.WriteString("\n\n【搜尋訂單】\n請提供搜尋關鍵詞（訂單號、客戶名稱或產品名稱）。")
			break
		}
		var orders []models.Order
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (order_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = orders.customer_id AND customers.name ILIKE ?) OR EXISTS (SELECT 1 FROM order_items oi LEFT JOIN products p ON oi.product_id = p.id WHERE oi.order_id = orders.id AND oi.trashed_at IS NULL AND (p.name ILIKE ? OR oi.item_name ILIKE ?)))",
			tenantID, "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%").
			Preload("Customer").
			Preload("OrderItems.Product").
			Order("created_at DESC").Limit(limit).Find(&orders)
		if len(orders) == 0 {
			dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋訂單】\n未找到符合「%s」的訂單。", query))
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋訂單 — 符合「%s」的結果（共 %d 筆）】\n", query, len(orders)))
		for i, o := range orders {
			if i > 0 {
				dataSummary.WriteString("\n")
			}
			dataSummary.WriteString(fmt.Sprintf("%d. 訂單編號: %s\n", i+1, o.OrderNumber))
			if o.Customer.Name != "" {
				dataSummary.WriteString(fmt.Sprintf("   客戶: %s\n", o.Customer.Name))
			}
			dataSummary.WriteString(fmt.Sprintf("   金額: %.2f\n", o.TotalAmount))
			dataSummary.WriteString(fmt.Sprintf("   日期: %s\n", o.OrderDate.Format("2006-01-02")))
			dataSummary.WriteString(fmt.Sprintf("   狀態: %s\n", o.Status))
			if len(o.OrderItems) > 0 {
				dataSummary.WriteString("   商品明細:\n")
				for j, item := range o.OrderItems {
					name := "未知商品"
					if item.Product != nil && item.Product.Name != "" {
						name = item.Product.Name
					} else if item.ItemName != nil && *item.ItemName != "" {
						name = *item.ItemName
					}
					dataSummary.WriteString(fmt.Sprintf("     %d) %s × %.0f @ %.2f = %.2f\n", j+1, name, item.Quantity, item.UnitPrice, item.TotalPrice))
				}
			}
		}

	// ======================== Tier 1 — search_product ========================
	case "search_product":
		if query == "" {
			dataSummary.WriteString("\n\n【搜尋產品】\n請提供搜尋關鍵詞（名稱、SKU 或條碼）。")
			break
		}
		var products []models.Product
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (name ILIKE ? OR sku ILIKE ? OR barcode ILIKE ?)",
			tenantID, "%"+query+"%", "%"+query+"%", "%"+query+"%").
			Order("created_at DESC").Limit(limit).Find(&products)
		if len(products) == 0 {
			dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋產品】\n未找到符合「%s」的產品。", query))
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋產品 — 符合「%s」的結果（共 %d 筆）】\n", query, len(products)))
		for i, p := range products {
			dataSummary.WriteString(fmt.Sprintf("%d. %s | SKU: %s | 價格: %.2f | 庫存: %d | 狀態: %s\n",
				i+1, p.Name, p.SKU, p.Price, p.StockQuantity, p.Status))
		}

	// ======================== Tier 1 — latest_invoices ========================
	case "latest_invoices":
		var invoices []models.Invoice
		database.DB.Where("tenant_id = ?", tenantID).
			Preload("Customer").
			Order("created_at DESC").Limit(limit).Find(&invoices)
		if len(invoices) == 0 {
			dataSummary.WriteString("\n\n【最新發票】\n目前沒有發票資料。")
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【最新發票（共 %d 筆）】\n", len(invoices)))
		for i, inv := range invoices {
			custName := "N/A"
			if inv.Customer.Name != "" {
				custName = inv.Customer.Name
			}
			dataSummary.WriteString(fmt.Sprintf("%d. %s | 客戶: %s | 金額: %.2f | 已付: %.2f | 狀態: %s | 日期: %s\n",
				i+1, inv.InvoiceNumber, custName, inv.TotalAmount, inv.PaidAmount, inv.Status, inv.InvoiceDate.Format("2006-01-02")))
		}

	// ======================== Tier 1 — search_invoice ========================
	case "search_invoice":
		if query == "" {
			dataSummary.WriteString("\n\n【搜尋發票】\n請提供搜尋關鍵詞（發票號或客戶名稱）。")
			break
		}
		var invoices []models.Invoice
		database.DB.Where("tenant_id = ? AND (invoice_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = invoices.customer_id AND customers.name ILIKE ?))",
			tenantID, "%"+query+"%", "%"+query+"%").
			Preload("Customer").
			Order("created_at DESC").Limit(limit).Find(&invoices)
		if len(invoices) == 0 {
			dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋發票】\n未找到符合「%s」的發票。", query))
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋發票 — 符合「%s」的結果（共 %d 筆）】\n", query, len(invoices)))
		for i, inv := range invoices {
			custName := "N/A"
			if inv.Customer.Name != "" {
				custName = inv.Customer.Name
			}
			dataSummary.WriteString(fmt.Sprintf("%d. %s | 客戶: %s | 金額: %.2f | 狀態: %s | 日期: %s\n",
				i+1, inv.InvoiceNumber, custName, inv.TotalAmount, inv.Status, inv.InvoiceDate.Format("2006-01-02")))
		}

	// ======================== Tier 1 — get_order_invoices ========================
	case "get_order_invoices":
		orderNumber := query
		if orderNumber == "" {
			dataSummary.WriteString("\n\n【查詢訂單發票】\n請提供訂單號碼。")
			break
		}
		// 找到訂單
		var order models.Order
		if err := database.DB.Where("tenant_id = ? AND order_number = ? AND trashed_at IS NULL", tenantID, orderNumber).
			Preload("Customer").First(&order).Error; err != nil {
			dataSummary.WriteString(fmt.Sprintf("\n\n【查詢訂單發票】\n未找到訂單「%s」。", orderNumber))
			break
		}
		// 確保 Invoice 已同步（處理歷史資料可能沒有 Invoice 的情況）
		syncOrderInvoice(database.DB, tenantID, "order", order.ID, nil)
		// 查詢關聯的 Invoice
		var invoices []models.Invoice
		database.DB.Where("tenant_id = ? AND order_id = ?", tenantID, order.ID).
			Preload("Customer").
			Order("created_at DESC").Find(&invoices)
		// 查詢 incomes 表中的付款記錄
		var incomes []models.Income
		database.DB.Where("tenant_id = ? AND reference_type = ? AND reference_id = ?", tenantID, "order", order.ID).
			Order("income_date DESC").Find(&incomes)
		if len(invoices) == 0 && len(incomes) == 0 {
			dataSummary.WriteString(fmt.Sprintf("\n\n【查詢訂單發票 — %s】\n該訂單尚無發票或付款記錄。", orderNumber))
			break
		}
		custName := "N/A"
		if order.Customer != nil {
			custName = order.Customer.Name
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【訂單 %s 的發票與付款記錄】\n客戶: %s | 訂單金額: %.2f\n", orderNumber, custName, order.TotalAmount))
		if len(invoices) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n發票（共 %d 筆）：\n", len(invoices)))
			for i, inv := range invoices {
				unpaid := inv.TotalAmount - inv.PaidAmount
				dataSummary.WriteString(fmt.Sprintf("  %d. %s | 總額: %.2f | 已付: %.2f | 未付: %.2f | 狀態: %s | 日期: %s\n",
					i+1, inv.InvoiceNumber, inv.TotalAmount, inv.PaidAmount, unpaid, inv.Status, inv.InvoiceDate.Format("2006-01-02")))
			}
		}
		if len(incomes) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n付款記錄（共 %d 筆）：\n", len(incomes)))
			for i, inc := range incomes {
				invoiceNum := ""
				if inc.ExtraFields != nil {
					if n, ok := inc.ExtraFields["invoice_number"].(string); ok {
						invoiceNum = n
					}
				}
				label := inc.Description
				if invoiceNum != "" {
					label = invoiceNum
				}
				dataSummary.WriteString(fmt.Sprintf("  %d. %s | 金額: %.2f | 付款方式: %s | 日期: %s\n",
					i+1, label, inc.Amount, inc.PaymentMethod, inc.IncomeDate.Format("2006-01-02")))
			}
			dataSummary.WriteString(fmt.Sprintf("\n可於管理頁面下載 PDF: %s/orders/%s/edit\n", baseURL, order.ID.String()))
		}

	// ======================== Tier 1 — global_search ========================
	case "global_search":
		if query == "" {
			dataSummary.WriteString("\n\n【全域搜尋】\n請提供搜尋關鍵詞。")
			break
		}
		perType := 5
		dataSummary.WriteString(fmt.Sprintf("\n\n【全域搜尋 — 「%s」】\n", query))
		totalFound := 0
		// Search customers
		var customers []models.Customer
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (code ILIKE ? OR name ILIKE ? OR phone ILIKE ?)",
			tenantID, "%"+query+"%", "%"+query+"%", "%"+query+"%").
			Order("created_at DESC").Limit(perType).Find(&customers)
		if len(customers) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n客戶（%d 筆）：\n", len(customers)))
			for _, c := range customers {
				dataSummary.WriteString(fmt.Sprintf("  - %s (%s) | 電話: %s\n", c.Name, c.Code, c.Phone))
			}
			totalFound += len(customers)
		}
		// Search orders
		var orders []models.Order
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND order_number ILIKE ?",
			tenantID, "%"+query+"%").
			Order("created_at DESC").Limit(perType).Find(&orders)
		if len(orders) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n訂單（%d 筆）：\n", len(orders)))
			for _, o := range orders {
				dataSummary.WriteString(fmt.Sprintf("  - %s | 金額: %.2f | 狀態: %s\n", o.OrderNumber, o.TotalAmount, o.Status))
			}
			totalFound += len(orders)
		}
		// Search products
		var products []models.Product
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (name ILIKE ? OR sku ILIKE ? OR barcode ILIKE ?)",
			tenantID, "%"+query+"%", "%"+query+"%", "%"+query+"%").
			Order("created_at DESC").Limit(perType).Find(&products)
		if len(products) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n產品（%d 筆）：\n", len(products)))
			for _, p := range products {
				dataSummary.WriteString(fmt.Sprintf("  - %s (SKU: %s) | 價格: %.2f\n", p.Name, p.SKU, p.Price))
			}
			totalFound += len(products)
		}
		// Search projects
		var projects []models.Project
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (code ILIKE ? OR name ILIKE ?)",
			tenantID, "%"+query+"%", "%"+query+"%").
			Order("created_at DESC").Limit(perType).Find(&projects)
		if len(projects) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n專案（%d 筆）：\n", len(projects)))
			for _, p := range projects {
				dataSummary.WriteString(fmt.Sprintf("  - %s (%s)\n", p.Name, p.Code))
			}
			totalFound += len(projects)
		}
		// Search users
		var users []models.User
		database.DB.Where("tenant_id = ? AND (name ILIKE ? OR email ILIKE ?)",
			tenantID, "%"+query+"%", "%"+query+"%").
			Order("created_at DESC").Limit(perType).Find(&users)
		if len(users) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n用戶（%d 筆）：\n", len(users)))
			for _, u := range users {
				dataSummary.WriteString(fmt.Sprintf("  - %s | %s\n", u.Name, u.Email))
			}
			totalFound += len(users)
		}
		// Search service orders
		var svcOrders []models.ServiceOrder
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (order_number ILIKE ? OR EXISTS (SELECT 1 FROM customers WHERE customers.id = service_orders.customer_id AND customers.name ILIKE ?) OR EXISTS (SELECT 1 FROM service_order_items soi JOIN services s ON soi.service_id = s.id WHERE soi.service_order_id = service_orders.id AND soi.trashed_at IS NULL AND s.name ILIKE ?))",
			tenantID, "%"+query+"%", "%"+query+"%", "%"+query+"%").
			Preload("Customer").
			Order("created_at DESC").Limit(perType).Find(&svcOrders)
		if len(svcOrders) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n服務單（%d 筆）：\n", len(svcOrders)))
			for _, so := range svcOrders {
				custName := ""
				if so.CustomerID != nil && so.Customer.Name != "" {
					custName = " | 客戶: " + so.Customer.Name
				}
				dataSummary.WriteString(fmt.Sprintf("  - %s%s | 金額: %.2f | 狀態: %s\n", so.OrderNumber, custName, so.TotalAmount, so.Status))
			}
			totalFound += len(svcOrders)
		}
		if totalFound == 0 {
			dataSummary.WriteString("未找到任何符合的結果。")
		} else {
			dataSummary.WriteString(fmt.Sprintf("\n共找到 %d 筆結果。", totalFound))
		}

	// ======================== Tier 2 — search_supplier ========================
	case "search_supplier":
		if query == "" {
			dataSummary.WriteString("\n\n【搜尋供應商】\n請提供搜尋關鍵詞（名稱、電話或電郵）。")
			break
		}
		var suppliers []models.Supplier
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND (name ILIKE ? OR phone ILIKE ? OR email ILIKE ?)",
			tenantID, "%"+query+"%", "%"+query+"%", "%"+query+"%").
			Order("created_at DESC").Limit(limit).Find(&suppliers)
		if len(suppliers) == 0 {
			dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋供應商】\n未找到符合「%s」的供應商。", query))
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【搜尋供應商 — 符合「%s」的結果（共 %d 筆）】\n", query, len(suppliers)))
		for i, s := range suppliers {
			dataSummary.WriteString(fmt.Sprintf("%d. %s | 電話: %s | 電郵: %s | 狀態: %s\n",
				i+1, s.Name, s.Phone, s.Email, s.Status))
		}

	// ======================== Tier 2 — latest_purchase_orders ========================
	case "latest_purchase_orders":
		var purchaseOrders []models.PurchaseOrder
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).
			Preload("Supplier").
			Order("created_at DESC").Limit(limit).Find(&purchaseOrders)
		if len(purchaseOrders) == 0 {
			dataSummary.WriteString("\n\n【最新採購單】\n目前沒有採購單資料。")
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【最新採購單（共 %d 筆）】\n", len(purchaseOrders)))
		for i, po := range purchaseOrders {
			supplierName := "N/A"
			if po.Supplier != nil {
				supplierName = po.Supplier.Name
			}
			dataSummary.WriteString(fmt.Sprintf("%d. %s | 供應商: %s | 金額: %.2f | 狀態: %s | 日期: %s\n",
				i+1, po.OrderNumber, supplierName, po.FinalAmount, po.Status, po.OrderDate.Format("2006-01-02")))
		}

	// ======================== Tier 2 — inventory_summary ========================
	case "inventory_summary":
		var totalProducts int64
		var totalStock int64
		var lowStockCount int64
		var outOfStockCount int64
		database.DB.Model(&models.Product{}).Where("tenant_id = ? AND trashed_at IS NULL", tenantID).Count(&totalProducts)
		database.DB.Model(&models.Product{}).Where("tenant_id = ? AND trashed_at IS NULL", tenantID).
			Select("COALESCE(SUM(stock_quantity), 0)").Scan(&totalStock)
		database.DB.Model(&models.Product{}).Where("tenant_id = ? AND trashed_at IS NULL AND stock_quantity > 0 AND stock_quantity < 10", tenantID).Count(&lowStockCount)
		database.DB.Model(&models.Product{}).Where("tenant_id = ? AND trashed_at IS NULL AND stock_quantity <= 0", tenantID).Count(&outOfStockCount)

		// Top 5 low stock products
		var lowStockProducts []models.Product
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND stock_quantity > 0 AND stock_quantity < 10", tenantID).
			Order("stock_quantity ASC").Limit(5).Find(&lowStockProducts)

		dataSummary.WriteString("\n\n【庫存概覽】\n")
		dataSummary.WriteString(fmt.Sprintf("- 總產品數: %d\n", totalProducts))
		dataSummary.WriteString(fmt.Sprintf("- 總庫存量: %d\n", totalStock))
		dataSummary.WriteString(fmt.Sprintf("- 低庫存產品（<10）: %d\n", lowStockCount))
		dataSummary.WriteString(fmt.Sprintf("- 缺貨產品（=0）: %d\n", outOfStockCount))
		if len(lowStockProducts) > 0 {
			dataSummary.WriteString("\n低庫存產品清單：\n")
			for i, p := range lowStockProducts {
				dataSummary.WriteString(fmt.Sprintf("  %d. %s (SKU: %s) — 剩餘: %d\n", i+1, p.Name, p.SKU, p.StockQuantity))
			}
		}

	// ======================== Tier 3 — upcoming_reminders ========================
	case "upcoming_reminders":
		now := utils.NowInTenantTimezone(tenantID)
		var reminders []models.Reminder
		database.DB.Where("tenant_id = ? AND user_id = ? AND is_completed = false AND remind_time >= ?",
			tenantID, middleware.GetUserID(c), now).
			Order("remind_time ASC").Limit(limit).Find(&reminders)
		if len(reminders) == 0 {
			dataSummary.WriteString("\n\n【即將到來的提醒】\n目前沒有待處理的提醒。")
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【即將到來的提醒（共 %d 筆）】\n", len(reminders)))
		for i, r := range reminders {
			timeStr := r.RemindTime.Format("2006-01-02 15:04")
			dataSummary.WriteString(fmt.Sprintf("%d. %s | 提醒時間: %s\n", i+1, r.Title, timeStr))
		}

	// ======================== Tier 3 — daily_report ========================
	case "daily_report":
		now := utils.NowInTenantTimezone(tenantID)
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		todayEnd := todayStart.Add(24 * time.Hour)

		var newOrders int64
		var orderRevenue float64
		var newCustomers int64
		var newAppointments int64
		database.DB.Model(&models.Order{}).Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at < ?", tenantID, todayStart, todayEnd).Count(&newOrders)
		database.DB.Model(&models.Order{}).Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at < ? AND status != ?", tenantID, todayStart, todayEnd, "quotation").
			Select("COALESCE(SUM(total_amount), 0)").Scan(&orderRevenue)
		database.DB.Model(&models.Customer{}).Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at < ?", tenantID, todayStart, todayEnd).Count(&newCustomers)
		database.DB.Model(&models.Appointment{}).Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at < ?", tenantID, todayStart, todayEnd).Count(&newAppointments)

		dataSummary.WriteString(fmt.Sprintf("\n\n【今日報告 — %s】\n", now.Format("2006-01-02")))
		dataSummary.WriteString(fmt.Sprintf("- 新訂單: %d 筆\n", newOrders))
		dataSummary.WriteString(fmt.Sprintf("- 訂單收入: %.2f\n", orderRevenue))
		dataSummary.WriteString(fmt.Sprintf("- 新客戶: %d 位\n", newCustomers))
		dataSummary.WriteString(fmt.Sprintf("- 新預約: %d 筆\n", newAppointments))

	// ======================== Tier 3 — monthly_report ========================
	case "monthly_report":
		now := utils.NowInTenantTimezone(tenantID)
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		monthEnd := monthStart.AddDate(0, 1, 0)

		var totalOrders int64
		var monthRevenue float64
		var newCustomers int64
		var completedOrders int64
		database.DB.Model(&models.Order{}).Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at < ?", tenantID, monthStart, monthEnd).Count(&totalOrders)
		database.DB.Model(&models.Order{}).Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at < ? AND status != ?", tenantID, monthStart, monthEnd, "quotation").
			Select("COALESCE(SUM(total_amount), 0)").Scan(&monthRevenue)
		database.DB.Model(&models.Customer{}).Where("tenant_id = ? AND trashed_at IS NULL AND created_at >= ? AND created_at < ?", tenantID, monthStart, monthEnd).Count(&newCustomers)
		database.DB.Model(&models.Order{}).Where("tenant_id = ? AND trashed_at IS NULL AND status = ? AND updated_at >= ? AND updated_at < ?", tenantID, "completed", monthStart, monthEnd).Count(&completedOrders)

		dataSummary.WriteString(fmt.Sprintf("\n\n【月報 — %s】\n", now.Format("2006年01月")))
		dataSummary.WriteString(fmt.Sprintf("- 總訂單數: %d 筆\n", totalOrders))
		dataSummary.WriteString(fmt.Sprintf("- 月收入: %.2f\n", monthRevenue))
		dataSummary.WriteString(fmt.Sprintf("- 新客戶: %d 位\n", newCustomers))
		dataSummary.WriteString(fmt.Sprintf("- 已完成訂單: %d 筆\n", completedOrders))

	// ======================== Tier 3 — leave_requests ========================
	case "leave_requests":
		var leaves []models.LeaveRequest
		q := database.DB.Where("tenant_id = ?", tenantID).Preload("User")
		if query == "pending" || query == "" {
			q = q.Where("status = ?", "pending")
		}
		q.Order("created_at DESC").Limit(limit).Find(&leaves)
		if len(leaves) == 0 {
			dataSummary.WriteString("\n\n【請假申請】\n目前沒有待審核的請假申請。")
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【請假申請（共 %d 筆）】\n", len(leaves)))
		for i, l := range leaves {
			userName := "N/A"
			if l.User.Name != "" {
				userName = l.User.Name
			}
			dataSummary.WriteString(fmt.Sprintf("%d. %s | 類型: %s | 期間: %s ~ %s | 天數: %.1f | 狀態: %s\n",
				i+1, userName, l.LeaveType, l.StartDate.Format("2006-01-02"), l.EndDate.Format("2006-01-02"), l.Days, l.Status))
		}

	// ======================== Tier 3 — latest_shipments ========================
	case "latest_shipments":
		var shipments []models.Shipment
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).
			Order("created_at DESC").Limit(limit).Find(&shipments)
		if len(shipments) == 0 {
			dataSummary.WriteString("\n\n【最新出貨單】\n目前沒有出貨單資料。")
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【最新出貨單（共 %d 筆）】\n", len(shipments)))
		for i, s := range shipments {
			dataSummary.WriteString(fmt.Sprintf("%d. %s | 追蹤號: %s | 收件人: %s | 狀態: %s\n",
				i+1, s.ShipmentNumber, s.TrackingNumber, s.RecipientName, s.Status))
		}

	// ======================== Tier 3 — dining_queue ========================
	case "dining_queue":
		var queues []models.DiningQueue
		q := database.DB.Where("tenant_id = ?", tenantID)
		if query == "" || query == "waiting" {
			q = q.Where("status = ?", "waiting")
		}
		q.Order("created_at ASC").Limit(limit).Find(&queues)
		if len(queues) == 0 {
			dataSummary.WriteString("\n\n【候位排隊】\n目前沒有候位中的客人。")
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【候位排隊（共 %d 組）】\n", len(queues)))
		for i, dq := range queues {
			dataSummary.WriteString(fmt.Sprintf("%d. 號碼 %s | 姓名: %s | 人數: %d | 狀態: %s\n",
				i+1, dq.TicketNumber, dq.Name, dq.PartySize, dq.Status))
		}

	// ======================== Tier 3 — pos_daily_summary ========================
	case "pos_daily_summary":
		now := utils.NowInTenantTimezone(tenantID)
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		todayEnd := todayStart.Add(24 * time.Hour)

		var totalSales int64
		var totalSaleAmount float64
		database.DB.Model(&models.POSSale{}).Where("tenant_id = ? AND sale_date >= ? AND sale_date < ?", tenantID, todayStart, todayEnd).Count(&totalSales)
		database.DB.Model(&models.POSSale{}).Where("tenant_id = ? AND sale_date >= ? AND sale_date < ? AND status = ?", tenantID, todayStart, todayEnd, "completed").
			Select("COALESCE(SUM(total_amount), 0)").Scan(&totalSaleAmount)

		dataSummary.WriteString(fmt.Sprintf("\n\n【POS 今日銷售摘要 — %s】\n", now.Format("2006-01-02")))
		dataSummary.WriteString(fmt.Sprintf("- 銷售筆數: %d\n", totalSales))
		dataSummary.WriteString(fmt.Sprintf("- 銷售總額: %.2f\n", totalSaleAmount))

	// ======================== Tier 3 — customer_history ========================
	case "customer_history":
		if query == "" {
			dataSummary.WriteString("\n\n【客戶歷史】\n請提供客戶名稱進行搜尋。")
			break
		}
		var customer models.Customer
		if err := database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND name ILIKE ?", tenantID, "%"+query+"%").First(&customer).Error; err != nil {
			dataSummary.WriteString(fmt.Sprintf("\n\n【客戶歷史】\n未找到符合「%s」的客戶。", query))
			break
		}
		dataSummary.WriteString(fmt.Sprintf("\n\n【客戶歷史 — %s】\n", customer.Name))
		dataSummary.WriteString(fmt.Sprintf("- 客戶編號: %s\n- 電話: %s\n- 電郵: %s\n- 狀態: %s\n", customer.Code, customer.Phone, customer.Email, customer.Status))

		// Orders
		var custOrders []models.Order
		database.DB.Where("tenant_id = ? AND customer_id = ? AND trashed_at IS NULL", tenantID, customer.ID).
			Preload("OrderItems.Product").
			Order("created_at DESC").Limit(10).Find(&custOrders)
		if len(custOrders) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n最近訂單（%d 筆）：\n", len(custOrders)))
			for i, o := range custOrders {
				dataSummary.WriteString(fmt.Sprintf("  %d. 訂單編號: %s\n", i+1, o.OrderNumber))
				dataSummary.WriteString(fmt.Sprintf("     金額: %.2f | 狀態: %s | 日期: %s\n", o.TotalAmount, o.Status, o.OrderDate.Format("2006-01-02")))
				if len(o.OrderItems) > 0 {
					dataSummary.WriteString("     商品明細:\n")
					for j, item := range o.OrderItems {
						name := "未知商品"
						if item.Product != nil && item.Product.Name != "" {
							name = item.Product.Name
						} else if item.ItemName != nil && *item.ItemName != "" {
							name = *item.ItemName
						}
						dataSummary.WriteString(fmt.Sprintf("       %d) %s × %.0f @ %.2f = %.2f\n", j+1, name, item.Quantity, item.UnitPrice, item.TotalPrice))
					}
				}
			}
		}
		// Appointments
		var custAppts []models.Appointment
		database.DB.Where("tenant_id = ? AND customer_id = ? AND trashed_at IS NULL", tenantID, customer.ID).
			Order("created_at DESC").Limit(10).Find(&custAppts)
		if len(custAppts) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n最近預約（%d 筆）：\n", len(custAppts)))
			for i, a := range custAppts {
				dataSummary.WriteString(fmt.Sprintf("  %d. %s | %s | %s\n",
					i+1, a.AppointmentDate.Format("2006-01-02"), a.StartTime.Format("15:04"), a.Status))
			}
		}
		// Invoices
		var custInvoices []models.Invoice
		database.DB.Where("tenant_id = ? AND customer_id = ?", tenantID, customer.ID).
			Order("created_at DESC").Limit(10).Find(&custInvoices)
		if len(custInvoices) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n最近發票（%d 筆）：\n", len(custInvoices)))
			for i, inv := range custInvoices {
				dataSummary.WriteString(fmt.Sprintf("  %d. %s | %.2f | %s | %s\n",
					i+1, inv.InvoiceNumber, inv.TotalAmount, inv.Status, inv.InvoiceDate.Format("2006-01-02")))
			}
		}

	// ======================== Tier 3 — product_sales_analysis ========================
	case "product_sales_analysis":
		type ProductSalesStat struct {
			ProductID     uuid.UUID
			ProductName   string
			TotalQuantity int64
			TotalRevenue  float64
			OrderCount    int64
		}
		// Best sellers — top 10 by quantity
		var bestSellers []ProductSalesStat
		database.DB.Raw(`
			SELECT oi.product_id, p.name as product_name,
				SUM(oi.quantity) as total_quantity,
				SUM(oi.total_amount) as total_revenue,
				COUNT(DISTINCT oi.order_id) as order_count
			FROM order_items oi
			JOIN products p ON p.id = oi.product_id
			JOIN orders o ON o.id = oi.order_id
			WHERE o.tenant_id = ? AND o.trashed_at IS NULL AND o.status != 'quotation'
			GROUP BY oi.product_id, p.name
			ORDER BY total_quantity DESC
			LIMIT 10
		`, tenantID).Scan(&bestSellers)

		dataSummary.WriteString("\n\n【產品銷售分析】\n")
		if len(bestSellers) > 0 {
			dataSummary.WriteString("\n暢銷產品 Top 10：\n")
			for i, bs := range bestSellers {
				dataSummary.WriteString(fmt.Sprintf("  %d. %s | 銷量: %d | 營收: %.2f | 訂單數: %d\n",
					i+1, bs.ProductName, bs.TotalQuantity, bs.TotalRevenue, bs.OrderCount))
			}
		} else {
			dataSummary.WriteString("暫無銷售數據。\n")
		}

		// Slow movers — products with 0 or very low sales
		var slowMovers []models.Product
		database.DB.Raw(`
			SELECT p.* FROM products p
			WHERE p.tenant_id = ? AND p.trashed_at IS NULL AND p.status = 'active'
			AND p.id NOT IN (
				SELECT DISTINCT oi.product_id FROM order_items oi
				JOIN orders o ON o.id = oi.order_id
				WHERE o.tenant_id = ? AND o.trashed_at IS NULL AND o.status != 'quotation'
				AND o.created_at >= ?
			)
			LIMIT 10
		`, tenantID, tenantID, time.Now().AddDate(0, -3, 0)).Scan(&slowMovers)
		if len(slowMovers) > 0 {
			dataSummary.WriteString(fmt.Sprintf("\n滯銷產品（近3個月無銷售，共 %d 筆）：\n", len(slowMovers)))
			for i, p := range slowMovers {
				dataSummary.WriteString(fmt.Sprintf("  %d. %s (SKU: %s) | 庫存: %d | 價格: %.2f\n",
					i+1, p.Name, p.SKU, p.StockQuantity, p.Price))
			}
		}

	// ======================== Business Goals — business_goals ========================
	case "business_goals":
		statusFilter := strings.TrimSpace(query)
		var goals []models.BusinessGoal
		db := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)
		if statusFilter != "" && statusFilter != "all" {
			db = db.Where("status = ?", statusFilter)
		} else {
			db = db.Where("status = ?", "active")
		}
		db.Order("priority DESC, end_date ASC").Limit(limit).Find(&goals)

		if len(goals) == 0 {
			dataSummary.WriteString("\n\n【業務目標】\n目前沒有業務目標。你可以請我幫你建立一個新的業務目標。\n")
			break
		}

		now := utils.NowInTenantTimezone(tenantID)
		dataSummary.WriteString(fmt.Sprintf("\n\n【業務目標（共 %d 個）】\n", len(goals)))
		for i, g := range goals {
			// Recalculate current value from real data for trackable metrics
			currentVal := calculateGoalCurrentValue(tenantID, g.MetricType, g.TargetEntityID, g.StartDate, g.EndDate)
			if currentVal != g.CurrentValue {
				g.CurrentValue = currentVal
				// Update in background
				go database.DB.Model(&models.BusinessGoal{}).Where("id = ?", g.ID).Update("current_value", currentVal)
			}
			pct := g.ProgressPercent()

			// Days remaining
			daysLeft := int(g.EndDate.Sub(now).Hours() / 24)
			daysStatus := ""
			if daysLeft < 0 {
				daysStatus = "（已逾期）"
			} else if daysLeft == 0 {
				daysStatus = "（今天到期）"
			} else {
				daysStatus = fmt.Sprintf("（剩餘 %d 天）", daysLeft)
			}

			priorityLabel := map[string]string{"high": "🔴高", "medium": "🟡中", "low": "🟢低"}[g.Priority]
			if priorityLabel == "" {
				priorityLabel = g.Priority
			}

			dataSummary.WriteString(fmt.Sprintf("%d. %s [%s]\n", i+1, g.Title, priorityLabel))
			dataSummary.WriteString(fmt.Sprintf("   指標: %s | 目標: %.0f %s | 目前: %.0f %s | 進度: %.1f%%\n",
				translateMetricType(g.MetricType), g.TargetValue, g.Unit, g.CurrentValue, g.Unit, pct))
			dataSummary.WriteString(fmt.Sprintf("   期間: %s ~ %s %s\n",
				g.StartDate.Format("2006-01-02"), g.EndDate.Format("2006-01-02"), daysStatus))

			// Progress bar visualization
			barLen := 20
			filled := int(pct / 100 * float64(barLen))
			if filled > barLen {
				filled = barLen
			}
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)
			dataSummary.WriteString(fmt.Sprintf("   [%s] %.1f%%\n", bar, pct))
		}

	// ======================== Business Goals — analyze_business_goal ========================
	case "analyze_business_goal":
		searchTerm := strings.TrimSpace(query)
		if searchTerm == "" {
			dataSummary.WriteString("\n\n【業務目標分析】\n請提供要分析的業務目標名稱或關鍵字。\n")
			break
		}

		var goal models.BusinessGoal
		searchPattern := "%" + searchTerm + "%"
		if err := database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND title ILIKE ?", tenantID, searchPattern).
			Order("created_at DESC").
			First(&goal).Error; err != nil {
			dataSummary.WriteString(fmt.Sprintf("\n\n【業務目標分析】\n找不到名稱包含「%s」的業務目標。\n", searchTerm))
			break
		}

		now := utils.NowInTenantTimezone(tenantID)

		// Recalculate current value from real data
		currentVal := calculateGoalCurrentValue(tenantID, goal.MetricType, goal.TargetEntityID, goal.StartDate, goal.EndDate)
		if currentVal != goal.CurrentValue {
			goal.CurrentValue = currentVal
			go database.DB.Model(&models.BusinessGoal{}).Where("id = ?", goal.ID).Update("current_value", currentVal)
		}
		pct := goal.ProgressPercent()

		// Time analysis
		totalDays := goal.EndDate.Sub(goal.StartDate).Hours() / 24
		elapsedDays := now.Sub(goal.StartDate).Hours() / 24
		daysLeft := goal.EndDate.Sub(now).Hours() / 24
		timeProgressPct := 0.0
		if totalDays > 0 {
			timeProgressPct = (elapsedDays / totalDays) * 100
			if timeProgressPct > 100 {
				timeProgressPct = 100
			}
		}

		dataSummary.WriteString(fmt.Sprintf("\n\n【業務目標深度分析 — %s】\n", goal.Title))
		dataSummary.WriteString(fmt.Sprintf("描述: %s\n", goal.Description))
		dataSummary.WriteString(fmt.Sprintf("指標類型: %s\n", translateMetricType(goal.MetricType)))
		dataSummary.WriteString(fmt.Sprintf("目標值: %.0f %s\n", goal.TargetValue, goal.Unit))
		dataSummary.WriteString(fmt.Sprintf("目前值: %.0f %s\n", goal.CurrentValue, goal.Unit))
		dataSummary.WriteString(fmt.Sprintf("達成率: %.1f%%\n", pct))
		dataSummary.WriteString(fmt.Sprintf("期間: %s ~ %s\n", goal.StartDate.Format("2006-01-02"), goal.EndDate.Format("2006-01-02")))
		dataSummary.WriteString(fmt.Sprintf("總天數: %.0f 天 | 已過: %.0f 天 | 剩餘: %.0f 天\n", totalDays, elapsedDays, daysLeft))
		dataSummary.WriteString(fmt.Sprintf("時間進度: %.1f%% | 目標進度: %.1f%%\n", timeProgressPct, pct))

		// On-track analysis
		if totalDays > 0 && elapsedDays > 0 {
			expectedProgress := (elapsedDays / totalDays) * goal.TargetValue
			gap := goal.CurrentValue - expectedProgress
			if gap >= 0 {
				dataSummary.WriteString(fmt.Sprintf("\n📈 進度評估: 超前預期（領先 %.0f %s）\n", gap, goal.Unit))
			} else {
				dataSummary.WriteString(fmt.Sprintf("\n📉 進度評估: 落後預期（差距 %.0f %s）\n", -gap, goal.Unit))
			}

			// Required daily rate to meet goal
			remaining := goal.TargetValue - goal.CurrentValue
			if daysLeft > 0 && remaining > 0 {
				dailyRequired := remaining / daysLeft
				dataSummary.WriteString(fmt.Sprintf("剩餘需完成: %.0f %s\n", remaining, goal.Unit))
				dataSummary.WriteString(fmt.Sprintf("每日需達成: %.1f %s/天\n", dailyRequired, goal.Unit))

				// Current daily rate
				if elapsedDays > 0 {
					currentDailyRate := goal.CurrentValue / elapsedDays
					dataSummary.WriteString(fmt.Sprintf("目前日均: %.1f %s/天\n", currentDailyRate, goal.Unit))

					if currentDailyRate >= dailyRequired {
						dataSummary.WriteString("✅ 按目前速度可如期達成目標\n")
					} else {
						growthNeeded := ((dailyRequired / currentDailyRate) - 1) * 100
						dataSummary.WriteString(fmt.Sprintf("⚠️ 需提升日均 %.0f%% 才能如期達成目標\n", growthNeeded))
					}
				}
			} else if remaining <= 0 {
				dataSummary.WriteString("🎉 目標已達成！\n")
			}
		}

		// Recent tracking records
		var trackings []models.BusinessGoalTracking
		database.DB.Where("goal_id = ?", goal.ID).Order("tracked_at DESC").Limit(5).Find(&trackings)
		if len(trackings) > 0 {
			dataSummary.WriteString("\n最近追蹤記錄:\n")
			for i, t := range trackings {
				dataSummary.WriteString(fmt.Sprintf("  %d. %s | 值: %.0f | 變化: %+.0f | 進度: %.1f%%\n",
					i+1, t.TrackedAt.Format("2006-01-02"), t.TrackedValue, t.DeltaValue, t.ProgressPct))
			}
		}

	// ======================== Accounting / Financial ========================
	case "latest_transactions":
		// 獲取最新收入
		var incomes []models.Income
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).
			Order("income_date DESC, created_at DESC").
			Limit(limit).
			Find(&incomes)
		// 獲取最新支出
		var expenses []models.Expense
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).
			Order("expense_date DESC, created_at DESC").
			Limit(limit).
			Find(&expenses)

		if len(incomes) > 0 || len(expenses) > 0 {
			dataSummary.WriteString("\n\n【最新交易記錄】\n")
			if len(incomes) > 0 {
				dataSummary.WriteString("--- 收入 ---\n")
				for i, inc := range incomes {
					dataSummary.WriteString(fmt.Sprintf("%d. %s | 金額: %.2f | 類別: %s | 狀態: %s | 日期: %s\n",
						i+1, inc.Description, inc.Amount, inc.Category, inc.Status, inc.IncomeDate.Format("2006-01-02")))
				}
			}
			if len(expenses) > 0 {
				dataSummary.WriteString("--- 支出 ---\n")
				for i, exp := range expenses {
					dataSummary.WriteString(fmt.Sprintf("%d. %s | 金額: %.2f | 類別: %s | 狀態: %s | 日期: %s\n",
						i+1, exp.Description, exp.Amount, exp.Category, exp.Status, exp.ExpenseDate.Format("2006-01-02")))
				}
			}
		} else {
			dataSummary.WriteString("\n\n【最新交易記錄】\n目前沒有任何交易記錄。\n")
		}

	case "income_summary":
		now := time.Now()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

		var totalIncome float64
		database.DB.Model(&models.Income{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND income_date >= ? AND income_date <= ?",
				tenantID, startOfMonth, endOfMonth).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&totalIncome)

		type CategorySum struct {
			Category string  `json:"category"`
			Total    float64 `json:"total"`
		}
		var byCategory []CategorySum
		database.DB.Model(&models.Income{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND income_date >= ? AND income_date <= ?",
				tenantID, startOfMonth, endOfMonth).
			Select("COALESCE(category, income_type) as category, SUM(amount) as total").
			Group("COALESCE(category, income_type)").
			Scan(&byCategory)

		dataSummary.WriteString(fmt.Sprintf("\n\n【收入總結】（%s 至 %s）\n", startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")))
		dataSummary.WriteString(fmt.Sprintf("本月總收入: %.2f\n", totalIncome))
		if len(byCategory) > 0 {
			dataSummary.WriteString("按類別:\n")
			for _, cat := range byCategory {
				dataSummary.WriteString(fmt.Sprintf("  - %s: %.2f\n", cat.Category, cat.Total))
			}
		}

	case "expense_summary":
		now := time.Now()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

		var totalExpense float64
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?",
				tenantID, startOfMonth, endOfMonth).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&totalExpense)

		type CategorySum struct {
			Category string  `json:"category"`
			Total    float64 `json:"total"`
		}
		var byCategory []CategorySum
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?",
				tenantID, startOfMonth, endOfMonth).
			Select("category, SUM(amount) as total").
			Group("category").
			Scan(&byCategory)

		dataSummary.WriteString(fmt.Sprintf("\n\n【支出總結】（%s 至 %s）\n", startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")))
		dataSummary.WriteString(fmt.Sprintf("本月總支出: %.2f\n", totalExpense))
		if len(byCategory) > 0 {
			dataSummary.WriteString("按類別:\n")
			for _, cat := range byCategory {
				dataSummary.WriteString(fmt.Sprintf("  - %s: %.2f\n", cat.Category, cat.Total))
			}
		}

	case "accounts_receivable":
		var invoices []models.Invoice
		database.DB.Where("tenant_id = ? AND status IN (?, ?)", tenantID, "pending", "partial").
			Preload("Customer").
			Order("due_date ASC").
			Limit(limit).
			Find(&invoices)

		var totalReceivable float64
		database.DB.Model(&models.Invoice{}).
			Where("tenant_id = ? AND status IN (?, ?)", tenantID, "pending", "partial").
			Select("COALESCE(SUM(total_amount - paid_amount), 0)").
			Scan(&totalReceivable)

		dataSummary.WriteString("\n\n【應收帳款】\n")
		dataSummary.WriteString(fmt.Sprintf("應收總額: %.2f\n", totalReceivable))
		if len(invoices) > 0 {
			dataSummary.WriteString(fmt.Sprintf("未收發票數: %d\n", len(invoices)))
			for i, inv := range invoices {
				customerName := "未知客戶"
				if inv.Customer != nil {
					customerName = inv.Customer.Name
				}
				outstanding := inv.TotalAmount - inv.PaidAmount
				dataSummary.WriteString(fmt.Sprintf("%d. 發票 #%s | 客戶: %s | 總額: %.2f | 已付: %.2f | 未收: %.2f | 到期: %s | 狀態: %s\n",
					i+1, inv.InvoiceNumber, customerName, inv.TotalAmount, inv.PaidAmount, outstanding, inv.DueDate.Format("2006-01-02"), inv.Status))
			}
		} else {
			dataSummary.WriteString("目前沒有未收帳款。\n")
		}

	case "accounts_payable":
		var expenses []models.Expense
		database.DB.Where("tenant_id = ? AND trashed_at IS NULL AND status = ?", tenantID, "pending").
			Order("expense_date ASC").
			Limit(limit).
			Find(&expenses)

		var totalPayable float64
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = ?", tenantID, "pending").
			Select("COALESCE(SUM(amount), 0)").
			Scan(&totalPayable)

		dataSummary.WriteString("\n\n【應付帳款】\n")
		dataSummary.WriteString(fmt.Sprintf("應付總額: %.2f\n", totalPayable))
		if len(expenses) > 0 {
			dataSummary.WriteString(fmt.Sprintf("待付款項數: %d\n", len(expenses)))
			for i, exp := range expenses {
				dataSummary.WriteString(fmt.Sprintf("%d. %s | 金額: %.2f | 類別: %s | 日期: %s\n",
					i+1, exp.Description, exp.Amount, exp.Category, exp.ExpenseDate.Format("2006-01-02")))
			}
		} else {
			dataSummary.WriteString("目前沒有待付帳款。\n")
		}

	case "profit_loss":
		now := time.Now()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

		var totalIncome float64
		database.DB.Model(&models.Income{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND income_date >= ? AND income_date <= ?",
				tenantID, startOfMonth, endOfMonth).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&totalIncome)

		var totalExpense float64
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?",
				tenantID, startOfMonth, endOfMonth).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&totalExpense)

		netProfit := totalIncome - totalExpense

		dataSummary.WriteString(fmt.Sprintf("\n\n【損益報表】（%s 至 %s）\n", startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")))
		dataSummary.WriteString(fmt.Sprintf("總收入: %.2f\n", totalIncome))
		dataSummary.WriteString(fmt.Sprintf("總支出: %.2f\n", totalExpense))
		dataSummary.WriteString(fmt.Sprintf("淨利潤: %.2f\n", netProfit))
		if netProfit > 0 {
			dataSummary.WriteString("狀態: 盈利\n")
		} else if netProfit < 0 {
			dataSummary.WriteString("狀態: 虧損\n")
		} else {
			dataSummary.WriteString("狀態: 收支平衡\n")
		}

	case "balance_sheet":
		// 應收帳款
		var accountsReceivable float64
		database.DB.Model(&models.Invoice{}).
			Where("tenant_id = ? AND status IN (?, ?)", tenantID, "pending", "partial").
			Select("COALESCE(SUM(total_amount - paid_amount), 0)").
			Scan(&accountsReceivable)

		// 應付帳款
		var accountsPayable float64
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = ?", tenantID, "pending").
			Select("COALESCE(SUM(amount), 0)").
			Scan(&accountsPayable)

		// 累計收入
		var totalIncome float64
		database.DB.Model(&models.Income{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed'", tenantID).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&totalIncome)

		// 累計支出
		var totalExpense float64
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed'", tenantID).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&totalExpense)

		retainedEarnings := totalIncome - totalExpense

		dataSummary.WriteString("\n\n【資產負債表（簡化版）】\n")
		dataSummary.WriteString("--- 資產 ---\n")
		dataSummary.WriteString(fmt.Sprintf("  應收帳款: %.2f\n", accountsReceivable))
		dataSummary.WriteString("--- 負債 ---\n")
		dataSummary.WriteString(fmt.Sprintf("  應付帳款: %.2f\n", accountsPayable))
		dataSummary.WriteString("--- 股東權益 ---\n")
		dataSummary.WriteString(fmt.Sprintf("  保留盈餘: %.2f\n", retainedEarnings))
		dataSummary.WriteString(fmt.Sprintf("  （累計收入: %.2f, 累計支出: %.2f）\n", totalIncome, totalExpense))

	case "cash_flow":
		now := time.Now()
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		endOfMonth := startOfMonth.AddDate(0, 1, 0).Add(-time.Second)

		var cashInflow float64
		database.DB.Model(&models.Income{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND income_date >= ? AND income_date <= ?",
				tenantID, startOfMonth, endOfMonth).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&cashInflow)

		var cashOutflow float64
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND trashed_at IS NULL AND status = 'confirmed' AND expense_date >= ? AND expense_date <= ?",
				tenantID, startOfMonth, endOfMonth).
			Select("COALESCE(SUM(amount), 0)").
			Scan(&cashOutflow)

		netCashFlow := cashInflow - cashOutflow

		dataSummary.WriteString(fmt.Sprintf("\n\n【現金流量表】（%s 至 %s）\n", startOfMonth.Format("2006-01-02"), endOfMonth.Format("2006-01-02")))
		dataSummary.WriteString(fmt.Sprintf("現金收入: %.2f\n", cashInflow))
		dataSummary.WriteString(fmt.Sprintf("現金支出: %.2f\n", cashOutflow))
		dataSummary.WriteString(fmt.Sprintf("淨現金流: %.2f\n", netCashFlow))
		if netCashFlow > 0 {
			dataSummary.WriteString("狀態: 正現金流\n")
		} else if netCashFlow < 0 {
			dataSummary.WriteString("狀態: 負現金流\n")
		} else {
			dataSummary.WriteString("狀態: 現金流平衡\n")
		}

	// ======================== Company / tenant info ========================
	case "company_info":
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			dataSummary.WriteString("\n\n【公司資訊】\n無法取得公司資訊。\n")
			break
		}

		var enterprise models.Enterprise
		database.DB.Where("tenant_id = ?", tenantID).First(&enterprise)

		dataSummary.WriteString("\n\n【公司資訊】\n")
		dataSummary.WriteString(fmt.Sprintf("公司名稱: %s\n", enterprise.Name))
		dataSummary.WriteString(fmt.Sprintf("方案: %s\n", tenant.Plan))
		dataSummary.WriteString(fmt.Sprintf("狀態: %s\n", tenant.Status))
		dataSummary.WriteString(fmt.Sprintf("子域名: %s\n", tenant.Subdomain))

		// Public site link
		publicSiteURL := baseURL + "/co/" + tenant.Subdomain + "/"
		dataSummary.WriteString(fmt.Sprintf("公開網站連結 (Public Site): %s\n", publicSiteURL))

		// Website config
		if tenant.WebsiteTheme != nil && *tenant.WebsiteTheme != "" {
			dataSummary.WriteString(fmt.Sprintf("網站主題: %s\n", *tenant.WebsiteTheme))
		}
		if tenant.WebsiteType != nil && *tenant.WebsiteType != "" {
			dataSummary.WriteString(fmt.Sprintf("網站類型: %s\n", *tenant.WebsiteType))
		}
		dataSummary.WriteString(fmt.Sprintf("網站啟用: %v\n", tenant.WebsiteEnabled))

		// Custom domain
		if cd, ok := tenant.ExtraFields["website_custom_domain"].(string); ok && cd != "" {
			dataSummary.WriteString(fmt.Sprintf("自訂域名: %s\n", cd))
			dataSummary.WriteString(fmt.Sprintf("自訂域名網站連結: https://%s/\n", cd))
		}

		// Enterprise extra info
		if enterprise.Domain != nil && *enterprise.Domain != "" {
			dataSummary.WriteString(fmt.Sprintf("企業域名: %s\n", *enterprise.Domain))
		}
		if enterprise.Address != nil && *enterprise.Address != "" {
			dataSummary.WriteString(fmt.Sprintf("地址: %s\n", *enterprise.Address))
		}

	}

	return dataSummary.String()
}

// convertLinksToHTML 將文本中的完整 URL 轉換為 HTML 鏈接
func convertLinksToHTML(text string) string {
	// 防止 regex 處理時 panic
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[convertLinksToHTML] panic recovered: %v", r)
		}
	}()

	if strings.TrimSpace(text) == "" {
		return text
	}

	normalizeAnchor := func(anchor string) string {
		if strings.TrimSpace(anchor) == "" {
			return anchor
		}
		// ensure target="_blank"
		targetRe := regexp.MustCompile(`(?i)\s+target\s*=\s*(['"][^'"]*['"])`)
		if targetRe.MatchString(anchor) {
			anchor = targetRe.ReplaceAllString(anchor, ` target="_blank"`)
		} else {
			idx := strings.Index(strings.ToLower(anchor), "<a")
			if idx >= 0 {
				anchor = anchor[:idx+2] + " target=\"_blank\"" + anchor[idx+2:]
			}
		}

		// ensure rel includes noopener noreferrer
		relRe := regexp.MustCompile(`(?i)\s+rel\s*=\s*["']([^"']*)["']`)
		if relRe.MatchString(anchor) {
			anchor = relRe.ReplaceAllStringFunc(anchor, func(m string) string {
				sub := relRe.FindStringSubmatch(m)
				if len(sub) < 2 {
					return m
				}
				current := sub[1]
				parts := strings.Fields(current)
				needNoopener := true
				needNoreferrer := true
				for _, p := range parts {
					if strings.EqualFold(p, "noopener") {
						needNoopener = false
					}
					if strings.EqualFold(p, "noreferrer") {
						needNoreferrer = false
					}
				}
				if needNoopener {
					parts = append(parts, "noopener")
				}
				if needNoreferrer {
					parts = append(parts, "noreferrer")
				}
				return ` rel="` + strings.Join(parts, " ") + `"`
			})
		} else {
			idx := strings.Index(strings.ToLower(anchor), "<a")
			if idx >= 0 {
				anchor = anchor[:idx+2] + " rel=\"noopener noreferrer\"" + anchor[idx+2:]
			}
		}

		return anchor
	}

	// 先把 LLM 回傳的 HTML-entity-escaped <a> 標籤還原回真正的 HTML
	// LLM 有時會輸出 &lt;a href="..."&gt;文字&lt;/a&gt; 而非 <a href="...">文字</a>
	escapedAnchorPattern := regexp.MustCompile(`&lt;a\s([^&]*(?:&[^l][^&]*)*)&gt;(.*?)&lt;/a&gt;`)
	text = escapedAnchorPattern.ReplaceAllStringFunc(text, func(match string) string {
		s := strings.ReplaceAll(match, "&lt;", "<")
		s = strings.ReplaceAll(s, "&gt;", ">")
		s = strings.ReplaceAll(s, "&amp;", "&")
		s = strings.ReplaceAll(s, "&quot;", `"`)
		return s
	})

	placeholders := []string{}
	wrapToken := func(html string) string {
		token := fmt.Sprintf("__VAI_LINK_TOKEN_%d__", len(placeholders))
		placeholders = append(placeholders, html)
		return token
	}

	// 先保護既有 <a> 標籤，避免重複 linkify
	anchorPattern := regexp.MustCompile(`(?is)<a\s+[^>]*>.*?</a>`)
	text = anchorPattern.ReplaceAllStringFunc(text, func(match string) string {
		return wrapToken(normalizeAnchor(match))
	})

	// 先處理 Markdown 連結 [label](url)
	markdownPattern := regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	text = markdownPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := markdownPattern.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		label := parts[1]
		link := parts[2]
		if parsedURL, err := url.Parse(link); err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			return wrapToken(fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`, link, label))
		}
		return match
	})

	// 再處理純文字 URL（避免把 Markdown 的 "](url)" 誤判進 URL）
	urlPattern := regexp.MustCompile(`(https?://[^\s<>"'\)\]]+)`)
	text = urlPattern.ReplaceAllStringFunc(text, func(match string) string {
		if parsedURL, err := url.Parse(match); err == nil && parsedURL.Scheme != "" && parsedURL.Host != "" {
			return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`, match, match)
		}
		return match
	})

	// 還原所有 placeholder
	for i, html := range placeholders {
		token := fmt.Sprintf("__VAI_LINK_TOKEN_%d__", i)
		text = strings.ReplaceAll(text, token, html)
	}

	return text
}

// parseIntentFromResponse 從 LLM 回應中解析意圖 JSON
func parseIntentFromResponse(responseText string) (*QueryIntent, string) {
	// 嘗試找到 JSON 格式的意圖標記
	// 格式: {"need_data": true, "intent": "latest_customers", "limit": 5}
	jsonStart := strings.Index(responseText, "{")
	jsonEnd := strings.LastIndex(responseText, "}")

	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, responseText
	}

	jsonStr := responseText[jsonStart : jsonEnd+1]
	var intent QueryIntent
	if err := json.Unmarshal([]byte(jsonStr), &intent); err != nil {
		return nil, responseText
	}

	// 提取非 JSON 部分的回應（如果有）
	otherText := strings.TrimSpace(responseText[:jsonStart] + responseText[jsonEnd+1:])

	return &intent, otherText
}

// ChatWithLLM 通過後端代理調用 LLM API
func ChatWithLLM(c *fiber.Ctx) error {
	// 防止 panic 導致整個系統崩潰
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ChatWithLLM] panic recovered: %v", r)
			c.Status(500).JSON(fiber.Map{"error": "AI 處理過程中發生內部錯誤，請稍後再試"})
		}
	}()

	cfg := config.Load()

	// 如果使用 Gemini，不需要 Endpoint（會自動構建）
	if cfg.LLM.Provider != "gemini" && cfg.LLM.Endpoint == "" {
		return c.Status(503).JSON(fiber.Map{"error": "LLM Endpoint 未配置"})
	}

	if cfg.LLM.Provider == "gemini" && cfg.LLM.APIKey == "" {
		return c.Status(503).JSON(fiber.Map{"error": "AI API Key 未配置"})
	}

	var req struct {
		Model          string       `json:"model"`
		Messages       []Message    `json:"messages"`
		Temperature    float64      `json:"temperature"`
		MaxTokens      int          `json:"max_tokens"`
		Attachments    []Attachment `json:"attachments"`
		ConversationID string       `json:"conversation_id"`
		WebSearch      bool         `json:"web_search"`
		PageContext    string       `json:"page_context"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Store page context (cms vs vai-chat) for navigate_to_page behavior
	if req.PageContext != "" {
		c.Locals("vai_page_context", req.PageContext)
	}

	// Parse conversation_id for backend message saving
	var convID *uuid.UUID
	if req.ConversationID != "" {
		if parsed, err := uuid.Parse(req.ConversationID); err == nil {
			convID = &parsed
		}
	}

	// Get user and tenant IDs for backend message saving
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	// 使用配置中的 model（如果請求中沒有指定）
	if req.Model == "" {
		req.Model = cfg.LLM.Model
	}

	// 系統提示詞：從數據庫讀取，如果沒有則使用默認值
	defaultPrompt := "你是 vAi，由 V-sys 開發的專業 AI 助手。你能回答任何問題——不論是業務數據查詢、一般知識、技術問題、日常生活、創意寫作或任何其他主題。你不會以「隱私」或「無法回答」為由拒絕合理的問題。\n" +
		"重要：你的名字是「vAi」，由 V-sys 開發。絕對不要提及 Google、Gemini、Bard 或任何其他 AI 供應商的名稱。如果用戶問你是誰或用什麼技術，只回答「我是 vAi，由 V-sys 開發的 AI 助手」。\n" +
		"你具有網路搜尋能力（透過 google_search 工具），可以搜尋最新的即時資訊、新聞、市場趨勢等。當用戶詢問需要即時資訊的問題（如新聞、最新消息、天氣、市場動態、時事等），請善用搜尋功能為用戶提供最新資訊。不要拒絕這類請求。"
	basePrompt := models.GetSystemSetting("ai_system_prompt", defaultPrompt)

	// 增強系統提示詞：添加數據查詢意圖識別指令（精簡版）
	dataQueryPrompt := `當用戶需要查詢業務數據、教學幫助、或執行快捷動作時，在回答末尾附加一個 JSON：
{"need_data": true, "intent": "<意圖>", "limit": <數量>, "query": "<用戶關鍵詞>", "params": {<參數>}}

一、查詢意圖（limit 最大 50，默認 5）：
latest_customers, top_spending_customers, search_customer, largest_order, latest_orders, recent_orders, latest_products, low_stock_products, latest_appointments, upcoming_appointments, available_rooms, available_equipments, available_vehicles, latest_services, search_service, latest_service_orders, search_service_order, latest_users, latest_service_staffs, departments, staff_shifts, holidays, latest_projects, latest_transactions, income_summary, expense_summary, accounts_receivable, accounts_payable, profit_loss, balance_sheet, cash_flow, total_statistics, dashboard_stats, help, search_order, search_product, latest_invoices, search_invoice, get_order_invoices, global_search, search_supplier, latest_purchase_orders, inventory_summary, upcoming_reminders, daily_report, monthly_report, leave_requests, latest_shipments, dining_queue, pos_daily_summary, customer_history, product_sales_analysis, business_goals

二、快捷動作意圖（需 params）：
- create_customer: params 可含 name(必填), phone, email, gender, address
- create_order: params 可含 customer_name/customer_phone(擇一識別客戶), items:[{product_name, quantity, unit_price}], notes
- create_service_order: params 可含 customer_name/customer_phone, items:[{service_name, quantity, unit_price}], service_date, notes
- create_product: params 可含 name(必填), sku, barcode, price, cost, unit, category, description, status(active/inactive 默認active)
- update_order_status: params 可含 order_number(必填), status(draft/confirmed/processing/completed/cancelled)
- create_appointment: params 可含 customer_name(必填), service_name, staff_name, start_time(YYYY-MM-DD HH:MM), end_time, notes
- create_invoice: params 可含 order_number(從訂單建立) 或 customer_name+amount(獨立建立), due_date, notes
- create_supplier: params 可含 name(必填), phone, email, address, status
- create_purchase_order: params 可含 supplier_name(必填), items:[{product_name, quantity, unit_price}], notes
- update_customer: params 可含 customer_name(必填,用於識別), phone, email, address, status
- update_product: params 可含 product_name(必填,用於識別), price, cost, stock_quantity, status, sku, barcode
- export_customers: params 無（返回導出鏈接）
- export_orders: params 無（返回導出鏈接）
- export_products: params 無（返回導出鏈接）
- quotation_to_order: params 可含 order_number(必填,報價單號)
- send_payment_link: params 可含 order_number(必填)
- create_reminder: params 可含 title(必填), description, remind_time(YYYY-MM-DD HH:MM)
- create_note: params 可含 title(必填), content, category
- approve_leave: params 可含 leave_id(必填)
- create_shipment: params 可含 order_number, recipient_name(必填), recipient_phone, recipient_address, tracking_number, notes
- create_business_goal: params 可含 title(必填), description, metric_type(order_count/revenue/customer_count/product_sales_qty/service_order_count/custom 默認custom), target_value(必填), unit, start_date(YYYY-MM-DD), end_date(YYYY-MM-DD), priority(low/medium/high 默認medium)

規則：
1. 不需要查詢數據時直接回答，不附加 JSON
2. "help" 意圖：用戶詢問如何使用系統功能時使用，query 填寫關鍵詞。不要自己生成任何教學URL
3. "search_customer" 意圖：用戶要查找特定客戶時使用，query 填寫客戶名字/電話/電郵
4. "search_order" 意圖：用戶要搜尋訂單時使用，query 填寫訂單號、客戶名或產品名
5. "search_product" 意圖：用戶要搜尋產品時使用，query 填寫產品名/SKU/條碼
6. "search_invoice" 意圖：用戶要搜尋發票時使用，query 填寫發票號或客戶名
7. "get_order_invoices" 意圖：用戶要查看某訂單的發票時使用，params 填 order_number
8. "search_supplier" 意圖：用戶要搜尋供應商時使用，query 填寫名稱/電話/電郵
9. "global_search" 意圖：用戶要跨實體搜尋時使用，query 填寫關鍵詞
10. "customer_history" 意圖：用戶要查看特定客戶的所有記錄時使用，query 填寫客戶名
11. "search_service_order" 意圖：用戶要搜尋服務單時使用，query 填寫服務單號、客戶名或服務名
12. 快捷動作：用戶要求新增/建立/修改記錄時使用，從用戶消息中提取 params。如果必填欄位缺失，先詢問用戶
13. limit=0 的意圖：help, available_rooms, available_equipments, available_vehicles, income_summary, expense_summary, profit_loss, balance_sheet, cash_flow, total_statistics, dashboard_stats, inventory_summary, daily_report, monthly_report, pos_daily_summary, product_sales_analysis
14. 回答要簡潔，先簡短回應用戶，再附 JSON
15. "business_goals" 意圖：用戶要查看業務目標進度時使用，可帶 status 參數(active/completed/failed/paused)。用戶要建立業務目標時使用 create_business_goal

示例：
用戶：「最新5個客人」→ 簡短回應 + {"need_data":true,"intent":"latest_customers","limit":5}
用戶：「查詢客戶王小明」→ 「我來幫您查找。」+ {"need_data":true,"intent":"search_customer","query":"王小明"}
用戶：「找電話0912345678的客戶」→ 「好的，幫您搜索。」+ {"need_data":true,"intent":"search_customer","query":"0912345678"}
用戶：「新增客戶張三，電話0987654321」→ 「好的，我來幫您建立客戶。」+ {"need_data":true,"intent":"create_customer","params":{"name":"張三","phone":"0987654321"}}
用戶：「幫我建一張訂單給王小明，買產品A 2個」→ 「好的，我來建立訂單。」+ {"need_data":true,"intent":"create_order","params":{"customer_name":"王小明","items":[{"product_name":"產品A","quantity":2}]}}
用戶：「新增一個商品叫蘋果汁，售價25元」→ 「好的，我來建立商品。」+ {"need_data":true,"intent":"create_product","params":{"name":"蘋果汁","price":25}}
用戶：「如何管理訂單？」→ 「我來幫您查找。」+ {"need_data":true,"intent":"help","query":"訂單"}
用戶：「搜尋訂單 ORD-001」→ 「好的，幫您搜尋。」+ {"need_data":true,"intent":"search_order","query":"ORD-001"}
用戶：「找包含產品A的訂單」→ 「好的，幫您搜尋。」+ {"need_data":true,"intent":"search_order","query":"產品A"}
用戶：「搜尋跟美容護理有關的服務單」→ 「好的，幫您搜尋。」+ {"need_data":true,"intent":"search_service_order","query":"美容護理"}
用戶：「今日報告」→ 「好的，我來查看今天的數據。」+ {"need_data":true,"intent":"daily_report"}
用戶：「把訂單ORD-001改為已完成」→ 「好的，我來更新。」+ {"need_data":true,"intent":"update_order_status","params":{"order_number":"ORD-001","status":"completed"}}
用戶：「幫我建立預約給客戶王小明，明天下午2點」→ 「好的，我來建立預約。」+ {"need_data":true,"intent":"create_appointment","params":{"customer_name":"王小明","start_time":"2026-02-19 14:00"}}
用戶：「查看業務目標」→ 「好的，我來查看目標進度。」+ {"need_data":true,"intent":"business_goals","limit":10}
用戶：「建立業務目標：本月訂單100張」→ 「好的，我來建立目標。」+ {"need_data":true,"intent":"create_business_goal","params":{"title":"本月訂單100張","metric_type":"order_count","target_value":100,"unit":"張","start_date":"2026-02-01","end_date":"2026-02-28"}}
用戶：「你好」→ 直接回答，不附 JSON`

	// 構建用戶上下文信息
	userContext := ""
	if userID != uuid.Nil && tenantID != uuid.Nil {
		var user models.User
		if err := database.DB.Where("id = ? AND tenant_id = ?", userID, tenantID).First(&user).Error; err == nil {
			userInfoParts := []string{
				fmt.Sprintf("用戶目前正處於 vWork 系統中"),
			}

			if user.Name != "" {
				userInfoParts = append(userInfoParts, fmt.Sprintf("用戶名：%s", user.Name))
			}

			if user.Email != "" {
				userInfoParts = append(userInfoParts, fmt.Sprintf("電子郵件：%s", user.Email))
			}

			if user.BirthDate != nil {
				userInfoParts = append(userInfoParts, fmt.Sprintf("出生日期：%s", user.BirthDate.Format("2006-01-02")))
			}

			if len(userInfoParts) > 0 {
				userContext = "\n\n" + strings.Join(userInfoParts, "\n")
			}
		}

		// 查詢企業和行業信息
		var tenant models.Tenant
		if err := database.DB.Preload("IndustryTemplate").Where("id = ?", tenantID).First(&tenant).Error; err == nil {
			businessInfoParts := []string{}

			// 行業信息
			if tenant.IndustryTemplate != nil {
				businessInfoParts = append(businessInfoParts, fmt.Sprintf("行業：%s", tenant.IndustryTemplate.Name))
			}

			// 查詢企業信息
			var enterprise models.Enterprise
			if err := database.DB.Where("tenant_id = ?", tenantID).First(&enterprise).Error; err == nil {
				if enterprise.Name != "" {
					businessInfoParts = append(businessInfoParts, fmt.Sprintf("企業名稱：%s", enterprise.Name))
				}
			}

			if len(businessInfoParts) > 0 {
				userContext = userContext + "\n" + strings.Join(businessInfoParts, "\n")
			}
		}
	}

	// 構建增強系統提示詞（基礎 prompt + 用戶上下文）
	enhancedSystemPrompt := basePrompt + userContext

	// Gemini: 加入導航功能說明，讓模型知道可以使用 navigate_to_page 工具
	if cfg.LLM.Provider == "gemini" {
		enhancedSystemPrompt = enhancedSystemPrompt + "\n\n" +
			"你具有頁面導航功能。當用戶要求前往、打開、查看某個頁面時（例如「到訂單頁」、「去客戶頁面」、「打開儀表板」、「我要看庫存」），" +
			"請務必使用 navigate_to_page 函數來導航。不要只是文字回覆告訴用戶怎麼去，而是直接調用函數幫用戶導航。" +
			"支援的頁面包括：dashboard, customers, orders, service-orders, purchase-orders, products, projects, suppliers, invoices, accounting, pos, inventory, appointments, staff-shifts, messages, notifications, vai-chat, billing, personal-data, vcoins。"
	}

	// 如果有租戶 ID，添加數據查詢功能說明（僅 OpenAI 路徑需要，Gemini 使用 Function Calling）
	if tenantID != uuid.Nil && cfg.LLM.Provider != "gemini" {
		enhancedSystemPrompt = enhancedSystemPrompt + "\n\n" + dataQueryPrompt
	}

	var reqBody []byte
	var apiURL string
	var err error

	if cfg.LLM.Provider == "gemini" {
		// Gemini API 格式 (使用 Function Calling)
		// 構建 Gemini API URL
		model := req.Model
		if model == "" {
			model = cfg.LLM.Model
		}
		// 如果模型名称是旧的，自动更新为最新版本
		if model == "gemini-pro" || model == "gemini-1.5-flash" || model == "gemini-1.5-pro" || model == "gemini-2.0-flash-exp" {
			model = "gemini-2.5-flash"
		}
		// 确保模型名称不包含 -latest 后缀（某些 API 版本不支持）
		model = strings.TrimSuffix(model, "-latest")
		// 使用 v1beta API 版本以支持 Function Calling
		apiURL = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

		// 構建 Gemini 請求格式
		// 將 messages 轉換為 Gemini 格式 (不再需要 fake user/model pair for system prompt)
		contents := []map[string]interface{}{}

		// 轉換用戶消息
		for i, msg := range req.Messages {
			role := "user"
			if msg.Role == "assistant" {
				role = "model"
			}

			// Build parts for this message
			parts := []map[string]interface{}{
				{"text": msg.Content},
			}

			// If this is the last user message and there are attachments, add inlineData parts
			if i == len(req.Messages)-1 && msg.Role == "user" && len(req.Attachments) > 0 {
				for _, att := range req.Attachments {
					if att.Data != "" && att.MimeType != "" {
						parts = append(parts, map[string]interface{}{
							"inlineData": map[string]interface{}{
								"mimeType": att.MimeType,
								"data":     att.Data,
							},
						})
					}
				}
			}

			contents = append(contents, map[string]interface{}{
				"role":  role,
				"parts": parts,
			})
		}

		geminiReq := map[string]interface{}{
			"contents": contents,
			"systemInstruction": map[string]interface{}{
				"parts": []map[string]interface{}{
					{"text": enhancedSystemPrompt},
				},
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

		// Gemini does NOT allow google_search (built-in tool) and
		// functionDeclarations (Function Calling) in the same request.
		// Use functionDeclarations when available; fall back to google_search only.
		var funcDecls []map[string]interface{}
		if tenantID != uuid.Nil {
			funcDecls = buildGeminiFunctionDeclarations()
		} else {
			funcDecls = buildNavigationOnlyDeclarations()
		}
		if len(funcDecls) > 0 {
			geminiReq["tools"] = []interface{}{
				map[string]interface{}{
					"functionDeclarations": funcDecls,
				},
			}
			geminiReq["toolConfig"] = map[string]interface{}{
				"functionCallingConfig": map[string]interface{}{
					"mode": "AUTO",
				},
			}
			log.Printf("[ChatWithLLM] Registered %d function declarations as tools (google_search disabled to avoid conflict)", len(funcDecls))
		} else {
			geminiReq["tools"] = []interface{}{
				map[string]interface{}{
					"google_search": map[string]interface{}{},
				},
			}
			log.Printf("[ChatWithLLM] No function declarations — google_search only")
		}

		if req.MaxTokens <= 0 {
			req.MaxTokens = 4096
		}
		if req.Temperature <= 0 {
			req.Temperature = 0.7
		}
		genCfg := map[string]interface{}{
			"temperature":     req.Temperature,
			"maxOutputTokens": req.MaxTokens,
		}
		// Gemini 2.5 models have thinking enabled by default, which consumes maxOutputTokens
		// and can cause empty responses. Disable thinking by default for reliability.
		if strings.HasPrefix(model, "gemini-2.5") {
			genCfg["thinkingConfig"] = map[string]interface{}{
				"thinkingBudget": 0,
			}
		}
		geminiReq["generationConfig"] = genCfg

		reqBody, err = json.Marshal(geminiReq)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to build AI request"})
		}
	} else {
		// OpenAI 格式（原有邏輯）
		// 添加系統提示詞到消息開頭
		messages := []Message{
			{Role: "system", Content: enhancedSystemPrompt},
		}
		messages = append(messages, req.Messages...)

		llmReq := map[string]interface{}{
			"model":       req.Model,
			"messages":    messages,
			"temperature": req.Temperature,
			"max_tokens":  req.MaxTokens,
		}

		reqBody, err = json.Marshal(llmReq)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to build request"})
		}

		apiURL = cfg.LLM.Endpoint
	}

	// Save user message to DB as backend safety net (mobile clients don't call POST /messages)
	if convID != nil && tenantID != uuid.Nil && userID != uuid.Nil {
		// Extract the last user message content from the request
		var userContent string
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == "user" {
				userContent = req.Messages[i].Content
				break
			}
		}
		if userContent != "" {
			go saveAIChatUserMessage(tenantID, userID, convID, userContent)
		}
	}

	// 創建 HTTP 請求
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create request"})
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// OpenAI 使用 Bearer token，Gemini 使用 query parameter（已在 URL 中）
	if cfg.LLM.Provider != "gemini" && cfg.LLM.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.LLM.APIKey)
	}

	// 發送請求
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to call LLM API: " + err.Error()})
	}
	defer resp.Body.Close()

	// 讀取響應
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to read LLM response"})
	}

	if resp.StatusCode != 200 {
		return c.Status(resp.StatusCode).JSON(fiber.Map{"error": string(body)})
	}

	// 解析響應
	var llmResponse map[string]interface{}
	if err := json.Unmarshal(body, &llmResponse); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse LLM response"})
	}

	// ========== Gemini Function Calling 路徑 ==========
	if cfg.LLM.Provider == "gemini" {
		// Debug: log raw Gemini response (truncated)
		rawStr := string(body)
		if len(rawStr) > 500 {
			rawStr = rawStr[:500] + "..."
		}
		log.Printf("[ChatWithLLM] Gemini raw response (truncated): %s", rawStr)

		// Extract the first candidate's content parts
		var responseParts []interface{}
		if candidates, ok := llmResponse["candidates"].([]interface{}); ok && len(candidates) > 0 {
			if candidate, ok := candidates[0].(map[string]interface{}); ok {
				if content, ok := candidate["content"].(map[string]interface{}); ok {
					if parts, ok := content["parts"].([]interface{}); ok {
						responseParts = parts
					}
				}
			}
		}

		// Check if any part contains a functionCall
		var functionCallPart map[string]interface{}
		var textResponse string
		for _, p := range responseParts {
			part, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			// Skip thinking parts from Gemini 2.5 models (thought: true)
			if thought, ok := part["thought"].(bool); ok && thought {
				continue
			}
			if fc, ok := part["functionCall"]; ok {
				if fcMap, ok := fc.(map[string]interface{}); ok {
					functionCallPart = fcMap
				}
			}
			if text, ok := part["text"].(string); ok {
				textResponse += text
			}
		}

		// If no function call, return text response directly
		if functionCallPart == nil {
			log.Printf("[ChatWithLLM] Gemini: no function call. textResponse length=%d, first100=%q", len(textResponse), textResponse[:min(len(textResponse), 100)])
			// Guard against empty response (e.g. safety filter, empty candidates, RECITATION finish reason)
			if strings.TrimSpace(textResponse) == "" {
				// Log finish reason and prompt feedback for debugging
				if candidates, ok := llmResponse["candidates"].([]interface{}); ok && len(candidates) > 0 {
					if candidate, ok := candidates[0].(map[string]interface{}); ok {
						log.Printf("[ChatWithLLM] Gemini empty response - finishReason: %v, safetyRatings: %v", candidate["finishReason"], candidate["safetyRatings"])
					}
				} else {
					log.Printf("[ChatWithLLM] Gemini empty response - no candidates. promptFeedback: %v", llmResponse["promptFeedback"])
				}
				log.Printf("[ChatWithLLM] Gemini returned empty text with no function call, returning error")
				return c.Status(500).JSON(fiber.Map{"error": "AI 未返回有效內容，請重新嘗試"})
			}
			textResponse = convertLinksToHTML(textResponse)
			textResponse = sanitizeBrandNames(textResponse)

			// Extract grounding metadata (web search sources) if available.
			// Always check — google_search is always registered as a tool.
			var groundingMeta map[string]interface{}
			if candidates, ok := llmResponse["candidates"].([]interface{}); ok && len(candidates) > 0 {
				if candidate, ok := candidates[0].(map[string]interface{}); ok {
					if gm, ok := candidate["groundingMetadata"].(map[string]interface{}); ok {
						groundingMeta = extractGroundingMetadata(gm)
						log.Printf("[ChatWithLLM] Extracted grounding metadata with %d sources", len(groundingMeta))
					}
				}
			}

			// Backend save: persist AI message as double insurance
			var textExtraData map[string]interface{}
			if groundingMeta != nil {
				textExtraData = map[string]interface{}{"grounding": groundingMeta}
			}
			go saveAIChatMessage(tenantID, userID, convID, textResponse, textExtraData)

			responsePayload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": textResponse,
						},
					},
				},
			}
			if groundingMeta != nil {
				responsePayload["grounding"] = groundingMeta
			}
			return c.JSON(responsePayload)
		}

		// Function call detected — execute it
		fnName, _ := functionCallPart["name"].(string)
		fnArgs := map[string]interface{}{}
		if args, ok := functionCallPart["args"].(map[string]interface{}); ok {
			fnArgs = args
		}

		log.Printf("[ChatWithLLM] Gemini function call: %s, args: %v", fnName, fnArgs)

		var dataContext string
		// navigate_to_page and generate_document are actions that don't strictly require tenant data
		if fnName == "navigate_to_page" || fnName == "generate_document" || fnName == "generate_image" || fnName == "generate_video" {
			log.Printf("[ChatWithLLM] Executing function %s (always-run path, tenantID=%s)", fnName, tenantID.String())
			dataContext = executeFunctionCall(c, tenantID, fnName, fnArgs)
			log.Printf("[ChatWithLLM] After executeFunctionCall for %s: vai_doc_info=%v", fnName, c.Locals("vai_doc_info"))
		} else if tenantID != uuid.Nil {
			dataContext = executeFunctionCall(c, tenantID, fnName, fnArgs)
		}

		// Build the functionResponse and send it back to Gemini for the final answer.
		// The conversation so far: original contents + model's functionCall + our functionResponse.
		model := req.Model
		if model == "" {
			model = cfg.LLM.Model
		}
		if model == "gemini-pro" || model == "gemini-1.5-flash" || model == "gemini-1.5-pro" || model == "gemini-2.0-flash-exp" {
			model = "gemini-2.5-flash"
		}
		model = strings.TrimSuffix(model, "-latest")
		secondApiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, cfg.LLM.APIKey)

		// Rebuild contents from the first request
		secondContents := []map[string]interface{}{}
		// Re-add the original user messages
		for i, msg := range req.Messages {
			role := "user"
			if msg.Role == "assistant" {
				role = "model"
			}
			parts := []map[string]interface{}{
				{"text": msg.Content},
			}
			if i == len(req.Messages)-1 && msg.Role == "user" && len(req.Attachments) > 0 {
				for _, att := range req.Attachments {
					if att.Data != "" && att.MimeType != "" {
						parts = append(parts, map[string]interface{}{
							"inlineData": map[string]interface{}{
								"mimeType": att.MimeType,
								"data":     att.Data,
							},
						})
					}
				}
			}
			secondContents = append(secondContents, map[string]interface{}{
				"role":  role,
				"parts": parts,
			})
		}

		// Add the model's function call response
		secondContents = append(secondContents, map[string]interface{}{
			"role": "model",
			"parts": []map[string]interface{}{
				{
					"functionCall": map[string]interface{}{
						"name": fnName,
						"args": fnArgs,
					},
				},
			},
		})

		// Add the function response with the data we fetched
		functionResponseContent := dataContext
		if functionResponseContent == "" {
			functionResponseContent = "No data found."
		}
		secondContents = append(secondContents, map[string]interface{}{
			"role": "user",
			"parts": []map[string]interface{}{
				{
					"functionResponse": map[string]interface{}{
						"name": fnName,
						"response": map[string]interface{}{
							"content": functionResponseContent,
						},
					},
				},
			},
		})

		secondGeminiReq := map[string]interface{}{
			"contents": secondContents,
			"systemInstruction": map[string]interface{}{
				"parts": []map[string]interface{}{
					{"text": basePrompt + userContext + "\n\n請基於函數返回的數據回答用戶的問題。只使用提供的鏈接和數據，不要自行編造任何 URL 或鏈接。"},
				},
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

		// For action-type function calls (generate_image, generate_document,
		// navigate_to_page), do NOT re-send tools in the second call.
		// This prevents Gemini from issuing another function call instead of
		// generating a text response. For data-query function calls, keep
		// tools so Gemini can chain additional queries if needed.
		isActionFn := fnName == "generate_image" || fnName == "generate_document" || fnName == "navigate_to_page" || fnName == "generate_video"
		if !isActionFn {
			// In the second Gemini call (after function execution), web search
			// was NOT needed (if it were, we wouldn't have entered the FC path).
			// So always use function calling tools here.
			var secondFuncDecls []map[string]interface{}
			if tenantID != uuid.Nil {
				secondFuncDecls = buildGeminiFunctionDeclarations()
			} else {
				secondFuncDecls = buildNavigationOnlyDeclarations()
			}
			secondToolsArray := []interface{}{
				map[string]interface{}{
					"functionDeclarations": secondFuncDecls,
				},
			}
			secondGeminiReq["tools"] = secondToolsArray
		}

		secondGenCfg := map[string]interface{}{}
		if req.Temperature > 0 {
			secondGenCfg["temperature"] = req.Temperature
		}
		if req.MaxTokens > 0 {
			secondGenCfg["maxOutputTokens"] = req.MaxTokens
		}
		// Gemini 2.5 models have thinking enabled by default, which consumes maxOutputTokens
		// and can cause empty responses. Disable thinking by default for reliability.
		if strings.HasPrefix(model, "gemini-2.5") {
			secondGenCfg["thinkingConfig"] = map[string]interface{}{
				"thinkingBudget": 0,
			}
		}
		if len(secondGenCfg) > 0 {
			secondGeminiReq["generationConfig"] = secondGenCfg
		}

		secondReqBody, err := json.Marshal(secondGeminiReq)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to build second AI request"})
		}

		secondHttpReq, err := http.NewRequest("POST", secondApiURL, bytes.NewBuffer(secondReqBody))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create second AI request"})
		}
		secondHttpReq.Header.Set("Content-Type", "application/json")

		secondResp, err := client.Do(secondHttpReq)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to call AI API second time: " + err.Error()})
		}
		defer secondResp.Body.Close()

		secondBody, err := io.ReadAll(secondResp.Body)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to read second AI response"})
		}

		// Helper: check if we already have action results (image_urls, doc_info,
		// navigate_url) from the first function call execution. If so, a failure
		// in the second Gemini call is non-fatal — we can provide a fallback
		// text response and still deliver the action results to the frontend.
		hasActionResults := false
		var fallbackText string
		if imgURLs, ok := c.Locals("vai_image_urls").([]string); ok && len(imgURLs) > 0 {
			hasActionResults = true
			fallbackText = "已為您生成圖片："
		}
		if docInfo, ok := c.Locals("vai_doc_info").(map[string]interface{}); ok && docInfo != nil {
			hasActionResults = true
			fallbackText = "已為您生成文件，請點擊下方卡片下載。"
		}
		if navURL, ok := c.Locals("vai_navigate_url").(string); ok && navURL != "" {
			hasActionResults = true
			fallbackText = "正在前往頁面..."
		}
		if videoInfo, ok := c.Locals("vai_video_info").(map[string]interface{}); ok && videoInfo != nil {
			hasActionResults = true
			fallbackText = "正在為您生成影片，請稍候..."
		}

		var finalResponseText string

		if secondResp.StatusCode != 200 {
			log.Printf("[ChatWithLLM] Gemini second call error (status %d): %s", secondResp.StatusCode, string(secondBody))
			if hasActionResults {
				// Non-fatal: use fallback text and continue to deliver action results
				log.Printf("[ChatWithLLM] Second call failed but action results exist, using fallback text")
				finalResponseText = fallbackText
			} else {
				return c.Status(secondResp.StatusCode).JSON(fiber.Map{"error": string(secondBody)})
			}
		}

		if finalResponseText == "" {
			var secondLlmResponse map[string]interface{}
			if err := json.Unmarshal(secondBody, &secondLlmResponse); err != nil {
				if hasActionResults {
					finalResponseText = fallbackText
				} else {
					return c.Status(500).JSON(fiber.Map{"error": "Failed to parse second AI response"})
				}
			} else {
				// Extract final text from the second response
				if candidates, ok := secondLlmResponse["candidates"].([]interface{}); ok && len(candidates) > 0 {
					if candidate, ok := candidates[0].(map[string]interface{}); ok {
						if content, ok := candidate["content"].(map[string]interface{}); ok {
							if parts, ok := content["parts"].([]interface{}); ok {
								for _, p := range parts {
									if part, ok := p.(map[string]interface{}); ok {
										// Skip thinking parts from Gemini 2.5 models (thought: true)
										if thought, ok := part["thought"].(bool); ok && thought {
											continue
										}
										if text, ok := part["text"].(string); ok {
											finalResponseText += text
										}
									}
								}
							}
						}
						// Extract grounding metadata from second response
						if gm, ok := candidate["groundingMetadata"].(map[string]interface{}); ok {
							secondGroundingMeta := extractGroundingMetadata(gm)
							if len(secondGroundingMeta) > 0 {
								c.Locals("vai_grounding", secondGroundingMeta)
							}
						}
					}
				}
			}
		}

		// If Gemini returned no text (e.g. another function call or empty
		// candidates due to safety filtering), use fallback for action results
		if finalResponseText == "" && hasActionResults {
			log.Printf("[ChatWithLLM] Second call returned empty text but action results exist, using fallback")
			finalResponseText = fallbackText
		}

		finalResponseText = convertLinksToHTML(finalResponseText)
		finalResponseText = sanitizeBrandNames(finalResponseText)

		responsePayload := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": finalResponseText,
					},
				},
			},
		}
		// Inject navigate_url if a navigate_to_page function call was executed
		if navURL, ok := c.Locals("vai_navigate_url").(string); ok && navURL != "" {
			responsePayload["navigate_url"] = navURL
		}
		// Inject payment_link_url if a send_payment_link or get_payment_link function call was executed
		if payLinkURL, ok := c.Locals("vai_payment_link_url").(string); ok && payLinkURL != "" {
			responsePayload["payment_link_url"] = payLinkURL
		}
		// Inject image_urls if a generate_image function call was executed
		if imgURLs, ok := c.Locals("vai_image_urls").([]string); ok && len(imgURLs) > 0 {
			responsePayload["image_urls"] = imgURLs
		}
		// Inject doc_info if a generate_document function call was executed
		if docInfo, ok := c.Locals("vai_doc_info").(map[string]interface{}); ok && docInfo != nil {
			responsePayload["doc_info"] = docInfo
			log.Printf("[DOC-GEN] Injecting doc_info into response: %+v", docInfo)
		} else {
			log.Printf("[DOC-GEN] WARNING: vai_doc_info NOT found or wrong type. Raw local: %v (type: %T)", c.Locals("vai_doc_info"), c.Locals("vai_doc_info"))
		}
		// Inject video_info if a generate_video function call was executed
		if videoInfo, ok := c.Locals("vai_video_info").(map[string]interface{}); ok && videoInfo != nil {
			responsePayload["video_info"] = videoInfo
			log.Printf("[VIDEO-GEN] Injecting video_info into response: %+v", videoInfo)
		}
		// Inject grounding metadata if web search was used
		if groundingData, ok := c.Locals("vai_grounding").(map[string]interface{}); ok && len(groundingData) > 0 {
			responsePayload["grounding"] = groundingData
		}
		// Backend save: persist AI message as double insurance (Gemini function-call path)
		extraData := map[string]interface{}{}
		if navURL, ok := c.Locals("vai_navigate_url").(string); ok && navURL != "" {
			extraData["navigate_url"] = navURL
		}
		if payLinkURL, ok := c.Locals("vai_payment_link_url").(string); ok && payLinkURL != "" {
			extraData["payment_link_url"] = payLinkURL
		}
		if imgURLs, ok := c.Locals("vai_image_urls").([]string); ok && len(imgURLs) > 0 {
			extraData["image_urls"] = imgURLs
		}
		if docInfo, ok := c.Locals("vai_doc_info").(map[string]interface{}); ok && docInfo != nil {
			extraData["doc_info"] = docInfo
		}
		if videoInfo, ok := c.Locals("vai_video_info").(map[string]interface{}); ok && videoInfo != nil {
			extraData["video_info"] = videoInfo
		}
		if groundingData, ok := c.Locals("vai_grounding").(map[string]interface{}); ok && len(groundingData) > 0 {
			extraData["grounding"] = groundingData
		}
		if len(extraData) == 0 {
			extraData = nil
		}
		go saveAIChatMessage(tenantID, userID, convID, finalResponseText, extraData)
		return c.JSON(responsePayload)
	}

	// ========== OpenAI 路徑 (保留 prompt engineering 方式) ==========
	// 提取第一次 LLM 回應的文本
	var firstResponseText string
	if choices, ok := llmResponse["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					firstResponseText = content
				}
			}
		}
	}

	// 解析意圖 (OpenAI prompt engineering fallback)
	intent, nonJsonText := parseIntentFromResponse(firstResponseText)

	// 如果沒有意圖或不需要數據，直接返回第一次回應
	if intent == nil || !intent.NeedData || tenantID == uuid.Nil {
		firstResponseText = convertLinksToHTML(firstResponseText)
		firstResponseText = sanitizeBrandNames(firstResponseText)
		// Backend save: persist AI message as double insurance (OpenAI no-intent path)
		go saveAIChatMessage(tenantID, userID, convID, firstResponseText, nil)
		return c.JSON(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": firstResponseText,
					},
				},
			},
		})
	}

	// 需要數據，獲取數據後再次調用 LLM (OpenAI path)
	var dataContext string
	if isActionIntent(intent.Intent) {
		dataContext = executeAction(c, tenantID, intent)
	} else {
		dataContext = getDataByIntent(c, tenantID, intent.Intent, intent.Limit, intent.Query)
	}
	if dataContext == "" {
		firstResponseText = convertLinksToHTML(firstResponseText)
		firstResponseText = sanitizeBrandNames(firstResponseText)
		// Backend save: persist AI message as double insurance (OpenAI intent-no-data path)
		go saveAIChatMessage(tenantID, userID, convID, firstResponseText, nil)
		return c.JSON(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": firstResponseText,
					},
				},
			},
		})
	}

	// 第二次調用 LLM (OpenAI path)
	finalMessages := make([]Message, 0)
	for i := 0; i < len(req.Messages)-1; i++ {
		finalMessages = append(finalMessages, req.Messages[i])
	}
	if len(req.Messages) > 0 {
		lastUserMsg := req.Messages[len(req.Messages)-1]
		if lastUserMsg.Role == "user" {
			finalMessages = append(finalMessages, Message{
				Role:    "user",
				Content: lastUserMsg.Content,
			})
		}
	}
	finalMessages = append(finalMessages, Message{
		Role:    "assistant",
		Content: nonJsonText,
	})
	secondCallInstruction := "請基於以下數據回答用戶的問題。只使用下面提供的鏈接和數據，不要自行編造任何 URL 或鏈接：" + dataContext
	finalMessages = append(finalMessages, Message{
		Role:    "user",
		Content: secondCallInstruction,
	})

	secondMessages := []Message{
		{Role: "system", Content: basePrompt},
	}
	secondMessages = append(secondMessages, finalMessages...)

	secondLlmReq := map[string]interface{}{
		"model":       req.Model,
		"messages":    secondMessages,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
	}

	secondReqBody, err := json.Marshal(secondLlmReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to build second request"})
	}

	secondApiURL := cfg.LLM.Endpoint

	// 發送第二次請求 (OpenAI path)
	secondHttpReq, err := http.NewRequest("POST", secondApiURL, bytes.NewBuffer(secondReqBody))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create second request"})
	}

	secondHttpReq.Header.Set("Content-Type", "application/json")
	if cfg.LLM.APIKey != "" {
		secondHttpReq.Header.Set("Authorization", "Bearer "+cfg.LLM.APIKey)
	}

	secondResp, err := client.Do(secondHttpReq)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to call LLM API second time: " + err.Error()})
	}
	defer secondResp.Body.Close()

	secondBody, err := io.ReadAll(secondResp.Body)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to read second LLM response"})
	}

	if secondResp.StatusCode != 200 {
		return c.Status(secondResp.StatusCode).JSON(fiber.Map{"error": string(secondBody)})
	}

	var secondLlmResponse map[string]interface{}
	if err := json.Unmarshal(secondBody, &secondLlmResponse); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to parse second LLM response"})
	}

	var finalResponseText string
	if choices, ok := secondLlmResponse["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].(string); ok {
					finalResponseText = content
				}
			}
		}
	}

	finalResponseText = convertLinksToHTML(finalResponseText)
	finalResponseText = sanitizeBrandNames(finalResponseText)
	// Backend save: persist AI message as double insurance (OpenAI with-data path)
	go saveAIChatMessage(tenantID, userID, convID, finalResponseText, nil)
	return c.JSON(map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": finalResponseText,
				},
			},
		},
	})
}

type Message struct {
	Role             string                 `json:"role"`
	Content          string                 `json:"content"`
	FunctionCallName string                 `json:"function_call_name,omitempty"` // For role="function_call": the function name Gemini wants to call
	FunctionCallArgs map[string]interface{} `json:"function_call_args,omitempty"` // For role="function_call": the function args
	ThoughtSignature string                 `json:"thought_signature,omitempty"`  // NEW: for Gemini 2.0+ reasoning models
	FunctionRespName string                 `json:"function_resp_name,omitempty"` // For role="function_response": the function name
	FunctionRespData map[string]interface{} `json:"function_resp_data,omitempty"` // For role="function_response": the result data
}

// Attachment represents a file attached to an AI chat message
type Attachment struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // base64 encoded
}

// extractGroundingMetadata extracts web search grounding information from a Gemini
// response's groundingMetadata field. Returns a structured map with sources and summary
// suitable for returning to the frontend.
func extractGroundingMetadata(gm map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}

	// Extract grounding chunks (web sources with title + URL)
	type groundingSource struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Text  string `json:"text"`
	}
	var sources []groundingSource

	if chunks, ok := gm["groundingChunks"].([]interface{}); ok {
		for _, chunk := range chunks {
			chunkMap, ok := chunk.(map[string]interface{})
			if !ok {
				continue
			}
			if web, ok := chunkMap["web"].(map[string]interface{}); ok {
				title, _ := web["title"].(string)
				uri, _ := web["uri"].(string)
				sources = append(sources, groundingSource{
					Title: title,
					URL:   uri,
				})
			}
		}
	}

	// Enrich sources with snippet text from groundingSupports
	if supports, ok := gm["groundingSupports"].([]interface{}); ok {
		for _, support := range supports {
			supportMap, ok := support.(map[string]interface{})
			if !ok {
				continue
			}
			segmentText := ""
			if segment, ok := supportMap["segment"].(map[string]interface{}); ok {
				segmentText, _ = segment["text"].(string)
			}
			if indices, ok := supportMap["groundingChunkIndices"].([]interface{}); ok {
				for _, idx := range indices {
					var i int
					switch v := idx.(type) {
					case float64:
						i = int(v)
					case int:
						i = v
					default:
						continue
					}
					if i >= 0 && i < len(sources) && sources[i].Text == "" {
						sources[i].Text = segmentText
					}
				}
			}
		}
	}

	if len(sources) > 0 {
		result["sources"] = sources
	}

	// Extract search entry point (rendered HTML snippet from Google)
	if sep, ok := gm["searchEntryPoint"].(map[string]interface{}); ok {
		if rendered, ok := sep["renderedContent"].(string); ok && rendered != "" {
			result["searchEntryPoint"] = rendered
		}
	}

	return result
}
