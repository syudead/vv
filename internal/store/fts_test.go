package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// alternativesHint はテストが落ちた開発者に、次に読むべきものを示す。
// 出力だけで代替手段の検討先が分かる状態にしておく。
const alternativesHint = "この前提が崩れた場合の代替手段: " +
	"mattn/go-sqlite3 + -tags sqlite_fts5 への切り替え（CGO が要る）、" +
	"2文字検索を instr 経路へ寄せる範囲の見直し、外部の全文検索エンジンの導入。"

// ftsFixture はマイグレーションを適用したデータベースに検証用の行を入れて返す。
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
	// 登録フォルダの下に無い所在は search_key が空で、索引に載らない。
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values ('/media', 1, 1, 1)`); err != nil {
		t.Fatalf("テスト用メディアフォルダを登録できない: %v", err)
	}

	rows := []struct {
		path  string
		title string
	}{
		{"/media/夏休みの旅行.mp4", "夏休みの旅行"},
		{"/media/花火大会.mp4", "花火大会"},
		{"/media/holiday-trip.mp4", "holiday trip"},
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

// countMatch は MATCH での一致件数を返す。search_key は照合形なので、語にも
// 同じ変換を掛けてから引く。
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

// TestFTS5TrigramIsAvailable は trigram トークナイザの仮想表が作れることを固定する。
// Phase 0 最大のリスクだった「CGO 不要のドライバで日本語の部分一致検索ができるか」の
// 前提そのものである。
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

// TestFTS5TrigramMatchesThreeCharacterQuery は3文字の語が MATCH で一致することを固定する。
func TestFTS5TrigramMatchesThreeCharacterQuery(t *testing.T) {
	db := ftsFixture(t)

	if got := countMatch(t, db, "夏休み"); got != 1 {
		t.Errorf("MATCH '夏休み' = %d 件, want 1。3文字の語は trigram の索引が効くはず\n%s",
			got, alternativesHint)
	}
}

// TestFTS5TrigramMatchesThreeCharactersInsideWord は語中の3文字でも一致することを固定する。
// 前方一致ではなく部分一致が効くことが、日本語の検索では要である。
func TestFTS5TrigramMatchesThreeCharactersInsideWord(t *testing.T) {
	db := ftsFixture(t)

	if got := countMatch(t, db, "みの旅"); got != 1 {
		t.Errorf("MATCH 'みの旅' = %d 件, want 1。語中の3文字でも部分一致するはず\n%s",
			got, alternativesHint)
	}
}

// TestFTS5TrigramDoesNotMatchTwoCharacterQuery は「2文字の語は MATCH で0件」という
// 挙動そのものを期待値として固定する。trigram は3文字単位で索引を作るため、
// 2文字以下の検索語は MATCH の対象になり得ない。
//
// 将来 SQLite 側の挙動が変わったらこのテストが落ちて気付ける。落ちた場合は
// 検索の2経路（MATCH と instr）の切り替えを見直す契機になる。
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

// TestSearchKeyMatchesTwoCharacterQueryWithInstr は、2文字の語でも search_key への
// instr なら一致することを固定する。日本語では「旅行」「花火」のような2文字の
// 検索語が現実に多いため、この経路が成立していることが実装の前提になる。
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

// TestFTS5RebuildRecoversIndex は、トリガの取りこぼしが疑われたときの復旧手段が
// 実際に動くことを固定する。
func TestFTS5RebuildRecoversIndex(t *testing.T) {
	db := ftsFixture(t)

	// トリガを外し、索引へ反映されない行を作る。取りこぼしの状況を再現する。
	if _, err := db.sql.Exec(`drop trigger location_search_fts_ai`); err != nil {
		t.Fatalf("トリガを外せない: %v", err)
	}
	res, err := db.sql.Exec(`insert into videos(added_at, updated_at, content_key, container) values (1, 1, 'sports-day', 'mp4')`)
	if err != nil {
		t.Fatal(err)
	}
	videoID, _ := res.LastInsertId()
	if _, err := db.sql.Exec(`insert into video_locations(video_id, path, title, size_bytes, mtime, created_at, updated_at, search_key)
		values (?, '/media/運動会.mp4', '運動会', 1, 1, 1, 1, '運動会')`, videoID); err != nil {
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

// 語の調べ方は2通りある。照合形で3文字以上なら MATCH、1〜2文字なら
// search_key への instr で調べる。
//
// trigram は3文字単位で索引を作るため2文字以下は MATCH に一致せず、
// 日本語では2文字の検索語が多い。この振り分けが検索の前提である。
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
		// 数えるのは照合形の文字である。NFD の「が」（か + 濁点）は照合形では
		// 1文字に畳まれる。ここを取り違えると、2文字の入力が MATCH へ回って0件になる。
		{"\u304b\u3099\u3063", false},      // が + っ = 2文字（符号位置では3）
		{"\u304b\u3099\u3063\u3053", true}, // が + っ + こ = 3文字
		// 絵文字も1文字として数える。
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

// searchFixture は検索の検証用に行を入れたデータベースを返す。
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
			Path:       "/media/" + row.title + ".mp4",
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

// searchTitles は検索結果の題名を返す。
func searchTitles(t *testing.T, db *DB, query string) []string {
	t.Helper()

	page, err := db.Library().ListVideos(context.Background(), domain.VideoQuery{Query: query, Limit: domain.MaxLimit})
	if err != nil {
		t.Fatalf("検索に失敗した (%q): %v\n%s", query, err, alternativesHint)
	}
	return titlesOf(page)
}

// 3文字以上は MATCH 経路で、先頭一致ではない部分一致が取れる。
func TestSearchMatchRoute(t *testing.T) {
	db := searchFixture(t)

	for query, want := range map[string]string{
		"夏休み":     "夏休みの旅行",
		"みの旅":     "夏休みの旅行", // 語中の3文字
		"の街並み":    "京都の街並み",
		"holiday": "holiday trip",
	} {
		got := searchTitles(t, db, query)
		if len(got) != 1 || got[0] != want {
			t.Errorf("検索 %q = %v, want [%s]", query, got, want)
		}
	}
}

// 1〜2文字は instr 経路。日本語では2文字の検索語が多く、これが
// 取れないと検索が実用にならない。
func TestSearchInstrRoute(t *testing.T) {
	db := searchFixture(t)

	for query, want := range map[string]string{
		"旅行": "夏休みの旅行",
		"花火": "花火大会",
		"京都": "京都の街並み",
		"海":  "海辺の散歩", // 1文字
	} {
		got := searchTitles(t, db, query)
		if len(got) != 1 || got[0] != want {
			t.Errorf("検索 %q = %v, want [%s]\n%s", query, got, want, alternativesHint)
		}
	}
}

// 検索語は照合形にする。macOS から送られる NFD の入力でも一致する。
func TestSearchNormalizesQuery(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: "/media/がっこう.mp4", Title: "がっこう", ContentKey: "key-1",
		SizeBytes: 1, MTime: fixedTime, AddedAt: fixedTime, Container: "mp4",
	}); err != nil {
		t.Fatal(err)
	}

	// NFD（か + 濁点）で検索する。
	decomposed := "がっこう"
	if got := searchTitles(t, db, decomposed); len(got) != 1 {
		t.Errorf("NFD の検索語で %d 件, want 1（照合形へ正規化していない）", len(got))
	}
}

// FTS5 の特殊文字は無効化する。引用符で包まないと、構文誤りで検索そのものが
// 失敗し、利用者には「検索が壊れた」ようにしか見えない。
func TestSearchEscapesSpecialCharacters(t *testing.T) {
	db := searchFixture(t)

	for _, query := range []string{
		`"`, `""`, `*`, `:`, `^`, `-`, `(`, `)`,
		`夏休み"`, `"夏休み" OR "花火"`, `NEAR(夏 花)`, `title:夏`,
		`夏休み*`, `夏 AND 花火`, `'; drop table videos; --`,
	} {
		page, err := db.Library().ListVideos(context.Background(), domain.VideoQuery{Query: query, Limit: domain.MaxLimit})
		if err != nil {
			t.Errorf("検索 %q で失敗した: %v\n%s", query, err, alternativesHint)
			continue
		}
		// 落ちなければよい。件数は問わない（特殊文字を字面として扱う）。
		_ = page
	}

	// 表が壊れていないことを確かめる。
	if total, err := db.Library().CountVideos(context.Background(), ""); err != nil || total != 5 {
		t.Errorf("検索のあと total = %d (err=%v), want 5", total, err)
	}
}

