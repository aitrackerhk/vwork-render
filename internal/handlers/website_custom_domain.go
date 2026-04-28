package handlers

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/middleware"
	"nwork/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	tenantExtraWebsiteCustomDomain      = "website_custom_domain"
	tenantExtraWebsiteCustomDomainSetAt = "website_custom_domain_set_at"
	tenantExtraWebsiteCustomDomains     = "website_custom_domains"
	tenantExtraWebsiteDomainSetupMethod = "website_domain_setup_method" // manual | cloudflare_domain_connect
)

func customDomainCnameTarget() string {
	// You should point customers' CNAME to this hostname (which must be on your Cloudflare zone for SSL for SaaS).
	// Example: cname.vworkai.com
	if v := strings.TrimSpace(os.Getenv("CUSTOM_DOMAIN_CNAME_TARGET")); v != "" {
		return strings.ToLower(v)
	}
	cfg := mustAppConfig()
	base := strings.TrimSpace(cfg.Domain.BaseDomain)
	if base == "" {
		return ""
	}
	return "cname." + strings.ToLower(base)
}

func normalizeHostname(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}

	// Allow user to paste full URL; extract host.
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("invalid url")
		}
		s = u.Host
	}

	// Strip path if any (common paste mistakes).
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}

	// Strip port.
	if h, _, err := net.SplitHostPort(s); err == nil && h != "" {
		s = h
	}

	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".")

	// Very basic validation: must have at least one dot (avoid "www" only).
	if s != "" && !strings.Contains(s, ".") {
		return "", fmt.Errorf("invalid hostname")
	}
	// Disallow spaces.
	if strings.ContainsAny(s, " \t\r\n") {
		return "", fmt.Errorf("invalid hostname")
	}
	return s, nil
}

// GetWebsiteCustomDomain returns the current tenant's configured custom domain (if any)
// plus the expected CNAME target to point to.
func GetWebsiteCustomDomain(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	domain := ""
	if v, ok := tenant.ExtraFields[tenantExtraWebsiteCustomDomain].(string); ok {
		domain = strings.TrimSpace(v)
	}
	method := "manual"
	if v, ok := tenant.ExtraFields[tenantExtraWebsiteDomainSetupMethod].(string); ok && strings.TrimSpace(v) != "" {
		method = strings.TrimSpace(v)
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"custom_domain": domain,
			"custom_domains": func() []string {
				out := []string{}
				if arr, ok := tenant.ExtraFields[tenantExtraWebsiteCustomDomains].([]interface{}); ok {
					for _, it := range arr {
						if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
							out = append(out, strings.TrimSpace(s))
						}
					}
				}
				// fallback: if not present, return single
				if len(out) == 0 && domain != "" {
					out = []string{domain}
				}
				return out
			}(),
			"cname_target": customDomainCnameTarget(),
			"setup_method": method,
		},
	})
}

// UpdateWebsiteDomainSetupMethod updates tenant's preferred DNS setup method:
// - manual: show DNS instructions
// - cloudflare_domain_connect: guide user through Cloudflare Domain Connect flow
func UpdateWebsiteDomainSetupMethod(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req struct {
		Method string `json:"method"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	m := strings.TrimSpace(req.Method)
	if m == "" {
		m = "manual"
	}
	if m != "manual" && m != "cloudflare_domain_connect" {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid method"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}
	if tenant.ExtraFields == nil {
		tenant.ExtraFields = models.JSONB{}
	}

	tenant.ExtraFields[tenantExtraWebsiteDomainSetupMethod] = m
	if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update setup method"})
	}

	return c.JSON(fiber.Map{
		"message": "Setup method updated successfully",
		"data": fiber.Map{
			"setup_method": m,
		},
	})
}

// UpdateWebsiteCustomDomain sets the tenant's desired custom domain (single hostname for MVP).
func UpdateWebsiteCustomDomain(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req struct {
		CustomDomain string `json:"custom_domain"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	host, err := normalizeHostname(req.CustomDomain)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid domain"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	if tenant.ExtraFields == nil {
		tenant.ExtraFields = models.JSONB{}
	}

	// Empty means disable custom domain.
	tenant.ExtraFields[tenantExtraWebsiteCustomDomain] = host
	tenant.ExtraFields[tenantExtraWebsiteCustomDomainSetAt] = time.Now().Format(time.RFC3339)
	if host == "" {
		tenant.ExtraFields[tenantExtraWebsiteCustomDomains] = []string{}
	} else {
		tenant.ExtraFields[tenantExtraWebsiteCustomDomains] = []string{host}
	}

	if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update custom domain"})
	}

	return c.JSON(fiber.Map{
		"message": "Custom domain updated successfully",
		"data": fiber.Map{
			"custom_domain": host,
			"custom_domains": func() []string {
				if host == "" {
					return []string{}
				}
				return []string{host}
			}(),
			"cname_target": customDomainCnameTarget(),
		},
	})
}

