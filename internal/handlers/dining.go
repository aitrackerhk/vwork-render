package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// isValidTableCode 驗證餐桌編號：只能是正整數，前方不能有 0
// 有效範例: "1", "2", "10", "123"
// 無效範例: "01", "007", "0", "A1", "1-2"
func isValidTableCode(code string) bool {
	// 必須是純數字
	matched, _ := regexp.MatchString(`^[1-9][0-9]*$`, code)
	return matched
}

// ============================
// 餐桌區域
// ============================

func GetDiningAreas(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var areas []models.DiningArea
	query := database.DB.Where("tenant_id = ?", tenantID)

	if storeID := c.Query("store_id"); storeID != "" {
		query = query.Where("store_id = ?", storeID)
	}

	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		query = query.Where("name ILIKE ? OR code ILIKE ?", like, like)
	}

	if active := c.Query("is_active"); active != "" {
		query = query.Where("is_active = ?", active)
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.DiningArea{}).Count(&total)

	if err := query.
		Preload("Store").
		Offset(offset).
		Limit(limit).
		Order("sort_order ASC, created_at DESC").
		Find(&areas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch dining areas: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  areas,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetDiningArea(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining area ID"})
	}

	var area models.DiningArea
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Preload("Store").First(&area).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining area not found"})
	}

	return c.JSON(area)
}

func CreateDiningArea(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}
	userID := middleware.GetUserID(c)

	var req struct {
		StoreID   *uuid.UUID  `json:"store_id"`
		Code      string      `json:"code"`
		Name      string      `json:"name"`
		MinSeats  int         `json:"min_seats"`
		MaxSeats  int         `json:"max_seats"`
		SortOrder int         `json:"sort_order"`
		IsActive  interface{} `json:"is_active"`
		Notes     string      `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Invalid request: %v", err)})
	}

	if strings.TrimSpace(req.Name) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if strings.TrimSpace(req.Code) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Code is required"})
	}

	var existing models.DiningArea
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.Code).First(&existing).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Dining area code already exists"})
	}

	// 解析 is_active（支援 bool、string "true"/"false"）
	isActive := true
	if req.IsActive != nil {
		switch v := req.IsActive.(type) {
		case bool:
			isActive = v
		case string:
			isActive = v == "true" || v == "1"
		}
	}

	now := time.Now()
	area := models.DiningArea{
		TenantID:  tenantID,
		StoreID:   req.StoreID,
		Code:      strings.TrimSpace(req.Code),
		Name:      strings.TrimSpace(req.Name),
		MinSeats:  req.MinSeats,
		MaxSeats:  req.MaxSeats,
		SortOrder: req.SortOrder,
		IsActive:  isActive,
		Notes:     req.Notes,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: &userID,
		UpdatedBy: &userID,
	}

	if area.MinSeats <= 0 {
		area.MinSeats = 1
	}
	if area.MaxSeats <= 0 {
		area.MaxSeats = area.MinSeats
	}
	if area.MaxSeats < area.MinSeats {
		area.MaxSeats = area.MinSeats
	}

	if err := database.DB.Create(&area).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create dining area"})
	}

	return c.Status(201).JSON(area)
}

func UpdateDiningArea(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining area ID"})
	}

	var area models.DiningArea
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&area).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining area not found"})
	}

	var req struct {
		StoreID   *uuid.UUID `json:"store_id"`
		Code      *string    `json:"code"`
		Name      *string    `json:"name"`
		MinSeats  *int       `json:"min_seats"`
		MaxSeats  *int       `json:"max_seats"`
		SortOrder *int       `json:"sort_order"`
		IsActive  *bool      `json:"is_active"`
		Notes     *string    `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.StoreID != nil {
		area.StoreID = req.StoreID
	}
	if req.Code != nil {
		code := strings.TrimSpace(*req.Code)
		if code == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Code is required"})
		}
		if code != area.Code {
			var existing models.DiningArea
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, code).First(&existing).Error; err == nil {
				return c.Status(400).JSON(fiber.Map{"error": "Dining area code already exists"})
			}
		}
		area.Code = code
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
		}
		area.Name = name
	}
	if req.MinSeats != nil {
		area.MinSeats = *req.MinSeats
	}
	if req.MaxSeats != nil {
		area.MaxSeats = *req.MaxSeats
	}
	if req.SortOrder != nil {
		area.SortOrder = *req.SortOrder
	}
	if req.IsActive != nil {
		area.IsActive = *req.IsActive
	}
	if req.Notes != nil {
		area.Notes = *req.Notes
	}

	if area.MinSeats <= 0 {
		area.MinSeats = 1
	}
	if area.MaxSeats <= 0 {
		area.MaxSeats = area.MinSeats
	}
	if area.MaxSeats < area.MinSeats {
		area.MaxSeats = area.MinSeats
	}

	area.UpdatedAt = time.Now()
	area.UpdatedBy = &userID

	if err := database.DB.Save(&area).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update dining area"})
	}

	return c.JSON(area)
}

