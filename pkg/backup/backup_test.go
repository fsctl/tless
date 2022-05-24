package backup

import (
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
