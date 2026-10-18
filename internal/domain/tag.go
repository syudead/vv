package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// TagNameMaxLength counts code points for canonical names and synonyms, matching the search-name limit.
const TagNameMaxLength = 100

// NormalizeTagName wraps ErrInvalidTagName in InvalidTagNameError so the API can report the specific violation.
var ErrInvalidTagName = errors.New("invalid tag name")

type TagNameProblem int

const (
	TagNameEmpty TagNameProblem = iota + 1
	TagNameControlCharacters
	TagNameTooLong
)

type InvalidTagNameError struct {
	Problem TagNameProblem
}

func (e *InvalidTagNameError) Error() string {
	switch e.Problem {
	case TagNameEmpty:
		return "tag name is empty"
	case TagNameControlCharacters:
		return "tag name contains control characters"
	case TagNameTooLong:
		return fmt.Sprintf("tag name must be at most %d characters", TagNameMaxLength)
	default:
		return ErrInvalidTagName.Error()
	}
}

func (e *InvalidTagNameError) Unwrap() error { return ErrInvalidTagName }

func invalidTagName(problem TagNameProblem) error {
	return &InvalidTagNameError{Problem: problem}
}

var ErrTagNotFound = errors.New("tag not found")

// TagNameConflict identifies the existing canonical-name or synonym owner via errors.As.
var ErrTagNameTaken = errors.New("name is already in use")

// TagMergeRequired identifies the canonical-name owner whose merge consent is missing or invalid.
var ErrTagMergeRequired = errors.New("merge confirmation is required")

var ErrTagNotTentative = errors.New("tag is not tentative")

// TagRef.Name is always the canonical name.
type TagRef struct {
	ID        int64
	Name      string
	Tentative bool
	Kind      *TagKindRef
}

// VideoTag merges manual and folder-derived assignments into one entry with both flags set.
type VideoTag struct {
	TagRef
	Manual     bool
	FromFolder bool
	// Unconfirmed describes a tentative assignment, independently of TagRef.Tentative; folder-derived assignments are always confirmed (docs/design-docs/auto-tagging.md).
	Unconfirmed bool
}

type Tag struct {
	ID         int64
	Name       string
	Synonyms   []string
	VideoCount int
	Tentative  bool
	Kind       *TagKindRef
	// CreatedAt uses Unix seconds.
	CreatedAt time.Time
}

type TagSummaryItem struct {
	Tag TagRef
	// Count includes either manual or folder-derived assignments; values below Total indicate partial assignment.
	Count       int
	ManualCount int
}

type TagSummary struct {
	Total int
	// Items contains only assigned tags in natural-name order.
	Items []TagSummaryItem
}

type TagNameConflict struct {
	Tag TagRef
}

func (e *TagNameConflict) Error() string {
	return fmt.Sprintf("name belongs to tag %q", e.Tag.Name)
}

func (e *TagNameConflict) Unwrap() error { return ErrTagNameTaken }

type TagMergeRequired struct {
	Tag TagRef
}

func (e *TagMergeRequired) Error() string {
	return fmt.Sprintf("merging into tag %q requires confirmation", e.Tag.Name)
}

func (e *TagMergeRequired) Unwrap() error { return ErrTagMergeRequired }

// NormalizeTagName rejects controls before trimming Unicode whitespace so tabs/newlines cannot silently become another name; it then rejects empty or overlong names without normalization or case folding.
func NormalizeTagName(input string) (string, error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) {
			return "", invalidTagName(TagNameControlCharacters)
		}
	}
	trimmed := strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
	if trimmed == "" {
		return "", invalidTagName(TagNameEmpty)
	}
	if utf8.RuneCountInString(trimmed) > TagNameMaxLength {
		return "", invalidTagName(TagNameTooLong)
	}
	return trimmed, nil
}

// SortTagNames breaks natural-order ties by original spelling.
func SortTagNames(names []string) {
	slices.SortStableFunc(names, func(a, b string) int {
		if order := CompareNatural(a, b); order != 0 {
			return order
		}
		return strings.Compare(a, b)
	})
}

// SortTagRefs breaks natural-name ties by ID for deterministic order even with noncanonical caller input.
func SortTagRefs(refs []TagRef) {
	slices.SortStableFunc(refs, compareTagRefs)
}

func SortVideoTags(tags []VideoTag) {
	slices.SortStableFunc(tags, func(a, b VideoTag) int {
		return compareTagRefs(a.TagRef, b.TagRef)
	})
}

func compareTagRefs(a, b TagRef) int {
	if order := CompareNatural(a.Name, b.Name); order != 0 {
		return order
	}
	if order := strings.Compare(a.Name, b.Name); order != 0 {
		return order
	}
	return cmp.Compare(a.ID, b.ID)
}

func SortTagSummaryItems(items []TagSummaryItem) {
	slices.SortStableFunc(items, func(a, b TagSummaryItem) int {
		return compareTagRefs(a.Tag, b.Tag)
	})
}
