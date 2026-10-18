package store

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

func entryPipelines(t *testing.T, db *DB, entryID int64) []int64 {
	t.Helper()
	ids, err := collectActivityIDs(db.sql.Query(`select pipeline_id from activity_pipeline_members where entry_id = ?
		order by pipeline_id`, entryID))
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func activityEntry(t *testing.T, db *DB, id int64) domain.ActivityEntry {
	t.Helper()
	entry, err := db.Activity().ActivityEntry(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

// onlyPipeline asserts the entry belongs to exactly one pipeline and returns it.
func onlyPipeline(t *testing.T, db *DB, entryID int64) domain.ActivityEntry {
	t.Helper()
	pipelines := entryPipelines(t, db, entryID)
	if len(pipelines) != 1 {
		t.Fatalf("pipelines of entry %d = %v, want one", entryID, pipelines)
	}
	return activityEntry(t, db, pipelines[0])
}

func pipelineCount(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from activity_entries where kind = 'pipeline'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestScanOpensOnePipelinePerQueuedVideo(t *testing.T) {
	db := migratedDB(t)
	scan, _, err := db.Scans().StartScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	queueNewFile(t, db, "first", domain.ActivityByOwner)
	queueNewFile(t, db, "second", domain.ActivityByOwner)

	entries := importEntries(t, db)
	if len(entries) != 2 {
		t.Fatalf("import entries = %+v, want two", entries)
	}
	seen := map[int64]bool{}
	for _, entry := range entries {
		pipeline := onlyPipeline(t, db, entry.ID)
		seen[pipeline.ID] = true
		if pipeline.Kind != domain.ActivityPipeline || pipeline.Feature != domain.ActivityFeatureImport ||
			!pipeline.Subject.SameAs(entry.Subject) || pipeline.Subject.Label != entry.Subject.Label ||
			pipeline.Decider != domain.ActivityByOwner || entryState(pipeline) != activityWaiting {
			t.Errorf("pipeline of %s = %+v, want a running/waiting owner pipeline on the same video",
				entry.Subject.Key, pipeline)
		}
		if pipeline.ScanID != scan.ID {
			t.Errorf("pipeline scan of %s = %d, want %d", entry.Subject.Key, pipeline.ScanID, scan.ID)
		}
	}
	if len(seen) != 2 {
		t.Errorf("pipelines = %v, want one per video", seen)
	}
}

func TestRescanAfterAFinishedRunOpensASecondPipeline(t *testing.T) {
	db := migratedDB(t)
	videoID := upsertForImport(t, db, "again")
	runImportStage(t, db, domain.JobProbe)
	runImportStage(t, db, domain.JobPreview)
	first := importEntries(t, db)[0]
	firstPipeline := onlyPipeline(t, db, first.ID)
	if entryState(firstPipeline) != activityDone {
		t.Fatalf("first pipeline = %+v, want done/completed", firstPipeline)
	}

	if err := db.Ingest().EnqueueJob(context.Background(), domain.JobProbe, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	second := importEntries(t, db)[0]
	secondPipeline := onlyPipeline(t, db, second.ID)
	if second.ID == first.ID || secondPipeline.ID == firstPipeline.ID || entryState(secondPipeline) != activityWaiting {
		t.Errorf("second run = entry %d in pipeline %+v, want a new running/waiting pipeline", second.ID, secondPipeline)
	}
	if got := activityEntry(t, db, firstPipeline.ID); entryState(got) != activityDone {
		t.Errorf("first pipeline after the rescan = %+v, want still done/completed", got)
	}
}

func TestRetryImportJoinsThePipelineOfTheRetriedEntry(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	queueNewFile(t, db, "retry", domain.ActivityByRule)
	giveUpStage(t, db, domain.JobProbe, errors.New("ffprobe failed"))
	for _, kind := range []domain.JobKind{domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobFingerprint} {
		runImportStage(t, db, kind)
	}
	failed := importEntries(t, db)[0]
	pipeline := onlyPipeline(t, db, failed.ID)
	if entryState(pipeline) != activityFailed {
		t.Fatalf("pipeline = %+v, want attention/failed", pipeline)
	}

	if err := db.Ingest().RetryImport(ctx, failed.ID, "key-retry"); err != nil {
		t.Fatal(err)
	}
	retried := importEntries(t, db)[0]
	if got := onlyPipeline(t, db, retried.ID); got.ID != pipeline.ID || entryState(got) != activityWaiting {
		t.Errorf("retried entry's pipeline = %+v, want pipeline %d running/waiting", got, pipeline.ID)
	}
	if count := pipelineCount(t, db); count != 1 {
		t.Errorf("pipelines = %d, want the retry to open none", count)
	}
	var supersededBy int64
	if err := db.sql.QueryRow(`select superseded_by from activity_entries where id = ?`, failed.ID).
		Scan(&supersededBy); err != nil || supersededBy != retried.ID {
		t.Errorf("superseded_by of the failed entry = %d, %v; want %d", supersededBy, err, retried.ID)
	}

	runImportStage(t, db, domain.JobProbe)
	runImportStage(t, db, domain.JobPreview)
	if got := activityEntry(t, db, pipeline.ID); entryState(got) != activityDone {
		t.Errorf("pipeline after the retry finished = %+v, want done/completed with the replaced member ignored", got)
	}
}

func TestClaimReopenJoinsThePipelineOfTheInterruptedEntry(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	queueNewFile(t, db, "restart", domain.ActivityByOwner)
	if _, err := db.Ingest().ClaimJob(ctx, domain.JobProbe); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Activity().RecoverInterruptedActivity(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().RequeueRunningJobs(ctx); err != nil {
		t.Fatal(err)
	}
	interrupted := importEntries(t, db)[0]
	pipeline := onlyPipeline(t, db, interrupted.ID)
	if want := (domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityInterrupted}); entryState(pipeline) != want {
		t.Fatalf("pipeline after restart = %+v, want %+v", pipeline, want)
	}

	runImportStage(t, db, domain.JobProbe)
	reopened := importEntries(t, db)[0]
	if reopened.ID == interrupted.ID {
		t.Fatal("the claim did not reopen the interrupted entry")
	}
	if got := onlyPipeline(t, db, reopened.ID); got.ID != pipeline.ID || entryState(got) != activityClaimed {
		t.Errorf("reopened entry's pipeline = %+v, want pipeline %d running/processing", got, pipeline.ID)
	}
	if got := pipelineCount(t, db); got != 1 {
		t.Errorf("pipelines = %d, want the claim to open none", got)
	}
}

// recordFollowUp writes a follow-up entry for the claimed job inside one transaction, as a stage result does.
func recordFollowUp(t *testing.T, db *DB, jobID int64, key string) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var c changes
	now := time.Now()
	id, err := recordActivity(ctx, tx, &c, autoTagActivityEntry(key, key, domain.ActivityWaiting, domain.ActivityByRule), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := joinJobPipelines(ctx, tx, jobID, id, now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestFollowUpJoinsThePipelinesOfTheJobsImportEntry(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	queueNewFile(t, db, "follow", domain.ActivityByRule)
	importEntry := importEntries(t, db)[0]
	pipeline := onlyPipeline(t, db, importEntry.ID)
	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}

	followUp := recordFollowUp(t, db, job.ID, "key-follow")
	if got := entryPipelines(t, db, followUp); !slices.Equal(got, []int64{pipeline.ID}) {
		t.Errorf("follow-up pipelines = %v, want [%d]", got, pipeline.ID)
	}

	execSQL(t, db, `update jobs set activity_id = null where id = ?`, job.ID)
	legacy := recordFollowUp(t, db, job.ID, "key-legacy")
	if got := entryPipelines(t, db, legacy); len(got) != 0 {
		t.Errorf("follow-up of a job without an entry joined %v, want none", got)
	}
}

func importStages(t *testing.T, db *DB, entryID int64) map[domain.JobKind]domain.ActivityStage {
	t.Helper()
	rows, err := db.sql.Query(`select stage, state, coalesce(detail, '') from activity_entry_stages where entry_id = ?`,
		entryID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	stages := map[domain.JobKind]domain.ActivityStage{}
	for rows.Next() {
		var stage domain.ActivityStage
		var kind, state string
		if err := rows.Scan(&kind, &state, &stage.Detail); err != nil {
			t.Fatal(err)
		}
		stage.Stage, stage.State = domain.JobKind(kind), domain.ActivityStageState(state)
		stages[stage.Stage] = stage
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return stages
}

func TestImportStagesStayReadableAfterTheVideoIsDeleted(t *testing.T) {
	db := migratedDB(t)
	videoID := queueNewFile(t, db, "deleted", domain.ActivityByRule)
	runImportStage(t, db, domain.JobProbe)
	giveUpStage(t, db, domain.JobThumbnail, errors.New("no frame"))
	entry := importEntries(t, db)[0]
	pipeline := onlyPipeline(t, db, entry.ID)

	if err := db.ScanIndex().DeleteVideos(context.Background(), []int64{videoID}); err != nil {
		t.Fatal(err)
	}
	want := map[domain.JobKind]domain.ActivityStage{
		domain.JobProbe:         {Stage: domain.JobProbe, State: domain.ActivityStageDone},
		domain.JobThumbnail:     {Stage: domain.JobThumbnail, State: domain.ActivityStageFailed, Detail: "no frame"},
		domain.JobSeekThumbnail: {Stage: domain.JobSeekThumbnail, State: domain.ActivityStageGone},
		domain.JobPreview:       {Stage: domain.JobPreview, State: domain.ActivityStageGone},
	}
	got := importStages(t, db, entry.ID)
	if len(got) != len(want) {
		t.Errorf("stages = %+v, want %+v (no fingerprint row: the entry never held one)", got, want)
	}
	for kind, stage := range want {
		if got[kind] != stage {
			t.Errorf("stage %s = %+v, want %+v", kind, got[kind], stage)
		}
	}
	gone := domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityGone}
	if got := activityEntry(t, db, pipeline.ID); entryState(got) != gone {
		t.Errorf("pipeline = %+v, want done/gone", got)
	}
}

func TestImportRunningBeforeTheMigrationStaysWithoutAPipeline(t *testing.T) {
	ctx := context.Background()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	files, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 63); err != nil {
		t.Fatal(err)
	}
	execSQL(t, db, `insert into videos(id, content_key) values (1, 'key-old')`)
	execSQL(t, db, `insert into activity_entries (id, kind, feature, subject_kind, subject_key, subject_label, status,
		reason, decider, created_at, updated_at)
		values (7, 'import', 'import', 'video', 'key-old', 'old', 'running', 'waiting', 'rule', 1, 1)`)
	execSQL(t, db, `insert into jobs (kind, video_id, state, attempts, created_at, updated_at, activity_id)
		values ('probe', 1, 'queued', 0, 1, 1, 7)`)
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	if err := db.Ingest().EnqueueJob(ctx, domain.JobThumbnail, 1, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	var attached int64
	if err := db.sql.QueryRow(`select activity_id from jobs where kind = 'thumbnail'`).Scan(&attached); err != nil ||
		attached != 7 {
		t.Fatalf("new job's entry = %d, %v; want the running entry 7", attached, err)
	}
	if got := entryPipelines(t, db, 7); len(got) != 0 {
		t.Errorf("pipelines of the earlier entry = %v, want none", got)
	}
	if got := pipelineCount(t, db); got != 0 {
		t.Errorf("pipelines = %d, want none", got)
	}
}
