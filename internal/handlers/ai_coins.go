package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/models"
	"nwork/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/checkout/session"
	"github.com/stripe/stripe-go/v80/customer"
	"gorm.io/gorm"
)

// GetAICoinsBalance 獲取 AI Coins 餘額
func GetAICoinsBalance(c *fiber.Ctx) error {
	tenantIDInterface := c.Locals("tenant_id")
	if tenantIDInterface == nil {
		return c.Status(401).JSON(fiber.Map{"error": "未登入"})
	}
	tenantID, ok := tenantIDInterface.(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "無效的租戶 ID"})
	}

	account, err := getOrCreateAICoinsAccount(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法獲取 AI Coins 帳戶"})
	}

	// 檢查是否需要重置配額
	if err := resetCoinsIfNeeded(account); err != nil {
		fmt.Printf("Error resetting coins: %v\n", err)
	}

	return c.JSON(fiber.Map{
		"balance":              account.GetAvailableCoins(),
		"monthly_allotment":    account.MonthlyAllotment,
		"monthly_used":         account.MonthlyUsed,
		"monthly_remaining":    account.MonthlyAllotment - account.MonthlyUsed,
		"purchased_balance":    account.PurchasedBalance,
		"monthly_reset_at":     account.MonthlyResetAt,
		"daily_free_limit":     models.DailyFreeQueries,
		"daily_free_used":      account.DailyFreeUsed,
		"daily_free_remaining": account.GetDailyFreeRemaining(),
		"daily_free_reset_at":  account.DailyFreeResetAt,
		"total_purchased":      account.TotalPurchased,
		"total_used":           account.TotalUsed,
	})
}

// GetAICoinsPlans 獲取 AI Coins 購買套餐
func GetAICoinsPlans(c *fiber.Ctx) error {
	var plans []models.AICoinsPlan
	if err := database.DB.Where("active = ?", true).Order("sort_order").Find(&plans).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法獲取套餐"})
	}

	// 如果沒有套餐，初始化預設套餐
	if len(plans) == 0 {
		for _, plan := range models.DefaultAICoinsPlans {
			database.DB.Create(&plan)
		}
		plans = models.DefaultAICoinsPlans
	}

	return c.JSON(plans)
}

// GetAICoinsTransactions 獲取 AI Coins 交易記錄
func GetAICoinsTransactions(c *fiber.Ctx) error {
	tenantIDInterface := c.Locals("tenant_id")
	if tenantIDInterface == nil {
		return c.Status(401).JSON(fiber.Map{"error": "未登入"})
	}
	tenantID, ok := tenantIDInterface.(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "無效的租戶 ID"})
	}
	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 20)
	txType := c.Query("type") // 可選：過濾類型

	offset := (page - 1) * limit

	query := database.DB.Model(&models.AICoinsTransaction{}).Where("tenant_id = ?", tenantID)
	if txType != "" {
		query = query.Where("type = ?", txType)
	}

	var total int64
	query.Count(&total)

	var transactions []models.AICoinsTransaction
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&transactions).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法獲取交易記錄"})
	}

	return c.JSON(fiber.Map{
		"transactions": transactions,
		"total":        total,
		"page":         page,
		"limit":        limit,
	})
}

