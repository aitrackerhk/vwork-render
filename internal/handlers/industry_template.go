package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func getUnknownTemplateAllModules() []string {
	modules := []string{
		"inventory", "pos", "customers", "orders", "products", "member",
		"coupons", "points", "service", "appointments", "service_orders",
		"users", "calendars", "reminders", "purchase", "suppliers",
		"warehouses", "warehouse", "logistics", "projects", "accounting", "invoices", "payments",
		"hr", "vehicles", "dining", "courses",
	}
	// 向後兼容：歷史上曾出現 project/projects 兩種代碼
	modules = append(modules, "project")
	return modules
}

func isUnknownTemplate(t models.IndustryTemplate) bool {
	code := strings.ToLower(strings.TrimSpace(t.Code))
	if code == "unknown" || code == "unknown-template" {
		return true
	}
	name := strings.TrimSpace(t.Name)
	if strings.Contains(name, "不清楚") {
		return true
	}
	if t.NameEn != nil {
		nameEn := strings.ToLower(strings.TrimSpace(*t.NameEn))
		if strings.Contains(nameEn, "unknown") || strings.Contains(nameEn, "uncertain") || strings.Contains(nameEn, "unsure") {
			return true
		}
	}
	return false
}

// GetIndustryTemplates 獲取所有行業模板
func GetIndustryTemplates(c *fiber.Ctx) error {
	var templates []models.IndustryTemplate
	query := database.DB.Model(&models.IndustryTemplate{})

	// 只返回激活的模板
	isActive := c.Query("is_active", "true")
	if isActive == "true" {
		query = query.Where("is_active = ?", true)
	}

	// 排序
	query = query.Order("sort_order ASC, name ASC")

	if err := query.Find(&templates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch industry templates",
		})
	}

	return c.JSON(fiber.Map{
		"data": templates,
	})
}

// GetIndustryTemplate 獲取單個行業模板
func GetIndustryTemplate(c *fiber.Ctx) error {
	id := c.Params("id")
	var template models.IndustryTemplate

	if err := database.DB.First(&template, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Industry template not found",
		})
	}

	return c.JSON(fiber.Map{
		"data": template,
	})
}

