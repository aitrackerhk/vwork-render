package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetExtraFields 獲取動態字段
func GetExtraFields(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	resourceType := c.Params("resource_type") // customers, products, orders, etc.
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	// 根據資源類型查詢
	var extraFields models.JSONB
	switch resourceType {
	case "customers":
		var customer models.Customer
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&customer).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		extraFields = customer.ExtraFields
	case "products":
		var product models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&product).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		extraFields = product.ExtraFields
	case "orders":
		var order models.Order
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&order).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		extraFields = order.ExtraFields
	case "invoices":
		var invoice models.Invoice
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&invoice).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		extraFields = invoice.ExtraFields
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid resource type"})
	}

	return c.JSON(fiber.Map{"extra_fields": extraFields})
}

// UpdateExtraFields 更新動態字段
func UpdateExtraFields(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	resourceType := c.Params("resource_type")
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}

	var req struct {
		Fields map[string]interface{} `json:"fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 根據資源類型更新
	switch resourceType {
	case "customers":
		var customer models.Customer
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&customer).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		
		// 合併現有字段
		if customer.ExtraFields == nil {
			customer.ExtraFields = models.JSONB{}
		}
		extraFields := make(map[string]interface{})
		if customer.ExtraFields != nil {
			for k, v := range customer.ExtraFields {
				extraFields[k] = v
			}
		}
		for k, v := range req.Fields {
			extraFields[k] = v
		}
		customer.ExtraFields = models.JSONB(extraFields)
		
		if err := database.DB.Save(&customer).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update fields"})
		}
		return c.JSON(fiber.Map{"extra_fields": customer.ExtraFields})

	case "products":
		var product models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&product).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		
		extraFields := make(map[string]interface{})
		if product.ExtraFields != nil {
			for k, v := range product.ExtraFields {
				extraFields[k] = v
			}
		}
		for k, v := range req.Fields {
			extraFields[k] = v
		}
		product.ExtraFields = models.JSONB(extraFields)
		
		if err := database.DB.Save(&product).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update fields"})
		}
		return c.JSON(fiber.Map{"extra_fields": product.ExtraFields})

	case "orders":
		var order models.Order
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&order).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		
		extraFields := make(map[string]interface{})
		if order.ExtraFields != nil {
			for k, v := range order.ExtraFields {
				extraFields[k] = v
			}
		}
		for k, v := range req.Fields {
			extraFields[k] = v
		}
		order.ExtraFields = models.JSONB(extraFields)
		
		if err := database.DB.Save(&order).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update fields"})
		}
		return c.JSON(fiber.Map{"extra_fields": order.ExtraFields})

	case "invoices":
		var invoice models.Invoice
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&invoice).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		
		extraFields := make(map[string]interface{})
		if invoice.ExtraFields != nil {
			for k, v := range invoice.ExtraFields {
				extraFields[k] = v
			}
		}
		for k, v := range req.Fields {
			extraFields[k] = v
		}
		invoice.ExtraFields = models.JSONB(extraFields)
		
		if err := database.DB.Save(&invoice).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to update fields"})
		}
		return c.JSON(fiber.Map{"extra_fields": invoice.ExtraFields})

	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid resource type"})
	}
}

// DeleteExtraField 刪除動態字段
func DeleteExtraField(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	resourceType := c.Params("resource_type")
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
	}
	fieldName := c.Params("field_name")

	// 根據資源類型刪除字段
	switch resourceType {
	case "customers":
		var customer models.Customer
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&customer).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		if customer.ExtraFields != nil {
			extraFields := make(map[string]interface{})
			for k, v := range customer.ExtraFields {
				if k != fieldName {
					extraFields[k] = v
				}
			}
			customer.ExtraFields = models.JSONB(extraFields)
			if err := database.DB.Save(&customer).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to delete field"})
			}
		}
		return c.JSON(fiber.Map{"message": "Field deleted successfully"})

	case "products":
		var product models.Product
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&product).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		if product.ExtraFields != nil {
			extraFields := make(map[string]interface{})
			for k, v := range product.ExtraFields {
				if k != fieldName {
					extraFields[k] = v
				}
			}
			product.ExtraFields = models.JSONB(extraFields)
			if err := database.DB.Save(&product).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to delete field"})
			}
		}
		return c.JSON(fiber.Map{"message": "Field deleted successfully"})

	case "orders":
		var order models.Order
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&order).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		if order.ExtraFields != nil {
			extraFields := make(map[string]interface{})
			for k, v := range order.ExtraFields {
				if k != fieldName {
					extraFields[k] = v
				}
			}
			order.ExtraFields = models.JSONB(extraFields)
			if err := database.DB.Save(&order).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to delete field"})
			}
		}
		return c.JSON(fiber.Map{"message": "Field deleted successfully"})

	case "invoices":
		var invoice models.Invoice
		if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&invoice).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Resource not found"})
		}
		if invoice.ExtraFields != nil {
			extraFields := make(map[string]interface{})
			for k, v := range invoice.ExtraFields {
				if k != fieldName {
					extraFields[k] = v
				}
			}
			invoice.ExtraFields = models.JSONB(extraFields)
			if err := database.DB.Save(&invoice).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to delete field"})
			}
		}
		return c.JSON(fiber.Map{"message": "Field deleted successfully"})

	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid resource type"})
	}
}

