package app

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Keep one in-memory session per video; reads poll provider state without emitting events.

// Sweep idle sessions and timed-out runs at this interval.
const thumbnailEditSweepInterval = 30 * time.Second

// Bound provider cancellation during shutdown.
const thumbnailEditCloseTimeout = 10 * time.Second

// Bound each provider status, cancel or output request.
const thumbnailEditProviderTimeout = 30 * time.Second

// Bound how long failed provider cancellations are retried by sweeps.
const thumbnailEditCancelRetryLimit = time.Hour

// ComfyUISettingsReader reads connection settings; implemented by store.SettingsStore.
type ComfyUISettingsReader interface {
	ComfyUISettings(ctx context.Context) (domain.ComfyUISettings, error)
}

// ThumbnailFrameSource extracts a PNG at positionMs, preserving the video long side up to domain.ThumbnailFrameMaxSide; implemented by media.Assets.
type ThumbnailFrameSource interface {
	FrameAt(ctx context.Context, path string, positionMs int64) (domain.ThumbnailFrame, error)
	// AutoThumbnailPositions returns automatic then first-frame positions in milliseconds for durationMs (zero means unknown); see docs/design-docs/thumbnail-editing.md.
	AutoThumbnailPositions(durationMs int64) []int64
}

// ThumbnailImageReader atomically reads the applied original (or legacy thumbnail) and selected position; implemented by store.OverrideStore.
type ThumbnailImageReader interface {
	CurrentThumbnailSelection(ctx context.Context, contentKey string) (domain.CurrentThumbnailSelection, error)
}

// ThumbnailImageApplier applies an image with its origin; implemented by Ingest. Missing or changed content returns ErrContentChanged; unreadable images return ErrThumbnailImageInvalid.
type ThumbnailImageApplier interface {
	ApplyThumbnailImage(
		ctx context.Context, videoID int64, contentKey string, image []byte, origin domain.ThumbnailImageOrigin,
	) (domain.Video, error)
}

// ForegroundReads holds the disk of a media file the owner is waiting on to one background read until end is called;
// implemented by jobs.Scheduler.
type ForegroundReads interface {
	ReadInForeground(path string) (end func())
}

type ThumbnailEditsOptions struct {
	Settings ComfyUISettingsReader
	Editor   ImageEditor
	Frames   ThumbnailFrameSource
	// Nil Foreground leaves background reads alone while a frame is read.
	Foreground ForegroundReads
	Images     ThumbnailImageReader
	Applier    ThumbnailImageApplier
	Logger     *slog.Logger
	// Now defaults to time.Now and is replaceable by a test clock.
	Now func() time.Time
}

// ThumbnailEdits holds one session per video; all sessions disappear on restart.
type ThumbnailEdits struct {
	settings ComfyUISettingsReader
	editor   ImageEditor
	frames   ThumbnailFrameSource
	// foreground is nil when nothing holds a disk for frame reads.
	foreground ForegroundReads
	images     ThumbnailImageReader
	applier    ThumbnailImageApplier
	logger     *slog.Logger
	now        func() time.Time

	// Protects byID and byVideo; may be acquired while holding a session lock, never in reverse order.
	mu      sync.Mutex
	byID    map[string]*thumbnailEditSession
	byVideo map[int64]string

	// Protects pending; never acquire other locks or call providers while held.
	pendingMu sync.Mutex
	// Failed cancellations keyed by job ID, retried by sweeps even after the session ends.
	pending map[string]*thumbnailEditPendingCancel
}

type thumbnailEditPendingCancel struct {
	videoID  int64
	job      domain.ImageEditJob
	state    domain.ThumbnailEditState
	failedAt time.Time
	// Non-nil only during a retry; closed when it finishes, under pendingMu.
	attempt chan struct{}
}

// The session mutex protects all fields below.
type thumbnailEditSession struct {
	mu         sync.Mutex
	id         string
	videoID    int64
	contentKey string
	frame      domain.ThumbnailFrame
	// source identifies the starting image; positionMs is nil for a saved original image.
	source     domain.ThumbnailEditSource
	positionMs *int64
	ended      bool
	lastReadAt time.Time
	run        *thumbnailEditRun
	result     []byte
	// write, when set, receives the applied result instead of this node's thumbnail store.
	write domain.ThumbnailImageWrite
}

