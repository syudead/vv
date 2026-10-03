package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// タグの一覧のページ読み（specs/036-tag-admin-scale/data-model.md §2、#726）。

// createTagsNamed は名前ごとにタグを作り、名前から id への対応を返す。
func createTagsNamed(t *testing.T, db *DB, names ...string) map[string]int64 {
	t.Helper()
	ids := make(map[string]int64, len(names))
	for _, name := range names {
		tag, err := db.Tags().CreateTag(context.Background(), name)
		if err != nil {
			t.Fatalf("CreateTag(%q) error = %v", name, err)
		}
		ids[name] = tag.ID
	}
	return ids
}

// addLibraryVideos はいまライブラリにある動画を n 本足し、その content_key を返す。
func addLibraryVideos(t *testing.T, db *DB, n int) []string {
	t.Helper()
	keys := make([]string, n)
	for i := range n {
		keys[i] = fmt.Sprintf("key-page-%d", i)
		path := fixturePath(fmt.Sprintf("/media/page-%d.mp4", i))
		if _, err := db.ScanIndex().UpsertVideo(context.Background(),
			sampleFile(path, fmt.Sprintf("page %d", i), keys[i], int64(i+1), 0)); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// readAllTagPages は query を Limit ずつ最後まで読み、ページごとの結果を返す。
func readAllTagPages(t *testing.T, db *DB, query domain.TagListQuery) []domain.TagPage {
	t.Helper()
	var pages []domain.TagPage
	for {
		page, err := db.Tags().ListTags(context.Background(), query)
		if err != nil {
			t.Fatalf("ListTags(%+v) error = %v", query, err)
		}
		pages = append(pages, page)
		if page.NextCursor == "" {
			return pages
		}
		if len(pages) > 1000 {
			t.Fatal("ページが終わらない")
		}
		query.Cursor = page.NextCursor
	}
}

func pagedTagNames(pages []domain.TagPage) []string {
	var names []string
	for _, page := range pages {
		for _, tag := range page.Items {
			names = append(names, tag.Name)
		}
	}
	return names
}

// naturalOrder は名前を (NaturalSortKey, id) の順に並べた写しを返す。
func naturalOrder(names []string, ids map[string]int64) []string {
	sorted := slices.Clone(names)
	slices.SortFunc(sorted, func(a, b string) int {
		return cmp.Or(
			strings.Compare(domain.NaturalSortKey(a), domain.NaturalSortKey(b)),
			cmp.Compare(ids[a], ids[b]),
		)
	})
	return sorted
}

func TestListTagsReadsAllTagsInPagesByNaturalOrder(t *testing.T) {
	db := migratedDB(t)

	// 照合形が同じ名前（全角・半角、かな）を含め、id で前後が決まる組を作る。
	names := []string{"tag 2", "tag 10", "ｔａｇ ２", "アニメ", "あにめ", "Tag 1", "ﾀｸﾞ", "タグ"}
	for i := 0; len(names) < 250; i++ {
		names = append(names, fmt.Sprintf("tag %d", 100+i))
	}
	ids := createTagsNamed(t, db, names...)

	pages := readAllTagPages(t, db, domain.TagListQuery{Limit: 100})
	if len(pages) != 3 {
		t.Fatalf("ページの数 = %d, want 3", len(pages))
	}
	for i, page := range pages {
		if page.Total != 250 || page.TotalAll != 250 {
			t.Errorf("pages[%d]: Total = %d, TotalAll = %d, want 250, 250", i, page.Total, page.TotalAll)
		}
	}
	if len(pages[0].Items) != 100 || len(pages[1].Items) != 100 || len(pages[2].Items) != 50 {
		t.Errorf("各ページの件数 = %d, %d, %d, want 100, 100, 50",
			len(pages[0].Items), len(pages[1].Items), len(pages[2].Items))
	}
	if got, want := pagedTagNames(pages), naturalOrder(names, ids); !slices.Equal(got, want) {
		t.Errorf("並び = %v\nwant %v", got, want)
	}
	// 照合形が同じ名前は id の順。
	got := pagedTagNames(pages)
	if slices.Index(got, "アニメ") > slices.Index(got, "あにめ") {
		t.Errorf("アニメ（先に作った）が あにめ の後ろにある: %v", got)
	}
}

func TestListTagsSearchMatchesFoldedNameOrSynonym(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := createTagsNamed(t, db, "action", "映画", "drama")
	if _, err := db.Tags().AddSynonym(ctx, ids["映画"], "Action Movie", nil); err != nil {
		t.Fatal(err)
	}

	page, err := db.Tags().ListTags(ctx, domain.TagListQuery{Search: "  ＡＣＴＩＯＮ "})
	if err != nil {
		t.Fatal(err)
	}
	if got := pagedTagNames([]domain.TagPage{page}); !slices.Equal(got, []string{"action", "映画"}) {
		t.Errorf("当たったタグ = %v, want [action 映画]", got)
	}
	if page.Total != 2 || page.TotalAll != 3 {
		t.Errorf("Total = %d, TotalAll = %d, want 2, 3", page.Total, page.TotalAll)
	}
	if !slices.Equal(page.Items[1].Synonyms, []string{"Action Movie"}) {
		t.Errorf("映画のシノニム = %v", page.Items[1].Synonyms)
	}

	// 空白だけの検索語は絞らない。
	page, err = db.Tags().ListTags(ctx, domain.TagListQuery{Search: "　 "})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 {
		t.Errorf("空白だけの検索語: Total = %d, want 3", page.Total)
	}
}

func TestListTagsFiltersCombineWithAnd(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := createTagsNamed(t, db, "cat used", "cat unused", "cat tentative unused", "dog unused", "dog tentative used")
	for _, name := range []string{"cat tentative unused", "dog tentative used"} {
		if _, err := db.sql.Exec(`update tags set tentative = 1 where id = ?`, ids[name]); err != nil {
			t.Fatal(err)
		}
	}
	keys := addLibraryVideos(t, db, 1)
	attachTag(t, db, keys[0], ids["cat used"])
	attachTag(t, db, keys[0], ids["dog tentative used"])

	cases := []struct {
		query domain.TagListQuery
		want  []string
	}{
		{domain.TagListQuery{UnusedOnly: true}, []string{"cat tentative unused", "cat unused", "dog unused"}},
		{domain.TagListQuery{UnusedOnly: true, TentativeOnly: true}, []string{"cat tentative unused"}},
		{domain.TagListQuery{UnusedOnly: true, Search: "DOG"}, []string{"dog unused"}},
		{domain.TagListQuery{TentativeOnly: true}, []string{"cat tentative unused", "dog tentative used"}},
		{domain.TagListQuery{TentativeOnly: true, Search: "cat", UnusedOnly: true, Limit: 1}, []string{"cat tentative unused"}},
	}
	for _, tc := range cases {
		page, err := db.Tags().ListTags(ctx, tc.query)
		if err != nil {
			t.Fatal(err)
		}
		if got := pagedTagNames([]domain.TagPage{page}); !slices.Equal(got, tc.want) {
			t.Errorf("%+v: %v, want %v", tc.query, got, tc.want)
		}
		if page.Total != len(tc.want) || page.TotalAll != 5 {
			t.Errorf("%+v: Total = %d, TotalAll = %d, want %d, 5", tc.query, page.Total, page.TotalAll, len(tc.want))
		}
		if tc.query.UnusedOnly {
			for _, tag := range page.Items {
				if tag.VideoCount != 0 {
					t.Errorf("%+v: %s の本数 = %d, want 0", tc.query, tag.Name, tag.VideoCount)
				}
			}
		}
	}
}

func TestListTagsSortsByCountAndCreatedAcrossPages(t *testing.T) {
	db := migratedDB(t)
	// 本数と作った日が同じタグが多く、どのページの境目でも前後が同じ値になる組。
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h", "tag 2", "tag 10"}
	ids := createTagsNamed(t, db, names...)
	counts := map[string]int{"a": 3, "b": 1, "c": 1, "d": 1, "e": 1, "f": 0, "g": 0, "h": 1, "tag 2": 3, "tag 10": 0}
	created := map[string]int64{"a": 100, "b": 200, "c": 200, "d": 200, "e": 300, "f": 300, "g": 300, "h": 300, "tag 2": 100, "tag 10": 400}

	keys := addLibraryVideos(t, db, 3)
	for _, name := range names {
		for i := range counts[name] {
			attachTag(t, db, keys[i], ids[name])
		}
		if _, err := db.sql.Exec(`update tags set created_at = ? where id = ?`, created[name], ids[name]); err != nil {
			t.Fatal(err)
		}
	}

	expected := func(value func(string) int64, desc bool) []string {
		sorted := naturalOrder(names, ids)
		slices.SortStableFunc(sorted, func(a, b string) int {
			if desc {
				return cmp.Compare(value(b), value(a))
			}
			return cmp.Compare(value(a), value(b))
		})
		return sorted
	}
	count := func(name string) int64 { return int64(counts[name]) }
	createdAt := func(name string) int64 { return created[name] }

	cases := []struct {
		sort domain.TagSort
		want []string
	}{
		{domain.TagSortCountDesc, expected(count, true)},
		{domain.TagSortCountAsc, expected(count, false)},
		{domain.TagSortCreatedDesc, expected(createdAt, true)},
		{domain.TagSortCreatedAsc, expected(createdAt, false)},
		{domain.TagSortName, naturalOrder(names, ids)},
	}
	for _, tc := range cases {
		for _, limit := range []int{1, 2, 3, 4} {
			pages := readAllTagPages(t, db, domain.TagListQuery{Sort: tc.sort, Limit: limit})
			if got := pagedTagNames(pages); !slices.Equal(got, tc.want) {
				t.Errorf("%s limit=%d: %v\nwant %v", tc.sort, limit, got, tc.want)
			}
		}
	}

	// 最多のタグが先頭、0 本が末尾、同数は名前の順。
	desc := expected(count, true)
	if desc[0] != "a" || desc[1] != "tag 2" || desc[len(desc)-1] != "tag 10" {
		t.Errorf("countDesc = %v", desc)
	}
	// 新しく作ったタグが先頭。
	page, err := db.Tags().ListTags(context.Background(), domain.TagListQuery{Sort: domain.TagSortCreatedDesc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "tag 10" {
		t.Errorf("createdDesc の先頭 = %+v, want tag 10", page.Items)
	}
	if !page.Items[0].CreatedAt.Equal(time.Unix(400, 0)) {
		t.Errorf("CreatedAt = %v", page.Items[0].CreatedAt)
	}
}

func TestListTagsRejectsCursorOfAnotherSortOrUnreadable(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	createTagsNamed(t, db, "a", "b", "c")

	page, err := db.Tags().ListTags(ctx, domain.TagListQuery{Sort: domain.TagSortCountDesc, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor == "" {
		t.Fatal("NextCursor が空")
	}
	for _, query := range []domain.TagListQuery{
		{Sort: domain.TagSortName, Limit: 1, Cursor: page.NextCursor},
		{Sort: domain.TagSortCreatedDesc, Limit: 1, Cursor: page.NextCursor},
		{Sort: domain.TagSortCountDesc, Limit: 1, Cursor: "!!!"},
		{Sort: domain.TagSortCountDesc, Limit: 1, Cursor: packCursor("addedDesc", "", "0", "1", "x")},
		{Sort: domain.TagSortCountDesc, Limit: 1, Cursor: packCursor(tagCursorKind, "countDesc", "x", "1", "a")},
	} {
		if _, err := db.Tags().ListTags(ctx, query); !errors.Is(err, domain.ErrInvalidCursor) {
			t.Errorf("%+v: err = %v, want ErrInvalidCursor", query, err)
		}
	}
}

func TestListTagsWithoutLimitReturnsEverythingWithoutCursor(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	names := make([]string, 0, 250)
	for i := range 250 {
		names = append(names, fmt.Sprintf("t%d", i))
	}
	ids := createTagsNamed(t, db, names...)

	// Limit 0 はカーソルを無視する。
	page, err := db.Tags().ListTags(ctx, domain.TagListQuery{Cursor: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != "" {
		t.Errorf("NextCursor = %q, want 空", page.NextCursor)
	}
	if page.Total != 250 || page.TotalAll != 250 {
		t.Errorf("Total = %d, TotalAll = %d", page.Total, page.TotalAll)
	}
	if got := pagedTagNames([]domain.TagPage{page}); !slices.Equal(got, naturalOrder(names, ids)) {
		t.Errorf("並び = %v", got)
	}
}
