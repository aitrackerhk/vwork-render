package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// Payment Methods
// ============================================

func GetPaymentMethods(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var methods []models.PaymentMethod

	query := database.DB.Where("tenant_id = ?", tenantID)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status = ?", "active")
	}

	// 按 is_default DESC 排序，確保默認值在第一個，然後按 name ASC 排序
	if err := query.Order("is_default DESC, name ASC").Find(&methods).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": methods})
}

func GetPaymentMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var method models.PaymentMethod
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&method).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Payment method not found"})
	}

	return c.JSON(method)
}

func CreatePaymentMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req models.PaymentMethod
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.TenantID = tenantID
	if req.Status == "" {
		req.Status = "active"
	}

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault {
		if err := tx.Model(&models.PaymentMethod{}).
			Where("tenant_id = ? AND is_default = ?", tenantID, true).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other payment methods: " + err.Error()})
		}
	}

	// 創建新的付款方式
	if err := tx.Create(&req).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	return c.Status(201).JSON(req)
}

func UpdatePaymentMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var method models.PaymentMethod
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&method).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Payment method not found"})
	}

	var req models.PaymentMethod
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

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault {
		if err := tx.Model(&models.PaymentMethod{}).
			Where("tenant_id = ? AND is_default = ? AND id != ?", tenantID, true, id).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other payment methods: " + err.Error()})
		}
	}

	method.Name = req.Name
	method.Code = req.Code
	method.IsDefault = req.IsDefault
	method.IsDefaultExpense = req.IsDefaultExpense
	method.IsOnlinePayment = req.IsOnlinePayment
	method.Status = req.Status
	if req.ExtraFields != nil {
		method.ExtraFields = req.ExtraFields
	}

	// 保存更新
	if err := tx.Save(&method).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	// 重新查詢以確保返回最新的數據（包括 is_default）
	var updatedMethod models.PaymentMethod
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&updatedMethod).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload payment method"})
	}

	return c.JSON(updatedMethod)
}

func DeletePaymentMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.PaymentMethod{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Payment method deleted successfully"})
}

// ============================================
// Shipping Methods
// ============================================

func GetShippingMethods(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var methods []models.ShippingMethod

	query := database.DB.Where("tenant_id = ?", tenantID)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status = ?", "active")
	}

	// 按 is_default DESC 排序，確保默認值在第一個，然後按 name ASC 排序
	if err := query.Order("is_default DESC, name ASC").Find(&methods).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": methods})
}

func GetShippingMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var method models.ShippingMethod
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&method).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shipping method not found"})
	}

	return c.JSON(method)
}

func CreateShippingMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req models.ShippingMethod
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.TenantID = tenantID
	if req.Status == "" {
		req.Status = "active"
	}

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault {
		database.DB.Model(&models.ShippingMethod{}).
			Where("tenant_id = ?", tenantID).
			Update("is_default", false)
	}

	if err := database.DB.Create(&req).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(req)
}

func UpdateShippingMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var method models.ShippingMethod
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&method).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Shipping method not found"})
	}

	var req models.ShippingMethod
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

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault {
		if err := tx.Model(&models.ShippingMethod{}).
			Where("tenant_id = ? AND is_default = ? AND id != ?", tenantID, true, id).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other shipping methods: " + err.Error()})
		}
	}

	method.Name = req.Name
	method.Code = req.Code
	method.RequiresShipping = req.RequiresShipping
	method.IsDefault = req.IsDefault
	method.Status = req.Status

	// 保存更新
	if err := tx.Save(&method).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	// 重新查詢以確保返回最新的數據（包括 is_default）
	var updatedMethod models.ShippingMethod
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&updatedMethod).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload shipping method"})
	}

	return c.JSON(updatedMethod)
}

func DeleteShippingMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.ShippingMethod{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Shipping method deleted successfully"})
}

// ============================================
// Logistics Companies
// ============================================

func GetLogisticsCompanies(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var companies []models.LogisticsCompany

	query := database.DB.Where("tenant_id = ?", tenantID)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status = ?", "active")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.LogisticsCompany{}).Count(&total)

	// 按 is_default DESC 排序，確保默認值在第一個，然後按 name ASC 排序
	if err := query.Offset(offset).Limit(limit).Order("is_default DESC, name ASC").Find(&companies).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  companies,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetLogisticsCompany(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var company models.LogisticsCompany
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&company).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Logistics company not found"})
	}

	return c.JSON(company)
}

