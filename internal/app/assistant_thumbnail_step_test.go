package app

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// editRunner answers a thumbnail edit session's stages, and its reads with the states in order.
type editRunner struct {
	fakeRunner
	states []string
}

func (r *editRunner) Read(_ context.Context, read domain.AssistantRead) (domain.AssistantReadResult, error) {
	state := r.states[0]
	r.states = r.states[1:]
	body, _ := json.Marshal(map[string]any{"state": state, "error": map[string]string{"reason": "node_error"}})
	return domain.AssistantReadResult{Body: body}, nil
}

func (r *editRunner) operations() []string {
	out := make([]string, len(r.calls))
	for i, call := range r.calls {
		out[i] = call.Operation
	}
	return out
}

func thumbnailEditProposal() (domain.AssistantProposal, domain.AssistantCurrent) {
	plan := domain.AssistantPlan{
		ActionID: "editVideoThumbnail",
		Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetVideo, VideoKeys: []string{"key-7"},
			Labels: []domain.AssistantLabel{{Kind: domain.AssistantLabelVideo, Key: "key-7", Name: "Clip 7"}}},
		Call: domain.AssistantPlanCall{VideoPathParam: "id", Body: map[string]any{"positionMs": int64(1500), "prompt": "brighter"}},
	}
	return runningProposal(plan), domain.AssistantCurrent{Videos: map[string]int64{"key-7": 7}}
}

func TestThumbnailEditStepRunsItsStagesAndAppliesTheResult(t *testing.T) {
	p, current := thumbnailEditProposal()
	store := &fakeAssistantStore{current: current}
	runner := &editRunner{fakeRunner: fakeRunner{bodies: map[int]string{0: `{"id":"e1"}`, 2: `{"id":7}`}}, states: []string{"succeeded"}}

	newTestAssistant(store).execute(context.Background(), runner, p)

	if got := runner.operations(); !slices.Equal(got, []string{"startThumbnailEdit", "runThumbnailEdit", "applyThumbnailEdit"}) {
		t.Fatalf("operations = %v", got)
	}
	start, run := runner.calls[0], runner.calls[1]
	if start.Path["id"] != "7" || start.Body["source"] != "frame" || start.Body["positionMs"] != int64(1500) ||
		run.Path["editId"] != "e1" || run.Body["prompt"] != "brighter" || runner.calls[2].Path["editId"] != "e1" {
		t.Fatalf("calls = %+v", runner.calls)
	}
	if store.finished == nil || store.finished.Status != domain.AssistantDone {
		t.Fatalf("finished = %+v", store.finished)
	}
}

func TestThumbnailEditStepThatFailsDiscardsTheSession(t *testing.T) {
	p, current := thumbnailEditProposal()
	store := &fakeAssistantStore{current: current}
	runner := &editRunner{fakeRunner: fakeRunner{bodies: map[int]string{0: `{"id":"e1"}`}}, states: []string{"failed"}}

	newTestAssistant(store).execute(context.Background(), runner, p)

	want := []string{"startThumbnailEdit", "runThumbnailEdit", "cancelThumbnailEdit", "discardThumbnailEdit"}
	if got := runner.operations(); !slices.Equal(got, want) {
		t.Fatalf("operations = %v, want %v", got, want)
	}
	if store.finished == nil || store.finished.Status != domain.AssistantFailed || store.finished.Error == nil ||
		store.finished.Error.Code != "thumbnail_edit_failed" || store.finished.Error.Reason != "node_error" {
		t.Fatalf("finished = %+v", store.finished)
	}
}
