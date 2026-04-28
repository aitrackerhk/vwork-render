package handlers

import (
	"errors"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func setCustomerIDCookie(c *fiber.Ctx, customerID uuid.UUID) {
	// Keep consistent with webstore guard: cookie name = customer_id
	c.Cookie(&fiber.Cookie{
		Name:     "customer_id",
		Value:    customerID.String(),
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HTTPOnly: true,
		Secure:   false, // dev
		SameSite: "Lax",
		Path:     "/",
	})
}

func clearCustomerIDCookie(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     "customer_id",
		Value:    "",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   false,
		SameSite: "Lax",
		Path:     "/",
	})
}

func parsePhoneIdentifier(identifier string) (phoneCountryCode string, phone string, ok bool) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", "", false
	}
	// Expect formats like: "+852 12345678" or "+852-12345678"
	if !strings.HasPrefix(identifier, "+") {
		return "", "", false
	}
	parts := strings.Fields(strings.ReplaceAll(identifier, "-", " "))
	if len(parts) < 2 {
		return "", "", false
	}
	cc := strings.TrimSpace(parts[0])
	p := strings.TrimSpace(strings.Join(parts[1:], ""))
	if cc == "" || p == "" {
		return "", "", false
	}
	return cc, p, true
}

// PublicGetPhoneCountryCodes returns phone country codes for a tenant (no auth).
func PublicGetPhoneCountryCodes(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	var codes []models.PhoneCountryCode
	if err := database.DB.
		Where("tenant_id = ?", tenant.ID).
		Order("is_default DESC, code ASC").
		Find(&codes).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch phone country codes"})
	}
	return c.JSON(fiber.Map{"data": codes})
}

// PublicCustomerRegister creates a webstore customer account and logs them in (sets customer_id cookie).
func PublicCustomerRegister(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	var req struct {
		Name             string `json:"name"`
		Email            string `json:"email"`
		Phone            string `json:"phone"`
		PhoneCountryCode string `json:"phone_country_code"`
		Password         string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.PhoneCountryCode = strings.TrimSpace(req.PhoneCountryCode)

	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if len(req.Password) < 6 || len(req.Password) > 20 {
		return c.Status(400).JSON(fiber.Map{"error": "password must be between 6 and 20 characters"})
	}
	if req.Phone != "" && req.PhoneCountryCode == "" {
		return c.Status(400).JSON(fiber.Map{"error": "phone_country_code is required when phone is provided"})
	}

	// Uniqueness checks (within tenant)
	if req.Email != "" {
		var existing models.Customer
		if err := database.DB.
			Where("tenant_id = ? AND LOWER(email) = LOWER(?) AND email != ''", tenant.ID, req.Email).
			First(&existing).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "email already registered"})
		}
	}
	if req.Phone != "" && req.PhoneCountryCode != "" {
		var existing models.Customer
		if err := database.DB.
			Where("tenant_id = ? AND phone = ? AND phone_country_code = ? AND phone != ''", tenant.ID, req.Phone, req.PhoneCountryCode).
			First(&existing).Error; err == nil {
			return c.Status(400).JSON(fiber.Map{"error": "phone already registered"})
		}
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to hash password"})
	}

	now := time.Now()
	customer := models.Customer{
		TenantID:         tenant.ID,
		Name:             req.Name,
		Email:            req.Email,
		Phone:            req.Phone,
		PhoneCountryCode: req.PhoneCountryCode,
		PasswordHash:     string(hashed),
		Status:           "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if err := database.DB.Create(&customer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create customer"})
	}

	setCustomerIDCookie(c, customer.ID)

	return c.Status(201).JSON(fiber.Map{
		"message": "registered",
		"customer": fiber.Map{
			"id":                 customer.ID,
			"name":               customer.Name,
			"email":              customer.Email,
			"phone":              customer.Phone,
			"phone_country_code": customer.PhoneCountryCode,
		},
	})
}

