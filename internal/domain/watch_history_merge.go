package domain

import (
	"slices"
	"strconv"
)

// WatchHistoryItemCursor is entry's place in a history page: the cursor that continues after it and its playedAt
// in Unix milliseconds as the sort value, so a merge compares both nodes' entries without decoding a cursor.
func WatchHistoryItemCursor(entry WatchHistoryEntry) ItemCursor {
	playedAtMs := entry.PlayedAt.UnixMilli()
	value := strconv.FormatInt(playedAtMs, 10)
	return ItemCursor{
		Cursor:    EncodeWatchHistoryCursor(WatchHistoryCursor{PlayedAtMs: playedAtMs, ID: entry.ID}),
		SortValue: &value,
	}
}

// MergeWatchHistoryPages takes up to limit entries of both nodes' history pages, newest first and the local entry
// first on equal times, with the cursor that continues both; more is false once both are done.
func MergeWatchHistoryPages[T any](cursor MergedCursor, local, peer MergeSide[T], limit int) ([]T, MergedCursor, bool) {
	return MergePages(SortPlayedDesc, cursor, local, peer, limit)
}

// MergeWatchHistoryDays is the union of two nodes' YYYY-MM-DD days, newest first.
func MergeWatchHistoryDays(local, peer []string) []string {
	days := slices.Concat(local, peer)
	slices.Sort(days)
	days = slices.Compact(days)
	slices.Reverse(days)
	return days
}
