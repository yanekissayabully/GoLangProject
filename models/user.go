package models

import (
	"time"

	"github.com/google/uuid"
)

// RFC3339Time marshals to RFC3339 without nanoseconds (iOS compatibility).
type RFC3339Time time.Time

func (t RFC3339Time) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).UTC().Format(time.RFC3339) + `"`), nil
}

func NewRFC3339Time(t time.Time) RFC3339Time { return RFC3339Time(t) }

// Roles
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

// Onboarding
type OnboardingStatus string

const (
	OnboardingCreated      OnboardingStatus = "created"
	OnboardingRoleSelected OnboardingStatus = "role_selected"
)

// User is the core user model.
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

func (u *User) FullName() string { return u.FirstName + " " + u.LastName }

// RefreshToken stored in DB.
type RefreshToken struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (r *RefreshToken) IsExpired() bool { return time.Now().After(r.ExpiresAt) }
func (r *RefreshToken) IsRevoked() bool { return r.RevokedAt != nil }

// LoginOTP for passwordless email login.
const LoginOTPMaxAttempts = 5

type LoginOTP struct {
	ID         uuid.UUID  `json:"id"`
	Email      string     `json:"email"`
	CodeHash   string     `json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	Attempts   int        `json:"attempts"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	IPAddress  *string    `json:"-"`
	UserAgent  *string    `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (o *LoginOTP) IsExpired()  bool { return time.Now().After(o.ExpiresAt) }
func (o *LoginOTP) IsConsumed() bool { return o.ConsumedAt != nil }
func (o *LoginOTP) IsLocked()   bool { return o.Attempts >= LoginOTPMaxAttempts }
