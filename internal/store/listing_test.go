package store

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Offset addition times so added order does not rely on IDs.
func listingFile(path, title, key string, offset int) domain.VideoFile {
	return domain.VideoFile{
		Path: path, Title: title, ContentKey: key,
		SizeBytes: int64(1000 + offset), MTime: fixedTime,
		AddedAt: fixedTime.Add(time.Duration(offset) * time.Minute), Container: "mp4",
	}
}

func upsertAll(t *testing.T, db *DB, files ...domain.VideoFile) map[string]int64 {
	t.Helper()
	ids := map[string]int64{}
	for _, file := range files {
		got, err := db.ScanIndex().UpsertVideo(context.Background(), file)
		if err != nil {
			t.Fatalf("取り込めない %s: %v", file.Path, err)
		}
		ids[file.Path] = got.ID
	}
	return ids
}

func sortedTitles(t *testing.T, db *DB, q domain.VideoQuery) []string {
	t.Helper()
	q.Limit = domain.MaxLimit
	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, q)
	if err != nil {
		t.Fatalf("一覧に失敗した (%+v): %v", q, err)
	}
	if page.Total != len(page.Items) {
		t.Errorf("%+v: total = %d, items = %d", q, page.Total, len(page.Items))
	}
	titles := titlesOf(page)
	slices.Sort(titles)
	return titles
}

func sorted(values ...string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	if out == nil {
		out = []string{}
	}
	return out
}

var searchExprTitles = []string{
	"京都旅行 2024", "京都旅行 2023", "2024 奈良", "猫と散歩",
	"ＡＢＣ１２３", "ABC", "ｶﾀｶﾅ", "たびにっき", "100% 達成", "a_b (test)",
}

func searchExprFixture(t *testing.T) *DB {
	t.Helper()
	db := migratedDB(t)
	// Register the fixture root before ingestion because search keys depend on it.
	if _, err := db.sql.Exec(`update media_folders set path = ?`, fixturePath("/media/videos")); err != nil {
		t.Fatal(err)
	}
	var files []domain.VideoFile
	for i, title := range searchExprTitles {
		files = append(files, listingFile(fixturePath("/media/videos/")+title+".mp4", title, fmt.Sprintf("key-%d", i), i))
	}
	upsertAll(t, db, files...)
	return db
}

func TestListVideosSearchExpressions(t *testing.T) {
	db := searchExprFixture(t)
	without := func(excluded ...string) []string {
		var out []string
		for _, title := range searchExprTitles {
			if !slices.Contains(excluded, title) {
				out = append(out, title)
			}
		}
		return sorted(out...)
	}

	tests := []struct {
		query string
		want  []string
	}{
		{"京都 2024", sorted("京都旅行 2024")},
		{"京都　2024", sorted("京都旅行 2024")},
		{`"京都旅行 2024"`, sorted("京都旅行 2024")},
		{`"旅行 20"`, sorted("京都旅行 2024", "京都旅行 2023")},
		{"京都 -2023", sorted("京都旅行 2024")},
		{"-京都", without("京都旅行 2024", "京都旅行 2023")},
		{"京都 OR 奈良", sorted("京都旅行 2024", "京都旅行 2023", "2024 奈良")},
		{"京都 | 奈良", sorted("京都旅行 2024", "京都旅行 2023", "2024 奈良")},
		{"2024 京都 OR 奈良", sorted("京都旅行 2024", "2024 奈良")},
		{`"京都`, sorted()},
		{"OR", sorted()},
		{"-", sorted()},
		{"100%", sorted("100% 達成")},
		{"a_b", sorted("a_b (test)")},
		{"(test)", sorted("a_b (test)")},
		{"title:x", sorted()},
		{"猫", sorted("猫と散歩")},
		{"猫 散歩", sorted("猫と散歩")},
		{"散歩 -犬", sorted("猫と散歩")},
		{"abc123", sorted("ＡＢＣ１２３")},
		{"ａｂｃ", sorted("ＡＢＣ１２３", "ABC")},
		{"カタカナ", sorted("ｶﾀｶﾅ")},
		{"タビ", sorted("たびにっき")},
		{"media", sorted()},
		{"videos", sorted()},
		{"", without()},
		{`""`, without()},
		{"|", without()},
	}
	for _, tc := range tests {
		if got := sortedTitles(t, db, domain.VideoQuery{Query: tc.query}); !slices.Equal(got, tc.want) {
			t.Errorf("検索 %q = %q, want %q", tc.query, got, tc.want)
		}
	}
}

