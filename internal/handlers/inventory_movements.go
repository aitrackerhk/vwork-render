package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func parseMovementDate(v interface{}, loc *time.Location) (time.Time, bool) {
	if v == nil {
		return time.Time{}, false
	}
	switch t := v.(type) {
	case time.Time:
		return t.In(loc), true
	case string:
		s := t
		if s == "" {
			return time.Time{}, false
		}
		// 常見：YYYY-MM-DD
		if tt, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			return tt, true
		}
		// 兼容：RFC3339 / ISO
		if tt, err := time.Parse(time.RFC3339, s); err == nil {
			return tt.In(loc), true
		}
		// 最後嘗試：把 interface 轉字串（有些 JSONB 會變成其它型別）
		ss := fmt.Sprintf("%v", v)
		if tt, err := time.ParseInLocation("2006-01-02", ss, loc); err == nil {
			return tt, true
		}
		return time.Time{}, false
	default:
		ss := fmt.Sprintf("%v", v)
		if ss == "" {
			return time.Time{}, false
		}
		if tt, err := time.ParseInLocation("2006-01-02", ss, loc); err == nil {
			return tt, true
		}
		if tt, err := time.Parse(time.RFC3339, ss); err == nil {
			return tt.In(loc), true
		}
		return time.Time{}, false
	}
}

