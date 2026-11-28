package domain

import (
	"context"
	"fmt"
)

// VocabularyLinks reaches the paired nodes a change of a tag, person or organization by name is written to, as
// the request making the change sees them (docs/design-docs/peer-library.md#vocabulary-by-name).
type VocabularyLinks interface {
	// Links are the links in slot order; none for a request of a peer or a node that reads none.
	Links(ctx context.Context) ([]PeerLinkState, error)
	// Entries are the whole list of kind on the link in slot, in that link's id space; ErrNotFound when no link
	// holds slot.
	Entries(ctx context.Context, slot int64, kind VocabularyKind) ([]LinkVocabularyEntry, error)
	// Apply applies change by name on link and answers what the link reported.
	Apply(ctx context.Context, link PeerLink, change VocabularyChange) (VocabularyOutcome, error)
}

// LinkVocabularyEntry is an entry of a link's whole list of a kind; only a tag has synonyms, and a provisional
// person has no name.
type LinkVocabularyEntry struct {
	ID       int64
	Name     string
	Synonyms []string
}

// VocabularyLinkError is a link that is not connected, could not be read, or refused or failed to apply a
// change; Link is empty when no link of the slot is known.
type VocabularyLinkError struct {
	Link string
	// Change is the change the link refused or failed to apply, nil when reading the link failed.
	Change *VocabularyChange
	Err    error
}

func (e *VocabularyLinkError) Error() string { return fmt.Sprintf("link %q: %v", e.Link, e.Err) }

func (e *VocabularyLinkError) Unwrap() error { return e.Err }

// VocabularyNameTakenError refuses a rename or promote before any node changes, because an entry of Kind other
// than the renamed one holds NewName: on this node when Link is empty, otherwise on the link named Link. Holder
// is that entry's canonical name.
type VocabularyNameTakenError struct {
	Link    string
	Kind    VocabularyKind
	NewName string
	Holder  string
}

func (e *VocabularyNameTakenError) Error() string {
	return fmt.Sprintf("%s name %q is held by %q on %q", e.Kind, e.NewName, e.Holder, e.Link)
}

// VocabularyStep is a read of this node a change by name failed at, before any node changed.
type VocabularyStep string

const (
	VocabularyReadLinks       VocabularyStep = "read the pairing"
	VocabularyLoadTag         VocabularyStep = "load the tag"
	VocabularyLookUpTagName   VocabularyStep = "look up the tag name"
	VocabularyReadCreditNames VocabularyStep = "read the names of people and organizations"
)

// VocabularyStepError is a failure of this node's own read at Step, so the caller can say which read failed.
type VocabularyStepError struct {
	Step VocabularyStep
	Err  error
}

func (e *VocabularyStepError) Error() string { return fmt.Sprintf("%s: %v", e.Step, e.Err) }

func (e *VocabularyStepError) Unwrap() error { return e.Err }
