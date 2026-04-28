package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ========== 廣告位置 API ==========

// GetAdPositions 獲取廣告位置列表
func GetAdPositions(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var positions []models.AdPosition

	query := database.DB.Where("tenant_id = ?", tenantID)

	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ? OR code ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 100)
	offset := (page - 1) * limit

	var total int64
	database.DB.Model(&models.AdPosition{}).Where("tenant_id = ?", tenantID).Count(&total)

	if err := query.
		Preload("Ads", func(db *gorm.DB) *gorm.DB {
			return db.Where("is_active = ?", true).Order("sort_order ASC")
		}).
		Offset(offset).Limit(limit).
		Order("created_at DESC").
		Find(&positions).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ad positions: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  positions,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetAdPosition 獲取單個廣告位置
func GetAdPosition(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	positionID := c.Params("id")

	var position models.AdPosition
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, positionID).
		Preload("Ads", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC")
		}).
		First(&position).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Ad position not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ad position: %v", err)})
	}

	return c.JSON(position)
}

// CreateAdPosition 創建廣告位置
func CreateAdPosition(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		Name          string `json:"name"`
		Code          string `json:"code"`
		Description   string `json:"description"`
		Width         int    `json:"width"`
		Height        int    `json:"height"`
		SlideInterval int    `json:"slide_interval"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if req.Code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Code is required"})
	}

	// 檢查 code 是否已存在
	var existing models.AdPosition
	if err := database.DB.Where("tenant_id = ? AND code = ?", tenantID, req.Code).First(&existing).Error; err == nil {
		return c.Status(400).JSON(fiber.Map{"error": "Code already exists"})
	}

	position := models.AdPosition{
		TenantID:      tenantID,
		Name:          req.Name,
		Code:          req.Code,
		Description:   req.Description,
		Width:         req.Width,
		Height:        req.Height,
		SlideInterval: req.SlideInterval,
	}

	if err := database.DB.Create(&position).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create ad position: %v", err)})
	}

	return c.Status(201).JSON(position)
}

// UpdateAdPosition 更新廣告位置
func UpdateAdPosition(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	positionID := c.Params("id")

	var req struct {
		Name          string `json:"name,omitempty"`
		Code          string `json:"code,omitempty"`
		Description   string `json:"description,omitempty"`
		Width         int    `json:"width,omitempty"`
		Height        int    `json:"height,omitempty"`
		SlideInterval int    `json:"slide_interval,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	var position models.AdPosition
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, positionID).First(&position).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Ad position not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ad position: %v", err)})
	}

	// 更新字段
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Code != "" {
		// 檢查 code 是否已被其他位置使用
		var existing models.AdPosition
		if err := database.DB.Where("tenant_id = ? AND code = ? AND id != ?", tenantID, req.Code, positionID).First(&existing).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "Code already exists"})
		}
		updates["code"] = req.Code
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Width > 0 {
		updates["width"] = req.Width
	}
	if req.Height > 0 {
		updates["height"] = req.Height
	}
	if req.SlideInterval > 0 {
		updates["slide_interval"] = req.SlideInterval
	}

	if err := database.DB.Model(&position).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update ad position: %v", err)})
	}

	// 重新加載
	database.DB.Where("id = ?", positionID).First(&position)

	return c.JSON(position)
}

// DeleteAdPosition 刪除廣告位置
func DeleteAdPosition(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	positionID := c.Params("id")

	var position models.AdPosition
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, positionID).First(&position).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Ad position not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ad position: %v", err)})
	}

	// 軟刪除
	if err := database.DB.Delete(&position).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete ad position: %v", err)})
	}

	return c.JSON(fiber.Map{"message": "Ad position deleted successfully"})
}

// ========== 廣告 API ==========

