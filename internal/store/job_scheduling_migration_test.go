package store

import (
	"context"
	"database/sql"
	"io/fs"
	"reflect"
	"testing"

	"github.com/pressly/goose/v3"
)

const jobSchedulingVersion = 83

// Existing rows keep their state, attempts, error, activity and force flags across the jobs rebuild and back.
func TestJobSchedulingMigrationRoundTripsExistingRows(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, jobSchedulingVersion-1); err != nil {
		t.Fatal(err)
	}
	execAll(t, db,
		`insert into videos(id, content_key, probe_state, added_at, indexed_at) values (1, 'key-a', 'done', 1, 1)`,
		`insert into jobs(id, kind, video_id, state, attempts, last_error, created_at, updated_at, location_id,
			location_version, location_path, activity_id, force, force_requested)
			values (1, 'thumbnail', 1, 'running', 2, 'ffmpeg failed', 100, 150, 7, 3, '/media/a.mp4', 9, 0, 1),
			       (2, 'probe', 1, 'done', 1, null, 200, 250, null, null, null, null, 1, 0),
			       (3, 'preview', 1, 'failed', 3, 'broken', 300, 350, 7, 3, '/media/a.mp4', 9, 0, 0)`,
	)
	before := jobRows(t, db.sql)

	if _, err := provider.UpTo(ctx, jobSchedulingVersion); err != nil {
		t.Fatal(err)
	}
	if got := jobRows(t, db.sql); !reflect.DeepEqual(got, before) {
		t.Fatalf("rows after up = %v, want %v", got, before)
	}
	if n := countRows(t, db, `select count(*) from jobs where priority = 1 and run_after = created_at`); n != 3 {
		t.Fatalf("rows in the import class due at their creation = %d, want 3", n)
	}

	if _, err := provider.DownTo(ctx, jobSchedulingVersion-1); err != nil {
		t.Fatal(err)
	}
	if got := jobRows(t, db.sql); !reflect.DeepEqual(got, before) {
		t.Fatalf("rows after down = %v, want %v", got, before)
	}
	if _, err := provider.UpTo(ctx, jobSchedulingVersion); err != nil {
		t.Fatal(err)
	}
	if got := jobRows(t, db.sql); !reflect.DeepEqual(got, before) {
		t.Fatalf("rows after up again = %v, want %v", got, before)
	}
}

func jobRows(t *testing.T, q *sql.DB) [][]any {
	t.Helper()
	rows, err := q.Query(`select id, kind, video_id, state, attempts, last_error, created_at, updated_at, location_id,
		location_version, location_path, activity_id, force, force_requested from jobs order by id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got [][]any
	for rows.Next() {
		row := make([]any, 14)
		pointers := make([]any, len(row))
		for i := range row {
			pointers[i] = &row[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}
