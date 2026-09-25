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

	// #267: 付け外し・要約・一覧のタグ引きの決め打ち。
	lastVideoIDs  []int64
	attachRef     domain.TagRef
	attachApplied int
	detachRef     domain.TagRef
	detachApplied int
	summary       domain.TagSummary
	byContentKey  map[string][]domain.TagRef
}

func (f *fakeTags) ListTags(context.Context) ([]domain.Tag, error) {
	return f.tags, f.err
}

func (f *fakeTags) CreateTag(_ context.Context, name string) (domain.Tag, error) {
	f.operation, f.lastName = "create", name
	// 実際の store.CreateTag と同じく、名前の規則違反を最初に検査する。これで
	// 具体的な理由（空・制御文字・長さ超過）が message に出ることを確かめられる。
	if _, err := domain.NormalizeTagName(name); err != nil {
		return domain.Tag{}, err
	}
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

func (f *fakeTags) AttachTagByID(_ context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error) {
	f.operation, f.lastID, f.lastVideoIDs = "attach-by-id", tagID, videoIDs
	if f.err != nil {
		return domain.TagRef{}, 0, f.err
	}
	return f.attachRef, f.attachApplied, nil
}

func (f *fakeTags) AttachTagByName(_ context.Context, videoIDs []int64, name string) (domain.TagRef, int, error) {
	f.operation, f.lastName, f.lastVideoIDs = "attach-by-name", name, videoIDs
	if f.err != nil {
		return domain.TagRef{}, 0, f.err
	}
	return f.attachRef, f.attachApplied, nil
}

func (f *fakeTags) DetachTag(_ context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error) {
	f.operation, f.lastID, f.lastVideoIDs = "detach", tagID, videoIDs
	if f.err != nil {
		return domain.TagRef{}, 0, f.err
	}
	return f.detachRef, f.detachApplied, nil
}

func (f *fakeTags) Summary(_ context.Context, videoIDs []int64) (domain.TagSummary, error) {
	f.operation, f.lastVideoIDs = "summary", videoIDs
	if f.err != nil {
		return domain.TagSummary{}, f.err
	}
	return f.summary, nil
}

func (f *fakeTags) TagsByContentKeys(_ context.Context, contentKeys []string) (map[string][]domain.TagRef, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string][]domain.TagRef, len(contentKeys))
	for _, key := range contentKeys {
		if refs, ok := f.byContentKey[key]; ok {
			out[key] = refs
		}
	}
	return out, nil
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

// TestTagNameTakenMessageDistinguishesOwnNameFromSynonym は、tag_name_taken の
// message が「送った名前が衝突したタグ自身の元の名前か、そのシノニムか」を
// 言い分けることを確かめる（contracts/tags-api.md §2、親 Issue の Edge Case
// 「シノニム名の衝突」）。
func TestTagNameTakenMessageDistinguishesOwnNameFromSynonym(t *testing.T) {
	t.Run("送った名前が衝突したタグ自身の元の名前", func(t *testing.T) {
		fake := &fakeTags{err: &domain.TagNameConflict{Tag: domain.TagRef{ID: 5, Name: "Bar"}}}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", `{"name":"Bar"}`)
		got := decode[gen.Error](t, rec)
		if want := "「Bar」という名前のタグが既にあります"; got.Message != want {
			t.Fatalf("message = %q, want %q", got.Message, want)
		}
	})

	t.Run("送った名前が衝突したタグのシノニム", func(t *testing.T) {
		fake := &fakeTags{err: &domain.TagNameConflict{Tag: domain.TagRef{ID: 5, Name: "Bar"}}}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", `{"name":"foo"}`)
		got := decode[gen.Error](t, rec)
		if want := "「foo」は「Bar」のシノニムとして使われています"; got.Message != want {
			t.Fatalf("message = %q, want %q", got.Message, want)
		}
	})

	t.Run("前後の空白を整えてから比較する", func(t *testing.T) {
		// nameConflict.Tag.Name は常に整えた形（domain.NormalizeTagName の結果）で
		// 保存されている。送った名前を整えずに比較すると、実際には自分の元の名前
		// なのにシノニムだと誤って報告してしまう。
		fake := &fakeTags{err: &domain.TagNameConflict{Tag: domain.TagRef{ID: 5, Name: "Bar"}}}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", `{"name":"  Bar  "}`)
		got := decode[gen.Error](t, rec)
		if want := "「Bar」という名前のタグが既にあります"; got.Message != want {
			t.Fatalf("message = %q, want %q", got.Message, want)
		}
	})

	t.Run("自分自身の元の名前を自分のシノニムに登録", func(t *testing.T) {
		// AddTagSynonym(id=5, name="Bar") で id=5 自身の元の名前が "Bar" のとき、
		// store は自分自身を owner とする TagNameConflict を返す
		// （data-model.md §4）。この場合も「自分の名前」の文言になる。
		fake := &fakeTags{err: &domain.TagNameConflict{Tag: domain.TagRef{ID: 5, Name: "Bar"}}}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/5/synonyms",
			`{"name":"Bar"}`)
		got := decode[gen.Error](t, rec)
		if want := "「Bar」という名前のタグが既にあります"; got.Message != want {
			t.Fatalf("message = %q, want %q", got.Message, want)
		}
	})
}

func TestInvalidTagNameMessageStatesTheReason(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		contains string
	}{
		{"空", `{"name":""}`, "入力してください"},
		{"制御文字", `{"name":"a\u0007b"}`, "制御文字"},
		{"101文字", `{"name":"` + strings.Repeat("あ", 101) + `"}`, "文字以内"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeTags{}
			rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("response = %d %s", rec.Code, rec.Body)
			}
			got := decode[gen.Error](t, rec)
			if got.Code != codeInvalidRequest || !strings.Contains(got.Message, tc.contains) {
				t.Fatalf("error = %#v, want message containing %q", got, tc.contains)
			}
		})
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
