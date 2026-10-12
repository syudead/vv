package domain

import (
	"errors"
	"testing"
)

// プレイリストの連続再生の順序（specs/044-playlists/data-model.md「Playback order」）。

func playbackVideo(id int64) PlaylistItem {
	video := Video{ID: id}
	return PlaylistItem{ID: id * 10, Video: &video}
}

// playbackGroup は id のグループの項目を、メンバー memberIDs と再生の記録 pattern
// （progressOf の書き方）で作る。
func playbackGroup(id int64, pattern string, memberIDs ...int64) PlaylistItem {
	members := make([]Video, len(memberIDs))
	for i, memberID := range memberIDs {
		members[i] = Video{ID: memberID}
	}
	group := NewLibraryGroup("/g", "g", members, progressOf(pattern))
	return PlaylistItem{ID: id * 10, Group: &group}
}

func hiddenItem(id int64) PlaylistItem { return PlaylistItem{ID: id * 10} }

func stepVideo(step *PlaylistStep) int64 {
	if step == nil {
		return 0
	}
	return step.Video.ID
}

func stepItem(step *PlaylistStep) int64 {
	if step == nil {
		return 0
	}
	return step.ItemID
}

func TestPlaylistPlaybackOf(t *testing.T) {
	// 動画 A(1)、グループ G(2: メンバー 21〜23、21 は視聴済み)、動画 B(3)。
	a, g, b := playbackVideo(1), playbackGroup(2, "dnn", 21, 22, 23), playbackVideo(3)
	// 全部見終わったグループ W(4: メンバー 41・42)。
	w := playbackGroup(4, "dd", 41, 42)
	// 動画 V(5) は単体の項目でもあり、グループ H(6) のメンバーでもある。
	v, h := playbackVideo(5), playbackGroup(6, "nn", 5, 61)

	tests := []struct {
		name       string
		items      []PlaylistItem
		item       int64
		video      int64
		position   int
		count      int
		next       int64 // 0 は無し
		nextItemID int64
		previous   int64
		nextItem   int64
	}{
		{name: "video item steps to the group's open member", items: []PlaylistItem{a, g, b},
			item: 10, video: 1, position: 1, count: 3, next: 22, nextItemID: 20, previous: 0, nextItem: 22},
		{name: "group member steps to the later member", items: []PlaylistItem{a, g, b},
			item: 20, video: 22, position: 2, count: 3, next: 23, nextItemID: 20, previous: 1, nextItem: 3},
		{name: "watched member after the start is played too", items: []PlaylistItem{a, playbackGroup(2, "ndn", 21, 22, 23), b},
			item: 20, video: 21, position: 2, count: 3, next: 22, nextItemID: 20, previous: 1, nextItem: 3},
		{name: "last member steps to the next shown item", items: []PlaylistItem{a, g, b},
			item: 20, video: 23, position: 2, count: 3, next: 3, nextItemID: 30, previous: 1, nextItem: 3},
		{name: "last shown item with no later member ends the playlist", items: []PlaylistItem{a, g, b},
			item: 30, video: 3, position: 3, count: 3, next: 0, previous: 22, nextItem: 0},
		{name: "last member of the last item ends the playlist", items: []PlaylistItem{b, a, g},
			item: 20, video: 23, position: 3, count: 3, next: 0, previous: 1, nextItem: 0},
		{name: "current video no longer a member steps to the next shown item", items: []PlaylistItem{a, g, b},
			item: 20, video: 99, position: 2, count: 3, next: 3, nextItemID: 30, previous: 1, nextItem: 3},
		{name: "hidden items are skipped and not counted", items: []PlaylistItem{hiddenItem(1), g, hiddenItem(7), b},
			item: 20, video: 23, position: 1, count: 2, next: 3, nextItemID: 30, previous: 0, nextItem: 3},
		{name: "hidden current item keeps its place", items: []PlaylistItem{a, hiddenItem(2), b},
			item: 20, video: 22, position: 0, count: 2, next: 3, nextItemID: 30, previous: 1, nextItem: 3},
		{name: "hidden current item at the end", items: []PlaylistItem{a, b, hiddenItem(2)},
			item: 20, video: 22, position: 0, count: 2, next: 0, previous: 3, nextItem: 0},
		{name: "fully watched group starts at its first member", items: []PlaylistItem{a, w},
			item: 10, video: 1, position: 1, count: 2, next: 41, nextItemID: 40, previous: 0, nextItem: 41},
		{name: "video played as a group member follows the group", items: []PlaylistItem{v, h, b},
			item: 60, video: 5, position: 2, count: 3, next: 61, nextItemID: 60, previous: 5, nextItem: 3},
		{name: "same video played as its own item follows the playlist", items: []PlaylistItem{v, h, b},
			item: 50, video: 5, position: 1, count: 3, next: 5, nextItemID: 60, previous: 0, nextItem: 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PlaylistPlaybackOf(tc.items, PlaylistCurrent{ItemID: tc.item, VideoID: tc.video})
			if err != nil {
				t.Fatalf("PlaylistPlaybackOf: %v", err)
			}
			if got.ItemID != tc.item || got.Position != tc.position || got.Count != tc.count {
				t.Errorf("item・position・count = %d・%d・%d, want %d・%d・%d",
					got.ItemID, got.Position, got.Count, tc.item, tc.position, tc.count)
			}
			if stepVideo(got.Next) != tc.next {
				t.Errorf("next = %d, want %d", stepVideo(got.Next), tc.next)
			}
			if tc.next != 0 && stepItem(got.Next) != tc.nextItemID {
				t.Errorf("next item = %d, want %d", stepItem(got.Next), tc.nextItemID)
			}
			if stepVideo(got.PreviousItem) != tc.previous {
				t.Errorf("previousItem = %d, want %d", stepVideo(got.PreviousItem), tc.previous)
			}
			if stepVideo(got.NextItem) != tc.nextItem {
				t.Errorf("nextItem = %d, want %d", stepVideo(got.NextItem), tc.nextItem)
			}
		})
	}
}

func TestPlaylistPlaybackOfMissingItem(t *testing.T) {
	_, err := PlaylistPlaybackOf([]PlaylistItem{playbackVideo(1)}, PlaylistCurrent{ItemID: 99, VideoID: 1})
	if !errors.Is(err, ErrPlaylistItemNotFound) {
		t.Fatalf("err = %v, want ErrPlaylistItemNotFound", err)
	}
}
