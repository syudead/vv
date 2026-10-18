package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const alternativesHint = "この前提が崩れた場合の代替手段: " +
	"mattn/go-sqlite3 + -tags sqlite_fts5 への切り替え（CGO が要る）、" +
	"2文字検索を instr 経路へ寄せる範囲の見直し、外部の全文検索エンジンの導入。"

func ftsFixture(t *testing.T) *DB {
	t.Helper()

	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("データベースを開けない: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("マイグレーションに失敗した: %v\n%s", err, alternativesHint)
	}
	// Locations outside registered folders have empty search keys and are not indexed.
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`, fixturePath("/media")); err != nil {
		t.Fatalf("テスト用メディアフォルダを登録できない: %v", err)
	}

	rows := []struct {
		path  string
		title string
	}{
		{fixturePath("/media/夏休みの旅行.mp4"), "夏休みの旅行"},
		{fixturePath("/media/花火大会.mp4"), "花火大会"},
		{fixturePath("/media/holiday-trip.mp4"), "holiday trip"},
	}
	for _, row := range rows {
		_, err := db.ScanIndex().UpsertVideo(context.Background(), domain.VideoFile{
			Path: row.path, Title: row.title, ContentKey: row.path,
			SizeBytes: 1024, MTime: time.Unix(1757000000, 0),
		})
		if err != nil {
			t.Fatalf("検証用の行を入れられない (%s): %v", row.path, err)
		}
	}

	return checkInvariants(t, db)
}

// Fold the query exactly as stored search keys are folded.
func countMatch(t *testing.T, db *DB, query string) int {
	t.Helper()

	var count int
	err := db.sql.QueryRow(
		`select count(*) from location_search_fts where location_search_fts match ?`, domain.FoldForMatch(query),
	).Scan(&count)
	if err != nil {
		t.Fatalf("MATCH の問い合わせに失敗した (%q): %v\n%s", query, err, alternativesHint)
	}
	return count
}

func TestFTS5TrigramIsAvailable(t *testing.T) {
	db := ftsFixture(t)

	var sqlText string
	err := db.sql.QueryRow(
		`select sql from sqlite_master where name = 'location_search_fts'`,
	).Scan(&sqlText)
	if err != nil {
		t.Fatalf("location_search_fts が作成されていない: %v\n%s", err, alternativesHint)
	}

	if !strings.Contains(sqlText, "tokenize='trigram'") {
		t.Errorf("location_search_fts が trigram で作られていない:\n%s\n%s", sqlText, alternativesHint)
	}
}

func TestFTS5TrigramMatchesThreeCharacterQuery(t *testing.T) {
	db := ftsFixture(t)

	if got := countMatch(t, db, "夏休み"); got != 1 {
		t.Errorf("MATCH '夏休み' = %d 件, want 1。3文字の語は trigram の索引が効くはず\n%s",
			got, alternativesHint)
	}
}

func TestFTS5TrigramMatchesThreeCharactersInsideWord(t *testing.T) {
	db := ftsFixture(t)

	if got := countMatch(t, db, "みの旅"); got != 1 {
		t.Errorf("MATCH 'みの旅' = %d 件, want 1。語中の3文字でも部分一致するはず\n%s",
			got, alternativesHint)
	}
}

// Lock in the trigram minimum so a driver behavior change prompts review of MATCH/instr routing.
func TestFTS5TrigramDoesNotMatchTwoCharacterQuery(t *testing.T) {
	db := ftsFixture(t)

	for _, query := range []string{"旅行", "旅行*", `"旅行"`} {
		if got := countMatch(t, db, query); got != 0 {
			t.Errorf("MATCH %q = %d 件, want 0。"+
				"2文字以下の検索語に MATCH が一致しないことを前提に、"+
				"検索は MATCH と instr の2経路に分けている。"+
				"一致するようになったのなら、その分岐を見直すこと\n%s",
				query, got, alternativesHint)
		}
	}
}

func TestSearchKeyMatchesTwoCharacterQueryWithInstr(t *testing.T) {
	db := ftsFixture(t)

	var count int
	err := db.sql.QueryRow(
		`select count(*) from video_locations where instr(search_key, ?) > 0`, "旅行",
	).Scan(&count)
	if err != nil {
		t.Fatalf("instr の問い合わせに失敗した: %v\n%s", err, alternativesHint)
	}
	if count != 1 {
		t.Errorf("instr '旅行' = %d 件, want 1\n%s", count, alternativesHint)
	}
}

func TestFTS5RebuildRecoversIndex(t *testing.T) {
	db := ftsFixture(t)

	// Remove triggers to simulate rows missing from the FTS index.
	if _, err := db.sql.Exec(`drop trigger location_search_fts_ai`); err != nil {
		t.Fatalf("トリガを外せない: %v", err)
	}
	res, err := db.sql.Exec(`insert into videos(added_at, indexed_at, content_key, container) values (1, 1, 'sports-day', 'mp4')`)
	if err != nil {
		t.Fatal(err)
	}
	videoID, _ := res.LastInsertId()
	if _, err := db.sql.Exec(`insert into video_locations(video_id, path, title, size_bytes, mtime, created_at, updated_at, search_key)
		values (?, ?, '運動会', 1, 1, 1, 1, '運動会')`, videoID, fixturePath("/media/運動会.mp4")); err != nil {
		t.Fatal(err)
	}
	if got := countMatch(t, db, "運動会"); got != 0 {
		t.Fatalf("前提が崩れている: トリガ無しで MATCH '運動会' = %d 件, want 0", got)
	}

	if _, err := db.sql.Exec(
		`insert into location_search_fts(location_search_fts) values ('rebuild')`,
	); err != nil {
		t.Fatalf("再構築に失敗した: %v\n%s", err, alternativesHint)
	}

	if got := countMatch(t, db, "運動会"); got != 1 {
		t.Errorf("再構築後の MATCH '運動会' = %d 件, want 1。"+
			"rebuild が取りこぼしの復旧手段として働いていない\n%s", got, alternativesHint)
	}
}

func TestSearchTermRouteSelection(t *testing.T) {
	tests := []struct {
		query     string
		wantMatch bool
	}{
		{"夏", false},
		{"旅行", false},
		{"夏休み", true},
		{"夏休みの旅行", true},
		{"ab", false},
		{"abc", true},
		// Count normalized code points; decomposed kana can shrink below the trigram minimum.
		{"\u304b\u3099\u3063", false},      // Two normalized characters, despite three input code points.
		{"\u304b\u3099\u3063\u3053", true}, // Three normalized characters.
		// Each emoji counts as one character.
		{"🎆🎇", false},
		{"🎆🎇🎈", true},
	}

	for _, tc := range tests {
		expr := domain.ParseSearchQuery(tc.query)
		if len(expr.Clauses) != 1 || len(expr.Clauses[0].Terms) != 1 {
			t.Fatalf("前提が崩れている: %q が1語にならない: %+v", tc.query, expr)
		}
		if got := termUsesMatch(expr.Clauses[0].Terms[0].Text); got != tc.wantMatch {
			t.Errorf("termUsesMatch(%q) = %v, want %v", tc.query, got, tc.wantMatch)
		}
	}
}

func searchFixture(t *testing.T) *DB {
	t.Helper()

	db := migratedDB(t)
	ctx := context.Background()

	rows := []struct{ title, key string }{
		{"夏休みの旅行", "key-1"},
		{"花火大会", "key-2"},
		{"holiday trip", "key-3"},
		{"京都の街並み", "key-4"},
		{"海辺の散歩", "key-5"},
	}
	for i, row := range rows {
		_, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path:       fixturePath("/media/") + row.title + ".mp4",
			Title:      row.title,
			ContentKey: row.key,
			SizeBytes:  int64(1000 + i),
			MTime:      fixedTime,
			AddedAt:    fixedTime.Add(time.Duration(i) * time.Minute),
			Container:  "mp4",
		})
		if err != nil {
			t.Fatalf("検証用の行を入れられない (%s): %v", row.title, err)
		}
	}
	return db
}

func searchTitles(t *testing.T, db *DB, query string) []string {
	t.Helper()

	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Query: query, Limit: domain.MaxLimit})
	if err != nil {
		t.Fatalf("検索に失敗した (%q): %v\n%s", query, err, alternativesHint)
	}
	return titlesOf(page)
}

func TestSearchMatchRoute(t *testing.T) {
	db := searchFixture(t)

	for query, want := range map[string]string{
		"夏休み":     "夏休みの旅行",
		"みの旅":     "夏休みの旅行",
		"の街並み":    "京都の街並み",
		"holiday": "holiday trip",
	} {
		got := searchTitles(t, db, query)
		if len(got) != 1 || got[0] != want {
			t.Errorf("検索 %q = %v, want [%s]", query, got, want)
		}
	}
}

func TestSearchInstrRoute(t *testing.T) {
	db := searchFixture(t)

	for query, want := range map[string]string{
		"旅行": "夏休みの旅行",
		"花火": "花火大会",
		"京都": "京都の街並み",
		"海":  "海辺の散歩",
	} {
		got := searchTitles(t, db, query)
		if len(got) != 1 || got[0] != want {
			t.Errorf("検索 %q = %v, want [%s]\n%s", query, got, want, alternativesHint)
		}
	}
}

func TestSearchNormalizesQuery(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: fixturePath("/media/がっこう.mp4"), Title: "がっこう", ContentKey: "key-1",
		SizeBytes: 1, MTime: fixedTime, AddedAt: fixedTime, Container: "mp4",
	}); err != nil {
		t.Fatal(err)
	}

	decomposed := "がっこう"
	if got := searchTitles(t, db, decomposed); len(got) != 1 {
		t.Errorf("NFD の検索語で %d 件, want 1（照合形へ正規化していない）", len(got))
	}
}

// Treat FTS punctuation as literal text rather than executable query syntax.
func TestSearchEscapesSpecialCharacters(t *testing.T) {
	db := searchFixture(t)

	for _, query := range []string{
		`"`, `""`, `*`, `:`, `^`, `-`, `(`, `)`,
		`夏休み"`, `"夏休み" OR "花火"`, `NEAR(夏 花)`, `title:夏`,
		`夏休み*`, `夏 AND 花火`, `'; drop table videos; --`,
	} {
		page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Query: query, Limit: domain.MaxLimit})
		if err != nil {
			t.Errorf("検索 %q で失敗した: %v\n%s", query, err, alternativesHint)
			continue
		}
		// Only successful execution matters here, not the number of literal matches.
		_ = page
	}

	if total, err := db.Library().CountVideos(context.Background(), domain.AudienceOwner, ""); err != nil || total != 5 {
		t.Errorf("検索のあと total = %d (err=%v), want 5", total, err)
	}
}

