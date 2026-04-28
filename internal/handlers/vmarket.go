package handlers

import (
	"fmt"
	"sort"
	"strings"

	"nwork/internal/database"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// vworkBaseURL builds the base URL for a tenant's public site (/co/ pages).
// On production, /co/ routes are served by vworkai.com, so links from vmarket
// must point there with an absolute URL.  On localhost the relative path is
// sufficient because the same server handles everything.
func vworkBaseURL(customDomain, subdomain string) string {
	if customDomain != "" && looksLikeDomain(customDomain) {
		return fmt.Sprintf("https://%s", customDomain)
	}
	if subdomain == "" {
		return ""
	}
	// Production: absolute URL to vworkai.com so links work on vmarketai.com.
	return fmt.Sprintf("https://www.vworkai.com/co/%s", subdomain)
}

// looksLikeDomain returns true when s looks like a real domain name
// (contains at least one dot and no characters that are invalid in
// hostnames).  Subdomain-style identifiers such as "test-WzA=" or
// bare words without a TLD will return false, preventing them from
// being used in https:// URLs.
func looksLikeDomain(s string) bool {
	if s == "" || !strings.Contains(s, ".") {
		return false
	}
	// Reject values that contain characters not valid in hostnames.
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '.') {
			return false
		}
	}
	return true
}

type vMarketCompany struct {
	Name           string
	EnterpriseName string
	LogoURL        string
	Subdomain      string
	Domain         string
	Category       string
	Address        string
	WebsiteURL     string
	ProductURL     string
	ServiceURL     string
	CompanyURL     string
	MapURL         string
	ProductCount   int
	ServiceCount   int
}

type vMarketCategory struct {
	Name  string
	Count int
}

type vMarketFeaturedProduct struct {
	Name        string
	Description string
	ImageURL    string
	Price       float64
	Category    string
	CompanyName string
	CompanyLogo string
	ProductURL  string
	CompanyURL  string
	ProductID   uuid.UUID
}

type vMarketFeaturedService struct {
	Name        string
	Description string
	Price       float64
	Duration    int // minutes, 0 if not set
	CompanyName string
	CompanyLogo string
	ServiceURL  string
	CompanyURL  string
}

type vMarketPageData struct {
	Title            string
	SEO              SEOData
	SEOPageKey       string
	Active           string
	Companies        []vMarketCompany
	Categories       []vMarketCategory
	SelectedCategory string
	CategoryBasePath string
	GoogleMapsAPIKey string
	BasePath         string // "/vmarket" on vworkai.com, "" on vmarketai.com
	CanonicalPath    string // canonical path without domain, e.g. "/" or "/products"
	FeaturedProducts []vMarketFeaturedProduct
	FeaturedServices []vMarketFeaturedService
	// Full product/service lists for /products and /services pages
	Products []vMarketFeaturedProduct
	Services []vMarketFeaturedService
}

// IsVMarketDomain returns true when the request is targeting the VMarket
// production domain (vmarketai.com). In local dev the /vmarket prefix routes
// are used instead, so this deliberately returns false for localhost.
func IsVMarketDomain(c *fiber.Ctx) bool {
	host := strings.ToLower(c.Hostname())
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	switch host {
	case "www.vmarketai.com", "vmarketai.com":
		return true
	}
	return false
}

// VmarketBasePath returns "" (empty) on the vmarketai.com production domain
// so that links like /products work directly, and "/vmarket" everywhere else
// (including localhost) so links become /vmarket/products — avoiding route
// conflicts with vWork CMS paths like /products, /services.
func VmarketBasePath(c *fiber.Ctx) string {
	lang := DetectPathLang(c.Path())
	prefix := ""
	if lang == "en" {
		prefix = "/en"
	} else if lang == "zh-CN" {
		prefix = "/zh-cn"
	}

	if IsVMarketDomain(c) {
		return prefix
	}
	return prefix + "/vmarket"
}

