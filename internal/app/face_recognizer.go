package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/faceproto"
)

// faceRetryDelay spaces attempts after the settings or the queue could not be read.
const faceRetryDelay = 5 * time.Second

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
	// Claim takes the next queued video with the recognizer and generation current at the claim, or ErrNoJob.
	Claim(ctx context.Context) (domain.FaceJob, error)
	// VideoPaths returns the registered locations of the content, or ErrNotFound when the library lost it.
	VideoPaths(ctx context.Context, contentKey string) ([]string, error)
	// Store refuses a result of an older generation with ErrFaceGenerationChanged and writes nothing.
	Store(ctx context.Context, result domain.FaceAnalysis) error
	Fail(ctx context.Context, contentKey, reason string) error
	RequeueRunning(ctx context.Context) (int, error)
}

// FaceImageStore keeps the vectors of people's face images and applies the face-image rule.
type FaceImageStore interface {
	// FaceImage returns the person's face with its stored vector, or ErrNotFound when the person has none.
	FaceImage(ctx context.Context, personID int64) (domain.FaceImageJob, error)
	// StaleFaceImage returns a face without a current vector, or ErrNoJob.
	StaleFaceImage(ctx context.Context) (domain.FaceImageJob, error)
	// StoreFaceImageVector refuses a job of an older generation with ErrFaceGenerationChanged and writes nothing.
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
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// FaceRecognizer analyses queued videos for faces one at a time while the feature is on and the runtime is ready,
// and stops the running video when the feature is turned off or the recognizer changes; see
// docs/design-docs/face-recognition.md.
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
	wake         chan struct{}

	mu sync.Mutex
	// run is the worker's current attempt, from before it reads the settings until the video ends.
	run *faceRun
	// castChecks are the videos whose work information changed since the worker last applied the cast rule.
	castChecks map[int64]struct{}
	// pending holds people whose face changed or was named since; the worker applies their face before the next video.
	pending map[int64]bool

	// helper is used by the worker goroutine only.
	helper           FaceHelper
	helperRecognizer domain.FaceRecognizer
	// stranded marks a claimed video left running because returning it to the queue failed; the worker returns it
	// before the next claim, since the owner's runs and retries skip running videos.
	stranded bool
}

type faceRun struct {
	cancel context.CancelFunc
	// recognizer is the claimed job's, empty before the claim.
	recognizer domain.FaceRecognizer
}

func NewFaceRecognizer(opts FaceRecognizerOptions) *FaceRecognizer {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &FaceRecognizer{
		queue: opts.Queue, images: opts.Images, settings: opts.Settings, runtime: opts.Runtime, newHelper: opts.Helper,
		frames: opts.Frames, mediaFolders: opts.MediaFolders, mediaFiles: opts.MediaFiles, cast: opts.Cast,
		logger: logger.With(slog.String("component", "face-recognition")),
		wake:   make(chan struct{}, 1), pending: map[int64]bool{},
	}
}

// Wake is nonblocking.
func (f *FaceRecognizer) Wake() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

// SettingsChanged is called after the settings are saved: turning the feature off or switching the recognizer
// cancels the running video, which goes back to the queue.
func (f *FaceRecognizer) SettingsChanged(settings domain.FaceSettings) {
	f.mu.Lock()
	if r := f.run; r != nil && (!settings.Enabled || r.recognizer != settings.Recognizer) {
		// Before the claim the recognizer is empty, so the worker reads the new settings again.
		r.cancel()
	}
	f.mu.Unlock()
	f.Wake()
}

// PersonImageChanged has the worker embed the person's face, or apply it again after a naming, before the next video.
func (f *FaceRecognizer) PersonImageChanged(event domain.PersonImageChanged) {
	f.pend(event.PersonID)
	f.Wake()
}

func (f *FaceRecognizer) pend(personID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending[personID] = true
}

// VideoImagesFinished only wakes the worker: the transaction that finishes the seek stage queued the video.
func (f *FaceRecognizer) VideoImagesFinished(context.Context, int64) { f.Wake() }

// WorkInfoChanged has the worker apply the cast rule to the videos while the feature is on; changes made while it is
// off are not checked later.
func (f *FaceRecognizer) WorkInfoChanged(videoIDs []int64) {
	f.addCastChecks(videoIDs)
	f.Wake()
}

