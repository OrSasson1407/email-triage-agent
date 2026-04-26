package notify

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/smtp"
	"time"

	"email-triage-agent/config"
	"email-triage-agent/db"
)

// EmailDigest sends a daily HTML digest via Gmail SMTP
type EmailDigest struct {
	cfg config.Config
}

func NewEmailDigest(cfg config.Config) *EmailDigest {
	return &EmailDigest{cfg: cfg}
}

var digestTmpl = template.Must(template.New("digest").Parse(`
<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
  <h2 style="color: #333;">📧 Daily Email Triage Digest</h2>
  <p style="color: #666;">{{ .Date }}</p>

  <div style="background: #f5f5f5; padding: 16px; border-radius: 8px; margin: 16px 0;">
    <h3 style="margin: 0 0 8px;">Summary</h3>
    <p>Total processed: <strong>{{ .Total }}</strong></p>
    <p>🔴 High urgency: <strong>{{ .High }}</strong></p>
    <p>🟡 Medium urgency: <strong>{{ .Medium }}</strong></p>
    <p>🟢 Low urgency: <strong>{{ .Low }}</strong></p>
  </div>

  <h3>Recent High Priority Emails</h3>
  <table style="width:100%; border-collapse: collapse;">
    <tr style="background: #eee;">
      <th style="padding: 8px; text-align: left;">From</th>
      <th style="padding: 8px; text-align: left;">Subject</th>
      <th style="padding: 8px; text-align: left;">Topic</th>
    </tr>
    {{ range .HighEmails }}
    <tr style="border-bottom: 1px solid #eee;">
      <td style="padding: 8px;">{{ .From }}</td>
      <td style="padding: 8px;">{{ .Subject }}</td>
      <td style="padding: 8px;"><code>{{ .Topic }}</code></td>
    </tr>
    {{ end }}
  </table>

  <p style="color: #999; font-size: 12px; margin-top: 24px;">
    Sent by Email Triage Agent at {{ .Date }}
  </p>
</body>
</html>
`))

type digestData struct {
	Date       string
	Total      int
	High       int
	Medium     int
	Low        int
	HighEmails []db.TriageLog
}

// Send sends the daily HTML digest email via SMTP
func (e *EmailDigest) Send(ctx context.Context, total, high, medium, low int, highEmails []db.TriageLog) error {
	if e.cfg.SMTPUser == "" || e.cfg.DigestRecipient == "" {
		return fmt.Errorf("SMTP not configured")
	}

	data := digestData{
		Date:       time.Now().Format(time.RFC1123),
		Total:      total,
		High:       high,
		Medium:     medium,
		Low:        low,
		HighEmails: highEmails,
	}

	var buf bytes.Buffer
	if err := digestTmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("render digest template: %w", err)
	}

	subject := fmt.Sprintf("Email Digest — %s | %d processed, %d urgent",
		time.Now().Format("Jan 2"), total, high)

	msg := buildMIMEMessage(e.cfg.SMTPUser, e.cfg.DigestRecipient, subject, buf.String())

	auth := smtp.PlainAuth("",
		e.cfg.SMTPUser,
		e.cfg.SMTPPass,
		e.cfg.SMTPHost,
	)

	addr := fmt.Sprintf("%s:%d", e.cfg.SMTPHost, e.cfg.SMTPPort)
	if err := smtp.SendMail(addr, auth, e.cfg.SMTPUser, []string{e.cfg.DigestRecipient}, []byte(msg)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	return nil
}

func buildMIMEMessage(from, to, subject, htmlBody string) string {
	return fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		from, to, subject, htmlBody,
	)
}