// GetInventoryMovements 獲取所有出/入貨記錄（從訂單和採購單中提取）
func GetInventoryMovements(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	// 獲取查詢參數
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset := (page - 1) * limit

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	warehouseID := c.Query("warehouse_id")
	productID := c.Query("product_id")
	movementType := c.Query("type") // "shipping" 或 "receiving"

	var movements []map[string]interface{}

	loc := utils.GetTenantLocation(tenantID)
	var startT, endT time.Time
	var hasStart, hasEnd bool
	if startDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", startDate, loc); err == nil {
			startT = t
			hasStart = true
		}
	}
	if endDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", endDate, loc); err == nil {
			endT = t
			hasEnd = true
		}
	}

	// 獲取出貨記錄（從訂單的 shipping_notes）
	if movementType == "" || movementType == "shipping" {
		var orders []models.Order
		query := database.DB.Where("tenant_id = ?", tenantID)

		if err := query.Find(&orders).Error; err == nil {
			for _, order := range orders {
				if order.ExtraFields != nil {
					fields := map[string]interface{}(order.ExtraFields)
					if shippingNotes, exists := fields["shipping_notes"]; exists {
						if notesList, ok := shippingNotes.([]interface{}); ok {
							for _, noteInterface := range notesList {
								if note, ok := noteInterface.(map[string]interface{}); ok {
									// 日期過濾（以 shipping_date 為準，不用 order_date）
									if hasStart || hasEnd {
										if noteT, ok := parseMovementDate(note["shipping_date"], loc); ok {
											if hasStart && noteT.Before(startT) {
												continue
											}
											if hasEnd && noteT.After(endT) {
												continue
											}
										}
									}

									if items, exists := note["items"].([]interface{}); exists {
										for _, itemInterface := range items {
											if item, ok := itemInterface.(map[string]interface{}); ok {
												// 過濾條件
												if warehouseID != "" {
													if noteWarehouseID, ok := note["warehouse_id"].(string); !ok || noteWarehouseID != warehouseID {
														continue
													}
												}
												if productID != "" {
													if itemProductID, ok := item["product_id"].(string); !ok || itemProductID != productID {
														continue
													}
												}

												// 獲取產品屬性（如果存在）
												var productAttributes interface{} = nil
												if itemAttrs, exists := item["product_attributes"]; exists {
													productAttributes = itemAttrs
												}
												
												movement := map[string]interface{}{
													"id":                uuid.New().String(),
													"type":              "shipping",
													"date":              note["shipping_date"],
													"warehouse_id":      note["warehouse_id"],
													"product_id":        item["product_id"],
													"quantity":          item["quantity"],
													"product_attributes": productAttributes,
													"order_id":          order.ID.String(),
													"order_number":      order.OrderNumber,
													"reference_type":    "order",
													"reference_id":      order.ID.String(),
													"notes":             note["notes"],
													"created_at":        order.CreatedAt,
												}
												movements = append(movements, movement)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 獲取出貨記錄（從採購單取消採購的 cancellation_shipping_notes：退回貨物出庫）
	if movementType == "" || movementType == "shipping" {
		var purchaseOrders []models.PurchaseOrder
		query := database.DB.Where("tenant_id = ?", tenantID)

		if err := query.Find(&purchaseOrders).Error; err == nil {
			for _, po := range purchaseOrders {
				if po.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(po.ExtraFields)
				if shippingNotes, exists := fields["cancellation_shipping_notes"]; exists {
					if notesList, ok := shippingNotes.([]interface{}); ok {
						for _, noteInterface := range notesList {
							note, ok := noteInterface.(map[string]interface{})
							if !ok {
								continue
							}

							// 日期過濾（以 shipping_date 為準）
							if hasStart || hasEnd {
								if noteT, ok := parseMovementDate(note["shipping_date"], loc); ok {
									if hasStart && noteT.Before(startT) {
										continue
									}
									if hasEnd && noteT.After(endT) {
										continue
									}
								}
							}

							if items, exists := note["items"].([]interface{}); exists {
								for _, itemInterface := range items {
									item, ok := itemInterface.(map[string]interface{})
									if !ok {
										continue
									}

									// 過濾條件
									if warehouseID != "" {
										if noteWarehouseID, ok := note["warehouse_id"].(string); !ok || noteWarehouseID != warehouseID {
											continue
										}
									}
									if productID != "" {
										if itemProductID, ok := item["product_id"].(string); !ok || itemProductID != productID {
											continue
										}
									}

									var productAttributes interface{} = nil
									if itemAttrs, exists := item["product_attributes"]; exists {
										productAttributes = itemAttrs
									}

									shipNo, _ := note["shipping_number"].(string)
									movement := map[string]interface{}{
										"id":                 uuid.New().String(),
										"type":               "shipping",
										"date":               note["shipping_date"],
										"warehouse_id":       note["warehouse_id"],
										"product_id":         item["product_id"],
										"quantity":           item["quantity"],
										"product_attributes": productAttributes,
										"purchase_order_id":  po.ID.String(),
										// 用出貨單號顯示在來源單號欄位（點擊仍會打開採購單）
										"order_number":    shipNo,
										"reference_type":  "purchase_order",
										"reference_id":    po.ID.String(),
										"notes":           note["notes"],
										"created_at":      po.CreatedAt,
									}
									if movement["order_number"] == "" {
										movement["order_number"] = po.OrderNumber
									}
									movements = append(movements, movement)
								}
							}
						}
					}
				}
			}
		}
	}

	// 獲取入貨記錄（從訂單的 receiving_notes：例如訂單退款取回產品直接入庫）
	// 注意：此類入貨不是採購單，不會出現在 purchase_orders.receiving_notes
	if movementType == "" || movementType == "receiving" {
		var orders []models.Order
		query := database.DB.Where("tenant_id = ?", tenantID)

		if err := query.Find(&orders).Error; err == nil {
			for _, order := range orders {
				if order.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(order.ExtraFields)
				if receivingNotes, exists := fields["receiving_notes"]; exists {
					if notesList, ok := receivingNotes.([]interface{}); ok {
						for _, noteInterface := range notesList {
							note, ok := noteInterface.(map[string]interface{})
							if !ok {
								continue
							}

							// 日期過濾（使用 receiving_date）
							if hasStart || hasEnd {
								if noteT, ok := parseMovementDate(note["receiving_date"], loc); ok {
									if hasStart && noteT.Before(startT) {
										continue
									}
									if hasEnd && noteT.After(endT) {
										continue
									}
								}
							}

							if items, exists := note["items"].([]interface{}); exists {
								for _, itemInterface := range items {
									item, ok := itemInterface.(map[string]interface{})
									if !ok {
										continue
									}

									// 過濾條件
									if warehouseID != "" {
										if noteWarehouseID, ok := note["warehouse_id"].(string); !ok || noteWarehouseID != warehouseID {
											continue
										}
									}
									if productID != "" {
										if itemProductID, ok := item["product_id"].(string); !ok || itemProductID != productID {
											continue
										}
									}

									// 獲取產品屬性（如果存在）
									var productAttributes interface{} = nil
									if itemAttrs, exists := item["product_attributes"]; exists {
										productAttributes = itemAttrs
									}

									movement := map[string]interface{}{
										"id":                 uuid.New().String(),
										"type":               "receiving",
										"date":               note["receiving_date"],
										"warehouse_id":       note["warehouse_id"],
										"product_id":         item["product_id"],
										"quantity":           item["quantity"],
										"product_attributes": productAttributes,
										"order_id":           order.ID.String(),
										"order_number":       order.OrderNumber,
										"reference_type":     "order",
										"reference_id":       order.ID.String(),
										"notes":              note["notes"],
										"created_at":         order.CreatedAt,
									}
									movements = append(movements, movement)
								}
							}
						}
					}
				}
			}
		}
	}

	// 獲取收貨記錄（從採購單的 receiving_notes）
	if movementType == "" || movementType == "receiving" {
		var purchaseOrders []models.PurchaseOrder
		query := database.DB.Where("tenant_id = ?", tenantID)

		if err := query.Find(&purchaseOrders).Error; err == nil {
			for _, po := range purchaseOrders {
				if po.ExtraFields != nil {
					fields := map[string]interface{}(po.ExtraFields)
					if receivingNotes, exists := fields["receiving_notes"]; exists {
						if notesList, ok := receivingNotes.([]interface{}); ok {
							for _, noteInterface := range notesList {
								if note, ok := noteInterface.(map[string]interface{}); ok {
									// 日期過濾（以 receiving_date 為準，不用 order_date）
									if hasStart || hasEnd {
										if noteT, ok := parseMovementDate(note["receiving_date"], loc); ok {
											if hasStart && noteT.Before(startT) {
												continue
											}
											if hasEnd && noteT.After(endT) {
												continue
											}
										}
									}

									if items, exists := note["items"].([]interface{}); exists {
										for _, itemInterface := range items {
											if item, ok := itemInterface.(map[string]interface{}); ok {
												// 過濾條件
												if warehouseID != "" {
													if noteWarehouseID, ok := note["warehouse_id"].(string); !ok || noteWarehouseID != warehouseID {
														continue
													}
												}
												if productID != "" {
													if itemProductID, ok := item["product_id"].(string); !ok || itemProductID != productID {
														continue
													}
												}

												// 獲取產品屬性（如果存在）
												var productAttributes interface{} = nil
												if itemAttrs, exists := item["product_attributes"]; exists {
													productAttributes = itemAttrs
												}
												
												movement := map[string]interface{}{
													"id":                uuid.New().String(),
													"type":              "receiving",
													"date":              note["receiving_date"],
													"warehouse_id":      note["warehouse_id"],
													"product_id":        item["product_id"],
													"quantity":          item["quantity"],
													"product_attributes": productAttributes,
													"purchase_order_id": po.ID.String(),
													"order_number":      po.OrderNumber,
													"reference_type":    "purchase_order",
													"reference_id":      po.ID.String(),
													"notes":             note["notes"],
													"created_at":        po.CreatedAt,
												}
												movements = append(movements, movement)
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 排序（按日期降序）
	for i := 0; i < len(movements)-1; i++ {
		for j := i + 1; j < len(movements); j++ {
			dateI, _ := parseMovementDate(movements[i]["date"], loc)
			dateJ, _ := parseMovementDate(movements[j]["date"], loc)
			if dateI.Before(dateJ) {
				movements[i], movements[j] = movements[j], movements[i]
			}
		}
	}

	// 分頁
	total := len(movements)
	start := offset
	end := offset + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	var paginatedMovements []map[string]interface{}
	if start < end {
		paginatedMovements = movements[start:end]
	}

	return c.JSON(fiber.Map{
		"data":  paginatedMovements,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
