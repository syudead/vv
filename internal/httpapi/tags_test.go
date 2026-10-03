package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
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
	lastTag   *int64 // AddSynonymのmergeTagID
	err       error

	// #267: 付け外し・要約・一覧のタグ引きの決め打ち。
	lastVideoIDs  []int64
	attachRef     domain.TagRef
	attachApplied int
	detachRef     domain.TagRef
	detachApplied int
	summary       domain.TagSummary
	byContentKey  map[string][]domain.VideoTag

	// 031: 却下した名前の一覧の決め打ち。nil なら nil を返す。
	rejectedNames []string
	// 036: 却下した名前の一覧に渡ったカーソルと件数。
	lastCursor string
	lastLimit  int

	// 036: まとめての操作と確認の数の決め打ち。
	lastBatchAction  domain.TagBatchAction
	lastImpactAction domain.TagImpactAction
	lastIDs          []int64
	batchOutcome     domain.TagBatchOutcome
	impact           domain.TagImpact
	// mergeNotFound は MergeTags が NotFoundIDs として返す id。
	mergeNotFound []int64
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

func (f *fakeTags) MergeTags(_ context.Context, targetID int64, sourceIDs []int64) (domain.TagMergeOutcome, error) {
	f.operation, f.lastID, f.lastIDs = "merge", targetID, sourceIDs
	if f.err != nil {
		return domain.TagMergeOutcome{}, f.err
	}
	return domain.TagMergeOutcome{
		Tag:         domain.Tag{ID: targetID, Name: "target", Synonyms: []string{}, VideoCount: 0},
		NotFoundIDs: f.mergeNotFound,
	}, nil
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

func (f *fakeTags) ApplyVideoTags(_ context.Context, _ []domain.VideoRef, _ domain.VideoTagsAction, _ []string, _ bool) (domain.VideoTagsOutcome, error) {
	f.operation = "apply-video-tags"
	return domain.VideoTagsOutcome{}, f.err
}

func (f *fakeTags) ConfirmTag(_ context.Context, id int64) (domain.Tag, error) {
	f.operation, f.lastID = "confirm", id
	return domain.Tag{ID: id}, f.err
}

func (f *fakeTags) RejectTag(_ context.Context, id int64) (string, error) {
	f.operation, f.lastID = "reject", id
	return "", f.err
}

func (f *fakeTags) ListRejectedTagNames(_ context.Context, cursor string, limit int) (domain.RejectedTagNamePage, error) {
	f.operation, f.lastCursor, f.lastLimit = "list-rejected", cursor, limit
	return domain.RejectedTagNamePage{Items: f.rejectedNames, Total: len(f.rejectedNames)}, f.err
}

func (f *fakeTags) ForgetRejectedTagName(_ context.Context, name string) error {
	f.operation, f.lastName = "forget-rejected", name
	return f.err
}

func (f *fakeTags) BatchTags(_ context.Context, action domain.TagBatchAction, ids []int64) (domain.TagBatchOutcome, error) {
	f.operation, f.lastBatchAction, f.lastIDs = "batch", action, ids
	return f.batchOutcome, f.err
}

func (f *fakeTags) TagImpact(_ context.Context, action domain.TagImpactAction, ids []int64) (domain.TagImpact, error) {
	f.operation, f.lastImpactAction, f.lastIDs = "impact", action, ids
	return f.impact, f.err
}

func (f *fakeTags) Summary(_ context.Context, videoIDs []int64) (domain.TagSummary, error) {
	f.operation, f.lastVideoIDs = "summary", videoIDs
	if f.err != nil {
		return domain.TagSummary{}, f.err
	}
	return f.summary, nil
}

// manualTag は手で付けただけのタグ1件を作る。
func manualTag(id int64, name string) domain.VideoTag {
	return domain.VideoTag{TagRef: domain.TagRef{ID: id, Name: name}, Manual: true}
}

func (f *fakeTags) TagsByContentKeys(_ context.Context, contentKeys []string) (map[string][]domain.VideoTag, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string][]domain.VideoTag, len(contentKeys))
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
		assertErrorBody(t, "自分の元の名前", rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusConflict, code: gen.ErrorCodeTagNameTaken, reason: reasonNameIsTag, tagName: "Bar",
		})
	})

	t.Run("送った名前が衝突したタグのシノニム", func(t *testing.T) {
		fake := &fakeTags{err: &domain.TagNameConflict{Tag: domain.TagRef{ID: 5, Name: "Bar"}}}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", `{"name":"foo"}`)
		assertErrorBody(t, "シノニム", rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusConflict, code: gen.ErrorCodeTagNameTaken, reason: reasonNameIsSynonym, tagName: "Bar",
		})
		if got := decode[gen.Error](t, rec); !strings.Contains(got.Message, `"foo"`) || !strings.Contains(got.Message, `"Bar"`) {
			t.Errorf("message = %q, want 送った名前とタグの名前を含む", got.Message)
		}
	})

	t.Run("前後の空白を整えてから比較する", func(t *testing.T) {
		// nameConflict.Tag.Name は常に整えた形（domain.NormalizeTagName の結果）で
		// 保存されている。送った名前を整えずに比較すると、実際には自分の元の名前
		// なのにシノニムだと誤って報告してしまう。
		fake := &fakeTags{err: &domain.TagNameConflict{Tag: domain.TagRef{ID: 5, Name: "Bar"}}}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", `{"name":"  Bar  "}`)
		assertErrorBody(t, "自分の元の名前", rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusConflict, code: gen.ErrorCodeTagNameTaken, reason: reasonNameIsTag, tagName: "Bar",
		})
	})

	t.Run("自分自身の元の名前を自分のシノニムに登録", func(t *testing.T) {
		// AddTagSynonym(id=5, name="Bar") で id=5 自身の元の名前が "Bar" のとき、
		// store は自分自身を owner とする TagNameConflict を返す
		// （data-model.md §4）。この場合も「自分の名前」の文言になる。
		fake := &fakeTags{err: &domain.TagNameConflict{Tag: domain.TagRef{ID: 5, Name: "Bar"}}}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/5/synonyms",
			`{"name":"Bar"}`)
		assertErrorBody(t, "自分の元の名前", rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusConflict, code: gen.ErrorCodeTagNameTaken, reason: reasonNameIsTag, tagName: "Bar",
		})
	})
}