func TestListVideosMatchesRelativeFolderNames(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.sql.Exec(`update media_folders set path = ?`, fixturePath("/media/videos")); err != nil {
		t.Fatal(err)
	}
	upsertAll(t, db,
		listingFile(fixturePath("/media/videos/2024/京都/a.mp4"), "a", "key-a", 0),
		listingFile(fixturePath("/media/videos/other/b.webm"), "b", "key-b", 1),
	)
	for query, want := range map[string][]string{
		"京都":        {"a"},
		"2024 京都":   {"a"},
		"2024/京都/a": {"a"},
		"webm":      {"b"},
		"media":     {},
	} {
		if got := sortedTitles(t, db, domain.VideoQuery{Query: query}); !slices.Equal(got, sorted(want...)) {
			t.Errorf("検索 %q = %q, want %q", query, got, want)
		}
	}
}

// All terms must match one location; returned attributes come from that matching location.
func TestListVideosMatchesWithinOneLocation(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile(fixturePath("/media/A/京都.mp4"), "京都", "same", 0),
		listingFile(fixturePath("/media/B/2024.mp4"), "2024", "same", 0),
	)
	ctx := context.Background()

	none, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: "京都 2024"})
	if err != nil {
		t.Fatal(err)
	}
	if none.Total != 0 || len(none.Items) != 0 {
		t.Errorf("京都 2024: total = %d, items = %v, want 0", none.Total, titlesOf(none))
	}

	for query, want := range map[string]string{
		"京都":   fixturePath("/media/A/京都.mp4"),
		"2024": fixturePath("/media/B/2024.mp4"),
		"":     fixturePath("/media/A/京都.mp4"),
	} {
		page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: query})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || len(page.Items) != 1 {
			t.Fatalf("検索 %q: total = %d, items = %d, want 1 件", query, page.Total, len(page.Items))
		}
		item := page.Items[0]
		wantTitle := strings.TrimSuffix(filepath.Base(want), ".mp4")
		if item.Path != want || item.Title != wantTitle {
			t.Errorf("検索 %q: path = %q, title = %q, want %q と %q", query, item.Path, item.Title, want, wantTitle)
		}
	}
}

func TestListingScopes(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile(fixturePath("/media/A/x 京都.mp4"), "x 京都", "key-x", 0),
		listingFile(fixturePath("/media/A/B/y 京都.mp4"), "y 京都", "key-y", 1),
		listingFile(fixturePath("/media/C/z 京都.mp4"), "z 京都", "key-z", 2),
		// The same content exists in both scopes, but only the out-of-scope location matches the query.
		listingFile(fixturePath("/media/A/B/w.mp4"), "w", "key-w", 3),
		listingFile(fixturePath("/media/C/w 京都.mp4"), "w 京都", "key-w", 3),
	)
	ctx := context.Background()

	folder := func(scope domain.FolderScope, query string) []string {
		t.Helper()
		page, err := db.Library().ListFolderVideos(ctx, domain.AudienceOwner, domain.FolderVideoQuery{Dir: fixturePath("/media/A"), Scope: scope, VideoQuery: domain.VideoQuery{Query: query, Limit: domain.MaxLimit}})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != len(page.Items) {
			t.Errorf("scope=%q query=%q: total = %d, items = %d", scope, query, page.Total, len(page.Items))
		}
		titles := titlesOf(page)
		slices.Sort(titles)
		return titles
	}

	if got, want := folder(domain.FolderScopeSubtree, "京都"), sorted("x 京都", "y 京都"); !slices.Equal(got, want) {
		t.Errorf("A の配下で 京都 = %q, want %q", got, want)
	}
	if got, want := folder(domain.FolderScopeDirect, "京都"), sorted("x 京都"); !slices.Equal(got, want) {
		t.Errorf("A の直下で 京都 = %q, want %q", got, want)
	}
	if got, want := folder("", "京都"), sorted("x 京都"); !slices.Equal(got, want) {
		t.Errorf("範囲の指定なしで 京都 = %q, want %q（直下と同じ）", got, want)
	}
	if got, want := folder(domain.FolderScopeSubtree, ""), sorted("x 京都", "y 京都", "w"); !slices.Equal(got, want) {
		t.Errorf("A の配下すべて = %q, want %q", got, want)
	}
	if got, want := sortedTitles(t, db, domain.VideoQuery{Query: "京都"}), sorted("x 京都", "y 京都", "z 京都", "w 京都"); !slices.Equal(got, want) {
		t.Errorf("ライブラリで 京都 = %q, want %q", got, want)
	}
}