// Use the same list order for MATCH and instr because short terms have no BM25 relevance.
func TestSearchUsesSameOrderAsListing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	// Insert in an order that differs from title order.
	rows := []struct{ title, key string }{
		{"ち旅", "key-1"},
		{"あ旅", "key-2"},
		{"は旅", "key-3"},
	}
	for i, row := range rows {
		if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: fixturePath("/media/") + row.title + ".mp4", Title: row.title, ContentKey: row.key,
			SizeBytes: int64(i + 1), MTime: fixedTime,
			AddedAt: fixedTime.Add(time.Duration(i) * time.Minute), Container: "mp4",
		}); err != nil {
			t.Fatal(err)
		}
	}

	added, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: "旅", Sort: domain.SortAddedDesc, Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"は旅", "あ旅", "ち旅"}; !equalStrings(titlesOf(added), want) {
		t.Errorf("addedDesc = %v, want %v", titlesOf(added), want)
	}

	byTitle, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: "旅", Sort: domain.SortTitleAsc, Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"あ旅", "ち旅", "は旅"}; !equalStrings(titlesOf(byTitle), want) {
		t.Errorf("titleAsc = %v, want %v", titlesOf(byTitle), want)
	}
}

func TestSearchTotalIsFiltered(t *testing.T) {
	db := searchFixture(t)

	page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Query: "旅行", Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Errorf("total = %d, want 1（絞り込み後の件数）", page.Total)
	}

	none, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Query: "該当しない語", Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if none.Total != 0 || len(none.Items) != 0 {
		t.Errorf("該当なし: total = %d, items = %d, want 0 と 0", none.Total, len(none.Items))
	}
}

