package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// ============================================
// 庫存調整 (Inventory Adjustment) CRUD
// ============================================

func GetInventoryAdjustments(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	
	var adjustments []models.InventoryAdjustment
	query := database.DB.Where("tenant_id = ?", tenantID)
	
	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Joins("JOIN products ON products.id = inventory_adjustments.product_id").
			Where("products.name ILIKE ? OR products.code ILIKE ? OR inventory_adjustments.reason ILIKE ?", 
				"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	
	// 過濾產品
	if productID := c.Query("product_id"); productID != "" {
		query = query.Where("product_id = ?", productID)
	}
	
	// 過濾調整類型
	if adjustmentType := c.Query("adjustment_type"); adjustmentType != "" {
		query = query.Where("adjustment_type = ?", adjustmentType)
	}
	
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit
	
	var total int64
	query.Model(&models.InventoryAdjustment{}).Count(&total)
	
	if err := query.Preload("Product").Preload("Warehouse").Preload("User").
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&adjustments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	
	return c.JSON(fiber.Map{
		"data":  adjustments,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetInventoryAdjustment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var adjustment models.InventoryAdjustment
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Product").Preload("Warehouse").Preload("User").First(&adjustment).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Inventory adjustment not found"})
	}
	
	return c.JSON(adjustment)
}

