package app

import (
	"context"
	"log/slog"

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
	backfill(ctx, store.NextKeyframeProbe, probe, store.SaveKeyframeProbe, logger, "could not backfill keyframe spacing")
}
