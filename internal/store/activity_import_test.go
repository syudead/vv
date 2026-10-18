package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func importEntries(t *testing.T, db *DB) []domain.ActivityEntry {
	t.Helper()
	return entriesOfKind(t, db, domain.ActivityImport)
}

// entriesOfKind reads every entry of the kind newest first; the list shows pipeline members only inside their
// pipelines.
func entriesOfKind(t *testing.T, db *DB, kind domain.ActivityKind) []domain.ActivityEntry {
	t.Helper()
	ids, err := collectActivityIDs(db.sql.Query(`select id from activity_entries where kind = ? order by id desc`,
		string(kind)))
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]domain.ActivityEntry, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, activityEntry(t, db, id))
	}
	return entries
}

func scanEntries(t *testing.T, db *DB) []domain.ActivityEntry {
	t.Helper()
	page := listActivity(t, db, domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureScan}}, "", 50)
	return page.Items
}

func entryState(entry domain.ActivityEntry) domain.ActivityState {
	return domain.ActivityState{Status: entry.Status, Reason: entry.Reason, Outcome: entry.Outcome}
}

// Queue all initial stages together, matching scan behavior.
func queueNewFile(t *testing.T, db *DB, name string, decider domain.ActivityDecider) int64 {
	t.Helper()
	ctx := context.Background()
	video, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/"+name+".mp4"), name, "key-"+name, 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJobs(ctx, video.ID, decider,
		domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail); err != nil {
		t.Fatal(err)
	}
	return video.ID
}

// Claim and persist stage results through the same sequence as a worker.
func runImportStage(t *testing.T, db *DB, kind domain.JobKind) {
	t.Helper()
	ctx := context.Background()
	ingest := db.Ingest()
	job, err := ingest.ClaimJob(ctx, kind)
	if err != nil {
		t.Fatalf("ClaimJob(%s): %v", kind, err)
	}
	switch kind {
	case domain.JobProbe:
		probe := domain.Probe{DurationMs: 100_000, VideoCodec: "h264", AudioCodec: "aac"}
		_, err = ingest.ApplyProbeForJob(ctx, job, probe, domain.EvaluatePlayability("mp4", probe))
	case domain.JobThumbnail:
		_, err = ingest.SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateDone, domain.SubstitutionUnknown)
	case domain.JobSeekThumbnail:
		_, err = ingest.SetSeekThumbnailStateForJob(ctx, job, domain.SeekThumbnailDone, domain.SubstitutionUnknown)
	case domain.JobPreview:
		_, err = ingest.SetPreviewStateForJob(ctx, job, domain.PreviewStateDone)
	case domain.JobFingerprint:
		_, err = ingest.ApplyFingerprintForJob(ctx, job, sampleFingerprint())
	}
	if err != nil {
		t.Fatalf("record %s: %v", kind, err)
	}
	if err := ingest.CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
}

func giveUpStage(t *testing.T, db *DB, kind domain.JobKind, cause error) {
	t.Helper()
	ctx := context.Background()
	for range domain.MaxJobAttempts {
		job, err := db.Ingest().ClaimJob(ctx, kind)
		if err != nil {
			t.Fatalf("ClaimJob(%s): %v", kind, err)
		}
		if err := db.Ingest().FailClaimedJob(ctx, job, cause); err != nil {
			t.Fatal(err)
		}
	}
}

func TestImportActivityRunsUntilFingerprintFinishes(t *testing.T) {
	db := migratedDB(t)
	queueNewFile(t, db, "clip", domain.ActivityByOwner)

	entries := importEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("import entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entryState(entry) != activityWaiting || entry.Decider != domain.ActivityByOwner {
		t.Fatalf("queued entry = %+v, want running/waiting by owner", entry)
	}
	if entry.Subject.Kind != domain.ActivitySubjectVideo || entry.Subject.Key != "key-clip" || entry.Subject.Label != "clip" {
		t.Errorf("subject = %+v, want the video key-clip labelled clip", entry.Subject)
	}

	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobPreview} {
		runImportStage(t, db, kind)
		entries = importEntries(t, db)
		if len(entries) != 1 || entryState(entries[0]) != activityClaimed {
			t.Fatalf("after %s: entries = %+v, want one running/processing", kind, entries)
		}
	}
	progress := entries[0].Progress
	if progress == nil || progress.Stage != domain.JobFingerprint || progress.Done != 4 || progress.Total != 5 {
		t.Errorf("progress = %+v, want fingerprint with 4 of 5 done", progress)
	}

	runImportStage(t, db, domain.JobFingerprint)
	entries = importEntries(t, db)
	if len(entries) != 1 || entryState(entries[0]) != activityDone || entries[0].ID != entry.ID {
		t.Fatalf("after fingerprint: entries = %+v, want the same entry done/completed", entries)
	}
	if entries[0].Progress != nil {
		t.Errorf("done entry progress = %+v, want none", entries[0].Progress)
	}
}