// PurchaseAICoins 購買 AI Coins（創建 Stripe Checkout Session）
func PurchaseAICoins(c *fiber.Ctx) error {
	tenantIDInterface := c.Locals("tenant_id")
	if tenantIDInterface == nil {
		return c.Status(401).JSON(fiber.Map{"error": "未登入"})
	}
	tenantID, ok := tenantIDInterface.(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "無效的租戶 ID"})
	}
	userIDInterface := c.Locals("user_id")
	if userIDInterface == nil {
		return c.Status(401).JSON(fiber.Map{"error": "未登入"})
	}
	userID, ok := userIDInterface.(uuid.UUID)
	if !ok {
		return c.Status(401).JSON(fiber.Map{"error": "無效的用戶 ID"})
	}

	var req struct {
		PlanID   string `json:"plan_id"`  // small, medium, large, mega
		Language string `json:"language"` // en, zh, zh-CN
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	// 獲取套餐
	var plan models.AICoinsPlan
	if err := database.DB.Where("id = ? AND active = ?", req.PlanID, true).First(&plan).Error; err != nil {
		// 如果 DB 沒有，使用預設套餐
		for _, p := range models.DefaultAICoinsPlans {
			if p.ID == req.PlanID && p.Active {
				plan = p
				break
			}
		}
		if plan.ID == "" {
			return c.Status(404).JSON(fiber.Map{"error": "套餐不存在"})
		}
	}

	// 獲取租戶資訊以確定貨幣
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法獲取租戶資訊"})
	}

	// 取得 Stripe API Key
	key, err := stripeKey()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "付款服務未配置"})
	}
	stripe.Key = key

	// 確保有 Stripe Customer
	var stripeCustomerID string
	if tenant.StripeCustomerID != nil {
		stripeCustomerID = strings.TrimSpace(*tenant.StripeCustomerID)
	}
	if stripeCustomerID == "" {
		cfg := mustAppConfig()
		params := &stripe.CustomerParams{
			Name: stripe.String(tenant.Name),
			Metadata: map[string]string{
				"tenant_id":  tenant.ID.String(),
				"subdomain":  tenant.Subdomain,
				"app":        cfg.AppName,
				"created_by": "ai_coins_purchase",
			},
		}
		cust, err := customer.New(params)
		if err != nil {
			log.Printf("[ai_coins] Failed to create Stripe customer: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "Stripe customer 建立失敗"})
		}
		stripeCustomerID = cust.ID
		_ = database.DB.Model(&models.Tenant{}).Where("id = ?", tenant.ID).Update("stripe_customer_id", stripeCustomerID).Error
	}

	// 根據租戶地區決定貨幣和價格（預設 HKD）
	currency := "hkd"
	unitAmount := int64(plan.PriceHKD * 100) // Stripe 使用最小貨幣單位
	// 如果需要 USD，可通過 ExtraFields 或其他方式配置
	if tenant.ExtraFields != nil {
		if curr, ok := tenant.ExtraFields["currency"].(string); ok && strings.ToLower(curr) == "usd" {
			currency = "usd"
			unitAmount = int64(plan.PriceUSD * 100)
		}
	}

	// 構建成功/取消 URL
	successURL := tenantHostURL(tenant.Subdomain, "/vcoins?checkout=success&session_id={CHECKOUT_SESSION_ID}")
	cancelURL := tenantHostURL(tenant.Subdomain, "/vcoins?checkout=cancel")

	// Metadata
	meta := map[string]string{
		"type":      "ai_coins_purchase",
		"tenant_id": tenant.ID.String(),
		"user_id":   userID.String(),
		"subdomain": tenant.Subdomain,
		"plan_id":   plan.ID,
		"coins":     fmt.Sprintf("%d", plan.Coins),
	}

	// 創建 Checkout Session
	// 根據語言決定產品名稱和描述
	var productName, productDesc string
	switch req.Language {
	case "en":
		productName = fmt.Sprintf("vCoin - %s (%d coins)", plan.Name, plan.Coins)
		productDesc = fmt.Sprintf("Purchase %d vCoin", plan.Coins)
	case "zh-CN":
		productName = fmt.Sprintf("vCoin - %s（%d 个币）", plan.Name, plan.Coins)
		productDesc = fmt.Sprintf("购买 %d vCoin", plan.Coins)
	default: // zh (繁體中文)
		productName = fmt.Sprintf("vCoin - %s（%d 個幣）", plan.Name, plan.Coins)
		productDesc = fmt.Sprintf("購買 %d vCoin", plan.Coins)
	}

	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		Customer:   stripe.String(stripeCustomerID),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name:        stripe.String(productName),
						Description: stripe.String(productDesc),
					},
					UnitAmount: stripe.Int64(unitAmount),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Metadata:          meta,
		ClientReferenceID: stripe.String(tenant.ID.String()),
	}

	s, err := session.New(params)
	if err != nil {
		log.Printf("[ai_coins] Failed to create Stripe Checkout Session: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Stripe Checkout Session 建立失敗"})
	}

	log.Printf("[ai_coins] Created checkout session %s for tenant %s, plan %s, coins %d", s.ID, tenant.Subdomain, plan.ID, plan.Coins)

	return c.JSON(fiber.Map{
		"checkout_url": s.URL,
		"session_id":   s.ID,
		"plan":         plan,
	})
}

