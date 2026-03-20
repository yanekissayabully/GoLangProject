package models

import (
	"time"

	"github.com/google/uuid"
)

// RFC3339Time is a time.Time that marshals to RFC3339 format without nanoseconds
// This ensures iOS can parse the date correctly
type RFC3339Time time.Time

func (t RFC3339Time) MarshalJSON() ([]byte, error) {
	// Format as RFC3339 without fractional seconds for maximum compatibility
	return []byte(`"` + time.Time(t).UTC().Format(time.RFC3339) + `"`), nil
}

func (t *RFC3339Time) UnmarshalJSON(data []byte) error {
	// Remove quotes
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try with nanoseconds
		parsed, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return err
		}
	}
	*t = RFC3339Time(parsed)
	return nil
}

func (t RFC3339Time) Time() time.Time {
	return time.Time(t)
}

func NewRFC3339Time(t time.Time) RFC3339Time {
	return RFC3339Time(t)
}

type Role string

const (
	RoleDriver   Role = "driver"
	RoleCarOwner Role = "car_owner"
	RoleAdmin    Role = "admin"
)

func (r Role) IsValid() bool {
	switch r {
	case RoleDriver, RoleCarOwner, RoleAdmin:
		return true
	}
	return false
}

type OnboardingStatus string

const (
	OnboardingCreated           OnboardingStatus = "created"
	OnboardingRoleSelected      OnboardingStatus = "role_selected"
	OnboardingPhotoUploaded     OnboardingStatus = "photo_uploaded"
	OnboardingDocumentsUploaded OnboardingStatus = "documents_uploaded"
	OnboardingComplete          OnboardingStatus = "complete"
)

func (s OnboardingStatus) IsValid() bool {
	switch s {
	case OnboardingCreated, OnboardingRoleSelected, OnboardingPhotoUploaded, OnboardingDocumentsUploaded, OnboardingComplete:
		return true
	}
	return false
}

type User struct {
	ID               uuid.UUID        `json:"id"`
	Email            string           `json:"email"`
	PasswordHash     *string          `json:"-"`
	Role             Role             `json:"role"`
	FirstName        string           `json:"first_name"`
	LastName         string           `json:"last_name"`
	Phone            *string          `json:"phone,omitempty"`
	IsEmailVerified  bool             `json:"is_email_verified"`
	OnboardingStatus OnboardingStatus `json:"onboarding_status"`
	ProfilePhotoURL  *string          `json:"profile_photo_url,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

func (u *User) FullName() string {
	return u.FirstName + " " + u.LastName
}

// IsOnboardingComplete checks if user has completed onboarding
func (u *User) IsOnboardingComplete() bool {
	return u.OnboardingStatus == OnboardingComplete
}

// NextOnboardingStep returns what the user needs to do next
func (u *User) NextOnboardingStep() string {
	switch u.OnboardingStatus {
	case OnboardingCreated:
		return "select_role"
	case OnboardingRoleSelected:
		return "upload_photo"
	case OnboardingPhotoUploaded:
		if u.Role == RoleDriver {
			return "upload_documents"
		}
		return "complete"
	case OnboardingDocumentsUploaded:
		return "complete"
	default:
		return "done"
	}
}

// Document types
type DocumentType string

const (
	DocumentDriversLicense DocumentType = "drivers_license"
	DocumentRegistration   DocumentType = "registration"
)

func (d DocumentType) IsValid() bool {
	switch d {
	case DocumentDriversLicense, DocumentRegistration:
		return true
	}
	return false
}

type DocumentStatus string

const (
	DocumentStatusUploaded DocumentStatus = "uploaded"
	DocumentStatusVerified DocumentStatus = "verified"
	DocumentStatusRejected DocumentStatus = "rejected"
)

type Document struct {
	ID        uuid.UUID      `json:"id"`
	UserID    uuid.UUID      `json:"user_id"`
	Type      DocumentType   `json:"type"`
	FileName  string         `json:"file_name"`
	FilePath  string         `json:"-"` // Don't expose path
	FileSize  int64          `json:"file_size"`
	MimeType  string         `json:"mime_type,omitempty"`
	Status    DocumentStatus `json:"status"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Password reset token (replaces OTP for password reset)
type PasswordResetToken struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (t *PasswordResetToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

func (t *PasswordResetToken) IsUsed() bool {
	return t.UsedAt != nil
}

// Keep these for backwards compatibility during migration
type OTPPurpose string

const (
	OTPPurposeVerifyEmail   OTPPurpose = "verify_email"
	OTPPurposeResetPassword OTPPurpose = "reset_password"
)

type EmailOTP struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Purpose    OTPPurpose `json:"purpose"`
	CodeHash   string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
}

func (o *EmailOTP) IsExpired() bool {
	return time.Now().After(o.ExpiresAt)
}

func (o *EmailOTP) IsConsumed() bool {
	return o.ConsumedAt != nil
}

type RefreshToken struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (r *RefreshToken) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

func (r *RefreshToken) IsRevoked() bool {
	return r.RevokedAt != nil
}

// LoginOTP represents a one-time password record for email-based passwordless login.
// Note: not linked to a user — the email may or may not exist in the users table.
const LoginOTPMaxAttempts = 5

type LoginOTP struct {
	ID         uuid.UUID  `json:"id"`
	Email      string     `json:"email"`
	CodeHash   string     `json:"-"` // SHA-256 of the raw code; never exposed
	ExpiresAt  time.Time  `json:"expires_at"`
	Attempts   int        `json:"attempts"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	IPAddress  *string    `json:"-"`
	UserAgent  *string    `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (o *LoginOTP) IsExpired() bool {
	return time.Now().After(o.ExpiresAt)
}

func (o *LoginOTP) IsConsumed() bool {
	return o.ConsumedAt != nil
}

// IsLocked returns true when the OTP has hit the max attempt ceiling.
func (o *LoginOTP) IsLocked() bool {
	return o.Attempts >= LoginOTPMaxAttempts
}
