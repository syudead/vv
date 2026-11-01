package app

import (
	"context"
	"errors"

	"github.com/syudead/vv/internal/domain"
)

// Backfill re-reads files so stored results from an older build gain what this build reads. Both handlers run as
// backfill-class scheduler jobs; neither changes import progress or queues import work. See
// docs/design-docs/job-scheduling.md.
type Backfill struct {
	// QueueAll handles the queue-all job: it queues one Read job per video whose stored result is out of date, in the
	// queue-all job's class.
	QueueAll func(ctx context.Context, job domain.Job) error
	// Read reads the job's file and saves the result if the file is still the one indexed.
	Read func(ctx context.Context, job domain.Job) error
}

// unavailable is the result read gives for a missing file, which the save records so the file is not read again.
func newBackfill[T any](
	queue func(context.Context, domain.JobPriority) (int, error), sources MediaFiles,
	read func(context.Context, string) T, unavailable T, save func(context.Context, domain.Job, T) error,
) Backfill {
	return Backfill{
		QueueAll: func(ctx context.Context, job domain.Job) error {
			_, err := queue(ctx, job.Priority)
			return err
		},
		Read: func(ctx context.Context, job domain.Job) error {
			result := unavailable
			path, err := sources.Resolve(ctx, job.LocationPath)
			switch {
			case errors.Is(err, domain.ErrMediaFileUnavailable):
			case err != nil:
				return err
			default:
				result = read(ctx, path)
			}
			// A read cut short by shutdown is not a result.
			if err := ctx.Err(); err != nil {
				return err
			}
			return save(ctx, job, result)
		},
	}
}
