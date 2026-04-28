package email

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

var ErrSMTPNotConfigured = errors.New("smtp not configured")

type Attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

type Message struct {
	FromEmail   string
	FromName    string
	ToEmail     string
	Subject     string
	TextBody    string
	HTMLBody    string
	Attachments []Attachment
}

func SendSMTP(msg Message) error {
	c := mustCfg()

	host := strings.TrimSpace(c.Email.SMTPHost)
	port := strings.TrimSpace(c.Email.SMTPPort)
	user := c.Email.SMTPUser
	pass := c.Email.SMTPPassword
	fromEmail := strings.TrimSpace(c.Email.FromEmail)
	fromName := strings.TrimSpace(c.Email.FromName)

	if host == "" || fromEmail == "" {
		return ErrSMTPNotConfigured
	}
	if port == "" {
		port = "587"
	}
	if msg.FromEmail == "" {
		msg.FromEmail = fromEmail
	}
	if msg.FromName == "" {
		msg.FromName = fromName
	}

	addr := net.JoinHostPort(host, port)

	dialer := net.Dialer{Timeout: time.Duration(c.Email.ConnectTimeoutSeconds) * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}
	defer conn.Close()

	// set write deadline for whole send (best-effort)
	_ = conn.SetDeadline(time.Now().Add(time.Duration(c.Email.SendTimeoutSeconds) * time.Second))

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	// STARTTLS
	if c.Email.UseStartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsCfg := &tls.Config{ServerName: host, InsecureSkipVerify: c.Email.InsecureSkipVerifyTLS}
			if err := client.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		}
	}

	// AUTH (optional)
	if strings.TrimSpace(user) != "" {
		auth := smtp.PlainAuth("", user, pass, host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(msg.FromEmail); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err := client.Rcpt(msg.ToEmail); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}

	bw := bufio.NewWriter(wc)
	mime := buildMIME(msg)
	if _, err := bw.WriteString(mime); err != nil {
		_ = wc.Close()
		return fmt.Errorf("write mime: %w", err)
	}
	if err := bw.Flush(); err != nil {
		_ = wc.Close()
		return fmt.Errorf("flush mime: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}

	return client.Quit()
}

func buildMIME(msg Message) string {
	from := msg.FromEmail
	if strings.TrimSpace(msg.FromName) != "" {
		from = fmt.Sprintf("%s <%s>", encodeHeaderWord(msg.FromName), msg.FromEmail)
	}

	subject := encodeHeaderWord(msg.Subject)
	boundary := "b_" + strings.ReplaceAll(fmt.Sprintf("%d", time.Now().UnixNano()), "-", "")
	mixedBoundary := "m_" + strings.ReplaceAll(fmt.Sprintf("%d", time.Now().UnixNano()+1), "-", "")

	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(msg.ToEmail)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(subject)
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")

	hasAttachments := len(msg.Attachments) > 0
	if hasAttachments {
		b.WriteString("Content-Type: multipart/mixed; boundary=\"")
		b.WriteString(mixedBoundary)
		b.WriteString("\"\r\n\r\n")

		b.WriteString("--")
		b.WriteString(mixedBoundary)
		b.WriteString("\r\n")
	}

	b.WriteString("Content-Type: multipart/alternative; boundary=\"")
	b.WriteString(boundary)
	b.WriteString("\"\r\n")
	b.WriteString("\r\n")

	// text part
	b.WriteString("--")
	b.WriteString(boundary)
	b.WriteString("\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(normalizeNewlines(msg.TextBody))
	b.WriteString("\r\n\r\n")

	// html part
	b.WriteString("--")
	b.WriteString(boundary)
	b.WriteString("\r\n")
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(msg.HTMLBody)
	b.WriteString("\r\n\r\n")

	b.WriteString("--")
	b.WriteString(boundary)
	b.WriteString("--\r\n")

	if hasAttachments {
		for _, att := range msg.Attachments {
			b.WriteString("\r\n--")
			b.WriteString(mixedBoundary)
			b.WriteString("\r\n")
			b.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.ContentType, encodeHeaderWord(att.Name)))
			b.WriteString("Content-Transfer-Encoding: base64\r\n")
			b.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", encodeHeaderWord(att.Name)))

			// Base64 encode the data and wrap it
			encoded := b64(att.Data)
			for i := 0; i < len(encoded); i += 76 {
				end := i + 76
				if end > len(encoded) {
					end = len(encoded)
				}
				b.WriteString(encoded[i:end])
				b.WriteString("\r\n")
			}
		}

		b.WriteString("\r\n--")
		b.WriteString(mixedBoundary)
		b.WriteString("--\r\n")
	}

	return b.String()
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// encodeHeaderWord encodes non-ascii safely (minimal RFC 2047 support).
func encodeHeaderWord(s string) string {
	// If ASCII only, return as-is.
	for _, r := range s {
		if r > 127 {
			// very small Q-encoding: fall back to UTF-8 base64
			return fmt.Sprintf("=?UTF-8?B?%s?=", b64([]byte(s)))
		}
	}
	return s
}

const b64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func b64(src []byte) string {
	if len(src) == 0 {
		return ""
	}
	var out strings.Builder
	out.Grow(((len(src) + 2) / 3) * 4)

	for i := 0; i < len(src); i += 3 {
		var n uint32
		remain := len(src) - i
		n |= uint32(src[i]) << 16
		if remain > 1 {
			n |= uint32(src[i+1]) << 8
		}
		if remain > 2 {
			n |= uint32(src[i+2])
		}

		out.WriteByte(b64Table[(n>>18)&63])
		out.WriteByte(b64Table[(n>>12)&63])
		if remain > 1 {
			out.WriteByte(b64Table[(n>>6)&63])
		} else {
			out.WriteByte('=')
		}
		if remain > 2 {
			out.WriteByte(b64Table[n&63])
		} else {
			out.WriteByte('=')
		}
	}
	return out.String()
}
