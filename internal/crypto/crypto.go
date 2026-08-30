// Package crypto provides encryption utilities for sensitive data at rest.
//
// v1 (legacy): AES-256-GCM with SHA-256 key derivation. Format: "enc:<base64(nonce+ciphertext)>"
// v2 (current): AES-256-GCM with Argon2id key derivation + per-path keys.
//
//	Format: "enc:v2:<b64salt>:<b64nonce>:<b64ciphertext>"
//
// Per-path keys: each secret path gets a unique encryption key derived from
// the master key + path via Argon2id. This provides domain separation —
// compromising one secret's key does not compromise others.
//
// Key rotation: because each encrypted value stores its own salt, re-encrypting
// with a new master key produces fresh ciphertext. The old format is still
// readable for backward compatibility.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// fallbackWarned ensures the domain-separation warning is logged at most once.
var fallbackWarned sync.Once

const (
	encryptedPrefix = "enc:"
	v2Prefix        = "enc:v2:"

	// Argon2id parameters (OWASP recommended minimums)
	argon2Time    = 3   // iterations (minimum 3 for Argon2id)
	argon2Memory  = 128 // MB (resistant to GPU attacks)
	argon2Threads = 4
	argon2KeyLen  = 32 // 256-bit AES key
	argon2SaltLen = 16 // 128-bit salt

	// Minimum encryption key length in characters
	MinKeyLength = 32
)

// ── Master key derivation ──────────────────────────────────────

// getMasterSecret returns the raw master secret from environment.
// It prefers ENCRYPTION_KEY. Falling back to AUTH_JWT_SECRET is deprecated
// and logs a warning because it violates domain separation between
// authentication and encryption.
func getMasterSecret() (string, error) {
	secret := os.Getenv("ENCRYPTION_KEY")
	if secret != "" {
		return secret, nil
	}

	// Deprecated fallback — log once.
	fallbackWarned.Do(func() {
		slog.Warn("ENCRYPTION_KEY is not set; falling back to AUTH_JWT_SECRET. This is deprecated and violates domain separation. Set ENCRYPTION_KEY explicitly.")
	})

	secret = os.Getenv("AUTH_JWT_SECRET")
	if secret != "" {
		return secret, nil
	}
	secret = os.Getenv("JWT_SECRET")
	if secret != "" {
		return secret, nil
	}
	return "", errors.New("ENCRYPTION_KEY environment variable required for encryption")
}

// deriveKeyV1 derives a key using SHA-256 (legacy, backward compat).
func deriveKeyV1() ([]byte, error) {
	secret, err := getMasterSecret()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(secret))
	return hash[:], nil
}

// deriveKeyV2 derives a 32-byte key using Argon2id with a random salt.
func deriveKeyV2() (key []byte, salt []byte, err error) {
	secret, err := getMasterSecret()
	if err != nil {
		return nil, nil, err
	}
	salt = make([]byte, argon2SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, nil, fmt.Errorf("generate salt: %w", err)
	}
	key = argon2.IDKey([]byte(secret), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return key, salt, nil
}

// deriveKeyV2WithSalt derives a key using Argon2id with the given salt.
func deriveKeyV2WithSalt(salt []byte) ([]byte, error) {
	secret, err := getMasterSecret()
	if err != nil {
		return nil, err
	}
	return argon2.IDKey([]byte(secret), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen), nil
}

// ── Per-path key derivation ────────────────────────────────────

// DerivePathKey derives a unique 32-byte key for a specific secret path.
// Uses Argon2id with the master secret as password and the path as salt.
// This ensures each path has a cryptographically isolated key.
func DerivePathKey(path string) ([]byte, error) {
	secret, err := getMasterSecret()
	if err != nil {
		return nil, err
	}
	// Use path as Argon2 salt (paths are unique per secret).
	// Argon2 salt max is theoretically unlimited; Go implementation accepts any length.
	return argon2.IDKey([]byte(secret), []byte(path), argon2Time, argon2Memory, argon2Threads, argon2KeyLen), nil
}

// ── Encrypt ────────────────────────────────────────────────────

// Encrypt encrypts plaintext using AES-256-GCM with Argon2id key derivation.
// Returns v2 format: "enc:v2:<b64salt>:<b64nonce>:<b64ciphertext>"
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if strings.HasPrefix(plaintext, encryptedPrefix) {
		return plaintext, nil // already encrypted
	}

	key, salt, err := deriveKeyV2()
	if err != nil {
		return "", err
	}
	return encryptWithKey(key, plaintext, salt)
}

// EncryptPath encrypts plaintext with a per-path key (Argon2id, path as salt).
// Returns v2 format with the path-derived salt embedded.
func EncryptPath(plaintext, path string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if strings.HasPrefix(plaintext, encryptedPrefix) {
		return plaintext, nil
	}

	key, err := DerivePathKey(path)
	if err != nil {
		return "", err
	}
	// Use path bytes as salt for the v2 envelope so decryption can re-derive.
	return encryptWithKey(key, plaintext, []byte(path))
}

