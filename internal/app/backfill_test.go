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
	probes []domain.KeyframeProbe
}

func (s *keyframeTestStore) QueueKeyframeJobs(_ context.Context, priority domain.JobPriority) (int, error) {
	s.queued = append(s.queued, priority)
	return 1, nil
}

func (s *keyframeTestStore) SaveKeyframeProbe(_ context.Context, job domain.Job, probe domain.KeyframeProbe) error {
	s.saved = append(s.saved, job)
	s.probes = append(s.probes, probe)
	return nil
}

func TestBackfillQueuesInTheQueueAllJobsClass(t *testing.T) {
	store := &keyframeTestStore{}
	backfill := KeyframeBackfill(store, MediaFiles{}, nil)
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
		job := domain.Job{ID: 3, VideoID: 5, LocationPath: "/media/old.mkv"}
		err := KeyframeBackfill(store, testSources(), func(_ context.Context, path string) domain.KeyframeProbe {
			if path != "resolved:/media/old.mkv" {
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

func testSources() MediaFiles {
	return MediaFiles{Folders: &fakeMediaFolders{}, Resolver: fakeMediaFiles{}}
}

// A location the media folders' access rules refuse is saved as unreadable without being read.
func TestBackfillReadSavesARefusedLocationAsUnavailable(t *testing.T) {
	store := &keyframeTestStore{}
	job := domain.Job{ID: 3, VideoID: 5, LocationPath: "/elsewhere/old.mkv"}
	err := KeyframeBackfill(store, testSources(), func(context.Context, string) domain.KeyframeProbe {
		t.Error("read a refused location")
		return domain.KeyframeProbe{}
	}).Read(context.Background(), job)
	want := domain.KeyframeProbe{Version: domain.KeyframeProbeVersion, Spacing: domain.KeyframesUnknown}
	if err != nil || len(store.probes) != 1 || store.probes[0] != want {
		t.Errorf("read = %v, saved %v, want %v", err, store.probes, want)
	}
}