func TestListingPagingSurvivesAddedLocations(t *testing.T) {
	for _, sort := range []domain.VideoSort{domain.SortAddedDesc, domain.SortTitleAsc} {
		for _, query := range []string{"", "旅"} {
			t.Run(fmt.Sprintf("%s/%q", sort, query), func(t *testing.T) {
				db := migratedDB(t)
				ctx := context.Background()
				var stable []string
				for i := range 6 {
					title := fmt.Sprintf("旅%d", i)
					stable = append(stable, title)
					upsertAll(t, db, listingFile(fixturePath("/media/")+title+".mp4", title, "key-"+title, i))
				}
				upsertAll(t, db, listingFile(fixturePath("/media/花火.mp4"), "花火", "key-hanabi", 10))

				var seen []string
				cursor := ""
				for pageIndex := range 10 {
					page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: query, Sort: sort, Limit: 2, Cursor: cursor})
					if err != nil {
						t.Fatal(err)
					}
					seen = append(seen, titlesOf(page)...)
					if pageIndex == 0 {
						// Add a later path without changing the representative, then add newly matching and entirely new videos.
						upsertAll(t, db,
							listingFile(fixturePath("/media/旅4~copy.mp4"), "copy", "key-旅4", 0),
							listingFile(fixturePath("/media/旅1~copy.mp4"), "copy", "key-旅1", 0),
							listingFile(fixturePath("/media/旅zz.mp4"), "旅zz", "key-hanabi", 0),
							listingFile(fixturePath("/media/旅new.mp4"), "旅new", "key-new", 20),
						)
					}
					if page.NextCursor == "" {
						break
					}
					cursor = page.NextCursor
				}
				for _, title := range stable {
					if count := countOf(seen, title); count != 1 {
						t.Errorf("%q が %d 回出た, want 1: %q", title, count, seen)
					}
				}
				for _, title := range seen {
					if countOf(seen, title) != 1 {
						t.Errorf("%q が重複した: %q", title, seen)
					}
				}
			})
		}
	}
}

func countOf(values []string, value string) int {
	count := 0
	for _, v := range values {
		if v == value {
			count++
		}
	}
	return count
}

func TestSearchExprCondition(t *testing.T) {
	clause, args := searchExprCondition(domain.ParseSearchQuery(`京都 -2023 夏休み OR 花 -"a b" ab"c`), "l", domain.AudienceOwner)

	fts := `l.id in (select rowid from location_search_fts where location_search_fts match ?)`
	instr := `instr(l.search_key, ?) > 0`
	tag := ownerFactsMatchCondition("l")
	want := `(` + instr + ` or ` + tag + `)` +
		` and not (` + fts + ` or ` + tag + `)` +
		` and ((` + fts + ` or ` + tag + `) or (` + instr + ` or ` + tag + `))` +
		` and not (` + fts + ` or ` + tag + `)` +
		` and (` + fts + ` or ` + tag + `)`
	if clause != want {
		t.Errorf("clause =\n%s\nwant\n%s", clause, want)
	}
	// Each location term is followed by nine unquoted folded terms for owner facts across both user-key paths and folder tags.
	nine := func(text string) []any {
		return []any{text, text, text, text, text, text, text, text, text}
	}
	var wantArgs []any
	for _, term := range [][2]string{{"京都", "京都"}, {`"2023"`, "2023"}, {`"夏休ミ"`, "夏休ミ"}, {"花", "花"},
		{`"a b"`, "a b"}, {`"ab""c"`, `ab"c`}} {
		wantArgs = append(append(wantArgs, term[0]), nine(term[1])...)
	}
	if fmt.Sprint(args) != fmt.Sprint(wantArgs) {
		t.Errorf("args = %q, want %q", args, wantArgs)
	}

	if clause, args := searchExprCondition(domain.SearchExpr{}, "l", domain.AudienceOwner); clause != "" || args != nil {
		t.Errorf("空の式 = %q %v, want 空", clause, args)
	}
	if got := quoteMatchPhrase(`a"b`); got != `"a""b"` {
		t.Errorf("quoteMatchPhrase = %q", got)
	}
}

