package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/mediafs"
)

// pausedPreview writes part of the preview and is then stopped by a pause, as an encode cancelled mid-run is.
type pausedPreview struct {
	*media.Assets
	pause context.CancelCauseFunc
}

func (g pausedPreview) Preview(ctx context.Context, _, output string, _ int64) error {
	if err := os.WriteFile(output, []byte("partial"), 0o644); err != nil {
		return err
	}
	g.pause(domain.ErrJobPaused)
	<-ctx.Done()
	return ctx.Err()
}

// previewReady answers the reads a preview stage makes before it encodes: a probed video whose content is current.
// Any other store call panics on the nil interface, since the stopped stage must record nothing.
type previewReady struct {
	app.IngestStore
	duration int64
}

func (s previewReady) GetVideo(_ context.Context, id int64) (domain.Video, error) {
	return domain.Video{ID: id, ProbeState: domain.ProbeStateDone, DurationMs: &s.duration}, nil
}

func (previewReady) ContentKeyCurrent(context.Context, int64, string) (bool, error) { return true, nil }

type oneMediaFolder string

func (f oneMediaFolder) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return []domain.MediaFolder{{ID: 1, Path: string(f)}}, nil
}

// A preview stage stopped by a pause leaves nothing of its partial output in the artifact store's temporary
// directory.
func TestIngestPreviewCancelledMidRunLeavesNoTemporaryFile(t *testing.T) {
	mediaDir := t.TempDir()
	path := filepath.Join(mediaDir, "movie.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	jobCtx, pause := context.WithCancelCause(context.Background())
	thumbnails := Config{DataDir: t.TempDir()}.ThumbnailsDir()
	ingest := app.NewIngest(app.IngestOptions{
		Store: previewReady{duration: 10_000}, Generator: pausedPreview{Assets: media.NewAssets(), pause: pause},
		Artifacts: artifacts.New(thumbnails),
		Sources:   app.MediaFiles{Folders: oneMediaFolder(mediaDir), Resolver: mediafs.New()},
		Logger:    slog.New(slog.DiscardHandler),
	})
	job := domain.Job{ID: 1, Kind: domain.JobPreview, VideoID: 1, ContentKey: "paused-preview", LocationPath: path}

	if err := ingest.Preview(jobCtx, job); !errors.Is(err, context.Canceled) {
		t.Fatalf("Preview = %v, want the cancellation", err)
	}
	entries, err := os.ReadDir(filepath.Join(thumbnails, ".tmp"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary entries after the pause = %v, want none", entries)
	}
}