func TestImportActivityRecordsTheStageThatGaveUp(t *testing.T) {
	db := migratedDB(t)
	queueNewFile(t, db, "broken", domain.ActivityByRule)
	runImportStage(t, db, domain.JobProbe)
	runImportStage(t, db, domain.JobPreview)
	giveUpStage(t, db, domain.JobThumbnail, errors.New("the thumbnail could not be decoded"))

	entries := importEntries(t, db)
	if len(entries) != 1 || entryState(entries[0]) != activityClaimed {
		t.Fatalf("entries = %+v, want one running while the seek thumbnail waits", entries)
	}
	runImportStage(t, db, domain.JobSeekThumbnail)
	runImportStage(t, db, domain.JobFingerprint)

	entries = importEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("import entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entryState(entry) != activityFailed || entry.Stage != domain.JobThumbnail ||
		entry.Detail != "the thumbnail could not be decoded" {
		t.Errorf("entry = %+v, want attention/failed at thumbnail with the message", entry)
	}
	if !entry.RetryOffered() {
		t.Error("a failed import does not offer retry")
	}
}

func TestImportActivityRescanOpensASecondEntry(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	videoID := upsertForImport(t, db, "again")
	runImportStage(t, db, domain.JobProbe)
	runImportStage(t, db, domain.JobPreview)
	first := importEntries(t, db)
	if len(first) != 1 || entryState(first[0]) != activityDone {
		t.Fatalf("first entries = %+v, want one done", first)
	}

	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	entries := importEntries(t, db)
	if len(entries) != 2 {
		t.Fatalf("import entries = %d, want 2", len(entries))
	}
	if entryState(entries[0]) != activityWaiting || entries[1].ID != first[0].ID || entryState(entries[1]) != activityDone {
		t.Errorf("entries = %+v, want a new running/waiting entry above the finished one", entries)
	}
}

func TestImportActivityInterruptedEntryIsSupersededWhenItsJobRuns(t *testing.T) {
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
	interrupted := importEntries(t, db)
	if len(interrupted) != 1 || interrupted[0].Status != domain.ActivityAttention ||
		interrupted[0].Reason != domain.ActivityInterrupted {
		t.Fatalf("entries after restart = %+v, want one attention/interrupted", interrupted)
	}

	runImportStage(t, db, domain.JobProbe)
	entries := importEntries(t, db)
	if len(entries) != 2 {
		t.Fatalf("import entries = %d, want 2", len(entries))
	}
	if entryState(entries[0]) != activityClaimed || entries[0].Decider != domain.ActivityByRule {
		t.Errorf("new entry = %+v, want running/processing by rule", entries[0])
	}
	superseded := domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySuperseded}
	if entries[1].ID != interrupted[0].ID || entryState(entries[1]) != superseded {
		t.Errorf("old entry = %+v, want done/superseded", entries[1])
	}

	for _, kind := range []domain.JobKind{domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobPreview, domain.JobFingerprint} {
		runImportStage(t, db, kind)
	}
	entries = importEntries(t, db)
	if len(entries) != 2 || entryState(entries[0]) != activityDone {
		t.Errorf("entries = %+v, want the new entry done", entries)
	}
}

