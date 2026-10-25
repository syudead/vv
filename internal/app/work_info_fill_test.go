package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeFillTargets struct {
	targets []domain.WorkInfoFillTarget
	calls   int
}

func (f *fakeFillTargets) WorkInfoFillTargets(context.Context, []int64) ([]domain.WorkInfoFillTarget, error) {
	f.calls++
	return f.targets, nil
}

type titledVideos map[int64]string

func (v titledVideos) GetVideo(_ context.Context, _ domain.Audience, id int64) (domain.Video, error) {
	return domain.Video{ID: id, Title: v[id]}, nil
}

// signalingApplier tells the test each apply has been made, since the fill runs on the worker.
type signalingApplier struct {
	*fakeFetchApplier
	applied chan struct{}
}

func (a signalingApplier) ApplyFetchedWorkInfo(
	ctx context.Context, apply domain.FetchedWorkInfoApply,
) (domain.FetchedWorkInfoApplied, error) {
	defer func() { a.applied <- struct{}{} }()
	return a.fakeFetchApplier.ApplyFetchedWorkInfo(ctx, apply)
}

func newFillEnv(capacity int) (*fetchEnv, *fakeFillTargets) {
	e := newFetchEnv()
	targets := &fakeFillTargets{targets: []domain.WorkInfoFillTarget{{VideoID: fetchVideo, ContentKey: "key-1", Code: "ABC-1"}}}
	e.plugins.fillTargets = targets
	e.plugins.links = &linkFetches{
		logger: discardLogger(), fills: workInfoFillQueue{jobs: make(chan workInfoFillJob, capacity)},
	}
	return e, targets
}

// The worker searches by the code, fetches the candidate whose ID equals it and applies the result as a fill.
func TestStartWorkInfoFillsFetchesTheMatchingCandidate(t *testing.T) {
	e, _ := newFillEnv(1)
	applied := make(chan struct{}, 1)
	e.plugins.applier = signalingApplier{e.applier, applied}
	e.plugins.StartLinkFetches(context.Background(), &recordingLinkStore{}, discardLogger())
	defer e.plugins.Close()
	e.runtime.result = domain.WorkInfoRunResult{
		Candidates: []domain.WorkInfoCandidate{{ID: "abc-10", Title: "Other"}, {ID: "abc-1", Title: "Work"}},
		WorkInfo:   domain.FetchedWorkInfo{Description: "notes"},
	}

	queued, err := e.plugins.StartWorkInfoFills(context.Background(), 1, []int64{fetchVideo})
	if err != nil || queued != 1 {
		t.Fatalf("StartWorkInfoFills = %d, %v; want 1 queued", queued, err)
	}
	<-applied
	reqs := e.runtime.requests
	if len(reqs) != 2 || reqs[0].Operation != domain.WorkInfoSearch || reqs[0].Query != "ABC-1" ||
		reqs[1].Operation != domain.WorkInfoFetch || reqs[1].ID != "abc-1" {
		t.Fatalf("requests = %+v; want a search by the code, then a fetch of abc-1", reqs)
	}
	if apply := e.applier.calls[0]; !apply.FillEmpty || apply.VideoID != fetchVideo || apply.ContentKey != "key-1" {
		t.Fatalf("apply = %+v; want a fill of the video", apply)
	}
}

// A video without a code is searched by its title, and a search with no matching candidate is recorded as a failed
// fetch without any apply.
func TestWorkInfoFillRecordsASearchWithoutAMatch(t *testing.T) {
	e, _ := newFillEnv(1)
	e.plugins.videos = titledVideos{fetchVideo: "Summer trip"}
	e.runtime.result = domain.WorkInfoRunResult{Candidates: []domain.WorkInfoCandidate{
		{ID: "w-1", Title: "Summer"}, {ID: "w-2", Title: "Trip"},
	}}
	e.plugins.runWorkInfoFill(context.Background(), workInfoFillJob{
		target: domain.WorkInfoFillTarget{VideoID: fetchVideo, ContentKey: "key-1"}, pluginID: 1,
	})
	if len(e.runtime.requests) != 1 || e.runtime.requests[0].Query != "Summer trip" {
		t.Fatalf("requests = %+v; want one search by the title", e.runtime.requests)
	}
	want := []recordedFetch{{videoID: fetchVideo, failure: `No search result matched "Summer trip".`}}
	if len(e.applier.calls) != 0 || len(e.applier.fetches) != 1 || e.applier.fetches[0] != want[0] {
		t.Fatalf("applies %d, fetches %+v; want no apply and %+v", len(e.applier.calls), e.applier.fetches, want)
	}
}

// A plugin that cannot search and fetch, or a request larger than the room left, queues nothing.
func TestStartWorkInfoFillsQueuesNothingItCannotRunWhole(t *testing.T) {
	e, targets := newFillEnv(1)
	e.store.plugins[1].ApprovedDigest = ""
	if _, err := e.plugins.StartWorkInfoFills(context.Background(), 1, []int64{fetchVideo}); !errors.Is(err, domain.ErrWorkInfoPluginNotApproved) {
		t.Fatalf("err = %v; want ErrWorkInfoPluginNotApproved", err)
	}
	e.store.plugins[1].ApprovedDigest = "d1"
	e.store.plugins[1].Manifest.Operations = []domain.WorkInfoOperation{domain.WorkInfoFetch}
	if _, err := e.plugins.StartWorkInfoFills(context.Background(), 1, []int64{fetchVideo}); !errors.Is(err, domain.ErrWorkInfoOperationUnsupported) ||
		targets.calls != 0 {
		t.Fatalf("err = %v after %d target reads; want a plugin without search refused before reading", err, targets.calls)
	}
	targets.targets = append(targets.targets, domain.WorkInfoFillTarget{VideoID: otherVideo, ContentKey: "key-2"})
	if _, err := e.plugins.StartWorkInfoFills(context.Background(), 2, []int64{fetchVideo, otherVideo}); !errors.Is(err, domain.ErrWorkInfoFillQueueFull) ||
		len(e.plugins.links.fills.jobs) != 0 {
		t.Fatalf("err = %v with %d queued; want ErrWorkInfoFillQueueFull and an empty queue", err, len(e.plugins.links.fills.jobs))
	}
}
