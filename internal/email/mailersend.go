package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

const mailerSendAPIURL = "https://api.mailersend.com/v1/email"

// OTPSender is the minimal interface for sending OTP login emails.
// It is intentionally separate from the existing Sender interface so
// that OTP delivery can use MailerSend independently of SendGrid.
type OTPSender interface {
	SendLoginOTP(toEmail, code string) error
}

// MailerSendOTPSender sends OTP emails via the MailerSend REST API.
type MailerSendOTPSender struct {
	apiKey    string
	fromEmail string
	fromName  string
	client    *http.Client
	logger    *slog.Logger
}

// ConsoleOTPSender prints OTP to stdout (used when MAILERSEND_API_KEY is unset).
type ConsoleOTPSender struct {
	logger *slog.Logger
}

// NewOTPSender creates an OTPSender. Falls back to console output when apiKey is empty.
func NewOTPSender(apiKey, fromEmail, fromName string, logger *slog.Logger) OTPSender {
	if apiKey == "" {
		logger.Warn("MAILERSEND_API_KEY not set — OTP emails will be printed to console")
		return &ConsoleOTPSender{logger: logger}
	}
	logger.Info("MailerSend OTP sender configured", "from_email", fromEmail)
	return &MailerSendOTPSender{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		fromName:  fromName,
		client:    &http.Client{},
		logger:    logger,
	}
}

// mailerSendPayload mirrors the MailerSend /v1/email request body.
type mailerSendPayload struct {
	From    mailerSendAddress   `json:"from"`
	To      []mailerSendAddress `json:"to"`
	Subject string              `json:"subject"`
	Text    string              `json:"text"`
	HTML    string              `json:"html"`
}

type mailerSendAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

func (s *MailerSendOTPSender) SendLoginOTP(toEmail, code string) error {
	plainText := fmt.Sprintf(
		"Your DrivaBai login code is: %s\n\nThis code expires in 10 minutes.\n\nIf you did not request this, you can safely ignore this email.\n\nThe DrivaBai Team",
		code,
	)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8">
<style>
  body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;line-height:1.6;color:#333}
  .container{max-width:600px;margin:0 auto;padding:20px}
  .code{font-size:36px;font-weight:bold;letter-spacing:10px;color:#4ECDC4;text-align:center;padding:24px;background:#f5f5f5;border-radius:8px;margin:24px 0}
  .footer{margin-top:30px;font-size:12px;color:#666}
</style>
</head>
<body>
<div class="container">
  <h2>Your DrivaBai login code</h2>
  <p>Use the code below to sign in. It expires in <strong>10 minutes</strong>.</p>
  <div class="code">%s</div>
  <p>If you did not request this code, you can safely ignore this email.</p>
  <div class="footer"><p>The DrivaBai Team</p></div>
</div>
</body>
</html>`, code)

	payload := mailerSendPayload{
		From:    mailerSendAddress{Email: s.fromEmail, Name: s.fromName},
		To:      []mailerSendAddress{{Email: toEmail}},
		Subject: "Your DrivaBai login code",
		Text:    plainText,
		HTML:    htmlBody,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("mailersend: marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, mailerSendAPIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("mailersend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Error("mailersend: request failed", "error", err, "to", toEmail)
		return fmt.Errorf("mailersend: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		s.logger.Error("mailersend: non-2xx response", "status", resp.StatusCode, "to", toEmail)
		return fmt.Errorf("mailersend: API returned status %d", resp.StatusCode)
	}

	s.logger.Info("OTP email sent via MailerSend", "to", toEmail, "status", resp.StatusCode)
	return nil
}

func (s *ConsoleOTPSender) SendLoginOTP(toEmail, code string) error {
	// NOTE: logging OTP code is intentional in dev/console mode only.
	// Production always uses MailerSendOTPSender which never logs the code.
	s.logger.Info("OTP EMAIL (dev mode — MailerSend not configured)",
		"to", toEmail,
	)
	fmt.Printf("\n"+
		"╔══════════════════════════════════════════════════════════╗\n"+
		"║  📧 LOGIN OTP EMAIL (MailerSend not configured)          ║\n"+
		"╠══════════════════════════════════════════════════════════╣\n"+
		"║  To:   %-50s ║\n"+
		"║  Code: %-50s ║\n"+
		"╚══════════════════════════════════════════════════════════╝\n\n",
		toEmail, code)
	return nil
}
