package middleware

import (
	"fmt"
	"log"
	"time"

	"nwork/internal/database"
	"nwork/internal/models"
	"nwork/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RequireAICoins 檢查並消耗 AI Coins 的 middleware
// 用於 AI 相關 API 端點
// 消耗優先順序：每日免費（只限簡單查詢）→ 月配額 → 購買的 coins
func RequireAICoins(coinsRequired int, description string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
		if !ok || tenantID == uuid.Nil {
			return c.Status(400).JSON(fiber.Map{"error": "未授權"})
		}

		userID, _ := c.Locals("user_id").(uuid.UUID)

		// 檢查餘額
		account, err := getOrCreateAICoinsAccountMW(tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "無法檢查 AI Coins 餘額"})
		}

		// 重置配額（每月/每日）
		_ = resetCoinsIfNeededMW(account)

		// 訂閱用戶（monthly/yearly）不提供每日免費，全部扣 coin
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "無法讀取租戶資訊"})
		}
		isPaid := models.IsPaidPlan(tenant.Plan)

		// 優先使用每日免費次數（交談+圖片生成，禁止文件/影片，且非訂閱用戶）
		usedDailyFree := false
		if !isPaid && account.CanUseDailyFree(coinsRequired) {
			// 使用每日免費
			usedDailyFree = true
			success, err := consumeDailyFree(account, userID, description)
			if !success || err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "每日免費消耗失敗"})
			}
			c.Locals("ai_coins_remaining", account.GetAvailableCoins())
			c.Locals("ai_coins_consumed", 0)
			c.Locals("used_daily_free", true)
			c.Locals("daily_free_remaining", account.GetDailyFreeRemaining())
			return c.Next()
		}

		// 不是免費次數，檢查 coins 餘額
		available := account.GetAvailableCoins()
		if available < coinsRequired {
			dailyFreeRemaining := account.GetDailyFreeRemaining()
			msg := fmt.Sprintf("AI Coins 餘額不足，需要 %d coins，當前餘額 %d", coinsRequired, available)
			if coinsRequired > models.AICoinsQuerySimple && dailyFreeRemaining > 0 {
				if coinsRequired >= models.AICoinsDocGenBase {
					msg += fmt.Sprintf("（每日免費不包含文件/影片生成，今日剩餘 %d 次）", dailyFreeRemaining)
				}
			}
			return c.Status(402).JSON(fiber.Map{
				"error":                "insufficient_ai_coins",
				"message":              msg,
				"required":             coinsRequired,
				"available":            available,
				"daily_free_remaining": dailyFreeRemaining,
				"purchase_url":         "/billing#ai-coins",
			})
		}

		// 消耗 coins
		success, remaining, err := consumeAICoins(account, userID, coinsRequired, description, nil)
		if !success || err != nil {
			return c.Status(402).JSON(fiber.Map{
				"error":     "insufficient_ai_coins",
				"message":   "AI Coins 消耗失敗",
				"available": remaining,
			})
		}

		// 將剩餘餘額放入 context
		c.Locals("ai_coins_remaining", remaining)
		c.Locals("ai_coins_consumed", coinsRequired)
		c.Locals("used_daily_free", usedDailyFree)

		return c.Next()
	}
}

// CheckAICoins 只檢查不消耗（用於預檢）
func CheckAICoins(coinsRequired int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
		if !ok || tenantID == uuid.Nil {
			return c.Status(400).JSON(fiber.Map{"error": "未授權"})
		}

		account, err := getOrCreateAICoinsAccountMW(tenantID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "無法檢查 AI Coins 餘額"})
		}

		// 重置配額
		_ = resetCoinsIfNeededMW(account)

		// 檢查每日免費（訂閱用戶不適用）
		var tenantChk models.Tenant
		_ = database.DB.Where("id = ?", tenantID).First(&tenantChk).Error
		if !models.IsPaidPlan(tenantChk.Plan) && account.CanUseDailyFree(coinsRequired) {
			c.Locals("ai_coins_available", account.GetAvailableCoins())
			c.Locals("daily_free_remaining", account.GetDailyFreeRemaining())
			c.Locals("can_use_daily_free", true)
			return c.Next()
		}

		available := account.GetAvailableCoins()
		if available < coinsRequired {
			return c.Status(402).JSON(fiber.Map{
				"error":                "insufficient_ai_coins",
				"message":              fmt.Sprintf("AI Coins 餘額不足，需要 %d coins，當前餘額 %d", coinsRequired, available),
				"required":             coinsRequired,
				"available":            available,
				"daily_free_remaining": account.GetDailyFreeRemaining(),
				"purchase_url":         "/billing#ai-coins",
			})
		}

		c.Locals("ai_coins_available", available)
		c.Locals("daily_free_remaining", account.GetDailyFreeRemaining())
		return c.Next()
	}
}