// GetAds 獲取廣告列表
func GetAds(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var ads []models.Ad

	query := database.DB.Where("tenant_id = ?", tenantID)

	// 按廣告位置過濾
	if positionID := c.Query("ad_position_id"); positionID != "" {
		query = query.Where("ad_position_id = ?", positionID)
	}

	// 按媒體類型過濾
	if mediaType := c.Query("media_type"); mediaType != "" {
		query = query.Where("media_type = ?", mediaType)
	}

	// 按狀態過濾
	if isActive := c.Query("is_active"); isActive != "" {
		active, _ := strconv.ParseBool(isActive)
		query = query.Where("is_active = ?", active)
	}

	// 搜索
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	// 分頁
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 100)
	offset := (page - 1) * limit

	var total int64
	countQuery := database.DB.Model(&models.Ad{}).Where("tenant_id = ?", tenantID)
	if positionID := c.Query("ad_position_id"); positionID != "" {
		countQuery = countQuery.Where("ad_position_id = ?", positionID)
	}
	countQuery.Count(&total)

	if err := query.
		Preload("AdPosition").
		Offset(offset).Limit(limit).
		Order("sort_order ASC, created_at DESC").
		Find(&ads).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ads: %v", err)})
	}

	return c.JSON(fiber.Map{
		"data":  ads,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetAd 獲取單個廣告
func GetAd(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	adID := c.Params("id")

	var ad models.Ad
	if err := database.DB.
		Where("tenant_id = ? AND id = ?", tenantID, adID).
		Preload("AdPosition").
		First(&ad).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Ad not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ad: %v", err)})
	}

	return c.JSON(ad)
}

// CreateAd 創建廣告
func CreateAd(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		AdPositionID string                 `json:"ad_position_id"`
		Name         string                 `json:"name"`
		MediaType    string                 `json:"media_type"`
		MediaURL     string                 `json:"media_url"`
		MediaPath    string                 `json:"media_path"`
		Duration     int                    `json:"duration"`
		SortOrder    int                    `json:"sort_order"`
		IsActive     *bool                  `json:"is_active"`
		StartDate    *time.Time             `json:"start_date"`
		EndDate      *time.Time             `json:"end_date"`
		ExtraFields  map[string]interface{} `json:"extra_fields"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}
	if req.AdPositionID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Ad position ID is required"})
	}
	if req.MediaURL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Media URL is required"})
	}

	positionUUID, err := uuid.Parse(req.AdPositionID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ad position ID"})
	}

	// 驗證廣告位置存在
	var position models.AdPosition
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, positionUUID).First(&position).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Ad position not found"})
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	ad := models.Ad{
		TenantID:     tenantID,
		AdPositionID: positionUUID,
		Name:         req.Name,
		MediaType:    req.MediaType,
		MediaURL:     req.MediaURL,
		MediaPath:    req.MediaPath,
		Duration:     req.Duration,
		SortOrder:    req.SortOrder,
		IsActive:     isActive,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
	}

	if req.ExtraFields != nil {
		ad.ExtraFields = models.JSONB(req.ExtraFields)
	}

	if err := database.DB.Create(&ad).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create ad: %v", err)})
	}

	// 更新輪播設定版本號
	incrementCarouselVersion(tenantID, positionUUID)

	return c.Status(201).JSON(ad)
}

// UpdateAd 更新廣告
func UpdateAd(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	adID := c.Params("id")

	var req struct {
		AdPositionID string                 `json:"ad_position_id,omitempty"`
		Name         string                 `json:"name,omitempty"`
		MediaType    string                 `json:"media_type,omitempty"`
		MediaURL     string                 `json:"media_url,omitempty"`
		MediaPath    string                 `json:"media_path,omitempty"`
		Duration     int                    `json:"duration,omitempty"`
		SortOrder    *int                   `json:"sort_order,omitempty"`
		IsActive     *bool                  `json:"is_active,omitempty"`
		StartDate    *time.Time             `json:"start_date,omitempty"`
		EndDate      *time.Time             `json:"end_date,omitempty"`
		ExtraFields  map[string]interface{} `json:"extra_fields,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	var ad models.Ad
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, adID).First(&ad).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Ad not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ad: %v", err)})
	}

	oldPositionID := ad.AdPositionID

	// 更新字段
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if req.AdPositionID != "" {
		positionUUID, err := uuid.Parse(req.AdPositionID)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid ad position ID"})
		}
		// 驗證廣告位置存在
		var position models.AdPosition
		if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, positionUUID).First(&position).Error; err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Ad position not found"})
		}
		updates["ad_position_id"] = positionUUID
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.MediaType != "" {
		updates["media_type"] = req.MediaType
	}
	if req.MediaURL != "" {
		updates["media_url"] = req.MediaURL
	}
	if req.MediaPath != "" {
		updates["media_path"] = req.MediaPath
	}
	if req.Duration > 0 {
		updates["duration"] = req.Duration
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if req.StartDate != nil {
		updates["start_date"] = req.StartDate
	}
	if req.EndDate != nil {
		updates["end_date"] = req.EndDate
	}
	if req.ExtraFields != nil {
		updates["extra_fields"] = models.JSONB(req.ExtraFields)
	}

	if err := database.DB.Model(&ad).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update ad: %v", err)})
	}

	// 更新輪播設定版本號
	incrementCarouselVersion(tenantID, oldPositionID)
	if req.AdPositionID != "" {
		newPositionUUID, _ := uuid.Parse(req.AdPositionID)
		if newPositionUUID != oldPositionID {
			incrementCarouselVersion(tenantID, newPositionUUID)
		}
	}

	// 重新加載
	database.DB.Where("id = ?", adID).Preload("AdPosition").First(&ad)

	return c.JSON(ad)
}

