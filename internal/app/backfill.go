package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// backfill re-reads one file at a time, pausing between files and for a minute between passes, so a background
// re-read never competes with import work. failure names the warning a store error logs.
func backfill[T any](ctx context.Context, next func(context.Context, int64) (domain.Job, error),
	read func(context.Context, string) T, save func(context.Context, domain.Job, T) error,
	logger *slog.Logger, failure string,
) {
	var afterID int64
	for ctx.Err() == nil {
		job, err := next(ctx, afterID)
		delay := 250 * time.Millisecond
		if err == nil {
			afterID = job.VideoID
			result := read(ctx, job.LocationPath)
			if ctx.Err() != nil {
				return
			}
			err = save(ctx, job, result)
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
