package handlers

import (
	"encoding/xml"
	"fmt"
	"html/template"
	"nwork/internal/database"
	"nwork/internal/models"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// sitemapCache holds a cached copy of the generated sitemap XML.
// Cache is invalidated when:
//   - TTL (1 hour) expires, OR
//   - Cronjob writes a newer timestamp to system_settings key "sitemap_invalidated_at"
var (
	sitemapCacheMu   sync.RWMutex
	sitemapCacheData []byte    // cached XML bytes (including xml header)
	sitemapCacheTime time.Time // when the cache was generated
	sitemapCacheTTL  = 1 * time.Hour
)

// InvalidateSitemapCache clears the cached sitemap so it regenerates on next request.
func InvalidateSitemapCache() {
	sitemapCacheMu.Lock()
	sitemapCacheData = nil
	sitemapCacheTime = time.Time{}
	sitemapCacheMu.Unlock()
}

// isSitemapCacheValid checks both TTL and the DB-based invalidation flag
// written by the cronjob worker (system_settings key "sitemap_invalidated_at").
func isSitemapCacheValid() bool {
	if sitemapCacheData == nil {
		return false
	}
	if time.Since(sitemapCacheTime) >= sitemapCacheTTL {
		return false
	}
	// Check if cronjob requested a refresh via DB flag
	ts := strings.TrimSpace(models.GetSystemSetting("sitemap_invalidated_at", ""))
	if ts == "" {
		return true
	}
	invalidatedAt, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return true
	}
	return sitemapCacheTime.After(invalidatedAt)
}

type SEOHrefLang struct {
	Lang string
	URL  string
}

type SEOData struct {
	Title                string
	Description          string
	Keywords             string
	CanonicalURL         string
	ImageURL             string
	Type                 string
	Locale               string
	LocaleAlternates     []string
	Hreflangs            []SEOHrefLang
	XDefaultURL          string
	JSONLD               template.JS
	ArticlePublishedTime string // ISO 8601, only for og:type=article
	ArticleAuthor        string // author name, only for og:type=article
}

func NormalizeSEOLang(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "en", "en-us", "en-gb":
		return "en"
	case "zh-cn", "zh_hans", "zh-hans", "zh-sg":
		return "zh-CN"
	default:
		return "zh"
	}
}

func DetectPathLang(path string) string {
	p := strings.ToLower(strings.TrimSpace(path))
	if p == "/en" || strings.HasPrefix(p, "/en/") {
		return "en"
	}
	if p == "/zh-cn" || strings.HasPrefix(p, "/zh-cn/") {
		return "zh-CN"
	}
	return "zh"
}

func stripLangPrefix(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return "/"
	}
	l := strings.ToLower(p)
	if l == "/en" || l == "/zh-cn" {
		return "/"
	}
	if strings.HasPrefix(l, "/en/") {
		return p[3:]
	}
	if strings.HasPrefix(l, "/zh-cn/") {
		return p[6:]
	}
	return p
}

