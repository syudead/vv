package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// People and organizations have stable IDs, duplicate names, aliases, and external IDs; only organizations have parents.

// CreditKind selects the persistence table.
type CreditKind string

const (
	CreditPerson       CreditKind = "person"
	CreditOrganization CreditKind = "organization"
)

func (k CreditKind) Valid() bool {
	return k == CreditPerson || k == CreditOrganization
}

const (
	// CreditNameMaxLength counts code points and matches the display-name limit.
	CreditNameMaxLength     = 200
	CreditAliasesMaxCount   = 50
	CreditLanguageMaxLength = 35
)

// Language is an optional BCP 47 syntax tag.
type Alias struct {
	Name     string
	Language string
}

type Credit struct {
	Kind CreditKind
	ID   int64
	Name string
	// Aliases use natural name order, breaking ties by the original string.
	Aliases []Alias
	// ExternalIDs are ordered by (source, value).
	ExternalIDs []ExternalID
	// Parent is nil for people and root organizations.
	Parent *CreditRef
	// Profile is empty for organizations.
	Profile PersonProfile
	// Favorite is false for organizations.
	Favorite bool
	// ImageRevision is zero unless the person has a face.
	ImageRevision int64
	// VideoCount counts visible video user-data keys linked in any role.
	VideoCount int
	CreatedAt  time.Time
}

// ParentID is used only for organizations and Profile only for people.
type CreditInput struct {
	Name        string
	Aliases     []Alias
	ExternalIDs []ExternalID
	ParentID    *int64
	Profile     PersonProfile
}

// CreditPatch preserves nil fields and replaces whole lists; pointers to nil lists clear them, while ParentSet controls whether ParentID changes (nil removes the parent).
type CreditPatch struct {
	Name        *string
	Aliases     *[]Alias
	ExternalIDs *[]ExternalID
	ParentSet   bool
	ParentID    *int64
	// Profile is nil when no profile property changes; people only.
	Profile *PersonProfilePatch
}

// People also sort by birth date; a partial date sorts as its first possible day and an unknown one comes last.
const (
	CreditSortAgeAsc  TagSort = "ageAsc"
	CreditSortAgeDesc TagSort = "ageDesc"
)

// ValidCreditSort reports whether kind can be listed in sort; the age sorts apply to people only.
func ValidCreditSort(kind CreditKind, sort TagSort) bool {
	if sort == CreditSortAgeAsc || sort == CreditSortAgeDesc {
		return kind == CreditPerson
	}
	return sort.Valid()
}

type CreditQuery struct {
	// Q is folded and trimmed by storage, then matched as a substring of canonical names and aliases.
	Q string
	// Sort defaults to TagSortName; ValidCreditSort lists the orders each kind supports.
	Sort TagSort
	// Cursor must match the credit kind and sort order or yields ErrInvalidCursor.
	Cursor string
	// Limit is 1–MaxTagPageLimit; zero returns all results and ignores Cursor.
	Limit int
	// FavoriteOnly keeps only favorite people; storage rejects it for organizations.
	FavoriteOnly bool
}

type CreditPage struct {
	Items      []Credit
	Total      int
	TotalAll   int
	NextCursor string
}

type CreditMergeOutcome struct {
	Credit Credit
	// NotFoundIDs deduplicates missing sources in input order and is empty when none are missing.
	NotFoundIDs []int64
}

var ErrCreditParentCycle = errors.New("the parent organization would form a cycle")

var ErrMergeSameCredit = errors.New("cannot merge an entity into itself")

// ErrExternalIDTaken wraps CreditExternalIDTakenError and maps to external_id_taken (409).
var ErrExternalIDTaken = errors.New("external ID is held by another entity")

// Field identifies the input path (e.g. externalIds[1]); HolderID identifies the current owner.
type CreditExternalIDTakenError struct {
	Kind     CreditKind
	Field    string
	HolderID int64
}

