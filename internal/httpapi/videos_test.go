package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
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
	if first.SeekThumbnailUrl == nil {
		t.Fatal("seekThumbnailUrl が入っていない")
	}
	if want := "/api/videos/1/seek-thumbnail?v=abcdef012345"; *first.SeekThumbnailUrl != want {
		t.Errorf("seekThumbnailUrl = %q, want %q", *first.SeekThumbnailUrl, want)
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
	if item.SeekThumbnailUrl != nil {
		t.Errorf("seekThumbnailUrl = %v, want 省略", *item.SeekThumbnailUrl)
	}
	if item.Playable {
		t.Error("解析前なのに playable = true")
	}
}

func TestListVideosOmitsSeekThumbnailWithoutContentVersion(t *testing.T) {
	video := sampleVideo(8, "版なし")
	video.ContentKey = ""
	handler := newTestServer(t, Options{Videos: &fakeLibrary{
		page: domain.VideoPage{Items: []domain.Video{video}, Total: 1},
	}})

	item := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos")).Items[0]
	if item.SeekThumbnailUrl != nil {
		t.Errorf("seekThumbnailUrl = %v, want 省略", *item.SeekThumbnailUrl)
	}
}

func TestListVideosOmitsSeekThumbnailWithoutVideoStream(t *testing.T) {
	video := sampleVideo(8, "音声のみ")
	video.VideoCodec = ""
	handler := newTestServer(t, Options{Videos: &fakeLibrary{
		page: domain.VideoPage{Items: []domain.Video{video}, Total: 1},
	}})

	item := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos")).Items[0]
	if item.SeekThumbnailUrl != nil {
		t.Errorf("seekThumbnailUrl = %v, want 省略", *item.SeekThumbnailUrl)
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
	if rec := do(t, handler, http.MethodGet, "/api/videos?sort=sideways"); rec.Code != http.StatusBadRequest {
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

// 検索語・視聴状態・並び順は VideoQuery に渡り、応答は保存側が返した項目と
// total をそのまま返す。各項目には置き場所（folder）が載る。
func TestListVideosPassesFiltersAndReturnsFolders(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("fixture uses slash-separated absolute paths")
	}
	top := sampleVideo(1, "京都旅行 2024")
	top.Path = "/a/movies/京都旅行 2024.mp4"
	nested := sampleVideo(2, "京都 夜")
	nested.Path = "/b/movies/X/Y/京都 夜.mp4"
	outside := sampleVideo(3, "京都 外")
	outside.Path = "/elsewhere/京都 外.mp4"
	library := &fakeLibrary{page: domain.VideoPage{Items: []domain.Video{top, nested, outside}, Total: 3}}
	handler := newTestServer(t, Options{Videos: library, Folders: folderFixture()})

	target := "/api/videos?query=" + url.QueryEscape("京都 -2023") + "&watch=unwatched&sort=durationDesc"
	rec := do(t, handler, http.MethodGet, target)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	want := domain.VideoQuery{
		Query: "京都 -2023", Watch: domain.WatchUnwatched, Sort: domain.SortDurationDesc, Limit: domain.DefaultLimit,
	}
	// domain.VideoQuery に TagIDs（[]int64）が足された（#265）ので struct の
	// 比較演算子は使えない。reflect.DeepEqual で比較する。
	if !reflect.DeepEqual(library.lastQuery, want) {
		t.Errorf("query = %+v, want %+v", library.lastQuery, want)
	}

	page := decode[gen.VideoPage](t, rec)
	if page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("page = %+v", page)
	}
	if got := page.Items[0].Folder; got == nil || *got != (gen.VideoFolder{RootId: 3, Path: ""}) {
		t.Errorf("top folder = %+v", got)
	}
	if got := page.Items[1].Folder; got == nil || *got != (gen.VideoFolder{RootId: 7, Path: "X/Y"}) {
		t.Errorf("nested folder = %+v", got)
	}
	// 登録フォルダの外の所在は一覧に出ないはずだが、出ても folder は作らない。
	if got := page.Items[2].Folder; got != nil {
		t.Errorf("outside folder = %+v, want omitted", got)
	}

	rec = do(t, handler, http.MethodGet, "/api/videos?playable=true&sort=random&seed=0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if q := library.lastQuery; !q.PlayableOnly || q.Sort != domain.SortRandom || q.Seed != 0 || q.Watch != domain.WatchAll {
		t.Errorf("query = %+v", q)
	}
}

// 未知の watch・sort と範囲外の seed は 400 にする。
func TestListVideosRejectsUnknownFilters(t *testing.T) {
	library := &fakeLibrary{}
	handler := newTestServer(t, Options{Videos: library})
	for _, target := range []string{
		"/api/videos?watch=someday",
		"/api/videos?sort=sideways",
		"/api/videos?seed=-1",
		"/api/videos?seed=2147483648",
	} {
		rec := do(t, handler, http.MethodGet, target)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, rec.Code)
			continue
		}
		if got := decode[gen.Error](t, rec); got.Code != gen.ErrorCodeInvalidRequest {
			t.Errorf("%s: code = %q", target, got.Code)
		}
	}
}

