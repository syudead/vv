package store

import (
	"context"
	"io/fs"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// 00052 が積んだ metadata の仕事と metadata_failed の問題は、00053 が消す。00052 を適用済みの
// データベースでは、対応するワーカーの無い仕事が直近の走査の残りに数えられ続けていたが、
// 移行のあとの起動時の積み直しで直近の走査が完了する。
func TestDropMetadataJobsMigrationSettlesLatestScan(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsys)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 51); err != nil {
		t.Fatal(err)
	}
	execAll(t, db,
		`insert into media_folders(path, version, created_at, updated_at) values ('`+fixturePath("/media")+`', 1, 1, 1)`,
		`insert into videos(id, content_key, probe_state, added_at, indexed_at) values (1, 'key-a', 'done', 1, 1)`,
		`insert into video_locations(video_id, path, title, size_bytes, mtime, created_at, updated_at)
			values (1, '`+fixturePath("/media/a.mp4")+`', 'a', 1, 1, 1, 1)`,
		`insert into jobs(kind, video_id, state, attempts, created_at, updated_at) values ('probe', 1, 'done', 1, 1, 1)`,
		`insert into scans(id, state, started_at, finished_at, total, completed, failed) values (1, 'done', 1, 5, 1, 1, 0)`,
		`insert into scan_videos(scan_id, video_id) values (1, 1)`,
	)
	// 以前の 00052 は、解析が done の動画に metadata の仕事を積む。
	if _, err := provider.UpTo(ctx, 52); err != nil {
		t.Fatal(err)
	}
	execAll(t, db,
		`insert into scan_issues(scan_id, video_id, path, kind, created_at)
			values (1, 1, '`+fixturePath("/media/a.mp4")+`', 'metadata_failed', 1)`,
	)
	if n := countRows(t, db, `select count(*) from jobs where kind = 'metadata' and state = 'queued'`); n != 1 {
		t.Fatalf("準備: 00052 が積んだ metadata の仕事 = %d, want 1", n)
	}
	if _, progress := currentImport(t, db); progress.Status == domain.ImportDone {
		t.Fatalf("準備: 00052 のあとの進捗 = %+v, want 完了していない", progress)
	}
	revision := countRows(t, db, `select issues_revision from scans where id = 1`)

	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	if n := countRows(t, db, `select count(*) from jobs where kind = 'metadata'`); n != 0 {
		t.Errorf("移行後の metadata の仕事 = %d, want 0", n)
	}
	if n := countRows(t, db, `select count(*) from scan_issues where kind = 'metadata_failed'`); n != 0 {
		t.Errorf("移行後の metadata_failed の問題 = %d, want 0", n)
	}
	if got := countRows(t, db, `select issues_revision from scans where id = 1`); got != revision+1 {
		t.Errorf("issues_revision = %d, want %d", got, revision+1)
	}
	if n := countRows(t, db, `select count(*) from jobs where kind = 'probe'`); n != 1 {
		t.Errorf("移行後の probe の仕事 = %d, want 1（消さない）", n)
	}

	if _, err := db.Ingest().RequeueRunningJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if scan, progress := currentImport(t, db); scan.SettledAt.IsZero() || progress.Status != domain.ImportDone {
		t.Fatalf("起動時の積み直しのあと = %+v（settled_at %v）, want 完了", progress, scan.SettledAt)
	}
}

func execAll(t *testing.T, db *DB, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := db.sql.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func countRows(t *testing.T, db *DB, query string) int64 {
	t.Helper()
	var n int64
	if err := db.sql.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}
