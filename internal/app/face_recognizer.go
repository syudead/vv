package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// faceDetectTimeout bounds one frame's round trip to the helper, its start included; a helper silent for this
// long is treated as hung and the video fails.
const faceDetectTimeout = 2 * time.Minute

// FaceAnalysisQueue is the face analysis queue and the store of its results.
type FaceAnalysisQueue interface {
	QueueLibrary(ctx context.Context, scope domain.FaceQueueScope, decider domain.ActivityDecider) (int, error)
	// RetryVideo queues the content again for the owner's retry of entryID, unless it is running.
	RetryVideo(ctx context.Context, contentKey string, entryID int64) (int, error)
	// ClearPeople deletes every person and face appearance and leaves every video to be analysed again.
	ClearPeople(ctx context.Context) (domain.PeopleCleared, error)
	// FaceJob reads the recognizer and generation current now, for an analysis of the content key that starts now.
	FaceJob(ctx context.Context, contentKey string) (domain.FaceJob, error)
	// Store refuses a result of an older generation with ErrFaceGenerationChanged and writes nothing.
	Store(ctx context.Context, result domain.FaceAnalysis) error
	// HasHelperWork reports a due face or face_image job, for which the helper is kept running.
	HasHelperWork(ctx context.Context) (bool, error)
}

// FaceImageStore keeps the vectors of people's face images and applies the face-image rule.
type FaceImageStore interface {
	// FaceImage returns the person's face with its stored vector, or ErrNotFound when the person has none.
	FaceImage(ctx context.Context, personID int64) (domain.FaceImageJob, error)
	// QueueStaleFaceImages queues a face_image job for every face without a current vector.
	QueueStaleFaceImages(ctx context.Context) (int, error)
	// StoreFaceImageVector refuses a job of an older generation with ErrFaceGenerationChanged, and one whose face
	// was replaced with ErrFaceImageChanged, and writes nothing.
	StoreFaceImageVector(ctx context.Context, job domain.FaceImageJob, vector []float32) (domain.FaceImageDecision, error)
}

// FaceHelper detects the faces of one frame with their vectors; it starts the helper process on first use.
type FaceHelper interface {
	Detect(ctx context.Context, req faceproto.DetectRequest) (faceproto.Detections, error)
	Close() error
}

// FaceFrameSource calls each with every sampled frame of the video at path, in order: a domain.FaceFrameSide
// square of packed BGR that is reused after each returns.
type FaceFrameSource func(ctx context.Context, path string, each func(pix []byte) error) error

// FaceCastMerger applies the cast rule to a video's work in one transaction and reports whether it merged; a video
// no longer in the library returns domain.ErrNotFound.
type FaceCastMerger interface {
	MergeByCast(ctx context.Context, videoID int64) (bool, error)
}

type FaceRuntimeState interface {
	Status(recognizer domain.FaceRecognizer) domain.FaceRuntimeStatus
}

type FaceSettingsReader interface {
	FaceSettings(ctx context.Context) (domain.FaceSettings, error)
}

type MediaFolderLister interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
}

// MediaFileResolver applies the media folders' access rules and returns a path another process may open.
type MediaFileResolver interface {
	ResolveMediaFile(roots []string, path string) (string, error)
}

