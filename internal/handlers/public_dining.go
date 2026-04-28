package handlers

import (
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"
)

func canAccessDiningMenu(tenant *models.Tenant) bool {
	if tenant == nil {
		return false
	}
	if tenant.WebsiteEnabled {
		return true
	}
	if tenant.ID == uuid.Nil {
		return false
	}
	var count int64
	if err := database.DB.Model(&models.TenantModule{}).
		Where("tenant_id = ? AND module_code IN ? AND is_enabled = ?", tenant.ID, []string{"dining", "food_and_beverage"}, true).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func RenderPublicDiningMenu(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !canAccessDiningMenu(tenant) {
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "暫時沒有此頁",
		}, "layouts/embed_layout")
	}

	tenantLogo := "/static/vworkicon.png"
	if tenant.ExtraFields != nil {
		if icon, ok := tenant.ExtraFields["website_icon"].(string); ok && strings.TrimSpace(icon) != "" {
			tenantLogo = strings.TrimSpace(icon)
		}
	}

	menuCategory := strings.TrimSpace(models.GetSystemSetting(diningMenuCategoryKey, diningMenuCategoryDefault))

	diningTableID := strings.TrimSpace(c.Query("dining_table_id"))
	if diningTableID == "" {
		diningTableID = strings.TrimSpace(c.Query("table_id"))
	}
	diningTableCode := strings.TrimSpace(c.Params("code"))
	if diningTableCode == "" {
		diningTableCode = strings.TrimSpace(c.Query("dining_table_code"))
	}
	if diningTableCode == "" {
		diningTableCode = strings.TrimSpace(c.Query("table_code"))
	}
	if diningTableCode == "" && diningTableID != "" {
		if id, err := uuid.Parse(diningTableID); err == nil {
			var table models.DiningTable
			if err := database.DB.Select("code").Where("id = ? AND tenant_id = ?", id, tenant.ID).First(&table).Error; err == nil {
				diningTableCode = table.Code
			}
		}
	}
	var categories []string
	if menuCategory != "" {
		raw := strings.FieldsFunc(menuCategory, func(r rune) bool {
			return r == ',' || r == '，' || r == '、' || r == ';' || r == '\n' || r == '\t'
		})
		for _, c := range raw {
			if s := strings.TrimSpace(c); s != "" {
				categories = append(categories, s)
			}
		}
	}

	query := database.DB.Where("tenant_id = ? AND status = ?", tenant.ID, "active")
	if len(categories) == 1 {
		query = query.Where("category = ?", categories[0])
	} else if len(categories) > 1 {
		query = query.Where("category IN ?", categories)
	}

	var products []models.Product
	if err := query.Order("created_at DESC").Find(&products).Error; err != nil {
		products = []models.Product{}
	}

	diningQueueID := strings.TrimSpace(c.Query("dining_queue_id"))
	if diningQueueID == "" {
		diningQueueID = strings.TrimSpace(c.Query("queue_id"))
	}
	diningTicketNumber := strings.TrimSpace(c.Query("dining_ticket_number"))
	if diningTicketNumber == "" {
		diningTicketNumber = strings.TrimSpace(c.Query("ticket_number"))
	}

	return c.Render("pages/public_dining_menu", fiber.Map{
		"Title":              "餐飲點餐",
		"Tenant":             tenant,
		"TenantName":         tenant.Name,
		"TenantLogo":         tenantLogo,
		"Subdomain":          subdomain,
		"Products":           products,
		"MenuCategory":       menuCategory,
		"DiningTableID":      diningTableID,
		"DiningTableCode":    diningTableCode,
		"DiningQueueID":      diningQueueID,
		"DiningTicketNumber": diningTicketNumber,
		"MenuOnly":           false,
		"MenuOnlyMessage":    "",
	}, "layouts/public_embed_layout")
}

