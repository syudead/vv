package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (f *fakeTags) ListTagKinds(context.Context) ([]domain.TagKind, error) {
	f.operation = "listKinds"
	return f.kinds, f.err
}

func (f *fakeTags) CreateTagKind(_ context.Context, name string) (domain.TagKind, error) {
	f.operation, f.lastName = "createKind", name
	// Validate the name first, matching the real store.
	if _, err := domain.NormalizeTagName(name); err != nil {
		return domain.TagKind{}, err
	}
	if f.err != nil {
		return domain.TagKind{}, f.err
	}
	return domain.TagKind{ID: 5, Name: name}, nil
}

func (f *fakeTags) RenameTagKind(_ context.Context, id int64, name string) (domain.TagKind, error) {
	f.operation, f.lastID, f.lastName = "renameKind", id, name
	if _, err := domain.NormalizeTagName(name); err != nil {
		return domain.TagKind{}, err
	}
	if f.err != nil {
		return domain.TagKind{}, f.err
	}
	return domain.TagKind{ID: id, Name: name, TagCount: 2}, nil
}

func (f *fakeTags) SetTagKindCardFirst(_ context.Context, id int64, cardFirst bool) (domain.TagKind, error) {
	f.operation, f.lastID, f.lastCardFirst = "cardFirstKind", id, cardFirst
	if f.err != nil {
		return domain.TagKind{}, f.err
	}
	return domain.TagKind{ID: id, Name: "Genre", CardFirst: cardFirst}, nil
}

func (f *fakeTags) DeleteTagKind(_ context.Context, id int64) error {
	f.operation, f.lastID = "deleteKind", id
	return f.err
}

func TestListTagKindsReturnsItems(t *testing.T) {
	created := time.Unix(1_700_000_000, 0).UTC()
	fake := &fakeTags{kinds: []domain.TagKind{
		{ID: 1, Name: "Genre", TagCount: 3, CreatedAt: created},
		{ID: 2, Name: "Work", TagCount: 0, CreatedAt: created},
	}}
	rec := do(t, newTestServer(t, Options{Tags: fake}), http.MethodGet, "/api/tag-kinds")
	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
	got := decode[gen.TagKindList](t, rec)
	if len(got.Items) != 2 || got.Items[0].Name != "Genre" || got.Items[0].TagCount != 3 ||
		!got.Items[0].CreatedAt.Equal(created) || got.Items[1].Id != 2 {
		t.Fatalf("items = %#v", got.Items)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatal("tag kind list is cacheable")
	}
}

func TestListTagKindsReturnsEmptyArray(t *testing.T) {
	rec := do(t, newTestServer(t, Options{Tags: &fakeTags{}}), http.MethodGet, "/api/tag-kinds")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("response = %d %s", rec.Code, rec.Body)
	}
}

func TestCreateTagKindReturnsCreated(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPost, "/api/tag-kinds", `{"name":"Era"}`)
	if rec.Code != http.StatusCreated || fake.operation != "createKind" || fake.lastName != "Era" {
		t.Fatalf("response = %d operation=%s name=%s: %s", rec.Code, fake.operation, fake.lastName, rec.Body)
	}
	if got := decode[gen.TagKind](t, rec); got.Id != 5 || got.Name != "Era" {
		t.Fatalf("kind = %#v", got)
	}
}

func TestRenameTagKindReturnsKind(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPatch, "/api/tag-kinds/3", `{"name":"Series"}`)
	if rec.Code != http.StatusOK || fake.operation != "renameKind" || fake.lastID != 3 || fake.lastName != "Series" {
		t.Fatalf("response = %d operation=%s id=%d: %s", rec.Code, fake.operation, fake.lastID, rec.Body)
	}
	if got := decode[gen.TagKind](t, rec); got.Id != 3 || got.Name != "Series" || got.TagCount != 2 {
		t.Fatalf("kind = %#v", got)
	}
}

func TestSetTagKindCardFirstReturnsKind(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPut, "/api/tag-kinds/3/card-first", `{"cardFirst":true}`)
	if rec.Code != http.StatusOK || fake.operation != "cardFirstKind" || fake.lastID != 3 || !fake.lastCardFirst {
		t.Fatalf("response = %d operation=%s id=%d cardFirst=%v: %s", rec.Code, fake.operation, fake.lastID, fake.lastCardFirst, rec.Body)
	}
	if got := decode[gen.TagKind](t, rec); got.Id != 3 || !got.CardFirst {
		t.Fatalf("kind = %#v", got)
	}
}

func TestSetTagKindCardFirstRejectsMissingValueAndKind(t *testing.T) {
	fake := &fakeTags{}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), http.MethodPut, "/api/tag-kinds/3/card-first", `{}`)
	assertErrorBody(t, "cardFirst なし", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	if fake.operation != "" {
		t.Errorf("operation = %q, want none", fake.operation)
	}
	rec = jsonRequest(t, newTestServer(t, Options{Tags: &fakeTags{err: domain.ErrTagKindNotFound}}),
		http.MethodPut, "/api/tag-kinds/999/card-first", `{"cardFirst":false}`)
	assertErrorBody(t, "無い id", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonTagKindNotFound,
	})
}

func TestDeleteTagKindReturnsNoContent(t *testing.T) {
	fake := &fakeTags{}
	rec := do(t, newTestServer(t, Options{Tags: fake}), http.MethodDelete, "/api/tag-kinds/4")
	if rec.Code != http.StatusNoContent || fake.operation != "deleteKind" || fake.lastID != 4 {
		t.Fatalf("response = %d operation=%s id=%d", rec.Code, fake.operation, fake.lastID)
	}
}

