package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GET /api/playlists/{id}/playback（specs/044-playlists/contracts/screen-api.md）を、本物の認証・
// 保存層・経路をつないで確かめる。順序の規則そのものは domain と store の試験が持つ。

type playlistPlaybackFixture struct {
	*libraryFixture
	id                  int64
	itemA, itemG, itemB int64
}

// newPlaylistPlaybackFixture は動画 solo、グループ show（ep1 は視聴済み、ep2 は途中）、動画 p1 の
// プレイリストを作る。
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
		VideoIDs: []int64{f.ids["solo"]}, FolderPaths: []string{filepath.Join(f.mediaDir, "show")},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := playlists.AddPlaylistItems(ctx, f.id, domain.PlaylistAdd{VideoIDs: []int64{f.ids["p1"]}}); err != nil {
		t.Fatal(err)
	}
	_, items, err := playlists.Playlist(ctx, f.id)
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
	// show は途中の ep2 から始まる。
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

// 外した項目と消したプレイリストは 404 not_found。
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

// ゲストは /api/* の既定で 401（research.md R-8）。
func TestPlaylistPlaybackIsOwnerOnly(t *testing.T) {
	f := newPlaylistPlaybackFixture(t)
	if rec := f.env.get(f.target(f.id, f.itemA, fmt.Sprint(f.ids["solo"]))); rec.Code != http.StatusUnauthorized {
		t.Errorf("ゲスト: status = %d, want 401: %s", rec.Code, rec.Body)
	}
}
