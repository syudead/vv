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
	current    domain.AssistantCurrent
	impactKind domain.AssistantTarget
	impactIDs  []int64
	// created is the first stored plan, which the one-plan turn tests read.
	created   domain.AssistantPlan
	plans     []domain.AssistantPlan
	progress  []domain.AssistantCounts
	finished  *domain.AssistantRunResult
	gone      map[string]bool
	discarded []int64
	// creating runs as a proposal is stored.
	creating func()
}

func (f *fakeAssistantStore) CreateProposal(
	_ context.Context, _ int64, request string, plans []domain.AssistantPlan, now time.Time,
) (domain.AssistantProposal, error) {
	if f.creating != nil {
		f.creating()
	}
	f.plans = plans
	if len(plans) > 0 {
		f.created = plans[0]
	}
	return domain.AssistantProposal{
		ID: 1, Request: request, Steps: domain.NewAssistantSteps(plans), CreatedAt: now,
	}, nil
}

func (f *fakeAssistantStore) MergeImpact(_ context.Context, target domain.AssistantTarget, ids []int64) (int, error) {
	f.impactKind, f.impactIDs = target, ids
	return 42, nil
}

func (f *fakeAssistantStore) CurrentLabels(context.Context, domain.AssistantPlan) (domain.AssistantCurrent, error) {
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

// RecordProgress keeps the sums after each batch; the write that only marks a step running repeats the last sums.
func (f *fakeAssistantStore) RecordProgress(_ context.Context, _ int64, steps []domain.AssistantStep, _ time.Time) error {
	counts := domain.AssistantStepCounts(steps)
	if n := len(f.progress); n == 0 || f.progress[n-1] != counts {
		f.progress = append(f.progress, counts)
	}
	return nil
}

func (f *fakeAssistantStore) FinishProposal(_ context.Context, _ int64, result domain.AssistantRunResult, _ time.Time) error {
	f.finished = &result
	return nil
}

// fakeRunner fails the calls whose index is in failAt, answers the bodies by call index, and runs before each
// call when set.
type fakeRunner struct {
	ActionRunner
	calls  []domain.AssistantCall
	failAt map[int]error
	bodies map[int]string
	before func(call int)
}

func (r *fakeRunner) Run(_ context.Context, call domain.AssistantCall) (domain.AssistantCallResult, error) {
	index := len(r.calls)
	if r.before != nil {
		r.before(index)
	}
	r.calls = append(r.calls, call)
	if err := r.failAt[index]; err != nil {
		return domain.AssistantCallResult{}, err
	}
	return domain.AssistantCallResult{Status: 200, Body: []byte(r.bodies[index])}, nil
}

func (r *fakeRunner) actions() []string {
	ids := make([]string, len(r.calls))
	for i, call := range r.calls {
		ids[i] = call.ActionID
	}
	return ids
}

func newTestAssistant(store *fakeAssistantStore) *Assistant {
	return NewAssistant(AssistantOptions{Store: store, Now: func() time.Time { return time.UnixMilli(1) }})
}

func TestAssistantProposeStoresMergeImpactFromSources(t *testing.T) {
	store := &fakeAssistantStore{}
	a := newTestAssistant(store)
	merge := domain.AssistantPlan{
		ActionID: "mergeTag",
		Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetTag, Labels: []domain.AssistantLabel{
			{Kind: domain.AssistantLabelTag, ID: 1, Name: "Anime"},
		}},
		Change: []domain.AssistantChange{{Argument: domain.AssistantArgNames, Labels: []domain.AssistantLabel{
			{Kind: domain.AssistantLabelTag, ID: 2, Name: "anime"}, {Kind: domain.AssistantLabelTag, ID: 3, Name: "アニメ"},
		}}},
	}
	attach := domain.AssistantPlan{ActionID: "updateVideoTags.add", Impact: &domain.AssistantImpact{Videos: 9}}
	p, err := a.Propose(context.Background(), 0, "アニメのタグをまとめて", []domain.AssistantPlan{merge, attach})
	if err != nil {
		t.Fatal(err)
	}
	if store.impactKind != domain.AssistantTargetTag || !slices.Equal(store.impactIDs, []int64{2, 3}) {
		t.Errorf("impact read for %s %v, want the sources [2 3]", store.impactKind, store.impactIDs)
	}
	if len(p.Steps) != 2 || p.Steps[0].Plan.Impact == nil || p.Steps[0].Plan.Impact.Videos != 42 {
		t.Fatalf("stored steps = %+v, want the merge with 42 videos first", p.Steps)
	}
	if p.Steps[1].Plan.Impact != nil {
		t.Errorf("updateVideoTags.add stored impact %+v", p.Steps[1].Plan.Impact)
	}
}