func (f *FaceRecognizer) addCastChecks(videoIDs []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.castChecks == nil {
		f.castChecks = map[int64]struct{}{}
	}
	for _, id := range videoIDs {
		f.castChecks[id] = struct{}{}
	}
}

func (f *FaceRecognizer) takeCastChecks() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := slices.Sorted(maps.Keys(f.castChecks))
	f.castChecks = nil
	return ids
}

// mergeByCast applies the cast rule to each video whose work information changed; the videos it could not check
// for a store error stay for the next attempt, and a merge refused for good is dropped, since retrying it would hold
// up the queue.
func (f *FaceRecognizer) mergeByCast(ctx context.Context) error {
	ids := f.takeCastChecks()
	for i, id := range ids {
		merged, err := f.cast.MergeByCast(ctx, id)
		switch {
		case errors.Is(err, domain.ErrNotFound):
		case errors.Is(err, domain.ErrFaceMergeRefused):
			f.logger.Warn("the cast rule's merge was refused", slog.Int64("video_id", id), slog.Any("error", err))
		case err != nil:
			f.addCastChecks(ids[i:])
			return err
		case merged:
			f.logger.Info("merged a provisional person into the only performer without a face",
				slog.Int64("video_id", id))
		}
	}
	return nil
}

// Recover must run before Run.
func (f *FaceRecognizer) Recover(ctx context.Context) error {
	requeued, err := f.queue.RequeueRunning(ctx)
	if err != nil {
		return err
	}
	if requeued > 0 {
		f.logger.Info("requeued interrupted face analyses", slog.Int("count", requeued))
	}
	return nil
}

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

// ClearPeople deletes every person and all face data, whether the feature is on or off, and wakes the worker for the
// videos it queued.
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

// faceStep is what the worker does after one attempt.
type faceStep int

const (
	faceNext faceStep = iota
	// faceIdle waits for a wake: the feature is off, the runtime is not ready, or nothing is queued.
	faceIdle
	faceRetry
)

// Run analyses one video at a time until ctx ends; a video interrupted by the end of ctx stays running for Recover.
func (f *FaceRecognizer) Run(ctx context.Context) {
	defer f.closeHelper()
	for ctx.Err() == nil {
		var wait <-chan time.Time
		switch f.step(ctx) {
		case faceNext:
			continue
		case faceIdle:
			// The helper holds the models in memory; it starts again with the next video.
			f.closeHelper()
		case faceRetry:
			wait = time.After(faceRetryDelay)
		}
		select {
		case <-ctx.Done():
		case <-f.wake:
		case <-wait:
		}
	}
}

func (f *FaceRecognizer) step(ctx context.Context) faceStep {
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	f.begin(cancel)
	defer f.end()
	settings, err := f.settings.FaceSettings(run)
	if err != nil {
		return f.retryUnless(run, "cannot read the face recognition settings", err)
	}
	if f.stranded {
		if _, err := f.queue.RequeueRunning(run); err != nil {
			return f.retryUnless(run, "cannot return the face analysis to the queue", err)
		}
		f.stranded = false
	}
	if !settings.Enabled {
		f.takeCastChecks()
		return faceIdle
	}
	if err := f.mergeByCast(run); err != nil {
		return f.retryUnless(run, "cannot apply the cast rule", err)
	}
	if f.runtime.Status(settings.Recognizer).State != domain.FaceRuntimeReady {
		return faceIdle
	}
	if step, ok := f.embedNext(run, settings); ok {
		return step
	}
	job, err := f.queue.Claim(run)
	if errors.Is(err, domain.ErrNoJob) {
		return faceIdle
	}
	if err != nil {
		return f.retryUnless(run, "cannot claim a face analysis job", err)
	}
	f.claimed(job.Recognizer)
	if job.Recognizer != settings.Recognizer || run.Err() != nil {
		// The settings changed between the read and the claim.
		f.requeue(ctx, job)
		return faceNext
	}
	return f.process(ctx, run, job)
}

func (f *FaceRecognizer) retryUnless(run context.Context, message string, err error) faceStep {
	if run.Err() != nil {
		return faceNext
	}
	f.logger.Error(message, slog.Any("error", err))
	return faceRetry
}

func (f *FaceRecognizer) begin(cancel context.CancelFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.run = &faceRun{cancel: cancel}
}

func (f *FaceRecognizer) claimed(recognizer domain.FaceRecognizer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.run.recognizer = recognizer
}

func (f *FaceRecognizer) end() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.run = nil
}

