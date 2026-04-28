package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ReserveNumber 預留編號
func ReserveNumber(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		FieldName  string `json:"field_name"`
		FieldValue string `json:"field_value"`
		PageName   string `json:"page_name"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.FieldName == "" || req.FieldValue == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Field name and value are required"})
	}

	// 檢查是否已被預留或使用
	var existing models.ReservedNumber
	if err := database.DB.Where("tenant_id = ? AND field_name = ? AND field_value = ?",
		tenantID, req.FieldName, req.FieldValue).First(&existing).Error; err == nil {
		// 已存在，返回成功（已預留）
		return c.JSON(fiber.Map{"message": "Number already reserved", "reserved": true})
	}

	// 檢查是否已在實際數據中使用
	// 這裡需要根據不同的 field_name 檢查不同的表
	if req.FieldName == "order_number" {
		var order models.Order
		if err := database.DB.Where("tenant_id = ? AND order_number = ?", tenantID, req.FieldValue).First(&order).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Order number already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "customers" {
		var customer models.Customer
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&customer).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Customer code already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "products" {
		var product models.Product
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&product).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Product code already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "suppliers" {
		var supplier models.Supplier
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&supplier).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Supplier code already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "rooms" {
		var room models.Room
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&room).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Room code already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "equipments" {
		var equipment models.Equipment
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&equipment).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Equipment code already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "vehicles" {
		var vehicle models.Vehicle
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&vehicle).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Vehicle code already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "projects" {
		var project models.Project
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&project).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Project code already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "stores" {
		var store models.Store
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&store).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Store code already in use"})
		}
	} else if req.FieldName == "code" && req.PageName == "warehouses" {
		var warehouse models.Warehouse
		if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.FieldValue).First(&warehouse).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Warehouse code already in use"})
		}
	} else if req.FieldName == "employee_number" && req.PageName == "users" {
		var user models.User
		if err := database.DB.Where("tenant_id = ? AND employee_number = ?", tenantID, req.FieldValue).First(&user).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Employee number already in use"})
		}
	} else if req.FieldName == "invoice_number" {
		var invoice models.Invoice
		if err := database.DB.Where("tenant_id = ? AND invoice_number = ?", tenantID, req.FieldValue).First(&invoice).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invoice number already in use"})
		}
	} else if req.FieldName == "ticket_number" && req.PageName == "dining-queues" {
		var queue models.DiningQueue
		if err := database.DB.Where("tenant_id = ? AND ticket_number = ?", tenantID, req.FieldValue).First(&queue).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Ticket number already in use"})
		}
	} else if req.FieldName == "shipping_number" {
		// 檢查發貨單號是否已在訂單中使用（從 extra_fields 中檢查）
		var orders []models.Order
		database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+req.FieldValue+"%").Find(&orders)
		for _, order := range orders {
			if order.ExtraFields != nil {
				fields := map[string]interface{}(order.ExtraFields)
				// 檢查 shipping_records
				if records, exists := fields["shipping_records"]; exists {
					if recordsList, ok := records.([]interface{}); ok {
						for _, r := range recordsList {
							if record, ok := r.(map[string]interface{}); ok {
								if num, ok := record["shipping_number"].(string); ok && num == req.FieldValue {
									return c.Status(400).JSON(fiber.Map{"error": "Shipping number already in use"})
								}
							}
						}
					}
				}
				// 檢查 shipping_notes
				if notes, exists := fields["shipping_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if num, ok := note["shipping_number"].(string); ok && num == req.FieldValue {
									return c.Status(400).JSON(fiber.Map{"error": "Shipping number already in use"})
								}
							}
						}
					}
				}
			}
		}
	} else if req.FieldName == "refund_number" {
		// 檢查退款單號是否已在訂單/服務單中使用（從 extra_fields.refund_notes 中檢查）
		if req.PageName == "service-orders" {
			var serviceOrders []models.ServiceOrder
			database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+req.FieldValue+"%").Find(&serviceOrders)
			for _, so := range serviceOrders {
				if so.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(so.ExtraFields)
				if notes, exists := fields["refund_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if num, ok := note["refund_number"].(string); ok && num == req.FieldValue {
									return c.Status(400).JSON(fiber.Map{"error": "Refund number already in use"})
								}
							}
						}
					}
				}
			}
		} else {
			var orders []models.Order
			database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+req.FieldValue+"%").Find(&orders)
			for _, order := range orders {
				if order.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(order.ExtraFields)
				if notes, exists := fields["refund_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if num, ok := note["refund_number"].(string); ok && num == req.FieldValue {
									return c.Status(400).JSON(fiber.Map{"error": "Refund number already in use"})
								}
							}
						}
					}
				}
			}
		}
	}

	// 創建預留記錄
	reserved := models.ReservedNumber{
		TenantID:   tenantID,
		FieldName:  req.FieldName,
		FieldValue: req.FieldValue,
		PageName:   req.PageName,
		CreatedAt:  time.Now(),
	}

	if err := database.DB.Create(&reserved).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to reserve number"})
	}

	return c.Status(201).JSON(fiber.Map{"message": "Number reserved", "reserved": true})
}

// CheckReservedNumber 檢查編號是否已被預留或使用
func CheckReservedNumber(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	fieldName := c.Query("field_name")
	fieldValue := c.Query("field_value")

	if fieldName == "" || fieldValue == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Field name and value are required"})
	}

	// 檢查是否已被預留
	var reserved models.ReservedNumber
	if err := database.DB.Where("tenant_id = ? AND field_name = ? AND field_value = ?",
		tenantID, fieldName, fieldValue).First(&reserved).Error; err == nil {
		return c.JSON(fiber.Map{"reserved": true, "message": "Number is reserved"})
	}

	// 檢查是否已在實際數據中使用
	// 這裡需要根據不同的 field_name 檢查不同的表
	pageName := c.Query("page_name")
	if fieldName == "order_number" {
		var order models.Order
		if err := database.DB.Where("tenant_id = ? AND order_number = ?", tenantID, fieldValue).First(&order).Error; err == nil {
			return c.JSON(fiber.Map{"reserved": true, "message": "Order number already in use"})
		}
	} else if fieldName == "code" {
		if pageName == "customers" {
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, fieldValue).First(&customer).Error; err == nil {
				return c.JSON(fiber.Map{"reserved": true, "message": "Customer code already in use"})
			}
		} else if pageName == "products" {
			var product models.Product
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, fieldValue).First(&product).Error; err == nil {
				return c.JSON(fiber.Map{"reserved": true, "message": "Product code already in use"})
			}
		} else if pageName == "rooms" {
			var room models.Room
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, fieldValue).First(&room).Error; err == nil {
				return c.JSON(fiber.Map{"reserved": true, "message": "Room code already in use"})
			}
		} else if pageName == "equipments" {
			var equipment models.Equipment
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, fieldValue).First(&equipment).Error; err == nil {
				return c.JSON(fiber.Map{"reserved": true, "message": "Equipment code already in use"})
			}
		} else if pageName == "vehicles" {
			var vehicle models.Vehicle
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, fieldValue).First(&vehicle).Error; err == nil {
				return c.JSON(fiber.Map{"reserved": true, "message": "Vehicle code already in use"})
			}
		} else if pageName == "projects" {
			var project models.Project
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, fieldValue).First(&project).Error; err == nil {
				return c.JSON(fiber.Map{"reserved": true, "message": "Project code already in use"})
			}
		} else {
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, fieldValue).First(&customer).Error; err == nil {
				return c.JSON(fiber.Map{"reserved": true, "message": "Code already in use"})
			}
		}
	} else if fieldName == "invoice_number" {
		var invoice models.Invoice
		if err := database.DB.Where("tenant_id = ? AND invoice_number = ?", tenantID, fieldValue).First(&invoice).Error; err == nil {
			return c.JSON(fiber.Map{"reserved": true, "message": "Invoice number already in use"})
		}
	} else if fieldName == "ticket_number" && pageName == "dining-queues" {
		var queue models.DiningQueue
		if err := database.DB.Where("tenant_id = ? AND ticket_number = ?", tenantID, fieldValue).First(&queue).Error; err == nil {
			return c.JSON(fiber.Map{"reserved": true, "message": "Ticket number already in use"})
		}
	} else if fieldName == "expense_number" {
		// 檢查支出單號是否已在 expenses 中使用（description 或 extra_fields）
		var expenses []models.Expense
		database.DB.
			Where("tenant_id = ? AND (description = ? OR extra_fields::text LIKE ?)", tenantID, fieldValue, "%"+fieldValue+"%").
			Select("id, description, extra_fields").
			Find(&expenses)
		for _, exp := range expenses {
			if exp.Description == fieldValue {
				return c.JSON(fiber.Map{"reserved": true, "message": "Expense number already in use"})
			}
			if exp.ExtraFields != nil {
				fields := map[string]interface{}(exp.ExtraFields)
				for _, k := range []string{"expense_number", "invoice_number"} {
					if v, ok := fields[k].(string); ok && v == fieldValue {
						return c.JSON(fiber.Map{"reserved": true, "message": "Expense number already in use"})
					}
				}
			}
		}
	} else if fieldName == "refund_number" {
		// 檢查退款單號是否已在訂單/服務單中使用（從 extra_fields.refund_notes 中檢查）
		if pageName == "service-orders" {
			var serviceOrders []models.ServiceOrder
			database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+fieldValue+"%").Find(&serviceOrders)
			for _, so := range serviceOrders {
				if so.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(so.ExtraFields)
				if notes, exists := fields["refund_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if num, ok := note["refund_number"].(string); ok && num == fieldValue {
									return c.JSON(fiber.Map{"reserved": true, "message": "Refund number already in use"})
								}
							}
						}
					}
				}
			}
		} else {
			var orders []models.Order
			database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+fieldValue+"%").Find(&orders)
			for _, order := range orders {
				if order.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(order.ExtraFields)
				if notes, exists := fields["refund_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if num, ok := note["refund_number"].(string); ok && num == fieldValue {
									return c.JSON(fiber.Map{"reserved": true, "message": "Refund number already in use"})
								}
							}
						}
					}
				}
			}
		}
	}

	return c.JSON(fiber.Map{"reserved": false, "message": "Number is available"})
}

// GetReservedNumbers 獲取已預留的編號列表
func GetReservedNumbers(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	fieldName := c.Query("field_name")
	pageName := c.Query("page_name")

	query := database.DB.Where("tenant_id = ?", tenantID)

	if fieldName != "" {
		query = query.Where("field_name = ?", fieldName)
	}

	if pageName != "" {
		query = query.Where("page_name = ?", pageName)
	}

	var reserved []models.ReservedNumber
	if err := query.Order("created_at DESC").Find(&reserved).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data": reserved,
	})
}

// ReleaseReservedNumber 釋放預留編號（當草稿被刪除或提交後）
func ReleaseReservedNumber(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		FieldName  string `json:"field_name"`
		FieldValue string `json:"field_value"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 刪除預留記錄
	if err := database.DB.Where("tenant_id = ? AND field_name = ? AND field_value = ?",
		tenantID, req.FieldName, req.FieldValue).Delete(&models.ReservedNumber{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to release number"})
	}

	return c.JSON(fiber.Map{"message": "Number released"})
}

// GetNextNumber 獲取下一個可用編號
// 使用事務和立即預留機制確保並發安全，避免同一秒內保存和生成草稿時編號重複
func GetNextNumber(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	fieldName := c.Query("field_name")
	pageName := c.Query("page_name")
	prefix := c.Query("prefix") // 可選的前綴，如 "CUST-", "ORD-"

	if fieldName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Field name is required"})
	}

	var nextNumber string
	var sequence int64 = 1
	ticketPrefix := ""
	var err error
	maxRetries := 10
	retryCount := 0

	// 根據不同的字段類型生成編號（在循環外計算初始編號）
	// 注意：由於編號計算依賴數據庫查詢，每次重試時需要重新查詢
	// 因此我們需要將編號計算邏輯提取到循環內
	// 但為了代碼簡潔，先計算一次，循環內遞增序號
	switch fieldName {
	case "code":
		if pageName == "customers" {
			// 客戶編號：CUST-YYYYMMDD-001
			var count int64
			today := time.Now().Format("20060102")
			datePrefix := "CUST-" + today + "-"

			// 查詢今天已創建的客戶數量（包括已預留的）
			database.DB.Model(&models.Customer{}).Where("tenant_id = ? AND code LIKE ?", tenantID, datePrefix+"%").Count(&count)

			// 查詢今天已預留的編號
			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		} else if pageName == "products" {
			// 產品編號：PROD-YYYYMMDD-001
			var count int64
			today := time.Now().Format("20060102")
			datePrefix := "PROD-" + today + "-"

			database.DB.Model(&models.Product{}).Where("tenant_id = ? AND code LIKE ?", tenantID, datePrefix+"%").Count(&count)

			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		} else if pageName == "suppliers" {
			// 供應商編號：SUP-YYYYMMDD-001
			var count int64
			today := time.Now().Format("20060102")
			datePrefix := "SUP-" + today + "-"

			database.DB.Model(&models.Supplier{}).Where("tenant_id = ? AND code LIKE ?", tenantID, datePrefix+"%").Count(&count)

			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		} else if pageName == "rooms" {
			// 房間編號：RM-00001（五位序號）
			var count int64
			prefix := "RM-"

			database.DB.Model(&models.Room{}).Where("tenant_id = ? AND code LIKE ?", tenantID, prefix+"%").Count(&count)

			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, prefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = prefix + fmt.Sprintf("%05d", sequence)
		} else if pageName == "equipments" {
			// 設備編號：EQ-00001（五位序號）
			var count int64
			prefix := "EQ-"

			database.DB.Model(&models.Equipment{}).Where("tenant_id = ? AND code LIKE ?", tenantID, prefix+"%").Count(&count)

			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, prefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = prefix + fmt.Sprintf("%05d", sequence)
		} else if pageName == "vehicles" {
			// 車輛編號：VEH-00001（五位序號）
			var count int64
			prefix := "VEH-"

			database.DB.Model(&models.Vehicle{}).Where("tenant_id = ? AND code LIKE ?", tenantID, prefix+"%").Count(&count)

			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, prefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = prefix + fmt.Sprintf("%05d", sequence)
		} else if pageName == "projects" {
			// 項目編號：PROJ-YYYYMMDD-001
			var count int64
			today := time.Now().Format("20060102")
			datePrefix := "PROJ-" + today + "-"

			database.DB.Model(&models.Project{}).Where("tenant_id = ? AND code LIKE ?", tenantID, datePrefix+"%").Count(&count)

			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		} else if pageName == "stores" {
			// 店舖編號：STORE-00001（五位序號）
			var count int64
			prefix := "STORE-"

			database.DB.Model(&models.Store{}).Where("tenant_id = ? AND code LIKE ?", tenantID, prefix+"%").Count(&count)

			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, prefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = prefix + fmt.Sprintf("%05d", sequence)
		} else if pageName == "warehouses" {
			// 倉庫編號：WH-YYYYMMDD-001
			var count int64
			today := time.Now().Format("20060102")
			datePrefix := "WH-" + today + "-"

			database.DB.Model(&models.Warehouse{}).Where("tenant_id = ? AND code LIKE ?", tenantID, datePrefix+"%").Count(&count)

			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		} else {
			// 默認格式
			var count int64
			database.DB.Model(&models.Customer{}).Where("tenant_id = ?", tenantID).Count(&count)
			sequence = count + 1
			nextNumber = fmt.Sprintf("%s%06d", prefix, sequence)
		}
	case "ticket_number":
		if pageName == "dining-queues" {
			areaIDStr := strings.TrimSpace(c.Query("area_id"))
			var areaID *uuid.UUID
			if areaIDStr != "" {
				id, err := uuid.Parse(areaIDStr)
				if err != nil {
					return c.Status(400).JSON(fiber.Map{"error": "Invalid area_id"})
				}
				areaID = &id
			}
			storeIDStr := strings.TrimSpace(c.Query("store_id"))
			var storeID *uuid.UUID
			if storeIDStr != "" {
				id, err := uuid.Parse(storeIDStr)
				if err != nil {
					return c.Status(400).JSON(fiber.Map{"error": "Invalid store_id"})
				}
				storeID = &id
			}

			if areaID == nil {
				return c.Status(400).JSON(fiber.Map{"error": "area_id is required"})
			}

			areaCode := getDiningAreaCode(tenantID, areaID)
			ticketPrefix = buildDiningQueueTicketPrefix(areaCode, time.Now())
			var seq int
			nextNumber, seq, err = nextDiningQueueTicket(tenantID, storeID, areaID, time.Now(), true)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to generate ticket number"})
			}
			sequence = int64(seq)
		} else {
			return c.Status(400).JSON(fiber.Map{"error": "Unsupported page for ticket_number"})
		}
	case "employee_number":
		if pageName == "users" {
			// 員工編號：EMP-YYYYMMDD-001
			var count int64
			today := time.Now().Format("20060102")
			datePrefix := "EMP-" + today + "-"

			// 查詢今天已創建的員工數量（包括已預留的）
			database.DB.Model(&models.User{}).Where("tenant_id = ? AND employee_number LIKE ?", tenantID, datePrefix+"%").Count(&count)

			// 查詢今天已預留的編號
			var reservedCount int64
			database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
				tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

			sequence = count + reservedCount + 1
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		} else {
			// 默認格式
			var count int64
			database.DB.Model(&models.Customer{}).Where("tenant_id = ?", tenantID).Count(&count)
			sequence = count + 1
			nextNumber = fmt.Sprintf("%s%06d", prefix, sequence)
		}
	case "order_number":
		// 根據 page_name 生成不同的訂單號前綴
		var datePrefix string
		var count int64
		today := time.Now().Format("20060102")

		if pageName == "service-orders" {
			// 服務單號：SVC-YYYYMMDD-001
			datePrefix = "SVC-" + today + "-"
			database.DB.Model(&models.ServiceOrder{}).Where("tenant_id = ? AND order_number LIKE ?", tenantID, datePrefix+"%").Count(&count)
		} else if pageName == "purchase-orders" {
			// 採購單號：PUR-YYYYMMDD-001
			datePrefix = "PUR-" + today + "-"
			database.DB.Model(&models.PurchaseOrder{}).Where("tenant_id = ? AND order_number LIKE ?", tenantID, datePrefix+"%").Count(&count)
		} else {
			// 訂單號：ORD-YYYYMMDD-001
			datePrefix = "ORD-" + today + "-"
			database.DB.Model(&models.Order{}).Where("tenant_id = ? AND order_number LIKE ?", tenantID, datePrefix+"%").Count(&count)
		}

		var reservedCount int64
		database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
			tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

		sequence = count + reservedCount + 1
		nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
	case "invoice_number":
		// 發票號：INV-YYYYMMDD-001
		var count int64
		today := time.Now().Format("20060102")
		datePrefix := "INV-" + today + "-"

		database.DB.Model(&models.Invoice{}).Where("tenant_id = ? AND invoice_number LIKE ?", tenantID, datePrefix+"%").Count(&count)

		var reservedCount int64
		database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
			tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

		sequence = count + reservedCount + 1
		nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
	case "expense_number":
		// 支出單號：EXP-YYYYMMDD-001
		// 注意：expenses 表本身沒有獨立欄位，歷史上多存於 description 或 extra_fields.invoice_number/expense_number
		var reservedCount int64
		today := time.Now().Format("20060102")
		datePrefix := "EXP-" + today + "-"

		// 以 reserved_numbers 作為主序列來源，確保「預留」不重複
		database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
			tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

		// 兼容：若歷史資料已存在 EXP-*（但未預留），把最大序列納入起算，避免新生成撞舊值
		maxNum := 0
		{
			var expenses []models.Expense
			// 粗略過濾：只撈可能包含 prefix 的資料（description 或 extra_fields）
			database.DB.
				Where("tenant_id = ? AND (description LIKE ? OR extra_fields::text LIKE ?)", tenantID, "%"+datePrefix+"%", "%"+datePrefix+"%").
				Select("id, description, extra_fields").
				Find(&expenses)
			for _, exp := range expenses {
				// 1) description
				if strings.HasPrefix(exp.Description, datePrefix) && len(exp.Description) > len(datePrefix) {
					var num int
					if _, err := fmt.Sscanf(exp.Description[len(datePrefix):], "%d", &num); err == nil && num > maxNum {
						maxNum = num
					}
				}
				// 2) extra_fields
				if exp.ExtraFields != nil {
					fields := map[string]interface{}(exp.ExtraFields)
					for _, k := range []string{"expense_number", "invoice_number"} {
						if v, ok := fields[k].(string); ok && strings.HasPrefix(v, datePrefix) && len(v) > len(datePrefix) {
							var num int
							if _, err := fmt.Sscanf(v[len(datePrefix):], "%d", &num); err == nil && num > maxNum {
								maxNum = num
							}
						}
					}
				}
			}
		}

		sequence = int64(maxNum) + reservedCount + 1
		nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
	case "refund_number":
		// 退款單號：
		// - 統一：REF-YYYYMMDD-001
		var count int64
		today := time.Now().Format("20060102")
		datePrefix := "REF-" + today + "-"

		// 查詢今天已使用的退款單號碼（從 orders/service_orders 的 extra_fields 中）
		maxNum := 0
		if pageName == "service-orders" {
			var serviceOrders []models.ServiceOrder
			database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").Find(&serviceOrders)
			for _, so := range serviceOrders {
				if so.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(so.ExtraFields)
				if notes, exists := fields["refund_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if numStr, ok := note["refund_number"].(string); ok {
									if strings.HasPrefix(numStr, datePrefix) && len(numStr) > len(datePrefix) {
										var num int
										if _, err := fmt.Sscanf(numStr[len(datePrefix):], "%d", &num); err == nil {
											if num > maxNum {
												maxNum = num
											}
										}
									}
								}
							}
						}
					}
				}
			}
		} else {
			var orders []models.Order
			database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").Find(&orders)
			for _, order := range orders {
				if order.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(order.ExtraFields)
				if notes, exists := fields["refund_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if numStr, ok := note["refund_number"].(string); ok {
									if strings.HasPrefix(numStr, datePrefix) && len(numStr) > len(datePrefix) {
										var num int
										if _, err := fmt.Sscanf(numStr[len(datePrefix):], "%d", &num); err == nil {
											if num > maxNum {
												maxNum = num
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

		if int64(maxNum) > count {
			count = int64(maxNum)
		}

		var reservedCount int64
		database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
			tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

		sequence = count + reservedCount + 1
		nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)

		// 確保退款單編號使用 REF- 前綴，不使用 NUM-
		if !strings.HasPrefix(nextNumber, "REF-") {
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		}
	case "cancellation_number":
		// 取消採購單號（cancellation_notes）：
		// - 統一：REF-YYYYMMDD-001（對齊訂單退款單格式）
		var count int64
		today := time.Now().Format("20060102")
		datePrefix := "REF-" + today + "-"

		// 查詢今天已使用的取消採購單號碼（從 purchase_orders 的 extra_fields.cancellation_notes 中）
		maxNum := 0
		var purchaseOrders []models.PurchaseOrder
		database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").Find(&purchaseOrders)
		for _, po := range purchaseOrders {
			if po.ExtraFields == nil {
				continue
			}
			fields := map[string]interface{}(po.ExtraFields)
			if notes, exists := fields["cancellation_notes"]; exists {
				if notesList, ok := notes.([]interface{}); ok {
					for _, n := range notesList {
						if note, ok := n.(map[string]interface{}); ok {
							if numStr, ok := note["cancellation_number"].(string); ok {
								if strings.HasPrefix(numStr, datePrefix) && len(numStr) > len(datePrefix) {
									var num int
									if _, err := fmt.Sscanf(numStr[len(datePrefix):], "%d", &num); err == nil {
										if num > maxNum {
											maxNum = num
										}
									}
								}
							}
						}
					}
				}
			}
		}

		if int64(maxNum) > count {
			count = int64(maxNum)
		}

		var reservedCount int64
		database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
			tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

		sequence = count + reservedCount + 1
		nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)

		// 確保取消採購單編號使用 REF- 前綴，不使用 NUM-
		if !strings.HasPrefix(nextNumber, "REF-") {
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		}
	case "shipping_number":
		// 發貨單號：SHIP-YYYYMMDD-001
		var count int64
		today := time.Now().Format("20060102")
		datePrefix := "SHIP-" + today + "-"

		// 查詢今天已生成的發貨單數量（從 orders 的 extra_fields 中）
		var orders []models.Order
		database.DB.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+datePrefix+"%").Find(&orders)

		// 統計今天已使用的發貨單號碼
		maxNum := 0
		for _, order := range orders {
			if order.ExtraFields != nil {
				fields := map[string]interface{}(order.ExtraFields)
				// 檢查 shipping_records
				if records, exists := fields["shipping_records"]; exists {
					if recordsList, ok := records.([]interface{}); ok {
						for _, r := range recordsList {
							if record, ok := r.(map[string]interface{}); ok {
								if shippingNum, ok := record["shipping_number"].(string); ok {
									if len(shippingNum) > len(datePrefix) {
										var num int
										if _, err := fmt.Sscanf(shippingNum[len(datePrefix):], "%d", &num); err == nil {
											if num > maxNum {
												maxNum = num
											}
										}
									}
								}
							}
						}
					}
				}
				// 檢查 shipping_notes
				if notes, exists := fields["shipping_notes"]; exists {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if shippingNum, ok := note["shipping_number"].(string); ok {
									if len(shippingNum) > len(datePrefix) {
										var num int
										if _, err := fmt.Sscanf(shippingNum[len(datePrefix):], "%d", &num); err == nil {
											if num > maxNum {
												maxNum = num
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

		if int64(maxNum) > count {
			count = int64(maxNum)
		}

		var reservedCount int64
		database.DB.Model(&models.ReservedNumber{}).Where("tenant_id = ? AND field_name = ? AND field_value LIKE ?",
			tenantID, fieldName, datePrefix+"%").Count(&reservedCount)

		sequence = count + reservedCount + 1
		nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)

		// 確保發貨單編號使用 SHIP- 前綴，不使用 NUM-
		if !strings.HasPrefix(nextNumber, "SHIP-") {
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		}
	default:
		// 默認：使用前綴 + 序號
		if prefix == "" {
			prefix = "NUM-"
		}
		var count int64
		database.DB.Model(&models.Customer{}).Where("tenant_id = ?", tenantID).Count(&count)
		sequence = count + 1
		nextNumber = fmt.Sprintf("%s%06d", prefix, sequence)
	}

	// 檢查並確保編號唯一（如果已被使用，遞增序號）
	// 同時嘗試預留編號，確保並發安全
	for retryCount < maxRetries {
		// 每次重試都重新開始事務
		tx := database.DB.Begin()

		// 檢查是否已在實際數據中使用
		exists := false
		if fieldName == "order_number" {
			if pageName == "service-orders" {
				var serviceOrder models.ServiceOrder
				if err := tx.Where("tenant_id = ? AND order_number = ?", tenantID, nextNumber).First(&serviceOrder).Error; err == nil {
					exists = true
				}
			} else if pageName == "purchase-orders" {
				var purchaseOrder models.PurchaseOrder
				if err := tx.Where("tenant_id = ? AND order_number = ?", tenantID, nextNumber).First(&purchaseOrder).Error; err == nil {
					exists = true
				}
			} else {
				var order models.Order
				if err := tx.Where("tenant_id = ? AND order_number = ?", tenantID, nextNumber).First(&order).Error; err == nil {
					exists = true
				}
			}
		} else if fieldName == "code" && pageName == "customers" {
			var customer models.Customer
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, nextNumber).First(&customer).Error; err == nil {
				exists = true
			}
		} else if fieldName == "code" && pageName == "products" {
			var product models.Product
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, nextNumber).First(&product).Error; err == nil {
				exists = true
			}
		} else if fieldName == "code" && pageName == "rooms" {
			var room models.Room
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, nextNumber).First(&room).Error; err == nil {
				exists = true
			}
		} else if fieldName == "code" && pageName == "equipments" {
			var equipment models.Equipment
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, nextNumber).First(&equipment).Error; err == nil {
				exists = true
			}
		} else if fieldName == "code" && pageName == "vehicles" {
			var vehicle models.Vehicle
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, nextNumber).First(&vehicle).Error; err == nil {
				exists = true
			}
		} else if fieldName == "code" && pageName == "stores" {
			var store models.Store
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, nextNumber).First(&store).Error; err == nil {
				exists = true
			}
		} else if fieldName == "code" && pageName == "warehouses" {
			var warehouse models.Warehouse
			if err := tx.Where("tenant_id = ? AND code = ?", tenantID, nextNumber).First(&warehouse).Error; err == nil {
				exists = true
			}
		} else if fieldName == "employee_number" && pageName == "users" {
			var user models.User
			if err := tx.Where("tenant_id = ? AND employee_number = ?", tenantID, nextNumber).First(&user).Error; err == nil {
				exists = true
			}
		} else if fieldName == "invoice_number" {
			var invoice models.Invoice
			if err := tx.Where("tenant_id = ? AND invoice_number = ?", tenantID, nextNumber).First(&invoice).Error; err == nil {
				exists = true
			}
		} else if fieldName == "shipping_number" {
			// 檢查發貨單號是否已在訂單中使用（從 extra_fields 中檢查）
			var orders []models.Order
			tx.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+nextNumber+"%").Find(&orders)
			for _, order := range orders {
				if order.ExtraFields != nil {
					fields := map[string]interface{}(order.ExtraFields)
					// 檢查 shipping_records
					if records, recExists := fields["shipping_records"]; recExists {
						if recordsList, ok := records.([]interface{}); ok {
							for _, r := range recordsList {
								if record, ok := r.(map[string]interface{}); ok {
									if num, ok := record["shipping_number"].(string); ok && num == nextNumber {
										exists = true
										break
									}
								}
							}
						}
					}
					// 檢查 shipping_notes
					if notes, notesExists := fields["shipping_notes"]; notesExists {
						if notesList, ok := notes.([]interface{}); ok {
							for _, n := range notesList {
								if note, ok := n.(map[string]interface{}); ok {
									if num, ok := note["shipping_number"].(string); ok && num == nextNumber {
										exists = true
										break
									}
								}
							}
						}
					}
				}
				if exists {
					break
				}
			}
		} else if fieldName == "refund_number" {
			// 檢查退款單號是否已在訂單/服務單中使用（從 extra_fields.refund_notes 中檢查）
			if pageName == "service-orders" {
				var serviceOrders []models.ServiceOrder
				tx.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+nextNumber+"%").Find(&serviceOrders)
				for _, so := range serviceOrders {
					if so.ExtraFields == nil {
						continue
					}
					fields := map[string]interface{}(so.ExtraFields)
					if notes, existsNotes := fields["refund_notes"]; existsNotes {
						if notesList, ok := notes.([]interface{}); ok {
							for _, n := range notesList {
								if note, ok := n.(map[string]interface{}); ok {
									if num, ok := note["refund_number"].(string); ok && num == nextNumber {
										exists = true
										break
									}
								}
							}
						}
					}
					if exists {
						break
					}
				}
			} else {
				var orders []models.Order
				tx.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+nextNumber+"%").Find(&orders)
				for _, order := range orders {
					if order.ExtraFields == nil {
						continue
					}
					fields := map[string]interface{}(order.ExtraFields)
					if notes, existsNotes := fields["refund_notes"]; existsNotes {
						if notesList, ok := notes.([]interface{}); ok {
							for _, n := range notesList {
								if note, ok := n.(map[string]interface{}); ok {
									if num, ok := note["refund_number"].(string); ok && num == nextNumber {
										exists = true
										break
									}
								}
							}
						}
					}
					if exists {
						break
					}
				}
			}
		} else if fieldName == "cancellation_number" {
			// 檢查取消採購單號是否已在採購單中使用（從 extra_fields.cancellation_notes 中檢查）
			var purchaseOrders []models.PurchaseOrder
			tx.Where("tenant_id = ? AND extra_fields::text LIKE ?", tenantID, "%"+nextNumber+"%").Find(&purchaseOrders)
			for _, po := range purchaseOrders {
				if po.ExtraFields == nil {
					continue
				}
				fields := map[string]interface{}(po.ExtraFields)
				if notes, existsNotes := fields["cancellation_notes"]; existsNotes {
					if notesList, ok := notes.([]interface{}); ok {
						for _, n := range notesList {
							if note, ok := n.(map[string]interface{}); ok {
								if num, ok := note["cancellation_number"].(string); ok && num == nextNumber {
									exists = true
									break
								}
							}
						}
					}
				}
				if exists {
					break
				}
			}
		}

		// 檢查是否已預留（在事務中檢查）
		var reserved models.ReservedNumber
		if err := tx.Where("tenant_id = ? AND field_name = ? AND field_value = ?",
			tenantID, fieldName, nextNumber).First(&reserved).Error; err == nil {
			exists = true
		}

		if !exists {
			// 嘗試立即預留編號（在事務中），確保並發安全
			// 如果預留成功，說明編號可用；如果失敗（唯一約束衝突），說明已被其他請求預留，需要重試
			reserved := models.ReservedNumber{
				TenantID:   tenantID,
				FieldName:  fieldName,
				FieldValue: nextNumber,
				PageName:   pageName,
				CreatedAt:  time.Now(),
			}

			if err := tx.Create(&reserved).Error; err != nil {
				// 預留失敗（可能是唯一約束衝突），需要重試
				exists = true
			} else {
				// 預留成功，提交事務並返回
				if err := tx.Commit().Error; err != nil {
					tx.Rollback()
					return c.Status(500).JSON(fiber.Map{"error": "Failed to commit transaction"})
				}
				return c.JSON(fiber.Map{
					"next_number": nextNumber,
					"sequence":    sequence,
				})
			}
		}

		// 如果已存在或預留失敗，回滾事務並遞增序號重試
		tx.Rollback()
		retryCount++
		if retryCount >= maxRetries {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate unique number after retries"})
		}

		// 如果已存在，遞增序號
		sequence++
		if fieldName == "shipping_number" {
			// 發貨單號使用 SHIP-YYYYMMDD-XXX 格式
			today := time.Now().Format("20060102")
			datePrefix := "SHIP-" + today + "-"
			nextNumber = datePrefix + fmt.Sprintf("%03d", sequence)
		} else if fieldName == "refund_number" {
			// 與上面生成規則對齊：REF-YYYYMMDD-###
			today := time.Now().Format("20060102")
			nextNumber = "REF-" + today + "-" + fmt.Sprintf("%03d", sequence)
		} else if fieldName == "cancellation_number" {
			// 與上面生成規則對齊：REF-YYYYMMDD-###
			today := time.Now().Format("20060102")
			nextNumber = "REF-" + today + "-" + fmt.Sprintf("%03d", sequence)
		} else if fieldName == "code" && (pageName == "customers" || pageName == "products" || pageName == "rooms" || pageName == "equipments" || pageName == "vehicles" || pageName == "suppliers" || pageName == "projects" || pageName == "stores" || pageName == "warehouses") {
			today := time.Now().Format("20060102")
			if pageName == "customers" {
				nextNumber = "CUST-" + today + "-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "products" {
				nextNumber = "PROD-" + today + "-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "suppliers" {
				nextNumber = "SUP-" + today + "-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "projects" {
				nextNumber = "PROJ-" + today + "-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "warehouses" {
				nextNumber = "WH-" + today + "-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "rooms" {
				nextNumber = "RM-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "equipments" {
				nextNumber = "EQ-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "vehicles" {
				nextNumber = "VEH-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "stores" {
				nextNumber = "STORE-" + fmt.Sprintf("%05d", sequence)
			}
		} else if fieldName == "employee_number" && pageName == "users" {
			today := time.Now().Format("20060102")
			nextNumber = "EMP-" + today + "-" + fmt.Sprintf("%03d", sequence)
		} else if fieldName == "order_number" {
			today := time.Now().Format("20060102")
			if pageName == "service-orders" {
				nextNumber = "SVC-" + today + "-" + fmt.Sprintf("%03d", sequence)
			} else if pageName == "purchase-orders" {
				nextNumber = "PUR-" + today + "-" + fmt.Sprintf("%03d", sequence)
			} else {
				nextNumber = "ORD-" + today + "-" + fmt.Sprintf("%03d", sequence)
			}
		} else if fieldName == "invoice_number" {
			today := time.Now().Format("20060102")
			nextNumber = "INV-" + today + "-" + fmt.Sprintf("%03d", sequence)
		} else if fieldName == "ticket_number" && pageName == "dining-queues" {
			if ticketPrefix == "" {
				areaCode := "X"
				ticketPrefix = buildDiningQueueTicketPrefix(areaCode, time.Now())
			}
			nextNumber = ticketPrefix + fmt.Sprintf("%03d", sequence)
		} else {
			nextNumber = fmt.Sprintf("%s%06d", prefix, sequence)
		}
	}

	return c.JSON(fiber.Map{
		"next_number": nextNumber,
		"sequence":    sequence,
	})
}

