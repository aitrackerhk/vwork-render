package handlers

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetAiSketchGenerations lists generation history for the current user.
// Supports pagination via ?limit=20&offset=0 query params.
// Does NOT return result_image in the list — use the /image endpoint or
// fetch individual generation for full image data.
// GET /api/v1/ai/sketch-generations
func GetAiSketchGenerations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	query := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID)

	// Filter by sketch_id if provided, or show orphaned (no sketch_id) records
	if sketchID := c.Query("sketch_id"); sketchID != "" {
		query = query.Where("sketch_id = ?", sketchID)
	} else if c.Query("orphaned") == "true" {
		query = query.Where("sketch_id IS NULL")
	}

	// Filter by source if provided (e.g. "sketch", "chat")
	if source := c.Query("source"); source != "" {
		query = query.Where("source = ?", source)
	}

	// Count total matching records (before pagination)
	var total int64
	if err := query.Model(&models.AiSketchGeneration{}).Count(&total).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Pagination: default limit=20, offset=0
	limit := 20
	offset := 0
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	var generations []models.AiSketchGeneration
	if err := query.Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&generations).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// Build response items — omit result_image to keep payload small.
	// Frontends should use /ai/sketch-generations/:id/image for thumbnails.
	items := make([]fiber.Map, 0, len(generations))
	for _, g := range generations {
		item := fiber.Map{
			"id":         g.ID,
			"prompt":     g.Prompt,
			"model":      g.Model,
			"source":     g.Source,
			"status":     g.Status,
			"sketch_id":  g.SketchID,
			"created_at": g.CreatedAt,
			"has_image":  g.ResultImage != "",
		}
		// Provide image_url so frontend can lazy-load via <img src>
		if g.ResultImage != "" {
			item["image_url"] = fmt.Sprintf("/api/v1/ai/sketch-generations/%s/image", g.ID)
		}
		items = append(items, item)
	}

	hasMore := int64(offset+limit) < total

	return c.JSON(fiber.Map{
		"data":     items,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": hasMore,
	})
}

// GetAiSketchGeneration returns a single generation with full image data
// GET /api/v1/ai/sketch-generations/:id
func GetAiSketchGeneration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var gen models.AiSketchGeneration
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&gen).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Generation not found"})
	}

	return c.JSON(fiber.Map{
		"id":           gen.ID,
		"prompt":       gen.Prompt,
		"result_image": gen.ResultImage,
		"model":        gen.Model,
		"source":       gen.Source,
		"status":       gen.Status,
		"sketch_id":    gen.SketchID,
		"created_at":   gen.CreatedAt,
	})
}

// GetAiSketchGenerationImage returns the generation image as raw binary.
// This allows frontends to use <img src="/api/v1/ai/sketch-generations/:id/image">
// instead of embedding base64 data URLs, which dramatically reduces list payload.
// GET /api/v1/ai/sketch-generations/:id/image
func GetAiSketchGenerationImage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var gen models.AiSketchGeneration
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&gen).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Generation not found"})
	}

	if gen.ResultImage == "" {
		return c.Status(404).JSON(fiber.Map{"error": "No image available"})
	}

	// Parse data URL: "data:<mime>;base64,<data>"
	raw := gen.ResultImage
	mime := "image/png"
	b64Data := raw

	if strings.HasPrefix(raw, "data:") {
		commaIdx := strings.Index(raw, ",")
		if commaIdx > 0 {
			header := raw[5:commaIdx] // e.g. "image/png;base64"
			b64Data = raw[commaIdx+1:]
			if semiIdx := strings.Index(header, ";"); semiIdx > 0 {
				mime = header[:semiIdx]
			} else {
				mime = header
			}
		}
	}

	imgBytes, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to decode image"})
	}

	c.Set("Content-Type", mime)
	c.Set("Cache-Control", "public, max-age=86400") // cache 24h — images are immutable
	return c.Send(imgBytes)
}

// DELETE /api/v1/ai/sketch-generations/:id
func DeleteAiSketchGeneration(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var gen models.AiSketchGeneration
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&gen).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Generation not found"})
	}

	if err := database.DB.Delete(&gen).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Generation deleted"})
}

// SaveSketchGeneration is a helper function called from GenerateLLMSketch
// to persist a generation record after successful AI image generation.
// source: "sketch" for vai-sketch tool, "chat" for vai-chat image generation.
func SaveSketchGeneration(tenantID, userID uuid.UUID, sketchID *uuid.UUID, prompt, resultImage, model, source string) error {
	if source == "" {
		source = "sketch"
	}
	gen := models.AiSketchGeneration{
		TenantID:    tenantID,
		UserID:      userID,
		SketchID:    sketchID,
		Prompt:      prompt,
		ResultImage: resultImage,
		Model:       model,
		Source:      source,
		Status:      "completed",
	}
	if err := database.DB.Create(&gen).Error; err != nil {
		return fmt.Errorf("failed to save sketch generation: %w", err)
	}
	return nil
}

// LinkOrphanedGenerations updates all generation records with sketch_id = NULL
// for the current user to point to the given sketch_id. This is called after
// a new sketch is saved for the first time, so that generations created before
// the sketch existed are linked to it and appear in history.
//
// PUT /api/v1/ai/sketch-generations/link-orphaned
func LinkOrphanedGenerations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var body struct {
		SketchID string `json:"sketch_id"`
	}
	if err := c.BodyParser(&body); err != nil || body.SketchID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "sketch_id is required"})
	}

	sketchUUID, err := uuid.Parse(body.SketchID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid sketch_id"})
	}

	result := database.DB.Model(&models.AiSketchGeneration{}).
		Where("tenant_id = ? AND user_id = ? AND sketch_id IS NULL AND source = 'sketch'", tenantID, userID).
		Update("sketch_id", sketchUUID)

	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": result.Error.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "Linked orphaned generations",
		"count":   result.RowsAffected,
	})
}
