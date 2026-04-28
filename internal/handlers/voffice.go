package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ─── Public API (called by vOffice client) ──────────────────────────────────

// checkUpdateReq is the unified request for check-update + installation upsert
type checkUpdateReq struct {
	// Installation info (for upsert)
	MachineID        string `json:"machine_id"`
	AppVersion       string `json:"app_version"`
	BuildNumber      string `json:"build_number"`
	UpdateChannel    string `json:"update_channel"`
	OSType           string `json:"os_type"`
	OSVersion        string `json:"os_version"`
	OSArch           string `json:"os_arch"`
	CPUModel         string `json:"cpu_model"`
	CPUCores         int    `json:"cpu_cores"`
	RAMGB            int    `json:"ram_gb"`
	ScreenResolution string `json:"screen_resolution"`
	DisplayCount     int    `json:"display_count"`
	Language         string `json:"language"`

	// Version check params
	Platform       string `json:"platform"`
	Channel        string `json:"channel"`
	CurrentVersion string `json:"current_version"`
}

// upsertInstallation creates or updates a vOffice installation record.
// Returns the installation record.
func upsertInstallation(req *checkUpdateReq, ip string, tenantID *uuid.UUID, userID *uuid.UUID) {
	if req.MachineID == "" {
		return
	}

	now := time.Now()

	var install models.VOfficeInstallation
	err := database.DB.Where("machine_id = ?", req.MachineID).First(&install).Error

	if err != nil {
		// New installation
		install = models.VOfficeInstallation{
			MachineID:        req.MachineID,
			AppVersion:       req.AppVersion,
			BuildNumber:      req.BuildNumber,
			UpdateChannel:    req.UpdateChannel,
			OSType:           req.OSType,
			OSVersion:        req.OSVersion,
			OSArch:           req.OSArch,
			CPUModel:         req.CPUModel,
			CPUCores:         req.CPUCores,
			RAMGB:            req.RAMGB,
			ScreenResolution: req.ScreenResolution,
			DisplayCount:     req.DisplayCount,
			IPAddress:        ip,
			Language:         req.Language,
			IsActive:         true,
			FirstSeenAt:      now,
			LastSeenAt:       now,
		}
		if tenantID != nil && *tenantID != uuid.Nil {
			install.TenantID = tenantID
		}
		if userID != nil && *userID != uuid.Nil {
			install.UserID = userID
		}
		if err := database.DB.Create(&install).Error; err != nil {
			log.Printf("upsertInstallation create error: %v", err)
		}
	} else {
		// Update existing
		updates := map[string]interface{}{
			"app_version":       req.AppVersion,
			"build_number":      req.BuildNumber,
			"os_type":           req.OSType,
			"os_version":        req.OSVersion,
			"os_arch":           req.OSArch,
			"cpu_model":         req.CPUModel,
			"cpu_cores":         req.CPUCores,
			"ram_gb":            req.RAMGB,
			"screen_resolution": req.ScreenResolution,
			"display_count":     req.DisplayCount,
			"ip_address":        ip,
			"language":          req.Language,
			"is_active":         true,
			"last_seen_at":      now,
			"updated_at":        now,
		}
		if req.UpdateChannel != "" {
			updates["update_channel"] = req.UpdateChannel
		}
		if tenantID != nil && *tenantID != uuid.Nil {
			updates["tenant_id"] = *tenantID
			updates["last_login_at"] = now
		}
		if userID != nil && *userID != uuid.Nil {
			updates["user_id"] = *userID
			updates["last_login_at"] = now
		}
		database.DB.Model(&install).Updates(updates)
	}
}

// getClientIP extracts IP from X-Forwarded-For or c.IP()
func getClientIP(c *fiber.Ctx) string {
	ip := c.IP()
	if forwarded := c.Get("X-Forwarded-For"); forwarded != "" {
		ip = strings.Split(forwarded, ",")[0]
		ip = strings.TrimSpace(ip)
	}
	return ip
}

