package fstraverse

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsExcludedByGlob(t *testing.T) {
	glob := "/Users/*/Trash"

	path := "/Users/wintermute/Trash"
	assert.True(t, isExcludedByGlob(path, glob))

	path = "/Users/wintermute/Trash/"
	assert.True(t, isExcludedByGlob(path, glob))

	path = "/Users/wintermute/Trash/DeletedFile1"
	assert.True(t, isExcludedByGlob(path, glob))

	path = "/usr/wintermute/Trash"
	assert.False(t, isExcludedByGlob(path, glob))
}

func TestIsExcluded(t *testing.T) {
	excludes := []string{"/Users/*/Trash", "/Users/minterwute"}

	path := "/Users/wintermute/Trash"
	assert.True(t, isExcluded(path, excludes))

	path = "/Users/minterwute/Trash"
	assert.True(t, isExcluded(path, excludes))

	path = "/Users/wintermute/anyfile"
	assert.False(t, isExcluded(path, excludes))

	path = "/Users/minterwute/anyfile"
	assert.True(t, isExcluded(path, excludes))
}
