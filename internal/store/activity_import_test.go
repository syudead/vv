package store

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 取り込みと失敗した走査の記録（specs/055-activity/data-model.md「Rules」、research.md R-3・R-4・R-5）。

// importEntries は import の記録を新しい順に返す。
func importEntries(t *testing.T, db *DB) []domain.ActivityEntry {
	t.Helper()
	page := listActivity(t, db, domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureImport}}, "", 50)
	return page.Items
}

// scanEntries は scan の記録を新しい順に返す。
func scanEntries(t *testing.T, db *DB) []domain.ActivityEntry {
	t.Helper()
	page := listActivity(t, db, domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureScan}}, "", 50)
	return page.Items
}

// entryState は記録の状態と理由か結果である。
func entryState(entry domain.ActivityEntry) domain.ActivityState {
	return domain.ActivityState{Status: entry.Status, Reason: entry.Reason, Outcome: entry.Outcome}
}

// queueNewFile は新しいファイルを登録し、走査と同じく解析・サムネイル・シーク用サムネイルの仕事を積む。
func queueNewFile(t *testing.T, db *DB, name string, decider domain.ActivityDecider) int64 {
	t.Helper()
	ctx := context.Background()
	video, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/"+name+".mp4"), name, "key-"+name, 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail} {
		if err := db.Ingest().EnqueueJob(ctx, kind, video.ID, decider); err != nil {
			t.Fatal(err)
		}
	}
	return video.ID
}

// runImportStage は kind の仕事を 1 件取り出し、ワーカーと同じく結果を書いて完了にする。取り出せる仕事が
// 無ければ失敗にする。
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
	}
	if err != nil {
		t.Fatalf("record %s: %v", kind, err)
	}
	if err := ingest.CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
}

// giveUpStage は kind の仕事を上限まで取り出して失敗させる。
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

// 新しいファイルは 1 件の import の記録になり、指紋の仕事が終わるまで running、終わると done になる。
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

// 諦めた段階は記録を attention/failed にし、stage にその段階、detail にその文が入る。
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

// 同じファイルの取り込みを積み直すと 2 つ目の記録が開き、終わった記録は変わらない。
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

// 再起動で中断した記録は attention/interrupted になり、積み直した仕事が走ると新しい記録が開いてそれを
// superseded で閉じる。新しい記録を始めたのは規則である。
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

	// 残りの段階も新しい記録に加わり、終えると新しい記録が done になる。
	for _, kind := range []domain.JobKind{domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobPreview, domain.JobFingerprint} {
		runImportStage(t, db, kind)
	}
	entries = importEntries(t, db)
	if len(entries) != 2 || entryState(entries[0]) != activityDone {
		t.Errorf("entries = %+v, want the new entry done", entries)
	}
}

// やり直しは失敗した段階を積み直し、所有者が始めた新しい記録を開いて失敗した記録を閉じる。
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

	if err := db.Ingest().RetryImport(ctx, "key-retry"); err != nil {
		t.Fatal(err)
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

	if err := db.Ingest().RetryImport(ctx, "key-retry"); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Errorf("retry with nothing failed = %v, want ErrActivityNotRetryable", err)
	}
	if err := db.Ingest().RetryImport(ctx, "key-gone"); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Errorf("retry of a gone video = %v, want ErrActivityNotRetryable", err)
	}
}

// 中断した記録のやり直しは、待っている仕事を所有者が始めた新しい記録にまとめる。
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
	if err := db.Ingest().RetryImport(ctx, "key-paused"); err != nil {
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

// interrupted 以外の理由で failed に終わった走査だけが記録を書き、次に done で終わった走査がそれを閉じる。
// 起動時に interrupted で閉じた走査は書かない。
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
