package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/email"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"nwork/internal/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ── Batch invite emails to join current tenant ──

// SendTenantInvitations sends invitation emails to multiple recipients.
// POST /api/v1/tenant-invitations
// Body: { "emails": ["a@b.com", "c@d.com"] }
func SendTenantInvitations(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	currentUserID := middleware.GetUserID(c)

	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant ID is required"})
	}

	var req struct {
		Emails []string `json:"emails"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if len(req.Emails) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "At least one email is required"})
	}
	if len(req.Emails) > 20 {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot invite more than 20 people at once"})
	}

	// Get tenant info
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Get inviter info
	var inviter models.User
	if err := database.DB.Where("id = ?", currentUserID).First(&inviter).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "User not found"})
	}

	type InviteResult struct {
		Email   string `json:"email"`
		Status  string `json:"status"` // "sent", "already_member", "error"
		Message string `json:"message"`
	}

	results := make([]InviteResult, 0, len(req.Emails))

	for _, rawEmail := range req.Emails {
		emailAddr := strings.TrimSpace(strings.ToLower(rawEmail))
		if emailAddr == "" {
			continue
		}

		// Basic email validation
		if !strings.Contains(emailAddr, "@") || !strings.Contains(emailAddr, ".") {
			results = append(results, InviteResult{Email: emailAddr, Status: "error", Message: "Invalid email format"})
			continue
		}

		// Check if user is already a member of this tenant
		var existingUser models.User
		userExists := database.DB.Where("LOWER(email) = ?", emailAddr).First(&existingUser).Error == nil
		if userExists {
			var userTenant models.UserTenant
			if err := database.DB.Where("user_id = ? AND tenant_id = ?", existingUser.ID, tenantID).First(&userTenant).Error; err == nil {
				results = append(results, InviteResult{Email: emailAddr, Status: "already_member", Message: "Already a member"})
				continue
			}
		}

		// Check if there's already a pending invitation for this email + tenant
		var existing models.TenantInvitation
		if err := database.DB.Where("tenant_id = ? AND LOWER(email) = ? AND status = 'pending' AND expires_at > NOW()", tenantID, emailAddr).First(&existing).Error; err == nil {
			// Re-send the same invitation email (don't create a new one)
			// Generate new token for resend
			token, tokenHash, err := newTenantInviteToken()
			if err != nil {
				log.Printf("⚠️  SendTenantInvitations: token gen failed: email=%s err=%v", emailAddr, err)
				results = append(results, InviteResult{Email: emailAddr, Status: "error", Message: "Failed to generate token"})
				continue
			}
			// Update existing invitation with new token
			database.DB.Model(&existing).Updates(map[string]interface{}{
				"token_hash": tokenHash,
				"expires_at": time.Now().Add(7 * 24 * time.Hour),
				"updated_at": time.Now(),
			})

			inviteURL, err := email.TenantInviteURL(tenant.Subdomain, token)
			if err != nil {
				log.Printf("⚠️  SendTenantInvitations: URL build failed: email=%s err=%v", emailAddr, err)
				results = append(results, InviteResult{Email: emailAddr, Status: "error", Message: "Failed to build invite URL"})
				continue
			}

			if err := email.EnqueueTenantInviteEmail(tenant.ID, tenant.Name, inviter.Name, emailAddr, inviteURL); err != nil {
				log.Printf("⚠️  SendTenantInvitations: enqueue failed: email=%s err=%v", emailAddr, err)
				results = append(results, InviteResult{Email: emailAddr, Status: "error", Message: "Failed to send email"})
				continue
			}

			results = append(results, InviteResult{Email: emailAddr, Status: "sent", Message: "Invitation resent"})
			continue
		}

		// Create new invitation
		token, tokenHash, err := newTenantInviteToken()
		if err != nil {
			log.Printf("⚠️  SendTenantInvitations: token gen failed: email=%s err=%v", emailAddr, err)
			results = append(results, InviteResult{Email: emailAddr, Status: "error", Message: "Failed to generate token"})
			continue
		}

		now := time.Now()
		invitation := models.TenantInvitation{
			ID:        uuid.New(),
			TenantID:  tenantID,
			InviterID: currentUserID,
			Email:     emailAddr,
			TokenHash: tokenHash,
			Status:    "pending",
			ExpiresAt: now.Add(7 * 24 * time.Hour),
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := database.DB.Create(&invitation).Error; err != nil {
			log.Printf("⚠️  SendTenantInvitations: create invitation failed: email=%s err=%v", emailAddr, err)
			results = append(results, InviteResult{Email: emailAddr, Status: "error", Message: "Failed to create invitation"})
			continue
		}

		inviteURL, err := email.TenantInviteURL(tenant.Subdomain, token)
		if err != nil {
			log.Printf("⚠️  SendTenantInvitations: URL build failed: email=%s err=%v", emailAddr, err)
			results = append(results, InviteResult{Email: emailAddr, Status: "error", Message: "Failed to build invite URL"})
			continue
		}

		if err := email.EnqueueTenantInviteEmail(tenant.ID, tenant.Name, inviter.Name, emailAddr, inviteURL); err != nil {
			log.Printf("⚠️  SendTenantInvitations: enqueue failed: email=%s err=%v", emailAddr, err)
			results = append(results, InviteResult{Email: emailAddr, Status: "error", Message: "Failed to send email"})
			continue
		}

		// Log activity
		changes := map[string]interface{}{
			"invited_email": emailAddr,
			"inviter_id":    currentUserID.String(),
		}
		utils.LogActivity(tenantID, currentUserID, "invite", "tenant_invitation", &invitation.ID,
			fmt.Sprintf(`{"key":"tenant.invite","params":{"email":%q}}`, emailAddr),
			changes, c)

		results = append(results, InviteResult{Email: emailAddr, Status: "sent", Message: "Invitation sent"})
	}

	return c.JSON(fiber.Map{
		"results": results,
		"message": "Invitations processed",
	})
}

// ── Dismiss invite card on dashboard ──

// DismissInviteCard marks the invite card as dismissed for the current user in the current tenant.
// POST /api/v1/dashboard/dismiss-invite-card
func DismissInviteCard(c *fiber.Ctx) error {
	currentUserID := middleware.GetUserID(c)
	if currentUserID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant required"})
	}

	// Update extra_fields to set invite_card_dismissed_<tenantID> = true
	var user models.User
	if err := database.DB.Where("id = ?", currentUserID).First(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "User not found"})
	}

	if user.ExtraFields == nil {
		user.ExtraFields = make(models.JSONB)
	}
	key := "invite_card_dismissed_" + tenantID.String()
	user.ExtraFields[key] = true

	if err := database.DB.Model(&user).Update("extra_fields", user.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update"})
	}

	return c.JSON(fiber.Map{"message": "Card dismissed"})
}

// ── Check if invite card should be shown ──

// GetInviteCardStatus returns whether the invite card should be shown on dashboard.
// GET /api/v1/dashboard/invite-card-status
func GetInviteCardStatus(c *fiber.Ctx) error {
	currentUserID := middleware.GetUserID(c)
	if currentUserID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant required"})
	}

	var user models.User
	if err := database.DB.Where("id = ?", currentUserID).First(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "User not found"})
	}

	dismissed := false
	if user.ExtraFields != nil {
		key := "invite_card_dismissed_" + tenantID.String()
		if v, ok := user.ExtraFields[key].(bool); ok {
			dismissed = v
		}
	}

	return c.JSON(fiber.Map{
		"show_card": !dismissed,
	})
}

// ── Validate invitation token (public) ──

// ValidateTenantInvitation validates a tenant invitation token.
// GET /api/v1/auth/validate-invite?token=xxx
func ValidateTenantInvitation(c *fiber.Ctx) error {
	tokenStr := strings.TrimSpace(c.Query("token"))
	if tokenStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Token is required"})
	}

	hash := sha256.Sum256([]byte(tokenStr))

	var invitation models.TenantInvitation
	if err := database.DB.Where("token_hash = ? AND status = 'pending' AND expires_at > NOW()", hash[:]).
		First(&invitation).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid or expired invitation"})
	}

	// Get tenant info
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", invitation.TenantID).First(&tenant).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Get inviter info
	var inviter models.User
	inviterName := ""
	if err := database.DB.Where("id = ?", invitation.InviterID).First(&inviter).Error; err == nil {
		inviterName = inviter.Name
	}

	// Check if the invited email already has an account
	var existingUser models.User
	hasAccount := database.DB.Where("LOWER(email) = ?", strings.ToLower(invitation.Email)).First(&existingUser).Error == nil

	return c.JSON(fiber.Map{
		"valid":        true,
		"email":        invitation.Email,
		"tenant_name":  tenant.Name,
		"inviter_name": inviterName,
		"has_account":  hasAccount,
	})
}

// ── Accept invitation (public) ──

// AcceptTenantInvitation accepts a tenant invitation.
// The user must be logged in (auth cookie set) but does NOT need a tenant yet.
// Registered under auth-only middleware (no TenantMiddleware).
// POST /api/v1/auth/accept-invite
// Body: { "token": "xxx" }
func AcceptTenantInvitation(c *fiber.Ctx) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	tokenStr := strings.TrimSpace(req.Token)
	if tokenStr == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Token is required"})
	}

	hash := sha256.Sum256([]byte(tokenStr))

	var invitation models.TenantInvitation
	if err := database.DB.Where("token_hash = ? AND status = 'pending' AND expires_at > NOW()", hash[:]).
		First(&invitation).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid or expired invitation"})
	}

	// Get the current user from auth
	currentUserID := middleware.GetUserID(c)
	if currentUserID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Please login first"})
	}

	var user models.User
	if err := database.DB.Where("id = ?", currentUserID).First(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "User not found"})
	}

	tenantID := invitation.TenantID

	// Check if user is already in this tenant
	var existingUT models.UserTenant
	if err := database.DB.Where("user_id = ? AND tenant_id = ?", user.ID, tenantID).First(&existingUT).Error; err == nil {
		// Already a member, just mark invitation as accepted
		now := time.Now()
		database.DB.Model(&invitation).Updates(map[string]interface{}{
			"status":           "accepted",
			"accepted_at":      now,
			"accepted_user_id": user.ID,
			"updated_at":       now,
		})

		// Switch to this tenant
		switchUserToTenant(user.ID, tenantID)

		// Generate new token for this tenant
		token, err := utils.GenerateToken(user.ID, tenantID, user.Email, user.UserRole, "web")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
		}
		setAuthCookie(c, token, 30*24*time.Hour)

		return c.JSON(fiber.Map{
			"message":   "You are already a member of this team",
			"token":     token,
			"tenant_id": tenantID,
			"redirect":  "/dashboard",
		})
	}

	// Add user to tenant
	now := time.Now()
	userTenant := models.UserTenant{
		ID:        uuid.New(),
		UserID:    user.ID,
		TenantID:  tenantID,
		Role:      "user",
		IsDefault: false,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := database.DB.Create(&userTenant).Error; err != nil {
		log.Printf("⚠️  AcceptTenantInvitation: failed to create user_tenant: user_id=%s tenant_id=%s err=%v", user.ID, tenantID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to join tenant"})
	}

	// If the user has no current tenant, set this as default
	if user.TenantID == nil || *user.TenantID == uuid.Nil {
		database.DB.Model(&user).Update("tenant_id", tenantID)
		userTenant.IsDefault = true
		database.DB.Model(&userTenant).Update("is_default", true)
	}

	// Mark invitation as accepted
	database.DB.Model(&invitation).Updates(map[string]interface{}{
		"status":           "accepted",
		"accepted_at":      now,
		"accepted_user_id": user.ID,
		"updated_at":       now,
	})

	// Switch to this tenant
	switchUserToTenant(user.ID, tenantID)

	// Assign default role if user doesn't have one in this tenant
	assignDefaultRole(user.ID, tenantID)

	// Generate employee number
	generateEmployeeNumber(user.ID, tenantID)

	// Generate new token for this tenant
	token, err := utils.GenerateToken(user.ID, tenantID, user.Email, user.UserRole, "web")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}
	setAuthCookie(c, token, 30*24*time.Hour)

	log.Printf("✅ User %s accepted invitation and joined tenant %s", user.Email, tenantID)

	return c.JSON(fiber.Map{
		"message":   "Successfully joined the team",
		"token":     token,
		"tenant_id": tenantID,
		"redirect":  "/dashboard",
	})
}

// ── Helper functions ──

func newTenantInviteToken() (plainToken string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	plainToken = hex.EncodeToString(b)
	hashBytes := sha256.Sum256([]byte(plainToken))
	return plainToken, hashBytes[:], nil
}

// switchUserToTenant updates user's current tenant and last_used_at
func switchUserToTenant(userID uuid.UUID, tenantID uuid.UUID) {
	now := time.Now()
	database.DB.Model(&models.User{}).Where("id = ?", userID).Update("tenant_id", tenantID)
	database.DB.Model(&models.UserTenant{}).Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Update("last_used_at", now)
}

// assignDefaultRole assigns the default role for a user in a tenant
func assignDefaultRole(userID uuid.UUID, tenantID uuid.UUID) {
	var defaultRole models.Role
	if err := database.DB.Where("tenant_id = ? AND (is_default = true OR LOWER(name) = 'user')", tenantID).
		First(&defaultRole).Error; err == nil {
		database.DB.Model(&models.User{}).Where("id = ? AND role_id IS NULL", userID).
			Update("role_id", defaultRole.ID)
	}
}

// generateEmployeeNumber generates an employee number for a user in a tenant
func generateEmployeeNumber(userID uuid.UUID, tenantID uuid.UUID) {
	var count int64
	database.DB.Model(&models.UserTenant{}).Where("tenant_id = ?", tenantID).Count(&count)
	empNumber := fmt.Sprintf("EMP%04d", count)

	// Update user_tenant
	database.DB.Model(&models.UserTenant{}).Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Update("employee_number", empNumber)

	// Also update user's employee_number if empty
	database.DB.Model(&models.User{}).Where("id = ? AND (employee_number IS NULL OR employee_number = '')", userID).
		Update("employee_number", empNumber)
}
