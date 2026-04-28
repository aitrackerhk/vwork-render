package handlers

import (
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// GetExpenseRequests 列表
func GetExpenseRequests(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	status := c.Query("status")

	var reqs []models.ExpenseRequest
	query := database.DB.Where("tenant_id = ?", tenantID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Preload("Expense").Preload("Approver").
		Order("created_at DESC").Find(&reqs).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(reqs)
}

// CreateExpenseRequest 建立申請（預設 pending）
func CreateExpenseRequest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)

	var req struct {
		Title       string  `json:"title"`
		Amount      float64 `json:"amount"`
		Description string  `json:"description"`
		RequestDate string  `json:"request_date"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}
	if req.Title == "" || req.Amount <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Title and amount are required"})
	}
	dateVal := utils.NowInTenantTimezone(tenantID)
	if req.RequestDate != "" {
		if d, err := utils.ParseDateInTenantTimezone(tenantID, req.RequestDate); err == nil {
			dateVal = d
		}
	}
	expReq := models.ExpenseRequest{
		TenantID:    tenantID,
		Title:       req.Title,
		Amount:      req.Amount,
		Description: req.Description,
		RequestDate: dateVal,
		Status:      "pending",
		CreatedBy:   &userID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := database.DB.Create(&expReq).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create expense request"})
	}
	return c.Status(201).JSON(expReq)
}

// ApproveExpenseRequest 批核 -> 產生支出
func ApproveExpenseRequest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request ID"})
	}

	var req models.ExpenseRequest
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&req).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Request not found"})
	}
	if req.Status != "pending" {
		return c.Status(400).JSON(fiber.Map{"error": "Request already processed"})
	}

	expense := models.Expense{
		TenantID:    tenantID,
		ExpenseType: "request",
		Category:    "expense_request",
		Description: req.Description,
		Amount:      req.Amount,
		ExpenseDate: req.RequestDate,
		Status:      "confirmed",
		CreatedBy:   &userID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := database.DB.Create(&expense).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create expense"})
	}

	now := time.Now()
	req.Status = "approved"
	req.ExpenseID = &expense.ID
	req.ApprovedBy = &userID
	req.ApprovedAt = &now
	req.UpdatedAt = now
	if err := database.DB.Save(&req).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update request"})
	}

	return c.JSON(req)
}

// RejectExpenseRequest 拒絕
func RejectExpenseRequest(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request ID"})
	}

	var req models.ExpenseRequest
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&req).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Request not found"})
	}
	if req.Status != "pending" {
		return c.Status(400).JSON(fiber.Map{"error": "Request already processed"})
	}

	now := time.Now()
	req.Status = "rejected"
	req.ApprovedBy = &userID
	req.ApprovedAt = &now
	req.UpdatedAt = now
	if err := database.DB.Save(&req).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update request"})
	}

	return c.JSON(req)
}


