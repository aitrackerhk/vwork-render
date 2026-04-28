package handlers

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nwork/internal/middleware"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// UploadSketchImage accepts either a multipart file upload or a JSON body
// containing a base64 data URL, saves the image to disk, and returns a
// persistent URL that can be stored in sketch objects instead of inline base64.
//
// POST /api/v1/ai/sketch-image-upload
//
// Multipart form: field name "file"
// OR JSON body: { "data_url": "data:image/png;base64,..." }
func UploadSketchImage(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	if userID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Try multipart first
	file, err := c.FormFile("file")
	if err == nil && file != nil {
		// Use existing helper for multipart uploads
		fileURL, _, _, _, _, saveErr := saveUploadedImageForTenant(tenantID, file)
		if saveErr != nil {
			return c.Status(400).JSON(fiber.Map{"error": saveErr.Error()})
		}
		return c.JSON(fiber.Map{"url": fileURL})
	}

	// Fallback: JSON body with base64 data URL
	var body struct {
		DataURL string `json:"data_url"`
	}
	if err := c.BodyParser(&body); err != nil || body.DataURL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "No file or data_url provided"})
	}

	fileURL, saveErr := saveBase64ImageToDisk(tenantID, body.DataURL)
	if saveErr != nil {
		return c.Status(500).JSON(fiber.Map{"error": saveErr.Error()})
	}

	return c.JSON(fiber.Map{"url": fileURL})
}

// saveBase64ImageToDisk decodes a base64 data URL and writes it to disk under
// web/uploads/<tenantID>/sketch/<date>/<uuid>.<ext>
func saveBase64ImageToDisk(tenantID uuid.UUID, dataURL string) (string, error) {
	if tenantID == uuid.Nil {
		return "", fmt.Errorf("tenant_id is required")
	}

	// Parse data URL: data:<mime>;base64,<data>
	if !strings.HasPrefix(dataURL, "data:") {
		return "", fmt.Errorf("invalid data URL format")
	}

	commaIdx := strings.Index(dataURL, ",")
	if commaIdx < 0 {
		return "", fmt.Errorf("invalid data URL format")
	}

	header := dataURL[5:commaIdx] // e.g. "image/png;base64"
	b64Data := dataURL[commaIdx+1:]

	// Determine mime type and extension
	mimeType := "image/png"
	semicolonIdx := strings.Index(header, ";")
	if semicolonIdx > 0 {
		mimeType = header[:semicolonIdx]
	} else {
		mimeType = header
	}

	ext := ".png"
	switch mimeType {
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}

	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		// Try RawStdEncoding (no padding)
		decoded, err = base64.RawStdEncoding.DecodeString(b64Data)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 image: %w", err)
		}
	}

	// Limit size: 20MB
	if len(decoded) > 20*1024*1024 {
		return "", fmt.Errorf("image too large (max 20MB)")
	}

	// Create upload directory
	dateStr := time.Now().Format("2006-01-02")
	uploadDir := filepath.Join("web", "uploads", tenantID.String(), "sketch", dateStr)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %w", err)
	}

	// Write file
	filename := uuid.New().String() + ext
	filePath := filepath.Join(uploadDir, filename)
	if err := os.WriteFile(filePath, decoded, 0644); err != nil {
		return "", fmt.Errorf("failed to write image file: %w", err)
	}

	fileURL := fmt.Sprintf("/uploads/%s/sketch/%s/%s", tenantID.String(), dateStr, filename)
	return fileURL, nil
}
