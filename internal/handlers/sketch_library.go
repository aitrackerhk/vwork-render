package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// Image library categories
var imageLibraryCategories = []fiber.Map{
	{"id": "all", "label": "全部"},
	{"id": "promo", "label": "促銷優惠"},
	{"id": "product", "label": "產品介紹"},
	{"id": "device", "label": "儀器推廣"},
	{"id": "result", "label": "效果見證"},
	{"id": "event", "label": "節日公告"},
}

// Image library items — AI-generated templates
var imageLibraryItems = []fiber.Map{
	{"id": "blank", "category": "all", "title": "空白畫布", "thumbnail": "", "is_blank": true},
	{"id": "tpl_01", "category": "device", "title": "EMS Body Sculpting Device", "thumbnail": "/static/img/sketch-library/template_01.png"},
	{"id": "tpl_02", "category": "device", "title": "Teeth Whitening Laser", "thumbnail": "/static/img/sketch-library/template_02.png"},
	{"id": "tpl_03", "category": "promo", "title": "Coffee BOGO Promo", "thumbnail": "/static/img/sketch-library/template_03.png"},
	{"id": "tpl_04", "category": "event", "title": "Studio Holiday Hours", "thumbnail": "/static/img/sketch-library/template_04.png"},
	{"id": "tpl_05", "category": "promo", "title": "Pet Grooming Trial $29", "thumbnail": "/static/img/sketch-library/template_05.png"},
	{"id": "tpl_06", "category": "product", "title": "Earbuds Comparison", "thumbnail": "/static/img/sketch-library/template_06.png"},
	{"id": "tpl_07", "category": "promo", "title": "Keratin Treatment Deal", "thumbnail": "/static/img/sketch-library/template_07.png"},
	{"id": "tpl_08", "category": "result", "title": "Fitness Transformation", "thumbnail": "/static/img/sketch-library/template_08.png"},
	{"id": "tpl_09", "category": "device", "title": "Bubble Tea Grand Opening", "thumbnail": "/static/img/sketch-library/template_09.png"},
}

// GetImageLibrary returns the image library categories and items
// GET /api/v1/ai/image-library
func GetImageLibrary(c *fiber.Ctx) error {
	category := c.Query("category", "all")

	// Filter items by category
	var filtered []fiber.Map
	for _, item := range imageLibraryItems {
		itemCat, _ := item["category"].(string)
		isBlank, _ := item["is_blank"].(bool)

		if category == "all" || itemCat == category || isBlank {
			// Build thumbnail URL with full host for mobile clients
			thumb, _ := item["thumbnail"].(string)
			if thumb != "" {
				// Return relative path — clients prepend the base URL
				// (web uses relative, mobile prepends AppConfig.apiBaseUrl)
			}
			filtered = append(filtered, item)
		}
	}

	return c.JSON(fiber.Map{
		"categories": imageLibraryCategories,
		"items":      filtered,
	})
}
