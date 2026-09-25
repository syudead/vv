package httpapi

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// postJSON は POST/JSON 本文の要求を1つ投げて応答を返す。
func postJSON(t *testing.T, handler http.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	return rec
}

// タグを付与する。id での指定はそのまま AttachTagByID へ渡る。
func TestUpdateVideoTagsAttachByID(t *testing.T) {
	tags := &fakeTags{attachRef: domain.TagRef{ID: 2, Name: "旅行"}, attachApplied: 3}
	handler := newTestServer(t, Options{Tags: tags})

	rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[1,2,3],"action":"add","tag":{"id":2}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.VideoTagsResponse](t, rec)
	if got.Tag.Id != 2 || got.Tag.Name != "旅行" || got.Applied != 3 {
		t.Fatalf("response = %+v", got)
	}
	if tags.operation != "attach-by-id" || tags.lastID != 2 {
		t.Fatalf("operation = %q id = %d", tags.operation, tags.lastID)
	}
	if !reflect.DeepEqual(tags.lastVideoIDs, []int64{1, 2, 3}) {
		t.Fatalf("videoIDs = %v", tags.lastVideoIDs)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
}

// 名前での付与は名前を引いて作る側（AttachTagByName）に渡る。
func TestUpdateVideoTagsAttachByName(t *testing.T) {
	tags := &fakeTags{attachRef: domain.TagRef{ID: 9, Name: "新しいタグ"}, attachApplied: 1}
	handler := newTestServer(t, Options{Tags: tags})

	rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[1],"action":"add","tag":{"name":"新しいタグ"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if tags.operation != "attach-by-name" || tags.lastName != "新しいタグ" {
		t.Fatalf("operation = %q name = %q", tags.operation, tags.lastName)
	}
}

// 取り外しは id での指定だけを受け付ける。name を送ると 400 になる。
func TestUpdateVideoTagsRemoveByID(t *testing.T) {
	tags := &fakeTags{detachRef: domain.TagRef{ID: 2, Name: "旅行"}, detachApplied: 2}
	handler := newTestServer(t, Options{Tags: tags})

	rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[1,2],"action":"remove","tag":{"id":2}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.VideoTagsResponse](t, rec)
	if got.Applied != 2 || tags.operation != "detach" {
		t.Fatalf("response = %+v operation = %q", got, tags.operation)
	}

	rec = postJSON(t, handler, "/api/video-tags", `{"videoIds":[1],"action":"remove","tag":{"name":"旅行"}}`)
	assertErrorCode(t, rec, http.StatusBadRequest, codeInvalidRequest)
}

// tag は id と name のちょうど一方でなければ 400。
func TestUpdateVideoTagsRejectsInvalidTagInput(t *testing.T) {
	handler := newTestServer(t, Options{Tags: &fakeTags{}})

	cases := []string{
		`{"videoIds":[1],"action":"add","tag":{}}`,
		`{"videoIds":[1],"action":"add","tag":{"id":1,"name":"x"}}`,
	}
	for _, body := range cases {
		rec := postJSON(t, handler, "/api/video-tags", body)
		assertErrorCode(t, rec, http.StatusBadRequest, codeInvalidRequest)
	}
}

// videoIds は1件以上20000件以下。0件・20001件は400。
func TestUpdateVideoTagsRejectsOutOfRangeVideoIDs(t *testing.T) {
	handler := newTestServer(t, Options{Tags: &fakeTags{}})

	assertErrorCode(t, postJSON(t, handler, "/api/video-tags", `{"videoIds":[],"action":"add","tag":{"id":1}}`),
		http.StatusBadRequest, codeInvalidRequest)

	var ids strings.Builder
	for i := 1; i <= 20001; i++ {
		if i > 1 {
			ids.WriteByte(',')
		}
		ids.WriteString(strconv.Itoa(i))
	}
	body := `{"videoIds":[` + ids.String() + `],"action":"add","tag":{"id":1}}`
	assertErrorCode(t, postJSON(t, handler, "/api/video-tags", body), http.StatusBadRequest, codeInvalidRequest)
}

