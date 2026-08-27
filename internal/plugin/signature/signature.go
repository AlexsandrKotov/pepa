// Package signature provides Ed25519 plugin signature verification.
//
// Each plugin binary is accompanied by two files:
//   - checksum: SHA-256 hex digest of the binary
//   - checksum.sig: Ed25519 signature of the checksum file contents
//
// The embedded public key is used to verify signatures. The private key
// never leaves the build environment (CI secrets).
package signature

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// VerifyPluginBinary checks that the plugin binary at binPath has not been
// tampered with and was signed by the PEPA private key.
//
// Expected files alongside the binary:
//
//	<binDir>/checksum      — hex-encoded SHA-256 of the binary (text file)
//	<binDir>/checksum.sig  — Ed25519 signature of the checksum file contents
//
// The signing script signs the hex text bytes directly, so we verify against
// the raw checksum file contents (not the decoded binary hash).
func VerifyPluginBinary(binPath string, pubKey ed25519.PublicKey) error {
	binDir := filepath.Dir(binPath)
	binName := filepath.Base(binPath)

	checksumPath := filepath.Join(binDir, "checksum")
	sigPath := filepath.Join(binDir, "checksum.sig")

	// 1. Read stored checksum (hex text as written by sign-plugin.sh)
	checksumRaw, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("missing checksum file for plugin %s: %w", binName, err)
	}
	storedHex := strings.TrimSpace(string(checksumRaw))
	expectedHash, err := hex.DecodeString(storedHex)
	if err != nil {
		return fmt.Errorf("invalid checksum format for plugin %s: %w", binName, err)
	}

	// 2. Read signature
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("missing signature file for plugin %s: %w", binName, err)
	}

	// 3. Compute actual hash of the binary
	actualHash, err := hashFile(binPath)
	if err != nil {
		return fmt.Errorf("failed to hash plugin binary %s: %w", binName, err)
	}

	// 4. Compare hashes — detect tampering (binary comparison)
	if !bytes.Equal(actualHash, expectedHash) {
		return fmt.Errorf("[SECURITY] plugin %s binary has been tampered with (hash mismatch)", binName)
	}

	// 5. Verify Ed25519 signature against the checksum file TEXT bytes.
	//    The signing script runs: openssl pkeyutl -sign -in checksum
	//    which signs the ASCII hex digest, not the binary hash.
	if !ed25519.Verify(pubKey, checksumRaw, sig) {
		return fmt.Errorf("[SECURITY] plugin %s signature verification failed — binary not signed by PEPA", binName)
	}

	return nil
}

// VerifyPluginYAML checks that a plugin.yaml manifest has not been tampered with.
//
// Expected files alongside plugin.yaml:
//
//	<dir>/plugin.yaml.checksum  — hex-encoded SHA-256 of plugin.yaml (text file)
//	<dir>/plugin.yaml.sig       — Ed25519 signature of the checksum file contents
func VerifyPluginYAML(yamlPath string, pubKey ed25519.PublicKey) error {
	dir := filepath.Dir(yamlPath)

	checksumPath := filepath.Join(dir, "plugin.yaml.checksum")
	sigPath := filepath.Join(dir, "plugin.yaml.sig")

	checksumRaw, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("missing plugin.yaml checksum: %w", err)
	}
	storedHex := strings.TrimSpace(string(checksumRaw))
	expectedHash, err := hex.DecodeString(storedHex)
	if err != nil {
		return fmt.Errorf("invalid plugin.yaml checksum format: %w", err)
	}

	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("missing plugin.yaml signature: %w", err)
	}

	actualHash, err := hashFile(yamlPath)
	if err != nil {
		return fmt.Errorf("failed to hash plugin.yaml: %w", err)
	}

	if !bytes.Equal(actualHash, expectedHash) {
		return fmt.Errorf("[SECURITY] plugin.yaml has been tampered with (hash mismatch)")
	}

	// Verify against the raw checksum text bytes (matches what sign-plugin.sh signs)
	if !ed25519.Verify(pubKey, checksumRaw, sig) {
		return fmt.Errorf("[SECURITY] plugin.yaml signature verification failed")
	}

	return nil
}

// ParsePublicKeyPEM decodes a PEM-encoded Ed25519 public key.
func ParsePublicKeyPEM(pemData []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("expected Ed25519 public key, got %T", pub)
	}
	return edPub, nil
}

// maxPluginSize is the maximum allowed plugin binary size (2 GB).
const maxPluginSize = 2 << 30

// hashFile computes SHA-256 hash of a file.
func hashFile(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > maxPluginSize {
		return nil, fmt.Errorf("file %s exceeds maximum size (%d > %d bytes)", path, fi.Size(), maxPluginSize)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