func CreateLogisticsCompany(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req models.LogisticsCompany
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.TenantID = tenantID
	if req.Status == "" {
		req.Status = "active"
	}

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault {
		if err := tx.Model(&models.LogisticsCompany{}).
			Where("tenant_id = ? AND is_default = ?", tenantID, true).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other logistics companies: " + err.Error()})
		}
	}

	if err := tx.Create(&req).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	tx.Commit()

	return c.Status(201).JSON(req)
}

func UpdateLogisticsCompany(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var company models.LogisticsCompany
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&company).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Logistics company not found"})
	}

	var req models.LogisticsCompany
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

	// 如果設置為默認，取消其他默認設置
	if req.IsDefault {
		if err := tx.Model(&models.LogisticsCompany{}).
			Where("tenant_id = ? AND is_default = ? AND id != ?", tenantID, true, id).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other logistics companies: " + err.Error()})
		}
	}

	company.Name = req.Name
	company.Code = req.Code
	company.BaseFee = req.BaseFee
	company.PerItemFee = req.PerItemFee
	company.PerWeightFee = req.PerWeightFee
	company.PerAreaFee = req.PerAreaFee
	company.Status = req.Status
	company.IsDefault = req.IsDefault
	if req.ExtraFields != nil {
		company.ExtraFields = req.ExtraFields
	}

	if err := tx.Save(&company).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	// 重新查詢以確保返回最新的數據（包括 is_default）
	var updatedCompany models.LogisticsCompany
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&updatedCompany).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reload logistics company"})
	}

	// 確保 is_default 字段被包含在響應中
	response := fiber.Map{
		"id":             updatedCompany.ID,
		"tenant_id":      updatedCompany.TenantID,
		"name":           updatedCompany.Name,
		"code":           updatedCompany.Code,
		"base_fee":       updatedCompany.BaseFee,
		"per_item_fee":   updatedCompany.PerItemFee,
		"per_weight_fee": updatedCompany.PerWeightFee,
		"per_area_fee":   updatedCompany.PerAreaFee,
		"status":         updatedCompany.Status,
		"is_default":     updatedCompany.IsDefault,
		"created_at":     updatedCompany.CreatedAt,
		"updated_at":     updatedCompany.UpdatedAt,
	}
	if updatedCompany.ExtraFields != nil {
		response["extra_fields"] = updatedCompany.ExtraFields
	}

	return c.JSON(response)
}

