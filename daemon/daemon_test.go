package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUnixTimeFromSnapshotName(t *testing.T) {
	snapshotName := "1970-01-01_00:00:07"

	unixTimestamp := getUnixTimeFromSnapshotName(snapshotName)

	assert.Equal(t, int64(7), unixTimestamp)
}
