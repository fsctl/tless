package snapshots

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeIsWithin(t *testing.T) {
	unixTimeFiveMinutesAgo := time.Now().Unix() - 5*60
	unixTimeOneHoursAgo := time.Now().Unix() - 1*60*60
	unixTimeTwoHoursAgo := time.Now().Unix() - 2*60*60
	unixTimeOverOneDayAgo := time.Now().Unix() - (1*24*60*60 + 1)
	unixTimeOverTwoDaysAgo := time.Now().Unix() - (2*24*60*60 + 1)

	tr := TimeRangeSecondsAgo{From: 0, BackTo: OneDayInSec}
	assert.True(t, tr.TimeIsWithin(unixTimeFiveMinutesAgo))
	assert.True(t, tr.TimeIsWithin(unixTimeOneHoursAgo))
	assert.True(t, tr.TimeIsWithin(unixTimeTwoHoursAgo))
	assert.False(t, tr.TimeIsWithin(unixTimeOverOneDayAgo))
	assert.False(t, tr.TimeIsWithin(unixTimeOverTwoDaysAgo))
}

func TestGetOrderedSliceWithin(t *testing.T) {
	unixTimeFiveMinutesAgo := time.Now().Unix() - 5*60
	ssiFiveMinAgo := SnapshotInfo{
		TimestampUnix: unixTimeFiveMinutesAgo,
	}
	unixTimeOneHoursAgo := time.Now().Unix() - 1*60*60
	ssiOneHourAgo := SnapshotInfo{
		TimestampUnix: unixTimeOneHoursAgo,
	}
	unixTimeTwoHoursAgo := time.Now().Unix() - 2*60*60
	ssiTwoHoursAgo := SnapshotInfo{
		TimestampUnix: unixTimeTwoHoursAgo,
	}
	unixTimeOverOneDayAgo := time.Now().Unix() - (1*24*60*60 + 1)
	ssiOverOneDayAgo := SnapshotInfo{
		TimestampUnix: unixTimeOverOneDayAgo,
	}
	unixTimeOverTwoDaysAgo := time.Now().Unix() - (2*24*60*60 + 1)
	ssiOverTwoDaysAgo := SnapshotInfo{
		TimestampUnix: unixTimeOverTwoDaysAgo,
	}

	// Append them to set in an unordered way
	ssiFullSet := make([]SnapshotInfo, 0)
	ssiFullSet = append(ssiFullSet, ssiTwoHoursAgo)
	ssiFullSet = append(ssiFullSet, ssiOneHourAgo)
	ssiFullSet = append(ssiFullSet, ssiOverTwoDaysAgo)
	ssiFullSet = append(ssiFullSet, ssiFiveMinAgo)
	ssiFullSet = append(ssiFullSet, ssiOverOneDayAgo)

	tr := TimeRangeSecondsAgo{From: OneDayInSec, BackTo: 2 * OneDayInSec}
	orderedSlice := tr.getOrderedSliceWithin(ssiFullSet)
	assert.Equal(t, len(orderedSlice), 1)
	assert.Equal(t, unixTimeOverOneDayAgo, orderedSlice[0].TimestampUnix)

	tr2 := TimeRangeSecondsAgo{From: 0, BackTo: OneDayInSec}
	orderedSlice2 := tr2.getOrderedSliceWithin(ssiFullSet)
	assert.Equal(t, len(orderedSlice2), 3)
	assert.Equal(t, unixTimeTwoHoursAgo, orderedSlice2[0].TimestampUnix)
	assert.Equal(t, unixTimeOneHoursAgo, orderedSlice2[1].TimestampUnix)
	assert.Equal(t, unixTimeFiveMinutesAgo, orderedSlice2[2].TimestampUnix)

	tr3 := TimeRangeSecondsAgo{From: 2 * OneDayInSec, BackTo: 3 * OneDayInSec}
	orderedSlice3 := tr3.getOrderedSliceWithin(ssiFullSet)
	assert.Equal(t, len(orderedSlice3), 1)
	assert.Equal(t, unixTimeOverTwoDaysAgo, orderedSlice3[0].TimestampUnix)

	tr4 := TimeRangeSecondsAgo{From: 3 * OneDayInSec, BackTo: 7 * OneDayInSec}
	orderedSlice4 := tr4.getOrderedSliceWithin(ssiFullSet)
	assert.Zero(t, len(orderedSlice4))
}

