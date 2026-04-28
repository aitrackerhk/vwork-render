package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// GetCustomers 獲取客戶列表
func GetCustomers(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var customers []models.Customer
	query := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID).Preload("MemberLevel").Preload("Labels")

	// 根據referral_code查找（優先）
	if referralCode := c.Query("referral_code"); referralCode != "" {
		query = query.Where("referral_code = ?", referralCode)
	} else if search := c.Query("search"); search != "" {
		// 搜索過濾
		query = query.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ? OR referral_code = ?", "%"+search+"%", "%"+search+"%", "%"+search+"%", search)
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 會員等級過濾
	if memberLevelIDs := parseUUIDListFromStrings(c.Query("member_level_id"), c.Query("member_level_ids")); len(memberLevelIDs) > 0 {
		query = query.Where("member_level_id IN ?", memberLevelIDs)
	}

	// 標籤過濾
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN customer_label_relations ON customers.id = customer_label_relations.customer_id").
			Where("customer_label_relations.label_id IN ?", labelIDs).
			Group("customers.id")
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Customer{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&customers).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch customers: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  customers,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetCustomer 獲取單個客戶
func GetCustomer(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}

	var customer models.Customer
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("MemberLevel").
		Preload("Labels").
		First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "Customer not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch customer: %v", err)})
	}

	customerMap := make(map[string]interface{})
	customerBytes, _ := json.Marshal(customer)
	_ = json.Unmarshal(customerBytes, &customerMap)

	labelIDs := make([]string, 0)
	if customer.Labels != nil {
		for _, l := range customer.Labels {
			if l.ID != uuid.Nil {
				labelIDs = append(labelIDs, l.ID.String())
			}
		}
	}
	customerMap["label_ids"] = labelIDs

	return c.JSON(customerMap)
}

// CheckCustomerDuplicate 檢查客戶電郵或電話是否重複
func CheckCustomerDuplicate(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Email            string `json:"email"`
		Phone            string `json:"phone"`
		PhoneCountryCode string `json:"phone_country_code"`
		ExcludeID        string `json:"exclude_id"` // 編輯時排除當前記錄
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	var duplicates []map[string]interface{}

	// 檢查電郵重複
	if req.Email != "" {
		var emailCustomer models.Customer
		query := database.DB.Where("tenant_id = ? AND email = ? AND email != ''", tenantID, req.Email)
		if req.ExcludeID != "" {
			query = query.Where("id != ?", req.ExcludeID)
		}
		if err := query.First(&emailCustomer).Error; err == nil {
			duplicates = append(duplicates, map[string]interface{}{
				"type":  "email",
				"value": emailCustomer.Email,
				"customer": map[string]interface{}{
					"id":        emailCustomer.ID,
					"code":      emailCustomer.Code,
					"name":      emailCustomer.Name,
					"last_name": emailCustomer.LastName,
				},
			})
		}
	}

	// 檢查電話重複（需要同時匹配電話號碼和區號）
	if req.Phone != "" && req.PhoneCountryCode != "" {
		var phoneCustomer models.Customer
		query := database.DB.Where("tenant_id = ? AND phone = ? AND phone_country_code = ? AND phone != ''",
			tenantID, req.Phone, req.PhoneCountryCode)
		if req.ExcludeID != "" {
			query = query.Where("id != ?", req.ExcludeID)
		}
		if err := query.First(&phoneCustomer).Error; err == nil {
			duplicates = append(duplicates, map[string]interface{}{
				"type":  "phone",
				"value": phoneCustomer.PhoneCountryCode + " " + phoneCustomer.Phone,
				"customer": map[string]interface{}{
					"id":        phoneCustomer.ID,
					"code":      phoneCustomer.Code,
					"name":      phoneCustomer.Name,
					"last_name": phoneCustomer.LastName,
				},
			})
		}
	}

	if len(duplicates) > 0 {
		return c.JSON(fiber.Map{
			"has_duplicate": true,
			"duplicates":    duplicates,
		})
	}

	return c.JSON(fiber.Map{
		"has_duplicate": false,
		"duplicates":    []map[string]interface{}{},
	})
}