// ============================================
// Drafts API (user-scoped)
// ============================================

// SaveDraft 保存或更新草稿
// POST /api/v1/drafts
func SaveDraft(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if tenantID == uuid.Nil || userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var req struct {
		DraftID       string                 `json:"draft_id"`
		PageName      string                 `json:"page_name"`
		KeyField      string                 `json:"key_field"`
		KeyFieldValue string                 `json:"key_field_value"`
		Data          map[string]interface{} `json:"data"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if strings.TrimSpace(req.PageName) == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Page name is required"})
	}

	if req.Data == nil {
		req.Data = map[string]interface{}{}
	}

	// 如果未提供 key_field_value，嘗試從 data 取
	keyFieldValue := strings.TrimSpace(req.KeyFieldValue)
	if keyFieldValue == "" && strings.TrimSpace(req.KeyField) != "" {
		if v, ok := req.Data[req.KeyField]; ok && v != nil {
			keyFieldValue = fmt.Sprint(v)
		}
	}

	var draft models.Draft
	var draftID uuid.UUID
	if strings.TrimSpace(req.DraftID) != "" {
		parsed, err := uuid.Parse(req.DraftID)
		if err == nil {
			draftID = parsed
		}
	}

	// 優先用 draft_id 查找
	if draftID != uuid.Nil {
		if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, draftID).First(&draft).Error; err != nil && err != gorm.ErrRecordNotFound {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to load draft"})
		}
	}

	// 找不到時，按 page + key_field_value 去查
	if draft.ID == uuid.Nil && keyFieldValue != "" {
		if err := database.DB.Where("tenant_id = ? AND user_id = ? AND page_name = ? AND key_field_value = ?", tenantID, userID, req.PageName, keyFieldValue).First(&draft).Error; err != nil && err != gorm.ErrRecordNotFound {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to load draft"})
		}
	}

	if draft.ID == uuid.Nil {
		// 新增
		if draftID != uuid.Nil {
			draft.ID = draftID
		}
		draft.TenantID = tenantID
		draft.UserID = userID
		draft.PageName = req.PageName
		draft.KeyFieldValue = keyFieldValue
		draft.Data = models.JSONB(req.Data)
		if err := database.DB.Create(&draft).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to save draft"})
		}
	} else {
		// 更新
		draft.PageName = req.PageName
		draft.KeyFieldValue = keyFieldValue
		draft.Data = models.JSONB(req.Data)
		if err := database.DB.Save(&draft).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update draft"})
		}
	}

	return c.JSON(fiber.Map{
		"id":              draft.ID,
		"page_name":       draft.PageName,
		"key_field_value": draft.KeyFieldValue,
		"data":            draft.Data,
		"created_at":      draft.CreatedAt,
		"updated_at":      draft.UpdatedAt,
	})
}

// GetDrafts 取得草稿列表（依頁面可篩選）
// GET /api/v1/drafts?page_name=orders
func GetDrafts(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if tenantID == uuid.Nil || userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	pageName := strings.TrimSpace(c.Query("page_name"))

	query := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID)
	if pageName != "" {
		query = query.Where("page_name = ?", pageName)
	}

	var drafts []models.Draft
	if err := query.Order("updated_at DESC").Find(&drafts).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load drafts"})
	}

	return c.JSON(fiber.Map{
		"drafts": drafts,
	})
}

// GetDraft 取得單一草稿
// GET /api/v1/drafts/:id
func GetDraft(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if tenantID == uuid.Nil || userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	idParam := strings.TrimSpace(c.Params("id"))
	if idParam == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Draft id is required"})
	}

	id, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid draft id"})
	}

	var draft models.Draft
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).First(&draft).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Draft not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load draft"})
	}

	return c.JSON(draft)
}

// DeleteDraft 刪除草稿
// DELETE /api/v1/drafts/:id
func DeleteDraft(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if tenantID == uuid.Nil || userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	idParam := strings.TrimSpace(c.Params("id"))
	if idParam == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Draft id is required"})
	}

	id, err := uuid.Parse(idParam)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid draft id"})
	}

	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).Delete(&models.Draft{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete draft"})
	}

	return c.JSON(fiber.Map{"success": true})
}
