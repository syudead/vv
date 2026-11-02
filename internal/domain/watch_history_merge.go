package domain

import (
	"slices"
	"strconv"
)

// WatchHistoryItemCursor is entry's place in a history page: the cursor that continues after it and its playedAt
// in Unix milliseconds as the sort value, so a merge compares the nodes' entries without decoding a cursor.
func WatchHistoryItemCursor(entry WatchHistoryEntry) ItemCursor {
	playedAtMs := entry.PlayedAt.UnixMilli()
	value := strconv.FormatInt(playedAtMs, 10)
	return ItemCursor{
		Cursor:    EncodeWatchHistoryCursor(WatchHistoryCursor{PlayedAtMs: playedAtMs, ID: entry.ID}),
		SortValue: &value,
	}
}

// MergeWatchHistoryPages takes up to limit entries of the nodes' history pages, newest first and on equal times the
// local entry first, then the links in slot order, with the cursor that continues every side; more is false once
// all are done.
func MergeWatchHistoryPages[T any](cursor MergedCursor, sides []MergeSide[T], limit int) ([]T, MergedCursor, bool) {
	return MergePages(SortPlayedDesc, cursor, sides, limit)
}

// MergeWatchHistoryDays is the union of the nodes' YYYY-MM-DD days, newest first.
func MergeWatchHistoryDays(nodes ...[]string) []string {
	days := slices.Concat(nodes...)
	slices.Sort(days)
	days = slices.Compact(days)
	slices.Reverse(days)
	return days
}
