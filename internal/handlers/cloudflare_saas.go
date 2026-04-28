package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	tenantExtraCFCustomHostnameID        = "cf_custom_hostname_id"
	tenantExtraCFCustomHostnameStatus    = "cf_custom_hostname_status"
	tenantExtraCFCustomHostnameSSLStatus = "cf_custom_hostname_ssl_status"
	tenantExtraCFCustomHostnameLastSync  = "cf_custom_hostname_last_sync"
)

func cloudflareAPIToken() string { return strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")) }
func cloudflareZoneID() string   { return strings.TrimSpace(os.Getenv("CLOUDFLARE_ZONE_ID")) }

func cloudflareEnabled() bool {
	return cloudflareAPIToken() != "" && cloudflareZoneID() != ""
}

type cfResp[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Messages []any `json:"messages"`
	Result   T     `json:"result"`
}

type cfCustomHostname struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
	SSL      struct {
		Status string `json:"status"`
		Method string `json:"method"`
		Type   string `json:"type"`
	} `json:"ssl"`
	VerificationErrors    []string `json:"verification_errors"`
	OwnershipVerification struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"ownership_verification"`
	OwnershipVerificationHTTP struct {
		HTTPURL  string `json:"http_url"`
		HTTPBody string `json:"http_body"`
	} `json:"ownership_verification_http"`
}

func cfErrMessage(errs []struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}) string {
	if len(errs) == 0 {
		return "unknown error"
	}
	if errs[0].Message != "" {
		return errs[0].Message
	}
	return fmt.Sprintf("cloudflare error code %d", errs[0].Code)
}

func cfRequest(method, url string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cloudflareAPIToken())
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try parse Cloudflare error shape
		var e cfResp[any]
		_ = json.Unmarshal(raw, &e)
		if len(e.Errors) > 0 {
			return fmt.Errorf("%s", cfErrMessage(e.Errors))
		}
		return fmt.Errorf("cloudflare http %d", resp.StatusCode)
	}

	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func cfFindCustomHostname(zoneID, hostname string) (*cfCustomHostname, error) {
	u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/custom_hostnames?hostname=%s", zoneID, urlQueryEscape(hostname))
	var resp cfResp[[]cfCustomHostname]
	if err := cfRequest(http.MethodGet, u, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", cfErrMessage(resp.Errors))
	}
	if len(resp.Result) == 0 {
		return nil, nil
	}
	return &resp.Result[0], nil
}

// minimal url query escape (avoid extra deps)
func urlQueryEscape(s string) string {
	repl := strings.NewReplacer(" ", "%20", "#", "%23", "%", "%25", "?", "%3F", "&", "%26", "=", "%3D", "+", "%2B")
	return repl.Replace(s)
}

func cfCreateCustomHostname(zoneID, hostname string) (*cfCustomHostname, error) {
	u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/custom_hostnames", zoneID)
	payload := map[string]any{
		"hostname": hostname,
		"ssl": map[string]any{
			"method": "http",
			"type":   "dv",
		},
	}
	var resp cfResp[cfCustomHostname]
	if err := cfRequest(http.MethodPost, u, payload, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", cfErrMessage(resp.Errors))
	}
	return &resp.Result, nil
}

func cfGetCustomHostname(zoneID, id string) (*cfCustomHostname, error) {
	u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/custom_hostnames/%s", zoneID, id)
	var resp cfResp[cfCustomHostname]
	if err := cfRequest(http.MethodGet, u, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", cfErrMessage(resp.Errors))
	}
	return &resp.Result, nil
}

func pickWWWDomain(domains []string) string {
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if strings.HasPrefix(d, "www.") {
			return d
		}
	}
	return ""
}

// CloudflareSyncCustomHostname creates (or finds) a Cloudflare Custom Hostname for the tenant's www domain.
// It stores Cloudflare id/status into tenant.extra_fields and returns current status.
func CloudflareSyncCustomHostname(c *fiber.Ctx) error {
	if !cloudflareEnabled() {
		return c.Status(400).JSON(fiber.Map{"error": "Cloudflare not configured (missing CLOUDFLARE_API_TOKEN / CLOUDFLARE_ZONE_ID)"})
	}

	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}

	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	// Read www domain from extra_fields.website_custom_domains
	domains := []string{}
	if arr, ok := tenant.ExtraFields[tenantExtraWebsiteCustomDomains].([]interface{}); ok {
		for _, it := range arr {
			if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
				domains = append(domains, strings.TrimSpace(s))
			}
		}
	}
	www := pickWWWDomain(domains)
	if www == "" {
		// fallback to single
		if v, ok := tenant.ExtraFields[tenantExtraWebsiteCustomDomain].(string); ok {
			www = strings.ToLower(strings.TrimSpace(v))
		}
	}
	if www == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Custom domain not set"})
	}
	if !strings.HasPrefix(www, "www.") {
		return c.Status(400).JSON(fiber.Map{"error": "Only www domain is supported"})
	}

	zoneID := cloudflareZoneID()

	// Find existing by hostname
	found, err := cfFindCustomHostname(zoneID, www)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Cloudflare lookup failed: " + err.Error()})
	}
	var ch *cfCustomHostname
	if found != nil {
		ch = found
	} else {
		created, err := cfCreateCustomHostname(zoneID, www)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Cloudflare create failed: " + err.Error()})
		}
		ch = created
	}

	// Refresh by id for latest ssl status (best effort)
	if ch != nil && ch.ID != "" {
		if latest, err := cfGetCustomHostname(zoneID, ch.ID); err == nil {
			ch = latest
		}
	}

	if tenant.ExtraFields == nil {
		tenant.ExtraFields = models.JSONB{}
	}
	tenant.ExtraFields[tenantExtraCFCustomHostnameID] = ch.ID
	tenant.ExtraFields[tenantExtraCFCustomHostnameStatus] = ch.Status
	tenant.ExtraFields[tenantExtraCFCustomHostnameSSLStatus] = ch.SSL.Status
	tenant.ExtraFields[tenantExtraCFCustomHostnameLastSync] = time.Now().Format(time.RFC3339)
	_ = database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"hostname":   ch.Hostname,
			"id":         ch.ID,
			"status":     ch.Status,
			"ssl_status": ch.SSL.Status,
			"ssl_method": ch.SSL.Method,
			"ssl_type":   ch.SSL.Type,
			"last_sync":  tenant.ExtraFields[tenantExtraCFCustomHostnameLastSync],
			"configured": true,
			"zone_id":    zoneID,
		},
	})
}

// CloudflareCustomHostnameStatus returns stored status and (optionally) refreshes from Cloudflare if id exists.
func CloudflareCustomHostnameStatus(c *fiber.Ctx) error {
	if !cloudflareEnabled() {
		return c.Status(400).JSON(fiber.Map{"error": "Cloudflare not configured (missing CLOUDFLARE_API_TOKEN / CLOUDFLARE_ZONE_ID)"})
	}
	tenantID := middleware.GetTenantID(c)
	if tenantID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "Tenant not found"})
	}
	var tenant models.Tenant
	if err := database.DB.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Tenant not found"})
	}

	id, _ := tenant.ExtraFields[tenantExtraCFCustomHostnameID].(string)
	id = strings.TrimSpace(id)
	zoneID := cloudflareZoneID()

	var ch *cfCustomHostname
	if id != "" {
		if latest, err := cfGetCustomHostname(zoneID, id); err == nil {
			ch = latest
			if tenant.ExtraFields == nil {
				tenant.ExtraFields = models.JSONB{}
			}
			tenant.ExtraFields[tenantExtraCFCustomHostnameStatus] = ch.Status
			tenant.ExtraFields[tenantExtraCFCustomHostnameSSLStatus] = ch.SSL.Status
			tenant.ExtraFields[tenantExtraCFCustomHostnameLastSync] = time.Now().Format(time.RFC3339)
			_ = database.DB.Model(&tenant).Update("extra_fields", tenant.ExtraFields).Error
		}
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"id":         id,
			"status":     tenant.ExtraFields[tenantExtraCFCustomHostnameStatus],
			"ssl_status": tenant.ExtraFields[tenantExtraCFCustomHostnameSSLStatus],
			"last_sync":  tenant.ExtraFields[tenantExtraCFCustomHostnameLastSync],
			"refreshed":  ch != nil,
			"ownership_verification": func() interface{} {
				if ch != nil {
					return ch.OwnershipVerification
				}
				return nil
			}(),
			"verification_errors": func() []string {
				if ch != nil {
					return ch.VerificationErrors
				}
				return nil
			}(),
		},
	})
}