func TestInvalidTagNameMessageStatesTheReason(t *testing.T) {
	tests := []struct {
		name string
		body string
		want wantError
	}{
		{"空", `{"name":""}`, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTagNameEmpty}},
		{"制御文字", `{"name":"a\u0007b"}`, wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTagNameControlCharacters,
		}},
		{"101文字", `{"name":"` + strings.Repeat("あ", 101) + `"}`, wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
			reason: reasonTagNameTooLong, limit: domain.TagNameMaxLength,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeTags{}
			rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags", tc.body)
			assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), tc.want)
		})
	}
}

func TestTagMutationsReturnNotFoundForMissingTag(t *testing.T) {
	tests := []struct {
		name, method, target, body string
	}{
		{"rename", http.MethodPatch, "/api/tags/999", `{"name":"新しい名前"}`},
		{"merge", http.MethodPost, "/api/tags/999/merge", `{"sourceIds":[1]}`},
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

func TestMergeTagsRejectsInvalidSources(t *testing.T) {
	cases := []struct {
		name string
		body string
		want wantError
	}{
		{"sourceIds に統合先を含む", `{"sourceIds":[5,3]}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonMergeSameTag}},
		{"sourceIds が空", `{"sourceIds":[]}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
		{"sourceIds が 20001 件", `{"sourceIds":` + idsJSON(domain.MaxTagBatch+1) + `}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyTags, limit: domain.MaxTagBatch}},
		{"廃止した sourceId", `{"sourceId":5}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}},
	}
	for _, tc := range cases {
		fake := &fakeTags{}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/merge", tc.body)
		assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(), tc.want)
		if fake.operation != "" {
			t.Errorf("%s: 保存層に届いた (%s)", tc.name, fake.operation)
		}
	}
}

func TestMergeTagsReturnsTagAndNotFoundIDs(t *testing.T) {
	fake := &fakeTags{mergeNotFound: []int64{6}}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/merge", `{"sourceIds":[5,6]}`)
	if rec.Code != http.StatusOK || fake.operation != "merge" || fake.lastID != 3 || !slices.Equal(fake.lastIDs, []int64{5, 6}) {
		t.Fatalf("response = %d operation=%s id=%d sources=%v: %s", rec.Code, fake.operation, fake.lastID, fake.lastIDs, rec.Body)
	}
	got := decode[gen.TagMergeResponse](t, rec)
	if got.Tag.Id != 3 || !slices.Equal(got.NotFoundIds, []int64{6}) {
		t.Errorf("response = %+v", got)
	}
}

// 統合元がすべて無かったときも 200 で、tag は変わらない統合先、notFoundIds は全部。
// NotFoundIDs が nil でも応答は [] にする。
func TestMergeTagsWithOnlyMissingSourcesReturnsOK(t *testing.T) {
	for _, tc := range []struct {
		name     string
		notFound []int64
		want     []int64
	}{
		{"全部無い", []int64{5, 6}, []int64{5, 6}},
		{"nil", nil, []int64{}},
	} {
		fake := &fakeTags{mergeNotFound: tc.notFound}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/merge", `{"sourceIds":[5,6]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: response = %d %s", tc.name, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"notFoundIds":[`) {
			t.Errorf("%s: notFoundIds が配列でない: %s", tc.name, rec.Body)
		}
		got := decode[gen.TagMergeResponse](t, rec)
		if got.Tag.Id != 3 || !slices.Equal(got.NotFoundIds, tc.want) {
			t.Errorf("%s: response = %+v", tc.name, got)
		}
	}
}

func TestAddTagSynonymWithoutMergeTagIDReturnsMergeRequired(t *testing.T) {
	fake := &fakeTags{err: &domain.TagMergeRequired{Tag: domain.TagRef{ID: 7, Name: "旧名"}}}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tags/3/synonyms", `{"name":"旧名"}`)
	assertErrorBody(t, "統合の承諾が無い", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeTagMergeRequired, tagName: "旧名"})
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