func localizedPath(path string, lang string) string {
	base := stripLangPrefix(path)
	if base == "" {
		base = "/"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	lang = NormalizeSEOLang(lang)
	switch lang {
	case "en":
		if base == "/" {
			return "/en"
		}
		return "/en" + base
	case "zh-CN":
		if base == "/" {
			return "/zh-cn"
		}
		return "/zh-cn" + base
	default:
		return base
	}
}

func localeFromLang(lang string) string {
	switch NormalizeSEOLang(lang) {
	case "en":
		return "en_US"
	case "zh-CN":
		return "zh_CN"
	default:
		return "zh_TW"
	}
}

func alternateLocales(lang string) []string {
	all := []string{"zh_TW", "en_US", "zh_CN"}
	current := localeFromLang(lang)
	out := make([]string, 0, 2)
	for _, l := range all {
		if l != current {
			out = append(out, l)
		}
	}
	return out
}

func buildHreflangs(c *fiber.Ctx, basePath string) []SEOHrefLang {
	return []SEOHrefLang{
		{Lang: "zh-TW", URL: buildAbsoluteURL(c, localizedPath(basePath, "zh"))},
		{Lang: "en", URL: buildAbsoluteURL(c, localizedPath(basePath, "en"))},
		{Lang: "zh-CN", URL: buildAbsoluteURL(c, localizedPath(basePath, "zh-CN"))},
	}
}

func buildWebsiteJSONLD(name string, description string, url string) template.JS {
	json := fmt.Sprintf(`{"@context":"https://schema.org","@type":"WebSite","name":%q,"description":%q,"url":%q}`, name, description, url)
	return template.JS(json)
}

func buildSoftwareJSONLD(name string, description string, url string) template.JS {
	json := fmt.Sprintf(`{"@context":"https://schema.org","@type":"SoftwareApplication","name":%q,"description":%q,"applicationCategory":"BusinessApplication","operatingSystem":"Web","url":%q}`, name, description, url)
	return template.JS(json)
}

func applySEOOverride(pageKey string, lang string, title string, description string, keywords string, imagePath string) (string, string, string, string) {
	lang = NormalizeSEOLang(lang)
	prefix := fmt.Sprintf("seo.%s.%s", pageKey, lang)

	t := strings.TrimSpace(models.GetSystemSetting(prefix+".title", title))
	d := strings.TrimSpace(models.GetSystemSetting(prefix+".description", description))
	k := strings.TrimSpace(models.GetSystemSetting(prefix+".keywords", keywords))
	i := strings.TrimSpace(models.GetSystemSetting(prefix+".image", imagePath))

	if t == "" {
		t = title
	}
	if d == "" {
		d = description
	}
	if i == "" {
		i = imagePath
	}

	return t, d, k, i
}

func buildAbsoluteURL(c *fiber.Ctx, path string) string {
	host := strings.TrimSpace(c.Hostname())
	if host == "" {
		host = "www.vworkai.com"
	}
	scheme := c.Protocol()
	if scheme == "" {
		scheme = "https"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}

func isSitemapSafeTenantSubdomain(subdomain string) bool {
	s := strings.ToLower(strings.TrimSpace(subdomain))
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "test") || strings.HasPrefix(s, "admin") {
		return false
	}
	if strings.ContainsAny(s, "=+ ") {
		return false
	}
	isDigitsOnly := true
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			isDigitsOnly = false
			break
		}
	}
	if isDigitsOnly {
		return false
	}
	return true
}

func isSitemapSafeTenantSlug(slug string) bool {
	s := strings.ToLower(strings.Trim(strings.TrimSpace(slug), "/"))
	if s == "" {
		return true
	}
	blocked := map[string]bool{
		"login":          true,
		"cart":           true,
		"checkout":       true,
		"user":           true,
		"user-profile":   true,
		"product-detail": true,
	}
	return !blocked[s]
}