// ApplyIndustryTemplate 應用行業模板到當前租戶
func ApplyIndustryTemplate(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	templateID := c.Params("id")
	if templateID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Template ID is required",
		})
	}

	// 處理"不清楚"選項：啟用所有模塊（支援 unknown 和 unknown-template）
	if templateID == "unknown" || templateID == "unknown-template" {
		allModules := getUnknownTemplateAllModules()

		// 開始事務
		tx := database.DB.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		// 啟用所有模塊
		for _, moduleCode := range allModules {
			tenantModule := models.TenantModule{
				TenantID:   tenantID,
				ModuleCode: moduleCode,
				IsEnabled:  true,
			}

			if err := tx.Where("tenant_id = ? AND module_code = ?", tenantID, moduleCode).
				FirstOrCreate(&tenantModule).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to create tenant module",
				})
			}

			if err := tx.Model(&tenantModule).
				Where("tenant_id = ? AND module_code = ?", tenantID, moduleCode).
				Update("is_enabled", true).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to enable module",
				})
			}
		}

		// 標記租戶已選擇"不清楚"模板（在 extra_fields 中存儲標記）
		var tenant models.Tenant
		if err := tx.First(&tenant, "id = ?", tenantID).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to find tenant",
			})
		}

		// 更新 extra_fields，標記已選擇"不清楚"
		if tenant.ExtraFields == nil {
			tenant.ExtraFields = make(models.JSONB)
		}
		tenant.ExtraFields["industry_template_unknown"] = true
		if err := tx.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update tenant",
			})
		}

		// 「不清楚」啟用所有模塊，自動建立「課程」預設服務類別
		courseCode := "course"
		var existingCourse models.ServiceType
		if err := tx.Where("tenant_id = ? AND code = ?", tenantID, courseCode).First(&existingCourse).Error; err != nil {
			courseType := models.ServiceType{
				TenantID:    tenantID,
				Name:        "課程",
				Code:        &courseCode,
				Description: "課程教學類服務",
				Status:      "active",
			}
			tx.Create(&courseType) // 非關鍵，忽略錯誤
		}

		if err := tx.Commit().Error; err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to apply template",
			})
		}

		return c.JSON(fiber.Map{
			"message":      "All modules enabled successfully",
			"is_ecommerce": false,
		})
	}

	// 獲取模板（支援 UUID 或 code）
	var template models.IndustryTemplate
	var queryErr error

	// 先嘗試按 ID 查找
	queryErr = database.DB.First(&template, "id = ? AND is_active = ?", templateID, true).Error
	if queryErr != nil {
		// 如果按 ID 找不到，嘗試按 code 查找（支援 "unknown" code）
		queryErr = database.DB.First(&template, "code = ? AND is_active = ?", templateID, true).Error
	}

	if queryErr != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Industry template not found or inactive",
		})
	}

	// 若選到的是「不清楚」模板（以 code/name 判斷），強制啟用所有模塊
	if isUnknownTemplate(template) {
		allModules := getUnknownTemplateAllModules()

		// 開始事務
		tx := database.DB.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		// 更新租戶的行業模板關聯
		if err := tx.Model(&models.Tenant{}).
			Where("id = ?", tenantID).
			Update("industry_template_id", template.ID).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update tenant template",
			})
		}

		// 啟用所有模塊
		for _, moduleCode := range allModules {
			tenantModule := models.TenantModule{
				TenantID:   tenantID,
				ModuleCode: moduleCode,
				IsEnabled:  true,
			}

			if err := tx.Where("tenant_id = ? AND module_code = ?", tenantID, moduleCode).
				FirstOrCreate(&tenantModule).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to create tenant module",
				})
			}

			if err := tx.Model(&tenantModule).
				Where("tenant_id = ? AND module_code = ?", tenantID, moduleCode).
				Update("is_enabled", true).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to enable module",
				})
			}
		}

		// 標記租戶已選擇"不清楚"模板（在 extra_fields 中存儲標記）
		var tenant models.Tenant
		if err := tx.First(&tenant, "id = ?", tenantID).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to find tenant",
			})
		}

		if tenant.ExtraFields == nil {
			tenant.ExtraFields = make(models.JSONB)
		}
		tenant.ExtraFields["industry_template_unknown"] = true
		if err := tx.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update tenant",
			})
		}

		// 「不清楚」啟用所有模塊，自動建立「課程」預設服務類別
		courseCode2 := "course"
		var existingCourse2 models.ServiceType
		if err := tx.Where("tenant_id = ? AND code = ?", tenantID, courseCode2).First(&existingCourse2).Error; err != nil {
			courseType := models.ServiceType{
				TenantID:    tenantID,
				Name:        "課程",
				Code:        &courseCode2,
				Description: "課程教學類服務",
				Status:      "active",
			}
			tx.Create(&courseType) // 非關鍵，忽略錯誤
		}

		if err := tx.Commit().Error; err != nil {
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to apply template",
			})
		}

		return c.JSON(fiber.Map{
			"message":      "All modules enabled successfully",
			"is_ecommerce": false,
		})
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 更新租戶的行業模板關聯
	if err := tx.Model(&models.Tenant{}).
		Where("id = ?", tenantID).
		Update("industry_template_id", template.ID).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update tenant template",
		})
	}

	// 清除 industry_template_unknown 標記（如果之前選了"不清楚"模板）
	var tenant models.Tenant
	if err := tx.First(&tenant, "id = ?", tenantID).Error; err == nil {
		if tenant.ExtraFields != nil {
			if _, ok := tenant.ExtraFields["industry_template_unknown"]; ok {
				delete(tenant.ExtraFields, "industry_template_unknown")
				tx.Model(&tenant).Update("extra_fields", tenant.ExtraFields)
			}
		}
	}

	// 重選模板時，先禁用所有現有模塊
	if err := tx.Model(&models.TenantModule{}).
		Where("tenant_id = ?", tenantID).
		Update("is_enabled", false).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to disable existing modules",
		})
	}

	// 應用模塊配置
	var enabledModules []interface{}

	// JSONB 是 map[string]interface{} 類型，但數據可能是數組
	// 檢查是否有 _data 鍵（當 JSONB 包裝數組時）
	if template.EnabledModules != nil {
		if data, ok := template.EnabledModules["_data"]; ok {
			if arr, ok := data.([]interface{}); ok {
				enabledModules = arr
			}
		} else {
			// 嘗試直接從 JSONB map 中提取數組
			// 通常 PostgreSQL JSONB 數組會被存儲為 map
			// 我們需要檢查是否有數組類型的值
			for _, val := range template.EnabledModules {
				if arr, ok := val.([]interface{}); ok {
					enabledModules = arr
					break
				}
			}
		}
	}

	// 如果模板有定義模塊，則應用
	if len(enabledModules) > 0 {
		// 展開模塊：處理向後兼容與關聯模塊
		expanded := make([]string, 0, len(enabledModules)+4)
		seen := map[string]bool{}
		add := func(code string) {
			code = strings.TrimSpace(code)
			if code == "" {
				return
			}
			if !seen[code] {
				seen[code] = true
				expanded = append(expanded, code)
			}
		}

		for _, module := range enabledModules {
			moduleCode, ok := module.(string)
			if !ok {
				continue
			}
			add(moduleCode)
			// 關聯/兼容規則：
			// - service_orders -> appointments（服務單通常伴隨預約/排程）
			// - inventory/warehouses -> warehouse（庫存主選單使用 warehouse 代碼）
			// - warehouse -> warehouses（保留舊版 warehouses 代碼）
			// - projects <-> project（歷史代碼兼容）
			if moduleCode == "service_orders" {
				add("appointments")
			}
			if moduleCode == "courses" {
				add("service")
				add("service_orders")
				add("appointments")
			}
			if moduleCode == "inventory" || moduleCode == "warehouses" {
				add("warehouse")
			}
			if moduleCode == "warehouse" {
				add("warehouses")
			}
			if moduleCode == "projects" {
				add("project")
			}
			if moduleCode == "project" {
				add("projects")
			}
		}

		// 主要預設模板應包含庫存與物流（與新菜單結構對齊）
		switch template.Code {
		case "retail", "ecommerce", "manufacturing", "logistics", "sme":
			add("warehouse")
			add("logistics")
		}

		// 全部模板都應包含會計與 HR 模組
		add("accounting")
		add("hr")

		for _, moduleCode := range expanded {
			// 創建或更新租戶模塊
			tenantModule := models.TenantModule{
				TenantID:   tenantID,
				ModuleCode: moduleCode,
				IsEnabled:  true,
			}

			// 使用 FirstOrCreate 確保不重複
			if err := tx.Where("tenant_id = ? AND module_code = ?", tenantID, moduleCode).
				FirstOrCreate(&tenantModule).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to create tenant module",
				})
			}

			// 確保啟用
			if err := tx.Model(&tenantModule).
				Where("tenant_id = ? AND module_code = ?", tenantID, moduleCode).
				Update("is_enabled", true).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to enable module",
				})
			}
		}
	}

	// 如果是電商模板，自動導入默認頁面
	if template.Code == "ecommerce" {
		if err := createDefaultEcommercePages(tx, tenantID); err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to create default pages: " + err.Error(),
			})
		}

		// 設置網站相關配置
		theme := "default"
		websiteType := "ecommerce"
		if err := tx.Model(&models.Tenant{}).
			Where("id = ?", tenantID).
			Updates(map[string]interface{}{
				"website_theme":   theme,
				"website_type":    websiteType,
				"website_enabled": true,
			}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to update website settings",
			})
		}
	}

	// 如果是教育模板，自動建立「課程」預設服務類別
	if template.Code == "education" {
		courseCode := "course"
		var existing models.ServiceType
		if err := tx.Where("tenant_id = ? AND code = ?", tenantID, courseCode).First(&existing).Error; err != nil {
			// 不存在則建立
			courseType := models.ServiceType{
				TenantID:    tenantID,
				Name:        "課程",
				Code:        &courseCode,
				Description: "課程教學類服務",
				Status:      "active",
			}
			if err := tx.Create(&courseType).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{
					"error": "Failed to create default course service type: " + err.Error(),
				})
			}
		}
	}

	// 如果模板包含 service 模組，也自動建立「課程」服務類別（非教育模板亦可使用）
	if template.Code == "service" {
		courseCode := "course"
		var existing models.ServiceType
		if err := tx.Where("tenant_id = ? AND code = ?", tenantID, courseCode).First(&existing).Error; err != nil {
			courseType := models.ServiceType{
				TenantID:    tenantID,
				Name:        "課程",
				Code:        &courseCode,
				Description: "課程教學類服務",
				Status:      "active",
			}
			tx.Create(&courseType) // 忽略錯誤，非關鍵
		}
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to apply template",
		})
	}

	return c.JSON(fiber.Map{
		"message":      "Industry template applied successfully",
		"data":         template,
		"is_ecommerce": template.Code == "ecommerce",
	})
}