// DeleteAd 刪除廣告
func DeleteAd(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	adID := c.Params("id")

	var ad models.Ad
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, adID).First(&ad).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(404).JSON(fiber.Map{"error": "Ad not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ad: %v", err)})
	}

	positionID := ad.AdPositionID

	// 軟刪除
	if err := database.DB.Delete(&ad).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete ad: %v", err)})
	}

	// 更新輪播設定版本號
	incrementCarouselVersion(tenantID, positionID)

	return c.JSON(fiber.Map{"message": "Ad deleted successfully"})
}

// UpdateAdSortOrder 更新廣告排序
func UpdateAdSortOrder(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)

	var req struct {
		AdPositionID string   `json:"ad_position_id"`
		AdIDs        []string `json:"ad_ids"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	positionUUID, err := uuid.Parse(req.AdPositionID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ad position ID"})
	}

	// 更新排序
	for i, adID := range req.AdIDs {
		database.DB.Model(&models.Ad{}).
			Where("tenant_id = ? AND id = ? AND ad_position_id = ?", tenantID, adID, positionUUID).
			Update("sort_order", i)
	}

	// 更新輪播設定版本號
	incrementCarouselVersion(tenantID, positionUUID)

	return c.JSON(fiber.Map{"message": "Sort order updated successfully"})
}

// ========== 輪播設定 API ==========

// GetCarouselSettings 獲取輪播設定
func GetCarouselSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	positionID := c.Params("ad_position_id")

	var settings models.CarouselSettings
	if err := database.DB.
		Where("tenant_id = ? AND ad_position_id = ?", tenantID, positionID).
		Preload("AdPosition").
		First(&settings).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 返回默認設定
			return c.JSON(models.CarouselSettings{
				SlideInterval:      5,
				TransitionDuration: 500,
				AutoUpdate:         true,
				UpdateInterval:     3600,
				Version:            1,
			})
		}
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch carousel settings: %v", err)})
	}

	return c.JSON(settings)
}

// UpdateCarouselSettings 更新輪播設定
func UpdateCarouselSettings(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	positionID := c.Params("ad_position_id")

	var req struct {
		SlideInterval      int  `json:"slide_interval"`
		TransitionDuration int  `json:"transition_duration"`
		AutoUpdate         bool `json:"auto_update"`
		UpdateInterval     int  `json:"update_interval"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	positionUUID, err := uuid.Parse(positionID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ad position ID"})
	}

	// 驗證廣告位置存在
	var position models.AdPosition
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, positionUUID).First(&position).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Ad position not found"})
	}

	var settings models.CarouselSettings
	err = database.DB.Where("tenant_id = ? AND ad_position_id = ?", tenantID, positionUUID).First(&settings).Error

	if err == gorm.ErrRecordNotFound {
		// 創建新設定
		settings = models.CarouselSettings{
			TenantID:           tenantID,
			AdPositionID:       positionUUID,
			SlideInterval:      req.SlideInterval,
			TransitionDuration: req.TransitionDuration,
			AutoUpdate:         req.AutoUpdate,
			UpdateInterval:     req.UpdateInterval,
			Version:            1,
		}
		if err := database.DB.Create(&settings).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create carousel settings: %v", err)})
		}
	} else if err == nil {
		// 更新現有設定
		updates := map[string]interface{}{
			"slide_interval":      req.SlideInterval,
			"transition_duration": req.TransitionDuration,
			"auto_update":         req.AutoUpdate,
			"update_interval":     req.UpdateInterval,
			"updated_at":          time.Now(),
		}
		if err := database.DB.Model(&settings).Updates(updates).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to update carousel settings: %v", err)})
		}
		database.DB.Where("id = ?", settings.ID).First(&settings)
	} else {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch carousel settings: %v", err)})
	}

	return c.JSON(settings)
}

