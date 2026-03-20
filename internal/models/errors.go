package models

import (
	"errors"
	"fmt"
)

// Error codes for API responses
const (
	ErrCodeEmailTaken            = "EMAIL_TAKEN"
	ErrCodeInvalidCredentials    = "INVALID_CREDENTIALS"
	ErrCodeOTPInvalid            = "OTP_INVALID"
	ErrCodeOTPExpired            = "OTP_EXPIRED"
	ErrCodeOTPAttemptsExceeded   = "OTP_ATTEMPTS_EXCEEDED"
	ErrCodeEmailNotVerified      = "EMAIL_NOT_VERIFIED"
	ErrCodeRateLimited           = "RATE_LIMITED"
	ErrCodeUserNotFound          = "USER_NOT_FOUND"
	ErrCodeInvalidRole           = "INVALID_ROLE"
	ErrCodeInvalidInput          = "INVALID_INPUT"
	ErrCodeUnauthorized          = "UNAUTHORIZED"
	ErrCodeTokenExpired          = "TOKEN_EXPIRED"
	ErrCodeTokenInvalid          = "TOKEN_INVALID"
	ErrCodeInternalError         = "INTERNAL_ERROR"
	ErrCodeRegistrationTokenRequired = "REGISTRATION_TOKEN_REQUIRED"
)

// APIError represents a structured API error
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Predefined errors
var (
	ErrEmailTaken                = &APIError{Code: ErrCodeEmailTaken, Message: "This email is already registered"}
	ErrInvalidCredentials        = &APIError{Code: ErrCodeInvalidCredentials, Message: "Invalid email or password"}
	ErrOTPInvalid                = &APIError{Code: ErrCodeOTPInvalid, Message: "Invalid verification code"}
	ErrOTPExpired                = &APIError{Code: ErrCodeOTPExpired, Message: "Verification code has expired"}
	ErrOTPAttemptsExceeded       = &APIError{Code: ErrCodeOTPAttemptsExceeded, Message: "Too many incorrect attempts. Please request a new code"}
	ErrEmailNotVerified          = &APIError{Code: ErrCodeEmailNotVerified, Message: "Please verify your email first"}
	ErrRateLimited               = &APIError{Code: ErrCodeRateLimited, Message: "Too many requests. Please try again later"}
	ErrUserNotFound              = &APIError{Code: ErrCodeUserNotFound, Message: "User not found"}
	ErrInvalidRole               = &APIError{Code: ErrCodeInvalidRole, Message: "Invalid role specified"}
	ErrUnauthorized              = &APIError{Code: ErrCodeUnauthorized, Message: "Authentication required"}
	ErrTokenExpired              = &APIError{Code: ErrCodeTokenExpired, Message: "Token has expired"}
	ErrTokenInvalid              = &APIError{Code: ErrCodeTokenInvalid, Message: "Invalid token"}
	ErrInternalError             = &APIError{Code: ErrCodeInternalError, Message: "An internal error occurred"}
	ErrRegistrationTokenRequired = &APIError{Code: ErrCodeRegistrationTokenRequired, Message: "A valid registration token is required"}
)

func NewAPIError(code, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

func NewValidationError(message string) *APIError {
	return &APIError{Code: ErrCodeInvalidInput, Message: message}
}

// IsAPIError checks if an error is an APIError
func IsAPIError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr)
}

// GetAPIError extracts APIError from an error
func GetAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}
