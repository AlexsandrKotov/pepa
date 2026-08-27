package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// DefaultBCryptCost is the recommended bcrypt cost for production use.
const DefaultBCryptCost = 12

// HashPassword hashes a plaintext password using bcrypt.
func HashPassword(password string, cost int) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}
	if cost <= 0 {
		cost = DefaultBCryptCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a bcrypt hash with a plaintext password.
// Returns nil if they match, bcrypt.ErrMismatchedHashAndPassword otherwise.
func CheckPassword(hash, password string) error {
	if hash == "" || password == "" {
		return errors.New("hash and password cannot be empty")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateToken creates a signed JWT for an authenticated user.
// tokenVersion is the user's current token_version from the database; it is
// used to revoke all outstanding sessions on password change or deactivation.
func GenerateToken(secret string, userID, tenantID, orgID uuid.UUID, email string, roles []string, tokenVersion int, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = 8 * time.Hour
	}
	claims := Claims{
		UserID:         userID,
		TenantID:       tenantID,
		OrganizationID: orgID,
		Email:          email,
		Roles:          roles,
		TokenVersion:   tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "pepa",
			Subject:   userID.String(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidatePasswordStrength enforces a secure password policy.
// Requires at least 8 characters with uppercase, lowercase, digit, and special character.
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch) || strings.ContainsRune("!@#$%^&*()_+-=[]{}|;':\",./<>?", ch):
			hasSpecial = true
		}
	}

	var missing []string
	if !hasUpper {
		missing = append(missing, "uppercase letter")
	}
	if !hasLower {
		missing = append(missing, "lowercase letter")
	}
	if !hasDigit {
		missing = append(missing, "digit")
	}
	if !hasSpecial {
		missing = append(missing, "special character")
	}
	if len(missing) > 0 {
		return fmt.Errorf("password must contain at least one %s", strings.Join(missing, ", "))
	}
	return nil
}
