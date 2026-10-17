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
)

// 取り込みと失敗した走査のやり直し。

// listActivityItems は GET /api/activity の項目を返す。
func (e *activityEnv) listActivityItems(t *testing.T, query string) []gen.ActivityEntry {
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

// failImport は登録フォルダの中に動画を登録して解析の仕事を積み、上限まで失敗させる。
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
	}
}

// 受け入れ: 失敗した取り込みのやり直しは 202 を返し、所有者が始めた新しい running の記録が続く。失敗した記録は
// superseded で閉じる。
func TestRetryFailedImportStartsANewRunningEntry(t *testing.T) {
	e := newActivityEnv(t)
	e.failImport(t, "broken")
	items := e.listActivityItems(t, "feature=import")
	if len(items) != 1 || items[0].Status != gen.ActivityStatusAttention || !items[0].RetryRun {
		t.Fatalf("items = %+v, want one failed import offering retry", items)
	}
	failed := items[0].Id

	rec := e.serve(http.MethodPost, "/api/activity/"+itoa(failed)+"/retry", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("retry = %d: %s", rec.Code, rec.Body)
	}
	items = e.listActivityItems(t, "feature=import")
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Status != gen.ActivityStatusRunning || items[0].Decider != gen.ActivityDeciderOwner {
		t.Errorf("new entry = %+v, want running by owner", items[0])
	}
	if items[1].Id != failed || items[1].Outcome == nil || *items[1].Outcome != gen.ActivityOutcomeSuperseded {
		t.Errorf("failed entry = %+v, want superseded", items[1])
	}

	rec = e.serve(http.MethodPost, "/api/activity/"+itoa(failed)+"/retry", "")
	assertErrorBody(t, "retry again", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeActivityNotRetryable,
	})
}

// recordFailedScan は登録フォルダを 1 つ置き、interrupted 以外の理由で失敗した走査を記録する。
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

// 受け入れ: 失敗した走査のやり直しは手動の走査を始め、それが done で終わると scan の記録が superseded になる。
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

// 走査中の scan のやり直しは、POST /api/scans と同じ判断で 409 scan_in_progress を返す。
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