// 以下是 middleware 內部使用的函數（避免循環依賴）

// WelcomeBonusCoinsMW 新用戶歡迎禮 vCoin 數量
const WelcomeBonusCoinsMW = 50

func getOrCreateAICoinsAccountMW(tenantID uuid.UUID) (*models.TenantAICoins, error) {
	var account models.TenantAICoins
	err := database.DB.Where("tenant_id = ?", tenantID).First(&account).Error

	if err == gorm.ErrRecordNotFound {
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			return nil, err
		}

		// 獲取加盟商配額設定
		salesPartnerCoins := getSalesPartnerCoins(&tenant)

		now := utils.NowInTenantTimezone(tenantID)
		account = models.TenantAICoins{
			TenantID:         tenantID,
			Balance:          WelcomeBonusCoinsMW, // 歡迎禮 50 vCoin
			MonthlyAllotment: models.GetPlanMonthlyCoinsWithPartner(tenant.Plan, salesPartnerCoins),
			MonthlyUsed:      0,
			MonthlyResetAt:   getNextMonthStartMW(),
			DailyFreeUsed:    0,
			DailyFreeResetAt: getNextDayStartMW(now),
			PurchasedBalance: WelcomeBonusCoinsMW, // 歡迎禮算作購買餘額（不過期）
			TotalPurchased:   WelcomeBonusCoinsMW,
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
			Amount:       WelcomeBonusCoinsMW,
			BalanceAfter: WelcomeBonusCoinsMW,
			Description:  "歡迎禮 Welcome Bonus",
			Metadata:     models.JSONB{"reason": "welcome_bonus"},
		}
		if err := database.DB.Create(&welcomeTx).Error; err != nil {
			log.Printf("⚠️  Failed to record welcome bonus transaction: %v", err)
		}
	} else if err != nil {
		return nil, err
	}

	return &account, nil
}

// getSalesPartnerCoins 獲取加盟商設定的 AI Coins 配額
func getSalesPartnerCoins(tenant *models.Tenant) *int {
	if tenant.ExtraFields == nil {
		return nil
	}

	// 檢查是否有加盟商 code
	codeAny, ok := tenant.ExtraFields["sales_partner_application_id"]
	if !ok {
		return nil
	}

	appIDStr, ok := codeAny.(string)
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

	// 如果加盟商沒設定，使用預設
	if app.MonthlyAICoins == nil {
		defaultCoins := models.SalesPartnerDefaultCoins
		return &defaultCoins
	}

	return app.MonthlyAICoins
}

// resetCoinsIfNeededMW 重置月配額和每日免費次數
func resetCoinsIfNeededMW(account *models.TenantAICoins) error {
	now := utils.NowInTenantTimezone(account.TenantID)
	needsSave := false

	// 重置每日免費
	if now.After(account.DailyFreeResetAt) || now.Equal(account.DailyFreeResetAt) {
		account.DailyFreeUsed = 0
		account.DailyFreeResetAt = getNextDayStartMW(now)
		needsSave = true
	}

	// 重置月配額
	if now.After(account.MonthlyResetAt) || now.Equal(account.MonthlyResetAt) {
		var tenant models.Tenant
		if err := database.DB.Where("id = ?", account.TenantID).First(&tenant).Error; err != nil {
			return err
		}

		// 獲取加盟商配額（如有）
		salesPartnerCoins := getSalesPartnerCoins(&tenant)
		newAllotment := models.GetPlanMonthlyCoinsWithPartner(tenant.Plan, salesPartnerCoins)
		account.MonthlyAllotment = newAllotment
		account.MonthlyUsed = 0
		account.MonthlyResetAt = getNextMonthStartMW()
		needsSave = true
	}

	if needsSave {
		return database.DB.Save(account).Error
	}
	return nil
}

