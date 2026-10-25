package app

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeAssistantStore keeps one proposal in memory and records what the worker writes.
type fakeAssistantStore struct {
	AssistantStore
	current     domain.AssistantCurrent
	impactKind  domain.AssistantTarget
	impactIDs   []int64
	created     domain.AssistantPlan
	progress    []domain.AssistantCounts
	finished    *domain.AssistantRunResult
	gone        map[string]bool
	currentRead int
}

func (f *fakeAssistantStore) CreateProposal(
	_ context.Context, request string, plan domain.AssistantPlan, now time.Time,
) (domain.AssistantProposal, error) {
	f.created = plan
	return domain.AssistantProposal{ID: 1, Request: request, Plan: plan, CreatedAt: now}, nil
}

func (f *fakeAssistantStore) MergeImpact(_ context.Context, target domain.AssistantTarget, ids []int64) (int, error) {
	f.impactKind, f.impactIDs = target, ids
	return 42, nil
}

func (f *fakeAssistantStore) CurrentLabels(context.Context, domain.AssistantPlan) (domain.AssistantCurrent, error) {
	f.currentRead++
	return f.current, nil
}

func (f *fakeAssistantStore) VideoIDs(_ context.Context, keys []string) (map[string]int64, error) {
	ids := map[string]int64{}
	for _, key := range keys {
		if id, ok := f.current.Videos[key]; ok && !f.gone[key] {
			ids[key] = id
		}
	}
	return ids, nil
}

func (f *fakeAssistantStore) RecordProgress(_ context.Context, _ int64, counts domain.AssistantCounts, _ time.Time) error {
	f.progress = append(f.progress, counts)
	return nil
}

func (f *fakeAssistantStore) FinishProposal(_ context.Context, _ int64, result domain.AssistantRunResult, _ time.Time) error {
	f.finished = &result
	return nil
}

// fakeRunner fails the calls whose index is in failAt.
type fakeRunner struct {
	calls  []domain.AssistantCall
	failAt map[int]error
}

func (r *fakeRunner) Run(_ context.Context, call domain.AssistantCall) (domain.AssistantCallResult, error) {
	r.calls = append(r.calls, call)
	if err := r.failAt[len(r.calls)-1]; err != nil {
		return domain.AssistantCallResult{}, err
	}
	return domain.AssistantCallResult{Status: 200}, nil
}

func newTestAssistant(store *fakeAssistantStore) *Assistant {
	return NewAssistant(AssistantOptions{Store: store, Now: func() time.Time { return time.UnixMilli(1) }})
}

func TestAssistantProposeStoresMergeImpactFromSources(t *testing.T) {
	store := &fakeAssistantStore{}
	a := newTestAssistant(store)
	plan := domain.AssistantPlan{
		ActionID: "mergeTag",
		Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetTag, Labels: []domain.AssistantLabel{
			{Kind: domain.AssistantLabelTag, ID: 1, Name: "Anime"},
		}},
		Change: []domain.AssistantChange{{Argument: domain.AssistantArgNames, Labels: []domain.AssistantLabel{
			{Kind: domain.AssistantLabelTag, ID: 2, Name: "anime"}, {Kind: domain.AssistantLabelTag, ID: 3, Name: "アニメ"},
		}}},
	}
	p, err := a.Propose(context.Background(), "アニメのタグをまとめて", plan)
	if err != nil {
		t.Fatal(err)
	}
	if store.impactKind != domain.AssistantTargetTag || !slices.Equal(store.impactIDs, []int64{2, 3}) {
		t.Errorf("impact read for %s %v, want the sources [2 3]", store.impactKind, store.impactIDs)
	}
	if p.Plan.Impact == nil || p.Plan.Impact.Videos != 42 {
		t.Errorf("stored impact = %+v, want 42 videos", p.Plan.Impact)
	}

	store.impactIDs = nil
	attach := domain.AssistantPlan{ActionID: "updateVideoTags.add", Impact: &domain.AssistantImpact{Videos: 9}}
	if _, err := a.Propose(context.Background(), "タグを付けて", attach); err != nil {
		t.Fatal(err)
	}
	if store.created.Impact != nil || store.impactIDs != nil {
		t.Errorf("updateVideoTags.add stored impact %+v", store.created.Impact)
	}
}

func batchProposal(n int) (domain.AssistantProposal, domain.AssistantCurrent) {
	current := domain.AssistantCurrent{Videos: map[string]int64{}}
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%d", i)
		current.Videos[keys[i]] = int64(i + 1)
	}
	plan := domain.AssistantPlan{
		ActionID: "updateVideoTags.add",
		Targets:  domain.AssistantPlanTargets{Kind: domain.AssistantTargetVideos, VideoKeys: keys},
		Change: []domain.AssistantChange{{Argument: domain.AssistantArgTagNames, Labels: []domain.AssistantLabel{
			{Kind: domain.AssistantLabelTag, ID: 7, Name: "Anime"},
		}}},
		Call: domain.AssistantPlanCall{Body: map[string]any{"tagIds": []int64{7}}, VideoField: "videoIds"},
	}
	current.Names = map[domain.AssistantLabelRef]string{{Kind: domain.AssistantLabelTag, ID: 7}: "Anime"}
	return domain.AssistantProposal{ID: 1, Status: domain.AssistantRunning, Plan: plan}, current
}

