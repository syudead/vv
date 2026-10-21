package app

import (
	"context"
	"log/slog"

	"github.com/syudead/vv/internal/domain"
)

// MediaInfoStore pages through imported videos whose stored media information is older than this build reads.
type MediaInfoStore interface {
	NextMediaInfoProbe(context.Context, int64) (domain.Job, error)
	SaveMediaInfoProbe(context.Context, domain.Job, domain.MediaInfoProbe) error
}

// BackfillMediaInfo re-reads one file at a time so videos imported before a MediaInfoVersion bump gain the new
// items without the owner doing anything. It never changes the video's other probe results or queues import work.
func BackfillMediaInfo(ctx context.Context, store MediaInfoStore,
	probe func(context.Context, string) domain.MediaInfoProbe, logger *slog.Logger,
) {
	backfill(ctx, store.NextMediaInfoProbe, probe, store.SaveMediaInfoProbe, logger, "could not backfill media information")
}
