package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// MediaInfoStore finds imported videos whose stored media information is older than this build reads.
type MediaInfoStore interface {
	QueueMediaInfoJobs(context.Context, domain.JobPriority) (int, error)
	SaveMediaInfoProbe(context.Context, domain.Job, domain.MediaInfoProbe) error
}

// MediaInfoBackfill re-reads files so videos imported before a MediaInfoVersion bump gain the new items without the
// owner doing anything. It never changes the video's other probe results.
func MediaInfoBackfill(store MediaInfoStore, probe func(context.Context, string) domain.MediaInfoProbe) Backfill {
	return newBackfill(store.QueueMediaInfoJobs, probe, store.SaveMediaInfoProbe)
}
