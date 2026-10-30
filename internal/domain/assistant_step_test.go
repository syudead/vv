package domain

import (
	"errors"
	"slices"
	"testing"
)

func TestAssistantStepStatusMoves(t *testing.T) {
	allowed := map[[2]AssistantStepStatus]bool{
		{AssistantStepPending, AssistantStepSkipped}: true, {AssistantStepPending, AssistantStepRunning}: true,
		{AssistantStepPending, AssistantStepNotRun}: true,
		{AssistantStepRunning, AssistantStepDone}:   true, {AssistantStepRunning, AssistantStepPartial}: true,
		{AssistantStepRunning, AssistantStepStale}: true, {AssistantStepRunning, AssistantStepFailed}: true,
	}
	statuses := []AssistantStepStatus{
		AssistantStepPending, AssistantStepSkipped, AssistantStepRunning, AssistantStepDone, AssistantStepPartial,
		AssistantStepStale, AssistantStepFailed, AssistantStepNotRun,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			if got := from.CanMove(to); got != allowed[[2]AssistantStepStatus{from, to}] {
				t.Errorf("%s -> %s: CanMove = %v", from, to, got)
			}
		}
	}
}

func threeSteps() AssistantProposal {
	return AssistantProposal{Status: AssistantProposed, Steps: NewAssistantSteps([]AssistantPlan{
		{ActionID: "createTag"},
		{ActionID: "updateVideoTags.add", CreatedBy: []AssistantCreatedBy{{Name: "Rewatch", Step: 0}}},
		{ActionID: "deleteTag"},
	})}
}

func stepStatuses(steps []AssistantStep) []AssistantStepStatus {
	out := make([]AssistantStepStatus, len(steps))
	for i, step := range steps {
		out[i] = step.Status
	}
	return out
}

func TestAssistantProposalSelectSkipsUntickedSteps(t *testing.T) {
	steps, err := threeSteps().Select([]int{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	want := []AssistantStepStatus{AssistantStepPending, AssistantStepPending, AssistantStepSkipped}
	if got := stepStatuses(steps); !slices.Equal(got, want) {
		t.Errorf("statuses = %v, want %v", got, want)
	}
}

func TestAssistantProposalSelectRefusesAStepWithoutItsCreator(t *testing.T) {
	var needs *AssistantStepNeedsSkippedError
	if _, err := threeSteps().Select([]int{1, 2}); !errors.As(err, &needs) || needs.Step != 1 {
		t.Errorf("err = %v, want step 1 needs a skipped step", err)
	}
	for _, kept := range [][]int{nil, {3}, {-1}} {
		if _, err := threeSteps().Select(kept); !errors.Is(err, ErrAssistantStepsInvalid) {
			t.Errorf("Select(%v) err = %v, want ErrAssistantStepsInvalid", kept, err)
		}
	}
}

func TestNewAssistantRunResultTakesTheLastStepThatRan(t *testing.T) {
	failure := &AssistantProposalError{Code: "tag_not_found"}
	steps := []AssistantStep{
		{AssistantStepState: AssistantStepState{Status: AssistantStepDone, Total: 1, Applied: 1}},
		{AssistantStepState: AssistantStepState{Status: AssistantStepSkipped, Total: 5}},
		{AssistantStepState: AssistantStepState{Status: AssistantStepFailed, Total: 2, Failed: 2, Error: failure}},
		{AssistantStepState: AssistantStepState{Status: AssistantStepPending, Total: 1}},
	}
	SkipAfter(steps, 2)
	result := NewAssistantRunResult(steps)
	if result.Status != AssistantFailed || result.Error != failure ||
		result.Counts != (AssistantCounts{Total: 4, Applied: 1, Failed: 2}) || steps[3].Status != AssistantStepNotRun {
		t.Errorf("result = %+v, last step %s", result, steps[3].Status)
	}
}

func TestAssistantPlanStaleIgnoresWhatEarlierStepsTouch(t *testing.T) {
	tag := AssistantLabelRef{Kind: AssistantLabelTag, ID: 7}
	plan := AssistantPlan{ActionID: "deleteTag", Targets: AssistantPlanTargets{Labels: []AssistantLabel{
		{Kind: AssistantLabelTag, ID: 7, Name: "Anime"},
	}}}
	renamed := AssistantCurrent{Names: map[AssistantLabelRef]string{tag: "Animation"}}
	if !plan.Stale(renamed, nil) {
		t.Error("a renamed tag is not stale")
	}
	steps := NewAssistantSteps([]AssistantPlan{{ActionID: "renameTag", Targets: plan.Targets}, plan})
	if plan.Stale(renamed, TouchedBefore(steps, 1)) {
		t.Error("a tag the earlier step renames is stale")
	}
	steps[0].Status = AssistantStepSkipped
	if !plan.Stale(renamed, TouchedBefore(steps, 1)) {
		t.Error("a tag only an unticked step names is not re-read")
	}
}

func TestAssistantPlanWithStepIDsFillsCreatedIDs(t *testing.T) {
	plan := AssistantPlan{ActionID: "updateVideoTags.add", Call: AssistantPlanCall{
		Body:    map[string]any{"tagIds": []int64{7}},
		StepIDs: []AssistantStepID{{Step: 0, Field: "tagIds"}, {Step: 1, Path: "id"}},
	}}
	call, ok := plan.WithStepIDs(plan.CallFor(nil), map[int]int64{0: 40, 1: 41})
	if !ok || !slices.Equal(call.Body["tagIds"].([]int64), []int64{7, 40}) || call.Path["id"] != "41" {
		t.Errorf("call = %+v, ok %v", call, ok)
	}
	if !slices.Equal(plan.Call.Body["tagIds"].([]int64), []int64{7}) {
		t.Error("WithStepIDs changed the plan's own body")
	}
	if _, ok := plan.WithStepIDs(plan.CallFor(nil), map[int]int64{0: 40}); ok {
		t.Error("a missing created id is accepted")
	}
}
