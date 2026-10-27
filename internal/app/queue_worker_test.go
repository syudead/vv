package app

import (
	"context"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// testLoop stands in for jobs.Loop, on which cmd/mdm runs every queue worker's Step; internal/app may not import
// that adapter, even in tests.
type testLoop struct {
	wake chan struct{}
}

func newTestLoop() *testLoop {
	return &testLoop{wake: make(chan struct{}, 1)}
}

func (l *testLoop) Wake() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// run steps until the test ends, waiting between steps as jobs.Loop does.
func (l *testLoop) run(t *testing.T, step func(context.Context) domain.WorkerWait) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			l.wait(ctx, step(ctx))
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func (l *testLoop) wait(ctx context.Context, wait domain.WorkerWait) {
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