// 登録フォルダの問い合わせ先が無ければ folder を省き、一覧は返す。
func TestListVideosOmitsFolderWithoutRoots(t *testing.T) {
	library := &fakeLibrary{page: domain.VideoPage{Items: []domain.Video{sampleVideo(1, "x")}, Total: 1}}
	handler := newTestServer(t, Options{Videos: library})
	rec := do(t, handler, http.MethodGet, "/api/videos")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if page := decode[gen.VideoPage](t, rec); len(page.Items) != 1 || page.Items[0].Folder != nil {
		t.Errorf("page = %+v", page)
	}
}

// #267: Video.tags は必須で、タグの無い動画では空配列（null ではない）。
func TestListVideosTagsIsEmptyArrayWithoutTags(t *testing.T) {
	video := sampleVideo(1, "タグなし")
	library := &fakeLibrary{page: domain.VideoPage{Items: []domain.Video{video}, Total: 1}}
	handler := newTestServer(t, Options{Videos: library, Tags: &fakeTags{}})

	rec := do(t, handler, http.MethodGet, "/api/videos")
	if !strings.Contains(rec.Body.String(), `"tags":[]`) {
		t.Fatalf("tags が空配列で出ていない: %s", rec.Body)
	}
	page := decode[gen.VideoPage](t, rec)
	if page.Items[0].Tags == nil || len(page.Items[0].Tags) != 0 {
		t.Errorf("tags = %+v, want 空配列", page.Items[0].Tags)
	}
}

// タグの経路（Tags）が設定されていなくても一覧は諦めない（progressFor と同じ扱い）。
func TestListVideosTagsOmittedWithoutTagsRoute(t *testing.T) {
	video := sampleVideo(1, "x")
	library := &fakeLibrary{page: domain.VideoPage{Items: []domain.Video{video}, Total: 1}}
	handler := newTestServer(t, Options{Videos: library})

	page := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos"))
	if len(page.Items[0].Tags) != 0 {
		t.Errorf("tags = %+v, want 空配列", page.Items[0].Tags)
	}
}

// TagStore が返したタグが一覧・詳細・関連動画・読み取りのやり直しの応答に載る。
func TestListVideosIncludesTagsFromTagStore(t *testing.T) {
	video := sampleVideo(1, "京都旅行")
	tags := &fakeTags{byContentKey: map[string][]domain.TagRef{
		video.ContentKey: {{ID: 2, Name: "旅行"}, {ID: 5, Name: "観光"}},
	}}
	library := &fakeLibrary{page: domain.VideoPage{Items: []domain.Video{video}, Total: 1}}
	handler := newTestServer(t, Options{Videos: library, Tags: tags})

	page := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos"))
	got := page.Items[0].Tags
	if len(got) != 2 || got[0].Id != 2 || got[0].Name != "旅行" || got[1].Id != 5 || got[1].Name != "観光" {
		t.Fatalf("tags = %+v", got)
	}
}

// tag は VideoQuery.TagIDs に渡り、17個以上は 400 にする。
func TestListVideosTagFilter(t *testing.T) {
	library := &fakeLibrary{}
	handler := newTestServer(t, Options{Videos: library})

	rec := do(t, handler, http.MethodGet, "/api/videos?tag=3&tag=8")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if want := []int64{3, 8}; !reflect.DeepEqual([]int64(library.lastQuery.TagIDs), want) {
		t.Errorf("tagIDs = %v, want %v", library.lastQuery.TagIDs, want)
	}

	many := "/api/videos?" + strings.Repeat("tag=1&", 17)
	rec = do(t, handler, http.MethodGet, strings.TrimSuffix(many, "&"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("17個: status = %d, want 400: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Error](t, rec).Code; got != codeInvalidRequest {
		t.Errorf("17個: code = %q", got)
	}
}

// 存在しなかった tag の id は VideoPage.missingTagIds に返る。1つも無ければ省く。
func TestListVideosMissingTagIDs(t *testing.T) {
	library := &fakeLibrary{page: domain.VideoPage{
		Items: []domain.Video{sampleVideo(1, "x")}, Total: 1, MissingTagIDs: []int64{9},
	}}
	handler := newTestServer(t, Options{Videos: library})

	page := decode[gen.VideoPage](t, do(t, handler, http.MethodGet, "/api/videos?tag=9"))
	if page.MissingTagIds == nil || !reflect.DeepEqual(*page.MissingTagIds, []int64{9}) {
		t.Fatalf("missingTagIds = %v, want [9]", page.MissingTagIds)
	}

	library.page.MissingTagIDs = nil
	rec := do(t, handler, http.MethodGet, "/api/videos")
	if strings.Contains(rec.Body.String(), "missingTagIds") {
		t.Fatalf("missingTagIds が省略されていない: %s", rec.Body)
	}
}
