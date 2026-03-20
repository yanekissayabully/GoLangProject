package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/drivebai/backend/internal/auth"
	"github.com/drivebai/backend/internal/email"
	"github.com/drivebai/backend/internal/models"
	"github.com/drivebai/backend/internal/repository"
)

// OTPAuthHandler handles passwordless email-OTP login and registration-completion flows.
type OTPAuthHandler struct {
	userRepo   *repository.UserRepository
	tokenRepo  *repository.TokenRepository
	otpRepo    *repository.LoginOTPRepository
	jwtSvc     *auth.JWTService
	otpSender  email.OTPSender
	logger     *slog.Logger
}

func NewOTPAuthHandler(
	userRepo *repository.UserRepository,
	tokenRepo *repository.TokenRepository,
	otpRepo *repository.LoginOTPRepository,
	jwtSvc *auth.JWTService,
	otpSender email.OTPSender,
	logger *slog.Logger,
) *OTPAuthHandler {
	return &OTPAuthHandler{
		userRepo:  userRepo,
		tokenRepo: tokenRepo,
		otpRepo:   otpRepo,
		jwtSvc:    jwtSvc,
		otpSender: otpSender,
		logger:    logger,
	}
}

// ─── Rate-limit constants ───────────────────────────────────────────────────

// maxOTPsPerEmail is the maximum number of OTPs that may be issued to a single
// email address within otpRateLimitWindow.
const maxOTPsPerEmail = 5

// maxIPOTPs is the maximum number of OTPs that may be issued from a single
// IP address within otpRateLimitWindow.
const maxIPOTPs = 10

// otpRateLimitWindow is the sliding window used for rate limiting.
const otpRateLimitWindow = 15 * time.Minute

// otpExpiry is how long a freshly issued OTP remains valid.
const otpExpiry = 10 * time.Minute

// ─── Request / Response types ───────────────────────────────────────────────

type OTPRequestLoginRequest struct {
	Email string `json:"email"`
}

type OTPVerifyLoginRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// OTPVerifyLoginResponse is returned when OTP verification succeeds and the
// email belongs to an existing user.  The `kind` discriminator lets the iOS
// client branch without inspecting field presence.
type OTPVerifyLoginResponse struct {
	Kind         string             `json:"kind"` // "login"
	AccessToken  string             `json:"access_token"`
	RefreshToken string             `json:"refresh_token"`
	ExpiresAt    models.RFC3339Time `json:"expires_at"`
	User         UserProfile        `json:"user"`
}

// OTPVerifyRegisterResponse is returned when the email is NOT in the users
// table.  The client should use registration_token + email to complete sign-up.
type OTPVerifyRegisterResponse struct {
	Kind              string `json:"kind"` // "register"
	RegistrationToken string `json:"registration_token"`
	Email             string `json:"email"`
}

// CompleteRegistrationRequest finishes account creation using a registration token
// that was obtained from OTPVerifyLogin.
type CompleteRegistrationRequest struct {
	RegistrationToken string      `json:"registration_token"`
	FirstName         string      `json:"first_name"`
	LastName          string      `json:"last_name"`
	Password          string      `json:"password"`
	Phone             string      `json:"phone,omitempty"`
	Role              models.Role `json:"role"`
}

// ─── Handlers ───────────────────────────────────────────────────────────────