func batchPlan(n int) (domain.AssistantPlan, domain.AssistantCurrent) {
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
	return plan, current
}

// runningProposal is a proposal the worker claimed, with every step kept.
func runningProposal(plans ...domain.AssistantPlan) domain.AssistantProposal {
	return domain.AssistantProposal{ID: 1, Status: domain.AssistantRunning, Steps: domain.NewAssistantSteps(plans)}
}

func batchProposal(n int) (domain.AssistantProposal, domain.AssistantCurrent) {
	plan, current := batchPlan(n)
	return runningProposal(plan), current
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
	if want := []domain.AssistantCounts{{Total: 1200}, {Total: 1200, Applied: 500}}; !slices.Equal(store.progress, want) {
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
		{Total: 1200}, {Total: 1200, Applied: 500}, {Total: 1200, Applied: 1000}, {Total: 1200, Applied: 1199, Failed: 1},
	}
	if !slices.Equal(store.progress, want) {
		t.Errorf("progress = %+v, want %+v", store.progress, want)
	}
	// A video gone between batches counts as failed, so the run is partial.
	if store.finished == nil || store.finished.Status != domain.AssistantPartial || store.finished.Counts != want[3] {
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

func finishedStatuses(t *testing.T, store *fakeAssistantStore) []domain.AssistantStepStatus {
	t.Helper()
	if store.finished == nil {
		t.Fatal("the proposal was not finished")
	}
	statuses := make([]domain.AssistantStepStatus, len(store.finished.Steps))
	for i, step := range store.finished.Steps {
		statuses[i] = step.Status
	}
	return statuses
}

// threeTagSteps are three one-call steps on tags 1, 2 and 3, each named in its targets.
func threeTagSteps() ([]domain.AssistantPlan, domain.AssistantCurrent) {
	current := domain.AssistantCurrent{Names: map[domain.AssistantLabelRef]string{}}
	plans := make([]domain.AssistantPlan, 3)
	for i, action := range []string{"confirmTag", "renameTag", "deleteTag"} {
		id := int64(i + 1)
		name := fmt.Sprintf("tag %d", id)
		plans[i] = domain.AssistantPlan{ActionID: action, Targets: domain.AssistantPlanTargets{
			Kind: domain.AssistantTargetTag, Labels: []domain.AssistantLabel{{Kind: domain.AssistantLabelTag, ID: id, Name: name}},
		}}
		current.Names[domain.AssistantLabelRef{Kind: domain.AssistantLabelTag, ID: id}] = name
	}
	return plans, current
}

func TestAssistantWorkerRunsKeptStepsInOrder(t *testing.T) {
	plans, current := threeTagSteps()
	store := &fakeAssistantStore{current: current}
	runner := &fakeRunner{}

	newTestAssistant(store).execute(context.Background(), runner, runningProposal(plans...))

	if want := []string{"confirmTag", "renameTag", "deleteTag"}; !slices.Equal(runner.actions(), want) {
		t.Errorf("calls = %v, want %v", runner.actions(), want)
	}
	want := []domain.AssistantStepStatus{domain.AssistantStepDone, domain.AssistantStepDone, domain.AssistantStepDone}
	if got := finishedStatuses(t, store); !slices.Equal(got, want) || store.finished.Status != domain.AssistantDone {
		t.Errorf("steps = %v, proposal %s; want %v and done", got, store.finished.Status, want)
	}
}

func TestAssistantWorkerStopsAtTheFailedStep(t *testing.T) {
	plans, current := threeTagSteps()
	store := &fakeAssistantStore{current: current}
	runner := &fakeRunner{failAt: map[int]error{
		1: &domain.AssistantCallError{Status: 409, Code: "tag_name_taken", Message: "taken"},
	}}

	newTestAssistant(store).execute(context.Background(), runner, runningProposal(plans...))

	if len(runner.calls) != 2 {
		t.Errorf("calls = %v, want the third step never called", runner.actions())
	}
	want := []domain.AssistantStepStatus{domain.AssistantStepDone, domain.AssistantStepFailed, domain.AssistantStepNotRun}
	if got := finishedStatuses(t, store); !slices.Equal(got, want) {
		t.Errorf("steps = %v, want %v", got, want)
	}
	failure := domain.AssistantProposalError{Code: "tag_name_taken", Message: "taken"}
	if e := store.finished.Steps[1].Error; e == nil || *e != failure {
		t.Errorf("second step error = %+v, want %+v", e, failure)
	}
	if store.finished.Status != domain.AssistantFailed || store.finished.Error == nil || *store.finished.Error != failure {
		t.Errorf("proposal = %s with %+v, want failed with the second step's error", store.finished.Status, store.finished.Error)
	}
}

func TestAssistantWorkerNeverCallsAnUntickedStep(t *testing.T) {
	plans, current := threeTagSteps()
	store := &fakeAssistantStore{current: current}
	runner := &fakeRunner{}
	p := runningProposal(plans...)
	p.Steps[1].Status = domain.AssistantStepSkipped

	newTestAssistant(store).execute(context.Background(), runner, p)

	if want := []string{"confirmTag", "deleteTag"}; !slices.Equal(runner.actions(), want) {
		t.Errorf("calls = %v, want %v", runner.actions(), want)
	}
	want := []domain.AssistantStepStatus{domain.AssistantStepDone, domain.AssistantStepSkipped, domain.AssistantStepDone}
	if got := finishedStatuses(t, store); !slices.Equal(got, want) {
		t.Errorf("steps = %v, want %v", got, want)
	}
}

func TestAssistantWorkerPassesTheCreatedIDToALaterStep(t *testing.T) {
	plan, current := batchPlan(2)
	create := domain.AssistantPlan{ActionID: "createTag", Call: domain.AssistantPlanCall{Body: map[string]any{"name": "Rewatch"}}}
	plan.Change = []domain.AssistantChange{{Argument: domain.AssistantArgTagNames, Labels: []domain.AssistantLabel{
		{Kind: domain.AssistantLabelTag, Name: "Rewatch"},
	}}}
	plan.Call.Body = map[string]any{"tagIds": []int64{}}
	plan.Call.StepIDs = []domain.AssistantStepID{{Step: 0, Field: "tagIds"}}
	plan.CreatedBy = []domain.AssistantCreatedBy{{Name: "Rewatch", Step: 0}}
	store := &fakeAssistantStore{current: current}
	runner := &fakeRunner{bodies: map[int]string{0: `{"id":40,"name":"Rewatch"}`}}

	newTestAssistant(store).execute(context.Background(), runner, runningProposal(create, plan))

	if len(runner.calls) != 2 {
		t.Fatalf("calls = %v, want both steps", runner.actions())
	}
	if ids, _ := runner.calls[1].Body["tagIds"].([]int64); !slices.Equal(ids, []int64{40}) {
		t.Errorf("second step tagIds = %v, want the created id [40]", runner.calls[1].Body["tagIds"])
	}
}

func TestAssistantWorkerEndsStaleAtAStepWhoseLabelChanged(t *testing.T) {
	plans, current := threeTagSteps()
	store := &fakeAssistantStore{current: current}
	runner := &fakeRunner{before: func(call int) {
		if call == 0 {
			store.current.Names[domain.AssistantLabelRef{Kind: domain.AssistantLabelTag, ID: 2}] = "renamed elsewhere"
		}
	}}

	newTestAssistant(store).execute(context.Background(), runner, runningProposal(plans...))

	if want := []string{"confirmTag"}; !slices.Equal(runner.actions(), want) {
		t.Errorf("calls = %v, want %v", runner.actions(), want)
	}
	want := []domain.AssistantStepStatus{domain.AssistantStepDone, domain.AssistantStepStale, domain.AssistantStepNotRun}
	if got := finishedStatuses(t, store); !slices.Equal(got, want) || store.finished.Status != domain.AssistantStale {
		t.Errorf("steps = %v, proposal %s; want %v and stale", got, store.finished.Status, want)
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
