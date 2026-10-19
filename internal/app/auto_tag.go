package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const autoTagRetryDelay = 5 * time.Second

// Model loading can take minutes, so allow a longer classifier check.
const classifierCheckTimeout = 5 * time.Minute

const maxClassifierErrorLength = 500

func failureReason(err error) string {
	reason := err.Error()
	if len(reason) > maxClassifierErrorLength {
		reason = reason[:maxClassifierErrorLength]
	}
	return reason
}

type AutoTagQueue interface {
	// Open an auto_tag activity for each queued video using its decider.
	QueueVideos(ctx context.Context, videoIDs []int64, mode domain.AutoTagQueueMode, decider domain.ActivityDecider) (int, error)
	QueueLibrary(ctx context.Context, mode domain.AutoTagQueueMode, decider domain.ActivityDecider) (int, error)
	// Queue the video again for the owner's retry of entryID, joining that entry's pipelines.
	RetryVideo(ctx context.Context, videoID, entryID int64) (int, error)
	// Claim the oldest queued classification or return ErrNoJob.
	Claim(ctx context.Context) (domain.AutoTagJob, error)
	// Subject returns the video and its representative location's registered folder, or ErrNotFound.
	Subject(ctx context.Context, contentKey string) (domain.AutoTagVideo, error)
	// Exclude tentative tags, already-confirmed tags, and tags rejected for this video.
	Candidates(ctx context.Context, contentKey string) ([]domain.AutoTagCandidate, error)
	// Finish stores probabilities and applied tags; a nonempty decider overrides the activity's
	// decision source.
	Finish(ctx context.Context, contentKey string, selection domain.AutoTagSelection, decider domain.ActivityDecider) error
	// Confirm/reject only tentative tags; otherwise return ErrNotFound.
	ConfirmTentative(ctx context.Context, videoID, tagID int64) error
	RejectTentative(ctx context.Context, videoID, tagID int64) error
	Fail(ctx context.Context, contentKey, reason string) error
	RequeueRunning(ctx context.Context) (int, error)
	Counts(ctx context.Context) (domain.AutoTagCounts, error)
}

type AutoTagSettingsStore interface {
	AutoTagSettings(ctx context.Context) (domain.AutoTagSettings, error)
	SaveAutoTagSettings(ctx context.Context, settings domain.AutoTagSettings) error
}

type VideoImageSource interface {
	ThumbnailJPEG(contentKey string) ([]byte, error)
	// Return evenly spaced JPEG frames in chronological order; missing previews are errors.
	SeekFramesJPEG(contentKey string, n int) ([][]byte, error)
}

type AutoTagUndoStore interface {
	// Undo the tag and write the owner's undo activity in one transaction.
	UndoAutoTag(ctx context.Context, entryID, tagID int64) (domain.ActivityEntry, error)
}

type AutoTaggerOptions struct {
	Queue    AutoTagQueue
	Settings AutoTagSettingsStore
	// Nil disables undo.
	Undo       AutoTagUndoStore
	Classifier Classifier
	Images     VideoImageSource
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// AutoTagger classifies sequentially through a provider-independent interface and applies existing
// tags by probability thresholds; see docs/design-docs/auto-tagging.md.
type AutoTagger struct {
	queue      AutoTagQueue
	settings   AutoTagSettingsStore
	undo       AutoTagUndoStore
	classifier Classifier
	images     VideoImageSource
	logger     *slog.Logger
	wake       chan struct{}

	saveMu sync.Mutex
}

// NewAutoTagger does not process work until Run.
func NewAutoTagger(opts AutoTaggerOptions) *AutoTagger {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoTagger{
		queue:      opts.Queue,
		settings:   opts.Settings,
		undo:       opts.Undo,
		classifier: opts.Classifier,
		images:     opts.Images,
		logger:     logger.With(slog.String("component", "auto-tagging")),
		// Coalesce wakeups in a one-element buffer.
		wake: make(chan struct{}, 1),
	}
}

// Wake is nonblocking.
func (a *AutoTagger) Wake() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *AutoTagger) Status(ctx context.Context) (domain.AutoTagStatus, error) {
	settings, err := a.settings.AutoTagSettings(ctx)
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	counts, err := a.queue.Counts(ctx)
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	return domain.AutoTagStatus{Settings: settings, Counts: counts}, nil
}