// 存在しない tag.id は 404 tag_not_found になる。
func TestUpdateVideoTagsTagNotFound(t *testing.T) {
	tags := &fakeTags{err: domain.ErrTagNotFound}
	handler := newTestServer(t, Options{Tags: tags})

	rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[1],"action":"add","tag":{"id":99}}`)
	assertErrorCode(t, rec, http.StatusNotFound, codeTagNotFound)
}

// 要約は total・items を返す。
func TestSummarizeVideoTags(t *testing.T) {
	tags := &fakeTags{summary: domain.TagSummary{
		Total: 3,
		Items: []domain.TagSummaryItem{
			{Tag: domain.TagRef{ID: 1, Name: "旅行"}, Count: 2, ManualCount: 1},
			{Tag: domain.TagRef{ID: 2, Name: "観光"}, Count: 3, ManualCount: 3},
		},
	}}
	handler := newTestServer(t, Options{Tags: tags})

	rec := postJSON(t, handler, "/api/video-tags/summary", `{"videoIds":[1,2,3]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.VideoTagsSummary](t, rec)
	if got.Total != 3 || len(got.Items) != 2 {
		t.Fatalf("summary = %+v", got)
	}
	if got.Items[0].Tag.Name != "旅行" || got.Items[0].Count != 2 || got.Items[0].ManualCount != 1 {
		t.Errorf("items[0] = %+v", got.Items[0])
	}
	if !reflect.DeepEqual(tags.lastVideoIDs, []int64{1, 2, 3}) {
		t.Errorf("videoIDs = %v", tags.lastVideoIDs)
	}
}

// videoIds が空なら 400。
func TestSummarizeVideoTagsRejectsEmptyVideoIDs(t *testing.T) {
	handler := newTestServer(t, Options{Tags: &fakeTags{}})
	rec := postJSON(t, handler, "/api/video-tags/summary", `{"videoIds":[]}`)
	assertErrorCode(t, rec, http.StatusBadRequest, codeInvalidRequest)
}

// GET /api/videos/ids は同じ条件の全件の id を返し、missingTagIds も渡す。
func TestListVideoIds(t *testing.T) {
	library := &fakeLibrary{ids: []int64{5, 1, 9}, missingTagIDs: []int64{7}}
	handler := newTestServer(t, Options{Videos: library})

	rec := do(t, handler, http.MethodGet, "/api/videos/ids?query=旅行&watch=unwatched&playable=true&tag=3&tag=7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.VideoIdsResponse](t, rec)
	if !reflect.DeepEqual(got.Ids, []int64{5, 1, 9}) {
		t.Errorf("ids = %v", got.Ids)
	}
	if got.MissingTagIds == nil || !reflect.DeepEqual(*got.MissingTagIds, []int64{7}) {
		t.Errorf("missingTagIds = %v", got.MissingTagIds)
	}

	want := domain.VideoQuery{
		Query: "旅行", Watch: domain.WatchUnwatched, PlayableOnly: true, TagIDs: []int64{3, 7},
	}
	if !reflect.DeepEqual(library.lastIDsQuery, want) {
		t.Errorf("query = %+v, want %+v", library.lastIDsQuery, want)
	}
}

// GET /api/videos/ids が GET /api/videos/{id} に取られないことは
// openapi_routes_test.go の TestVideoIdsRouteNotShadowedByVideoIDRoute で見る
// （二重に持たない）。

// 17個以上の tag は400。
func TestListVideoIdsRejectsTooManyTags(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}})
	target := "/api/videos/ids?" + strings.TrimSuffix(strings.Repeat("tag=1&", 17), "&")
	assertErrorCode(t, do(t, handler, http.MethodGet, target), http.StatusBadRequest, codeInvalidRequest)
}
