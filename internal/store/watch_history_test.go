package store

import (
	"context"
	"errors"
	"io/fs"
	"maps"
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

// insertHistory は視聴履歴を 1 行入れ、その id を返す。
func insertHistory(t *testing.T, db *DB, contentKey, title string, playedAtMs int64) int64 {
	t.Helper()
	result, err := db.sql.Exec(`insert into watch_history (content_key, title, played_at) values (?, ?, ?)`,
		contentKey, title, playedAtMs)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func listHistory(t *testing.T, db *DB, audience domain.Audience, cursor string, limit int) domain.WatchHistoryPage {
	t.Helper()
	page, err := db.Playback().ListWatchHistory(context.Background(), audience, domain.WatchHistoryQuery{}, cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func historyIDs(page domain.WatchHistoryPage) []int64 {
	ids := make([]int64, 0, len(page.Items))
	for _, entry := range page.Items {
		ids = append(ids, entry.ID)
	}
	return ids
}

// 一覧は played_at の新しい順で、同じ時刻は id の大きい順。nextCursor からの次のページは重複なく続き、
// 最後のページにはカーソルが無い。
func TestListWatchHistoryOrderAndPages(t *testing.T) {
	db := migratedDB(t)
	oldest := insertHistory(t, db, "key-a", "a", 1000)
	tieLow := insertHistory(t, db, "key-b", "b", 2000)
	tieHigh := insertHistory(t, db, "key-c", "c", 2000)
	newest := insertHistory(t, db, "key-d", "d", 3000)

	first := listHistory(t, db, domain.AudienceOwner, "", 2)
	if got, want := historyIDs(first), []int64{newest, tieHigh}; !slices.Equal(got, want) {
		t.Fatalf("1 ページ目 = %v, want %v", got, want)
	}
	if first.NextCursor == "" {
		t.Fatal("1 ページ目に nextCursor が無い")
	}
	if got := first.Items[0].PlayedAt.UnixMilli(); got != 3000 || first.Items[0].Title != "d" {
		t.Errorf("先頭 = %+v", first.Items[0])
	}
	second := listHistory(t, db, domain.AudienceOwner, first.NextCursor, 2)
	if got, want := historyIDs(second), []int64{tieLow, oldest}; !slices.Equal(got, want) {
		t.Fatalf("2 ページ目 = %v, want %v", got, want)
	}
	if second.NextCursor != "" {
		t.Errorf("最後のページの nextCursor = %q, want 空", second.NextCursor)
	}

	if _, err := db.Playback().ListWatchHistory(context.Background(), domain.AudienceOwner, domain.WatchHistoryQuery{}, "not a cursor", 2); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("読めないカーソル = %v, want ErrInvalidCursor", err)
	}
	if page := listHistory(t, migratedDB(t), domain.AudienceOwner, "", 60); len(page.Items) != 0 || page.NextCursor != "" {
		t.Errorf("空の履歴 = %+v", page)
	}
}

// 所在のある内容の件は動画を持ち、ライブラリに無い内容の件は持たない。読みは渡された見る人の条件で
// 動画を結ぶ（ゲストには公開の動画だけ）。代表でない集まりのメンバーの件は、そのメンバーを持つ。
func TestListWatchHistoryResolvesVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2),
		listingFile(fixturePath("/media/c.mp4"), "c", "key-c", 3),
	)
	a, b, c := ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")], ids[fixturePath("/media/c.mp4")]
	if _, err := db.Versions().Bundle(ctx, []int64{a, b}, a); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{c}, true); err != nil {
		t.Fatal(err)
	}
	gone := insertHistory(t, db, "key-gone", "消えた", 1000)
	member := insertHistory(t, db, "key-b", "b の当時", 2000)
	public := insertHistory(t, db, "key-c", "c", 3000)

	videoOf := func(page domain.WatchHistoryPage) map[int64]int64 {
		out := map[int64]int64{}
		for _, entry := range page.Items {
			if entry.Video != nil {
				out[entry.ID] = entry.Video.ID
			}
		}
		return out
	}
	owner := listHistory(t, db, domain.AudienceOwner, "", 60)
	if got, want := videoOf(owner), map[int64]int64{member: b, public: c}; !maps.Equal(got, want) {
		t.Errorf("所有者の件の動画 = %v, want %v（%d は動画なし）", got, want, gone)
	}
	for _, entry := range owner.Items {
		if entry.ID == gone && entry.Title != "消えた" {
			t.Errorf("動画の無い件の題名 = %q", entry.Title)
		}
	}
	guest := listHistory(t, db, domain.AudienceGuest, "", 60)
	if got, want := videoOf(guest), map[int64]int64{public: c}; !maps.Equal(got, want) {
		t.Errorf("ゲストの件の動画 = %v, want %v", got, want)
	}
}

// 1 件の削除はあれば true、もう一度で false。全件の削除は空でも誤りにならない。どちらも再生位置を
// 変えない（要件 9）。
func TestDeleteAndClearWatchHistory(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	savePlay(t, db, "key-a", 4000, &domain.Play{PlaybackID: testPlaybackA, ContentKey: "key-a", Title: "a"})
	savePlay(t, db, "key-a", 5000, &domain.Play{PlaybackID: testPlaybackB, ContentKey: "key-a", Title: "a"})
	page := listHistory(t, db, domain.AudienceOwner, "", 60)
	if len(page.Items) != 2 {
		t.Fatalf("履歴 = %+v, want 2 件", page.Items)
	}

	target := page.Items[0].ID
	for attempt, want := range []bool{true, false} {
		deleted, err := db.Playback().DeleteWatchHistoryEntry(ctx, target)
		if err != nil || deleted != want {
			t.Errorf("%d 回目の削除 = %v, %v, want %v", attempt+1, deleted, err, want)
		}
	}
	if rows := historyRows(t, db); len(rows) != 1 {
		t.Errorf("1 件消したあとの履歴 = %+v", rows)
	}
	for range 2 {
		if err := db.Playback().ClearWatchHistory(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if rows := historyRows(t, db); len(rows) != 0 {
		t.Errorf("全件消したあとの履歴 = %+v", rows)
	}
	if got := progressOf(t, db, "key-a"); got != 5000 {
		t.Errorf("再生位置 = %d, want 5000", got)
	}
}
