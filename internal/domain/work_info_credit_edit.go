package domain

import (
	"fmt"
	"slices"
)

// CreditEditAction is how a bulk edit changes one role's credits on every selected video.
type CreditEditAction string

const (
	CreditEditAdd    CreditEditAction = "add"
	CreditEditRemove CreditEditAction = "remove"
	// CreditEditReplace sets the role to exactly IDs; an empty list clears the role.
	CreditEditReplace CreditEditAction = "replace"
)

func (a CreditEditAction) Valid() bool {
	switch a {
	case CreditEditAdd, CreditEditRemove, CreditEditReplace:
		return true
	}
	return false
}

// CreditEdit changes one role; Role is a PersonRole for people and an OrganizationRole for organizations.
type CreditEdit struct {
	Kind   CreditKind
	Role   string
	Action CreditEditAction
	IDs    []int64
}

// CreditEditsMaxCount allows one edit per person and organization role.
var CreditEditsMaxCount = len(PersonRoles) + len(OrganizationRoles)

// ValidateCreditEdits checks the request shape before any video is read; errors name edits[i] fields.
func ValidateCreditEdits(edits []CreditEdit) error {
	if len(edits) == 0 {
		return invalidWorkInfo("edits", WorkInfoEmpty, 0)
	}
	if len(edits) > CreditEditsMaxCount {
		return invalidWorkInfo("edits", WorkInfoTooMany, CreditEditsMaxCount)
	}
	seenRoles := make(map[string]bool, len(edits))
	for i, edit := range edits {
		field := fmt.Sprintf("edits[%d]", i)
		limit, ok := creditRoleLimit(edit.Kind, edit.Role)
		if !ok {
			return invalidWorkInfo(field+".role", WorkInfoOutOfRange, 0)
		}
		if !edit.Action.Valid() {
			return invalidWorkInfo(field+".action", WorkInfoOutOfRange, 0)
		}
		roleKey := string(edit.Kind) + ":" + edit.Role
		if seenRoles[roleKey] {
			return invalidWorkInfo(field+".role", WorkInfoDuplicate, 0)
		}
		seenRoles[roleKey] = true
		if len(edit.IDs) == 0 && edit.Action != CreditEditReplace {
			return invalidWorkInfo(field+".ids", WorkInfoEmpty, 0)
		}
		if len(edit.IDs) > limit {
			return invalidWorkInfo(field+".ids", WorkInfoTooMany, limit)
		}
		seenIDs := make(map[int64]bool, len(edit.IDs))
		for j, id := range edit.IDs {
			if seenIDs[id] {
				return invalidWorkInfo(fmt.Sprintf("%s.ids[%d]", field, j), WorkInfoDuplicate, 0)
			}
			seenIDs[id] = true
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
// fields as they are. Added credits follow the role's existing ones; a role over its limit fails at edits[i].ids.
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
			return WorkInfo{}, invalidWorkInfo(fmt.Sprintf("edits[%d].ids", i), WorkInfoTooMany, limit)
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
	switch edit.Action {
	case CreditEditAdd:
		out := slices.Clone(current)
		for _, id := range edit.IDs {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
		return out
	case CreditEditRemove:
		return slices.DeleteFunc(slices.Clone(current), func(id int64) bool { return slices.Contains(edit.IDs, id) })
	default:
		return slices.Clone(edit.IDs)
	}
}
