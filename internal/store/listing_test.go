package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// listingFile は title を題名とする所在1件を組み立てる。追加時刻は offset 分
// ずらし、追加順の並びが id 頼みにならないようにする。
func listingFile(path, title, key string, offset int) domain.VideoFile {
	return domain.VideoFile{
		Path: path, Title: title, ContentKey: key,
		SizeBytes: int64(1000 + offset), MTime: fixedTime,
		AddedAt: fixedTime.Add(time.Duration(offset) * time.Minute), Container: "mp4",
	}
}

// upsertAll は所在をすべて取り込み、パスごとの動画の id を返す。
func upsertAll(t *testing.T, db *DB, files ...domain.VideoFile) map[string]int64 {
	t.Helper()
	ids := map[string]int64{}
	for _, file := range files {
		got, err := db.UpsertVideo(context.Background(), file)
		if err != nil {
			t.Fatalf("取り込めない %s: %v", file.Path, err)
		}
		ids[file.Path] = got.ID
	}
	return ids
}

// sortedTitles は全件の題名を並べ替えて返す。並びに依らない比較に使う。
func sortedTitles(t *testing.T, db *DB, q domain.VideoQuery) []string {
	t.Helper()
	q.Limit = domain.MaxLimit
	page, err := db.ListVideos(context.Background(), q)
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

// searchExprTitles は受け入れ条件 1〜7 の題名である。
var searchExprTitles = []string{
	"京都旅行 2024", "京都旅行 2023", "2024 奈良", "猫と散歩",
	"ＡＢＣ１２３", "ABC", "ｶﾀｶﾅ", "たびにっき", "100% 達成", "a_b (test)",
}

// searchExprFixture は受け入れ条件 1〜7・9 の題名を、登録フォルダ
// /media/videos の直下に取り込む。
func searchExprFixture(t *testing.T) *DB {
	t.Helper()
	db := migratedDB(t)
	// 鍵は取り込み時の登録フォルダから作るので、取り込む前に差し替える。
	if _, err := db.SQL().Exec(`update media_folders set path = '/media/videos'`); err != nil {
		t.Fatal(err)
	}
	var files []domain.VideoFile
	for i, title := range searchExprTitles {
		files = append(files, listingFile("/media/videos/"+title+".mp4", title, fmt.Sprintf("key-%d", i), i))
	}
	upsertAll(t, db, files...)
	return db
}

// 親 Issue #195 の受け入れ条件 1〜7・9。
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
		// 1. 複数語は AND。
		{"京都 2024", sorted("京都旅行 2024")},
		{"京都　2024", sorted("京都旅行 2024")}, // 全角の空白
		// 2. フレーズ。途中で切ったフレーズでも当たる。
		{`"京都旅行 2024"`, sorted("京都旅行 2024")},
		{`"旅行 20"`, sorted("京都旅行 2024", "京都旅行 2023")},
		// 3. 除外。除外語だけでも、その語を含まないすべてを返す。
		{"京都 -2023", sorted("京都旅行 2024")},
		{"-京都", without("京都旅行 2024", "京都旅行 2023")},
		// 4. OR は空白の AND より強く結び付く。
		{"京都 OR 奈良", sorted("京都旅行 2024", "京都旅行 2023", "2024 奈良")},
		{"京都 | 奈良", sorted("京都旅行 2024", "京都旅行 2023", "2024 奈良")},
		{"2024 京都 OR 奈良", sorted("京都旅行 2024", "2024 奈良")},
		// 5. 構文として読めない入力は字面の語として探す。
		{`"京都`, sorted()},
		{"OR", sorted()},
		{"-", sorted()},
		{"100%", sorted("100% 達成")},
		{"a_b", sorted("a_b (test)")},
		{"(test)", sorted("a_b (test)")},
		{"title:x", sorted()},
		// 6. 1文字・2文字の語は、複数語や除外語に混じっていても当たる。
		{"猫", sorted("猫と散歩")},
		{"猫 散歩", sorted("猫と散歩")},
		{"散歩 -犬", sorted("猫と散歩")},
		// 7. 表記の揺れ。
		{"abc123", sorted("ＡＢＣ１２３")},
		{"ａｂｃ", sorted("ＡＢＣ１２３", "ABC")},
		{"カタカナ", sorted("ｶﾀｶﾅ")},
		{"タビ", sorted("たびにっき")},
		// 9. 登録フォルダ自身のパスには当たらない。
		{"media", sorted()},
		{"videos", sorted()},
		// 語が残らない入力は絞り込まない。
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

// 9. 登録フォルダより下のフォルダ名では当たる。拡張子も照合の対象である。
func TestListVideosMatchesRelativeFolderNames(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.SQL().Exec(`update media_folders set path = '/media/videos'`); err != nil {
		t.Fatal(err)
	}
	upsertAll(t, db,
		listingFile("/media/videos/2024/京都/a.mp4", "a", "key-a", 0),
		listingFile("/media/videos/other/b.webm", "b", "key-b", 1),
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

// 10. 語ごとに別の所在で満たした動画は当てない。当たった動画は1件として出て、
// 当たった所在の題名とパスで出る。
func TestListVideosMatchesWithinOneLocation(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile("/media/A/京都.mp4", "京都", "same", 0),
		listingFile("/media/B/2024.mp4", "2024", "same", 0),
	)
	ctx := context.Background()

	none, err := db.ListVideos(ctx, domain.VideoQuery{Query: "京都 2024"})
	if err != nil {
		t.Fatal(err)
	}
	if none.Total != 0 || len(none.Items) != 0 {
		t.Errorf("京都 2024: total = %d, items = %v, want 0", none.Total, titlesOf(none))
	}

	for query, want := range map[string]string{
		"京都":   "/media/A/京都.mp4",
		"2024": "/media/B/2024.mp4",
		"":     "/media/A/京都.mp4", // 検索語が無ければパスの最小の所在
	} {
		page, err := db.ListVideos(ctx, domain.VideoQuery{Query: query})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != 1 || len(page.Items) != 1 {
			t.Fatalf("検索 %q: total = %d, items = %d, want 1 件", query, page.Total, len(page.Items))
		}
		item := page.Items[0]
		wantTitle := strings.TrimSuffix(want[strings.LastIndex(want, "/")+1:], ".mp4")
		if item.Path != want || item.Title != wantTitle {
			t.Errorf("検索 %q: path = %q, title = %q, want %q と %q", query, item.Path, item.Title, want, wantTitle)
		}
	}
}

// 受け入れ条件 17・18 の配置で、範囲ごとに当たる動画。照合は範囲にある所在だけを
// 対象にする。
func TestListingScopes(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile("/media/A/x 京都.mp4", "x 京都", "key-x", 0),
		listingFile("/media/A/B/y 京都.mp4", "y 京都", "key-y", 1),
		listingFile("/media/C/z 京都.mp4", "z 京都", "key-z", 2),
		// 同じ内容が A の配下と C にある。A の配下の所在は「京都」を含まない。
		listingFile("/media/A/B/w.mp4", "w", "key-w", 3),
		listingFile("/media/C/w 京都.mp4", "w 京都", "key-w", 3),
	)
	ctx := context.Background()

	folder := func(scope domain.FolderScope, query string) []string {
		t.Helper()
		page, err := db.ListFolderVideos(ctx, domain.FolderVideoQuery{Dir: "/media/A", Scope: scope, Query: query, Limit: domain.MaxLimit})
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

// watchFixture は視聴状態と再生可否の組をすべて作る。題名は「状態-再生可否」。
func watchFixture(t *testing.T) *DB {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	states := []struct {
		name     string
		progress *domain.Progress
	}{
		{"none", nil},
		{"zero", &domain.Progress{PositionMs: 0}},
		{"middle", &domain.Progress{PositionMs: 30_000}},
		{"done", &domain.Progress{PositionMs: 100_000, Completed: true}},
	}
	offset := 0
	for _, state := range states {
		for _, playable := range []bool{true, false} {
			title := fmt.Sprintf("%s-%v", state.name, playable)
			key := "key-" + title
			ids := upsertAll(t, db, listingFile("/media/"+title+".mp4", title, key, offset))
			offset++
			if playable {
				if err := db.ApplyProbe(ctx, ids["/media/"+title+".mp4"], domain.Probe{DurationMs: 100_000, VideoCodec: "h264"},
					domain.Playability{Playable: true}); err != nil {
					t.Fatal(err)
				}
			}
			if state.progress != nil {
				if _, err := db.SaveProgress(ctx, key, *state.progress); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return db
}

// 視聴状態と再生可否で絞ると、Total は条件に合う全件の数になり、ページを
// 繋ぐと条件に合う動画だけがすべて出る。
func TestListVideosWatchAndPlayableFilters(t *testing.T) {
	db := watchFixture(t)
	ctx := context.Background()

	tests := []struct {
		watch    domain.WatchFilter
		playable bool
		want     []string
	}{
		{"", false, sorted("none-true", "none-false", "zero-true", "zero-false", "middle-true", "middle-false", "done-true", "done-false")},
		{domain.WatchAll, true, sorted("none-true", "zero-true", "middle-true", "done-true")},
		{domain.WatchUnwatched, false, sorted("none-true", "none-false", "zero-true", "zero-false")},
		{domain.WatchUnwatched, true, sorted("none-true", "zero-true")},
		{domain.WatchInProgress, false, sorted("middle-true", "middle-false")},
		{domain.WatchInProgress, true, sorted("middle-true")},
		{domain.WatchWatched, false, sorted("done-true", "done-false")},
		{domain.WatchWatched, true, sorted("done-true")},
	}
	for _, tc := range tests {
		var seen []string
		cursor := ""
		for range 10 {
			page, err := db.ListVideos(ctx, domain.VideoQuery{Watch: tc.watch, PlayableOnly: tc.playable, Limit: 1, Cursor: cursor})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != len(tc.want) {
				t.Errorf("watch=%q playable=%v: total = %d, want %d", tc.watch, tc.playable, page.Total, len(tc.want))
			}
			seen = append(seen, titlesOf(page)...)
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		slices.Sort(seen)
		if !slices.Equal(seen, tc.want) {
			t.Errorf("watch=%q playable=%v = %q, want %q", tc.watch, tc.playable, seen, tc.want)
		}
	}

	// 検索語とも AND で組み合わさる。
	if got, want := sortedTitles(t, db, domain.VideoQuery{Query: "none OR done", Watch: domain.WatchWatched, PlayableOnly: true}), sorted("done-true"); !slices.Equal(got, want) {
		t.Errorf("検索と絞り込み = %q, want %q", got, want)
	}
}

// SQL の条件は domain.ClassifyWatch と同じ定義である。
func TestWatchConditionMatchesDomainClassification(t *testing.T) {
	db := watchFixture(t)
	ctx := context.Background()
	for _, filter := range []domain.WatchFilter{domain.WatchUnwatched, domain.WatchInProgress, domain.WatchWatched} {
		page, err := db.ListVideos(ctx, domain.VideoQuery{Watch: filter, Limit: domain.MaxLimit})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			var progress *domain.Progress
			if got, err := db.Progress(ctx, item.ContentKey); err == nil {
				progress = &got
			}
			if state := domain.ClassifyWatch(progress); !filter.Matches(state) {
				t.Errorf("%q の一覧に %q（%q）が出た", filter, item.Title, state)
			}
		}
	}
}

// フォルダの一覧でも視聴状態と再生可否で絞れる。
func TestListFolderVideosFilters(t *testing.T) {
	db := watchFixture(t)
	page, err := db.ListFolderVideos(context.Background(), domain.FolderVideoQuery{
		Dir: "/media", Watch: domain.WatchInProgress, PlayableOnly: true, Limit: domain.MaxLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || !slices.Equal(titlesOf(page), []string{"middle-true"}) {
		t.Errorf("total = %d, items = %q, want middle-true だけ", page.Total, titlesOf(page))
	}
}

// ページを送る途中で別の動画の所在を足しても、1ページ目の時点で条件に合い、
// 並べ替えの値が変わらない動画に重複と取りこぼしが無い（contracts/list-api.md §5）。
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
					upsertAll(t, db, listingFile("/media/"+title+".mp4", title, "key-"+title, i))
				}
				// 当たらない動画も混ぜる。
				upsertAll(t, db, listingFile("/media/花火.mp4", "花火", "key-hanabi", 10))

				var seen []string
				cursor := ""
				for pageIndex := range 10 {
					page, err := db.ListVideos(ctx, domain.VideoQuery{Query: query, Sort: sort, Limit: 2, Cursor: cursor})
					if err != nil {
						t.Fatal(err)
					}
					seen = append(seen, titlesOf(page)...)
					if pageIndex == 0 {
						// 後ろのページに出る動画に、パスが後ろに来る所在を足す（題名は
						// 変わらない）。当たらなかった動画に、当たる所在を足す。新しい
						// 動画も足す。
						upsertAll(t, db,
							listingFile("/media/旅4~copy.mp4", "copy", "key-旅4", 0),
							listingFile("/media/旅1~copy.mp4", "copy", "key-旅1", 0),
							listingFile("/media/旅zz.mp4", "旅zz", "key-hanabi", 0),
							listingFile("/media/旅new.mp4", "旅new", "key-new", 20),
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

// 検索式は所在1行に対する条件句になる。3文字以上は MATCH、1〜2文字は instr で、
// 除外は not、OR は括弧で包んだ or、項どうしは and で結ぶ。
func TestSearchExprCondition(t *testing.T) {
	clause, args := searchExprCondition(domain.ParseSearchQuery(`京都 -2023 夏休み OR 花 -"a b" ab"c`), "l")
	want := `instr(l.search_key, ?) > 0` +
		` and not (l.id in (select rowid from location_search_fts where location_search_fts match ?))` +
		` and (l.id in (select rowid from location_search_fts where location_search_fts match ?) or instr(l.search_key, ?) > 0)` +
		` and not (l.id in (select rowid from location_search_fts where location_search_fts match ?))` +
		` and l.id in (select rowid from location_search_fts where location_search_fts match ?)`
	if clause != want {
		t.Errorf("clause =\n%s\nwant\n%s", clause, want)
	}
	wantArgs := []any{"京都", `"2023"`, `"夏休ミ"`, "花", `"a b"`, `"ab""c"`}
	if fmt.Sprint(args) != fmt.Sprint(wantArgs) {
		t.Errorf("args = %q, want %q", args, wantArgs)
	}

	if clause, args := searchExprCondition(domain.SearchExpr{}, "l"); clause != "" || args != nil {
		t.Errorf("空の式 = %q %v, want 空", clause, args)
	}
	if got := quoteMatchPhrase(`a"b`); got != `"a""b"` {
		t.Errorf("quoteMatchPhrase = %q", got)
	}
}

// 語を 16 個より多く並べても誤りにならない。
func TestListVideosWithManyTerms(t *testing.T) {
	db := searchExprFixture(t)
	query := strings.Repeat("京都 ", 40) + "OR 奈良"
	page, err := db.ListVideos(context.Background(), domain.VideoQuery{Query: query})
	if err != nil {
		t.Fatalf("語の多い検索で失敗した: %v", err)
	}
	// 先頭 16 語（すべて「京都」）だけが効き、後ろの OR 奈良 は無視される。
	want, err := db.ListVideos(context.Background(), domain.VideoQuery{Query: "京都"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != want.Total {
		t.Errorf("語の多い検索: total = %d, want %d（京都 だけと同じ）", page.Total, want.Total)
	}
}

// 除外語は所在ごとに評価する（contracts/list-api.md §1-8・§1-9）。除外語を含まない
// 所在が1つでもあれば動画は当たり、項目の題名と所在はその所在から取る。
func TestListVideosExclusionIsEvaluatedPerLocation(t *testing.T) {
	db := migratedDB(t)
	upsertAll(t, db,
		listingFile("/media/A/京都.mp4", "京都", "same", 0),
		listingFile("/media/B/x.mp4", "x", "same", 0),
	)
	page, err := db.ListVideos(context.Background(), domain.VideoQuery{Query: "-京都"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("-京都: total = %d, items = %d, want 1 件", page.Total, len(page.Items))
	}
	if item := page.Items[0]; item.Path != "/media/B/x.mp4" || item.Title != "x" {
		t.Errorf("-京都: path = %q, title = %q, want /media/B/x.mp4 と x", item.Path, item.Title)
	}
}

// 内容の識別子が空の動画は、空の識別子で記録された再生位置があっても未視聴として
// 絞る。API は空の識別子の再生位置を返さない（httpapi の progressFor）。
func TestWatchFilterIgnoresProgressOfEmptyContentKey(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile("/media/legacy.mp4", "legacy", "legacy-key", 0))
	if _, err := db.SQL().Exec(`update videos set content_key = '' where id = ?`, ids["/media/legacy.mp4"]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveProgress(ctx, "", domain.Progress{PositionMs: 100_000, Completed: true}); err != nil {
		t.Fatal(err)
	}
	for watch, want := range map[domain.WatchFilter]int{
		domain.WatchWatched:   0,
		domain.WatchUnwatched: 1,
	} {
		page, err := db.ListVideos(ctx, domain.VideoQuery{Watch: watch})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != want {
			t.Errorf("watch=%s: total = %d, want %d", watch, page.Total, want)
		}
	}
}
