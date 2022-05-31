// Package cryptography provides cryptography functions for
// operations like key derivation, file name encryption/decryption,
// and file contents encryption/decryption.
package cryptography

import (
	"errors"

	"golang.org/x/crypto/argon2"
)

var (
	// ErrorBadSalt is returned by DeriveKey if salt is empty or too short
	ErrorBadSalt = errors.New("salt was empty or too short")
)

// DeriveKey derives a 32-byte (256-bit) encryption key from the user's
// master password and a non-secret but random salt string
func DeriveKey(salt string, masterPassword string) ([]byte, error) {
	if len(salt) < 8 {
		return nil, ErrorBadSalt
	}
	key := argon2.IDKey([]byte(masterPassword), []byte(salt), 10, 256*1024, 4, 32)
	return key, nil
}