func RenderVMarketHome(c *fiber.Ctx) error {
	bp := VmarketBasePath(c)
	data, err := buildVMarketPageData(c, "home", "VMarket", "home", "", bp, "/", "vMarket 商家市集首頁，快速搜尋商品、服務與企業資訊。")
	if err != nil {
		return c.Status(500).SendString("Failed to load VMarket")
	}
	return c.Render("pages/vmarket_home", data, "layouts/vmarket_layout")
}

func RenderVMarketProducts(c *fiber.Ctx) error {
	bp := VmarketBasePath(c)
	data, err := buildVMarketPageData(c, "products", "VMarket｜商品", "products", bp+"/products", bp, "/products", "瀏覽 vMarket 商品分類與企業供應資訊，快速找到合適產品。")
	if err != nil {
		return c.Status(500).SendString("Failed to load VMarket products")
	}
	return c.Render("pages/vmarket_products", data, "layouts/vmarket_layout")
}

func RenderVMarketServices(c *fiber.Ctx) error {
	bp := VmarketBasePath(c)
	data, err := buildVMarketPageData(c, "services", "VMarket｜服務", "services", bp+"/services", bp, "/services", "探索 vMarket 企業服務項目，包含顧問、技術與商業支援服務。")
	if err != nil {
		return c.Status(500).SendString("Failed to load VMarket services")
	}
	return c.Render("pages/vmarket_services", data, "layouts/vmarket_layout")
}

func RenderVMarketCompanies(c *fiber.Ctx) error {
	bp := VmarketBasePath(c)
	data, err := buildVMarketPageData(c, "companies", "VMarket｜公司", "companies", bp+"/companies", bp, "/companies", "查找 vMarket 入駐企業，了解公司資料、商品與服務能力。")
	if err != nil {
		return c.Status(500).SendString("Failed to load VMarket companies")
	}
	return c.Render("pages/vmarket_companies", data, "layouts/vmarket_layout")
}

func RenderVMarketMap(c *fiber.Ctx) error {
	bp := VmarketBasePath(c)
	data, err := buildVMarketPageData(c, "map", "VMarket｜地圖", "map", "", bp, "/map", "透過地圖模式探索附近的 vMarket 商家與服務據點。")
	if err != nil {
		return c.Status(500).SendString("Failed to load VMarket map")
	}
	return c.Render("pages/vmarket_map", data, "layouts/vmarket_layout")
}

func RenderVMarketJoin(c *fiber.Ctx) error {
	bp := VmarketBasePath(c)
	data, err := buildVMarketPageData(c, "join", "VMarket｜加入 VMarket", "join", "", bp, "/join", "立即加入 vMarket，拓展你的企業曝光與商機來源。")
	if err != nil {
		return c.Status(500).SendString("Failed to load VMarket join")
	}
	return c.Render("pages/vmarket_join", data, "layouts/vmarket_layout")
}