// SaveSettings validates then wakes the worker so enabling resumes retained work.
func (a *AutoTagger) SaveSettings(ctx context.Context, settings domain.AutoTagSettings) (domain.AutoTagStatus, error) {
	validated, err := domain.ValidateAutoTagSettings(settings)
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	a.saveMu.Lock()
	err = a.settings.SaveAutoTagSettings(ctx, validated)
	a.saveMu.Unlock()
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	a.Wake()
	return a.Status(ctx)
}

// CheckClassifier sends one short probability question only on explicit request, regardless of
// feature enablement; model loading may take minutes.
func (a *AutoTagger) CheckClassifier(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, classifierCheckTimeout)
	defer cancel()
	const key = "check"
	probabilities, err := a.classifier.Classify(ctx, domain.ClassifierRequest{
		Input:     domain.ClassifierInput{Facts: map[string]string{"title": "Classifier check"}},
		Questions: []domain.ClassifierQuestion{{Key: key, Text: "Is this a check of the classifier?"}},
	})
	if err != nil {
		return err
	}
	if _, ok := probabilities[key]; !ok {
		return errors.New("the classifier did not answer the question")
	}
	return nil
}

func (a *AutoTagger) enabled(ctx context.Context) (bool, error) {
	settings, err := a.settings.AutoTagSettings(ctx)
	return settings.Enabled, err
}

// QueueVideo may reclassify completed videos, but skips active classifications and videos outside
// registered folders; disabled tagging returns ErrAutoTaggingOff.
func (a *AutoTagger) QueueVideo(ctx context.Context, videoID int64) (bool, error) {
	if on, err := a.enabled(ctx); err != nil {
		return false, err
	} else if !on {
		return false, domain.ErrAutoTaggingOff
	}
	queued, err := a.queue.QueueVideos(ctx, []int64{videoID}, domain.AutoTagQueueAgain, domain.ActivityByOwner)
	if err != nil {
		return false, err
	}
	if queued > 0 {
		a.Wake()
	}
	return queued > 0, nil
}

// QueueLibrary returns the queued count and rejects disabled tagging.
func (a *AutoTagger) QueueLibrary(ctx context.Context, scope domain.AutoTagScope) (int, error) {
	if !scope.Valid() {
		return 0, fmt.Errorf("unknown auto-tagging scope %q", scope)
	}
	if on, err := a.enabled(ctx); err != nil {
		return 0, err
	} else if !on {
		return 0, domain.ErrAutoTaggingOff
	}
	queued, err := a.queue.QueueLibrary(ctx, scope.QueueMode(), domain.ActivityByOwner)
	if err != nil {
		return 0, err
	}
	if queued > 0 {
		a.Wake()
	}
	return queued, nil
}

func (a *AutoTagger) ConfirmTag(ctx context.Context, videoID, tagID int64) error {
	return a.queue.ConfirmTentative(ctx, videoID, tagID)
}

// RejectTag also prevents automatic reattachment to this video.
func (a *AutoTagger) RejectTag(ctx context.Context, videoID, tagID int64) error {
	return a.queue.RejectTentative(ctx, videoID, tagID)
}

// RetryActivity creates a new owner-initiated run and closes the previous activity; reject disabled
// tagging or a missing video.
func (a *AutoTagger) RetryActivity(ctx context.Context, entry domain.ActivityEntry) error {
	if on, err := a.enabled(ctx); err != nil {
		return err
	} else if !on {
		return domain.ErrAutoTaggingOff
	}
	videoID := entry.VideoIDs[0]
	if videoID == 0 {
		return domain.ErrActivityNotRetryable
	}
	queued, err := a.queue.RetryVideo(ctx, videoID, entry.ID)
	if err != nil {
		return err
	}
	if queued > 0 {
		a.Wake()
	}
	return nil
}

// UndoActivity returns the owner's undo record; absent undo support returns ErrActivityNotUndoable.
func (a *AutoTagger) UndoActivity(ctx context.Context, entry domain.ActivityEntry, tagID int64) (domain.ActivityEntry, error) {
	if a.undo == nil {
		return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
	}
	return a.undo.UndoAutoTag(ctx, entry.ID, tagID)
}

// VideoImagesFinished only wakes the worker: the transaction that finishes the seek stage has already queued the
// video when after-import tagging is on, so the import pipeline never looks done before tagging joins it.
func (a *AutoTagger) VideoImagesFinished(context.Context, int64) {
	a.Wake()
}

