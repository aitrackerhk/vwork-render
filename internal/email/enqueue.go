package email

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

var ErrEmailConfigMissing = errors.New("email config missing")

func validateSendConfig() error {
	c, err := GetConfig()
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Email.SMTPHost) == "" {
		return ErrEmailConfigMissing
	}
	if strings.TrimSpace(c.Email.FromEmail) == "" {
		return ErrEmailConfigMissing
	}
	return nil
}

// resolveEmailLanguage decides which language to use for an outbound email.
//
// Priority:
// - user.extra_fields.language (or lang) if userID provided
// - tenant.extra_fields.default_language (or defaultLanguage/default_lang) if tenantID provided
// - fallback: "zh"
//
// Returned values are normalized: "en" or "zh" (zh-Hant/zh-TW/zh-CN will map to "zh").
func resolveEmailLanguage(tenantID *uuid.UUID, userID *uuid.UUID) string {
	// default
	lang := "zh-hant"

	// 1) user preference
	if userID != nil && *userID != uuid.Nil {
		var u models.User
		if err := database.DB.Select("id", "extra_fields").Where("id = ?", *userID).First(&u).Error; err == nil {
			if u.ExtraFields != nil {
				if v, ok := u.ExtraFields["language"].(string); ok {
					lang = normalizeLang(v, lang)
				} else if v, ok := u.ExtraFields["lang"].(string); ok {
					lang = normalizeLang(v, lang)
				}
			}
		}
	}

	// 2) tenant default language
	if tenantID != nil && *tenantID != uuid.Nil {
		var t models.Tenant
		if err := database.DB.Select("id", "extra_fields").Where("id = ?", *tenantID).First(&t).Error; err == nil {
			if t.ExtraFields != nil {
				if v, ok := t.ExtraFields["default_language"].(string); ok {
					lang = normalizeLang(v, lang)
				} else if v, ok := t.ExtraFields["defaultLanguage"].(string); ok {
					lang = normalizeLang(v, lang)
				} else if v, ok := t.ExtraFields["default_lang"].(string); ok {
					lang = normalizeLang(v, lang)
				}
			}
		}
	}

	return normalizeLang(lang, "zh-hant")
}

func normalizeLang(raw string, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return fallback
	}
	// Accept variants
	if strings.HasPrefix(s, "en") {
		return "en"
	}
	// zh-Hans (Simplified)
	if strings.HasPrefix(s, "zh-cn") || strings.Contains(s, "hans") || strings.HasPrefix(s, "zh-sg") {
		return "zh-hans"
	}
	// zh-Hant (Traditional)
	if strings.HasPrefix(s, "zh-tw") || strings.Contains(s, "hant") || strings.HasPrefix(s, "zh-hk") || strings.HasPrefix(s, "zh-mo") || s == "zh" || strings.HasPrefix(s, "zh-") || strings.HasPrefix(s, "zh_") || strings.HasPrefix(s, "zh") {
		return "zh-hant"
	}
	return fallback
}

func resolveCustomerEmailLanguage(tenantID uuid.UUID, customerID *uuid.UUID) string {
	lang := resolveEmailLanguage(&tenantID, nil) // tenant default (zh-hant/zh-hans/en)
	if customerID == nil || *customerID == uuid.Nil {
		return lang
	}
	var c models.Customer
	if err := database.DB.Select("id", "extra_fields").Where("id = ? AND tenant_id = ?", *customerID, tenantID).First(&c).Error; err == nil {
		if c.ExtraFields != nil {
			if v, ok := c.ExtraFields["language"].(string); ok {
				return normalizeLang(v, lang)
			}
			if v, ok := c.ExtraFields["lang"].(string); ok {
				return normalizeLang(v, lang)
			}
		}
	}
	return lang
}

