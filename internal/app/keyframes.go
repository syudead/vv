package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// KeyframeStore keeps optional seek hints separate from import progress.
type KeyframeStore interface {
	NextKeyframeProbe(context.Context, int64) (domain.Job, error)
	SaveKeyframeProbe(context.Context, domain.Job, domain.KeyframeProbe) error
}

// BackfillKeyframes samples one file at a time, with a pause between files and
// between passes. It never changes playback eligibility or queues import work.
// The caller cancels and joins this loop before closing the store.
func BackfillKeyframes(ctx context.Context, store KeyframeStore,
	probe func(context.Context, string) domain.KeyframeProbe, logger *slog.Logger,
) {
	backfill(ctx, store.NextKeyframeProbe, func(ctx context.Context, job domain.Job) error {
		result := probe(ctx, job.LocationPath)
		if ctx.Err() != nil {
			return nil
		}
		return store.SaveKeyframeProbe(ctx, job, result)
	}, "could not backfill keyframe spacing", logger)
}

// FingerprintBackfillStore finds videos whose fingerprint is missing or from an older version.
type FingerprintBackfillStore interface {
	NextStaleFingerprint(context.Context, int64) (domain.Job, error)
	ApplyFingerprintForJob(context.Context, domain.Job, domain.Fingerprint) (bool, error)
}

// BackfillFingerprints rebuilds fingerprints of the current version one file at a time, rebuilding their duplicate
// candidates as an import does, without queueing import work or opening import activity.
func BackfillFingerprints(ctx context.Context, store FingerprintBackfillStore,
	sample func(context.Context, string) (domain.Fingerprint, error), logger *slog.Logger,
) {
	backfill(ctx, store.NextStaleFingerprint, func(ctx context.Context, job domain.Job) error {
		fingerprint, err := sample(ctx, job.LocationPath)
		if err != nil || ctx.Err() != nil {
			return err
		}
		_, err = store.ApplyFingerprintForJob(ctx, job, fingerprint)
		return err
	}, "could not backfill a fingerprint", logger)
}

// backfill pages through next by video ID, pausing between files and for a minute between passes; a failed file is
// logged and retried on the next pass.
func backfill(ctx context.Context, next func(context.Context, int64) (domain.Job, error),
	work func(context.Context, domain.Job) error, failure string, logger *slog.Logger,
) {
	var afterID int64
	for ctx.Err() == nil {
		job, err := next(ctx, afterID)
		delay := 250 * time.Millisecond
		if err == nil {
			afterID = job.VideoID
			err = work(ctx, job)
		} else {
			afterID, delay = 0, time.Minute
		}
		if err != nil && !errors.Is(err, domain.ErrNoJob) && ctx.Err() == nil {
			logger.Warn(failure, slog.Any("error", err))
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