type FaceRecognizerOptions struct {
	Queue    FaceAnalysisQueue
	Images   FaceImageStore
	Settings FaceSettingsReader
	Runtime  FaceRuntimeState
	// Helper returns a client of the recognizer's helper.
	Helper       func(recognizer domain.FaceRecognizer) FaceHelper
	Frames       FaceFrameSource
	MediaFolders MediaFolderLister
	MediaFiles   MediaFileResolver
	Cast         FaceCastMerger
	// Wake wakes the job scheduler, which runs the face jobs through HandleJob.
	Wake Waker
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// FaceRecognizer runs the face jobs the scheduler grants on the one face lane: it analyses videos, embeds people's
// face images and applies the cast rule. Its kinds are paused while the feature is off or the runtime is not ready,
// and a settings change stops the running analysis or embedding; see docs/design-docs/face-recognition.md.
type FaceRecognizer struct {
	queue        FaceAnalysisQueue
	images       FaceImageStore
	settings     FaceSettingsReader
	runtime      FaceRuntimeState
	newHelper    func(domain.FaceRecognizer) FaceHelper
	frames       FaceFrameSource
	mediaFolders MediaFolderLister
	mediaFiles   MediaFileResolver
	cast         FaceCastMerger
	logger       *slog.Logger
	Waker

	mu sync.Mutex
	// run is the face lane's running analysis or embedding, from before it reads the settings until it ends.
	run *faceRun

	// helperMu guards the helper, which the running job uses and a settings change or shutdown may stop.
	helperMu         sync.Mutex
	helper           FaceHelper
	helperRecognizer domain.FaceRecognizer
}

type faceRun struct {
	cancel context.CancelFunc
	// recognizer is the job's, empty before the job read the settings.
	recognizer domain.FaceRecognizer
}

// NewFaceRecognizer does not process work until the scheduler runs its HandleJob.
func NewFaceRecognizer(opts FaceRecognizerOptions) *FaceRecognizer {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &FaceRecognizer{
		queue: opts.Queue, images: opts.Images, settings: opts.Settings, runtime: opts.Runtime, newHelper: opts.Helper,
		frames: opts.Frames, mediaFolders: opts.MediaFolders, mediaFiles: opts.MediaFiles, cast: opts.Cast,
		logger: logger.With(slog.String("component", "face-recognition")), Waker: opts.Wake,
	}
}

// SettingsChanged is called after the settings are saved: turning the feature off or switching the recognizer
// cancels the running analysis or embedding, which goes back to the queue, and turning it off stops an idle helper.
// It wakes the scheduler, so turning the feature on resumes the paused jobs.
func (f *FaceRecognizer) SettingsChanged(settings domain.FaceSettings) {
	f.mu.Lock()
	switch r := f.run; {
	case r != nil && (!settings.Enabled || r.recognizer != settings.Recognizer):
		// Before the job read the settings its recognizer is empty, so it reads the new ones again.
		r.cancel()
	case r == nil && !settings.Enabled:
		f.closeHelper()
	}
	f.mu.Unlock()
	f.Wake()
}

// PausedJobKinds pauses the face lane while the feature is off or the recognizer's runtime is not ready, so its
// jobs keep their attempts until the runtime's Ready or a settings change wakes the scheduler.
func (f *FaceRecognizer) PausedJobKinds(ctx context.Context) ([]domain.JobKind, error) {
	settings, err := f.settings.FaceSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.Enabled && f.runtime.Status(settings.Recognizer).State == domain.FaceRuntimeReady {
		return nil, nil
	}
	return slices.Clone(domain.FaceLaneJobKinds), nil
}

// VideoImagesFinished only wakes the scheduler: the transaction that finishes the seek stage queued the video.
func (f *FaceRecognizer) VideoImagesFinished(context.Context, int64) { f.Wake() }

func (f *FaceRecognizer) enabled(ctx context.Context) error {
	settings, err := f.settings.FaceSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return domain.ErrFaceRecognitionOff
	}
	return nil
}

// QueueLibrary queues videos for analysis while the feature is on; videos queued while the runtime is not ready
// wait for it.
func (f *FaceRecognizer) QueueLibrary(ctx context.Context, scope domain.FaceQueueScope) (int, error) {
	if !scope.Valid() {
		return 0, fmt.Errorf("unknown face analysis scope %q", scope)
	}
	if err := f.enabled(ctx); err != nil {
		return 0, err
	}
	queued, err := f.queue.QueueLibrary(ctx, scope, domain.ActivityByOwner)
	if err != nil {
		return 0, err
	}
	if queued > 0 {
		f.Wake()
	}
	return queued, nil
}

// ClearPeople deletes every person and all face data, whether the feature is on or off, and wakes the scheduler for
// the videos it queued.
func (f *FaceRecognizer) ClearPeople(ctx context.Context) (domain.PeopleCleared, error) {
	cleared, err := f.queue.ClearPeople(ctx)
	if err == nil && cleared.Queued > 0 {
		f.Wake()
	}
	return cleared, err
}

// RetryActivity queues the entry's video again as the owner's run; a video no longer in the library, or one
// already running, is not retryable.
func (f *FaceRecognizer) RetryActivity(ctx context.Context, entry domain.ActivityEntry) error {
	if err := f.enabled(ctx); err != nil {
		return err
	}
	if entry.Subject.Kind != domain.ActivitySubjectVideo || entry.Subject.Key == "" {
		return domain.ErrActivityNotRetryable
	}
	queued, err := f.queue.RetryVideo(ctx, entry.Subject.Key, entry.ID)
	if err != nil {
		return err
	}
	if queued == 0 {
		return domain.ErrActivityNotRetryable
	}
	f.Wake()
	return nil
}

