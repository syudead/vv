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
		if got := TagBatchApplies(TagBatchRequest{Action: tc.action}, TagBatchTarget{Tentative: tc.tentative}); got != tc.want {
			t.Errorf("TagBatchApplies(%q, %v) = %v, want %v", tc.action, tc.tentative, got, tc.want)
		}
	}
}

func TestTagBatchAppliesToKinds(t *testing.T) {
	setGenre := TagBatchRequest{Action: TagBatchSetKind, KindID: 1}
	clearKind := TagBatchRequest{Action: TagBatchClearKind}
	cases := []struct {
		req    TagBatchRequest
		target TagBatchTarget
		want   bool
	}{
		{setGenre, TagBatchTarget{}, true},
		{setGenre, TagBatchTarget{Tentative: true}, true},
		{setGenre, TagBatchTarget{KindID: 2}, true},
		{setGenre, TagBatchTarget{KindID: 1}, false},
		{setGenre, TagBatchTarget{KindID: 1, Tentative: true}, false},
		{clearKind, TagBatchTarget{KindID: 1}, true},
		{clearKind, TagBatchTarget{KindID: 1, Tentative: true}, true},
		{clearKind, TagBatchTarget{}, false},
	}
	for _, tc := range cases {
		if got := TagBatchApplies(tc.req, tc.target); got != tc.want {
			t.Errorf("TagBatchApplies(%+v, %+v) = %v, want %v", tc.req, tc.target, got, tc.want)
		}
	}
}

func TestTagKindFilterMatches(t *testing.T) {
	genre := &TagKindRef{ID: 1, Name: "Genre"}
	work := &TagKindRef{ID: 2, Name: "Work"}
	cases := []struct {
		filter TagKindFilter
		kind   *TagKindRef
		want   bool
	}{
		{TagKindFilter{}, nil, true},
		{TagKindFilter{}, genre, true},
		{TagKindFilter{ID: 1}, genre, true},
		{TagKindFilter{ID: 1}, work, false},
		{TagKindFilter{ID: 1}, nil, false},
		{TagKindFilter{None: true}, nil, true},
		{TagKindFilter{None: true}, genre, false},
	}
	for _, tc := range cases {
		if got := tc.filter.Matches(tc.kind); got != tc.want {
			t.Errorf("%+v.Matches(%+v) = %v, want %v", tc.filter, tc.kind, got, tc.want)
		}
	}
}

func TestTagImpactApplies(t *testing.T) {
	for _, tentative := range []bool{true, false} {
		if got, want := TagImpactApplies(TagImpactReject, tentative), TagBatchApplies(TagBatchRequest{Action: TagBatchReject}, TagBatchTarget{Tentative: tentative}); got != want {
			t.Errorf("reject tentative=%v: %v, want %v", tentative, got, want)
		}
		if got, want := TagImpactApplies(TagImpactDelete, tentative), TagBatchApplies(TagBatchRequest{Action: TagBatchDelete}, TagBatchTarget{Tentative: tentative}); got != want {
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