func CreateInventoryAdjustment(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	
	var req struct {
		ProductID         uuid.UUID  `json:"product_id"`
		WarehouseID       *uuid.UUID `json:"warehouse_id"`
		AdjustmentType    string     `json:"adjustment_type"` // increase, decrease, set
		Quantity          int        `json:"quantity"`
		Reason            string     `json:"reason"`
		Notes             string     `json:"notes"`
		WarehouseLocation string     `json:"warehouse_location"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.ProductID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Product ID is required"})
	}
	if req.WarehouseID != nil && *req.WarehouseID == uuid.Nil {
		req.WarehouseID = nil
	}
	
	if req.AdjustmentType != "increase" && req.AdjustmentType != "decrease" && req.AdjustmentType != "set" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid adjustment type"})
	}
	
	// 獲取當前產品庫存
	var product models.Product
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, req.ProductID).First(&product).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product not found"})
	}
	
	previousQuantity := product.StockQuantity
	var newQuantity int
	
	switch req.AdjustmentType {
	case "increase":
		newQuantity = previousQuantity + req.Quantity
	case "decrease":
		newQuantity = previousQuantity - req.Quantity
		if newQuantity < 0 {
			newQuantity = 0
		}
	case "set":
		newQuantity = req.Quantity
		if newQuantity < 0 {
			newQuantity = 0
		}
	}
	
	// 創建調整記錄
	adjustment := models.InventoryAdjustment{
		TenantID:          tenantID,
		ProductID:         req.ProductID,
		AdjustmentType:    req.AdjustmentType,
		Quantity:          req.Quantity,
		PreviousQuantity:  previousQuantity,
		NewQuantity:       newQuantity,
		Reason:            req.Reason,
		Notes:             req.Notes,
		WarehouseLocation: req.WarehouseLocation,
		WarehouseID:       req.WarehouseID,
		CreatedBy:         &userID,
		CreatedAt:         time.Now(),
	}
	
	if err := database.DB.Create(&adjustment).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create adjustment"})
	}
	
	// 更新產品庫存
	product.StockQuantity = newQuantity
	if err := database.DB.Save(&product).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update product stock"})
	}
	
	return c.Status(201).JSON(adjustment)
}

// ============================================
// 庫存盤點 (Inventory Count) CRUD
// ============================================

func GetInventoryCounts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	
	var counts []models.InventoryCount
	query := database.DB.Where("tenant_id = ?", tenantID)
	
	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("count_number ILIKE ? OR notes ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	
	// 過濾狀態
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 過濾倉庫
	if warehouseID := c.Query("warehouse_id"); warehouseID != "" {
		if wid, err := uuid.Parse(warehouseID); err == nil {
			query = query.Where("warehouse_id = ?", wid)
		}
	}
	
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit
	
	var total int64
	query.Model(&models.InventoryCount{}).Count(&total)
	
	if err := query.Preload("User").Preload("Warehouse").Preload("CountItems.Product").
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&counts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 組裝響應，附帶盤點項目數
	type CountResponse struct {
		models.InventoryCount
		ItemsCount int `json:"items_count"`
	}
	resp := make([]CountResponse, 0, len(counts))
	for _, cItem := range counts {
		resp = append(resp, CountResponse{
			InventoryCount: cItem,
			ItemsCount:     len(cItem.CountItems),
		})
	}
	
	return c.JSON(fiber.Map{
		"data":  resp,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetInventoryCount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var count models.InventoryCount
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("User").Preload("Warehouse").Preload("CountItems.Product").First(&count).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Inventory count not found"})
	}
	
	return c.JSON(fiber.Map{
		"data":        count,
		"items_count": len(count.CountItems),
	})
}

func CreateInventoryCount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	
	var req struct {
		CountNumber       string     `json:"count_number"`
		CountDate         string     `json:"count_date"`
		WarehouseID       *uuid.UUID `json:"warehouse_id"`
		WarehouseLocation string     `json:"warehouse_location"`
		Notes             string     `json:"notes"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.CountNumber == "" {
		// 自動生成盤點編號
		var count int64
		database.DB.Model(&models.InventoryCount{}).Where("tenant_id = ?", tenantID).Count(&count)
		req.CountNumber = "COUNT-" + time.Now().Format("20060102") + "-" + strconv.FormatInt(count+1, 10)
	}
	
	countDate, err := utils.ParseDateInTenantTimezone(tenantID, req.CountDate)
	if err != nil {
		countDate = utils.NowInTenantTimezone(tenantID)
	}
	
	count := models.InventoryCount{
		TenantID:          tenantID,
		CountNumber:       req.CountNumber,
		CountDate:         countDate,
		WarehouseLocation: req.WarehouseLocation,
		WarehouseID:       req.WarehouseID,
		Status:            "draft",
		Notes:             req.Notes,
		CreatedBy:         &userID,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	
	if err := database.DB.Create(&count).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create count"})
	}
	
	return c.Status(201).JSON(count)
}

func UpdateInventoryCount(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var count models.InventoryCount
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&count).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Inventory count not found"})
	}
	
	var req struct {
		Status      string     `json:"status"`
		Notes       string     `json:"notes"`
		WarehouseID *uuid.UUID `json:"warehouse_id"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	if req.Status != "" {
		count.Status = req.Status
	}
	if req.Notes != "" {
		count.Notes = req.Notes
	}
	if req.WarehouseID != nil {
		if *req.WarehouseID == uuid.Nil {
			count.WarehouseID = nil
		} else {
			count.WarehouseID = req.WarehouseID
		}
	}
	
	count.UpdatedAt = time.Now()
	
	if err := database.DB.Save(&count).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update count"})
	}
	
	// 如果狀態為 completed，更新庫存
	if count.Status == "completed" {
		var items []models.InventoryCountItem
		if err := database.DB.Where("tenant_id = ? AND count_id = ?", tenantID, id).Find(&items).Error; err == nil {
			for _, item := range items {
				// 更新產品庫存
				var product models.Product
				if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, item.ProductID).First(&product).Error; err == nil {
					product.StockQuantity = item.CountedQuantity
					database.DB.Save(&product)
					
					// 創建調整記錄
					adjustment := models.InventoryAdjustment{
						TenantID:          tenantID,
						ProductID:         item.ProductID,
						AdjustmentType:    "set",
						Quantity:          item.Variance,
						PreviousQuantity:  item.SystemQuantity,
						NewQuantity:       item.CountedQuantity,
						Reason:            "盤點調整",
						Notes:             "來自盤點: " + count.CountNumber,
						WarehouseLocation: count.WarehouseLocation,
						CreatedBy:         count.CreatedBy,
						CreatedAt:         time.Now(),
					}
					database.DB.Create(&adjustment)
				}
			}
		}
	}
	
	return c.JSON(count)
}

func AddInventoryCountItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	countID := c.Params("count_id")
	
	var req struct {
		ProductID       uuid.UUID `json:"product_id"`
		CountedQuantity int       `json:"counted_quantity"`
		Notes           string    `json:"notes"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	
	// 檢查盤點是否存在
	var count models.InventoryCount
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, countID).First(&count).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Inventory count not found"})
	}
	
	// 獲取產品當前庫存
	var product models.Product
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, req.ProductID).First(&product).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product not found"})
	}
	
	systemQuantity := product.StockQuantity
	variance := req.CountedQuantity - systemQuantity
	
	// 檢查是否已存在
	var existingItem models.InventoryCountItem
	if err := database.DB.Where("tenant_id = ? AND count_id = ? AND product_id = ?", 
		tenantID, countID, req.ProductID).First(&existingItem).Error; err == nil {
		// 更新現有記錄
		existingItem.CountedQuantity = req.CountedQuantity
		existingItem.Variance = variance
		existingItem.Notes = req.Notes
		if err := database.DB.Save(&existingItem).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update count item"})
		}
		return c.JSON(existingItem)
	}
	
	// 創建新記錄
	item := models.InventoryCountItem{
		TenantID:        tenantID,
		CountID:         uuid.MustParse(countID),
		ProductID:       req.ProductID,
		SystemQuantity:  systemQuantity,
		CountedQuantity: req.CountedQuantity,
		Variance:        variance,
		Notes:           req.Notes,
		CreatedAt:       time.Now(),
	}
	
	if err := database.DB.Create(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create count item"})
	}
	
	return c.Status(201).JSON(item)
}

