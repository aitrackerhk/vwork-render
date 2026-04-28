package email

import (
	"fmt"
	"strings"
)

// ValidateSMTPConfig validates the minimal SMTP config required to send emails.
// It never prints secret values; only indicates which environment keys are missing.
func ValidateSMTPConfig() error {
	c := mustCfg()

	var missing []string
	if strings.TrimSpace(c.Email.SMTPHost) == "" {
		missing = append(missing, "SMTP_HOST")
	}
	if strings.TrimSpace(c.Email.FromEmail) == "" {
		missing = append(missing, "SMTP_FROM_EMAIL")
	}

	// AUTH is optional for some SMTP servers, but if either is provided,
	// require the pair to avoid confusing auth failures.
	user := strings.TrimSpace(c.Email.SMTPUser)
	pass := strings.TrimSpace(c.Email.SMTPPassword)
	if user != "" && pass == "" {
		missing = append(missing, "SMTP_PASSWORD")
	}
	if pass != "" && user == "" {
		missing = append(missing, "SMTP_USER")
	}

	if len(missing) > 0 {
		return fmt.Errorf("smtp config missing: %s", strings.Join(missing, ", "))
	}
	return nil
}
