func NewLandingSEO(c *fiber.Ctx, product string, lang string) SEOData {
	type landingMeta struct {
		title       string
		description string
		keywords    string
		imagePath   string
	}

	meta := map[string]map[string]landingMeta{
		"vwork": {
			"zh":    {title: "vWork - AI 智能企業管理平台", description: "vWork 提供 POS、庫存、訂單、客戶管理的一站式企業管理方案，協助企業提升營運效率。", keywords: "vWork, ERP, POS, 庫存管理, 訂單管理, 企業管理系統", imagePath: "/static/vworkicon.png"},
			"en":    {title: "vWork - AI Business Management Platform", description: "vWork provides POS, inventory, order, and customer management in one platform to improve business efficiency.", keywords: "vWork, ERP, POS, inventory management, order management, business software", imagePath: "/static/vworkicon.png"},
			"zh-CN": {title: "vWork - AI 智能企业管理平台", description: "vWork 提供 POS、库存、订单、客户管理的一站式企业管理方案，帮助企业提升运营效率。", keywords: "vWork, ERP, POS, 库存管理, 订单管理, 企业管理系统", imagePath: "/static/vworkicon.png"},
		},
		"vsys": {
			"zh":    {title: "V-sys - 企業數位化解決方案", description: "V-sys 透過 AI 技術提供企業數位化解決方案，包含 vWork、vAI、vOffice 與 vMarket。", keywords: "V-sys, AI, 數位化, 企業方案, vWork, vAI, vOffice, vMarket", imagePath: "/static/vicon.png"},
			"en":    {title: "V-sys - Digital Solutions for Enterprises", description: "V-sys delivers AI-powered digital solutions including vWork, vAI, vOffice, and vMarket.", keywords: "V-sys, AI, digital transformation, enterprise solutions", imagePath: "/static/vicon.png"},
			"zh-CN": {title: "V-sys - 企业数字化解决方案", description: "V-sys 通过 AI 技术提供企业数字化解决方案，涵盖 vWork、vAI、vOffice 和 vMarket。", keywords: "V-sys, AI, 数字化, 企业方案, vWork, vAI, vOffice, vMarket", imagePath: "/static/vicon.png"},
		},
		"vai": {
			"zh":    {title: "vAI - 智能 AI 助手", description: "vAI 協助企業快速完成對話、文件、圖片與影片工作流程，提升日常營運效率。", keywords: "vAI, AI 助手, 智能客服, AI 文件, AI 圖片, AI 影片", imagePath: "/static/vaiicon.png"},
			"en":    {title: "vAI - Intelligent AI Assistant", description: "vAI helps teams handle chat, documents, images, and videos with faster AI workflows.", keywords: "vAI, AI assistant, AI chat, AI documents, AI image, AI video", imagePath: "/static/vaiicon.png"},
			"zh-CN": {title: "vAI - 智能 AI 助手", description: "vAI 帮助企业更快完成对话、文档、图片和视频工作流程，提升日常效率。", keywords: "vAI, AI 助手, 智能客服, AI 文档, AI 图片, AI 视频", imagePath: "/static/vaiicon.png"},
		},
		"voffice": {
			"zh":    {title: "vOffice - 智能辦公套件", description: "vOffice 提供文件、表格與簡報的辦公能力，並整合 AI 助手，提升團隊協作效率。", keywords: "vOffice, 辦公套件, 文件編輯, 表格, 簡報, AI 辦公", imagePath: "/static/vofficeicon.png"},
			"en":    {title: "vOffice - Smart Office Suite", description: "vOffice offers documents, spreadsheets, and presentations with built-in AI for better team productivity.", keywords: "vOffice, office suite, documents, spreadsheets, presentations, AI office", imagePath: "/static/vofficeicon.png"},
			"zh-CN": {title: "vOffice - 智能办公套件", description: "vOffice 提供文档、表格与演示文稿能力，并整合 AI 助手，提升团队协作效率。", keywords: "vOffice, 办公套件, 文档编辑, 表格, 演示, AI 办公", imagePath: "/static/vofficeicon.png"},
		},
	}

	lang = NormalizeSEOLang(lang)
	if lang == "" {
		lang = DetectPathLang(c.Path())
	}

	productMeta, ok := meta[product]
	if !ok {
		productMeta = meta["vwork"]
	}

	m, ok := productMeta[lang]
	if !ok {
		m = productMeta["zh"]
	}

	pageKey := "landing." + product
	title, description, keywords, imagePath := applySEOOverride(pageKey, lang, m.title, m.description, m.keywords, m.imagePath)

	canonicalPath := localizedPath(c.Path(), lang)
	canonical := buildAbsoluteURL(c, canonicalPath)
	image := buildAbsoluteURL(c, imagePath)
	hreflangs := buildHreflangs(c, stripLangPrefix(c.Path()))

	var jsonld template.JS
	if product == "vsys" {
		jsonld = buildWebsiteJSONLD("V-sys", description, canonical)
	} else {
		jsonld = buildSoftwareJSONLD(strings.ToUpper(product[:1])+product[1:], description, canonical)
	}

	return SEOData{
		Title:            title,
		Description:      description,
		Keywords:         keywords,
		CanonicalURL:     canonical,
		ImageURL:         image,
		Type:             "website",
		Locale:           localeFromLang(lang),
		LocaleAlternates: alternateLocales(lang),
		Hreflangs:        hreflangs,
		XDefaultURL:      buildAbsoluteURL(c, stripLangPrefix(c.Path())),
		JSONLD:           jsonld,
	}
}

func NewVMarketSEO(c *fiber.Ctx, pageKey string, title string, description string, canonicalPath string, lang string) SEOData {
	if strings.TrimSpace(description) == "" {
		description = "vMarket 匯集優質商家與商品服務，協助使用者快速搜尋、比較與找到合適的合作夥伴。"
	}
	lang = NormalizeSEOLang(lang)
	if lang == "" {
		lang = DetectPathLang(c.Path())
	}
	title, description, keywords, imagePath := applySEOOverride("vmarket."+pageKey, lang, title, description, "vMarket, 商家平台, 商品搜尋, 服務搜尋, 企業配對", "/static/vmarketicon.png")
	canonicalPath = localizedPath(canonicalPath, lang)
	canonical := buildAbsoluteURL(c, canonicalPath)
	hreflangs := buildHreflangs(c, stripLangPrefix(canonicalPath))
	return SEOData{
		Title:            title,
		Description:      description,
		Keywords:         keywords,
		CanonicalURL:     canonical,
		ImageURL:         buildAbsoluteURL(c, imagePath),
		Type:             "website",
		Locale:           localeFromLang(lang),
		LocaleAlternates: alternateLocales(lang),
		Hreflangs:        hreflangs,
		XDefaultURL:      buildAbsoluteURL(c, stripLangPrefix(canonicalPath)),
		JSONLD:           buildWebsiteJSONLD("vMarket", description, canonical),
	}
}

