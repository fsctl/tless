package cryptography

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveKey(t *testing.T) {
	key, err := DeriveKey("saltSALTsaltSALTsaltSALTsaltSALT", "verysecretpassword")
	assert.NoError(t, err)

	expect := []byte{0x47, 0x0e, 0x0b, 0x8b, 0xee, 0x2c, 0x22, 0x07, 0x58, 0x00, 0xf3, 0x33, 0x42, 0xd9, 0x2e, 0x34, 0xf7, 0x1f, 0x20, 0xff, 0xb7, 0x98, 0xa2, 0x5c, 0x2c, 0x6a, 0xfc, 0x79, 0x36, 0x8f, 0x62, 0xba}
	assert.Equal(t, expect, key)
	//expectPaths := map[string]int{"dir/dir2/file.txt": 1, "dir/file": 2}
	//assert.Equal(t, expectPaths, paths)
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