// RenderPublicDiningMenuOnly 渲染只讀菜單頁面（只看不能下單）
func RenderPublicDiningMenuOnly(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !canAccessDiningMenu(tenant) {
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "暫時沒有此頁",
		}, "layouts/embed_layout")
	}

	tenantLogo := "/static/vworkicon.png"
	if tenant.ExtraFields != nil {
		if icon, ok := tenant.ExtraFields["website_icon"].(string); ok && strings.TrimSpace(icon) != "" {
			tenantLogo = strings.TrimSpace(icon)
		}
	}

	menuCategory := strings.TrimSpace(models.GetSystemSetting(diningMenuCategoryKey, diningMenuCategoryDefault))
	menuOnlyMessage := strings.TrimSpace(models.GetSystemSetting(diningMenuOnlyMessageKey, diningMenuOnlyMessageDefault))

	var categories []string
	if menuCategory != "" {
		raw := strings.FieldsFunc(menuCategory, func(r rune) bool {
			return r == ',' || r == '，' || r == '、' || r == ';' || r == '\n' || r == '\t'
		})
		for _, c := range raw {
			if s := strings.TrimSpace(c); s != "" {
				categories = append(categories, s)
			}
		}
	}

	query := database.DB.Where("tenant_id = ? AND status = ?", tenant.ID, "active")
	if len(categories) == 1 {
		query = query.Where("category = ?", categories[0])
	} else if len(categories) > 1 {
		query = query.Where("category IN ?", categories)
	}

	var products []models.Product
	if err := query.Order("created_at DESC").Find(&products).Error; err != nil {
		products = []models.Product{}
	}

	return c.Render("pages/public_dining_menu", fiber.Map{
		"Title":              "菜單",
		"Tenant":             tenant,
		"TenantName":         tenant.Name,
		"TenantLogo":         tenantLogo,
		"Subdomain":          subdomain,
		"Products":           products,
		"MenuCategory":       menuCategory,
		"DiningTableID":      "",
		"DiningTableCode":    "",
		"DiningQueueID":      "",
		"DiningTicketNumber": "",
		"MenuOnly":           true,
		"MenuOnlyMessage":    menuOnlyMessage,
	}, "layouts/public_embed_layout")
}

type publicDiningOrderItem struct {
	ProductID  string  `json:"product_id"`
	Quantity   float64 `json:"quantity"`
	Attributes []struct {
		AttributeID string `json:"attribute_id"`
		Value       string `json:"value"`
	} `json:"product_attributes"`
}

type publicDiningOrderReq struct {
	Items              []publicDiningOrderItem `json:"items"`
	DiningTableID      string                  `json:"dining_table_id"`
	DiningTableCode    string                  `json:"dining_table_code"`
	DiningQueueID      string                  `json:"dining_queue_id"`
	DiningTicketNumber string                  `json:"dining_ticket_number"`
}