// GetTenantIndustryTemplate 獲取當前租戶的行業模板
func GetTenantIndustryTemplate(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	var tenant models.Tenant
	if err := database.DB.Preload("IndustryTemplate").First(&tenant, "id = ?", tenantID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Tenant not found",
		})
	}

	// 檢查是否選擇了"不清楚"模板
	if tenant.ExtraFields != nil {
		if unknown, ok := tenant.ExtraFields["industry_template_unknown"].(bool); ok && unknown {
			// 返回虛擬模板對象，表示已選擇"不清楚"
			return c.JSON(fiber.Map{
				"data": fiber.Map{
					"id":   "unknown",
					"code": "unknown",
					"name": "不清楚",
				},
			})
		}
	}

	if tenant.IndustryTemplate == nil {
		return c.JSON(fiber.Map{
			"data": nil,
		})
	}

	return c.JSON(fiber.Map{
		"data": tenant.IndustryTemplate,
	})
}

// createDefaultEcommercePages 為電商模板創建默認頁面
func createDefaultEcommercePages(tx *gorm.DB, tenantID uuid.UUID) error {
	var tenant models.Tenant
	if err := tx.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return err
	}

	// Keep in sync with default page generation logic in page.go
	copy := defaultCopyForLang(resolveTenantDefaultPageLang(tenant))

	// 獲取租戶 logo：如果有上傳 logo 圖片則用圖片，否則用文字
	logoURL, logoText := getTenantLogoForPages(tx, tenantID, tenant.Name)

	// Create shared nav/footer blocks
	navData := models.JSONB{
		"logo": logoURL, "logo_text": logoText, "show_login_icon": true, "show_cart_icon": true,
		"fixed": false, "menu_position": "right", "padding": "0.75rem 2rem",
		"menu_items": defaultEcommerceNavMenuItems(tenant.Subdomain, copy),
	}
	footerData := models.JSONB{
		"logo": logoURL, "column1_content": "公司簡介文字", "column1_content_i18n": "publicSite.footer.companyIntro",
		"column2_menu_items": []interface{}{}, "column3_menu_items": []interface{}{},
		"column4_menu_items": []interface{}{}, "copyright": "© 2025 版權所有", "copyright_i18n": "publicSite.footer.copyright",
		"padding": "2rem 0",
	}
	navBlock, footerBlock, err := createOrGetNavFooterBlocks(tx, tenantID, "nav", navData, footerData)
	if err != nil {
		return err
	}

	navComp := func(sortOrder int) ComponentConfig {
		return ComponentConfig{componentType: "nav", componentData: navData, sortOrder: sortOrder, blockID: &navBlock.ID}
	}
	footerComp := func(sortOrder int) ComponentConfig {
		return ComponentConfig{componentType: "footer", componentData: footerData, sortOrder: sortOrder, blockID: &footerBlock.ID}
	}

	pages := []PageConfig{
		{
			name:   "首頁",
			slug:   "home",
			isHome: true,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "hero", componentData: models.JSONB{
					"title": "歡迎", "title_i18n": "publicSite.hero.welcome",
					"subtitle":    tenant.Name,
					"button_text": "開始探索", "button_text_i18n": "publicSite.hero.startExploring", "button_link": "#",
					"background_image": "",
					"min_height":       "300px", "padding": "3rem", "text_align": "center",
				}, sortOrder: 1},
				{componentType: "product-list", componentData: models.JSONB{
					"full_list": false, "limit": 12, "columns": 4,
					"show_product_type_filter": false, "show_brand_filter": false,
					"product_detail_page": "/page/product-detail",
					"padding":             "2rem 0", "gap": "1rem",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   "登入",
			slug:   "login",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "login-register", componentData: models.JSONB{
					"show_login": true, "show_register": true,
					"login_method": "phone_or_email", "redirect_after_login": "/",
					"redirect_after_register": "/",
				}, sortOrder: 1},
				footerComp(2),
			},
		},
		{
			name:   "商店",
			slug:   "shop",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": "商店", "text_i18n": "publicSite.pages.shop.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "product-list", componentData: models.JSONB{
					"full_list": true, "limit": 24, "columns": 4,
					"show_product_type_filter": true, "show_brand_filter": true,
					"padding": "2rem 0", "gap": "1rem",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   "產品詳情",
			slug:   "product-detail",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "product-detail", componentData: models.JSONB{
					"product_name": "產品名稱", "product_name_i18n": "publicSite.product.placeholder.name",
					"product_price":       "$0.00",
					"product_description": "產品描述", "product_description_i18n": "publicSite.product.placeholder.description",
					"padding": "2rem 0",
				}, sortOrder: 1},
				footerComp(2),
			},
		},
		{
			name:   "購物車",
			slug:   "cart",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "cart", componentData: models.JSONB{
					"show_checkout_button": true, "show_continue_shopping": true,
					"show_coupon": true, "show_points": true,
				}, sortOrder: 1},
				footerComp(2),
			},
		},
		{
			name:   "結帳",
			slug:   "checkout",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "checkout", componentData: models.JSONB{
					"show_shipping_form": true, "show_payment_form": true,
				}, sortOrder: 1},
				footerComp(2),
			},
		},
		{
			name:   "用戶資料",
			slug:   "user",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "user-area", componentData: models.JSONB{
					"show_profile": true, "show_orders": true, "show_addresses": true,
					"login_page": "",
				}, sortOrder: 1},
				footerComp(2),
			},
		},
	}

	return createPagesFromConfig(tx, tenantID, pages)
}
