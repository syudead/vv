package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// 一覧 API の検索・絞り込み・並べ替えを、本物の保存層と経路をつないで確かめる
// （specs/013-library-search/contracts/list-api.md）。httpapi の単体テストは
// 保存層を差し替えるので、条件どおりの項目が返ることはここで見る。

type listAPIFixture struct {
	ctx     context.Context
	db      *store.DB
	root    domain.MediaFolder
	handler http.Handler
}

func newListAPIFixture(t *testing.T) listAPIFixture {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	root, err := db.AddMediaFolder(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewRouter(httpapi.Options{
		Videos: db.Library(), Playback: db.Playback(), MediaFolders: db.Settings(),
		Folders: db.Library(), Assets: fstest.MapFS{},
	})
	return listAPIFixture{ctx: ctx, db: db, root: root, handler: handler}
}

// add は登録フォルダからの相対パス rel に動画を取り込む。durationMs が 0 より
// 大きければ解析済みにする。
func (f listAPIFixture) add(t *testing.T, rel, title string, durationMs int64) (int64, string) {
	t.Helper()
	key := "key-" + rel
	got, err := f.db.UpsertVideo(f.ctx, domain.VideoFile{
		Path: filepath.Join(f.root.Path, filepath.FromSlash(rel)), Title: title, ContentKey: key,
		SizeBytes: 1, MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if durationMs > 0 {
		if err := f.db.ApplyProbe(f.ctx, got.ID, domain.Probe{DurationMs: durationMs, VideoCodec: "h264"},
			domain.Playability{Playable: true}); err != nil {
			t.Fatal(err)
		}
	}
	return got.ID, key
}

func (f listAPIFixture) get(t *testing.T, target string) gen.VideoPage {
	t.Helper()
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", target, rec.Code, rec.Body)
	}
	var page gen.VideoPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func titles(page gen.VideoPage) []string {
	out := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.Title)
	}
	return out
}

func TestListVideosAppliesQueryWatchAndSort(t *testing.T) {
	f := newListAPIFixture(t)
	f.add(t, "京都旅行 2024.mp4", "京都旅行 2024", 1_000)
	f.add(t, "京都旅行 2023.mp4", "京都旅行 2023", 9_000)
	f.add(t, "sub/京都 散歩.mp4", "京都 散歩", 5_000)
	_, watchedKey := f.add(t, "京都 夜.mp4", "京都 夜", 7_000)
	f.add(t, "奈良.mp4", "奈良", 8_000)
	if _, err := f.db.SaveProgress(f.ctx, watchedKey, domain.Progress{
		PositionMs: 7_000, DurationMs: 7_000, Completed: true, UpdatedAt: time.Unix(2, 0),
	}); err != nil {
		t.Fatal(err)
	}

	page := f.get(t, "/api/videos?query="+url.QueryEscape("京都 -2023")+"&watch=unwatched&sort=durationDesc")
	if want := []string{"京都 散歩", "京都旅行 2024"}; !slices.Equal(titles(page), want) {
		t.Errorf("titles = %q, want %q", titles(page), want)
	}
	if page.Total != 2 {
		t.Errorf("total = %d, want 2", page.Total)
	}
	wantFolders := []gen.VideoFolder{{RootId: f.root.ID, Path: "sub"}, {RootId: f.root.ID, Path: ""}}
	for i, item := range page.Items {
		if item.Folder == nil || *item.Folder != wantFolders[i] {
			t.Errorf("items[%d].folder = %+v, want %+v", i, item.Folder, wantFolders[i])
		}
	}
}

// 受け入れ条件 17: root/A で配下すべてを検索すると x と y が返り、C の z は返らない。
func TestListFolderVideosSearchesSubtreeWithStore(t *testing.T) {
	f := newListAPIFixture(t)
	f.add(t, "A/x 京都.mp4", "x 京都", 1_000)
	f.add(t, "A/B/y 京都.mp4", "y 京都", 1_000)
	f.add(t, "C/z 京都.mp4", "z 京都", 1_000)

	base := "/api/folders/" + strconv.FormatInt(f.root.ID, 10) + "/videos?path=A&query=" + url.QueryEscape("京都")
	page := f.get(t, base+"&scope=subtree&sort=titleAsc")
	if want := []string{"x 京都", "y 京都"}; !slices.Equal(titles(page), want) || page.Total != 2 {
		t.Fatalf("titles = %q (total %d), want %q", titles(page), page.Total, want)
	}
	if folder := page.Items[1].Folder; folder == nil || folder.Path != "A/B" || folder.RootId != f.root.ID {
		t.Errorf("y folder = %+v, want A/B", folder)
	}
	if folder := page.Items[0].Folder; folder == nil || folder.Path != "A" {
		t.Errorf("x folder = %+v, want A", folder)
	}

	// 既定の範囲（直下）では、孫のフォルダの y は返らない。
	page = f.get(t, base)
	if want := []string{"x 京都"}; !slices.Equal(titles(page), want) || page.Total != 1 {
		t.Errorf("direct titles = %q (total %d), want %q", titles(page), page.Total, want)
	}
}
