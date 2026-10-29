package store

import (
	"context"
	"io/fs"
	"reflect"
	"testing"

	"github.com/pressly/goose/v3"
)

const autoTagJobsVersion = 85

// Unfinished auto-tagging work becomes auto_tag jobs and outcomes become results; going back restores the queue,
// with an interrupted row queued again as startup recovery would do.
func TestAutoTagJobsMigrationRoundTripsTheQueue(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, autoTagJobsVersion-1); err != nil {
		t.Fatal(err)
	}
	execAll(t, db, `insert into auto_tag_queue (content_key, state, error, queued_at, updated_at, activity_id)
		values ('key-queued', 'queued', '', 100000, 100000, 5),
		       ('key-running', 'running', '', 200000, 250000, 6),
		       ('key-done', 'done', '', 300000, 300000, null),
		       ('key-failed', 'failed', 'boom', 400000, 400000, null)`)

	if _, err := provider.UpTo(ctx, autoTagJobsVersion); err != nil {
		t.Fatal(err)
	}
	wantJobs := [][]any{
		{"key-queued", "queued", int64(0), int64(1), int64(100), int64(100), int64(5)},
		{"key-running", "queued", int64(0), int64(1), int64(200), int64(200), int64(6)},
	}
	if got := queryRows(t, db.sql, `select subject_key, state, attempts, priority, run_after, created_at, activity_id
		from jobs where kind = 'auto_tag' order by id`, 7); !reflect.DeepEqual(got, wantJobs) {
		t.Fatalf("auto_tag jobs = %v, want %v", got, wantJobs)
	}
	wantResults := [][]any{{"key-done", "done", ""}, {"key-failed", "failed", "boom"}}
	if got := queryRows(t, db.sql, `select content_key, state, error from auto_tag_results order by content_key`,
		3); !reflect.DeepEqual(got, wantResults) {
		t.Fatalf("results = %v, want %v", got, wantResults)
	}

	if _, err := provider.DownTo(ctx, autoTagJobsVersion-1); err != nil {
		t.Fatal(err)
	}
	wantQueue := [][]any{
		{"key-done", "done", "", nil},
		{"key-failed", "failed", "boom", nil},
		{"key-queued", "queued", "", int64(5)},
		{"key-running", "queued", "", int64(6)},
	}
	if got := queryRows(t, db.sql, `select content_key, state, error, activity_id from auto_tag_queue
		order by content_key`, 4); !reflect.DeepEqual(got, wantQueue) {
		t.Fatalf("queue after down = %v, want %v", got, wantQueue)
	}
	if n := countRows(t, db, `select count(*) from auto_tag_queue where state = 'queued' and queued_at in (100000, 200000)`); n != 2 {
		t.Fatalf("queued rows keeping their queue time = %d, want 2", n)
	}
	if n := countRows(t, db, `select count(*) from jobs where kind = 'auto_tag'`); n != 0 {
		t.Fatalf("auto_tag jobs after down = %d, want 0", n)
	}
}