// checkLatestRelease looks up the latest release for a given platform+channel
func checkLatestRelease(platform, channel, currentVersion string) fiber.Map {
	if platform == "" {
		platform = "windows"
	}
	if channel == "" {
		channel = "stable"
	}

	var release models.VOfficeRelease
	err := database.DB.Where("platform = ? AND channel = ? AND is_latest = true", platform, channel).
		First(&release).Error

	if err != nil {
		return fiber.Map{
			"has_update": false,
			"message":    "No update available",
		}
	}

	hasUpdate := currentVersion != "" && release.Version != currentVersion

	return fiber.Map{
		"has_update":      hasUpdate,
		"latest_version":  release.Version,
		"current_version": currentVersion,
		"download_url":    release.DownloadURL,
		"file_size":       release.FileSize,
		"checksum":        release.Checksum,
		"release_notes":   release.ReleaseNotes,
		"is_mandatory":    release.IsMandatory,
		"published_at":    release.PublishedAt,
	}
}

// VOfficeLatestRelease returns the latest release info for all platforms.
// Used by the voffice.vsysai.com landing page and /voffice-download page
// to dynamically show download links. No auth required.
// GET /api/v1/voffice/latest-release
func VOfficeLatestRelease(c *fiber.Ctx) error {
	// Optionally filter by platform
	platform := c.Query("platform", "")

	var releases []models.VOfficeRelease
	query := database.DB.Where("is_latest = true")
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	query.Order("platform ASC").Find(&releases)

	// Build a map by platform for easy frontend consumption
	byPlatform := make(map[string]fiber.Map)
	for _, r := range releases {
		byPlatform[r.Platform] = fiber.Map{
			"version":       r.Version,
			"download_url":  r.DownloadURL,
			"file_size":     r.FileSize,
			"checksum":      r.Checksum,
			"release_notes": r.ReleaseNotes,
			"is_mandatory":  r.IsMandatory,
			"published_at":  r.PublishedAt,
			"channel":       r.Channel,
		}
	}

	return c.JSON(fiber.Map{
		"releases": byPlatform,
	})
}

// VOfficeCheckUpdate handles version check + installation upsert (no auth).
// Called on vOffice app launch before login.
// POST /api/v1/voffice/check-update
func VOfficeCheckUpdate(c *fiber.Ctx) error {
	var req checkUpdateReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// Upsert installation info (best-effort, don't fail on error)
	ip := getClientIP(c)
	upsertInstallation(&req, ip, nil, nil)

	// Use request fields for version check, with fallbacks
	platform := req.Platform
	if platform == "" {
		platform = req.OSType
	}
	channel := req.Channel
	if channel == "" {
		channel = req.UpdateChannel
	}
	currentVersion := req.CurrentVersion
	if currentVersion == "" {
		currentVersion = req.AppVersion
	}

	resp := checkLatestRelease(platform, channel, currentVersion)
	return c.JSON(resp)
}

// VOfficeCheckUpdateAuth handles version check + installation upsert with JWT (after login).
// Binds tenant_id + user_id to the installation.
// POST /api/v1/voffice/check-update/auth
func VOfficeCheckUpdateAuth(c *fiber.Ctx) error {
	var req checkUpdateReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	tenantID, _ := c.Locals("tenant_id").(uuid.UUID)
	userID, _ := c.Locals("user_id").(uuid.UUID)

	// Upsert installation info with tenant/user binding
	ip := getClientIP(c)
	upsertInstallation(&req, ip, &tenantID, &userID)

	// Use request fields for version check, with fallbacks
	platform := req.Platform
	if platform == "" {
		platform = req.OSType
	}
	channel := req.Channel
	if channel == "" {
		channel = req.UpdateChannel
	}
	currentVersion := req.CurrentVersion
	if currentVersion == "" {
		currentVersion = req.AppVersion
	}

	resp := checkLatestRelease(platform, channel, currentVersion)
	return c.JSON(resp)
}

// ─── vWorkAdmin API (super admin only) ──────────────────────────────────────