func TestRetryImportOpensANewEntryForTheFailedStages(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	queueNewFile(t, db, "retry", domain.ActivityByRule)
	giveUpStage(t, db, domain.JobProbe, errors.New("ffprobe failed"))
	for _, kind := range []domain.JobKind{domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobFingerprint} {
		runImportStage(t, db, kind)
	}
	failed := importEntries(t, db)
	if len(failed) != 1 || entryState(failed[0]) != activityFailed || failed[0].Stage != domain.JobProbe {
		t.Fatalf("entries = %+v, want one attention/failed at probe", failed)
	}

	if err := db.Ingest().RetryImport(ctx, failed[0].ID, "key-gone"); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Errorf("retry of a gone video = %v, want ErrActivityNotRetryable", err)
	}
	if err := db.Ingest().RetryImport(ctx, failed[0].ID, "key-retry"); err != nil {
		t.Fatal(err)
	}
	// The second retry must recheck the closed activity even while the first retry's jobs are still waiting.
	if err := db.Ingest().RetryImport(ctx, failed[0].ID, "key-retry"); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Errorf("second retry of the same entry = %v, want ErrActivityNotRetryable", err)
	}
	entries := importEntries(t, db)
	if len(entries) != 2 {
		t.Fatalf("import entries = %d, want 2", len(entries))
	}
	if entryState(entries[0]) != activityWaiting || entries[0].Decider != domain.ActivityByOwner {
		t.Errorf("new entry = %+v, want running/waiting by owner", entries[0])
	}
	if entries[1].Outcome != domain.ActivitySuperseded {
		t.Errorf("failed entry = %+v, want superseded", entries[1])
	}
	runImportStage(t, db, domain.JobProbe)
	runImportStage(t, db, domain.JobPreview)
	entries = importEntries(t, db)
	if entryState(entries[0]) != activityDone {
		t.Errorf("retried entry = %+v, want done", entries[0])
	}

	if err := db.Ingest().RetryImport(ctx, entries[0].ID, "key-retry"); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Errorf("retry of a done entry = %v, want ErrActivityNotRetryable", err)
	}
}

func TestRetryImportOfAnInterruptedEntryAdoptsTheWaitingJobs(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	queueNewFile(t, db, "paused", domain.ActivityByRule)
	if _, err := db.Ingest().ClaimJob(ctx, domain.JobProbe); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Activity().RecoverInterruptedActivity(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().RequeueRunningJobs(ctx); err != nil {
		t.Fatal(err)
	}
	paused := importEntries(t, db)
	if err := db.Ingest().RetryImport(ctx, paused[0].ID, "key-paused"); err != nil {
		t.Fatal(err)
	}
	entries := importEntries(t, db)
	if len(entries) != 2 || entryState(entries[0]) != activityWaiting || entries[0].Decider != domain.ActivityByOwner ||
		entries[1].Outcome != domain.ActivitySuperseded {
		t.Fatalf("entries = %+v, want a new running/waiting owner entry above the superseded one", entries)
	}
	runImportStage(t, db, domain.JobProbe)
	if entries := importEntries(t, db); len(entries) != 2 || entryState(entries[0]) != activityClaimed {
		t.Errorf("entries = %+v, want the owner entry running/processing", entries)
	}
}

// Retry progress includes stages completed under the previous activity.
func TestRetriedImportProgressCountsTheStagesAlreadyDone(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	queueNewFile(t, db, "partial", domain.ActivityByRule)
	runImportStage(t, db, domain.JobProbe)
	giveUpStage(t, db, domain.JobThumbnail, errors.New("no frame"))
	for _, kind := range []domain.JobKind{domain.JobSeekThumbnail, domain.JobPreview, domain.JobFingerprint} {
		runImportStage(t, db, kind)
	}
	failed := importEntries(t, db)
	if len(failed) != 1 || entryState(failed[0]) != activityFailed {
		t.Fatalf("entries = %+v, want one attention/failed", failed)
	}
	if err := db.Ingest().RetryImport(ctx, failed[0].ID, "key-partial"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().ClaimJob(ctx, domain.JobThumbnail); err != nil {
		t.Fatal(err)
	}
	entries := importEntries(t, db)
	progress := entries[0].Progress
	if progress == nil || progress.Stage != domain.JobThumbnail || progress.Done != 4 || progress.Total != 5 {
		t.Errorf("progress = %+v, want thumbnail with 4 of 5 done", progress)
	}
}

func TestImportActivityOfADeletedVideoIsGone(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	videoID := queueNewFile(t, db, "deleted", domain.ActivityByRule)
	queueNewFile(t, db, "kept", domain.ActivityByRule)
	runImportStage(t, db, domain.JobProbe)

	if err := db.ScanIndex().DeleteVideos(ctx, []int64{videoID}); err != nil {
		t.Fatal(err)
	}
	gone := domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityGone}
	for _, entry := range importEntries(t, db) {
		want := activityWaiting
		if entry.Subject.Key == "key-deleted" {
			want = gone
		}
		if entryState(entry) != want {
			t.Errorf("entry %s = %+v, want %+v", entry.Subject.Key, entryState(entry), want)
		}
	}
}

func TestImportActivityOfAnUnplacedVideoIsGone(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	videoID := queueNewFile(t, db, "unplaced", domain.ActivityByRule)
	var encoded string
	if err := db.sql.QueryRowContext(ctx, `select json_group_array(id) from (select id from jobs where video_id = ?
		order by id)`, videoID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	if err := json.Unmarshal([]byte(encoded), &ids); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		dropJob(t, db, id)
		state := entryState(importEntries(t, db)[0])
		if last := i == len(ids)-1; last && state != (domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityGone}) {
			t.Errorf("after the last drop: entry = %+v, want done/gone", state)
		} else if !last && state.Status != domain.ActivityRunning {
			t.Errorf("after drop %d: entry = %+v, want running", i, state)
		}
	}
}