// EnqueueWelcomeEmail queues a welcome/login email for a newly registered user.
func EnqueueWelcomeEmail(tenantID uuid.UUID, userID uuid.UUID, tenantSubdomain string, toEmail string, toName string) error {
	_ = validateSendConfig() // allow enqueue even if SMTP not configured (worker can fail/retry)

	lang := resolveEmailLanguage(&tenantID, &userID)
	log.Printf("📧 EnqueueWelcomeEmail: tenant_id=%s user_id=%s email=%s lang=%s", tenantID, userID, toEmail, lang)

	// Use system Branding (vWork logo and V-sys Limited) for welcome emails
	b := SystemBranding()
	b.LogoURL = PublicAssetURL(b.LogoURL)
	log.Printf("📧 EnqueueWelcomeEmail: Branding company=%s logo=%s", b.CompanyName, b.LogoURL)

	subject, textBody, htmlBody, err := WelcomeEmail(b, tenantSubdomain, toName, lang)
	if err != nil {
		log.Printf("❌ EnqueueWelcomeEmail: welcomeEmail failed: tenant_id=%s user_id=%s err=%v", tenantID, userID, err)
		return err
	}
	log.Printf("📧 EnqueueWelcomeEmail: email template generated, subject=%s", subject)

	key := "welcome:" + userID.String()
	now := time.Now()

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         ptrUUID(userID),
		Kind:           "welcome",
		IdempotencyKey: &key,
		ToEmail:        toEmail,
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	result := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job)

	// 检查是否真的创建了记录（OnConflict DoNothing 时 RowsAffected 为 0）
	if result.Error != nil {
		log.Printf("❌ EnqueueWelcomeEmail: database create failed: tenant_id=%s user_id=%s err=%v", tenantID, userID, result.Error)
		return result.Error
	}
	if result.RowsAffected == 0 {
		// 记录已存在（可能是重复注册或之前的邮件任务），这不是错误，但记录一下
		// 注意：对于新用户注册，这通常不应该发生，除非是重复注册
		log.Printf("⚠️  welcome email job already exists (idempotency_key conflict): user_id=%s email=%s", userID, toEmail)
		return nil // 不返回错误，因为这是幂等性保护
	}

	log.Printf("✅ EnqueueWelcomeEmail: job created successfully: tenant_id=%s user_id=%s email=%s job_id=%d", tenantID, userID, toEmail, job.ID)
	return nil
}

// EnqueuePasswordResetEmail queues a password reset email.
func EnqueuePasswordResetEmail(tenantID uuid.UUID, userID uuid.UUID, tenantSubdomain string, toEmail string, toName string, resetURL string, idempotencyKey string) error {
	_ = validateSendConfig()

	lang := resolveEmailLanguage(&tenantID, &userID)
	// Use system branding (vWork logo and V-sys Limited) for password reset emails
	b := SystemBranding()
	b.LogoURL = PublicAssetURL(b.LogoURL)
	subject, textBody, htmlBody, err := PasswordResetEmail(b, tenantSubdomain, toName, resetURL, lang)
	if err != nil {
		return err
	}

	now := time.Now()
	key := strings.TrimSpace(idempotencyKey)
	var keyPtr *string
	if key != "" {
		keyPtr = &key
	}

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         ptrUUID(userID),
		Kind:           "password_reset",
		IdempotencyKey: keyPtr,
		ToEmail:        toEmail,
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	q := database.DB
	if keyPtr != nil {
		q = q.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true})
	}
	return q.Create(&job).Error
}

