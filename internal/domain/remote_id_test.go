package domain

import (
	"slices"
	"testing"
)

func TestRemoteIDOffsetBoundary(t *testing.T) {
	const offset = int64(1099511627776)
	if got := RemoteID(1, 0); got != offset {
		t.Fatalf("スロット 1 のオフセット = %d, want 2^40 = %d", got, offset)
	}
	for _, tc := range []struct {
		id     int64
		remote bool
	}{{1, false}, {offset - 1, false}, {offset, true}, {offset + 7, true}} {
		if got := IsRemoteID(tc.id); got != tc.remote {
			t.Errorf("IsRemoteID(%d) = %v, want %v", tc.id, got, tc.remote)
		}
	}
}

func TestRemoteIDRoundTripsThroughTheSlot(t *testing.T) {
	const maxSafe = int64(1)<<53 - 1
	if MaxRemoteSlot != 8191 {
		t.Fatalf("MaxRemoteSlot = %d, want 8191", MaxRemoteSlot)
	}
	for _, tc := range []struct{ slot, id int64 }{{1, 7}, {1, 1<<40 - 1}, {MaxRemoteSlot, 7}, {MaxRemoteSlot, 1<<40 - 1}} {
		remote := RemoteID(tc.slot, tc.id)
		if remote > maxSafe {
			t.Errorf("RemoteID(%d, %d) = %d, above 2^53 - 1", tc.slot, tc.id, remote)
		}
		if got := RemoteSlot(remote); got != tc.slot {
			t.Errorf("RemoteSlot(%d) = %d, want %d", remote, got, tc.slot)
		}
		if got := FromRemoteID(remote); got != tc.id {
			t.Errorf("FromRemoteID(%d) = %d, want %d", remote, got, tc.id)
		}
	}
	if got := RemoteSlot(1<<40 - 1); got != 0 {
		t.Errorf("手元の ID のスロット = %d, want 0", got)
	}
}

// An id a link answers outside its own range stays in that link's slot instead of naming another link's entity.
func TestRemoteIDKeepsAnOutOfRangeIDInItsSlot(t *testing.T) {
	for _, id := range []int64{1<<40 + 42, -1} {
		if got, want := RemoteID(1, id), RemoteID(1, 0); got != want {
			t.Errorf("RemoteID(1, %d) = %d (slot %d), want %d in slot 1", id, got, RemoteSlot(got), want)
		}
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
		if got := RemoteVideoURL(1, tc.in); got != tc.want {
			t.Errorf("RemoteVideoURL(1, %q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got, want := RemoteVideoURL(2, "/api/videos/7"), "/api/videos/2199023255559"; got != want {
		t.Errorf("RemoteVideoURL(2, ...) = %q, want %q", got, want)
	}
}

func TestVocabularyJoinTakesTheLocalIDByNameElseTheOffset(t *testing.T) {
	join := VocabularyJoin{
		Tags:   map[string]int64{"海": 3},
		People: map[string]int64{"": 9, "山田": 4},
	}.InSlot(2)
	for _, tc := range []struct {
		label string
		got   int64
		want  int64
	}{
		{"同じ名前のタグ", join.Tag(50, "海"), 3},
		{"手元に無いタグ", join.Tag(51, "山"), RemoteID(2, 51)},
		{"同じ名前の人物", join.Person(60, "山田"), 4},
		{"名前の無い仮の人物", join.Person(61, ""), RemoteID(2, 61)},
		{"表の無い種別", join.TagKind(70, "ジャンル"), RemoteID(2, 70)},
		{"名前で結ばない実体", join.Remote(80), RemoteID(2, 80)},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: id = %d, want %d", tc.label, tc.got, tc.want)
		}
	}
}

func TestIDNodesAndSplitByNode(t *testing.T) {
	remote := RemoteID(1, 3)
	offset := RemoteID(1, 0)
	for _, tc := range []struct {
		ids           []int64
		local, remote bool
	}{
		{[]int64{1, 2}, true, false},
		{[]int64{remote, 0}, false, true},
		{[]int64{offset - 1, offset}, true, true},
		{[]int64{0, -1}, false, false},
	} {
		local, gotRemote := IDNodes(tc.ids...)
		if local != tc.local || gotRemote != tc.remote {
			t.Errorf("IDNodes(%v) = %v, %v, want %v, %v", tc.ids, local, gotRemote, tc.local, tc.remote)
		}
	}

	local, peer := SplitRemoteIDs([]int64{5, remote, 6, RemoteID(1, 1)})
	if !slices.Equal(local, []int64{5, 6}) || !slices.Equal(peer, []int64{3, 1}) {
		t.Errorf("SplitRemoteIDs = %v, %v, want [5 6] and [3 1]", local, peer)
	}
	ids := []int64{remote, 0}
	ToPeerIDs(ids)
	if !slices.Equal(ids, []int64{3, 0}) {
		t.Errorf("ToPeerIDs = %v, want [3 0]", ids)
	}
}

// IDSlot names the one link of the remote ids, so an operation never sends one link's ids to another.
func TestIDSlotNamesOneLink(t *testing.T) {
	for _, tc := range []struct {
		ids  []int64
		slot int64
		ok   bool
	}{
		{[]int64{4, 5}, 0, true},
		{[]int64{4, RemoteID(2, 1), RemoteID(2, 9)}, 2, true},
		{[]int64{RemoteID(1, 1), RemoteID(2, 1)}, 0, false},
	} {
		if slot, ok := IDSlot(tc.ids...); slot != tc.slot || ok != tc.ok {
			t.Errorf("IDSlot(%v) = %d, %v, want %d, %v", tc.ids, slot, ok, tc.slot, tc.ok)
		}
	}
}