func TestListVideosWithManyTerms(t *testing.T) {
	db := searchExprFixture(t)
	query := strings.Repeat("京都 ", 40) + "OR 奈良"
	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Query: query})
	if err != nil {
		t.Fatalf("語の多い検索で失敗した: %v", err)
	}
	want, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Query: "京都"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != want.Total {
		t.Errorf("語の多い検索: total = %d, want %d（京都 だけと同じ）", page.Total, want.Total)
	}
}

func TestListVideosExclusionIsEvaluatedPerLocation(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile(fixturePath("/media/A/京都.mp4"), "京都", "same", 0),
		listingFile(fixturePath("/media/B/x.mp4"), "x", "same", 0),
	)
	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Query: "-京都"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("-京都: total = %d, items = %d, want 1 件", page.Total, len(page.Items))
	}
	if item := page.Items[0]; item.Path != fixturePath("/media/B/x.mp4") || item.Title != "x" {
		t.Errorf("-京都: path = %q, title = %q, want /media/B/x.mp4 と x", item.Path, item.Title)
	}
}

func TestListVideosFiltersByTagIDsWithAndAndCombinesWithOtherFilters(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tagA, err := db.Tags().CreateTag(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	tagB, err := db.Tags().CreateTag(ctx, "B")
	if err != nil {
		t.Fatal(err)
	}
	upsertAll(t, db,
		listingFile(fixturePath("/media/both.mp4"), "both", "key-both", 0),
		listingFile(fixturePath("/media/a-only.mp4"), "a-only", "key-a", 1),
		listingFile(fixturePath("/media/b-only.mp4"), "b-only", "key-b", 2),
		listingFile(fixturePath("/media/neither.mp4"), "neither", "key-neither", 3),
	)
	attachTag(t, db, "key-both", tagA.ID)
	attachTag(t, db, "key-both", tagB.ID)
	attachTag(t, db, "key-a", tagA.ID)
	attachTag(t, db, "key-b", tagB.ID)

	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tagA.ID, tagB.ID}})
	if err != nil {
		t.Fatalf("ListVideos() error = %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Path != fixturePath("/media/both.mp4") {
		t.Fatalf("タグ A・B の AND = %+v, want /media/both.mp4 だけ", page)
	}
	if len(page.MissingTagIDs) != 0 {
		t.Errorf("MissingTagIDs = %v, want 空", page.MissingTagIDs)
	}

	searched, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tagA.ID}, Query: "b-only"})
	if err != nil {
		t.Fatal(err)
	}
	if searched.Total != 0 {
		t.Errorf("タグ A + 検索語 b-only = %d 件, want 0 (a-only と both だけが A を持つ)", searched.Total)
	}

	missing, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tagA.ID, 999999}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(missing.MissingTagIDs, []int64{999999}) {
		t.Errorf("MissingTagIDs = %v, want [999999]", missing.MissingTagIDs)
	}
	wantTitles := sorted("both", "a-only")
	if got := titlesOf(missing); !slices.Equal(sorted(got...), wantTitles) {
		t.Errorf("無い tag_id を無視した結果 = %q, want %q", got, wantTitles)
	}
	if missing.Total != 2 {
		t.Errorf("Total = %d, want 2 (無い tag_id は条件から落ちる)", missing.Total)
	}
}

