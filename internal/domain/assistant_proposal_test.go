package domain

import (
	"fmt"
	"slices"
	"testing"
)

func assistantKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%04d", i)
	}
	return keys
}

func TestAssistantPlanBatchesSplitVideoKeysInOrder(t *testing.T) {
	keys := assistantKeys(1200)
	plan := AssistantPlan{ActionID: "updateVideoTags.add", Targets: AssistantPlanTargets{VideoKeys: keys}}

	batches := plan.Batches()
	var sizes []int
	for _, batch := range batches {
		sizes = append(sizes, len(batch))
	}
	if !slices.Equal(sizes, []int{500, 500, 200}) {
		t.Fatalf("batch sizes = %v, want [500 500 200]", sizes)
	}
	if !slices.Equal(slices.Concat(batches...), keys) {
		t.Error("the batches do not keep the plan order")
	}
	if plan.Total() != 1200 {
		t.Errorf("Total = %d, want 1200", plan.Total())
	}

	// An operation without batches is one call over every key and counts as that one call.
	group := AssistantPlan{ActionID: "createGroup", Targets: AssistantPlanTargets{VideoKeys: keys}}
	if got := group.Batches(); len(got) != 1 || len(got[0]) != 1200 || group.Total() != 1 {
		t.Errorf("createGroup: %d batches, total %d", len(got), group.Total())
	}

	// A batching operation on groups only still makes its one call.
	groupsOnly := AssistantPlan{
		ActionID: "updateFavorites",
		Targets:  AssistantPlanTargets{Kind: AssistantTargetVideosOrGroups, Labels: []AssistantLabel{{Kind: AssistantLabelGroup, ID: 7, Name: "g"}}},
	}
	if got := groupsOnly.Batches(); len(got) != 1 || len(got[0]) != 0 || groupsOnly.Total() != 1 {
		t.Errorf("updateFavorites on groups only: batches %v, total %d; want one empty batch and total 1", got, groupsOnly.Total())
	}
}

func TestAssistantPlanStale(t *testing.T) {
	plan := AssistantPlan{
		ActionID: "updateVideoTags.add",
		Targets:  AssistantPlanTargets{VideoKeys: []string{"k1", "k2"}},
		Change: []AssistantChange{{Argument: AssistantArgTagNames, Labels: []AssistantLabel{
			{Kind: AssistantLabelTag, ID: 7, Name: "Anime"},
		}}},
	}
	current := func() AssistantCurrent {
		return AssistantCurrent{
			Names:  map[AssistantLabelRef]string{{Kind: AssistantLabelTag, ID: 7}: "Anime"},
			Videos: map[string]int64{"k1": 1, "k2": 2},
		}
	}
	if plan.Stale(current()) {
		t.Fatal("an unchanged plan is stale")
	}

	// A tag the plan creates has no id and no current name; it must not make the plan stale.
	creates := plan
	creates.Change = []AssistantChange{{Argument: AssistantArgTagNames, Labels: []AssistantLabel{
		{Kind: AssistantLabelTag, Name: "Rewatch"},
	}}}
	creates.Created = []string{"Rewatch"}
	if creates.Stale(current()) {
		t.Error("a plan that creates its tag is stale")
	}

	renamed := current()
	renamed.Names[AssistantLabelRef{Kind: AssistantLabelTag, ID: 7}] = "Animation"
	deleted := current()
	delete(deleted.Names, AssistantLabelRef{Kind: AssistantLabelTag, ID: 7})
	gone := current()
	delete(gone.Videos, "k2")
	for name, c := range map[string]AssistantCurrent{"renamed tag": renamed, "deleted tag": deleted, "gone video": gone} {
		if !plan.Stale(c) {
			t.Errorf("%s: the plan is not stale", name)
		}
	}
}

func TestAssistantProposalIrreversibleFollowsCatalog(t *testing.T) {
	for id, want := range map[string]bool{
		"mergeTag": true, "batchTags.reject": true, "batchTags.confirm": false, "updateVideoTags.add": false,
	} {
		p := AssistantProposal{Plan: AssistantPlan{ActionID: id}}
		if got := p.Irreversible(); got != want {
			t.Errorf("%s: Irreversible = %v, want %v", id, got, want)
		}
	}
}

func TestAssistantProposalStatusMoves(t *testing.T) {
	allowed := map[[2]AssistantProposalStatus]bool{
		{AssistantProposed, AssistantApproved}: true, {AssistantProposed, AssistantDiscarded}: true,
		{AssistantApproved, AssistantRunning}: true,
		{AssistantRunning, AssistantDone}:     true, {AssistantRunning, AssistantPartial}: true,
		{AssistantRunning, AssistantStale}: true, {AssistantRunning, AssistantFailed}: true,
	}
	statuses := []AssistantProposalStatus{
		AssistantProposed, AssistantApproved, AssistantRunning, AssistantDone, AssistantPartial, AssistantStale,
		AssistantFailed, AssistantDiscarded,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			if got := from.CanMove(to); got != allowed[[2]AssistantProposalStatus{from, to}] {
				t.Errorf("%s -> %s: CanMove = %v", from, to, got)
			}
		}
	}
}

func TestAssistantPlanCallForPutsBatchIDs(t *testing.T) {
	plan := AssistantPlan{
		ActionID: "updateVideoTags.add",
		Call: AssistantPlanCall{
			Body: map[string]any{"tagIds": []int64{7}}, VideoField: "videoIds",
		},
	}
	call := plan.CallFor([]int64{3, 4})
	if call.ActionID != "updateVideoTags.add" || !slices.Equal(call.Body["videoIds"].([]int64), []int64{3, 4}) {
		t.Errorf("call = %+v", call)
	}
	if _, ok := plan.Call.Body["videoIds"]; ok {
		t.Error("CallFor changed the plan's own body")
	}

	one := AssistantPlan{ActionID: "setVideoDisplayName", Call: AssistantPlanCall{VideoPathParam: "id"}}
	if got := one.CallFor([]int64{9}).Path["id"]; got != "9" {
		t.Errorf("path id = %q, want 9", got)
	}
}
