package domain

import (
	"fmt"
	"slices"
)

// CreditEdit changes one role on every selected video: Remove drops its IDs, then Add appends the IDs a video
// lacks after its own credits of the role. Role is a PersonRole for people and an OrganizationRole for organizations.
type CreditEdit struct {
	Kind   CreditKind
	Role   string
	Add    []int64
	Remove []int64
}

// CreditEditsMaxCount allows one edit per person and organization role.
var CreditEditsMaxCount = len(PersonRoles) + len(OrganizationRoles)

// ValidateCreditEdits checks the request shape before any video is read; errors name credits[i] fields.
func ValidateCreditEdits(edits []CreditEdit) error {
	if len(edits) > CreditEditsMaxCount {
		return invalidWorkInfo("credits", WorkInfoTooMany, CreditEditsMaxCount)
	}
	seenRoles := make(map[string]bool, len(edits))
	for i, edit := range edits {
		field := fmt.Sprintf("credits[%d]", i)
		limit, ok := creditRoleLimit(edit.Kind, edit.Role)
		if !ok {
			return invalidWorkInfo(field+".role", WorkInfoOutOfRange, 0)
		}
		roleKey := string(edit.Kind) + ":" + edit.Role
		if seenRoles[roleKey] {
			return invalidWorkInfo(field+".role", WorkInfoDuplicate, 0)
		}
		seenRoles[roleKey] = true
		if len(edit.Add)+len(edit.Remove) == 0 {
			return invalidWorkInfo(field, WorkInfoEmpty, 0)
		}
		if len(edit.Add) > limit {
			return invalidWorkInfo(field+".add", WorkInfoTooMany, limit)
		}
		if err := checkCreditEditIDs(field, edit); err != nil {
			return err
		}
	}
	return nil
}

// Each ID appears once across add and remove; the later occurrence is reported.
func checkCreditEditIDs(field string, edit CreditEdit) error {
	seen := make(map[int64]bool, len(edit.Add)+len(edit.Remove))
	for _, list := range creditEditLists(edit) {
		for j, id := range list.ids {
			if seen[id] {
				return invalidWorkInfo(fmt.Sprintf("%s.%s[%d]", field, list.name, j), WorkInfoDuplicate, 0)
			}
			seen[id] = true
		}
	}
	return nil
}

// creditEditList names one ID list of a CreditEdit for field paths.
type creditEditList struct {
	name string
	ids  []int64
}

func creditEditLists(edit CreditEdit) []creditEditList {
	return []creditEditList{{"add", edit.Add}, {"remove", edit.Remove}}
}

// EachCreditEditID visits every ID of edits in request order with its field path, such as credits[0].add[1].
func EachCreditEditID(edits []CreditEdit, visit func(kind CreditKind, id int64, field string) error) error {
	for i, edit := range edits {
		for _, list := range creditEditLists(edit) {
			for j, id := range list.ids {
				if err := visit(edit.Kind, id, fmt.Sprintf("credits[%d].%s[%d]", i, list.name, j)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func creditRoleLimit(kind CreditKind, role string) (int, bool) {
	switch kind {
	case CreditPerson:
		return WorkInfoLimits.PeoplePerRoleMaxCount, PersonRole(role).Valid()
	case CreditOrganization:
		return WorkInfoLimits.OrganizationsPerRoleMaxCount, OrganizationRole(role).Valid()
	}
	return 0, false
}

// ApplyCreditEdits applies validated edits in order to one video's work information, leaving other roles and
// fields as they are. Added credits follow the role's existing ones; a role over its limit fails at credits[i].add.
func ApplyCreditEdits(info WorkInfo, edits []CreditEdit) (WorkInfo, error) {
	people := make(map[PersonRole][]int64, len(PersonRoles))
	for _, p := range info.People {
		people[p.Role] = append(people[p.Role], p.Person.ID)
	}
	organizations := make(map[OrganizationRole][]int64, len(OrganizationRoles))
	for _, o := range info.Organizations {
		organizations[o.Role] = append(organizations[o.Role], o.Organization.ID)
	}
	for i, edit := range edits {
		limit, _ := creditRoleLimit(edit.Kind, edit.Role)
		var ids []int64
		if edit.Kind == CreditPerson {
			ids = editCreditIDs(people[PersonRole(edit.Role)], edit)
			people[PersonRole(edit.Role)] = ids
		} else {
			ids = editCreditIDs(organizations[OrganizationRole(edit.Role)], edit)
			organizations[OrganizationRole(edit.Role)] = ids
		}
		if len(ids) > limit {
			return WorkInfo{}, invalidWorkInfo(fmt.Sprintf("credits[%d].add", i), WorkInfoTooMany, limit)
		}
	}
	out := info
	out.People = make([]WorkPerson, 0, len(info.People))
	for _, role := range PersonRoles {
		for _, id := range people[role] {
			out.People = append(out.People, WorkPerson{Role: role, Person: CreditRef{ID: id}})
		}
	}
	out.Organizations = make([]WorkOrganization, 0, len(info.Organizations))
	for _, role := range OrganizationRoles {
		for _, id := range organizations[role] {
			out.Organizations = append(out.Organizations, WorkOrganization{Role: role, Organization: CreditRef{ID: id}})
		}
	}
	return out, nil
}

func editCreditIDs(current []int64, edit CreditEdit) []int64 {
	out := slices.DeleteFunc(slices.Clone(current), func(id int64) bool { return slices.Contains(edit.Remove, id) })
	for _, id := range edit.Add {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
