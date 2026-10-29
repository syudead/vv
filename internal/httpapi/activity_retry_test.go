package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

func (e *activityEnv) listActivityItems(t *testing.T, query string) []gen.ActivityListItem {
	t.Helper()
	rec := e.serve(http.MethodGet, "/api/activity?"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/activity?%s = %d: %s", query, rec.Code, rec.Body)
	}
	var page gen.ActivityPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func (e *activityEnv) failImport(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	if _, err := e.db.Settings().AddMediaFolder(ctx, dir); err != nil {
		t.Fatal(err)
	}
	video, err := e.db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(dir, name+".mp4"), Title: name, ContentKey: "key-" + name, SizeBytes: 1024,
		MTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Container: domain.ContainerFromPath(name + ".mp4"),
	})
	if err != nil {
		t.Fatal(err)
	}
	ingest := e.db.Ingest()
	if err := ingest.EnqueueJob(ctx, domain.JobProbe, video.ID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	for range domain.MaxJobAttempts {
		job, err := ingest.ClaimJob(ctx, domain.JobProbe)
		if err != nil {
			t.Fatal(err)
		}
		if err := ingest.FailClaimedJob(ctx, job, errors.New("ffprobe failed")); err != nil {
			t.Fatal(err)
		}
		if err := store.MakeJobsDueForTest(ctx, e.db); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetryFailedImportStartsANewRunningEntry(t *testing.T) {
	e := newActivityEnv(t)
	e.failImport(t, "broken")
	members := e.onlyPipelineMembers(t)
	if len(members) != 1 || members[0].Status != gen.ActivityStatusAttention || !members[0].RetryRun {
		t.Fatalf("members = %+v, want one failed import offering retry", members)
	}
	failed := members[0].Id

	rec := e.serve(http.MethodPost, "/api/activity/"+itoa(failed)+"/retry", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("retry = %d: %s", rec.Code, rec.Body)
	}
	members = e.onlyPipelineMembers(t)
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
	if members[0].Id != failed || members[0].Outcome == nil || *members[0].Outcome != gen.ActivityOutcomeSuperseded ||
		!members[0].Replaced {
		t.Errorf("failed member = %+v, want superseded and replaced", members[0])
	}
	if members[1].Status != gen.ActivityStatusRunning || members[1].Decider != gen.ActivityDeciderOwner {
		t.Errorf("new member = %+v, want running by owner", members[1])
	}

	rec = e.serve(http.MethodPost, "/api/activity/"+itoa(failed)+"/retry", "")
	assertErrorBody(t, "retry again", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeActivityNotRetryable,
	})
}

// onlyPipelineMembers lists the import feature and returns the members of its only entry, a pipeline.
func (e *activityEnv) onlyPipelineMembers(t *testing.T) []gen.ActivityMemberSummary {
	t.Helper()
	items := e.listActivityItems(t, "feature=import")
	if len(items) != 1 || items[0].Kind != gen.ActivityKindPipeline || items[0].Members == nil {
		t.Fatalf("items = %+v, want one pipeline", items)
	}
	return *items[0].Members
}

func (e *activityEnv) recordFailedScan(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	if _, err := e.db.Settings().AddMediaFolder(ctx, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	scan, _, err := e.db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cause := domain.NewScanFailure(domain.ScanErrorMediaFolderUnreadable, "", errors.New("permission denied"))
	if err := e.db.Scans().FinishScan(ctx, scan.ID, domain.ScanFailed, cause, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	items := e.listActivityItems(t, "feature=scan")
	if len(items) != 1 || !items[0].RetryRun {
		t.Fatalf("items = %+v, want one failed scan offering retry", items)
	}
	return items[0].Id
}

func TestRetryFailedScanStartsAManualScan(t *testing.T) {
	e := newActivityEnv(t)
	id := e.recordFailedScan(t)

	rec := e.serve(http.MethodPost, "/api/activity/"+itoa(id)+"/retry", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("retry = %d: %s", rec.Code, rec.Body)
	}
	e.scans.Wait()
	scan, err := e.db.Scans().CurrentScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.State != domain.ScanDone || scan.Origin != domain.ScanOriginManual {
		t.Errorf("scan = %+v, want a manual scan done", scan)
	}
	items := e.listActivityItems(t, "feature=scan")
	if len(items) != 1 || items[0].Id != id || items[0].Outcome == nil || *items[0].Outcome != gen.ActivityOutcomeSuperseded {
		t.Errorf("items = %+v, want the failed scan superseded", items)
	}
}

func TestRetryFailedScanWhileScanningAnswersConflict(t *testing.T) {
	e := newActivityEnv(t)
	id := e.recordFailedScan(t)
	if _, _, err := e.db.Scans().StartScan(context.Background()); err != nil {
		t.Fatal(err)
	}

	rec := e.serve(http.MethodPost, "/api/activity/"+itoa(id)+"/retry", "")
	assertErrorBody(t, "retry while scanning", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeScanInProgress,
	})
}
