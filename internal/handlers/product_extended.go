package handlers

import (
	"encoding/json"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ============================================
// 產品類型 (ProductType) CRUD
// ============================================

func GetProductTypes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	parentID := c.Query("parent_id")
	
	var productTypes []models.ProductType
	query := database.DB.Where("tenant_id = ?", tenantID)

	if parentID != "" {
		query = query.Where("parent_id = ?", parentID)
	} else if c.Query("root_only") == "true" {
		query = query.Where("parent_id IS NULL")
	}

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ProductType{}).Count(&total)

	if err := query.Preload("Parent").Offset(offset).Limit(limit).Find(&productTypes).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  productTypes,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetProductType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var productType models.ProductType

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("Parent").First(&productType).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product type not found"})
	}

	return c.JSON(productType)
}

func CreateProductType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var productType models.ProductType
	if err := c.BodyParser(&productType); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	productType.TenantID = tenantID

	if err := database.DB.Create(&productType).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(productType)
}

func UpdateProductType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var productType models.ProductType

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&productType).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product type not found"})
	}

	if err := c.BodyParser(&productType); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	productType.TenantID = tenantID

	if err := database.DB.Save(&productType).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(productType)
}

func DeleteProductType(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.ProductType{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Product type deleted"})
}

// ============================================
// 產品屬性 (ProductAttribute) CRUD
// ============================================

func GetProductAttributes(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var attributes []models.ProductAttribute
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.ProductAttribute{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Find(&attributes).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 處理每個屬性的 options 字段：如果是包裝在 map 中的數組，提取出來
	processedAttributes := make([]map[string]interface{}, len(attributes))
	for i, attr := range attributes {
		attrBytes, _ := json.Marshal(attr)
		var attrMap map[string]interface{}
		json.Unmarshal(attrBytes, &attrMap)
		
		if optionsMap, ok := attrMap["options"].(map[string]interface{}); ok {
			if arrayData, ok := optionsMap["_array"].([]interface{}); ok {
				attrMap["options"] = arrayData
			} else if arrayData, ok := optionsMap["_array"].([]string); ok {
				attrMap["options"] = arrayData
			}
		}
		
		processedAttributes[i] = attrMap
	}

	return c.JSON(fiber.Map{
		"data":  processedAttributes,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetProductAttribute(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var attribute models.ProductAttribute

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&attribute).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product attribute not found"})
	}

	// 處理 options 字段：如果是包裝在 map 中的數組，提取出來
	result := make(map[string]interface{})
	resultBytes, _ := json.Marshal(attribute)
	json.Unmarshal(resultBytes, &result)
	
	if optionsMap, ok := result["options"].(map[string]interface{}); ok {
		if arrayData, ok := optionsMap["_array"].([]interface{}); ok {
			result["options"] = arrayData
		} else if arrayData, ok := optionsMap["_array"].([]string); ok {
			result["options"] = arrayData
		}
	}

	return c.JSON(result)
}

func CreateProductAttribute(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var req struct {
		Name          string          `json:"name"`
		Code          *string         `json:"code"`
		AttributeType string          `json:"attribute_type"`
		Options       interface{}     `json:"options"` // 可能是數組、字符串或其他格式
		IsRequired    interface{}     `json:"is_required"` // 可能是 bool, string "yes"/"no", 或其他
		Status        string          `json:"status"`
		ExtraFields   models.JSONB    `json:"extra_fields"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	// 處理 options 字段：支持數組、字符串或其他格式
	var options models.JSONB
	if req.Options != nil {
		switch v := req.Options.(type) {
		case []interface{}:
			// 如果是數組，包裝在 map 中
			options = models.JSONB{"_array": v}
		case []string:
			// 如果是字符串數組，包裝在 map 中
			options = models.JSONB{"_array": v}
		case string:
			// 如果是字符串（逗號分隔），轉換為數組
			if v != "" {
				parts := strings.Split(v, ",")
				trimmed := make([]string, 0, len(parts))
				for _, part := range parts {
					trimmed = append(trimmed, strings.TrimSpace(part))
				}
				options = models.JSONB{"_array": trimmed}
			} else {
				options = models.JSONB{}
			}
		case map[string]interface{}:
			// 如果已經是 map，直接使用
			options = models.JSONB(v)
		default:
			// 其他類型，包裝在 map 中
			options = models.JSONB{"_data": v}
		}
	} else {
		options = models.JSONB{}
	}

	// 處理 is_required 字段：支持 bool, "yes"/"no" 字符串
	var isRequired bool
	switch v := req.IsRequired.(type) {
	case bool:
		isRequired = v
	case string:
		isRequired = v == "yes" || v == "true"
	default:
		isRequired = false
	}

	attribute := models.ProductAttribute{
		TenantID:      tenantID,
		Name:          req.Name,
		Code:          req.Code,
		AttributeType: req.AttributeType,
		Options:       options,
		IsRequired:    isRequired,
		Status:        req.Status,
		ExtraFields:   req.ExtraFields,
	}

	if err := database.DB.Create(&attribute).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 處理返回的 options 字段：如果是包裝在 map 中的數組，提取出來
	result := make(map[string]interface{})
	resultBytes, _ := json.Marshal(attribute)
	json.Unmarshal(resultBytes, &result)
	
	if optionsMap, ok := result["options"].(map[string]interface{}); ok {
		if arrayData, ok := optionsMap["_array"].([]interface{}); ok {
			result["options"] = arrayData
		} else if arrayData, ok := optionsMap["_array"].([]string); ok {
			result["options"] = arrayData
		}
	}

	return c.Status(201).JSON(result)
}

func UpdateProductAttribute(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var attribute models.ProductAttribute

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&attribute).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product attribute not found"})
	}

	var req struct {
		Name          string          `json:"name"`
		Code          *string         `json:"code"`
		AttributeType string          `json:"attribute_type"`
		Options       interface{}     `json:"options"` // 可能是數組、字符串或其他格式
		IsRequired    interface{}     `json:"is_required"` // 可能是 bool, string "yes"/"no", 或其他
		Status        string          `json:"status"`
		ExtraFields   models.JSONB    `json:"extra_fields"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request: " + err.Error()})
	}

	// 處理 options 字段：支持數組、字符串或其他格式
	var options models.JSONB
	if req.Options != nil {
		switch v := req.Options.(type) {
		case []interface{}:
			// 如果是數組，包裝在 map 中
			options = models.JSONB{"_array": v}
		case []string:
			// 如果是字符串數組，包裝在 map 中
			options = models.JSONB{"_array": v}
		case string:
			// 如果是字符串（逗號分隔），轉換為數組
			if v != "" {
				parts := strings.Split(v, ",")
				trimmed := make([]string, 0, len(parts))
				for _, part := range parts {
					trimmed = append(trimmed, strings.TrimSpace(part))
				}
				options = models.JSONB{"_array": trimmed}
			} else {
				options = models.JSONB{}
			}
		case map[string]interface{}:
			// 如果已經是 map，直接使用
			options = models.JSONB(v)
		default:
			// 其他類型，包裝在 map 中
			options = models.JSONB{"_data": v}
		}
	} else {
		options = models.JSONB{}
	}

	// 處理 is_required 字段：支持 bool, "yes"/"no" 字符串
	var isRequired bool
	switch v := req.IsRequired.(type) {
	case bool:
		isRequired = v
	case string:
		isRequired = v == "yes" || v == "true"
	default:
		isRequired = false
	}

	attribute.Name = req.Name
	attribute.Code = req.Code
	attribute.AttributeType = req.AttributeType
	attribute.Options = options
	attribute.IsRequired = isRequired
	attribute.Status = req.Status
	attribute.ExtraFields = req.ExtraFields
	attribute.TenantID = tenantID

	if err := database.DB.Save(&attribute).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 處理返回的 options 字段：如果是包裝在 map 中的數組，提取出來
	result := make(map[string]interface{})
	resultBytes, _ := json.Marshal(attribute)
	json.Unmarshal(resultBytes, &result)
	
	if optionsMap, ok := result["options"].(map[string]interface{}); ok {
		if arrayData, ok := optionsMap["_array"].([]interface{}); ok {
			result["options"] = arrayData
		} else if arrayData, ok := optionsMap["_array"].([]string); ok {
			result["options"] = arrayData
		}
	}

	return c.JSON(result)
}

func DeleteProductAttribute(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.ProductAttribute{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Product attribute deleted"})
}

// ============================================
// 產品屬性值 (ProductAttributeValue) CRUD
// ============================================

func GetProductAttributeValues(c *fiber.Ctx) error {
	productID := c.Query("product_id")
	attributeID := c.Query("attribute_id")
	
	var values []models.ProductAttributeValue
	query := database.DB

	if productID != "" {
		query = query.Where("product_id = ?", productID)
	}
	if attributeID != "" {
		query = query.Where("attribute_id = ?", attributeID)
	}

	if err := query.Preload("Product").Preload("Attribute").Find(&values).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"data": values})
}

func CreateProductAttributeValue(c *fiber.Ctx) error {
	var value models.ProductAttributeValue
	if err := c.BodyParser(&value); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Create(&value).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(value)
}

func UpdateProductAttributeValue(c *fiber.Ctx) error {
	id := c.Params("id")
	var value models.ProductAttributeValue

	if err := database.DB.First(&value, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Product attribute value not found"})
	}

	if err := c.BodyParser(&value); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if err := database.DB.Save(&value).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(value)
}

func DeleteProductAttributeValue(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := database.DB.Delete(&models.ProductAttributeValue{}, "id = ?", id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Product attribute value deleted"})
}

// ============================================
// 品牌 (Brand) CRUD
// ============================================

func GetBrands(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var brands []models.Brand
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Brand{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Find(&brands).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  brands,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetBrand(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var brand models.Brand

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&brand).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Brand not found"})
	}

	return c.JSON(brand)
}

func CreateBrand(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var brand models.Brand
	if err := c.BodyParser(&brand); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	brand.TenantID = tenantID

	if err := database.DB.Create(&brand).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(brand)
}

func UpdateBrand(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var brand models.Brand

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&brand).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Brand not found"})
	}

	if err := c.BodyParser(&brand); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	brand.TenantID = tenantID

	if err := database.DB.Save(&brand).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(brand)
}

func DeleteBrand(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Brand{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Brand deleted"})
}

