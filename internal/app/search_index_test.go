package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type searchIndexTestStore struct {
	durationMs int64
	jobs       []domain.Job
	indexes    []domain.SearchIndex
}

func (s *searchIndexTestStore) GetVideo(_ context.Context, id int64) (domain.Video, error) {
	return domain.Video{ID: id, DurationMs: &s.durationMs}, nil
}

func (*searchIndexTestStore) QueueFrameIndexJobs(context.Context, domain.JobPriority) (int, error) {
	return 0, nil
}

func (s *searchIndexTestStore) ApplySearchIndexForJob(_ context.Context, job domain.Job, index domain.SearchIndex) (bool, error) {
	s.jobs = append(s.jobs, job)
	s.indexes = append(s.indexes, index)
	return true, nil
}

// The job's resolved file is read and its keyframes kept at the spacing of the video's duration: 1.5 s for a
// 30,000-second video.
func TestSearchIndexBuilderSavesTheKeyframesOfTheJobsFile(t *testing.T) {
	store := &searchIndexTestStore{durationMs: 30_000_000}
	job := domain.Job{ID: 3, VideoID: 5, LocationPath: "/media/a.mkv"}
	read := func(_ context.Context, path string, keep func(domain.KeyframeHash)) error {
		if path != "resolved:/media/a.mkv" {
			t.Errorf("path = %q", path)
		}
		for i, timeMs := range []int64{0, 1000, 2000} {
			keep(domain.KeyframeHash{TimeMs: timeMs, Frame: domain.FrameHash{Hash: uint64(i + 1)}})
		}
		return nil
	}
	if err := NewSearchIndexBuilder(store, testSources(), read).Build(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	want := domain.SearchIndex{Version: domain.SearchIndexVersion,
		Entries: []domain.SearchIndexEntry{{TimeMs: 0, Hash: 1}, {TimeMs: 2000, Hash: 3}}}
	if len(store.jobs) != 1 || store.jobs[0] != job || !reflect.DeepEqual(store.indexes[0], want) {
		t.Fatalf("saved %v %+v, want the job with %+v", store.jobs, store.indexes, want)
	}
}

// A refused location or a failed read fails the job for a retry and saves nothing.
func TestSearchIndexBuilderSavesNothingWhenTheFileCannotBeRead(t *testing.T) {
	failed := errors.New("ffmpeg failed")
	for name, path := range map[string]string{"refused": "/elsewhere/a.mkv", "unreadable": "/media/a.mkv"} {
		store := &searchIndexTestStore{}
		read := func(_ context.Context, _ string, keep func(domain.KeyframeHash)) error {
			keep(domain.KeyframeHash{Frame: domain.FrameHash{Hash: 1}})
			return failed
		}
		err := NewSearchIndexBuilder(store, testSources(), read).Build(context.Background(), domain.Job{LocationPath: path})
		if err == nil || len(store.jobs) != 0 {
			t.Errorf("%s: build = %v, saved %v, want an error and nothing saved", name, err, store.jobs)
		}
	}
}