func TestIsOldestNewestWithin(t *testing.T) {
	unixTimeFiveMinutesAgo := time.Now().Unix() - 5*60
	ssiFiveMinAgo := SnapshotInfo{
		TimestampUnix: unixTimeFiveMinutesAgo,
	}
	unixTimeOneHoursAgo := time.Now().Unix() - 1*60*60
	ssiOneHourAgo := SnapshotInfo{
		TimestampUnix: unixTimeOneHoursAgo,
	}
	unixTimeTwoHoursAgo := time.Now().Unix() - 2*60*60
	ssiTwoHoursAgo := SnapshotInfo{
		TimestampUnix: unixTimeTwoHoursAgo,
	}
	unixTimeOverOneDayAgo := time.Now().Unix() - (1*24*60*60 + 1)
	ssiOverOneDayAgo := SnapshotInfo{
		TimestampUnix: unixTimeOverOneDayAgo,
	}
	unixTimeOverTwoDaysAgo := time.Now().Unix() - (2*24*60*60 + 1)
	ssiOverTwoDaysAgo := SnapshotInfo{
		TimestampUnix: unixTimeOverTwoDaysAgo,
	}

	// Append them to set in an unordered way
	ssiFullSet := make([]SnapshotInfo, 0)
	ssiFullSet = append(ssiFullSet, ssiTwoHoursAgo)
	ssiFullSet = append(ssiFullSet, ssiOneHourAgo)
	ssiFullSet = append(ssiFullSet, ssiOverTwoDaysAgo)
	ssiFullSet = append(ssiFullSet, ssiFiveMinAgo)
	ssiFullSet = append(ssiFullSet, ssiOverOneDayAgo)

	tr := TimeRangeSecondsAgo{From: 0, BackTo: OneDayInSec}

	assert.False(t, tr.IsOldestWithin(unixTimeFiveMinutesAgo, ssiFullSet))
	assert.False(t, tr.IsOldestWithin(unixTimeOneHoursAgo, ssiFullSet))
	assert.True(t, tr.IsOldestWithin(unixTimeTwoHoursAgo, ssiFullSet))
	assert.False(t, tr.IsOldestWithin(unixTimeOverOneDayAgo, ssiFullSet))
	assert.False(t, tr.IsOldestWithin(unixTimeOverTwoDaysAgo, ssiFullSet))

	assert.True(t, tr.IsNewestWithin(unixTimeFiveMinutesAgo, ssiFullSet))
	assert.False(t, tr.IsNewestWithin(unixTimeOneHoursAgo, ssiFullSet))
	assert.False(t, tr.IsNewestWithin(unixTimeTwoHoursAgo, ssiFullSet))
	assert.False(t, tr.IsNewestWithin(unixTimeOverOneDayAgo, ssiFullSet))
	assert.False(t, tr.IsNewestWithin(unixTimeOverTwoDaysAgo, ssiFullSet))

	///

	tr2 := TimeRangeSecondsAgo{From: OneDayInSec, BackTo: 2 * OneDayInSec}

	assert.False(t, tr2.IsOldestWithin(unixTimeFiveMinutesAgo, ssiFullSet))
	assert.False(t, tr2.IsOldestWithin(unixTimeOneHoursAgo, ssiFullSet))
	assert.False(t, tr2.IsOldestWithin(unixTimeTwoHoursAgo, ssiFullSet))
	assert.True(t, tr2.IsOldestWithin(unixTimeOverOneDayAgo, ssiFullSet))
	assert.False(t, tr2.IsOldestWithin(unixTimeOverTwoDaysAgo, ssiFullSet))

	assert.False(t, tr2.IsNewestWithin(unixTimeFiveMinutesAgo, ssiFullSet))
	assert.False(t, tr2.IsNewestWithin(unixTimeOneHoursAgo, ssiFullSet))
	assert.False(t, tr2.IsNewestWithin(unixTimeTwoHoursAgo, ssiFullSet))
	assert.True(t, tr2.IsNewestWithin(unixTimeOverOneDayAgo, ssiFullSet))
	assert.False(t, tr2.IsNewestWithin(unixTimeOverTwoDaysAgo, ssiFullSet))

	///

	tr3 := TimeRangeSecondsAgo{From: 3 * OneDayInSec, BackTo: 7 * OneDayInSec}

	assert.False(t, tr3.IsOldestWithin(unixTimeFiveMinutesAgo, ssiFullSet))
	assert.False(t, tr3.IsOldestWithin(unixTimeOneHoursAgo, ssiFullSet))
	assert.False(t, tr3.IsOldestWithin(unixTimeTwoHoursAgo, ssiFullSet))
	assert.False(t, tr3.IsOldestWithin(unixTimeOverOneDayAgo, ssiFullSet))
	assert.False(t, tr3.IsOldestWithin(unixTimeOverTwoDaysAgo, ssiFullSet))

	assert.False(t, tr3.IsNewestWithin(unixTimeFiveMinutesAgo, ssiFullSet))
	assert.False(t, tr3.IsNewestWithin(unixTimeOneHoursAgo, ssiFullSet))
	assert.False(t, tr3.IsNewestWithin(unixTimeTwoHoursAgo, ssiFullSet))
	assert.False(t, tr3.IsNewestWithin(unixTimeOverOneDayAgo, ssiFullSet))
	assert.False(t, tr3.IsNewestWithin(unixTimeOverTwoDaysAgo, ssiFullSet))
}

