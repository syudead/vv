package store

import (
	"context"
	"io/fs"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// 視聴履歴の記録（specs/043-watch-history/data-model.md）。

const (
	testPlaybackA = "0f8fad5b-d9cb-469f-a165-70867728950e"
	testPlaybackB = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
)

// historyRow は watch_history の 1 行である。
type historyRow struct {
	contentKey string
	playbackID string
	title      string
	playedAt   int64
}

// historyRows は watch_history を id の順に読み切る。playback_id が null の行は空にする。
func historyRows(t *testing.T, db *DB) []historyRow {
	t.Helper()
	rows, err := db.sql.Query(`select content_key, coalesce(playback_id, ''), title, played_at
		from watch_history order by id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []historyRow
	for rows.Next() {
		var row historyRow
		if err := rows.Scan(&row.contentKey, &row.playbackID, &row.title, &row.playedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func savePlay(t *testing.T, db *DB, userKey string, positionMs int64, play *domain.Play) {
	t.Helper()
	if _, err := db.Playback().SaveProgress(context.Background(), userKey,
		domain.Progress{PositionMs: positionMs, DurationMs: 100_000}, play); err != nil {
		t.Fatal(err)
	}
}

// 識別子の無い保存と、content_key が空の視聴は履歴を書かない。
func TestSaveProgressWithoutPlayWritesNoHistory(t *testing.T) {
	db := migratedDB(t)
	savePlay(t, db, "key-a", 1000, nil)
	savePlay(t, db, "key-a", 2000, &domain.Play{PlaybackID: testPlaybackA, Title: "a"})
	if rows := historyRows(t, db); len(rows) != 0 {
		t.Errorf("履歴 = %+v, want なし", rows)
	}
	if got := progressOf(t, db, "key-a"); got != 2000 {
		t.Errorf("再生位置 = %d, want 2000", got)
	}
}

// 同じ識別子の 2 回目以降の保存は何も足さず、時刻と題名は最初の保存のものに留まる。
func TestSaveProgressRecordsOneEntryPerPlaybackID(t *testing.T) {
	db := migratedDB(t)
	savePlay(t, db, "key-a", 1000, &domain.Play{PlaybackID: testPlaybackA, ContentKey: "key-a", Title: "最初"})
	first := historyRows(t, db)
	if len(first) != 1 || first[0].playedAt <= 0 {
		t.Fatalf("履歴 = %+v, want 1 件", first)
	}
	savePlay(t, db, "key-a", 9000, &domain.Play{PlaybackID: testPlaybackA, ContentKey: "key-a", Title: "改名後"})
	if got := historyRows(t, db); !slices.Equal(got, first) {
		t.Errorf("履歴 = %+v, want %+v", got, first)
	}
	want := historyRow{contentKey: "key-a", playbackID: testPlaybackA, title: "最初", playedAt: first[0].playedAt}
	if first[0] != want {
		t.Errorf("行 = %+v, want %+v", first[0], want)
	}
	if got := progressOf(t, db, "key-a"); got != 9000 {
		t.Errorf("再生位置 = %d, want 9000", got)
	}
}

// 1 つの動画を 2 つの識別子で保存すると 2 件。集まりの鍵で保存しても、履歴の鍵は視聴した
// バージョンの content_key である。
func TestSaveProgressRecordsEachPlayback(t *testing.T) {
	db := migratedDB(t)
	savePlay(t, db, "bundle:1", 1000, &domain.Play{PlaybackID: testPlaybackA, ContentKey: "key-a", Title: "a"})
	savePlay(t, db, "bundle:1", 2000, &domain.Play{PlaybackID: testPlaybackB, ContentKey: "key-a", Title: "a"})
	rows := historyRows(t, db)
	if len(rows) != 2 || rows[0].playbackID != testPlaybackA || rows[1].playbackID != testPlaybackB {
		t.Fatalf("履歴 = %+v, want 2 件", rows)
	}
	for _, row := range rows {
		if row.contentKey != "key-a" {
			t.Errorf("鍵 = %q, want key-a", row.contentKey)
		}
	}
}

// 履歴を消しても再生位置は変わらず、同じ識別子の次の保存は新しい 1 件を書く
// （Edge Case「再生中に消した場合」、要件 9）。
func TestSaveProgressAfterHistoryDeleted(t *testing.T) {
	db := migratedDB(t)
	play := &domain.Play{PlaybackID: testPlaybackA, ContentKey: "key-a", Title: "a"}
	savePlay(t, db, "key-a", 4000, play)
	if _, err := db.sql.Exec(`delete from watch_history`); err != nil {
		t.Fatal(err)
	}
	if got := progressOf(t, db, "key-a"); got != 4000 {
		t.Fatalf("履歴を消したら再生位置 = %d, want 4000", got)
	}
	savePlay(t, db, "key-a", 5000, play)
	if rows := historyRows(t, db); len(rows) != 1 || rows[0].playbackID != testPlaybackA {
		t.Errorf("履歴 = %+v, want 新しい 1 件", rows)
	}
}

// progressOf は鍵の再生位置を返す。
func progressOf(t *testing.T, db *DB, key string) int64 {
	t.Helper()
	progress, err := db.Playback().Progress(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return progress.PositionMs
}

// 同じパスの引き継ぎで履歴は新しい鍵へ移り、新しい鍵にすでにある履歴も残る。
func TestSuccessionMovesWatchHistory(t *testing.T) {
	db, _ := taggedVideoFixture(t)
	savePlay(t, db, "key-a", 40_000, &domain.Play{PlaybackID: testPlaybackA, ContentKey: "key-a", Title: "a"})
	scan := startTestScan(t, db)
	b := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-b", 2))
	savePlay(t, db, "key-b", 1000, &domain.Play{PlaybackID: testPlaybackB, ContentKey: "key-b", Title: "a"})
	finishTestScan(t, db, scan, domain.ScanDone)
	probeDuration(t, db, b, 100_000)

	rows := historyRows(t, db)
	if len(rows) != 2 {
		t.Fatalf("履歴 = %+v, want 2 件", rows)
	}
	for _, row := range rows {
		if row.contentKey != "key-b" {
			t.Errorf("行 %+v の鍵, want key-b", row)
		}
	}
}

// migrateTo は空のデータベースを version まで移行し、移行の操作を返す。
func migrateTo(t *testing.T, version int64) (*DB, *goose.Provider) {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(context.Background(), version); err != nil {
		t.Fatal(err)
	}
	return db, provider
}

func execSQL(t *testing.T, db *DB, query string, args ...any) {
	t.Helper()
	if _, err := db.sql.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

// 移行は再生位置の記録 1 行につき 1 件を、記録の時刻といまの題名で入れる。集まりの鍵は代表の
// content_key になり、集まりの行が無い集まりの鍵と空の鍵は捨てる。Down は表を落とす（R-4）。
func TestWatchHistoryMigrationBackfills(t *testing.T) {
	db, provider := migrateTo(t, 33)
	ctx := context.Background()
	execSQL(t, db, `insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`, fixturePath("/media"))
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2),
		listingFile(fixturePath("/media/c.mp4"), "c", "key-c", 3),
		listingFile(fixturePath("/media/d.mp4"), "d", "key-d", 4),
	)
	a, b := ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")]
	if _, err := db.Versions().Bundle(ctx, []int64{a, b}, a); err != nil {
		t.Fatal(err)
	}
	var bundleKey string
	if err := db.sql.QueryRow(`select user_key from video_bundles`).Scan(&bundleKey); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `insert into video_overrides (content_key, display_name, updated_at) values ('key-c', 'C の表示名', 1)`)
	for key, updatedAt := range map[string]int64{bundleKey: 100, "key-c": 200, "key-d": 300, "bundle:999": 400, "": 500} {
		execSQL(t, db, `insert into playback_progress (content_key, position_ms, completed, updated_at)
			values (?, 1000, 0, ?)`, key, updatedAt)
	}

	if _, err := provider.UpTo(ctx, 34); err != nil {
		t.Fatal(err)
	}
	want := []historyRow{
		{contentKey: "key-a", title: "a", playedAt: 100_000},
		{contentKey: "key-c", title: "C の表示名", playedAt: 200_000},
		{contentKey: "key-d", title: "d", playedAt: 300_000},
	}
	if got := historyRows(t, db); !slices.Equal(got, want) {
		t.Errorf("履歴 = %+v, want %+v", got, want)
	}

	if _, err := provider.DownTo(ctx, 33); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master where name like 'watch_history%'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("Down のあとも %d 個残った", tables)
	}
}

// 移行の時点で動画が無い記録も 1 件になり、題名は表示名、無ければ空。その内容が取り込み直される
// と、その件は再び動画を持つ（R-4、Edge Case「動画ファイルが消えて戻る」）。
func TestWatchHistoryMigrationKeepsRemovedContent(t *testing.T) {
	db, provider := migrateTo(t, 33)
	ctx := context.Background()
	execSQL(t, db, `insert into video_overrides (content_key, display_name, updated_at) values ('key-named', '名前付き', 1)`)
	for key, updatedAt := range map[string]int64{"key-named": 10, "key-plain": 20} {
		execSQL(t, db, `insert into playback_progress (content_key, position_ms, completed, updated_at)
			values (?, 1000, 0, ?)`, key, updatedAt)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	want := []historyRow{
		{contentKey: "key-named", title: "名前付き", playedAt: 10_000},
		{contentKey: "key-plain", title: "", playedAt: 20_000},
	}
	if got := historyRows(t, db); !slices.Equal(got, want) {
		t.Fatalf("履歴 = %+v, want %+v", got, want)
	}

	upsertOne(t, db, listingFile(fixturePath("/media/plain.mp4"), "plain", "key-plain", 1))
	var withVideo []string
	rows, err := db.sql.Query(`select h.content_key from watch_history h
		join videos v on v.content_key = h.content_key order by h.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		withVideo = append(withVideo, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(withVideo, []string{"key-plain"}) {
		t.Errorf("動画を持つ件 = %v, want [key-plain]", withVideo)
	}
}
