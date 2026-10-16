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
	group := NewLibraryGroup(1, "/g", "g", members, progressOf(pattern))
	return PlaylistItem{ID: id * 10, Group: &group}
}

// playbackSmartFolder は id のスマートフォルダの項目を、読んだ時点の再生順 memberIDs と再生の記録
// pattern で作る。
func playbackSmartFolder(id int64, pattern string, memberIDs ...int64) PlaylistItem {
	group := playbackGroup(id, pattern, memberIDs...).Group
	group.ID = id
	return PlaylistItem{ID: id * 10, SmartFolder: &LibrarySmartFolder{LibraryGroup: *group}}
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
	// スマートフォルダ F(7: 再生順 71〜73、71 は視聴済み)。
	f := playbackSmartFolder(7, "dnn", 71, 72, 73)

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
		{name: "video item steps to the smart folder's open member", items: []PlaylistItem{a, f, b},
			item: 10, video: 1, position: 1, count: 3, next: 72, nextItemID: 70, previous: 0, nextItem: 72},
		{name: "smart folder member steps to the later member of its play order", items: []PlaylistItem{a, f, b},
			item: 70, video: 72, position: 2, count: 3, next: 73, nextItemID: 70, previous: 1, nextItem: 3},
		{name: "last smart folder member steps to the next shown item", items: []PlaylistItem{a, f, b},
			item: 70, video: 73, position: 2, count: 3, next: 3, nextItemID: 30, previous: 1, nextItem: 3},
		{name: "item after a smart folder goes back to its open member", items: []PlaylistItem{a, f, b},
			item: 30, video: 3, position: 3, count: 3, next: 0, previous: 72, nextItem: 0},
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

// 項目がスマートフォルダの項目の step だけがフォルダの id を持つ（contracts/screen-api.md の
// PlaylistStep）。フォルダの中の次の動画も持つ。
func TestPlaylistPlaybackOfSmartFolderSteps(t *testing.T) {
	a, f, g := playbackVideo(1), playbackSmartFolder(7, "dnn", 71, 72, 73), playbackGroup(2, "nn", 21, 22)
	folderOf := func(step *PlaylistStep) int64 {
		if step == nil {
			return -1
		}
		return step.SmartFolderID
	}
	tests := []struct {
		name                         string
		items                        []PlaylistItem
		item, video                  int64
		next, previousItem, nextItem int64 // -1 は step 無し
	}{
		{name: "steps onto the folder item carry the folder", items: []PlaylistItem{a, f, g},
			item: 10, video: 1, next: 7, previousItem: -1, nextItem: 7},
		{name: "next inside the folder carries the folder", items: []PlaylistItem{a, f, g},
			item: 70, video: 72, next: 7, previousItem: 0, nextItem: 0},
		{name: "steps onto video and group items carry none", items: []PlaylistItem{a, f, g},
			item: 70, video: 73, next: 0, previousItem: 0, nextItem: 0},
		{name: "previous step onto the folder item carries the folder", items: []PlaylistItem{a, f, g},
			item: 20, video: 21, next: 0, previousItem: 7, nextItem: -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PlaylistPlaybackOf(tc.items, PlaylistCurrent{ItemID: tc.item, VideoID: tc.video})
			if err != nil {
				t.Fatalf("PlaylistPlaybackOf: %v", err)
			}
			if folderOf(got.Next) != tc.next || folderOf(got.PreviousItem) != tc.previousItem || folderOf(got.NextItem) != tc.nextItem {
				t.Errorf("smartFolderId next・previousItem・nextItem = %d・%d・%d, want %d・%d・%d",
					folderOf(got.Next), folderOf(got.PreviousItem), folderOf(got.NextItem), tc.next, tc.previousItem, tc.nextItem)
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
