package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeFillTargets struct {
	targets []domain.WorkInfoFillTarget
}

func (f *fakeFillTargets) WorkInfoFillTargets(context.Context, []int64) ([]domain.WorkInfoFillTarget, error) {
	return f.targets, nil
}

type titledVideos map[int64]string

func (v titledVideos) GetVideo(_ context.Context, _ domain.Audience, id int64) (domain.Video, error) {
	title, ok := v[id]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return domain.Video{ID: id, ContentKey: "key-1", Title: title}, nil
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

func newFillEnv(capacity int) *fetchEnv {
	e := newFetchEnv()
	e.plugins.videos = titledVideos{fetchVideo: "Summer trip"}
	e.plugins.links = &linkFetches{
		logger: discardLogger(), fills: workInfoFillQueue{jobs: make(chan workInfoFillJob, capacity)},
	}
	return e
}

// A target searches by its code, or by its title without one; a video gone since is left out.
func TestWorkInfoFillQueriesUseTheCodeOrTheTitle(t *testing.T) {
	e := newFillEnv(1)
	e.plugins.videos = titledVideos{fetchVideo: "Summer trip", otherVideo: "Winter"}
	e.plugins.fillTargets = &fakeFillTargets{targets: []domain.WorkInfoFillTarget{
		{VideoID: fetchVideo}, {VideoID: otherVideo, Code: "ABC-1"}, {VideoID: 99},
	}}
	got, err := e.plugins.WorkInfoFillQueries(context.Background(), []int64{fetchVideo, otherVideo, 99})
	want := []domain.WorkInfoFillQuery{
		{VideoID: fetchVideo, Title: "Summer trip", Query: "Summer trip"}, {VideoID: otherVideo, Title: "Winter", Query: "ABC-1"},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("WorkInfoFillQueries = %+v, %v; want %+v", got, err, want)
	}
}

// The worker fetches the chosen work by its trimmed ID and applies the result as a fill of the video.
func TestStartWorkInfoFillsFetchesTheChosenWork(t *testing.T) {
	e := newFillEnv(1)
	applied := make(chan struct{}, 1)
	e.plugins.applier = signalingApplier{e.applier, applied}
	e.plugins.StartLinkFetches(context.Background(), &recordingLinkStore{}, discardLogger())
	defer e.plugins.Close()
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: domain.FetchedWorkInfo{Description: "notes"}}

	queued, err := e.plugins.StartWorkInfoFills(context.Background(), 1, []domain.WorkInfoFillItem{{VideoID: fetchVideo, WorkID: " w-1 "}})
	if err != nil || queued != 1 {
		t.Fatalf("StartWorkInfoFills = %d, %v; want 1 queued", queued, err)
	}
	<-applied
	if reqs := e.runtime.requests; len(reqs) != 1 || reqs[0].Operation != domain.WorkInfoFetch || reqs[0].ID != "w-1" {
		t.Fatalf("requests = %+v; want one fetch of w-1", reqs)
	}
	if apply := e.applier.calls[0]; !apply.FillEmpty || apply.VideoID != fetchVideo || apply.ContentKey != "key-1" {
		t.Fatalf("apply = %+v; want a fill of the video", apply)
	}
}

// An invalid work ID, a plugin that cannot fetch, or a request larger than the room left queues nothing.
func TestStartWorkInfoFillsQueuesNothingItCannotRunWhole(t *testing.T) {
	e := newFillEnv(1)
	one := []domain.WorkInfoFillItem{{VideoID: fetchVideo, WorkID: "w-1"}}
	if _, err := e.plugins.StartWorkInfoFills(context.Background(), 1, []domain.WorkInfoFillItem{{VideoID: fetchVideo, WorkID: " "}}); !errors.Is(err, domain.ErrWorkInfoRunInvalid) {
		t.Fatalf("err = %v; want ErrWorkInfoRunInvalid for a blank work ID", err)
	}
	e.store.plugins[1].ApprovedDigest = ""
	if _, err := e.plugins.StartWorkInfoFills(context.Background(), 1, one); !errors.Is(err, domain.ErrWorkInfoPluginNotApproved) {
		t.Fatalf("err = %v; want ErrWorkInfoPluginNotApproved", err)
	}
	two := append(one, domain.WorkInfoFillItem{VideoID: otherVideo, WorkID: "w-2"})
	if _, err := e.plugins.StartWorkInfoFills(context.Background(), 2, two); !errors.Is(err, domain.ErrWorkInfoFillQueueFull) {
		t.Fatalf("err = %v; want ErrWorkInfoFillQueueFull", err)
	}
	if n := len(e.plugins.links.fills.jobs); n != 0 {
		t.Fatalf("%d queued; want none", n)
	}
}
