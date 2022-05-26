package objstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeETag(t *testing.T) {
	bufEmpty := []byte{}
	eTag := ComputeETag(bufEmpty)
	assert.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", eTag)

	bufSmall := []byte{0x68, 0x65, 0x6c, 0x6c, 0x6f}
	eTag = ComputeETag(bufSmall)
	assert.Equal(t, "5d41402abc4b2a76b9719d911017c592", eTag)

	assert.Equal(t, 16*1024*1024, ObjStoreMultiPartUploadPartSize, "the precomputed hash here is based on a part size of 16mb")
	bufMultipleParts := make([]byte, 0)
	for i := 0; i < 134217728+1; i++ { // create buffer of 128mb + 1 byte
		bufMultipleParts = append(bufMultipleParts, 0x00)
	}
	eTag = ComputeETag(bufMultipleParts)
	assert.Equal(t, "86264857aa7680b4be19eb2dd95be60a-9", eTag)
}
