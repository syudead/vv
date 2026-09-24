package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakeTags はタグ管理の経路をテストするための決め打ちの実装である。
type fakeTags struct {
	tags []domain.Tag

	operation string
	lastID    int64
	lastName  string
	lastTag   *int64 // MergeTagのsourceID・AddSynonymのmergeTagID
	err       error
}

func (f *fakeTags) ListTags(context.Context) ([]domain.Tag, error) {
	return f.tags, f.err
}

func (f *fakeTags) CreateTag(_ context.Context, name string) (domain.Tag, error) {
	f.operation, f.lastName = "create", name
	if f.err != nil {
		return domain.Tag{}, f.err
	}
	return domain.Tag{ID: 1, Name: name, Synonyms: []string{}, VideoCount: 0}, nil
}

func (f *fakeTags) RenameTag(_ context.Context, id int64, name string) (domain.Tag, error) {
	f.operation, f.lastID, f.lastName = "rename", id, name
	if f.err != nil {
		return domain.Tag{}, f.err
	}
	return domain.Tag{ID: id, Name: name, Synonyms: []string{}, VideoCount: 0}, nil
}

func (f *fakeTags) DeleteTag(_ context.Context, id int64) error {
	f.operation, f.lastID = "delete", id
	return f.err
}

func (f *fakeTags) MergeTag(_ context.Context, targetID, sourceID int64) (domain.Tag, error) {
	f.operation, f.lastID, f.lastTag = "merge", targetID, &sourceID
	if f.err != nil {
		return domain.Tag{}, f.err
	}
	return domain.Tag{ID: targetID, Name: "target", Synonyms: []string{}, VideoCount: 0}, nil
}

func (f *fakeTags) AddSynonym(_ context.Context, tagID int64, name string, mergeTagID *int64) (domain.Tag, error) {
	f.operation, f.lastID, f.lastName, f.lastTag = "add-synonym", tagID, name, mergeTagID
	if f.err != nil {
		return domain.Tag{}, f.err
	}
	return domain.Tag{ID: tagID, Name: "target", Synonyms: []string{name}, VideoCount: 0}, nil
}

func (f *fakeTags) RemoveSynonym(_ context.Context, tagID int64, name string) error {
	f.operation, f.lastID, f.lastName = "remove-synonym", tagID, name
	return f.err
}

func TestListTagsReturnsItems(t *testing.T) {
	fake := &fakeTags{tags: []domain.Tag{
		{ID: 1, Name: "旅行", Synonyms: []string{"Travel"}, VideoCount: 3},
	}}
	rec := do(t, newTestServer(t, Options{Tags: fake}), http.MethodGet, "/api/tags")
	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	got := decode[gen.TagList](t, rec)
	if len(got.Items) != 1 || got.Items[0].Name != "旅行" || got.Items[0].VideoCount != 3 {
		t.Fatalf("items = %#v", got.Items)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatal("tag list is cacheable")
	}
}

func TestListTagsReturnsEmptySynonymsArray(t *testing.T) {
	fake := &fakeTags{tags: []domain.Tag{{ID: 1, Name: "旅行", Synonyms: nil, VideoCount: 0}}}
	rec := do(t, newTestServer(t, Options{Tags: fake}), http.MethodGet, "/api/tags")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"synonyms":[]`) {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
}

func TestCreateTagReturnsCreated(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", `{"name":"旅行"}`)
	if rec.Code != http.StatusCreated || fake.operation != "create" || fake.lastName != "旅行" {
		t.Fatalf("response = %d operation=%s name=%s", rec.Code, fake.operation, fake.lastName)
	}
	got := decode[gen.Tag](t, rec)
	if got.Name != "旅行" {
		t.Fatalf("tag = %#v", got)
	}
}

func TestCreateTagNameTakenReturnsConflict(t *testing.T) {
	fake := &fakeTags{err: &domain.TagNameConflict{Tag: domain.TagRef{ID: 5, Name: "旅行"}}}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", `{"name":"旅行"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	got := decode[gen.Error](t, rec)
	if got.Code != codeTagNameTaken {
		t.Fatalf("code = %s", got.Code)
	}
}

func TestTagMutationsReturnNotFoundForMissingTag(t *testing.T) {
	tests := []struct {
		name, method, target, body string
	}{
		{"rename", http.MethodPatch, "/api/tags/999", `{"name":"新しい名前"}`},
		{"merge", http.MethodPost, "/api/tags/999/merge", `{"sourceId":1}`},
		{"delete", http.MethodDelete, "/api/tags/999", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeTags{err: domain.ErrTagNotFound}
			handler := newTestServer(t, Options{Tags: fake})
			var rec *httptest.ResponseRecorder
			if tc.body == "" {
				rec = request(t, handler, tc.method, tc.target, "", nil)
			} else {
				rec = jsonRequest(t, handler, tc.method, tc.target, tc.body)
			}
			if rec.Code != http.StatusNotFound {
				t.Fatalf("response = %d %s", rec.Code, rec.Body)
			}
			got := decode[gen.Error](t, rec)
			if got.Code != codeTagNotFound {
				t.Fatalf("code = %s", got.Code)
			}
		})
	}
}