// CreateCustomer 創建客戶
func CreateCustomer(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Code             string                 `json:"code"`
		Name             string                 `json:"name"`
		LastName         string                 `json:"last_name"`
		Email            string                 `json:"email"`
		Password         string                 `json:"password"` // 密碼（用於網店登入）
		Phone            string                 `json:"phone"`
		PhoneCountryCode string                 `json:"phone_country_code"`
		ProfilePic       string                 `json:"profile_pic"`
		BirthDateStr     string                 `json:"birth_date"` // ISO 字串
		Gender           string                 `json:"gender"`     // 性別：male, female, unknown
		Address          string                 `json:"address"`
		Status           string                 `json:"status"`
		ReferralCode     string                 `json:"referral_code"` // 客戶填寫的介紹人代碼
		LabelIDs         []uuid.UUID            `json:"label_ids"`
		ExtraFields      map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	if req.Status == "" {
		req.Status = "active"
	}

	var birthDatePtr *time.Time
	if req.BirthDateStr != "" {
		if t, err := utils.ParseDateInTenantTimezone(tenantID, req.BirthDateStr); err == nil {
			birthDatePtr = &t
		}
	}

	// 處理密碼（如果提供）
	var passwordHash string
	if req.Password != "" {
		if len(req.Password) < 6 || len(req.Password) > 20 {
			return c.Status(400).JSON(fiber.Map{"error": "Password must be between 6 and 20 characters"})
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
		}
		passwordHash = string(hashedPassword)
	}

	now := utils.NowInTenantTimezone(tenantID)

	// 自動生成客戶編號（如果未提供）
	autoCode, err := generateAutoCode(tenantID, req.Code, autoCodeConfig{
		Prefix:     "CUST-",
		FieldName:  "code",
		PageName:   "customers",
		Format:     codeFormatDate,
		TableModel: &models.Customer{},
		Column:     "code",
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate customer code: " + err.Error()})
	}

	customer := models.Customer{
		TenantID:         tenantID,
		Code:             autoCode,
		Name:             req.Name,
		LastName:         req.LastName,
		Email:            req.Email,
		PasswordHash:     passwordHash,
		Phone:            req.Phone,
		PhoneCountryCode: req.PhoneCountryCode,
		ProfilePic:       req.ProfilePic,
		BirthDate:        birthDatePtr,
		Gender:           req.Gender,
		Address:          req.Address,
		Status:           req.Status,
		ReferralCode:     req.ReferralCode, // 客戶填寫的介紹人代碼（如果有的話）
		CreatedBy:        &userID,
		UpdatedBy:        &userID,
		CreatedAt:        now,
		UpdatedAt:        now,
		ExtraFields:      models.JSONB(req.ExtraFields),
	}

	if err := database.DB.Create(&customer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create customer: " + err.Error()})
	}

	// 成功建立後釋放預留編號
	releaseReservedCode(tenantID, "code", customer.Code)

	if len(req.LabelIDs) > 0 {
		for _, labelID := range req.LabelIDs {
			if labelID == uuid.Nil {
				continue
			}
			_ = database.DB.Create(&models.CustomerLabelRelation{
				CustomerID: customer.ID,
				LabelID:    labelID,
			}).Error
		}
	}

	// 記錄建立客戶活動
	utils.LogActivity(tenantID, userID, "create", "customer", &customer.ID,
		fmt.Sprintf(`{"key":"customer.create","params":{"name":%q}}`, customer.Name), nil, c)

	// Auto-refresh business goals related to customers
	go RefreshActiveBusinessGoals(tenantID, []string{"customer_count"})

	return c.Status(201).JSON(customer)
}

// UpdateCustomer 更新客戶
func UpdateCustomer(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}

	var customer models.Customer
	var oldCustomer models.Customer
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&customer).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Customer not found"})
	}
	// 保存舊值用於活動記錄
	oldCustomer = customer

	var req struct {
		Code             *string                 `json:"code"`
		Name             *string                 `json:"name"`
		LastName         *string                 `json:"last_name"`
		Email            *string                 `json:"email"`
		Password         *string                 `json:"password"` // 密碼（用於網店登入）
		Phone            *string                 `json:"phone"`
		PhoneCountryCode *string                 `json:"phone_country_code"`
		ProfilePic       *string                 `json:"profile_pic"`
		BirthDateStr     *string                 `json:"birth_date"`
		Gender           *string                 `json:"gender"` // 性別：male, female, unknown
		Address          *string                 `json:"address"`
		Status           *string                 `json:"status"`
		MemberLevelID    *uuid.UUID              `json:"member_level_id"`
		TotalPoints      *int                    `json:"total_points"`
		ReferralCode     *string                 `json:"referral_code"`
		ExtraFields      *map[string]interface{} `json:"extra_fields"`
		LabelIDs         *[]uuid.UUID            `json:"label_ids"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.Code != nil {
		customer.Code = *req.Code
	}
	if req.Name != nil {
		customer.Name = *req.Name
	}
	if req.LastName != nil {
		customer.LastName = *req.LastName
	}
	if req.LastName != nil {
		customer.LastName = *req.LastName // 允許空字符串，用於清空字段
	}
	if req.Email != nil {
		customer.Email = *req.Email // 允許空字符串，用於清空字段
	}
	if req.Password != nil && *req.Password != "" {
		if len(*req.Password) < 6 || len(*req.Password) > 20 {
			return c.Status(400).JSON(fiber.Map{"error": "Password must be between 6 and 20 characters"})
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to hash password"})
		}
		customer.PasswordHash = string(hashedPassword)
	}
	if req.Phone != nil {
		customer.Phone = *req.Phone // 允許空字符串，用於清空字段
	}
	if req.PhoneCountryCode != nil {
		customer.PhoneCountryCode = *req.PhoneCountryCode
	}
	if req.ProfilePic != nil {
		customer.ProfilePic = *req.ProfilePic // 允許空字符串，用於清空頭像
	}
	if req.BirthDateStr != nil {
		if *req.BirthDateStr == "" {
			customer.BirthDate = nil
		} else if t, err := utils.ParseDateInTenantTimezone(tenantID, *req.BirthDateStr); err == nil {
			customer.BirthDate = &t
		}
	}
	if req.Gender != nil {
		customer.Gender = *req.Gender // 允許空字符串，用於清空字段
	}
	if req.Address != nil {
		customer.Address = *req.Address // 允許空字符串，用於清空字段
	}
	if req.Status != nil {
		customer.Status = *req.Status
	}
	if req.MemberLevelID != nil {
		// 如果 MemberLevelID 是空 UUID，設置為 nil 以清空關聯
		if *req.MemberLevelID == uuid.Nil {
			customer.MemberLevelID = nil
		} else {
			customer.MemberLevelID = req.MemberLevelID
		}
	}
	if req.TotalPoints != nil {
		customer.TotalPoints = *req.TotalPoints
	}
	if req.ReferralCode != nil {
		customer.ReferralCode = *req.ReferralCode // 允許空字符串，用於清空字段
	}
	if req.ExtraFields != nil {
		customer.ExtraFields = models.JSONB(*req.ExtraFields)
	}

	customer.UpdatedBy = &userID
	customer.UpdatedAt = time.Now()

	if err := database.DB.Save(&customer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update customer: " + err.Error()})
	}

	if req.LabelIDs != nil {
		database.DB.Where("customer_id = ?", customer.ID).Delete(&models.CustomerLabelRelation{})
		for _, labelID := range *req.LabelIDs {
			if labelID == uuid.Nil {
				continue
			}
			_ = database.DB.Create(&models.CustomerLabelRelation{
				CustomerID: customer.ID,
				LabelID:    labelID,
			}).Error
		}
	}

	// 重新載入關聯數據
	database.DB.Where("id = ?", customer.ID).Preload("MemberLevel").Preload("Labels").First(&customer)

	// 記錄更新客戶活動
	changes := utils.GetChangesMap(oldCustomer, customer)
	utils.LogActivity(tenantID, userID, "update", "customer", &customer.ID,
		fmt.Sprintf(`{"key":"customer.update","params":{"name":%q}}`, customer.Name), changes, c)

	return c.JSON(customer)
}

// DeleteCustomer 刪除客戶（軟刪除：移到垃圾筒）
func DeleteCustomer(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}

	// 檢查是否為系統預設資料
	if IsSystemDefault(database.DB, "customers", id, tenantID) {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot delete system default data"})
	}

	// 先獲取客戶信息用於活動記錄
	var customer models.Customer
	if err := database.DB.Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).First(&customer).Error; err == nil {
		// 記錄刪除客戶活動
		utils.LogActivity(tenantID, userID, "delete", "customer", &id,
			fmt.Sprintf(`{"key":"customer.delete","params":{"name":%q}}`, customer.Name), nil, c)
	}

	// 軟刪除：設置 trashed_at 時間
	result := database.DB.Model(&models.Customer{}).Where("id = ? AND tenant_id = ? AND trashed_at IS NULL", id, tenantID).Update("trashed_at", time.Now())
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete customer"})
	}
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Customer not found"})
	}

	return c.JSON(fiber.Map{
		"message": "Customer moved to trash",
		"info":    "Data will be automatically deleted after 7 days",
	})
}

// ExportCustomersToExcel 導出客戶到 Excel
func ExportCustomersToExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var customers []models.Customer
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if memberLevelIDs := parseUUIDListFromStrings(c.Query("member_level_id"), c.Query("member_level_ids")); len(memberLevelIDs) > 0 {
		query = query.Where("member_level_id IN ?", memberLevelIDs)
	}
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN customer_label_relations ON customers.id = customer_label_relations.customer_id").
			Where("customer_label_relations.label_id IN ?", labelIDs).
			Group("customers.id")
	}

	if err := query.Order("created_at DESC").Find(&customers).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch customers"})
	}

	f := excelize.NewFile()
	defer f.Close()
	sheetName := "客戶列表"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	headers := []string{"編號", "名稱", "郵箱", "電話", "地址", "狀態"}
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheetName, cell, header)
		style, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
		})
		f.SetCellStyle(sheetName, cell, cell, style)
	}

	for i, customer := range customers {
		row := i + 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), customer.Code)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), customer.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), customer.Email)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), customer.Phone)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), customer.Address)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), customer.Status)
	}

	for i := 0; i < len(headers); i++ {
		f.SetColWidth(sheetName, string(rune('A'+i)), string(rune('A'+i)), 20)
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename=customers.xlsx")
	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate Excel file"})
	}
	return nil
}

// ExportCustomersToPDF 導出客戶到 PDF
func ExportCustomersToPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var customers []models.Customer
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR email ILIKE ? OR phone ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if memberLevelIDs := parseUUIDListFromStrings(c.Query("member_level_id"), c.Query("member_level_ids")); len(memberLevelIDs) > 0 {
		query = query.Where("member_level_id IN ?", memberLevelIDs)
	}
	if labelIDs := parseUUIDListFromStrings(c.Query("label_id"), c.Query("label_ids")); len(labelIDs) > 0 {
		query = query.Joins("JOIN customer_label_relations ON customers.id = customer_label_relations.customer_id").
			Where("customer_label_relations.label_id IN ?", labelIDs).
			Group("customers.id")
	}

	if err := query.Order("created_at DESC").Find(&customers).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch customers"})
	}

	headers := []string{"編號", "名稱", "郵箱", "電話", "狀態"}
	rows := make([][]string, 0, len(customers))
	for _, customer := range customers {
		rows = append(rows, []string{
			customer.Code,
			customer.Name,
			customer.Email,
			customer.Phone,
			customer.Status,
		})
	}
	pdfBytes, _ := utils.BuildTablePDFBytes("客戶列表", headers, rows)
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", "attachment; filename=customers.pdf")
	return c.Send(pdfBytes)
}

// generateUniqueReferralCode 生成唯一的推薦碼
func generateUniqueReferralCode(tenantID uuid.UUID) string {
	const maxAttempts = 10
	const codeLength = 8 // 8位字符的推薦碼

	for i := 0; i < maxAttempts; i++ {
		// 生成隨機推薦碼（8位大寫字母和數字）
		bytes := make([]byte, codeLength/2)
		if _, err := rand.Read(bytes); err != nil {
			continue
		}
		code := hex.EncodeToString(bytes)
		// 轉換為大寫並只取前8位
		if len(code) > codeLength {
			code = code[:codeLength]
		}
		// 轉為大寫字母
		referralCode := strings.ToUpper(code)

		// 檢查是否已存在
		var existing models.Customer
		if err := database.DB.Where("tenant_id = ? AND referral_code = ?", tenantID, referralCode).First(&existing).Error; err != nil {
			// 如果是記錄未找到錯誤，說明可以使用
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return referralCode
			}
			// 其他錯誤（如數據庫連接問題），繼續嘗試
			continue
		}
		// 如果找到了記錄，說明已存在，繼續嘗試下一個
	}

	// 如果所有嘗試都失敗，使用 UUID 的前8位（轉大寫）
	uuidStr := strings.ReplaceAll(uuid.New().String(), "-", "")
	if len(uuidStr) >= codeLength {
		return strings.ToUpper(uuidStr[:codeLength])
	}
	// 如果 UUID 長度不夠，補零
	return strings.ToUpper(uuidStr + strings.Repeat("0", codeLength-len(uuidStr)))[:codeLength]
}

// SendCustomerInviteEmail 發送客戶邀請郵件（用於設定密碼）
func SendCustomerInviteEmail(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}

	var customer models.Customer
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&customer).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Customer not found"})
	}

	if customer.Email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Customer email is required"})
	}

	// 獲取租戶 subdomain
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// 生成 invite token（類似 password reset token）
	token, tokenHash, err := newCustomerInviteToken()
	if err != nil {
		log.Printf("⚠️  SendCustomerInviteEmail new token failed: customer_id=%s err=%v", customer.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate invite token"})
	}

	// 創建 invite token（使用 password_reset_tokens 表，但用於客戶邀請）
	inviteToken := models.PasswordResetToken{
		ID:        uuid.New(),
		TenantID:  tenant.ID,
		UserID:    uuid.Nil, // 客戶邀請不使用 UserID
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // 7天有效期
		CreatedAt: time.Now(),
	}

	// 將 customer ID 存儲在 ExtraFields 中（如果需要的話，可以擴展模型）
	// 暫時使用一個簡單的方法：在 token 中編碼 customer ID
	if err := database.DB.Create(&inviteToken).Error; err != nil {
		log.Printf("⚠️  SendCustomerInviteEmail save token failed: customer_id=%s err=%v", customer.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save invite token"})
	}

	inviteURL, err := email.CustomerInviteURL(tenant.Subdomain, token)
	if err != nil {
		log.Printf("⚠️  CustomerInviteURL failed: customer_id=%s err=%v", customer.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate invite URL"})
	}

	if err := email.EnqueueCustomerInviteEmail(tenant.ID, tenant.Subdomain, customer.ID, customer.Email, customer.Name, inviteURL); err != nil {
		log.Printf("⚠️  enqueue customer invite email failed: customer_id=%s err=%v", customer.ID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to enqueue invite email"})
	}

	log.Printf("✅ Customer invite email sent: customer_id=%s email=%s", customer.ID, customer.Email)

	return c.JSON(fiber.Map{
		"message": "Invite email sent successfully",
	})
}

// newCustomerInviteToken 生成客戶邀請 token（類似 auth.go 中的 newOpaqueToken）
func newCustomerInviteToken() (plainToken string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	plainToken = hex.EncodeToString(b)
	hashBytes := sha256.Sum256([]byte(plainToken))
	return plainToken, hashBytes[:], nil
}

// ============================================
// Customer Address Management
// ============================================

// GetCustomerAddresses 獲取客戶地址列表
func GetCustomerAddresses(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID, err := uuid.Parse(c.Params("customerId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}

	// 驗證客戶屬於該租戶
	var customer models.Customer
	if err := database.DB.Where("id = ? AND tenant_id = ?", customerID, tenantID).First(&customer).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Customer not found"})
	}

	var addresses []models.CustomerAddress
	if err := database.DB.Where("customer_id = ? AND tenant_id = ?", customerID, tenantID).
		Order("is_default DESC, created_at DESC").Find(&addresses).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch addresses"})
	}

	return c.JSON(fiber.Map{"data": addresses})
}

// GetCustomerAddress 獲取單個地址
func GetCustomerAddress(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID, err := uuid.Parse(c.Params("customerId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}
	addressID, err := uuid.Parse(c.Params("addressId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid address ID"})
	}

	var address models.CustomerAddress
	if err := database.DB.Where("id = ? AND customer_id = ? AND tenant_id = ?", addressID, customerID, tenantID).
		First(&address).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Address not found"})
	}

	return c.JSON(address)
}

// CreateCustomerAddress 創建客戶地址
func CreateCustomerAddress(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID, err := uuid.Parse(c.Params("customerId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}

	// 驗證客戶屬於該租戶
	var customer models.Customer
	if err := database.DB.Where("id = ? AND tenant_id = ?", customerID, tenantID).First(&customer).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Customer not found"})
	}

	var req struct {
		CountryCode  string `json:"country_code"`
		CountryName  string `json:"country_name"`
		RegionCode   string `json:"region_code"`
		RegionName   string `json:"region_name"`
		PostalCode   string `json:"postal_code"`
		AddressLine1 string `json:"address_line1"`
		AddressLine2 string `json:"address_line2"`
		IsDefault    bool   `json:"is_default"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 如果設置為默認地址，先取消其他默認地址
	if req.IsDefault {
		database.DB.Model(&models.CustomerAddress{}).
			Where("customer_id = ? AND tenant_id = ? AND is_default = ?", customerID, tenantID, true).
			Update("is_default", false)
	}

	address := models.CustomerAddress{
		CustomerID:   customerID,
		TenantID:     tenantID,
		CountryCode:  req.CountryCode,
		CountryName:  req.CountryName,
		RegionCode:   req.RegionCode,
		RegionName:   req.RegionName,
		PostalCode:   req.PostalCode,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		IsDefault:    req.IsDefault,
	}

	if err := database.DB.Create(&address).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create address"})
	}

	return c.Status(201).JSON(address)
}

