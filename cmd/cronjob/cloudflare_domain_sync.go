package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"io"
	"os"
	"strings"
	"time"

	"nwork/internal/database"
	"nwork/internal/models"
)

// Extra fields keys (must match API side)
const (
	extraWebsiteCustomDomains = "website_custom_domains"
	extraWebsiteCustomDomain  = "website_custom_domain"

	extraCFCustomHostnameID        = "cf_custom_hostname_id"
	extraCFCustomHostnameStatus    = "cf_custom_hostname_status"
	extraCFCustomHostnameSSLStatus = "cf_custom_hostname_ssl_status"
	extraCFCustomHostnameLastSync  = "cf_custom_hostname_last_sync"
)

func cfToken() string { return strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")) }
func cfZone() string  { return strings.TrimSpace(os.Getenv("CLOUDFLARE_ZONE_ID")) }

func cfEnabled() bool { return cfToken() != "" && cfZone() != "" }

func cnameTarget() string {
	if v := strings.TrimSpace(os.Getenv("CUSTOM_DOMAIN_CNAME_TARGET")); v != "" {
		return strings.ToLower(strings.TrimSuffix(v, "."))
	}
	// fallback: cname.<BASE_DOMAIN> (only for convenience)
	base := strings.TrimSpace(os.Getenv("BASE_DOMAIN"))
	if base == "" {
		return ""
	}
	return "cname." + strings.ToLower(strings.TrimSuffix(base, "."))
}

func pickWWWFromTenant(t models.Tenant) string {
	// Prefer array
	if arr, ok := t.ExtraFields[extraWebsiteCustomDomains].([]interface{}); ok {
		for _, it := range arr {
			if s, ok := it.(string); ok {
				h := strings.ToLower(strings.TrimSpace(s))
				if strings.HasPrefix(h, "www.") {
					return h
				}
			}
		}
	}
	// Fallback single
	if s, ok := t.ExtraFields[extraWebsiteCustomDomain].(string); ok {
		h := strings.ToLower(strings.TrimSpace(s))
		if strings.HasPrefix(h, "www.") {
			return h
		}
	}
	return ""
}

func cnamePointsToExpected(host string, expected string) (bool, string, error) {
	cn, err := net.LookupCNAME(host)
	observed := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cn), "."))
	if err != nil {
		return false, observed, err
	}
	if observed == expected || strings.HasSuffix(observed, "."+expected) {
		return true, observed, nil
	}
	return false, observed, nil
}

// ---- Minimal Cloudflare Custom Hostnames API (copy of handlers logic, for cronjob use) ----

type cfResp[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result T `json:"result"`
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
}

