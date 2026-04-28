package handlers

import (
	"fmt"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func ListProjectFiles(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	var files []models.ProjectFile
	if err := database.DB.Where("tenant_id = ? AND project_id = ?", tenantID, projectID).
		Order("created_at DESC").
		Find(&files).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to fetch project files: %v", err)})
	}
	return c.JSON(fiber.Map{"data": files})
}

func CreateProjectFile(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project ID"})
	}

	var req struct {
		FileURL   string `json:"file_url"`
		FileName  string `json:"file_name"`
		MimeType  string `json:"mime_type"`
		FileSize  int64  `json:"file_size"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.FileURL == "" || req.FileName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "file_url and file_name are required"})
	}

	now := time.Now()
	f := models.ProjectFile{
		TenantID:    tenantID,
		ProjectID:   projectID,
		FileURL:     req.FileURL,
		FileName:    req.FileName,
		MimeType:    req.MimeType,
		FileSize:    req.FileSize,
		Description: req.Description,
		UploadedBy:  &userID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := database.DB.Create(&f).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to create project file: %v", err)})
	}
	return c.Status(201).JSON(f)
}

func DeleteProjectFile(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	projectID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid project ID"})
	}
	fileID, err := uuid.Parse(c.Params("fileId"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid file ID"})
	}

	if err := database.DB.Where("tenant_id = ? AND project_id = ? AND id = ?", tenantID, projectID, fileID).
		Delete(&models.ProjectFile{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("Failed to delete project file: %v", err)})
	}
	return c.JSON(fiber.Map{"message": "Project file deleted"})
}


