package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/drivebai/backend/internal/auth"
	"github.com/drivebai/backend/internal/config"
	"github.com/drivebai/backend/internal/email"
	"github.com/drivebai/backend/internal/models"
	"github.com/drivebai/backend/internal/repository"
	"github.com/google/uuid"
)

type AuthHandler struct {
	userRepo  *repository.UserRepository
	tokenRepo *repository.TokenRepository
	jwtSvc    *auth.JWTService
	emailSvc  email.Sender
	cfg       *config.Config
	logger    *slog.Logger
}

func NewAuthHandler(
	userRepo *repository.UserRepository,
	tokenRepo *repository.TokenRepository,
	jwtSvc *auth.JWTService,
	emailSvc email.Sender,
	cfg *config.Config,
	logger *slog.Logger,
) *AuthHandler {
	return &AuthHandler{
		userRepo:  userRepo,
		tokenRepo: tokenRepo,
		jwtSvc:    jwtSvc,
		emailSvc:  emailSvc,
		cfg:       cfg,
		logger:    logger,
	}
}

// Request/Response types

type RegisterRequest struct {
	Email     string      `json:"email"`
	Password  string      `json:"password"`
	FirstName string      `json:"first_name"`
	LastName  string      `json:"last_name"`
	Phone     string      `json:"phone,omitempty"`
	Role      models.Role `json:"role"`
}

type RegisterResponse struct {
	AccessToken  string             `json:"access_token"`
	RefreshToken string             `json:"refresh_token"`
	ExpiresAt    models.RFC3339Time `json:"expires_at"`
	User         UserProfile        `json:"user"`
}

type VerifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken  string             `json:"access_token"`
	RefreshToken string             `json:"refresh_token"`
	ExpiresAt    models.RFC3339Time `json:"expires_at"`
	User         UserProfile        `json:"user"`
}

