package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 視聴履歴の一覧と削除（specs/043-watch-history/contracts/screen-api.md）を、本物の認証と保存層で
// 確かめる。

const (
	historyPlaybackA    = "0f8fad5b-d9cb-469f-a165-70867728950e"
	historyPlaybackB    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	historyPlaybackGone = "16fd2706-8baf-433b-82eb-8c7fada847da"
)

// recordHistory は guestFixture の a、b、ライブラリに無い内容の順に 1 件ずつ視聴を記録し、
// 新しい順の id を返す（ライブラリに無い内容、b、a）。
func (f *guestFixture) recordHistory(t *testing.T) []int64 {
	t.Helper()
	ctx := context.Background()
	playback := f.env.db.Playback()
	for _, play := range []domain.Play{
		{PlaybackID: historyPlaybackA, ContentKey: "content-key-a", Title: "a"},
		{PlaybackID: historyPlaybackB, ContentKey: "content-key-b", Title: "b"},
		{PlaybackID: historyPlaybackGone, ContentKey: "content-key-gone", Title: "消えた動画"},
	} {
		if _, err := playback.SaveProgress(ctx, play.ContentKey,
			domain.Progress{PositionMs: 30_000, DurationMs: 60_000}, &play); err != nil {
			t.Fatal(err)
		}
	}
	page, err := playback.ListWatchHistory(ctx, domain.AudienceOwner, "", 60)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(page.Items))
	for _, entry := range page.Items {
		ids = append(ids, entry.ID)
	}
	return ids
}

func (f *guestFixture) deleteHistory(target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: http.MethodDelete, target: target, cookies: cookies})
}

func historyEntryPath(id int64) string {
	return "/api/watch-history/" + strconv.FormatInt(id, 10)
}

// 一覧は新しい順で、nextCursor からの 2 ページ目は重複なく続く。ライブラリに無い内容の件は video を
// 持たず、ある内容の件は再生位置・タグ・お気に入りの入った video を持つ。
func TestListWatchHistory(t *testing.T) {
	f := newGuestFixture(t, true)
	ids := f.recordHistory(t)
	if len(ids) != 3 {
		t.Fatalf("記録した履歴 = %v, want 3 件", ids)
	}

	first := f.env.get("/api/watch-history?limit=2", f.owner)
	if first.Code != http.StatusOK {
		t.Fatalf("1 ページ目: %d %s", first.Code, first.Body)
	}
	if got := first.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q, want %q", got, cacheNoStore)
	}
	page1 := decode[gen.WatchHistoryPage](t, first)
	if page1.NextCursor == nil {
		t.Fatal("1 ページ目に nextCursor が無い")
	}
	page2 := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history?limit=2&cursor="+*page1.NextCursor, f.owner))
	if page2.NextCursor != nil {
		t.Errorf("最後のページの nextCursor = %q", *page2.NextCursor)
	}
	items := slices.Concat(page1.Items, page2.Items)
	got := make([]int64, 0, len(items))
	for _, item := range items {
		got = append(got, item.Id)
	}
	if !slices.Equal(got, ids) {
		t.Fatalf("2 ページの id = %v, want %v", got, ids)
	}

	gone, b, a := items[0], items[1], items[2]
	if gone.Video != nil || gone.Title != "消えた動画" || gone.PlayedAt.IsZero() {
		t.Errorf("ライブラリに無い内容の件 = %+v", gone)
	}
	if b.Video == nil || b.Video.Id != f.ids["b"] {
		t.Errorf("b の件の video = %+v", b.Video)
	}
	if a.Video == nil || a.Video.Id != f.ids["a"] {
		t.Fatalf("a の件の video = %+v", a.Video)
	}
	if a.Video.Progress == nil || a.Video.Progress.PositionMs != 30_000 {
		t.Errorf("a の件の progress = %+v, want 30000", a.Video.Progress)
	}
	if len(a.Video.Tags) != 1 || a.Video.Tags[0].Name != guestSecretTag {
		t.Errorf("a の件の tags = %+v", a.Video.Tags)
	}
	if a.Video.Favorite == nil {
		t.Error("a の件に favorite が無い")
	}
	if want := (gen.VideoFolder{RootId: f.rootID, Path: "pub"}); a.Video.Folder == nil || *a.Video.Folder != want {
		t.Errorf("a の件の folder = %+v, want %+v", a.Video.Folder, want)
	}
}

// limit の範囲外と読めないカーソルは 400。
func TestListWatchHistoryRejectsBadParameters(t *testing.T) {
	f := newGuestFixture(t, true)
	for target, want := range map[string]wantError{
		"/api/watch-history?limit=201":         {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?limit=0":           {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?cursor=not-cursor": {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor},
	} {
		rec := f.env.get(target, f.owner)
		assertErrorBody(t, target, rec.Code, rec.Body.Bytes(), want)
	}
}

// 1 件の削除は 204、もう一度で 404。全件の削除は 204 で、空の履歴でも 204。
func TestDeleteWatchHistory(t *testing.T) {
	f := newGuestFixture(t, true)
	ids := f.recordHistory(t)

	if rec := f.deleteHistory(historyEntryPath(ids[1]), f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("1 件の削除: %d %s", rec.Code, rec.Body)
	}
	again := f.deleteHistory(historyEntryPath(ids[1]), f.owner)
	assertErrorBody(t, "消した件の削除", again.Code, again.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	if page := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history", f.owner)); len(page.Items) != 2 {
		t.Errorf("1 件消したあとの一覧 = %+v", page.Items)
	}

	for attempt := range 2 {
		if rec := f.deleteHistory("/api/watch-history", f.owner); rec.Code != http.StatusNoContent {
			t.Errorf("%d 回目の全件の削除: %d %s", attempt+1, rec.Code, rec.Body)
		}
	}
	if page := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history", f.owner)); len(page.Items) != 0 {
		t.Errorf("全件消したあとの一覧 = %+v", page.Items)
	}
}

// 1 未満の id は 400 で、何も消えない。
func TestDeleteWatchHistoryEntryRejectsInvalidID(t *testing.T) {
	f := newGuestFixture(t, true)
	ids := f.recordHistory(t)

	for _, id := range []int64{0, -1} {
		rec := f.deleteHistory(historyEntryPath(id), f.owner)
		assertErrorBody(t, historyEntryPath(id), rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	if page := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history", f.owner)); len(page.Items) != len(ids) {
		t.Errorf("400 のあとの一覧 = %d 件, want %d", len(page.Items), len(ids))
	}
}

// ゲストには 3 つの経路とも 401 で、何も消えない。
func TestWatchHistoryRequiresOwner(t *testing.T) {
	f := newGuestFixture(t, true)
	ids := f.recordHistory(t)

	for label, rec := range map[string]*httptest.ResponseRecorder{
		"一覧":    f.env.get("/api/watch-history"),
		"1 件削除": f.deleteHistory(historyEntryPath(ids[0])),
		"全件削除":  f.deleteHistory("/api/watch-history"),
	} {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("ゲストの%s: status = %d, want 401: %s", label, rec.Code, rec.Body)
		}
	}
	if page := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history", f.owner)); len(page.Items) != len(ids) {
		t.Errorf("ゲストの操作のあとの一覧 = %d 件, want %d", len(page.Items), len(ids))
	}
}