// A submission failure has no job and is already failed.
type thumbnailEditRun struct {
	job         domain.ImageEditJob
	submitted   bool
	state       domain.ThumbnailEditState
	submittedAt time.Time
	endedAt     time.Time
	progress    *domain.ImageEditProgress
	failure     *domain.ImageEditError
}

func (r *thumbnailEditRun) inProgress() bool {
	return r != nil && r.submitted && !r.state.Terminal()
}

// NewThumbnailEdits creates the session manager.
func NewThumbnailEdits(opts ThumbnailEditsOptions) *ThumbnailEdits {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &ThumbnailEdits{
		settings:   opts.Settings,
		editor:     opts.Editor,
		frames:     opts.Frames,
		foreground: opts.Foreground,
		images:     opts.Images,
		applier:    opts.Applier,
		logger:     logger,
		now:        now,
		byID:       map[string]*thumbnailEditSession{},
		byVideo:    map[int64]string{},
		pending:    map[string]*thumbnailEditPendingCancel{},
	}
}

// Start snapshots the requested source into a ready session and cancels/replaces any prior session for the video. It needs no ComfyUI connection, so the page can read a frame or the current thumbnail to frame it; runs need one. An image source that does not decode returns ErrThumbnailUploadInvalid, unread content ErrNotFound, invalid/missing positions the CheckThumbnailPosition error, unavailable images ErrThumbnailFrameUnavailable, and an empty path needing frame extraction ErrMediaFileUnavailable.
func (e *ThumbnailEdits) Start(
	ctx context.Context, video domain.Video, path string, start domain.ThumbnailEditStart,
) (domain.ThumbnailEdit, error) {
	if video.ContentKey == "" {
		return domain.ThumbnailEdit{}, domain.ErrNotFound
	}
	image, err := e.startImage(ctx, video, path, start)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	return e.open(ctx, video, start.Source, image, nil)
}

// StartFrom starts a session for a video this node does not hold from start.Image, the frame at start.PositionMs
// or the current thumbnail the caller read; Apply hands the result to write and never touches this node's
// thumbnail store. An image that does not decode returns ErrThumbnailUploadInvalid for an image source and
// ErrThumbnailFrameUnavailable otherwise.
func (e *ThumbnailEdits) StartFrom(
	ctx context.Context, video domain.Video, start domain.ThumbnailEditStart, write domain.ThumbnailImageWrite,
) (domain.ThumbnailEdit, error) {
	if video.ContentKey == "" {
		return domain.ThumbnailEdit{}, domain.ErrNotFound
	}
	frame, err := thumbnailEditFrameFromImage(start.Image)
	if err != nil {
		unreadable := domain.ErrThumbnailFrameUnavailable
		if start.Source == domain.ThumbnailEditFromImage {
			unreadable = domain.ErrThumbnailUploadInvalid
		}
		return domain.ThumbnailEdit{}, errors.Join(unreadable, err)
	}
	image := thumbnailEditImage{frame: frame}
	if start.Source == domain.ThumbnailEditFromFrame {
		image.positionMs = start.PositionMs
	}
	return e.open(ctx, video, start.Source, image, write)
}

// open publishes a ready session for video, replacing its previous one.
func (e *ThumbnailEdits) open(
	ctx context.Context, video domain.Video, source domain.ThumbnailEditSource, image thumbnailEditImage, write domain.ThumbnailImageWrite,
) (domain.ThumbnailEdit, error) {
	id, err := newThumbnailEditID()
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	session := &thumbnailEditSession{
		id: id, videoID: video.ID, contentKey: video.ContentKey, frame: image.frame,
		source: source, positionMs: image.positionMs, lastReadAt: e.now(), write: write,
	}
	view := e.view(session)
	e.replace(ctx, session)
	return view, nil
}

// Starting image and its video position, if extracted from a frame.
type thumbnailEditImage struct {
	frame      domain.ThumbnailFrame
	positionMs *int64
}

