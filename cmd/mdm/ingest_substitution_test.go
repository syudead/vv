package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/store"
)

type substitutingGenerator struct{}

func (substitutingGenerator) CheckSource(string) error { return nil }
func (substitutingGenerator) Probe(context.Context, string) (domain.Probe, error) {
	return domain.Probe{DurationMs: 60_000, VideoCodec: "h264", AudioCodec: "aac"}, nil
}
func (substitutingGenerator) Thumbnail(_ context.Context, _ string, _ int64, output, _ string) (bool, error) {
	return true, os.WriteFile(output, []byte("jpeg"), 0o600)
}
func (substitutingGenerator) ThumbnailAt(_ context.Context, _ string, _ int64, output, _ string) error {
	return os.WriteFile(output, []byte("jpeg"), 0o600)
}
func (substitutingGenerator) VideoFingerprint(context.Context, string) (domain.Fingerprint, error) {
	return domain.Fingerprint{}, nil
}
func (substitutingGenerator) SeekSprite(context.Context, string, string, domain.SeekSpriteLayout) (bool, error) {
	return false, nil
}
func (substitutingGenerator) Preview(context.Context, string, string, int64) error { return nil }
func (substitutingGenerator) FitThumbnailImage(_ context.Context, image []byte, _ float64) ([]byte, error) {
	return image, nil
}
func (substitutingGenerator) FitThumbnailSource(_ context.Context, image []byte, _ float64) ([]byte, error) {
	return image, nil
}

// Verify the real SQLite success transaction records thumbnail substitution without failing
// ingestion.
func TestThumbnailSubstitutionIsReportedAsImportIssue(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := store.OpenMigratedForTest(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}

	scan, _, err := db.Scans().StartScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	video, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(mediaDir, "clip.mp4"), Title: "clip", ContentKey: "clip", SizeBytes: 1,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail} {
		if err := db.Ingest().EnqueueJob(ctx, kind, video.ID, domain.ActivityByRule); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Scans().FinishScan(ctx, scan.ID, domain.ScanDone, nil, domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}

	ingest := app.NewIngest(app.IngestOptions{
		Store:     db.Ingest(),
		Generator: substitutingGenerator{},
		Artifacts: artifacts.New(Config{DataDir: dataDir}.ThumbnailsDir()),
		Logger:    slog.New(slog.DiscardHandler),
	})
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobPreview} {
		job, err := db.Ingest().ClaimJob(ctx, kind)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if kind != domain.JobPreview {
			if err := ingest.Handler(kind)(ctx, job); err != nil {
				t.Fatalf("%s: %v", kind, err)
			}
		}
		if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}

	scans := app.NewScans(app.ScansOptions{Store: db.Scans(), Jobs: db.Ingest(), Logger: slog.New(slog.DiscardHandler)})
	current, err := scans.CurrentScan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.Import.Status != domain.ImportDone {
		t.Fatalf("取り込み = %+v, want done", current.Import)
	}
	if current.Issues.Substituted != 1 || current.Issues.Failed != 0 {
		t.Fatalf("本数 = %+v, want 代用 1・失敗 0", current.Issues)
	}
	page, err := scans.ListScanIssues(ctx, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].VideoID != video.ID || page.Items[0].Severity != domain.IssueSubstituted ||
		!slices.Equal(page.Items[0].Kinds, []domain.ScanIssueKind{domain.IssueThumbnailFirstFrame}) {
		t.Fatalf("一覧 = %+v, want clip の thumbnail_first_frame", page.Items)
	}
}