// RequestOTP handles POST /auth/otp/request.
// Always returns 200 with a generic message — never reveals whether the email exists.
func (h *OTPAuthHandler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var req OTPRequestLoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || !strings.Contains(req.Email, "@") {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("A valid email address is required"))
		return
	}

	ctx := r.Context()

	// ── Rate limiting ──────────────────────────────────────────────────────

	// Per-email rate limit (reuse existing otp_rate_limits table)
	since := time.Now().Add(-otpRateLimitWindow)
	emailCount, err := h.userRepo.GetOTPSendCount(ctx, req.Email, since)
	if err != nil {
		h.logger.Error("otp/request: failed to get email rate limit count", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	if emailCount >= maxOTPsPerEmail {
		WriteError(w, http.StatusTooManyRequests, models.ErrRateLimited)
		return
	}

	// Per-IP rate limit (also via otp_rate_limits table)
	ip := realIP(r)
	ipCount, err := h.userRepo.GetOTPSendCount(ctx, ip, since)
	if err != nil {
		h.logger.Error("otp/request: failed to get IP rate limit count", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	if ipCount >= maxIPOTPs {
		WriteError(w, http.StatusTooManyRequests, models.ErrRateLimited)
		return
	}

	// ── Generate OTP ───────────────────────────────────────────────────────

	rawCode, codeHash, err := auth.GenerateOTP()
	if err != nil {
		h.logger.Error("otp/request: failed to generate OTP", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	expiresAt := time.Now().Add(otpExpiry)
	userAgent := r.UserAgent()

	if _, err := h.otpRepo.Create(ctx, req.Email, codeHash, expiresAt, ip, userAgent); err != nil {
		h.logger.Error("otp/request: failed to store OTP", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Record the send in the rate-limit table (email row + IP row)
	_ = h.userRepo.RecordOTPSend(ctx, req.Email, ip)
	_ = h.userRepo.RecordOTPSend(ctx, ip, ip)

	// ── Send email ─────────────────────────────────────────────────────────

	// Fire and forget: email failures must NOT reveal user enumeration info
	// via a different response shape. We log the error and return success.
	if err := h.otpSender.SendLoginOTP(req.Email, rawCode); err != nil {
		h.logger.Error("otp/request: failed to send OTP email", "error", err, "email", req.Email)
	}

	// Always respond generically
	WriteJSON(w, http.StatusOK, map[string]string{
		"message": "If this email is valid, a login code has been sent.",
	})
}

// VerifyOTP handles POST /auth/otp/verify.
// On success:
//   - existing user  → returns AuthTokens (kind=login)
//   - new user       → returns a short-lived registration_token (kind=register)
func (h *OTPAuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req OTPVerifyLoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	if req.Email == "" || req.Code == "" {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Email and code are required"))
		return
	}
	if !auth.ValidateOTPFormat(req.Code) {
		WriteError(w, http.StatusBadRequest, models.ErrOTPInvalid)
		return
	}

	ctx := r.Context()

	// ── Fetch latest unconsumed OTP ────────────────────────────────────────

	otp, err := h.otpRepo.GetLatestUnconsumed(ctx, req.Email)
	if err != nil {
		h.logger.Error("otp/verify: failed to fetch OTP", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Respond generically on missing/expired/locked OTP so we don't expose
	// details about the OTP lifecycle to an attacker.
	if otp == nil || otp.IsExpired() || otp.IsConsumed() {
		WriteError(w, http.StatusUnauthorized, models.ErrOTPExpired)
		return
	}

	if otp.IsLocked() {
		WriteError(w, http.StatusUnauthorized, models.ErrOTPAttemptsExceeded)
		return
	}

	// ── Compare hash ───────────────────────────────────────────────────────

	expectedHash := auth.HashOTP(req.Code)
	if expectedHash != otp.CodeHash {
		// Increment attempts; lock if ceiling reached
		newAttempts, incErr := h.otpRepo.IncrementAttempts(ctx, otp.ID)
		if incErr != nil {
			h.logger.Error("otp/verify: failed to increment attempts", "error", incErr)
		}
		if newAttempts >= models.LoginOTPMaxAttempts {
			WriteError(w, http.StatusUnauthorized, models.ErrOTPAttemptsExceeded)
			return
		}
		WriteError(w, http.StatusUnauthorized, models.ErrOTPInvalid)
		return
	}

	// ── OTP is valid — consume it ──────────────────────────────────────────

	if err := h.otpRepo.MarkConsumed(ctx, otp.ID); err != nil {
		h.logger.Error("otp/verify: failed to mark OTP consumed", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// ── Check whether user exists ──────────────────────────────────────────

	user, err := h.userRepo.GetByEmail(ctx, req.Email)
	if err != nil && !models.IsAPIError(err) {
		h.logger.Error("otp/verify: failed to look up user", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	if user != nil {
		// ── Existing user: issue full auth tokens ──────────────────────────
		tokens, err := h.generateTokens(ctx, user)
		if err != nil {
			h.logger.Error("otp/verify: failed to generate tokens", "error", err)
			WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
			return
		}

		WriteJSON(w, http.StatusOK, OTPVerifyLoginResponse{
			Kind:         "login",
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
			ExpiresAt:    models.NewRFC3339Time(tokens.ExpiresAt),
			User:         toUserProfile(user),
		})
		return
	}

	// ── New user: issue a short-lived registration token ───────────────────
	regToken, err := h.jwtSvc.GenerateRegistrationToken(req.Email)
	if err != nil {
		h.logger.Error("otp/verify: failed to generate registration token", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusOK, OTPVerifyRegisterResponse{
		Kind:              "register",
		RegistrationToken: regToken,
		Email:             req.Email,
	})
}

// CompleteRegistration handles POST /auth/otp/complete-registration.
// Creates a new account using a registration token obtained from VerifyOTP.
func (h *OTPAuthHandler) CompleteRegistration(w http.ResponseWriter, r *http.Request) {
	var req CompleteRegistrationRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	// ── Validate registration token ────────────────────────────────────────

	if req.RegistrationToken == "" {
		WriteError(w, http.StatusBadRequest, models.ErrRegistrationTokenRequired)
		return
	}

	verifiedEmail, err := h.jwtSvc.ValidateRegistrationToken(req.RegistrationToken)
	if err != nil {
		if models.IsAPIError(err) {
			WriteError(w, http.StatusUnauthorized, models.GetAPIError(err))
		} else {
			WriteError(w, http.StatusUnauthorized, models.ErrTokenInvalid)
		}
		return
	}

	// ── Validate other fields ──────────────────────────────────────────────

	if err := validateCompleteRegistration(&req); err != nil {
		WriteError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()

	// Guard against double-registration (e.g. concurrent requests)
	exists, err := h.userRepo.EmailExists(ctx, verifiedEmail)
	if err != nil {
		h.logger.Error("otp/complete-registration: failed to check email", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	if exists {
		WriteError(w, http.StatusConflict, models.ErrEmailTaken)
		return
	}

	// ── Hash password ──────────────────────────────────────────────────────

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("otp/complete-registration: failed to hash password", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// ── Create user ────────────────────────────────────────────────────────

	var phone *string
	if req.Phone != "" {
		phone = &req.Phone
	}

	onboardingStatus := models.OnboardingCreated
	if req.Role.IsValid() && req.Role != "" {
		onboardingStatus = models.OnboardingRoleSelected
	}

	user := &models.User{
		Email:            verifiedEmail, // from the validated token, not the request body
		PasswordHash:     &passwordHash,
		Role:             req.Role,
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		Phone:            phone,
		IsEmailVerified:  true, // OTP already proved ownership
		OnboardingStatus: onboardingStatus,
	}

	if err := h.userRepo.Create(ctx, user); err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			WriteError(w, http.StatusConflict, apiErr)
		} else {
			h.logger.Error("otp/complete-registration: failed to create user", "error", err)
			WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	// ── Issue tokens ───────────────────────────────────────────────────────

	tokens, err := h.generateTokens(ctx, user)
	if err != nil {
		h.logger.Error("otp/complete-registration: failed to generate tokens", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusCreated, RegisterResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    models.NewRFC3339Time(tokens.ExpiresAt),
		User:         toUserProfile(user),
	})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// generateTokens issues a new access + refresh token pair for the given user.
func (h *OTPAuthHandler) generateTokens(ctx context.Context, user *models.User) (*auth.TokenPair, error) {
	accessToken, expiresAt, err := h.jwtSvc.GenerateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, refreshHash, refreshExpires, err := h.jwtSvc.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	storedToken := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: refreshHash,
		ExpiresAt: refreshExpires,
	}
	if err := h.tokenRepo.CreateRefreshToken(ctx, storedToken); err != nil {
		return nil, err
	}

	return &auth.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

// toUserProfile converts a models.User to the UserProfile response type.
// Defined here as a package-level function so OTPAuthHandler can use it
// without depending on AuthHandler.
func toUserProfile(user *models.User) UserProfile {
	return UserProfile{
		ID:               user.ID,
		Email:            user.Email,
		Role:             user.Role,
		FirstName:        user.FirstName,
		LastName:         user.LastName,
		Phone:            user.Phone,
		IsEmailVerified:  user.IsEmailVerified,
		OnboardingStatus: user.OnboardingStatus,
		ProfilePhotoURL:  user.ProfilePhotoURL,
	}
}

// realIP returns the best-effort client IP from the request.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// X-Forwarded-For may contain multiple IPs; first is the client
		if idx := strings.Index(ip, ","); idx != -1 {
			return strings.TrimSpace(ip[:idx])
		}
		return strings.TrimSpace(ip)
	}
	// Fall back to RemoteAddr (strip port)
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

func validateCompleteRegistration(req *CompleteRegistrationRequest) *models.APIError {
	if req.FirstName == "" {
		return models.NewValidationError("First name is required")
	}
	if req.LastName == "" {
		return models.NewValidationError("Last name is required")
	}
	if req.Password == "" {
		return models.NewValidationError("Password is required")
	}
	if len(req.Password) < 8 {
		return models.NewValidationError("Password must be at least 8 characters")
	}
	if !req.Role.IsValid() || req.Role == models.RoleAdmin {
		return models.NewValidationError("Role must be 'driver' or 'car_owner'")
	}
	return nil
}