func DeleteLogisticsCompany(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.LogisticsCompany{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Logistics company deleted successfully"})
}

// CalculateLogisticsFee 計算物流費用
func CalculateLogisticsFee(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		LogisticsCompanyID uuid.UUID `json:"logistics_company_id"`
		OrderItems         []struct {
			ProductID uuid.UUID `json:"product_id"`
			Quantity  float64   `json:"quantity"`
		} `json:"order_items"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 獲取物流公司
	var company models.LogisticsCompany
	if err := database.DB.Where("id = ? AND tenant_id = ?", req.LogisticsCompanyID, tenantID).First(&company).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Logistics company not found"})
	}

	// 計算費用
	totalFee := company.BaseFee
	totalWeight := 0.0
	totalArea := 0.0
	totalItems := 0

	for _, item := range req.OrderItems {
		var product models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ?", item.ProductID, tenantID).First(&product).Error; err == nil {
			totalItems += int(item.Quantity)
			totalWeight += product.Weight * item.Quantity
			totalArea += product.Area * item.Quantity
		}
	}

	totalFee += company.PerItemFee * float64(totalItems)
	totalFee += company.PerWeightFee * totalWeight
	totalFee += company.PerAreaFee * totalArea

	return c.JSON(fiber.Map{
		"base_fee":       company.BaseFee,
		"per_item_fee":   company.PerItemFee * float64(totalItems),
		"per_weight_fee": company.PerWeightFee * totalWeight,
		"per_area_fee":   company.PerAreaFee * totalArea,
		"total_fee":      totalFee,
		"total_items":    totalItems,
		"total_weight":   totalWeight,
		"total_area":     totalArea,
	})
}

// CalculateBestLogisticsFee 計算所有物流公司費用並返回最便宜的
func CalculateBestLogisticsFee(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		// Optional: for country/region filtering
		CountryCode string `json:"country_code"`
		RegionCode  string `json:"region_code"`
		OrderItems []struct {
			ProductID uuid.UUID `json:"product_id"`
			Quantity  float64   `json:"quantity"`
		} `json:"order_items"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 獲取所有活躍的物流公司
	var companies []models.LogisticsCompany
	if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").Find(&companies).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch logistics companies"})
	}

	if len(companies) == 0 {
		return c.JSON(fiber.Map{
			"best_company": nil,
			"best_fee":     0.0,
			"all_fees":     []interface{}{},
		})
	}

	// 計算每個物流公司的費用
	type CompanyFee struct {
		CompanyID   uuid.UUID `json:"company_id"`
		CompanyName string    `json:"company_name"`
		TotalFee    float64   `json:"total_fee"`
	}

	var allFees []CompanyFee
	totalWeight := 0.0
	totalArea := 0.0
	totalItems := 0

	// 先計算總重量、面積和件數
	for _, item := range req.OrderItems {
		var product models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ?", item.ProductID, tenantID).First(&product).Error; err == nil {
			totalItems += int(item.Quantity)
			totalWeight += product.Weight * item.Quantity
			totalArea += product.Area * item.Quantity
		}
	}

	// 計算每個公司的費用
	for _, company := range companies {
		// Optional country/region filter:
		// - extra_fields.allowed_country_codes: []string (empty => allow all)
		// - extra_fields.allowed_region_keys: []string, values like "US-CA" (empty => allow all regions within allowed countries)
		if !logisticsCompanyMatchesLocation(company, req.CountryCode, req.RegionCode) {
			continue
		}

		totalFee := company.BaseFee
		totalFee += company.PerItemFee * float64(totalItems)
		totalFee += company.PerWeightFee * totalWeight
		totalFee += company.PerAreaFee * totalArea

		allFees = append(allFees, CompanyFee{
			CompanyID:   company.ID,
			CompanyName: company.Name,
			TotalFee:    totalFee,
		})
	}

	// 找到最便宜的公司
	if len(allFees) == 0 {
		return c.JSON(fiber.Map{
			"best_company": nil,
			"best_fee":     0.0,
			"all_fees":     []interface{}{},
		})
	}
	bestFee := allFees[0]
	for _, fee := range allFees {
		if fee.TotalFee < bestFee.TotalFee {
			bestFee = fee
		}
	}

	return c.JSON(fiber.Map{
		"best_company": fiber.Map{
			"id":   bestFee.CompanyID,
			"name": bestFee.CompanyName,
		},
		"best_fee": bestFee.TotalFee,
		"all_fees": allFees,
	})
}

func logisticsCompanyMatchesLocation(company models.LogisticsCompany, countryCode string, regionCode string) bool {
	cc := strings.ToUpper(strings.TrimSpace(countryCode))
	rc := strings.TrimSpace(regionCode)
	allowedCountries := getExtraStringSlice(company.ExtraFields, "allowed_country_codes")
	allowedRegions := getExtraStringSlice(company.ExtraFields, "allowed_region_keys")

	// If no country provided, do not filter (backward compatible).
	if cc == "" {
		return true
	}
	if len(allowedCountries) > 0 && !stringSliceContainsCI(allowedCountries, cc) {
		return false
	}
	// If company specifies region restrictions but caller didn't provide region, treat as not match.
	if len(allowedRegions) > 0 {
		if rc == "" {
			return false
		}
		key := cc + "-" + rc
		if !stringSliceContainsCI(allowedRegions, key) {
			return false
		}
	}
	return true
}

func getExtraStringSlice(extra models.JSONB, key string) []string {
	if extra == nil {
		return nil
	}
	raw, ok := extra[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, it := range v {
			if it == nil {
				continue
			}
			s := strings.TrimSpace(fmt.Sprint(it))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		// single value fallback
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return nil
		}
		return []string{s}
	}
}

func stringSliceContainsCI(list []string, want string) bool {
	wantU := strings.ToUpper(strings.TrimSpace(want))
	if wantU == "" {
		return false
	}
	for _, s := range list {
		if strings.ToUpper(strings.TrimSpace(s)) == wantU {
			return true
		}
	}
	return false
}

// ============================================
// Bank Accounts
// ============================================

func GetBankAccounts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var accounts []models.BankAccount

	query := database.DB.Where("tenant_id = ?", tenantID)

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	} else {
		query = query.Where("status = ?", "active")
	}

	if err := query.Order("name ASC").Find(&accounts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": accounts})
}

func GetBankAccount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var account models.BankAccount
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&account).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Bank account not found"})
	}

	return c.JSON(account)
}