// Recover must run before Run.
func (a *AutoTagger) Recover(ctx context.Context) error {
	requeued, err := a.queue.RequeueRunning(ctx)
	if err != nil {
		return err
	}
	if requeued > 0 {
		a.logger.Info("requeued interrupted auto-tagging jobs", slog.Int("count", requeued))
	}
	return nil
}

// Run checks immediately at startup, then processes serially; disabled tagging retains queued work
// without contacting the classifier until SaveSettings wakes it.
func (a *AutoTagger) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		on, err := a.enabled(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.logger.Error("cannot read the auto-tagging settings", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(autoTagRetryDelay):
			}
			continue
		}
		if !on {
			select {
			case <-ctx.Done():
				return
			case <-a.wake:
			}
			continue
		}
		job, err := a.queue.Claim(ctx)
		switch {
		case errors.Is(err, domain.ErrNoJob):
			select {
			case <-ctx.Done():
				return
			case <-a.wake:
			}
			continue
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			a.logger.Error("cannot claim an auto-tagging job", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(autoTagRetryDelay):
			}
			continue
		}
		a.process(ctx, job)
	}
}

// Interrupted work is left for startup Recover rather than recorded as failed.
func (a *AutoTagger) process(ctx context.Context, job domain.AutoTagJob) {
	selection, decider, err := a.decide(ctx, job)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		a.logger.Warn("auto-tagging failed", slog.String("content_key", job.ContentKey), slog.Any("error", err))
		if err := a.queue.Fail(ctx, job.ContentKey, failureReason(err)); err != nil {
			a.logger.Error("cannot record the auto-tagging failure", slog.Any("error", err))
		}
		return
	}
	if err := a.queue.Finish(ctx, job.ContentKey, selection, decider); err != nil {
		a.logger.Error("cannot record the auto-tagging result", slog.String("content_key", job.ContentKey), slog.Any("error", err))
	}
}

// Missing videos yield no decision for Finish to remove; no candidate questions preserves the
// initiating decider, while classifier-backed decisions use ai.
func (a *AutoTagger) decide(ctx context.Context, job domain.AutoTagJob) (domain.AutoTagSelection, domain.ActivityDecider, error) {
	var selection domain.AutoTagSelection
	settings, err := a.settings.AutoTagSettings(ctx)
	if err != nil {
		return selection, "", err
	}
	found, err := a.queue.Subject(ctx, job.ContentKey)
	if errors.Is(err, domain.ErrNotFound) {
		return selection, "", nil
	}
	if err != nil {
		return selection, "", err
	}
	candidates, err := a.queue.Candidates(ctx, job.ContentKey)
	if errors.Is(err, domain.ErrNotFound) {
		return selection, "", nil
	}
	if err != nil {
		return selection, "", err
	}
	if len(candidates) == 0 {
		return selection, "", nil
	}

	video := found.Video
	subject := domain.NewAutoTagSubject(video.Title, found.MediaFolder, video.Path)
	subject.Images = a.videoImages(job.ContentKey, video)

	questions := make([]domain.ClassifierQuestion, len(candidates))
	for i, candidate := range candidates {
		questions[i] = domain.AutoTagQuestionFor(candidate)
	}
	probabilities, err := a.classifier.Classify(ctx, domain.ClassifierRequest{
		Input:     subject.ClassifierInput(),
		Questions: questions,
	})
	if err != nil {
		return selection, "", err
	}
	selection = domain.SelectAutoTags(candidates, probabilities, settings)
	return selection, domain.ActivityByAI, nil
}

// Skip unreadable images and classify with available metadata and images.
func (a *AutoTagger) videoImages(contentKey string, video domain.Video) [][]byte {
	if a.images == nil {
		return nil
	}
	var images [][]byte
	if video.HasThumbnail() {
		image, err := a.images.ThumbnailJPEG(contentKey)
		if err != nil {
			a.logger.Warn("cannot read the thumbnail for auto-tagging",
				slog.String("content_key", contentKey), slog.Any("error", err))
		} else {
			images = append(images, image)
		}
	}
	if video.SeekThumbnailState == domain.SeekThumbnailDone {
		frames, err := a.images.SeekFramesJPEG(contentKey, domain.AutoTagFrameCount)
		if err != nil {
			a.logger.Warn("cannot read the seek preview frames for auto-tagging",
				slog.String("content_key", contentKey), slog.Any("error", err))
		} else {
			images = append(images, frames...)
		}
	}
	return images
}