// handleAICoinsCheckoutCompleted 處理 AI Coins 購買完成的 Stripe webhook
func handleAICoinsCheckoutCompleted(cs *stripe.CheckoutSession) error {
	if cs == nil || cs.Metadata == nil {
		return nil
	}

	// 檢查 payment 是否成功
	if cs.PaymentStatus != stripe.CheckoutSessionPaymentStatusPaid {
		log.Printf("[ai_coins] Checkout session %s not paid, status: %s", cs.ID, cs.PaymentStatus)
		return nil
	}

	tenantIDStr := cs.Metadata["tenant_id"]
	userIDStr := cs.Metadata["user_id"]
	planID := cs.Metadata["plan_id"]
	coinsStr := cs.Metadata["coins"]

	tenantID, err := uuid.Parse(strings.TrimSpace(tenantIDStr))
	if err != nil || tenantID == uuid.Nil {
		log.Printf("[ai_coins] Invalid tenant_id in metadata: %s", tenantIDStr)
		return nil
	}

	userID, err := uuid.Parse(strings.TrimSpace(userIDStr))
	if err != nil || userID == uuid.Nil {
		log.Printf("[ai_coins] Invalid user_id in metadata: %s", userIDStr)
		return nil
	}

	var coins int
	fmt.Sscanf(coinsStr, "%d", &coins)
	if coins <= 0 {
		log.Printf("[ai_coins] Invalid coins in metadata: %s", coinsStr)
		return nil
	}

	// 添加 AI Coins 到帳戶
	metadata := models.JSONB{
		"checkout_session_id": cs.ID,
		"plan_id":             planID,
		"amount_total":        cs.AmountTotal,
		"currency":            string(cs.Currency),
	}
	if cs.Customer != nil {
		metadata["stripe_customer_id"] = cs.Customer.ID
	}

	description := fmt.Sprintf("購買 AI Coins - %s 套餐 (%d coins)", planID, coins)
	err = AddAICoins(tenantID, userID, coins, models.AICoinsTypePurchase, description, metadata)
	if err != nil {
		log.Printf("[ai_coins] Failed to add coins for tenant %s: %v", tenantIDStr, err)
		return err
	}

	log.Printf("[ai_coins] Successfully added %d coins to tenant %s from checkout session %s", coins, tenantIDStr, cs.ID)

	// 通知管理員：vCoin 購買
	var tenant models.Tenant
	if err := database.DB.Select("id", "name", "subdomain").Where("id = ?", tenantID).First(&tenant).Error; err == nil {
		amount := ""
		if cs.AmountTotal > 0 {
			amount = fmt.Sprintf("$%.2f %s", float64(cs.AmountTotal)/100.0, strings.ToUpper(string(cs.Currency)))
		}
		go email.EnqueueAdminNotification("vcoin_purchase", map[string]string{
			"tenant_name": tenant.Name,
			"subdomain":   tenant.Subdomain,
			"coins":       fmt.Sprintf("%d", coins),
			"amount":      amount,
			"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
		})
	}

	return nil
}

// AddAICoins 添加 AI Coins（購買成功後調用，或手動添加）
func AddAICoins(tenantID uuid.UUID, userID uuid.UUID, amount int, txType string, description string, metadata models.JSONB) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}

	account, err := getOrCreateAICoinsAccount(tenantID)
	if err != nil {
		return err
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		// 更新餘額
		account.PurchasedBalance += amount
		if txType == models.AICoinsTypePurchase {
			account.TotalPurchased += amount
		}

		if err := tx.Save(account).Error; err != nil {
			return err
		}

		// 記錄交易
		transaction := models.AICoinsTransaction{
			TenantID:     tenantID,
			UserID:       userID,
			Type:         txType,
			Amount:       amount,
			BalanceAfter: account.GetAvailableCoins(),
			Description:  description,
			Metadata:     metadata,
			CreatedAt:    time.Now(),
		}
		return tx.Create(&transaction).Error
	})
}