func RobotsTxt(c *fiber.Ctx) error {
	sitemapURL := buildAbsoluteURL(c, "/sitemap.xml")
	body := strings.Join([]string{
		"User-agent: *",
		"Allow: /",
		"Disallow: /api/",
		"Disallow: /dashboard",
		"Disallow: /orders",
		"Disallow: /products",
		"Disallow: /customers",
		"Disallow: /billing",
		"Disallow: /personal-data",
		"Disallow: /setup-tenant",
		"Disallow: /profile-guide",
		"",
		"Sitemap: " + sitemapURL,
	}, "\n")

	c.Type("txt", "utf-8")
	return c.SendString(body)
}

type sitemapURL struct {
	Loc        string              `xml:"loc"`
	LastMod    string              `xml:"lastmod,omitempty"`
	ChangeFreq string              `xml:"changefreq,omitempty"`
	Priority   string              `xml:"priority,omitempty"`
	Links      []sitemapAltLinkXML `xml:"xhtml:link,omitempty"`
}

type sitemapAltLinkXML struct {
	XMLName  xml.Name `xml:"xhtml:link"`
	Rel      string   `xml:"rel,attr"`
	HrefLang string   `xml:"hreflang,attr"`
	Href     string   `xml:"href,attr"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	XMLNSXH string       `xml:"xmlns:xhtml,attr,omitempty"`
	URLs    []sitemapURL `xml:"url"`
}

func SitemapXML(c *fiber.Ctx) error {
	// Check cache first (TTL + DB-based invalidation from cronjob)
	sitemapCacheMu.RLock()
	if isSitemapCacheValid() {
		data := sitemapCacheData
		sitemapCacheMu.RUnlock()
		c.Set("Content-Type", "application/xml; charset=utf-8")
		return c.Send(data)
	}
	sitemapCacheMu.RUnlock()

	base := buildAbsoluteURL(c, "")
	base = strings.TrimSuffix(base, "/")

	urls := make([]sitemapURL, 0, 128)
	add := func(path string, changefreq string, priority string) {
		urls = append(urls, sitemapURL{
			Loc:        base + path,
			ChangeFreq: changefreq,
			Priority:   priority,
		})
	}
	addLocalized := func(path string, changefreq string, priority string) {
		basePath := stripLangPrefix(path)
		links := []sitemapAltLinkXML{
			{Rel: "alternate", HrefLang: "zh-TW", Href: base + localizedPath(basePath, "zh")},
			{Rel: "alternate", HrefLang: "en", Href: base + localizedPath(basePath, "en")},
			{Rel: "alternate", HrefLang: "zh-CN", Href: base + localizedPath(basePath, "zh-CN")},
			{Rel: "alternate", HrefLang: "x-default", Href: base + stripLangPrefix(basePath)},
		}

		for _, p := range []string{
			localizedPath(basePath, "zh"),
			localizedPath(basePath, "en"),
			localizedPath(basePath, "zh-CN"),
		} {
			urls = append(urls, sitemapURL{
				Loc:        base + p,
				ChangeFreq: changefreq,
				Priority:   priority,
				Links:      links,
			})
		}
	}

	// Public static pages
	addLocalized("/", "daily", "1.0")
	add("/contact", "monthly", "0.7")
	add("/terms", "monthly", "0.3")
	add("/privacy", "monthly", "0.3")
	addLocalized("/help", "weekly", "0.6")
	addLocalized("/help/vwork", "weekly", "0.5")
	addLocalized("/help/vai", "weekly", "0.5")
	addLocalized("/help/vmarket", "weekly", "0.5")
	addLocalized("/help/voffice", "weekly", "0.5")
	addLocalized("/help/vwork/dns-setup", "monthly", "0.5")
	for _, category := range []string{"account", "product", "order", "customer", "service", "pos", "pos-self-service", "vbuilder", "getting-started", "tutorial-videos", "import-export", "inventory", "accounting", "hr", "dynamic-fields", "reports", "ai", "purchase", "supplier", "warehouse", "project", "store", "website", "promotion", "resource", "personal-tools"} {
		addLocalized("/help/vwork/"+category, "monthly", "0.4")
	}
	for _, category := range []string{"getting-started", "chat", "sketch", "video", "docs", "vcoins", "account", "faq"} {
		addLocalized("/help/vai/"+category, "monthly", "0.4")
	}
	for _, category := range []string{"getting-started", "browse", "search", "map", "join", "manage", "account", "faq"} {
		addLocalized("/help/vmarket/"+category, "monthly", "0.4")
	}
	addLocalized("/vmarket", "daily", "0.8")
	addLocalized("/vmarket/products", "daily", "0.8")
	addLocalized("/vmarket/services", "daily", "0.8")
	addLocalized("/vmarket/companies", "daily", "0.8")
	addLocalized("/vmarket/map", "daily", "0.7")
	addLocalized("/vmarket/join", "weekly", "0.6")

	// Industry solution pages (SEO-critical for AI ERP keywords)
	for _, industry := range []string{"catering", "retail", "ecommerce", "services", "wholesale", "sme"} {
		addLocalized("/industry/"+industry, "weekly", "0.8")
	}

	// Custom / Enterprise solution pages
	add("/custom/website", "monthly", "0.7")
	add("/custom/features", "monthly", "0.7")
	add("/enterprise-custom", "monthly", "0.7")

	// Sales partner page
	add("/sales-partner", "monthly", "0.5")

	// Platform events (listing + individual events from DB)
	addLocalized("/vwork-events", "daily", "0.7")

	// Platform events — individual event pages
	type platformEventRow struct {
		Slug      string
		UpdatedAt time.Time
	}
	var eventRows []platformEventRow
	_ = database.DB.
		Table("platform_events").
		Select("slug, updated_at").
		Where("status = ?", "published").
		Order("updated_at DESC").
		Scan(&eventRows).Error

	for _, row := range eventRows {
		slug := strings.Trim(strings.TrimSpace(row.Slug), "/")
		if slug == "" {
			continue
		}
		urls = append(urls, sitemapURL{
			Loc:        base + "/vwork-events/" + slug,
			LastMod:    row.UpdatedAt.UTC().Format("2006-01-02"),
			ChangeFreq: "weekly",
			Priority:   "0.6",
		})
	}

	// Tenant public pages
	type tenantPageRow struct {
		Subdomain  string
		Slug       string
		IsHomepage bool
		UpdatedAt  time.Time
	}

	var rows []tenantPageRow
	_ = database.DB.
		Table("pages p").
		Select("t.subdomain, p.slug, p.is_homepage, p.updated_at").
		Joins("JOIN tenants t ON t.id = p.tenant_id").
		Where("t.status = ?", "active").
		Where("t.website_enabled = ?", true).
		Where("p.trashed_at IS NULL").
		Where("p.status = ?", "published").
		Scan(&rows).Error

	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		sub := strings.TrimSpace(row.Subdomain)
		if !isSitemapSafeTenantSubdomain(sub) {
			continue
		}
		path := ""
		if row.IsHomepage {
			path = fmt.Sprintf("/co/%s/", sub)
		} else {
			slug := strings.Trim(strings.TrimSpace(row.Slug), "/")
			if slug == "" || !isSitemapSafeTenantSlug(slug) {
				continue
			}
			path = fmt.Sprintf("/co/%s/%s/", sub, slug)
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		urls = append(urls, sitemapURL{
			Loc:        base + path,
			LastMod:    row.UpdatedAt.UTC().Format("2006-01-02"),
			ChangeFreq: "weekly",
			Priority:   "0.6",
		})
	}

	// Tenant blog posts (server-rendered at /co/:subdomain/blog/:slug/)
	type tenantBlogRow struct {
		Subdomain string
		Slug      string
		UpdatedAt time.Time
	}

	var blogRows []tenantBlogRow
	_ = database.DB.
		Table("blogs b").
		Select("t.subdomain, b.slug, b.updated_at").
		Joins("JOIN tenants t ON t.id = b.tenant_id").
		Where("t.status = ?", "active").
		Where("t.website_enabled = ?", true).
		Where("b.trashed_at IS NULL").
		Where("b.status = ?", "published").
		Scan(&blogRows).Error

	for _, row := range blogRows {
		sub := strings.TrimSpace(row.Subdomain)
		if !isSitemapSafeTenantSubdomain(sub) {
			continue
		}
		slug := strings.Trim(strings.TrimSpace(row.Slug), "/")
		if slug == "" || !isSitemapSafeTenantSlug(slug) {
			continue
		}
		path := fmt.Sprintf("/co/%s/blog/%s/", sub, slug)
		if seen[path] {
			continue
		}
		seen[path] = true
		urls = append(urls, sitemapURL{
			Loc:        base + path,
			LastMod:    row.UpdatedAt.UTC().Format("2006-01-02"),
			ChangeFreq: "weekly",
			Priority:   "0.5",
		})
	}

	// Platform blog (vWork official website blog at /vwork-blog)
	// Uses ?lang= query param for multi-language, not path prefix.
	platformBlogHreflangMap := map[string]string{
		"zh":    "zh-TW",
		"zh-CN": "zh-CN",
		"en":    "en",
	}
	platformBlogLangs := []string{"zh", "zh-CN", "en"}

	// 1) Blog listing page — one entry per language, all cross-linked
	{
		listLinks := make([]sitemapAltLinkXML, 0, 4)
		for _, lang := range platformBlogLangs {
			listLinks = append(listLinks, sitemapAltLinkXML{
				Rel:      "alternate",
				HrefLang: platformBlogHreflangMap[lang],
				Href:     base + "/vwork-blog?lang=" + lang,
			})
		}
		listLinks = append(listLinks, sitemapAltLinkXML{
			Rel:      "alternate",
			HrefLang: "x-default",
			Href:     base + "/vwork-blog",
		})
		for _, lang := range platformBlogLangs {
			urls = append(urls, sitemapURL{
				Loc:        base + "/vwork-blog?lang=" + lang,
				ChangeFreq: "daily",
				Priority:   "0.7",
				Links:      listLinks,
			})
		}
	}

	// 2) Individual platform blog posts — group by slug, cross-link available languages
	type platformBlogRow struct {
		Slug      string
		Lang      string
		UpdatedAt time.Time
	}
	var pbRows []platformBlogRow
	_ = database.DB.
		Table("platform_blogs").
		Select("slug, lang, updated_at").
		Where("status = ?", "published").
		Order("slug, lang").
		Scan(&pbRows).Error

	// Group by slug
	slugLangs := make(map[string][]platformBlogRow)
	var slugOrder []string
	for _, row := range pbRows {
		if _, exists := slugLangs[row.Slug]; !exists {
			slugOrder = append(slugOrder, row.Slug)
		}
		slugLangs[row.Slug] = append(slugLangs[row.Slug], row)
	}

	for _, slug := range slugOrder {
		langRows := slugLangs[slug]

		// Build hreflang links for all available languages of this slug
		altLinks := make([]sitemapAltLinkXML, 0, len(langRows)+1)
		for _, lr := range langRows {
			hl, ok := platformBlogHreflangMap[lr.Lang]
			if !ok {
				continue
			}
			altLinks = append(altLinks, sitemapAltLinkXML{
				Rel:      "alternate",
				HrefLang: hl,
				Href:     base + "/vwork-blog/" + slug + "?lang=" + lr.Lang,
			})
		}
		// x-default points to the zh version if available, otherwise first available
		xDefaultLang := langRows[0].Lang
		for _, lr := range langRows {
			if lr.Lang == "zh" {
				xDefaultLang = "zh"
				break
			}
		}
		altLinks = append(altLinks, sitemapAltLinkXML{
			Rel:      "alternate",
			HrefLang: "x-default",
			Href:     base + "/vwork-blog/" + slug + "?lang=" + xDefaultLang,
		})

		// Add one sitemap entry per language version
		for _, lr := range langRows {
			urls = append(urls, sitemapURL{
				Loc:        base + "/vwork-blog/" + slug + "?lang=" + lr.Lang,
				LastMod:    lr.UpdatedAt.UTC().Format("2006-01-02"),
				ChangeFreq: "weekly",
				Priority:   "0.6",
				Links:      altLinks,
			})
		}
	}

	sort.Slice(urls, func(i, j int) bool {
		return urls[i].Loc < urls[j].Loc
	})

	data := sitemapURLSet{
		Xmlns:   "http://www.sitemaps.org/schemas/sitemap/0.9",
		XMLNSXH: "http://www.w3.org/1999/xhtml",
		URLs:    urls,
	}

	xmlBytes, err := xml.MarshalIndent(data, "", "  ")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("failed to build sitemap")
	}

	result := []byte(xml.Header + string(xmlBytes))

	// Store in cache
	sitemapCacheMu.Lock()
	sitemapCacheData = result
	sitemapCacheTime = time.Now()
	sitemapCacheMu.Unlock()

	c.Set("Content-Type", "application/xml; charset=utf-8")
	return c.Send(result)
}