// ========== 輪播 ZIP 生成 API ==========

// CarouselManifest 輪播播放器配置文件結構
type CarouselManifest struct {
	Version            int             `json:"version"`
	SlideInterval      int             `json:"slide_interval"`
	TransitionDuration int             `json:"transition_duration"`
	AutoUpdate         bool            `json:"auto_update"`
	UpdateInterval     int             `json:"update_interval"`
	UpdateURL          string          `json:"update_url"`
	Items              []CarouselItem  `json:"items"`
	GeneratedAt        string          `json:"generated_at"`
	AdPositionID       string          `json:"ad_position_id"`
	AdPositionCode     string          `json:"ad_position_code"`
}

// CarouselItem 輪播項目
type CarouselItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	FileName  string `json:"file_name"`
	Duration  int    `json:"duration"`
	SortOrder int    `json:"sort_order"`
}

// GenerateCarouselZip 生成輪播 ZIP 包
func GenerateCarouselZip(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	positionID := c.Params("ad_position_id")

	positionUUID, err := uuid.Parse(positionID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ad position ID"})
	}

	// 獲取廣告位置
	var position models.AdPosition
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, positionUUID).First(&position).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Ad position not found"})
	}

	// 獲取活動廣告
	var ads []models.Ad
	now := time.Now()
	query := database.DB.Where("tenant_id = ? AND ad_position_id = ? AND is_active = ?", tenantID, positionUUID, true)
	query = query.Where("(start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date >= ?)", now, now)
	if err := query.Order("sort_order ASC").Find(&ads).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch ads: %v", err)})
	}

	if len(ads) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No active ads found for this position"})
	}

	// 獲取輪播設定
	var settings models.CarouselSettings
	if err := database.DB.Where("tenant_id = ? AND ad_position_id = ?", tenantID, positionUUID).First(&settings).Error; err != nil {
		// 使用默認設定
		settings = models.CarouselSettings{
			SlideInterval:      position.SlideInterval,
			TransitionDuration: 500,
			AutoUpdate:         true,
			UpdateInterval:     3600,
			Version:            1,
		}
	}

	// 創建臨時目錄
	tmpDir, err := os.MkdirTemp("", "carousel-*")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create temp directory"})
	}
	defer os.RemoveAll(tmpDir)

	// 創建 media 目錄
	mediaDir := filepath.Join(tmpDir, "media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create media directory"})
	}

	// 準備 manifest
	manifest := CarouselManifest{
		Version:            settings.Version,
		SlideInterval:      settings.SlideInterval,
		TransitionDuration: settings.TransitionDuration,
		AutoUpdate:         settings.AutoUpdate,
		UpdateInterval:     settings.UpdateInterval,
		UpdateURL:          fmt.Sprintf("/api/v1/carousel/%s/check-update", positionID),
		GeneratedAt:        time.Now().Format(time.RFC3339),
		AdPositionID:       positionID,
		AdPositionCode:     position.Code,
		Items:              make([]CarouselItem, 0, len(ads)),
	}

	// 複製媒體文件並更新 manifest
	for i, ad := range ads {
		// 確定文件擴展名
		ext := filepath.Ext(ad.MediaURL)
		if ext == "" {
			if ad.MediaType == "video" {
				ext = ".mp4"
			} else {
				ext = ".jpg"
			}
		}

		fileName := fmt.Sprintf("%d_%s%s", i+1, ad.ID.String()[:8], ext)
		destPath := filepath.Join(mediaDir, fileName)

		// 複製文件
		if ad.MediaPath != "" {
			// 本地文件
			srcPath := ad.MediaPath
			if !filepath.IsAbs(srcPath) {
				srcPath = filepath.Join("web", srcPath)
			}
			if err := copyFile(srcPath, destPath); err != nil {
				// 嘗試從 URL 複製
				if strings.HasPrefix(ad.MediaURL, "/") {
					srcPath = filepath.Join("web", ad.MediaURL)
					if err := copyFile(srcPath, destPath); err != nil {
						continue // 跳過無法複製的文件
					}
				} else {
					continue
				}
			}
		} else if strings.HasPrefix(ad.MediaURL, "/") {
			// 相對 URL
			srcPath := filepath.Join("web", ad.MediaURL)
			if err := copyFile(srcPath, destPath); err != nil {
				continue
			}
		} else {
			// 遠程 URL - 下載文件
			if err := downloadFile(ad.MediaURL, destPath); err != nil {
				continue
			}
		}

		manifest.Items = append(manifest.Items, CarouselItem{
			ID:        ad.ID.String(),
			Name:      ad.Name,
			MediaType: ad.MediaType,
			FileName:  fileName,
			Duration:  ad.Duration,
			SortOrder: ad.SortOrder,
		})
	}

	if len(manifest.Items) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "No media files could be processed"})
	}

	// 寫入 manifest.json
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to write manifest"})
	}

	// 複製播放器 exe（如果存在）
	playerExePath := filepath.Join("dist", "carousel-player", "carousel-player.exe")
	if _, err := os.Stat(playerExePath); err == nil {
		destExePath := filepath.Join(tmpDir, "carousel-player.exe")
		copyFile(playerExePath, destExePath)
	}

	// 創建 ZIP 文件
	zipPath := filepath.Join(os.TempDir(), fmt.Sprintf("carousel_%s_%d.zip", position.Code, time.Now().Unix()))
	if err := createZip(tmpDir, zipPath); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create ZIP: %v", err)})
	}
	defer os.Remove(zipPath)

	// 更新 last_generated_at
	now = time.Now()
	database.DB.Model(&settings).Where("tenant_id = ? AND ad_position_id = ?", tenantID, positionUUID).
		Updates(map[string]interface{}{
			"last_generated_at": now,
			"updated_at":        now,
		})

	// 發送 ZIP 文件
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="carousel_%s.zip"`, position.Code))
	return c.SendFile(zipPath)
}

// CheckCarouselUpdate 檢查輪播更新
func CheckCarouselUpdate(c *fiber.Ctx) error {
	positionID := c.Params("ad_position_id")
	currentVersion := c.QueryInt("version", 0)

	positionUUID, err := uuid.Parse(positionID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ad position ID"})
	}

	// 嘗試通過 API key 獲取 tenant（公開 API）
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		apiKey = c.Query("api_key")
	}

	var tenantID uuid.UUID
	if apiKey != "" {
		var tenant models.Tenant
		if err := database.DB.Where("api_key = ?", apiKey).First(&tenant).Error; err == nil {
			tenantID = tenant.ID
		}
	}

	// 如果沒有 API key，嘗試使用 auth middleware
	if tenantID == uuid.Nil {
		tenantID = middleware.GetTenantID(c)
	}

	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var settings models.CarouselSettings
	if err := database.DB.Where("tenant_id = ? AND ad_position_id = ?", tenantID, positionUUID).First(&settings).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Carousel settings not found"})
	}

	needUpdate := settings.Version > currentVersion

	response := fiber.Map{
		"need_update":     needUpdate,
		"current_version": settings.Version,
		"your_version":    currentVersion,
	}

	if needUpdate {
		// 獲取活動廣告列表（僅 ID 和基本信息）
		var ads []models.Ad
		now := time.Now()
		query := database.DB.Where("tenant_id = ? AND ad_position_id = ? AND is_active = ?", tenantID, positionUUID, true)
		query = query.Where("(start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date >= ?)", now, now)
		query.Order("sort_order ASC").Select("id", "name", "media_type", "media_url", "duration", "sort_order").Find(&ads)

		items := make([]fiber.Map, 0, len(ads))
		for _, ad := range ads {
			items = append(items, fiber.Map{
				"id":         ad.ID.String(),
				"name":       ad.Name,
				"media_type": ad.MediaType,
				"media_url":  ad.MediaURL,
				"duration":   ad.Duration,
				"sort_order": ad.SortOrder,
			})
		}
		response["items"] = items
		response["slide_interval"] = settings.SlideInterval
		response["transition_duration"] = settings.TransitionDuration
	}

	return c.JSON(response)
}

// ========== 輔助函數 ==========

// incrementCarouselVersion 增加輪播版本號
func incrementCarouselVersion(tenantID, positionID uuid.UUID) {
	database.DB.Model(&models.CarouselSettings{}).
		Where("tenant_id = ? AND ad_position_id = ?", tenantID, positionID).
		UpdateColumn("version", gorm.Expr("version + 1"))
}

// copyFile 複製文件
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// downloadFile 下載文件
func downloadFile(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// createZip 創建 ZIP 文件
func createZip(srcDir, destPath string) error {
	zipFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Method = zip.Deflate

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// UploadAdMedia 上傳廣告媒體文件
func UploadAdMedia(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File is required"})
	}

	// 檢查文件類型
	contentType := file.Header.Get("Content-Type")
	isImage := strings.HasPrefix(contentType, "image/")
	isVideo := strings.HasPrefix(contentType, "video/")

	if !isImage && !isVideo {
		// 根據擴展名判斷
		ext := strings.ToLower(filepath.Ext(file.Filename))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif", ".webp":
			isImage = true
		case ".mp4", ".webm", ".mov", ".avi":
			isVideo = true
		}
	}

	if !isImage && !isVideo {
		return c.Status(400).JSON(fiber.Map{"error": "Only image and video files are allowed"})
	}

	// 創建上傳目錄
	uploadDir := filepath.Join("web", "uploads", tenantID.String(), "ads", time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create upload directory"})
	}

	// 生成唯一文件名
	ext := filepath.Ext(file.Filename)
	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	filePath := filepath.Join(uploadDir, filename)

	// 保存文件
	if err := c.SaveFile(file, filePath); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to save file"})
	}

	// 生成 URL
	fileURL := "/" + strings.ReplaceAll(filePath, "\\", "/")
	fileURL = strings.TrimPrefix(fileURL, "/web")

	mediaType := "image"
	if isVideo {
		mediaType = "video"
	}

	return c.JSON(fiber.Map{
		"url":        fileURL,
		"path":       filePath,
		"media_type": mediaType,
		"filename":   filename,
	})
}
