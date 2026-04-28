package handlers

import (
	"log"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func tenantAllowsPublicChat(t models.Tenant) bool {
	if t.ExtraFields == nil {
		// default enabled
		return true
	}
	if v, ok := t.ExtraFields["public_chat_enabled"]; ok {
		switch vv := v.(type) {
		case bool:
			return vv
		case string:
			return strings.ToLower(strings.TrimSpace(vv)) == "true" || strings.TrimSpace(vv) == "1" || strings.ToLower(strings.TrimSpace(vv)) == "on"
		case float64:
			return int(vv) == 1
		case int:
			return vv == 1
		}
	}
	// default enabled
	return true
}

func findActiveTenantBySubdomain(subdomain string) (*models.Tenant, error) {
	s := strings.TrimSpace(subdomain)
	if s == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "Subdomain is required")
	}
	var tenant models.Tenant
	if err := database.DB.Where("subdomain = ? AND status = ?", s, "active").First(&tenant).Error; err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "Tenant not found")
	}
	return &tenant, nil
}

// PublicGetChatConfig 公開端點：回傳該租戶是否啟用「網站浮動聊天」。
// GET /api/v1/public/:subdomain/chat/config
func PublicGetChatConfig(c *fiber.Ctx) error {
	tenant, err := findActiveTenantBySubdomain(c.Params("subdomain"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"enabled": tenantAllowsPublicChat(*tenant),
		},
	})
}

// PublicGetChatConversation 公開端點：讀取某個 visitor_id 的對話訊息。
// GET /api/v1/public/:subdomain/chat/conversation?visitor_id=xxx&limit=200
func PublicGetChatConversation(c *fiber.Ctx) error {
	tenant, err := findActiveTenantBySubdomain(c.Params("subdomain"))
	if err != nil {
		return err
	}
	if !tenantAllowsPublicChat(*tenant) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Public chat is disabled"})
	}

	visitorID := strings.TrimSpace(c.Query("visitor_id"))
	if visitorID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "visitor_id is required"})
	}

	limit := c.QueryInt("limit", 200)
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}

	var messages []models.Message
	if err := database.DB.
		Where("tenant_id = ? AND status = ?", tenant.ID, "active").
		Where("(message_type = ? OR extra_fields->>'public_chat' = 'true')", "public_chat").
		Where("extra_fields->>'visitor_id' = ?", visitorID).
		Order("created_at ASC").
		Limit(limit).
		Preload("FromUser").
		Find(&messages).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to load messages"})
	}

	return c.JSON(fiber.Map{"data": messages})
}

// PublicCreateChatMessage 公開端點：訪客送出訊息（匿名，使用 visitor_id）。
// POST /api/v1/public/:subdomain/chat/messages
func PublicCreateChatMessage(c *fiber.Ctx) error {
	tenant, err := findActiveTenantBySubdomain(c.Params("subdomain"))
	if err != nil {
		return err
	}
	if !tenantAllowsPublicChat(*tenant) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Public chat is disabled"})
	}

	var req struct {
		VisitorID   string `json:"visitor_id"`
		VisitorName string `json:"visitor_name"`
		Content     string `json:"content"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return c.Status(400).JSON(fiber.Map{"error": "content is required"})
	}

	visitorID := strings.TrimSpace(req.VisitorID)
	if visitorID == "" {
		visitorID = uuid.New().String()
	}
	visitorName := strings.TrimSpace(req.VisitorName)

	// 初始化 ExtraFields
	extraFields := make(models.JSONB)
	extraFields["public_chat"] = true
	extraFields["visitor_id"] = visitorID
	if visitorName != "" {
		extraFields["visitor_name"] = visitorName
	}

	msg := models.Message{
		ID:          uuid.New(), // 明確生成 ID
		TenantID:    tenant.ID,
		Subject:     "",
		Content:     content,
		IsRead:      false,
		MessageType: "public_chat",
		Status:      "active",
		ExtraFields: extraFields,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		// public chat 不需要 FromUserID, ToUserID, ToCustomerID
		FromUserID:   nil,
		ToUserID:     nil,
		ToCustomerID: nil,
	}

	// 使用 Omit 明確排除關聯字段，避免 GORM 嘗試插入不存在的列
	if err := database.DB.Omit("FromUser", "ToUser", "ToCustomer", "Tenant").Create(&msg).Error; err != nil {
		// 記錄詳細錯誤
		log.Printf("❌ Failed to create public chat message: %v", err)
		log.Printf("   TenantID: %s, VisitorID: %s, Content: %s", tenant.ID, visitorID, content)
		// 返回詳細錯誤以便調試
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create message", "details": err.Error()})
	}
	
	log.Printf("✅ Created public chat message: ID=%s, VisitorID=%s", msg.ID, visitorID)

	// 回傳訊息（含 id）與 visitor_id（方便前端保存）
	return c.Status(201).JSON(fiber.Map{
		"data": fiber.Map{
			"message":    msg,
			"visitor_id": visitorID,
		},
	})
}


