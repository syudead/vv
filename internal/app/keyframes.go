package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// KeyframeStore keeps optional seek hints separate from import progress.
type KeyframeStore interface {
	QueueKeyframeJobs(context.Context, domain.JobPriority) (int, error)
	SaveKeyframeProbe(context.Context, domain.Job, domain.KeyframeProbe) error
}

// KeyframeBackfill samples the keyframe spacing of videos whose stored sample is missing or out of date. It never
// changes playback eligibility.
func KeyframeBackfill(store KeyframeStore, probe func(context.Context, string) domain.KeyframeProbe) Backfill {
	return newBackfill(store.QueueKeyframeJobs, probe, store.SaveKeyframeProbe)
}