func buildVMarketPageData(c *fiber.Ctx, seoPageKey string, title string, active string, categoryBasePath string, basePath string, canonicalPath string, description string) (vMarketPageData, error) {
	cfg := mustAppConfig()
	selectedCategory := strings.TrimSpace(c.Query("category"))

	data := vMarketPageData{
		Title:            title,
		SEO:              NewVMarketSEO(c, seoPageKey, title, description, canonicalPath, DetectPathLang(c.Path())),
		SEOPageKey:       seoPageKey,
		Active:           active,
		SelectedCategory: selectedCategory,
		CategoryBasePath: categoryBasePath,
		GoogleMapsAPIKey: strings.TrimSpace(cfg.GoogleMapsAPIKey),
		BasePath:         basePath,
		CanonicalPath:    canonicalPath,
	}

	switch seoPageKey {
	case "products":
		// Products page: show actual products, categories from product categories
		allProducts := loadFeaturedProducts(0) // 0 = no limit
		catCounts := map[string]int{}
		for _, p := range allProducts {
			if p.Category != "" {
				catCounts[p.Category]++
			}
		}
		data.Categories = buildCategoryList(catCounts)
		if selectedCategory != "" && selectedCategory != "all" {
			filtered := make([]vMarketFeaturedProduct, 0)
			for _, p := range allProducts {
				if p.Category == selectedCategory {
					filtered = append(filtered, p)
				}
			}
			data.Products = filtered
		} else {
			data.Products = allProducts
		}

	case "services":
		// Services page: show actual services, categories from company names
		allServices := loadFeaturedServices(0) // 0 = no limit
		catCounts := map[string]int{}
		for _, s := range allServices {
			if s.CompanyName != "" {
				catCounts[s.CompanyName]++
			}
		}
		data.Categories = buildCategoryList(catCounts)
		if selectedCategory != "" && selectedCategory != "all" {
			filtered := make([]vMarketFeaturedService, 0)
			for _, s := range allServices {
				if s.CompanyName == selectedCategory {
					filtered = append(filtered, s)
				}
			}
			data.Services = filtered
		} else {
			data.Services = allServices
		}

	case "home":
		// Home page: featured items + company list
		data.FeaturedProducts = loadFeaturedProducts(8)
		data.FeaturedServices = loadFeaturedServices(8)
		companies, err := loadVMarketCompanies()
		if err != nil {
			return vMarketPageData{}, err
		}
		data.Companies = companies

	default:
		// Companies, map, join, etc.: load companies with industry categories
		companies, err := loadVMarketCompanies()
		if err != nil {
			return vMarketPageData{}, err
		}
		catCounts := map[string]int{}
		for _, company := range companies {
			if company.Category != "" {
				catCounts[company.Category]++
			}
		}
		data.Categories = buildCategoryList(catCounts)
		if selectedCategory != "" && selectedCategory != "all" {
			filtered := make([]vMarketCompany, 0, len(companies))
			for _, company := range companies {
				if company.Category == selectedCategory {
					filtered = append(filtered, company)
				}
			}
			data.Companies = filtered
		} else {
			data.Companies = companies
		}
	}

	return data, nil
}

func buildCategoryList(counts map[string]int) []vMarketCategory {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	list := make([]vMarketCategory, 0, len(names))
	for _, name := range names {
		list = append(list, vMarketCategory{Name: name, Count: counts[name]})
	}
	return list
}

