package domain

import (
	"slices"
	"testing"
)

func TestAssistantNamesNear(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"selfie", "selfy", true},
		{"cat", "car", false},
		{"Cats", "ｃａｔｓ", true},
		{"cats", "maps", false},
	}
	for _, c := range cases {
		if got := AssistantNamesNear(c.a, c.b); got != c.want {
			t.Errorf("AssistantNamesNear(%q, %q) = %t, want %t", c.a, c.b, got, c.want)
		}
	}
}

func TestAssistantWorkKeyFoldsATrailingPartMarker(t *testing.T) {
	if a, b := AssistantWorkKey("Title 1"), AssistantWorkKey("title part 2"); a != b || a == "" {
		t.Errorf("work keys = %q and %q, want one non-empty key", a, b)
	}
	if a, b := AssistantWorkKey("Title 1"), AssistantWorkKey("Other 1"); a == b {
		t.Errorf("different titles share the key %q", a)
	}
}

func TestAssistantWorkSetsGroupPartsAndCodesButNotCopies(t *testing.T) {
	sets := AssistantWorkSets([]AssistantWorkVideo{
		{Key: "a1", Title: "Trip 1"}, {Key: "a2", Title: "Trip part 2"},
		{Key: "c1", Title: "Alpha", CodeKey: "abc-001"}, {Key: "c2", Title: "Beta", CodeKey: "abc-001"},
		{Key: "x1", Title: "Copy"}, {Key: "x2", Title: "Copy"},
		{Key: "lone", Title: "Lone 1"},
	})
	var keys []string
	for _, set := range sets {
		keys = append(keys, set.SetKey())
	}
	if want := []string{"a1,a2", "c1,c2"}; !slices.Equal(keys, want) {
		t.Errorf("sets = %v, want %v", keys, want)
	}
}

func TestAssistantReviewPlanMergesIntoTheMoreUsedMember(t *testing.T) {
	for _, c := range []struct {
		kind   AssistantReviewKind
		action string
	}{{AssistantReviewTags, "mergeTag"}, {AssistantReviewPeople, "mergePeople"}} {
		plan, err := AssistantReviewPlan(c.kind, []AssistantReviewMember{
			{ID: 1, Name: "selfy", Count: 2}, {ID: 2, Name: "selfie", Count: 9},
		})
		if err != nil {
			t.Fatal(err)
		}
		if plan.ActionID != c.action || plan.Call.Path["id"] != "2" ||
			!slices.Equal(plan.Call.Body["sourceIds"].([]int64), []int64{1}) || plan.Targets.Labels[0].Name != "selfie" {
			t.Errorf("%s plan = %+v, want %s into 2 from 1", c.kind, plan, c.action)
		}
	}
	tie := AssistantReviewDestination([]AssistantReviewMember{{ID: 7, Count: 3}, {ID: 4, Count: 3}})
	if tie != 1 {
		t.Errorf("tie destination = %d, want the older id", tie)
	}
}

func TestAssistantReviewPlanGroupsWorksByTheCommonTitle(t *testing.T) {
	plan, err := AssistantReviewPlan(AssistantReviewWorks, []AssistantReviewMember{
		{Kind: AssistantLabelVideo, Key: "a1", Name: "Trip 1"}, {Kind: AssistantLabelVideo, Key: "a2", Name: "Trip part 2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ActionID != "createGroup" || plan.Call.Body["name"] != "Trip" || plan.Call.VideoField != "videoIds" ||
		!slices.Equal(plan.Targets.VideoKeys, []string{"a1", "a2"}) {
		t.Errorf("plan = %+v", plan)
	}
}
