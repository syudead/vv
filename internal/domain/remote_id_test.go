package domain

import (
	"slices"
	"testing"
)

func TestRemoteIDOffsetBoundary(t *testing.T) {
	const offset = int64(1099511627776)
	if PeerIDOffset != offset {
		t.Fatalf("オフセット = %d, want 2^40 = %d", PeerIDOffset, offset)
	}
	for _, tc := range []struct {
		id     int64
		remote bool
	}{{1, false}, {offset - 1, false}, {offset, true}, {offset + 7, true}} {
		if got := IsRemoteID(tc.id); got != tc.remote {
			t.Errorf("IsRemoteID(%d) = %v, want %v", tc.id, got, tc.remote)
		}
	}
	if got := FromRemoteID(ToRemoteID(7)); got != 7 {
		t.Errorf("往復した ID = %d, want 7", got)
	}
}

func TestRemoteVideoURLRewritesThePeersVideoID(t *testing.T) {
	remote7 := "/api/videos/1099511627783"
	for _, tc := range []struct{ in, want string }{
		{"/api/videos/7/thumbnail?v=abc-r2", remote7 + "/thumbnail?v=abc-r2"},
		{"/api/videos/7/seek-thumbnail/0?v=abc", remote7 + "/seek-thumbnail/0?v=abc"},
		{"/api/videos/7", remote7},
		// Already remote on the peer means this node's own video; it is not shifted twice.
		{remote7 + "/preview", remote7 + "/preview"},
		{"/api/people/7/image", "/api/people/7/image"},
		{"/api/videos/x/thumbnail", "/api/videos/x/thumbnail"},
	} {
		if got := RemoteVideoURL(tc.in); got != tc.want {
			t.Errorf("RemoteVideoURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestVocabularyJoinTakesTheLocalIDByNameElseTheOffset(t *testing.T) {
	join := VocabularyJoin{
		Tags:   map[string]int64{"海": 3},
		People: map[string]int64{"": 9, "山田": 4},
	}
	for _, tc := range []struct {
		label string
		got   int64
		want  int64
	}{
		{"同じ名前のタグ", join.Tag(50, "海"), 3},
		{"手元に無いタグ", join.Tag(51, "山"), ToRemoteID(51)},
		{"同じ名前の人物", join.Person(60, "山田"), 4},
		{"名前の無い仮の人物", join.Person(61, ""), ToRemoteID(61)},
		{"表の無い種別", join.TagKind(70, "ジャンル"), ToRemoteID(70)},
		{"ゼロ値の結合", VocabularyJoin{}.Organization(80, "スタジオ"), ToRemoteID(80)},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: id = %d, want %d", tc.label, tc.got, tc.want)
		}
	}
}

func TestIDNodesAndSplitByNode(t *testing.T) {
	remote := ToRemoteID(3)
	for _, tc := range []struct {
		ids           []int64
		local, remote bool
	}{
		{[]int64{1, 2}, true, false},
		{[]int64{remote, 0}, false, true},
		{[]int64{PeerIDOffset - 1, PeerIDOffset}, true, true},
		{[]int64{0, -1}, false, false},
	} {
		local, gotRemote := IDNodes(tc.ids...)
		if local != tc.local || gotRemote != tc.remote {
			t.Errorf("IDNodes(%v) = %v, %v, want %v, %v", tc.ids, local, gotRemote, tc.local, tc.remote)
		}
	}

	local, peer := SplitRemoteIDs([]int64{5, remote, 6, ToRemoteID(1)})
	if !slices.Equal(local, []int64{5, 6}) || !slices.Equal(peer, []int64{3, 1}) {
		t.Errorf("SplitRemoteIDs = %v, %v, want [5 6] and [3 1]", local, peer)
	}
	ids := []int64{remote, 0}
	ToPeerIDs(ids)
	if !slices.Equal(ids, []int64{3, 0}) {
		t.Errorf("ToPeerIDs = %v, want [3 0]", ids)
	}
}
