package utils

import (
	"net"
	"os"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/oschwald/geoip2-golang"
)

var (
	geoipOnce sync.Once
	geoipDB   *geoip2.Reader
)

func geoipPath() string {
	// Optional override
	if p := strings.TrimSpace(os.Getenv("GEOIP_DB_PATH")); p != "" {
		return p
	}
	// Common convention in this repo
	return "./data/GeoLite2-Country.mmdb"
}

func geoipInit() {
	p := geoipPath()
	r, err := geoip2.Open(p)
	if err != nil {
		geoipDB = nil
		return
	}
	geoipDB = r
}

func firstClientIP(c *fiber.Ctx) net.IP {
	// Prefer proxy headers if present (e.g. GCLB / Nginx). Take the first valid IP.
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		v := strings.TrimSpace(c.Get(header))
		if v == "" {
			continue
		}
		parts := strings.Split(v, ",")
		for _, part := range parts {
			ipStr := strings.TrimSpace(part)
			if ipStr == "" {
				continue
			}
			if ip := net.ParseIP(ipStr); ip != nil {
				return ip
			}
		}
	}
	// Fallback
	if ip := net.ParseIP(strings.TrimSpace(c.IP())); ip != nil {
		return ip
	}
	return nil
}

// DetectCountryFromRequest returns ISO country code (e.g. "HK") based on client IP.
// Requires GeoLite2 mmdb present on disk; otherwise returns "".
func DetectCountryFromRequest(c *fiber.Ctx) string {
	geoipOnce.Do(geoipInit)
	if geoipDB == nil {
		return ""
	}
	ip := firstClientIP(c)
	if ip == nil {
		return ""
	}
	rec, err := geoipDB.Country(ip)
	if err != nil {
		return ""
	}
	code := strings.ToUpper(strings.TrimSpace(rec.Country.IsoCode))
	if len(code) != 2 || code == "XX" {
		return ""
	}
	return code
}


