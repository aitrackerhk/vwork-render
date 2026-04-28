package handlers

import (
	"fmt"
	"net/url"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/themes"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetPages 獲取頁面列表
func GetPages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var pages []models.Page

	query := database.DB.Where("tenant_id = ? AND trashed_at IS NULL", tenantID)

	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR slug ILIKE ? OR title ILIKE ?", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	// 狀態過濾
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.Page{}).Where("tenant_id = ? AND trashed_at IS NULL", tenantID).Count(&total)

	if err := query.
		Offset(offset).Limit(limit).
		Order("created_at DESC").
		Find(&pages).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch pages: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  pages,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetPage 獲取單個頁面（包含元件）
func GetPage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	pageID := c.Params("id")

	var page models.Page
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, pageID).
		Preload("Components", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC")
		}).
		First(&page).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Page not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch page: %v", err)})
	}

	// Resolve linked blocks so editor always shows the block's latest data
	resolveLinkedBlocks(tenantID, &page)

	return c.JSON(page)
}

// CreatePage 創建頁面
func CreatePage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Name           string                 `json:"name"`
		Slug           string                 `json:"slug"`
		Title          *string                `json:"title,omitempty"`
		Description    *string                `json:"description,omitempty"`
		TopnavStyle    string                 `json:"topnav_style"`
		Status         string                 `json:"status"`
		IsHomepage     bool                   `json:"is_homepage"`
		SEOTitle       *string                `json:"seo_title,omitempty"`
		SEODescription *string                `json:"seo_description,omitempty"`
		SEOKeywords    *string                `json:"seo_keywords,omitempty"`
		ExtraFields    map[string]interface{} `json:"extra_fields,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// 驗證必填字段
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if req.Slug == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Slug is required"})
	}

	// 檢查 slug 是否已存在
	var existingPage models.Page
	if err := database.DB.Where("tenant_id = ? AND slug = ?", tenantID, req.Slug).First(&existingPage).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Slug already exists"})
	}

	// 如果設為首頁，取消其他首頁標記
	if req.IsHomepage {
		database.DB.Model(&models.Page{}).
			Where("tenant_id = ? AND is_homepage = ?", tenantID, true).
			Update("is_homepage", false)
	}

	now := time.Now()
	page := models.Page{
		TenantID:       tenantID,
		Name:           req.Name,
		Slug:           req.Slug,
		Title:          req.Title,
		Description:    req.Description,
		TopnavStyle:    req.TopnavStyle,
		Status:         req.Status,
		IsHomepage:     req.IsHomepage,
		SEOTitle:       req.SEOTitle,
		SEODescription: req.SEODescription,
		SEOKeywords:    req.SEOKeywords,
		CreatedBy:      &userID,
		UpdatedBy:      &userID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if req.ExtraFields != nil {
		page.ExtraFields = models.JSONB(req.ExtraFields)
	} else {
		page.ExtraFields = make(models.JSONB)
	}

	if req.TopnavStyle == "" {
		page.TopnavStyle = "default"
	}
	if req.Status == "" {
		page.Status = "draft"
	}

	if err := database.DB.Create(&page).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create page: %v", err)})
	}

	return c.Status(201).JSON(page)
}

// UpdatePage 更新頁面
func UpdatePage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	pageID := c.Params("id")

	var req struct {
		Name           string                 `json:"name"`
		Slug           string                 `json:"slug"`
		Title          *string                `json:"title,omitempty"`
		Description    *string                `json:"description,omitempty"`
		TopnavStyle    string                 `json:"topnav_style"`
		Status         string                 `json:"status"`
		IsHomepage     bool                   `json:"is_homepage"`
		SEOTitle       *string                `json:"seo_title,omitempty"`
		SEODescription *string                `json:"seo_description,omitempty"`
		SEOKeywords    *string                `json:"seo_keywords,omitempty"`
		ExtraFields    map[string]interface{} `json:"extra_fields,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	var page models.Page
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, pageID).First(&page).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Page not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch page: %v", err)})
	}

	// 如果 slug 改變，檢查新 slug 是否已存在
	if req.Slug != "" && req.Slug != page.Slug {
		var existingPage models.Page
		if err := database.DB.Where("tenant_id = ? AND slug = ? AND id != ?", tenantID, req.Slug, pageID).First(&existingPage).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Slug already exists"})
		}
		page.Slug = req.Slug
	}

	// 如果設為首頁，取消其他首頁標記
	if req.IsHomepage && !page.IsHomepage {
		database.DB.Model(&models.Page{}).
			Where("tenant_id = ? AND is_homepage = ? AND id != ?", tenantID, true, pageID).
			Update("is_homepage", false)
	}

	// 更新字段
	if req.Name != "" {
		page.Name = req.Name
	}
	if req.Title != nil {
		page.Title = req.Title
	}
	if req.Description != nil {
		page.Description = req.Description
	}
	if req.TopnavStyle != "" {
		page.TopnavStyle = req.TopnavStyle
	}
	if req.Status != "" {
		page.Status = req.Status
	}
	page.IsHomepage = req.IsHomepage
	if req.SEOTitle != nil {
		page.SEOTitle = req.SEOTitle
	}
	if req.SEODescription != nil {
		page.SEODescription = req.SEODescription
	}
	if req.SEOKeywords != nil {
		page.SEOKeywords = req.SEOKeywords
	}
	if req.ExtraFields != nil {
		page.ExtraFields = models.JSONB(req.ExtraFields)
	}

	page.UpdatedBy = &userID
	page.UpdatedAt = time.Now()

	if err := database.DB.Save(&page).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update page: %v", err)})
	}

	return c.JSON(page)
}

// DeletePage 刪除頁面
func DeletePage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	pageID := c.Params("id")

	var page models.Page
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, pageID).First(&page).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Page not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch page: %v", err)})
	}

	// 刪除頁面（會級聯刪除元件）
	if err := database.DB.Delete(&page).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete page: %v", err)})
	}

	return c.JSON(fiber.Map{"message": "Page deleted successfully"})
}

// GetPageComponents 獲取頁面的所有元件
func GetPageComponents(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	pageID := c.Params("id")

	var components []models.PageComponent
	if err := database.DB.
		Where("tenant_id = ? AND page_id = ?", tenantID, pageID).
		Order("sort_order ASC").
		Find(&components).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch components: %v", err)})
	}

	// Resolve linked blocks so components always reflect block's latest data
	tmpPage := &models.Page{Components: components}
	resolveLinkedBlocks(tenantID, tmpPage)

	return c.JSON(tmpPage.Components)
}

