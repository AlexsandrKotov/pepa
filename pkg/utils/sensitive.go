// Package utils provides shared utility functions.
package utils

import "strings"

// sensitiveSubstrings contains patterns that indicate a key holds sensitive data.
// Matching is case-insensitive and uses substring comparison so that compound
// keys like "db_password" or "api_token_ref" are caught.
var sensitiveSubstrings = []string{
	"token", "password", "secret", "api_key", "apikey",
	"private_key", "access_key", "secret_key", "kubeconfig",
	"ssh_key", "api_token",
}

// IsSensitiveKey reports whether the given map key likely holds a sensitive
// value (password, token, key, …). The comparison is case-insensitive and
// matches substrings so that compound names like "db_password" are detected.
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveSubstrings {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