func (e *CreditExternalIDTakenError) Error() string {
	return fmt.Sprintf("%s is held by %s %d", e.Field, e.Kind, e.HolderID)
}

func (e *CreditExternalIDTakenError) Unwrap() error { return ErrExternalIDTaken }

// CreditNotFoundError records the input field path and matches ErrNotFound.
type CreditNotFoundError struct {
	Kind  CreditKind
	ID    int64
	Field string
}

func (e *CreditNotFoundError) Error() string {
	return fmt.Sprintf("%s %d named by %s not found", e.Kind, e.ID, e.Field)
}

func (e *CreditNotFoundError) Unwrap() error { return ErrNotFound }

// NormalizeCredit trims without case folding, drops blank aliases and fully blank external IDs, and returns the first InvalidWorkInfoError in name/alias/external-ID/profile order; storage validates parent existence and cycles.
func NormalizeCredit(input CreditInput) (CreditInput, error) {
	name, err := normalizeWorkInfoText("name", input.Name, CreditNameMaxLength, false)
	if err != nil {
		return CreditInput{}, err
	}
	if name == "" {
		return CreditInput{}, invalidWorkInfo("name", WorkInfoEmpty, 0)
	}
	aliases, err := normalizeAliases(name, input.Aliases)
	if err != nil {
		return CreditInput{}, err
	}
	externalIDs, err := normalizeExternalIDs("externalIds", input.ExternalIDs)
	if err != nil {
		return CreditInput{}, err
	}
	profile, err := NormalizePersonProfile(input.Profile, time.Now())
	if err != nil {
		return CreditInput{}, err
	}
	return CreditInput{Name: name, Aliases: aliases, ExternalIDs: externalIDs, ParentID: input.ParentID, Profile: profile}, nil
}

func normalizeAliases(name string, aliases []Alias) ([]Alias, error) {
	normalized := make([]Alias, 0, len(aliases))
	seen := map[string]bool{name: true}
	for i, raw := range aliases {
		field := fmt.Sprintf("aliases[%d]", i)
		alias, err := normalizeWorkInfoText(field+".name", raw.Name, CreditNameMaxLength, false)
		if err != nil {
			return nil, err
		}
		if alias == "" {
			// Drop nameless aliases without validating their language.
			continue
		}
		language, err := normalizeWorkInfoText(field+".language", raw.Language, CreditLanguageMaxLength, false)
		if err != nil {
			return nil, err
		}
		if language != "" && !isLanguageTagSyntax(language) {
			return nil, invalidWorkInfo(field+".language", WorkInfoInvalidLanguage, 0)
		}
		if seen[alias] {
			return nil, invalidWorkInfo(field+".name", WorkInfoDuplicate, 0)
		}
		seen[alias] = true
		normalized = append(normalized, Alias{Name: alias, Language: language})
	}
	if len(normalized) > CreditAliasesMaxCount {
		return nil, invalidWorkInfo("aliases", WorkInfoTooMany, CreditAliasesMaxCount)
	}
	return normalized, nil
}

// isLanguageTagSyntax checks BCP 47 syntax, not registration; callers reject overlong values first.
func isLanguageTagSyntax(s string) bool {
	if len(s) < 2 || len(s) > CreditLanguageMaxLength {
		return false
	}
	letters := 0
	for letters < len(s) && isASCIILetter(s[letters]) {
		letters++
	}
	if letters < 2 || letters > 3 || (letters < len(s) && s[letters] != '-') {
		return false
	}
	for i := letters; i < len(s); i++ {
		c := s[i]
		if !isASCIILetter(c) && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func SortAliases(aliases []Alias) {
	slices.SortStableFunc(aliases, func(a, b Alias) int {
		if order := CompareNatural(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.Name, b.Name)
	})
}

func SortExternalIDs(ids []ExternalID) {
	slices.SortFunc(ids, func(a, b ExternalID) int {
		if order := strings.Compare(a.Source, b.Source); order != 0 {
			return order
		}
		return strings.Compare(a.Value, b.Value)
	})
}
