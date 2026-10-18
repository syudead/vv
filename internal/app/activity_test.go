package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeActivityFolders struct {
	roots []domain.MediaFolder
}

func (f fakeActivityFolders) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return f.roots, nil
}

func newActivityScans(t *testing.T, root string) (*Scans, *fakePublisher) {
	t.Helper()
	store := newFakeScanStore()
	store.current = domain.Scan{ID: 1, State: domain.ScanDone}
	store.hasScan = true
	publisher := &fakePublisher{}
	scans := NewScans(ScansOptions{
		Store:     store,
		Jobs:      store,
		Folders:   fakeActivityFolders{roots: []domain.MediaFolder{{ID: 7, Path: root}}},
		Publisher: publisher,
		Logger:    discardLogger(),
	})
	return scans, publisher
}

func currentActivity(t *testing.T, scans *Scans) domain.ScanActivity {
	t.Helper()
	scan, err := scans.CurrentScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return scan.Activity
}

func activityChanges(publisher *fakePublisher) int {
	count := 0
	for _, event := range publisher.published() {
		if _, ok := event.(domain.ScanActivityChanged); ok {
			count++
		}
	}
	return count
}

func TestScanActivityShowsLatestAndFallsBack(t *testing.T) {
	root := t.TempDir()
	scans, publisher := newActivityScans(t, root)
	first := domain.Job{ID: 1, Kind: domain.JobThumbnail, VideoID: 10, LocationPath: filepath.Join(root, "a", "one.mp4")}
	second := domain.Job{ID: 2, Kind: domain.JobPreview, VideoID: 20, LocationPath: filepath.Join(root, "two.mp4")}

	if got := currentActivity(t, scans); got.Active() {
		t.Fatalf("何も動いていないのに今の処理がある: %+v", got)
	}

	scans.JobStarted(first)
	scans.JobStarted(second)
	got := currentActivity(t, scans)
	if got.Kind != domain.ActivityPreview || got.VideoID != 20 || got.FileName() != "two.mp4" {
		t.Errorf("重なったとき = %+v, want 後に始まった preview の two.mp4", got)
	}
	if !got.Located || got.Folder != (domain.VideoFolder{RootID: 7, Path: ""}) {
		t.Errorf("フォルダ = %+v (located %v), want 登録フォルダ 7 の直下", got.Folder, got.Located)
	}

	scans.JobFinished(second)
	got = currentActivity(t, scans)
	if got.Kind != domain.ActivityThumbnail || got.VideoID != 10 || got.FileName() != "one.mp4" {
		t.Errorf("後の方が終わったとき = %+v, want 残りの thumbnail の one.mp4", got)
	}
	if got.Folder != (domain.VideoFolder{RootID: 7, Path: "a"}) {
		t.Errorf("フォルダ = %+v, want 登録フォルダ 7 の a", got.Folder)
	}

	scans.JobFinished(first)
	if got := currentActivity(t, scans); got.Active() {
		t.Errorf("すべて終わったのに今の処理がある: %+v", got)
	}
	// Two starts and two visible completions emit events.
	if changes := activityChanges(publisher); changes != 4 {
		t.Errorf("ScanActivityChanged = %d, want 4", changes)
	}
}

func TestScanActivityIgnoresEndOfHiddenJob(t *testing.T) {
	root := t.TempDir()
	scans, publisher := newActivityScans(t, root)
	first := domain.Job{ID: 1, Kind: domain.JobProbe, VideoID: 10, LocationPath: filepath.Join(root, "one.mp4")}
	second := domain.Job{ID: 2, Kind: domain.JobSeekThumbnail, VideoID: 20, LocationPath: filepath.Join(root, "two.mp4")}

	scans.JobStarted(first)
	scans.JobStarted(second)
	scans.JobFinished(first)
	if got := currentActivity(t, scans); got.Kind != domain.ActivitySeekThumbnail || got.VideoID != 20 {
		t.Errorf("今の処理 = %+v, want seekThumbnail の two.mp4", got)
	}
	if changes := activityChanges(publisher); changes != 2 {
		t.Errorf("ScanActivityChanged = %d, want 2", changes)
	}
}

func TestScanActivityRegistering(t *testing.T) {
	root := t.TempDir()
	scans, _ := newActivityScans(t, root)
	job := domain.Job{ID: 1, Kind: domain.JobProbe, VideoID: 10, LocationPath: filepath.Join(root, "job.mp4")}

	scans.JobStarted(job)
	scans.ReportScanFile(filepath.Join(root, "a.mp4"), 0)
	scans.ReportScanFile(filepath.Join(root, "b.mp4"), 5)
	got := currentActivity(t, scans)
	if got.Kind != domain.ActivityRegistering || got.FileName() != "b.mp4" || got.VideoID != 5 {
		t.Errorf("登録中 = %+v, want registering の b.mp4 (動画 5)", got)
	}

	scans.ReportScanFile("", 0)
	if got := currentActivity(t, scans); got.Kind != domain.ActivityProbe || got.FileName() != "job.mp4" {
		t.Errorf("登録を終えたあと = %+v, want probe の job.mp4", got)
	}
	scans.JobFinished(job)
	if got := currentActivity(t, scans); got.Active() {
		t.Errorf("すべて終わったのに今の処理がある: %+v", got)
	}
}

func TestScanActivityOutsideRootsHasNoFolder(t *testing.T) {
	scans, _ := newActivityScans(t, t.TempDir())
	scans.JobStarted(domain.Job{ID: 1, Kind: domain.JobProbe, LocationPath: filepath.Join(t.TempDir(), "x.mp4")})
	got := currentActivity(t, scans)
	if !got.Active() || got.Located {
		t.Errorf("今の処理 = %+v, want フォルダの無い probe", got)
	}
}