// VWorkAdminVOfficeStats vOffice 安裝統計總覽
// GET /api/v1/vworkadmin/voffice-stats
func VWorkAdminVOfficeStats(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Forbidden"})
	}
	type stats struct {
		TotalInstalls    int64 `json:"total_installs"`
		ActiveInstalls   int64 `json:"active_installs"`
		TodayNew         int64 `json:"today_new"`
		WeekNew          int64 `json:"week_new"`
		LoggedInInstalls int64 `json:"logged_in_installs"`
	}

	var s stats
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -7)

	database.DB.Model(&models.VOfficeInstallation{}).Count(&s.TotalInstalls)
	database.DB.Model(&models.VOfficeInstallation{}).Where("is_active = true").Count(&s.ActiveInstalls)
	database.DB.Model(&models.VOfficeInstallation{}).Where("first_seen_at >= ?", todayStart).Count(&s.TodayNew)
	database.DB.Model(&models.VOfficeInstallation{}).Where("first_seen_at >= ?", weekStart).Count(&s.WeekNew)
	database.DB.Model(&models.VOfficeInstallation{}).Where("tenant_id IS NOT NULL").Count(&s.LoggedInInstalls)

	// OS breakdown
	type osCount struct {
		OSType string `json:"os_type"`
		Count  int64  `json:"count"`
	}
	var osCounts []osCount
	database.DB.Model(&models.VOfficeInstallation{}).
		Select("os_type, count(*) as count").
		Group("os_type").
		Order("count desc").
		Scan(&osCounts)

	// Version breakdown
	type versionCount struct {
		AppVersion string `json:"app_version"`
		Count      int64  `json:"count"`
	}
	var versionCounts []versionCount
	database.DB.Model(&models.VOfficeInstallation{}).
		Select("app_version, count(*) as count").
		Group("app_version").
		Order("count desc").
		Scan(&versionCounts)

	// Country breakdown
	type countryCount struct {
		Country string `json:"country"`
		Count   int64  `json:"count"`
	}
	var countryCounts []countryCount
	database.DB.Model(&models.VOfficeInstallation{}).
		Where("country != ''").
		Select("country, count(*) as count").
		Group("country").
		Order("count desc").
		Limit(20).
		Scan(&countryCounts)

	return c.JSON(fiber.Map{
		"stats":     s,
		"os":        osCounts,
		"versions":  versionCounts,
		"countries": countryCounts,
	})
}

// VWorkAdminVOfficeInstallations 安裝列表（分頁）
// GET /api/v1/vworkadmin/voffice-installations?page=1&limit=50
func VWorkAdminVOfficeInstallations(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Forbidden"})
	}
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 50)
	if page < 1 {
		page = 1
	}
	if limit > 200 {
		limit = 200
	}
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.VOfficeInstallation{}).Count(&total)

	var installs []models.VOfficeInstallation
	database.DB.Order("last_seen_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&installs)

	return c.JSON(fiber.Map{
		"installations": installs,
		"total":         total,
		"page":          page,
		"limit":         limit,
	})
}

// VWorkAdminVOfficeReleases 版本列表
// GET /api/v1/vworkadmin/voffice-releases
func VWorkAdminVOfficeReleases(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Forbidden"})
	}
	var releases []models.VOfficeRelease
	database.DB.Order("published_at DESC").Find(&releases)
	return c.JSON(fiber.Map{"releases": releases})
}

// VWorkAdminCreateVOfficeRelease 新增版本
// POST /api/v1/vworkadmin/voffice-releases
func VWorkAdminCreateVOfficeRelease(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Forbidden"})
	}
	var release models.VOfficeRelease
	if err := c.BodyParser(&release); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if release.Version == "" || release.Platform == "" || release.DownloadURL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "version, platform, download_url are required"})
	}

	// If marking as latest, unmark others
	if release.IsLatest {
		database.DB.Model(&models.VOfficeRelease{}).
			Where("platform = ? AND channel = ? AND is_latest = true", release.Platform, release.Channel).
			Update("is_latest", false)
	}

	if err := database.DB.Create(&release).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create release"})
	}

	return c.Status(201).JSON(release)
}