// consumeDailyFree 消耗每日免費次數
func consumeDailyFree(account *models.TenantAICoins, userID uuid.UUID, description string) (bool, error) {
	if account.GetDailyFreeRemaining() <= 0 {
		return false, nil
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		account.DailyFreeUsed++
		if err := tx.Save(account).Error; err != nil {
			return err
		}

		// 記錄交易（金額為 0，標記為免費）
		transaction := models.AICoinsTransaction{
			TenantID:     account.TenantID,
			UserID:       userID,
			Type:         "daily_free",
			Amount:       0,
			BalanceAfter: account.GetAvailableCoins(),
			Description:  fmt.Sprintf("[每日免費] %s", description),
			Metadata: models.JSONB{
				"daily_free_used":      account.DailyFreeUsed,
				"daily_free_remaining": account.GetDailyFreeRemaining(),
			},
			CreatedAt: utils.NowInTenantTimezone(account.TenantID),
		}
		return tx.Create(&transaction).Error
	})

	return err == nil, err
}

// consumeAICoins 消耗 AI Coins（月配額優先，然後購買的）
func consumeAICoins(account *models.TenantAICoins, userID uuid.UUID, amount int, description string, metadata models.JSONB) (bool, int, error) {
	available := account.GetAvailableCoins()
	if available < amount {
		return false, available, nil
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		monthlyRemaining := account.MonthlyAllotment - account.MonthlyUsed
		consumeAmount := amount

		if monthlyRemaining >= consumeAmount {
			account.MonthlyUsed += consumeAmount
		} else {
			if monthlyRemaining > 0 {
				account.MonthlyUsed = account.MonthlyAllotment
				consumeAmount -= monthlyRemaining
			}
			account.PurchasedBalance -= consumeAmount
		}
		account.TotalUsed += amount

		if err := tx.Save(account).Error; err != nil {
			return err
		}

		transaction := models.AICoinsTransaction{
			TenantID:     account.TenantID,
			UserID:       userID,
			Type:         models.AICoinsTypeConsume,
			Amount:       -amount,
			BalanceAfter: account.GetAvailableCoins(),
			Description:  description,
			Metadata:     metadata,
			CreatedAt:    utils.NowInTenantTimezone(account.TenantID),
		}
		return tx.Create(&transaction).Error
	})

	if err != nil {
		return false, account.GetAvailableCoins(), err
	}

	return true, account.GetAvailableCoins(), nil
}

func getNextMonthStartMW() time.Time {
	now := time.Now()
	nextMonth := now.AddDate(0, 1, 0)
	return time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, now.Location())
}

func getNextDayStartMW(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}

// ---------------------------------------------------------------------------
// Feature gating middleware
// ---------------------------------------------------------------------------

// RequireVideoGen 檢查租戶方案是否包含 vAi 影片生成功能（vSuite Pro 及以上）。
// 若不包含，API 端點回傳 403 JSON；頁面端點回傳 403 JSON（前端用 modal 顯示升級提示）。
func RequireVideoGen() fiber.Handler {
	return requireFeature(models.PlanHasVideoGen, models.RequiredPlanForVideo(), "vAi 影片生成")
}

// RequireLeadFinder 檢查租戶方案是否包含自動搵客功能（僅 vSuite Pro+）。
func RequireLeadFinder() fiber.Handler {
	return requireFeature(models.PlanHasLeadFinder, models.RequiredPlanForLeadFinder(), "自動搵客")
}

// requireFeature 通用方案功能檢查 middleware
func requireFeature(checker func(string) bool, requiredPlan string, featureName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tenantID, ok := c.Locals("tenant_id").(uuid.UUID)
		if !ok || tenantID == uuid.Nil {
			return c.Status(400).JSON(fiber.Map{"error": "未授權"})
		}

		var tenant models.Tenant
		if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "無法讀取租戶資訊"})
		}

		if !checker(tenant.Plan) {
			return c.Status(403).JSON(fiber.Map{
				"error":         "feature_not_available",
				"message":       fmt.Sprintf("您的方案不包含「%s」功能，請升級至 %s 或以上方案。", featureName, requiredPlan),
				"feature":       featureName,
				"required_plan": requiredPlan,
				"current_plan":  tenant.Plan,
				"upgrade_url":   "/billing",
				"redirect":      "/billing",
			})
		}

		// 將方案資訊放入 context 供下游使用
		c.Locals("tenant_plan", tenant.Plan)
		return c.Next()
	}
}
