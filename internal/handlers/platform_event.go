package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// ── vWorkAdmin: Platform Event CRUD ──

// VWorkAdminGetEvents lists all platform events (admin, supports search/filter/pagination)
func VWorkAdminGetEvents(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	var events []models.PlatformEvent
	var total int64

	query := database.DB.Model(&models.PlatformEvent{})

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	if lang := c.Query("lang"); lang != "" {
		query = query.Where("lang = ?", lang)
	}
	if search := c.Query("search"); search != "" {
		query = query.Where("title ILIKE ?", "%"+search+"%")
	}

	query.Count(&total)

	if err := query.Order("sort_order ASC, created_at DESC").
		Offset(offset).Limit(limit).
		Find(&events).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch events"})
	}

	// Populate registration counts
	for i := range events {
		var count int64
		database.DB.Model(&models.PlatformEventRegistration{}).
			Where("event_id = ?", events[i].ID).Count(&count)
		events[i].RegistrationCount = int(count)
	}

	return c.JSON(fiber.Map{
		"data":  events,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// VWorkAdminGetEvent returns a single platform event by ID
func VWorkAdminGetEvent(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	var event models.PlatformEvent
	if err := database.DB.Where("id = ?", id).First(&event).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Event not found"})
	}

	// Populate registration count
	var count int64
	database.DB.Model(&models.PlatformEventRegistration{}).
		Where("event_id = ?", event.ID).Count(&count)
	event.RegistrationCount = int(count)

	return c.JSON(event)
}

// VWorkAdminCreateEvent creates a new platform event
func VWorkAdminCreateEvent(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	var event models.PlatformEvent
	if err := c.BodyParser(&event); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if event.Title == "" {
		return c.Status(400).JSON(fiber.Map{"error": "title is required"})
	}

	// Auto-generate slug if empty
	if event.Slug == "" {
		event.Slug = generateSlug(event.Title)
	}
	event.Slug = strings.ToLower(strings.TrimSpace(event.Slug))

	event.Lang = normalizePlatformBlogLang(event.Lang)

	// Ensure slug uniqueness within language
	var slugCount int64
	database.DB.Model(&models.PlatformEvent{}).Where("slug = ? AND lang = ?", event.Slug, event.Lang).Count(&slugCount)
	if slugCount > 0 {
		event.Slug = fmt.Sprintf("%s-%d", event.Slug, time.Now().UnixMilli())
	}

	// Default values
	if event.Status == "" {
		event.Status = "draft"
	}
	if event.Lang == "" {
		event.Lang = "zh"
	}

	// Auto-set published_at when publishing
	if event.Status == "published" && event.PublishedAt == nil {
		now := time.Now()
		event.PublishedAt = &now
	}

	if err := database.DB.Create(&event).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create event: " + err.Error()})
	}

	return c.Status(201).JSON(event)
}

// VWorkAdminUpdateEvent updates an existing platform event
func VWorkAdminUpdateEvent(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	var event models.PlatformEvent
	if err := database.DB.Where("id = ?", id).First(&event).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Event not found"})
	}

	oldStatus := event.Status

	if err := c.BodyParser(&event); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	event.Lang = normalizePlatformBlogLang(event.Lang)

	// Auto-set published_at when transitioning to published
	if event.Status == "published" && oldStatus != "published" && event.PublishedAt == nil {
		now := time.Now()
		event.PublishedAt = &now
	}

	if err := database.DB.Save(&event).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update event: " + err.Error()})
	}

	return c.JSON(event)
}

// VWorkAdminDeleteEvent deletes a platform event and its registrations
func VWorkAdminDeleteEvent(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")

	// Delete registrations first
	database.DB.Where("event_id = ?", id).Delete(&models.PlatformEventRegistration{})

	if err := database.DB.Where("id = ?", id).Delete(&models.PlatformEvent{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete event: " + err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Event deleted"})
}

// VWorkAdminGetEventRegistrations returns all registrations for an event
func VWorkAdminGetEventRegistrations(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
	}

	eventID := c.Params("id")
	var registrations []models.PlatformEventRegistration
	if err := database.DB.Where("event_id = ?", eventID).
		Order("created_at DESC").
		Find(&registrations).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch registrations"})
	}

	return c.JSON(fiber.Map{"data": registrations})
}

// ── Public: Platform Events (vWork Official Website) ──

