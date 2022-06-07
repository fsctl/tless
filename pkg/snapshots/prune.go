package snapshots

import (
	"sort"
	"time"
)

type TimeRangeSecondsAgo struct {
	From   int64
	BackTo int64
}

func (tr *TimeRangeSecondsAgo) TimeIsWithin(unixTimestamp int64) bool {
	return unixTimestamp <= time.Now().Unix()-tr.From &&
		unixTimestamp > time.Now().Unix()-tr.BackTo
}

func (tr *TimeRangeSecondsAgo) getOrderedSliceWithin(ssiFullSet []SnapshotInfo) []SnapshotInfo {
	ret := make([]SnapshotInfo, 0)
	for _, ssi := range ssiFullSet {
		if tr.TimeIsWithin(ssi.TimestampUnix) {
			ret = append(ret, ssi)
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].TimestampUnix < ret[j].TimestampUnix
	})
	return ret
}

func (tr *TimeRangeSecondsAgo) IsOldestWithin(unixTimestamp int64, ssiFullSet []SnapshotInfo) bool {
	orderedSlice := tr.getOrderedSliceWithin(ssiFullSet)
	if len(orderedSlice) == 0 {
		// Zero elements in range, so the specified one cannot be oldest or newest within that range
		return false
	} else {
		oldestInSlice := orderedSlice[0]
		return oldestInSlice.TimestampUnix == unixTimestamp
	}
}

func (tr *TimeRangeSecondsAgo) IsNewestWithin(unixTimestamp int64, ssiFullSet []SnapshotInfo) bool {
	orderedSlice := tr.getOrderedSliceWithin(ssiFullSet)
	if len(orderedSlice) == 0 {
		// Zero elements in range, so the specified one cannot be oldest or newest within that range
		return false
	} else {
		newestInSlice := orderedSlice[len(orderedSlice)-1]
		return newestInSlice.TimestampUnix == unixTimestamp
	}
}

const (
	OneHourInSec   int64 = 60 * 60
	OneDayInSec    int64 = 24 * OneHourInSec
	ThreeDaysInSec int64 = 3 * OneDayInSec
	OneWeekInSec   int64 = 7 * OneDayInSec
	OneMonthInSec  int64 = 30 * OneDayInSec
	OneYearInSec   int64 = 12 * OneMonthInSec
)

var (
	KeepEverything   = TimeRangeSecondsAgo{From: 0, BackTo: OneDayInSec}
	KeepOldestNewest = []TimeRangeSecondsAgo{
		{From: OneDayInSec, BackTo: ThreeDaysInSec},
		{From: ThreeDaysInSec, BackTo: OneWeekInSec},
		{From: OneWeekInSec, BackTo: OneMonthInSec},
		{From: OneMonthInSec, BackTo: OneYearInSec},
	}
)

func GetPruneKeepsList(snapshotInfos []SnapshotInfo) []SnapshotInfo {
	keeps := make([]SnapshotInfo, 0)

	for _, ss := range snapshotInfos {
		if KeepEverything.TimeIsWithin(ss.TimestampUnix) {
			keeps = append(keeps, ss)
			continue
		}
		for _, agoRange := range KeepOldestNewest {
			if agoRange.IsOldestWithin(ss.TimestampUnix, snapshotInfos) {
				keeps = append(keeps, ss)
				continue
			}
			if agoRange.IsNewestWithin(ss.TimestampUnix, snapshotInfos) {
				keeps = append(keeps, ss)
				continue
			}
		}
	}
	return keeps
}

func KeepsContains(searchTimestamp int64, keeps []SnapshotInfo) bool {
	for _, k := range keeps {
		if k.TimestampUnix == searchTimestamp {
			return true
		}
	}
	return false
}
