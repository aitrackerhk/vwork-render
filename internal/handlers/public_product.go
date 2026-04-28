package handlers

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"nwork/internal/database"
	"nwork/internal/models"
)

// PublicGetProduct 獲取公開產品（用於網店）
func PublicGetProduct(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	productID := c.Params("id")

	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	id, err := uuid.Parse(productID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid product ID"})
	}

	var product models.Product
	if err := database.DB.
		Where("id = ? AND tenant_id = ? AND status = ?", id, tenant.ID, "active").
		Preload("ProductType").
		Preload("Brand").
		First(&product).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product not found"})
	}

	// load attribute values
	var values []models.ProductAttributeValue
	_ = database.DB.
		Preload("Attribute").
		Where("product_id = ? AND status = ?", product.ID, "active").
		Find(&values).Error

	attrMap := map[string]map[string]interface{}{}
	for _, v := range values {
		attrID := v.AttributeID.String()
		if _, ok := attrMap[attrID]; !ok {
			attrMap[attrID] = map[string]interface{}{
				"id":             attrID,
				"name":           v.Attribute.Name,
				"attribute_type": v.Attribute.AttributeType,
				"is_required":    v.Attribute.IsRequired,
				"options":        v.Attribute.Options,
				"values":         []string{},
			}
		}
		if v.Value != "" {
			if list, ok := attrMap[attrID]["values"].([]string); ok {
				// Split by comma to support multiple values in one entry (Fix for dining menu dropdowns)
				parts := strings.Split(v.Value, ",")
				for _, p := range parts {
					if val := strings.TrimSpace(p); val != "" {
						list = append(list, val)
					}
				}
				attrMap[attrID]["values"] = list
			}
		}
	}
	attrs := make([]map[string]interface{}, 0, len(attrMap))
	for _, v := range attrMap {
		attrs = append(attrs, v)
	}

	return c.JSON(fiber.Map{
		"product":    product,
		"attributes": attrs,
	})
}
