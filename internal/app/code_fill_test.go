package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeCodes struct {
	coded []domain.CodedVideo
	keys  int
	calls int
}

func (c *fakeCodes) CodedVideos(context.Context, []int64) ([]domain.CodedVideo, int, error) {
	c.calls++
	return c.coded, c.keys, nil
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

func newCodeFillEnv(capacity int) (*fetchEnv, *fakeCodes) {
	e := newFetchEnv()
	codes := &fakeCodes{coded: []domain.CodedVideo{{VideoID: fetchVideo, ContentKey: "key-1", Code: " ABC-1 "}}, keys: 2}
	e.plugins.codes = codes
	e.plugins.links = &linkFetches{logger: discardLogger(), codes: codeFillQueue{jobs: make(chan codeFillJob, capacity)}}
	return e, codes
}

// The worker fetches each coded video by its trimmed code and applies the result as a fill of empty fields.
func TestStartCodeFillsFillsEachCodedVideoByItsCode(t *testing.T) {
	e, _ := newCodeFillEnv(1)
	applied := make(chan struct{}, 1)
	e.plugins.applier = signalingApplier{e.applier, applied}
	e.plugins.StartLinkFetches(context.Background(), &recordingLinkStore{}, discardLogger())
	defer e.plugins.Close()
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: domain.FetchedWorkInfo{Description: "notes"}}

	queued, err := e.plugins.StartCodeFills(context.Background(), 1, []int64{fetchVideo, otherVideo})
	if err != nil || queued != (domain.CodeFillsQueued{Queued: 1, WithoutCode: 1}) {
		t.Fatalf("StartCodeFills = %+v, %v; want 1 queued and 1 without a code", queued, err)
	}
	<-applied
	req := e.runtime.requests[0]
	apply := e.applier.calls[0]
	if req.Operation != domain.WorkInfoFetch || req.ID != "ABC-1" || req.URL != "" {
		t.Fatalf("request = %+v; want a fetch by the code", req)
	}
	if !apply.FillEmpty || apply.VideoID != fetchVideo || apply.ContentKey != "key-1" || apply.PluginID != 1 {
		t.Fatalf("apply = %+v; want a fill of the coded video", apply)
	}
}

// A plugin that cannot fetch by ID, or a request larger than the room left, queues nothing.
func TestStartCodeFillsQueuesNothingItCannotRunWhole(t *testing.T) {
	e, codes := newCodeFillEnv(1)
	e.store.plugins[1].ApprovedDigest = ""
	if _, err := e.plugins.StartCodeFills(context.Background(), 1, []int64{fetchVideo}); !errors.Is(err, domain.ErrWorkInfoPluginNotApproved) ||
		codes.calls != 0 {
		t.Fatalf("err = %v after %d code reads; want ErrWorkInfoPluginNotApproved before reading codes", err, codes.calls)
	}
	e.store.plugins[1].ApprovedDigest = "d1"
	e.store.plugins[1].Manifest.Name = domain.LinkPluginName
	if _, err := e.plugins.StartCodeFills(context.Background(), 1, []int64{fetchVideo}); !errors.Is(err, domain.ErrWorkInfoOperationUnsupported) ||
		codes.calls != 0 {
		t.Fatalf("err = %v after %d code reads; want the link-only plugin refused", err, codes.calls)
	}
	codes.coded = append(codes.coded, domain.CodedVideo{VideoID: otherVideo, ContentKey: "key-2", Code: "ABC-2"})
	if _, err := e.plugins.StartCodeFills(context.Background(), 2, []int64{fetchVideo, otherVideo}); !errors.Is(err, domain.ErrCodeFillQueueFull) ||
		len(e.plugins.links.codes.jobs) != 0 {
		t.Fatalf("err = %v with %d queued; want ErrCodeFillQueueFull and an empty queue", err, len(e.plugins.links.codes.jobs))
	}
}
