package app

import (
	"context"
	"errors"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

type FaceSettingsStore interface {
	FaceSettings(ctx context.Context) (domain.FaceSettings, error)
	// SaveFaceSettings increases face.generation in the same transaction when the recognizer changes.
	SaveFaceSettings(ctx context.Context, settings domain.FaceSettings) error
}

type FaceQueueCounter interface {
	Counts(ctx context.Context) (domain.QueueCounts, error)
}

// FaceRuntime keeps the face runtime and model files; the state is read from the files, never stored.
type FaceRuntime interface {
	Recognizers() []domain.FaceRecognizerDownload
	Status(recognizer domain.FaceRecognizer) domain.FaceRuntimeStatus
	// Start downloads in the background what recognizer needs and is not verified yet, cancelling a download for
	// another recognizer; it returns domain.ErrFaceRecognitionUnsupported where no runtime is published.
	Start(recognizer domain.FaceRecognizer) error
	// Stop cancels a running download and waits for it.
	Stop()
}

type FaceRecognitionSettingsOptions struct {
	Settings FaceSettingsStore
	Queue    FaceQueueCounter
	Runtime  FaceRuntime
}

// FaceRecognitionSettings saves the face recognition settings and downloads the runtime only while the feature is
// on; see docs/design-docs/face-recognition.md.
type FaceRecognitionSettings struct {
	settings FaceSettingsStore
	queue    FaceQueueCounter
	runtime  FaceRuntime
	// mu orders saves with the runtime calls they make.
	mu sync.Mutex
}

func NewFaceRecognitionSettings(opts FaceRecognitionSettingsOptions) *FaceRecognitionSettings {
	return &FaceRecognitionSettings{settings: opts.Settings, queue: opts.Queue, runtime: opts.Runtime}
}

// Resume restarts at startup a download that was interrupted while the feature was on. A file that fails its hash
// stays failed until the owner downloads again.
func (f *FaceRecognitionSettings) Resume(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	settings, err := f.settings.FaceSettings(ctx)
	if err != nil || !settings.Enabled {
		return err
	}
	if f.runtime.Status(settings.Recognizer).State != domain.FaceRuntimeAbsent {
		return nil
	}
	return f.runtime.Start(settings.Recognizer)
}

func (f *FaceRecognitionSettings) Status(ctx context.Context) (domain.FaceRecognitionStatus, error) {
	settings, err := f.settings.FaceSettings(ctx)
	if err != nil {
		return domain.FaceRecognitionStatus{}, err
	}
	counts, err := f.queue.Counts(ctx)
	if err != nil {
		return domain.FaceRecognitionStatus{}, err
	}
	return domain.FaceRecognitionStatus{
		Settings:    settings,
		Runtime:     f.runtime.Status(settings.Recognizer),
		Recognizers: f.runtime.Recognizers(),
		Counts:      counts,
	}, nil
}

// SaveSettings starts the download of what the recognizer needs when the feature is on, and cancels a download
// when it is off, so nothing is downloaded while it stays off.
func (f *FaceRecognitionSettings) SaveSettings(ctx context.Context, settings domain.FaceSettings) (domain.FaceRecognitionStatus, error) {
	validated, err := domain.ValidateFaceSettings(settings)
	if err != nil {
		return domain.FaceRecognitionStatus{}, err
	}
	if err := f.save(ctx, validated); err != nil {
		return domain.FaceRecognitionStatus{}, err
	}
	return f.Status(ctx)
}

func (f *FaceRecognitionSettings) save(ctx context.Context, settings domain.FaceSettings) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.settings.SaveFaceSettings(ctx, settings); err != nil {
		return err
	}
	if !settings.Enabled {
		f.runtime.Stop()
		return nil
	}
	// The status reports an unsupported platform; saving the choice still succeeds.
	if err := f.runtime.Start(settings.Recognizer); err != nil && !errors.Is(err, domain.ErrFaceRecognitionUnsupported) {
		return err
	}
	return nil
}

// RetryDownload downloads again what the recognizer needs; it refuses while the feature is off or unsupported.
func (f *FaceRecognitionSettings) RetryDownload(ctx context.Context) (domain.FaceRecognitionStatus, error) {
	if err := f.retry(ctx); err != nil {
		return domain.FaceRecognitionStatus{}, err
	}
	return f.Status(ctx)
}

func (f *FaceRecognitionSettings) retry(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	settings, err := f.settings.FaceSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return domain.ErrFaceRecognitionOff
	}
	return f.runtime.Start(settings.Recognizer)
}