// HandleJob runs one job of the face lane. Work that a settings change, a recognizer switch, a replaced face or a
// helper that did not start interrupted returns a snooze, so it runs again without counting an attempt; any other
// failure is retried after its delay. After the job the helper, which holds the models in memory, stops unless
// another analysis or embedding is due; the next one starts it again.
func (f *FaceRecognizer) HandleJob(ctx context.Context, job domain.Job) error {
	var err error
	switch job.Kind {
	case domain.JobFace:
		err = f.analyseJob(ctx, job)
	case domain.JobFaceImage:
		err = f.embedJob(ctx, job)
	case domain.JobFaceCast:
		err = f.castJob(ctx, job)
	case domain.JobQueueFaceImages:
		_, err = f.images.QueueStaleFaceImages(ctx)
	default:
		err = fmt.Errorf("unknown face job kind %q", job.Kind)
	}
	f.closeIdleHelper(ctx)
	return err
}

// Close stops the helper once the scheduler has returned.
func (f *FaceRecognizer) Close() {
	f.helperMu.Lock()
	defer f.helperMu.Unlock()
	f.closeHelperLocked()
}

// closeIdleHelper stops the helper unless another analysis or embedding is due and the lane may run it.
func (f *FaceRecognizer) closeIdleHelper(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	due, err := f.queue.HasHelperWork(ctx)
	if err == nil && due {
		paused, pausedErr := f.PausedJobKinds(ctx)
		if pausedErr == nil && len(paused) == 0 {
			return
		}
	}
	f.closeHelper()
}

// analyseJob analyses and stores the video of a claimed face job, read from the location the claim took.
func (f *FaceRecognizer) analyseJob(ctx context.Context, job domain.Job) error {
	run, end := f.begin(ctx)
	defer end()
	face, err := f.queue.FaceJob(run, job.ContentKey)
	if err != nil {
		return f.outcome(ctx, run, fmt.Errorf("%w: %w", errFaceDatabase, err))
	}
	if err := f.start(run, face.Recognizer); err != nil {
		return f.outcome(ctx, run, err)
	}
	result, err := f.analyse(run, face, job.LocationPath)
	if err == nil {
		if err = f.queue.Store(run, result); err != nil && !errors.Is(err, domain.ErrFaceGenerationChanged) {
			err = fmt.Errorf("%w: %w", errFaceDatabase, err)
		}
	}
	if errors.Is(err, domain.ErrFaceHelperStart) {
		// The runtime is marked failed; the video waits until it is ready again.
		f.logger.Warn("the face helper did not start", slog.String("content_key", job.ContentKey), slog.Any("error", err))
	}
	return f.outcome(ctx, run, err)
}

// start records the job's recognizer, so a switch cancels it, and refuses a job the feature or recognizer no longer
// allows.
func (f *FaceRecognizer) start(run context.Context, recognizer domain.FaceRecognizer) error {
	settings, err := f.settings.FaceSettings(run)
	if err != nil {
		return fmt.Errorf("%w: %w", errFaceDatabase, err)
	}
	f.mu.Lock()
	f.run.recognizer = recognizer
	f.mu.Unlock()
	if !settings.Enabled || settings.Recognizer != recognizer {
		return errFaceSettingsChanged
	}
	return run.Err()
}

// begin registers the job's run, which a settings change cancels, until end.
func (f *FaceRecognizer) begin(ctx context.Context) (context.Context, func()) {
	run, cancel := context.WithCancel(ctx)
	f.mu.Lock()
	f.run = &faceRun{cancel: cancel}
	f.mu.Unlock()
	return run, func() {
		f.mu.Lock()
		f.run = nil
		f.mu.Unlock()
		cancel()
	}
}

// outcome turns a face job's error into the scheduler's outcome. A stop of the server is returned as it is, since the
// scheduler leaves the job for startup recovery.
func (f *FaceRecognizer) outcome(ctx, run context.Context, err error) error {
	switch {
	case err == nil, ctx.Err() != nil:
		return err
	case run.Err() != nil, errors.Is(err, errFaceSettingsChanged), errors.Is(err, domain.ErrFaceGenerationChanged),
		errors.Is(err, domain.ErrFaceImageChanged), errors.Is(err, domain.ErrFaceHelperStart):
		return fmt.Errorf("%w: %w", domain.ErrJobSnoozed, err)
	default:
		return shortFailure{err}
	}
}