func loadVMarketCompanies() ([]vMarketCompany, error) {
	var rows []struct {
		EnterpriseName   string
		EnterpriseLogo   *string
		EnterpriseDomain *string
		IndustryName     *string
		EnterpriseAddr   *string
		TenantSubdomain  string
		TenantID         string
		ProductCount     int
		ServiceCount     int
	}

	// Query from enterprises, count products/services with show_on_vmarket=true.
	// All companies that joined vmarket are shown, even if they have no products/services yet.
	err := database.DB.Table("enterprises").
		Select(`enterprises.name AS enterprise_name,
			enterprises.logo_url AS enterprise_logo,
			enterprises.domain AS enterprise_domain,
			industry_templates.name AS industry_name,
			enterprises.address AS enterprise_addr,
			tenants.subdomain AS tenant_subdomain,
			tenants.id AS tenant_id,
			COALESCE((SELECT COUNT(*) FROM products p WHERE p.tenant_id = tenants.id AND p.trashed_at IS NULL AND p.status = 'active' AND COALESCE(p.extra_fields->>'show_on_vmarket','false') = 'true'), 0) AS product_count,
			COALESCE((SELECT COUNT(*) FROM services s WHERE s.tenant_id = tenants.id AND s.trashed_at IS NULL AND s.status = 'active' AND COALESCE(s.extra_fields->>'show_on_vmarket','false') = 'true'), 0) AS service_count`).
		Joins("JOIN tenants ON tenants.id = enterprises.tenant_id").
		Joins("LEFT JOIN industry_templates ON industry_templates.id = tenants.industry_template_id").
		Where("enterprises.status = ?", "active").
		Where("tenants.status = ?", "active").
		Where("COALESCE(tenants.extra_fields->>'vmarket_joined','false') = 'true'").
		Group("enterprises.id, enterprises.name, enterprises.logo_url, enterprises.domain, enterprises.address, tenants.subdomain, tenants.id, industry_templates.name").
		Order("enterprises.name ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	companies := make([]vMarketCompany, 0, len(rows))
	for _, row := range rows {
		logo := ""
		if row.EnterpriseLogo != nil {
			logo = strings.TrimSpace(*row.EnterpriseLogo)
		}
		domain := ""
		if row.EnterpriseDomain != nil {
			domain = strings.TrimSpace(*row.EnterpriseDomain)
		}
		address := ""
		if row.EnterpriseAddr != nil {
			address = strings.TrimSpace(*row.EnterpriseAddr)
		}
		category := ""
		if row.IndustryName != nil {
			category = strings.TrimSpace(*row.IndustryName)
		}
		subdomain := strings.TrimSpace(row.TenantSubdomain)

		baseURL := vworkBaseURL(domain, subdomain)
		baseURL = strings.TrimRight(baseURL, "/")

		productURL := ""
		serviceURL := ""
		companyURL := ""
		mapURL := ""
		if baseURL != "" {
			if row.ProductCount > 0 {
				productURL = baseURL + "/products"
			}
			if row.ServiceCount > 0 {
				serviceURL = baseURL + "/services"
			}
			companyURL = baseURL + "/"
			mapURL = baseURL + "/map"
		}

		companies = append(companies, vMarketCompany{
			Name:           row.EnterpriseName,
			EnterpriseName: row.EnterpriseName,
			LogoURL:        logo,
			Subdomain:      subdomain,
			Domain:         domain,
			Category:       category,
			Address:        address,
			WebsiteURL:     baseURL,
			ProductURL:     productURL,
			ServiceURL:     serviceURL,
			CompanyURL:     companyURL,
			MapURL:         mapURL,
			ProductCount:   row.ProductCount,
			ServiceCount:   row.ServiceCount,
		})
	}

	return companies, nil
}

func loadFeaturedProducts(limit int) []vMarketFeaturedProduct {
	var rows []struct {
		ProductID      uuid.UUID
		ProductName    string
		ProductDesc    *string
		ProductImage   *string
		ProductPrice   float64
		ProductCat     *string
		EnterpriseName string
		EnterpriseLogo *string
		Subdomain      string
		Domain         *string
	}

	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = -1 // GORM: -1 means no limit
	}

	err := database.DB.Table("products").
		Select(`products.id AS product_id,
			products.name AS product_name,
			LEFT(products.description, 120) AS product_desc,
			products.image_url AS product_image,
			products.price AS product_price,
			products.category AS product_cat,
			enterprises.name AS enterprise_name,
			enterprises.logo_url AS enterprise_logo,
			tenants.subdomain,
			enterprises.domain`).
		Joins("JOIN tenants ON tenants.id = products.tenant_id").
		Joins("JOIN enterprises ON enterprises.tenant_id = tenants.id").
		Where("products.trashed_at IS NULL").
		Where("products.status = ?", "active").
		Where("tenants.status = ?", "active").
		Where("enterprises.status = ?", "active").
		Where("COALESCE(tenants.extra_fields->>'vmarket_joined','false') = 'true'").
		Where("COALESCE(products.extra_fields->>'show_on_vmarket','false') = 'true'").
		Order("products.updated_at DESC").
		Limit(effectiveLimit).
		Scan(&rows).Error
	if err != nil {
		return nil
	}

	items := make([]vMarketFeaturedProduct, 0, len(rows))
	for _, r := range rows {
		d := ""
		if r.Domain != nil {
			d = strings.TrimSpace(*r.Domain)
		}
		baseURL := vworkBaseURL(d, r.Subdomain)
		baseURL = strings.TrimRight(baseURL, "/")

		img := ""
		if r.ProductImage != nil {
			img = strings.TrimSpace(*r.ProductImage)
		}
		desc := ""
		if r.ProductDesc != nil {
			desc = strings.TrimSpace(*r.ProductDesc)
		}
		cat := ""
		if r.ProductCat != nil {
			cat = strings.TrimSpace(*r.ProductCat)
		}
		logo := ""
		if r.EnterpriseLogo != nil {
			logo = strings.TrimSpace(*r.EnterpriseLogo)
		}

		// Link to the individual product detail page.
		productURL := ""
		if baseURL != "" {
			productURL = fmt.Sprintf("%s/product-detail?product_id=%s", baseURL, r.ProductID)
		}

		items = append(items, vMarketFeaturedProduct{
			Name:        r.ProductName,
			Description: desc,
			ImageURL:    img,
			Price:       r.ProductPrice,
			Category:    cat,
			CompanyName: r.EnterpriseName,
			CompanyLogo: logo,
			ProductURL:  productURL,
			CompanyURL:  baseURL + "/",
			ProductID:   r.ProductID,
		})
	}
	return items
}