// Use the claim path's BEGIN IMMEDIATE transaction when dropping an unplaced job.
func dropJob(t *testing.T, db *DB, id int64) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.sql.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `begin immediate`); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().dropUnplacedJob(ctx, conn, id); err != nil {
		_, _ = conn.ExecContext(ctx, `rollback`)
		t.Fatal(err)
	}
}

func TestScanActivityRecordsFailedScansUntilOneSucceeds(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	scans := db.Scans()

	failed, _, err := scans.StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cause := domain.NewScanFailure(domain.ScanErrorMediaFolderUnreadable, "/media", errors.New("permission denied"))
	if err := scans.FinishScan(ctx, failed.ID, domain.ScanFailed, cause, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	entries := scanEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("scan entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entryState(entry) != activityFailed || entry.Decider != domain.ActivityByOwner || entry.Detail != cause.Error() {
		t.Errorf("entry = %+v, want attention/failed by owner with the cause", entry)
	}
	if entry.Subject.Kind != domain.ActivitySubjectScan || entry.Subject.Key != strconv.FormatInt(failed.ID, 10) ||
		entry.Subject.Label != string(domain.ScanOriginManual) {
		t.Errorf("subject = %+v, want scan %d labelled manual", entry.Subject, failed.ID)
	}

	if _, _, err := scans.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := scans.FailInterruptedScans(ctx); err != nil {
		t.Fatal(err)
	}
	stopped, _, err := scans.StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := domain.NewScanFailure(domain.ScanErrorInterrupted, "", errors.New("stopped"))
	if err := scans.FinishScan(ctx, stopped.ID, domain.ScanFailed, interrupted, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	if entries := scanEntries(t, db); len(entries) != 1 {
		t.Fatalf("scan entries after interrupted scans = %d, want 1", len(entries))
	}

	done, _, err := scans.StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finishScan(t, db, done.ID)
	entries = scanEntries(t, db)
	superseded := domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySuperseded}
	if len(entries) != 1 || entryState(entries[0]) != superseded {
		t.Errorf("entries after a done scan = %+v, want the failed scan superseded", entries)
	}
}

func finishImport(t *testing.T, db *DB) {
	t.Helper()
	for _, kind := range []domain.JobKind{
		domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobPreview, domain.JobFingerprint,
	} {
		runImportStage(t, db, kind)
	}
}

// Location-only imports create activity only for new paths without remaining stage work.
func TestImportActivityRecordsANewLocationWithoutJobs(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	queueNewFile(t, db, "clip", domain.ActivityByRule)
	finishImport(t, db)
	first := importEntries(t, db)
	if len(first) != 1 || entryState(first[0]) != activityDone {
		t.Fatalf("first entries = %+v, want one done", first)
	}

	copied := sampleFile(fixturePath("/media/copy/clip.mp4"), "clip", "key-clip", 1024, 0)
	copied.Decider = domain.ActivityByOwner
	upserted, err := db.ScanIndex().UpsertVideo(ctx, copied)
	if err != nil {
		t.Fatal(err)
	}
	if upserted.Outcome != domain.OutcomeMoved {
		t.Fatalf("Outcome = %q, want %q", upserted.Outcome, domain.OutcomeMoved)
	}
	entries := importEntries(t, db)
	if len(entries) != 2 || entries[1].ID != first[0].ID {
		t.Fatalf("entries = %+v, want a second entry above the first", entries)
	}
	entry := entries[0]
	if entryState(entry) != activityDone || entry.Decider != domain.ActivityByOwner || entry.Subject.Key != "key-clip" {
		t.Errorf("entry = %+v, want done/completed by owner on key-clip", entry)
	}

	if _, err := db.ScanIndex().UpsertVideo(ctx, copied); err != nil {
		t.Fatal(err)
	}
	if entries := importEntries(t, db); len(entries) != 2 {
		t.Errorf("entries after an unchanged rescan = %d, want 2", len(entries))
	}
}

func queueUntilFingerprint(t *testing.T, db *DB) {
	t.Helper()
	queueNewFile(t, db, "clip", domain.ActivityByRule)
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobPreview} {
		runImportStage(t, db, kind)
	}
}

