package domain

import "testing"

func TestTagBatchActionValid(t *testing.T) {
	for _, a := range []TagBatchAction{TagBatchConfirm, TagBatchReject, TagBatchDelete} {
		if !a.Valid() {
			t.Errorf("%q.Valid() = false", a)
		}
	}
	for _, a := range []TagBatchAction{"", "merge", "Confirm"} {
		if a.Valid() {
			t.Errorf("%q.Valid() = true", a)
		}
	}
}

func TestTagImpactActionValid(t *testing.T) {
	for _, a := range []TagImpactAction{TagImpactReject, TagImpactDelete, TagImpactMerge} {
		if !a.Valid() {
			t.Errorf("%q.Valid() = false", a)
		}
	}
	for _, a := range []TagImpactAction{"", "confirm", "Delete"} {
		if a.Valid() {
			t.Errorf("%q.Valid() = true", a)
		}
	}
}

func TestTagBatchApplies(t *testing.T) {
	cases := []struct {
		action    TagBatchAction
		tentative bool
		want      bool
	}{
		{TagBatchConfirm, true, true},
		{TagBatchConfirm, false, false},
		{TagBatchReject, true, true},
		{TagBatchReject, false, false},
		{TagBatchDelete, true, false},
		{TagBatchDelete, false, true},
		{"unknown", true, false},
		{"unknown", false, false},
	}
	for _, tc := range cases {
		if got := TagBatchApplies(tc.action, tc.tentative); got != tc.want {
			t.Errorf("TagBatchApplies(%q, %v) = %v, want %v", tc.action, tc.tentative, got, tc.want)
		}
	}
}

func TestTagImpactApplies(t *testing.T) {
	for _, tentative := range []bool{true, false} {
		if got, want := TagImpactApplies(TagImpactReject, tentative), TagBatchApplies(TagBatchReject, tentative); got != want {
			t.Errorf("reject tentative=%v: %v, want %v", tentative, got, want)
		}
		if got, want := TagImpactApplies(TagImpactDelete, tentative), TagBatchApplies(TagBatchDelete, tentative); got != want {
			t.Errorf("delete tentative=%v: %v, want %v", tentative, got, want)
		}
		if !TagImpactApplies(TagImpactMerge, tentative) {
			t.Errorf("merge tentative=%v: false, want true", tentative)
		}
		if TagImpactApplies("unknown", tentative) {
			t.Errorf("unknown tentative=%v: true, want false", tentative)
		}
	}
}