func PublicCreateDiningOrder(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !tenant.WebsiteEnabled {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req publicDiningOrderReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if len(req.Items) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Items is required"})
	}

	// resolve table
	var table models.DiningTable
	var tableID *uuid.UUID
	if strings.TrimSpace(req.DiningTableID) != "" {
		if id, err := uuid.Parse(strings.TrimSpace(req.DiningTableID)); err == nil {
			tableID = &id
			_ = database.DB.Where("id = ? AND tenant_id = ?", id, tenant.ID).First(&table).Error
		}
	} else if strings.TrimSpace(req.DiningTableCode) != "" {
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenant.ID, strings.TrimSpace(req.DiningTableCode)).First(&table).Error; err == nil {
			id := table.ID
			tableID = &id
		}
	}
	if tableID == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Dining table is required"})
	}

	// resolve queue by table if missing
	queueID := strings.TrimSpace(req.DiningQueueID)
	ticketNumber := strings.TrimSpace(req.DiningTicketNumber)
	if queueID == "" {
		var queue models.DiningQueue
		if err := database.DB.
			Where("tenant_id = ? AND table_id = ? AND status = ?", tenant.ID, *tableID, "seated").
			Order("updated_at DESC").
			First(&queue).Error; err == nil {
			queueID = queue.ID.String()
			ticketNumber = queue.TicketNumber
		}
	}

	orderNumber, err := utils.GenerateOrderNumber(tenant.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate order number"})
	}

	now := utils.NowInTenantTimezone(tenant.ID)

	extraFields := map[string]interface{}{
		"source_type":       "dining",
		"dining_table_id":   tableID.String(),
		"dining_table_code": strings.TrimSpace(req.DiningTableCode),
	}
	if queueID != "" {
		extraFields["dining_queue_id"] = queueID
	}
	if ticketNumber != "" {
		extraFields["dining_ticket_number"] = ticketNumber
	}

	order := models.Order{
		TenantID:    tenant.ID,
		OrderNumber: orderNumber,
		OrderDate:   now,
		Status:      "confirmed",
		SourceType:  "dining",
		StoreID:     table.StoreID,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExtraFields: models.JSONB(extraFields),
	}

	var total float64
	var orderItems []models.OrderItem
	for _, it := range req.Items {
		pid, err := uuid.Parse(strings.TrimSpace(it.ProductID))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid product_id"})
		}
		if it.Quantity <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid quantity"})
		}
		var p models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ?", pid, tenant.ID, "active").First(&p).Error; err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Product not found"})
		}
		lineTotal := it.Quantity * p.Price
		total += lineTotal

		itemExtra := models.JSONB{}
		if len(it.Attributes) > 0 {
			attrs := make([]map[string]interface{}, len(it.Attributes))
			for i, a := range it.Attributes {
				attrs[i] = map[string]interface{}{
					"attribute_id": strings.TrimSpace(a.AttributeID),
					"value":        strings.TrimSpace(a.Value),
				}
			}
			itemExtra["product_attributes"] = attrs
		}

		orderItems = append(orderItems, models.OrderItem{
			TenantID:    tenant.ID,
			ProductID:   &pid,
			Quantity:    it.Quantity,
			UnitPrice:   p.Price,
			TotalPrice:  lineTotal,
			ExtraFields: itemExtra,
			CreatedAt:   now,
		})
	}
	order.TotalAmount = total

	tx := database.DB.Begin()
	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create order"})
	}
	for _, oi := range orderItems {
		oi.OrderID = order.ID
		if err := tx.Create(&oi).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create order item"})
		}
	}
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit order"})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"order_id":     order.ID.String(),
		"order_number": order.OrderNumber,
	})
}

func PublicGetDiningOrder(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !tenant.WebsiteEnabled {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	queueID := strings.TrimSpace(c.Query("dining_queue_id"))
	if queueID == "" {
		queueID = strings.TrimSpace(c.Query("queue_id"))
	}
	tableID := strings.TrimSpace(c.Query("dining_table_id"))
	if tableID == "" {
		tableID = strings.TrimSpace(c.Query("table_id"))
	}

	if queueID == "" && tableID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "dining_queue_id or dining_table_id is required"})
	}

	query := database.DB.Where("tenant_id = ? AND source_type = ?", tenant.ID, "dining")
	if queueID != "" {
		query = query.Where("extra_fields->>'dining_queue_id' = ?", queueID)
	} else if tableID != "" {
		query = query.Where("extra_fields->>'dining_table_id' = ?", tableID)
	}

	var order models.Order
	if err := query.Order("created_at DESC").First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	var items []models.OrderItem
	_ = database.DB.Where("order_id = ?", order.ID).Preload("Product").Find(&items).Error

	respItems := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		entry := map[string]interface{}{
			"id":          it.ID.String(),
			"quantity":    it.Quantity,
			"unit_price":  it.UnitPrice,
			"total_price": it.TotalPrice,
		}
		if it.Product != nil {
			entry["product_name"] = it.Product.Name
			entry["product"] = it.Product
		}
		if it.ExtraFields != nil {
			if raw, ok := map[string]interface{}(it.ExtraFields)["product_attributes"]; ok {
				entry["product_attributes"] = raw
			}
		}
		respItems = append(respItems, entry)
	}

	return c.JSON(fiber.Map{
		"id":           order.ID.String(),
		"order_id":     order.ID.String(),
		"order_number": order.OrderNumber,
		"items":        respItems,
	})
}