func loadFeaturedServices(limit int) []vMarketFeaturedService {
	var rows []struct {
		ServiceName  string
		ServiceDesc  *string
		ServicePrice float64
		Duration     *int
		EntName      string
		EntLogo      *string
		Subdomain    string
		Domain       *string
	}

	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = -1 // GORM: -1 means no limit
	}

	err := database.DB.Table("services").
		Select(`services.name AS service_name,
			LEFT(services.description, 120) AS service_desc,
			services.price AS service_price,
			services.duration_minutes AS duration,
			enterprises.name AS ent_name,
			enterprises.logo_url AS ent_logo,
			tenants.subdomain,
			enterprises.domain`).
		Joins("JOIN tenants ON tenants.id = services.tenant_id").
		Joins("JOIN enterprises ON enterprises.tenant_id = tenants.id").
		Where("services.trashed_at IS NULL").
		Where("services.status = ?", "active").
		Where("tenants.status = ?", "active").
		Where("enterprises.status = ?", "active").
		Where("COALESCE(tenants.extra_fields->>'vmarket_joined','false') = 'true'").
		Where("COALESCE(services.extra_fields->>'show_on_vmarket','false') = 'true'").
		Order("services.updated_at DESC").
		Limit(effectiveLimit).
		Scan(&rows).Error
	if err != nil {
		return nil
	}

	items := make([]vMarketFeaturedService, 0, len(rows))
	for _, r := range rows {
		d := ""
		if r.Domain != nil {
			d = strings.TrimSpace(*r.Domain)
		}
		baseURL := vworkBaseURL(d, r.Subdomain)
		baseURL = strings.TrimRight(baseURL, "/")

		desc := ""
		if r.ServiceDesc != nil {
			desc = strings.TrimSpace(*r.ServiceDesc)
		}
		logo := ""
		if r.EntLogo != nil {
			logo = strings.TrimSpace(*r.EntLogo)
		}
		dur := 0
		if r.Duration != nil {
			dur = *r.Duration
		}

		items = append(items, vMarketFeaturedService{
			Name:        r.ServiceName,
			Description: desc,
			Price:       r.ServicePrice,
			Duration:    dur,
			CompanyName: r.EntName,
			CompanyLogo: logo,
			ServiceURL:  baseURL + "/services",
			CompanyURL:  baseURL + "/",
		})
	}
	return items
}

// VMarketSearchResult 搜尋結果項目
type VMarketSearchResult struct {
	Type    string `json:"type"` // "product", "service", "company"
	Name    string `json:"name"`
	Company string `json:"company"`
	URL     string `json:"url"`
}

// VMarketSearchResponse 搜尋 API 回應
type VMarketSearchResponse struct {
	Results []VMarketSearchResult `json:"results"`
	Total   int                   `json:"total"`
}