func (e *ThumbnailEdits) startImage(
	ctx context.Context, video domain.Video, path string, start domain.ThumbnailEditStart,
) (thumbnailEditImage, error) {
	switch start.Source {
	case domain.ThumbnailEditFromFrame:
		if start.PositionMs == nil {
			return thumbnailEditImage{}, domain.ErrThumbnailPositionOutOfRange
		}
		if err := domain.CheckThumbnailPosition(video, *start.PositionMs); err != nil {
			return thumbnailEditImage{}, err
		}
		return e.frameAt(ctx, path, *start.PositionMs)
	case domain.ThumbnailEditFromThumbnail:
		return e.currentThumbnail(ctx, video, path)
	case domain.ThumbnailEditFromImage:
		frame, err := thumbnailEditFrameFromImage(start.Image)
		if err != nil {
			return thumbnailEditImage{}, errors.Join(domain.ErrThumbnailUploadInvalid, err)
		}
		return thumbnailEditImage{frame: frame}, nil
	default:
		return thumbnailEditImage{}, fmt.Errorf("unknown thumbnail edit source %q", start.Source)
	}
}

// Prefer the applied original (legacy thumbnail fallback), then a selected frame, then automatic/first-frame fallback. Read image and position atomically from storage to avoid mixing changes since the video was read.
func (e *ThumbnailEdits) currentThumbnail(
	ctx context.Context, video domain.Video, path string,
) (thumbnailEditImage, error) {
	selection, err := e.images.CurrentThumbnailSelection(ctx, video.ContentKey)
	if err != nil {
		return thumbnailEditImage{}, err
	}
	if selection.Image != nil {
		frame, err := thumbnailEditFrameFromImage(selection.Image)
		if err != nil {
			return thumbnailEditImage{}, errors.Join(domain.ErrThumbnailFrameUnavailable, err)
		}
		return thumbnailEditImage{frame: frame}, nil
	}
	if path == "" {
		return thumbnailEditImage{}, domain.ErrMediaFileUnavailable
	}
	if selection.PositionMs != nil {
		return e.frameAt(ctx, path, *selection.PositionMs)
	}
	var durationMs int64
	if video.DurationMs != nil {
		durationMs = *video.DurationMs
	}
	var first error
	for _, positionMs := range e.frames.AutoThumbnailPositions(durationMs) {
		image, err := e.frameAt(ctx, path, positionMs)
		if err == nil {
			return image, nil
		}
		first = cmp.Or(first, err)
	}
	return thumbnailEditImage{}, cmp.Or(first, error(domain.ErrThumbnailFrameUnavailable))
}

func (e *ThumbnailEdits) frameAt(ctx context.Context, path string, positionMs int64) (thumbnailEditImage, error) {
	if e.foreground != nil {
		defer e.foreground.ReadInForeground(path)()
	}
	frame, err := e.frames.FrameAt(ctx, path, positionMs)
	if err != nil {
		return thumbnailEditImage{}, errors.Join(domain.ErrThumbnailFrameUnavailable, err)
	}
	return thumbnailEditImage{frame: frame, positionMs: &positionMs}, nil
}

// End the prior session under its lock before publishing the replacement, waiting for active apply/run requests. Repeat if another Start installs a session while waiting; only end removes sessions, so acquire cannot accept a removed one.
func (e *ThumbnailEdits) replace(ctx context.Context, session *thumbnailEditSession) {
	for {
		e.mu.Lock()
		previous := e.byID[e.byVideo[session.videoID]]
		if previous == nil {
			e.byID[session.id] = session
			e.byVideo[session.videoID] = session.id
			e.mu.Unlock()
			return
		}
		e.mu.Unlock()
		previous.mu.Lock()
		if !previous.ended {
			e.end(ctx, previous)
		}
		previous.mu.Unlock()
	}
}

// Get polls active runs and fetches successful output while renewing the session heartbeat. Ended sessions return ErrThumbnailEditNotFound.
func (e *ThumbnailEdits) Get(ctx context.Context, id string) (domain.ThumbnailEdit, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	defer session.mu.Unlock()
	e.refresh(ctx, session)
	return e.view(session), nil
}