// EnqueueTrialLimitExceededEmail queues a trial limit exceeded notice email.
func EnqueueTrialLimitExceededEmail(tenantID uuid.UUID, tenantSubdomain string, userID uuid.UUID, toEmail string, toName string, graceUntil time.Time) error {
	_ = validateSendConfig()

	lang := resolveEmailLanguage(&tenantID, &userID)
	b := SystemBranding()
	b.LogoURL = PublicAssetURL(b.LogoURL)

	subject, textBody, htmlBody, err := TrialLimitExceededEmail(b, tenantSubdomain, toName, graceUntil, lang)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("trial_limit_exceeded:%s:%s:%s", tenantID.String(), userID.String(), graceUntil.Format("20060102"))
	now := time.Now()

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         ptrUUID(userID),
		Kind:           "trial_limit_exceeded",
		IdempotencyKey: &key,
		ToEmail:        toEmail,
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	result := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// EnqueueContactEmail queues a contact form email to the configured contact email address.
func EnqueueContactEmail(name, email, phone, product, subject, message string) error {
	_ = validateSendConfig() // allow enqueue even if SMTP not configured (worker can fail/retry)

	c, err := GetConfig()
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Email.ContactEmail) == "" {
		// 如果沒有配置 CONTACT_EMAIL，跳過發送（不返回錯誤，避免影響表單提交）
		return nil
	}

	// Contact form has no tenant/user context. Use system branding (vWork logo and V-sys Limited)
	b := SystemBranding()
	b.LogoURL = PublicAssetURL(b.LogoURL)
	subjectOut, textBody, htmlBody, err := ContactEmail(b, name, email, phone, product, subject, message, "zh-hant")
	if err != nil {
		return err
	}

	// 使用 name + email + timestamp 作為 idempotency key，避免重複發送
	key := "contact:" + strings.TrimSpace(email) + ":" + strings.TrimSpace(subject) + ":" + time.Now().Format("20060102150405")
	now := time.Now()

	job := models.EmailJob{
		TenantID:       nil, // 公開聯絡，沒有租戶
		UserID:         nil, // 公開聯絡，沒有用戶
		Kind:           "contact_form",
		IdempotencyKey: &key,
		ToEmail:        c.Email.ContactEmail, // 發送到配置的聯絡 email
		Subject:        subjectOut,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job).Error
}

// EnqueueMemberLevelUpgradeEmail queues a member level upgrade email.
func EnqueueMemberLevelUpgradeEmail(tenantID uuid.UUID, tenantSubdomain string, customerID uuid.UUID, customerEmail string, customerName string, oldLevelName string, newLevelName string) error {
	_ = validateSendConfig()

	if strings.TrimSpace(customerEmail) == "" {
		// 沒有 email，跳過
		return nil
	}

	lang := resolveCustomerEmailLanguage(tenantID, &customerID)
	b := TenantBranding(tenantID)
	b.LogoURL = PublicAssetURL(b.LogoURL)
	subject, textBody, htmlBody, err := MemberLevelUpgradeEmail(b, tenantSubdomain, customerName, oldLevelName, newLevelName, lang)
	if err != nil {
		return err
	}

	key := "member_upgrade:" + customerID.String() + ":" + newLevelName + ":" + time.Now().Format("20060102")
	now := time.Now()

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         nil,
		Kind:           "member_level_upgrade",
		IdempotencyKey: &key,
		ToEmail:        customerEmail,
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job).Error
}

// EnqueueOrderConfirmationEmail queues an order confirmation email.
func EnqueueOrderConfirmationEmail(tenantID uuid.UUID, tenantSubdomain string, customerID uuid.UUID, customerEmail string, customerName string, orderNumber string, orderDate string, totalAmount float64, orderType string) error {
	_ = validateSendConfig()

	if strings.TrimSpace(customerEmail) == "" {
		// 沒有 email，跳過
		return nil
	}

	lang := resolveCustomerEmailLanguage(tenantID, &customerID)
	b := TenantBranding(tenantID)
	b.LogoURL = PublicAssetURL(b.LogoURL)
	subject, textBody, htmlBody, err := OrderConfirmationEmail(b, tenantSubdomain, customerName, orderNumber, orderDate, totalAmount, orderType, lang)
	if err != nil {
		return err
	}

	key := "order_confirmation:" + orderNumber + ":" + time.Now().Format("20060102")
	now := time.Now()

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         nil,
		Kind:           "order_confirmation",
		IdempotencyKey: &key,
		ToEmail:        customerEmail,
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job).Error
}

