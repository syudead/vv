package store

import (
	"context"
	"strings"
	"testing"
)

// alternativesHint はテストが落ちた開発者に、次に読むべきものを示す（FR-015）。
// 出力だけで代替手段の検討先が分かる状態にしておく。
const alternativesHint = "この前提が崩れた場合の代替手段は " +
	"specs/001-initial-setup/research.md の R-001 \"Alternatives considered\" にある" +
	"（mattn/go-sqlite3 + -tags sqlite_fts5 への切り替え、2文字検索の扱い、" +
	"外部の全文検索エンジンの導入）。まずそこを読むこと。"

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

	rows := []struct {
		path  string
		title string
	}{
		{"/media/夏休みの旅行.mp4", "夏休みの旅行"},
		{"/media/花火大会.mp4", "花火大会"},
		{"/media/holiday-trip.mp4", "holiday trip"},
	}
	for _, row := range rows {
		_, err := db.SQL().Exec(
			`insert into videos(path, title, size_bytes, mtime) values (?, ?, ?, ?)`,
			row.path, row.title, 1024, 1757000000,
		)
		if err != nil {
			t.Fatalf("検証用の行を入れられない (%s): %v", row.path, err)
		}
	}

	return db
}

// countMatch は MATCH での一致件数を返す。
func countMatch(t *testing.T, db *DB, query string) int {
	t.Helper()

	var count int
	err := db.SQL().QueryRow(
		`select count(*) from videos_fts where videos_fts match ?`, query,
	).Scan(&count)
	if err != nil {
		t.Fatalf("MATCH の問い合わせに失敗した (%q): %v\n%s", query, err, alternativesHint)
	}
	return count
}

// TestFTS5TrigramIsAvailable は trigram トークナイザの仮想表が作れることを固定する。
// Phase 0 最大のリスクだった「CGO 不要のドライバで日本語の部分一致検索ができるか」の
// 前提そのものである（FR-014／SC-007、research.md R-001）。
func TestFTS5TrigramIsAvailable(t *testing.T) {
	db := ftsFixture(t)

	var sqlText string
	err := db.SQL().QueryRow(
		`select sql from sqlite_master where name = 'videos_fts'`,
	).Scan(&sqlText)
	if err != nil {
		t.Fatalf("videos_fts が作成されていない: %v\n%s", err, alternativesHint)
	}

	if !strings.Contains(sqlText, "tokenize='trigram'") {
		t.Errorf("videos_fts が trigram で作られていない:\n%s\n%s", sqlText, alternativesHint)
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
// 検索の2経路（MATCH と LIKE）の切り替えを見直す契機になる。
func TestFTS5TrigramDoesNotMatchTwoCharacterQuery(t *testing.T) {
	db := ftsFixture(t)

	for _, query := range []string{"旅行", "旅行*", `"旅行"`} {
		if got := countMatch(t, db, query); got != 0 {
			t.Errorf("MATCH %q = %d 件, want 0。"+
				"2文字以下の検索語に MATCH が一致しないことを前提に、"+
				"検索は MATCH と LIKE の2経路に分けている。"+
				"一致するようになったのなら、その分岐を見直すこと\n%s",
				query, got, alternativesHint)
		}
	}
}

// TestFTS5TrigramMatchesTwoCharacterQueryWithLike は、2文字の語でも同じ FTS5 表への
// LIKE なら一致することを固定する。日本語では「旅行」「花火」のような2文字の
// 検索語が現実に多いため、この経路が成立していることが実装の前提になる。
func TestFTS5TrigramMatchesTwoCharacterQueryWithLike(t *testing.T) {
	db := ftsFixture(t)

	var count int
	err := db.SQL().QueryRow(
		`select count(*) from videos_fts where title like ?`, "%旅行%",
	).Scan(&count)
	if err != nil {
		t.Fatalf("LIKE の問い合わせに失敗した: %v\n%s", err, alternativesHint)
	}
	if count != 1 {
		t.Errorf("LIKE '%%旅行%%' = %d 件, want 1。"+
			"trigram 表への LIKE は索引で処理される前提である\n%s", count, alternativesHint)
	}

	// 索引が使われていることも確認する（research.md R-001 の実行計画に対応）。
	var plan strings.Builder
	rows, err := db.SQL().Query(
		`explain query plan select rowid from videos_fts where title like ?`, "%旅行%",
	)
	if err != nil {
		t.Fatalf("実行計画を取得できない: %v\n%s", err, alternativesHint)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("実行計画を読めない: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("実行計画の読み出しに失敗した: %v", err)
	}

	if !strings.Contains(plan.String(), "VIRTUAL TABLE INDEX") {
		t.Errorf("LIKE が trigram 索引で処理されていない:\n%s\n%s",
			plan.String(), alternativesHint)
	}
}

// TestFTS5RebuildRecoversIndex は、トリガの取りこぼしが疑われたときの復旧手段が
// 実際に動くことを固定する（data-model.md「一貫性と再構築」）。
func TestFTS5RebuildRecoversIndex(t *testing.T) {
	db := ftsFixture(t)

	// トリガを外し、索引へ反映されない行を作る。取りこぼしの状況を再現する。
	if _, err := db.SQL().Exec(`drop trigger videos_ai`); err != nil {
		t.Fatalf("トリガを外せない: %v", err)
	}
	if _, err := db.SQL().Exec(
		`insert into videos(path, title, size_bytes, mtime) values ('/media/運動会.mp4', '運動会', 1, 1)`,
	); err != nil {
		t.Fatal(err)
	}
	if got := countMatch(t, db, "運動会"); got != 0 {
		t.Fatalf("前提が崩れている: トリガ無しで MATCH '運動会' = %d 件, want 0", got)
	}

	if _, err := db.SQL().Exec(
		`insert into videos_fts(videos_fts) values ('rebuild')`,
	); err != nil {
		t.Fatalf("再構築に失敗した: %v\n%s", err, alternativesHint)
	}

	if got := countMatch(t, db, "運動会"); got != 1 {
		t.Errorf("再構築後の MATCH '運動会' = %d 件, want 1。"+
			"rebuild が取りこぼしの復旧手段として働いていない\n%s", got, alternativesHint)
	}
}
