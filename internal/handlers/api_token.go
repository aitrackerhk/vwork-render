package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ---------- request / response types ----------

// validProducts is the set of allowed values for the Product field.
var validProducts = map[string]bool{
	"vai":     true,
	"vwork":   true,
	"vmarket": true,
	"voffice": true,
}

type createAPITokenRequest struct {
	Name      string `json:"name"`
	Product   string `json:"product"`    // required: vai, vwork, vmarket, voffice
	Scopes    string `json:"scopes"`     // comma-separated, default "*"
	ExpiresIn *int   `json:"expires_in"` // optional: seconds until expiry (nil = never)
}

type updateAPITokenRequest struct {
	Name    *string `json:"name"`
	Product *string `json:"product"`
	Scopes  *string `json:"scopes"`
}

// ---------- helpers ----------

// generateRawToken creates a cryptographically random token string.
// Format: "vwk_" + 48 random hex chars = 52 chars total.
func generateRawToken() (string, error) {
	b := make([]byte, 24) // 24 bytes = 48 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "vwk_" + hex.EncodeToString(b), nil
}

// hashToken returns the SHA-256 hex digest of a raw token.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// ---------- CRUD handlers ----------

// CreateAPIToken generates a new system API token for the current tenant.
// The raw token is returned ONCE in the response; only the hash is persisted.
func CreateAPIToken(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	userID := middleware.GetUserID(c)
	if tenantID == uuid.Nil || userID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var req createAPITokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Name is required"})
	}

	product := strings.TrimSpace(strings.ToLower(req.Product))
	if !validProducts[product] {
		return c.Status(400).JSON(fiber.Map{"error": "Product is required and must be one of: vai, vwork, vmarket, voffice"})
	}

	scopes := strings.TrimSpace(req.Scopes)
	if scopes == "" {
		scopes = "*"
	}

	// Generate raw token
	rawToken, err := generateRawToken()
	if err != nil {
		log.Printf("❌ Failed to generate API token: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to generate token"})
	}

	// Compute expiry
	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	token := models.APIToken{
		TenantID:    tenantID,
		Name:        name,
		Product:     product,
		TokenHash:   hashToken(rawToken),
		TokenPrefix: rawToken[:12], // "vwk_" + first 8 hex chars
		Scopes:      scopes,
		Status:      "active",
		CreatedByID: userID,
		ExpiresAt:   expiresAt,
	}

	if err := database.DB.Create(&token).Error; err != nil {
		log.Printf("❌ Failed to save API token: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create token"})
	}

	log.Printf("✅ API token created: id=%s tenant=%s name=%s prefix=%s", token.ID, tenantID, name, token.TokenPrefix)

	// Return the raw token ONCE — after this the client can never retrieve it again.
	return c.Status(201).JSON(fiber.Map{
		"token":        rawToken,
		"token_prefix": token.TokenPrefix,
		"id":           token.ID,
		"name":         token.Name,
		"product":      token.Product,
		"scopes":       token.Scopes,
		"status":       token.Status,
		"expires_at":   token.ExpiresAt,
		"created_at":   token.CreatedAt,
		"message":      "Save this token now — it will not be shown again.",
	})
}

// ListAPITokens returns all tokens for the current tenant (raw token is never exposed).
func ListAPITokens(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var tokens []models.APIToken
	if err := database.DB.
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC").
		Find(&tokens).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to list tokens"})
	}

	// Build safe response (no hash exposed)
	list := make([]fiber.Map, 0, len(tokens))
	for _, t := range tokens {
		list = append(list, fiber.Map{
			"id":            t.ID,
			"name":          t.Name,
			"product":       t.Product,
			"token_prefix":  t.TokenPrefix,
			"scopes":        t.Scopes,
			"status":        t.Status,
			"created_by_id": t.CreatedByID,
			"last_used_at":  t.LastUsedAt,
			"expires_at":    t.ExpiresAt,
			"revoked_at":    t.RevokedAt,
			"created_at":    t.CreatedAt,
			"updated_at":    t.UpdatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"tokens": list,
		"total":  len(list),
	})
}

// GetAPIToken returns a single token's metadata.
func GetAPIToken(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid token ID"})
	}

	var token models.APIToken
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&token).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Token not found"})
	}

	return c.JSON(fiber.Map{
		"id":            token.ID,
		"name":          token.Name,
		"product":       token.Product,
		"token_prefix":  token.TokenPrefix,
		"scopes":        token.Scopes,
		"status":        token.Status,
		"created_by_id": token.CreatedByID,
		"last_used_at":  token.LastUsedAt,
		"expires_at":    token.ExpiresAt,
		"revoked_at":    token.RevokedAt,
		"created_at":    token.CreatedAt,
		"updated_at":    token.UpdatedAt,
	})
}

// UpdateAPIToken allows renaming or changing scopes.
func UpdateAPIToken(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid token ID"})
	}

	var token models.APIToken
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&token).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Token not found"})
	}

	var req updateAPITokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return c.Status(400).JSON(fiber.Map{"error": "Name cannot be empty"})
		}
		token.Name = name
	}
	if req.Product != nil {
		product := strings.TrimSpace(strings.ToLower(*req.Product))
		if !validProducts[product] {
			return c.Status(400).JSON(fiber.Map{"error": "Product must be one of: vai, vwork, vmarket, voffice"})
		}
		token.Product = product
	}
	if req.Scopes != nil {
		token.Scopes = strings.TrimSpace(*req.Scopes)
	}

	token.UpdatedAt = time.Now()
	if err := database.DB.Save(&token).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update token"})
	}

	return c.JSON(fiber.Map{
		"id":      token.ID,
		"name":    token.Name,
		"product": token.Product,
		"scopes":  token.Scopes,
		"status":  token.Status,
	})
}

// RevokeAPIToken permanently disables a token.
func RevokeAPIToken(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid token ID"})
	}

	var token models.APIToken
	if err := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).First(&token).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Token not found"})
	}

	if token.Status == "revoked" {
		return c.Status(400).JSON(fiber.Map{"error": "Token is already revoked"})
	}

	now := time.Now()
	token.Status = "revoked"
	token.RevokedAt = &now
	token.UpdatedAt = now

	if err := database.DB.Save(&token).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to revoke token"})
	}

	log.Printf("🔒 API token revoked: id=%s tenant=%s", token.ID, tenantID)

	return c.JSON(fiber.Map{
		"message": "Token revoked successfully",
		"id":      token.ID,
		"status":  token.Status,
	})
}

// DeleteAPIToken permanently removes a token record.
func DeleteAPIToken(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid token ID"})
	}

	result := database.DB.Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&models.APIToken{})
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to delete token"})
	}
	if result.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "Token not found"})
	}

	log.Printf("🗑️ API token deleted: id=%s tenant=%s", id, tenantID)

	return c.JSON(fiber.Map{"message": "Token deleted successfully"})
}
