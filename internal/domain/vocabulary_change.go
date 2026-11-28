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

// VocabularyKindOf is the vocabulary kind of a person or organization.
func VocabularyKindOf(kind CreditKind) VocabularyKind {
	if kind == CreditPerson {
		return VocabularyPerson
	}
	return VocabularyOrganization
}

// DisplayNameOf is the first of normalized names, the name the paired nodes join an entry by, or "" for a
// provisional person's empty list.
func DisplayNameOf(names []CreditName) string {
	if len(names) == 0 {
		return ""
	}
	return names[0].Name
}

type VocabularyAction string

const (
	VocabularyRename VocabularyAction = "rename"
	VocabularyMerge  VocabularyAction = "merge"
	VocabularyDelete VocabularyAction = "delete"
	// VocabularyUnname empties a person's names, making it provisional; an organization always has a name.
	VocabularyUnname VocabularyAction = "unname"
	// VocabularyPromote makes NewName a tag's canonical name: a synonym NewName of the tag named Name swaps with
	// Name, and a tag without that synonym is renamed.
	VocabularyPromote VocabularyAction = "promote"
)

// VocabularyChange is a rename, merge or deletion of a tag, person or organization, the promote of a tag's synonym,
// or the unnaming of a person, by
// canonical name, which the paired nodes apply to the entity of that name; synonyms, other names and the rest stay
// with each node, except that an unnamed person keeps no name.
type VocabularyChange struct {
	Kind   VocabularyKind
	Action VocabularyAction
	// Name is the current name of a rename, promote, deletion or unnaming.
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
	case VocabularyRename, VocabularyPromote:
		return (c.Action == VocabularyRename || c.Kind == VocabularyTag) &&
			c.Name != "" && c.NewName != "" && c.Into == "" && len(c.Sources) == 0
	case VocabularyDelete, VocabularyUnname:
		return (c.Action == VocabularyDelete || c.Kind == VocabularyPerson) &&
			c.Name != "" && c.NewName == "" && c.Into == "" && len(c.Sources) == 0
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
