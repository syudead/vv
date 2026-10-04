package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// GET /api/v1/tags と MCP の list_tags の検索・絞り込み・並び順・ページ
// （specs/039-external-tag-admin/contracts/external-api.md §1・§8、research.md R-1）を、
// 本物の保存先と Bearer の境界で確かめる。

// addTentativeTags は名前 names の仮のタグを作り、動画 videoID に付ける。
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

// 共通の語を持つ仮のタグ 2,000 個を tentative・q・limit=200 で cursor を辿って読むと、全部を
// 1 回ずつ読め、最後のページに nextCursor が無い。どのページにも total・totalAll があり、各タグに
// createdAt がある（受け入れ条件 1）。
func TestExternalListTagsPagesFilteredTentativeTags(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	ctx := context.Background()
	const matching = 2000
	names := make([]string, 0, matching)
	for i := range matching {
		names = append(names, fmt.Sprintf("selfie variant %04d", i))
	}
	addTentativeTags(t, f, f.videoA, names)
	// 語に合わない仮のタグと、語に合う確定したタグは入らない。
	addTentativeTags(t, f, f.videoA, []string{"other tentative"})
	if _, err := f.env.db.Tags().CreateTag(ctx, "selfie confirmed"); err != nil {
		t.Fatal(err)
	}
	const all = matching + 2

	seen := map[int64]bool{}
	query := "?tentative=true&q=" + url.QueryEscape("SELFIE") + "&limit=200"
	pages := 0
	for target := query; ; pages++ {
		if pages > matching/200 {
			t.Fatalf("ページが終わらない（%d ページ）", pages)
		}
		list := f.getTags(t, target)
		if list.Total != matching || list.TotalAll != all {
			t.Errorf("page %d: total = %d, totalAll = %d, want %d, %d", pages, list.Total, list.TotalAll, matching, all)
		}
		if len(list.Items) != 200 {
			t.Errorf("page %d: items = %d, want 200", pages, len(list.Items))
		}
		for _, tag := range list.Items {
			if seen[tag.Id] {
				t.Errorf("%s (%d) を 2 度読んだ", tag.Name, tag.Id)
			}
			seen[tag.Id] = true
			if !tag.Tentative || !strings.HasPrefix(tag.Name, "selfie variant ") {
				t.Errorf("条件の外のタグ: %+v", tag)
			}
			if tag.CreatedAt.IsZero() {
				t.Errorf("%s に createdAt が無い", tag.Name)
			}
		}
		if list.NextCursor == nil {
			break
		}
		target = query + "&cursor=" + url.QueryEscape(*list.NextCursor)
	}
	if len(seen) != matching || pages+1 != matching/200 {
		t.Errorf("読めたタグ = %d（%d ページ）, want %d（%d ページ）", len(seen), pages+1, matching, matching/200)
	}
}

// limit を省いた要求は今までどおり全件を返し、nextCursor を付けない。
func TestExternalListTagsWithoutLimitReturnsEverything(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	names := make([]string, 0, 250)
	for i := range 250 {
		names = append(names, fmt.Sprintf("tag %03d", i))
	}
	addTentativeTags(t, f, f.videoA, names)

	rec := f.env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(f.secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "nextCursor") {
		t.Error("limit 無しの応答に nextCursor がある")
	}
	list := decode[extgen.TagList](t, rec)
	if len(list.Items) != 250 || list.Total != 250 || list.TotalAll != 250 {
		t.Errorf("items = %d, total = %d, totalAll = %d, want 250", len(list.Items), list.Total, list.TotalAll)
	}
	// cursor は limit が無ければ無視する（画面の GET /api/tags と同じ）。
	if again := f.getTags(t, "?cursor=nope"); len(again.Items) != 250 {
		t.Errorf("limit 無しの cursor = %d 件", len(again.Items))
	}
}

// sort が 5 値の外、limit が 1〜200 の外、q が 100 文字を超える要求、読めないカーソルと別の sort で
// 作ったカーソルは 400 invalid_request で、カーソルは invalid_cursor。
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
	// 100 文字ちょうどは通る。
	f.getTags(t, "?q="+strings.Repeat("あ", 100))
}

// 引数なしの list_tags は 100 件と nextCursor を返し、limit: 10・sort: countDesc は本数の
// 順に 10 件を返す（contracts/external-api.md §8）。
func TestMCPListTagsPagesByDefault(t *testing.T) {
	f := newMCPFixture(t, Options{Scans: &fakeScans{}})
	ctx := context.Background()
	for i := range 150 {
		if _, err := f.env.db.Tags().CreateTag(ctx, fmt.Sprintf("unused %03d", i)); err != nil {
			t.Fatal(err)
		}
	}
	used := []string{"used 1", "used 2", "used 3"}
	addTentativeTags(t, f, f.videoA, used)
	session := f.connect(t)

	var page extgen.TagList
	if callTool(t, session, "list_tags", nil, &page) {
		t.Fatalf("list_tags: isError: %+v", page)
	}
	if len(page.Items) != 100 || page.NextCursor == nil || page.Total != 153 || page.TotalAll != 153 {
		t.Errorf("引数なし: items = %d, nextCursor = %v, total = %d, totalAll = %d",
			len(page.Items), page.NextCursor, page.Total, page.TotalAll)
	}
	var rest extgen.TagList
	if callTool(t, session, "list_tags", map[string]any{"cursor": *page.NextCursor}, &rest) ||
		len(rest.Items) != 53 || rest.NextCursor != nil {
		t.Errorf("2 ページ目: items = %d, nextCursor = %v", len(rest.Items), rest.NextCursor)
	}

	var counted extgen.TagList
	if callTool(t, session, "list_tags", map[string]any{"limit": 10, "sort": "countDesc"}, &counted) {
		t.Fatalf("countDesc: isError: %+v", counted)
	}
	if len(counted.Items) != 10 || counted.NextCursor == nil {
		t.Fatalf("countDesc: items = %d, nextCursor = %v", len(counted.Items), counted.NextCursor)
	}
	for i, tag := range counted.Items {
		want := 0
		if i < len(used) {
			want = 1
		}
		if tag.VideoCount != want {
			t.Errorf("countDesc[%d] = %s（%d 本）, want %d 本", i, tag.Name, tag.VideoCount, want)
		}
	}

	var body extgen.Error
	if !callTool(t, session, "list_tags", map[string]any{"cursor": "nope"}, &body) ||
		body.Reason == nil || *body.Reason != extgen.InvalidCursor {
		t.Errorf("不正なカーソル: %+v", body)
	}
}