func TestGetPruneKeepsList(t *testing.T) {
	unixTimeFiveMinutesAgo := time.Now().Unix() - 5*60
	ssiFiveMinAgo := SnapshotInfo{
		TimestampUnix: unixTimeFiveMinutesAgo,
	}
	unixTimeOneHoursAgo := time.Now().Unix() - 1*60*60
	ssiOneHourAgo := SnapshotInfo{
		TimestampUnix: unixTimeOneHoursAgo,
	}
	unixTimeTwoHoursAgo := time.Now().Unix() - 2*60*60
	ssiTwoHoursAgo := SnapshotInfo{
		TimestampUnix: unixTimeTwoHoursAgo,
	}
	unixTimeOverOneDayAgo := time.Now().Unix() - (1*24*60*60 + 1)
	ssiOverOneDayAgo := SnapshotInfo{
		TimestampUnix: unixTimeOverOneDayAgo,
	}
	unixTimeOverOneAndHalfDayAgo := time.Now().Unix() - (1*24*60*60 + 12*60*60 + 1)
	ssiOverOneAndHalfDayAgo := SnapshotInfo{
		TimestampUnix: unixTimeOverOneAndHalfDayAgo,
	}
	unixTimeOverTwoDaysAgo := time.Now().Unix() - (2*24*60*60 + 1)
	ssiOverTwoDaysAgo := SnapshotInfo{
		TimestampUnix: unixTimeOverTwoDaysAgo,
	}
	unixTimeOverTwoAndHalfDaysAgo := time.Now().Unix() - (2*24*60*60 + 12*60*60 + 1)
	ssiOverTwoAndHalfDaysAgo := SnapshotInfo{
		TimestampUnix: unixTimeOverTwoAndHalfDaysAgo,
	}

	// Append them to set in an unordered way
	ssiFullSet := make([]SnapshotInfo, 0)
	ssiFullSet = append(ssiFullSet, ssiOverOneAndHalfDayAgo)
	ssiFullSet = append(ssiFullSet, ssiTwoHoursAgo)
	ssiFullSet = append(ssiFullSet, ssiOneHourAgo)
	ssiFullSet = append(ssiFullSet, ssiOverTwoDaysAgo)
	ssiFullSet = append(ssiFullSet, ssiFiveMinAgo)
	ssiFullSet = append(ssiFullSet, ssiOverOneDayAgo)
	ssiFullSet = append(ssiFullSet, ssiOverTwoAndHalfDaysAgo)

	keeps := GetPruneKeepsList(ssiFullSet)
	assert.True(t, KeepsContains(ssiFiveMinAgo.TimestampUnix, keeps))
	assert.True(t, KeepsContains(ssiOneHourAgo.TimestampUnix, keeps))
	assert.True(t, KeepsContains(ssiTwoHoursAgo.TimestampUnix, keeps))

	assert.True(t, KeepsContains(ssiOverOneDayAgo.TimestampUnix, keeps))
	assert.False(t, KeepsContains(ssiOverOneAndHalfDayAgo.TimestampUnix, keeps))
	assert.False(t, KeepsContains(ssiOverTwoDaysAgo.TimestampUnix, keeps))
	assert.True(t, KeepsContains(ssiOverTwoAndHalfDaysAgo.TimestampUnix, keeps))
}
