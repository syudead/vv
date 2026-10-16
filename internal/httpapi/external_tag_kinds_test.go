package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// 外部連携 API と MCP のタグの種類（specs/051-tag-kinds/contracts/external-api.md）を、本物の保存先と
// Bearer の境界で確かめる。

// postExternalAs は外部連携 API に body を POST し、200 なら本文を T に読む。200 でなければ止める。
func postExternalAs[T any](t *testing.T, f mcpFixture, path string, body any) T {
	t.Helper()
	res := f.postExternal(t, path, body)
	if res.status != http.StatusOK {
		t.Fatalf("POST /api/v1%s: status = %d: %s", path, res.status, res.body)
	}
	var out T
	if err := json.Unmarshal(res.body, &out); err != nil {
		t.Fatalf("POST /api/v1%s: %v: %s", path, err, res.body)
	}
	return out
}

// postExternalRec は外部連携 API に body を POST し、応答を assertExternalError に渡せる形で返す。
func (f mcpFixture) postExternalRec(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	res := f.postExternal(t, path, body)
	rec := httptest.NewRecorder()
	rec.Code = res.status
	rec.Body.Write(res.body)
	return rec
}

// 外部連携 API で種類を作り、batchTags の setKind でタグに付けると、画面の GET /api/tags からその種類が
// 読め（受け入れ条件 4）、GET /api/v1/tags の kind で絞り込める。名前の変更・重複・削除は画面の経路と
// 同じ状態と reason で、消した種類のタグは種類なしで残る。
func TestExternalTagKindsRoundTripWithScreen(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	genre := postExternalAs[extgen.TagKind](t, f, "/tag-kinds", map[string]any{"name": "ジャンル"})
	if genre.Name != "ジャンル" || genre.TagCount != 0 || genre.CreatedAt.IsZero() {
		t.Errorf("createTagKind = %+v", genre)
	}
	rec := f.postExternalRec(t, "/tag-kinds", map[string]any{"name": "ジャンル"})
	assertExternalError(t, "重複", rec,
		wantExternalError{http.StatusConflict, extgen.ErrorCodeConflict, extgen.TagKindNameTaken, -1, 0})
	if conflict := decode[extgen.Error](t, rec); conflict.TagName == nil || *conflict.TagName != "ジャンル" {
		t.Errorf("重複の tagName = %v", conflict.TagName)
	}
	assertExternalError(t, "空の名前", f.postExternalRec(t, "/tag-kinds", map[string]any{"name": " "}),
		wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.TagNameEmpty, -1, 0})

	tagged, plain := f.createTag(t, "SF"), f.createTag(t, "ほか")
	batch := postExternalAs[extgen.TagBatchResponse](t, f, "/tags/batch",
		map[string]any{"action": "setKind", "kindId": genre.Id, "ids": []int64{tagged, 9999}})
	if !slices.Equal(batch.AppliedIds, []int64{tagged}) || !slices.Equal(batch.NotFoundIds, []int64{9999}) {
		t.Errorf("setKind = %+v", batch)
	}

	// 画面の一覧が、外部連携 API で付けた種類を返す（受け入れ条件 4）。
	var screen struct {
		Items []struct {
			ID   int64
			Kind *struct {
				ID   int64
				Name string
			}
		}
	}
	if err := json.Unmarshal(f.screenTags(t), &screen); err != nil {
		t.Fatal(err)
	}
	for _, item := range screen.Items {
		hasGenre := item.Kind != nil && item.Kind.ID == genre.Id && item.Kind.Name == "ジャンル"
		if hasGenre != (item.ID == tagged) || (item.ID == plain && item.Kind != nil) {
			t.Errorf("画面のタグ %d の kind = %+v", item.ID, item.Kind)
		}
	}

	id := strconv.FormatInt(genre.Id, 10)
	if got := f.getTags(t, "?kind="+id).Items; len(got) != 1 || got[0].Id != tagged || got[0].Kind == nil ||
		got[0].Kind.Name != "ジャンル" {
		t.Errorf("kind=%s = %+v", id, got)
	}
	if got := f.getTags(t, "?kind=none").Items; len(got) != 1 || got[0].Id != plain || got[0].Kind != nil {
		t.Errorf("kind=none = %+v", got)
	}
	for _, kind := range []string{"0", "abc", "-1"} {
		rec = f.env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags?kind=" + kind, header: bearer(f.secret)})
		assertExternalError(t, "kind="+kind, rec, wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", -1, 0})
	}

	renamed := postExternalAs[extgen.TagKind](t, f, "/tag-kinds/update", map[string]any{"id": genre.Id, "name": "分類"})
	if renamed.Id != genre.Id || renamed.Name != "分類" || renamed.TagCount != 1 {
		t.Errorf("renameTagKind = %+v", renamed)
	}
	postExternalAs[extgen.TagKind](t, f, "/tag-kinds", map[string]any{"name": "作品"})
	// 移行が入れる既定の種類の後に、作った順（id の順）で並ぶ。
	if names := kindNames(f.listTagKinds(t)); len(names) < 2 || !slices.Equal(names[len(names)-2:], []string{"分類", "作品"}) {
		t.Errorf("listTagKinds = %v", names)
	}

	deleted := postExternalAs[extgen.TagKindDeleteResponse](t, f, "/tag-kinds/delete",
		map[string]any{"ids": []int64{genre.Id, 9999, genre.Id}})
	if !slices.Equal(deleted.DeletedIds, []int64{genre.Id}) || !slices.Equal(deleted.NotFoundIds, []int64{9999}) {
		t.Errorf("deleteTagKinds = %+v", deleted)
	}
	if got := f.getTags(t, "?kind=none").Items; len(got) != 2 {
		t.Errorf("種類を消した後の kind=none = %+v, want 2 つのタグ", got)
	}
}

