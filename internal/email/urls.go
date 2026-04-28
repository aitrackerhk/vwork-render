package email

import (
	"net/url"
	"strings"
)

func buildTenantURL(scheme, subdomain, baseDomain, path string) string {
	s := strings.TrimSpace(scheme)
	if s == "" {
		s = "https"
	}

	host := strings.TrimSpace(baseDomain)
	if strings.TrimSpace(subdomain) != "" {
		host = strings.TrimSpace(subdomain) + "." + host
	}

	p := path
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	return s + "://" + host + p
}

// LoginURL builds the tenant-scoped login URL.
func LoginURL(tenantSubdomain string) (string, error) {
	c, err := GetConfig()
	if err != nil {
		return "", err
	}
	return buildTenantURL(c.Domain.Scheme, strings.TrimSpace(tenantSubdomain), c.Domain.BaseDomain, "/login"), nil
}

// ResetPasswordURL builds the reset password page URL using target domain (www.vworkai.com).
func ResetPasswordURL(tenantSubdomain string, token string) (string, error) {
	c, err := GetConfig()
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("token", token)
	// Use www.vworkai.com for reset password URL (target domain) instead of tenant subdomain
	baseDomain := c.Domain.BaseDomain
	if strings.Contains(baseDomain, "vworkai.com") {
		baseDomain = "www.vworkai.com"
	}
	return buildTenantURL(c.Domain.Scheme, "", baseDomain, "/reset-password?"+q.Encode()), nil
}

// CustomerInviteURL builds the tenant-scoped customer invite page URL (for setting password).
func CustomerInviteURL(tenantSubdomain string, token string) (string, error) {
	c, err := GetConfig()
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("token", token)
	return buildTenantURL(c.Domain.Scheme, strings.TrimSpace(tenantSubdomain), c.Domain.BaseDomain, "/co/"+strings.TrimSpace(tenantSubdomain)+"/login?"+q.Encode()), nil
}

// UserInviteURL builds the user invite page URL (for setting password to join tenant).
func UserInviteURL(tenantSubdomain string, token string) (string, error) {
	c, err := GetConfig()
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("token", token)
	return buildTenantURL(c.Domain.Scheme, strings.TrimSpace(tenantSubdomain), c.Domain.BaseDomain, "/set-password?"+q.Encode()), nil
}

// TenantInviteURL builds the accept-invite page URL.
// The link lands on /accept-invite?token=xxx which handles both new and existing users.
func TenantInviteURL(tenantSubdomain string, token string) (string, error) {
	c, err := GetConfig()
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("token", token)
	return buildTenantURL(c.Domain.Scheme, strings.TrimSpace(tenantSubdomain), c.Domain.BaseDomain, "/accept-invite?"+q.Encode()), nil
}