// VideoFrame returns a bounded PNG of the video file at positionMs without starting a session, as the person page
// reads a frame to frame a face from; an unreadable frame wraps domain.ErrThumbnailFrameUnavailable.
func (e *ThumbnailEdits) VideoFrame(ctx context.Context, path string, positionMs int64) (domain.ThumbnailFrame, error) {
	image, err := e.frameAt(ctx, path, positionMs)
	return image.frame, err
}

// Frame returns the session source PNG.
func (e *ThumbnailEdits) Frame(ctx context.Context, id string) ([]byte, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return nil, err
	}
	defer session.mu.Unlock()
	return session.frame.PNG, nil
}

// Result returns successful output or ErrThumbnailEditNoResult.
func (e *ThumbnailEdits) Result(ctx context.Context, id string) ([]byte, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return nil, err
	}
	defer session.mu.Unlock()
	if !hasResult(session) {
		return nil, domain.ErrThumbnailEditNoResult
	}
	return session.result, nil
}

// StartRun validates mask/reference before uploads, snapshots current settings, and cancels any prior run; wait for in-flight cancellation retries and fail unreachable if cancellation still cannot complete. Invalid inputs return ErrThumbnailEditMaskInvalid or ErrThumbnailEditReferenceInvalid, missing settings ErrThumbnailEditNotReady, and provider rejection marks the session failed with the returned ImageEditError.
func (e *ThumbnailEdits) StartRun(ctx context.Context, id string, input domain.ThumbnailEditRunInput) (domain.ThumbnailEdit, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	defer session.mu.Unlock()
	if err := checkThumbnailEditMask(input.Mask, session.frame); err != nil {
		return domain.ThumbnailEdit{}, err
	}
	if err := checkThumbnailEditReference(input.Reference); err != nil {
		return domain.ThumbnailEdit{}, err
	}
	conn, err := e.settings.ComfyUISettings(ctx)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	if !conn.Saved() {
		return domain.ThumbnailEdit{}, domain.ErrThumbnailEditNotReady
	}
	if (session.run.inProgress() && !e.cancelRun(ctx, session)) || e.retryCancels(ctx, &session.videoID, true) > 0 {
		failure := &domain.ImageEditError{
			Reason: domain.ImageEditUnreachable, Message: "the previous run could not be cancelled at the provider",
		}
		now := e.now()
		session.result = nil
		session.run = &thumbnailEditRun{state: domain.ThumbnailEditFailed, submittedAt: now, endedAt: now, failure: failure}
		return domain.ThumbnailEdit{}, failure
	}
	session.result = nil

	job, err := e.editor.Start(ctx, conn, domain.ImageEditInput{
		Frame: session.frame.PNG, Mask: input.Mask, Reference: input.Reference, Instruction: input.Instruction,
	})
	now := e.now()
	if err != nil {
		failure := &domain.ImageEditError{Reason: domain.ImageEditRunFailed}
		if !errors.As(err, &failure) {
			e.logger.Warn("could not start a thumbnail edit run", slog.Any("error", err))
		}
		session.run = &thumbnailEditRun{state: domain.ThumbnailEditFailed, submittedAt: now, endedAt: now, failure: failure}
		return domain.ThumbnailEdit{}, err
	}
	session.run = &thumbnailEditRun{job: job, submitted: true, state: domain.ThumbnailEditQueued, submittedAt: now}
	return e.view(session), nil
}

// Cancel marks an active run cancelled without changing the thumbnail; inactive runs return their current view.
func (e *ThumbnailEdits) Cancel(ctx context.Context, id string) (domain.ThumbnailEdit, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return domain.ThumbnailEdit{}, err
	}
	defer session.mu.Unlock()
	if session.run.inProgress() {
		e.cancelRun(ctx, session)
	}
	return e.view(session), nil
}

// Apply installs successful output, through the session's write when StartFrom gave one, and ends the session; ErrThumbnailEditNoResult and ErrThumbnailImageInvalid leave it open. Missing or changed video content returns ErrContentChanged and ends the session.
func (e *ThumbnailEdits) Apply(ctx context.Context, id string) (domain.Video, error) {
	session, err := e.acquire(ctx, id)
	if err != nil {
		return domain.Video{}, err
	}
	defer session.mu.Unlock()
	if !hasResult(session) {
		return domain.Video{}, domain.ErrThumbnailEditNoResult
	}
	var video domain.Video
	if session.write != nil {
		video, err = session.write(ctx, session.result)
	} else {
		video, err = e.applier.ApplyThumbnailImage(
			ctx, session.videoID, session.contentKey, session.result, domain.ThumbnailImageGenerated)
	}
	if errors.Is(err, domain.ErrContentChanged) {
		e.end(ctx, session)
		return domain.Video{}, err
	}
	if err != nil {
		return domain.Video{}, err
	}
	e.end(ctx, session)
	return video, nil
}

