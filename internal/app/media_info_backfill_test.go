package app

import (
	"context"
	"log/slog"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type mediaInfoTestStore struct {
	job    domain.Job
	saved  []domain.MediaInfoProbe
	cancel context.CancelFunc
}

func (s *mediaInfoTestStore) NextMediaInfoProbe(context.Context, int64) (domain.Job, error) {
	return s.job, nil
}

func (s *mediaInfoTestStore) SaveMediaInfoProbe(_ context.Context, job domain.Job, p domain.MediaInfoProbe) error {
	if job != s.job {
		panic("saved another job")
	}
	s.saved = append(s.saved, p)
	s.cancel()
	return nil
}

// The paced loop is covered by the keyframe backfill; this checks the media information wiring.
func TestBackfillMediaInfoSavesTheReadOfTheJobsFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &mediaInfoTestStore{job: domain.Job{VideoID: 5, LocationPath: "old.mkv"}, cancel: cancel}
	want := domain.MediaInfoProbe{Source: domain.FileStamp{SizeBytes: 1}, Info: &domain.MediaInfo{}}
	BackfillMediaInfo(ctx, store, func(_ context.Context, path string) domain.MediaInfoProbe {
		if path != "old.mkv" {
			t.Errorf("path = %q", path)
		}
		return want
	}, slog.New(slog.DiscardHandler))
	if len(store.saved) != 1 || store.saved[0] != want {
		t.Errorf("saved = %+v, want %+v", store.saved, want)
	}
}
