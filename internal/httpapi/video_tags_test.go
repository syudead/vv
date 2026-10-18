package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func postJSON(t *testing.T, handler http.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	return rec
}

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

func TestUpdateVideoTagsRejectsOutOfRangeVideoIDs(t *testing.T) {
	handler := newTestServer(t, Options{Tags: &fakeTags{}})

	tooMany := wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxVideoTagsIDs,
	}
	rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[],"action":"add","tag":{"id":1}}`)
	assertErrorBody(t, "0件", rec.Code, rec.Body.Bytes(), tooMany)

	var ids strings.Builder
	for i := 1; i <= 20001; i++ {
		if i > 1 {
			ids.WriteByte(',')
		}
		ids.WriteString(strconv.Itoa(i))
	}
	body := `{"videoIds":[` + ids.String() + `],"action":"add","tag":{"id":1}}`
	rec = postJSON(t, handler, "/api/video-tags", body)
	assertErrorBody(t, "20001件", rec.Code, rec.Body.Bytes(), tooMany)
}

func TestUpdateVideoTagsInvalidTagNameReason(t *testing.T) {
	cases := []struct {
		problem domain.TagNameProblem
		want    wantError
	}{
		{domain.TagNameEmpty, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTagNameEmpty}},
		{domain.TagNameControlCharacters, wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTagNameControlCharacters,
		}},
		{domain.TagNameTooLong, wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
			reason: reasonTagNameTooLong, limit: domain.TagNameMaxLength,
		}},
	}
	for _, tc := range cases {
		handler := newTestServer(t, Options{Tags: &fakeTags{err: &domain.InvalidTagNameError{Problem: tc.problem}}})
		rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[1],"action":"add","tag":{"name":"x"}}`)
		assertErrorBody(t, string(tc.want.reason), rec.Code, rec.Body.Bytes(), tc.want)
	}
}

func TestUpdateVideoTagsTagNotFound(t *testing.T) {
	tags := &fakeTags{err: domain.ErrTagNotFound}
	handler := newTestServer(t, Options{Tags: tags})

	rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[1],"action":"add","tag":{"id":99}}`)
	assertErrorCode(t, rec, http.StatusNotFound, codeTagNotFound)
}

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

func TestSummarizeVideoTagsRejectsEmptyVideoIDs(t *testing.T) {
	handler := newTestServer(t, Options{Tags: &fakeTags{}})
	rec := postJSON(t, handler, "/api/video-tags/summary", `{"videoIds":[]}`)
	assertErrorBody(t, "要約の 0件", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxVideoTagsIDs,
	})
}

// memberFolders resolves every requested smart folder to members and records the requested IDs.
type memberFolders struct {
	failingSmartFolders
	members   []int64
	requested []int64
}

func (f *memberFolders) SmartFolderVideoIDs(_ context.Context, ids []int64) ([]int64, error) {
	f.requested = ids
	return f.members, nil
}

func TestUpdateVideoTagsAppliesToSmartFolderMembers(t *testing.T) {
	tags := &fakeTags{attachRef: domain.TagRef{ID: 2, Name: "旅行"}, attachApplied: 3}
	folders := &memberFolders{members: []int64{3, 4}}
	handler := newTestServer(t, Options{Tags: tags, SmartFolders: folders})

	rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[1,3],"smartFolderIds":[7],"action":"add","tag":{"id":2}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.VideoTagsResponse](t, rec); got.Applied != 3 {
		t.Errorf("applied = %d, want 3", got.Applied)
	}
	if !reflect.DeepEqual(folders.requested, []int64{7}) {
		t.Errorf("smart folders = %v, want [7]", folders.requested)
	}
	if !reflect.DeepEqual(tags.lastVideoIDs, []int64{1, 3, 4}) {
		t.Errorf("videoIDs = %v, want [1 3 4]", tags.lastVideoIDs)
	}
}

// The limit applies to distinct videos after resolution, not to the request's array lengths.
func TestUpdateVideoTagsLimitCountsSmartFolderMembers(t *testing.T) {
	members := make([]int64, maxVideoTagsIDs)
	for i := range members {
		members[i] = int64(i + 2)
	}
	tags := &fakeTags{}
	handler := newTestServer(t, Options{Tags: tags, SmartFolders: &memberFolders{members: members}})

	rec := postJSON(t, handler, "/api/video-tags", `{"videoIds":[1],"smartFolderIds":[7],"action":"add","tag":{"id":2}}`)
	assertErrorBody(t, "解決後 20001件", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxVideoTagsIDs,
	})
	if tags.operation != "" {
		t.Errorf("over-limit request reached the store: %q", tags.operation)
	}
}

func TestSummarizeVideoTagsResolvesSmartFolders(t *testing.T) {
	tags := &fakeTags{}
	handler := newTestServer(t, Options{Tags: tags, SmartFolders: &memberFolders{members: []int64{5, 6}}})

	rec := postJSON(t, handler, "/api/video-tags/summary", `{"videoIds":[],"smartFolderIds":[7]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if !reflect.DeepEqual(tags.lastVideoIDs, []int64{5, 6}) {
		t.Errorf("videoIDs = %v, want [5 6]", tags.lastVideoIDs)
	}
}
