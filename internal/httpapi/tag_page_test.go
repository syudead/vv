package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GET /api/tags のページ読み（specs/036-tag-admin-scale/contracts/screen-api.md §5、#726）を、
// 本物の保存先で確かめる。

func TestListTagsPagesWithCursorAndTotals(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})
	ctx := context.Background()
	for i := range 250 {
		if _, err := env.db.Tags().CreateTag(ctx, fmt.Sprintf("tag %d", i)); err != nil {
			t.Fatal(err)
		}
	}

	var names []string
	target := "/api/tags?limit=100&sort=countDesc"
	for page := 0; ; page++ {
		rec := env.get(target, cookie)
		if rec.Code != http.StatusOK {
			t.Fatalf("page %d: status = %d: %s", page, rec.Code, rec.Body)
		}
		if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
			t.Errorf("Cache-Control = %q", got)
		}
		list := decode[gen.TagList](t, rec)
		if list.Total != 250 || list.TotalAll != 250 {
			t.Errorf("page %d: total = %d, totalAll = %d, want 250, 250", page, list.Total, list.TotalAll)
		}
		wantItems := 100
		if page == 2 {
			wantItems = 50
		}
		if len(list.Items) != wantItems {
			t.Errorf("page %d: items = %d, want %d", page, len(list.Items), wantItems)
		}
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		if list.NextCursor == nil {
			if page != 2 {
				t.Errorf("nextCursor が page %d で尽きた", page)
			}
			break
		}
		if page >= 2 {
			t.Fatalf("page %d に nextCursor がある", page)
		}
		target = "/api/tags?limit=100&sort=countDesc&cursor=" + url.QueryEscape(*list.NextCursor)
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Errorf("%s が重複した", name)
		}
		seen[name] = true
	}
	if len(seen) != 250 {
		t.Errorf("読めたタグ = %d, want 250", len(seen))
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

// TestListTagsReturnsExactSpelling は、綴りが完全に一致するタグが 1 ページ目に入らなくても
// 応答の exact に入ることを確かめる（統合の窓の統合先）。
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
	// 別の並び順のカーソルも 400。
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
	// 100 文字ちょうどは受ける。
	if rec := env.get("/api/tags?q="+strings.Repeat("a", 100), cookie); rec.Code != http.StatusOK {
		t.Errorf("100 文字の q: status = %d: %s", rec.Code, rec.Body)
	}
}

// 外部連携 API の一覧は limit を省けば画面と同じ並びで全件を返し、nextCursor を持たない
// （specs/039-external-tag-admin/contracts/external-api.md §1）。
func TestExternalListTagsWithoutLimitMatchesScreenFullList(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})
	ctx := context.Background()
	for i := range 250 {
		if _, err := env.db.Tags().CreateTag(ctx, fmt.Sprintf("tag %d", i)); err != nil {
			t.Fatal(err)
		}
	}
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
	screen := decode[gen.TagList](t, env.get("/api/tags", cookie))
	var names, screenNames []string
	for _, item := range list.Items {
		names = append(names, item.Name)
	}
	for _, item := range screen.Items {
		screenNames = append(screenNames, item.Name)
	}
	if len(names) != 250 || !slices.Equal(names, screenNames) || list.Total != 250 || list.TotalAll != 250 {
		t.Errorf("外部 %d 件（total %d、totalAll %d）、画面の全件と並びが違う", len(names), list.Total, list.TotalAll)
	}
}

// ゲストは 401 で、保存層に届かない。
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
