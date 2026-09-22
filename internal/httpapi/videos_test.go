package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 一覧は items・total・nextCursor を返す。
func TestListVideosReturnsPage(t *testing.T) {
	library := &fakeLibrary{page: domain.VideoPage{
		Items:      []domain.Video{sampleVideo(1, "海辺の散歩"), sampleVideo(2, "京都の街並み")},
		Total:      42,
		NextCursor: "Y3Vyc29y",
	}}
	handler := newTestServer(t, Options{Videos: library})

	rec := do(t, handler, http.MethodGet, "/api/videos")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	page := decode[gen.VideoPage](t, rec)
	if len(page.Items) != 2 {
		t.Errorf("items = %d 件, want 2", len(page.Items))
	}
	if page.Total != 42 {
		t.Errorf("total = %d, want 42", page.Total)
	}
	if page.NextCursor == nil || *page.NextCursor != "Y3Vyc29y" {
		t.Errorf("nextCursor = %v, want Y3Vyc29y", page.NextCursor)
	}

	first := page.Items[0]
	if first.Title != "海辺の散歩" {
		t.Errorf("title = %q", first.Title)
	}
	if first.DurationMs == nil || *first.DurationMs != 8533 {
		t.Errorf("durationMs = %v, want 8533", first.DurationMs)
	}
	if !first.Playable {
		t.Error("playable = false")
	}
	// サムネイルは生成済みのときだけ URL を入れ、内容由来の版を付ける。
	if first.ThumbnailUrl == nil {
		t.Fatal("thumbnailUrl が入っていない")
	}
	if want := "/api/videos/1/thumbnail?v=abcdef012345"; *first.ThumbnailUrl != want {
		t.Errorf("thumbnailUrl = %q, want %q", *first.ThumbnailUrl, want)
	}
}

// 取り込み直後の動画も一覧に並ぶ。未取得の値は省略する。
func TestListVideosOmitsUnknownValues(t *testing.T) {
	pending := domain.Video{
		ID: 7, Title: "解析前", SizeBytes: 10,
		ContentKey: "key:10", ProbeState: domain.ProbeStatePending,
		ThumbnailState: domain.ThumbnailStatePending,
	}
	handler := newTestServer(t, Options{Videos: &fakeLibrary{
		page: domain.VideoPage{Items: []domain.Video{pending}, Total: 1},
	}})

	page := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos"))
	item := page.Items[0]

	if item.DurationMs != nil {
		t.Errorf("durationMs = %v, want 省略", *item.DurationMs)
	}
	if item.Width != nil || item.Height != nil {
		t.Errorf("解像度が 0 で埋まっている: %v x %v", item.Width, item.Height)
	}
	// 未生成のサムネイルは URL を出さない。クライアントは枠だけを描く。
	if item.ThumbnailUrl != nil {
		t.Errorf("thumbnailUrl = %v, want 省略", *item.ThumbnailUrl)
	}
	if item.Playable {
		t.Error("解析前なのに playable = true")
	}
}

// 再生できない動画は、理由まで一覧に出す。
func TestListVideosExposesUnplayableReason(t *testing.T) {
	unplayable := sampleVideo(3, "対応外の動画")
	unplayable.Playable = false
	unplayable.UnplayableReason = domain.ReasonContainer
	unplayable.Container = "mkv"

	handler := newTestServer(t, Options{Videos: &fakeLibrary{
		page: domain.VideoPage{Items: []domain.Video{unplayable}, Total: 1},
	}})

	item := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos")).Items[0]
	if item.Playable {
		t.Error("playable = true")
	}
	if item.UnplayableReason == nil || *item.UnplayableReason != gen.Container {
		t.Errorf("unplayableReason = %v, want container", item.UnplayableReason)
	}
}