// process analyses and stores one claimed video. run ends when the settings change; ctx when the server stops.
// A database error returns the video to the queue and backs off, so a transient error does not strand it running.
func (f *FaceRecognizer) process(ctx, run context.Context, job domain.FaceJob) faceStep {
	result, err := f.analyse(run, job)
	if err == nil {
		if err = f.queue.Store(run, result); err != nil && !errors.Is(err, domain.ErrFaceGenerationChanged) {
			err = fmt.Errorf("%w: %w", errFaceDatabase, err)
		}
	}
	switch {
	case err == nil:
	case ctx.Err() != nil:
		// The server is stopping; Recover queues the video again at startup.
	case run.Err() != nil, errors.Is(err, domain.ErrFaceGenerationChanged):
		f.requeue(ctx, job)
	case errors.Is(err, domain.ErrFaceHelperStart):
		// The runtime is marked failed; the video waits until it is ready again.
		f.logger.Warn("the face helper did not start", slog.String("content_key", job.ContentKey), slog.Any("error", err))
		f.requeue(ctx, job)
	case errors.Is(err, errFaceDatabase):
		f.logger.Error("face analysis hit a database error", slog.String("content_key", job.ContentKey), slog.Any("error", err))
		f.requeue(ctx, job)
		return faceRetry
	default:
		f.logger.Warn("face analysis failed", slog.String("content_key", job.ContentKey), slog.Any("error", err))
		if err := f.queue.Fail(ctx, job.ContentKey, failureReason(err)); err != nil {
			f.logger.Error("cannot record the face analysis failure", slog.Any("error", err))
			f.requeue(ctx, job)
			return faceRetry
		}
	}
	return faceNext
}

func (f *FaceRecognizer) requeue(ctx context.Context, job domain.FaceJob) {
	if _, err := f.queue.RequeueRunning(ctx); err != nil {
		f.stranded = true
		f.logger.Error("cannot return the face analysis to the queue", slog.String("content_key", job.ContentKey),
			slog.Any("error", err))
	}
}

// errFaceDatabase marks a failure of the database rather than of the video.
var errFaceDatabase = errors.New("face analysis database error")

// analyse reads every sampled frame of the video and returns its main people. A video no longer in the library
// gives an empty result, which removes its queue row.
func (f *FaceRecognizer) analyse(ctx context.Context, job domain.FaceJob) (domain.FaceAnalysis, error) {
	result := domain.FaceAnalysis{ContentKey: job.ContentKey, Recognizer: job.Recognizer, Generation: job.Generation}
	spec, ok := domain.LookupFaceRecognizer(job.Recognizer)
	if !ok {
		return result, fmt.Errorf("unknown face recognizer %q", job.Recognizer)
	}
	paths, err := f.queue.VideoPaths(ctx, job.ContentKey)
	if errors.Is(err, domain.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("%w: %w", errFaceDatabase, err)
	}
	path, err := f.resolve(ctx, paths)
	if err != nil {
		return result, err
	}
	helper := f.helperFor(job.Recognizer)
	video := newFaceVideo(spec.Thresholds)
	err = f.frames(ctx, path, func(pix []byte) error {
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

// resolve returns the first location the media folders' access rules let another process open.
func (f *FaceRecognizer) resolve(ctx context.Context, paths []string) (string, error) {
	folders, err := f.mediaFolders.ListMediaFolders(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errFaceDatabase, err)
	}
	roots := make([]string, len(folders))
	for i, folder := range folders {
		roots[i] = folder.Path
	}
	var errs []error
	for _, path := range paths {
		resolved, err := f.mediaFiles.ResolveMediaFile(roots, path)
		if err == nil {
			return resolved, nil
		}
		errs = append(errs, err)
	}
	return "", fmt.Errorf("cannot open the video's file: %w", errors.Join(errs...))
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

// helperFor keeps one helper for the recognizer across videos and stops the other recognizer's.
func (f *FaceRecognizer) helperFor(r domain.FaceRecognizer) FaceHelper {
	if f.helper != nil && f.helperRecognizer == r {
		return f.helper
	}
	f.closeHelper()
	f.helper, f.helperRecognizer = f.newHelper(r), r
	return f.helper
}

func (f *FaceRecognizer) closeHelper() {
	if f.helper == nil {
		return
	}
	if err := f.helper.Close(); err != nil {
		f.logger.Warn("the face helper did not stop cleanly", slog.Any("error", err))
	}
	f.helper = nil
}
