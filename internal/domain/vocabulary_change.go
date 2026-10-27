package domain

import (
	"errors"
	"fmt"
)

// VocabularyKind is the entity a change by name applies to on the paired nodes.
type VocabularyKind string

const (
	VocabularyTag          VocabularyKind = "tag"
	VocabularyPerson       VocabularyKind = "person"
	VocabularyOrganization VocabularyKind = "organization"
)

// CreditKind is the credit kind of a person or organization; a tag has none.
func (k VocabularyKind) CreditKind() (CreditKind, bool) {
	switch k {
	case VocabularyPerson:
		return CreditPerson, true
	case VocabularyOrganization:
		return CreditOrganization, true
	}
	return "", false
}

type VocabularyAction string

const (
	VocabularyRename VocabularyAction = "rename"
	VocabularyMerge  VocabularyAction = "merge"
	VocabularyDelete VocabularyAction = "delete"
)

// VocabularyChange is a rename, merge or deletion of a tag, person or organization by canonical name, which the
// paired nodes apply to the entity of that name; synonyms, aliases and the rest stay with each node.
type VocabularyChange struct {
	Kind   VocabularyKind
	Action VocabularyAction
	// Name is the current name of a rename or deletion.
	Name    string
	NewName string
	// Into is the destination of a merge, created when a source exists and it does not.
	Into    string
	Sources []string
}

// Valid reports whether the change names a kind and an action with the fields that action needs.
func (c VocabularyChange) Valid() bool {
	if c.Kind != VocabularyTag && c.Kind != VocabularyPerson && c.Kind != VocabularyOrganization {
		return false
	}
	switch c.Action {
	case VocabularyRename:
		return c.Name != "" && c.NewName != "" && c.Into == "" && len(c.Sources) == 0
	case VocabularyDelete:
		return c.Name != "" && c.NewName == "" && c.Into == "" && len(c.Sources) == 0
	case VocabularyMerge:
		return c.Into != "" && c.Name == "" && c.NewName == "" && len(c.Sources) > 0 && len(c.Sources) <= MaxTagBatch
	}
	return false
}

// VocabularyOutcome reports whether a change by name changed anything, and the merge sources that were absent.
type VocabularyOutcome struct {
	Changed bool
	Skipped []string
}

// ErrCreditNameTaken is a rename of a person or organization to a name another entry of its kind holds, which the
// paired nodes refuse because the name joins them.
var ErrCreditNameTaken = errors.New("the name is already in use")

type CreditNameConflict struct {
	Kind CreditKind
	Name string
}

func (e *CreditNameConflict) Error() string {
	return fmt.Sprintf("%s name %q is already in use", e.Kind, e.Name)
}

func (e *CreditNameConflict) Unwrap() error { return ErrCreditNameTaken }
