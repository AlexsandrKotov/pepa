package crypto

import (
	"os"
	"strings"
	"testing"
)

func setupTestKey(t *testing.T) {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "test-encryption-key-that-is-at-least-32-characters-long")
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	setupTestKey(t)

	plaintext := "hello secret world"
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == plaintext {
		t.Fatal("encrypted value should differ from plaintext")
	}
	if !strings.HasPrefix(encrypted, "enc:v2:") {
		t.Fatalf("expected v2 format, got: %s", encrypted)
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptEmptyString(t *testing.T) {
	setupTestKey(t)

	encrypted, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty failed: %v", err)
	}
	if encrypted != "" {
		t.Fatalf("expected empty string, got %q", encrypted)
	}
}

func TestEncryptAlreadyEncrypted(t *testing.T) {
	setupTestKey(t)

	plaintext := "secret"
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Encrypting again should return the same value (idempotent)
	doubleEncrypted, err := Encrypt(encrypted)
	if err != nil {
		t.Fatalf("double Encrypt failed: %v", err)
	}
	if doubleEncrypted != encrypted {
		t.Fatal("double encryption should return the same value")
	}
}

func TestDecryptPlaintext(t *testing.T) {
	setupTestKey(t)

	// Decrypting a non-encrypted value should return it as-is
	result, err := Decrypt("plain text value")
	if err != nil {
		t.Fatalf("Decrypt plaintext failed: %v", err)
	}
	if result != "plain text value" {
		t.Fatalf("expected plaintext, got %q", result)
	}
}

func TestEncryptPathDecryptPath(t *testing.T) {
	setupTestKey(t)

	plaintext := "path-secret-value"
	path := "vault/connections/my-db"

	encrypted, err := EncryptPath(plaintext, path)
	if err != nil {
		t.Fatalf("EncryptPath failed: %v", err)
	}
	if !strings.HasPrefix(encrypted, "enc:v2:") {
		t.Fatalf("expected v2 format, got: %s", encrypted)
	}

	decrypted, err := DecryptPath(encrypted, path)
	if err != nil {
		t.Fatalf("DecryptPath failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestDecryptPathWrongPath(t *testing.T) {
	setupTestKey(t)

	plaintext := "path-secret-value"
	path := "vault/connections/my-db"
	wrongPath := "vault/connections/other-db"

	encrypted, err := EncryptPath(plaintext, path)
	if err != nil {
		t.Fatalf("EncryptPath failed: %v", err)
	}

	// Decrypting with wrong path should fail (authentication tag mismatch)
	_, err = DecryptPath(encrypted, wrongPath)
	if err == nil {
		t.Fatal("DecryptPath with wrong path should fail")
	}
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"", false},
		{"plaintext", false},
		{"enc:abc", true},
		{"enc:v2:salt:nonce:ct", true},
	}
	for _, tt := range tests {
		if got := IsEncrypted(tt.value); got != tt.expected {
			t.Errorf("IsEncrypted(%q) = %v, want %v", tt.value, got, tt.expected)
		}
	}
}

func TestIsV2Encrypted(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"", false},
		{"enc:abc", false},
		{"enc:v2:salt:nonce:ct", true},
	}
	for _, tt := range tests {
		if got := IsV2Encrypted(tt.value); got != tt.expected {
			t.Errorf("IsV2Encrypted(%q) = %v, want %v", tt.value, got, tt.expected)
		}
	}
}

func TestDerivePathKeyUniqueness(t *testing.T) {
	setupTestKey(t)

	key1, err := DerivePathKey("path/one")
	if err != nil {
		t.Fatalf("DerivePathKey failed: %v", err)
	}
	key2, err := DerivePathKey("path/two")
	if err != nil {
		t.Fatalf("DerivePathKey failed: %v", err)
	}

	if string(key1) == string(key2) {
		t.Fatal("different paths should produce different keys")
	}
}

func TestValidateKeyStrength(t *testing.T) {
	// Test with no key set
	t.Setenv("ENCRYPTION_KEY", "")
	t.Setenv("AUTH_JWT_SECRET", "")
	t.Setenv("JWT_SECRET", "")

	err := ValidateKeyStrength(false)
	if err == nil {
		t.Fatal("expected error when no key is set")
	}

	// Test with weak key in production
	t.Setenv("ENCRYPTION_KEY", "dev-secret-change-me-in-production")
	err = ValidateKeyStrength(true)
	if err == nil {
		t.Fatal("expected error for weak key in production")
	}

	// Test with weak key in development (should pass with warning)
	err = ValidateKeyStrength(false)
	if err != nil {
		t.Fatalf("dev mode should allow weak key, got: %v", err)
	}

	// Test with strong key in production
	t.Setenv("ENCRYPTION_KEY", "a-very-long-and-strong-random-key-at-least-32-chars")
	err = ValidateKeyStrength(true)
	if err != nil {
		t.Fatalf("strong key should pass validation: %v", err)
	}
}

func TestDecryptInvalidFormat(t *testing.T) {
	setupTestKey(t)

	// Invalid v2 format (missing parts)
	_, err := Decrypt("enc:v2:only-one-part")
	if err == nil {
		t.Fatal("expected error for invalid v2 format")
	}
}

func TestEncryptNoMasterKey(t *testing.T) {
	// Clear all key sources
	_ = os.Unsetenv("ENCRYPTION_KEY")
	_ = os.Unsetenv("AUTH_JWT_SECRET")
	_ = os.Unsetenv("JWT_SECRET")

	_, err := Encrypt("test")
	if err == nil {
		t.Fatal("expected error when no master key is available")
	}
}
