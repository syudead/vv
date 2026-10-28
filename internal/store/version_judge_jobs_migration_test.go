package store

import (
	"context"
	"io/fs"
	"reflect"
	"testing"

	"github.com/pressly/goose/v3"
)

const versionJudgeJobsVersion = 85

// Waiting, interrupted and failed pairs become version_judge jobs, a failed pair keeping its attempts and retry time;
// going back restores the candidates' retry columns, with an interrupted pair waiting again.
func TestVersionJudgeJobsMigrationRoundTripsTheCandidates(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, versionJudgeJobsVersion-1); err != nil {
		t.Fatal(err)
	}
	execAll(t, db, `insert into video_version_candidates
		(key_a, key_b, distance, created_at, judgement, attempts, retry_at, judgement_error, activity_id) values
		('a', 'b', 1, 100, null, 0, null, null, 5),
		('a', 'c', 1, 200, 'running', 2, null, null, 6),
		('a', 'd', 1, 300, 'failed', 3, 5000, 'down', 7),
		('a', 'e', 1, 400, 'no_connection', 0, null, null, null),
		('a', 'f', 1, 500, 'undecided', 0, null, null, null)`)

	if _, err := provider.UpTo(ctx, versionJudgeJobsVersion); err != nil {
		t.Fatal(err)
	}
	wantJobs := [][]any{
		{"a", "b", "queued", int64(0), int64(1), int64(100), int64(5)},
		{"a", "c", "queued", int64(0), int64(1), int64(200), int64(6)},
		{"a", "d", "queued", int64(3), int64(1), int64(5000), int64(7)},
	}
	if got := queryRows(t, db.sql, `select subject_key, subject_key_b, state, attempts, priority, run_after, activity_id
		from jobs where kind = 'version_judge' order by id`, 7); !reflect.DeepEqual(got, wantJobs) {
		t.Fatalf("version_judge jobs = %v, want %v", got, wantJobs)
	}
	wantCandidates := [][]any{{"b", nil}, {"c", nil}, {"d", "failed"}, {"e", "no_connection"}, {"f", "undecided"}}
	if got := queryRows(t, db.sql, `select key_b, judgement from video_version_candidates order by key_b`,
		2); !reflect.DeepEqual(got, wantCandidates) {
		t.Fatalf("candidates = %v, want %v", got, wantCandidates)
	}
	execAll(t, db, `delete from video_version_candidates where key_b = 'b'`)
	if n := countRows(t, db, `select count(*) from jobs where kind = 'version_judge'`); n != 2 {
		t.Fatalf("jobs after deleting a candidate = %d, want 2", n)
	}

	if _, err := provider.DownTo(ctx, versionJudgeJobsVersion-1); err != nil {
		t.Fatal(err)
	}
	wantDown := [][]any{
		{"c", nil, int64(0), nil},
		{"d", "failed", int64(3), int64(5000)},
		{"e", "no_connection", int64(0), nil},
		{"f", "undecided", int64(0), nil},
	}
	if got := queryRows(t, db.sql, `select key_b, judgement, attempts, retry_at from video_version_candidates
		order by key_b`, 4); !reflect.DeepEqual(got, wantDown) {
		t.Fatalf("candidates after down = %v, want %v", got, wantDown)
	}
	if n := countRows(t, db, `select count(*) from jobs where kind = 'version_judge'`); n != 0 {
		t.Fatalf("version_judge jobs after down = %d, want 0", n)
	}
}