// ConsumeAICoins 消耗 AI Coins
// 返回: 是否成功, 剩餘餘額, 錯誤
func ConsumeAICoins(tenantID uuid.UUID, userID uuid.UUID, amount int, description string, metadata models.JSONB) (bool, int, error) {
	if amount <= 0 {
		return false, 0, fmt.Errorf("amount must be positive")
	}

	account, err := getOrCreateAICoinsAccount(tenantID)
	if err != nil {
		return false, 0, err
	}

	// 檢查是否需要重置配額
	if err := resetCoinsIfNeeded(account); err != nil {
		return false, 0, err
	}

	available := account.GetAvailableCoins()
	if available < amount {
		return false, available, fmt.Errorf("餘額不足：需要 %d coins，當前餘額 %d", amount, available)
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// 優先消耗月配額，再消耗購買的
		monthlyRemaining := account.MonthlyAllotment - account.MonthlyUsed
		if monthlyRemaining >= amount {
			// 全部從月配額扣除
			account.MonthlyUsed += amount
		} else {
			// 先用完月配額，再用購買的
			if monthlyRemaining > 0 {
				account.MonthlyUsed = account.MonthlyAllotment
				amount -= monthlyRemaining
			}
			account.PurchasedBalance -= amount
		}
		account.TotalUsed += amount

		if err := tx.Save(account).Error; err != nil {
			return err
		}

		// 記錄交易
		transaction := models.AICoinsTransaction{
			TenantID:     tenantID,
			UserID:       userID,
			Type:         models.AICoinsTypeConsume,
			Amount:       -amount, // 負數表示消耗
			BalanceAfter: account.GetAvailableCoins(),
			Description:  description,
			Metadata:     metadata,
			CreatedAt:    time.Now(),
		}
		return tx.Create(&transaction).Error
	})

	if err != nil {
		return false, account.GetAvailableCoins(), err
	}

	return true, account.GetAvailableCoins(), nil
}

// CheckAICoinsBalance 檢查是否有足夠的 AI Coins（用於 middleware）
func CheckAICoinsBalance(c *fiber.Ctx, requiredCoins int) error {
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	account, err := getOrCreateAICoinsAccount(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法檢查 AI Coins 餘額"})
	}

	// 重置配額
	_ = resetCoinsIfNeeded(account)

	available := account.GetAvailableCoins()
	if available < requiredCoins {
		return c.Status(402).JSON(fiber.Map{
			"error":        "insufficient_ai_coins",
			"message":      fmt.Sprintf("AI Coins 餘額不足，需要 %d coins，當前餘額 %d", requiredCoins, available),
			"required":     requiredCoins,
			"available":    available,
			"purchase_url": "/billing#ai-coins",
		})
	}

	return nil
}

// WelcomeBonusCoins 新用戶歡迎禮 vCoin 數量
const WelcomeBonusCoins = 50

// getOrCreateAICoinsAccount 獲取或創建 vCoin 帳戶
func getOrCreateAICoinsAccount(tenantID uuid.UUID) (*models.TenantAICoins, error) {
	var account models.TenantAICoins
	err := database.DB.Where("tenant_id = ?", tenantID).First(&account).Error

	if err == gorm.ErrRecordNotFound {
		// 獲取租戶方案
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			return nil, err
		}

		// 獲取加盟商配額（如有）
		salesPartnerCoins := getSalesPartnerCoinsHandler(&tenant)

		now := time.Now()
		// 創建新帳戶（包含歡迎禮 vCoin）
		account = models.TenantAICoins{
			TenantID:         tenantID,
			Balance:          WelcomeBonusCoins, // 歡迎禮 50 vCoin
			MonthlyAllotment: models.GetPlanMonthlyCoinsWithPartner(tenant.Plan, salesPartnerCoins),
			MonthlyUsed:      0,
			MonthlyResetAt:   getNextMonthStart(),
			DailyFreeUsed:    0,
			DailyFreeResetAt: getNextDayStart(now),
			PurchasedBalance: WelcomeBonusCoins, // 歡迎禮算作購買餘額（不過期）
			TotalPurchased:   WelcomeBonusCoins,
			TotalUsed:        0,
		}
		if err := database.DB.Create(&account).Error; err != nil {
			return nil, err
		}

		// 記錄歡迎禮交易
		welcomeTx := models.AICoinsTransaction{
			TenantID:     tenantID,
			UserID:       uuid.Nil, // 系統操作
			Type:         models.AICoinsTypeBonus,
			Amount:       WelcomeBonusCoins,
			BalanceAfter: WelcomeBonusCoins,
			Description:  "歡迎禮 Welcome Bonus",
			Metadata:     models.JSONB{"reason": "welcome_bonus"},
		}
		if err := database.DB.Create(&welcomeTx).Error; err != nil {
			// 記錄失敗不影響帳戶創建，只打 log
			log.Printf("⚠️  Failed to record welcome bonus transaction: %v", err)
		}
	} else if err != nil {
		return nil, err
	}

	return &account, nil
}

