package domain

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestRelatedCluesGiveNoDirectoryPoints(t *testing.T) {
	added := time.Unix(1_757_000_000, 0)
	self := RelatedCandidate{VideoID: 1, Path: "/media/show/夏の旅 first.mp4", Title: "夏の旅", AddedAt: added, TagIDs: []int64{7, 8}}
	other := RelatedCandidate{VideoID: 2, Path: "/media/show/夏の海 first.mp4", Title: "夏の海", AddedAt: added, TagIDs: []int64{7, 9}}

	direct := DefaultRelatedScorer.For(self)(other)
	clues := CluesOf(self, map[int64]string{7: "旅行", 8: "海"})
	// 旅行 joins this node's tag 7 by name; 海 is missing here and still counts on the open video's side.
	byClues := DefaultRelatedScorer.For(clues.Candidate(map[string]int64{"旅行": 7}))(other)

	if direct-byClues != relatedFolderPoints*RelatedScoreUnit {
		t.Fatalf("同じ台の得点 %d と手がかりの得点 %d の差 = %d, want フォルダの %d 点だけ",
			direct, byClues, direct-byClues, relatedFolderPoints)
	}
}

func TestMergedRelatedCursorForms(t *testing.T) {
	after := RelatedCursor{Score: 25_000_000, AddedAt: 1_757_000_000, VideoID: 4}
	cases := []struct {
		name   string
		cursor MergedRelatedCursor
		text   string
	}{
		{"リンクの分が終わると一台のときと同じ形", MergedRelatedCursor{
			Local: RelatedSide{After: &after}, Links: map[int64]RelatedSide{1: {Done: true}, 2: {Done: true}},
		}, after.String()},
		{"スロットごとの位置", MergedRelatedCursor{
			Local: RelatedSide{After: &after}, Links: map[int64]RelatedSide{2: {After: &after}, 1: {Done: true}},
		}, after.String() + "~1:-~2:" + after.String()},
		{"手元が先頭のままリンクが終わり", MergedRelatedCursor{Links: map[int64]RelatedSide{1: {Done: true}}}, "~1:-"},
		{"手元が終わり", MergedRelatedCursor{Local: RelatedSide{Done: true}, Links: map[int64]RelatedSide{3: {}}}, "-~3:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cursor.String(); got != tc.text {
				t.Fatalf("String() = %q, want %q", got, tc.text)
			}
			parsed, err := ParseMergedRelatedCursor(tc.text)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.String() != tc.text || parsed.Local.Done != tc.cursor.Local.Done {
				t.Fatalf("読み戻し = %+v, want %+v", parsed, tc.cursor)
			}
			for slot, side := range tc.cursor.Links {
				if parsed.Link(slot).Done != side.Done {
					t.Errorf("スロット %d の読み戻し = %+v, want %+v", slot, parsed.Link(slot), side)
				}
			}
		})
	}
	// The two-side local~peer form names no slot.
	for _, text := range []string{"-~1:-", "1.2.3~4.5.6", "~-", "-~" + after.String(), "x~1:", "~0:", "~1:~1:", "~a:"} {
		if _, err := ParseMergedRelatedCursor(text); !errors.Is(err, ErrInvalidRelatedCursor) {
			t.Errorf("%q: err = %v, want ErrInvalidRelatedCursor", text, err)
		}
	}
}

// pagedRelated pages a fixed ranking of one node in slot as its RelatedPeer would.
func pagedRelated(slot int64, ranked []RankedRelated, side RelatedSide, limit int) RelatedSidePage {
	if side.Done {
		return RelatedSidePage{Slot: slot}
	}
	items, next := PageRelated(ranked, side.After, limit)
	return RelatedSidePage{Slot: slot, Items: items, More: next != nil}
}

func TestMergeRelatedContinuesEverySlotByItsCursor(t *testing.T) {
	ranked := func(scores ...RelatedScore) []RankedRelated {
		out := make([]RankedRelated, len(scores))
		for i, score := range scores {
			out[i] = RankedRelated{Score: score, AddedAt: 100, VideoID: int64(i + 1)}
		}
		return out
	}
	nodes := map[int64][]RankedRelated{0: ranked(50, 30, 10), 1: ranked(40, 30), 2: ranked(45, 20, 5)}
	// On equal scores and times the larger id comes first, which a link's video has by its remote id.
	want := []int64{1, RemoteID(2, 1), RemoteID(1, 1), RemoteID(1, 2), 2, RemoteID(2, 2), 3, RemoteID(2, 3)}

	for _, limit := range []int{1, 2, 3, 20} {
		var got []int64
		at := MergedRelatedCursor{}
		for range 10 {
			sides := []RelatedSidePage{pagedRelated(2, nodes[2], at.Link(2), limit), pagedRelated(0, nodes[0], at.Local, limit),
				pagedRelated(1, nodes[1], at.Link(1), limit)}
			ids, next := MergeRelated(sides, at, limit)
			got = append(got, ids...)
			if next == nil {
				break
			}
			parsed, err := ParseMergedRelatedCursor(next.String())
			if err != nil {
				t.Fatalf("limit %d: %q: %v", limit, next.String(), err)
			}
			at = parsed
		}
		if !slices.Equal(got, want) {
			t.Errorf("limit %d: %v, want %v", limit, got, want)
		}
	}
}

func TestMergeRelatedLeavesOutALinkTheListDoesNotHold(t *testing.T) {
	video := []RankedRelated{{Score: 10, AddedAt: 1, VideoID: 7}, {Score: 5, AddedAt: 1, VideoID: 8}}
	// The list started with link 1 only; link 2, connected since, is not part of it.
	at := MergedRelatedCursor{Local: RelatedSide{Done: true}, Links: map[int64]RelatedSide{1: {}}}
	ids, next := MergeRelated([]RelatedSidePage{{Slot: 0}, pagedRelated(1, video, at.Link(1), 1),
		pagedRelated(2, video, at.Link(2), 1)}, at, 1)
	if want := []int64{RemoteID(1, 7)}; !slices.Equal(ids, want) || next == nil {
		t.Fatalf("ids = %v, next = %v, want %v and a next page", ids, next, want)
	}
	if side := next.Link(2); !side.Done || side.After != nil {
		t.Errorf("link 2 = %+v, want done without a position", side)
	}
}