func cfErr(errs []struct {
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
	// Cronjob is a separate entrypoint; keep this minimal here.
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
	req.Header.Set("Authorization", "Bearer "+cfToken())
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e cfResp[any]
		_ = json.Unmarshal(raw, &e)
		if len(e.Errors) > 0 {
			return fmt.Errorf("%s", cfErr(e.Errors))
		}
		return fmt.Errorf("cloudflare http %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func cfFindHostname(zoneID, hostname string) (*cfCustomHostname, error) {
	u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/custom_hostnames?hostname=%s", zoneID, urlQueryEscape(hostname))
	var resp cfResp[[]cfCustomHostname]
	if err := cfRequest(http.MethodGet, u, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", cfErr(resp.Errors))
	}
	if len(resp.Result) == 0 {
		return nil, nil
	}
	return &resp.Result[0], nil
}

func cfCreateHostname(zoneID, hostname string) (*cfCustomHostname, error) {
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
		return nil, fmt.Errorf("%s", cfErr(resp.Errors))
	}
	return &resp.Result, nil
}

func cfGetHostname(zoneID, id string) (*cfCustomHostname, error) {
	u := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/custom_hostnames/%s", zoneID, id)
	var resp cfResp[cfCustomHostname]
	if err := cfRequest(http.MethodGet, u, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", cfErr(resp.Errors))
	}
	return &resp.Result, nil
}

func urlQueryEscape(s string) string {
	repl := strings.NewReplacer(" ", "%20", "#", "%23", "%", "%25", "?", "%3F", "&", "%26", "=", "%3D", "+", "%2B")
	return repl.Replace(s)
}

func getCFDomainSyncInterval() time.Duration {
	sec := getEnvInt("CF_CUSTOM_DOMAIN_SYNC_INTERVAL_SECONDS", 120)
	if sec <= 0 {
		sec = 120
	}
	return time.Duration(sec) * time.Second
}

func getCFDomainSyncLimit() int {
	lim := getEnvInt("CF_CUSTOM_DOMAIN_SYNC_LIMIT", 200)
	if lim <= 0 {
		lim = 200
	}
	return lim
}

// syncCloudflareCustomDomains scans tenants: if www CNAME is correct, auto create/sync Cloudflare custom hostname
// until ssl_status becomes "active".
func syncCloudflareCustomDomains() {
	if !cfEnabled() {
		// Not configured; don't spam logs too hard.
		return
	}
	expected := cnameTarget()
	if expected == "" {
		return
	}

	// Avoid multiple cronjob instances doing same work (best effort).
	var gotLock bool
	_ = database.DB.Raw("SELECT pg_try_advisory_lock(?)", int64(928374623)).Scan(&gotLock).Error
	if !gotLock {
		return
	}
	defer func() { _ = database.DB.Exec("SELECT pg_advisory_unlock(?)", int64(928374623)).Error }()

	limit := getCFDomainSyncLimit()

	var tenants []models.Tenant
	// Only tenants that have at least 1 custom domain configured
	if err := database.DB.
		Where("status = ?", "active").
		Where("COALESCE(jsonb_array_length(extra_fields->'website_custom_domains'),0) > 0").
		Limit(limit).
		Find(&tenants).Error; err != nil {
		log.Printf("❌ Cloudflare sync: failed to fetch tenants: %v", err)
		return
	}

	zoneID := cfZone()

	for _, t := range tenants {
		www := pickWWWFromTenant(t)
		if www == "" {
			continue
		}

		// Skip if already active
		if s, ok := t.ExtraFields[extraCFCustomHostnameSSLStatus].(string); ok {
			if strings.ToLower(strings.TrimSpace(s)) == "active" {
				continue
			}
		}

		ok, observed, err := cnamePointsToExpected(www, expected)
		if err != nil {
			// DNS not ready (NXDOMAIN etc). Just skip.
			continue
		}
		if !ok {
			_ = observed
			continue
		}

		// Create or find custom hostname
		ch, err := cfFindHostname(zoneID, www)
		if err != nil {
			log.Printf("❌ Cloudflare sync: lookup failed tenant=%s host=%s err=%v", t.ID, www, err)
			continue
		}
		if ch == nil {
			ch, err = cfCreateHostname(zoneID, www)
			if err != nil {
				log.Printf("❌ Cloudflare sync: create failed tenant=%s host=%s err=%v", t.ID, www, err)
				continue
			}
		}
		if ch != nil && ch.ID != "" {
			if latest, err := cfGetHostname(zoneID, ch.ID); err == nil {
				ch = latest
			}
		}

		if t.ExtraFields == nil {
			t.ExtraFields = models.JSONB{}
		}
		t.ExtraFields[extraCFCustomHostnameID] = ch.ID
		t.ExtraFields[extraCFCustomHostnameStatus] = ch.Status
		t.ExtraFields[extraCFCustomHostnameSSLStatus] = ch.SSL.Status
		t.ExtraFields[extraCFCustomHostnameLastSync] = time.Now().Format(time.RFC3339)

		if err := database.DB.Model(&models.Tenant{}).Where("id = ?", t.ID).Update("extra_fields", t.ExtraFields).Error; err != nil {
			log.Printf("❌ Cloudflare sync: save failed tenant=%s err=%v", t.ID, err)
		}
	}
}


