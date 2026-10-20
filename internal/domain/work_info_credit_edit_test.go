package domain

import (
	"errors"
	"reflect"
	"testing"
)

func personEdit(role PersonRole, action CreditEditAction, ids ...int64) CreditEdit {
	return CreditEdit{Kind: CreditPerson, Role: string(role), Action: action, IDs: ids}
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
			edits: []CreditEdit{personEdit(PersonPerformer, CreditEditAdd, 2, 3)},
			people: []WorkPerson{
				{Role: PersonPerformer, Person: CreditRef{ID: 1}}, {Role: PersonPerformer, Person: CreditRef{ID: 2}},
				{Role: PersonPerformer, Person: CreditRef{ID: 3}}, {Role: PersonDirector, Person: CreditRef{ID: 9}},
			},
			organizations: info.Organizations,
		},
		{
			name:  "remove keeps the same person in another role",
			edits: []CreditEdit{personEdit(PersonPerformer, CreditEditRemove, 1, 9)},
			people: []WorkPerson{
				{Role: PersonPerformer, Person: CreditRef{ID: 2}}, {Role: PersonDirector, Person: CreditRef{ID: 9}},
			},
			organizations: info.Organizations,
		},
		{
			name: "replace with no IDs clears the role, and a new role takes its read position",
			edits: []CreditEdit{
				personEdit(PersonPerformer, CreditEditReplace),
				{Kind: CreditOrganization, Role: string(OrganizationLabel), Action: CreditEditReplace, IDs: []int64{6}},
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
		personEdit(PersonPerformer, CreditEditAdd, 1),
		{Kind: CreditOrganization, Role: string(OrganizationMaker), Action: CreditEditAdd, IDs: []int64{1000}},
	}
	_, err := ApplyCreditEdits(info, edits)
	var invalid *InvalidWorkInfoError
	if !errors.As(err, &invalid) || invalid.Field != "edits[1].ids" || invalid.Problem != WorkInfoTooMany || invalid.Limit != limit {
		t.Fatalf("err = %v, want too many at edits[1].ids with limit %d", err, limit)
	}
}

func TestValidateCreditEditsNamesTheFailingField(t *testing.T) {
	cases := []struct {
		name    string
		edits   []CreditEdit
		field   string
		problem WorkInfoProblem
	}{
		{"no edits", nil, "edits", WorkInfoEmpty},
		{"role of the other kind", []CreditEdit{{Kind: CreditPerson, Role: "maker", Action: CreditEditAdd, IDs: []int64{1}}},
			"edits[0].role", WorkInfoOutOfRange},
		{"unknown action", []CreditEdit{personEdit(PersonPerformer, "set", 1)}, "edits[0].action", WorkInfoOutOfRange},
		{"same role twice", []CreditEdit{personEdit(PersonPerformer, CreditEditAdd, 1), personEdit(PersonPerformer, CreditEditRemove, 2)},
			"edits[1].role", WorkInfoDuplicate},
		{"add without IDs", []CreditEdit{personEdit(PersonPerformer, CreditEditAdd)}, "edits[0].ids", WorkInfoEmpty},
		{"duplicate ID", []CreditEdit{personEdit(PersonPerformer, CreditEditAdd, 1, 1)}, "edits[0].ids[1]", WorkInfoDuplicate},
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
	if err := ValidateCreditEdits([]CreditEdit{personEdit(PersonPerformer, CreditEditReplace)}); err != nil {
		t.Errorf("replace without IDs: err = %v, want nil", err)
	}
}