// getSalesPartnerCoinsHandler 獲取加盟商的 AI Coins 設定
func getSalesPartnerCoinsHandler(tenant *models.Tenant) *int {
	if tenant.ExtraFields == nil {
		return nil
	}

	// 檢查 ExtraFields 是否有 sales_partner_application_id
	appIDRaw, exists := tenant.ExtraFields["sales_partner_application_id"]
	if !exists || appIDRaw == nil {
		return nil
	}

	appIDStr, ok := appIDRaw.(string)
	if !ok || appIDStr == "" {
		return nil
	}

	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		return nil
	}

	// 查詢加盟商設定
	var app models.SalesPartnerApplication
	if err := database.DB.Where("id = ?", appID).First(&app).Error; err != nil {
		return nil
	}

	// 沒設定就返回默認值
	if app.MonthlyAICoins == nil {
		defaultCoins := models.SalesPartnerDefaultCoins
		return &defaultCoins
	}

	return app.MonthlyAICoins
}

// resetCoinsIfNeeded 重置月配額和每日免費（如果已到重置時間）
func resetCoinsIfNeeded(account *models.TenantAICoins) error {
	now := time.Now()
	needsSave := false

	// 重置每日免費
	if now.After(account.DailyFreeResetAt) || now.Equal(account.DailyFreeResetAt) {
		account.DailyFreeUsed = 0
		account.DailyFreeResetAt = getNextDayStart(now)
		needsSave = true
	}

	// 重置月配額
	if now.After(account.MonthlyResetAt) || now.Equal(account.MonthlyResetAt) {
		return resetMonthlyCoinsWithTx(account, now)
	}

	if needsSave {
		return database.DB.Save(account).Error
	}
	return nil
}

// resetMonthlyCoinsWithTx 重置月配額（帶交易記錄）
func resetMonthlyCoinsWithTx(account *models.TenantAICoins, now time.Time) error {
	// 獲取租戶方案以確定新配額
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", account.TenantID).First(&tenant).Error; err != nil {
		return err
	}

	// 獲取加盟商配額（如有）
	salesPartnerCoins := getSalesPartnerCoinsHandler(&tenant)

	return database.DB.Transaction(func(tx *gorm.DB) error {
		// 記錄過期的月配額（如果有剩餘）
		expiredCoins := account.MonthlyAllotment - account.MonthlyUsed
		if expiredCoins > 0 {
			transaction := models.AICoinsTransaction{
				TenantID:     account.TenantID,
				UserID:       uuid.Nil,
				Type:         models.AICoinsTypeExpire,
				Amount:       -expiredCoins,
				BalanceAfter: account.PurchasedBalance, // 只剩購買的
				Description:  fmt.Sprintf("月配額過期：%d coins", expiredCoins),
				CreatedAt:    now,
			}
			if err := tx.Create(&transaction).Error; err != nil {
				return err
			}
		}

		// 重置月配額（支持加盟商配額）
		newAllotment := models.GetPlanMonthlyCoinsWithPartner(tenant.Plan, salesPartnerCoins)
		account.MonthlyAllotment = newAllotment
		account.MonthlyUsed = 0
		account.MonthlyResetAt = getNextMonthStart()

		if err := tx.Save(account).Error; err != nil {
			return err
		}

		// 記錄新月配額（只有付費用戶才有月配額）
		if newAllotment > 0 {
			transaction := models.AICoinsTransaction{
				TenantID:     account.TenantID,
				UserID:       uuid.Nil,
				Type:         models.AICoinsTypeMonthlyReset,
				Amount:       newAllotment,
				BalanceAfter: account.GetAvailableCoins(),
				Description:  fmt.Sprintf("月配額重置：%d coins", newAllotment),
				CreatedAt:    now,
			}
			return tx.Create(&transaction).Error
		}
		return nil
	})
}

