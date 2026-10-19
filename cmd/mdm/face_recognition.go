package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/faceruntime"
	"github.com/syudead/vv/internal/media"
	"github.com/syudead/vv/internal/store"
)

// startFaceRecognition reads the face runtime's files in the background and resumes a download interrupted while
// the feature was on; the returned stop cancels a running download. See docs/design-docs/face-recognition.md.
func startFaceRecognition(ctx context.Context, dataDir string, db *store.DB, logger *slog.Logger) (*app.FaceRecognitionSettings, func()) {
	faceRuntime := faceruntime.New(faceruntime.Options{
		Dir:     filepath.Join(dataDir, "face"),
		Catalog: faceruntime.DefaultCatalog(runtime.GOOS, runtime.GOARCH),
		Launch:  faceruntime.ExecLauncher(ctx, media.Command),
		Logger:  logger,
	})
	settings := app.NewFaceRecognitionSettings(app.FaceRecognitionSettingsOptions{
		Settings: db.Settings(), Queue: db.Faces(), Runtime: faceRuntime,
	})
	go func() {
		if err := settings.Resume(ctx); err != nil {
			logger.Warn("could not resume the face runtime download", slog.Any("error", err))
		}
	}()
	return settings, faceRuntime.Stop
}
