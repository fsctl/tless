package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripTrailingSlashes(t *testing.T) {
	s1 := "/a/b/c/"
	s2 := "/a/b/c"
	s3 := "/"
	s4 := "/a/b/c/////"

	r1 := StripTrailingSlashes(s1)
	r2 := StripTrailingSlashes(s2)
	r3 := StripTrailingSlashes(s3)
	r4 := StripTrailingSlashes(s4)

	assert.Equal(t, r1, "/a/b/c")
	assert.Equal(t, r2, "/a/b/c")
	assert.Equal(t, r3, "/")
	assert.Equal(t, r4, "/a/b/c")
}
