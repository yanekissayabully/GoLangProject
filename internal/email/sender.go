package email

import (
	"fmt"
	"log/slog"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type Sender interface {
	SendVerificationEmail(toEmail, toName, code string) error
	SendPasswordResetEmail(toEmail, toName, token string) error
}

type SendGridSender struct {
	client         *sendgrid.Client
	fromEmail      string
	fromName       string
	deeplinkScheme string
	baseURL        string
	logger         *slog.Logger
}

type ConsoleSender struct {
	deeplinkScheme string
	baseURL        string
	logger         *slog.Logger
}

// NewSender creates the appropriate email sender based on configuration.
// When apiKey is empty, it falls back to console sender (for development).
// deeplinkScheme should be the app's URL scheme (e.g., "drivebai").
// baseURL is the API base URL used for web fallback links.
func NewSender(apiKey, fromEmail, fromName, deeplinkScheme, baseURL string, logger *slog.Logger) Sender {
	if apiKey == "" {
		logger.Warn("SENDGRID_API_KEY not set, using console sender (emails will be logged to console)")
		return &ConsoleSender{
			deeplinkScheme: deeplinkScheme,
			baseURL:        baseURL,
			logger:         logger,
		}
	}

	// Validate configuration when SendGrid is enabled
	if fromEmail == "" {
		logger.Error("SENDGRID_FROM_EMAIL is required when SENDGRID_API_KEY is set")
		logger.Warn("Falling back to console sender due to missing configuration")
		return &ConsoleSender{
			deeplinkScheme: deeplinkScheme,
			baseURL:        baseURL,
			logger:         logger,
		}
	}

	logger.Info("Using SendGrid for email delivery",
		"from_email", fromEmail,
		"from_name", fromName,
		"deeplink_scheme", deeplinkScheme,
		"base_url", baseURL,
	)
	return &SendGridSender{
		client:         sendgrid.NewSendClient(apiKey),
		fromEmail:      fromEmail,
		fromName:       fromName,
		deeplinkScheme: deeplinkScheme,
		baseURL:        baseURL,
		logger:         logger,
	}
}

// redactToken returns a redacted version of the token for logging in production.
// Shows first 6 characters followed by "..." to avoid exposing full tokens.
func redactToken(token string) string {
	if len(token) <= 6 {
		return "***"
	}
	return token[:6] + "..."
}

// SendGrid implementation

func (s *SendGridSender) SendVerificationEmail(toEmail, toName, code string) error {
	from := mail.NewEmail(s.fromName, s.fromEmail)
	to := mail.NewEmail(toName, toEmail)
	subject := "Verify your DriveBai account"

	plainTextContent := fmt.Sprintf(`Hello %s,

Your verification code is: %s

This code will expire in 10 minutes.

If you didn't create a DriveBai account, you can safely ignore this email.

Best,
The DriveBai Team`, toName, code)

	htmlContent := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .code { font-size: 32px; font-weight: bold; letter-spacing: 8px; color: #4ECDC4; text-align: center; padding: 20px; background: #f5f5f5; border-radius: 8px; margin: 20px 0; }
        .footer { margin-top: 30px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <h2>Verify your email</h2>
        <p>Hello %s,</p>
        <p>Your verification code is:</p>
        <div class="code">%s</div>
        <p>This code will expire in 10 minutes.</p>
        <p>If you didn't create a DriveBai account, you can safely ignore this email.</p>
        <div class="footer">
            <p>Best,<br>The DriveBai Team</p>
        </div>
    </div>
</body>
</html>`, toName, code)

	message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)

	response, err := s.client.Send(message)
	if err != nil {
		s.logger.Error("failed to send verification email",
			"error", err,
			"to", toEmail,
		)
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	if response.StatusCode >= 400 {
		s.logger.Error("SendGrid returned error for verification email",
			"status", response.StatusCode,
			"body", response.Body,
			"to", toEmail,
		)
		return fmt.Errorf("sendgrid error: status %d - %s", response.StatusCode, response.Body)
	}

	s.logger.Info("verification email sent successfully",
		"to", toEmail,
		"status", response.StatusCode,
	)
	return nil
}

func (s *SendGridSender) SendPasswordResetEmail(toEmail, toName, token string) error {
	from := mail.NewEmail(s.fromName, s.fromEmail)
	to := mail.NewEmail(toName, toEmail)
	subject := "Reset your DriveBai password"

	// Build deep link URL using configurable scheme
	resetLink := fmt.Sprintf("%s://reset-password?token=%s", s.deeplinkScheme, token)
	// Web fallback link for browsers/desktop
	webLink := fmt.Sprintf("%s/reset-password?token=%s", s.baseURL, token)

	plainTextContent := fmt.Sprintf(`Hello %s,

You requested to reset your password.

Open in DriveBai app:
%s

Or use this web link:
%s

This link will expire in 1 hour.

If you didn't request a password reset, you can safely ignore this email.

Best,
The DriveBai Team`, toName, resetLink, webLink)

	htmlContent := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .button { display: inline-block; background-color: #4ECDC4; color: white; text-decoration: none; padding: 14px 28px; border-radius: 8px; font-weight: bold; margin: 20px 0; }
        .button:hover { background-color: #45b8b0; }
        .button-secondary { background-color: #6c757d; }
        .button-secondary:hover { background-color: #5a6268; }
        .link { word-break: break-all; color: #4ECDC4; font-size: 12px; }
        .footer { margin-top: 30px; font-size: 12px; color: #666; }
        .divider { margin: 20px 0; text-align: center; color: #999; }
    </style>
</head>
<body>
    <div class="container">
        <h2>Reset your password</h2>
        <p>Hello %s,</p>
        <p>You requested to reset your password. Click the button below to open the DriveBai app:</p>
        <p style="text-align: center;">
            <a href="%s" class="button">Reset Password in App</a>
        </p>
        <p class="divider">— or —</p>
        <p>If the button doesn't work, use this web link:</p>
        <p style="text-align: center;">
            <a href="%s" class="button button-secondary">Reset via Web</a>
        </p>
        <p class="link">%s</p>
        <p>This link will expire in 1 hour.</p>
        <p>If you didn't request a password reset, you can safely ignore this email.</p>
        <div class="footer">
            <p>Best,<br>The DriveBai Team</p>
        </div>
    </div>
</body>
</html>`, toName, resetLink, webLink, webLink)

	message := mail.NewSingleEmail(from, subject, to, plainTextContent, htmlContent)

	response, err := s.client.Send(message)
	if err != nil {
		s.logger.Error("failed to send password reset email",
			"error", err,
			"to", toEmail,
		)
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	if response.StatusCode >= 400 {
		s.logger.Error("SendGrid returned error for password reset email",
			"status", response.StatusCode,
			"body", response.Body,
			"to", toEmail,
		)
		return fmt.Errorf("sendgrid error: status %d - %s", response.StatusCode, response.Body)
	}

	// Log success with redacted token for security
	s.logger.Info("password reset email sent successfully",
		"to", toEmail,
		"status", response.StatusCode,
		"token_prefix", redactToken(token),
	)
	return nil
}

// Console implementation (for development)

func (c *ConsoleSender) SendVerificationEmail(toEmail, toName, code string) error {
	c.logger.Info("VERIFICATION EMAIL (dev mode)",
		"to", toEmail,
		"name", toName,
		"code", code,
	)
	fmt.Printf("\n"+
		"╔════════════════════════════════════════════════════════════╗\n"+
		"║  📧 VERIFICATION EMAIL (SendGrid not configured)           ║\n"+
		"╠════════════════════════════════════════════════════════════╣\n"+
		"║  To: %-52s ║\n"+
		"║  Name: %-50s ║\n"+
		"║  Code: %-50s ║\n"+
		"╚════════════════════════════════════════════════════════════╝\n\n",
		toEmail, toName, code)
	return nil
}

func (c *ConsoleSender) SendPasswordResetEmail(toEmail, toName, token string) error {
	// Build deep link URL using configurable scheme
	resetLink := fmt.Sprintf("%s://reset-password?token=%s", c.deeplinkScheme, token)
	webLink := fmt.Sprintf("%s/reset-password?token=%s", c.baseURL, token)

	c.logger.Info("PASSWORD RESET EMAIL (dev mode)",
		"to", toEmail,
		"name", toName,
		"token", token,
		"app_link", resetLink,
		"web_link", webLink,
	)
	fmt.Printf("\n"+
		"╔══════════════════════════════════════════════════════════════════════════════╗\n"+
		"║  📧 PASSWORD RESET EMAIL (SendGrid not configured)                           ║\n"+
		"╠══════════════════════════════════════════════════════════════════════════════╣\n"+
		"║  To: %-72s ║\n"+
		"║  Name: %-70s ║\n"+
		"║  Token: %-69s ║\n"+
		"║  App Link: %-67s ║\n"+
		"║  Web Link: %-67s ║\n"+
		"╚══════════════════════════════════════════════════════════════════════════════╝\n\n",
		toEmail, toName, token, resetLink, webLink)
	return nil
}
