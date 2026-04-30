package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/handlers"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/themes"
	"nwork/internal/utils"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"html/template"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	// 切換到可執行文件所在目錄，確保相對路徑（如 ./web）正確解析
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		// 檢查是否在 bin 子目錄中（例如 vwork/bin/api.exe）
		if filepath.Base(exeDir) == "bin" {
			exeDir = filepath.Dir(exeDir)
		}
		if err := os.Chdir(exeDir); err != nil {
			log.Printf("⚠️  Warning: Failed to change to executable directory: %v", err)
		} else {
			log.Printf("📁 Working directory: %s", exeDir)
		}
	}

	// 加載 .env 文件
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  Warning: .env file not found, using environment variables")
	}

	// 加載配置
	cfg := config.Load()

	// 初始化上傳配置
	handlers.SetUploadConfig(&cfg.Upload)
	// 初始化 App 配置（handlers 會用到 Stripe 等設定）
	handlers.SetAppConfig(cfg)
	// 初始化 Email 配置（API 只負責 enqueue，worker 負責實際發送）
	email.SetConfig(cfg)
	// 初始化 WhatsApp 客戶端
	handlers.InitWhatsApp(cfg)

	// 連接數據庫
	if err := database.Connect(cfg); err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}

	// 測試數據庫連接
	if err := database.Ping(); err != nil {
		log.Fatalf("❌ Failed to ping database: %v", err)
	}
	log.Println("✅ Database ping successful")

	// Auto-migrate platform events tables
	if err := handlers.AutoMigrateEvents(database.DB); err != nil {
		log.Printf("⚠️ Failed to auto-migrate events tables: %v", err)
	}

	// Auto-migrate invoices & payments tables (may not exist if 001_initial_schema was skipped)
	if err := database.DB.AutoMigrate(&models.Invoice{}, &models.Payment{}); err != nil {
		log.Printf("⚠️ Failed to auto-migrate invoices/payments tables: %v", err)
	}

	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	isProd := appEnv == "prod" || appEnv == "production"

	// 初始化模板引擎
	engine := html.New("./web/templates", ".html")
	// 性能要點：production 一律關閉 Reload/Debug（它們會顯著拖慢模板渲染）
	engine.Reload(!isProd)
	engine.Debug(!isProd)

	// 添加自定義函數
	engine.AddFunc("makeSlice", func(n interface{}) []int {
		var count int
		switch v := n.(type) {
		case int:
			count = v
		case int64:
			count = int(v)
		case float64:
			count = int(v)
		default:
			count = 12 // 默認值
		}
		if count <= 0 {
			count = 12
		}
		slice := make([]int, count)
		for i := range slice {
			slice[i] = i
		}
		return slice
	})

	// 添加類型轉換函數
	engine.AddFunc("toInt", func(n interface{}) int {
		switch v := n.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			return int(v)
		default:
			return 0
		}
	})

	// 添加 mod 函數（用於模板中的取模運算）
	engine.AddFunc("mod", func(a, b interface{}) int {
		var aInt, bInt int
		switch v := a.(type) {
		case int:
			aInt = v
		case int64:
			aInt = int(v)
		case float64:
			aInt = int(v)
		}
		switch v := b.(type) {
		case int:
			bInt = v
		case int64:
			bInt = int(v)
		case float64:
			bInt = int(v)
		}
		if bInt == 0 {
			return 0
		}
		return aInt % bInt
	})

	// 添加 default 函數（用於模板中的默認值）
	// Signature: default(defaultValue, givenValue) — follows Go template pipeline
	// convention where the piped value arrives as the LAST argument.
	// Usage in template: {{someValue | default "fallback"}}
	//   → default("fallback", someValue)  →  returns someValue if non-nil/non-empty, else "fallback"
	engine.AddFunc("default", func(defaultValue interface{}, givenValue interface{}) interface{} {
		if givenValue == nil {
			return defaultValue
		}
		// 檢查是否為空字符串
		if str, ok := givenValue.(string); ok && str == "" {
			return defaultValue
		}
		return givenValue
	})

	// 添加計算列寬度的函數（用於 product-list）
	engine.AddFunc("calcColumnWidth", func(columns interface{}) string {
		var cols int
		switch v := columns.(type) {
		case int:
			cols = v
		case int64:
			cols = int(v)
		case float64:
			cols = int(v)
		default:
			cols = 3
		}
		if cols <= 0 {
			cols = 3
		}
		// g-2 = 8px gap
		gap := 8
		gaps := (cols - 1) * gap
		return fmt.Sprintf("calc((100%% - %dpx) / %d)", gaps, cols)
	})

	// 添加 safeHTML 函數（用於渲染 HTML 內容而不轉義）
	engine.AddFunc("safeHTML", func(s interface{}) template.HTML {
		if s == nil {
			return template.HTML("")
		}
		// Handle *string pointers (e.g. model fields like Content *string)
		// fmt.Sprint on a *string prints the pointer address, not the value
		if sp, ok := s.(*string); ok {
			if sp == nil {
				return template.HTML("")
			}
			return template.HTML(*sp)
		}
		return template.HTML(fmt.Sprint(s))
	})

	// 添加 safeCSS 函數（用於渲染 inline CSS，而不被 html/template 注入保護替換成 ZgotmplZ）
	// 注意：這會把內容視為可信任 CSS，請確保輸入來源受控（例如後台編輯器）
	engine.AddFunc("safeCSS", func(s interface{}) template.CSS {
		if s == nil {
			return template.CSS("")
		}
		if sp, ok := s.(*string); ok {
			if sp == nil {
				return template.CSS("")
			}
			return template.CSS(*sp)
		}
		return template.CSS(fmt.Sprint(s))
	})

	// cssSize normalises a CSS size value: "400pxpx" → "400px", "400" → "400px", "100%" → "100%"
	engine.AddFunc("cssSize", func(s interface{}, fallback ...string) template.CSS {
		val := strings.TrimSpace(fmt.Sprint(s))
		if val == "" || val == "<nil>" {
			if len(fallback) > 0 {
				val = fallback[0]
			} else {
				val = "auto"
			}
		}
		// Strip repeated unit suffixes (e.g. "400pxpx" → "400")
		units := []string{"px", "rem", "em", "vh", "vw", "%"}
		changed := true
		for changed {
			changed = false
			for _, u := range units {
				if strings.HasSuffix(val, u+u) {
					val = strings.TrimSuffix(val, u)
					changed = true
				}
			}
		}
		// If pure number, append "px"
		if _, err := strconv.ParseFloat(val, 64); err == nil {
			val += "px"
		}
		return template.CSS(val)
	})

	// 添加 toJSON 函數（用於模板中的 JSON 輸出）
	engine.AddFunc("toJSON", func(v interface{}) template.JS {
		b, _ := json.Marshal(v)
		return template.JS(b)
	})

	// 添加 urlquery 函數（用於模板中的 querystring 編碼）
	engine.AddFunc("urlquery", func(s string) string {
		return url.QueryEscape(s)
	})

	// 添加加法函數
	engine.AddFunc("add", func(a, b interface{}) int {
		var aInt, bInt int
		switch v := a.(type) {
		case int:
			aInt = v
		case int64:
			aInt = int(v)
		case float64:
			aInt = int(v)
		}
		switch v := b.(type) {
		case int:
			bInt = v
		case int64:
			bInt = int(v)
		case float64:
			bInt = int(v)
		}
		return aInt + bInt
	})

	// 添加減法函數
	engine.AddFunc("sub", func(a, b interface{}) int {
		var aInt, bInt int
		switch v := a.(type) {
		case int:
			aInt = v
		case int64:
			aInt = int(v)
		case float64:
			aInt = int(v)
		}
		switch v := b.(type) {
		case int:
			bInt = v
		case int64:
			bInt = int(v)
		case float64:
			bInt = int(v)
		}
		return aInt - bInt
	})

	// 添加乘法函數
	engine.AddFunc("mul", func(a, b interface{}) int {
		var aInt, bInt int
		switch v := a.(type) {
		case int:
			aInt = v
		case int64:
			aInt = int(v)
		case float64:
			aInt = int(v)
		}
		switch v := b.(type) {
		case int:
			bInt = v
		case int64:
			bInt = int(v)
		case float64:
			bInt = int(v)
		}
		return aInt * bInt
	})

	// navPrefix: returns product URL prefix based on PageName (e.g. "vai-billing" → "vai", "voffice-download" → "voffice")
	engine.AddFunc("navPrefix", func(pageName interface{}) string {
		s, _ := pageName.(string)
		if strings.HasPrefix(s, "vai-") {
			return "vai"
		}
		if strings.HasPrefix(s, "voffice-") {
			return "voffice"
		}
		if strings.HasPrefix(s, "vmarket-") {
			return "vmarket"
		}
		return ""
	})

	prefork := strings.ToLower(strings.TrimSpace(os.Getenv("FIBER_PREFORK"))) == "true"

	// 創建 Fiber 應用（使用模板引擎）
	// Body limit: default 2GB to support large vOffice installer uploads.
	// Can be overridden via UPLOAD_MAX_FILE_SIZE env var.
	maxBodySize := 2 * 1024 * 1024 * 1024 // 2GB
	if cfg.Upload.MaxFileSize > 0 {
		maxBodySize = cfg.Upload.MaxFileSize
	}
	app := fiber.New(fiber.Config{
		AppName:           cfg.AppName + " ERP",
		Views:             engine,
		PassLocalsToViews: true,
		Prefork:           prefork,
		BodyLimit:         maxBodySize,
	})

	// Favicon: serve the website icon consistently across all pages.
	// Browsers still request /favicon.ico even if <link rel="icon"> is present.
	app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		return c.Redirect("/static/vworkicon.png", fiber.StatusFound) // 302
	})

	// 中間件
	// Recover 中間件：攔截所有 panic，避免整個程序被打死
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
	}))

	// 性能要點：request logger 會顯著影響吞吐；production 預設關閉（可用 APP_REQUEST_LOGGER=true 開回來）
	enableReqLogger := strings.ToLower(strings.TrimSpace(os.Getenv("APP_REQUEST_LOGGER"))) == "true"
	if !isProd || enableReqLogger {
		app.Use(logger.New())
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization,X-Tenant-Subdomain",
	}))

	// 重定向根域名到 www 子域名（例如 vworkai.com -> www.vworkai.com）
	app.Use(func(c *fiber.Ctx) error {
		host := strings.ToLower(strings.TrimSpace(c.Hostname()))
		host = strings.TrimSuffix(host, ".")

		base := strings.ToLower(strings.TrimSpace(cfg.Domain.BaseDomain))
		// 如果 host 正好是根域名（不含 www），重定向到 www 版本
		if base != "" && host == base {
			// 保持原始路径和查询参数
			newURL := fmt.Sprintf("%s://www.%s%s",
				cfg.Domain.Scheme,
				base,
				c.OriginalURL())
			return c.Redirect(newURL, fiber.StatusMovedPermanently) // 301 永久重定向
		}
		return c.Next()
	})

	// 靜態文件服務
	// locale JSON 可長期 cache（1 小時），減少每次頁面載入都重新 fetch，降低翻譯 load 不到的機率
	app.Static("/static/locales", "./web/static/locales", fiber.Static{
		MaxAge: 3600, // 1 hour
	})
	app.Static("/static", "./web/static")
	app.Static("/uploads", "./web/uploads")

	// 自訂網域（Custom Domain）請求：若 Host 命中租戶設定的自訂網域，則把 / 或 /:slug 映射到該租戶的公開頁面
	// 注意：這裡只做「網站顯示」的 routing；SSL 自動化仍建議搭配 Cloudflare for SaaS/SSL for SaaS。
	app.Use(func(c *fiber.Ctx) error {
		// Only handle GET/HEAD for web pages.
		if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
			return c.Next()
		}

		path := c.Path()
		// Skip API/static/uploads and existing /co/ routes.
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/uploads/") || strings.HasPrefix(path, "/co/") {
			return c.Next()
		}
		if path == "/favicon.ico" || path == "/health" || path == "/test" {
			return c.Next()
		}

		host := strings.ToLower(strings.TrimSpace(c.Hostname()))
		host = strings.TrimSuffix(host, ".")
		if host == "" {
			return c.Next()
		}

		// Do not treat base domain (and www) as custom domain.
		base := strings.ToLower(strings.TrimSpace(cfg.Domain.BaseDomain))
		if base != "" && (host == base || host == "www."+base || strings.HasSuffix(host, "."+base)) {
			return c.Next()
		}

		// Resolve tenant by extra_fields.website_custom_domains (jsonb array) or fallback website_custom_domain (string)
		var tenant models.Tenant
		if err := database.DB.
			Where("status = ? AND (jsonb_exists(extra_fields->'website_custom_domains', ?) OR extra_fields->>'website_custom_domain' = ?)", "active", host, host).
			First(&tenant).Error; err != nil {
			return c.Next()
		}

		// Map "/" -> homepage, "/slug" -> page slug
		slug := strings.Trim(path, "/")
		if strings.Contains(slug, "/") {
			return c.Next()
		}
		return handlers.RenderPublicPageBySubdomainAndSlug(c, tenant.Subdomain, slug)
	})

	// 測試路由
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("<h1>Test Page - Server is working!</h1><p>If you see this, the server is OK.</p>")
	})

	// 公開路由（不需要認證）
	app.Get("/robots.txt", handlers.RobotsTxt)
	app.Get("/sitemap.xml", handlers.SitemapXML)
	renderLocalizedHome := func(c *fiber.Ctx, forcedLang string) error {
		host := strings.ToLower(strings.TrimSpace(c.Hostname()))
		host = strings.TrimSuffix(host, ".")
		domainParam := strings.ToLower(strings.TrimSpace(c.Query("domain")))

		if domainParam == "vsys" || domainParam == "vsysai" {
			return c.Render("index_vsys", fiber.Map{"SEO": handlers.NewLandingSEO(c, "vsys", forcedLang)})
		}
		if domainParam == "vmarket" || domainParam == "vmarketai" {
			return c.Redirect("/", fiber.StatusMovedPermanently)
		}
		if domainParam == "vwork" || domainParam == "vworkai" {
			return c.Render("index", fiber.Map{
				"SEO":                  handlers.NewLandingSEO(c, "vwork", forcedLang),
				"CompanyName":          cfg.CompanyName,
				"TrialDays":            cfg.TrialDays,
				"StripePublishableKey": cfg.Stripe.PublishableKey,
				"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
			})
		}
		if domainParam == "voffice" {
			return c.Render("index_voffice", fiber.Map{"SEO": handlers.NewLandingSEO(c, "voffice", forcedLang)})
		}
		if domainParam == "vai" {
			tokenString := c.Cookies("auth_token")
			if tokenString == "" {
				authHeader := c.Get("Authorization")
				if authHeader != "" {
					tokenString = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			if tokenString != "" {
				if claims, err := utils.ValidateToken(tokenString); err == nil && claims.UserID != uuid.Nil {
					_ = claims
					return c.Redirect("/vai-chat")
				}
			}
			return c.Render("index_vai", fiber.Map{"SEO": handlers.NewLandingSEO(c, "vai", forcedLang)})
		}

		isLocal := host == "localhost" || host == "127.0.0.1" ||
			strings.HasPrefix(host, "localhost:") ||
			strings.HasPrefix(host, "127.0.0.1:") ||
			strings.Contains(host, "localhost") ||
			strings.Contains(host, "127.0.0.1") ||
			host == "" ||
			(!strings.Contains(host, ".") && host != "" && !strings.Contains(host, ":"))
		if isLocal {
			return c.Render("index", fiber.Map{
				"SEO":                  handlers.NewLandingSEO(c, "vwork", forcedLang),
				"CompanyName":          cfg.CompanyName,
				"TrialDays":            cfg.TrialDays,
				"StripePublishableKey": cfg.Stripe.PublishableKey,
				"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
			})
		}

		switch host {
		case "www.vsysai.com", "vsysai.com":
			return c.Render("index_vsys", fiber.Map{"SEO": handlers.NewLandingSEO(c, "vsys", forcedLang)})
		case "www.vmarketai.com", "vmarketai.com":
			return c.Redirect("https://www.vworkai.com/", fiber.StatusMovedPermanently)
		case "voffice.vsysai.com", "www.voffice.vsysai.com":
			return c.Render("index_voffice", fiber.Map{"SEO": handlers.NewLandingSEO(c, "voffice", forcedLang)})
		case "vai.vsysai.com", "www.vai.vsysai.com":
			tokenString := c.Cookies("auth_token")
			if tokenString == "" {
				authHeader := c.Get("Authorization")
				if authHeader != "" {
					tokenString = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			if tokenString != "" {
				if claims, err := utils.ValidateToken(tokenString); err == nil && claims.UserID != uuid.Nil {
					_ = claims
					return c.Redirect("/vai-chat")
				}
			}
			return c.Render("index_vai", fiber.Map{"SEO": handlers.NewLandingSEO(c, "vai", forcedLang)})
		case "www.vwork.com", "vwork.com":
			return c.Render("index", fiber.Map{
				"SEO":                  handlers.NewLandingSEO(c, "vwork", forcedLang),
				"CompanyName":          cfg.CompanyName,
				"TrialDays":            cfg.TrialDays,
				"StripePublishableKey": cfg.Stripe.PublishableKey,
				"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
			})
		default:
			return c.Render("index", fiber.Map{
				"SEO":                  handlers.NewLandingSEO(c, "vwork", forcedLang),
				"CompanyName":          cfg.CompanyName,
				"TrialDays":            cfg.TrialDays,
				"StripePublishableKey": cfg.Stripe.PublishableKey,
				"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
			})
		}
	}
	app.Get("/en", func(c *fiber.Ctx) error { return renderLocalizedHome(c, "en") })
	app.Get("/zh-cn", func(c *fiber.Ctx) error { return renderLocalizedHome(c, "zh-CN") })

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"message": cfg.AppName + " API is running",
		})
	})

	// 主頁 - 根據域名導向不同頁面
	app.Get("/", func(c *fiber.Ctx) error {
		host := strings.ToLower(strings.TrimSpace(c.Hostname()))
		host = strings.TrimSuffix(host, ".")
		domainParam := strings.ToLower(strings.TrimSpace(c.Query("domain")))

		// 查詢參數優先：如果提供了 ?domain=vsys 或 ?domain=vsysai，直接返回對應頁面
		// 這在本地開發和生產環境都適用
		if domainParam == "vsys" || domainParam == "vsysai" {
			return c.Render("index_vsys", fiber.Map{"SEO": handlers.NewLandingSEO(c, "vsys", handlers.DetectPathLang(c.Path()))})
		}
		if domainParam == "vmarket" || domainParam == "vmarketai" {
			return c.Redirect("/", fiber.StatusMovedPermanently)
		}
		if domainParam == "vwork" || domainParam == "vworkai" {
			return c.Render("index", fiber.Map{
				"SEO":                  handlers.NewLandingSEO(c, "vwork", handlers.DetectPathLang(c.Path())),
				"CompanyName":          cfg.CompanyName,
				"TrialDays":            cfg.TrialDays,
				"StripePublishableKey": cfg.Stripe.PublishableKey,
				"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
			})
		}
		if domainParam == "voffice" {
			return c.Render("index_voffice", fiber.Map{"SEO": handlers.NewLandingSEO(c, "voffice", handlers.DetectPathLang(c.Path()))})
		}
		if domainParam == "vai" {
			// Check if user is logged in (full server-side auth check matching AuthMiddleware)
			tokenString := c.Cookies("auth_token")
			if tokenString == "" {
				authHeader := c.Get("Authorization")
				if authHeader != "" {
					tokenString = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			validSession := false
			if tokenString != "" {
				if claims, err := utils.ValidateToken(tokenString); err == nil && claims.UserID != uuid.Nil {
					// Full validation: check user exists, is active, and not logged out (same as AuthMiddleware)
					var user models.User
					var dbErr error
					if claims.TenantID == uuid.Nil {
						dbErr = database.DB.Select("id", "status", "logged_out_at", "web_logged_out_at", "desktop_logged_out_at").
							Where("id = ?", claims.UserID).First(&user).Error
					} else {
						dbErr = database.DB.Select("id", "status", "logged_out_at", "web_logged_out_at", "desktop_logged_out_at").
							Where("id = ? AND tenant_id = ?", claims.UserID, claims.TenantID).First(&user).Error
					}
					if dbErr == nil && strings.TrimSpace(strings.ToLower(user.Status)) == "active" {
						// SSO platform-aware logout check
						ssoValid := true
						if claims.IssuedAt != nil {
							var loggedOutAt *time.Time
							if claims.Platform == "desktop" {
								loggedOutAt = user.DesktopLoggedOutAt
							} else {
								loggedOutAt = user.WebLoggedOutAt
								if loggedOutAt == nil {
									loggedOutAt = user.LoggedOutAt
								}
							}
							if loggedOutAt != nil && claims.IssuedAt.Time.Before(*loggedOutAt) {
								ssoValid = false
								// Clear the stale cookie so the browser stops sending it
								c.ClearCookie("auth_token")
							}
						}
						if ssoValid {
							validSession = true
						}
					}
				}
			}
			if validSession {
				return c.Redirect("/vai-chat")
			}
			// Not logged in or session invalid: render landing page
			return c.Render("index_vai", fiber.Map{"SEO": handlers.NewLandingSEO(c, "vai", handlers.DetectPathLang(c.Path()))})
		}

		// 檢查是否為本地開發環境
		isLocal := host == "localhost" || host == "127.0.0.1" ||
			strings.HasPrefix(host, "localhost:") ||
			strings.HasPrefix(host, "127.0.0.1:") ||
			strings.Contains(host, "localhost") ||
			strings.Contains(host, "127.0.0.1") ||
			host == "" ||
			(!strings.Contains(host, ".") && host != "" && !strings.Contains(host, ":"))

		// 本地開發環境：如果沒有查詢參數，預設使用 vwork
		if isLocal {
			return c.Render("index", fiber.Map{
				"SEO":                  handlers.NewLandingSEO(c, "vwork", handlers.DetectPathLang(c.Path())),
				"CompanyName":          cfg.CompanyName,
				"TrialDays":            cfg.TrialDays,
				"StripePublishableKey": cfg.Stripe.PublishableKey,
				"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
			})
		}

		// 生產環境：根據實際域名判斷
		switch host {
		case "www.vsysai.com", "vsysai.com":
			// vsysai.com 使用專屬的 vSys 首頁
			return c.Render("index_vsys", fiber.Map{"SEO": handlers.NewLandingSEO(c, "vsys", handlers.DetectPathLang(c.Path()))})
		case "www.vmarketai.com", "vmarketai.com":
			return c.Redirect("https://www.vworkai.com/", fiber.StatusMovedPermanently)
		case "voffice.vsysai.com", "www.voffice.vsysai.com":
			// voffice.vsysai.com 使用 vOffice 首頁
			return c.Render("index_voffice", fiber.Map{"SEO": handlers.NewLandingSEO(c, "voffice", handlers.DetectPathLang(c.Path()))})
		case "vai.vsysai.com", "www.vai.vsysai.com":
			// vai.vsysai.com — redirect to /vai-chat if logged in, show landing if not
			// Full auth check matching AuthMiddleware (JWT + DB user + SSO logout timestamp)
			tokenString := c.Cookies("auth_token")
			if tokenString == "" {
				authHeader := c.Get("Authorization")
				if authHeader != "" {
					tokenString = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}
			vaiValidSession := false
			if tokenString != "" {
				if claims, err := utils.ValidateToken(tokenString); err == nil && claims.UserID != uuid.Nil {
					var user models.User
					var dbErr error
					if claims.TenantID == uuid.Nil {
						dbErr = database.DB.Select("id", "status", "logged_out_at", "web_logged_out_at", "desktop_logged_out_at").
							Where("id = ?", claims.UserID).First(&user).Error
					} else {
						dbErr = database.DB.Select("id", "status", "logged_out_at", "web_logged_out_at", "desktop_logged_out_at").
							Where("id = ? AND tenant_id = ?", claims.UserID, claims.TenantID).First(&user).Error
					}
					if dbErr == nil && strings.TrimSpace(strings.ToLower(user.Status)) == "active" {
						ssoValid := true
						if claims.IssuedAt != nil {
							var loggedOutAt *time.Time
							if claims.Platform == "desktop" {
								loggedOutAt = user.DesktopLoggedOutAt
							} else {
								loggedOutAt = user.WebLoggedOutAt
								if loggedOutAt == nil {
									loggedOutAt = user.LoggedOutAt
								}
							}
							if loggedOutAt != nil && claims.IssuedAt.Time.Before(*loggedOutAt) {
								ssoValid = false
								c.ClearCookie("auth_token")
							}
						}
						if ssoValid {
							vaiValidSession = true
						}
					}
				}
			}
			if vaiValidSession {
				return c.Redirect("/vai-chat")
			}
			return c.Render("index_vai", fiber.Map{"SEO": handlers.NewLandingSEO(c, "vai", handlers.DetectPathLang(c.Path()))})
		case "www.vwork.com", "vwork.com":
			// vwork.com 使用標準 vWork 首頁
			return c.Render("index", fiber.Map{
				"SEO":                  handlers.NewLandingSEO(c, "vwork", handlers.DetectPathLang(c.Path())),
				"CompanyName":          cfg.CompanyName,
				"TrialDays":            cfg.TrialDays,
				"StripePublishableKey": cfg.Stripe.PublishableKey,
				"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
			})
		default:
			// 其他域名使用預設 vWork 頁面
			return c.Render("index", fiber.Map{
				"SEO":                  handlers.NewLandingSEO(c, "vwork", handlers.DetectPathLang(c.Path())),
				"CompanyName":          cfg.CompanyName,
				"TrialDays":            cfg.TrialDays,
				"StripePublishableKey": cfg.Stripe.PublishableKey,
				"WhatsAppPhone":        cfg.WhatsApp.PhoneNumber,
			})
		}
	})

	// vWork 官網 Blog（平台級，非租戶）
	app.Get("/vwork-blog", handlers.RenderPlatformBlogList)
	app.Get("/vwork-blog/:slug", handlers.RenderPlatformBlogPost)
	app.Get("/api/v1/platform-blogs", handlers.PublicPlatformBlogList)

	// vWork 官網 Events（平台級，非租戶）
	app.Get("/vwork-events", handlers.RenderPlatformEventList)
	app.Get("/vwork-events/:slug", handlers.RenderPlatformEventDetail)
	app.Get("/api/v1/platform-events", handlers.PublicPlatformEventList)
	app.Post("/api/v1/platform-events/:slug/register", handlers.PublicRegisterForEvent)

	// vWork 官網 Industry / Custom pages
	app.Get("/industry/:industry", handlers.RenderIndustryPage)
	app.Get("/custom/:page", handlers.RenderCustomPage)

	// VMarket 已停用：所有公開頁面導回 vWork 首頁
	vmarketDisabled := func(c *fiber.Ctx) error {
		return c.Redirect("/", fiber.StatusMovedPermanently)
	}
	app.Get("/vmarket", vmarketDisabled)
	app.Get("/vmarket/products", vmarketDisabled)
	app.Get("/vmarket/services", vmarketDisabled)
	app.Get("/vmarket/companies", vmarketDisabled)
	app.Get("/vmarket/map", vmarketDisabled)
	app.Get("/vmarket/join", vmarketDisabled)
	app.Get("/en/vmarket", vmarketDisabled)
	app.Get("/en/vmarket/products", vmarketDisabled)
	app.Get("/en/vmarket/services", vmarketDisabled)
	app.Get("/en/vmarket/companies", vmarketDisabled)
	app.Get("/en/vmarket/map", vmarketDisabled)
	app.Get("/en/vmarket/join", vmarketDisabled)
	app.Get("/zh-cn/vmarket", vmarketDisabled)
	app.Get("/zh-cn/vmarket/products", vmarketDisabled)
	app.Get("/zh-cn/vmarket/services", vmarketDisabled)
	app.Get("/zh-cn/vmarket/companies", vmarketDisabled)
	app.Get("/zh-cn/vmarket/map", vmarketDisabled)
	app.Get("/zh-cn/vmarket/join", vmarketDisabled)
	app.Get("/vmarket/api/search", vmarketDisabled)

	// VMarket routes without /vmarket prefix (for vmarketai.com production domain ONLY).
	// NOTE: /products and /services are NOT registered here — they conflict with
	// vWork CMS routes at the same path and Fiber v2 c.Next() cannot fall through
	// from app.Get() to cmsRoutes.Get() for the same path. Instead, they are
	// handled inside cmsRoutes with a domain check. AuthMiddleware skips auth for
	// VMarket domain on /products and /services so that cmsRoutes can serve
	// VMarket pages to unauthenticated vmarketai.com visitors.
	// Local dev uses the /vmarket/* prefix routes above, so no conflict.
	vmarketDomainOnly := func(handler fiber.Handler) fiber.Handler {
		return func(c *fiber.Ctx) error {
			if handlers.IsVMarketDomain(c) {
				return c.Redirect("https://www.vworkai.com/", fiber.StatusMovedPermanently)
			}
			return c.Next()
		}
	}
	app.Get("/companies", vmarketDomainOnly(handlers.RenderVMarketCompanies))
	app.Get("/map", vmarketDomainOnly(handlers.RenderVMarketMap))
	app.Get("/join", vmarketDomainOnly(handlers.RenderVMarketJoin))
	app.Get("/api/vmarket/search", vmarketDisabled)

	// 登錄頁面（公開路由，不需要認證）
	app.Get("/login", func(c *fiber.Ctx) error {
		// 不檢查 cookie，直接顯示登錄頁面
		// 讓前端 JavaScript 來處理已登錄狀態的重定向，避免後端重定向導致的循環
		// 設置響應頭，防止緩存
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")

		// 供前端在 production 使用（避免瀏覽器直連第三方 IP API 造成 CORS/429/403）
		// 常見來源：Cloudflare (CF-IPCountry)、CloudFront (CloudFront-Viewer-Country)、AppEngine (X-Appengine-Country-Code)
		ipCountry := strings.TrimSpace(c.Get("CF-IPCountry"))
		if ipCountry == "" {
			ipCountry = strings.TrimSpace(c.Get("CloudFront-Viewer-Country"))
		}
		if ipCountry == "" {
			ipCountry = strings.TrimSpace(c.Get("X-Appengine-Country"))
		}
		if ipCountry == "" {
			ipCountry = strings.TrimSpace(c.Get("X-Appengine-Country-Code"))
		}
		ipCountry = strings.ToUpper(strings.TrimSpace(ipCountry))
		if len(ipCountry) != 2 || ipCountry == "XX" {
			ipCountry = ""
		}
		// Compute Engine VM 通常沒有自動 country header；用 GeoIP（若有 mmdb）補上
		if ipCountry == "" {
			ipCountry = utils.DetectCountryFromRequest(c)
		}

		return c.Render("login", fiber.Map{
			"CompanyName": cfg.CompanyName,
			"TrialDays":   cfg.TrialDays,
			"BaseDomain":  cfg.Domain.BaseDomain,
			"IPCountry":   ipCountry,
		})
	})

	// 重設密碼頁面（公開路由，從 email 連結進來）
	app.Get("/reset-password", func(c *fiber.Ctx) error {
		token := c.Query("token", "")
		// 防止被緩存
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		return c.Render("reset_password", fiber.Map{
			"CompanyName": cfg.CompanyName,
			"BaseDomain":  cfg.Domain.BaseDomain,
			"Token":       token,
		})
	})

	// 設定密碼頁面（公開路由，從邀請 email 連結進來）
	app.Get("/set-password", func(c *fiber.Ctx) error {
		token := c.Query("token", "")
		tenantName := ""

		// 從 token 解析租戶名稱（如果可能的話）
		if token != "" {
			hash := sha256.Sum256([]byte(token))
			var resetToken models.PasswordResetToken
			if err := database.DB.Where("token_hash = ? AND used_at IS NULL AND expires_at > NOW()", hash[:]).First(&resetToken).Error; err == nil {
				// 查找租戶名稱
				var tenant models.Tenant
				if err := database.DB.Where("id = ?", resetToken.TenantID).First(&tenant).Error; err == nil {
					tenantName = tenant.Name
				}
			}
		}

		// 防止被緩存
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		return c.Render("set_password", fiber.Map{
			"CompanyName": cfg.CompanyName,
			"BaseDomain":  cfg.Domain.BaseDomain,
			"Token":       token,
			"TenantName":  tenantName,
		})
	})

	// 接受租戶邀請頁面（公開路由，從邀請 email 連結進來）
	app.Get("/accept-invite", func(c *fiber.Ctx) error {
		token := c.Query("token", "")
		tenantName := ""
		inviterName := ""
		inviteEmail := ""
		hasAccount := false

		if token != "" {
			hash := sha256.Sum256([]byte(token))
			var invitation models.TenantInvitation
			if err := database.DB.Where("token_hash = ? AND status = 'pending' AND expires_at > NOW()", hash[:]).First(&invitation).Error; err == nil {
				inviteEmail = invitation.Email
				var tenant models.Tenant
				if err := database.DB.Where("id = ?", invitation.TenantID).First(&tenant).Error; err == nil {
					tenantName = tenant.Name
				}
				var inviter models.User
				if err := database.DB.Where("id = ?", invitation.InviterID).First(&inviter).Error; err == nil {
					inviterName = inviter.Name
				}
				// Check if email already has account
				var existingUser models.User
				if database.DB.Where("LOWER(email) = ?", strings.ToLower(invitation.Email)).First(&existingUser).Error == nil {
					hasAccount = true
				}
			}
		}

		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		return c.Render("accept_invite", fiber.Map{
			"CompanyName": cfg.CompanyName,
			"BaseDomain":  cfg.Domain.BaseDomain,
			"Token":       token,
			"TenantName":  tenantName,
			"InviterName": inviterName,
			"InviteEmail": inviteEmail,
			"HasAccount":  hasAccount,
		})
	})

	// 聯絡客服頁面（公開路由，不需要登錄）
	// IMPORTANT: 此路由必須保持為公開路由（不使用 AuthMiddleware），任何人都可以訪問
	// DO NOT CHANGE: 不要將此路由移到 cmsRoutes 組中，否則會要求登錄
	app.Get("/contact", func(c *fiber.Ctx) error {
		return c.Render("pages/contact", fiber.Map{
			"CompanyName":   cfg.CompanyName,
			"WhatsAppPhone": cfg.WhatsApp.PhoneNumber,
			"Path":          c.Path(),
		})
	})

	// 企業客製化方案頁面（公開路由，不需要登錄）
	app.Get("/enterprise-custom", func(c *fiber.Ctx) error {
		return c.Render("pages/enterprise_custom", fiber.Map{
			"CompanyName":   cfg.CompanyName,
			"WhatsAppPhone": cfg.WhatsApp.PhoneNumber,
			"Path":          c.Path(),
		})
	})

	// 客製化 AI Agent 頁面（公開路由，不需要登錄）
	app.Get("/custom-ai-agent", func(c *fiber.Ctx) error {
		return c.Render("pages/custom_ai_agent", fiber.Map{
			"CompanyName":   cfg.CompanyName,
			"WhatsAppPhone": cfg.WhatsApp.PhoneNumber,
			"Path":          c.Path(),
		})
	})

	// Terms of Service（公開路由）
	app.Get("/terms", func(c *fiber.Ctx) error {
		return c.Render("pages/terms", fiber.Map{
			"CompanyName": cfg.CompanyName,
		})
	})

	// Privacy Policy（公開路由）
	app.Get("/privacy", func(c *fiber.Ctx) error {
		return c.Render("pages/privacy", fiber.Map{
			"CompanyName": cfg.CompanyName,
		})
	})

	// 銷售商加盟頁面（公開路由）
	app.Get("/sales-partner", func(c *fiber.Ctx) error {
		return c.Render("pages/sales_partner_apply", fiber.Map{
			"CompanyName": cfg.CompanyName,
		})
	})

	// 銷售商加盟申請（公開 API）
	app.Post("/api/v1/sales-partner/apply", handlers.ApplySalesPartner)

	// 教學腳本播放頁（公開路由，用於自動化教學/錄影；實際操作仍依登入狀態而定）
	app.Get("/tutorial/scenarios", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")

		type ScenarioInfo struct {
			Name  string `json:"name"`
			Title string `json:"title"`
		}

		type ScenarioFile struct {
			Name  string `json:"name"`
			Title string `json:"title"`
		}

		matches, err := filepath.Glob("./web/static/tutorials/*.json")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		out := make([]ScenarioInfo, 0, len(matches))
		for _, p := range matches {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var parsed ScenarioFile
			if err := json.Unmarshal(b, &parsed); err != nil {
				continue
			}
			name := strings.TrimSpace(parsed.Name)
			if name == "" {
				base := filepath.Base(p)
				name = strings.TrimSuffix(base, filepath.Ext(base))
			}
			out = append(out, ScenarioInfo{
				Name:  name,
				Title: strings.TrimSpace(parsed.Title),
			})
		}

		sort.Slice(out, func(i, j int) bool {
			return out[i].Name < out[j].Name
		})

		return c.JSON(out)
	})

	app.Get("/tutorial/run", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")

		scenario := c.Query("scenario", "login_demo")
		cbPort := c.Query("cbPort", "")
		token := c.Query("token", "")
		email := c.Query("email", "")
		password := c.Query("password", "")
		autostart := c.Query("autostart", "true")
		autostartDelayMs := c.Query("autostartDelayMs", "")
		uiOverlay := c.Query("uiOverlay", "false")
		guide := c.Query("guide", "true")
		startOverlay := c.Query("startOverlay", "false")
		stepDelayMs := c.Query("stepDelayMs", "")

		return c.Render("pages/tutorial_run", fiber.Map{
			"Title":            "教學腳本播放",
			"Scenario":         scenario,
			"CallbackPort":     cbPort,
			"CallbackToken":    token,
			"Email":            email,
			"Password":         password,
			"AutoStart":        autostart,
			"AutoStartDelayMs": autostartDelayMs,
			"UiOverlay":        uiOverlay,
			"Guide":            guide,
			"StartOverlay":     startOverlay,
			"StepDelayMs":      stepDelayMs,
			"IsTutorialRun":    true,
			"CacheBust":        time.Now().UnixNano(),
			"Path":             c.Path(),
		}, "layouts/embed_layout")
	})

	// 支援中心 Hub 頁面（公開路由，不需要登錄）
	renderHelpHub := func(c *fiber.Ctx) error {
		return c.Render("pages/help_hub", fiber.Map{
			"Title":       "支援中心",
			"CompanyName": cfg.CompanyName,
			"Path":        c.Path(),
		})
	}
	renderHelpVWork := func(c *fiber.Ctx) error {
		return c.Render("pages/help", fiber.Map{
			"Title":         "支援中心 - vWork",
			"CompanyName":   cfg.CompanyName,
			"WhatsAppPhone": cfg.WhatsApp.PhoneNumber,
			"Product":       "vwork",
			"Category":      "",
			"CategoryTitle": "",
			"Path":          c.Path(),
		})
	}
	renderHelpVai := func(c *fiber.Ctx) error {
		return c.Render("pages/help_vai", fiber.Map{
			"Title":         "支援中心 - vAI",
			"CompanyName":   cfg.CompanyName,
			"Product":       "vai",
			"Category":      "",
			"CategoryTitle": "",
			"Path":          c.Path(),
		})
	}
	renderHelpVaiCategory := func(c *fiber.Ctx) error {
		category := c.Params("category")
		categoryTitles := map[string]string{
			"getting-started": "快速入門",
			"chat":            "vAI 交談",
			"sketch":          "vAI 圖片",
			"video":           "vAI 影片",
			"docs":            "vAI 文件",
			"vcoins":          "vCoin 虛擬代幣",
			"account":         "帳戶與訂閱",
			"faq":             "常見問題",
		}
		categoryTitle := categoryTitles[category]
		if categoryTitle == "" {
			categoryTitle = category
		}
		return c.Render("pages/help_vai", fiber.Map{
			"Title":         "支援中心 - vAI - " + categoryTitle,
			"CompanyName":   cfg.CompanyName,
			"Product":       "vai",
			"Category":      category,
			"CategoryTitle": categoryTitle,
			"Path":          c.Path(),
		})
	}
	renderHelpVMarket := func(c *fiber.Ctx) error {
		return c.Render("pages/help_vmarket", fiber.Map{
			"Title":         "支援中心 - vMarket",
			"CompanyName":   cfg.CompanyName,
			"Product":       "vmarket",
			"Category":      "",
			"CategoryTitle": "",
			"Path":          c.Path(),
		})
	}
	renderHelpVMarketCategory := func(c *fiber.Ctx) error {
		category := c.Params("category")
		categoryTitles := map[string]string{
			"getting-started": "快速入門",
			"browse":          "瀏覽市集",
			"search":          "搜尋功能",
			"map":             "地圖功能",
			"join":            "加入 vMarket",
			"manage":          "管理上架內容",
			"account":         "帳戶與訂閱",
			"faq":             "常見問題",
		}
		categoryTitle := categoryTitles[category]
		if categoryTitle == "" {
			categoryTitle = category
		}
		return c.Render("pages/help_vmarket", fiber.Map{
			"Title":         "支援中心 - vMarket - " + categoryTitle,
			"CompanyName":   cfg.CompanyName,
			"Product":       "vmarket",
			"Category":      category,
			"CategoryTitle": categoryTitle,
			"Path":          c.Path(),
		})
	}
	renderHelpVOffice := func(c *fiber.Ctx) error {
		return c.Render("pages/help_voffice", fiber.Map{
			"Title":       "支援中心 - vOffice",
			"CompanyName": cfg.CompanyName,
			"Product":     "voffice",
			"Path":        c.Path(),
		})
	}
	renderHelpVWorkDNS := func(c *fiber.Ctx) error {
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}
		cnameTarget := strings.TrimSpace(os.Getenv("CUSTOM_DOMAIN_CNAME_TARGET"))
		if cnameTarget == "" && baseDomain != "" {
			cnameTarget = "cname." + baseDomain
		}
		return c.Render("pages/help", fiber.Map{
			"Title":                   "支援中心 - vWork",
			"CompanyName":             cfg.CompanyName,
			"WhatsAppPhone":           cfg.WhatsApp.PhoneNumber,
			"Product":                 "vwork",
			"Category":                "dns-setup",
			"CategoryTitle":           "DNS 設定",
			"CustomDomainCnameTarget": cnameTarget,
			"Path":                    c.Path(),
		})
	}
	renderHelpVWorkCategory := func(c *fiber.Ctx) error {
		category := c.Params("category")
		categoryTitles := map[string]string{
			"account":          "帳戶管理",
			"product":          "商品管理",
			"order":            "訂單管理",
			"customer":         "客戶管理",
			"service":          "服務管理",
			"pos":              "POS 收銀系統",
			"pos-self-service": "POS 自助",
			"vbuilder":         "vBuilder 網站",
			"getting-started":  "快速入門指南",
			"tutorial-videos":  "教學影片庫",
			"import-export":    "匯入 / 匯出",
			"inventory":        "庫存管理",
			"accounting":       "會計管理",
			"hr":               "HR 人力資源",
			"dynamic-fields":   "動態字段功能",
			"reports":          "報表功能",
			"ai":               "vAi 智能助手",
			"purchase":         "採購管理",
			"supplier":         "供應商管理",
			"warehouse":        "倉庫管理",
			"project":          "項目管理",
			"store":            "店舖管理",
			"website":          "網站管理",
			"promotion":        "優惠管理",
			"resource":         "資源管理",
			"personal-tools":   "個人工具",
			"dns-setup":        "DNS 設定",
		}
		categoryTitle := categoryTitles[category]
		if categoryTitle == "" {
			categoryTitle = category
		}
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}
		cnameTarget := strings.TrimSpace(os.Getenv("CUSTOM_DOMAIN_CNAME_TARGET"))
		if cnameTarget == "" && baseDomain != "" {
			cnameTarget = "cname." + baseDomain
		}
		return c.Render("pages/help", fiber.Map{
			"Title":                   "支援中心 - vWork",
			"CompanyName":             cfg.CompanyName,
			"WhatsAppPhone":           cfg.WhatsApp.PhoneNumber,
			"Product":                 "vwork",
			"Category":                category,
			"CategoryTitle":           categoryTitle,
			"CustomDomainCnameTarget": cnameTarget,
			"Path":                    c.Path(),
		})
	}

	// /help → 產品選擇頁（vWork / vMarket / vOffice）
	app.Get("/help", renderHelpHub)
	app.Get("/en/help", renderHelpHub)
	app.Get("/zh-cn/help", renderHelpHub)

	// vWork 支援中心首頁（顯示分類列表）
	app.Get("/help/vwork", renderHelpVWork)
	app.Get("/en/help/vwork", renderHelpVWork)
	app.Get("/zh-cn/help/vwork", renderHelpVWork)

	// vAI 支援中心
	app.Get("/help/vai", renderHelpVai)
	app.Get("/en/help/vai", renderHelpVai)
	app.Get("/zh-cn/help/vai", renderHelpVai)
	app.Get("/help/vai/:category", renderHelpVaiCategory)
	app.Get("/en/help/vai/:category", renderHelpVaiCategory)
	app.Get("/zh-cn/help/vai/:category", renderHelpVaiCategory)

	// vMarket 支援中心已停用
	app.Get("/help/vmarket", vmarketDisabled)
	app.Get("/en/help/vmarket", vmarketDisabled)
	app.Get("/zh-cn/help/vmarket", vmarketDisabled)
	app.Get("/help/vmarket/:category", vmarketDisabled)
	app.Get("/en/help/vmarket/:category", vmarketDisabled)
	app.Get("/zh-cn/help/vmarket/:category", vmarketDisabled)

	// vOffice 支援中心
	app.Get("/help/voffice", renderHelpVOffice)
	app.Get("/en/help/voffice", renderHelpVOffice)
	app.Get("/zh-cn/help/voffice", renderHelpVOffice)

	// vWork DNS 設定教學（公開頁面）
	// IMPORTANT: 必須放在 /help/vwork/:category 之前，避免被參數路由吃掉
	app.Get("/help/vwork/dns-setup", renderHelpVWorkDNS)
	app.Get("/en/help/vwork/dns-setup", renderHelpVWorkDNS)
	app.Get("/zh-cn/help/vwork/dns-setup", renderHelpVWorkDNS)
	app.Get("/help/vwork/:category", renderHelpVWorkCategory)
	app.Get("/en/help/vwork/:category", renderHelpVWorkCategory)
	app.Get("/zh-cn/help/vwork/:category", renderHelpVWorkCategory)

	// 向後相容：舊的 /help/:category 路由重定向到 /help/vwork/:category
	app.Get("/help/:category", func(c *fiber.Ctx) error {
		category := c.Params("category")
		// 如果是產品名稱，不重定向
		if category == "vwork" || category == "vmarket" || category == "voffice" {
			return c.Next()
		}
		return c.Redirect("/help/vwork/"+category, fiber.StatusMovedPermanently)
	})

	// WhatsApp Webhook（公開路由，Meta 會調用此端點）
	app.Get("/api/whatsapp/webhook", handlers.WhatsAppWebhook)
	app.Post("/api/whatsapp/webhook", handlers.WhatsAppWebhook)

	// Shopee OAuth Callback（公開路由，Shopee 會調用此端點）
	app.Get("/api/shopee/callback", handlers.ShopeeCallback)

	// 外賣平台 Webhook（公開路由，Foodpanda/Keeta/Deliveroo 會調用此端點）
	app.Post("/api/v1/delivery/webhook/:platform", handlers.DeliveryWebhookV2)

	// 公司頁面（公開路由，根據子域名顯示）
	app.Get("/co/:subdomain/", handlers.RenderPublicHomepage)

	// 餐飲：公開點餐入口與候位取號
	app.Get("/co/:subdomain/dining/order/", handlers.RenderPublicDiningOrder)
	app.Get("/co/:subdomain/dining/queue/", handlers.RenderPublicDiningQueuePage)
	app.Get("/co/:subdomain/dining/queue/take/", handlers.RenderPublicDiningQueueTakePage)
	app.Get("/co/:subdomain/dining/table/:code", handlers.RenderPublicDiningMenu)
	app.Get("/co/:subdomain/menu/", handlers.RenderPublicDiningMenuOnly)

	// 部落格文章獨立頁面（SEO 友好的伺服器端渲染）
	// 注意：必須在 catch-all :slug 路由之前註冊
	app.Get("/co/:subdomain/blog/:slug/", handlers.RenderPublicBlogPost)

	// 公司頁面下的自定義頁面（公開路由，根據子域名和 slug 顯示）
	// 注意：此路由會處理所有 /co/:subdomain/:slug/ 的請求，包括首頁（slug 為空時）
	app.Get("/co/:subdomain/:slug/", handlers.RenderPublicPage)

	// 公開產品 API（用於網店）
	app.Get("/api/v1/public/:subdomain/products/:id", handlers.PublicGetProduct)

	// 公開 Blog API（用於 blog-list 元件）
	app.Get("/api/v1/public/:subdomain/blogs", handlers.PublicGetBlogs)
	app.Get("/api/v1/public/:subdomain/blogs/:slug", handlers.PublicGetBlog)

	// vOffice oform templates API (public, no auth required)
	// Compatible with OnlyOffice oforms API format for paneltemplates.js
	// IMPORTANT: Must be BEFORE cmsRoutes group (which uses AuthMiddleware on all non-/api/ paths)
	app.Get("/dashboard/api/oforms", handlers.GetOformTemplates)

	// 訂閱頁面（需要認證，但允許試用期過期的用戶訪問）
	app.Get("/subscription-required", middleware.AuthMiddleware, func(c *fiber.Ctx) error {
		return c.Render("pages/subscription_required", fiber.Map{
			"Title": "訂閱方案",
		})
	})

	// 建立租戶後立即訂閱頁面（需要認證，但在 CMS 路由之前，避免 TrialExpiredMiddleware）
	app.Get("/subscribe-now", middleware.AuthMiddleware, func(c *fiber.Ctx) error {
		return c.Render("pages/subscribe_now", fiber.Map{
			"Title": "選擇訂閱方案",
		})
	})

	// CMS 頁面路由（需要認證）
	cmsRoutes := app.Group("", middleware.AuthMiddleware)

	// 試用期檢查中間件（在所有 CMS 路由之前，但訂閱頁面除外）
	cmsRoutes.Use(middleware.TrialExpiredMiddleware)

	// 創建一個中間件來為所有 CMS 頁面添加公司名稱、租戶子域名和基礎域名
	// 並檢查用戶是否有租戶（除了 setup-tenant 頁面）
	cmsRoutes.Use(func(c *fiber.Ctx) error {
		// 跳過所有 API 路由，讓 API 路由組處理
		if strings.HasPrefix(c.Path(), "/api/") {
			return c.Next()
		}

		c.Locals("CompanyName", cfg.CompanyName)
		c.Locals("BaseDomain", cfg.Domain.BaseDomain)
		c.Locals("GoogleAdSensePublisherID", cfg.GoogleAdSensePublisherID)

		// 獲取當前租戶的子域名（用於公司頁面鏈接）
		tenantID := middleware.GetTenantID(c)
		userID := middleware.GetUserID(c)

		// 如果訪問的不是 setup-tenant、profile-guide 和 subscribe-now 頁面，且用戶沒有租戶，重定向到設置頁面
		// vAi 產品（/vai-* 路徑）不需要租戶即可使用，跳過強制設置
		if c.Path() != "/setup-tenant" && c.Path() != "/profile-guide" && c.Path() != "/subscribe-now" && !strings.HasPrefix(c.Path(), "/vai-") && tenantID == uuid.Nil && userID != uuid.Nil {
			// Check if user truly has no tenants (using UserTenant table)
			var tenantCount int64
			if err := database.DB.Model(&models.UserTenant{}).Where("user_id = ?", userID).Count(&tenantCount).Error; err == nil {
				if tenantCount == 0 {
					target := "/setup-tenant"
					// Preserve product context from path
					if c.Path() == "/vmarket-search" || strings.Contains(c.Path(), "vmarket") {
						target += "?product=vmarket"
					} else if c.Path() == "/voffice-download" || strings.Contains(c.Path(), "voffice") {
						target += "?product=voffice"
					}
					return c.Redirect(target)
				}
			}
		}

		if tenantID != uuid.Nil {
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				c.Locals("TenantSubdomain", tenant.Subdomain)
				// Extract sales partner code from tenant extra fields for frontend use
				if tenant.ExtraFields != nil {
					if spCode, ok := tenant.ExtraFields["sales_partner_code"]; ok {
						if codeStr, ok := spCode.(string); ok && codeStr != "" {
							c.Locals("SalesPartnerCode", codeStr)
						}
					}
				}
			}
		}

		return c.Next()
	})

	// 個人資料引導頁面（需要認證）
	cmsRoutes.Get("/profile-guide", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}
		return c.Render("pages/profile_guide", fiber.Map{
			"PageName":    "profile_guide",
			"Title":       "完善個人資料",
			"CompanyName": companyName,
			"BaseDomain":  baseDomain,
		}, "layouts/guide_layout")
	})

	// 租戶設置頁面（需要認證，但用戶沒有租戶時必須訪問）
	cmsRoutes.Get("/setup-tenant", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}
		return c.Render("pages/setup_tenant", fiber.Map{
			"PageName":    "setup_tenant",
			"Title":       "建立公司",
			"CompanyName": companyName,
			"BaseDomain":  baseDomain,
		}, "layouts/guide_layout")
	})

	// 行業模板選擇頁面（需要認證，但首次登錄時可選）
	cmsRoutes.Get("/industry-template-selector", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}
		return c.Render("pages/industry_template_selector", fiber.Map{
			"PageName":        "industry_template_selector",
			"Title":           "選擇行業模板",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
			"BaseDomain":      baseDomain,
		}, "layouts/guide_layout")
	})

	cmsRoutes.Get("/dashboard", func(c *fiber.Ctx) error {
		userID := middleware.GetUserID(c)
		tenantID := middleware.GetTenantID(c)

		// 檢查個人資料是否完整（如果未跳過）
		if userID != uuid.Nil {
			var user models.User
			if err := database.DB.Where("id = ?", userID).First(&user).Error; err == nil {
				isProfileComplete := strings.TrimSpace(user.Name) != "" && strings.TrimSpace(user.Email) != ""
				profileGuideSkipped := false
				if user.ExtraFields != nil {
					if skipped, ok := user.ExtraFields["profile_guide_skipped"].(bool); ok {
						profileGuideSkipped = skipped
					}
				}

				// 如果個人資料不完整且未跳過，重定向到個人資料頁面
				if !isProfileComplete && !profileGuideSkipped {
					return c.Redirect("/profile-guide")
				}
			}
		}

		// 檢查租戶是否已選擇行業模板（首次登錄時引導）
		shouldRedirect := false
		if tenantID != uuid.Nil {
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				// 如果沒有選擇行業模板，且不是從選擇頁面跳過來的，重定向到選擇頁面
				if tenant.IndustryTemplateID == nil {
					// 檢查是否是從選擇頁面跳過來的（通過 referer 或 query 參數）
					referer := c.Get("Referer")
					skipParam := c.Query("skip_template")
					if !strings.Contains(referer, "industry-template-selector") && skipParam != "true" {
						shouldRedirect = true
					}
				}
			}
		}

		if shouldRedirect {
			return c.Redirect("/industry-template-selector")
		}

		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/dashboard", fiber.Map{
			"Title":       "儀表板",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	// vAI 全頁聊天（登入後）
	cmsRoutes.Get("/vai-chat", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/vai_chat", fiber.Map{
			"Title":       "vAI Chat",
			"PageName":    "vai-chat",
			"CompanyName": companyName,
		}, "layouts/vai_cms_layout")
	})

	// vAI 草圖工具（登入後）
	cmsRoutes.Get("/vai-sketch", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/vai_sketch", fiber.Map{
			"Title":       "vAI Sketch",
			"PageName":    "vai-sketch",
			"CompanyName": companyName,
		}, "layouts/vai_cms_layout")
	})

	// vAI 影片生成工具（登入後）
	cmsRoutes.Get("/vai-video", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		// 取得租戶方案，供前端判斷是否有 vAi Video 權限
		tenantPlan := ""
		hasVideoGen := false
		tid := middleware.GetTenantID(c)
		if tid != uuid.Nil {
			var t models.Tenant
			if err := database.DB.Where("id = ?", tid).First(&t).Error; err == nil {
				tenantPlan = t.Plan
				hasVideoGen = models.PlanHasVideoGen(t.Plan)
			}
		}
		return c.Render("pages/vai_video", fiber.Map{
			"Title":        "vAI Video",
			"PageName":     "vai-video",
			"CompanyName":  companyName,
			"TenantPlan":   tenantPlan,
			"HasVideoGen":  hasVideoGen,
			"RequiredPlan": models.RequiredPlanForVideo(),
		}, "layouts/vai_cms_layout")
	})

	// vAI 文件生成工具（登入後）
	cmsRoutes.Get("/vai-docs", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/vai_docs", fiber.Map{
			"Title":       "vAI Docs",
			"PageName":    "vai-docs",
			"CompanyName": companyName,
		}, "layouts/vai_cms_layout")
	})

	// vAI 應用中心（登入後）
	cmsRoutes.Get("/vai-apps", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/vai_apps", fiber.Map{
			"Title":       "vAI Apps",
			"PageName":    "vai-apps",
			"CompanyName": companyName,
		}, "layouts/vai_cms_layout")
	})

	// vAI 費用中心（使用 vai plain layout，保持 vAI topnav，沒有 icon sidebar）
	cmsRoutes.Get("/vai-billing", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/billing", fiber.Map{
			"Title":       "費用中心",
			"PageName":    "vai-billing",
			"CompanyName": companyName,
		}, "layouts/vai_plain_layout")
	})

	// vAI vCoin（使用 vai plain layout，保持 vAI topnav，沒有 icon sidebar）
	cmsRoutes.Get("/vai-vcoins", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/ai_coins", fiber.Map{
			"Title":       "vCoin",
			"PageName":    "vai-vcoins",
			"CompanyName": companyName,
		}, "layouts/vai_plain_layout")
	})

	// vAI 帳戶設置（使用 vai plain layout，保持 vAI topnav，沒有 icon sidebar）
	cmsRoutes.Get("/vai-personal-data", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/personal_data", fiber.Map{
			"Title":       "帳戶設置",
			"PageName":    "vai-personal-data",
			"CompanyName": companyName,
		}, "layouts/vai_plain_layout")
	})

	// vOffice 軟件下載頁（登入後）
	cmsRoutes.Get("/voffice-download", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/voffice_download", fiber.Map{
			"Title":       "軟件下載",
			"PageName":    "voffice-download",
			"CompanyName": companyName,
		}, "layouts/voffice_cms_layout")
	})

	// vOffice 費用中心（使用 voffice layout，保持 vOffice topnav）
	cmsRoutes.Get("/voffice-billing", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/billing", fiber.Map{
			"Title":       "費用中心",
			"PageName":    "voffice-billing",
			"CompanyName": companyName,
		}, "layouts/voffice_cms_layout")
	})

	// vOffice vCoin（使用 voffice layout，保持 vOffice topnav）
	cmsRoutes.Get("/voffice-vcoins", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/ai_coins", fiber.Map{
			"Title":       "vCoin",
			"PageName":    "voffice-vcoins",
			"CompanyName": companyName,
		}, "layouts/voffice_cms_layout")
	})

	// vOffice 帳戶設置（使用 voffice layout，保持 vOffice topnav）
	cmsRoutes.Get("/voffice-personal-data", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/personal_data", fiber.Map{
			"Title":       "帳戶設置",
			"PageName":    "voffice-personal-data",
			"CompanyName": companyName,
		}, "layouts/voffice_cms_layout")
	})

	// vMarket 搜尋頁已停用
	cmsRoutes.Get("/vmarket-search", func(c *fiber.Ctx) error {
		return c.Redirect("/", fiber.StatusMovedPermanently)
	})

	// vMarket 費用中心已停用
	cmsRoutes.Get("/vmarket-billing", func(c *fiber.Ctx) error {
		return c.Redirect("/billing", fiber.StatusMovedPermanently)
	})

	// vMarket vCoin 已停用
	cmsRoutes.Get("/vmarket-vcoins", func(c *fiber.Ctx) error {
		return c.Redirect("/vcoins", fiber.StatusMovedPermanently)
	})

	// vMarket 帳戶設置已停用
	cmsRoutes.Get("/vmarket-personal-data", func(c *fiber.Ctx) error {
		return c.Redirect("/personal-data", fiber.StatusMovedPermanently)
	})

	// CMS 全站搜尋頁
	cmsRoutes.Get("/cms-search", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/cms_search", fiber.Map{
			"Title":       "全站搜尋",
			"PageName":    "cms-search",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	// 倉庫管理 UI 路由（移到前面確保優先匹配）
	cmsRoutes.Get("/warehouses", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		log.Printf("✅ /warehouses route hit - Path: %s, Method: %s", c.Path(), c.Method())
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":           "倉庫管理",
			"PageName":        "warehouses",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/warehouses/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增倉庫",
			"PageName": "warehouses",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/warehouses/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯倉庫",
			"PageName": "warehouses",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 店舖管理 UI 路由
	cmsRoutes.Get("/stores", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":           "店舖管理",
			"PageName":        "stores",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/stores/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增店舖",
			"PageName": "stores",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/stores/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯店舖",
			"PageName": "stores",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 餐飲桌區管理 UI 路由
	cmsRoutes.Get("/dining-areas", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":           "餐桌區管理",
			"PageName":        "dining-areas",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/dining-areas/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增餐桌區",
			"PageName": "dining-areas",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/dining-areas/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯餐桌區",
			"PageName": "dining-areas",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 餐桌管理 UI 路由
	cmsRoutes.Get("/dining-tables", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":           "餐桌管理",
			"PageName":        "dining-tables",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/dining-tables/board", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/dining_table_board", fiber.Map{
			"Title":           "餐桌管理",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/dining-tables/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增餐桌",
			"PageName": "dining-tables",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/dining-tables/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯餐桌",
			"PageName": "dining-tables",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 餐飲排隊管理 UI 路由
	cmsRoutes.Get("/dining-queues", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":           "候位排隊",
			"PageName":        "dining-queues",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/dining-queue/board", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		// 獲取餐飲設定
		requirePhoneStr := strings.TrimSpace(models.GetSystemSetting("dining_queue_require_phone", "false"))
		requirePhone := strings.EqualFold(requirePhoneStr, "true") || requirePhoneStr == "1"
		return c.Render("pages/dining_queue_board", fiber.Map{
			"Title":             "候位看板",
			"CompanyName":       companyName,
			"TenantSubdomain":   tenantSubdomain,
			"Subdomain":         tenantSubdomain,
			"StoreID":           strings.TrimSpace(c.Query("store_id")),
			"StoreName":         companyName, // 使用公司名稱作為店名
			"UseKioskNav":       true,
			"HideSidebar":       true,
			"HideTopnav":        false,
			"QueueRequirePhone": requirePhone,
		}, "layouts/cms_layout")
	})
	// 餐飲設定 UI
	cmsRoutes.Get("/dining-settings", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/dining_settings", fiber.Map{
			"Title":           "餐飲設定",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})

	// 餐飲流程指南
	cmsRoutes.Get("/guide/dining-flow", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/guide_dining_flow", fiber.Map{
			"PageName":        "guide_dining_flow",
			"Title":           "餐飲流程指南",
			"CompanyName":     companyName,
			"BaseDomain":      baseDomain,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})

	// 訂單流程指南
	cmsRoutes.Get("/guide/order-flow", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/guide_order_flow", fiber.Map{
			"PageName":        "guide_order_flow",
			"Title":           "訂單流程指南",
			"CompanyName":     companyName,
			"BaseDomain":      baseDomain,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/dining-queues/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增候位",
			"PageName": "dining-queues",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/dining-queues/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯候位",
			"PageName": "dining-queues",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 支出申請 UI 路由（移到前面確保優先匹配）
	cmsRoutes.Get("/expense-requests", func(c *fiber.Ctx) error {
		log.Printf("✅ /expense-requests route hit - Path: %s, Method: %s", c.Path(), c.Method())
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "支出申請",
			"PageName": "expense-requests",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/expense-requests/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增支出申請",
			"PageName": "expense-requests",
		}, "layouts/cms_layout")
	})

	// 動態列表和表單路由（使用配置驅動）
	cmsRoutes.Get("/customers", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":           "客戶管理",
			"PageName":        "customers",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/customers/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增客戶",
			"PageName": "customers",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/customers/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯客戶",
			"PageName": "customers",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/customer-labels", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "客戶標籤",
			"PageName": "customer-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/customer-labels/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增客戶標籤",
			"PageName": "customer-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/customer-labels/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯客戶標籤",
			"PageName": "customer-labels",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 客戶/項目分析報告
	cmsRoutes.Get("/customer-analysis-report", func(c *fiber.Ctx) error {
		return c.Render("pages/customer_analysis_report", fiber.Map{
			"Title":    "客戶分析報告",
			"PageName": "customer-analysis-report",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/project-analysis-report", func(c *fiber.Ctx) error {
		return c.Render("pages/project_analysis_report", fiber.Map{
			"Title":    "項目分析報告",
			"PageName": "project-analysis-report",
		}, "layouts/cms_layout")
	})

	// 自動搵客系統（Lead Finder）
	cmsRoutes.Get("/lead-finder", func(c *fiber.Ctx) error {
		// 取得租戶方案，供前端判斷是否有自動搵客權限
		hasLeadFinder := false
		tid := middleware.GetTenantID(c)
		if tid != uuid.Nil {
			var t models.Tenant
			if err := database.DB.Where("id = ?", tid).First(&t).Error; err == nil {
				hasLeadFinder = models.PlanHasLeadFinder(t.Plan)
			}
		}
		return c.Render("pages/lead_finder", fiber.Map{
			"Title":         "搵客工具",
			"PageName":      "lead-finder",
			"HasLeadFinder": hasLeadFinder,
			"RequiredPlan":  models.RequiredPlanForLeadFinder(),
		}, "layouts/cms_layout")
	})
	// Lead 列表（所有搜尋結果匯總）— 必須在 /lead-finder/results/:id 之前註冊
	cmsRoutes.Get("/lead-finder/results", func(c *fiber.Ctx) error {
		hasLeadFinder := false
		tid := middleware.GetTenantID(c)
		if tid != uuid.Nil {
			var t models.Tenant
			if err := database.DB.Where("id = ?", tid).First(&t).Error; err == nil {
				hasLeadFinder = models.PlanHasLeadFinder(t.Plan)
			}
		}
		return c.Render("pages/lead_finder_list", fiber.Map{
			"Title":         "Lead 列表",
			"PageName":      "lead-list",
			"HasLeadFinder": hasLeadFinder,
			"RequiredPlan":  models.RequiredPlanForLeadFinder(),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/lead-finder/results/:id", func(c *fiber.Ctx) error {
		hasLeadFinder := false
		tid := middleware.GetTenantID(c)
		if tid != uuid.Nil {
			var t models.Tenant
			if err := database.DB.Where("id = ?", tid).First(&t).Error; err == nil {
				hasLeadFinder = models.PlanHasLeadFinder(t.Plan)
			}
		}
		return c.Render("pages/lead_finder_results", fiber.Map{
			"Title":         "搵客結果",
			"PageName":      "lead-finder",
			"SearchID":      c.Params("id"),
			"HasLeadFinder": hasLeadFinder,
			"RequiredPlan":  models.RequiredPlanForLeadFinder(),
		}, "layouts/cms_layout")
	})

	// 自動搵客
	cmsRoutes.Get("/auto-outreach", func(c *fiber.Ctx) error {
		return c.Render("pages/auto_outreach", fiber.Map{
			"Title":    "自動搵客",
			"PageName": "auto-outreach",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/auto-outreach/new", func(c *fiber.Ctx) error {
		return c.Render("pages/auto_outreach", fiber.Map{
			"Title":    "自動搵客",
			"PageName": "auto-outreach",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/auto-outreach/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/auto_outreach", fiber.Map{
			"Title":    "自動搵客",
			"PageName": "auto-outreach",
		}, "layouts/cms_layout")
	})

	// 電話區號管理
	cmsRoutes.Get("/phone-country-codes", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "電話區號",
			"PageName": "phone-country-codes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/phone-country-codes/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增電話區號",
			"PageName": "phone-country-codes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/phone-country-codes/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯電話區號",
			"PageName": "phone-country-codes",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 供應商 UI 路由
	cmsRoutes.Get("/suppliers", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "供應商管理",
			"PageName": "suppliers",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/suppliers/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增供應商",
			"PageName": "suppliers",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/suppliers/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯供應商",
			"PageName": "suppliers",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/products", func(c *fiber.Ctx) error {
		// On vmarketai.com domain, serve VMarket products page (public, no auth needed for this path)
		if handlers.IsVMarketDomain(c) {
			return handlers.RenderVMarketProducts(c)
		}
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "產品管理",
			"PageName": "products",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/products/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增產品",
			"PageName": "products",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/products/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯產品",
			"PageName": "products",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 產品稅 UI 路由
	cmsRoutes.Get("/product-taxes", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "產品稅",
			"PageName": "product-taxes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/product-taxes/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增產品稅",
			"PageName": "product-taxes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/product-taxes/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯產品稅",
			"PageName": "product-taxes",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/orders", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "訂單管理",
			"PageName": "orders",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/orders/new", func(c *fiber.Ctx) error {
		return c.Render("pages/orders_new", fiber.Map{
			"Title": "新增訂單",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/orders/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/orders_new", fiber.Map{
			"Title": "編輯訂單",
			"ID":    c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/quotations", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "報價單管理",
			"PageName": "quotations",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/quotations/new", func(c *fiber.Ctx) error {
		return c.Render("pages/quotations_new", fiber.Map{
			"Title": "新增報價單",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/quotations/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/quotations_new", fiber.Map{
			"Title": "編輯報價單",
			"ID":    c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/order-labels", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "訂單標籤管理",
			"PageName": "order-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/order-labels/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增訂單標籤",
			"PageName": "order-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/order-labels/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯訂單標籤",
			"PageName": "order-labels",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/purchase-order-labels", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "採購標籤管理",
			"PageName": "purchase-order-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/purchase-order-labels/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增採購標籤",
			"PageName": "purchase-order-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/purchase-order-labels/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯採購標籤",
			"PageName": "purchase-order-labels",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/service-order-labels", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "服務單標籤管理",
			"PageName": "service-order-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-order-labels/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增服務單標籤",
			"PageName": "service-order-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-order-labels/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯服務單標籤",
			"PageName": "service-order-labels",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 出租單標籤 CMS 頁面
	cmsRoutes.Get("/rental-order-labels", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "出租單標籤管理",
			"PageName": "rental-order-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/rental-order-labels/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增出租單標籤",
			"PageName": "rental-order-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/rental-order-labels/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯出租單標籤",
			"PageName": "rental-order-labels",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/product-labels", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "產品標籤管理",
			"PageName": "product-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/product-labels/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增產品標籤",
			"PageName": "product-labels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/product-labels/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯產品標籤",
			"PageName": "product-labels",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/invoices", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "發票管理",
			"PageName": "invoices",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/invoices/new", func(c *fiber.Ctx) error {
		return c.Render("pages/invoices_new", fiber.Map{
			"Title": "新增發票",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/invoices/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/invoices_new", fiber.Map{
			"Title": "編輯發票",
			"ID":    c.Params("id"),
		}, "layouts/cms_layout")
	})

	// ============================================
	// 設定模組頁面路由
	// ============================================

	cmsRoutes.Get("/enterprises", func(c *fiber.Ctx) error {
		return c.Render("pages/enterprises", fiber.Map{"Title": "企業設置"}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/google-ads", func(c *fiber.Ctx) error {
		return c.Render("pages/google_ads", fiber.Map{"Title": "Google 廣告"}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/document-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/document_settings", fiber.Map{"Title": "文件設定"}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/document-auto-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/document_auto_settings", fiber.Map{"Title": "單據設定"}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/notification-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/notification_settings", fiber.Map{
			"Title":    "系統提示設定",
			"PageName": "notification-settings",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/api-tokens", func(c *fiber.Ctx) error {
		return c.Render("pages/api_tokens", fiber.Map{
			"Title":    "API Tokens",
			"PageName": "api-tokens",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/api-tokens/create", func(c *fiber.Ctx) error {
		return c.Render("pages/api_token_create", fiber.Map{
			"Title":    "Create API Token",
			"PageName": "api-tokens",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/shipping-integrations", func(c *fiber.Ctx) error {
		return c.Render("pages/shipping_integrations", fiber.Map{
			"Title":    "配送連接設定",
			"PageName": "shipping-integrations",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/delivery-integrations", func(c *fiber.Ctx) error {
		return c.Render("pages/delivery_integrations", fiber.Map{
			"Title":    "外賣平台整合",
			"PageName": "delivery-integrations",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/delivery-orders", func(c *fiber.Ctx) error {
		return c.Render("pages/delivery_orders", fiber.Map{
			"Title":    "外賣訂單",
			"PageName": "delivery-orders",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/delivery-product-mappings", func(c *fiber.Ctx) error {
		return c.Render("pages/delivery_product_mappings", fiber.Map{
			"Title":    "外賣產品映射",
			"PageName": "delivery-product-mappings",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/departments", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "部門管理",
			"PageName": "departments",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/departments/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增部門",
			"PageName": "departments",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/departments/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯部門",
			"PageName": "departments",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/roles", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "角色管理",
			"PageName": "roles",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/roles/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增角色",
			"PageName": "roles",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/roles/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯角色",
			"PageName": "roles",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// regions 頁面暫時隱藏

	cmsRoutes.Get("/currencies", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "貨幣管理",
			"PageName": "currencies",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/currencies/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增貨幣",
			"PageName": "currencies",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/currencies/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯貨幣",
			"PageName": "currencies",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// ============================================
	// 個人模組頁面路由
	// ============================================

	cmsRoutes.Get("/calendars", func(c *fiber.Ctx) error {
		return c.Render("pages/calendars", fiber.Map{
			"Title": "日曆管理",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/calendars/new", func(c *fiber.Ctx) error {
		// 根據類別參數跳轉到不同頁面
		category := c.Query("category", "")
		if category == "holiday" {
			return c.Redirect("/holidays/new")
		}
		// 其他類別使用日曆表單
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增日曆",
			"PageName": "calendars",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/calendars/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯日曆",
			"PageName": "calendars",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/reminders", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "提示管理",
			"PageName": "reminders",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/reminders/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增提示",
			"PageName": "reminders",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/reminders/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯提示",
			"PageName": "reminders",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/messages", func(c *fiber.Ctx) error {
		return c.Render("pages/messages", fiber.Map{
			"Title": "訊息",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/notifications", func(c *fiber.Ctx) error {
		return c.Render("pages/notifications", fiber.Map{
			"Title": "提示資訊",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/activity-logs", handlers.ActivityLogsPage)

	cmsRoutes.Get("/notes", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "備忘管理",
			"PageName": "notes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/notes/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增備忘",
			"PageName": "notes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/notes/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯備忘",
			"PageName": "notes",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// ============================================
	// 客戶擴展模組頁面路由
	// ============================================

	cmsRoutes.Get("/member-levels", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "會員等級管理",
			"PageName": "member-levels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/member-levels/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增會員等級",
			"PageName": "member-levels",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/member-levels/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯會員等級",
			"PageName": "member-levels",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/points-history", func(c *fiber.Ctx) error {
		return c.Render("pages/points_history", fiber.Map{
			"Title":    "積分記錄",
			"PageName": "points-history",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/points", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "積分管理",
			"PageName": "points",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/points/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增積分",
			"PageName": "points",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/points/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯積分",
			"PageName": "points",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/referrals", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "介紹記錄",
			"PageName": "referrals",
		}, "layouts/cms_layout")
	})

	// 優惠券 UI
	cmsRoutes.Get("/coupons", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "優惠券管理",
			"PageName": "coupons",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/coupons/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增優惠券",
			"PageName": "coupons",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/coupons/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯優惠券",
			"PageName": "coupons",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 積分設置 UI
	cmsRoutes.Get("/point-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/point_settings", fiber.Map{
			"Title": "積分設置",
		}, "layouts/cms_layout")
	})

	// 印花設定 UI
	cmsRoutes.Get("/stamp-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "印花設定",
			"PageName": "stamp-settings",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/stamp-settings/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增印花設定",
			"PageName": "stamp-settings",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/stamp-settings/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯印花設定",
			"PageName": "stamp-settings",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 印花記錄 UI
	cmsRoutes.Get("/stamp-records", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "印花記錄",
			"PageName": "stamp-records",
		}, "layouts/cms_layout")
	})

	// ============================================
	// 產品擴展模組頁面路由
	// ============================================

	cmsRoutes.Get("/product-types", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "產品類型管理",
			"PageName": "product_types",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/product-types/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增產品類型",
			"PageName": "product_types",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/product-types/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯產品類型",
			"PageName": "product_types",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/product-attributes", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "產品屬性管理",
			"PageName": "product_attributes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/product-attributes/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增產品屬性",
			"PageName": "product_attributes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/product-attributes/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯產品屬性",
			"PageName": "product_attributes",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/brands", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "品牌管理",
			"PageName": "brands",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/brands/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增品牌",
			"PageName": "brands",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/brands/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯品牌",
			"PageName": "brands",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// ============================================
	// 服務模組頁面路由
	// ============================================

	cmsRoutes.Get("/service-types", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "服務種類管理",
			"PageName": "service_types",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-types/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增服務種類",
			"PageName": "service_types",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-types/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯服務種類",
			"PageName": "service_types",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/services", func(c *fiber.Ctx) error {
		// On vmarketai.com domain, serve VMarket services page (public, no auth needed for this path)
		if handlers.IsVMarketDomain(c) {
			return handlers.RenderVMarketServices(c)
		}
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "服務管理",
			"PageName": "services",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/services/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增服務",
			"PageName": "services",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/services/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯服務",
			"PageName": "services",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 服務稅 UI 路由
	cmsRoutes.Get("/service-taxes", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "服務稅",
			"PageName": "service-taxes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-taxes/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增服務稅",
			"PageName": "service-taxes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-taxes/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯服務稅",
			"PageName": "service-taxes",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 服務設定 UI
	cmsRoutes.Get("/service-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/service_settings", fiber.Map{
			"Title": "服務設定",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/appointments", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "預約管理",
			"PageName": "appointments",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/appointments/new", func(c *fiber.Ctx) error {
		// 檢查是否啟用友善預約模式
		tenantID := middleware.GetTenantID(c)
		useFriendlyBooking := true
		if tenantID != uuid.Nil {
			var tenant models.Tenant
			if err := database.DB.Select("extra_fields").Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				if raw, ok := tenant.ExtraFields["service_friendly_booking"]; ok {
					switch v := raw.(type) {
					case bool:
						useFriendlyBooking = v
					case string:
						val := strings.ToLower(strings.TrimSpace(v))
						useFriendlyBooking = val == "true" || val == "1" || val == "on" || val == "yes"
					case float64:
						useFriendlyBooking = int(v) == 1
					case int:
						useFriendlyBooking = v == 1
					}
				}
			}
		}
		if useFriendlyBooking {
			return c.Render("pages/appointments_new", fiber.Map{
				"Title": "新增預約",
			}, "layouts/cms_layout")
		}
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增預約",
			"PageName": "appointments",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/appointments/:id/edit", func(c *fiber.Ctx) error {
		// 檢查是否啟用友善預約模式
		tenantID := middleware.GetTenantID(c)
		useFriendlyBooking := true
		if tenantID != uuid.Nil {
			var tenant models.Tenant
			if err := database.DB.Select("extra_fields").Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				if raw, ok := tenant.ExtraFields["service_friendly_booking"]; ok {
					switch v := raw.(type) {
					case bool:
						useFriendlyBooking = v
					case string:
						val := strings.ToLower(strings.TrimSpace(v))
						useFriendlyBooking = val == "true" || val == "1" || val == "on" || val == "yes"
					case float64:
						useFriendlyBooking = int(v) == 1
					case int:
						useFriendlyBooking = v == 1
					}
				}
			}
		}
		if useFriendlyBooking {
			return c.Render("pages/appointments_new", fiber.Map{
				"Title": "編輯預約",
				"ID":    c.Params("id"),
			}, "layouts/cms_layout")
		}
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯預約",
			"PageName": "appointments",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/service-orders", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "服務單管理",
			"PageName": "service-orders",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-orders/new", func(c *fiber.Ctx) error {
		return c.Render("pages/service_orders_new", fiber.Map{
			"Title": "新增服務單",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-orders/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/service_orders_new", fiber.Map{
			"Title": "編輯服務單",
			"ID":    c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 出租單 CMS 頁面
	cmsRoutes.Get("/rental-orders", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "出租單管理",
			"PageName": "rental-orders",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/rental-orders/new", func(c *fiber.Ctx) error {
		return c.Render("pages/rental_orders_new", fiber.Map{
			"Title": "新增出租單",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/rental-orders/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/rental_orders_new", fiber.Map{
			"Title": "編輯出租單",
			"ID":    c.Params("id"),
		}, "layouts/cms_layout")
	})

	// ============================================
	// 其他模組頁面路由
	// ============================================

	cmsRoutes.Get("/purchase-orders", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "採購單管理",
			"PageName": "purchase_orders",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/purchase-orders/new", func(c *fiber.Ctx) error {
		return c.Render("pages/purchase_orders_new", fiber.Map{
			"Title": "新增採購單",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/purchase-orders/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/purchase_orders_new", fiber.Map{
			"Title": "編輯採購單",
			"ID":    c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/support-communications", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "客服通訊管理",
			"PageName": "support-communications",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/support-communications/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增客服通訊",
			"PageName": "support-communications",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/support-communications/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯客服通訊",
			"PageName": "support-communications",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/promotions", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "推廣發送管理",
			"PageName": "promotions",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/promotions/new", func(c *fiber.Ctx) error {
		return c.Render("pages/promotions_new", fiber.Map{
			"Title":    "新增推廣發送",
			"PageName": "promotions",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/promotions/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯推廣發送",
			"PageName": "promotions",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	app.Get("/google-ads/oauth/callback", handlers.GoogleAdsOAuthCallback)

	// 訂單設置頁面（確保在 promotions 之後，避免路由衝突）
	cmsRoutes.Get("/payment-methods", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "付款方式管理",
			"PageName": "payment-methods",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/payment-methods/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增付款方式",
			"PageName": "payment-methods",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/payment-methods/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯付款方式",
			"PageName": "payment-methods",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/stripe-connect", func(c *fiber.Ctx) error {
		return c.Render("pages/stripe_connect", fiber.Map{
			"Title": "Stripe Connect",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/shipping-methods", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "運送方式管理",
			"PageName": "shipping-methods",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/shipping-methods/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增運送方式",
			"PageName": "shipping-methods",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/shipping-methods/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯運送方式",
			"PageName": "shipping-methods",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/logistics-companies", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "物流公司管理",
			"PageName": "logistics-companies",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/logistics-companies/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增物流公司",
			"PageName": "logistics-companies",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/logistics-companies/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯物流公司",
			"PageName": "logistics-companies",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 配送管理頁面
	cmsRoutes.Get("/shipments", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "配送管理",
			"PageName": "shipments",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/shipments/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增配送",
			"PageName": "shipments",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/shipments/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯配送",
			"PageName": "shipments",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 銀行賬戶管理頁面
	cmsRoutes.Get("/bank-accounts", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "銀行賬戶管理",
			"PageName": "bank-accounts",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/bank-accounts/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增銀行賬戶",
			"PageName": "bank-accounts",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/bank-accounts/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯銀行賬戶",
			"PageName": "bank-accounts",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/bank-accounts/stats", func(c *fiber.Ctx) error {
		return c.Render("pages/bank_accounts_stats", fiber.Map{
			"Title": "銀行賬戶統計",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/pos", func(c *fiber.Ctx) error {
		return c.Render("pages/pos", fiber.Map{"Title": "POS 收銀台"}, "layouts/cms_layout")
	})

	// 會計模組 UI
	cmsRoutes.Get("/accounting", func(c *fiber.Ctx) error {
		return c.Render("pages/accounting", fiber.Map{"Title": "會計總覽"}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/accounting/reports", func(c *fiber.Ctx) error {
		return c.Render("pages/accounting_reports", fiber.Map{"Title": "專業會計報表"}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/accounts", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "會計科目表",
			"PageName": "accounts",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/accounts/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增會計科目",
			"PageName": "accounts",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/accounts/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯會計科目",
			"PageName": "accounts",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/journal-entries", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "日記帳",
			"PageName": "journal-entries",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/journal-entries/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增日記帳分錄",
			"PageName": "journal-entries",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/journal-entries/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯日記帳分錄",
			"PageName": "journal-entries",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/tax-configs", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "稅務配置",
			"PageName": "tax-configs",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/tax-configs/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增稅務配置",
			"PageName": "tax-configs",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/tax-configs/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯稅務配置",
			"PageName": "tax-configs",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/posting-rules", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "自動過帳規則",
			"PageName": "posting-rules",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/accounting/posting-rules", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "自動過帳規則",
			"PageName": "posting-rules",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/posting-rules/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增自動過帳規則",
			"PageName": "posting-rules",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/posting-rules/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯自動過帳規則",
			"PageName": "posting-rules",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/incomes", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "收入管理",
			"PageName": "incomes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/incomes/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增收入",
			"PageName": "incomes",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/incomes/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯收入",
			"PageName": "incomes",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/expenses", func(c *fiber.Ctx) error {
		return c.Render("pages/expenses", fiber.Map{
			"Title":    "支出管理",
			"PageName": "expenses",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/expenses/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增支出",
			"PageName": "expenses",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/expenses/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯支出",
			"PageName": "expenses",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// ============================================
	// 庫存管理 UI 路由
	// ============================================

	// 出入貨記錄（顯示所有出/入貨單的所有細項）
	cmsRoutes.Get("/inventory-movements", func(c *fiber.Ctx) error {
		return c.Render("pages/inventory_movements", fiber.Map{
			"Title": "出入貨記錄",
		}, "layouts/cms_layout")
	})

	// 出入庫處理（處理待出庫/入庫的發貨單和收貨單）
	cmsRoutes.Get("/inventory-processing", func(c *fiber.Ctx) error {
		return c.Render("pages/inventory_processing", fiber.Map{
			"Title": "出入庫處理",
		}, "layouts/cms_layout")
	})
	// 出入庫設定頁面
	cmsRoutes.Get("/inventory-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/inventory_settings", fiber.Map{
			"Title": "出入庫設定",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/pos-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/pos_settings", fiber.Map{
			"Title": "POS 設定",
		}, "layouts/cms_layout")
	})
	// POS 設定（embed，用於 POS popup，不要 sidebar/topnav）
	cmsRoutes.Get("/pos-settings/embed", func(c *fiber.Ctx) error {
		return c.Render("pages/pos_settings", fiber.Map{
			"Title": "POS 設定",
			"Embed": true,
		}, "layouts/embed_layout")
	})
	// 打印機設定
	cmsRoutes.Get("/printer-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/printer_settings", fiber.Map{
			"Title": "打印機設定",
		}, "layouts/cms_layout")
	})
	// 卡機設定
	cmsRoutes.Get("/card-terminal-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/card_terminal_settings", fiber.Map{
			"Title": "卡機設定",
		}, "layouts/cms_layout")
	})
	// 候位票打印機設定
	cmsRoutes.Get("/dining-ticket-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/dining_ticket_settings", fiber.Map{
			"Title": "候位票打印機設定",
		}, "layouts/cms_layout")
	})
	// POS 小票打印機設定
	cmsRoutes.Get("/pos-ticket-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/pos_ticket_settings", fiber.Map{
			"Title": "POS 小票打印機設定",
		}, "layouts/cms_layout")
	})
	// 廚房點餐單打印機設定
	cmsRoutes.Get("/diningorder-ticket-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/diningorder_ticket_settings", fiber.Map{
			"Title": "廚房點餐單打印機設定",
		}, "layouts/cms_layout")
	})

	// 庫存盤點（產品和庫存的匯總報告）
	cmsRoutes.Get("/inventory-counts", func(c *fiber.Ctx) error {
		return c.Render("pages/inventory_counts", fiber.Map{
			"Title": "庫存盤點",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/inventory/low-stock", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "低庫存預警",
			"PageName": "low-stock",
		}, "layouts/cms_layout")
	})

	// ============================================
	// 個人資料、服務員、房間設備、訂單報表 UI 路由
	// ============================================

	cmsRoutes.Get("/personal-data", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/personal_data", fiber.Map{
			"Title":       "帳戶設置",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	// 費用中心
	cmsRoutes.Get("/billing", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/billing", fiber.Map{
			"Title":       "費用中心",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	// 手機 App 管理
	cmsRoutes.Get("/app-manager", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/app_manager", fiber.Map{
			"Title":       "手機 App 管理",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	// vCoin
	cmsRoutes.Get("/vcoins", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/ai_coins", fiber.Map{
			"Title":       "vCoin",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	// 購買硬件
	cmsRoutes.Get("/hardware-purchase", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/hardware_purchase", fiber.Map{
			"Title":       "購買硬件",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/service-staffs", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "服務員管理",
			"PageName": "service_staffs",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-staffs/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增服務員",
			"PageName": "service_staffs",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/service-staffs/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯服務員",
			"PageName": "service_staffs",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 房間管理
	cmsRoutes.Get("/rooms", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "房間管理",
			"PageName": "rooms",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/rooms/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增房間",
			"PageName": "rooms",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/rooms/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯房間",
			"PageName": "rooms",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 設備管理
	cmsRoutes.Get("/equipments", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "設備管理",
			"PageName": "equipments",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/equipments/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增設備",
			"PageName": "equipments",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/equipments/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯設備",
			"PageName": "equipments",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/order-reports", func(c *fiber.Ctx) error {
		return c.Render("pages/order_reports", fiber.Map{
			"Title": "訂單報表",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/order-reports/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增訂單報表",
			"PageName": "order_reports",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/order-reports/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯訂單報表",
			"PageName": "order_reports",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 使用者管理
	cmsRoutes.Get("/users", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "使用者管理",
			"PageName": "users",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/users/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增使用者",
			"PageName": "users",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/users/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯使用者",
			"PageName": "users",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 車輛管理
	cmsRoutes.Get("/vehicles", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "車輛管理",
			"PageName": "vehicles",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/vehicles/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增車輛",
			"PageName": "vehicles",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/vehicles/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯車輛",
			"PageName": "vehicles",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 資源使用日曆（整合預約 + 項目資源預留）
	cmsRoutes.Get("/resource-usage-calendar", func(c *fiber.Ctx) error {
		return c.Render("pages/resource_usage_calendar", fiber.Map{
			"Title": "資源使用日曆",
		}, "layouts/cms_layout")
	})

	// 預約日曆（獨立預約日曆，支持服務種類/服務員/客戶篩選）
	cmsRoutes.Get("/appointment-calendar", func(c *fiber.Ctx) error {
		return c.Render("pages/appointment_calendar", fiber.Map{
			"Title": "預約日曆",
		}, "layouts/cms_layout")
	})

	// 業務目標
	cmsRoutes.Get("/business-goals", func(c *fiber.Ctx) error {
		return c.Render("pages/business_goals", fiber.Map{
			"Title": "業務目標",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/business-goals/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增目標",
			"PageName": "business_goals",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/business-goals/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯業務目標",
			"PageName": "business_goals",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 項目管理
	cmsRoutes.Get("/projects", func(c *fiber.Ctx) error {
		return c.Render("pages/projects", fiber.Map{
			"Title": "項目管理",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/projects/new", func(c *fiber.Ctx) error {
		return c.Render("pages/project_form", fiber.Map{
			"Title": "新增項目",
			"ID":    "",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/projects/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/project_form", fiber.Map{
			"Title": "編輯項目",
			"ID":    c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 項目類型（用 dynamic list/form 管理）
	cmsRoutes.Get("/project-types", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "項目類型",
			"PageName": "project_types",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/project-types/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增項目類型",
			"PageName": "project_types",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/project-types/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯項目類型",
			"PageName": "project_types",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 網站主題設置頁面（必須在 /pages/:id/edit 之前，避免路由衝突）
	cmsRoutes.Get("/website-theme", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/website_theme", fiber.Map{
			"Title":       "網站主題",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	// 網站設定頁面
	cmsRoutes.Get("/website-settings", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		return c.Render("pages/website_settings", fiber.Map{
			"Title":       "網站設定",
			"PageName":    "website_theme",
			"CompanyName": companyName,
		}, "layouts/cms_layout")
	})

	// VMarket 設定頁面已停用
	cmsRoutes.Get("/vmarket-settings", func(c *fiber.Ctx) error {
		return c.Redirect("/", fiber.StatusMovedPermanently)
	})

	// 廣告位置管理頁面
	cmsRoutes.Get("/ad-positions", func(c *fiber.Ctx) error {
		return c.Render("pages/ad_positions", fiber.Map{
			"Title":    "廣告位置管理",
			"PageName": "ad-positions",
		}, "layouts/cms_layout")
	})

	// 廣告位置詳情頁面（包含廣告管理和輪播設定）
	cmsRoutes.Get("/ad-positions/:id/ads", func(c *fiber.Ctx) error {
		return c.Render("pages/ad_position_detail", fiber.Map{
			"Title":        "廣告位置詳情",
			"PageName":     "ad-position-detail",
			"AdPositionID": c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 輪播設定頁面（重定向到合併頁面）
	cmsRoutes.Get("/carousel-settings/:id", func(c *fiber.Ctx) error {
		return c.Redirect("/ad-positions/" + c.Params("id") + "/ads")
	})

	// 產品同步設定頁面
	cmsRoutes.Get("/product-sync-settings", func(c *fiber.Ctx) error {
		return c.Render("pages/product_sync_settings", fiber.Map{
			"Title":    "產品同步設定",
			"PageName": "product-sync-settings",
		}, "layouts/cms_layout")
	})

	// 建站完成後的 VMarket 推薦頁已停用
	cmsRoutes.Get("/vmarket-join-recommendation", func(c *fiber.Ctx) error {
		return c.Redirect("/", fiber.StatusMovedPermanently)
	})

	// 自訂網域 & SSL（vBuilder）
	cmsRoutes.Get("/website-domains", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}

		// CNAME target：優先 env CUSTOM_DOMAIN_CNAME_TARGET，否則預設 cname.<baseDomain>
		cnameTarget := strings.TrimSpace(os.Getenv("CUSTOM_DOMAIN_CNAME_TARGET"))
		if cnameTarget == "" && baseDomain != "" {
			cnameTarget = "cname." + baseDomain
		}

		return c.Render("pages/website_domains", fiber.Map{
			"Title":                   "自訂網域 & SSL",
			"CompanyName":             companyName,
			"CustomDomainCnameTarget": cnameTarget,
		}, "layouts/cms_layout")
	})

	// vBuilder：瀏覽報告（每頁瀏覽量）
	cmsRoutes.Get("/website-page-views", func(c *fiber.Ctx) error {
		return c.Render("pages/website_page_views", fiber.Map{
			"Title":    "瀏覽報告",
			"PageName": "website-page-views",
		}, "layouts/cms_layout")
	})

	// 網站設置引導頁面（必須在 /pages/:id/edit 之前，避免路由衝突）
	cmsRoutes.Get("/website-setup-guide", func(c *fiber.Ctx) error {
		companyName := c.Locals("CompanyName").(string)
		tenantSubdomain := ""
		if subdomain, ok := c.Locals("TenantSubdomain").(string); ok {
			tenantSubdomain = subdomain
		}
		baseDomain := ""
		if domain, ok := c.Locals("BaseDomain").(string); ok {
			baseDomain = domain
		}
		return c.Render("pages/website_setup_guide", fiber.Map{
			"Title":           "網站設置",
			"PageName":        "website_setup_guide",
			"CompanyName":     companyName,
			"TenantSubdomain": tenantSubdomain,
			"BaseDomain":      baseDomain,
		}, "layouts/guide_layout")
	})

	// 頁面管理
	cmsRoutes.Get("/pages", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "頁面管理",
			"PageName": "pages",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/pages/new", func(c *fiber.Ctx) error {
		// Resolve website theme for page editor preview
		themeID := "default"
		tenantID := middleware.GetTenantID(c)
		if tenantID != uuid.Nil {
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				if tenant.WebsiteTheme != nil && *tenant.WebsiteTheme != "" {
					themeID = *tenant.WebsiteTheme
				}
			}
		}
		themeCSS := themes.GetThemeCSS(themeID)
		return c.Render("pages/page_editor", fiber.Map{
			"Title":            "新增頁面",
			"ID":               "",
			"ThemeCSS":         themeCSS,
			"ThemeID":          themeID,
			"GoogleMapsAPIKey": cfg.GoogleMapsAPIKey,
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/pages/:id/edit", func(c *fiber.Ctx) error {
		// Resolve website theme for page editor preview
		themeID := "default"
		tenantID := middleware.GetTenantID(c)
		if tenantID != uuid.Nil {
			var tenant models.Tenant
			if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err == nil {
				if tenant.WebsiteTheme != nil && *tenant.WebsiteTheme != "" {
					themeID = *tenant.WebsiteTheme
				}
			}
		}
		themeCSS := themes.GetThemeCSS(themeID)
		return c.Render("pages/page_editor", fiber.Map{
			"Title":            "編輯頁面",
			"ID":               c.Params("id"),
			"ThemeCSS":         themeCSS,
			"ThemeID":          themeID,
			"GoogleMapsAPIKey": cfg.GoogleMapsAPIKey,
		}, "layouts/cms_layout")
	})

	// Blog CMS
	cmsRoutes.Get("/blogs", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "博客管理",
			"PageName": "blogs",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/blogs/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增博客",
			"PageName": "blogs",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/blogs/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯博客",
			"PageName": "blogs",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// ============================================
	// HR 模組 UI 路由
	// ============================================

	cmsRoutes.Get("/attendances", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "打卡記錄",
			"PageName": "attendances",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/attendances/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增打卡記錄",
			"PageName": "attendances",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/attendances/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯打卡記錄",
			"PageName": "attendances",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 打卡報告（共用打卡記錄的列表）
	cmsRoutes.Get("/attendance-reports", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "打卡報告",
			"PageName": "attendance-reports",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/attendance/clock", func(c *fiber.Ctx) error {
		return c.Render("pages/attendance_clock", fiber.Map{
			"Title": "打卡",
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/leave-requests", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "請假申請",
			"PageName": "leave-requests",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/leave-requests/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增請假申請",
			"PageName": "leave-requests",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/leave-requests/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯請假申請",
			"PageName": "leave-requests",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 假期
	cmsRoutes.Get("/holidays", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":           "假期",
			"PageName":        "holidays",
			"TenantSubdomain": c.Locals("TenantSubdomain"),
		}, "layouts/cms_layout")
	})

	// 工作時段
	cmsRoutes.Get("/shifts", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "工作時段",
			"PageName": "shifts",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/shifts/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增工作時段",
			"PageName": "shifts",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/shifts/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯工作時段",
			"PageName": "shifts",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/holidays/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":           "新增假期",
			"PageName":        "holidays",
			"TenantSubdomain": c.Locals("TenantSubdomain"),
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/holidays/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":           "編輯假期",
			"PageName":        "holidays",
			"TenantSubdomain": c.Locals("TenantSubdomain"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/payrolls", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "薪資記錄",
			"PageName": "payrolls",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/payrolls/new", func(c *fiber.Ctx) error {
		return c.Render("pages/payrolls_new", fiber.Map{
			"Title":    "新增薪資記錄",
			"PageName": "payrolls",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/payrolls/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/payrolls_new", fiber.Map{
			"Title":    "編輯薪資記錄",
			"PageName": "payrolls",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 薪資附加項目 presets（用 dynamic list/form 管理）
	cmsRoutes.Get("/payroll-adjustment-presets", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "薪資附加項目",
			"PageName": "payroll_adjustment_presets",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/payroll-adjustment-presets/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增薪資附加項目",
			"PageName": "payroll_adjustment_presets",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/payroll-adjustment-presets/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯薪資附加項目",
			"PageName": "payroll_adjustment_presets",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// HR：空缺 / 聘請（用 dynamic list/form 管理）
	cmsRoutes.Get("/job-vacancies", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "空缺",
			"PageName": "job_vacancies",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/job-vacancies/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增空缺",
			"PageName": "job_vacancies",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/job-vacancies/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯空缺",
			"PageName": "job_vacancies",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/job-applicants", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "求職者",
			"PageName": "job_applicants",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/job-applicants/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增求職者",
			"PageName": "job_applicants",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/job-applicants/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯求職者",
			"PageName": "job_applicants",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	cmsRoutes.Get("/job-hires", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_list", fiber.Map{
			"Title":    "聘請",
			"PageName": "job_hires",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/job-hires/new", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "新增聘請",
			"PageName": "job_hires",
		}, "layouts/cms_layout")
	})
	cmsRoutes.Get("/job-hires/:id/edit", func(c *fiber.Ctx) error {
		return c.Render("pages/dynamic_form", fiber.Map{
			"Title":    "編輯聘請",
			"PageName": "job_hires",
			"ID":       c.Params("id"),
		}, "layouts/cms_layout")
	})

	// 認證路由（不需要租戶中間件）
	app.Get("/api/v1/auth/google/config", handlers.GetGoogleOAuthConfig) // 公開 API，獲取 Google OAuth 配置
	app.Post("/api/v1/auth/register", handlers.Register)
	app.Post("/api/v1/auth/login", handlers.Login)
	app.Post("/api/v1/auth/logout", handlers.Logout)
	app.Post("/api/v1/auth/google", handlers.GoogleLogin)
	app.Post("/api/v1/auth/forgot-password", handlers.ForgotPassword)
	app.Post("/api/v1/auth/reset-password", handlers.ResetPassword)

	// Tenant invitation (public endpoints - no auth required)
	app.Get("/api/v1/auth/validate-invite", handlers.ValidateTenantInvitation)
	// Accept invitation (auth required, but NO tenant required - user may have no tenant yet)
	app.Post("/api/v1/auth/accept-invite", middleware.AuthMiddleware, handlers.AcceptTenantInvitation)

	// SSO 跨域單點登入
	app.Post("/api/v1/sso/ticket", handlers.GenerateSSOTicket)   // 需要已登入
	app.Post("/api/v1/sso/validate", handlers.ValidateSSOTicket) // 公開端點

	// vWork Admin (簡易超級後台)
	app.Get("/vworkadmin", handlers.VWorkAdminPage)
	app.Post("/vworkadmin/login", handlers.VWorkAdminLogin)
	app.Get("/vworkadmin/logout", handlers.VWorkAdminLogout)
	app.Get("/api/v1/vworkadmin/overview", handlers.VWorkAdminOverview)
	app.Get("/api/v1/vworkadmin/hardware-purchases", handlers.VWorkAdminHardwarePurchases)
	app.Post("/api/v1/vworkadmin/login-as-user", handlers.VWorkAdminLoginAsUser)
	app.Get("/api/v1/vworkadmin/prompt", handlers.VWorkAdminGetPrompt)
	app.Post("/api/v1/vworkadmin/prompt", handlers.VWorkAdminUpdatePrompt)
	app.Get("/api/v1/vworkadmin/field-match-prompt", handlers.VWorkAdminGetFieldMatchPrompt)
	app.Post("/api/v1/vworkadmin/field-match-prompt", handlers.VWorkAdminUpdateFieldMatchPrompt)
	app.Get("/api/v1/vworkadmin/seo-settings", handlers.VWorkAdminGetSEOSettings)
	app.Post("/api/v1/vworkadmin/seo-settings", handlers.VWorkAdminUpdateSEOSettings)
	app.Get("/api/v1/vworkadmin/sales-partners", handlers.VWorkAdminSalesPartnerList)
	app.Post("/api/v1/vworkadmin/sales-partners/:id/approve", handlers.VWorkAdminApproveSalesPartner)
	app.Post("/api/v1/vworkadmin/sales-partners/:id/reject", handlers.VWorkAdminRejectSalesPartner)
	app.Post("/api/v1/vworkadmin/sales-partners/:id/pricing", handlers.VWorkAdminUpdateSalesPartnerPricing)
	app.Post("/api/v1/vworkadmin/assign-tenant-partner", handlers.VWorkAdminAssignTenantToSalesPartner)
	app.Post("/api/v1/vworkadmin/unassign-tenant-partner", handlers.VWorkAdminUnassignTenantFromSalesPartner)
	app.Get("/api/v1/vworkadmin/ad-config", handlers.VWorkAdminGetAdConfig)
	app.Post("/api/v1/vworkadmin/ad-config", handlers.VWorkAdminUpdateAdConfig)
	app.Delete("/api/v1/vworkadmin/users/:id", handlers.VWorkAdminDeleteUser)
	app.Post("/api/v1/vworkadmin/convert-password", handlers.VWorkAdminConvertPassword)
	app.Post("/api/v1/vworkadmin/update-user-email", handlers.VWorkAdminUpdateUserEmail)
	app.Post("/api/v1/vworkadmin/add-bonus-coins", handlers.VWorkAdminAddBonusCoins)
	app.Get("/api/v1/vworkadmin/tenant-coins/:id", handlers.VWorkAdminGetTenantCoins)
	app.Post("/api/v1/vworkadmin/add-subscription", handlers.VWorkAdminAddSubscription)
	app.Get("/api/v1/vworkadmin/subscription-plans", handlers.VWorkAdminGetSubscriptionPlans)

	// vWorkAdmin: Sitemap 重新產生
	app.Post("/api/v1/vworkadmin/regenerate-sitemap", handlers.VWorkAdminRegenerateSitemap)

	// vWorkAdmin: Email 配額 / 管理員通知
	app.Get("/api/v1/vworkadmin/admin-emails", handlers.VWorkAdminGetAdminEmails)
	app.Post("/api/v1/vworkadmin/admin-emails", handlers.VWorkAdminUpdateAdminEmails)
	app.Get("/api/v1/vworkadmin/email-quota", handlers.VWorkAdminGetEmailQuota)

	// Public ad config API (no auth required, used by frontend ad sidebar)
	app.Get("/api/v1/ad-config", handlers.PublicGetAdConfig)

	// vWorkAdmin: oform template management
	app.Get("/api/v1/vworkadmin/oform-templates", handlers.AdminGetOformTemplates)
	app.Post("/api/v1/vworkadmin/oform-templates", handlers.AdminCreateOformTemplate)
	app.Put("/api/v1/vworkadmin/oform-templates/:id", handlers.AdminUpdateOformTemplate)
	app.Delete("/api/v1/vworkadmin/oform-templates/:id", handlers.AdminDeleteOformTemplate)

	// vWorkAdmin: Platform Blog 管理
	app.Get("/api/v1/vworkadmin/platform-blogs", handlers.VWorkAdminGetBlogs)
	app.Get("/api/v1/vworkadmin/platform-blogs/:id", handlers.VWorkAdminGetBlog)
	app.Post("/api/v1/vworkadmin/platform-blogs", handlers.VWorkAdminCreateBlog)
	app.Put("/api/v1/vworkadmin/platform-blogs/:id", handlers.VWorkAdminUpdateBlog)
	app.Delete("/api/v1/vworkadmin/platform-blogs/:id", handlers.VWorkAdminDeleteBlog)
	app.Post("/api/v1/vworkadmin/platform-blog-cover/:id", handlers.VWorkAdminGenerateBlogCover)

	// vWorkAdmin: Platform Event 管理
	app.Get("/api/v1/vworkadmin/platform-events", handlers.VWorkAdminGetEvents)
	app.Get("/api/v1/vworkadmin/platform-events/:id", handlers.VWorkAdminGetEvent)
	app.Post("/api/v1/vworkadmin/platform-events", handlers.VWorkAdminCreateEvent)
	app.Put("/api/v1/vworkadmin/platform-events/:id", handlers.VWorkAdminUpdateEvent)
	app.Delete("/api/v1/vworkadmin/platform-events/:id", handlers.VWorkAdminDeleteEvent)
	app.Get("/api/v1/vworkadmin/platform-events/:id/registrations", handlers.VWorkAdminGetEventRegistrations)

	// vWorkAdmin: vOffice 安裝統計 + 版本管理
	app.Get("/api/v1/vworkadmin/voffice-stats", handlers.VWorkAdminVOfficeStats)
	app.Get("/api/v1/vworkadmin/voffice-installations", handlers.VWorkAdminVOfficeInstallations)
	app.Get("/api/v1/vworkadmin/voffice-releases", handlers.VWorkAdminVOfficeReleases)
	app.Post("/api/v1/vworkadmin/voffice-releases/upload", handlers.VWorkAdminUploadVOfficeRelease)
	app.Post("/api/v1/vworkadmin/voffice-releases", handlers.VWorkAdminCreateVOfficeRelease)
	app.Put("/api/v1/vworkadmin/voffice-releases/:id", handlers.VWorkAdminUpdateVOfficeRelease)
	app.Delete("/api/v1/vworkadmin/voffice-releases/:id", handlers.VWorkAdminDeleteVOfficeRelease)

	// vOffice public API (no auth — called by vOffice client on app launch)
	app.Post("/api/v1/voffice/check-update", handlers.VOfficeCheckUpdate)
	app.Get("/api/v1/voffice/latest-release", handlers.VOfficeLatestRelease)

	// 聯絡客服 API（公開路由，不需要登錄）
	// IMPORTANT: 此 API 必須保持為公開路由（不使用 AuthMiddleware），任何人都可以訪問
	// DO NOT CHANGE: 不要將此 API 移到需要認證的組中，否則會要求登錄
	app.Post("/api/v1/contact", handlers.ContactSupport)

	// Countries and Regions (公開 API，不需要認證)
	app.Get("/api/v1/countries", handlers.GetCountries)
	app.Get("/api/v1/country-regions", handlers.GetCountryRegions)

	// 輪播更新檢查 API（公開 API，供輪播播放器使用）
	app.Get("/api/v1/carousel/:ad_position_id/check-update", handlers.CheckCarouselUpdate)

	// NOTE:
	// /api/v1/phone-country-codes 必須留給 API 組（帶 Auth + Tenant middleware）使用，
	// 否則會被這個公開路由先匹配，導致 CMS 內的 Select2（例如 /customers/new）拿不到資料而變成 404。
	// 前台（/co/...）請使用 /api/v1/public/:subdomain/phone-country-codes

	// 當前用戶信息 API（需要認證，不需要租戶中間件）
	userAPI := app.Group("/api/v1/user", middleware.AuthMiddleware)
	userAPI.Get("/me", handlers.GetCurrentUser)
	userAPI.Get("/tenants", handlers.GetUserTenants)
	userAPI.Put("/me", handlers.UpdateCurrentUser)
	userAPI.Post("/select-tenant", handlers.SelectTenant)
	userAPI.Post("/change-password", handlers.ChangePassword)
	userAPI.Post("/verify-password", handlers.VerifyPassword)
	userAPI.Get("/profile/check", handlers.CheckProfileComplete)
	userAPI.Post("/profile/guide", handlers.UpdateProfileGuide)
	userAPI.Get("/menu-settings", handlers.GetUserMenuSettings)
	userAPI.Post("/menu-settings", handlers.SaveUserMenuSettings)
	userAPI.Get("/form-field-settings/:pageName", handlers.GetUserFormFieldSettings)
	userAPI.Post("/form-field-settings/:pageName", handlers.SaveUserFormFieldSettings)
	userAPI.Delete("/form-field-settings/:pageName", handlers.DeleteUserFormFieldSettings)

	// API 路由（需要租戶和認證）
	// 注意：先執行 AuthMiddleware（從 JWT 獲取租戶），再執行 TenantMiddleware（如果沒有則從 header/子域名獲取）
	api := app.Group("/api/v1", middleware.AuthMiddleware, middleware.TenantMiddleware)

	// 試用期過期鎖（API 也要鎖，避免仍可繼續呼叫業務 API）
	// 內部已放行：/api/v1/billing/*（付款解鎖必須可用）
	api.Use(middleware.TrialExpiredMiddleware)

	// System API Token 管理（僅 admin 可操作）
	apiTokenAPI := api.Group("/api-tokens", middleware.RequireRole("admin"))
	apiTokenAPI.Get("/", handlers.ListAPITokens)
	apiTokenAPI.Get("/:id", handlers.GetAPIToken)
	apiTokenAPI.Post("/", handlers.CreateAPIToken)
	apiTokenAPI.Put("/:id", handlers.UpdateAPIToken)
	apiTokenAPI.Post("/:id/revoke", handlers.RevokeAPIToken)
	apiTokenAPI.Delete("/:id", handlers.DeleteAPIToken)

	// 草稿 API
	api.Get("/drafts", handlers.GetDrafts)
	api.Get("/drafts/:id", handlers.GetDraft)
	api.Post("/drafts", handlers.SaveDraft)
	api.Delete("/drafts/:id", handlers.DeleteDraft)

	// 客戶 API
	api.Get("/customers", handlers.GetCustomers)
	api.Get("/customers/export/excel", handlers.ExportCustomersToExcel)
	api.Get("/customers/export/pdf", handlers.ExportCustomersToPDF)
	api.Get("/customers/:id", handlers.GetCustomer)
	api.Post("/customers/check-duplicate", handlers.CheckCustomerDuplicate)
	api.Post("/customers", handlers.CreateCustomer)
	api.Put("/customers/:id", handlers.UpdateCustomer)
	api.Delete("/customers/:id", handlers.DeleteCustomer)
	api.Post("/customers/:id/send-invite", handlers.SendCustomerInviteEmail)
	api.Get("/customer-labels", handlers.GetCustomerLabels)
	api.Get("/customer-labels/:id", handlers.GetCustomerLabel)
	api.Post("/customer-labels", handlers.CreateCustomerLabel)
	api.Put("/customer-labels/:id", handlers.UpdateCustomerLabel)
	api.Delete("/customer-labels/:id", handlers.DeleteCustomerLabel)

	// 客戶地址 API
	api.Get("/customers/:customerId/addresses", handlers.GetCustomerAddresses)
	api.Get("/customers/:customerId/addresses/:addressId", handlers.GetCustomerAddress)
	api.Post("/customers/:customerId/addresses", handlers.CreateCustomerAddress)
	api.Put("/customers/:customerId/addresses/:addressId", handlers.UpdateCustomerAddress)
	api.Delete("/customers/:customerId/addresses/:addressId", handlers.DeleteCustomerAddress)

	// 供應商 API
	api.Get("/suppliers", handlers.GetSuppliers)
	api.Get("/suppliers/:id", handlers.GetSupplier)
	api.Post("/suppliers", handlers.CreateSupplier)
	api.Put("/suppliers/:id", handlers.UpdateSupplier)
	api.Delete("/suppliers/:id", handlers.DeleteSupplier)

	// 產品 API
	api.Get("/products", handlers.GetProducts)
	api.Get("/products/categories", handlers.GetProductCategories)
	api.Get("/products/:id", handlers.GetProduct)
	api.Post("/products", handlers.CreateProduct)
	api.Put("/products/:id", handlers.UpdateProduct)
	api.Delete("/products/:id", handlers.DeleteProduct)
	api.Get("/products/export/excel", handlers.ExportProductsToExcel)
	api.Get("/products/export/pdf", handlers.ExportProductsToPDF)

	// 產品稅
	api.Get("/product-taxes", handlers.GetProductTaxes)
	api.Get("/product-taxes/:id", handlers.GetProductTax)
	api.Post("/product-taxes", handlers.CreateProductTax)
	api.Put("/product-taxes/:id", handlers.UpdateProductTax)
	api.Delete("/product-taxes/:id", handlers.DeleteProductTax)

	// 訂單 API
	api.Get("/orders", handlers.GetOrders)
	// 報價單 API
	api.Get("/quotations", handlers.GetQuotations)
	// 具體路由必須在通用路由之前
	api.Get("/orders/report", handlers.GetOrderReportData)
	api.Get("/orders/export/excel", handlers.ExportOrdersToExcel)
	api.Get("/orders/export/pdf", handlers.ExportOrdersToPDF)
	api.Get("/orders/import-template/excel", handlers.DownloadOrdersImportTemplateExcel)
	api.Post("/orders/import/excel", handlers.ImportOrdersFromExcel)
	api.Get("/orders/:orderId/payment-records/:index/invoice/pdf", handlers.GenerateInvoicePDF)
	api.Post("/orders/:orderId/payment-records/:index/invoice/email", handlers.SendInvoiceEmail)
	api.Get("/orders/:orderId/quotation/pdf", handlers.GenerateQuotationPDF)
	api.Get("/orders/:orderId/shipping-records/:index/shipping-note/pdf", handlers.GenerateShippingNotePDF)
	api.Get("/orders/:orderId/shipping-notes/:index/shipping-note/pdf", handlers.GenerateShippingNotePDF)
	api.Get("/orders/:orderId/refund-notes/:index/refund-note/pdf", handlers.GenerateOrderRefundNotePDF)
	api.Get("/orders/:id", handlers.GetOrder)
	api.Get("/service-orders/:serviceOrderId/payment-records/:index/invoice/pdf", handlers.GenerateServiceOrderPaymentPDF)
	api.Get("/service-orders/:serviceOrderId/refund-notes/:index/refund-note/pdf", handlers.GenerateServiceOrderRefundNotePDF)
	api.Get("/purchase-orders/:purchaseOrderId/payment-records/:index/payment/pdf", handlers.GeneratePurchaseOrderPaymentPDF)
	api.Get("/purchase-orders/:purchaseOrderId/receiving-notes/:index/receiving-note/pdf", handlers.GenerateReceivingNotePDF)
	api.Post("/orders", handlers.CreateOrder)
	api.Put("/orders/:id", handlers.UpdateOrder)
	api.Delete("/orders/:id", handlers.DeleteOrder)
	api.Post("/orders/:id/convert-to-order", handlers.ConvertQuotationToOrder)
	api.Post("/orders/:id/payment-link", handlers.GeneratePaymentLink)
	api.Delete("/orders/:id/payment-link", handlers.RevokePaymentLink)

	// 訂單標籤 API
	api.Get("/order-labels", handlers.GetOrderLabels)
	api.Get("/order-labels/:id", handlers.GetOrderLabel)
	api.Post("/order-labels", handlers.CreateOrderLabel)
	api.Put("/order-labels/:id", handlers.UpdateOrderLabel)
	api.Delete("/order-labels/:id", handlers.DeleteOrderLabel)

	// 服務單標籤 API
	api.Get("/service-order-labels", handlers.GetServiceOrderLabels)
	api.Get("/service-order-labels/:id", handlers.GetServiceOrderLabel)
	api.Post("/service-order-labels", handlers.CreateServiceOrderLabel)
	api.Put("/service-order-labels/:id", handlers.UpdateServiceOrderLabel)
	api.Delete("/service-order-labels/:id", handlers.DeleteServiceOrderLabel)

	// 出租單標籤 API
	api.Get("/rental-order-labels", handlers.GetRentalOrderLabels)
	api.Get("/rental-order-labels/:id", handlers.GetRentalOrderLabel)
	api.Post("/rental-order-labels", handlers.CreateRentalOrderLabel)
	api.Put("/rental-order-labels/:id", handlers.UpdateRentalOrderLabel)
	api.Delete("/rental-order-labels/:id", handlers.DeleteRentalOrderLabel)

	// 採購標籤 API
	api.Get("/purchase-order-labels", handlers.GetPurchaseOrderLabels)
	api.Get("/purchase-order-labels/:id", handlers.GetPurchaseOrderLabel)
	api.Post("/purchase-order-labels", handlers.CreatePurchaseOrderLabel)
	api.Put("/purchase-order-labels/:id", handlers.UpdatePurchaseOrderLabel)
	api.Delete("/purchase-order-labels/:id", handlers.DeletePurchaseOrderLabel)

	// 產品標籤 API
	api.Get("/product-labels", handlers.GetProductLabels)
	api.Get("/product-labels/:id", handlers.GetProductLabel)
	api.Post("/product-labels", handlers.CreateProductLabel)
	api.Put("/product-labels/:id", handlers.UpdateProductLabel)
	api.Delete("/product-labels/:id", handlers.DeleteProductLabel)

	// 發票 API
	api.Get("/invoices", handlers.GetInvoices)
	api.Get("/invoices/:id", handlers.GetInvoice)
	api.Post("/invoices", handlers.CreateInvoice)
	api.Put("/invoices/:id", handlers.UpdateInvoice)
	api.Delete("/invoices/:id", handlers.DeleteInvoice)
	api.Get("/invoices/export/excel", handlers.ExportInvoicesToExcel)
	api.Get("/invoices/export/pdf", handlers.ExportInvoicesToPDF)

	// 文件設定 API
	api.Get("/document-settings", handlers.GetDocumentSettings)
	api.Post("/document-settings", handlers.CreateOrUpdateDocumentSettings)
	// 單據自動生成設定
	api.Get("/document-auto-settings", handlers.GetDocumentAutoSettings)
	api.Post("/document-auto-settings", handlers.CreateOrUpdateDocumentAutoSettings)
	api.Put("/document-settings", handlers.CreateOrUpdateDocumentSettings)

	// POS 設定 API
	api.Get("/pos-settings", handlers.GetPosSettings)
	api.Post("/pos-settings", handlers.CreateOrUpdatePosSettings)

	// 餐飲桌區 API
	api.Get("/dining-areas", handlers.GetDiningAreas)
	api.Get("/dining-areas/:id", handlers.GetDiningArea)
	api.Post("/dining-areas", handlers.CreateDiningArea)
	api.Put("/dining-areas/:id", handlers.UpdateDiningArea)
	api.Delete("/dining-areas/:id", handlers.DeleteDiningArea)

	// 餐桌 API
	api.Get("/dining-tables", handlers.GetDiningTables)
	api.Get("/dining-tables/by-code", handlers.GetDiningTableByCode)
	api.Get("/dining-tables/:id", handlers.GetDiningTable)
	api.Post("/dining-tables", handlers.CreateDiningTable)
	api.Post("/dining-tables/auto-generate", handlers.AutoGenerateDiningTables)
	api.Put("/dining-tables/:id", handlers.UpdateDiningTable)
	api.Delete("/dining-tables/:id", handlers.DeleteDiningTable)
	api.Post("/dining-tables/:id/release", handlers.ReleaseDiningTable)

	// 候位排隊 API
	api.Get("/dining-queues", handlers.GetDiningQueues)
	api.Get("/dining-queues/:id", handlers.GetDiningQueue)
	api.Post("/dining-queues", handlers.CreateDiningQueue)
	api.Put("/dining-queues/:id", handlers.UpdateDiningQueue)
	api.Delete("/dining-queues/:id", handlers.DeleteDiningQueue)
	api.Post("/dining-queues/:id/seat", handlers.SeatDiningQueue)

	// 餐飲設定 API
	api.Get("/dining-settings", handlers.GetDiningSettings)
	api.Put("/dining-settings", handlers.UpdateDiningSettings)
	api.Put("/pos-settings", handlers.CreateOrUpdatePosSettings)

	// 系統提示設定 API
	api.Get("/notification-settings", handlers.GetNotificationSettings)
	api.Put("/notification-settings", handlers.UpdateNotificationSettings)

	// LLM 配置 API（只返回非敏感信息）
	api.Get("/llm/config", handlers.GetLLMConfig)
	// LLM 聊天 API（後端代理，處理 API key，需扣 AI Coins）
	api.Post("/llm/chat", middleware.RequireAICoins(models.AICoinsQuerySimple, "AI 聊天"), handlers.ChatWithLLM)
	// LLM 串流聊天 API（SSE，需扣 AI Coins）
	api.Post("/llm/chat/stream", middleware.RequireAICoins(models.AICoinsQuerySimple, "AI 串流聊天"), handlers.ChatWithLLMStream)
	// LLM 串流聊天 API — vOffice desktop 專用（修正 alt=sse + role mapping）
	api.Post("/llm/chat/stream/desktop", middleware.RequireAICoins(models.AICoinsQuerySimple, "AI 串流聊天(Desktop)"), handlers.ChatWithLLMStreamDesktop)
	// LLM 圖片生成 API（Gemini，需扣 AI Coins — 內容生成）
	api.Post("/llm/image", middleware.RequireAICoins(models.AICoinsImageGen, "AI 圖片生成"), handlers.GenerateLLMImage)

	// vOffice 登入後檢查更新（綁定 tenant + user）
	api.Post("/voffice/check-update/auth", handlers.VOfficeCheckUpdateAuth)

	// vAI 數據查詢 API（供 AI 助手查詢業務數據）
	api.Get("/vai/customers/latest", handlers.GetLatestCustomers)
	api.Get("/vai/customers/top-spending", handlers.GetTopCustomersBySpending)
	api.Get("/vai/orders/largest", handlers.GetLargestOrder)
	api.Get("/vai/stats", handlers.GetVAIStats)
	api.Get("/vai/holidays", handlers.GetVAIHolidays)
	api.Get("/vai/appointments", handlers.GetVAIAppointments)
	api.Get("/vai/staff-shifts", handlers.GetVAIStaffShifts)
	// vAI 會計查詢 API
	api.Get("/vai/accounting/transactions", handlers.GetVAILatestTransactions)
	api.Get("/vai/accounting/income-summary", handlers.GetVAIIncomeSummary)
	api.Get("/vai/accounting/expense-summary", handlers.GetVAIExpenseSummary)
	api.Get("/vai/accounting/accounts-receivable", handlers.GetVAIAccountsReceivable)
	api.Get("/vai/accounting/accounts-payable", handlers.GetVAIAccountsPayable)
	api.Get("/vai/accounting/profit-loss", handlers.GetVAIProfitLoss)
	api.Get("/vai/accounting/balance-sheet", handlers.GetVAIBalanceSheet)
	api.Get("/vai/accounting/cash-flow", handlers.GetVAICashFlow)

	// vAI 業務目標查詢 API
	api.Get("/vai/business-goals", handlers.GetVAIBusinessGoals)

	// 業務目標 API
	api.Get("/business-goals", handlers.GetBusinessGoals)
	api.Get("/business-goals/dashboard", handlers.GetBusinessGoalsDashboard)
	api.Get("/business-goals/:id", handlers.GetBusinessGoal)
	api.Post("/business-goals", handlers.CreateBusinessGoal)
	api.Put("/business-goals/:id", handlers.UpdateBusinessGoal)
	api.Delete("/business-goals/:id", handlers.DeleteBusinessGoal)
	api.Post("/business-goals/:id/refresh", handlers.RefreshBusinessGoalProgress)
	api.Get("/business-goals/:id/trackings", handlers.GetBusinessGoalTrackings)
	api.Post("/business-goals/:id/trackings", handlers.AddManualTracking)
	api.Post("/business-goals/:id/ai-suggestion", handlers.GetBusinessGoalAISuggestion)
	api.Get("/business-goals/:id/ai-analyses", handlers.GetBusinessGoalAIAnalyses)
	api.Post("/business-goals/:id/ai-analyses", handlers.CreateBusinessGoalAIAnalysis)
	api.Delete("/business-goals/:id/ai-analyses/:analysisId", handlers.DeleteBusinessGoalAIAnalysis)

	// AI Input APIs (OCR, STT, Field Matching, File Upload)
	api.Post("/ai/ocr", handlers.ProcessOCR)
	api.Post("/ai/stt", handlers.ProcessSTT)
	api.Post("/ai/match-fields", handlers.MatchFieldsWithLLM)
	api.Post("/ai/upload-file", handlers.UploadAIFile)

	// AI Conversations API
	api.Get("/ai/conversations", handlers.GetAiConversations)
	api.Get("/ai/conversations/search", handlers.SearchAiConversations)
	api.Post("/ai/conversations", handlers.CreateAiConversation)
	api.Get("/ai/conversations/:id/messages", handlers.GetAiConversationMessages)
	api.Put("/ai/conversations/:id", handlers.UpdateAiConversation)
	api.Delete("/ai/conversations/:id", handlers.DeleteAiConversation)

	// AI Sketch Image Upload (base64 data URL → server file)
	api.Post("/ai/sketch-image-upload", handlers.UploadSketchImage)

	// AI Sketch CRUD API
	api.Get("/ai/sketches", handlers.GetAiSketches)
	api.Post("/ai/sketches", handlers.CreateAiSketch)
	api.Get("/ai/sketches/:id", handlers.GetAiSketch)
	api.Put("/ai/sketches/:id", handlers.UpdateAiSketch)
	api.Delete("/ai/sketches/:id", handlers.DeleteAiSketch)
	// AI Sketch Image Library (template metadata)
	api.Get("/ai/image-library", handlers.GetImageLibrary)
	// AI 草圖生成（需扣 AI Coins — 內容生成）
	api.Post("/ai/sketch-generate", middleware.RequireAICoins(models.AICoinsSketchGen, "AI 草圖生成"), handlers.GenerateLLMSketch)

	// AI 影片生成 API（Kling 3.0 Omni — multi-shot with native audio）
	api.Post("/llm/video/storyboard", middleware.RequireVideoGen(), handlers.GenerateStoryboard) // AI storyboard generation (chat-first UX)
	api.Post("/llm/video/chat", middleware.RequireVideoGen(), handlers.VideoChatMessage)         // multi-turn AI chat for video planning
	api.Post("/llm/video", middleware.RequireVideoGen(), middleware.RequireAICoins(models.AICoinsVideoGen, "AI 影片生成"), handlers.GenerateVideo)
	api.Get("/llm/video/history", handlers.GetVideoHistory)                         // must be before wildcard
	api.Post("/llm/video/history", handlers.SaveVideoRecord)                        // create video project
	api.Patch("/llm/video/history/:id", handlers.UpdateVideoRecord)                 // update video project (shots, status)
	api.Delete("/llm/video/history/:id", handlers.DeleteVideoRecord)                // soft-delete video project
	api.Patch("/llm/video/history/:id/mark-deleted", handlers.MarkChatVideoDeleted) // mark chat video as deleted
	api.Get("/llm/video/*", handlers.PollVideoOperation)                            // Poll Kling task status

	// AI Sketch Generation History API
	api.Get("/ai/sketch-generations", handlers.GetAiSketchGenerations)
	api.Put("/ai/sketch-generations/link-orphaned", handlers.LinkOrphanedGenerations)
	api.Get("/ai/sketch-generations/:id", handlers.GetAiSketchGeneration)
	api.Get("/ai/sketch-generations/:id/image", handlers.GetAiSketchGenerationImage)
	api.Delete("/ai/sketch-generations/:id", handlers.DeleteAiSketchGeneration)

	// AI Document Generation API
	api.Get("/ai/documents", handlers.GetAiDocuments)
	api.Get("/ai/documents/:id", handlers.GetAiDocument)
	api.Get("/ai/documents/:id/download", handlers.DownloadAiDocument)
	api.Delete("/ai/documents/:id", handlers.DeleteAiDocument)
	api.Post("/ai/doc-generate", middleware.RequireAICoins(models.AICoinsDocGenBase, "AI 文件生成"), handlers.GenerateAiDocument)

	// 訂單設置 API
	// 付款方式
	api.Get("/payment-methods", handlers.GetPaymentMethods)
	api.Get("/payment-methods/:id", handlers.GetPaymentMethod)
	api.Post("/payment-methods", handlers.CreatePaymentMethod)
	api.Put("/payment-methods/:id", handlers.UpdatePaymentMethod)
	api.Delete("/payment-methods/:id", handlers.DeletePaymentMethod)

	// 運送方式
	api.Get("/shipping-methods", handlers.GetShippingMethods)
	api.Get("/shipping-methods/:id", handlers.GetShippingMethod)
	api.Post("/shipping-methods", handlers.CreateShippingMethod)
	api.Put("/shipping-methods/:id", handlers.UpdateShippingMethod)
	api.Delete("/shipping-methods/:id", handlers.DeleteShippingMethod)

	// 物流公司
	api.Get("/logistics-companies", handlers.GetLogisticsCompanies)
	api.Get("/logistics-companies/:id", handlers.GetLogisticsCompany)
	api.Post("/logistics-companies", handlers.CreateLogisticsCompany)

	// 銀行賬戶
	api.Get("/bank-accounts", handlers.GetBankAccounts)
	api.Get("/bank-accounts/:id", handlers.GetBankAccount)
	api.Post("/bank-accounts", handlers.CreateBankAccount)
	api.Put("/bank-accounts/:id", handlers.UpdateBankAccount)
	api.Delete("/bank-accounts/:id", handlers.DeleteBankAccount)
	api.Get("/bank-accounts/stats", handlers.GetBankAccountStats)
	api.Put("/logistics-companies/:id", handlers.UpdateLogisticsCompany)
	api.Delete("/logistics-companies/:id", handlers.DeleteLogisticsCompany)
	api.Post("/logistics-companies/calculate-fee", handlers.CalculateLogisticsFee)
	api.Post("/logistics-companies/calculate-best-fee", handlers.CalculateBestLogisticsFee)

	// 配送管理
	api.Get("/shipments", handlers.GetShipments)
	api.Get("/shipments/:id", handlers.GetShipment)
	api.Post("/shipments", handlers.CreateShipment)
	api.Put("/shipments/:id", handlers.UpdateShipment)
	api.Delete("/shipments/:id", handlers.DeleteShipment)
	api.Put("/shipments/:id/status", handlers.UpdateShipmentStatus)
	api.Get("/shipments/:id/history", handlers.GetShipmentHistory)
	api.Post("/inventory-movements/:id/create-shipment", handlers.CreateShipmentFromInventoryMovement)

	// 配送連接設定
	api.Get("/shipping-integrations", handlers.GetShippingIntegrations)
	api.Put("/shipping-integrations", handlers.UpdateShippingIntegrations)
	api.Post("/shipping-integrations/sfexpress/test", handlers.TestSFExpressConnection)
	api.Post("/shipping-integrations/lalamove/test", handlers.TestLalamoveConnection)
	api.Post("/shipping-integrations/dhl/test", handlers.TestDHLConnection)
	api.Post("/shipping-integrations/ups/test", handlers.TestUPSConnection)
	api.Post("/shipping-integrations/fedex/test", handlers.TestFedExConnection)
	api.Post("/shipping-integrations/amazon/test", handlers.TestAmazonShippingConnection)

	// 外賣平台整合 API（Foodpanda, Keeta, Deliveroo）
	api.Get("/delivery-integrations", handlers.GetDeliveryIntegrations)
	api.Get("/delivery-integrations/:id", handlers.GetDeliveryIntegration)
	api.Post("/delivery-integrations", handlers.CreateOrUpdateDeliveryIntegration)
	api.Put("/delivery-integrations/:id", handlers.CreateOrUpdateDeliveryIntegration)
	api.Delete("/delivery-integrations/:id", handlers.DeleteDeliveryIntegration)
	api.Post("/delivery-integrations/test", handlers.TestDeliveryIntegrationDirect)
	api.Post("/delivery-integrations/:id/test", handlers.TestDeliveryIntegration)
	api.Post("/delivery-integrations/:id/sync-menu", handlers.SyncMenuHandler)
	api.Put("/delivery-integrations/:id/item-availability", handlers.UpdateItemAvailabilityHandler)

	// 外賣訂單 API（整合架構 V2）
	api.Get("/delivery-orders", handlers.GetDeliveryOrdersV2)
	api.Get("/delivery-orders/:id", handlers.GetDeliveryOrderV2)
	api.Post("/delivery-orders/:id/accept", handlers.AcceptDeliveryOrderV2)
	api.Post("/delivery-orders/:id/reject", handlers.RejectDeliveryOrderV2)
	api.Post("/delivery-orders/:id/status", handlers.UpdateDeliveryOrderStatusV2)
	api.Post("/delivery-orders/sync", handlers.SyncDeliveryOrdersV2)

	// 外賣產品映射 API
	api.Get("/delivery-product-mappings", handlers.GetDeliveryProductMappings)
	api.Post("/delivery-product-mappings", handlers.CreateDeliveryProductMapping)
	api.Put("/delivery-product-mappings/:id", handlers.UpdateDeliveryProductMappingHandler)
	api.Delete("/delivery-product-mappings/:id", handlers.DeleteDeliveryProductMapping)

	// ============================================
	// Public webstore checkout (no CMS auth; uses /co/:subdomain and customer_id cookie)
	// ============================================
	// Public customer auth/profile (no CMS auth; uses /co/:subdomain and customer_id cookie)
	app.Get("/api/v1/public/:subdomain/phone-country-codes", handlers.PublicGetPhoneCountryCodes)
	app.Post("/api/v1/public/:subdomain/customer/register", handlers.PublicCustomerRegister)
	app.Post("/api/v1/public/:subdomain/customer/login", handlers.PublicCustomerLogin)
	app.Post("/api/v1/public/:subdomain/customer/logout", handlers.PublicCustomerLogout)
	app.Get("/api/v1/public/:subdomain/customer/me", handlers.PublicCustomerMe)
	app.Put("/api/v1/public/:subdomain/customer/me", handlers.PublicCustomerUpdate)
	app.Get("/api/v1/public/:subdomain/customer/orders", handlers.PublicGetCustomerOrders)
	app.Post("/api/v1/public/:subdomain/contact", handlers.PublicCreateSupportCommunication)
	// Public tenant chat (no login; visitor_id based)
	app.Get("/api/v1/public/:subdomain/chat/config", handlers.PublicGetChatConfig)
	app.Get("/api/v1/public/:subdomain/chat/conversation", handlers.PublicGetChatConversation)
	app.Post("/api/v1/public/:subdomain/chat/messages", handlers.PublicCreateChatMessage)
	// Public LLM chat — REMOVED for security (was exposing API key without auth)
	// All LLM endpoints now require authentication: /api/v1/llm/chat, /api/v1/llm/chat/stream

	app.Post("/api/v1/public/:subdomain/checkout/stripe/payment-intent", handlers.PublicCreateStripePaymentIntent)
	app.Post("/api/v1/public/:subdomain/checkout/stripe/confirm", handlers.PublicConfirmStripePayment)
	app.Post("/api/v1/public/:subdomain/checkout/paypal/create-order", handlers.PublicCreatePayPalOrder)
	app.Post("/api/v1/public/:subdomain/checkout/paypal/capture-order", handlers.PublicCapturePayPalOrder)
	// Stripe native methods: Alipay, WeChat Pay, Apple Pay, Google Pay
	app.Post("/api/v1/public/:subdomain/checkout/stripe-native/payment-intent", handlers.PublicCreateStripeNativePaymentIntent)
	app.Post("/api/v1/public/:subdomain/checkout/stripe-native/confirm", handlers.PublicConfirmStripeNativePayment)
	// QFPay methods: FPS, PayMe, Alipay HK, WeChat Pay HK, BoC Pay, Octopus
	app.Post("/api/v1/public/:subdomain/checkout/qfpay/create-payment", handlers.PublicCreateQFPayPayment)
	app.Post("/api/v1/public/:subdomain/checkout/qfpay/check-status", handlers.PublicCheckQFPayStatus)
	// UnionPay (銀聯/雲閃付)
	app.Post("/api/v1/public/:subdomain/checkout/unionpay/create-payment", handlers.PublicCreateUnionPayPayment)
	app.Post("/api/v1/public/:subdomain/checkout/unionpay/check-status", handlers.PublicCheckUnionPayStatus)

	// Payment Link — public payment page and APIs (no auth, token-based)
	app.Get("/pay/:token", handlers.RenderPaymentPage)
	app.Post("/api/v1/pay/:token/stripe/payment-intent", handlers.PaymentLinkCreateStripeIntent)
	app.Post("/api/v1/pay/:token/stripe/confirm", handlers.PaymentLinkConfirmStripe)
	app.Post("/api/v1/pay/:token/paypal/create-order", handlers.PaymentLinkCreatePayPalOrder)
	app.Post("/api/v1/pay/:token/paypal/capture-order", handlers.PaymentLinkCapturePayPal)
	// Payment Link — Stripe native methods
	app.Post("/api/v1/pay/:token/stripe-native/payment-intent", handlers.PaymentLinkCreateStripeNativeIntent)
	app.Post("/api/v1/pay/:token/stripe-native/confirm", handlers.PaymentLinkConfirmStripeNative)
	// Payment Link — QFPay methods
	app.Post("/api/v1/pay/:token/qfpay/create-payment", handlers.PaymentLinkCreateQFPayPayment)
	app.Post("/api/v1/pay/:token/qfpay/check-status", handlers.PaymentLinkCheckQFPayStatus)
	// Payment Link — UnionPay
	app.Post("/api/v1/pay/:token/unionpay/create-payment", handlers.PaymentLinkCreateUnionPayPayment)
	app.Post("/api/v1/pay/:token/unionpay/check-status", handlers.PaymentLinkCheckUnionPayStatus)
	// QFPay Webhook (handles FPS, PayMe, Alipay HK, WeChat Pay HK, BoC Pay, Octopus, UnionPay)
	app.Post("/api/v1/webhooks/qfpay/notify", handlers.QFPayWebhookNotify)
	// Payment gateway registry (public — for frontend to know available gateway types)
	app.Get("/api/v1/public/payment-gateways", handlers.GetPaymentGatewayRegistry)
	// Public helpers for webstore (requires customer_id cookie)
	app.Get("/api/v1/public/:subdomain/customer/addresses", handlers.PublicGetCustomerAddresses)
	app.Post("/api/v1/public/:subdomain/logistics-companies/calculate-best-fee", handlers.PublicCalculateBestLogisticsFee)
	// Public dining queue (no auth)
	app.Post("/api/v1/public/:subdomain/dining/queues", handlers.PublicCreateDiningQueue)
	app.Post("/api/v1/public/:subdomain/dining/orders", handlers.PublicCreateDiningOrder)
	app.Get("/api/v1/public/:subdomain/dining/orders", handlers.PublicGetDiningOrder)
	app.Post("/api/v1/public/:subdomain/dining/orders/:id/items", handlers.PublicAppendDiningOrderItems)
	app.Get("/api/v1/public/:subdomain/dining/areas", handlers.PublicGetDiningAreas)
	app.Get("/api/v1/public/:subdomain/dining/queue/status", handlers.PublicGetDiningQueueStatus)
	// Public service booking (no auth)
	app.Get("/api/v1/public/:subdomain/services", handlers.PublicGetServices)
	app.Get("/api/v1/public/:subdomain/service-staff", handlers.PublicGetServiceStaff)
	app.Post("/api/v1/public/:subdomain/appointments", handlers.PublicCreateAppointment)

	// Phone country codes
	api.Get("/phone-country-codes", handlers.GetPhoneCountryCodes)
	api.Get("/phone-country-codes/:id", handlers.GetPhoneCountryCode)
	api.Post("/phone-country-codes", handlers.CreatePhoneCountryCode)
	api.Put("/phone-country-codes/:id", handlers.UpdatePhoneCountryCode)
	api.Put("/phone-country-codes/:code/default", handlers.SetDefaultPhoneCountryCode)
	api.Delete("/phone-country-codes/:id", handlers.DeletePhoneCountryCode)

	// 付款 API
	api.Get("/payments", handlers.GetPayments)
	api.Post("/payments", handlers.CreatePayment)

	// 儀表板統計 API
	api.Get("/dashboard/stats", handlers.GetDashboardStats)
	api.Get("/dashboard/invite-card-status", handlers.GetInviteCardStatus)
	api.Post("/dashboard/dismiss-invite-card", handlers.DismissInviteCard)

	// Tenant invitations (send invites to join company)
	api.Post("/tenant-invitations", handlers.SendTenantInvitations)

	// 動態字段 API
	api.Get("/fields/:model/:id", handlers.GetExtraFields)
	api.Put("/fields/:model/:id", handlers.UpdateExtraFields)
	api.Delete("/fields/:model/:id/:field", handlers.DeleteExtraField)

	// ============================================
	// 設定模組 API
	// ============================================

	// 企業 API（每個租戶只有一個企業，作為設置頁面）
	enterpriseAPI := app.Group("/api/v1/enterprises", middleware.AuthMiddleware, middleware.TenantMiddleware)
	enterpriseAPI.Get("/me", handlers.GetCurrentEnterprise)
	enterpriseAPI.Put("/me", handlers.UpdateCurrentEnterprise)

	// 公司 API
	companyAPI := app.Group("/api/v1/companies", middleware.AuthMiddleware)
	companyAPI.Get("/", handlers.GetCompanies)
	companyAPI.Get("/:id", handlers.GetCompany)
	companyAPI.Post("/", handlers.CreateCompany)
	companyAPI.Put("/:id", handlers.UpdateCompany)
	companyAPI.Delete("/:id", handlers.DeleteCompany)

	// 部門 API
	departmentAPI := app.Group("/api/v1/departments", middleware.AuthMiddleware)
	departmentAPI.Get("/", handlers.GetDepartments)
	departmentAPI.Get("/:id", handlers.GetDepartment)
	departmentAPI.Post("/", handlers.CreateDepartment)
	departmentAPI.Put("/:id", handlers.UpdateDepartment)
	departmentAPI.Delete("/:id", handlers.DeleteDepartment)

	// 角色 API（原級別）
	roleAPI := app.Group("/api/v1/roles", middleware.AuthMiddleware)
	roleAPI.Get("/", handlers.GetRoles)
	roleAPI.Get("/:id", handlers.GetRole)
	roleAPI.Post("/", handlers.CreateRole)
	roleAPI.Put("/:id", handlers.UpdateRole)
	roleAPI.Delete("/:id", handlers.DeleteRole)

	// 地區 API
	regionAPI := app.Group("/api/v1/regions", middleware.AuthMiddleware)
	regionAPI.Get("/", handlers.GetRegions)
	regionAPI.Get("/:id", handlers.GetRegion)
	regionAPI.Post("/", handlers.CreateRegion)
	regionAPI.Put("/:id", handlers.UpdateRegion)
	regionAPI.Delete("/:id", handlers.DeleteRegion)

	// 貨幣 API
	currencyAPI := app.Group("/api/v1/currencies", middleware.AuthMiddleware)
	currencyAPI.Get("/", handlers.GetCurrencies)
	currencyAPI.Get("/:id", handlers.GetCurrency)
	currencyAPI.Post("/", handlers.CreateCurrency)
	currencyAPI.Put("/:id", handlers.UpdateCurrency)
	currencyAPI.Delete("/:id", handlers.DeleteCurrency)
	currencyAPI.Post("/update-rates", handlers.UpdateCurrencyRates)

	// ============================================
	// 個人模組 API
	// ============================================

	// 日曆 API
	api.Get("/calendars", handlers.GetCalendars)
	api.Get("/calendars/:id", handlers.GetCalendar)
	api.Post("/calendars", handlers.CreateCalendar)
	api.Put("/calendars/:id", handlers.UpdateCalendar)
	api.Delete("/calendars/:id", handlers.DeleteCalendar)

	// 提醒 API
	api.Get("/reminders", handlers.GetReminders)
	api.Get("/reminders/:id", handlers.GetReminder)
	api.Post("/reminders", handlers.CreateReminder)
	api.Put("/reminders/:id", handlers.UpdateReminder)
	api.Put("/reminders/:id/complete", handlers.CompleteReminder)
	api.Delete("/reminders/:id", handlers.DeleteReminder)

	// 訊息 API
	api.Get("/messages", handlers.GetMessages)
	api.Get("/messages/unread-count", handlers.GetUnreadMessageCount)   // 必须在 /messages/:id 之前
	api.Get("/messages/conversations", handlers.GetConversations)       // 必须在 /messages/:id 之前
	api.Get("/messages/conversation", handlers.GetConversationMessages) // 必须在 /messages/:id 之前
	api.Get("/messages/:id", handlers.GetMessage)
	api.Post("/messages", handlers.CreateMessage)
	api.Patch("/messages/:id", handlers.UpdateMessage)
	api.Put("/messages/:id/read", handlers.MarkMessageRead)
	api.Delete("/messages/:id", handlers.DeleteMessage)

	// 提示資訊 API
	api.Get("/notifications", handlers.GetNotificationAlerts)
	api.Get("/notifications/unread-count", handlers.GetUnreadNotificationCount) // 必须在 /notifications/:id 之前
	api.Get("/notifications/:id", handlers.GetNotificationAlert)
	api.Put("/notifications/:id/read", handlers.MarkNotificationAlertAsRead)
	api.Put("/notifications/read-all", handlers.MarkAllNotificationAlertsAsRead)

	// 活動記錄 API
	api.Get("/activity-logs", handlers.GetActivityLogs)
	api.Get("/activity-logs/stats", handlers.GetActivityLogStats)
	api.Get("/activity-logs/:id", handlers.GetActivityLog)

	// 備忘 API
	api.Get("/notes", handlers.GetNotes)
	api.Get("/notes/:id", handlers.GetNote)
	api.Post("/notes", handlers.CreateNote)
	api.Put("/notes/:id", handlers.UpdateNote)
	api.Delete("/notes/:id", handlers.DeleteNote)

	// 個人資料 API
	api.Get("/personal-data", handlers.GetPersonalData)
	api.Get("/personal-data/:id", handlers.GetPersonalDataItem)
	api.Post("/personal-data", handlers.CreatePersonalData)
	api.Put("/personal-data/:id", handlers.UpdatePersonalData)
	api.Delete("/personal-data/:id", handlers.DeletePersonalData)

	// ============================================
	// 客戶擴展模組 API
	// ============================================

	// 會員等級 API
	api.Get("/member-levels", handlers.GetMemberLevels)
	api.Get("/member-levels/:id", handlers.GetMemberLevel)
	api.Post("/member-levels", handlers.CreateMemberLevel)
	api.Put("/member-levels/:id", handlers.UpdateMemberLevel)
	api.Delete("/member-levels/:id", handlers.DeleteMemberLevel)

	// 積分 API
	api.Get("/points", handlers.GetPoints)
	api.Get("/points/:id", handlers.GetPoint)
	api.Post("/points", handlers.CreatePoint)
	api.Delete("/points/:id", handlers.DeletePoint)

	// 優惠券 API
	api.Get("/coupons", handlers.GetCoupons)
	// 具體路由必須在通用路由之前，避免路由衝突
	api.Get("/coupons/validate", handlers.ValidateCoupon)
	api.Get("/coupons/:id/usage", handlers.GetCouponUsage)
	api.Get("/coupons/:id", handlers.GetCoupon)
	api.Post("/coupons", handlers.CreateCoupon)
	api.Put("/coupons/:id", handlers.UpdateCoupon)
	api.Delete("/coupons/:id", handlers.DeleteCoupon)

	// 優惠券條件 API
	api.Get("/coupons/:coupon_id/conditions", handlers.GetCouponConditions)
	api.Post("/coupons/:coupon_id/conditions", handlers.CreateCouponCondition)
	api.Put("/coupons/:coupon_id/conditions/:id", handlers.UpdateCouponCondition)
	api.Delete("/coupons/:coupon_id/conditions/:id", handlers.DeleteCouponCondition)

	// 預留編號 API
	api.Post("/reserved-numbers", handlers.ReserveNumber)
	api.Get("/reserved-numbers/check", handlers.CheckReservedNumber)
	api.Get("/reserved-numbers/next", handlers.GetNextNumber)
	api.Delete("/reserved-numbers", handlers.ReleaseReservedNumber)

	// CMS 全站搜尋（Topnav searchbox）
	api.Get("/search", handlers.GlobalSearch)

	// ============================================
	// 垃圾筒 API（軟刪除）
	// ============================================
	api.Get("/trash/resources", handlers.GetSupportedTrashResources)
	api.Get("/trash/:resource", handlers.GetTrashItems)
	api.Post("/trash/:resource/:id/restore", handlers.RestoreTrashItem)
	api.Delete("/trash/:resource/:id", handlers.PermanentDeleteTrashItem)
	api.Post("/trash/:resource/bulk-restore", handlers.BulkRestoreTrashItems)
	api.Post("/trash/:resource/bulk-delete", handlers.BulkPermanentDeleteTrashItems)

	// 用戶管理 API（同一租戶下）
	api.Get("/users", handlers.GetUsers)
	api.Get("/users/:id", handlers.GetUser)
	api.Post("/users", handlers.CreateUser)
	api.Put("/users/:id", handlers.UpdateUser)
	api.Delete("/users/:id", handlers.DeleteUser)
	api.Post("/users/invite", handlers.InviteUser)
	api.Post("/users/:id/send-invite", handlers.SendUserInviteEmail)

	// 訂閱和付款 API
	api.Get("/billing/subscription", handlers.GetSubscription)
	api.Post("/billing/create-checkout-session", handlers.CreateCheckoutSession)
	api.Get("/billing/sync-checkout-session", handlers.SyncCheckoutSession)
	api.Post("/billing/cancel-subscription", handlers.CancelSubscription)
	api.Get("/billing/payment-history", handlers.GetPaymentHistory)

	// Stripe Connect API（讓租戶一鍵連接 Stripe 收款）
	api.Get("/stripe-connect/status", handlers.GetStripeConnectStatus)
	api.Post("/stripe-connect/create-account", handlers.CreateStripeConnectAccount)
	api.Post("/stripe-connect/dashboard-link", handlers.CreateStripeConnectLoginLink)
	api.Post("/stripe-connect/disconnect", handlers.DisconnectStripeConnect)

	// 手機 App 構建 API
	appBuildHandler := handlers.NewAppBuildHandler(database.DB)
	api.Get("/app/config", appBuildHandler.GetAppConfig)
	api.Post("/app/config", appBuildHandler.SaveAppConfig)
	api.Post("/app/build", appBuildHandler.TriggerBuild)
	api.Get("/app/build/status", appBuildHandler.GetBuildStatus)
	// App 構建完成回調（公開 API，需驗證簽名）
	app.Post("/api/v1/app/build/webhook", appBuildHandler.BuildWebhook)

	// AI Coins API
	api.Get("/ai-coins/balance", handlers.GetAICoinsBalance)
	api.Get("/ai-coins/plans", handlers.GetAICoinsPlans)
	api.Get("/ai-coins/transactions", handlers.GetAICoinsTransactions)
	api.Post("/ai-coins/purchase", handlers.PurchaseAICoins)

	// 硬件購買 API
	api.Get("/hardware-purchase/settings", handlers.GetHardwarePurchaseSettings)
	api.Put("/hardware-purchase/settings", handlers.UpdateHardwarePurchaseSettings)
	api.Post("/hardware-purchase/create-checkout-session", handlers.CreateHardwarePurchaseCheckoutSession)
	api.Get("/hardware-purchase/sync-checkout-session", handlers.SyncHardwarePurchaseCheckoutSession)
	api.Get("/hardware-purchase/records", handlers.GetHardwarePurchaseRecords)
	api.Post("/hardware-purchase/submit-company-info", handlers.SubmitCompanyInfo)

	// Stripe Webhook（不需要租戶中間件，但需要驗證）
	app.Post("/api/v1/billing/webhook", handlers.HandleStripeWebhook)

	// IAP (In-App Purchase) API — Google Play + Apple App Store
	api.Post("/billing/iap/verify", handlers.HandleIAPVerify)
	api.Get("/billing/iap/products", handlers.HandleGetIAPProducts)
	// IAP Webhooks（不需要認證中間件 — 由 Google/Apple 服務器推送）
	app.Post("/api/v1/billing/webhook/google", handlers.HandleGooglePlayWebhook)
	app.Post("/api/v1/billing/webhook/apple", handlers.HandleAppleWebhook)

	// 行業模板 API（公開，用於註冊時選擇）
	app.Get("/api/v1/industry-templates", handlers.GetIndustryTemplates)
	app.Get("/api/v1/industry-templates/:id", handlers.GetIndustryTemplate)

	// 租戶行業模板和模塊管理 API（需要認證）
	api.Get("/tenant/industry-template", handlers.GetTenantIndustryTemplate)
	api.Post("/tenant/apply-template/:id", handlers.ApplyIndustryTemplate)

	// 租戶設置 API - 添加專門的日誌中間件
	api.Post("/tenant/setup", func(c *fiber.Ctx) error {
		log.Printf("🔵 Route matched: POST /api/v1/tenant/setup")
		log.Printf("🔵 Request Method: %s, Path: %s", c.Method(), c.Path())
		log.Printf("🔵 Content-Type: %s", c.Get("Content-Type"))
		log.Printf("🔵 Accept: %s", c.Get("Accept"))
		return handlers.SetupTenant(c)
	})

	// 網站主題管理 API
	api.Get("/tenant/website-theme", handlers.GetWebsiteTheme)
	api.Put("/tenant/website-theme", handlers.UpdateWebsiteTheme)

	// 網站設定管理 API
	api.Get("/tenant/website-settings", handlers.GetWebsiteSettings)
	api.Put("/tenant/website-settings", handlers.UpdateWebsiteSettings)
	api.Get("/tenant/vmarket-settings", handlers.GetVMarketSettings)
	api.Put("/tenant/vmarket-settings", handlers.UpdateVMarketSettings)
	api.Get("/tenant/service-settings", handlers.GetServiceSettings)
	api.Put("/tenant/service-settings", handlers.UpdateServiceSettings)
	api.Get("/tenant/product-sync-settings", handlers.GetProductSyncSettings)
	api.Put("/tenant/product-sync-settings", handlers.UpdateProductSyncSettings)

	// Shopee 電商同步 API
	api.Get("/shopee/auth-url", handlers.GetShopeeAuthURL)
	api.Get("/shopee/shop-info", handlers.GetShopeeShopInfo)
	api.Get("/shopee/products", handlers.GetShopeeProducts)
	api.Get("/shopee/orders", handlers.GetShopeeOrders)
	api.Post("/shopee/sync-inventory", handlers.SyncShopeeInventory)
	api.Post("/shopee/sync-price", handlers.SyncShopeePrice)
	api.Post("/shopee/refresh-token", handlers.RefreshShopeeToken)
	api.Post("/shopee/sync", handlers.ManualShopeeSync)
	api.Get("/shopee/sync-status", handlers.GetShopeeSyncStatus)
	api.Get("/shopee/test-connection", handlers.TestShopeeConnection)

	// Amazon 電商同步 API
	api.Get("/amazon/auth-url", handlers.GetAmazonAuthURL)
	api.Get("/amazon/seller-info", handlers.GetAmazonSellerInfo)
	api.Get("/amazon/listings", handlers.GetAmazonListings)
	api.Get("/amazon/orders", handlers.GetAmazonOrders)
	api.Get("/amazon/orders/:order_id", handlers.GetAmazonOrderDetail)
	api.Post("/amazon/sync-inventory", handlers.SyncAmazonInventory)
	api.Post("/amazon/sync-price", handlers.SyncAmazonPrice)
	api.Post("/amazon/refresh-token", handlers.RefreshAmazonToken)
	api.Post("/amazon/sync", handlers.ManualAmazonSync)
	api.Get("/amazon/sync-status", handlers.GetAmazonSyncStatus)
	api.Get("/amazon/test-connection", handlers.TestAmazonConnection)
	api.Put("/amazon/settings", handlers.SaveAmazonSettings)

	// Lazada 電商同步 API
	api.Get("/lazada/auth-url", handlers.GetLazadaAuthURL)
	api.Get("/lazada/callback", handlers.LazadaCallback)
	api.Get("/lazada/seller-info", handlers.GetLazadaSellerInfo)
	api.Get("/lazada/products", handlers.GetLazadaProducts)
	api.Get("/lazada/orders", handlers.GetLazadaOrders)
	api.Get("/lazada/orders/:order_id", handlers.GetLazadaOrderDetail)
	api.Get("/lazada/categories", handlers.GetLazadaCategories)
	api.Post("/lazada/sync-inventory", handlers.SyncLazadaInventory)
	api.Post("/lazada/sync-price", handlers.SyncLazadaPrice)
	api.Post("/lazada/refresh-token", handlers.RefreshLazadaToken)
	api.Post("/lazada/sync", handlers.ManualLazadaSync)
	api.Get("/lazada/sync-status", handlers.GetLazadaSyncStatus)
	api.Get("/lazada/test-connection", handlers.TestLazadaConnection)
	api.Put("/lazada/settings", handlers.SaveLazadaSettingsHandler)
	api.Delete("/lazada/disconnect", handlers.DisconnectLazada)

	// Rakuten 電商同步 API（樂天市場）
	api.Get("/rakuten/auth-url", handlers.RakutenGetAuthURL)
	api.Get("/rakuten/callback", handlers.RakutenCallback)
	api.Get("/rakuten/shop-info", handlers.RakutenGetShopInfo)
	api.Get("/rakuten/products", handlers.RakutenGetProducts)
	api.Get("/rakuten/orders", handlers.RakutenGetOrders)
	api.Get("/rakuten/genres", handlers.RakutenGetGenres)
	api.Post("/rakuten/sync-inventory", handlers.RakutenSyncInventory)
	api.Post("/rakuten/sync-price", handlers.RakutenSyncPrice)
	api.Post("/rakuten/refresh-token", handlers.RakutenRefreshToken)
	api.Post("/rakuten/sync", handlers.RakutenSyncOrders)
	api.Get("/rakuten/sync-status", handlers.RakutenGetSyncStatus)
	api.Get("/rakuten/test-connection", handlers.RakutenTestConnection)
	api.Put("/rakuten/settings", handlers.RakutenUpdateSettings)
	api.Delete("/rakuten/disconnect", handlers.RakutenDisconnect)
	api.Put("/rakuten/orders/:orderNumber/status", handlers.RakutenUpdateOrderStatus)
	api.Post("/rakuten/orders/:orderNumber/ship", handlers.RakutenShipOrder)

	// 網站自訂網域（MVP）
	api.Get("/tenant/custom-domain", handlers.GetWebsiteCustomDomain)
	api.Put("/tenant/custom-domain", handlers.UpdateWebsiteCustomDomain)
	api.Put("/tenant/custom-domains", handlers.UpdateWebsiteCustomDomains)
	api.Post("/tenant/custom-domain/check-dns", handlers.CheckWebsiteCustomDomainDNS)
	api.Put("/tenant/custom-domain/setup-method", handlers.UpdateWebsiteDomainSetupMethod)

	// Cloudflare for SaaS: Custom Hostnames (auto SSL)
	api.Post("/tenant/custom-domain/cloudflare/sync", handlers.CloudflareSyncCustomHostname)
	api.Get("/tenant/custom-domain/cloudflare/status", handlers.CloudflareCustomHostnameStatus)

	// 客戶地址管理 API（補充默認地址 API）
	api.Get("/customers/:customerId/addresses/default", handlers.GetCustomerDefaultAddress)

	// 租戶模塊管理 API
	api.Get("/tenant-modules", handlers.GetTenantModules)
	api.Get("/tenant-modules/:moduleCode", handlers.GetTenantModule)
	api.Put("/tenant-modules/:moduleCode", handlers.UpdateTenantModule)
	api.Post("/tenant-modules/batch", handlers.BatchUpdateTenantModules)
	api.Get("/tenant-modules/:moduleCode/check", handlers.IsModuleEnabled)

	// HR 模組 API
	// 打卡記錄
	api.Get("/attendances", handlers.GetAttendances)
	api.Get("/attendances/:id", handlers.GetAttendance)
	api.Get("/attendance-reports", handlers.GetAttendanceReports)
	api.Post("/attendances/clock-in", handlers.ClockIn)
	api.Post("/attendances/clock-out", handlers.ClockOut)
	api.Put("/attendances/:id", handlers.UpdateAttendance)

	// 請假申請
	api.Get("/leave-requests", handlers.GetLeaveRequests)
	api.Post("/leave-requests", handlers.CreateLeaveRequest)
	api.Post("/leave-requests/:id/approve", handlers.ApproveLeaveRequest) // 必须在 /leave-requests/:id 之前
	api.Post("/leave-requests/:id/reject", handlers.RejectLeaveRequest)   // 必须在 /leave-requests/:id 之前
	api.Get("/leave-requests/:id", handlers.GetLeaveRequest)
	api.Put("/leave-requests/:id", handlers.UpdateLeaveRequest)

	// 假期
	api.Get("/holidays", handlers.GetHolidays)
	api.Get("/holidays/:id", handlers.GetHoliday)
	api.Post("/holidays", handlers.CreateHoliday)
	api.Put("/holidays/:id", handlers.UpdateHoliday)
	api.Delete("/holidays/:id", handlers.DeleteHoliday)

	// 工作時段 API
	api.Get("/shifts", handlers.GetShifts)
	api.Get("/shifts/:id", handlers.GetShift)
	api.Post("/shifts", handlers.CreateShift)
	api.Put("/shifts/:id", handlers.UpdateShift)
	api.Delete("/shifts/:id", handlers.DeleteShift)

	// 薪資記錄
	api.Get("/payrolls", handlers.GetPayrolls)
	api.Get("/payrolls/:id", handlers.GetPayroll)
	api.Post("/payrolls", handlers.CreatePayroll)
	api.Put("/payrolls/:id", handlers.UpdatePayroll)
	api.Delete("/payrolls/:id", handlers.DeletePayroll)
	api.Post("/payrolls/generate-current-month", handlers.GenerateCurrentMonthPayrolls)

	// 薪資附加項目 presets
	api.Get("/payroll-adjustment-presets", handlers.GetPayrollAdjustmentPresets)
	api.Get("/payroll-adjustment-presets/:id", handlers.GetPayrollAdjustmentPreset)
	api.Post("/payroll-adjustment-presets", handlers.CreatePayrollAdjustmentPreset)
	api.Put("/payroll-adjustment-presets/:id", handlers.UpdatePayrollAdjustmentPreset)
	api.Delete("/payroll-adjustment-presets/:id", handlers.DeletePayrollAdjustmentPreset)

	// HR：空缺 / 聘請
	api.Get("/job-vacancies", handlers.GetJobVacancies)
	api.Get("/job-vacancies/:id", handlers.GetJobVacancy)
	api.Post("/job-vacancies", handlers.CreateJobVacancy)
	api.Put("/job-vacancies/:id", handlers.UpdateJobVacancy)
	api.Delete("/job-vacancies/:id", handlers.DeleteJobVacancy)

	// HR：求職者 / 候選人
	api.Get("/job-applicants", handlers.GetJobApplicants)
	api.Get("/job-applicants/:id", handlers.GetJobApplicant)
	api.Post("/job-applicants", handlers.CreateJobApplicant)
	api.Put("/job-applicants/:id", handlers.UpdateJobApplicant)
	api.Delete("/job-applicants/:id", handlers.DeleteJobApplicant)

	api.Get("/job-hires", handlers.GetJobHires)
	api.Get("/job-hires/:id", handlers.GetJobHire)
	api.Post("/job-hires", handlers.CreateJobHire)
	api.Put("/job-hires/:id", handlers.UpdateJobHire)
	api.Delete("/job-hires/:id", handlers.DeleteJobHire)

	// Reports
	api.Get("/reports/customer-analysis", handlers.GetCustomerAnalysisReport)
	api.Get("/reports/project-analysis", handlers.GetProjectAnalysisReport)
	// vBuilder：網站瀏覽報告（每頁瀏覽量）
	api.Get("/website/page-views", handlers.GetWebsitePageViews)

	// 庫存管理 API
	// 庫存調整
	// 出入貨記錄（從訂單和採購單中提取）
	api.Get("/inventory-movements", handlers.GetInventoryMovements)

	// 保留 inventory-adjustments API 以向後兼容（但不再使用）
	api.Get("/inventory-adjustments", handlers.GetInventoryAdjustments)
	api.Get("/inventory-adjustments/:id", handlers.GetInventoryAdjustment)
	api.Post("/inventory-adjustments", handlers.CreateInventoryAdjustment)

	// 庫存盤點
	api.Get("/inventory-counts", handlers.GetInventoryCounts)
	api.Get("/inventory-counts/:id", handlers.GetInventoryCount)
	api.Post("/inventory-counts", handlers.CreateInventoryCount)
	api.Put("/inventory-counts/:id", handlers.UpdateInventoryCount)
	api.Post("/inventory-counts/:count_id/items", handlers.AddInventoryCountItem)
	api.Delete("/inventory-count-items/:id", handlers.DeleteInventoryCountItem)

	// 庫存預警
	api.Get("/inventory/low-stock", handlers.GetLowStockProducts)
	api.Get("/inventory/low-stock/export/excel", handlers.ExportLowStockProductsToExcel)
	api.Get("/inventory/low-stock/export/pdf", handlers.ExportLowStockProductsToPDF)

	// 倉庫管理 API
	api.Get("/warehouses", handlers.GetWarehouses)
	api.Get("/warehouses/:id", handlers.GetWarehouse)
	api.Post("/warehouses", handlers.CreateWarehouse)
	api.Put("/warehouses/:id", handlers.UpdateWarehouse)
	api.Delete("/warehouses/:id", handlers.DeleteWarehouse)

	// 倉庫區管理 API
	api.Get("/warehouse-zones", handlers.GetWarehouseZones)
	api.Get("/warehouse-zones/:id", handlers.GetWarehouseZone)
	api.Post("/warehouse-zones", handlers.CreateWarehouseZone)
	api.Put("/warehouse-zones/:id", handlers.UpdateWarehouseZone)
	api.Delete("/warehouse-zones/:id", handlers.DeleteWarehouseZone)

	// 出入庫設定 API
	api.Get("/inventory-settings", handlers.GetInventorySettings)
	api.Put("/inventory-settings", handlers.UpdateInventorySettings)

	// 產品倉庫庫存 API
	api.Get("/product-warehouse-stocks", handlers.GetProductWarehouseStocks)
	api.Post("/product-warehouse-stocks", handlers.UpdateProductWarehouseStock)

	// 店舖管理 API
	api.Get("/stores", handlers.GetStores)
	api.Get("/stores/:id", handlers.GetStore)
	api.Post("/stores", handlers.CreateStore)
	api.Put("/stores/:id", handlers.UpdateStore)
	api.Delete("/stores/:id", handlers.DeleteStore)

	// 積分設置 API
	api.Get("/point-settings", handlers.GetPointSetting)
	api.Get("/point-settings/me", handlers.GetPointSetting)
	api.Put("/point-settings", handlers.UpdatePointSetting)
	api.Put("/point-settings/me", handlers.UpdatePointSetting)

	// 印花設定 API
	api.Get("/stamp-settings", handlers.GetStampSettings)
	api.Get("/stamp-settings/:id", handlers.GetStampSetting)
	api.Post("/stamp-settings", handlers.CreateStampSetting)
	api.Put("/stamp-settings/:id", handlers.UpdateStampSetting)
	api.Delete("/stamp-settings/:id", handlers.DeleteStampSetting)

	// 印花獲取產品 API
	api.Get("/stamp-earning-products", handlers.GetStampEarningProducts)
	api.Post("/stamp-earning-products", handlers.CreateStampEarningProduct)
	api.Delete("/stamp-earning-products/:id", handlers.DeleteStampEarningProduct)

	// 印花獲取服務 API
	api.Get("/stamp-earning-services", handlers.GetStampEarningServices)
	api.Post("/stamp-earning-services", handlers.CreateStampEarningService)
	api.Delete("/stamp-earning-services/:id", handlers.DeleteStampEarningService)

	// 印花可換購產品 API
	api.Get("/stamp-redeemable-products", handlers.GetStampRedeemableProducts)
	api.Get("/stamp-redeemable-products/:id", handlers.GetStampRedeemableProduct)
	api.Post("/stamp-redeemable-products", handlers.CreateStampRedeemableProduct)
	api.Put("/stamp-redeemable-products/:id", handlers.UpdateStampRedeemableProduct)
	api.Delete("/stamp-redeemable-products/:id", handlers.DeleteStampRedeemableProduct)

	// 印花記錄 API
	api.Get("/stamp-records", handlers.GetStampRecords)
	api.Get("/stamp-records/:id", handlers.GetStampRecord)
	api.Post("/stamp-records", handlers.CreateStampRecord)

	// 客戶印花餘額 API
	api.Get("/customer-stamp-balances", handlers.GetCustomerStampBalances)
	api.Get("/customer-stamps", handlers.GetCustomerStamps)
	api.Get("/stamp-redeemable-products-available", handlers.GetAvailableRedeemableProducts)
	api.Post("/stamp-redeem", handlers.RedeemStamps)

	// 加盟 API
	api.Get("/referrals", handlers.GetReferrals)

	// ============================================
	// 產品擴展模組 API
	// ============================================

	// 產品類型 API
	api.Get("/product-types", handlers.GetProductTypes)
	api.Get("/product-types/:id", handlers.GetProductType)
	api.Post("/product-types", handlers.CreateProductType)
	api.Put("/product-types/:id", handlers.UpdateProductType)
	api.Delete("/product-types/:id", handlers.DeleteProductType)

	// 產品屬性 API
	api.Get("/product-attributes", handlers.GetProductAttributes)
	api.Get("/product-attributes/:id", handlers.GetProductAttribute)
	api.Post("/product-attributes", handlers.CreateProductAttribute)
	api.Put("/product-attributes/:id", handlers.UpdateProductAttribute)
	api.Delete("/product-attributes/:id", handlers.DeleteProductAttribute)

	// 產品屬性值 API
	api.Get("/product-attribute-values", handlers.GetProductAttributeValues)
	api.Post("/product-attribute-values", handlers.CreateProductAttributeValue)
	api.Put("/product-attribute-values/:id", handlers.UpdateProductAttributeValue)
	api.Delete("/product-attribute-values/:id", handlers.DeleteProductAttributeValue)

	// 品牌 API
	api.Get("/brands", handlers.GetBrands)
	api.Get("/brands/:id", handlers.GetBrand)
	api.Post("/brands", handlers.CreateBrand)
	api.Put("/brands/:id", handlers.UpdateBrand)
	api.Delete("/brands/:id", handlers.DeleteBrand)

	// ============================================
	// 服務模組 API
	// ============================================

	// 服務種類 API
	api.Get("/service-types", handlers.GetServiceTypes)
	api.Get("/service-types/:id", handlers.GetServiceType)
	api.Post("/service-types", handlers.CreateServiceType)
	api.Put("/service-types/:id", handlers.UpdateServiceType)
	api.Delete("/service-types/:id", handlers.DeleteServiceType)

	// 服務 API
	api.Get("/services", handlers.GetServices)
	api.Get("/services/:id", handlers.GetService)
	api.Post("/services", handlers.CreateService)
	api.Put("/services/:id", handlers.UpdateService)
	api.Delete("/services/:id", handlers.DeleteService)

	// 服務稅
	api.Get("/service-taxes", handlers.GetServiceTaxes)
	api.Get("/service-taxes/:id", handlers.GetServiceTax)
	api.Post("/service-taxes", handlers.CreateServiceTax)
	api.Put("/service-taxes/:id", handlers.UpdateServiceTax)
	api.Delete("/service-taxes/:id", handlers.DeleteServiceTax)

	// 預約 API
	api.Get("/appointments", handlers.GetAppointments)
	api.Get("/appointments/:id", handlers.GetAppointment)
	api.Post("/appointments", handlers.CreateAppointment)
	api.Post("/appointments/check-conflict", handlers.CheckAppointmentConflict)
	api.Put("/appointments/:id", handlers.UpdateAppointment)
	api.Delete("/appointments/:id", handlers.DeleteAppointment)

	// 資源使用日曆（events）
	api.Get("/resource-usage-calendar/events", handlers.GetResourceUsageCalendarEvents)
	api.Get("/appointment-calendar/events", handlers.GetAppointmentCalendarEvents)

	// 服務單 API
	api.Get("/service-orders", handlers.GetServiceOrders)
	api.Get("/service-orders/import-template/excel", handlers.DownloadServiceOrdersImportTemplateExcel)
	api.Post("/service-orders/import/excel", handlers.ImportServiceOrdersFromExcel)
	api.Get("/service-orders/:id", handlers.GetServiceOrder)
	api.Post("/service-orders", handlers.CreateServiceOrder)
	api.Put("/service-orders/:id", handlers.UpdateServiceOrder)
	api.Delete("/service-orders/:id", handlers.DeleteServiceOrder)
	api.Get("/service-orders/:id/contract", handlers.GenerateServiceOrderContractPDF) // 生成服務單合約PDF

	// 出租單 API
	api.Get("/rental-orders", handlers.GetRentalOrders)
	api.Get("/rental-orders/:id", handlers.GetRentalOrder)
	api.Post("/rental-orders", handlers.CreateRentalOrder)
	api.Put("/rental-orders/:id", handlers.UpdateRentalOrder)
	api.Delete("/rental-orders/:id", handlers.DeleteRentalOrder)

	// 服務員 API
	api.Get("/service-staffs", handlers.GetServiceStaffs)
	api.Get("/service-staffs/:id", handlers.GetServiceStaff)
	api.Post("/service-staffs", handlers.CreateServiceStaff)
	api.Put("/service-staffs/:id", handlers.UpdateServiceStaff)
	api.Delete("/service-staffs/:id", handlers.DeleteServiceStaff)

	// 房間 API
	api.Get("/rooms", handlers.GetRooms)
	api.Get("/rooms/:id", handlers.GetRoom)
	api.Post("/rooms", handlers.CreateRoom)
	api.Put("/rooms/:id", handlers.UpdateRoom)
	api.Delete("/rooms/:id", handlers.DeleteRoom)

	// 設備 API
	api.Get("/equipments", handlers.GetEquipments)
	api.Get("/equipments/:id", handlers.GetEquipment)
	api.Post("/equipments", handlers.CreateEquipment)
	api.Put("/equipments/:id", handlers.UpdateEquipment)
	api.Delete("/equipments/:id", handlers.DeleteEquipment)

	// 車輛 API
	api.Get("/vehicles", handlers.GetVehicles)
	api.Get("/vehicles/:id", handlers.GetVehicle)
	api.Post("/vehicles", handlers.CreateVehicle)
	api.Put("/vehicles/:id", handlers.UpdateVehicle)
	api.Delete("/vehicles/:id", handlers.DeleteVehicle)

	// 項目管理
	api.Get("/projects", handlers.GetProjects)
	api.Get("/projects/:id", handlers.GetProject)
	api.Post("/projects", handlers.CreateProject)
	api.Put("/projects/:id", handlers.UpdateProject)
	api.Delete("/projects/:id", handlers.DeleteProject)

	// 項目類型
	api.Get("/project-types", handlers.GetProjectTypes)
	api.Get("/project-types/:id", handlers.GetProjectType)
	api.Post("/project-types", handlers.CreateProjectType)
	api.Put("/project-types/:id", handlers.UpdateProjectType)
	api.Delete("/project-types/:id", handlers.DeleteProjectType)

	// 項目檔案
	api.Get("/projects/:id/files", handlers.ListProjectFiles)
	api.Post("/projects/:id/files", handlers.CreateProjectFile)
	api.Delete("/projects/:id/files/:fileId", handlers.DeleteProjectFile)

	// 項目資源預留
	api.Get("/projects/:id/reservations", handlers.ListProjectReservations)
	api.Put("/projects/:id/reservations", handlers.ReplaceProjectReservations)

	// 頁面管理
	api.Get("/pages", handlers.GetPages)
	api.Get("/pages/:id", handlers.GetPage)
	api.Post("/pages", handlers.CreatePage)
	api.Put("/pages/:id", handlers.UpdatePage)
	api.Delete("/pages/:id", handlers.DeletePage)
	api.Get("/pages/:id/components", handlers.GetPageComponents)
	api.Put("/pages/:id/components", handlers.UpdatePageComponents)
	api.Post("/pages/create-default-ecommerce", handlers.CreateDefaultEcommercePages)
	api.Post("/pages/create-default-general", handlers.CreateDefaultGeneralPages)
	api.Post("/pages/create-default-dining", handlers.CreateDefaultDiningPages)
	api.Post("/pages/create-default-service", handlers.CreateDefaultServicePages)
	api.Post("/pages/reset", handlers.ResetWebsitePages)

	// Blog routes
	api.Get("/blogs", handlers.GetBlogs)
	api.Get("/blogs/:id", handlers.GetBlog)
	api.Post("/blogs", handlers.CreateBlog)
	api.Put("/blogs/:id", handlers.UpdateBlog)
	api.Delete("/blogs/:id", handlers.DeleteBlog)

	// 區塊管理
	api.Get("/blocks", handlers.GetBlocks)
	api.Get("/blocks/:id", handlers.GetBlock)
	api.Post("/blocks", handlers.CreateBlock)
	api.Put("/blocks/:id", handlers.UpdateBlock)
	api.Delete("/blocks/:id", handlers.DeleteBlock)

	// ============================================
	// 廣告模組 API
	// ============================================

	// 廣告位置 API
	api.Get("/ad-positions", handlers.GetAdPositions)
	api.Get("/ad-positions/:id", handlers.GetAdPosition)
	api.Post("/ad-positions", handlers.CreateAdPosition)
	api.Put("/ad-positions/:id", handlers.UpdateAdPosition)
	api.Delete("/ad-positions/:id", handlers.DeleteAdPosition)

	// 廣告 API
	api.Get("/ads", handlers.GetAds)
	api.Get("/ads/:id", handlers.GetAd)
	api.Post("/ads", handlers.CreateAd)
	api.Put("/ads/:id", handlers.UpdateAd)
	api.Delete("/ads/:id", handlers.DeleteAd)
	api.Put("/ads/sort-order", handlers.UpdateAdSortOrder)
	api.Post("/ads/upload", handlers.UploadAdMedia)

	// 輪播設定 API
	api.Get("/carousel/:ad_position_id/settings", handlers.GetCarouselSettings)
	api.Put("/carousel/:ad_position_id/settings", handlers.UpdateCarouselSettings)
	api.Get("/carousel/:ad_position_id/download", handlers.GenerateCarouselZip)

	// 下載 API
	api.Get("/downloads/connector", handlers.DownloadConnectorZip)

	// ============================================
	// 採購模組 API
	// ============================================

	// 採購單 API
	api.Get("/purchase-orders", handlers.GetPurchaseOrders)
	api.Get("/purchase-orders/last-quotation-price", handlers.GetLastSupplierQuotationPrice)
	api.Get("/purchase-orders/import-template/excel", handlers.DownloadPurchaseOrdersImportTemplateExcel)
	api.Post("/purchase-orders/import/excel", handlers.ImportPurchaseOrdersFromExcel)
	api.Get("/purchase-orders/:id", handlers.GetPurchaseOrder)
	api.Post("/purchase-orders", handlers.CreatePurchaseOrder)
	api.Put("/purchase-orders/:id", handlers.UpdatePurchaseOrder)
	api.Delete("/purchase-orders/:id", handlers.DeletePurchaseOrder)

	// 採購單明細 API
	api.Get("/purchase-order-items", handlers.GetPurchaseOrderItems)
	api.Get("/purchase-order-items/:id", handlers.GetPurchaseOrderItem)
	api.Post("/purchase-order-items", handlers.CreatePurchaseOrderItem)
	api.Put("/purchase-order-items/:id", handlers.UpdatePurchaseOrderItem)
	api.Delete("/purchase-order-items/:id", handlers.DeletePurchaseOrderItem)

	// ============================================
	// 客服模組 API
	// ============================================

	// 客服通訊 API
	api.Get("/support-communications", handlers.GetSupportCommunications)
	api.Get("/support-communications/:id", handlers.GetSupportCommunication)
	api.Post("/support-communications", handlers.CreateSupportCommunication)
	api.Put("/support-communications/:id", handlers.UpdateSupportCommunication)
	api.Put("/support-communications/:id/resolve", handlers.ResolveSupportCommunication)
	api.Delete("/support-communications/:id", handlers.DeleteSupportCommunication)

	// Email 配額查詢（CMS 用戶）
	api.Get("/email-quota", handlers.GetEmailQuota)

	// 推廣發送 API
	api.Get("/promotions", handlers.GetPromotions)
	api.Get("/promotions/:id", handlers.GetPromotion)
	api.Post("/promotions", handlers.CreatePromotion)
	api.Put("/promotions/:id", handlers.UpdatePromotion)
	api.Put("/promotions/:id/send", handlers.SendPromotion)
	api.Delete("/promotions/:id", handlers.DeletePromotion)
	api.Post("/google-ads/connect-url", handlers.GetGoogleAdsConnectURL)
	api.Post("/google-ads/disconnect", handlers.DisconnectGoogleAds)
	api.Post("/google-ads/generate", handlers.GenerateGoogleAd)

	// ============================================
	// POS模組 API
	// ============================================

	// POS銷售 API
	api.Get("/pos-sales", handlers.GetPOSSales)
	api.Get("/pos-sales/:id", handlers.GetPOSSale)
	api.Post("/pos-sales", handlers.CreatePOSSale)
	api.Put("/pos-sales/:id", handlers.UpdatePOSSale)
	api.Delete("/pos-sales/:id", handlers.DeletePOSSale)

	// POS銷售明細 API
	api.Get("/pos-sale-items", handlers.GetPOSSaleItems)
	api.Post("/pos-sale-items", handlers.CreatePOSSaleItem)
	api.Put("/pos-sale-items/:id", handlers.UpdatePOSSaleItem)
	api.Delete("/pos-sale-items/:id", handlers.DeletePOSSaleItem)

	// POS支付 API
	api.Get("/pos-payments", handlers.GetPOSPayments)
	api.Get("/pos-payments/:id", handlers.GetPOSPayment)
	api.Post("/pos-payments", handlers.CreatePOSPayment)
	api.Put("/pos-payments/:id", handlers.UpdatePOSPayment)
	api.Delete("/pos-payments/:id", handlers.DeletePOSPayment)

	// ============================================
	// 訂單報表 API
	// ============================================

	api.Get("/order-reports", handlers.GetOrderReports)
	api.Get("/order-reports/:id", handlers.GetOrderReport)
	api.Post("/order-reports", handlers.CreateOrderReport)
	api.Put("/order-reports/:id", handlers.UpdateOrderReport)
	api.Delete("/order-reports/:id", handlers.DeleteOrderReport)

	// 會計模組 API
	api.Get("/incomes", handlers.GetIncomes)
	api.Get("/incomes/:id", handlers.GetIncome)
	api.Post("/incomes", handlers.CreateIncome)
	api.Put("/incomes/:id", handlers.UpdateIncome)
	api.Delete("/incomes/:id", handlers.DeleteIncome)

	api.Get("/expenses", handlers.GetExpenses)
	api.Get("/expenses/:id", handlers.GetExpense)
	api.Post("/expenses", handlers.CreateExpense)
	api.Post("/expenses/generate-monthly-commissions", handlers.GenerateMonthlyCommissions)
	api.Put("/expenses/:id", handlers.UpdateExpense)
	api.Delete("/expenses/:id", handlers.DeleteExpense)
	// 支出申請
	api.Get("/expense-requests", handlers.GetExpenseRequests)
	api.Post("/expense-requests", handlers.CreateExpenseRequest)
	api.Post("/expense-requests/:id/approve", handlers.ApproveExpenseRequest)
	api.Post("/expense-requests/:id/reject", handlers.RejectExpenseRequest)

	api.Get("/accounting/summary", handlers.GetAccountingSummary)
	api.Get("/accounting/reports/:type", handlers.GetAccountingReport)
	api.Get("/accounting/reports/:type/export/excel", handlers.ExportAccountingReportExcel)
	api.Get("/accounting/reports/:type/export/pdf", handlers.ExportAccountingReportPDF)

	api.Get("/accounts", handlers.GetAccounts)
	api.Get("/accounts/:id", handlers.GetAccount)
	api.Post("/accounts", handlers.CreateAccount)
	api.Put("/accounts/:id", handlers.UpdateAccount)
	api.Delete("/accounts/:id", handlers.DeleteAccount)
	api.Post("/accounts/initialize-default", handlers.InitializeDefaultAccounts)

	api.Get("/journal-entries", handlers.GetJournalEntries)
	api.Get("/journal-entries/:id", handlers.GetJournalEntry)
	api.Post("/journal-entries", handlers.CreateJournalEntry)
	api.Put("/journal-entries/:id", handlers.UpdateJournalEntry)
	api.Delete("/journal-entries/:id", handlers.DeleteJournalEntry)

	api.Get("/tax-configs", handlers.GetTaxConfigs)
	api.Get("/tax-configs/:id", handlers.GetTaxConfig)
	api.Post("/tax-configs", handlers.CreateTaxConfig)
	api.Put("/tax-configs/:id", handlers.UpdateTaxConfig)
	api.Delete("/tax-configs/:id", handlers.DeleteTaxConfig)

	api.Get("/posting-rules", handlers.GetPostingRules)
	api.Get("/posting-rules/:id", handlers.GetPostingRule)
	api.Post("/posting-rules", handlers.CreatePostingRule)
	api.Put("/posting-rules/:id", handlers.UpdatePostingRule)
	api.Delete("/posting-rules/:id", handlers.DeletePostingRule)

	api.Get("/accounting/setup-status", handlers.GetAccountingSetupStatus)
	api.Post("/accounting/backfill", handlers.BackfillAccountingEntries)

	// 文件上傳 API
	api.Post("/upload", handlers.UploadFile)
	// 圖片裁剪 API
	api.Post("/crop", handlers.CropImage)

	// Lead Finder API（自動搵客系統）
	api.Post("/lead-finder/analyze", middleware.RequireLeadFinder(), middleware.RequireAICoins(models.AICoinsLeadAnalyze, "Lead Finder AI 分析"), handlers.LeadFinderAnalyze)
	api.Post("/lead-finder/search", middleware.RequireLeadFinder(), handlers.LeadFinderSearch)
	api.Get("/lead-finder/searches", middleware.RequireLeadFinder(), handlers.LeadFinderGetSearches)
	api.Delete("/lead-finder/searches/:id", middleware.RequireLeadFinder(), handlers.LeadFinderDeleteSearch)
	api.Get("/lead-finder/searches/:id/results", middleware.RequireLeadFinder(), handlers.LeadFinderGetResults)
	// Lead 列表 API（所有結果匯總）— 必須在 /lead-finder/results/:id 之前註冊
	api.Get("/lead-finder/results", middleware.RequireLeadFinder(), handlers.LeadFinderGetAllResults)
	api.Put("/lead-finder/results/:id", middleware.RequireLeadFinder(), handlers.LeadFinderUpdateResult)
	api.Delete("/lead-finder/results/:id", middleware.RequireLeadFinder(), handlers.LeadFinderDeleteResult)
	api.Post("/lead-finder/results/:id/convert", middleware.RequireLeadFinder(), handlers.LeadFinderConvertToCustomer)
	api.Get("/lead-finder/searches/:id/export/excel", middleware.RequireLeadFinder(), handlers.LeadFinderExportExcel)
	api.Get("/lead-finder/searches/:id/export/pdf", middleware.RequireLeadFinder(), handlers.LeadFinderExportPDF)

	// Auto Outreach API
	api.Get("/auto-outreach", handlers.GetAutoOutreachCampaigns)
	api.Get("/auto-outreach/:id", handlers.GetAutoOutreachCampaign)
	api.Post("/auto-outreach", handlers.CreateAutoOutreachCampaign)
	api.Put("/auto-outreach/:id", handlers.UpdateAutoOutreachCampaign)
	api.Put("/auto-outreach/:id/toggle", handlers.ToggleAutoOutreachCampaign)
	api.Delete("/auto-outreach/:id", handlers.DeleteAutoOutreachCampaign)
	api.Get("/auto-outreach/:id/logs", handlers.GetAutoOutreachLogs)
	api.Post("/auto-outreach/generate-content", handlers.AutoOutreachGenerateContent)

	// 啟動服務器
	addr := cfg.Server.Host + ":" + cfg.Server.Port
	log.Printf("🚀 vWork server starting on http://%s", addr)

	// 打印所有 CMS 路由（調試用）
	log.Println("📋 All CMS routes:")
	routes := app.GetRoutes(true)
	warehousesFound := false
	expenseRequestsFound := false
	customersFound := false
	orderLabelsFound := false
	paymentMethodsFound := false
	shippingMethodsFound := false
	logisticsCompaniesFound := false
	for _, route := range routes {
		// 只打印 CMS 路由（不是 API 路由）
		if route.Path != "" && !strings.HasPrefix(route.Path, "/api") &&
			!strings.HasPrefix(route.Path, "/static") &&
			!strings.HasPrefix(route.Path, "/uploads") &&
			route.Method == "GET" {
			log.Printf("  %s %s", route.Method, route.Path)
			if route.Path == "/warehouses" {
				warehousesFound = true
			}
			if route.Path == "/expense-requests" {
				expenseRequestsFound = true
			}
			if route.Path == "/customers" {
				customersFound = true
			}
			if route.Path == "/order-labels" {
				orderLabelsFound = true
			}
			if route.Path == "/payment-methods" {
				paymentMethodsFound = true
			}
			if route.Path == "/shipping-methods" {
				shippingMethodsFound = true
			}
			if route.Path == "/logistics-companies" {
				logisticsCompaniesFound = true
			}
			if route.Path == "/quotations" {
				log.Printf("✅ Found /quotations route!")
			}
			if route.Path == "/website-setup-guide" {
				log.Printf("✅ Found /website-setup-guide route!")
			}
			if route.Path == "/website-theme" {
				log.Printf("✅ Found /website-theme route!")
			}
		}
	}
	log.Printf("📊 Summary: customers=%v, warehouses=%v, expense-requests=%v, order-labels=%v, payment-methods=%v, shipping-methods=%v, logistics-companies=%v",
		customersFound, warehousesFound, expenseRequestsFound, orderLabelsFound, paymentMethodsFound, shippingMethodsFound, logisticsCompaniesFound)

	// Sales partner landing page — catch-all route (must be LAST)
	// Matches /:code for approved partner codes; redirects to / if not found
	app.Get("/:code", handlers.SalesPartnerLandingPage)

	log.Fatal(app.Listen(addr))
}
