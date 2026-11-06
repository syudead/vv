package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// People and organizations have stable IDs, an ordered list of names that may repeat across entities, and external
// IDs; only organizations have parents (docs/design-docs/credit-names.md).

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
	CreditNameMaxLength = 200
	// CreditNamesMaxCount counts every name, the display name included.
	CreditNamesMaxCount     = 50
	CreditLanguageMaxLength = 35
)

// CreditName is one spelling of a person's or organization's name; Language is an optional BCP 47 syntax tag.
type CreditName struct {
	Name     string
	Language string
}

type Credit struct {
	Kind CreditKind
	ID   int64
	// Name is the display name, the first of Names, and empty exactly when the credit is provisional.
	Name string
	// Provisional is true exactly when a person has no names; organizations always have one.
	Provisional bool
	// Names are in the owner's order, the display name first.
	Names []CreditName
	// ExternalIDs are ordered by (source, value).
	ExternalIDs []ExternalID
	// Parent is nil for people and root organizations.
	Parent *CreditRef
	// Profile is empty for organizations.
	Profile PersonProfile
	// OrganizationProfile is empty for people.
	OrganizationProfile OrganizationProfile
	Favorite            bool
	// ImageRevision is zero unless the credit has an image: a person's face or an organization's logo.
	ImageRevision int64
	// VideoCount counts visible video user-data keys linked in any role.
	VideoCount int
	CreatedAt  time.Time
}

// ParentID and OrganizationProfile are used only for organizations and Profile only for people.
type CreditInput struct {
	// Names are in the owner's order; an empty list makes a provisional person.
	Names               []CreditName
	ExternalIDs         []ExternalID
	ParentID            *int64
	Profile             PersonProfile
	OrganizationProfile OrganizationProfile
}

// CreditPatch preserves nil fields and replaces whole lists; pointers to nil lists clear them, while ParentSet controls whether ParentID changes (nil removes the parent).
type CreditPatch struct {
	// Name respells the display name, keeping its language and the other names; on a provisional person it is the
	// only name. It applies after Names when both are set.
	Name        *string
	Names       *[]CreditName
	ExternalIDs *[]ExternalID
	ParentSet   bool
	ParentID    *int64
	// Profile is nil when no profile property changes; people only.
	Profile *PersonProfilePatch
	// OrganizationProfile is nil when no profile property changes; organizations only.
	OrganizationProfile *OrganizationProfilePatch
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
	// Q is folded and trimmed by storage, then matched as a substring of every name.
	Q string
	// Sort defaults to TagSortName; ValidCreditSort lists the orders each kind supports.
	Sort TagSort
	// Cursor must match the credit kind and sort order or yields ErrInvalidCursor.
	Cursor string
	// Limit is 1–MaxTagPageLimit; zero returns all results and ignores Cursor.
	Limit int
	// FavoriteOnly keeps only favorites.
	FavoriteOnly bool
	// Provisional, when set, keeps only provisional (true) or named (false) people; storage rejects it for organizations.
	Provisional *bool
}

type CreditPage struct {
	Items      []Credit
	Total      int
	TotalAll   int
	NextCursor string
}

// FaceCandidateConfirmation is the person a confirmed face pair merged into, with the face_merge entry that undoes
// the merge, or 0 when the merged person had a name and the merge cannot be undone.
type FaceCandidateConfirmation struct {
	Person         Credit
	UndoActivityID int64
}

type CreditMergeOutcome struct {
	Credit Credit
	// NotFoundIDs deduplicates missing sources in input order and is empty when none are missing.
	NotFoundIDs []int64
}

var ErrCreditParentCycle = errors.New("the parent organization would form a cycle")

var ErrMergeSameCredit = errors.New("cannot merge an entity into itself")

// ErrPersonNotProvisional reports an automatic merge whose source has been named since the merge was decided.
var ErrPersonNotProvisional = errors.New("the person is not provisional")

// ErrFaceMergeRefused wraps a refusal of an automatic merge that trying again cannot change, such as a target whose
// merged names would exceed their limits.
var ErrFaceMergeRefused = errors.New("the automatic merge was refused")

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

// DisplayName is the first name, or "" for an empty list.
func (in CreditInput) DisplayName() string {
	if len(in.Names) == 0 {
		return ""
	}
	return in.Names[0].Name
}

// NormalizeCredit trims without case folding, drops fully blank external IDs, and returns the first
// InvalidWorkInfoError in names/external-ID/profile order; storage validates parent existence and cycles.
func NormalizeCredit(kind CreditKind, input CreditInput) (CreditInput, error) {
	return normalizeCredit(kind, input, true)
}

