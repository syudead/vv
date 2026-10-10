package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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
	page, err := playback.ListWatchHistory(ctx, domain.AudienceOwner, domain.WatchHistoryQuery{}, "", 60)
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

// limit の範囲外、読めないカーソル、未知の watch・tz、tz の無い date、形の違う date、101 文字の
// query は 400 invalid_request。日付の一覧は tz の欠けにも 400。
func TestListWatchHistoryRejectsBadParameters(t *testing.T) {
	f := newGuestFixture(t, true)
	for target, want := range map[string]wantError{
		"/api/watch-history?limit=201":                     {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?limit=0":                       {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?cursor=not-cursor":             {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor},
		"/api/watch-history?watch=unwatched":               {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?watch=bogus":                   {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?tz=Nowhere/Zone":               {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?date=2026-09&tz=Nowhere/Zone":  {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?date=2026-09&tz=":              {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?date=2026-09&tz=Local":         {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?date=2026-09":                  {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?date=2026-9&tz=Asia/Tokyo":     {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?date=2026-09-31&tz=Asia/Tokyo": {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history?query=" + strings.Repeat("a", 101): {
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonSearchTooLong, limit: maxQueryLength,
		},
		"/api/watch-history/dates":                        {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history/dates?tz=":                    {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history/dates?tz=Nowhere/Zone":        {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history/dates?tz=UTC&watch=unwatched": {status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest},
		"/api/watch-history/dates?tz=UTC&query=" + strings.Repeat("a", 101): {
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonSearchTooLong, limit: maxQueryLength,
		},
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

// ゲストには 4 つの経路とも 401 で、何も消えない。
func TestWatchHistoryRequiresOwner(t *testing.T) {
	f := newGuestFixture(t, true)
	ids := f.recordHistory(t)

	for label, rec := range map[string]*httptest.ResponseRecorder{
		"一覧":    f.env.get("/api/watch-history"),
		"1 件削除": f.deleteHistory(historyEntryPath(ids[0])),
		"全件削除":  f.deleteHistory("/api/watch-history"),
		"日付の一覧": f.env.get("/api/watch-history/dates?tz=UTC"),
	} {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("ゲストの%s: status = %d, want 401: %s", label, rec.Code, rec.Body)
		}
	}
	if page := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history", f.owner)); len(page.Items) != len(ids) {
		t.Errorf("ゲストの操作のあとの一覧 = %d 件, want %d", len(page.Items), len(ids))
	}
}

func historyItemIDs(page gen.WatchHistoryPage) []int64 {
	ids := make([]int64, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.Id)
	}
	return ids
}

// watch と query は一覧と日付の一覧の両方で件を絞る。動画の無い件は inProgress に出ない。
func TestListWatchHistoryFilterAndSearch(t *testing.T) {
	f := newGuestFixture(t, true)
	ids := f.recordHistory(t) // ライブラリに無い内容、b、a。a と b は視聴途中。
	gone, b, a := ids[0], ids[1], ids[2]

	for target, want := range map[string][]int64{
		"/api/watch-history?watch=all":                                        {gone, b, a},
		"/api/watch-history?watch=inProgress":                                 {b, a},
		"/api/watch-history?watch=watched":                                    {},
		"/api/watch-history?query=" + url.QueryEscape("消えた"):                  {gone},
		"/api/watch-history?query=" + url.QueryEscape("-消えた"):                 {b, a},
		"/api/watch-history?watch=inProgress&query=" + url.QueryEscape("消えた"): {},
	} {
		rec := f.env.get(target, f.owner)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d %s", target, rec.Code, rec.Body)
			continue
		}
		if got := historyItemIDs(decode[gen.WatchHistoryPage](t, rec)); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", target, got, want)
		}
	}

	page := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history", f.owner))
	today := page.Items[0].PlayedAt.UTC().Format(time.DateOnly)
	for target, want := range map[string][]string{
		"/api/watch-history/dates?tz=UTC":                    {today},
		"/api/watch-history/dates?tz=UTC&watch=watched":      {},
		"/api/watch-history/dates?tz=UTC&watch=inProgress":   {today},
		"/api/watch-history/dates?tz=UTC&query=nothingmatch": {},
	} {
		rec := f.env.get(target, f.owner)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d %s", target, rec.Code, rec.Body)
			continue
		}
		if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
			t.Errorf("%s: Cache-Control = %q", target, got)
		}
		if got := decode[gen.WatchHistoryDates](t, rec).Days; !slices.Equal(got, want) || got == nil {
			t.Errorf("%s = %#v, want %#v", target, got, want)
		}
	}
}

// date はその日か月の終わりより前の件から始め、nextCursor で古い件へ続く。前の月なら件は無い。
func TestListWatchHistoryDate(t *testing.T) {
	f := newGuestFixture(t, true)
	ids := f.recordHistory(t)
	page := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history", f.owner))
	newest := page.Items[0].PlayedAt.UTC()
	oldest := page.Items[len(page.Items)-1].PlayedAt.UTC()

	month := "/api/watch-history?tz=UTC&limit=2&date=" + newest.Format("2006-01")
	first := decode[gen.WatchHistoryPage](t, f.env.get(month, f.owner))
	if first.NextCursor == nil {
		t.Fatalf("その月の 1 ページ目に nextCursor が無い: %+v", first)
	}
	second := decode[gen.WatchHistoryPage](t, f.env.get(month+"&cursor="+*first.NextCursor, f.owner))
	if got := slices.Concat(historyItemIDs(first), historyItemIDs(second)); !slices.Equal(got, ids) {
		t.Errorf("その月から続けた件 = %v, want %v", got, ids)
	}

	day := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history?tz=Asia/Tokyo&date="+
		newest.In(mustLoadLocation(t, "Asia/Tokyo")).Format(time.DateOnly), f.owner))
	if got := historyItemIDs(day); !slices.Equal(got, ids) {
		t.Errorf("その日 = %v, want %v", got, ids)
	}

	previous := time.Date(oldest.Year(), oldest.Month()-1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
	before := decode[gen.WatchHistoryPage](t, f.env.get("/api/watch-history?tz=UTC&date="+previous, f.owner))
	if len(before.Items) != 0 || before.NextCursor != nil {
		t.Errorf("前の月 = %+v, want 空", before)
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}