func TestTagKindErrorsFollowTheContract(t *testing.T) {
	taken := &domain.TagKindNameConflict{Kind: domain.TagKindRef{ID: 1, Name: "Genre"}}
	nameRules := []struct {
		name string
		body string
		want wantError
	}{
		{"空", `{"name":"  "}`, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTagNameEmpty}},
		{"制御文字", `{"name":"a\tb"}`, wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTagNameControlCharacters,
		}},
		{"101文字", `{"name":"` + strings.Repeat("あ", 101) + `"}`, wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
			reason: reasonTagNameTooLong, limit: domain.TagNameMaxLength,
		}},
	}
	for _, route := range []struct{ method, target string }{
		{http.MethodPost, "/api/tag-kinds"},
		{http.MethodPatch, "/api/tag-kinds/1"},
	} {
		for _, tc := range nameRules {
			label := route.method + " " + tc.name
			rec := jsonRequest(t, newTestServer(t, Options{Tags: &fakeTags{}}), route.method, route.target, tc.body)
			assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), tc.want)
		}
		label := route.method + " 名前の重複"
		rec := jsonRequest(t, newTestServer(t, Options{Tags: &fakeTags{err: taken}}), route.method, route.target, `{"name":"Genre"}`)
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonTagKindNameTaken, tagName: "Genre",
		})
	}

	notFound := wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonTagKindNotFound}
	rec := jsonRequest(t, newTestServer(t, Options{Tags: &fakeTags{err: domain.ErrTagKindNotFound}}),
		http.MethodPatch, "/api/tag-kinds/999", `{"name":"Era"}`)
	assertErrorBody(t, "PATCH 無い id", rec.Code, rec.Body.Bytes(), notFound)
	rec = do(t, newTestServer(t, Options{Tags: &fakeTags{err: domain.ErrTagKindNotFound}}),
		http.MethodDelete, "/api/tag-kinds/999")
	assertErrorBody(t, "DELETE 無い id", rec.Code, rec.Body.Bytes(), notFound)
}

func TestTagKindStoreFailuresAndMalformedBodies(t *testing.T) {
	internal := wantError{status: http.StatusInternalServerError, code: gen.ErrorCodeInternal}
	broken := errors.New("disk I/O error")
	for _, route := range []struct{ method, target, body string }{
		{http.MethodGet, "/api/tag-kinds", ""},
		{http.MethodPost, "/api/tag-kinds", `{"name":"Era"}`},
		{http.MethodPatch, "/api/tag-kinds/1", `{"name":"Era"}`},
		{http.MethodDelete, "/api/tag-kinds/1", ""},
	} {
		handler := newTestServer(t, Options{Tags: &fakeTags{err: broken}})
		rec := jsonRequest(t, handler, route.method, route.target, route.body)
		assertErrorBody(t, route.method+" 保存先の失敗", rec.Code, rec.Body.Bytes(), internal)
	}

	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	for _, route := range []struct{ method, target string }{
		{http.MethodPost, "/api/tag-kinds"},
		{http.MethodPatch, "/api/tag-kinds/1"},
	} {
		fake := &fakeTags{}
		rec := jsonRequest(t, newTestServer(t, Options{Tags: fake}), route.method, route.target, `{"name":`)
		assertErrorBody(t, route.method+" 壊れた本文", rec.Code, rec.Body.Bytes(), invalid)
		if fake.operation != "" {
			t.Errorf("%s: 壊れた本文が保存層に届いた (%s)", route.method, fake.operation)
		}
	}
}

func TestTagKindRoutesAreOwnerOnly(t *testing.T) {
	fake := &fakeTags{}
	env := newAuthEnv(t, t.TempDir(), Options{Tags: fake})
	env.setup()
	for _, req := range []authRequest{
		{method: http.MethodGet, target: "/api/tag-kinds"},
		{method: http.MethodPost, target: "/api/tag-kinds", body: `{"name":"Era"}`},
		{method: http.MethodPatch, target: "/api/tag-kinds/1", body: `{"name":"Era"}`},
		{method: http.MethodDelete, target: "/api/tag-kinds/1"},
	} {
		rec := env.serve(req)
		assertUnauthenticated(t, req.method+" "+req.target, rec)
	}
	if fake.operation != "" {
		t.Errorf("ゲストの要求が保存層に届いた (%s)", fake.operation)
	}
}

func TestTagShapesCarryKind(t *testing.T) {
	genre := &domain.TagKindRef{ID: 1, Name: "Genre", CardFirst: true}
	fake := &fakeTags{tags: []domain.Tag{
		{ID: 1, Name: "Action", Kind: genre},
		{ID: 2, Name: "Misc"},
	}}
	rec := do(t, newTestServer(t, Options{Tags: fake}), http.MethodGet, "/api/tags")
	body := rec.Body.String()
	if !strings.Contains(body, `"kind":{"cardFirst":true,"id":1,"name":"Genre"}`) || !strings.Contains(body, `"kind":null`) {
		t.Fatalf("tag list = %s", body)
	}
	if got := toAPITagRef(domain.TagRef{ID: 1, Name: "Action", Kind: genre}); got.Kind == nil || got.Kind.Name != "Genre" {
		t.Fatalf("TagRef.kind = %#v", got.Kind)
	}
	if got := toAPIVideoTag(domain.VideoTag{TagRef: domain.TagRef{ID: 2, Name: "Misc"}}); got.Kind != nil {
		t.Fatalf("VideoTag.kind = %#v, want nil", got.Kind)
	}
}