// Discard cancels the active run and removes the session; already-ended sessions succeed.
func (e *ThumbnailEdits) Discard(ctx context.Context, id string) {
	e.mu.Lock()
	session := e.byID[id]
	e.mu.Unlock()
	if session == nil {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.ended {
		e.end(ctx, session)
	}
}

// Run sweeps until ctx ends, then cancels provider jobs and closes all sessions.
func (e *ThumbnailEdits) Run(ctx context.Context) {
	ticker := time.NewTicker(thumbnailEditSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), thumbnailEditCloseTimeout)
			e.Close(closeCtx)
			cancel()
			return
		case <-ticker.C:
			e.Sweep(ctx)
		}
	}
}

// Sweep ends sessions idle for five minutes, fails ten-minute runs as timed_out, and retries pending provider cancellations.
func (e *ThumbnailEdits) Sweep(ctx context.Context) {
	defer e.retryCancels(ctx, nil, false)
	for _, session := range e.sessions() {
		session.mu.Lock()
		switch {
		case session.ended:
		case e.idle(session):
			e.end(ctx, session)
		default:
			e.checkTimeLimit(ctx, session)
		}
		session.mu.Unlock()
	}
}

// Close cancels active runs, ends all sessions and retries pending cancellations once more.
func (e *ThumbnailEdits) Close(ctx context.Context) {
	defer e.retryCancels(ctx, nil, false)
	for _, session := range e.sessions() {
		session.mu.Lock()
		if !session.ended {
			e.end(ctx, session)
		}
		session.mu.Unlock()
	}
}

func (e *ThumbnailEdits) sessions() []*thumbnailEditSession {
	e.mu.Lock()
	defer e.mu.Unlock()
	sessions := make([]*thumbnailEditSession, 0, len(e.byID))
	for _, session := range e.byID {
		sessions = append(sessions, session)
	}
	return sessions
}

// acquire locks and refreshes the session heartbeat; missing, ended or five-minute-idle sessions return ErrThumbnailEditNotFound. Idle sessions are ended here.
func (e *ThumbnailEdits) acquire(ctx context.Context, id string) (*thumbnailEditSession, error) {
	e.mu.Lock()
	session := e.byID[id]
	e.mu.Unlock()
	if session == nil {
		return nil, domain.ErrThumbnailEditNotFound
	}
	session.mu.Lock()
	if session.ended {
		session.mu.Unlock()
		return nil, domain.ErrThumbnailEditNotFound
	}
	if e.idle(session) {
		e.end(ctx, session)
		session.mu.Unlock()
		return nil, domain.ErrThumbnailEditNotFound
	}
	session.lastReadAt = e.now()
	return session, nil
}

func (e *ThumbnailEdits) idle(session *thumbnailEditSession) bool {
	return e.now().Sub(session.lastReadAt) >= domain.ThumbnailEditIdleLimit
}

// end cancels the active run and removes the session; caller holds the session lock.
func (e *ThumbnailEdits) end(ctx context.Context, session *thumbnailEditSession) {
	if session.run.inProgress() {
		e.cancelRun(ctx, session)
	}
	session.ended = true
	session.frame = domain.ThumbnailFrame{}
	session.result = nil
	e.mu.Lock()
	if e.byID[session.id] == session {
		delete(e.byID, session.id)
	}
	if e.byVideo[session.videoID] == session.id {
		delete(e.byVideo, session.videoID)
	}
	e.mu.Unlock()
}

