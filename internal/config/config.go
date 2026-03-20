package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	Env         string
	DatabaseURL string

	JWTSecret          string
	JWTAccessTokenTTL  time.Duration
	JWTRefreshTokenTTL time.Duration

	// Email configuration (SendGrid - existing flows)
	SendGridAPIKey    string
	SendGridFromEmail string
	SendGridFromName  string

	// MailerSend configuration (OTP login emails)
	MailerSendAPIKey string
	MailerFromEmail  string
	MailerFromName   string

	// App configuration
	AppDeeplinkScheme string
	AppBaseURL        string

	UploadDir string

	// Stripe configuration
	StripeSecretKey      string
	StripePublishableKey string
	StripeWebhookSecret  string
	PlatformFeeBPS       int // basis points, e.g. 500 = 5%
}

func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		Env:         getEnv("ENV", "development"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://drivebai:drivebai_secret@localhost:5432/drivebai?sslmode=disable"),

		JWTSecret:          getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTAccessTokenTTL:  getDuration("JWT_ACCESS_TOKEN_TTL", 15*time.Minute),
		JWTRefreshTokenTTL: getDuration("JWT_REFRESH_TOKEN_TTL", 30*24*time.Hour),

		SendGridAPIKey:    getEnv("SENDGRID_API_KEY", ""),
		SendGridFromEmail: getEnv("SENDGRID_FROM_EMAIL", "noreply@drivebai.com"),
		SendGridFromName:  getEnv("SENDGRID_FROM_NAME", "DriveBai"),

		MailerSendAPIKey: getEnv("MAILERSEND_API_KEY", ""),
		MailerFromEmail:  getEnv("MAIL_FROM_EMAIL", "noreply@drivebai.com"),
		MailerFromName:   getEnv("MAIL_FROM_NAME", "DrivaBai"),

		AppDeeplinkScheme: getEnv("APP_DEEPLINK_SCHEME", "drivebai"),
		AppBaseURL:        getEnv("APP_BASE_URL", "http://localhost:8080"),

		UploadDir: getEnv("UPLOAD_DIR", "./uploads"),

		StripeSecretKey:      getEnv("STRIPE_SECRET_KEY", ""),
		StripePublishableKey: getEnv("STRIPE_PUBLISHABLE_KEY", ""),
		StripeWebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
		PlatformFeeBPS:       getIntEnv("PLATFORM_FEE_BPS", 500), // default 5%
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}
