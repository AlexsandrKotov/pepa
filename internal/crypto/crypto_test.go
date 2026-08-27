package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"
)

func TestV2EncryptDecrypt(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-secret-key-for-vault")

	ct, err := Encrypt("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if !IsV2Encrypted(ct) {
		t.Fatal("expected v2 format")
	}

	pt, err := Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "hello world" {
		t.Fatalf("expected 'hello world', got %q", pt)
	}
}

func TestPerPathEncryptDecrypt(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-secret-key-for-vault")

	ct, err := EncryptPath("my-secret-data", "db/password")
	if err != nil {
		t.Fatal(err)
	}

	pt, err := DecryptPath(ct, "db/password")
	if err != nil {
		t.Fatal(err)
	}
	if pt != "my-secret-data" {
		t.Fatalf("expected 'my-secret-data', got %q", pt)
	}
}

func TestPerPathWrongPath(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-secret-key-for-vault")

	ct, err := EncryptPath("secret", "correct/path")
	if err != nil {
		t.Fatal(err)
	}

	_, err = DecryptPath(ct, "wrong/path")
	if err == nil {
		t.Fatal("expected error for wrong path, got nil")
	}
}

func TestV1BackwardCompat(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-secret-key-for-vault")

	// Simulate v1 encrypted data (SHA-256 derived key)
	key, err := deriveKeyV1()
	if err != nil {
		t.Fatal(err)
	}
	v1ct, err := encryptV1Compat("legacy data", key)
	if err != nil {
		t.Fatal(err)
	}

	// Should decrypt with auto-detection
	pt, err := Decrypt(v1ct)
	if err != nil {
		t.Fatal(err)
	}
	if pt != "legacy data" {
		t.Fatalf("expected 'legacy data', got %q", pt)
	}

	if IsV2Encrypted(v1ct) {
		t.Fatal("v1 should not be detected as v2")
	}
}

func TestEncryptionInfo(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "test-secret-key-for-vault")

	ct, _ := Encrypt("test")
	info := EncryptionInfo(ct)
	if info != "aes-256-gcm + argon2id" {
		t.Fatalf("expected argon2id info, got %q", info)
	}

	info2 := EncryptionInfo("plaintext")
	if info2 != "plaintext" {
		t.Fatalf("expected plaintext info, got %q", info2)
	}
}

// encryptV1Compat simulates the old v1 encryption for backward compat testing.
func encryptV1Compat(plaintext string, key []byte) (string, error) {
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
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}
