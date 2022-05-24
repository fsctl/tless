package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueue(t *testing.T) {
	db, err := NewDB("./trustlessbak-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	assert.NoError(t, db.EnqueueBackupItem(5))
	assert.NoError(t, db.EnqueueBackupItem(99))

	item, err := db.DequeueNextItem()
	assert.NoError(t, err)
	assert.NotEqual(t, nil, item)
	assert.Equal(t, QueueActionBackup, item.Action)
	assert.Equal(t, "5", item.Arg1)
	assert.NoError(t, db.CompleteQueueItem(item.QueueId))

	item, err = db.DequeueNextItem()
	assert.NoError(t, err)
	assert.NotEqual(t, nil, item)
	assert.Equal(t, QueueActionBackup, item.Action)
	assert.Equal(t, "99", item.Arg1)
	assert.NoError(t, db.CompleteQueueItem(item.QueueId))

	item, err = db.DequeueNextItem()
	assert.ErrorIs(t, ErrNoWork, err)
	assert.Equal(t, (*QueueItemDescription)(nil), item)
}

func TestReEnqueuQueueItem(t *testing.T) {
	db, err := NewDB("./trustlessbak-state.db")
	assert.NoError(t, err)
	defer db.Close()

	assert.NoError(t, db.DropAllTables())

	assert.NoError(t, db.CreateTablesIfNotExist())

	assert.NoError(t, db.EnqueueBackupItem(1))

	item1, err := db.DequeueNextItem()
	assert.NoError(t, err)
	assert.NotEqual(t, (*QueueItemDescription)(nil), item1)

	// Assert that there are no items left
	item2, err := db.DequeueNextItem()
	assert.ErrorIs(t, ErrNoWork, err)
	assert.Equal(t, (*QueueItemDescription)(nil), item2)

	// Re-enqueue item and assert that we can dequeue it again
	err = db.ReEnqueuQueueItem(item1.QueueId)
	assert.NoError(t, err)

	item3, err := db.DequeueNextItem()
	assert.NoError(t, err)
	assert.NotEqual(t, (*QueueItemDescription)(nil), item3)
	assert.Equal(t, item1, item3)
}
