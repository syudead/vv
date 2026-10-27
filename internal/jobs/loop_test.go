package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// A step's own wait, such as the next scheduled retry, ends at its time or earlier on a wakeup.
func TestLoopWaitsTheStepsTimeUnlessWoken(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const wait = time.Hour
		var steps atomic.Int32
		loop := NewLoop()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			loop.Run(ctx, func(context.Context) domain.WorkerWait {
				steps.Add(1)
				return domain.WorkerWait(wait)
			})
		}()
		defer func() { cancel(); <-done }()

		synctest.Wait()
		loop.Wake()
		synctest.Wait()
		if got := steps.Load(); got != 2 {
			t.Fatalf("steps after Wake = %d, want 2", got)
		}
		time.Sleep(wait - time.Nanosecond)
		synctest.Wait()
		if got := steps.Load(); got != 2 {
			t.Fatalf("steps before the wait ends = %d, want 2", got)
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if got := steps.Load(); got != 3 {
			t.Fatalf("steps after the wait = %d, want 3", got)
		}
	})
}
