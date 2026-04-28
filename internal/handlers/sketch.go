package handlers

import (
	"encoding/json"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// sketchToResponse converts an AiSketch model to a JSON-friendly response
// where Objects (stored as string in DB) is output as raw JSON array
func sketchToResponse(s models.AiSketch) fiber.Map {
	// Parse objects string back to raw JSON so it's not double-quoted
	var objectsRaw json.RawMessage
	if s.Objects != "" {
		if json.Valid([]byte(s.Objects)) {
			objectsRaw = json.RawMessage(s.Objects)
		} else {
			objectsRaw = json.RawMessage("[]")
		}
	} else {
		objectsRaw = json.RawMessage("[]")
	}

	resp := fiber.Map{
		"id":            s.ID,
		"tenant_id":     s.TenantID,
		"user_id":       s.UserID,
		"title":         s.Title,
		"objects":       objectsRaw,
		"canvas_width":  s.CanvasWidth,
		"canvas_height": s.CanvasHeight,
		"aspect_ratio":  s.AspectRatio,
		"created_at":    s.CreatedAt,
		"updated_at":    s.UpdatedAt,
	}
	if s.Thumbnail != "" {
		resp["thumbnail"] = s.Thumbnail
	}
	return resp
}

// GetAiSketches 列出當前用戶的所有草圖
// GET /api/v1/ai/sketches
func GetAiSketches(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var sketches []models.AiSketch
	if err := database.DB.Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("updated_at DESC").
		Find(&sketches).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// For list view, omit objects and thumbnail to keep response small
	items := make([]fiber.Map, 0, len(sketches))
	for _, s := range sketches {
		items = append(items, fiber.Map{
			"id":         s.ID,
			"title":      s.Title,
			"created_at": s.CreatedAt,
			"updated_at": s.UpdatedAt,
		})
	}

	return c.JSON(fiber.Map{"data": items})
}

// CreateAiSketch 建立新草圖
// POST /api/v1/ai/sketches
func CreateAiSketch(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var body struct {
		Title        string          `json:"title"`
		Objects      json.RawMessage `json:"objects"`
		CanvasWidth  int             `json:"canvas_width"`
		CanvasHeight int             `json:"canvas_height"`
		AspectRatio  string          `json:"aspect_ratio"`
		Thumbnail    string          `json:"thumbnail"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if body.Title == "" {
		body.Title = "未命名草圖"
	}
	if body.CanvasWidth <= 0 {
		body.CanvasWidth = 800
	}
	if body.CanvasHeight <= 0 {
		body.CanvasHeight = 600
	}
	if body.AspectRatio == "" {
		body.AspectRatio = "free"
	}

	// Store objects as JSON string
	objectsStr := "[]"
	if body.Objects != nil && json.Valid(body.Objects) {
		objectsStr = string(body.Objects)
	}

	sketch := models.AiSketch{
		TenantID:     tenantID,
		UserID:       userID,
		Title:        body.Title,
		Objects:      objectsStr,
		CanvasWidth:  body.CanvasWidth,
		CanvasHeight: body.CanvasHeight,
		AspectRatio:  body.AspectRatio,
		Thumbnail:    body.Thumbnail,
	}

	if err := database.DB.Create(&sketch).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(sketchToResponse(sketch))
}

// GetAiSketch 取得單個草圖
// GET /api/v1/ai/sketches/:id
func GetAiSketch(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var sketch models.AiSketch
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&sketch).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Sketch not found"})
	}

	return c.JSON(sketchToResponse(sketch))
}

// UpdateAiSketch 更新草圖
// PUT /api/v1/ai/sketches/:id
func UpdateAiSketch(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var sketch models.AiSketch
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&sketch).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Sketch not found"})
	}

	var body struct {
		Title        string          `json:"title"`
		Objects      json.RawMessage `json:"objects"`
		CanvasWidth  int             `json:"canvas_width"`
		CanvasHeight int             `json:"canvas_height"`
		AspectRatio  string          `json:"aspect_ratio"`
		Thumbnail    string          `json:"thumbnail"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if body.Title != "" {
		sketch.Title = body.Title
	}
	if body.Objects != nil && json.Valid(body.Objects) {
		sketch.Objects = string(body.Objects)
	}
	if body.CanvasWidth > 0 {
		sketch.CanvasWidth = body.CanvasWidth
	}
	if body.CanvasHeight > 0 {
		sketch.CanvasHeight = body.CanvasHeight
	}
	if body.AspectRatio != "" {
		sketch.AspectRatio = body.AspectRatio
	}
	if body.Thumbnail != "" {
		sketch.Thumbnail = body.Thumbnail
	}

	if err := database.DB.Save(&sketch).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(sketchToResponse(sketch))
}

// DeleteAiSketch 刪除草圖
// DELETE /api/v1/ai/sketches/:id
func DeleteAiSketch(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id := c.Params("id")

	var sketch models.AiSketch
	if err := database.DB.Where("tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id).
		First(&sketch).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Sketch not found"})
	}

	if err := database.DB.Delete(&sketch).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Sketch deleted"})
}