func DeleteDiningArea(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining area ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.DiningArea{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete dining area"})
	}

	return c.JSON(fiber.Map{"message": "Dining area deleted"})
}

// ============================
// 餐桌
// ============================

func GetDiningTables(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var tables []models.DiningTable
	query := database.DB.Where("tenant_id = ?", tenantID)

	if storeID := c.Query("store_id"); storeID != "" {
		query = query.Where("store_id = ?", storeID)
	}
	if areaID := c.Query("area_id"); areaID != "" {
		query = query.Where("area_id = ?", areaID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if active := c.Query("is_active"); active != "" {
		query = query.Where("is_active = ?", active)
	}
	if availableOnly := c.Query("available_only"); availableOnly == "true" {
		query = query.Where("status = ?", "available")
	}
	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		query = query.Where("code ILIKE ? OR name ILIKE ?", like, like)
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.DiningTable{}).Count(&total)

	if err := query.
		Preload("Area").
		Preload("Store").
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&tables).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch dining tables: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  tables,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetDiningTable(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining table ID"})
	}

	var table models.DiningTable
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Preload("Area").Preload("Store").First(&table).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining table not found"})
	}

	return c.JSON(table)
}

func GetDiningTableByCode(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Code is required"})
	}

	query := database.DB.Where("tenant_id = ? AND code = ?", tenantID, code)
	if storeID := c.Query("store_id"); storeID != "" {
		query = query.Where("store_id = ?", storeID)
	}

	var table models.DiningTable
	if err := query.Preload("Area").Preload("Store").First(&table).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining table not found"})
	}

	return c.JSON(table)
}

func CreateDiningTable(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		StoreID  *uuid.UUID  `json:"store_id"`
		AreaID   *uuid.UUID  `json:"area_id"`
		Code     string      `json:"code"`
		Name     string      `json:"name"`
		Seats    int         `json:"seats"`
		Status   string      `json:"status"`
		IsActive interface{} `json:"is_active"`
		Notes    string      `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Invalid request: %v", err)})
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Code is required"})
	}
	// 編號只能是數字，且前方不能有 0（除非只有一位數 "0" 也不允許）
	if !isValidTableCode(code) {
		return c.Status(400).JSON(fiber.Map{"error": "編號只能是正整數，前方不能有 0"})
	}

	var existing models.DiningTable
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, code).First(&existing).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Dining table code already exists"})
	}

	// 解析 is_active（支援 bool、string "true"/"false"）
	isActive := true
	if req.IsActive != nil {
		switch v := req.IsActive.(type) {
		case bool:
			isActive = v
		case string:
			isActive = v == "true" || v == "1"
		}
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "available"
	}

	if req.Seats <= 0 {
		req.Seats = 1
	}

	now := time.Now()
	table := models.DiningTable{
		TenantID:  tenantID,
		StoreID:   req.StoreID,
		AreaID:    req.AreaID,
		Code:      strings.TrimSpace(req.Code),
		Name:      strings.TrimSpace(req.Name),
		Seats:     req.Seats,
		Status:    status,
		IsActive:  isActive,
		Notes:     req.Notes,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: &userID,
		UpdatedBy: &userID,
	}

	if err := database.DB.Create(&table).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create dining table"})
	}

	return c.Status(201).JSON(table)
}

// AutoGenerateDiningTables 自動生成餐桌（依桌區批量建立）
func AutoGenerateDiningTables(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		StoreID       *uuid.UUID  `json:"store_id"`
		TablesPerArea int         `json:"tables_per_area"`
		StartCode     int         `json:"start_code"`
		OnlyEmpty     bool        `json:"only_empty"`
		AreaIDs       []uuid.UUID `json:"area_ids"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.TablesPerArea <= 0 {
		req.TablesPerArea = 10
	}

	// 取得桌區
	var areas []models.DiningArea
	areaQuery := database.DB.Where("tenant_id = ?", tenantID)
	if req.StoreID != nil && *req.StoreID != uuid.Nil {
		areaQuery = areaQuery.Where("store_id = ?", req.StoreID)
	}
	if len(req.AreaIDs) > 0 {
		areaQuery = areaQuery.Where("id IN ?", req.AreaIDs)
	}
	if err := areaQuery.Order("sort_order ASC, created_at ASC").Find(&areas).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch dining areas"})
	}
	if len(areas) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No dining areas found"})
	}

	// 取得現有桌號最大值
	var codes []string
	codeQuery := database.DB.Model(&models.DiningTable{}).Where("tenant_id = ?", tenantID)
	if req.StoreID != nil && *req.StoreID != uuid.Nil {
		codeQuery = codeQuery.Where("store_id = ?", req.StoreID)
	}
	_ = codeQuery.Pluck("code", &codes).Error
	maxCode := 0
	for _, c := range codes {
		if v, err := strconv.Atoi(strings.TrimSpace(c)); err == nil && v > maxCode {
			maxCode = v
		}
	}
	startCode := req.StartCode
	if startCode <= 0 {
		startCode = maxCode + 1
	}
	if startCode <= maxCode {
		startCode = maxCode + 1
	}

	created := 0
	now := time.Now()
	currentCode := startCode

	for _, area := range areas {
		if req.OnlyEmpty {
			var count int64
			database.DB.Model(&models.DiningTable{}).Where("tenant_id = ? AND area_id = ?", tenantID, area.ID).Count(&count)
			if count > 0 {
				continue
			}
		}
		seats := area.MaxSeats
		if seats <= 0 {
			seats = area.MinSeats
		}
		if seats <= 0 {
			seats = 1
		}
		for i := 0; i < req.TablesPerArea; i++ {
			code := strconv.Itoa(currentCode)
			currentCode++
			storeID := req.StoreID
			if storeID == nil || *storeID == uuid.Nil {
				storeID = area.StoreID
			}
			table := models.DiningTable{
				TenantID:  tenantID,
				StoreID:   storeID,
				AreaID:    &area.ID,
				Code:      code,
				Seats:     seats,
				Status:    "available",
				IsActive:  true,
				CreatedAt: now,
				UpdatedAt: now,
				CreatedBy: &userID,
				UpdatedBy: &userID,
			}
			if err := database.DB.Create(&table).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to create dining tables"})
			}
			created++
		}
	}

	return c.JSON(fiber.Map{
		"message":    "Auto generated dining tables",
		"created":    created,
		"start_code": startCode,
	})
}