type UserProfile struct {
	ID               uuid.UUID              `json:"id"`
	Email            string                 `json:"email"`
	Role             models.Role            `json:"role"`
	FirstName        string                 `json:"first_name"`
	LastName         string                 `json:"last_name"`
	Phone            *string                `json:"phone,omitempty"`
	IsEmailVerified  bool                   `json:"is_email_verified"`
	OnboardingStatus models.OnboardingStatus `json:"onboarding_status"`
	ProfilePhotoURL  *string                `json:"profile_photo_url,omitempty"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type ResendOTPRequest struct {
	Email   string `json:"email"`
	Purpose string `json:"purpose"` // "verify_email" or "reset_password"
}

// Handlers

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	// Validate input
	if err := h.validateRegisterRequest(&req); err != nil {
		WriteError(w, http.StatusBadRequest, err)
		return
	}

	ctx := r.Context()

	// Check if email exists
	exists, err := h.userRepo.EmailExists(ctx, req.Email)
	if err != nil {
		h.logger.Error("failed to check email", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}
	if exists {
		WriteError(w, http.StatusConflict, models.ErrEmailTaken)
		return
	}

	// Hash password
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("failed to hash password", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Create user - no email verification required
	var phone *string
	if req.Phone != "" {
		phone = &req.Phone
	}

	// Set initial onboarding status based on whether role was provided
	onboardingStatus := models.OnboardingCreated
	if req.Role.IsValid() && req.Role != "" {
		onboardingStatus = models.OnboardingRoleSelected
	}

	user := &models.User{
		Email:            strings.ToLower(req.Email),
		PasswordHash:     &passwordHash,
		Role:             req.Role,
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		Phone:            phone,
		IsEmailVerified:  true, // No email verification required
		OnboardingStatus: onboardingStatus,
	}

	if err := h.userRepo.Create(ctx, user); err != nil {
		if apiErr := models.GetAPIError(err); apiErr != nil {
			WriteError(w, http.StatusConflict, apiErr)
		} else {
			h.logger.Error("failed to create user", "error", err)
			WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	// Generate tokens immediately - user is active
	tokens, err := h.generateTokens(ctx, user)
	if err != nil {
		h.logger.Error("failed to generate tokens", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusCreated, RegisterResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    models.NewRFC3339Time(tokens.ExpiresAt),
		User:         h.toUserProfile(user),
	})
}

func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	// Email verification via OTP is no longer required
	// Users are verified immediately upon registration
	WriteJSON(w, http.StatusOK, map[string]string{"message": "Email verified successfully"})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if req.Email == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Email and password are required"))
		return
	}

	ctx := r.Context()

	// Find user
	user, err := h.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if models.IsAPIError(err) {
			WriteError(w, http.StatusUnauthorized, models.ErrInvalidCredentials)
		} else {
			h.logger.Error("failed to get user", "error", err)
			WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		}
		return
	}

	// Check password
	if user.PasswordHash == nil || !auth.CheckPassword(req.Password, *user.PasswordHash) {
		WriteError(w, http.StatusUnauthorized, models.ErrInvalidCredentials)
		return
	}

	// Generate tokens - no email verification check needed
	tokens, err := h.generateTokens(ctx, user)
	if err != nil {
		h.logger.Error("failed to generate tokens", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusOK, LoginResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    models.NewRFC3339Time(tokens.ExpiresAt),
		User:         h.toUserProfile(user),
	})
}

func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if req.RefreshToken == "" {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Refresh token is required"))
		return
	}

	ctx := r.Context()

	// Find token
	tokenHash := h.jwtSvc.HashRefreshToken(req.RefreshToken)
	storedToken, err := h.tokenRepo.GetByHash(ctx, tokenHash)
	if err != nil {
		h.logger.Error("failed to get refresh token", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	if storedToken == nil {
		WriteError(w, http.StatusUnauthorized, models.ErrTokenInvalid)
		return
	}

	if storedToken.IsRevoked() {
		WriteError(w, http.StatusUnauthorized, models.ErrTokenInvalid)
		return
	}

	if storedToken.IsExpired() {
		WriteError(w, http.StatusUnauthorized, models.ErrTokenExpired)
		return
	}

	// Revoke old token (rotation)
	if err := h.tokenRepo.RevokeToken(ctx, storedToken.ID); err != nil {
		h.logger.Error("failed to revoke old token", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Get user
	user, err := h.userRepo.GetByID(ctx, storedToken.UserID)
	if err != nil {
		h.logger.Error("failed to get user", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Generate new tokens
	tokens, err := h.generateTokens(ctx, user)
	if err != nil {
		h.logger.Error("failed to generate tokens", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	WriteJSON(w, http.StatusOK, LoginResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    models.NewRFC3339Time(tokens.ExpiresAt),
		User:         h.toUserProfile(user),
	})
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if req.Email == "" {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Email is required"))
		return
	}

	ctx := r.Context()

	// Find user (don't reveal if user exists)
	user, err := h.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		// Always return success to prevent email enumeration
		WriteJSON(w, http.StatusAccepted, map[string]string{
			"message": "If an account exists with this email, you will receive a password reset link.",
		})
		return
	}

	// Invalidate any existing reset tokens
	h.tokenRepo.InvalidatePasswordResetTokensForUser(ctx, user.ID)

	// Generate password reset token (1 hour expiry)
	rawToken, hashedToken, expiresAt, err := auth.GeneratePasswordResetToken(time.Hour)
	if err != nil {
		h.logger.Error("failed to generate reset token", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Store token
	resetToken := &models.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: hashedToken,
		ExpiresAt: expiresAt,
	}
	if err := h.tokenRepo.CreatePasswordResetToken(ctx, resetToken); err != nil {
		h.logger.Error("failed to store reset token", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Send email with reset link
	if err := h.emailSvc.SendPasswordResetEmail(user.Email, user.FullName(), rawToken); err != nil {
		h.logger.Error("failed to send reset email", "error", err)
	}

	WriteJSON(w, http.StatusAccepted, map[string]string{
		"message": "If an account exists with this email, you will receive a password reset link.",
	})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if req.Token == "" || req.NewPassword == "" {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Token and new password are required"))
		return
	}

	if len(req.NewPassword) < 8 {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Password must be at least 8 characters"))
		return
	}

	ctx := r.Context()

	// Find token
	tokenHash := auth.HashPasswordResetToken(req.Token)
	resetToken, err := h.tokenRepo.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err != nil {
		h.logger.Error("failed to get reset token", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	if resetToken == nil {
		WriteError(w, http.StatusBadRequest, models.ErrTokenInvalid)
		return
	}

	if resetToken.IsExpired() {
		WriteError(w, http.StatusBadRequest, models.ErrTokenExpired)
		return
	}

	if resetToken.IsUsed() {
		WriteError(w, http.StatusBadRequest, models.ErrTokenInvalid)
		return
	}

	// Mark token as used
	if err := h.tokenRepo.MarkPasswordResetTokenUsed(ctx, resetToken.ID); err != nil {
		h.logger.Error("failed to mark token used", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Hash new password
	passwordHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		h.logger.Error("failed to hash password", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Update password
	if err := h.userRepo.UpdatePassword(ctx, resetToken.UserID, passwordHash); err != nil {
		h.logger.Error("failed to update password", "error", err)
		WriteError(w, http.StatusInternalServerError, models.ErrInternalError)
		return
	}

	// Revoke all refresh tokens for security
	if err := h.tokenRepo.RevokeAllForUser(ctx, resetToken.UserID); err != nil {
		h.logger.Error("failed to revoke tokens", "error", err)
	}

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Password updated successfully"})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, http.StatusBadRequest, models.NewValidationError("Invalid request body"))
		return
	}

	if req.RefreshToken == "" {
		WriteJSON(w, http.StatusOK, map[string]string{"message": "Logged out"})
		return
	}

	ctx := r.Context()

	// Revoke the refresh token
	tokenHash := h.jwtSvc.HashRefreshToken(req.RefreshToken)
	storedToken, err := h.tokenRepo.GetByHash(ctx, tokenHash)
	if err != nil {
		h.logger.Error("failed to get refresh token", "error", err)
	}

	if storedToken != nil {
		if err := h.tokenRepo.RevokeToken(ctx, storedToken.ID); err != nil {
			h.logger.Error("failed to revoke token", "error", err)
		}
	}

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Logged out"})
}

func (h *AuthHandler) ResendOTP(w http.ResponseWriter, r *http.Request) {
	// OTP is no longer used - return success for backwards compatibility
	WriteJSON(w, http.StatusAccepted, map[string]string{
		"message": "If an account exists with this email, a new code will be sent.",
	})
}

// Helper methods

func (h *AuthHandler) validateRegisterRequest(req *RegisterRequest) *models.APIError {
	if req.Email == "" {
		return models.NewValidationError("Email is required")
	}
	if !strings.Contains(req.Email, "@") {
		return models.NewValidationError("Invalid email format")
	}
	if req.Password == "" {
		return models.NewValidationError("Password is required")
	}
	if len(req.Password) < 8 {
		return models.NewValidationError("Password must be at least 8 characters")
	}
	if req.FirstName == "" {
		return models.NewValidationError("First name is required")
	}
	if req.LastName == "" {
		return models.NewValidationError("Last name is required")
	}
	if !req.Role.IsValid() || req.Role == models.RoleAdmin {
		return models.NewValidationError("Role must be 'driver' or 'car_owner'")
	}
	return nil
}

func (h *AuthHandler) generateTokens(ctx context.Context, user *models.User) (*auth.TokenPair, error) {
	// Generate access token
	accessToken, expiresAt, err := h.jwtSvc.GenerateAccessToken(user)
	if err != nil {
		return nil, err
	}

	// Generate refresh token
	refreshToken, refreshHash, refreshExpires, err := h.jwtSvc.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	// Store refresh token
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

func (h *AuthHandler) toUserProfile(user *models.User) UserProfile {
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