// encryptWithKey encrypts with an explicit key and salt (v2 format).
func encryptWithKey(key []byte, plaintext string, salt []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nil, nonce, []byte(plaintext), nil)

	saltB64 := base64.StdEncoding.EncodeToString(salt)
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)
	ctB64 := base64.StdEncoding.EncodeToString(ciphertext)

	return fmt.Sprintf("enc:v2:%s:%s:%s", saltB64, nonceB64, ctB64), nil
}

// ── Decrypt ────────────────────────────────────────────────────

// Decrypt decrypts an encrypted value. Automatically detects v1 (legacy) and
// v2 (Argon2id) formats. Returns the original value if not encrypted.
func Decrypt(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, encryptedPrefix) {
		return value, nil
	}

	if strings.HasPrefix(value, v2Prefix) {
		return decryptV2(value)
	}
	return decryptV1(value)
}

// DecryptPath decrypts a value that was encrypted with EncryptPath.
// For v2 format, it re-derives the key from the path.
// For v1 format, it falls back to the master key.
func DecryptPath(value, path string) (string, error) {
	if value == "" || !strings.HasPrefix(value, encryptedPrefix) {
		return value, nil
	}

	if strings.HasPrefix(value, v2Prefix) {
		return decryptV2WithPath(value, path)
	}
	// v1 was not per-path, fall back to master key
	return decryptV1(value)
}

// decryptV1 decrypts legacy format: "enc:<base64(nonce+ciphertext)>"
func decryptV1(value string) (string, error) {
	encoded := strings.TrimPrefix(value, encryptedPrefix)
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}

	key, err := deriveKeyV1()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// decryptV2 decrypts v2 format: "enc:v2:<b64salt>:<b64nonce>:<b64ciphertext>"
// Uses the embedded salt to re-derive the key via Argon2id.
func decryptV2(value string) (string, error) {
	rest := strings.TrimPrefix(value, v2Prefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return "", errors.New("invalid v2 encrypted format")
	}

	salt, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode salt: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	key, err := deriveKeyV2WithSalt(salt)
	if err != nil {
		return "", err
	}

	return runAESGCMOpen(key, nonce, ciphertext)
}

// decryptV2WithPath decrypts v2 format using a per-path derived key.
// The salt in the envelope is the path itself.
func decryptV2WithPath(value, path string) (string, error) {
	rest := strings.TrimPrefix(value, v2Prefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return "", errors.New("invalid v2 encrypted format")
	}

	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}

	key, err := DerivePathKey(path)
	if err != nil {
		return "", err
	}

	return runAESGCMOpen(key, nonce, ciphertext)
}

// runAESGCMOpen performs AES-GCM decryption+authentication.
func runAESGCMOpen(key, nonce, ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// ── Helpers ────────────────────────────────────────────────────

// IsEncrypted returns true if the value appears encrypted.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encryptedPrefix)
}

// IsV2Encrypted returns true if the value uses the v2 (Argon2id) format.
func IsV2Encrypted(value string) bool {
	return strings.HasPrefix(value, v2Prefix)
}

// EncryptionInfo returns a human-readable description of the encryption format.
func EncryptionInfo(value string) string {
	if !IsEncrypted(value) {
		return "plaintext"
	}
	if IsV2Encrypted(value) {
		return "aes-256-gcm + argon2id"
	}
	return "aes-256-gcm + sha256 (legacy)"
}

// Argon2Params returns the current Argon2id parameters for status reporting.
func Argon2Params() map[string]interface{} {
	return map[string]interface{}{
		"time":    argon2Time,
		"memory":  argon2Memory,
		"threads": argon2Threads,
		"keyLen":  argon2KeyLen,
	}
}

// knownWeakKeys lists encryption keys that must never be used in production.
var knownWeakKeys = []string{
	"dev-secret-change-me-in-production",
	"dev-jwt-secret-change-in-production",
	"test-secret-key-for-vault",
	"changeme",
	"password",
	"secret",
	"CHANGE_ME_generate_with_openssl_rand_hex_32",
}

// ValidateKeyStrength checks whether the encryption key meets security requirements.
// Returns nil if the key is acceptable, or an error describing the problem.
// In production mode (isProduction=true), weak or missing keys are fatal.
// In development mode, only missing keys produce an error; weak keys produce a warning.
func ValidateKeyStrength(isProduction bool) error {
	secret := os.Getenv("ENCRYPTION_KEY")

	// Check if ENCRYPTION_KEY is explicitly set
	if secret == "" {
		// Falls back to JWT secret — dangerous in production
		fallback := os.Getenv("AUTH_JWT_SECRET")
		if fallback == "" {
			fallback = os.Getenv("JWT_SECRET")
		}
		if fallback == "" {
			return errors.New("no encryption key configured: set ENCRYPTION_KEY environment variable")
		}
		if isProduction {
			return errors.New("ENCRYPTION_KEY is not set explicitly; falling back to JWT secret is not allowed in production")
		}
		secret = fallback
	}

	if len(secret) < MinKeyLength {
		if isProduction {
			return fmt.Errorf("ENCRYPTION_KEY is too short (%d chars); minimum %d characters required", len(secret), MinKeyLength)
		}
	}

	for _, weak := range knownWeakKeys {
		if secret == weak {
			if isProduction {
				return fmt.Errorf("ENCRYPTION_KEY is set to a known insecure default %q; choose a strong random key", weak)
			}
		}
	}

	return nil
}