func DeleteInventoryCountItem(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Delete(&models.InventoryCountItem{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete count item"})
	}
	
	return c.JSON(fiber.Map{"message": "Count item deleted"})
}

// ============================================
// 庫存預警 (Low Stock Alert)
// ============================================

func GetLowStockProducts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	threshold, _ := strconv.Atoi(c.Query("threshold", "10"))
	
	var products []models.Product
	query := database.DB.Where("tenant_id = ? AND status = ? AND stock_quantity <= ?", 
		tenantID, "active", threshold)
	
	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit
	
	var total int64
	query.Model(&models.Product{}).Count(&total)
	
	if err := query.Order("stock_quantity ASC").Offset(offset).Limit(limit).Find(&products).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	
	return c.JSON(fiber.Map{
		"data":      products,
		"total":     total,
		"page":      page,
		"limit":     limit,
		"threshold": threshold,
	})
}

// ExportLowStockProductsToExcel 導出低庫存產品到 Excel
func ExportLowStockProductsToExcel(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	threshold, _ := strconv.Atoi(c.Query("threshold", "10"))

	var products []models.Product
	query := database.DB.Where("tenant_id = ? AND status = ? AND stock_quantity <= ?",
		tenantID, "active", threshold)

	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Order("stock_quantity ASC").Find(&products).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch products"})
	}

	f := excelize.NewFile()
	defer f.Close()
	sheetName := "低庫存預警"
	f.NewSheet(sheetName)
	f.DeleteSheet("Sheet1")

	headers := []string{"編號", "產品名稱", "當前庫存", "價格", "分類"}
	for i, header := range headers {
		cell := fmt.Sprintf("%c1", 'A'+i)
		f.SetCellValue(sheetName, cell, header)
		style, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"#E0E0E0"}, Pattern: 1},
		})
		f.SetCellStyle(sheetName, cell, cell, style)
	}

	for i, product := range products {
		row := i + 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), product.Code)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), product.Name)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), product.StockQuantity)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), product.Price)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), product.Category)
	}

	for i := 0; i < len(headers); i++ {
		f.SetColWidth(sheetName, string(rune('A'+i)), string(rune('A'+i)), 15)
	}

	c.Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Set("Content-Disposition", "attachment; filename=low_stock_products.xlsx")
	if err := f.Write(c.Response().BodyWriter()); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate Excel file"})
	}
	return nil
}

// ExportLowStockProductsToPDF 導出低庫存產品到 PDF
func ExportLowStockProductsToPDF(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	threshold, _ := strconv.Atoi(c.Query("threshold", "10"))

	var products []models.Product
	query := database.DB.Where("tenant_id = ? AND status = ? AND stock_quantity <= ?",
		tenantID, "active", threshold)

	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	if err := query.Order("stock_quantity ASC").Find(&products).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch products"})
	}

	headers := []string{"編號", "產品名稱", "當前庫存", "價格", "分類"}
	rows := make([][]string, 0, len(products))
	for _, product := range products {
		rows = append(rows, []string{
			product.Code,
			product.Name,
			fmt.Sprintf("%d", product.StockQuantity),
			fmt.Sprintf("%.2f", product.Price),
			product.Category,
		})
	}
	title := fmt.Sprintf("低庫存預警（閾值：%d）", threshold)
	pdfBytes, _ := utils.BuildTablePDFBytes(title, headers, rows)
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", "attachment; filename=low_stock_products.pdf")
	return c.Send(pdfBytes)
}
