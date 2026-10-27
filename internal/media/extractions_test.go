package media

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/syudead/vv/internal/domain"
)

// fakeSlots lends up to lendable pairs at once, as a disk with that many free slots would.
type fakeSlots struct {
	mu       sync.Mutex
	lendable int
	held     int
}

func (s *fakeSlots) TryAcquire(domain.JobResources) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held >= s.lendable {
		return false
	}
	s.held++
	return true
}

func (s *fakeSlots) Release(domain.JobResources) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.held--
}

func (s *fakeSlots) lent() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held
}

func TestExtractionsRunOnOwnAndLentSlots(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lendable int
		parallel int
	}{
		{name: "1-slot disk", lendable: 0, parallel: 1},
		{name: "4-slot disk", lendable: 3, parallel: 4},
		{name: "more free slots than the cap", lendable: 8, parallel: domain.MaxDiskReadSlots},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				slots := &fakeSlots{lendable: tc.lendable}
				ctx := domain.WithJobSlots(context.Background(), slots)
				parts := newExtractions(ctx, domain.JobResources{Read: 1, CPU: 1})
				release := make(chan struct{})
				var mu sync.Mutex
				running, ran := 0, 0
				dispatched := make(chan struct{})
				go func() {
					defer close(dispatched)
					for range 6 {
						parts.Go(ctx, func() {
							mu.Lock()
							running++
							mu.Unlock()
							<-release
							mu.Lock()
							running--
							ran++
							mu.Unlock()
						})
					}
					parts.Wait()
				}()
				synctest.Wait()
				mu.Lock()
				got := running
				mu.Unlock()
				if got != tc.parallel || slots.lent() != tc.parallel-1 {
					t.Fatalf("running = %d on %d lent pairs, want %d on %d", got, slots.lent(), tc.parallel, tc.parallel-1)
				}
				close(release)
				<-dispatched
				if ran != 6 || slots.lent() != 0 {
					t.Fatalf("ran %d parts with %d pairs still lent, want 6 with none", ran, slots.lent())
				}
			})
		})
	}
}

func TestExtractionsStopStartingWhenCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(domain.WithJobSlots(context.Background(), &fakeSlots{}))
		parts := newExtractions(ctx, domain.JobResources{Read: 1})
		release := make(chan struct{})
		if !parts.Go(ctx, func() { <-release }) {
			t.Fatal("the first part did not start on the job's own slots")
		}
		result := make(chan bool)
		go func() { result <- parts.Go(ctx, func() { t.Error("a part started after cancellation") }) }()
		synctest.Wait()
		cancel()
		if <-result {
			t.Fatal("Go reported a start after cancellation")
		}
		close(release)
		parts.Wait()
	})
}