func TestMergeTagSameIDIsInvalidRequest(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/merge", `{"sourceId":3}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	got := decode[gen.Error](t, rec)
	if got.Code != codeInvalidRequest {
		t.Fatalf("code = %s", got.Code)
	}
	if fake.operation != "" {
		t.Fatal("MergeTag was called for sourceId == id")
	}
}

func TestMergeTagCallsStoreForDifferentIDs(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/merge", `{"sourceId":5}`)
	if rec.Code != http.StatusOK || fake.operation != "merge" || fake.lastID != 3 || fake.lastTag == nil || *fake.lastTag != 5 {
		t.Fatalf("response = %d operation=%s id=%d source=%v", rec.Code, fake.operation, fake.lastID, fake.lastTag)
	}
}

func TestAddTagSynonymWithoutMergeTagIDReturnsMergeRequired(t *testing.T) {
	fake := &fakeTags{err: &domain.TagMergeRequired{Tag: domain.TagRef{ID: 7, Name: "旧名"}}}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/synonyms", `{"name":"旧名"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	got := decode[gen.Error](t, rec)
	if got.Code != codeTagMergeRequired {
		t.Fatalf("code = %s", got.Code)
	}
	if fake.lastTag != nil {
		t.Fatalf("mergeTagId should be nil, got %v", *fake.lastTag)
	}
}

func TestAddTagSynonymWithWrongMergeTagIDReturnsMergeRequired(t *testing.T) {
	fake := &fakeTags{err: &domain.TagMergeRequired{Tag: domain.TagRef{ID: 7, Name: "旧名"}}}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/synonyms",
		`{"name":"旧名","mergeTagId":99}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	if fake.lastTag == nil || *fake.lastTag != 99 {
		t.Fatalf("mergeTagId = %v", fake.lastTag)
	}
}

func TestAddTagSynonymWithMatchingMergeTagIDSucceeds(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/synonyms",
		`{"name":"旧名","mergeTagId":7}`)
	if rec.Code != http.StatusOK || fake.lastTag == nil || *fake.lastTag != 7 {
		t.Fatalf("response = %d source=%v", rec.Code, fake.lastTag)
	}
}

func TestRemoveTagSynonymNormalizesName(t *testing.T) {
	fake := &fakeTags{}
	handler := newTestServer(t, Options{Tags: fake})
	rec := request(t, handler, http.MethodDelete, "/api/tags/3/synonyms?name=%20%E6%97%A7%E5%90%8D%20", "", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	if fake.lastName != "旧名" {
		t.Fatalf("name passed to store = %q, want normalized value without surrounding spaces", fake.lastName)
	}
}

func TestRemoveTagSynonymMissingTagReturnsNotFound(t *testing.T) {
	fake := &fakeTags{err: domain.ErrTagNotFound}
	handler := newTestServer(t, Options{Tags: fake})
	rec := request(t, handler, http.MethodDelete, "/api/tags/999/synonyms?name=x", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
}

func TestTagMutationsRequireSameOrigin(t *testing.T) {
	fake := &fakeTags{}
	handler := newTestServer(t, Options{Tags: fake})
	rec := request(t, handler, http.MethodPost, "/api/tags", `{"name":"旅行"}`, map[string]string{
		"Content-Type": "application/json", "Origin": "https://attacker.example",
	})
	if rec.Code != http.StatusForbidden || fake.operation != "" {
		t.Fatalf("response = %d operation=%s", rec.Code, fake.operation)
	}
}

func TestTagInternalErrorWhenNotConfigured(t *testing.T) {
	handler := newTestServer(t, Options{})
	rec := do(t, handler, http.MethodGet, "/api/tags")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
}

func TestTagUnexpectedErrorReturnsInternal(t *testing.T) {
	fake := &fakeTags{err: errors.New("database unavailable")}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", `{"name":"旅行"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
}
