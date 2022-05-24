package backup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncrementNonce(t *testing.T) {
	nonce := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	assert.Equal(t, 12, len(nonce))
	incrementedNonce := incrementNonce(nonce)
	expected := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	assert.Equal(t, expected, incrementedNonce)

	nonce = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff}
	assert.Equal(t, 12, len(nonce))
	incrementedNonce = incrementNonce(nonce)
	expected = []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00}
	assert.Equal(t, expected, incrementedNonce)

	nonce = []byte{0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	assert.Equal(t, 12, len(nonce))
	incrementedNonce = incrementNonce(nonce)
	expected = []byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	assert.Equal(t, expected, incrementedNonce)
}

func TestIsNonceOneMoreThanPrev(t *testing.T) {
	nonce := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	assert.Equal(t, 12, len(nonce))
	incrementedNonce := incrementNonce(nonce)
	bIsOneMore := isNonceOneMoreThanPrev(incrementedNonce, nonce)
	assert.Equal(t, true, bIsOneMore)

	doubleIncrementedNonce := incrementNonce(incrementedNonce)
	bIsOneMore = isNonceOneMoreThanPrev(doubleIncrementedNonce, nonce)
	assert.Equal(t, false, bIsOneMore)
}

func TestInsertSlashIntoEncRelPath(t *testing.T) {
	encRelPathOrig := "WGNvZGUuYXBwL0NvbnRlbnRzL0RldmVsb3Blci9QbGF0Zm9ybXMvaVBob25lT1MucGxhdGZvcm0vTGlicmFyeS9EZXZlbG9wZXIvQ29yZVNpbXVsYXRvci9Qcm9maWxlcy9SdW50aW1lcy9pT1Muc2ltcnVudGltZS9Db250ZW50cy9SZXNvdXJjZXMvUnVudGltZVJvb3QvU3lzdGVtL0xpYnJhcnkvQXNzaXN0YW50L1VJUGx1Z2lucy9HZW5lcmFsS25vd2xlZGdlLnNpcmlVSUJ1bmRsZS9lbl9BVS5scHJvai9JbmZvUGxpc3Quc3RyaW5ncwo="

	encRelPathWithSlash := InsertSlashIntoEncRelPath(encRelPathOrig)

	// Test it
	encRelPathParts := strings.Split(encRelPathWithSlash, "/")
	assert.Equal(t, 2, len(encRelPathParts))
	encRelPath1 := encRelPathParts[0]
	encRelPath2 := encRelPathParts[1]
	assert.Equal(t, encRelPathOrig, encRelPath1+encRelPath2)
}
