package cryptography

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveKey(t *testing.T) {
	key, err := DeriveKey("saltSALTsaltSALTsaltSALTsaltSALT", "verysecretpassword")
	assert.NoError(t, err)

	expect := []byte{0xbd, 0x99, 0x25, 0x45, 0x6c, 0x49, 0xf3, 0x7f, 0x43, 0xd1, 0xc1, 0x5d, 0x80, 0x64, 0x4, 0x16, 0xca, 0x1a, 0xb6, 0x72, 0x6d, 0x3d, 0xd8, 0xb9, 0xe1, 0x8, 0x4e, 0x13, 0xd8, 0xa4, 0xe0, 0x18}
	assert.Equal(t, expect, key)
}

func TestGenerateRandomSalt(t *testing.T) {
	salt := GenerateRandomSalt()
	assert.Equal(t, 32, len(salt))
}

func TestGenerateRandomPassphrase(t *testing.T) {
	passphrase := GenerateRandomPassphrase(5)
	componentWords := strings.Split(passphrase, "-")
	assert.Equal(t, 5, len(componentWords))

	for _, word := range componentWords {
		assert.Greater(t, len(word), 0)
	}
}
