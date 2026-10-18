package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Tag-list handlers pass paging conditions and serialize store results.

func TestListTagsPagesWithCursorAndTotals(t *testing.T) {
	fake := &fakeTags{}
	handler := newTestServer(t, Options{Tags: fake})
	pages := []domain.TagPage{
		{Items: []domain.Tag{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}, Total: 3, TotalAll: 4, NextCursor: "next"},
		{Items: []domain.Tag{{ID: 3, Name: "c"}}, Total: 3, TotalAll: 4},
	}
	cursor := ""
	for _, page := range pages {
		fake.listPage = &page
		rec := do(t, handler, http.MethodGet, "/api/tags?limit=2&sort=countDesc&cursor="+url.QueryEscape(cursor))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body)
		}
		if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
			t.Errorf("Cache-Control = %q", got)
		}
		wantQuery := domain.TagListQuery{Limit: 2, Sort: domain.TagSortCountDesc, Cursor: cursor}
		if fake.lastListQuery != wantQuery {
			t.Errorf("query = %+v, want %+v", fake.lastListQuery, wantQuery)
		}
		list := decode[gen.TagList](t, rec)
		if list.Total != page.Total || list.TotalAll != page.TotalAll || len(list.Items) != len(page.Items) {
			t.Fatalf("list = %+v, want page %+v", list, page)
		}
		for i, item := range list.Items {
			if item.Id != page.Items[i].ID || item.Name != page.Items[i].Name {
				t.Errorf("item = %+v, want %+v", item, page.Items[i])
			}
		}
		if page.NextCursor == "" {
			if list.NextCursor != nil {
				t.Errorf("last page cursor = %v", list.NextCursor)
			}
		} else if list.NextCursor == nil || *list.NextCursor != page.NextCursor {
			t.Errorf("nextCursor = %v, want %q", list.NextCursor, page.NextCursor)
		}
		cursor = page.NextCursor
	}
}

func TestListTagsPassesConditionsToStore(t *testing.T) {
	fake := &fakeTags{}
	handler := newTestServer(t, Options{Tags: fake})
	rec := do(t, handler, http.MethodGet, "/api/tags?q=%E7%8C%AB&tentative=true&unused=true&sort=createdAsc&limit=5&cursor=abc")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	want := domain.TagListQuery{
		Search: "猫", TentativeOnly: true, UnusedOnly: true, Sort: domain.TagSortCreatedAsc, Cursor: "abc", Limit: 5,
	}
	if fake.lastListQuery != want {
		t.Errorf("ListTags(%+v), want %+v", fake.lastListQuery, want)
	}
}

func TestListTagsWithoutParametersReturnsEverything(t *testing.T) {
	fake := &fakeTags{tags: []domain.Tag{{ID: 1, Name: "a", Synonyms: []string{}}, {ID: 2, Name: "b", Synonyms: []string{}}}}
	rec := do(t, newTestServer(t, Options{Tags: fake}), http.MethodGet, "/api/tags")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if fake.lastListQuery != (domain.TagListQuery{Sort: domain.TagSortName}) {
		t.Errorf("ListTags(%+v), want 全件の名前の順", fake.lastListQuery)
	}
	if strings.Contains(rec.Body.String(), "nextCursor") {
		t.Errorf("nextCursor がある: %s", rec.Body)
	}
	list := decode[gen.TagList](t, rec)
	if len(list.Items) != 2 || list.Total != 2 || list.TotalAll != 2 {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestListTagsReturnsExactSpelling(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})
	ctx := context.Background()
	for _, name := range []string{"a1cat", "a2cat", "cat"} {
		if _, err := env.db.Tags().CreateTag(ctx, name); err != nil {
			t.Fatal(err)
		}
	}

	rec := env.get("/api/tags?q=cat&limit=2", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	list := decode[gen.TagList](t, rec)
	if len(list.Items) != 2 || list.Items[0].Name != "a1cat" {
		t.Errorf("items = %+v", list.Items)
	}
	if list.Exact == nil || list.Exact.Name != "cat" {
		t.Errorf("exact = %+v, want cat", list.Exact)
	}

	rec = env.get("/api/tags?q=ca&limit=2", cookie)
	if strings.Contains(rec.Body.String(), `"exact"`) {
		t.Errorf("綴りの一致が無いのに exact がある: %s", rec.Body)
	}
}

func TestListTagsRejectsInvalidParameters(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})
	for _, target := range []string{
		"/api/tags?limit=0",
		"/api/tags?limit=201",
		"/api/tags?sort=foo",
		"/api/tags?limit=10&cursor=%21%21%21",
		"/api/tags?q=" + strings.Repeat("a", 101),
	} {
		rec := env.get(target, cookie)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_request"`) {
			t.Errorf("%s: status = %d: %s", target, rec.Code, rec.Body)
		}
	}
	if _, err := env.db.Tags().CreateTag(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.db.Tags().CreateTag(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	list := decode[gen.TagList](t, env.get("/api/tags?limit=1&sort=countDesc", cookie))
	if list.NextCursor == nil {
		t.Fatal("nextCursor が無い")
	}
	rec := env.get("/api/tags?limit=1&sort=name&cursor="+url.QueryEscape(*list.NextCursor), cookie)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_request"`) {
		t.Errorf("別の並び順のカーソル: status = %d: %s", rec.Code, rec.Body)
	}
	if rec := env.get("/api/tags?q="+strings.Repeat("a", 100), cookie); rec.Code != http.StatusOK {
		t.Errorf("100 文字の q: status = %d: %s", rec.Code, rec.Body)
	}
}

func TestExternalListTagsWithoutLimitMatchesScreenFullList(t *testing.T) {
	fake := &fakeTags{tags: []domain.Tag{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}}}
	env, cookie := newExternalEnv(t, Options{Tags: fake})
	token := env.createAPIToken(cookie, "scraper")
	rec := env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(token.Secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["nextCursor"] != nil {
		t.Errorf("limit 無しの応答に nextCursor がある")
	}
	var list extgen.TagList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if fake.lastListQuery.Limit != 0 {
		t.Errorf("external limit = %d, want unpaged", fake.lastListQuery.Limit)
	}
	screen := decode[gen.TagList](t, env.get("/api/tags", cookie))
	if fake.lastListQuery.Limit != 0 {
		t.Errorf("screen limit = %d, want unpaged", fake.lastListQuery.Limit)
	}
	var names, screenNames []string
	for _, item := range list.Items {
		names = append(names, item.Name)
	}
	for _, item := range screen.Items {
		screenNames = append(screenNames, item.Name)
	}
	if len(names) != 3 || !slices.Equal(names, screenNames) || list.Total != 3 || list.TotalAll != 3 {
		t.Errorf("外部 %d 件（total %d、totalAll %d）、画面の全件と並びが違う", len(names), list.Total, list.TotalAll)
	}
}

func TestListTagsIsOwnerOnly(t *testing.T) {
	fake := &fakeTags{}
	env := newAuthEnv(t, t.TempDir(), Options{Tags: fake})
	env.setup()
	for _, target := range []string{"/api/tags", "/api/tags?limit=10&sort=countDesc"} {
		assertUnauthenticated(t, target, env.get(target))
	}
	if fake.lastListQuery != (domain.TagListQuery{}) {
		t.Errorf("ゲストの要求が保存層に届いた (%+v)", fake.lastListQuery)
	}
}
