package domain

import (
	"reflect"
	"testing"
)

func TestMergeVideoEditSummariesJoinsEntriesAndKeepsSharedDates(t *testing.T) {
	alice := CreditRef{ID: 1, Name: "Alice"}
	provisional := CreditRef{ID: ToRemoteID(9), Provisional: true}
	local := VideoEditSummary{
		Total: 2, Public: 1,
		People: []VideoEditPersonCount{{Role: PersonRoles[0], Person: alice, Count: 1}},
		Dates:  []VideoEditDateCount{{Kind: DateKinds[0], Count: 2, Date: "2020"}, {Kind: DateKinds[1], Count: 1, Date: "2021"}},
	}
	peer := VideoEditSummary{
		Total: 1, Public: 1,
		People: []VideoEditPersonCount{{Role: PersonRoles[0], Person: provisional, Count: 1}, {Role: PersonRoles[0], Person: alice, Count: 1}},
		Dates:  []VideoEditDateCount{{Kind: DateKinds[0], Count: 1, Date: "2020"}, {Kind: DateKinds[1], Count: 1, Date: "2022"}},
	}

	got := MergeVideoEditSummaries(local, peer)
	want := VideoEditSummary{
		Total: 3, Public: 2, Tags: []TagSummaryItem{}, Organizations: []VideoEditOrganizationCount{},
		People: []VideoEditPersonCount{
			{Role: PersonRoles[0], Person: alice, Count: 2}, {Role: PersonRoles[0], Person: provisional, Count: 1},
		},
		Dates: []VideoEditDateCount{{Kind: DateKinds[0], Count: 3, Date: "2020"}, {Kind: DateKinds[1], Count: 2}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %+v, want %+v", got, want)
	}
}
