package store

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestMediaFolderMigrationPreservesExistingLibrary(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.SQL(), fsy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	res, err := db.SQL().Exec(`insert into videos(path, title, size_bytes, mtime, content_key, added_at, updated_at)
		values ('/media/a.mp4', 'a', 10, 20, 'key-a', 30, 40)`)
	if err != nil {
		t.Fatal(err)
	}
	videoID, _ := res.LastInsertId()
	if _, err := db.SQL().Exec(`insert into jobs(kind, video_id, state, created_at, updated_at)
		values ('probe', ?, 'queued', 1, 1)`, videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`insert into playback_progress(content_key, position_ms, completed, updated_at)
		values ('key-a', 99, 0, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var migratedID int64
	var migratedPath, migratedKey string
	if err := db.SQL().QueryRow(`select v.id, l.path, v.content_key from videos v join video_locations l on l.video_id = v.id where v.id = ?`, videoID).
		Scan(&migratedID, &migratedPath, &migratedKey); err != nil {
		t.Fatal(err)
	}
	if migratedID != videoID || migratedPath != "/media/a.mp4" || migratedKey != "key-a" {
		t.Fatalf("migrated video = %d %q %q", migratedID, migratedPath, migratedKey)
	}
	var jobs, progress int
	if err := db.SQL().QueryRow(`select count(*) from jobs where video_id = ?`, videoID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := db.SQL().QueryRow(`select count(*) from playback_progress where content_key = 'key-a'`).Scan(&progress); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || progress != 1 {
		t.Fatalf("jobs=%d progress=%d, want 1 and 1", jobs, progress)
	}
}

// FR-005: 初回起動でスキーマが手作業なしに適用される。
func TestMigrateAppliesSchemaOnEmptyDirectory(t *testing.T) {
	dataDir := t.TempDir()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatalf("空のディレクトリから開けない: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	result, err := Migrate(context.Background(), db)
	if err != nil {
		t.Fatalf("マイグレーションに失敗した: %v", err)
	}
	if result.Applied == 0 {
		t.Error("適用された数が 0 件")
	}
	if result.Version < 1 {
		t.Errorf("適用後の版 = %d, want >= 1", result.Version)
	}

	for _, name := range []string{"videos", "videos_fts", versionTableName} {
		var count int
		err := db.SQL().QueryRow(
			`select count(*) from sqlite_master where name = ?`, name,
		).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Errorf("%s が作成されていない", name)
		}
	}
}

// 2 度目の呼び出しで何も適用されないこと（冪等）。
func TestMigrateIsIdempotent(t *testing.T) {
	dataDir := t.TempDir()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	first, err := Migrate(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Migrate(context.Background(), db)
	if err != nil {
		t.Fatalf("2 度目の適用で失敗した: %v", err)
	}

	if second.Applied != 0 {
		t.Errorf("2 度目に %d 件適用された, want 0", second.Applied)
	}
	if second.Version != first.Version {
		t.Errorf("版が変わった: %d -> %d", first.Version, second.Version)
	}
}

// SC-006: データベースファイルを削除しても、手作業なしに次の起動で復旧する。
func TestMigrateRecoversAfterDatabaseFileIsDeleted(t *testing.T) {
	dataDir := t.TempDir()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// WAL の付随ファイルも含めて消す。利用者が丸ごと削除した状況を再現する。
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := DatabasePath(dataDir) + suffix
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(DatabasePath(dataDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("削除できていない: %v", err)
	}

	reopened, err := Open(dataDir)
	if err != nil {
		t.Fatalf("削除後に開けない: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	result, err := Migrate(context.Background(), reopened)
	if err != nil {
		t.Fatalf("削除後の復旧に失敗した: %v", err)
	}
	if result.Applied == 0 {
		t.Error("復旧時に何も適用されていない")
	}
}

// ダウングレードによる破壊を防ぐ。将来の版を持つデータベースは書き換えずに中止する。
func TestMigrateAbortsOnFutureSchemaWithoutWriting(t *testing.T) {
	dataDir := t.TempDir()

	db, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	// アプリケーションが知らない将来の版が適用済みである状況を作る。
	const futureVersion = 9999
	_, err = db.SQL().Exec(
		`insert into `+versionTableName+` (version_id, is_applied, tstamp) values (?, 1, current_timestamp)`,
		futureVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	// 将来の版が触るはずのない印を置き、書き換えられないことを確かめる。
	if _, err := db.SQL().Exec(`insert into videos(added_at, content_key, updated_at) values (1, 'sentinel', 1)`); err != nil {
		t.Fatal(err)
	}

	_, err = Migrate(context.Background(), db)
	if err == nil {
		t.Fatal("将来の版を持つデータベースで成功してしまった")
	}
	if !errors.Is(err, ErrFutureSchema) {
		t.Errorf("err = %v, want ErrFutureSchema", err)
	}
	if !strings.Contains(err.Error(), "9999") {
		t.Errorf("出力にデータベース側の版が含まれていない:\n%s", err)
	}

	var version int64
	if err := db.SQL().QueryRow(
		`select max(version_id) from ` + versionTableName,
	).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != futureVersion {
		t.Errorf("記録された版が書き換えられた: %d, want %d", version, futureVersion)
	}

	var rows int
	if err := db.SQL().QueryRow(`select count(*) from videos`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("既存のデータが失われた: %d 行, want 1", rows)
	}
}

// データベースのパスは DataDir/mdm.db に固定する（設定項目にしない）。
func TestDatabasePathIsFixedUnderDataDir(t *testing.T) {
	got := DatabasePath("/data")
	want := filepath.Join("/data", DatabaseFileName)
	if got != want {
		t.Errorf("DatabasePath = %q, want %q", got, want)
	}
	if DatabaseFileName != "mdm.db" {
		t.Errorf("DatabaseFileName = %q, want mdm.db", DatabaseFileName)
	}
}

// 00002 で足した列が揃っていること。取り込み・再生可否・サムネイルの状態は
// すべて videos の列として読めなければ、一覧が列を読むだけで描けない（SC-007）。
func TestMigrateAddsCoreColumnsToVideos(t *testing.T) {
	db := migratedDB(t)

	want := []string{
		"content_key", "duration_ms", "width", "height", "container",
		"video_codec", "audio_codec", "playable", "unplayable_reason",
		"probe_state", "probe_error", "thumbnail_state", "updated_at",
	}
	got := tableColumns(t, db, "videos")

	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("videos に %s 列が無い", name)
		}
	}

	// 解析前は「再生できない」側に倒す（R-103）。既定値がここで崩れると、
	// 未解析の動画が再生できるものとして一覧に出てしまう。
	if notNull, ok := got["playable"]; !ok || !notNull {
		t.Error("playable が not null ではない")
	}
	if notNull, ok := got["probe_state"]; !ok || !notNull {
		t.Error("probe_state が not null ではない")
	}
	if notNull, ok := got["thumbnail_state"]; !ok || !notNull {
		t.Error("thumbnail_state が not null ではない")
	}
}

// playback_progress は「再構築できない利用者データ」なので、索引側の videos に
// 引きずられて消えてはならない。外部キーを持たないこと自体が要件である
// （FR-025／data-model.md）。
func TestPlaybackProgressHasNoForeignKeyToVideos(t *testing.T) {
	db := migratedDB(t)

	rows, err := db.SQL().Query(`select "table" from pragma_foreign_key_list('playback_progress')`)
	if err != nil {
		t.Fatalf("外部キーを読み出せない: %v", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			t.Fatal(err)
		}
		t.Errorf("playback_progress が %s への外部キーを持っている。"+
			"動画が消えても再生位置は残さなければならない（FR-025）", target)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// 動画を消しても再生位置が残ること。外部キーを張らない判断が、実際の削除で
// 成立していることを確かめる。
func TestDeletingVideoKeepsPlaybackProgress(t *testing.T) {
	db := migratedDB(t)

	if _, err := db.SQL().Exec(`insert into videos(added_at, content_key, updated_at) values (1, 'key-a', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(
		`insert into playback_progress(content_key, position_ms, completed, updated_at)
		 values ('key-a', 4000, 0, 1)`,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.SQL().Exec(`delete from videos where content_key = 'key-a'`); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.SQL().QueryRow(
		`select count(*) from playback_progress where content_key = 'key-a'`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("動画の削除で再生位置が消えた: %d 行, want 1", count)
	}
}

// 同じ (kind, video_id) の未完了ジョブは1件だけ。再スキャンのたびにジョブを
// 積んでも待ち行列が膨らまないことを、制約として持つ（R-106）。
func TestJobsPartialUniqueIndexRejectsSecondPendingJob(t *testing.T) {
	db := migratedDB(t)

	if _, err := db.SQL().Exec(`insert into videos(added_at, content_key, updated_at) values (1, 'key-a', 1)`); err != nil {
		t.Fatal(err)
	}

	insertJob := func(state string) error {
		_, err := db.SQL().Exec(
			`insert into jobs(kind, video_id, state, created_at, updated_at)
			 select 'probe', id, ?, 1, 1 from videos where content_key = 'key-a'`, state)
		return err
	}

	if err := insertJob("queued"); err != nil {
		t.Fatalf("1件目を積めない: %v", err)
	}
	if err := insertJob("queued"); err == nil {
		t.Error("同じ (kind, video_id) の queued が2件積めてしまった")
	}
	if err := insertJob("running"); err == nil {
		t.Error("queued があるのに running を積めてしまった")
	}

	// 完了した行は制約の対象外。同じ対象を再解析できなければならない。
	if _, err := db.SQL().Exec(`update jobs set state = 'done' where state = 'queued'`); err != nil {
		t.Fatal(err)
	}
	if err := insertJob("queued"); err != nil {
		t.Errorf("完了後に積み直せない: %v", err)
	}
}

// 動画を消すとジョブは連鎖して消える。ジョブは索引側のデータである。
func TestDeletingVideoCascadesJobs(t *testing.T) {
	db := migratedDB(t)

	if _, err := db.SQL().Exec(`insert into videos(added_at, content_key, updated_at) values (1, 'key-a', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(
		`insert into jobs(kind, video_id, state, created_at, updated_at)
		 select 'probe', id, 'queued', 1, 1 from videos where content_key = 'key-a'`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL().Exec(`delete from videos where content_key = 'key-a'`); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.SQL().QueryRow(`select count(*) from jobs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("ジョブが連鎖削除されていない: %d 行, want 0", count)
	}
}

// running なスキャンは同時に1件だけ。POST /api/scans が「実行中ならそれを返す」
// 振る舞い（R-108）は、この制約に依存している。
func TestScansAllowOnlyOneRunning(t *testing.T) {
	db := migratedDB(t)

	insertScan := func(state string) error {
		_, err := db.SQL().Exec(
			`insert into scans(state, started_at) values (?, 1)`, state)
		return err
	}

	if err := insertScan("running"); err != nil {
		t.Fatalf("1件目の running を作れない: %v", err)
	}
	if err := insertScan("running"); err == nil {
		t.Error("running なスキャンが同時に2件作れてしまった")
	}

	// 終わったスキャンは何件あってもよい。履歴として残る。
	if _, err := db.SQL().Exec(`update scans set state = 'done' where state = 'running'`); err != nil {
		t.Fatal(err)
	}
	if err := insertScan("running"); err != nil {
		t.Errorf("終了後に次のスキャンを始められない: %v", err)
	}
	if err := insertScan("done"); err != nil {
		t.Errorf("done は複数持てなければならない: %v", err)
	}
}

// 位置は負にならない。クライアントの申告をそのまま入れても壊れない最後の防壁。
func TestPlaybackProgressRejectsNegativePosition(t *testing.T) {
	db := migratedDB(t)

	_, err := db.SQL().Exec(
		`insert into playback_progress(content_key, position_ms, completed, updated_at)
		 values ('key-a', -1, 0, 1)`)
	if err == nil {
		t.Error("負の position_ms が入ってしまった")
	}
}

// Down で 001 の状態へ戻ること。スキーマ変更を取り消せることは、
// 適用を自動化している以上（起動時に適用する）必要な出口である。
func TestMigrateDownReturnsToInitialSchema(t *testing.T) {
	db := migratedDB(t)

	for range 2 {
		if err := Down(context.Background(), db); err != nil {
			t.Fatalf("Down に失敗した: %v", err)
		}
	}

	// 002 が足した表は消えている。
	for _, name := range []string{"playback_progress", "jobs", "scans"} {
		var count int
		if err := db.SQL().QueryRow(
			`select count(*) from sqlite_master where type = 'table' and name = ?`, name,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s が残っている", name)
		}
	}

	// 002 が足した列も消えている。
	columns := tableColumns(t, db, "videos")
	for _, name := range []string{"content_key", "probe_state", "thumbnail_state"} {
		if _, ok := columns[name]; ok {
			t.Errorf("videos に %s 列が残っている", name)
		}
	}

	// 001 の表と索引は残っている。
	for _, name := range []string{"videos", "videos_fts"} {
		var count int
		if err := db.SQL().QueryRow(
			`select count(*) from sqlite_master where name = ?`, name,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Errorf("001 の %s が失われた", name)
		}
	}
}

func TestMediaFolderMigrationRejectsLossyDown(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/a/movie.mp4", "movie", "shared", 1, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, sampleFile("/media/b/movie.mp4", "movie", "shared", 1, 0)); err != nil {
		t.Fatal(err)
	}

	if err := Down(ctx, db); err == nil {
		t.Fatal("multiple locations were silently collapsed by Down")
	}
	var locations int
	if err := db.SQL().QueryRow(`select count(*) from video_locations`).Scan(&locations); err != nil {
		t.Fatalf("failed Down did not preserve the new schema: %v", err)
	}
	if locations != 2 {
		t.Fatalf("locations after failed Down = %d, want 2", locations)
	}
}

// migratedDB はマイグレーションを適用したデータベースを返す。
func migratedDB(t *testing.T) *DB {
	t.Helper()

	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("データベースを開けない: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("マイグレーションに失敗した: %v", err)
	}
	if _, err := db.SQL().Exec(`insert into media_folders(path, version, created_at, updated_at) values ('/media', 1, 1, 1)`); err != nil {
		t.Fatalf("テスト用メディアフォルダを登録できない: %v", err)
	}
	return db
}

// tableColumns は列名から「not null かどうか」への対応を返す。
func tableColumns(t *testing.T, db *DB, table string) map[string]bool {
	t.Helper()

	rows, err := db.SQL().Query(`select name, "notnull" from pragma_table_info(?)`, table)
	if err != nil {
		t.Fatalf("%s の列を読み出せない: %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	columns := map[string]bool{}
	for rows.Next() {
		var name string
		var notNull int
		if err := rows.Scan(&name, &notNull); err != nil {
			t.Fatal(err)
		}
		columns[name] = notNull == 1
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}