// VWorkAdminUpdateVOfficeRelease 更新版本
// PUT /api/v1/vworkadmin/voffice-releases/:id
func VWorkAdminUpdateVOfficeRelease(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Forbidden"})
	}
	id := c.Params("id")
	releaseID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid release ID"})
	}

	var release models.VOfficeRelease
	if err := database.DB.Where("id = ?", releaseID).First(&release).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Release not found"})
	}

	type updateReq struct {
		DownloadURL  *string `json:"download_url"`
		ReleaseNotes *string `json:"release_notes"`
		IsMandatory  *bool   `json:"is_mandatory"`
		IsLatest     *bool   `json:"is_latest"`
		FileSize     *int64  `json:"file_size"`
		Checksum     *string `json:"checksum"`
	}

	var req updateReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.DownloadURL != nil {
		release.DownloadURL = *req.DownloadURL
	}
	if req.ReleaseNotes != nil {
		release.ReleaseNotes = *req.ReleaseNotes
	}
	if req.IsMandatory != nil {
		release.IsMandatory = *req.IsMandatory
	}
	if req.FileSize != nil {
		release.FileSize = *req.FileSize
	}
	if req.Checksum != nil {
		release.Checksum = *req.Checksum
	}
	if req.IsLatest != nil && *req.IsLatest {
		// Unmark others, then mark this one
		database.DB.Model(&models.VOfficeRelease{}).
			Where("platform = ? AND channel = ? AND is_latest = true AND id != ?", release.Platform, release.Channel, release.ID).
			Update("is_latest", false)
		release.IsLatest = true
	}

	database.DB.Save(&release)
	return c.JSON(release)
}

// VWorkAdminDeleteVOfficeRelease 刪除版本
// DELETE /api/v1/vworkadmin/voffice-releases/:id
func VWorkAdminDeleteVOfficeRelease(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Forbidden"})
	}
	id := c.Params("id")
	releaseID, err := uuid.Parse(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid release ID"})
	}

	if err := database.DB.Where("id = ?", releaseID).Delete(&models.VOfficeRelease{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete release"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// VWorkAdminUploadVOfficeRelease handles file upload for vOffice release installers.
// The file is saved to web/uploads/voffice-releases/<platform>/<filename> and
// served publicly via /uploads/voffice-releases/...
// POST /api/v1/vworkadmin/voffice-releases/upload
func VWorkAdminUploadVOfficeRelease(c *fiber.Ctx) error {
	if !isVWorkAdmin(c) {
		return c.Status(403).JSON(fiber.Map{"error": "Forbidden"})
	}
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "No file provided"})
	}

	platform := c.FormValue("platform", "windows")
	if platform == "" {
		platform = "windows"
	}

	// Validate platform
	switch platform {
	case "windows", "macos", "linux":
		// ok
	default:
		return c.Status(400).JSON(fiber.Map{"error": "Invalid platform, must be windows/macos/linux"})
	}

	// Sanitize filename: keep original name but remove path separators
	originalName := filepath.Base(file.Filename)
	originalName = strings.ReplaceAll(originalName, " ", "_")

	// Create upload directory
	uploadDir := filepath.Join("web", "uploads", "voffice-releases", platform)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("Failed to create upload dir %s: %v", uploadDir, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create upload directory"})
	}

	destPath := filepath.Join(uploadDir, originalName)

	// Save the file
	if err := c.SaveFile(file, destPath); err != nil {
		log.Printf("Failed to save file %s: %v", destPath, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save file"})
	}

	// Calculate SHA-256 checksum
	f, err := os.Open(destPath)
	if err != nil {
		log.Printf("Failed to open saved file for checksum: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "File saved but failed to compute checksum"})
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		log.Printf("Failed to compute checksum: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "File saved but failed to compute checksum"})
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Build the public download URL (relative path)
	downloadURL := fmt.Sprintf("/uploads/voffice-releases/%s/%s", platform, originalName)

	return c.JSON(fiber.Map{
		"download_url": downloadURL,
		"file_size":    file.Size,
		"checksum":     checksum,
		"filename":     originalName,
	})
}