func requireNoLocationEntry(t *testing.T, db *DB) {
	t.Helper()
	before := len(importEntries(t, db))
	if _, err := db.ScanIndex().UpsertVideo(context.Background(),
		sampleFile(fixturePath("/media/copy/clip.mp4"), "clip", "key-clip", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	if entries := importEntries(t, db); len(entries) != before {
		t.Errorf("entries = %+v, want still %d", entries, before)
	}
}

// Pending fingerprint activity already represents this import.
func TestImportActivityNewLocationWithQueuedFingerprintWritesNothing(t *testing.T) {
	db := migratedDB(t)
	queueUntilFingerprint(t, db)
	requireNoLocationEntry(t, db)
}

// The scan's fingerprint retry supplies the activity.
func TestImportActivityNewLocationWithMissingFingerprintWritesNothing(t *testing.T) {
	db := migratedDB(t)
	queueUntilFingerprint(t, db)
	giveUpStage(t, db, domain.JobFingerprint, errors.New("the fingerprint could not be read"))
	requireNoLocationEntry(t, db)
}

// Remaining stage jobs supply the import activity.
func TestImportActivityNewLocationWithPendingStagesWritesNothing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/clip.mp4"), "clip", "key-clip", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx,
		sampleFile(fixturePath("/media/copy/clip.mp4"), "clip", "key-clip", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	if entries := importEntries(t, db); len(entries) != 0 {
		t.Errorf("entries = %+v, want none before jobs are queued", entries)
	}
}

// Failure to read remaining jobs or write activity must roll back the new location too.
func TestImportActivityNewLocationFailureRollsBack(t *testing.T) {
	for name, breakage := range map[string]string{
		"remaining work": `alter table jobs rename to jobs_broken`,
		"entry": `create trigger fail_location_entry before insert on activity_entries
			begin select raise(abort, 'broken'); end`,
	} {
		t.Run(name, func(t *testing.T) {
			db := migratedDB(t)
			queueNewFile(t, db, "clip", domain.ActivityByRule)
			finishImport(t, db)
			if _, err := db.sql.Exec(breakage); err != nil {
				t.Fatal(err)
			}
			requireLocationRollback(t, db)
		})
	}
}

func requireLocationRollback(t *testing.T, db *DB) {
	t.Helper()
	copied := fixturePath("/media/copy/clip.mp4")
	if _, err := db.ScanIndex().UpsertVideo(context.Background(),
		sampleFile(copied, "clip", "key-clip", 1024, 0)); err == nil {
		t.Fatal("UpsertVideo succeeded, want the activity failure")
	}
	var locations int
	if err := db.sql.QueryRow(`select count(*) from video_locations where path = ?`, copied).
		Scan(&locations); err != nil {
		t.Fatal(err)
	}
	if locations != 0 {
		t.Errorf("locations at %s = %d, want 0 after the rollback", copied, locations)
	}
}
