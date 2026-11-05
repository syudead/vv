package domain

import (
	"reflect"
	"testing"
)

func TestMergeVideoEditSummariesJoinsEntriesAndKeepsSharedDates(t *testing.T) {
	alice := CreditRef{ID: 1, Name: "Alice"}
	provisional := CreditRef{ID: RemoteID(1, 9), Provisional: true}
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

// A name a link's summary gives under a remote id joins a later link's entry of that name; a provisional person
// and a name this node holds keep their ids, and the join Detached was made from is unchanged.
func TestHoldJoinsLaterLinksByName(t *testing.T) {
	base := VocabularyJoin{People: map[string]int64{"Bob": 3}}
	join := base.Detached()
	join.Hold(VideoEditSummary{
		People: []VideoEditPersonCount{
			{Person: CreditRef{ID: RemoteID(1, 5), Name: "Alice"}}, {Person: CreditRef{ID: RemoteID(1, 6), Provisional: true}},
			{Person: CreditRef{ID: 3, Name: "Bob"}},
		},
		Organizations: []VideoEditOrganizationCount{{Organization: CreditRef{ID: RemoteID(1, 7), Name: "Acme"}}},
	})

	later := join.InSlot(2)
	if got := later.Person(9, "Alice"); got != RemoteID(1, 5) {
		t.Errorf("Alice = %d, want slot 1's %d", got, RemoteID(1, 5))
	}
	if got := later.Person(6, ""); got != RemoteID(2, 6) {
		t.Errorf("provisional = %d, want its own %d", got, RemoteID(2, 6))
	}
	if got := later.Organization(8, "Acme"); got != RemoteID(1, 7) {
		t.Errorf("Acme = %d, want slot 1's %d", got, RemoteID(1, 7))
	}
	if _, held := base.People["Alice"]; held {
		t.Error("Hold changed the join Detached was made from")
	}
}
