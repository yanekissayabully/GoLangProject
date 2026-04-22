package models

import (
	"errors"
	"fmt"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

var (
	ErrEmailTaken          = &APIError{Code: "EMAIL_TAKEN", Message: "This email is already registered"}
	ErrInvalidCredentials  = &APIError{Code: "INVALID_CREDENTIALS", Message: "Invalid email or password"}
	ErrOTPInvalid          = &APIError{Code: "OTP_INVALID", Message: "Invalid verification code"}
	ErrOTPExpired          = &APIError{Code: "OTP_EXPIRED", Message: "Verification code has expired"}
	ErrOTPAttemptsExceeded = &APIError{Code: "OTP_ATTEMPTS_EXCEEDED", Message: "Too many incorrect attempts. Please request a new code"}
	ErrRateLimited         = &APIError{Code: "RATE_LIMITED", Message: "Too many requests. Please try again later"}
	ErrUserNotFound        = &APIError{Code: "USER_NOT_FOUND", Message: "User not found"}
	ErrUnauthorized        = &APIError{Code: "UNAUTHORIZED", Message: "Authentication required"}
	ErrTokenExpired        = &APIError{Code: "TOKEN_EXPIRED", Message: "Token has expired"}
	ErrTokenInvalid        = &APIError{Code: "TOKEN_INVALID", Message: "Invalid token"}
	ErrInternalError       = &APIError{Code: "INTERNAL_ERROR", Message: "An internal error occurred"}
	ErrRegTokenRequired    = &APIError{Code: "REGISTRATION_TOKEN_REQUIRED", Message: "A valid registration token is required"}
)

func NewAPIError(code, message string) *APIError {
	return &APIError{Code: code, Message: message}
}

func NewValidationError(message string) *APIError {
	return &APIError{Code: "INVALID_INPUT", Message: message}
}

func IsAPIError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr)
}

func GetAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}