// getNextMonthStart 獲取下個月的開始時間
func getNextMonthStart() time.Time {
	now := time.Now()
	nextMonth := now.AddDate(0, 1, 0)
	return time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, now.Location())
}

// getNextDayStart 獲取明天的開始時間
func getNextDayStart(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}

// UpdateTenantAICoinsAllotment 更新租戶的月配額（方案變更時調用）
func UpdateTenantAICoinsAllotment(tenantID uuid.UUID, newPlan string) error {
	account, err := getOrCreateAICoinsAccount(tenantID)
	if err != nil {
		return err
	}

	newAllotment := models.GetPlanMonthlyCoins(newPlan)
	if newAllotment == account.MonthlyAllotment {
		return nil // 配額沒變
	}

	account.MonthlyAllotment = newAllotment
	return database.DB.Save(account).Error
}

// GetAICoinsUsageStats 獲取 AI Coins 使用統計
func GetAICoinsUsageStats(c *fiber.Ctx) error {
	tenantID := c.Locals("tenant_id").(uuid.UUID)

	// 獲取帳戶
	account, err := getOrCreateAICoinsAccount(tenantID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "無法獲取帳戶"})
	}

	// 獲取本月使用明細
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var usageByType []struct {
		Description string `json:"description"`
		TotalCoins  int    `json:"total_coins"`
		Count       int    `json:"count"`
	}

	database.DB.Model(&models.AICoinsTransaction{}).
		Select("description, SUM(ABS(amount)) as total_coins, COUNT(*) as count").
		Where("tenant_id = ? AND type = ? AND created_at >= ?", tenantID, models.AICoinsTypeConsume, monthStart).
		Group("description").
		Order("total_coins DESC").
		Scan(&usageByType)

	// 獲取每日使用趨勢（最近 30 天）
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	var dailyUsage []struct {
		Date       string `json:"date"`
		TotalCoins int    `json:"total_coins"`
	}

	database.DB.Model(&models.AICoinsTransaction{}).
		Select("DATE(created_at) as date, SUM(ABS(amount)) as total_coins").
		Where("tenant_id = ? AND type = ? AND created_at >= ?", tenantID, models.AICoinsTypeConsume, thirtyDaysAgo).
		Group("DATE(created_at)").
		Order("date").
		Scan(&dailyUsage)

	return c.JSON(fiber.Map{
		"account": fiber.Map{
			"balance":           account.GetAvailableCoins(),
			"monthly_allotment": account.MonthlyAllotment,
			"monthly_used":      account.MonthlyUsed,
			"monthly_remaining": account.MonthlyAllotment - account.MonthlyUsed,
			"purchased_balance": account.PurchasedBalance,
			"monthly_reset_at":  account.MonthlyResetAt,
		},
		"usage_by_type": usageByType,
		"daily_usage":   dailyUsage,
	})
}

// AdminAddBonusCoins 管理員添加贈送 coins（用於促銷、補償等）
func AdminAddBonusCoins(c *fiber.Ctx) error {
	userID := c.Locals("userID").(uuid.UUID)
	role := c.Locals("role").(string)

	if role != "admin" && role != "super_admin" {
		return c.Status(403).JSON(fiber.Map{"error": "無權限"})
	}

	var req struct {
		TenantID    string `json:"tenant_id"`
		Amount      int    `json:"amount"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的請求"})
	}

	targetTenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "無效的租戶 ID"})
	}

	if req.Amount <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "數量必須大於 0"})
	}

	err = AddAICoins(targetTenantID, userID, req.Amount, models.AICoinsTypeBonus, req.Description, models.JSONB{
		"added_by": userID.String(),
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "添加失敗：" + err.Error()})
	}

	utils.LogActivity(c.Locals("tenant_id").(uuid.UUID), userID, "add_bonus_coins", "ai_coins", nil,
		fmt.Sprintf(`{"key":"ai_coins.add_bonus","params":{"tenant_id":%q,"amount":%d}}`, targetTenantID, req.Amount), nil, c)

	return c.JSON(fiber.Map{
		"success": true,
		"message": fmt.Sprintf("成功添加 %d coins", req.Amount),
	})
}
