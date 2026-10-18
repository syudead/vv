package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// Tag-list transport tests use store fakes for query and response mapping.
// Invalid-cursor integration cases below retain a small real-store fixture.

func addTentativeTags(t *testing.T, f mcpFixture, videoID int64, names []string) {
	t.Helper()
	_, err := f.env.db.Tags().ApplyVideoTags(context.Background(),
		[]domain.VideoRef{{ID: videoID}}, domain.VideoTagsAdd, names, true)
	if err != nil {
		t.Fatal(err)
	}
}

func (f mcpFixture) getTags(t *testing.T, query string) extgen.TagList {
	t.Helper()
	rec := f.env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags" + query, header: bearer(f.secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/tags%s: status = %d: %s", query, rec.Code, rec.Body)
	}
	return decode[extgen.TagList](t, rec)
}

// Filter and paging parameters reach the store; page metadata reaches the caller.
func TestExternalListTagsPassesFiltersAndReturnsPage(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fake := &fakeTags{listPage: &domain.TagPage{
		Items: []domain.Tag{{ID: 7, Name: "selfie", Tentative: true, CreatedAt: created}},
		Total: 3, TotalAll: 5, NextCursor: "next",
	}}
	f := newMCPFixture(t, Options{Tags: fake, Scans: &fakeScans{}})
	list := f.getTags(t, "?tentative=true&q=SELFIE&limit=2&cursor=previous")
	want := domain.TagListQuery{TentativeOnly: true, Search: "SELFIE", Limit: 2, Cursor: "previous", Sort: domain.TagSortName}
	if fake.lastListQuery != want {
		t.Errorf("query = %+v, want %+v", fake.lastListQuery, want)
	}
	if list.Total != 3 || list.TotalAll != 5 || list.NextCursor == nil || *list.NextCursor != "next" || len(list.Items) != 1 {
		t.Fatalf("list = %+v", list)
	}
	tag := list.Items[0]
	if tag.Id != 7 || tag.Name != "selfie" || !tag.Tentative || !tag.CreatedAt.Equal(created) {
		t.Errorf("tag = %+v", tag)
	}
}

func TestExternalListTagsWithoutLimitReturnsEverything(t *testing.T) {
	fake := &fakeTags{tags: []domain.Tag{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}}}
	f := newMCPFixture(t, Options{Tags: fake, Scans: &fakeScans{}})

	rec := f.env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(f.secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "nextCursor") {
		t.Error("limit 無しの応答に nextCursor がある")
	}
	if fake.lastListQuery.Limit != 0 {
		t.Errorf("limit = %d, want unpaged", fake.lastListQuery.Limit)
	}
	list := decode[extgen.TagList](t, rec)
	if len(list.Items) != 3 || list.Total != 3 || list.TotalAll != 3 {
		t.Errorf("items = %d, total = %d, totalAll = %d, want 3", len(list.Items), list.Total, list.TotalAll)
	}
	// A cursor-only request still asks the store for an unpaged result.
	if again := f.getTags(t, "?cursor=nope"); len(again.Items) != 3 {
		t.Errorf("limit 無しの cursor = %d 件", len(again.Items))
	}
	if fake.lastListQuery.Limit != 0 {
		t.Errorf("cursor-only request limit = %d, want unpaged", fake.lastListQuery.Limit)
	}
}

func TestExternalListTagsRejectsInvalidParameters(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	addTentativeTags(t, f, f.videoA, []string{"a", "b", "c"})
	first := f.getTags(t, "?limit=1&sort=countDesc")
	if first.NextCursor == nil {
		t.Fatal("1 ページ目に nextCursor が無い")
	}
	otherSort := "?limit=1&sort=name&cursor=" + url.QueryEscape(*first.NextCursor)

	invalid := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, "", -1, 0}
	cursor := wantExternalError{http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, extgen.InvalidCursor, -1, 0}
	for label, tc := range map[string]struct {
		query string
		want  wantExternalError
	}{
		"sort が enum の外":   {"?sort=popular", invalid},
		"limit 0":          {"?limit=0", invalid},
		"limit 201":        {"?limit=201", invalid},
		"limit が数でない":      {"?limit=many", invalid},
		"tentative が真偽でない": {"?tentative=maybe", invalid},
		"q が 101 文字":       {"?q=" + strings.Repeat("あ", 101), invalid},
		"読めないカーソル":         {"?limit=1&cursor=nope", cursor},
		"別の sort のカーソル":    {otherSort, cursor},
	} {
		rec := f.env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags" + tc.query, header: bearer(f.secret)})
		assertExternalError(t, label, rec, tc.want)
	}
	f.getTags(t, "?q="+strings.Repeat("あ", 100))
}

// MCP supplies the default limit and forwards explicit paging and sort parameters.
func TestMCPListTagsPagesByDefault(t *testing.T) {
	fake := &fakeTags{listPage: &domain.TagPage{
		Items: []domain.Tag{{ID: 1, Name: "a"}}, Total: 3, TotalAll: 5, NextCursor: "next",
	}}
	f := newMCPFixture(t, Options{Tags: fake, Scans: &fakeScans{}})
	session := f.connect(t)

	var page extgen.TagList
	if callTool(t, session, "list_tags", nil, &page) {
		t.Fatalf("list_tags: isError: %+v", page)
	}
	if want := (domain.TagListQuery{Limit: 100, Sort: domain.TagSortName}); fake.lastListQuery != want {
		t.Errorf("default query = %+v, want %+v", fake.lastListQuery, want)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "a" || page.NextCursor == nil || *page.NextCursor != "next" || page.Total != 3 || page.TotalAll != 5 {
		t.Fatalf("page = %+v", page)
	}

	fake.listPage = &domain.TagPage{Items: []domain.Tag{{ID: 2, Name: "b"}}, Total: 3, TotalAll: 5}
	var rest extgen.TagList
	if callTool(t, session, "list_tags", map[string]any{"cursor": *page.NextCursor}, &rest) || len(rest.Items) != 1 || rest.NextCursor != nil {
		t.Fatalf("last page = %+v", rest)
	}
	if want := (domain.TagListQuery{Limit: 100, Sort: domain.TagSortName, Cursor: "next"}); fake.lastListQuery != want {
		t.Errorf("next query = %+v, want %+v", fake.lastListQuery, want)
	}

	fake.listPage = &domain.TagPage{Items: []domain.Tag{{ID: 3, Name: "used", VideoCount: 1}}, Total: 3, TotalAll: 5}
	var counted extgen.TagList
	if callTool(t, session, "list_tags", map[string]any{"limit": 2, "sort": "countDesc"}, &counted) {
		t.Fatalf("countDesc: isError: %+v", counted)
	}
	if want := (domain.TagListQuery{Limit: 2, Sort: domain.TagSortCountDesc}); fake.lastListQuery != want {
		t.Errorf("explicit query = %+v, want %+v", fake.lastListQuery, want)
	}
	if len(counted.Items) != 1 || counted.Items[0].VideoCount != 1 {
		t.Errorf("countDesc = %+v", counted)
	}

	fake.err = domain.ErrInvalidCursor
	var body extgen.Error
	if !callTool(t, session, "list_tags", map[string]any{"cursor": "nope"}, &body) || body.Reason == nil || *body.Reason != extgen.InvalidCursor {
		t.Errorf("invalid cursor: %+v", body)
	}
}