// PublicPlatformEventList returns published platform events as JSON
func PublicPlatformEventList(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 12)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 12
	}
	offset := (page - 1) * limit

	var events []models.PlatformEvent
	var total int64

	query := database.DB.Model(&models.PlatformEvent{}).
		Where("status = ?", "published")

	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}
	query = query.Where("lang = ?", resolvePlatformBlogLang(c))

	query.Count(&total)

	if err := query.Order("sort_order ASC, event_date DESC").
		Offset(offset).Limit(limit).
		Find(&events).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch events"})
	}

	// Populate registration counts
	for i := range events {
		var count int64
		database.DB.Model(&models.PlatformEventRegistration{}).
			Where("event_id = ?", events[i].ID).Count(&count)
		events[i].RegistrationCount = int(count)
	}

	return c.JSON(fiber.Map{
		"data":  events,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// RenderPlatformEventList renders the event listing page (SSR)
func RenderPlatformEventList(c *fiber.Ctx) error {
	return c.Render("pages/vwork_events_list", fiber.Map{
		"PageTitle": "活動 | vWork",
	})
}

// RenderPlatformEventDetail renders a single event detail page (SSR with SEO)
func RenderPlatformEventDetail(c *fiber.Ctx) error {
	slug := c.Params("slug")
	if slug == "" {
		return c.Redirect("/vwork-events")
	}

	lang := resolvePlatformBlogLang(c)

	var event models.PlatformEvent
	if err := database.DB.Where("slug = ? AND status = ? AND lang = ?", slug, "published", lang).First(&event).Error; err != nil {
		// Fallback: try to find the same slug in any language
		var fallback models.PlatformEvent
		if fbErr := database.DB.Where("slug = ? AND status = ?", slug, "published").First(&fallback).Error; fbErr == nil {
			return c.Redirect("/vwork-events/" + slug + "?lang=" + fallback.Lang)
		}
		return c.Status(404).Render("pages/vwork_events_list", fiber.Map{
			"PageTitle": "活動 | vWork",
			"Error":     "Event not found",
		})
	}

	// Increment view count
	database.DB.Model(&event).UpdateColumn("view_count", event.ViewCount+1)

	// Registration count
	var regCount int64
	database.DB.Model(&models.PlatformEventRegistration{}).
		Where("event_id = ?", event.ID).Count(&regCount)
	event.RegistrationCount = int(regCount)

	// Check if registration is still open
	registrationOpen := true
	if event.MaxAttendees > 0 && int(regCount) >= event.MaxAttendees {
		registrationOpen = false
	}
	if event.Status == "cancelled" || event.Status == "archived" {
		registrationOpen = false
	}

	seoTitle := event.Title + " | vWork 活動"
	seoDesc := ""
	if event.Excerpt != nil {
		seoDesc = *event.Excerpt
	}

	return c.Render("pages/vwork_event_detail", fiber.Map{
		"Event":            event,
		"PageTitle":        seoTitle,
		"SEODesc":          seoDesc,
		"CurrentLang":      lang,
		"RegistrationOpen": registrationOpen,
	})
}

// PublicRegisterForEvent handles public event registration
func PublicRegisterForEvent(c *fiber.Ctx) error {
	slug := c.Params("slug")

	// Find the event
	var event models.PlatformEvent
	if err := database.DB.Where("slug = ? AND status = ?", slug, "published").First(&event).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Event not found"})
	}

	// Check capacity
	if event.MaxAttendees > 0 {
		var count int64
		database.DB.Model(&models.PlatformEventRegistration{}).
			Where("event_id = ?", event.ID).Count(&count)
		if int(count) >= event.MaxAttendees {
			return c.Status(400).JSON(fiber.Map{"error": "Event is full"})
		}
	}

	var reg models.PlatformEventRegistration
	if err := c.BodyParser(&reg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if reg.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if reg.Phone == "" {
		return c.Status(400).JSON(fiber.Map{"error": "phone is required"})
	}

	reg.EventID = event.ID

	// Check for duplicate registration (same phone + event)
	var existing int64
	database.DB.Model(&models.PlatformEventRegistration{}).
		Where("event_id = ? AND phone = ?", event.ID, reg.Phone).Count(&existing)
	if existing > 0 {
		return c.Status(400).JSON(fiber.Map{"error": "You have already registered for this event"})
	}

	if err := database.DB.Create(&reg).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to register: " + err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Registration successful",
		"data":    reg,
	})
}

// ── Render static pages (industry + customization) ──

func RenderIndustryPage(c *fiber.Ctx) error {
	industry := c.Params("industry")
	templateMap := map[string]string{
		"catering":  "pages/industry_catering",
		"retail":    "pages/industry_retail",
		"ecommerce": "pages/industry_ecommerce",
		"services":  "pages/industry_services",
		"wholesale": "pages/industry_wholesale",
		"sme":       "pages/industry_sme",
	}

	titleMap := map[string]string{
		"catering":  "餐飲業解決方案 | vWork",
		"retail":    "零售業解決方案 | vWork",
		"ecommerce": "電商解決方案 | vWork",
		"services":  "服務業解決方案 | vWork",
		"wholesale": "批發/製造業解決方案 | vWork",
		"sme":       "中小企業解決方案 | vWork",
	}

	tmpl, ok := templateMap[industry]
	if !ok {
		return c.Status(404).SendString("Page not found")
	}

	return c.Render(tmpl, fiber.Map{
		"PageTitle":     titleMap[industry],
		"WhatsAppPhone": getWhatsAppPhone(),
	})
}

func RenderCustomPage(c *fiber.Ctx) error {
	page := c.Params("page")
	templateMap := map[string]string{
		"website":  "pages/custom_website",
		"features": "pages/custom_features",
	}

	titleMap := map[string]string{
		"website":  "客制化網站 | vWork",
		"features": "客制化功能 | vWork",
	}

	tmpl, ok := templateMap[page]
	if !ok {
		return c.Status(404).SendString("Page not found")
	}

	return c.Render(tmpl, fiber.Map{
		"PageTitle":     titleMap[page],
		"WhatsAppPhone": getWhatsAppPhone(),
	})
}

// getWhatsAppPhone returns the configured WhatsApp phone number
func getWhatsAppPhone() string {
	// Use the same config pattern as the rest of the app
	return "85246237234"
}

// ── Auto-migrate helper ──

func AutoMigrateEvents(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.PlatformEvent{},
		&models.PlatformEventRegistration{},
		&models.AutoOutreachCampaign{},
		&models.AutoOutreachLog{},
	)
}
