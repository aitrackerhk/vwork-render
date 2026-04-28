package handlers

import (
	"nwork/config"
	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/whatsapp"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

var whatsappClient *whatsapp.Client
var whatsappVerifier *whatsapp.WebhookVerifier

// InitWhatsApp 初始化 WhatsApp 客戶端
func InitWhatsApp(cfg *config.Config) {
	whatsappClient = whatsapp.NewClient(cfg)
	whatsappVerifier = whatsapp.NewWebhookVerifier(cfg)
}

// WhatsAppWebhook 處理 WhatsApp Webhook 請求
func WhatsAppWebhook(c *fiber.Ctx) error {
	// GET 請求：Webhook 驗證
	if c.Method() == "GET" {
		mode := c.Query("hub.mode")
		token := c.Query("hub.verify_token")
		challenge := c.Query("hub.challenge")

		verified, response := whatsappVerifier.VerifyWebhook(mode, token, challenge)
		if verified {
			return c.SendString(response)
		}

		return c.Status(403).JSON(fiber.Map{
			"error": "Verification failed",
		})
	}

	// POST 請求：接收消息和狀態更新
	if c.Method() == "POST" {
		// 驗證簽名（如果配置了 app secret）
		signature := c.Get("X-Hub-Signature-256")
		if signature != "" {
			if !whatsappVerifier.VerifySignature(signature, c.Body()) {
				return c.Status(403).JSON(fiber.Map{
					"error": "Invalid signature",
				})
			}
		}

		// 解析 Webhook 事件
		event, err := whatsapp.ParseWebhookEvent(c.Body())
		if err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error":   "Failed to parse webhook event",
				"details": err.Error(),
			})
		}

		// 處理接收到的消息
		messages := event.GetIncomingMessages()
		for _, msg := range messages {
			go handleIncomingWhatsAppMessage(msg)
		}

		// 處理狀態更新
		statusUpdates := event.GetStatusUpdates()
		for _, update := range statusUpdates {
			go handleWhatsAppStatusUpdate(update)
		}

		return c.JSON(fiber.Map{
			"status": "ok",
		})
	}

	return c.Status(405).JSON(fiber.Map{
		"error": "Method not allowed",
	})
}

// handleIncomingWhatsAppMessage 處理接收到的 WhatsApp 消息
func handleIncomingWhatsAppMessage(msg whatsapp.IncomingMessage) {
	// TODO: 根據業務邏輯處理消息
	// 例如：創建 SupportCommunication 記錄、自動回覆等

	// 示例：記錄到日誌或數據庫
	// 這裡可以根據電話號碼查找對應的客戶，並創建 SupportCommunication
	_ = msg
}

// handleWhatsAppStatusUpdate 處理 WhatsApp 消息狀態更新
func handleWhatsAppStatusUpdate(update whatsapp.StatusUpdate) {
	// TODO: 更新消息狀態到數據庫
	// 例如：更新 Promotion 的發送狀態
	_ = update
}

// SendWhatsAppMessage 發送 WhatsApp 消息（用於推廣等功能）
func SendWhatsAppMessage(tenantID uuid.UUID, to, message string) error {
	if !whatsappClient.IsConfigured() {
		return whatsapp.ErrWhatsAppNotConfigured
	}

	// 發送消息
	if err := whatsappClient.SendTextMessage(to, message); err != nil {
		return err
	}

	// 可以選擇記錄到數據庫
	// TODO: 創建消息記錄

	return nil
}

// SendWhatsAppPromotion 發送 WhatsApp 推廣消息
func SendWhatsAppPromotion(tenantID uuid.UUID, promotion models.Promotion) error {
	if !whatsappClient.IsConfigured() {
		return whatsapp.ErrWhatsAppNotConfigured
	}

	successCount := 0
	failCount := 0
	totalRecipients := 0

	// 檢查是否為 lead_list 模式
	var audienceMode string
	if promotion.TargetAudience != nil {
		if m, ok := promotion.TargetAudience["mode"].(string); ok {
			audienceMode = m
		}
	}

	if audienceMode == "lead_list" {
		// Lead 列表模式：發送 WhatsApp 給 Lead Finder 結果
		leadResultIDs := parseUUIDListFromInterface(promotion.TargetAudience["lead_result_ids"])
		if len(leadResultIDs) == 0 {
			return nil
		}

		var leadResults []models.LeadFinderResult
		if err := database.DB.Where("tenant_id = ? AND id IN ?", tenantID, leadResultIDs).Find(&leadResults).Error; err != nil {
			return err
		}

		totalRecipients = len(leadResults)
		for _, lead := range leadResults {
			if lead.Phone == "" {
				failCount++
				continue
			}

			if err := whatsappClient.SendTextMessage(lead.Phone, promotion.Content); err != nil {
				failCount++
				continue
			}

			// 更新 lead 狀態為 contacted
			database.DB.Model(&lead).Update("status", "contacted")
			successCount++
		}
	} else {
		// 標準客戶模式
		var customers []models.Customer
		query := database.DB.Where("tenant_id = ?", tenantID)

		// 根據 target_audience 過濾客戶
		if promotion.TargetAudience != nil {
			if statusValue, ok := promotion.TargetAudience["status"].(string); ok && statusValue != "" && statusValue != "all" {
				query = query.Where("status = ?", statusValue)
			}

			if memberLevelIDs := parseUUIDListFromInterface(promotion.TargetAudience["member_level_ids"]); len(memberLevelIDs) > 0 {
				query = query.Where("member_level_id IN ?", memberLevelIDs)
			}

			if labelIDs := parseUUIDListFromInterface(promotion.TargetAudience["label_ids"]); len(labelIDs) > 0 {
				query = query.Joins("JOIN customer_label_relations ON customers.id = customer_label_relations.customer_id").
					Where("customer_label_relations.label_id IN ?", labelIDs).
					Group("customers.id")
			}

			if customerIDs := parseUUIDListFromInterface(promotion.TargetAudience["customer_ids"]); len(customerIDs) > 0 {
				query = query.Where("id IN ?", customerIDs)
			}
		}

		if err := query.Find(&customers).Error; err != nil {
			return err
		}

		totalRecipients = len(customers)
		for _, customer := range customers {
			if customer.Phone == "" {
				failCount++
				continue
			}

			if err := whatsappClient.SendTextMessage(customer.Phone, promotion.Content); err != nil {
				failCount++
				continue
			}

			successCount++
		}
	}

	// 更新推廣狀態
	promotion.TotalRecipients = totalRecipients
	promotion.SuccessCount = successCount
	promotion.FailCount = failCount

	if err := database.DB.Save(&promotion).Error; err != nil {
		return err
	}

	return nil
}
