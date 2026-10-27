package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// nextBackfill runs kind's queue-all pass, as its startup job does, and claims the first job it queued. Jobs of kind
// left by an earlier call are deleted first, as their completion would delete them.
func nextBackfill(ctx context.Context, db *DB, kind domain.JobKind) (domain.Job, error) {
	if _, err := db.sql.ExecContext(ctx, `delete from jobs where kind = ?`, string(kind)); err != nil {
		return domain.Job{}, err
	}
	queue := db.Ingest().QueueKeyframeJobs
	if kind == domain.JobMediaInfo {
		queue = db.Ingest().QueueMediaInfoJobs
	}
	if _, err := queue(ctx, domain.JobPriorityBackfill); err != nil {
		return domain.Job{}, err
	}
	return db.Ingest().ClaimJob(ctx, kind)
}

func TestQueueKeyframeJobsQueuesOnlyH264VideosOnce(t *testing.T) {
	db, h264, _ := keyframeFixture(t)
	ctx := context.Background()
	other, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/b.mp4"), "b", "key-b", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	hevc := domain.Probe{DurationMs: 300000, VideoCodec: "hevc"}
	if err := db.Ingest().ApplyProbe(ctx, other.ID, hevc, domain.EvaluatePlayability("mp4", hevc)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{1, 0} {
		if n, err := db.Ingest().QueueKeyframeJobs(ctx, domain.JobPriorityBackfill); err != nil || n != want {
			t.Fatalf("QueueKeyframeJobs = %d, %v, want %d (the second pass keeps the queued job)", n, err, want)
		}
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobKeyframes)
	if err != nil || job.VideoID != h264 || job.Priority != domain.JobPriorityBackfill {
		t.Fatalf("claimed = %+v, %v, want the H.264 video in the backfill class", job, err)
	}
	if _, err := db.Ingest().ClaimJob(ctx, domain.JobKeyframes); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("second claim: %v, want ErrNoJob", err)
	}
}

func TestBackfillJobStaysOutOfImportTracking(t *testing.T) {
	db, id, _ := keyframeFixture(t)
	ctx := context.Background()
	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := addScanVideos(ctx, db.sql, id); err != nil {
		t.Fatal(err)
	}
	finishScan(t, db, scan.ID)
	settled, _ := currentImport(t, db)
	if settled.SettledAt.IsZero() {
		t.Fatal("the import with no work left did not settle")
	}
	entries := countRows(t, db, `select count(*) from activity_entries`)

	if n, err := db.Ingest().QueueKeyframeJobs(ctx, domain.JobPriorityBackfill); err != nil || n != 1 {
		t.Fatalf("QueueKeyframeJobs = %d, %v", n, err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobKeyframes)
	if err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `select count(*) from activity_entries`); got != entries {
		t.Errorf("activity entries = %d, want %d: a running backfill job opened an entry", got, entries)
	}
	if scan, progress := currentImport(t, db); !scan.SettledAt.Equal(settled.SettledAt) ||
		progress.Status != domain.ImportDone {
		t.Errorf("import while the backfill runs = %+v (settled_at %v), want done at %v",
			progress, scan.SettledAt, settled.SettledAt)
	}
	if unfinished, err := db.Ingest().HasUnfinishedJobs(ctx); err != nil || unfinished {
		t.Errorf("HasUnfinishedJobs = %v, %v, want false", unfinished, err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `select count(*) from jobs`); got != 0 {
		t.Errorf("jobs after completion = %d, want the backfill job deleted", got)
	}
}

func TestStartupJobsQueueOnceAndEndWhenFinished(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	for range 2 {
		if err := db.Ingest().EnqueueStartupJobs(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := countRows(t, db, `select count(*) from jobs`); got != int64(len(domain.StartupJobKinds)) {
		t.Fatalf("jobs = %d, want one per startup kind while unfinished", got)
	}

	done, err := db.Ingest().ClaimJob(ctx, domain.JobVirtualThumbnails)
	if err != nil || done.Priority != domain.JobPriorityBackfill || done.Attempts != 1 {
		t.Fatalf("claimed = %+v, %v", done, err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, done); err != nil {
		t.Fatal(err)
	}
	failed, err := db.Ingest().ClaimJob(ctx, domain.JobQueueKeyframes)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, failed, domain.ErrFailureRepeats); err != nil {
		t.Fatal(err)
	}
	var left string
	if err := db.sql.QueryRowContext(ctx, `select group_concat(kind) from jobs`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != string(domain.JobQueueMediaInfo) {
		t.Fatalf("jobs left = %v, want only the unclaimed queue_media_info", left)
	}
}