// EnqueueUserInviteEmail queues a user invite email (for inviting user to join tenant).
func EnqueueUserInviteEmail(tenantID uuid.UUID, tenantSubdomain string, tenantName string, userID uuid.UUID, userEmail string, userName string, inviteURL string) error {
	_ = validateSendConfig()

	if strings.TrimSpace(userEmail) == "" {
		return nil
	}

	lang := resolveEmailLanguage(&tenantID, &userID)
	b := TenantBranding(tenantID)
	b.LogoURL = PublicAssetURL(b.LogoURL)
	subject, textBody, htmlBody, err := UserInviteEmail(b, tenantSubdomain, tenantName, userName, inviteURL, lang)
	if err != nil {
		return err
	}

	key := "user_invite:" + userID.String() + ":" + tenantID.String() + ":" + time.Now().Format("20060102")
	now := time.Now()

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         ptrUUID(userID),
		Kind:           "user_invite",
		IdempotencyKey: &key,
		ToEmail:        userEmail,
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job).Error
}

// EnqueueTenantInviteEmail queues an email inviting someone to join a tenant.
// This is for the new "invite to company" flow where the recipient may or may not have an account.
func EnqueueTenantInviteEmail(tenantID uuid.UUID, tenantName string, inviterName string, recipientEmail string, inviteURL string) error {
	_ = validateSendConfig()

	if strings.TrimSpace(recipientEmail) == "" {
		return nil
	}

	lang := resolveEmailLanguage(&tenantID, nil)
	b := TenantBranding(tenantID)
	b.LogoURL = PublicAssetURL(b.LogoURL)
	subject, textBody, htmlBody, err := TenantInviteEmail(b, tenantName, inviterName, inviteURL, lang)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("tenant_invite:%s:%s:%s", tenantID.String(), strings.ToLower(strings.TrimSpace(recipientEmail)), time.Now().Format("20060102150405"))
	now := time.Now()

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         nil,
		Kind:           "tenant_invite",
		IdempotencyKey: &key,
		ToEmail:        strings.TrimSpace(recipientEmail),
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job).Error
}

// EnqueueCustomerInviteEmail queues a customer invite email (for setting password).
func EnqueueCustomerInviteEmail(tenantID uuid.UUID, tenantSubdomain string, customerID uuid.UUID, customerEmail string, customerName string, inviteURL string) error {
	_ = validateSendConfig()

	if strings.TrimSpace(customerEmail) == "" {
		// 沒有 email，跳過
		return nil
	}

	lang := resolveCustomerEmailLanguage(tenantID, &customerID)
	b := TenantBranding(tenantID)
	b.LogoURL = PublicAssetURL(b.LogoURL)
	subject, textBody, htmlBody, err := CustomerInviteEmail(b, tenantSubdomain, customerName, inviteURL, lang)
	if err != nil {
		return err
	}

	key := "customer_invite:" + customerID.String() + ":" + time.Now().Format("20060102")
	now := time.Now()

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         nil,
		Kind:           "customer_invite",
		IdempotencyKey: &key,
		ToEmail:        customerEmail,
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job).Error
}

func ptrUUID(id uuid.UUID) *uuid.UUID { return &id }

// GetAdminEmails returns the list of admin notification emails.
// Priority: DB admin_settings table > config.Email.AdminEmails env var.
func GetAdminEmails() []string {
	// Try DB first
	var setting models.AdminSetting
	if err := database.DB.Where("key = ?", "admin_emails").First(&setting).Error; err == nil {
		val := strings.TrimSpace(setting.Value)
		if val != "" {
			var result []string
			for _, e := range strings.Split(val, ",") {
				e = strings.TrimSpace(e)
				if e != "" && strings.Contains(e, "@") {
					result = append(result, e)
				}
			}
			if len(result) > 0 {
				return result
			}
		}
	}

	// Fallback to env var
	c, err := GetConfig()
	if err != nil {
		return nil
	}
	val := strings.TrimSpace(c.Email.AdminEmails)
	if val == "" {
		return nil
	}
	var result []string
	for _, e := range strings.Split(val, ",") {
		e = strings.TrimSpace(e)
		if e != "" && strings.Contains(e, "@") {
			result = append(result, e)
		}
	}
	return result
}

