package util

import (
	"strings"
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

func TestSliceToCommaSeparatedString(t *testing.T) {
	s1 := []string{"a"}
	s1Result := sliceToCommaSeparatedString(s1)
	assert.Equal(t, `"a"`, s1Result)

	s2 := []string{"a", "b"}
	s2Result := sliceToCommaSeparatedString(s2)
	assert.Equal(t, `"a", "b"`, s2Result)

	s3 := []string{""}
	s3Result := sliceToCommaSeparatedString(s3)
	assert.Equal(t, `""`, s3Result)

	s4 := []string{}
	s4Result := sliceToCommaSeparatedString(s4)
	assert.Equal(t, ``, s4Result)
}

func TestMakeLogSafe(t *testing.T) {
	s := ""
	assert.Equal(t, "", MakeLogSafe(s))

	s = "abc"
	assert.Equal(t, "***", MakeLogSafe(s))
}

func TestGetUnixTimeFromSnapshotName(t *testing.T) {
	snapshotName := "1970-01-01_00.00.07"

	unixTimestamp := GetUnixTimeFromSnapshotName(snapshotName)

	assert.Equal(t, int64(7), unixTimestamp)
}

func TestPathComponents(t *testing.T) {
	result := pathComponents("/usr/local/bin/")
	assert.Equal(t, len(result), 4)
	assert.Equal(t, result[0], "/")
	assert.Equal(t, result[1], "usr")
	assert.Equal(t, result[2], "local")
	assert.Equal(t, result[3], "bin")

	result = pathComponents("")
	assert.Equal(t, len(result), 0)

	result = pathComponents("/")
	assert.Equal(t, len(result), 1)
	assert.Equal(t, result[0], "/")

	result = pathComponents("///////////")
	assert.Equal(t, len(result), 1)
	assert.Equal(t, result[0], "/")

	result = pathComponents("dir1/file1")
	assert.Equal(t, len(result), 2)
	assert.Equal(t, result[0], "dir1")
	assert.Equal(t, result[1], "file1")
}