// VMarketSearch 搜尋商品、服務和公司
func VMarketSearch(c *fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		return c.JSON(VMarketSearchResponse{Results: []VMarketSearchResult{}, Total: 0})
	}

	results := []VMarketSearchResult{}
	searchPattern := "%" + strings.ToLower(query) + "%"

	// 搜尋公司
	companies, err := loadVMarketCompanies()
	if err == nil {
		for _, company := range companies {
			haystack := strings.ToLower(company.Name + " " + company.EnterpriseName + " " + company.Category)
			if strings.Contains(haystack, strings.ToLower(query)) {
				results = append(results, VMarketSearchResult{
					Type:    "company",
					Name:    company.Name,
					Company: company.EnterpriseName,
					URL:     company.CompanyURL,
				})
			}
		}
	}

	// 搜尋商品 (products with show_on_vmarket = true)
	var productRows []struct {
		ProductName    string
		EnterpriseName string
		Subdomain      string
		Domain         *string
		ProductID      uuid.UUID
	}
	err = database.DB.Table("products").
		Select("products.name AS product_name, enterprises.name AS enterprise_name, tenants.subdomain, enterprises.domain, products.id AS product_id").
		Joins("JOIN tenants ON tenants.id = products.tenant_id").
		Joins("JOIN enterprises ON enterprises.tenant_id = tenants.id").
		Where("products.trashed_at IS NULL").
		Where("products.status = ?", "active").
		Where("tenants.status = ?", "active").
		Where("COALESCE(tenants.extra_fields->>'vmarket_joined','false') = 'true'").
		Where("COALESCE(products.extra_fields->>'show_on_vmarket','false') = 'true'").
		Where("LOWER(products.name) LIKE ?", searchPattern).
		Limit(20).
		Scan(&productRows).Error
	if err == nil {
		for _, row := range productRows {
			d := ""
			if row.Domain != nil {
				d = strings.TrimSpace(*row.Domain)
			}
			baseURL := vworkBaseURL(d, row.Subdomain)
			productURL := ""
			if baseURL != "" {
				productURL = fmt.Sprintf("%s/product-detail?product_id=%s", strings.TrimRight(baseURL, "/"), row.ProductID)
			}
			results = append(results, VMarketSearchResult{
				Type:    "product",
				Name:    row.ProductName,
				Company: row.EnterpriseName,
				URL:     productURL,
			})
		}
	}

	// 搜尋服務 (services with show_on_vmarket = true)
	var serviceRows []struct {
		ServiceName    string
		EnterpriseName string
		Subdomain      string
		Domain         *string
		ServiceID      uuid.UUID
	}
	err = database.DB.Table("services").
		Select("services.name AS service_name, enterprises.name AS enterprise_name, tenants.subdomain, enterprises.domain, services.id AS service_id").
		Joins("JOIN tenants ON tenants.id = services.tenant_id").
		Joins("JOIN enterprises ON enterprises.tenant_id = tenants.id").
		Where("services.trashed_at IS NULL").
		Where("services.status = ?", "active").
		Where("tenants.status = ?", "active").
		Where("COALESCE(tenants.extra_fields->>'vmarket_joined','false') = 'true'").
		Where("COALESCE(services.extra_fields->>'show_on_vmarket','false') = 'true'").
		Where("LOWER(services.name) LIKE ?", searchPattern).
		Limit(20).
		Scan(&serviceRows).Error
	if err == nil {
		for _, row := range serviceRows {
			d := ""
			if row.Domain != nil {
				d = strings.TrimSpace(*row.Domain)
			}
			baseURL := vworkBaseURL(d, row.Subdomain)
			serviceURL := ""
			if baseURL != "" {
				serviceURL = strings.TrimRight(baseURL, "/") + "/services"
			}
			results = append(results, VMarketSearchResult{
				Type:    "service",
				Name:    row.ServiceName,
				Company: row.EnterpriseName,
				URL:     serviceURL,
			})
		}
	}

	// 限制總結果數量
	if len(results) > 20 {
		results = results[:20]
	}

	return c.JSON(VMarketSearchResponse{
		Results: results,
		Total:   len(results),
	})
}
