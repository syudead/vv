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
	var afterID int64
	for ctx.Err() == nil {
		job, err := store.NextKeyframeProbe(ctx, afterID)
		delay := 250 * time.Millisecond
		if err == nil {
			afterID = job.VideoID
			result := probe(ctx, job.LocationPath)
			if ctx.Err() != nil {
				return
			}
			err = store.SaveKeyframeProbe(ctx, job, result)
		} else {
			afterID, delay = 0, time.Minute
		}
		if err != nil && !errors.Is(err, domain.ErrNoJob) && ctx.Err() == nil {
			logger.Warn("could not backfill keyframe spacing", slog.Any("error", err))
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
