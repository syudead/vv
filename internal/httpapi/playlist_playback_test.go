package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Domain/storage tests cover playback ordering; this fixture verifies route integration.

type playlistPlaybackFixture struct {
	*libraryFixture
	id                  int64
	itemA, itemG, itemB int64
}

func newPlaylistPlaybackFixture(t *testing.T) *playlistPlaybackFixture {
	t.Helper()
	ctx := context.Background()
	f := &playlistPlaybackFixture{libraryFixture: newLibraryFixture(t)}
	playlists := f.env.db.Playlists()
	playlist, err := playlists.CreatePlaylist(ctx, "連続再生")
	if err != nil {
		t.Fatal(err)
	}
	f.id = playlist.ID
	if _, err := playlists.AddPlaylistItems(ctx, f.id, domain.PlaylistAdd{
		VideoIDs: []int64{f.ids["solo"]}, GroupIDs: []int64{f.groupID(t, "show")},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := playlists.AddPlaylistItems(ctx, f.id, domain.PlaylistAdd{VideoIDs: []int64{f.ids["p1"]}}); err != nil {
		t.Fatal(err)
	}
	_, items, _, err := playlists.Playlist(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("項目 = %+v", items)
	}
	f.itemA, f.itemG, f.itemB = items[0].ID, items[1].ID, items[2].ID
	return f
}

func (f *playlistPlaybackFixture) target(id, item int64, video string) string {
	return fmt.Sprintf("/api/playlists/%d/playback?item=%d&video=%s", id, item, video)
}

func TestPlaylistPlaybackAnswersThePositionAndTheNextVideo(t *testing.T) {
	f := newPlaylistPlaybackFixture(t)

	rec := f.env.get(f.target(f.id, f.itemA, fmt.Sprint(f.ids["solo"])), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	playback := decode[gen.PlaylistPlayback](t, rec)
	if playback.Playlist.Id != f.id || playback.Playlist.Name != "連続再生" || playback.Playlist.ItemCount != 3 {
		t.Errorf("playlist = %+v", playback.Playlist)
	}
	if playback.ItemId != f.itemA || playback.Position != 1 || playback.Count != 3 || playback.PreviousItem != nil {
		t.Errorf("playback = %+v", playback)
	}
	for name, step := range map[string]*gen.PlaylistStep{"next": playback.Next, "nextItem": playback.NextItem} {
		if step == nil || step.ItemId != f.itemG || step.Video.Id != f.ids["ep2"] {
			t.Fatalf("%s = %+v, want show の ep2", name, step)
		}
		if step.Video.Progress == nil || step.Video.Favorite == nil {
			t.Errorf("%s の動画に所有者の値が無い: %+v", name, step.Video)
		}
	}

	rec = f.env.get(f.target(f.id, f.itemB, fmt.Sprint(f.ids["p1"])), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("B: status = %d: %s", rec.Code, rec.Body)
	}
	end := decode[gen.PlaylistPlayback](t, rec)
	if end.Next != nil || end.NextItem != nil || end.Position != 3 || end.Count != 3 {
		t.Errorf("B = %+v, want 終わり", end)
	}
	if end.PreviousItem == nil || end.PreviousItem.ItemId != f.itemG || end.PreviousItem.Video.Id != f.ids["ep2"] {
		t.Errorf("B の previousItem = %+v", end.PreviousItem)
	}
}

func TestPlaylistPlaybackRejectsAMalformedQuery(t *testing.T) {
	f := newPlaylistPlaybackFixture(t)
	for _, target := range []string{
		fmt.Sprintf("/api/playlists/%d/playback?item=%d", f.id, f.itemA),
		fmt.Sprintf("/api/playlists/%d/playback?video=%d", f.id, f.ids["solo"]),
		fmt.Sprintf("/api/playlists/%d/playback?item=x&video=%d", f.id, f.ids["solo"]),
		f.target(f.id, f.itemA, "1.5"),
	} {
		rec := f.env.get(target, f.owner)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
			continue
		}
		if body := decode[gen.Error](t, rec); body.Code != codeInvalidRequest {
			t.Errorf("%s: code = %q", target, body.Code)
		}
	}
}

func TestPlaylistPlaybackNotFound(t *testing.T) {
	f := newPlaylistPlaybackFixture(t)
	ctx := context.Background()
	playlists := f.env.db.Playlists()
	if err := playlists.RemovePlaylistItem(ctx, f.id, f.itemA); err != nil {
		t.Fatal(err)
	}
	solo := fmt.Sprint(f.ids["solo"])
	if rec := f.env.get(f.target(f.id, f.itemA, solo), f.owner); rec.Code != http.StatusNotFound ||
		decode[gen.Error](t, rec).Code != codeNotFound {
		t.Errorf("外した項目: status = %d: %s", rec.Code, rec.Body)
	}
	if err := playlists.DeletePlaylist(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	if rec := f.env.get(f.target(f.id, f.itemB, solo), f.owner); rec.Code != http.StatusNotFound ||
		decode[gen.Error](t, rec).Code != codeNotFound {
		t.Errorf("消したプレイリスト: status = %d: %s", rec.Code, rec.Body)
	}
}

func TestPlaylistPlaybackIsOwnerOnly(t *testing.T) {
	f := newPlaylistPlaybackFixture(t)
	if rec := f.env.get(f.target(f.id, f.itemA, fmt.Sprint(f.ids["solo"]))); rec.Code != http.StatusUnauthorized {
		t.Errorf("ゲスト: status = %d, want 401: %s", rec.Code, rec.Body)
	}
}

func TestPlaylistPlaybackNamesTheSmartFolderOfAStep(t *testing.T) {
	f := newLibraryFixture(t)
	ctx := context.Background()
	tag := f.smartFolderTag(t, "旅", "p1", "p2")
	folder := f.createSmartFolder(t, map[string]any{"name": "旅", "rule": map[string]any{"tags": []int64{tag}}})
	playlists := f.env.db.Playlists()
	playlist, err := playlists.CreatePlaylist(ctx, "フォルダ")
	if err != nil {
		t.Fatal(err)
	}
	for _, add := range []domain.PlaylistAdd{
		{VideoIDs: []int64{f.ids["solo"]}},
		{SmartFolderIDs: []int64{folder.Id}},
		{GroupIDs: []int64{f.groupID(t, "show")}},
	} {
		if _, err := playlists.AddPlaylistItems(ctx, playlist.ID, add); err != nil {
			t.Fatal(err)
		}
	}
	_, items, _, err := playlists.Playlist(ctx, playlist.ID)
	if err != nil || len(items) != 3 {
		t.Fatalf("項目 = %+v, %v", items, err)
	}
	playbackAt := func(item, video int64) gen.PlaylistPlayback {
		t.Helper()
		rec := f.env.get(fmt.Sprintf("/api/playlists/%d/playback?item=%d&video=%d", playlist.ID, item, video), f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		return decode[gen.PlaylistPlayback](t, rec)
	}
	folderOf := func(step *gen.PlaylistStep) int64 {
		if step == nil || step.SmartFolderId == nil {
			return 0
		}
		return *step.SmartFolderId
	}

	fromSolo := playbackAt(items[0].ID, f.ids["solo"])
	if fromSolo.Next == nil || fromSolo.Next.ItemId != items[1].ID || folderOf(fromSolo.Next) != folder.Id ||
		folderOf(fromSolo.NextItem) != folder.Id {
		t.Fatalf("solo から: next = %+v, nextItem = %+v, want フォルダ %d", fromSolo.Next, fromSolo.NextItem, folder.Id)
	}

	inFolder := playbackAt(items[1].ID, fromSolo.Next.Video.Id)
	if inFolder.Next == nil || inFolder.Next.ItemId != items[1].ID || folderOf(inFolder.Next) != folder.Id {
		t.Fatalf("フォルダの中: next = %+v, want フォルダの次の動画", inFolder.Next)
	}
	if folderOf(inFolder.PreviousItem) != 0 || folderOf(inFolder.NextItem) != 0 {
		t.Errorf("動画とグループの項目への step にフォルダ: previousItem = %+v, nextItem = %+v", inFolder.PreviousItem, inFolder.NextItem)
	}

	last := playbackAt(items[1].ID, inFolder.Next.Video.Id)
	if last.Next == nil || last.Next.ItemId != items[2].ID || last.Next.SmartFolderId != nil {
		t.Errorf("フォルダの最後から: next = %+v, want smartFolderId の無いグループの項目", last.Next)
	}
}