// kindNames は種類の一覧の名前を順に返す。
func kindNames(list extgen.TagKindList) []string {
	names := make([]string, 0, len(list.Items))
	for _, kind := range list.Items {
		names = append(names, kind.Name)
	}
	return names
}

// listTagKinds は GET /api/v1/tag-kinds の本文を返す。
func (f mcpFixture) listTagKinds(t *testing.T) extgen.TagKindList {
	t.Helper()
	rec := f.env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tag-kinds", header: bearer(f.secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/tag-kinds: status = %d: %s", rec.Code, rec.Body)
	}
	return decode[extgen.TagKindList](t, rec)
}

// 断る要求は画面の経路と同じ状態で、何も変えない。
func TestExternalTagKindsRejectInvalidRequests(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	tag := f.createTag(t, "SF")
	notFound := wantExternalError{http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.TagKindNotFound, -1, 0}
	invalid := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", -1, 0}
	cases := []struct {
		name string
		path string
		body any
		want wantExternalError
	}{
		{"無い種類の setKind", "/tags/batch", map[string]any{"action": "setKind", "kindId": 999, "ids": []int64{tag}}, notFound},
		{"kindId の無い setKind", "/tags/batch", map[string]any{"action": "setKind", "ids": []int64{tag}}, invalid},
		{"kindId を付けた clearKind", "/tags/batch", map[string]any{"action": "clearKind", "kindId": 1, "ids": []int64{tag}}, invalid},
		{"kindId が null の delete", "/tags/batch", map[string]any{"action": "delete", "kindId": nil, "ids": []int64{tag}}, invalid},
		{"無い種類の名前の変更", "/tag-kinds/update", map[string]any{"id": 999, "name": "x"}, notFound},
		{"id の無い名前の変更", "/tag-kinds/update", map[string]any{"name": "x"}, invalid},
		{"name の無い名前の変更", "/tag-kinds/update", map[string]any{"id": 999}, invalid},
		{"name が null の名前の変更", "/tag-kinds/update", map[string]any{"id": 999, "name": nil}, invalid},
		{"name の無い作成", "/tag-kinds", map[string]any{}, invalid},
		{"name が null の作成", "/tag-kinds", map[string]any{"name": nil}, invalid},
		{"空の ids の削除", "/tag-kinds/delete", map[string]any{"ids": []int64{}}, invalid},
		{"20,001 件の削除", "/tag-kinds/delete", map[string]any{"ids": make([]int64, domain.MaxTagBatch+1)}, invalid},
	}
	for _, tc := range cases {
		assertExternalError(t, tc.name, f.postExternalRec(t, tc.path, tc.body), tc.want)
	}
	if got := f.getTags(t, "?kind=none").Items; len(got) != 1 || got[0].Id != tag {
		t.Errorf("断った後のタグ = %+v, want 種類なしのまま", got)
	}
}