// Treat the run as cancelled locally even if the provider refuses; retain it in pending for sweep retries and return false.
func (e *ThumbnailEdits) cancelRun(ctx context.Context, session *thumbnailEditSession) bool {
	run := session.run
	err := e.cancelJob(ctx, run.job, run.state)
	if err != nil {
		e.logger.Warn("could not cancel a thumbnail edit run; retrying on the next sweep",
			slog.Int64("video", session.videoID), slog.Any("error", err))
		e.pendingMu.Lock()
		e.pending[run.job.ID] = &thumbnailEditPendingCancel{
			videoID: session.videoID, job: run.job, state: run.state, failedAt: e.now(),
		}
		e.pendingMu.Unlock()
	}
	e.finish(run, domain.ThumbnailEditCancelled, nil)
	return err == nil
}

// Detach request cancellation so leaving the page cannot prevent cleanup, but preserve the shutdown deadline.
func (e *ThumbnailEdits) cancelJob(ctx context.Context, job domain.ImageEditJob, state domain.ThumbnailEditState) error {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), thumbnailEditProviderTimeout)
	defer cancel()
	if deadline, ok := ctx.Deadline(); ok {
		var stop context.CancelFunc
		cancelCtx, stop = context.WithDeadline(cancelCtx, deadline)
		defer stop()
	}
	return e.editor.Cancel(cancelCtx, job, state)
}

// Retry distinct jobs concurrently but serialize retries per job: a late Comfy Cloud account-wide interrupt could otherwise cancel a newer run. Filter by videoID when provided, log and abandon jobs past thumbnailEditCancelRetryLimit, and with wait=true await in-flight retries before retrying survivors; cancellation while waiting counts as unfinished.
func (e *ThumbnailEdits) retryCancels(ctx context.Context, videoID *int64, wait bool) int {
	tried := map[string]bool{}
	remaining := 0
	for {
		claimed, busy := e.claimCancels(videoID, tried)
		remaining += e.attemptCancels(ctx, claimed)
		if !wait || len(busy) == 0 {
			if len(claimed) == 0 {
				return remaining
			}
			continue
		}
		for _, done := range busy {
			select {
			case <-done:
			case <-ctx.Done():
				return remaining + len(busy)
			}
		}
	}
}

// Claim untried jobs without an active retry; return completion channels for jobs already claimed elsewhere.
func (e *ThumbnailEdits) claimCancels(videoID *int64, tried map[string]bool) ([]*thumbnailEditPendingCancel, []chan struct{}) {
	e.pendingMu.Lock()
	defer e.pendingMu.Unlock()
	var claimed []*thumbnailEditPendingCancel
	var busy []chan struct{}
	for id, pending := range e.pending {
		switch {
		case videoID != nil && pending.videoID != *videoID:
		case pending.attempt != nil:
			busy = append(busy, pending.attempt)
		case !tried[id]:
			tried[id] = true
			pending.attempt = make(chan struct{})
			claimed = append(claimed, pending)
		}
	}
	return claimed, busy
}

// Retry claimed jobs concurrently and return the unfinished count.
func (e *ThumbnailEdits) attemptCancels(ctx context.Context, claimed []*thumbnailEditPendingCancel) int {
	var wg sync.WaitGroup
	var remaining atomic.Int64
	for _, pending := range claimed {
		wg.Go(func() {
			if !e.attemptCancel(ctx, pending) {
				remaining.Add(1)
			}
		})
	}
	wg.Wait()
	return int(remaining.Load())
}

// Remove completed or abandoned jobs from pending, return true, and notify waiters when the claimed attempt ends.
func (e *ThumbnailEdits) attemptCancel(ctx context.Context, pending *thumbnailEditPendingCancel) bool {
	err := e.cancelJob(ctx, pending.job, pending.state)
	resolved := true
	switch {
	case err == nil:
	case e.now().Sub(pending.failedAt) >= thumbnailEditCancelRetryLimit:
		e.logger.Warn("gave up cancelling a thumbnail edit run", slog.Int64("video", pending.videoID), slog.Any("error", err))
	default:
		e.logger.Debug("could not cancel a thumbnail edit run", slog.Int64("video", pending.videoID), slog.Any("error", err))
		resolved = false
	}
	e.pendingMu.Lock()
	if resolved {
		delete(e.pending, pending.job.ID)
	}
	close(pending.attempt)
	pending.attempt = nil
	e.pendingMu.Unlock()
	return resolved
}