func UpdateDiningTable(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining table ID"})
	}

	var table models.DiningTable
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&table).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining table not found"})
	}

	var req struct {
		StoreID  *uuid.UUID `json:"store_id"`
		AreaID   *uuid.UUID `json:"area_id"`
		Code     *string    `json:"code"`
		Name     *string    `json:"name"`
		Seats    *int       `json:"seats"`
		Status   *string    `json:"status"`
		IsActive *bool      `json:"is_active"`
		Notes    *string    `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.StoreID != nil {
		table.StoreID = req.StoreID
	}
	if req.AreaID != nil {
		table.AreaID = req.AreaID
	}
	if req.Code != nil {
		code := strings.TrimSpace(*req.Code)
		if code == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Code is required"})
		}
		// 編號只能是數字，且前方不能有 0
		if !isValidTableCode(code) {
			return c.Status(400).JSON(fiber.Map{"error": "編號只能是正整數，前方不能有 0"})
		}
		if code != table.Code {
			var existing models.DiningTable
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, code).First(&existing).Error; err == nil {
				return c.Status(400).JSON(fiber.Map{"error": "Dining table code already exists"})
			}
		}
		table.Code = code
	}
	if req.Name != nil {
		table.Name = strings.TrimSpace(*req.Name)
	}
	if req.Seats != nil {
		if *req.Seats > 0 {
			table.Seats = *req.Seats
		}
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != "" {
			table.Status = status
		}
	}
	if req.IsActive != nil {
		table.IsActive = *req.IsActive
	}
	if req.Notes != nil {
		table.Notes = *req.Notes
	}

	table.UpdatedAt = time.Now()
	table.UpdatedBy = &userID

	if err := database.DB.Save(&table).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update dining table"})
	}

	return c.JSON(table)
}

func DeleteDiningTable(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining table ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.DiningTable{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete dining table"})
	}

	return c.JSON(fiber.Map{"message": "Dining table deleted"})
}

func ReleaseDiningTable(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining table ID"})
	}

	var table models.DiningTable
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&table).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining table not found"})
	}

	now := time.Now()
	table.Status = "available"
	table.UpdatedAt = now
	table.UpdatedBy = &userID
	if err := database.DB.Save(&table).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to release dining table"})
	}

	// 將最新的已入座候位標記為取消（若存在）
	var queue models.DiningQueue
	if err := database.DB.Where("tenant_id = ? AND table_id = ? AND status = ?", tenantID, table.ID, "seated").
		Order("seated_at DESC, created_at DESC").
		First(&queue).Error; err == nil {
		queue.Status = "cancelled"
		queue.CancelledAt = &now
		queue.UpdatedAt = now
		queue.UpdatedBy = &userID
		_ = database.DB.Save(&queue).Error
	}

	return c.JSON(fiber.Map{"message": "released"})
}

// ============================
// 排隊候位
// ============================

func GetDiningQueues(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var queues []models.DiningQueue
	query := database.DB.Where("tenant_id = ?", tenantID)

	if storeID := c.Query("store_id"); storeID != "" {
		query = query.Where("store_id = ?", storeID)
	}
	if areaID := c.Query("area_id"); areaID != "" {
		query = query.Where("area_id = ?", areaID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if search := c.Query("search"); search != "" {
		like := "%" + search + "%"
		query = query.Where("name ILIKE ? OR phone ILIKE ?", like, like)
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.DiningQueue{}).Count(&total)

	if err := query.
		Preload("Table").
		Preload("Table.Area").
		Preload("Store").
		Preload("Area").
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&queues).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch dining queues: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  queues,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetDiningQueue(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining queue ID"})
	}

	var queue models.DiningQueue
	if err := database.DB.
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Preload("Area").
		Preload("Table").
		Preload("Table.Area").
		Preload("Store").
		First(&queue).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining queue not found"})
	}

	return c.JSON(queue)
}

func generateDiningQueueTicket(tenantID uuid.UUID, storeID *uuid.UUID, areaID *uuid.UUID, now time.Time) (string, int, error) {
	return nextDiningQueueTicket(tenantID, storeID, areaID, now, true)
}

func CreateDiningQueue(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		StoreID      *uuid.UUID `json:"store_id"`
		AreaID       *uuid.UUID `json:"area_id"`
		TicketNumber *string    `json:"ticket_number"`
		Name         string     `json:"name"`
		Phone        string     `json:"phone"`
		PartySize    int        `json:"party_size"`
		Status       string     `json:"status"`
		TableID      *uuid.UUID `json:"table_id"`
		Notes        string     `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.PartySize <= 0 {
		req.PartySize = 1
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "waiting"
	}

	now := time.Now()
	var ticketNumber string
	var ticketSeq int
	if req.TicketNumber != nil && strings.TrimSpace(*req.TicketNumber) != "" {
		ticketNumber = strings.TrimSpace(*req.TicketNumber)
		// 嘗試從末段解析序號（例如 Q-20260122-001）
		if req.AreaID != nil {
			areaCode := getDiningAreaCode(tenantID, req.AreaID)
			expectedPrefix := buildDiningQueueTicketPrefix(areaCode, now)
			if !strings.HasPrefix(ticketNumber, expectedPrefix) {
				ticketNumber = ""
			}
		}
		if ticketNumber != "" && isDiningQueueTicketInUse(tenantID, ticketNumber) {
			ticketNumber = ""
		}
		if ticketNumber != "" {
			if seq, ok := parseDiningQueueTicketSeq(ticketNumber); ok {
				ticketSeq = seq
			}
		}
		if ticketNumber == "" {
			var err error
			ticketNumber, ticketSeq, err = generateDiningQueueTicket(tenantID, req.StoreID, req.AreaID, now)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to generate queue ticket"})
			}
		} else if ticketSeq == 0 {
			var err error
			_, ticketSeq, err = generateDiningQueueTicket(tenantID, req.StoreID, req.AreaID, now)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to generate queue ticket"})
			}
		}
	} else {
		var err error
		ticketNumber, ticketSeq, err = generateDiningQueueTicket(tenantID, req.StoreID, req.AreaID, now)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate queue ticket"})
		}
	}
	queue := models.DiningQueue{
		TenantID:     tenantID,
		StoreID:      req.StoreID,
		AreaID:       req.AreaID,
		Name:         strings.TrimSpace(req.Name),
		Phone:        strings.TrimSpace(req.Phone),
		PartySize:    req.PartySize,
		Status:       status,
		TableID:      req.TableID,
		Notes:        req.Notes,
		TicketNumber: ticketNumber,
		TicketSeq:    ticketSeq,
		CreatedAt:    now,
		UpdatedAt:    now,
		CreatedBy:    &userID,
		UpdatedBy:    &userID,
	}

	if req.TableID != nil {
		if table, err := getDiningTableByID(tenantID, *req.TableID); err == nil {
			queue.TableCode = table.Code
			if table.AreaID != nil {
				queue.AreaID = table.AreaID
			}
			if status == "waiting" {
				queue.Status = "seated"
			}
			seatedAt := now
			queue.SeatedAt = &seatedAt
			setDiningTableStatus(tenantID, table.ID, "occupied")
		}
	}

	if err := database.DB.Create(&queue).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create dining queue"})
	}

	return c.Status(201).JSON(queue)
}