// MCP の 4 つの種類のツールと、batch_tags の setKind・clearKind、list_tags の kind が外部連携 API と同じ
// 操作を呼ぶ。
func TestMCPTagKindTools(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	tag := f.createTag(t, "SF")
	session := f.connect(t)

	var kind extgen.TagKind
	if callTool(t, session, "create_tag_kind", map[string]any{"name": "ジャンル"}, &kind) || kind.Name != "ジャンル" {
		t.Fatalf("create_tag_kind = %+v", kind)
	}
	var batch extgen.TagBatchResponse
	if callTool(t, session, "batch_tags", map[string]any{"action": "setKind", "kindId": kind.Id, "ids": []int64{tag}}, &batch) ||
		!slices.Equal(batch.AppliedIds, []int64{tag}) {
		t.Fatalf("batch_tags setKind = %+v", batch)
	}
	var tags extgen.TagList
	if callTool(t, session, "list_tags", map[string]any{"kind": strconv.FormatInt(kind.Id, 10)}, &tags) ||
		len(tags.Items) != 1 || tags.Items[0].Kind == nil || tags.Items[0].Kind.Id != kind.Id {
		t.Errorf("list_tags kind = %+v", tags)
	}
	var renamed extgen.TagKind
	if callTool(t, session, "rename_tag_kind", map[string]any{"id": kind.Id, "name": "分類"}, &renamed) ||
		renamed.Name != "分類" || renamed.TagCount != 1 {
		t.Errorf("rename_tag_kind = %+v", renamed)
	}
	var kinds extgen.TagKindList
	if callTool(t, session, "list_tag_kinds", map[string]any{}, &kinds) ||
		!slices.Equal(kindNames(kinds), kindNames(f.listTagKinds(t))) || !slices.Contains(kindNames(kinds), "分類") {
		t.Errorf("list_tag_kinds = %+v", kinds)
	}
	var missing extgen.Error
	if !callTool(t, session, "batch_tags", map[string]any{"action": "setKind", "kindId": 999, "ids": []int64{tag}}, &missing) ||
		missing.Reason == nil || *missing.Reason != extgen.TagKindNotFound {
		t.Errorf("無い種類の setKind = %+v", missing)
	}
	if callTool(t, session, "batch_tags", map[string]any{"action": "clearKind", "ids": []int64{tag}}, &batch) ||
		!slices.Equal(batch.AppliedIds, []int64{tag}) {
		t.Errorf("batch_tags clearKind = %+v", batch)
	}
	var deleted extgen.TagKindDeleteResponse
	if callTool(t, session, "delete_tag_kinds", map[string]any{"ids": []int64{kind.Id, 999}}, &deleted) ||
		!slices.Equal(deleted.DeletedIds, []int64{kind.Id}) || !slices.Equal(deleted.NotFoundIds, []int64{999}) {
		t.Errorf("delete_tag_kinds = %+v", deleted)
	}
}

// failingKindDeletes は failAt の id の削除だけを failure で失敗させ、消した id を deleted に残す。
type failingKindDeletes struct {
	*fakeTags
	failAt  int64
	failure error
	deleted []int64
}