// UpdateCustomerAddress 更新客戶地址
func UpdateCustomerAddress(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID, err := uuid.Parse(c.Params("customerId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}
	addressID, err := uuid.Parse(c.Params("addressId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid address ID"})
	}

	var address models.CustomerAddress
	if err := database.DB.Where("id = ? AND customer_id = ? AND tenant_id = ?", addressID, customerID, tenantID).
		First(&address).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Address not found"})
	}

	var req struct {
		CountryCode  *string `json:"country_code"`
		CountryName  *string `json:"country_name"`
		RegionCode   *string `json:"region_code"`
		RegionName   *string `json:"region_name"`
		PostalCode   *string `json:"postal_code"`
		AddressLine1 *string `json:"address_line1"`
		AddressLine2 *string `json:"address_line2"`
		IsDefault    *bool   `json:"is_default"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.CountryCode != nil {
		address.CountryCode = *req.CountryCode
	}
	if req.CountryName != nil {
		address.CountryName = *req.CountryName
	}
	if req.RegionCode != nil {
		address.RegionCode = *req.RegionCode
	}
	if req.RegionName != nil {
		address.RegionName = *req.RegionName
	}
	if req.PostalCode != nil {
		address.PostalCode = *req.PostalCode
	}
	if req.AddressLine1 != nil {
		address.AddressLine1 = *req.AddressLine1
	}
	if req.AddressLine2 != nil {
		address.AddressLine2 = *req.AddressLine2
	}
	if req.IsDefault != nil {
		// 如果設置為默認地址，先取消其他默認地址
		if *req.IsDefault && !address.IsDefault {
			database.DB.Model(&models.CustomerAddress{}).
				Where("customer_id = ? AND tenant_id = ? AND is_default = ? AND id != ?", customerID, tenantID, true, addressID).
				Update("is_default", false)
		}
		address.IsDefault = *req.IsDefault
	}

	if err := database.DB.Save(&address).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update address"})
	}

	return c.JSON(address)
}

// DeleteCustomerAddress 刪除客戶地址
func DeleteCustomerAddress(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID, err := uuid.Parse(c.Params("customerId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}
	addressID, err := uuid.Parse(c.Params("addressId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid address ID"})
	}

	if err := database.DB.Where("id = ? AND customer_id = ? AND tenant_id = ?", addressID, customerID, tenantID).
		Delete(&models.CustomerAddress{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete address"})
	}

	return c.JSON(fiber.Map{"message": "Address deleted successfully"})
}

// GetCustomerDefaultAddress 獲取客戶默認地址（如果沒有默認地址，返回第一條）
func GetCustomerDefaultAddress(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID, err := uuid.Parse(c.Params("customerId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid customer ID"})
	}

	var address models.CustomerAddress
	// 先查找默認地址
	if err := database.DB.
		Where("tenant_id = ? AND customer_id = ? AND is_default = ?", tenantID, customerID, true).
		First(&address).Error; err != nil {
		// 如果沒有默認地址，查找第一條地址
		if err == gorm.ErrRecordNotFound {
			if err := database.DB.
				Where("tenant_id = ? AND customer_id = ?", tenantID, customerID).
				Order("created_at ASC").
				First(&address).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "No address found"})
			}
		} else {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch address"})
		}
	}

	// 獲取語言設置（從 query 參數或默認）
	lang := c.Query("lang", "zh")
	formattedAddress := address.FormatAddress(lang)

	return c.JSON(fiber.Map{
		"data":              address,
		"formatted_address": formattedAddress,
	})
}
