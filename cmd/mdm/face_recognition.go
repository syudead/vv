package main

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceruntime"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/mediafs"
	"github.com/syudead/vv/internal/store"
)

// faceRecognition is the face runtime, its settings and the handler of the face lane's jobs; see
// docs/design-docs/face-recognition.md.
type faceRecognition struct {
	settings *app.FaceRecognitionSettings
	analysis *app.FaceRecognizer
	runtime  *faceruntime.Manager
}

// newFaceRecognition wakes the job scheduler through wake, also when the runtime becomes ready, which ends the pause
// of the face lane.
func newFaceRecognition(
	ctx context.Context, dataDir string, db *store.DB, wake func(), logger *slog.Logger,
) faceRecognition {
	faceRuntime := faceruntime.New(faceruntime.Options{
		Dir:     filepath.Join(dataDir, "face"),
		Catalog: faceruntime.DefaultCatalog(runtime.GOOS, runtime.GOARCH),
		Launch:  faceruntime.ExecLauncher(ctx, lowPriorityCommand),
		Logger:  logger,
		Ready:   wake,
	})
	analysis := app.NewFaceRecognizer(app.FaceRecognizerOptions{
		Queue: db.Faces(), Images: db.Faces(), Settings: db.Settings(), Runtime: faceRuntime,
		Helper:       func(r domain.FaceRecognizer) app.FaceHelper { return faceRuntime.Helper(r) },
		Frames:       media.FaceFrames,
		MediaFolders: db.Settings(), MediaFiles: mediafs.New(), Cast: db.Faces(),
		Wake: wake, Logger: logger,
	})
	settings := app.NewFaceRecognitionSettings(app.FaceRecognitionSettingsOptions{
		Settings: db.Settings(), Queue: db.Faces(), Runtime: faceRuntime, Analysis: analysis,
	})
	return faceRecognition{settings: settings, analysis: analysis, runtime: faceRuntime}
}

// start reads the face runtime's files in the background and resumes a download interrupted while the feature was
// on; the returned stop cancels a running download.
func (f faceRecognition) start(ctx context.Context, logger *slog.Logger) func() {
	go func() {
		if err := f.settings.Resume(ctx); err != nil {
			logger.Warn("could not resume the face runtime download", slog.Any("error", err))
		}
	}()
	return f.runtime.Stop
}

// lowPriorityCommand builds a face helper process that starts at low priority, like the jobs it serves.
func lowPriorityCommand(ctx context.Context, name string, args ...string) (*exec.Cmd, func() error) {
	cmd := media.Command(ctx, media.PriorityLow, name, args...)
	return cmd.Cmd, cmd.Start
}
