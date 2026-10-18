package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func completeStageJob(t *testing.T, db *DB, kind domain.JobKind, substitution domain.Substitution) domain.Job {
	t.Helper()
	ctx := context.Background()
	job, err := db.Ingest().ClaimJob(ctx, kind)
	if err != nil {
		t.Fatal(err)
	}
	setStageDone(t, db, job, substitution)
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	return job
}

func setStageDone(t *testing.T, db *DB, job domain.Job, substitution domain.Substitution) {
	t.Helper()
	ctx := context.Background()
	var (
		applied bool
		err     error
	)
	switch job.Kind {
	case domain.JobThumbnail:
		applied, err = db.Ingest().SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateDone, substitution)
	case domain.JobSeekThumbnail:
		applied, err = db.Ingest().SetSeekThumbnailStateForJob(ctx, job, domain.SeekThumbnailDone, substitution)
	default:
		t.Fatalf("代用の無い段階 %s", job.Kind)
	}
	if err != nil || !applied {
		t.Fatalf("成功の記録 = %v, %v", applied, err)
	}
}

func importWithStages(t *testing.T, db *DB) int64 {
	t.Helper()
	ctx := context.Background()
	_, ids := startImport(t, db, "a")
	runOneJob(t, db, domain.JobProbe)
	for _, kind := range []domain.JobKind{domain.JobThumbnail, domain.JobSeekThumbnail} {
		if err := db.Ingest().EnqueueJob(ctx, kind, ids[0], domain.ActivityByRule); err != nil {
			t.Fatal(err)
		}
	}
	return ids[0]
}

func TestThumbnailFirstFrameIsSubstitutedIssue(t *testing.T) {
	db := migratedDB(t)
	id := importWithStages(t, db)
	completeStageJob(t, db, domain.JobThumbnail, domain.SubstitutionUsed)
	completeStageJob(t, db, domain.JobSeekThumbnail, domain.SubstitutionNone)
	runAllJobs(t, db)

	scan, issues, progress := importIssues(t, db)
	if len(issues) != 1 || issues[0].VideoID != id || issues[0].Severity != domain.IssueSubstituted ||
		!slices.Equal(issues[0].Kinds, []domain.ScanIssueKind{domain.IssueThumbnailFirstFrame}) ||
		issues[0].FileName != "a.mp4" {
		t.Fatalf("問題 = %+v, want a の thumbnail_first_frame", issues)
	}
	counts := domain.CountScanIssues(issues)
	if counts.Substituted != 1 || counts.Failed != 0 {
		t.Fatalf("本数 = %+v, want 代用 1・失敗 0", counts)
	}
	if progress.Status != domain.ImportDone {
		t.Fatalf("状態 = %+v, want done", progress)
	}
	if scan.IssuesRevision == 0 {
		t.Fatal("代用を記録したのに issues_revision が増えていない")
	}
}

func TestSeekThumbnailFullDecodeJoinsTheSameVideo(t *testing.T) {
	db := migratedDB(t)
	id := importWithStages(t, db)
	completeStageJob(t, db, domain.JobThumbnail, domain.SubstitutionUsed)
	completeStageJob(t, db, domain.JobSeekThumbnail, domain.SubstitutionUsed)
	runAllJobs(t, db)

	_, issues, progress := importIssues(t, db)
	want := []domain.ScanIssueKind{domain.IssueThumbnailFirstFrame, domain.IssueSeekThumbnailFullDecode}
	if len(issues) != 1 || issues[0].VideoID != id || issues[0].Severity != domain.IssueSubstituted ||
		!slices.Equal(issues[0].Kinds, want) {
		t.Fatalf("問題 = %+v, want a の %v", issues, want)
	}
	if progress.Status != domain.ImportDone {
		t.Fatalf("状態 = %+v, want done", progress)
	}
}

func TestSubstitutionClearedWhenRebuiltWithoutIt(t *testing.T) {
	db := migratedDB(t)
	importWithStages(t, db)
	thumbnail := completeStageJob(t, db, domain.JobThumbnail, domain.SubstitutionUsed)
	seek := completeStageJob(t, db, domain.JobSeekThumbnail, domain.SubstitutionUsed)
	runAllJobs(t, db)
	before, _, _ := importIssues(t, db)

	setStageDone(t, db, seek, domain.SubstitutionUnknown)
	unchanged, issues, _ := importIssues(t, db)
	if unchanged.IssuesRevision != before.IssuesRevision || len(issues) != 1 || len(issues[0].Kinds) != 2 {
		t.Fatalf("採用で問題が変わった: %+v (revision %d → %d)", issues, before.IssuesRevision, unchanged.IssuesRevision)
	}

	setStageDone(t, db, seek, domain.SubstitutionNone)
	after, issues, _ := importIssues(t, db)
	if len(issues) != 1 || !slices.Equal(issues[0].Kinds, []domain.ScanIssueKind{domain.IssueThumbnailFirstFrame}) {
		t.Fatalf("問題 = %+v, want thumbnail_first_frame だけ", issues)
	}
	if after.IssuesRevision <= before.IssuesRevision {
		t.Fatalf("行を消したのに issues_revision が増えていない: %d → %d", before.IssuesRevision, after.IssuesRevision)
	}

	setStageDone(t, db, thumbnail, domain.SubstitutionNone)
	// Seek completion queues fingerprints again; finish them before checking settled status.
	runAllJobs(t, db)
	if _, issues, progress := importIssues(t, db); len(issues) != 0 || progress.Status != domain.ImportDone {
		t.Fatalf("問題 = %+v, 状態 = %+v, want なし・done", issues, progress)
	}
}

func TestSubstitutionWithFailureIsFailed(t *testing.T) {
	db := migratedDB(t)
	id := importWithStages(t, db)
	completeStageJob(t, db, domain.JobThumbnail, domain.SubstitutionUsed)
	failJobToLimit(t, db, domain.JobSeekThumbnail)
	runAllJobs(t, db)

	_, issues, progress := importIssues(t, db)
	if len(issues) != 1 || issues[0].VideoID != id || issues[0].Severity != domain.IssueFailed {
		t.Fatalf("問題 = %+v, want a の失敗", issues)
	}
	if counts := domain.CountScanIssues(issues); counts.Failed != 1 || counts.Substituted != 0 {
		t.Fatalf("本数 = %+v, want 失敗 1・代用 0", counts)
	}
	if progress.Status != domain.ImportPartial {
		t.Fatalf("状態 = %+v, want partial", progress)
	}
}