// UpdatePageComponents 批量更新頁面元件（用於拖拽排序和編輯）
func UpdatePageComponents(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	pageID := c.Params("id")

	var req struct {
		Components []struct {
			ID            *uuid.UUID             `json:"id,omitempty"`       // 新建時為 null
			BlockID       *uuid.UUID             `json:"block_id,omitempty"` // Reference to a linked block
			ComponentType string                 `json:"component_type"`
			ComponentData map[string]interface{} `json:"component_data"`
			SortOrder     int                    `json:"sort_order"`
			IsActive      bool                   `json:"is_active"`
		} `json:"components"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// 驗證頁面存在
	var page models.Page
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, pageID).First(&page).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Page not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch page: %v", err)})
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 刪除現有元件
	if err := tx.Where("tenant_id = ? AND page_id = ?", tenantID, pageID).Delete(&models.PageComponent{}).Error; err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete existing components: %v", err)})
	}

	// 創建新元件
	now := time.Now()
	for _, compReq := range req.Components {
		component := models.PageComponent{
			TenantID:      tenantID,
			PageID:        uuid.MustParse(pageID),
			BlockID:       compReq.BlockID,
			ComponentType: compReq.ComponentType,
			ComponentData: models.JSONB(compReq.ComponentData),
			SortOrder:     compReq.SortOrder,
			IsActive:      compReq.IsActive,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if component.ComponentData == nil {
			component.ComponentData = make(models.JSONB)
		}

		// 如果提供了 ID，先检查是否存在，如果存在则更新，否则创建新记录
		if compReq.ID != nil {
			var existingComponent models.PageComponent
			if err := tx.Where("id = ? AND tenant_id = ? AND page_id = ?", *compReq.ID, tenantID, pageID).First(&existingComponent).Error; err == nil {
				// 存在，更新
				existingComponent.BlockID = compReq.BlockID
				existingComponent.ComponentType = compReq.ComponentType
				existingComponent.ComponentData = models.JSONB(compReq.ComponentData)
				existingComponent.SortOrder = compReq.SortOrder
				existingComponent.IsActive = compReq.IsActive
				existingComponent.UpdatedAt = now
				if err := tx.Save(&existingComponent).Error; err != nil {
					tx.Rollback()
					return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update component: %v", err)})
				}
				continue
			}
			// 不存在，使用提供的 ID 创建新记录
			component.ID = *compReq.ID
		}

		if err := tx.Create(&component).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create component: %v", err)})
		}
	}

	// Sync linked blocks: when a component references a block, push the
	// component's latest component_data back to the block so that the
	// public page (which reads from the block) reflects the editor changes.
	for _, compReq := range req.Components {
		if compReq.BlockID != nil && *compReq.BlockID != uuid.Nil && compReq.ComponentData != nil {
			if err := tx.Model(&models.Block{}).
				Where("id = ? AND tenant_id = ?", *compReq.BlockID, tenantID).
				Updates(map[string]interface{}{
					"component_data": models.JSONB(compReq.ComponentData),
					"component_type": compReq.ComponentType,
					"updated_at":     now,
				}).Error; err != nil {
				tx.Rollback()
				return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to sync block data: %v", err)})
			}
		}
	}

	tx.Commit()

	// 返回更新後的元件列表
	var components []models.PageComponent
	database.DB.Where("tenant_id = ? AND page_id = ?", tenantID, pageID).
		Order("sort_order ASC").
		Find(&components)

	return c.JSON(components)
}

func renderPublicPage(c *fiber.Ctx, subdomain string, slug string) error {

	// 根據子域名查找租戶
	var tenant models.Tenant
	if err := database.DB.Where("subdomain = ? AND status = ?", subdomain, "active").First(&tenant).Error; err != nil {
		// 租戶不存在，顯示友好的錯誤頁面
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "暫時沒有此頁",
		}, "layouts/embed_layout")
	}

	// 檢查網站功能是否啟用
	if !tenant.WebsiteEnabled {
		// 網站功能未啟用，顯示友好的錯誤頁面
		return c.Render("pages/public_page_not_found", fiber.Map{
			"Message": "網站暫時關閉",
		}, "layouts/embed_layout")
	}

	// 如果 slug 為空，查找首頁
	var page models.Page
	if slug == "" {
		// 查找首頁（排除已放入垃圾筒的頁面）
		if err := database.DB.
			Where("tenant_id = ? AND is_homepage = ? AND status IN ? AND trashed_at IS NULL", tenant.ID, true, []string{"draft", "published"}).
			Preload("Components", func(db *gorm.DB) *gorm.DB {
				return db.Where("is_active = ?", true).Order("sort_order ASC")
			}).
			First(&page).Error; err != nil {
			// 沒有首頁，顯示友好的錯誤頁面
			return c.Render("pages/public_page_not_found", fiber.Map{
				"Message": "暫時沒有此頁",
			}, "layouts/embed_layout")
		}
	} else {
		// 查找指定 slug 的頁面（允許 draft 和 published 狀態，排除已放入垃圾筒的頁面）
		if err := database.DB.
			Where("tenant_id = ? AND slug = ? AND status IN ? AND trashed_at IS NULL", tenant.ID, slug, []string{"draft", "published"}).
			Preload("Components", func(db *gorm.DB) *gorm.DB {
				return db.Where("is_active = ?", true).Order("sort_order ASC")
			}).
			First(&page).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// 頁面不存在，顯示友好的錯誤頁面
				return c.Render("pages/public_page_not_found", fiber.Map{
					"Message": "暫時沒有此頁",
				}, "layouts/embed_layout")
			}
			return c.Status(500).SendString("查詢頁面失敗")
		}
	}

	// Resolve linked blocks: for components with block_id, fetch the block's latest data
	// and override component_type + component_data so changes to a block propagate to all pages.
	resolveLinkedBlocks(tenant.ID, &page)

	// Resolve relative nav/footer links: convert stored relative paths (e.g. "/shop")
	// to full tenant paths (e.g. "/co/{subdomain}/shop") for rendering.
	resolveComponentLinks(&page, subdomain)

	// 只統計 GET（不統計 HEAD）；且只有在成功找到頁面後才記錄
	if c.Method() == fiber.MethodGet {
		recordPageViewDaily(tenant.ID, page.ID, time.Now())
	}

	// 前台預設語言：跟隨租戶網站設定（tenant.extra_fields.default_language），避免依瀏覽器語言「自動翻譯」
	// map 到 i18n.js 支援的語系代碼：zh / zh-CN / en
	publicDefaultLang := "zh"
	switch resolveTenantDefaultPageLang(tenant) {
	case "en":
		publicDefaultLang = "en"
	case "zh-hans":
		publicDefaultLang = "zh-CN"
	default:
		publicDefaultLang = "zh"
	}

	// 如果是 cart 或 checkout 頁面，檢查客戶是否已登錄
	if slug == "cart" || slug == "checkout" {
		// 從 cookie 中獲取客戶 ID
		customerIDCookie := c.Cookies("customer_id")
		if customerIDCookie == "" {
			// 未登錄，重定向到登錄頁面
			return c.Redirect("/co/" + subdomain + "/login")
		}
	}

	// 獲取產品列表（用於產品列表元件）
	// 先獲取所有活躍產品，模板中會根據 limit 限制顯示數量
	var products []models.Product
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenant.ID, "active").
		Order("created_at DESC").
		Limit(100). // 限制最多100個產品
		Find(&products).Error; err != nil {
		// 如果獲取產品失敗，使用空列表
		products = []models.Product{}
	}

	// 獲取服務列表（用於服務列表元件）
	var services []models.Service
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenant.ID, "active").
		Order("created_at DESC").
		Limit(100).
		Find(&services).Error; err != nil {
		services = []models.Service{}
	}

	// 獲取網店付款方式（用於結帳元件）
	var paymentMethods []models.PaymentMethod
	if err := database.DB.
		Where("tenant_id = ? AND status = ? AND is_online_payment = ?", tenant.ID, "active", true).
		Order("created_at ASC").
		Find(&paymentMethods).Error; err != nil {
		// 如果獲取付款方式失敗，使用空列表
		paymentMethods = []models.PaymentMethod{}
	}

	// 獲取運送方式 / 店舖（用於 checkout：送貨 vs 自取）
	var shippingMethods []models.ShippingMethod
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenant.ID, "active").
		Order("is_default DESC, created_at ASC").
		Find(&shippingMethods).Error; err != nil {
		shippingMethods = []models.ShippingMethod{}
	}
	var stores []models.Store
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenant.ID, "active").
		Order("created_at ASC").
		Find(&stores).Error; err != nil {
		stores = []models.Store{}
	}

	// Public chat enabled? default enabled
	publicChatEnabled := true
	if tenant.ExtraFields != nil {
		if v, ok := tenant.ExtraFields["public_chat_enabled"]; ok {
			switch vv := v.(type) {
			case bool:
				publicChatEnabled = vv
			case string:
				s := strings.ToLower(strings.TrimSpace(vv))
				publicChatEnabled = (s == "true" || s == "1" || s == "on")
			case float64:
				publicChatEnabled = int(vv) == 1
			case int:
				publicChatEnabled = vv == 1
			}
		}
	}

	// 獲取租戶圖標（從 extra_fields 中獲取）
	tenantIcon := "/static/vworkicon.png" // 默認使用 vwork 圖標
	if tenant.ExtraFields != nil {
		if icon, ok := tenant.ExtraFields["website_icon"].(string); ok && icon != "" {
			tenantIcon = icon
		}
	}

	// 餐飲模組是否啟用（industry template 對應）
	diningEnabled := false
	if tenant.ID != uuid.Nil {
		var count int64
		if err := database.DB.Model(&models.TenantModule{}).
			Where("tenant_id = ? AND module_code IN ? AND is_enabled = ?", tenant.ID, []string{"dining", "food_and_beverage"}, true).
			Count(&count).Error; err == nil {
			diningEnabled = count > 0
		}
	}

	// Resolve website theme → CSS variables
	themeID := "default"
	if tenant.WebsiteTheme != nil && *tenant.WebsiteTheme != "" {
		themeID = *tenant.WebsiteTheme
	}
	themeCSS := themes.GetThemeCSS(themeID)

	// NOTE: Component color fields are intentionally NOT filled with concrete theme values here.
	// Empty color fields mean "follow the current theme" — the public_page.html template uses
	// CSS var(--theme-xxx) fallbacks for empty values, so colors adapt when the theme is changed.
	// Only non-empty values (user-customized colors) are used as-is.

	// 渲染頁面模板
	pageTitle := page.Name
	if page.Title != nil && strings.TrimSpace(*page.Title) != "" {
		pageTitle = strings.TrimSpace(*page.Title)
	}
	if page.SEOTitle != nil && strings.TrimSpace(*page.SEOTitle) != "" {
		pageTitle = strings.TrimSpace(*page.SEOTitle)
	}
	pageDescription := ""
	if page.SEODescription != nil {
		pageDescription = strings.TrimSpace(*page.SEODescription)
	}
	if pageDescription == "" && page.Description != nil {
		pageDescription = strings.TrimSpace(*page.Description)
	}
	pageKeywords := ""
	if page.SEOKeywords != nil {
		pageKeywords = strings.TrimSpace(*page.SEOKeywords)
	}

	canonicalPath := c.Path()
	if strings.TrimSpace(canonicalPath) == "" {
		canonicalPath = "/"
	}
	imageURL := tenantIcon
	if imageURL != "" && !strings.HasPrefix(imageURL, "http://") && !strings.HasPrefix(imageURL, "https://") {
		imageURL = buildAbsoluteURL(c, imageURL)
	}

	seo := SEOData{
		Title:        pageTitle,
		Description:  pageDescription,
		Keywords:     pageKeywords,
		CanonicalURL: buildAbsoluteURL(c, canonicalPath),
		ImageURL:     imageURL,
		Type:         "website",
		Locale:       "zh_TW",
	}

	return c.Render("pages/public_page", fiber.Map{
		"Page":              page,
		"SEO":               seo,
		"Subdomain":         subdomain,
		"Products":          products,
		"Services":          services,
		"PaymentMethods":    paymentMethods,
		"PublicDefaultLang": publicDefaultLang,
		"ShippingMethods":   shippingMethods,
		"Stores":            stores,
		"PublicChatEnabled": publicChatEnabled,
		"TenantIcon":        tenantIcon,
		"DiningEnabled":     diningEnabled,
		"ThemeCSS":          themeCSS,
		"ThemeID":           themeID,
		"GoogleMapsAPIKey":  mustAppConfig().GoogleMapsAPIKey,
	})
}

// resolveLinkedBlocks resolves components that reference a block (via block_id).
// It batch-loads the referenced blocks and overrides component_type + component_data
// with the block's latest data, so editing a block propagates to all pages.
func resolveLinkedBlocks(tenantID uuid.UUID, page *models.Page) {
	// Collect all unique block IDs
	blockIDSet := make(map[uuid.UUID]bool)
	for _, comp := range page.Components {
		if comp.BlockID != nil && *comp.BlockID != uuid.Nil {
			blockIDSet[*comp.BlockID] = true
		}
	}
	if len(blockIDSet) == 0 {
		return
	}

	// Batch-load blocks
	blockIDs := make([]uuid.UUID, 0, len(blockIDSet))
	for id := range blockIDSet {
		blockIDs = append(blockIDs, id)
	}

	var blocks []models.Block
	if err := database.DB.
		Where("tenant_id = ? AND id IN ?", tenantID, blockIDs).
		Find(&blocks).Error; err != nil {
		return // Silently fall back to component's own data
	}

	blockMap := make(map[uuid.UUID]*models.Block, len(blocks))
	for i := range blocks {
		blockMap[blocks[i].ID] = &blocks[i]
	}

	// Override component data with block's latest data
	for i := range page.Components {
		comp := &page.Components[i]
		if comp.BlockID == nil || *comp.BlockID == uuid.Nil {
			continue
		}
		block, ok := blockMap[*comp.BlockID]
		if !ok {
			// Block was deleted; clear the reference and use component's own data
			comp.BlockID = nil
			continue
		}
		comp.ComponentType = block.ComponentType
		comp.ComponentData = block.ComponentData
	}
}

// resolveComponentLinks converts relative links stored in nav/header/footer/list
// component menu_items (e.g. "/shop", "/about") into full tenant-scoped paths
// (e.g. "/co/{subdomain}/shop") at render time.
// This ensures users only need to enter relative paths like "/shop" in the editor,
// and changing the tenant subdomain won't break links.
func resolveComponentLinks(page *models.Page, subdomain string) {
	prefix := "/co/" + subdomain
	for i := range page.Components {
		comp := &page.Components[i]
		if comp.ComponentData == nil {
			continue
		}
		switch comp.ComponentType {
		case "nav", "header":
			resolveMenuItemsLinks(comp.ComponentData, "menu_items", prefix)
			resolveSingleLink(comp.ComponentData, "login_icon_link", prefix)
			resolveSingleLink(comp.ComponentData, "cart_icon_link", prefix)
		case "list":
			resolveMenuItemsLinks(comp.ComponentData, "menu_items", prefix)
		case "footer":
			resolveMenuItemsLinks(comp.ComponentData, "menu_items", prefix)
			resolveMenuItemsLinks(comp.ComponentData, "column2_menu_items", prefix)
			resolveMenuItemsLinks(comp.ComponentData, "column3_menu_items", prefix)
			resolveMenuItemsLinks(comp.ComponentData, "column4_menu_items", prefix)
		case "hero":
			resolveSingleLink(comp.ComponentData, "button_link", prefix)
		case "button":
			resolveSingleLink(comp.ComponentData, "link", prefix)
		case "banner-slider":
			resolveSlidesLinks(comp.ComponentData, prefix)
		case "product-list":
			resolveSingleLink(comp.ComponentData, "product_detail_page", prefix)
		case "service-list":
			resolveSingleLink(comp.ComponentData, "service_detail_page", prefix)
		case "login-register":
			resolveSingleLink(comp.ComponentData, "redirect_after_login", prefix)
			resolveSingleLink(comp.ComponentData, "redirect_after_register", prefix)
		case "user-area", "order-list":
			resolveSingleLink(comp.ComponentData, "login_page", prefix)
		}
	}
}

// resolveMenuItemsLinks resolves link fields within a menu_items array inside component data.
// A link is considered "relative" and will be prefixed if:
//   - It starts with "/" but NOT with "/co/" (already resolved) and NOT with "/page/" (page-type link handled separately)
//   - It is not an absolute URL (http:// or https://)
//   - It is not empty or "#"
func resolveMenuItemsLinks(data models.JSONB, key string, prefix string) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return
	}
	items, ok := raw.([]interface{})
	if !ok {
		return
	}
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		link, _ := m["link"].(string)
		m["link"] = resolveOneLink(link, prefix)
	}
}

// resolveSlidesLinks resolves button_link fields inside a banner-slider's slides array.
func resolveSlidesLinks(data models.JSONB, prefix string) {
	raw, ok := data["slides"]
	if !ok || raw == nil {
		return
	}
	slides, ok := raw.([]interface{})
	if !ok {
		return
	}
	for _, slide := range slides {
		m, ok := slide.(map[string]interface{})
		if !ok {
			continue
		}
		link, _ := m["button_link"].(string)
		m["button_link"] = resolveOneLink(link, prefix)
	}
}

// resolveSingleLink resolves a single link field in component data (e.g. login_icon_link, button_link).
func resolveSingleLink(data models.JSONB, key string, prefix string) {
	raw, ok := data[key]
	if !ok || raw == nil {
		return
	}
	link, ok := raw.(string)
	if !ok {
		return
	}
	data[key] = resolveOneLink(link, prefix)
}

// resolveOneLink converts a single relative link to a full tenant-scoped path.
func resolveOneLink(link string, prefix string) string {
	if link == "" || link == "#" {
		return link
	}
	// Already absolute URL — don't touch
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		return link
	}
	// Already has /co/ prefix — don't touch
	if strings.HasPrefix(link, "/co/") {
		return link
	}
	// /page/slug links are resolved by the frontend — don't touch
	if strings.HasPrefix(link, "/page/") {
		return prefix + "/" + strings.TrimPrefix(link, "/page/")
	}
	// Relative path starting with "/" — add prefix
	if strings.HasPrefix(link, "/") {
		// "/shop" -> "/co/{subdomain}/shop", "/" -> "/co/{subdomain}/"
		return prefix + link
	}
	// No leading slash — add prefix + slash
	return prefix + "/" + link
}

// RenderPublicHomepage 渲染公開首頁（/co/:subdomain/）
func RenderPublicHomepage(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	if strings.TrimSpace(c.Query("dining_table_id")) != "" || strings.TrimSpace(c.Query("dining_table_code")) != "" {
		return RenderPublicDiningMenu(c)
	}
	return renderPublicPage(c, subdomain, "")
}

// RenderPublicPageBySubdomainAndSlug 用於「自訂網域」場景：沒有 /co/:subdomain/ 路由參數時，直接用 subdomain+slug 渲染。
// slug 允許為空（表示首頁）。
func RenderPublicPageBySubdomainAndSlug(c *fiber.Ctx, subdomain string, slug string) error {
	return renderPublicPage(c, strings.TrimSpace(subdomain), strings.TrimSpace(slug))
}

// RenderPublicPage 渲染公開頁面（根據子域名和 slug）
func RenderPublicPage(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	if decoded, err := url.PathUnescape(subdomain); err == nil {
		subdomain = decoded
	}
	slug := c.Params("slug")
	return renderPublicPage(c, subdomain, slug)
}

// CreateDefaultEcommercePages 創建電商默認頁面
func CreateDefaultEcommercePages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	requestLang := strings.TrimSpace(c.Query("lang"))

	// 使用數據庫事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 調用創建默認頁面的函數（從 industry_template.go 中提取的邏輯）
	if err := createDefaultEcommercePagesHandler(tx, tenantID, requestLang); err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create default pages: " + err.Error(),
		})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to commit transaction",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Default ecommerce pages created successfully",
	})
}

// CreateDefaultGeneralPages 創建一般網站默認頁面
func CreateDefaultGeneralPages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	requestLang := strings.TrimSpace(c.Query("lang"))

	// 使用數據庫事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 調用創建默認頁面的函數
	if err := createDefaultGeneralPagesHandler(tx, tenantID, requestLang); err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create default pages: " + err.Error(),
		})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to commit transaction",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Default general pages created successfully",
	})
}

type defaultPageCopy struct {
	Home                string
	Shop                string
	About               string
	Contact             string
	News                string
	Login               string
	ProductDetail       string
	UserArea            string
	Cart                string
	Checkout            string
	Welcome             string
	StartExploring      string
	CompanyIntro        string
	Copyright           string
	HeroGeneralTitle    string
	HeroGeneralSubtitle string
	LearnMore           string
	AboutContent        string
	Submit              string
	ProductName         string
	ProductDescription  string
	// 餐飲專用
	Menu         string
	MenuSubtitle string
	ViewFullMenu string
	// 服務業專用
	Booking     string
	BookingDesc string
	BookNow     string
	// Contact 地址/電話顯示
	Address string
	Phone   string
}

func normalizeDefaultPageLang(raw string, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return fallback
	}
	// Accept variants
	if strings.HasPrefix(s, "en") {
		return "en"
	}
	// zh-Hans (Simplified)
	if strings.HasPrefix(s, "zh-cn") || strings.Contains(s, "hans") || strings.HasPrefix(s, "zh-sg") {
		return "zh-hans"
	}
	// zh-Hant (Traditional)
	if strings.HasPrefix(s, "zh-tw") || strings.Contains(s, "hant") || strings.HasPrefix(s, "zh-hk") || strings.HasPrefix(s, "zh-mo") || s == "zh" || strings.HasPrefix(s, "zh-") || strings.HasPrefix(s, "zh_") || strings.HasPrefix(s, "zh") {
		return "zh-hant"
	}
	return fallback
}

func resolveTenantDefaultPageLang(t models.Tenant) string {
	// default
	fallback := "zh-hant"
	if t.ExtraFields == nil {
		return fallback
	}
	if v, ok := t.ExtraFields["default_language"].(string); ok {
		return normalizeDefaultPageLang(v, fallback)
	}
	if v, ok := t.ExtraFields["defaultLanguage"].(string); ok {
		return normalizeDefaultPageLang(v, fallback)
	}
	if v, ok := t.ExtraFields["default_lang"].(string); ok {
		return normalizeDefaultPageLang(v, fallback)
	}
	return fallback
}

func defaultCopyForLang(lang string) defaultPageCopy {
	l := normalizeDefaultPageLang(lang, "zh-hant")
	year := time.Now().Year()

	switch l {
	case "en":
		return defaultPageCopy{
			Home:                "Home",
			Shop:                "Shop",
			About:               "About",
			Contact:             "Contact",
			News:                "News",
			Login:               "Login",
			ProductDetail:       "Product details",
			UserArea:            "Account",
			Cart:                "Cart",
			Checkout:            "Checkout",
			Welcome:             "Welcome",
			StartExploring:      "Start exploring",
			CompanyIntro:        "Company introduction text",
			Copyright:           fmt.Sprintf("© %d All rights reserved", year),
			HeroGeneralTitle:    "Welcome to our website",
			HeroGeneralSubtitle: "This is the hero banner on the homepage",
			LearnMore:           "Learn more",
			AboutContent:        "This is the About page content.",
			Submit:              "Submit",
			ProductName:         "Product name",
			ProductDescription:  "Product description",
			// 餐飲專用
			Menu:         "Menu",
			MenuSubtitle: "Browse our delicious offerings",
			ViewFullMenu: "View full menu",
			// 服務業專用
			Booking:     "Booking",
			BookingDesc: "Book an appointment with us",
			BookNow:     "Book now",
			// Contact
			Address: "Address",
			Phone:   "Phone",
		}
	case "zh-hans":
		return defaultPageCopy{
			Home:                "首页",
			Shop:                "商店",
			About:               "关于我们",
			Contact:             "联系我们",
			News:                "消息",
			Login:               "登录",
			ProductDetail:       "产品详情",
			UserArea:            "用户资料",
			Cart:                "购物车",
			Checkout:            "结账",
			Welcome:             "欢迎",
			StartExploring:      "开始探索",
			CompanyIntro:        "公司简介文字",
			Copyright:           fmt.Sprintf("© %d 版权所有", year),
			HeroGeneralTitle:    "欢迎来到我们的网站",
			HeroGeneralSubtitle: "这是首页的 Hero 横幅",
			LearnMore:           "了解更多",
			AboutContent:        "这里是关于我们的公司简介内容。",
			Submit:              "提交",
			ProductName:         "产品名称",
			ProductDescription:  "产品描述",
			// 餐飲專用
			Menu:         "菜单",
			MenuSubtitle: "浏览我们的美味佳肴",
			ViewFullMenu: "查看完整菜单",
			// 服務業專用
			Booking:     "预约",
			BookingDesc: "与我们预约服务",
			BookNow:     "立即预约",
			// Contact
			Address: "地址",
			Phone:   "电话",
		}
	default: // zh-hant
		return defaultPageCopy{
			Home:                "首頁",
			Shop:                "商店",
			About:               "關於我們",
			Contact:             "聯絡我們",
			News:                "消息",
			Login:               "登入",
			ProductDetail:       "產品詳情",
			UserArea:            "用戶資料",
			Cart:                "購物車",
			Checkout:            "結帳",
			Welcome:             "歡迎",
			StartExploring:      "開始探索",
			CompanyIntro:        "公司簡介文字",
			Copyright:           fmt.Sprintf("© %d 版權所有", year),
			HeroGeneralTitle:    "歡迎來到我們的網站",
			HeroGeneralSubtitle: "這是首頁的 Hero 橫幅",
			LearnMore:           "了解更多",
			AboutContent:        "這裡是關於我們的公司簡介內容。",
			Submit:              "送出",
			ProductName:         "產品名稱",
			ProductDescription:  "產品描述",
			// 餐飲專用
			Menu:         "菜單",
			MenuSubtitle: "瀏覽我們的美味佳餚",
			ViewFullMenu: "查看完整菜單",
			// 服務業專用
			Booking:     "預約",
			BookingDesc: "與我們預約服務",
			BookNow:     "立即預約",
			// Contact
			Address: "地址",
			Phone:   "電話",
		}
	}
}

func defaultEcommerceNavMenuItems(subdomain string, copy defaultPageCopy) []interface{} {
	return []interface{}{
		map[string]interface{}{"text": copy.Home, "text_i18n": "publicSite.pages.home.title", "link": "/"},
		map[string]interface{}{"text": copy.Shop, "text_i18n": "publicSite.pages.shop.title", "link": "/shop"},
		map[string]interface{}{"text": copy.Contact, "text_i18n": "publicSite.pages.contact.title", "link": "/contact"},
		map[string]interface{}{"text": copy.News, "text_i18n": "publicSite.pages.news.title", "link": "/news"},
	}
}

func defaultGeneralNavMenuItems(subdomain string, copy defaultPageCopy) []interface{} {
	return []interface{}{
		map[string]interface{}{"text": copy.Home, "text_i18n": "publicSite.pages.home.title", "link": "/"},
		map[string]interface{}{"text": copy.About, "text_i18n": "publicSite.pages.about.title", "link": "/about"},
		map[string]interface{}{"text": copy.Contact, "text_i18n": "publicSite.pages.contact.title", "link": "/contact"},
		map[string]interface{}{"text": copy.News, "text_i18n": "publicSite.pages.news.title", "link": "/news"},
	}
}

// getTenantLogoForPages 獲取租戶的 logo 信息，用於建立預設頁面
// 如果 Enterprise 有上傳 logo 圖片，則回傳 (logoURL, "")，頁面使用圖片 logo
// 否則回傳 ("", tenantName)，頁面使用文字 logo
func getTenantLogoForPages(tx *gorm.DB, tenantID uuid.UUID, tenantName string) (logoURL string, logoText string) {
	var enterprise models.Enterprise
	if err := tx.Where("tenant_id = ?", tenantID).First(&enterprise).Error; err == nil {
		if enterprise.LogoURL != nil && strings.TrimSpace(*enterprise.LogoURL) != "" {
			return strings.TrimSpace(*enterprise.LogoURL), ""
		}
	}
	return "", tenantName
}

// createOrGetNavFooterBlocks creates (or retrieves existing) shared nav and footer blocks for a tenant.
// These blocks are linked to page components so that editing the block updates all pages.
func createOrGetNavFooterBlocks(tx *gorm.DB, tenantID uuid.UUID, navType string, navData, footerData models.JSONB) (*models.Block, *models.Block, error) {
	now := time.Now()

	// Try to find existing nav block
	var navBlock models.Block
	navBlockName := "__global_" + navType // e.g. "__global_nav" or "__global_header"
	if err := tx.Where("tenant_id = ? AND name = ?", tenantID, navBlockName).First(&navBlock).Error; err != nil {
		// Create new nav block
		navBlock = models.Block{
			ID:            uuid.New(),
			TenantID:      tenantID,
			Name:          navBlockName,
			ComponentType: navType,
			ComponentData: navData,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&navBlock).Error; err != nil {
			return nil, nil, fmt.Errorf("failed to create nav block: %w", err)
		}
	}

	// Try to find existing footer block
	var footerBlock models.Block
	footerBlockName := "__global_footer"
	if err := tx.Where("tenant_id = ? AND name = ?", tenantID, footerBlockName).First(&footerBlock).Error; err != nil {
		// Create new footer block
		footerBlock = models.Block{
			ID:            uuid.New(),
			TenantID:      tenantID,
			Name:          footerBlockName,
			ComponentType: "footer",
			ComponentData: footerData,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := tx.Create(&footerBlock).Error; err != nil {
			return nil, nil, fmt.Errorf("failed to create footer block: %w", err)
		}
	}

	return &navBlock, &footerBlock, nil
}

// createDefaultEcommercePagesHandler 為電商創建默認頁面（可重用函數）
func createDefaultEcommercePagesHandler(tx *gorm.DB, tenantID uuid.UUID, requestLang string) error {
	var tenant models.Tenant
	if err := tx.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return fmt.Errorf("failed to get tenant: %w", err)
	}

	lang := normalizeDefaultPageLang(requestLang, resolveTenantDefaultPageLang(tenant))
	copy := defaultCopyForLang(lang)

	// 獲取租戶 logo：如果有上傳 logo 圖片則用圖片，否則用文字
	logoURL, logoText := getTenantLogoForPages(tx, tenantID, tenant.Name)

	// Create shared nav/footer blocks so editing one updates all pages
	navData := models.JSONB{
		"logo": logoURL, "logo_text": logoText, "show_login_icon": true, "show_cart_icon": true,
		"fixed": false, "menu_position": "right", "padding": "0.75rem 2rem",
		"menu_items": defaultEcommerceNavMenuItems(tenant.Subdomain, copy),
	}
	footerData := models.JSONB{
		"logo": logoURL, "column1_content": copy.CompanyIntro, "column1_content_i18n": "publicSite.footer.companyIntro",
		"column2_menu_items": []interface{}{}, "column3_menu_items": []interface{}{},
		"column4_menu_items": []interface{}{}, "copyright": copy.Copyright, "copyright_i18n": "publicSite.footer.copyright",
		"padding": "2rem 0",
	}
	navBlock, footerBlock, err := createOrGetNavFooterBlocks(tx, tenantID, "nav", navData, footerData)
	if err != nil {
		return err
	}

	// Helper: build nav/footer component configs that reference the shared blocks
	navComp := func(sortOrder int) ComponentConfig {
		return ComponentConfig{componentType: "nav", componentData: navData, sortOrder: sortOrder, blockID: &navBlock.ID}
	}
	footerComp := func(sortOrder int) ComponentConfig {
		return ComponentConfig{componentType: "footer", componentData: footerData, sortOrder: sortOrder, blockID: &footerBlock.ID}
	}

	pages := []PageConfig{
		{
			name:   copy.Home,
			slug:   "home",
			isHome: true,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "hero", componentData: models.JSONB{
					"title": copy.Welcome, "title_i18n": "publicSite.hero.welcome",
					"subtitle":    tenant.Name,
					"button_text": copy.StartExploring, "button_text_i18n": "publicSite.hero.startExploring", "button_link": "#",
					"background_image": "",
					"min_height":       "300px", "padding": "3rem", "text_align": "center",
				}, sortOrder: 1},
				{componentType: "product-list", componentData: models.JSONB{
					"full_list": false, "limit": 12, "columns": 4,
					"show_product_type_filter": false, "show_brand_filter": false,
					"product_detail_page": "",
					"padding":             "2rem 0", "gap": "1rem",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   copy.Login,
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
			name:   copy.Shop,
			slug:   "shop",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.Shop, "text_i18n": "publicSite.pages.shop.title", "level": "h3",
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
			name:   copy.ProductDetail,
			slug:   "product-detail",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "product-detail", componentData: models.JSONB{
					"product_name": copy.ProductName, "product_name_i18n": "publicSite.product.placeholder.name",
					"product_price":       "$0.00",
					"product_description": copy.ProductDescription, "product_description_i18n": "publicSite.product.placeholder.description",
					"padding": "2rem 0",
				}, sortOrder: 1},
				footerComp(2),
			},
		},
		{
			name:   copy.Cart,
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
			name:   copy.Checkout,
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
			name:   copy.UserArea,
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
		{
			name:   copy.News,
			slug:   "news",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.News, "text_i18n": "publicSite.pages.news.title", "level": "h3",
				}, sortOrder: 1},
				{componentType: "blog-list", componentData: models.JSONB{
					"limit": 10, "columns": 2, "show_image": true, "show_excerpt": true, "category": "",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   copy.Contact,
			slug:   "contact",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.Contact, "text_i18n": "publicSite.pages.contact.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "contact-form", componentData: models.JSONB{
					"show_name": true, "show_email": true, "show_phone": true,
					"show_message": true, "submit_button_text": copy.Submit, "submit_button_text_i18n": "publicSite.contact.submit",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
	}

	return createPagesFromConfig(tx, tenantID, pages)
}

// createDefaultGeneralPagesHandler 為一般網站創建默認頁面
func createDefaultGeneralPagesHandler(tx *gorm.DB, tenantID uuid.UUID, requestLang string) error {
	var tenant models.Tenant
	if err := tx.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return fmt.Errorf("failed to get tenant: %w", err)
	}

	lang := normalizeDefaultPageLang(requestLang, resolveTenantDefaultPageLang(tenant))
	copy := defaultCopyForLang(lang)

	// 獲取租戶 logo：如果有上傳 logo 圖片則用圖片，否則用文字
	logoURL, logoText := getTenantLogoForPages(tx, tenantID, tenant.Name)

	// Create shared nav/footer blocks
	navData := models.JSONB{
		"logo": logoURL, "logo_text": logoText, "show_login_icon": true, "show_cart_icon": false,
		"fixed": false, "menu_position": "right", "padding": "0.75rem 2rem",
		"menu_items": defaultGeneralNavMenuItems(tenant.Subdomain, copy),
	}
	footerData := models.JSONB{
		"logo": logoURL, "column1_content": copy.CompanyIntro, "column1_content_i18n": "publicSite.footer.companyIntro",
		"column2_menu_items": []interface{}{}, "column3_menu_items": []interface{}{},
		"column4_menu_items": []interface{}{}, "copyright": copy.Copyright, "copyright_i18n": "publicSite.footer.copyright",
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
			name:   copy.Home,
			slug:   "home",
			isHome: true,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "hero", componentData: models.JSONB{
					"title": copy.HeroGeneralTitle, "title_i18n": "publicSite.hero.generalTitle",
					"subtitle": copy.HeroGeneralSubtitle, "subtitle_i18n": "publicSite.hero.generalSubtitle",
					"button_text": copy.LearnMore, "button_text_i18n": "publicSite.hero.learnMore", "button_link": "/page/about",
					"background_image": "",
					"min_height":       "300px", "padding": "3rem", "text_align": "center",
				}, sortOrder: 1},
				footerComp(2),
			},
		},
		{
			name:   copy.About,
			slug:   "about",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.About, "text_i18n": "publicSite.pages.about.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "text", componentData: models.JSONB{
					"content": copy.AboutContent, "text_i18n": "publicSite.about.content",
					"padding": "2rem 0",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   copy.Contact,
			slug:   "contact",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.Contact, "text_i18n": "publicSite.pages.contact.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "contact-form", componentData: models.JSONB{
					"show_name": true, "show_email": true, "show_phone": true,
					"show_message": true, "submit_button_text": copy.Submit, "submit_button_text_i18n": "publicSite.contact.submit",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
	}

	return createPagesFromConfig(tx, tenantID, pages)
}

// PageConfig 頁面配置結構
type PageConfig struct {
	name       string
	slug       string
	isHome     bool
	status     string
	components []ComponentConfig
}

// ComponentConfig 組件配置結構
type ComponentConfig struct {
	componentType string
	componentData models.JSONB
	sortOrder     int
	blockID       *uuid.UUID // Optional reference to a linked block
}

// createPagesFromConfig 從配置創建頁面（通用函數）
func createPagesFromConfig(tx *gorm.DB, tenantID uuid.UUID, pages []PageConfig) error {
	for _, pageData := range pages {
		var existingPage models.Page
		var page models.Page

		if err := tx.Where("tenant_id = ? AND slug = ?", tenantID, pageData.slug).First(&existingPage).Error; err == nil {
			// 頁面已存在，檢查是否有組件
			page = existingPage

			var componentCount int64
			tx.Model(&models.PageComponent{}).Where("page_id = ?", page.ID).Count(&componentCount)

			if componentCount > 0 {
				continue
			}
		} else {
			// 創建新頁面
			page = models.Page{
				ID:         uuid.New(),
				TenantID:   tenantID,
				Name:       pageData.name,
				Slug:       pageData.slug,
				Status:     pageData.status,
				IsHomepage: pageData.isHome,
			}

			if pageData.isHome {
				// 如果設置為首頁，取消其他首頁標記
				tx.Model(&models.Page{}).
					Where("tenant_id = ? AND is_homepage = ?", tenantID, true).
					Update("is_homepage", false)
			}

			if err := tx.Create(&page).Error; err != nil {
				return fmt.Errorf("failed to create page '%s': %w", pageData.slug, err)
			}
		}

		// 創建頁面組件
		for _, compConfig := range pageData.components {
			component := models.PageComponent{
				ID:            uuid.New(),
				TenantID:      tenantID,
				PageID:        page.ID,
				BlockID:       compConfig.blockID,
				ComponentType: compConfig.componentType,
				ComponentData: compConfig.componentData,
				SortOrder:     compConfig.sortOrder,
				IsActive:      true,
			}

			if err := tx.Create(&component).Error; err != nil {
				return fmt.Errorf("failed to create component '%s' for page '%s': %w", compConfig.componentType, pageData.slug, err)
			}
		}
	}
	return nil
}

// getTenantContactInfo 獲取租戶的地址和電話信息
func getTenantContactInfo(tenant models.Tenant) (address, phone, phoneCountryCode string) {
	if tenant.ExtraFields == nil {
		return "", "", ""
	}
	if addr, ok := tenant.ExtraFields["address"].(string); ok {
		address = strings.TrimSpace(addr)
	}
	if p, ok := tenant.ExtraFields["phone"].(string); ok {
		phone = strings.TrimSpace(p)
	}
	if pcc, ok := tenant.ExtraFields["phone_country_code"].(string); ok {
		phoneCountryCode = strings.TrimSpace(pcc)
	}
	return
}

// buildContactFormWithInfo 構建帶有地址/電話信息的 contact-form 組件配置
func buildContactFormWithInfo(tenant models.Tenant, copy defaultPageCopy) models.JSONB {
	address, phone, phoneCountryCode := getTenantContactInfo(tenant)

	formData := models.JSONB{
		"show_name": true, "show_email": true, "show_phone": true,
		"show_message": true, "submit_button_text": copy.Submit, "submit_button_text_i18n": "publicSite.contact.submit",
	}

	// 如果有地址或電話，添加到組件配置中
	if address != "" {
		formData["company_address"] = address
		formData["address_label"] = copy.Address
		formData["address_label_i18n"] = "publicSite.contact.address"
	}
	if phone != "" {
		fullPhone := phone
		if phoneCountryCode != "" {
			fullPhone = "+" + phoneCountryCode + " " + phone
		}
		formData["company_phone"] = fullPhone
		formData["phone_label"] = copy.Phone
		formData["phone_label_i18n"] = "publicSite.contact.phone"
	}

	return formData
}

// defaultDiningNavMenuItems 餐飲業導航菜單
func defaultDiningNavMenuItems(subdomain string, copy defaultPageCopy) []interface{} {
	return []interface{}{
		map[string]interface{}{"text": copy.Home, "text_i18n": "publicSite.pages.home.title", "link": "/"},
		map[string]interface{}{"text": copy.Menu, "text_i18n": "publicSite.pages.menu.title", "link": "/menu"},
		map[string]interface{}{"text": copy.Contact, "text_i18n": "publicSite.pages.contact.title", "link": "/contact"},
		map[string]interface{}{"text": copy.News, "text_i18n": "publicSite.pages.news.title", "link": "/news"},
	}
}

// defaultServiceNavMenuItems 服務業導航菜單
func defaultServiceNavMenuItems(subdomain string, copy defaultPageCopy) []interface{} {
	return []interface{}{
		map[string]interface{}{"text": copy.Home, "text_i18n": "publicSite.pages.home.title", "link": "/"},
		map[string]interface{}{"text": copy.Booking, "text_i18n": "publicSite.pages.booking.title", "link": "/booking"},
		map[string]interface{}{"text": copy.About, "text_i18n": "publicSite.pages.about.title", "link": "/about"},
		map[string]interface{}{"text": copy.Contact, "text_i18n": "publicSite.pages.contact.title", "link": "/contact"},
	}
}

// CreateDefaultDiningPages 創建餐飲網站默認頁面（包含菜單頁面）
func CreateDefaultDiningPages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	requestLang := strings.TrimSpace(c.Query("lang"))

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := createDefaultDiningPagesHandler(tx, tenantID, requestLang); err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create default pages: " + err.Error(),
		})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to commit transaction",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Default dining pages created successfully",
	})
}

// createDefaultDiningPagesHandler 為餐飲業創建默認頁面（包含菜單頁面）
func createDefaultDiningPagesHandler(tx *gorm.DB, tenantID uuid.UUID, requestLang string) error {
	var tenant models.Tenant
	if err := tx.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return fmt.Errorf("failed to get tenant: %w", err)
	}

	lang := normalizeDefaultPageLang(requestLang, resolveTenantDefaultPageLang(tenant))
	copy := defaultCopyForLang(lang)

	// 獲取租戶 logo：如果有上傳 logo 圖片則用圖片，否則用文字
	logoURL, logoText := getTenantLogoForPages(tx, tenantID, tenant.Name)

	// Create shared nav/footer blocks
	navData := models.JSONB{
		"logo": logoURL, "logo_text": logoText, "show_login_icon": true, "show_cart_icon": true,
		"fixed": false, "menu_position": "right", "padding": "0.75rem 2rem",
		"menu_items": defaultDiningNavMenuItems(tenant.Subdomain, copy),
	}
	footerData := models.JSONB{
		"logo": logoURL, "column1_content": copy.CompanyIntro, "column1_content_i18n": "publicSite.footer.companyIntro",
		"column2_menu_items": []interface{}{}, "column3_menu_items": []interface{}{},
		"column4_menu_items": []interface{}{}, "copyright": copy.Copyright, "copyright_i18n": "publicSite.footer.copyright",
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
			name:   copy.Home,
			slug:   "home",
			isHome: true,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "hero", componentData: models.JSONB{
					"title": copy.Welcome, "title_i18n": "publicSite.hero.welcome",
					"subtitle":    tenant.Name,
					"button_text": copy.ViewFullMenu, "button_text_i18n": "publicSite.hero.viewFullMenu", "button_link": "/menu",
					"background_image": "",
					"min_height":       "300px", "padding": "3rem", "text_align": "center",
				}, sortOrder: 1},
				{componentType: "product-list", componentData: models.JSONB{
					"full_list": false, "limit": 8, "columns": 4,
					"show_product_type_filter": false, "show_brand_filter": false,
					"padding": "2rem 0", "gap": "1rem",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   copy.Menu,
			slug:   "menu",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.Menu, "text_i18n": "publicSite.pages.menu.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "text", componentData: models.JSONB{
					"content": copy.MenuSubtitle, "text_i18n": "publicSite.pages.menu.subtitle",
					"text_align": "center", "padding": "0 0 1rem 0",
				}, sortOrder: 2},
				{componentType: "product-list", componentData: models.JSONB{
					"full_list": true, "limit": 50, "columns": 3,
					"show_product_type_filter": true, "show_brand_filter": false,
					"padding": "2rem 0", "gap": "1rem",
				}, sortOrder: 3},
				footerComp(4),
			},
		},
		{
			name:   copy.Login,
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
			name:   copy.Cart,
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
			name:   copy.Checkout,
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
			name:   copy.News,
			slug:   "news",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.News, "text_i18n": "publicSite.pages.news.title", "level": "h3",
				}, sortOrder: 1},
				{componentType: "blog-list", componentData: models.JSONB{
					"limit": 10, "columns": 2, "show_image": true, "show_excerpt": true, "category": "",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   copy.Contact,
			slug:   "contact",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.Contact, "text_i18n": "publicSite.pages.contact.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "contact-form", componentData: buildContactFormWithInfo(tenant, copy), sortOrder: 2},
				footerComp(3),
			},
		},
	}

	return createPagesFromConfig(tx, tenantID, pages)
}

// CreateDefaultServicePages 創建服務業網站默認頁面（包含預約頁面）
func CreateDefaultServicePages(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	requestLang := strings.TrimSpace(c.Query("lang"))

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := createDefaultServicePagesHandler(tx, tenantID, requestLang); err != nil {
		tx.Rollback()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to create default pages: " + err.Error(),
		})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to commit transaction",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Default service pages created successfully",
	})
}

// createDefaultServicePagesHandler 為服務業創建默認頁面（包含預約頁面）
func createDefaultServicePagesHandler(tx *gorm.DB, tenantID uuid.UUID, requestLang string) error {
	var tenant models.Tenant
	if err := tx.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return fmt.Errorf("failed to get tenant: %w", err)
	}

	lang := normalizeDefaultPageLang(requestLang, resolveTenantDefaultPageLang(tenant))
	copy := defaultCopyForLang(lang)

	// 獲取租戶 logo：如果有上傳 logo 圖片則用圖片，否則用文字
	logoURL, logoText := getTenantLogoForPages(tx, tenantID, tenant.Name)

	// Create shared nav/footer blocks
	navData := models.JSONB{
		"logo": logoURL, "logo_text": logoText, "show_login_icon": true, "show_cart_icon": false,
		"fixed": false, "menu_position": "right", "padding": "0.75rem 2rem",
		"menu_items": defaultServiceNavMenuItems(tenant.Subdomain, copy),
	}
	footerData := models.JSONB{
		"logo": logoURL, "column1_content": copy.CompanyIntro, "column1_content_i18n": "publicSite.footer.companyIntro",
		"column2_menu_items": []interface{}{}, "column3_menu_items": []interface{}{},
		"column4_menu_items": []interface{}{}, "copyright": copy.Copyright, "copyright_i18n": "publicSite.footer.copyright",
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
			name:   copy.Home,
			slug:   "home",
			isHome: true,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "hero", componentData: models.JSONB{
					"title": copy.Welcome, "title_i18n": "publicSite.hero.welcome",
					"subtitle":    tenant.Name,
					"button_text": copy.BookNow, "button_text_i18n": "publicSite.hero.bookNow", "button_link": "/booking",
					"background_image": "",
					"min_height":       "300px", "padding": "3rem", "text_align": "center",
				}, sortOrder: 1},
				{componentType: "service-list", componentData: models.JSONB{
					"full_list": false, "limit": 6, "columns": 3,
					"show_price": true, "show_duration": true,
					"padding": "2rem 0", "gap": "1rem",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   copy.Booking,
			slug:   "booking",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.Booking, "text_i18n": "publicSite.pages.booking.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "text", componentData: models.JSONB{
					"content": copy.BookingDesc, "text_i18n": "publicSite.pages.booking.description",
					"text_align": "center", "padding": "0 0 1rem 0",
				}, sortOrder: 2},
				{componentType: "booking-form", componentData: models.JSONB{
					"show_service_select": true, "show_staff_select": true,
					"show_date_picker": true, "show_time_picker": true,
					"show_customer_info": true, "submit_button_text": copy.BookNow, "submit_button_text_i18n": "publicSite.booking.bookNow",
				}, sortOrder: 3},
				footerComp(4),
			},
		},
		{
			name:   copy.Login,
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
			name:   copy.About,
			slug:   "about",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.About, "text_i18n": "publicSite.pages.about.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "text", componentData: models.JSONB{
					"content": copy.AboutContent, "text_i18n": "publicSite.about.content",
					"padding": "2rem 0",
				}, sortOrder: 2},
				footerComp(3),
			},
		},
		{
			name:   copy.Contact,
			slug:   "contact",
			isHome: false,
			status: "published",
			components: []ComponentConfig{
				navComp(0),
				{componentType: "heading", componentData: models.JSONB{
					"text": copy.Contact, "text_i18n": "publicSite.pages.contact.title", "level": "h3",
					"text_align": "center", "margin": "2rem 0",
				}, sortOrder: 1},
				{componentType: "contact-form", componentData: buildContactFormWithInfo(tenant, copy), sortOrder: 2},
				footerComp(3),
			},
		},
	}

	return createPagesFromConfig(tx, tenantID, pages)
}
