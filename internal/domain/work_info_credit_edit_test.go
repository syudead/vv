package domain

import (
	"errors"
	"reflect"
	"testing"
)

func personEdit(role PersonRole, add []int64, remove ...int64) CreditEdit {
	return CreditEdit{Kind: CreditPerson, Role: string(role), Add: add, Remove: remove}
}

func TestApplyCreditEditsChangesOnlyTheEditedRole(t *testing.T) {
	info := WorkInfo{
		Code: "ABC-1",
		People: []WorkPerson{
			{Role: PersonPerformer, Person: CreditRef{ID: 1}},
			{Role: PersonPerformer, Person: CreditRef{ID: 2}},
			{Role: PersonDirector, Person: CreditRef{ID: 9}},
		},
		Organizations: []WorkOrganization{{Role: OrganizationMaker, Organization: CreditRef{ID: 5}}},
	}
	cases := []struct {
		name          string
		edits         []CreditEdit
		people        []WorkPerson
		organizations []WorkOrganization
	}{
		{
			name:  "add appends only missing IDs after the existing ones",
			edits: []CreditEdit{personEdit(PersonPerformer, []int64{2, 3})},
			people: []WorkPerson{
				{Role: PersonPerformer, Person: CreditRef{ID: 1}}, {Role: PersonPerformer, Person: CreditRef{ID: 2}},
				{Role: PersonPerformer, Person: CreditRef{ID: 3}}, {Role: PersonDirector, Person: CreditRef{ID: 9}},
			},
			organizations: info.Organizations,
		},
		{
			name:  "remove runs before add and keeps the same person in another role",
			edits: []CreditEdit{personEdit(PersonPerformer, []int64{4}, 1, 9)},
			people: []WorkPerson{
				{Role: PersonPerformer, Person: CreditRef{ID: 2}}, {Role: PersonPerformer, Person: CreditRef{ID: 4}},
				{Role: PersonDirector, Person: CreditRef{ID: 9}},
			},
			organizations: info.Organizations,
		},
		{
			name: "a new role takes its read position",
			edits: []CreditEdit{
				personEdit(PersonPerformer, nil, 1, 2),
				{Kind: CreditOrganization, Role: string(OrganizationLabel), Add: []int64{6}},
			},
			people: []WorkPerson{{Role: PersonDirector, Person: CreditRef{ID: 9}}},
			organizations: []WorkOrganization{
				{Role: OrganizationMaker, Organization: CreditRef{ID: 5}},
				{Role: OrganizationLabel, Organization: CreditRef{ID: 6}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyCreditEdits(info, tc.edits)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.People, tc.people) || !reflect.DeepEqual(got.Organizations, tc.organizations) {
				t.Errorf("people = %+v, organizations = %+v; want %+v, %+v", got.People, got.Organizations, tc.people, tc.organizations)
			}
			if got.Code != info.Code {
				t.Errorf("code = %q, want %q", got.Code, info.Code)
			}
		})
	}
}

func TestApplyCreditEditsRefusesARoleOverItsLimit(t *testing.T) {
	limit := WorkInfoLimits.OrganizationsPerRoleMaxCount
	info := WorkInfo{}
	for i := range limit {
		info.Organizations = append(info.Organizations, WorkOrganization{Role: OrganizationMaker, Organization: CreditRef{ID: int64(i + 1)}})
	}
	edits := []CreditEdit{
		personEdit(PersonPerformer, []int64{1}),
		{Kind: CreditOrganization, Role: string(OrganizationMaker), Add: []int64{1000}},
	}
	_, err := ApplyCreditEdits(info, edits)
	var invalid *InvalidWorkInfoError
	if !errors.As(err, &invalid) || invalid.Field != "credits[1].add" || invalid.Problem != WorkInfoTooMany || invalid.Limit != limit {
		t.Fatalf("err = %v, want too many at credits[1].add with limit %d", err, limit)
	}
}

func TestValidateCreditEditsNamesTheFailingField(t *testing.T) {
	cases := []struct {
		name    string
		edits   []CreditEdit
		field   string
		problem WorkInfoProblem
	}{
		{"role of the other kind", []CreditEdit{{Kind: CreditPerson, Role: "maker", Add: []int64{1}}},
			"credits[0].role", WorkInfoOutOfRange},
		{"same role twice", []CreditEdit{personEdit(PersonPerformer, []int64{1}), personEdit(PersonPerformer, nil, 2)},
			"credits[1].role", WorkInfoDuplicate},
		{"no IDs", []CreditEdit{personEdit(PersonPerformer, nil)}, "credits[0]", WorkInfoEmpty},
		{"duplicate ID in add", []CreditEdit{personEdit(PersonPerformer, []int64{1, 1})}, "credits[0].add[1]", WorkInfoDuplicate},
		{"ID in add and remove", []CreditEdit{personEdit(PersonPerformer, []int64{1}, 2, 1)}, "credits[0].remove[1]", WorkInfoDuplicate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var invalid *InvalidWorkInfoError
			err := ValidateCreditEdits(tc.edits)
			if !errors.As(err, &invalid) || invalid.Field != tc.field || invalid.Problem != tc.problem {
				t.Fatalf("err = %v, want problem %d at %s", err, tc.problem, tc.field)
			}
		})
	}
	if err := ValidateCreditEdits([]CreditEdit{personEdit(PersonPerformer, nil, 1)}); err != nil {
		t.Errorf("remove only: err = %v, want nil", err)
	}
}