func UpdateDiningQueue(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining queue ID"})
	}

	var queue models.DiningQueue
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&queue).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining queue not found"})
	}

	prevTableID := queue.TableID

	var req struct {
		StoreID   *uuid.UUID `json:"store_id"`
		AreaID    *uuid.UUID `json:"area_id"`
		Name      *string    `json:"name"`
		Phone     *string    `json:"phone"`
		PartySize *int       `json:"party_size"`
		Status    *string    `json:"status"`
		TableID   *uuid.UUID `json:"table_id"`
		Notes     *string    `json:"notes"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.StoreID != nil {
		queue.StoreID = req.StoreID
	}
	if req.AreaID != nil {
		queue.AreaID = req.AreaID
	}
	if req.Name != nil {
		queue.Name = strings.TrimSpace(*req.Name)
	}
	if req.Phone != nil {
		queue.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.PartySize != nil {
		if *req.PartySize > 0 {
			queue.PartySize = *req.PartySize
		}
	}
	if req.Status != nil {
		queue.Status = strings.TrimSpace(*req.Status)
	}
	if req.TableID != nil {
		queue.TableID = req.TableID
		if req.TableID != nil {
			if table, err := getDiningTableByID(tenantID, *req.TableID); err == nil {
				queue.TableCode = table.Code
				if table.AreaID != nil {
					queue.AreaID = table.AreaID
				}
			}
		}
	}
	if req.Notes != nil {
		queue.Notes = *req.Notes
	}

	now := time.Now()
	queue.UpdatedAt = now
	queue.UpdatedBy = &userID

	// 若狀態改為 seated，設 seated_at 並更新桌況
	if queue.Status == "seated" && queue.SeatedAt == nil {
		queue.SeatedAt = &now
	}
	if queue.Status == "cancelled" && queue.CancelledAt == nil {
		queue.CancelledAt = &now
	}

	if err := database.DB.Save(&queue).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update dining queue"})
	}

	// 更新桌況
	if prevTableID != nil && (queue.TableID == nil || *prevTableID != *queue.TableID) {
		setDiningTableStatus(tenantID, *prevTableID, "available")
	}
	if queue.TableID != nil {
		if queue.Status == "seated" {
			setDiningTableStatus(tenantID, *queue.TableID, "occupied")
		} else if queue.Status == "cancelled" {
			setDiningTableStatus(tenantID, *queue.TableID, "available")
		}
	}

	return c.JSON(queue)
}

func DeleteDiningQueue(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining queue ID"})
	}

	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.DiningQueue{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete dining queue"})
	}

	return c.JSON(fiber.Map{"message": "Dining queue deleted"})
}

// 排隊入座
func SeatDiningQueue(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid dining queue ID"})
	}

	var req struct {
		TableID uuid.UUID `json:"table_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.TableID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "table_id is required"})
	}

	var queue models.DiningQueue
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&queue).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining queue not found"})
	}

	table, err := getDiningTableByID(tenantID, req.TableID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Dining table not found"})
	}

	now := time.Now()
	queue.TableID = &table.ID
	queue.TableCode = table.Code
	queue.Status = "seated"
	queue.SeatedAt = &now
	queue.UpdatedAt = now
	queue.UpdatedBy = &userID

	if err := database.DB.Save(&queue).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update dining queue"})
	}

	setDiningTableStatus(tenantID, table.ID, "occupied")

	return c.JSON(queue)
}

// helper
func getDiningTableByID(tenantID uuid.UUID, id uuid.UUID) (*models.DiningTable, error) {
	var table models.DiningTable
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Preload("Area").First(&table).Error; err != nil {
		return nil, err
	}
	return &table, nil
}

func setDiningTableStatus(tenantID uuid.UUID, id uuid.UUID, status string) {
	if id == uuid.Nil || strings.TrimSpace(status) == "" {
		return
	}
	database.DB.Model(&models.DiningTable{}).
		Where("tenant_id = ? AND id = ?", tenantID, id).
		Update("status", status)
}
