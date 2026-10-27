package domain

import (
	"errors"
	"slices"
	"strconv"
	"testing"
)

// testRow is one listed item: its id and its sort value, nil when missing.
type testRow struct {
	id    string
	value *string
}

func value(text string) *string { return &text }

// pageOf reads rows, already in one node's order, after cursor (an index) as a store does.
func pageOf(t *testing.T, rows []testRow, cursor string, limit int) MergeSide[string] {
	t.Helper()
	start := 0
	if cursor != "" {
		index, err := strconv.Atoi(cursor)
		if err != nil {
			t.Fatalf("cursor %q", cursor)
		}
		start = index + 1
	}
	end := min(start+limit, len(rows))
	side := MergeSide[string]{}
	for i := start; i < end; i++ {
		side.Items = append(side.Items, rows[i].id)
		side.Cursors = append(side.Cursors, ItemCursor{Cursor: strconv.Itoa(i), SortValue: rows[i].value})
	}
	if end < len(rows) {
		side.Next = strconv.Itoa(end - 1)
	}
	return side
}

// readAll pages through both sides with limit as the merged handler does, reading a done side as empty.
func readAll(t *testing.T, sort VideoSort, local, peer []testRow, limit int) []string {
	t.Helper()
	var out []string
	cursor := MergedCursor{}
	for range len(local) + len(peer) + 2 {
		localSide, peerSide := MergeSide[string]{}, MergeSide[string]{}
		if !cursor.LocalDone {
			localSide = pageOf(t, local, cursor.Local, limit)
		}
		if !cursor.PeerDone {
			peerSide = pageOf(t, peer, cursor.Peer, limit)
		}
		items, next, more := MergePages(sort, cursor, localSide, peerSide, limit)
		if len(items) > limit {
			t.Fatalf("%s: a page of %d items over limit %d", sort, len(items), limit)
		}
		out = append(out, items...)
		if !more {
			return out
		}
		parsed, err := ParseMergedCursor(next.Encode())
		if err != nil {
			t.Fatal(err)
		}
		cursor = parsed
	}
	t.Fatalf("%s: the listing did not end", sort)
	return nil
}

func TestMergePagesListsEveryItemOnceInOrder(t *testing.T) {
	for _, tc := range []struct {
		sort        VideoSort
		local, peer []testRow
		want        []string
	}{
		{
			sort:  SortAddedDesc,
			local: []testRow{{"L1", value("50")}, {"L2", value("30")}, {"L3", value("30")}, {"L4", value("9")}},
			peer:  []testRow{{"P1", value("40")}, {"P2", value("30")}, {"P3", value("10")}, {"P4", value("1")}, {"P5", value("0")}},
			want:  []string{"L1", "P1", "L2", "L3", "P2", "P3", "L4", "P4", "P5"},
		},
		{
			// Titles compare by key bytes, not as numbers.
			sort:  SortTitleAsc,
			local: []testRow{{"L1", value("10")}, {"L2", value("b")}},
			peer:  []testRow{{"P1", value("9")}, {"P2", value("a")}, {"P3", value("b")}},
			want:  []string{"L1", "P1", "P2", "L2", "P3"},
		},
		{
			// Numbers compare numerically, and missing values follow every value.
			sort:  SortDurationAsc,
			local: []testRow{{"L1", value("9")}, {"L2", nil}, {"L3", nil}},
			peer:  []testRow{{"P1", value("10")}, {"P2", value("100")}, {"P3", nil}},
			want:  []string{"L1", "P1", "P2", "L2", "L3", "P3"},
		},
		{
			sort:  SortSizeDesc,
			local: []testRow{{"L1", value("5")}, {"L2", nil}},
			peer:  []testRow{{"P1", value("7")}, {"P2", nil}},
			want:  []string{"P1", "L1", "L2", "P2"},
		},
		{
			sort:  SortRandom,
			local: []testRow{{"L1", value("3")}, {"L2", value("8")}},
			peer:  []testRow{{"P1", value("1")}, {"P2", value("8")}, {"P3", value("9")}},
			want:  []string{"P1", "L1", "L2", "P2", "P3"},
		},
		{
			// The peer has no history here; its items follow every local one in its own order.
			sort:  SortPlayedDesc,
			local: []testRow{{"L1", value("5")}, {"L2", nil}, {"L3", nil}},
			peer:  []testRow{{"P1", value("9")}, {"P2", value("1")}},
			want:  []string{"L1", "L2", "L3", "P1", "P2"},
		},
		{
			sort:  SortAddedAsc,
			local: nil,
			peer:  []testRow{{"P1", value("1")}, {"P2", value("2")}, {"P3", value("3")}},
			want:  []string{"P1", "P2", "P3"},
		},
	} {
		for _, limit := range []int{1, 2, 3, 20} {
			got := readAll(t, tc.sort, tc.local, tc.peer, limit)
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s, limit %d: %v, want %v", tc.sort, limit, got, tc.want)
			}
		}
	}
}

func TestMergedCursorRoundTripsAndRefusesOtherCursors(t *testing.T) {
	cursor := MergedCursor{Local: "local\x1fcursor", Peer: "peer", LocalDone: true, PeerDone: true, PeerSkipped: true}
	got, err := ParseMergedCursor(cursor.Encode())
	if err != nil || got != cursor {
		t.Errorf("ParseMergedCursor(Encode()) = %+v, %v, want %+v", got, err, cursor)
	}
	if got, err := ParseMergedCursor(""); err != nil || got != (MergedCursor{}) {
		t.Errorf("empty cursor = %+v, %v, want the start", got, err)
	}
	for _, text := range []string{"YWRkZWREZXNj", "m1.!", "m1.bnVsbA-x"} {
		if _, err := ParseMergedCursor(text); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("ParseMergedCursor(%q) = %v, want ErrInvalidCursor", text, err)
		}
	}
}

func TestMergePagesKeepsTheSkippedPeer(t *testing.T) {
	local := MergeSide[string]{Items: []string{"L1"}, Cursors: []ItemCursor{{Cursor: "c1", SortValue: value("1")}}, Next: "c1"}
	_, next, more := MergePages(SortAddedDesc, MergedCursor{PeerSkipped: true, PeerDone: true}, local, MergeSide[string]{}, 1)
	if !more || !next.PeerSkipped || !next.PeerDone || next.Local != "c1" {
		t.Errorf("next = %+v, more = %v, want the peer still skipped and the local side continuing", next, more)
	}
}
