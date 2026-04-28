package handlers

import (
	"log"
	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ============================================
// 客服通訊 (SupportCommunication) CRUD
// ============================================

func GetSupportCommunications(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	customerID := c.Query("customer_id")
	staffID := c.Query("staff_id")

	var communications []models.SupportCommunication
	query := database.DB.Where("tenant_id = ?", tenantID)

	if customerID != "" {
		query = query.Where("customer_id = ?", customerID)
	}
	if staffID != "" {
		query = query.Where("staff_id = ?", staffID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if priority := c.Query("priority"); priority != "" {
		query = query.Where("priority = ?", priority)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.SupportCommunication{}).Count(&total)

	if err := query.Preload("Customer").Preload("Staff").
		Offset(offset).Limit(limit).Order("created_at DESC").
		Find(&communications).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  communications,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetSupportCommunication(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var communication models.SupportCommunication

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).
		Preload("Customer").Preload("Staff").First(&communication).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Support communication not found"})
	}

	return c.JSON(communication)
}

func CreateSupportCommunication(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	staffID := middleware.GetUserID(c)

	var req struct {
		CustomerID        string `json:"customer_id"`
		CommunicationType string `json:"communication_type"`
		Subject           string `json:"subject"`
		Content           string `json:"content"`
		Direction         string `json:"direction"`
		Status            string `json:"status"`
		Priority          string `json:"priority"`
		// 支持从 contact form 直接提交的字段
		Name    string `json:"name"`
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Message string `json:"message"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 如果提供了 name/email/phone/message，说明是从 contact form 提交的
	if req.Name != "" || req.Email != "" || req.Message != "" {
		// 查找或创建客户
		var customerID uuid.UUID
		if req.Email != "" {
			var customer models.Customer
			if err := database.DB.Where("tenant_id = ? AND email = ?", tenantID, req.Email).First(&customer).Error; err != nil {
				// 客户不存在，创建新客户
				customer = models.Customer{
					TenantID: tenantID,
					Name:     req.Name,
					Email:    req.Email,
					Phone:    req.Phone,
					Status:   "active",
				}
				if err := database.DB.Create(&customer).Error; err != nil {
					return c.Status(500).JSON(fiber.Map{"error": "Failed to create customer"})
				}
			}
			customerID = customer.ID
		} else {
			return c.Status(400).JSON(fiber.Map{"error": "Email is required"})
		}

		// 构建内容
		content := ""
		if req.Name != "" {
			content += "姓名: " + req.Name + "\n"
		}
		if req.Email != "" {
			content += "電郵: " + req.Email + "\n"
		}
		if req.Phone != "" {
			content += "電話: " + req.Phone + "\n"
		}
		if req.Message != "" {
			content += "\n訊息:\n" + req.Message
		}

		communication := models.SupportCommunication{
			TenantID:          tenantID,
			CustomerID:        customerID,
			CommunicationType: "contact_form",
			Subject:           req.Name + " 的聯絡表單",
			Content:           content,
			Direction:         "inbound",
			Status:            "open",
			Priority:          "normal",
		}
		if staffID != uuid.Nil {
			communication.StaffID = &staffID
		}

		if err := database.DB.Create(&communication).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(201).JSON(communication)
	}

	// 原有的支持方式（直接提供 communication 对象）
	var communication models.SupportCommunication
	if err := c.BodyParser(&communication); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	communication.TenantID = tenantID
	if communication.StaffID == nil {
		communication.StaffID = &staffID
	}

	if err := database.DB.Create(&communication).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(communication)
}

func UpdateSupportCommunication(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var communication models.SupportCommunication

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&communication).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Support communication not found"})
	}

	if err := c.BodyParser(&communication); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	communication.TenantID = tenantID

	// 如果狀態變為 resolved，設置 resolved_at
	if communication.Status == "resolved" && communication.ResolvedAt == nil {
		now := time.Now()
		communication.ResolvedAt = &now
	}

	if err := database.DB.Save(&communication).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(communication)
}

func ResolveSupportCommunication(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var communication models.SupportCommunication

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&communication).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Support communication not found"})
	}

	now := time.Now()
	communication.Status = "resolved"
	communication.ResolvedAt = &now

	if err := database.DB.Save(&communication).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(communication)
}

func DeleteSupportCommunication(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.SupportCommunication{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Support communication deleted"})
}

// ============================================
// 推廣發送 (Promotion) CRUD
// ============================================

func GetPromotions(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var promotions []models.Promotion
	query := database.DB.Where("tenant_id = ?", tenantID)

	if search := c.Query("search"); search != "" {
		query = query.Where("title ILIKE ?", "%"+search+"%")
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if promotionType := c.Query("promotion_type"); promotionType != "" {
		query = query.Where("promotion_type = ?", promotionType)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset := (page - 1) * limit

	var total int64
	query.Model(&models.Promotion{}).Count(&total)

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&promotions).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  promotions,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetPromotion(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var promotion models.Promotion

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&promotion).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Promotion not found"})
	}

	return c.JSON(promotion)
}

func CreatePromotion(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	var promotion models.Promotion
	if err := c.BodyParser(&promotion); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	promotion.TenantID = tenantID

	// 如果沒有設置狀態，默認為 scheduled（已排程）
	if promotion.Status == "" {
		promotion.Status = "scheduled"
	}

	if err := database.DB.Create(&promotion).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(promotion)
}

func UpdatePromotion(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var promotion models.Promotion

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&promotion).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Promotion not found"})
	}

	if err := c.BodyParser(&promotion); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	promotion.TenantID = tenantID

	if err := database.DB.Save(&promotion).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(promotion)
}

func SendPromotion(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")
	var promotion models.Promotion

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).First(&promotion).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Promotion not found"})
	}

	// Bulk customer message：每天限二次（只針對 message 類型，使用企業時區）
	if promotion.PromotionType == "message" {
		loc := time.Local
		var enterprise models.Enterprise
		if err := database.DB.Select("timezone").Where("tenant_id = ?", tenantID).First(&enterprise).Error; err == nil {
			if tz := strings.TrimSpace(enterprise.Timezone); tz != "" {
				if l, err := time.LoadLocation(tz); err == nil {
					loc = l
				}
			}
		}
		now := time.Now().In(loc)
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()
		endOfDay := startOfDay.Add(24 * time.Hour)
		var sentCount int64
		if err := database.DB.Model(&models.Promotion{}).
			Where("tenant_id = ? AND promotion_type = ? AND status = ? AND sent_at >= ? AND sent_at < ?",
				tenantID, "message", "sent", startOfDay, endOfDay).
			Count(&sentCount).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to check daily limit"})
		}
		if sentCount >= 2 {
			return c.Status(429).JSON(fiber.Map{
				"error": "Daily bulk message limit reached",
				"limit": 2,
			})
		}
	}

	// 根據發送模式處理
	switch promotion.PromotionType {
	case "message":
		// Message 模式：發送給客戶（通過系統消息）
		var audienceMode string
		if promotion.TargetAudience != nil {
			if m, ok := promotion.TargetAudience["mode"].(string); ok {
				audienceMode = m
			}
		}

		if audienceMode == "lead_list" {
			// Lead 列表模式：發送給 Lead Finder 結果中的聯絡人
			leadResultIDs := parseUUIDListFromInterface(promotion.TargetAudience["lead_result_ids"])
			if len(leadResultIDs) == 0 {
				return c.Status(400).JSON(fiber.Map{"error": "No lead results selected"})
			}

			var leadResults []models.LeadFinderResult
			if err := database.DB.Where("tenant_id = ? AND id IN ?", tenantID, leadResultIDs).Find(&leadResults).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to get lead results"})
			}

			// Lead 不是客戶，無法創建 SupportCommunication 記錄
			// 記錄發送數量，實際發送通過 email API 等外部管道處理
			successCount := 0
			failCount := 0
			for _, lead := range leadResults {
				if lead.Email == "" && lead.Phone == "" {
					failCount++
					continue
				}
				// 更新 lead 狀態為 contacted
				database.DB.Model(&lead).Update("status", "contacted")
				successCount++
			}

			promotion.TotalRecipients = len(leadResults)
			promotion.SuccessCount = successCount
			promotion.FailCount = failCount
		} else {
			// 標準客戶模式
			// 獲取目標客戶列表
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
				return c.Status(500).JSON(fiber.Map{"error": "Failed to get customers"})
			}

			// 為每個客戶創建消息
			successCount := 0
			failCount := 0
			for _, customer := range customers {
				// 創建消息記錄
				comm := models.SupportCommunication{
					TenantID:          tenantID,
					CustomerID:        customer.ID,
					CommunicationType: "message", // internal message
					Subject:           promotion.Title,
					Content:           promotion.Content,
					Direction:         "outbound",
					Status:            "sent", // 標記為已發送
					Priority:          "normal",
				}

				if err := database.DB.Create(&comm).Error; err != nil {
					failCount++
					// 可以選擇記錄錯誤日誌
					continue
				}
				successCount++
			}

			promotion.TotalRecipients = len(customers)
			promotion.SuccessCount = successCount
			promotion.FailCount = failCount
		}

	case "whatsapp":
		// Whatsapp 模式：使用 WhatsApp API 發送
		// 先計算收件人數量用於 vCoin 預扣
		var whatsappRecipientCount int
		var whatsappAudienceMode string
		if promotion.TargetAudience != nil {
			if m, ok := promotion.TargetAudience["mode"].(string); ok {
				whatsappAudienceMode = m
			}
		}

		if whatsappAudienceMode == "lead_list" {
			leadResultIDs := parseUUIDListFromInterface(promotion.TargetAudience["lead_result_ids"])
			var leadResults []models.LeadFinderResult
			if err := database.DB.Where("tenant_id = ? AND id IN ?", tenantID, leadResultIDs).Find(&leadResults).Error; err == nil {
				for _, lead := range leadResults {
					if lead.Phone != "" {
						whatsappRecipientCount++
					}
				}
			}
		} else {
			var customers []models.Customer
			query := database.DB.Where("tenant_id = ?", tenantID)
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
			if err := query.Find(&customers).Error; err == nil {
				for _, customer := range customers {
					if customer.Phone != "" {
						whatsappRecipientCount++
					}
				}
			}
		}

		// vCoin 計費：每則 3 vCoin
		coinsNeeded := models.CalcWhatsAppPromoCoins(whatsappRecipientCount)
		if coinsNeeded > 0 {
			userID := middleware.GetUserID(c)
			ok, balance, err := ConsumeAICoins(tenantID, userID, coinsNeeded,
				"WhatsApp 推廣："+promotion.Title,
				models.JSONB{"promotion_id": promotion.ID.String(), "recipients": whatsappRecipientCount, "type": "whatsapp"})
			if err != nil || !ok {
				return c.Status(402).JSON(fiber.Map{
					"error":          "vCoin 餘額不足",
					"coins_required": coinsNeeded,
					"coins_balance":  balance,
					"recipients":     whatsappRecipientCount,
				})
			}
		}

		if err := SendWhatsAppPromotion(tenantID, promotion); err != nil {
			// 如果發送失敗，更新狀態為 failed
			promotion.Status = "failed"
			database.DB.Save(&promotion)
			return c.Status(500).JSON(fiber.Map{
				"error":   "Failed to send WhatsApp promotion",
				"details": err.Error(),
			})
		}

	case "email":
		// Email 模式：通過 email_jobs 佇列 bulk 發送 EDM
		var audienceMode string
		if promotion.TargetAudience != nil {
			if m, ok := promotion.TargetAudience["mode"].(string); ok {
				audienceMode = m
			}
		}

		emailSubject := strings.TrimSpace(promotion.EmailSubject)
		if emailSubject == "" {
			emailSubject = promotion.Title
		}

		// Build unsubscribe URL (placeholder — customer can opt out)
		// For now, link to the tenant's contact page; a proper unsubscribe endpoint can be added later
		unsubscribeURL := ""

		if audienceMode == "lead_list" {
			// Lead 列表模式
			leadResultIDs := parseUUIDListFromInterface(promotion.TargetAudience["lead_result_ids"])
			if len(leadResultIDs) == 0 {
				return c.Status(400).JSON(fiber.Map{"error": "No lead results selected"})
			}

			var leadResults []models.LeadFinderResult
			if err := database.DB.Where("tenant_id = ? AND id IN ?", tenantID, leadResultIDs).Find(&leadResults).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Failed to get lead results"})
			}

			// 計算有效 email 收件人數量
			validRecipients := 0
			for _, lead := range leadResults {
				if strings.TrimSpace(lead.Email) != "" {
					validRecipients++
				}
			}

			// vCoin 計費：每 10 封 1 vCoin
			coinsNeeded := models.CalcEmailPromoCoins(validRecipients)
			if coinsNeeded > 0 {
				userID := middleware.GetUserID(c)
				ok, balance, err := ConsumeAICoins(tenantID, userID, coinsNeeded,
					"Email 推廣："+promotion.Title,
					models.JSONB{"promotion_id": promotion.ID.String(), "recipients": validRecipients, "type": "email"})
				if err != nil || !ok {
					return c.Status(402).JSON(fiber.Map{
						"error":          "vCoin 餘額不足",
						"coins_required": coinsNeeded,
						"coins_balance":  balance,
						"recipients":     validRecipients,
					})
				}
			}

			successCount := 0
			failCount := 0
			for _, lead := range leadResults {
				if strings.TrimSpace(lead.Email) == "" {
					failCount++
					continue
				}
				if err := email.EnqueuePromotionEmail(tenantID, promotion.ID, lead.Email, lead.CompanyName, emailSubject, promotion.Content, unsubscribeURL); err != nil {
					log.Printf("❌ Promotion email enqueue failed: lead=%s err=%v", lead.Email, err)
					failCount++
					continue
				}
				database.DB.Model(&lead).Update("status", "contacted")
				successCount++
			}

			promotion.TotalRecipients = len(leadResults)
			promotion.SuccessCount = successCount
			promotion.FailCount = failCount

		} else {
			// 標準客戶模式
			var customers []models.Customer
			query := database.DB.Where("tenant_id = ?", tenantID)

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
				return c.Status(500).JSON(fiber.Map{"error": "Failed to get customers"})
			}

			// 計算有效 email 收件人數量
			validRecipients := 0
			for _, customer := range customers {
				if strings.TrimSpace(customer.Email) != "" {
					validRecipients++
				}
			}

			// vCoin 計費：每 10 封 1 vCoin
			coinsNeeded := models.CalcEmailPromoCoins(validRecipients)
			if coinsNeeded > 0 {
				userID := middleware.GetUserID(c)
				ok, balance, err := ConsumeAICoins(tenantID, userID, coinsNeeded,
					"Email 推廣："+promotion.Title,
					models.JSONB{"promotion_id": promotion.ID.String(), "recipients": validRecipients, "type": "email"})
				if err != nil || !ok {
					return c.Status(402).JSON(fiber.Map{
						"error":          "vCoin 餘額不足",
						"coins_required": coinsNeeded,
						"coins_balance":  balance,
						"recipients":     validRecipients,
					})
				}
			}

			successCount := 0
			failCount := 0
			for _, customer := range customers {
				if strings.TrimSpace(customer.Email) == "" {
					failCount++
					continue
				}
				if err := email.EnqueuePromotionEmail(tenantID, promotion.ID, customer.Email, customer.Name, emailSubject, promotion.Content, unsubscribeURL); err != nil {
					log.Printf("❌ Promotion email enqueue failed: customer=%s err=%v", customer.Email, err)
					failCount++
					continue
				}
				successCount++
			}

			promotion.TotalRecipients = len(customers)
			promotion.SuccessCount = successCount
			promotion.FailCount = failCount
		}
	}

	now := time.Now()
	promotion.Status = "sent"
	promotion.SentAt = &now

	if err := database.DB.Save(&promotion).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(promotion)
}

func DeletePromotion(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	id := c.Params("id")

	if err := database.DB.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Promotion{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Promotion deleted"})
}

// PublicCreateSupportCommunication 公開端點：從客戶網站聯繫表單創建客服通訊
// 從 subdomain 獲取 tenantID，不需要認證
func PublicCreateSupportCommunication(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	if subdomain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Subdomain is required"})
	}

	// 根據 subdomain 查找租戶
	var tenant models.Tenant
	if err := database.DB.Where("subdomain = ? AND status = ?", subdomain, "active").First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Message string `json:"message"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 驗證必填字段
	if req.Name == "" || req.Email == "" || req.Message == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name, email, and message are required"})
	}

	// 查找或創建客戶
	var customerID uuid.UUID
	var customer models.Customer
	if err := database.DB.Where("tenant_id = ? AND email = ?", tenant.ID, strings.TrimSpace(req.Email)).First(&customer).Error; err != nil {
		// 客戶不存在，創建新客戶
		customer = models.Customer{
			TenantID: tenant.ID,
			Name:     req.Name,
			Email:    strings.TrimSpace(req.Email),
			Phone:    req.Phone,
			Status:   "active",
		}
		if err := database.DB.Create(&customer).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to create customer"})
		}
	}
	customerID = customer.ID

	// 構建內容
	content := ""
	if req.Name != "" {
		content += "姓名: " + req.Name + "\n"
	}
	if req.Email != "" {
		content += "電郵: " + req.Email + "\n"
	}
	if req.Phone != "" {
		content += "電話: " + req.Phone + "\n"
	}
	if req.Message != "" {
		content += "\n訊息:\n" + req.Message
	}

	communication := models.SupportCommunication{
		TenantID:          tenant.ID,
		CustomerID:        customerID,
		CommunicationType: "contact_form",
		Subject:           req.Name + " 的聯絡表單",
		Content:           content,
		Direction:         "inbound",
		Status:            "open",
		Priority:          "normal",
	}

	if err := database.DB.Create(&communication).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Your message has been received. We will contact you soon.",
		"id":      communication.ID,
	})
}
