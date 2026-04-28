package utils

import (
	"nwork/internal/database"
	"nwork/internal/models"
	"time"

	"github.com/google/uuid"
)

// UpdateWarehouseStock 更新產品在指定倉庫的庫存
// operation: "increase" 或 "decrease"
func UpdateWarehouseStock(tenantID uuid.UUID, productID uuid.UUID, warehouseID uuid.UUID, quantity int, operation string) error {
	if productID == uuid.Nil || warehouseID == uuid.Nil {
		return nil // 如果沒有倉庫ID，跳過
	}

	// 查找或創建庫存記錄
	var stock models.ProductWarehouseStock
	err := database.DB.Where("tenant_id = ? AND product_id = ? AND warehouse_id = ?", tenantID, productID, warehouseID).First(&stock).Error

	if err != nil {
		// 創建新記錄
		var initialQuantity int
		if operation == "increase" {
			initialQuantity = quantity
		} else {
			initialQuantity = 0 // 減少時，如果不存在記錄，設為0
		}
		stock = models.ProductWarehouseStock{
			TenantID:      tenantID,
			ProductID:     productID,
			WarehouseID:   warehouseID,
			Quantity:      initialQuantity,
			LastUpdatedAt: time.Now(),
		}
		if err := database.DB.Create(&stock).Error; err != nil {
			return err
		}
	} else {
		// 更新現有記錄
		if operation == "increase" {
			stock.Quantity += quantity
		} else if operation == "decrease" {
			stock.Quantity -= quantity
			if stock.Quantity < 0 {
				stock.Quantity = 0
			}
		}
		stock.LastUpdatedAt = time.Now()
		if err := database.DB.Save(&stock).Error; err != nil {
			return err
		}
	}

	return nil
}
