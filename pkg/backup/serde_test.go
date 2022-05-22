package backup

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSerde(t *testing.T) {
	metadata := dirEntMetadata{
		IsDir:  true,
		MTime:  1653231330,
		XAttrs: "test,test,test",
	}

	// serialize the struct
	buf, err := serializeMetadataStruct(metadata)
	assert.NoError(t, err)

	// append some simulated file contents after header
	buf = append(buf, []byte{0x01, 0x02, 0x03}...)

	// check that we can deserialize and get struct back plus the file contents
	metadataPtr, fileContents, err := deserializeMetadataStruct(buf)
	assert.NoError(t, err)
	assert.Equal(t, metadata.IsDir, metadataPtr.IsDir)
	assert.Equal(t, metadata.MTime, metadataPtr.MTime)
	assert.Equal(t, metadata.XAttrs, metadataPtr.XAttrs)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, fileContents)
}
