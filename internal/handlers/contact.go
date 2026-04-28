package handlers

import (
	"log"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/models"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ContactRequest 聯絡客服請求
type ContactRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Product string `json:"product"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

// ContactSupport 處理聯絡客服請求
func ContactSupport(c *fiber.Ctx) error {
	var req ContactRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request",
		})
	}

	// 驗證必填字段
	if req.Name == "" || req.Email == "" || req.Subject == "" || req.Message == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "Name, email, subject, and message are required",
		})
	}

	// 驗證長度
	if utf8.RuneCountInString(req.Name) > 50 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Name must be at most 50 characters",
		})
	}
	if len(req.Subject) > 100 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Subject must be at most 100 characters",
		})
	}
	if len(req.Message) > 2000 {
		return c.Status(400).JSON(fiber.Map{
			"error": "Message must be at most 2000 characters",
		})
	}

	// 保存聯絡記錄到數據庫（使用 support_communications 表）
	// 注意：SupportCommunication 需要 TenantID 和 CustomerID，但這是公開聯絡表單
	// 我們使用 uuid.Nil 作為佔位符，並將聯繫信息存儲在 ExtraFields 中
	var supportComm models.SupportCommunication
	supportComm.TenantID = uuid.Nil   // 公開聯絡，沒有租戶
	supportComm.CustomerID = uuid.Nil // 公開聯絡，沒有客戶
	supportComm.CommunicationType = "contact_form"
	supportComm.Subject = req.Subject
	supportComm.Content = req.Message
	supportComm.Direction = "inbound"
	supportComm.Status = "open"
	supportComm.Priority = "normal"
	supportComm.CreatedAt = time.Now()
	supportComm.UpdatedAt = time.Now()

	// 將聯繫信息存儲在 ExtraFields 中
	supportComm.ExtraFields = models.JSONB{
		"name":    req.Name,
		"email":   req.Email,
		"phone":   req.Phone,
		"product": req.Product,
	}

	if err := database.DB.Create(&supportComm).Error; err != nil {
		log.Printf("Failed to save contact message: %v", err)
		// 即使保存失敗，也返回成功（避免用戶重複提交）
	}

	// 生成 email job，由 worker 發送郵件到配置的聯絡 email
	if err := email.EnqueueContactEmail(req.Name, req.Email, req.Phone, req.Product, req.Subject, req.Message); err != nil {
		log.Printf("Failed to enqueue contact email: %v", err)
		// 即使 email job 創建失敗，也返回成功（避免用戶重複提交）
	} else {
		log.Printf("Contact form submitted - Name: %s, Email: %s, Product: %s, Subject: %s (email job queued)", req.Name, req.Email, req.Product, req.Subject)
	}

	return c.JSON(fiber.Map{
		"message": "Your message has been received. We will contact you soon.",
	})
}