func CreateBankAccount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req models.BankAccount
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	req.TenantID = tenantID
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Currency == "" {
		req.Currency = "HKD"
	}

	// 使用事務確保原子性
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 如果設置為默認收款帳號，取消其他默認收款帳號設置
	if req.IsDefaultReceiving {
		if err := tx.Model(&models.BankAccount{}).
			Where("tenant_id = ?", tenantID).
			Update("is_default_receiving", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other receiving accounts: " + err.Error()})
		}
	}

	// 如果設置為默認付款帳號，取消其他默認付款帳號設置
	if req.IsDefaultPayment {
		if err := tx.Model(&models.BankAccount{}).
			Where("tenant_id = ?", tenantID).
			Update("is_default_payment", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other payment accounts: " + err.Error()})
		}
	}

	// 如果設置為默認，取消其他默認設置（保留以兼容舊數據）
	if req.IsDefault {
		if err := tx.Model(&models.BankAccount{}).
			Where("tenant_id = ?", tenantID).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update other default accounts: " + err.Error()})
		}
	}

	if err := tx.Create(&req).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction: " + err.Error()})
	}

	return c.Status(201).JSON(req)
}

func UpdateBankAccount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var account models.BankAccount
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&account).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Bank account not found"})
	}

	var req models.BankAccount
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

	// 如果設置為默認收款帳號，取消其他默認收款帳號設置
	if req.IsDefaultReceiving {
		if err := tx.Model(&models.BankAccount{}).
			Where("tenant_id = ? AND id != ?", tenantID, id).
			Update("is_default_receiving", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 如果設置為默認付款帳號，取消其他默認付款帳號設置
	if req.IsDefaultPayment {
		if err := tx.Model(&models.BankAccount{}).
			Where("tenant_id = ? AND id != ?", tenantID, id).
			Update("is_default_payment", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}

	// 如果設置為默認，取消其他默認設置（保留以兼容舊數據）
	if req.IsDefault {
		if err := tx.Model(&models.BankAccount{}).
			Where("tenant_id = ? AND id != ?", tenantID, id).
			Update("is_default", false).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}

	account.Name = req.Name
	account.BankName = req.BankName
	account.AccountNumber = req.AccountNumber
	account.AccountHolder = req.AccountHolder
	account.Currency = req.Currency
	account.IsDefault = req.IsDefault
	account.IsDefaultReceiving = req.IsDefaultReceiving
	account.IsDefaultPayment = req.IsDefaultPayment
	account.Status = req.Status
	account.Notes = req.Notes
	if req.ExtraFields != nil {
		account.ExtraFields = req.ExtraFields
	}

	if err := tx.Save(&account).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 提交事務
	tx.Commit()

	// 重新查詢以獲取最新數據
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&account).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(account)
}

func DeleteBankAccount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.BankAccount{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Bank account deleted successfully"})
}

// GetBankAccountStats 獲取銀行賬戶統計信息
func GetBankAccountStats(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	// 獲取所有銀行賬戶
	var accounts []models.BankAccount
	if err := database.DB.Where("tenant_id = ? AND status = ?", tenantID, "active").
		Order("is_default DESC, name ASC").Find(&accounts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	var stats []map[string]interface{}

	for _, account := range accounts {
		// 統計收入
		var totalIncome float64
		var incomeCount int64
		database.DB.Model(&models.Income{}).
			Where("tenant_id = ? AND bank_account_id = ? AND status = ?", tenantID, account.ID, "confirmed").
			Select("COALESCE(SUM(amount), 0)").Scan(&totalIncome)
		database.DB.Model(&models.Income{}).
			Where("tenant_id = ? AND bank_account_id = ? AND status = ?", tenantID, account.ID, "confirmed").
			Count(&incomeCount)

		// 統計支出
		var totalExpense float64
		var expenseCount int64
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND bank_account_id = ? AND status = ?", tenantID, account.ID, "confirmed").
			Select("COALESCE(SUM(amount), 0)").Scan(&totalExpense)
		database.DB.Model(&models.Expense{}).
			Where("tenant_id = ? AND bank_account_id = ? AND status = ?", tenantID, account.ID, "confirmed").
			Count(&expenseCount)

		// 計算淨額
		netAmount := totalIncome - totalExpense

		stats = append(stats, map[string]interface{}{
			"account_id":         account.ID,
			"account_name":       account.Name,
			"bank_name":          account.BankName,
			"account_number":     account.AccountNumber,
			"currency":           account.Currency,
			"is_default":         account.IsDefault,
			"total_income":       totalIncome,
			"total_expense":      totalExpense,
			"net_amount":         netAmount,
			"income_count":       incomeCount,
			"expense_count":      expenseCount,
			"total_transactions": incomeCount + expenseCount,
		})
	}

	return c.JSON(fiber.Map{"data": stats})
}