func PublicAppendDiningOrderItems(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	orderIDStr := strings.TrimSpace(c.Params("id"))
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !tenant.WebsiteEnabled {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid order id"})
	}

	var req publicDiningOrderReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if len(req.Items) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Items is required"})
	}

	var order models.Order
	if err := database.DB.Where("id = ? AND tenant_id = ? AND source_type = ?", orderID, tenant.ID, "dining").First(&order).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Order not found"})
	}

	now := utils.NowInTenantTimezone(tenant.ID)
	var addedTotal float64
	var orderItems []models.OrderItem
	for _, it := range req.Items {
		pid, err := uuid.Parse(strings.TrimSpace(it.ProductID))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid product_id"})
		}
		if it.Quantity <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid quantity"})
		}
		var p models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ?", pid, tenant.ID, "active").First(&p).Error; err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Product not found"})
		}
		lineTotal := it.Quantity * p.Price
		addedTotal += lineTotal

		itemExtra := models.JSONB{}
		if len(it.Attributes) > 0 {
			attrs := make([]map[string]interface{}, len(it.Attributes))
			for i, a := range it.Attributes {
				attrs[i] = map[string]interface{}{
					"attribute_id": strings.TrimSpace(a.AttributeID),
					"value":        strings.TrimSpace(a.Value),
				}
			}
			itemExtra["product_attributes"] = attrs
		}

		orderItems = append(orderItems, models.OrderItem{
			TenantID:    tenant.ID,
			OrderID:     order.ID,
			ProductID:   &pid,
			Quantity:    it.Quantity,
			UnitPrice:   p.Price,
			TotalPrice:  lineTotal,
			ExtraFields: itemExtra,
			CreatedAt:   now,
		})
	}

	tx := database.DB.Begin()
	for _, oi := range orderItems {
		if err := tx.Create(&oi).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create order item"})
		}
	}
	order.TotalAmount = order.TotalAmount + addedTotal
	order.UpdatedAt = now
	if err := tx.Save(&order).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update order"})
	}
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to commit order"})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"order_id":     order.ID.String(),
		"order_number": order.OrderNumber,
	})
}

func RenderPublicDiningOrder(c *fiber.Ctx) error {
	return RenderPublicDiningMenu(c)
}

func RenderPublicDiningQueuePage(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !tenant.WebsiteEnabled {
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "暫時沒有此頁",
		}, "layouts/embed_layout")
	}

	return c.Render("pages/public_page_not_found", fiber.Map{
		"Message": "暫時沒有此頁",
	}, "layouts/embed_layout")
}

func RenderPublicDiningQueueTakePage(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !tenant.WebsiteEnabled {
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "暫時沒有此頁",
		}, "layouts/embed_layout")
	}

	var stores []models.Store
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenant.ID, "active").
		Order("created_at ASC").
		Find(&stores).Error; err != nil {
		stores = []models.Store{}
	}

	return c.Render("pages/dining_queue", fiber.Map{
		"Title":     "候位取號",
		"Tenant":    tenant,
		"Subdomain": subdomain,
		"Stores":    stores,
		"StoreID":   strings.TrimSpace(c.Query("store_id")),
	}, "layouts/embed_layout")
}

type publicQueueCreateReq struct {
	StoreID       string `json:"store_id"`
	AreaID        string `json:"area_id"`
	Name          string `json:"name"`
	Phone         string `json:"phone"`
	PartySize     int    `json:"party_size"`
	Notes         string `json:"notes"`
	ReservationAt string `json:"reservation_at"`
}

