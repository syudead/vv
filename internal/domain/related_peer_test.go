package domain

import (
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
		{"相手の分が終わると一台のときと同じ形", MergedRelatedCursor{Local: RelatedSide{After: &after}, Peer: RelatedSide{Done: true}}, after.String()},
		{"両方の位置", MergedRelatedCursor{Local: RelatedSide{After: &after}, Peer: RelatedSide{After: &after}}, after.String() + "~" + after.String()},
		{"手元が先頭のまま相手が終わり", MergedRelatedCursor{Peer: RelatedSide{Done: true}}, "~-"},
		{"手元が終わり", MergedRelatedCursor{Local: RelatedSide{Done: true}, Peer: RelatedSide{After: &after}}, "-~" + after.String()},
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
			if parsed.String() != tc.text || parsed.Local.Done != tc.cursor.Local.Done || parsed.Peer.Done != tc.cursor.Peer.Done {
				t.Fatalf("読み戻し = %+v, want %+v", parsed, tc.cursor)
			}
		})
	}
	for _, text := range []string{"-~-", "1.2~-", "x~", "1.2.3~4.5.6~"} {
		if _, err := ParseMergedRelatedCursor(text); err == nil {
			t.Errorf("%q を受け付けた", text)
		}
	}
}