var (
	// errFaceDatabase marks a failure of the database rather than of the video.
	errFaceDatabase = errors.New("face analysis database error")
	// errFaceSettingsChanged marks a job the feature or the recognizer no longer allows when it starts.
	errFaceSettingsChanged = errors.New("the face recognition settings changed")
)

// analyse reads every sampled frame of the video at path and returns its main people.
func (f *FaceRecognizer) analyse(ctx context.Context, job domain.FaceJob, path string) (domain.FaceAnalysis, error) {
	result := domain.FaceAnalysis{ContentKey: job.ContentKey, Recognizer: job.Recognizer, Generation: job.Generation}
	spec, ok := domain.LookupFaceRecognizer(job.Recognizer)
	if !ok {
		return result, fmt.Errorf("unknown face recognizer %q", job.Recognizer)
	}
	resolved, err := f.resolve(ctx, path)
	if err != nil {
		return result, err
	}
	helper := f.helperFor(job.Recognizer)
	video := newFaceVideo(spec.Thresholds)
	err = f.frames(ctx, resolved, func(pix []byte) error {
		faces, err := detectFaces(ctx, helper, faceproto.DetectRequest{
			Width: domain.FaceFrameSide, Height: domain.FaceFrameSide, Pix: pix,
		})
		if err != nil {
			return err
		}
		return video.add(pix, faces)
	})
	if err != nil {
		return result, err
	}
	result.People = video.people()
	return result, nil
}

// resolve returns the path the media folders' access rules let another process open.
func (f *FaceRecognizer) resolve(ctx context.Context, path string) (string, error) {
	folders, err := f.mediaFolders.ListMediaFolders(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errFaceDatabase, err)
	}
	roots := make([]string, len(folders))
	for i, folder := range folders {
		roots[i] = folder.Path
	}
	resolved, err := f.mediaFiles.ResolveMediaFile(roots, path)
	if err != nil {
		return "", fmt.Errorf("cannot open the video's file: %w", err)
	}
	return resolved, nil
}

// castJob applies the cast rule to the job's video. A video gone from the library needs nothing, and a merge refused
// for good is logged and not tried again, since retrying cannot change it.
func (f *FaceRecognizer) castJob(ctx context.Context, job domain.Job) error {
	merged, err := f.cast.MergeByCast(ctx, job.VideoID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
	case errors.Is(err, domain.ErrFaceMergeRefused):
		f.logger.Warn("the cast rule's merge was refused", slog.Int64("video_id", job.VideoID), slog.Any("error", err))
	case err != nil:
		return err
	case merged:
		f.logger.Info("merged a provisional person into the only performer without a face",
			slog.Int64("video_id", job.VideoID))
	}
	return nil
}

func detectFaces(ctx context.Context, helper FaceHelper, req faceproto.DetectRequest) ([]domain.FaceDetection, error) {
	frameCtx, cancel := context.WithTimeout(ctx, faceDetectTimeout)
	defer cancel()
	d, err := helper.Detect(frameCtx, req)
	if err != nil {
		if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("the face helper did not answer a frame within %s", faceDetectTimeout)
		}
		return nil, err
	}
	return faceDetections(d), nil
}

// helperFor keeps one helper for the recognizer across jobs and stops the other recognizer's.
func (f *FaceRecognizer) helperFor(r domain.FaceRecognizer) FaceHelper {
	f.helperMu.Lock()
	defer f.helperMu.Unlock()
	if f.helper != nil && f.helperRecognizer == r {
		return f.helper
	}
	f.closeHelperLocked()
	f.helper, f.helperRecognizer = f.newHelper(r), r
	return f.helper
}

func (f *FaceRecognizer) closeHelper() {
	f.helperMu.Lock()
	defer f.helperMu.Unlock()
	f.closeHelperLocked()
}

func (f *FaceRecognizer) closeHelperLocked() {
	if f.helper == nil {
		return
	}
	if err := f.helper.Close(); err != nil {
		f.logger.Warn("the face helper did not stop cleanly", slog.Any("error", err))
	}
	f.helper = nil
}