// limit は既定 60・上限 200。
func TestListVideosLimitDefaultsAndCaps(t *testing.T) {
	tests := []struct {
		target string
		want   int
	}{
		{"/api/videos", domain.DefaultLimit},
		{"/api/videos?limit=10", 10},
		{"/api/videos?limit=200", domain.MaxLimit},
		{"/api/videos?limit=1000", domain.MaxLimit},
	}

	for _, tc := range tests {
		library := &fakeLibrary{}
		handler := newTestServer(t, Options{Videos: library})

		rec := do(t, handler, http.MethodGet, tc.target)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200: %s", tc.target, rec.Code, rec.Body)
			continue
		}
		if library.lastQuery.Limit != tc.want {
			t.Errorf("%s: limit = %d, want %d", tc.target, library.lastQuery.Limit, tc.want)
		}
	}
}

// 数値として読めない limit は誤りにする。黙って既定へ倒すと、指定が効いて
// いないことに気付けない。
func TestListVideosRejectsMalformedLimit(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}})

	for _, target := range []string{"/api/videos?limit=たくさん", "/api/videos?limit=0", "/api/videos?limit=-1"} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
			continue
		}
		if got := decode[gen.Error](t, rec); got.Code != codeInvalidRequest {
			t.Errorf("%s: code = %q, want %s", target, got.Code, codeInvalidRequest)
		}
	}
}

// 並び順を渡せること。未知の値は誤りにする。
func TestListVideosSort(t *testing.T) {
	for target, want := range map[string]domain.VideoSort{
		"/api/videos":                domain.SortAddedDesc,
		"/api/videos?sort=addedDesc": domain.SortAddedDesc,
		"/api/videos?sort=titleAsc":  domain.SortTitleAsc,
	} {
		library := &fakeLibrary{}
		handler := newTestServer(t, Options{Videos: library})

		if rec := do(t, handler, http.MethodGet, target); rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200: %s", target, rec.Code, rec.Body)
			continue
		}
		if library.lastQuery.Sort != want {
			t.Errorf("%s: sort = %q, want %q", target, library.lastQuery.Sort, want)
		}
	}

	handler := newTestServer(t, Options{Videos: &fakeLibrary{}})
	if rec := do(t, handler, http.MethodGet, "/api/videos?sort=random"); rec.Code != http.StatusBadRequest {
		t.Errorf("未知の並び順で status = %d, want 400", rec.Code)
	}
}

// 壊れたカーソルは 400 + invalid_request。黙って先頭から返すと、無限
// スクロールが巻き戻って同じ内容を延々と表示することになる。
func TestListVideosRejectsBrokenCursor(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{
		listErr: domain.ErrInvalidCursor,
	}})

	rec := do(t, handler, http.MethodGet, "/api/videos?cursor=壊れている")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Error](t, rec); got.Code != codeInvalidRequest {
		t.Errorf("code = %q, want %s", got.Code, codeInvalidRequest)
	}
}

// 一覧・詳細は中間キャッシュに残さない。取り込みで内容が変わり続けるため。
func TestVideoResponsesAreNotCached(t *testing.T) {
	library := &fakeLibrary{videos: map[int64]domain.Video{1: sampleVideo(1, "a")}}
	handler := newTestServer(t, Options{Videos: library})

	for _, target := range []string{"/api/videos", "/api/videos/1"} {
		rec := do(t, handler, http.MethodGet, target)
		if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
			t.Errorf("%s: Cache-Control = %q, want %q", target, got, cacheNoStore)
		}
	}
}

// 詳細を返せること。
func TestGetVideo(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{
		videos: map[int64]domain.Video{7: sampleVideo(7, "京都の街並み")},
	}})

	rec := do(t, handler, http.MethodGet, "/api/videos/7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	video := decode[gen.Video](t, rec)
	if video.Id != 7 || video.Title != "京都の街並み" {
		t.Errorf("応答 = %+v", video)
	}
}

// 存在しない id は 404 + not_found。
func TestGetVideoNotFound(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}})

	rec := do(t, handler, http.MethodGet, "/api/videos/999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Error](t, rec); got.Code != codeNotFound {
		t.Errorf("code = %q, want %s", got.Code, codeNotFound)
	}
}

