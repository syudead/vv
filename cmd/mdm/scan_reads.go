package main

import (
	"context"

	"github.com/syudead/vv/internal/jobs"
)

// scanReadSlots hands the scanner's content-key reads to the scheduler, which is created after the scans service
// builds its scanner and before any scan starts.
type scanReadSlots struct {
	scheduler **jobs.Scheduler
}

func (s scanReadSlots) AcquireRead(ctx context.Context, root string) (func(), error) {
	return (*s.scheduler).AcquireRead(ctx, root)
}

func (s scanReadSlots) KeepAhead(root string) func() {
	return (*s.scheduler).KeepAhead(root)
}