func (f *failingKindDeletes) DeleteTagKind(_ context.Context, id int64) error {
	if id == f.failAt {
		return f.failure
	}
	f.deleted = append(f.deleted, id)
	return nil
}

// newExternalKindsEnv は tags を保存先にした外部連携 API と、その Bearer の値を返す。tags が nil なら
// 保存先の無い構成になる。
func newExternalKindsEnv(t *testing.T, tags Tags) (*authEnv, map[string]string) {
	t.Helper()
	env := newAuthEnv(t, t.TempDir(), Options{Tags: tags})
	owner := env.setup()
	return env, bearer(env.createAPIToken(owner, "agent").Secret)
}

// 保存先の想定外の失敗は 500 internal で、削除は失敗した id より前の id を消したまま止まる
// （contracts/external-api.md、deleteCredits と同じ）。
func TestExternalTagKindsStoreFailures(t *testing.T) {
	internal := wantExternalError{http.StatusInternalServerError, extgen.ErrorCodeInternal, "", -1, 0}
	broken := errors.New("disk I/O error")
	env, auth := newExternalKindsEnv(t, &fakeTags{err: broken})
	for _, req := range []authRequest{
		{method: http.MethodGet, target: "/api/v1/tag-kinds"},
		{method: http.MethodPost, target: "/api/v1/tag-kinds", body: `{"name":"Era"}`},
		{method: http.MethodPost, target: "/api/v1/tag-kinds/update", body: `{"id":1,"name":"Era"}`},
	} {
		req.header = auth
		assertExternalError(t, req.target, env.serve(req), internal)
	}

	fake := &failingKindDeletes{fakeTags: &fakeTags{}, failAt: 2, failure: broken}
	env, auth = newExternalKindsEnv(t, fake)
	rec := env.serve(authRequest{
		method: http.MethodPost, target: "/api/v1/tag-kinds/delete", body: `{"ids":[1,2,3]}`, header: auth,
	})
	assertExternalError(t, "途中の削除の失敗", rec, internal)
	if !slices.Equal(fake.deleted, []int64{1}) {
		t.Errorf("消した id = %v, want [1]（失敗の後の id は消さない）", fake.deleted)
	}
}

// 断る要求のうち、本物の保存先では作れないもの: 壊れた本文、*InvalidTagNameError を伴わない
// ErrInvalidTagName（Tags の約束はこの番兵だけを求める）、保存先の無い構成。
func TestExternalTagKindsRejectWithoutRealStore(t *testing.T) {
	invalid := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", -1, 0}
	fake := &fakeTags{}
	env, auth := newExternalKindsEnv(t, fake)
	rec := env.serve(authRequest{method: http.MethodPost, target: "/api/v1/tag-kinds/delete", body: `{"ids":`, header: auth})
	assertExternalError(t, "壊れた本文の削除", rec, invalid)
	if fake.operation != "" {
		t.Errorf("壊れた本文が保存層に届いた (%s)", fake.operation)
	}

	env, auth = newExternalKindsEnv(t, &fakeTags{err: domain.ErrInvalidTagName})
	rec = env.serve(authRequest{
		method: http.MethodPost, target: "/api/v1/tag-kinds/update", body: `{"id":1,"name":"Era"}`, header: auth,
	})
	assertExternalError(t, "番兵だけの名前の誤り", rec, invalid)

	internal := wantExternalError{http.StatusInternalServerError, extgen.ErrorCodeInternal, "", -1, 0}
	env, auth = newExternalKindsEnv(t, nil)
	for _, req := range []authRequest{
		{method: http.MethodGet, target: "/api/v1/tag-kinds"},
		{method: http.MethodPost, target: "/api/v1/tag-kinds/delete", body: `{"ids":[1]}`},
	} {
		req.header = auth
		assertExternalError(t, "保存先の無い "+req.target, env.serve(req), internal)
	}
}
