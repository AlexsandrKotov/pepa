package signature

import (
	"crypto/ed25519"
	_ "embed"
	"fmt"
	"sync"
)

// EmbeddedPublicKeyPEM is the PEPA plugin signing public key, compiled into
// the binary at build time. The corresponding private key never leaves the
// secure build environment.
//
//go:embed pepa-plugins-public.pem
var EmbeddedPublicKeyPEM []byte

var (
	parsedPubKey     ed25519.PublicKey
	parsedPubKeyErr  error
	parsedPubKeyOnce sync.Once
)

// EmbeddedPublicKey returns the parsed Ed25519 public key embedded in the
// binary. The result is cached after the first call.
func EmbeddedPublicKey() (ed25519.PublicKey, error) {
	parsedPubKeyOnce.Do(func() {
		parsedPubKey, parsedPubKeyErr = ParsePublicKeyPEM(EmbeddedPublicKeyPEM)
		if parsedPubKeyErr != nil {
			parsedPubKeyErr = fmt.Errorf("failed to parse embedded public key: %w", parsedPubKeyErr)
		}
	})
	return parsedPubKey, parsedPubKeyErr
}