func PublicCreateDiningQueue(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !tenant.WebsiteEnabled {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req publicQueueCreateReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	requirePhone := strings.EqualFold(models.GetSystemSetting(diningQueueRequirePhoneKey, diningQueueRequirePhoneDefault), "true") || models.GetSystemSetting(diningQueueRequirePhoneKey, diningQueueRequirePhoneDefault) == "1"
	if requirePhone && strings.TrimSpace(req.Phone) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Phone is required"})
	}

	name := strings.TrimSpace(req.Name)
	partySize := req.PartySize
	if partySize <= 0 {
		partySize = 1
	}

	var storeID *uuid.UUID
	storeIDStr := strings.TrimSpace(req.StoreID)
	if storeIDStr != "" {
		id, err := uuid.Parse(storeIDStr)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid store_id"})
		}
		var store models.Store
		if err := database.DB.Where("id = ? AND tenant_id = ? AND status = ?", id, tenant.ID, "active").First(&store).Error; err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Store not found"})
		}
		storeID = &id
	}
	var areaID *uuid.UUID
	areaIDStr := strings.TrimSpace(req.AreaID)
	if areaIDStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "area_id is required"})
	}
	id, err := uuid.Parse(areaIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid area_id"})
	}
	areaID = &id

	var reservationAt *time.Time
	reservationAtStr := strings.TrimSpace(req.ReservationAt)
	if reservationAtStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02T15:04", reservationAtStr, time.Local)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid reservation_at"})
		}
		reservationAt = &parsed
	}

	now := time.Now()
	ticketNumber, ticketSeq, err := generateDiningQueueTicket(tenant.ID, storeID, areaID, now)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate queue ticket"})
	}

	queue := models.DiningQueue{
		TenantID:      tenant.ID,
		StoreID:       storeID,
		AreaID:        areaID,
		Name:          name,
		Phone:         strings.TrimSpace(req.Phone),
		PartySize:     partySize,
		ReservationAt: reservationAt,
		Status:        "waiting",
		Notes:         strings.TrimSpace(req.Notes),
		TicketNumber:  ticketNumber,
		TicketSeq:     ticketSeq,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := database.DB.Create(&queue).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create dining queue"})
	}

	return c.Status(201).JSON(queue)
}

func PublicGetDiningAreas(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !tenant.WebsiteEnabled {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	storeIDStr := strings.TrimSpace(c.Query("store_id"))
	areasQuery := database.DB.Where("tenant_id = ? AND is_active = ?", tenant.ID, true)
	if storeIDStr != "" {
		if id, err := uuid.Parse(storeIDStr); err == nil {
			areasQuery = areasQuery.Where("store_id = ?", id)
		} else {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid store_id"})
		}
	}

	var areas []models.DiningArea
	if err := areasQuery.Order("sort_order ASC, created_at ASC").Find(&areas).Error; err != nil {
		areas = []models.DiningArea{}
	}

	return c.JSON(fiber.Map{
		"data": areas,
	})
}

type publicQueueStatusArea struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CurrentCall string `json:"current_call"`
	LastTicket  string `json:"last_ticket"`
}

func PublicGetDiningQueueStatus(c *fiber.Ctx) error {
	subdomain := strings.TrimSpace(c.Params("subdomain"))
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil || tenant == nil || !tenant.WebsiteEnabled {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	storeIDStr := strings.TrimSpace(c.Query("store_id"))
	var storeID *uuid.UUID
	if storeIDStr != "" {
		id, err := uuid.Parse(storeIDStr)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid store_id"})
		}
		storeID = &id
	} else {
		var stores []models.Store
		if err := database.DB.
			Where("tenant_id = ? AND status = ?", tenant.ID, "active").
			Order("created_at ASC").
			Find(&stores).Error; err == nil && len(stores) == 1 {
			storeID = &stores[0].ID
		}
	}

	areasQuery := database.DB.Where("tenant_id = ? AND is_active = ?", tenant.ID, true)
	if storeID != nil {
		areasQuery = areasQuery.Where("store_id = ?", *storeID)
	}
	var areas []models.DiningArea
	if err := areasQuery.Order("sort_order ASC, created_at ASC").Find(&areas).Error; err != nil {
		areas = []models.DiningArea{}
	}

	result := make([]publicQueueStatusArea, 0, len(areas))
	for _, area := range areas {
		var current models.DiningQueue
		database.DB.
			Where("tenant_id = ? AND area_id = ? AND status = ?", tenant.ID, area.ID, "seated").
			Order("seated_at DESC, created_at DESC").
			First(&current)

		var last models.DiningQueue
		database.DB.
			Where("tenant_id = ? AND area_id = ?", tenant.ID, area.ID).
			Order("created_at DESC").
			First(&last)

		result = append(result, publicQueueStatusArea{
			ID:          area.ID.String(),
			Name:        area.Name,
			CurrentCall: strings.TrimSpace(current.TicketNumber),
			LastTicket:  strings.TrimSpace(last.TicketNumber),
		})
	}

	resp := fiber.Map{
		"store_id":   "",
		"areas":      result,
		"updated_at": time.Now().Format(time.RFC3339),
	}
	if storeID != nil {
		resp["store_id"] = storeID.String()
	}
	return c.JSON(resp)
}