func TestListVideosPagesWithTagFilterCoverEveryMatch(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "対象")
	if err != nil {
		t.Fatal(err)
	}
	var tagged []int64
	for i := range 5 {
		key := fmt.Sprintf("key-tagged-%d", i)
		got := upsertAll(t, db, listingFile(fmt.Sprintf(fixturePath("/media/tagged-%d.mp4"), i), fmt.Sprintf("tagged-%d", i), key, i))
		id := got[fmt.Sprintf(fixturePath("/media/tagged-%d.mp4"), i)]
		tagged = append(tagged, id)
		attachTag(t, db, key, tag.ID)
	}
	upsertAll(t, db, listingFile(fixturePath("/media/untagged.mp4"), "untagged", "key-untagged", 100))

	query := domain.VideoQuery{TagIDs: []int64{tag.ID}}
	wantIDs := slices.Clone(tagged)
	slices.Sort(wantIDs)

	var paged []int64
	cursor := ""
	query.Limit = 2
	for range 10 {
		query.Cursor = cursor
		page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, query)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			paged = append(paged, item.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	slices.Sort(paged)
	if !slices.Equal(paged, wantIDs) {
		t.Errorf("全ページの id = %v, want %v (タグの付いた動画の集合)", paged, wantIDs)
	}

	randomQuery := domain.VideoQuery{TagIDs: []int64{tag.ID}, Sort: domain.SortRandom, Seed: 12345, Limit: 2}
	var randomPaged []int64
	cursor = ""
	for range 10 {
		randomQuery.Cursor = cursor
		page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, randomQuery)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			randomPaged = append(randomPaged, item.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	slices.Sort(randomPaged)
	if !slices.Equal(randomPaged, wantIDs) {
		t.Errorf("ランダム並び順の全ページの id = %v, want %v (タグの付いた動画の集合)", randomPaged, wantIDs)
	}
}

func TestTagAttachmentSurvivesLocationMoveAndRescan(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db, listingFile(fixturePath("/media/old/a.mp4"), "a", "same-key", 0))
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids[fixturePath("/media/old/a.mp4")]}, tag.ID); err != nil {
		t.Fatal(err)
	}

	moved := listingFile(fixturePath("/media/new/a.mp4"), "a", "same-key", 1)
	if _, err := db.ScanIndex().UpsertVideo(ctx, moved); err != nil {
		t.Fatal(err)
	}
	var oldLocationID int64
	if err := db.sql.QueryRow(`select id from video_locations where path = ?`, fixturePath("/media/old/a.mp4")).Scan(&oldLocationID); err != nil {
		t.Fatal(err)
	}
	if err := db.ScanIndex().DeleteVideoLocations(ctx, []int64{oldLocationID}); err != nil {
		t.Fatal(err)
	}

	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tag.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Path != fixturePath("/media/new/a.mp4") {
		t.Errorf("移動後のタグでの絞り込み = %+v, want /media/new/a.mp4 だけ", page)
	}

	tags, err := db.Tags().TagsByContentKeys(ctx, []string{"same-key"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tags["same-key"]) != 1 || tags["same-key"][0].ID != tag.ID {
		t.Errorf("TagsByContentKeys(same-key) = %+v, want %s が付いている", tags["same-key"], tag.Name)
	}
}

func TestSearchMatchesTagNamesAcrossScopesAndFoldsSynonymsAndRenames(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.sql.Exec(`update media_folders set path = ?`, fixturePath("/media")); err != nil {
		t.Fatal(err)
	}

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	upsertAll(t, db,
		listingFile(fixturePath("/media/A/no-title-match.mp4"), "猫", "tagged-key", 0),
		listingFile(fixturePath("/media/A/other.mp4"), "犬", "other-key", 1),
	)
	if _, _, err := db.Tags().AttachTagByID(ctx, mustVideoID(t, db, fixturePath("/media/A/no-title-match.mp4")), tag.ID); err != nil {
		t.Fatal(err)
	}

	assertMatches := func(t *testing.T, query string, want bool) {
		t.Helper()
		page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: query})
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(titlesOf(page), "猫"); got != want {
			t.Errorf("ライブラリで %q = %v, want %v", query, got, want)
		}
		direct, err := db.Library().ListFolderVideos(ctx, domain.AudienceOwner, domain.FolderVideoQuery{Dir: fixturePath("/media/A"), VideoQuery: domain.VideoQuery{Query: query}})
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(titlesOf(direct), "猫"); got != want {
			t.Errorf("フォルダ直下で %q = %v, want %v", query, got, want)
		}
		subtree, err := db.Library().ListFolderVideos(ctx, domain.AudienceOwner, domain.FolderVideoQuery{Dir: fixturePath("/media"), Scope: domain.FolderScopeSubtree, VideoQuery: domain.VideoQuery{Query: query}})
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(titlesOf(subtree), "猫"); got != want {
			t.Errorf("フォルダ配下で %q = %v, want %v", query, got, want)
		}
	}

	assertMatches(t, "旅行", true)
	assertMatches(t, "-旅行", false)

	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "たび", nil); err != nil {
		t.Fatal(err)
	}
	assertMatches(t, "たび", true)

	assertMatches(t, "ﾀﾋﾞ", true)
	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "りょこう", nil); err != nil {
		t.Fatal(err)
	}
	assertMatches(t, "りょこう", true)
	assertMatches(t, "ﾘｮｺｳ", true)

	if _, err := db.Tags().RenameTag(ctx, tag.ID, "旅"); err != nil {
		t.Fatal(err)
	}
	assertMatches(t, "旅", true)
	assertMatches(t, "旅行", false)
}

func mustVideoID(t *testing.T, db *DB, path string) []int64 {
	t.Helper()
	var id int64
	if err := db.sql.QueryRow(`select video_id from video_locations where path = ?`, path).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return []int64{id}
}
