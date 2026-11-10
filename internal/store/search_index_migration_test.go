package store

import (
	"context"
	"reflect"
	"testing"
)

const searchIndexVersion = 100

// The jobs rebuild keeps every row, admits the frame_index kind only with a video, and keeps the candidate trigger.
func TestSearchIndexMigrationRebuildsJobs(t *testing.T) {
	db, provider := migrateTo(t, searchIndexVersion-1)
	ctx := context.Background()
	execAll(t, db,
		`insert into videos(id, content_key, probe_state, added_at, indexed_at) values (1, 'key-a', 'done', 1, 1)`,
		`insert into jobs(id, kind, video_id, state, attempts, last_error, created_at, updated_at, location_id,
			location_version, location_path, activity_id, force, force_requested, priority, run_after)
			values (1, 'thumbnail', 1, 'running', 2, 'ffmpeg failed', 100, 150, 7, 3, '/media/a.mp4', 9, 0, 1, 0, 120)`,
		`insert into jobs(id, kind, subject_key, subject_key_b, state, created_at, updated_at, priority)
			values (2, 'version_judge', 'key-a', 'key-b', 'queued', 200, 200, 1)`,
		`insert into video_version_candidates(key_a, key_b, distance, created_at) values ('key-a', 'key-b', 3, 1)`,
	)
	before := jobRows(t, db.sql)

	if _, err := provider.UpTo(ctx, searchIndexVersion); err != nil {
		t.Fatal(err)
	}
	if got := jobRows(t, db.sql); !reflect.DeepEqual(got, before) {
		t.Fatalf("rows after up = %v, want %v", got, before)
	}
	execAll(t, db,
		`insert into jobs(id, kind, video_id, state, created_at, updated_at, priority) values (3, 'frame_index', 1, 'queued', 1, 1, 2)`,
		`insert into jobs(id, kind, state, created_at, updated_at, priority) values (4, 'queue_frame_index', 'queued', 1, 1, 2)`,
		`insert into video_search_index(content_key, version, entries, updated_at) values ('key-a', 1, x'', 1)`,
	)
	if _, err := db.sql.Exec(`insert into jobs(kind, state, created_at, updated_at) values ('frame_index', 'queued', 1, 1)`); err == nil {
		t.Fatal("a frame_index job without a video was accepted")
	}
	execAll(t, db, `delete from video_version_candidates`)
	if n := countRows(t, db, `select count(*) from jobs where kind = 'version_judge'`); n != 0 {
		t.Fatalf("version_judge jobs after their candidate went = %d, want 0", n)
	}

	if _, err := provider.DownTo(ctx, searchIndexVersion-1); err != nil {
		t.Fatal(err)
	}
	if got := jobRows(t, db.sql); !reflect.DeepEqual(got, before[:1]) {
		t.Fatalf("rows after down = %v, want %v", got, before[:1])
	}
}
