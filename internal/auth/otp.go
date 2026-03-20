package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

const otpLength = 6

// GenerateOTP creates a 6-digit OTP code and returns both the plain code and its hash
func GenerateOTP() (string, string, error) {
	// Generate a random 6-digit code
	code := ""
	for i := 0; i < otpLength; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", "", err
		}
		code += fmt.Sprintf("%d", n.Int64())
	}

	// Hash the code for storage
	hash := HashOTP(code)

	return code, hash, nil
}

// HashOTP creates a SHA-256 hash of the OTP code
func HashOTP(code string) string {
	hash := sha256.Sum256([]byte(code))
	return hex.EncodeToString(hash[:])
}

// ValidateOTPFormat checks if the OTP code has the correct format
func ValidateOTPFormat(code string) bool {
	if len(code) != otpLength {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