func TestAssistantWorkerReportsEachBatchAndStopsAtFailure(t *testing.T) {
	p, current := batchProposal(1200)
	store := &fakeAssistantStore{current: current}
	runner := &fakeRunner{failAt: map[int]error{
		1: &domain.AssistantCallError{Status: 404, Code: "not_found", Reason: "video_not_found"},
	}}
	a := newTestAssistant(store)

	a.execute(context.Background(), runner, p)

	if len(runner.calls) != 2 {
		t.Fatalf("calls = %d, want 2: the run stops at the failed second batch", len(runner.calls))
	}
	if ids := runner.calls[1].Body["videoIds"].([]int64); len(ids) != 500 || ids[0] != 501 {
		t.Errorf("second batch ids start %d with %d ids", ids[0], len(ids))
	}
	if want := []domain.AssistantCounts{{Total: 1200, Applied: 500}}; !slices.Equal(store.progress, want) {
		t.Errorf("progress = %+v, want %+v", store.progress, want)
	}
	want := domain.AssistantRunResult{
		Status: domain.AssistantPartial, Counts: domain.AssistantCounts{Total: 1200, Applied: 500, Failed: 500},
	}
	if store.finished == nil || store.finished.Status != want.Status || store.finished.Counts != want.Counts ||
		store.finished.Error == nil || *store.finished.Error != (domain.AssistantProposalError{Code: "not_found", Reason: "video_not_found"}) {
		t.Errorf("finished = %+v, want %+v with the call's code", store.finished, want)
	}
}

func TestAssistantWorkerFinishesEveryBatch(t *testing.T) {
	p, current := batchProposal(1200)
	store := &fakeAssistantStore{current: current, gone: map[string]bool{"k1100": true}}
	runner := &fakeRunner{}

	newTestAssistant(store).execute(context.Background(), runner, p)

	if len(runner.calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(runner.calls))
	}
	want := []domain.AssistantCounts{
		{Total: 1200, Applied: 500}, {Total: 1200, Applied: 1000}, {Total: 1200, Applied: 1199, Failed: 1},
	}
	if !slices.Equal(store.progress, want) {
		t.Errorf("progress = %+v, want %+v", store.progress, want)
	}
	// A video gone between batches counts as failed, so the run is partial.
	if store.finished == nil || store.finished.Status != domain.AssistantPartial || store.finished.Counts != want[2] {
		t.Errorf("finished = %+v", store.finished)
	}
}

func TestAssistantWorkerFailsWhenTheFirstBatchFails(t *testing.T) {
	p, current := batchProposal(3)
	store := &fakeAssistantStore{current: current}
	runner := &fakeRunner{failAt: map[int]error{0: &domain.AssistantCallError{Status: 400, Code: "invalid_request"}}}

	newTestAssistant(store).execute(context.Background(), runner, p)

	if store.finished == nil || store.finished.Status != domain.AssistantFailed ||
		store.finished.Counts != (domain.AssistantCounts{Total: 3, Failed: 3}) {
		t.Errorf("finished = %+v", store.finished)
	}
}

func TestAssistantWorkerEndsStaleWithoutCalls(t *testing.T) {
	p, current := batchProposal(3)
	current.Names[domain.AssistantLabelRef{Kind: domain.AssistantLabelTag, ID: 7}] = "Animation"
	store := &fakeAssistantStore{current: current}
	runner := &fakeRunner{}

	newTestAssistant(store).execute(context.Background(), runner, p)

	if len(runner.calls) != 0 {
		t.Errorf("calls = %d, want none", len(runner.calls))
	}
	if store.finished == nil || store.finished.Status != domain.AssistantStale || store.finished.Counts.Applied != 0 {
		t.Errorf("finished = %+v", store.finished)
	}
}

// claimingStore hands the worker its proposals, then stops the run on the next claim.
type claimingStore struct {
	*fakeAssistantStore
	pending []domain.AssistantProposal
	stop    context.CancelFunc
}

func (s *claimingStore) ExpireProposals(context.Context, time.Time) (int, error) { return 0, nil }

func (s *claimingStore) ClaimProposal(context.Context, time.Time) (domain.AssistantProposal, error) {
	if len(s.pending) == 0 {
		s.stop()
		return domain.AssistantProposal{}, domain.ErrNoJob
	}
	p := s.pending[0]
	s.pending = s.pending[1:]
	return p, nil
}

func TestAssistantRunExecutesClaimedProposalsWithTheRegisteredRunner(t *testing.T) {
	p, current := batchProposal(2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := &fakeAssistantStore{current: current}
	a := NewAssistant(AssistantOptions{
		Store: &claimingStore{fakeAssistantStore: fake, pending: []domain.AssistantProposal{p}, stop: cancel},
	})
	runner := &fakeRunner{}
	a.UseRunner(runner)

	a.Run(ctx)

	if len(runner.calls) != 1 || fake.finished == nil || fake.finished.Status != domain.AssistantDone {
		t.Errorf("calls = %d, finished = %+v", len(runner.calls), fake.finished)
	}
}