// Cancel runs exceeding ten minutes, mark timed_out and return true.
func (e *ThumbnailEdits) checkTimeLimit(ctx context.Context, session *thumbnailEditSession) bool {
	run := session.run
	if !run.inProgress() || e.now().Sub(run.submittedAt) < domain.ThumbnailEditRunLimit {
		return false
	}
	e.cancelRun(ctx, session)
	e.finish(run, domain.ThumbnailEditFailed, &domain.ImageEditError{
		Reason: domain.ImageEditTimedOut, Message: "no result 10 minutes after the run was submitted",
	})
	return true
}

// Poll active runs and fetch successful output; unreadable status preserves the prior state for the next poll.
func (e *ThumbnailEdits) refresh(ctx context.Context, session *thumbnailEditSession) {
	run := session.run
	if !run.inProgress() || e.checkTimeLimit(ctx, session) {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, thumbnailEditProviderTimeout)
	defer cancel()
	state, failure, err := e.editor.State(readCtx, run.job, run.state)
	if err != nil {
		e.logger.Debug("could not read a thumbnail edit run", slog.Int64("video", session.videoID), slog.Any("error", err))
		return
	}
	if progress := e.editor.Progress(run.job); progress != nil {
		run.progress = progress
	}
	switch state {
	case domain.ThumbnailEditSucceeded:
		e.fetchOutput(readCtx, session)
	case domain.ThumbnailEditFailed:
		if failure == nil {
			failure = &domain.ImageEditError{Reason: domain.ImageEditRunFailed}
		}
		e.finish(run, domain.ThumbnailEditFailed, failure)
	case domain.ThumbnailEditCancelled:
		e.finish(run, domain.ThumbnailEditCancelled, nil)
	default:
		run.state = state
	}
}

// Mark succeeded only after fully decoding output; retry unreachable or unclassified failures until the ten-minute run limit. Missing/invalid images fail as no_output; other errors retain their reason.
func (e *ThumbnailEdits) fetchOutput(ctx context.Context, session *thumbnailEditSession) {
	run := session.run
	output, err := e.editor.Output(ctx, run.job)
	if err != nil {
		var failure *domain.ImageEditError
		if !errors.As(err, &failure) || failure.Reason == domain.ImageEditUnreachable {
			e.logger.Debug("could not fetch a thumbnail edit output", slog.Int64("video", session.videoID), slog.Any("error", err))
			return
		}
		e.finish(run, domain.ThumbnailEditFailed, failure)
		return
	}
	if err := checkThumbnailEditImage(output); err != nil {
		e.finish(run, domain.ThumbnailEditFailed, &domain.ImageEditError{
			Reason: domain.ImageEditNoOutput, Message: "the output image cannot be decoded",
		})
		return
	}
	session.result = output
	e.finish(run, domain.ThumbnailEditSucceeded, nil)
}

func (e *ThumbnailEdits) finish(run *thumbnailEditRun, state domain.ThumbnailEditState, failure *domain.ImageEditError) {
	run.state = state
	run.failure = failure
	run.endedAt = e.now()
}

// Caller holds the session lock.
func (e *ThumbnailEdits) view(session *thumbnailEditSession) domain.ThumbnailEdit {
	view := domain.ThumbnailEdit{
		ID: session.id, VideoID: session.videoID, State: domain.ThumbnailEditReady,
		Source: session.source, PositionMs: session.positionMs,
		FrameWidth: session.frame.Width, FrameHeight: session.frame.Height,
		HasResult: hasResult(session),
	}
	run := session.run
	if run == nil {
		return view
	}
	view.State = run.state
	end := e.now()
	if run.state.Terminal() {
		end = run.endedAt
	}
	elapsed := max(end.Sub(run.submittedAt), 0)
	view.Elapsed = &elapsed
	if run.progress != nil {
		progress := *run.progress
		view.Progress = &progress
	}
	if run.failure != nil {
		failure := *run.failure
		view.Error = &failure
	}
	return view
}

func hasResult(session *thumbnailEditSession) bool {
	return session.run != nil && session.run.state == domain.ThumbnailEditSucceeded && session.result != nil
}

// Generate a random 128-bit session ID.
func newThumbnailEditID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("cannot make a thumbnail edit id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}