// PublicCustomerLogin logs in a webstore customer (sets customer_id cookie).
func PublicCustomerLogin(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	var req struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" || req.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "identifier and password are required"})
	}

	var customer models.Customer

	// 只允許使用 email 登入
	if !strings.Contains(identifier, "@") {
		return c.Status(400).JSON(fiber.Map{"error": "please login with email"})
	}

	q := database.DB.Where("tenant_id = ? AND LOWER(email) = LOWER(?) AND email != ''", tenant.ID, identifier)

	if err := q.First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "failed to login"})
	}

	if customer.Status != "" && customer.Status != "active" {
		return c.Status(401).JSON(fiber.Map{"error": "account is inactive"})
	}
	if strings.TrimSpace(customer.PasswordHash) == "" {
		return c.Status(401).JSON(fiber.Map{"error": "password not set for this account"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(req.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "invalid credentials"})
	}

	setCustomerIDCookie(c, customer.ID)

	return c.JSON(fiber.Map{
		"message": "ok",
		"customer": fiber.Map{
			"id":                 customer.ID,
			"name":               customer.Name,
			"email":              customer.Email,
			"phone":              customer.Phone,
			"phone_country_code": customer.PhoneCountryCode,
		},
	})
}

// PublicCustomerLogout clears customer_id cookie.
func PublicCustomerLogout(c *fiber.Ctx) error {
	clearCustomerIDCookie(c)
	return c.JSON(fiber.Map{"message": "logged out"})
}

// PublicCustomerMe returns current logged-in customer (requires customer_id cookie).
func PublicCustomerMe(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	customerID, err := getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var customer models.Customer
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenant.ID, *customerID).First(&customer).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "customer not found"})
	}
	if customer.Status != "" && customer.Status != "active" {
		return c.Status(401).JSON(fiber.Map{"error": "account is inactive"})
	}

	return c.JSON(fiber.Map{
		"customer": fiber.Map{
			"id":                 customer.ID,
			"name":               customer.Name,
			"email":              customer.Email,
			"phone":              customer.Phone,
			"phone_country_code": customer.PhoneCountryCode,
			"address":            customer.Address,
		},
	})
}

// PublicCustomerUpdate updates current logged-in customer info (requires customer_id cookie).
func PublicCustomerUpdate(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	customerID, err := getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var customer models.Customer
	if err := database.DB.Where("tenant_id = ? AND id = ?", tenant.ID, *customerID).First(&customer).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "customer not found"})
	}

	var req struct {
		Name             *string `json:"name"`
		Email            *string `json:"email"`
		Phone            *string `json:"phone"`
		PhoneCountryCode *string `json:"phone_country_code"`
		Address          *string `json:"address"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}

	// Update fields if provided
	if req.Name != nil {
		customer.Name = strings.TrimSpace(*req.Name)
	}
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		// Check uniqueness if email changed
		if email != "" && email != customer.Email {
			var existing models.Customer
			if err := database.DB.Where("tenant_id = ? AND LOWER(email) = LOWER(?) AND id != ?", tenant.ID, email, customer.ID).First(&existing).Error; err == nil {
				return c.Status(400).JSON(fiber.Map{"error": "email already taken"})
			}
		}
		customer.Email = email
	}
	if req.Phone != nil {
		customer.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.PhoneCountryCode != nil {
		customer.PhoneCountryCode = strings.TrimSpace(*req.PhoneCountryCode)
	}
	if req.Address != nil {
		customer.Address = strings.TrimSpace(*req.Address)
	}

	customer.UpdatedAt = time.Now()

	if err := database.DB.Save(&customer).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to update profile"})
	}

	return c.JSON(fiber.Map{
		"message": "profile updated",
		"customer": fiber.Map{
			"id":                 customer.ID,
			"name":               customer.Name,
			"email":              customer.Email,
			"phone":              customer.Phone,
			"phone_country_code": customer.PhoneCountryCode,
			"address":            customer.Address,
		},
	})
}

// PublicGetCustomerOrders returns orders for the logged-in customer (requires customer_id cookie).
func PublicGetCustomerOrders(c *fiber.Ctx) error {
	subdomain := c.Params("subdomain")
	tenant, err := getTenantBySubdomain(subdomain)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	customerID, err := getCustomerIDFromCookie(c)
	if err != nil {
		return c.Status(401).JSON(fiber.Map{"error": err.Error()})
	}

	var orders []models.Order
	if err := database.DB.
		Where("tenant_id = ? AND customer_id = ?", tenant.ID, *customerID).
		Order("created_at DESC").
		Limit(200).
		Find(&orders).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to fetch orders"})
	}

	return c.JSON(fiber.Map{
		"data":  orders,
		"total": len(orders),
	})
}
