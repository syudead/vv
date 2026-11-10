package store

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func frameIndexJobs(t *testing.T, db *DB) map[domain.JobPriority]int {
	t.Helper()
	rows, err := db.sql.Query(`select priority, count(*) from jobs where kind = 'frame_index' and state = 'queued'
		group by priority`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[domain.JobPriority]int{}
	for rows.Next() {
		var priority domain.JobPriority
		var n int
		if err := rows.Scan(&priority, &n); err != nil {
			t.Fatal(err)
		}
		got[priority] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func storedSearchIndex(t *testing.T, db *DB, key string) (domain.SearchIndex, bool) {
	t.Helper()
	var version int
	var entries []byte
	err := db.sql.QueryRow(`select version, entries from video_search_index where content_key = ?`, key).
		Scan(&version, &entries)
	if err != nil {
		return domain.SearchIndex{}, false
	}
	index, err := domain.DecodeSearchIndex(entries, version)
	if err != nil {
		t.Fatal(err)
	}
	return index, true
}

func applyProbe(t *testing.T, db *DB, job domain.Job) {
	t.Helper()
	probe := domain.Probe{DurationMs: 60_000, VideoCodec: "h264"}
	if written, err := db.Ingest().ApplyProbeForJob(context.Background(), job, probe,
		domain.Playability{Playable: true}); err != nil || !written {
		t.Fatalf("ApplyProbeForJob = %v, %v", written, err)
	}
}

// A probe result queues one backfill-class frame_index job; once the content has a current index, a forced rescan's
// probe queues none.
func TestProbeResultQueuesFrameIndexUntilTheContentIsIndexed(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	first := claimProbeJob(t, db, videoID)
	applyProbe(t, db, first)
	if err := db.Ingest().CompleteClaimedJob(ctx, first); err != nil {
		t.Fatal(err)
	}
	if got := frameIndexJobs(t, db); len(got) != 1 || got[domain.JobPriorityBackfill] != 1 {
		t.Fatalf("queued frame_index jobs by class = %v, want one backfill job", got)
	}

	job, err := db.Ingest().ClaimJob(ctx, domain.JobFrameIndex)
	if err != nil {
		t.Fatal(err)
	}
	index := domain.SearchIndex{Version: domain.SearchIndexVersion, Entries: []domain.SearchIndexEntry{{Hash: 7}}}
	if applied, err := db.Ingest().ApplySearchIndexForJob(ctx, job, index); err != nil || !applied {
		t.Fatalf("ApplySearchIndexForJob = %v, %v", applied, err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().ForceJobs(ctx, videoID, domain.ActivityByOwner, domain.JobProbe); err != nil {
		t.Fatal(err)
	}
	forced, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil || !forced.Force {
		t.Fatalf("forced probe = %+v, %v", forced, err)
	}
	applyProbe(t, db, forced)
	if got := frameIndexJobs(t, db); len(got) != 0 {
		t.Fatalf("queued frame_index jobs after a forced rescan = %v, want none", got)
	}
}

// A job whose content or location changed after the claim writes nothing; releasing the content deletes its row.
func TestApplySearchIndexForJobWritesOnlyForTheClaimedContent(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	applyProbe(t, db, claimProbeJob(t, db, videoID))
	job, err := db.Ingest().ClaimJob(ctx, domain.JobFrameIndex)
	if err != nil {
		t.Fatal(err)
	}
	index := domain.SearchIndex{Version: domain.SearchIndexVersion, Entries: []domain.SearchIndexEntry{
		{TimeMs: 0, Hash: 7}, {TimeMs: 2000, Hash: 9},
	}}
	otherContent, movedLocation := job, job
	otherContent.ContentKey = "key-other"
	movedLocation.LocationVersion++
	for name, stale := range map[string]domain.Job{"content": otherContent, "location": movedLocation} {
		if applied, err := db.Ingest().ApplySearchIndexForJob(ctx, stale, index); err != nil || applied {
			t.Fatalf("changed %s: applied = %v, %v, want false", name, applied, err)
		}
	}
	if n := countRows(t, db, `select count(*) from video_search_index`); n != 0 {
		t.Fatalf("rows after stale jobs = %d, want 0", n)
	}

	if applied, err := db.Ingest().ApplySearchIndexForJob(ctx, job, index); err != nil || !applied {
		t.Fatalf("ApplySearchIndexForJob = %v, %v", applied, err)
	}
	if got, ok := storedSearchIndex(t, db, job.ContentKey); !ok || len(got.Entries) != 2 || got.Entries[1] != index.Entries[1] {
		t.Fatalf("stored index = %+v, %v", got, ok)
	}
	if err := db.ScanIndex().DeleteVideos(ctx, []int64{videoID}); err != nil {
		t.Fatal(err)
	}
	if _, ok := storedSearchIndex(t, db, job.ContentKey); ok {
		t.Fatal("the index outlived its content's last reference")
	}
}
