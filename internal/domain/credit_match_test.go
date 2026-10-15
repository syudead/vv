package domain

import (
	"reflect"
	"testing"
)

func TestMatchEntriesKeepOrderNamesAndExternalIDs(t *testing.T) {
	ids := []ExternalID{{Source: "fanza", Value: "a-1"}}
	people := PersonMatchEntries([]FetchedPerson{
		{Role: PersonRole("cast"), Name: "Aoi", ExternalIDs: ids},
		{Role: PersonRole("director"), Name: "Mika"},
	})
	if want := []CreditMatchEntry{{Name: "Aoi", ExternalIDs: ids}, {Name: "Mika"}}; !reflect.DeepEqual(people, want) {
		t.Errorf("PersonMatchEntries = %+v, want %+v", people, want)
	}
	organizations := OrganizationMatchEntries([]FetchedOrganization{{Role: OrganizationRole("maker"), Name: "Studio A", ExternalIDs: ids}})
	if want := []CreditMatchEntry{{Name: "Studio A", ExternalIDs: ids}}; !reflect.DeepEqual(organizations, want) {
		t.Errorf("OrganizationMatchEntries = %+v, want %+v", organizations, want)
	}
}
