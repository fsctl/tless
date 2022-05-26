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

	bufTwoParts := make([]byte, 0)
	for i := 0; i < ObjStoreMultiPartUploadPartSize+1; i++ {
		bufTwoParts = append(bufTwoParts, 0x00)
	}
	eTag = ComputeETag(bufTwoParts)
	assert.Equal(t, "0cb34dc976627c8d711d213ea1c83f08-2", eTag)
}
