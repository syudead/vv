package domain

import (
	"errors"
	"testing"
)

func playbackVideo(id int64) PlaylistItem {
	video := Video{ID: id}
	return PlaylistItem{ID: id * 10, Video: &video}
}

func playbackGroup(id int64, pattern string, memberIDs ...int64) PlaylistItem {
	members := make([]Video, len(memberIDs))
	for i, memberID := range memberIDs {
		members[i] = Video{ID: memberID}
	}
	group := NewLibraryGroup(1, "/g", "g", members, progressOf(pattern))
	return PlaylistItem{ID: id * 10, Group: &group}
}

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
	a, g, b := playbackVideo(1), playbackGroup(2, "dnn", 21, 22, 23), playbackVideo(3)
	w := playbackGroup(4, "dd", 41, 42)
	v, h := playbackVideo(5), playbackGroup(6, "nn", 5, 61)
	f := playbackSmartFolder(7, "dnn", 71, 72, 73)
	// An empty smart folder counts as visible but has no starting video.
	e := playbackSmartFolder(8, "")

	tests := []struct {
		name       string
		items      []PlaylistItem
		item       int64
		video      int64
		position   int
		count      int
		next       int64 // Zero means absent.
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
		{name: "empty smart folder is counted but stepped over to the next item", items: []PlaylistItem{a, e, b},
			item: 10, video: 1, position: 1, count: 3, next: 3, nextItemID: 30, previous: 0, nextItem: 3},
		{name: "empty smart folder is stepped over to the previous item", items: []PlaylistItem{a, e, b},
			item: 30, video: 3, position: 3, count: 3, next: 0, previous: 1, nextItem: 0},
		{name: "empty smart folder at the end ends the playlist", items: []PlaylistItem{a, b, e},
			item: 30, video: 3, position: 2, count: 3, next: 0, previous: 1, nextItem: 0},
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
		next, previousItem, nextItem int64 // Minus one means no step.
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

func TestPlaylistPlaybackOfVirtualVideos(t *testing.T) {
	virtual := func(id int64) Video { return Video{ID: id, ContentKey: VirtualKeyPrefix + "k"} }
	virtualItem := func(id int64) PlaylistItem {
		video := virtual(id)
		return PlaylistItem{ID: id * 10, Video: &video}
	}
	a, x, y, b := playbackVideo(1), virtualItem(5), virtualItem(6), playbackVideo(3)
	f := playbackSmartFolder(7, "nnn", 71, 72, 73)
	f.SmartFolder.Members[1] = virtual(72)

	tests := []struct {
		name     string
		items    []PlaylistItem
		item     int64
		video    int64
		next     int64 // Zero means absent.
		nextItem int64
		playable int
	}{
		{name: "continuous play passes over a virtual item between files, the manual step does not",
			items: []PlaylistItem{a, x, b}, item: 10, video: 1, next: 3, nextItem: 5, playable: 2},
		{name: "only virtual items after the current one end the playlist",
			items: []PlaylistItem{a, x, y}, item: 10, video: 1, next: 0, nextItem: 5, playable: 1},
		{name: "a playlist of only virtual items has nothing to play",
			items: []PlaylistItem{x, y}, item: 50, video: 5, next: 0, nextItem: 6, playable: 0},
		{name: "a virtual member of a smart folder is passed over",
			items: []PlaylistItem{f, b}, item: 70, video: 71, next: 73, nextItem: 3, playable: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PlaylistPlaybackOf(tc.items, PlaylistCurrent{ItemID: tc.item, VideoID: tc.video})
			if err != nil {
				t.Fatalf("PlaylistPlaybackOf: %v", err)
			}
			if stepVideo(got.Next) != tc.next || stepVideo(got.NextItem) != tc.nextItem || got.PlayableCount != tc.playable {
				t.Errorf("next・nextItem・playableCount = %d・%d・%d, want %d・%d・%d",
					stepVideo(got.Next), stepVideo(got.NextItem), got.PlayableCount, tc.next, tc.nextItem, tc.playable)
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
