package domain

import (
	"slices"
	"testing"
	"time"
)

// Both nodes' entries interleave by playedAt, newest first, whichever node runs out first; on equal times the
// local entry comes first.
func TestMergeWatchHistoryPagesListsBothNodesNewestFirst(t *testing.T) {
	for _, tc := range []struct {
		label       string
		local, peer []testRow
		want        []string
	}{
		{
			label: "local runs out first",
			local: []testRow{{"L1", value("500")}, {"L2", value("300")}},
			peer:  []testRow{{"P1", value("900")}, {"P2", value("300")}, {"P3", value("200")}, {"P4", value("50")}},
			want:  []string{"P1", "L1", "L2", "P2", "P3", "P4"},
		},
		{
			label: "peer runs out first",
			local: []testRow{{"L1", value("2000")}, {"L2", value("100")}, {"L3", value("99")}},
			peer:  []testRow{{"P1", value("1000")}},
			want:  []string{"L1", "P1", "L2", "L3"},
		},
		{label: "peer has none", local: []testRow{{"L1", value("1")}}, want: []string{"L1"}},
	} {
		for _, limit := range []int{1, 2, 20} {
			got := readAllWith(t, tc.label, MergeWatchHistoryPages[string], tc.local, tc.peer, limit)
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s, limit %d: %v, want %v", tc.label, limit, got, tc.want)
			}
		}
	}
}

// An entry's cursor continues the store's listing after it, and its sort value is its playedAt.
func TestWatchHistoryItemCursorContinuesAfterTheEntry(t *testing.T) {
	entry := WatchHistoryEntry{ID: 7, PlayedAt: time.UnixMilli(1_700_000_000_123)}
	got := WatchHistoryItemCursor(entry)
	cursor, err := DecodeWatchHistoryCursor(got.Cursor)
	if err != nil || cursor != (WatchHistoryCursor{PlayedAtMs: 1_700_000_000_123, ID: 7}) {
		t.Errorf("cursor = %+v, %v, want after entry 7 at its playedAt", cursor, err)
	}
	if got.SortValue == nil || *got.SortValue != "1700000000123" {
		t.Errorf("sort value = %v, want 1700000000123", got.SortValue)
	}
}

func TestMergeWatchHistoryDaysIsTheUnionNewestFirst(t *testing.T) {
	got := MergeWatchHistoryDays([]string{"2026-10-30", "2026-09-01"}, []string{"2026-10-31", "2026-10-30", "2025-12-31"})
	want := []string{"2026-10-31", "2026-10-30", "2026-09-01", "2025-12-31"}
	if !slices.Equal(got, want) {
		t.Errorf("days = %v, want %v", got, want)
	}
}