// 保存層の予期しない失敗は 500 にする。400 や 404 に丸めると、利用者にも
// 運用者にも原因が伝わらない。
func TestListVideosReportsUnexpectedFailure(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{
		listErr: errors.New("データベースが壊れています"),
	}})

	rec := do(t, handler, http.MethodGet, "/api/videos")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := decode[gen.Error](t, rec); got.Code != codeInternal {
		t.Errorf("code = %q, want %s", got.Code, codeInternal)
	}
}

// 一覧の問い合わせ先が無い構成でも、500 を返して動き続ける。
func TestListVideosWithoutStore(t *testing.T) {
	handler := newTestServer(t, Options{})

	if rec := do(t, handler, http.MethodGet, "/api/videos"); rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// 検索語を渡すと絞り込まれ、total は絞り込み後の件数になる。
func TestListVideosPassesQuery(t *testing.T) {
	library := &fakeLibrary{page: domain.VideoPage{
		Items: []domain.Video{sampleVideo(1, "夏休みの旅行")},
		Total: 1,
	}}
	handler := newTestServer(t, Options{Videos: library})

	rec := do(t, handler, http.MethodGet, "/api/videos?query="+url.QueryEscape("旅行"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if library.lastQuery.Query != "旅行" {
		t.Errorf("検索語 = %q, want 旅行", library.lastQuery.Query)
	}

	page := decode[gen.VideoPage](t, rec)
	if page.Total != 1 {
		t.Errorf("total = %d, want 1", page.Total)
	}
}

// 該当が無ければ items は空で total は 0。画面はここで「該当なし」と
// 次に取れる操作を示す。
func TestListVideosWithNoMatches(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{
		page: domain.VideoPage{Items: []domain.Video{}, Total: 0},
	}})

	rec := do(t, handler, http.MethodGet, "/api/videos?query="+url.QueryEscape("該当しない語"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	page := decode[gen.VideoPage](t, rec)
	if len(page.Items) != 0 || page.Total != 0 {
		t.Errorf("items = %d 件, total = %d, want 0 と 0", len(page.Items), page.Total)
	}
	// null ではなく空配列で返す。クライアントが分岐を持たずに描ける。
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("items が空配列で返っていない: %s", rec.Body)
	}
}

// 検索語の長さの上限は契約（api/openapi.yaml の maxLength）と同じ 100 文字。
func TestListVideosRejectsOverlongQuery(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}})

	ok := strings.Repeat("あ", maxQueryLength)
	if rec := do(t, handler, http.MethodGet, "/api/videos?query="+url.QueryEscape(ok)); rec.Code != http.StatusOK {
		t.Errorf("100 文字で status = %d, want 200", rec.Code)
	}

	tooLong := strings.Repeat("あ", maxQueryLength+1)
	rec := do(t, handler, http.MethodGet, "/api/videos?query="+url.QueryEscape(tooLong))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("101 文字で status = %d, want 400", rec.Code)
	}
	if got := decode[gen.Error](t, rec); got.Code != codeInvalidRequest {
		t.Errorf("code = %q, want %s", got.Code, codeInvalidRequest)
	}
}

// 検索とカーソルを併用できること。検索語はカーソルと一緒に渡し続ける。
func TestListVideosCombinesQueryAndCursor(t *testing.T) {
	library := &fakeLibrary{}
	handler := newTestServer(t, Options{Videos: library})

	target := "/api/videos?query=" + url.QueryEscape("旅行") + "&cursor=Y3Vyc29y&limit=2"
	if rec := do(t, handler, http.MethodGet, target); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	if library.lastQuery.Query != "旅行" {
		t.Errorf("検索語 = %q", library.lastQuery.Query)
	}
	if library.lastQuery.Cursor != "Y3Vyc29y" {
		t.Errorf("カーソル = %q", library.lastQuery.Cursor)
	}
	if library.lastQuery.Limit != 2 {
		t.Errorf("limit = %d, want 2", library.lastQuery.Limit)
	}
}
