package media

import (
	"context"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// extractions runs a job's parts in parallel within the scheduler's limits, as the make jobserver does: the first
// part runs on the job's own slots, and each further part on slots the scheduler lends without waiting and gets back
// when the part ends. At most domain.MaxDiskReadSlots parts run at once. See docs/design-docs/job-scheduling.md.
type extractions struct {
	slots  domain.JobSlots
	demand domain.JobResources
	// done receives whether a finished part ran on lent slots; Go drains it on the caller's goroutine.
	done    chan bool
	running int
	ownFree bool
	workers sync.WaitGroup
}

// newExtractions splits work whose every part needs demand: a read slot for a read, and a read and a CPU slot for
// an ffmpeg extraction.
func newExtractions(ctx context.Context, demand domain.JobResources) *extractions {
	return &extractions{
		slots:   domain.JobSlotsFrom(ctx),
		demand:  demand,
		done:    make(chan bool, domain.MaxDiskReadSlots),
		ownFree: true,
	}
}

// Go starts part once slots allow, waiting for a running part to end otherwise; false means ctx ended first and part
// did not start. Only one goroutine calls Go.
func (e *extractions) Go(ctx context.Context, part func()) bool {
	for {
		if lent, ok := e.take(); ok {
			e.running++
			e.workers.Go(func() {
				part()
				if lent {
					e.slots.Release(e.demand)
				}
				e.done <- lent
			})
			return true
		}
		select {
		case lent := <-e.done:
			e.finished(lent)
		case <-ctx.Done():
			return false
		}
	}
}

func (e *extractions) take() (lent, ok bool) {
	if e.ownFree {
		e.ownFree = false
		return false, true
	}
	if e.running < domain.MaxDiskReadSlots && e.slots.TryAcquire(e.demand) {
		return true, true
	}
	return false, false
}

func (e *extractions) finished(lent bool) {
	e.running--
	if !lent {
		e.ownFree = true
	}
}

// Wait returns when every started part has ended and returned its slots.
func (e *extractions) Wait() {
	e.workers.Wait()
}