// PatchCredit applies patch to current and normalizes the result. A patch without Names does not count the list, so
// a stored list over the limit survives it, and an error in a respelled display name is reported at name.
func PatchCredit(current Credit, patch CreditPatch) (CreditInput, error) {
	merged := CreditInput{
		Names: current.Names, ExternalIDs: current.ExternalIDs, Profile: current.Profile,
		OrganizationProfile: current.OrganizationProfile,
	}
	if current.Parent != nil {
		parent := current.Parent.ID
		merged.ParentID = &parent
	}
	if patch.Names != nil {
		merged.Names = *patch.Names
	}
	if patch.Name != nil {
		merged.Names = respellDisplayName(merged.Names, *patch.Name)
	}
	if patch.ExternalIDs != nil {
		merged.ExternalIDs = *patch.ExternalIDs
	}
	if patch.ParentSet {
		merged.ParentID = patch.ParentID
	}
	if patch.Profile != nil {
		merged.Profile = patch.Profile.Apply(current.Profile)
	}
	if patch.OrganizationProfile != nil {
		merged.OrganizationProfile = patch.OrganizationProfile.Apply(current.OrganizationProfile)
	}
	normalized, err := normalizeCredit(current.Kind, merged, patch.Names != nil)
	var invalid *InvalidWorkInfoError
	if patch.Names == nil && patch.Name != nil && errors.As(err, &invalid) && strings.HasPrefix(invalid.Field, "names[") {
		// The stored list was valid, so the respelled display name is the entry at fault.
		return CreditInput{}, &InvalidWorkInfoError{Field: "name", Problem: invalid.Problem, Limit: invalid.Limit}
	}
	return normalized, err
}

func respellDisplayName(names []CreditName, name string) []CreditName {
	if len(names) == 0 {
		return []CreditName{{Name: name}}
	}
	out := slices.Clone(names)
	out[0].Name = name
	return out
}

// NormalizeCreditName normalizes one display name, which may not be blank, reporting errors at name.
func NormalizeCreditName(name string) (string, error) {
	normalized, err := normalizeWorkInfoText("name", name, CreditNameMaxLength, false)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", invalidWorkInfo("name", WorkInfoEmpty, 0)
	}
	return normalized, nil
}

func normalizeCredit(kind CreditKind, input CreditInput, countNames bool) (CreditInput, error) {
	names, err := normalizeCreditNames(kind, input.Names, countNames)
	if err != nil {
		return CreditInput{}, err
	}
	externalIDs, err := normalizeExternalIDs("externalIds", input.ExternalIDs)
	if err != nil {
		return CreditInput{}, err
	}
	today := time.Now()
	profile, err := NormalizePersonProfile(input.Profile, today)
	if err != nil {
		return CreditInput{}, err
	}
	organizationProfile, err := NormalizeOrganizationProfile(input.OrganizationProfile, today)
	if err != nil {
		return CreditInput{}, err
	}
	return CreditInput{
		Names: names, ExternalIDs: externalIDs, ParentID: input.ParentID, Profile: profile,
		OrganizationProfile: organizationProfile,
	}, nil
}

// normalizeCreditNames checks each entry in order, then the count when countNames is set, then the kind's empty-list
// rule: an organization needs a name, and a person without one is provisional.
func normalizeCreditNames(kind CreditKind, names []CreditName, countNames bool) ([]CreditName, error) {
	normalized := make([]CreditName, 0, len(names))
	seen := make(map[string]bool, len(names))
	for i, raw := range names {
		entry, err := normalizeCreditNameEntry(fmt.Sprintf("names[%d]", i), raw)
		if err != nil {
			return nil, err
		}
		if seen[entry.Name] {
			return nil, invalidWorkInfo(fmt.Sprintf("names[%d].name", i), WorkInfoDuplicate, 0)
		}
		seen[entry.Name] = true
		normalized = append(normalized, entry)
	}
	if countNames && len(normalized) > CreditNamesMaxCount {
		return nil, invalidWorkInfo("names", WorkInfoTooMany, CreditNamesMaxCount)
	}
	if len(normalized) == 0 && kind != CreditPerson {
		return nil, invalidWorkInfo("names", WorkInfoEmpty, 0)
	}
	return normalized, nil
}

func normalizeCreditNameEntry(field string, raw CreditName) (CreditName, error) {
	name, err := normalizeWorkInfoText(field+".name", raw.Name, CreditNameMaxLength, false)
	if err != nil {
		return CreditName{}, err
	}
	if name == "" {
		return CreditName{}, invalidWorkInfo(field+".name", WorkInfoEmpty, 0)
	}
	language, err := normalizeWorkInfoText(field+".language", raw.Language, CreditLanguageMaxLength, false)
	if err != nil {
		return CreditName{}, err
	}
	if language != "" && !isLanguageTagSyntax(language) {
		return CreditName{}, invalidWorkInfo(field+".language", WorkInfoInvalidLanguage, 0)
	}
	return CreditName{Name: name, Language: language}, nil
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

// CompareAliasOrder is the order the screens listed aliases in before names had positions; migration 98 numbers the
// stored names by it through the vv_alias_order collation.
func CompareAliasOrder(a, b string) int {
	if order := CompareNatural(a, b); order != 0 {
		return order
	}
	return strings.Compare(a, b)
}

func SortExternalIDs(ids []ExternalID) {
	slices.SortFunc(ids, func(a, b ExternalID) int {
		if order := strings.Compare(a.Source, b.Source); order != 0 {
			return order
		}
		return strings.Compare(a.Value, b.Value)
	})
}
