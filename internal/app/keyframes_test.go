package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type keyframeTestStore struct {
	job    domain.Job
	saved  []domain.KeyframeProbe
	cancel context.CancelFunc
	idle   chan struct{}
}

func (s *keyframeTestStore) NextKeyframeProbe(_ context.Context, afterID int64) (domain.Job, error) {
	if afterID != 0 || s.job.VideoID == 0 {
		if s.idle != nil {
			close(s.idle)
			s.idle = nil
		}
		return domain.Job{}, domain.ErrNoJob
	}
	return s.job, nil
}
func (s *keyframeTestStore) SaveKeyframeProbe(_ context.Context, _ domain.Job, p domain.KeyframeProbe) error {
	s.saved = append(s.saved, p)
	s.cancel()
	return nil
}

func TestBackfillKeyframes(t *testing.T) {
	for _, cancelDuringProbe := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		store := &keyframeTestStore{job: domain.Job{VideoID: 5, LocationPath: "old.mkv"}, cancel: cancel}
		want := domain.KeyframeProbe{Version: domain.KeyframeProbeVersion, Spacing: domain.KeyframesUnknown}
		calls := 0
		BackfillKeyframes(ctx, store, func(_ context.Context, path string) domain.KeyframeProbe {
			calls++
			if path != "old.mkv" {
				t.Fatalf("path = %q", path)
			}
			if cancelDuringProbe {
				cancel()
			}
			return want
		}, slog.New(slog.DiscardHandler))
		cancel()
		if calls != 1 {
			t.Fatalf("probe calls = %d", calls)
		}
		if cancelDuringProbe {
			if len(store.saved) != 0 {
				t.Fatal("cancelled sample was saved")
			}
		} else if len(store.saved) != 1 || store.saved[0] != want {
			t.Fatalf("saved = %+v", store.saved)
		}
	}
}

func TestBackfillKeyframesIdleCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	idle, done := make(chan struct{}), make(chan struct{})
	store := &keyframeTestStore{idle: idle}
	go func() {
		defer close(done)
		BackfillKeyframes(ctx, store, nil, slog.New(slog.DiscardHandler))
	}()
	<-idle
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle worker did not stop")
	}
}
