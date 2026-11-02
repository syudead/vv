package domain

import (
	"errors"
	"reflect"
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
func pageOf(t *testing.T, slot int64, rows []testRow, cursor string, limit int) MergeSide[string] {
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
	side := MergeSide[string]{Slot: slot}
	for i := start; i < end; i++ {
		side.Items = append(side.Items, rows[i].id)
		side.Cursors = append(side.Cursors, ItemCursor{Cursor: strconv.Itoa(i), SortValue: rows[i].value})
	}
	if end < len(rows) {
		side.Next = strconv.Itoa(end - 1)
	}
	return side
}

type mergeFunc func(cursor MergedCursor, sides []MergeSide[string], limit int) ([]string, MergedCursor, bool)

func mergeIn(sort VideoSort) mergeFunc {
	return func(cursor MergedCursor, sides []MergeSide[string], limit int) ([]string, MergedCursor, bool) {
		return MergePages(sort, cursor, sides, limit)
	}
}

// readAllWith pages through the nodes with limit as the merged handler does: nodes[0] is this node and nodes[k]
// the link in slot k, and a done side is read as empty.
func readAllWith(t *testing.T, label string, merge mergeFunc, nodes [][]testRow, limit int) []string {
	t.Helper()
	links := make([]LinkReach, 0, len(nodes)-1)
	total := len(nodes[0])
	for slot := 1; slot < len(nodes); slot++ {
		links = append(links, LinkReach{Slot: int64(slot), Connected: true})
		total += len(nodes[slot])
	}
	var out []string
	cursor := MergedCursor{}.Begin(links)
	for range total + 2 {
		sides := make([]MergeSide[string], len(nodes))
		for slot, rows := range nodes {
			sides[slot] = MergeSide[string]{Slot: int64(slot)}
			if side := cursor.Side(int64(slot)); !side.Done {
				sides[slot] = pageOf(t, int64(slot), rows, side.Cursor, limit)
			}
		}
		// The handler may pass the sides in any order; the merge orders them by slot.
		slices.Reverse(sides)
		items, next, more := merge(cursor, sides, limit)
		if len(items) > limit {
			t.Fatalf("%s: a page of %d items over limit %d", label, len(items), limit)
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
	t.Fatalf("%s: the listing did not end", label)
	return nil
}

func TestMergePagesListsEveryItemOnceInOrder(t *testing.T) {
	for _, tc := range []struct {
		sort  VideoSort
		nodes [][]testRow
		want  []string
	}{
		{
			sort: SortAddedDesc,
			nodes: [][]testRow{
				{{"L1", value("50")}, {"L2", value("30")}, {"L3", value("30")}, {"L4", value("9")}},
				{{"P1", value("40")}, {"P2", value("30")}, {"P3", value("10")}, {"P4", value("1")}, {"P5", value("0")}},
			},
			want: []string{"L1", "P1", "L2", "L3", "P2", "P3", "L4", "P4", "P5"},
		},
		{
			// On equal values the local item comes first, then the links in slot order.
			sort: SortAddedDesc,
			nodes: [][]testRow{
				{{"L1", value("30")}, {"L2", value("5")}},
				{{"A1", value("40")}, {"A2", value("30")}, {"A3", value("1")}},
				{{"B1", value("30")}, {"B2", value("20")}, {"B3", value("5")}},
			},
			want: []string{"A1", "L1", "A2", "B1", "B2", "L2", "B3", "A3"},
		},
		{
			// Titles compare by key bytes, not as numbers.
			sort: SortTitleAsc,
			nodes: [][]testRow{
				{{"L1", value("10")}, {"L2", value("b")}},
				{{"A1", value("9")}, {"A2", value("a")}, {"A3", value("b")}},
				{{"B1", value("a")}, {"B2", value("b")}},
			},
			want: []string{"L1", "A1", "A2", "B1", "L2", "A3", "B2"},
		},
		{
			// Numbers compare numerically, and missing values follow every value.
			sort: SortDurationAsc,
			nodes: [][]testRow{
				{{"L1", value("9")}, {"L2", nil}, {"L3", nil}},
				{{"A1", value("10")}, {"A2", value("100")}, {"A3", nil}},
				{{"B1", value("50")}, {"B2", nil}},
			},
			want: []string{"L1", "A1", "B1", "A2", "L2", "L3", "A3", "B2"},
		},
		{
			sort: SortSizeDesc,
			nodes: [][]testRow{
				{{"L1", value("5")}, {"L2", nil}},
				{{"P1", value("7")}, {"P2", nil}},
			},
			want: []string{"P1", "L1", "L2", "P2"},
		},
		{
			sort: SortRandom,
			nodes: [][]testRow{
				{{"L1", value("3")}, {"L2", value("8")}},
				{{"P1", value("1")}, {"P2", value("8")}, {"P3", value("9")}},
			},
			want: []string{"P1", "L1", "L2", "P2", "P3"},
		},
		{
			// The nodes' play times interleave, and the never-played videos of every node follow.
			sort: SortPlayedDesc,
			nodes: [][]testRow{
				{{"L1", value("5")}, {"L2", nil}, {"L3", nil}},
				{{"P1", value("9")}, {"P2", value("1")}, {"P3", nil}},
			},
			want: []string{"P1", "L1", "P2", "L2", "L3", "P3"},
		},
		{
			sort:  SortAddedAsc,
			nodes: [][]testRow{nil, nil, {{"B1", value("1")}, {"B2", value("2")}, {"B3", value("3")}}},
			want:  []string{"B1", "B2", "B3"},
		},
	} {
		for _, limit := range []int{1, 2, 3, 20} {
			got := readAllWith(t, string(tc.sort), mergeIn(tc.sort), tc.nodes, limit)
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s over %d nodes, limit %d: %v, want %v", tc.sort, len(tc.nodes), limit, got, tc.want)
			}
		}
	}
}

func TestMergedCursorRoundTripsAndRefusesOtherCursors(t *testing.T) {
	cursor := MergedCursor{Sides: map[int64]SideCursor{
		0: {Cursor: "local\x1fcursor", Done: true}, 1: {Cursor: "a"}, 3: {Done: true, Skipped: true},
	}}
	got, err := ParseMergedCursor(cursor.Encode())
	if err != nil || !reflect.DeepEqual(got, cursor) {
		t.Errorf("ParseMergedCursor(Encode()) = %+v, %v, want %+v", got, err, cursor)
	}
	if got, err := ParseMergedCursor(""); err != nil || got.Started() {
		t.Errorf("empty cursor = %+v, %v, want the start", got, err)
	}
	// m1. continued a list merged with one peer before links had slots; the last one holds no local side.
	for _, text := range []string{"YWRkZWREZXNj", "m2.!", "m2.bnVsbA", "m1.eyJsIjoiYyIsInAiOiJkIn0", "m2.eyJzIjp7IjEiOnt9fX0"} {
		if _, err := ParseMergedCursor(text); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("ParseMergedCursor(%q) = %v, want ErrInvalidCursor", text, err)
		}
	}
}

func TestMergedCursorKeepsALinkOutsideTheListingSkipped(t *testing.T) {
	start := MergedCursor{}.Begin([]LinkReach{{Slot: 1, Connected: true}, {Slot: 2}})
	if got := start.Side(2); !got.Skipped || !got.Done {
		t.Errorf("a link not connected at the start = %+v, want skipped", got)
	}
	if got := start.Links(); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("links = %v, want [1 2]", got)
	}
	// A link paired after the first page stays out, and so does one skipped after a failure.
	if got := start.Side(5); !got.Skipped {
		t.Errorf("a link the listing does not hold = %+v, want skipped", got)
	}
	if got := start.Skip(1).Side(1); !got.Skipped || !got.Done {
		t.Errorf("a skipped link = %+v, want skipped", got)
	}
	if again := start.Begin([]LinkReach{{Slot: 2, Connected: true}}); !reflect.DeepEqual(again, start) {
		t.Errorf("Begin on a started listing = %+v, want it unchanged", again)
	}

	local := MergeSide[string]{Items: []string{"L1"}, Cursors: []ItemCursor{{Cursor: "c1", SortValue: value("1")}}, Next: "c1"}
	_, next, more := MergePages(SortAddedDesc, start, []MergeSide[string]{local, {Slot: 1}, {Slot: 2}}, 1)
	if !more || !next.Side(2).Skipped || !next.Side(1).Done || next.Side(0).Cursor != "c1" {
		t.Errorf("next = %+v, more = %v, want link 2 still skipped and the local side continuing", next, more)
	}
}
