package wasmplugin

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Exercise admission control without starting a WASM runtime. The production
// wait elapses on synctest's clock, not the wall clock.
func TestAcquireWaitsForTheSlot(t *testing.T) {
	for _, outcome := range []string{"release", "cancel", "timeout"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				rt := &Runtime{slot: make(chan struct{}, 1)}
				release, err := rt.acquire(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					nextRelease, err := rt.acquire(ctx)
					if nextRelease != nil {
						nextRelease()
					}
					done <- err
				}()
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("acquire returned while the slot was held: %v", err)
				default:
				}
				switch outcome {
				case "release":
					release()
					if err := <-done; err != nil {
						t.Fatal(err)
					}
				case "cancel":
					cancel()
					if err := <-done; !errors.Is(err, context.Canceled) {
						t.Fatalf("acquire = %v, want canceled", err)
					}
					release()
				case "timeout":
					start := time.Now()
					wantFailure(t, <-done, domain.WorkInfoRunBusy)
					if elapsed := time.Since(start); elapsed != domain.WorkInfoPluginBusyWait {
						t.Fatalf("waited %v, want %v", elapsed, domain.WorkInfoPluginBusyWait)
					}
					release()
				}
				if len(rt.slot) != 0 {
					t.Fatal("the execution slot was not released")
				}
			})
		})
	}
}

func TestStoppedDistinguishesRunTimeoutFromCallerCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(t.Context())
	defer cancelParent()
	runCtx, cancelRun := context.WithCancelCause(parent)
	defer cancelRun(nil)
	if err := stopped(parent, runCtx); err != nil {
		t.Fatalf("active run = %v", err)
	}
	cancelRun(errTimedOut)
	wantFailure(t, stopped(parent, runCtx), domain.WorkInfoRunTimedOut)
	cancelParent()
	if err := stopped(parent, runCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation = %v, want canceled even after the run timeout", err)
	}
}

func TestSleepStopsWhenTheRunEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() {
			sleep(ctx, time.Hour)
			close(done)
		}()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("sleep returned before cancellation")
		default:
		}
		start := time.Now()
		cancel()
		<-done
		if elapsed := time.Since(start); elapsed != 0 {
			t.Fatalf("sleep waited %v after cancellation", elapsed)
		}
	})
}

func TestHTTPFailureClassifiesAnExpiredRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 0)
	defer cancel()
	req, ok := buildRequest(ctx, []byte(get("https://site.test/page")))
	if !ok {
		t.Fatal("invalid request")
	}
	session := &httpSession{}
	if code := session.failure(req, context.DeadlineExceeded); code != hostTimedOut {
		t.Fatalf("failure = %d, want %d", code, hostTimedOut)
	}
	if session.unreachable != "" {
		t.Fatalf("a timeout marked %q as unreachable", session.unreachable)
	}
}