func TestSearchPagesWithCursor(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	for i := 0; i < 7; i++ {
		title := fmt.Sprintf("旅%d", i)
		if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: fixturePath("/media/") + title + ".mp4", Title: title,
			ContentKey: fmt.Sprintf("key-%d", i), SizeBytes: int64(i + 1), MTime: fixedTime,
			AddedAt: fixedTime.Add(time.Duration(i) * time.Minute), Container: "mp4",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: fixturePath("/media/花火.mp4"), Title: "花火", ContentKey: "key-x",
		SizeBytes: 99, MTime: fixedTime, AddedAt: fixedTime, Container: "mp4",
	}); err != nil {
		t.Fatal(err)
	}

	var seen []string
	cursor := ""
	for page := 0; page < 10; page++ {
		got, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Query: "旅", Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if got.Total != 7 {
			t.Errorf("%d ページ目の total = %d, want 7", page, got.Total)
		}
		seen = append(seen, titlesOf(got)...)
		if got.NextCursor == "" {
			break
		}
		cursor = got.NextCursor
	}

	if len(seen) != 7 {
		t.Errorf("ページを繋いだ件数 = %d, want 7: %v", len(seen), seen)
	}
	for _, title := range seen {
		if !strings.Contains(title, "旅") {
			t.Errorf("該当しない行が混ざった: %q", title)
		}
	}
}

func TestSearchWithBlankQuery(t *testing.T) {
	db := searchFixture(t)

	for _, query := range []string{"", "   ", "\t\n"} {
		page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner, domain.VideoQuery{Query: query, Limit: domain.MaxLimit})
		if err != nil {
			t.Fatalf("検索 %q で失敗した: %v", query, err)
		}
		if page.Total != 5 {
			t.Errorf("検索 %q: total = %d, want 5", query, page.Total)
		}
	}
}
