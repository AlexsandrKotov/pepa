package repository

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pepa/pepa/internal/crypto"
)

func TestConnectionConfigProtectsTLSKey(t *testing.T) {
	crypto.SetMasterSecret("connection-encryption-regression-test-key", "")
	t.Cleanup(func() { crypto.SetMasterSecret("", "") })
	config := map[string]any{
		"tls_key": "private-key-test-value", "ssh_key": "ssh-test-value",
		"api_key": "api-test-value", "token": "vault:secret/docker/token",
		"host": "tcp://docker.example.com:2376", "tls_cert": "public-certificate",
		"insecure_tls": false,
	}
	encrypted, err := encryptConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tls_key", "ssh_key", "api_key", "token"} {
		value, ok := encrypted[key].(string)
		if !ok || !crypto.IsEncrypted(value) || value == config[key] {
			t.Errorf("%s was not encrypted", key)
		}
	}
	if encrypted["host"] != config["host"] || encrypted["tls_cert"] != config["tls_cert"] {
		t.Fatal("public config changed")
	}
	if config["tls_key"] != "private-key-test-value" {
		t.Fatal("input was mutated")
	}
	decrypted, err := decryptConfig(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decrypted, config) {
		t.Fatal("config round-trip mismatch")
	}
	again, err := encryptConfig(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, encrypted) {
		t.Fatal("startup re-encryption must be idempotent")
	}
}

func TestConnectionConfigTLSKeyEncryptionFailsClosed(t *testing.T) {
	for _, key := range []string{"ENCRYPTION_KEY", "AUTH_JWT_SECRET", "JWT_SECRET"} {
		t.Setenv(key, "")
	}
	config, err := encryptConfig(map[string]any{"tls_key": "private-test-value"})
	if err == nil || config != nil {
		t.Fatal("plaintext key accepted without encryption key")
	}
	if strings.Contains(err.Error(), "private-test-value") {
		t.Fatal("error leaked private key")
	}
}

func TestConnectionSensitiveKeyPolicy(t *testing.T) {
	for _, key := range []string{"tls_key", "TLS_KEY", "docker_tls_key", "api_key", "private_key", "ssh_key"} {
		if !isSensitiveKey(key) {
			t.Errorf("%s is not classified as sensitive", key)
		}
	}
	for _, key := range []string{"host", "host_type", "tls_cert", "tls_ca_cert", "ssh_host_key", "backend_mode"} {
		if isSensitiveKey(key) {
			t.Errorf("public field %s is classified as sensitive", key)
		}
	}
}