// 検索時の並び順は一覧と同じ規則を使う。関連度（bm25）にしないのは、
// instr 経路に関連度が無く、2つの経路で並びが変わると利用者から見て
// 不可解になるためである。
func TestSearchUsesSameOrderAsListing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	// すべて「旅」を含む。追加順と題名順が食い違うように入れる。
	rows := []struct{ title, key string }{
		{"ち旅", "key-1"},
		{"あ旅", "key-2"},
		{"は旅", "key-3"},
	}
	for i, row := range rows {
		if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: "/media/" + row.title + ".mp4", Title: row.title, ContentKey: row.key,
			SizeBytes: int64(i + 1), MTime: fixedTime,
			AddedAt: fixedTime.Add(time.Duration(i) * time.Minute), Container: "mp4",
		}); err != nil {
			t.Fatal(err)
		}
	}

	added, err := db.Library().ListVideos(ctx, domain.VideoQuery{Query: "旅", Sort: domain.SortAddedDesc, Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"は旅", "あ旅", "ち旅"}; !equalStrings(titlesOf(added), want) {
		t.Errorf("addedDesc = %v, want %v", titlesOf(added), want)
	}

	byTitle, err := db.Library().ListVideos(ctx, domain.VideoQuery{Query: "旅", Sort: domain.SortTitleAsc, Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"あ旅", "ち旅", "は旅"}; !equalStrings(titlesOf(byTitle), want) {
		t.Errorf("titleAsc = %v, want %v", titlesOf(byTitle), want)
	}
}

// total は絞り込み後の件数である。
func TestSearchTotalIsFiltered(t *testing.T) {
	db := searchFixture(t)

	page, err := db.Library().ListVideos(context.Background(), domain.VideoQuery{Query: "旅行", Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Errorf("total = %d, want 1（絞り込み後の件数）", page.Total)
	}

	none, err := db.Library().ListVideos(context.Background(), domain.VideoQuery{Query: "該当しない語", Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	if none.Total != 0 || len(none.Items) != 0 {
		t.Errorf("該当なし: total = %d, items = %d, want 0 と 0", none.Total, len(none.Items))
	}
}

// 検索とカーソルを併用してもページングが破綻しないこと。
func TestSearchPagesWithCursor(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	for i := 0; i < 7; i++ {
		title := fmt.Sprintf("旅%d", i)
		if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: "/media/" + title + ".mp4", Title: title,
			ContentKey: fmt.Sprintf("key-%d", i), SizeBytes: int64(i + 1), MTime: fixedTime,
			AddedAt: fixedTime.Add(time.Duration(i) * time.Minute), Container: "mp4",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// 該当しない行も混ぜる。
	if _, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: "/media/花火.mp4", Title: "花火", ContentKey: "key-x",
		SizeBytes: 99, MTime: fixedTime, AddedAt: fixedTime, Container: "mp4",
	}); err != nil {
		t.Fatal(err)
	}

	var seen []string
	cursor := ""
	for page := 0; page < 10; page++ {
		got, err := db.Library().ListVideos(ctx, domain.VideoQuery{Query: "旅", Limit: 2, Cursor: cursor})
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

// 空白だけの検索語は絞り込まない。入力欄を消したときに0件にしない。
func TestSearchWithBlankQuery(t *testing.T) {
	db := searchFixture(t)

	for _, query := range []string{"", "   ", "\t\n"} {
		page, err := db.Library().ListVideos(context.Background(), domain.VideoQuery{Query: query, Limit: domain.MaxLimit})
		if err != nil {
			t.Fatalf("検索 %q で失敗した: %v", query, err)
		}
		if page.Total != 5 {
			t.Errorf("検索 %q: total = %d, want 5", query, page.Total)
		}
	}
}