// GetBrevoFreeDailyLimit returns the configured Brevo free daily email limit.
// Priority: DB admin_settings > config env var > default 300.
func GetBrevoFreeDailyLimit() int {
	var setting models.AdminSetting
	if err := database.DB.Where("key = ?", "brevo_free_daily_limit").First(&setting).Error; err == nil {
		val := strings.TrimSpace(setting.Value)
		if val != "" {
			var n int
			if _, err := fmt.Sscanf(val, "%d", &n); err == nil && n > 0 {
				return n
			}
		}
	}
	c, err := GetConfig()
	if err != nil {
		return 300
	}
	if c.Email.BrevoFreeDailyLimit > 0 {
		return c.Email.BrevoFreeDailyLimit
	}
	return 300
}

// GetDailyEmailUsage returns the count of emails sent/queued today.
func GetDailyEmailUsage() (int64, error) {
	var count int64
	today := time.Now().Format("2006-01-02")
	err := database.DB.Model(&models.EmailJob{}).
		Where("DATE(created_at) = ? AND status IN ('sent', 'queued', 'sending')", today).
		Count(&count).Error
	return count, err
}

// EnqueueAdminNotification sends admin notification emails to all configured admin emails.
// eventType: "new_registration", "new_subscription", "vcoin_purchase",
// "auto_outreach_quota_exceeded", "serper_credit_exhausted"
func EnqueueAdminNotification(eventType string, details map[string]string) {
	adminEmails := GetAdminEmails()
	if len(adminEmails) == 0 {
		log.Printf("⚠️  EnqueueAdminNotification: no admin emails configured, skipping %s notification", eventType)
		return
	}

	_ = validateSendConfig()

	b := SystemBranding()
	b.LogoURL = PublicAssetURL(b.LogoURL)

	subject, textBody, htmlBody, err := AdminNotificationEmail(b, eventType, details, "zh-hant")
	if err != nil {
		log.Printf("❌ EnqueueAdminNotification: template error: %v", err)
		return
	}

	now := time.Now()
	for _, adminEmail := range adminEmails {
		key := fmt.Sprintf("admin_notify:%s:%s:%s", eventType, strings.TrimSpace(adminEmail), now.Format("20060102150405"))
		job := models.EmailJob{
			TenantID:       nil, // system-level notification
			UserID:         nil,
			Kind:           "admin_notification",
			IdempotencyKey: &key,
			ToEmail:        strings.TrimSpace(adminEmail),
			Subject:        subject,
			BodyText:       textBody,
			BodyHTML:       htmlBody,
			Status:         "queued",
			Attempts:       0,
			RunAt:          now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		result := database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(&job)
		if result.Error != nil {
			log.Printf("❌ EnqueueAdminNotification: failed to enqueue for %s: %v", adminEmail, result.Error)
		} else {
			log.Printf("✅ EnqueueAdminNotification: queued %s notification to %s", eventType, adminEmail)
		}
	}
}

// EnqueuePromotionEmail queues a single promotion email for one recipient.
// Called in a loop by the SendPromotion handler for each target customer/lead.
func EnqueuePromotionEmail(tenantID uuid.UUID, promotionID uuid.UUID, toEmail string, toName string, emailSubject string, htmlContent string, unsubscribeURL string) error {
	_ = validateSendConfig()

	if strings.TrimSpace(toEmail) == "" {
		return nil
	}

	lang := resolveCustomerEmailLanguage(tenantID, nil)
	b := TenantBranding(tenantID)
	b.LogoURL = PublicAssetURL(b.LogoURL)

	subject, textBody, htmlBody, err := PromotionEmail(b, "", toName, emailSubject, htmlContent, unsubscribeURL, lang)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("promotion:%s:%s:%s", promotionID.String(), strings.TrimSpace(toEmail), time.Now().Format("20060102150405"))
	now := time.Now()

	job := models.EmailJob{
		TenantID:       ptrUUID(tenantID),
		UserID:         nil,
		Kind:           "promotion",
		IdempotencyKey: &key,
		ToEmail:        strings.TrimSpace(toEmail),
		Subject:        subject,
		BodyText:       textBody,
		BodyHTML:       htmlBody,
		Status:         "queued",
		Attempts:       0,
		RunAt:          now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "idempotency_key"}},
		DoNothing: true,
	}).Create(&job).Error
}
