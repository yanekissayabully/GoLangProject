package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/drivebai/backend/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type JWTService struct {
	secret           []byte
	accessTokenTTL   time.Duration
	refreshTokenTTL  time.Duration
}

type AccessTokenClaims struct {
	UserID uuid.UUID   `json:"user_id"`
	Email  string      `json:"email"`
	Role   models.Role `json:"role"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService {
	return &JWTService{
		secret:          []byte(secret),
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}
}

func (s *JWTService) GenerateAccessToken(user *models.User) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.accessTokenTTL)

	claims := AccessTokenClaims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "drivebai",
			Subject:   user.ID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

func (s *JWTService) GenerateRefreshToken() (string, string, time.Time, error) {
	// Generate a random token
	tokenBytes := make([]byte, 32)
	tokenID := uuid.New()
	copy(tokenBytes, tokenID[:])

	// Create a unique token string
	rawToken := uuid.New().String() + "-" + uuid.New().String()

	// Hash the token for storage
	hash := sha256.Sum256([]byte(rawToken))
	hashedToken := hex.EncodeToString(hash[:])

	expiresAt := time.Now().Add(s.refreshTokenTTL)

	return rawToken, hashedToken, expiresAt, nil
}

func (s *JWTService) ValidateAccessToken(tokenString string) (*AccessTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AccessTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, models.ErrTokenExpired
		}
		return nil, models.ErrTokenInvalid
	}

	claims, ok := token.Claims.(*AccessTokenClaims)
	if !ok || !token.Valid {
		return nil, models.ErrTokenInvalid
	}

	return claims, nil
}

func (s *JWTService) HashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *JWTService) GetRefreshTokenTTL() time.Duration {
	return s.refreshTokenTTL
}

// GeneratePasswordResetToken creates a secure token for password reset
// Returns: raw token (to send to user), hashed token (to store), expiry time
func GeneratePasswordResetToken(ttl time.Duration) (string, string, time.Time, error) {
	// Create a unique token string
	rawToken := uuid.New().String() + "-" + uuid.New().String()

	// Hash the token for storage
	hash := sha256.Sum256([]byte(rawToken))
	hashedToken := hex.EncodeToString(hash[:])

	expiresAt := time.Now().Add(ttl)

	return rawToken, hashedToken, expiresAt, nil
}

// HashPasswordResetToken hashes a password reset token for lookup
func HashPasswordResetToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// RegistrationClaims are embedded in a short-lived registration token.
// It proves that the given email address was OTP-verified and allows the
// iOS client to complete account creation without re-verifying.
type RegistrationClaims struct {
	Email   string `json:"email"`
	Purpose string `json:"purpose"` // always "otp_registration"
	jwt.RegisteredClaims
}

const registrationTokenTTL = 15 * time.Minute
const registrationPurpose = "otp_registration"

// GenerateRegistrationToken issues a short-lived JWT proving email OTP was verified.
func (s *JWTService) GenerateRegistrationToken(email string) (string, error) {
	expiresAt := time.Now().Add(registrationTokenTTL)

	claims := RegistrationClaims{
		Email:   email,
		Purpose: registrationPurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "drivebai",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateRegistrationToken validates a registration token and returns the verified email.
func (s *JWTService) ValidateRegistrationToken(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &RegistrationClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", models.ErrTokenExpired
		}
		return "", models.ErrTokenInvalid
	}

	claims, ok := token.Claims.(*RegistrationClaims)
	if !ok || !token.Valid || claims.Purpose != registrationPurpose {
		return "", models.ErrTokenInvalid
	}

	return claims.Email, nil
}
