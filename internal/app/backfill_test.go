package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type keyframeTestStore struct {
	queued []domain.JobPriority
	saved  []domain.Job
}

func (s *keyframeTestStore) QueueKeyframeJobs(_ context.Context, priority domain.JobPriority) (int, error) {
	s.queued = append(s.queued, priority)
	return 1, nil
}

func (s *keyframeTestStore) SaveKeyframeProbe(_ context.Context, job domain.Job, _ domain.KeyframeProbe) error {
	s.saved = append(s.saved, job)
	return nil
}

func TestBackfillQueuesInTheQueueAllJobsClass(t *testing.T) {
	store := &keyframeTestStore{}
	backfill := KeyframeBackfill(store, nil)
	if err := backfill.QueueAll(context.Background(), domain.Job{Priority: domain.JobPriorityOwner}); err != nil {
		t.Fatal(err)
	}
	if len(store.queued) != 1 || store.queued[0] != domain.JobPriorityOwner {
		t.Fatalf("queued classes = %v, want the queue-all job's class", store.queued)
	}
}

func TestBackfillReadSavesTheReadOfTheJobsFile(t *testing.T) {
	for _, cancelDuringRead := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		store := &keyframeTestStore{}
		job := domain.Job{ID: 3, VideoID: 5, LocationPath: "old.mkv"}
		err := KeyframeBackfill(store, func(_ context.Context, path string) domain.KeyframeProbe {
			if path != "old.mkv" {
				t.Errorf("path = %q", path)
			}
			if cancelDuringRead {
				cancel()
			}
			return domain.KeyframeProbe{}
		}).Read(ctx, job)
		cancel()
		if cancelDuringRead {
			if !errors.Is(err, context.Canceled) || len(store.saved) != 0 {
				t.Errorf("cancelled read = %v, saved %v, want nothing saved", err, store.saved)
			}
		} else if err != nil || len(store.saved) != 1 || store.saved[0] != job {
			t.Errorf("read = %v, saved %v, want the job saved", err, store.saved)
		}
	}
}
