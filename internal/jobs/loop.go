package jobs

import (
	"context"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Step does one unit of a worker's work and says how long to wait before the next.
type Step func(ctx context.Context) domain.WorkerWait

// Loop runs one worker's steps one at a time and sleeps between them as each step asks, so no worker polls an
// empty queue; every background queue worker runs on one.
type Loop struct {
	// Buffer one notification so a wakeup sent during a step is not lost.
	wake chan struct{}
}

func NewLoop() *Loop {
	return &Loop{wake: make(chan struct{}, 1)}
}

// Wake is nonblocking; wakeups sent during one step coalesce into one.
func (l *Loop) Wake() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// Run steps at once, so work left from before a restart resumes, and returns when ctx ends.
func (l *Loop) Run(ctx context.Context, step Step) {
	for ctx.Err() == nil {
		l.wait(ctx, step(ctx))
	}
}

func (l *Loop) wait(ctx context.Context, wait domain.WorkerWait) {
	if wait == domain.WorkerNext {
		return
	}
	var timeout <-chan time.Time
	if wait > 0 {
		timer := time.NewTimer(time.Duration(wait))
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-ctx.Done():
	case <-l.wake:
	case <-timeout:
	}
}