// UpdateWebsiteCustomDomains sets multiple hostnames for the tenant (e.g. www + apex).
func UpdateWebsiteCustomDomains(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req struct {
		CustomDomains []string `json:"custom_domains"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	seen := map[string]bool{}
	out := make([]string, 0, len(req.CustomDomains))
	for _, raw := range req.CustomDomains {
		h, err := normalizeHostname(raw)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid domain"})
		}
		if h == "" {
			continue
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}
	if tenant.ExtraFields == nil {
		tenant.ExtraFields = models.JSONB{}
	}

	tenant.ExtraFields[tenantExtraWebsiteCustomDomains] = out
	tenant.ExtraFields[tenantExtraWebsiteCustomDomainSetAt] = time.Now().Format(time.RFC3339)
	// Backward compatibility: keep single field as first item.
	if len(out) > 0 {
		tenant.ExtraFields[tenantExtraWebsiteCustomDomain] = out[0]
	} else {
		tenant.ExtraFields[tenantExtraWebsiteCustomDomain] = ""
	}

	if err := database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to update custom domains"})
	}

	return c.JSON(fiber.Map{
		"message": "Custom domains updated successfully",
		"data": fiber.Map{
			"custom_domains": out,
			"cname_target":   customDomainCnameTarget(),
		},
	})
}

// CheckWebsiteCustomDomainDNS checks whether a domain's CNAME points to the expected target.
// This is a best-effort DNS check (useful for guiding users in the UI).
func CheckWebsiteCustomDomainDNS(c *fiber.Ctx) error {
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var req struct {
		Domain string `json:"domain"`
	}
	_ = c.BodyParser(&req)

	domain, err := normalizeHostname(req.Domain)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid domain"})
	}

	// If not provided, fallback to first configured domain.
	if domain == "" {
		if v, ok := tenant.ExtraFields[tenantExtraWebsiteCustomDomain].(string); ok {
			domain = strings.TrimSpace(v)
		}
		if domain == "" {
			// try array
			if arr, ok := tenant.ExtraFields[tenantExtraWebsiteCustomDomains].([]interface{}); ok {
				for _, it := range arr {
					if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
						domain = strings.TrimSpace(s)
						break
					}
				}
			}
		}
	}
	if domain == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Custom domain not set"})
	}

	expected := strings.TrimSuffix(strings.ToLower(customDomainCnameTarget()), ".")
	if expected == "" {
		return c.Status(500).JSON(fiber.Map{"error": "CNAME target not configured"})
	}

	// Debug logging
	fmt.Printf("DNS DEBUG: Checking domain=%s, expected=%s\n", domain, expected)

	// LookupCNAME returns canonical name with trailing dot.
	cname, err := net.LookupCNAME(domain)
	cname = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(cname)), ".")

	// Debug logging
	fmt.Printf("DNS DEBUG: Raw CNAME result=%s, error=%v\n", cname, err)
	fmt.Printf("DNS DEBUG: Normalized CNAME=%s\n", cname)

	ok := err == nil && cname != "" && (cname == expected || strings.HasSuffix(cname, "."+expected))

	// Debug logging
	fmt.Printf("DNS DEBUG: Final OK=%t (match=%t, suffix_match=%t)\n", ok, cname == expected, strings.HasSuffix(cname, "."+expected))

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"custom_domain":  domain,
			"expected_cname": expected,
			"observed_cname": cname,
			"ok":             ok,
			"note": func() string {
				// Apex domains often can't use CNAME on many DNS providers.
				if err != nil {
					return "若你設定的是「無 www 的裸網域」，很多註冊商不支援 CNAME；請改用 www（CNAME）或使用支援 ALIAS/ANAME / Cloudflare DNS flattening / 交 NS 給 Cloudflare。"
				}
				return ""
			}(),
			"error": func() string {
				if err != nil {
					return err.Error()
				}
				return ""
			}(),
		},
	})
}
