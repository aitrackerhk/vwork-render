package email

import (
	"strings"

	"nwork/internal/database"
	"nwork/internal/models"

	"github.com/google/uuid"
)

type Branding struct {
	CompanyName string
	LogoURL     string
}

// SystemBranding returns the platform-level branding (vWork logo and V-sys Limited)
// Used for system emails like welcome, password_reset, contact_form
func SystemBranding() Branding {
	c := mustCfg()
	return Branding{
		CompanyName: strings.TrimSpace(c.CompanyName),
		LogoURL:     "/static/logo3.png", // vWork logo
	}
}

func TenantBranding(tenantID uuid.UUID) Branding {
	// Default to config (platform-level) if tenant is missing
	c := mustCfg()
	b := Branding{
		CompanyName: strings.TrimSpace(c.CompanyName),
		LogoURL:     "",
	}
	if b.CompanyName == "" {
		b.CompanyName = strings.TrimSpace(c.AppName)
	}

	if tenantID == uuid.Nil {
		return b
	}

	// Prefer enterprise record for company name/logo
	var e models.Enterprise
	if err := database.DB.Where("tenant_id = ?", tenantID).First(&e).Error; err == nil {
		if strings.TrimSpace(e.Name) != "" {
			b.CompanyName = strings.TrimSpace(e.Name)
		}
		if e.LogoURL != nil {
			b.LogoURL = strings.TrimSpace(*e.LogoURL)
		}
		return b
	}

	// Fallback to tenant name
	var t models.Tenant
	if err := database.DB.Select("id", "name").Where("id = ?", tenantID).First(&t).Error; err == nil {
		if strings.TrimSpace(t.Name) != "" {
			b.CompanyName = strings.TrimSpace(t.Name)
		}
	}
	return b
}

// PublicAssetURL converts a stored logo URL (often "/uploads/...") into an absolute URL for email clients.
// If already absolute (http/https), it's returned as-is.
// For relative paths, it uses the base host (no tenant subdomain), matching how uploads are served in CMS.
// For system emails, it uses www.vworkai.com instead of the configured base domain.
func PublicAssetURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	ls := strings.ToLower(s)
	if strings.HasPrefix(ls, "http://") || strings.HasPrefix(ls, "https://") {
		return s
	}
	if strings.HasPrefix(s, "/") {
		c := mustCfg()
		// For system emails (logo3.png), use www.vworkai.com
		// For other assets, use the configured base domain
		baseDomain := c.Domain.BaseDomain
		if strings.Contains(s, "logo3.png") || strings.Contains(s, "/static/logo3.png") {
			// Use www.vworkai.com for system logo
			if strings.Contains(baseDomain, "vworkai.com") {
				baseDomain = "www.vworkai.com"
			}
		}
		return buildTenantURL(c.Domain.Scheme, "", baseDomain, s)
	}
	return s
}
