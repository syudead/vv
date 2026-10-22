package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"
)

// Smart folders store conditions as JSON in smart_folders.rule and resolve current matching videos rather than persisting membership.

const SmartRuleMaxQueryLength = 500

const SmartRuleMaxTags = 16

var (
	ErrInvalidSmartRule = errors.New("invalid smart rule")
	// ErrSmartFolderNotFound intentionally does not wrap ErrNotFound so missing parents retain the distinct smart_folder_not_found API error.
	ErrSmartFolderNotFound = errors.New("smart folder not found")
	ErrSmartFolderCycle    = errors.New("smart folder cycle")
)

// SmartRule omits zero IDs; omitted fields do not filter, and empty rules match all videos.
type SmartRule struct {
	Q        string `json:"q,omitempty"`
	Favorite bool   `json:"favorite,omitempty"`
	// Tags uses AND matching with unique positive IDs.
	Tags         []int64 `json:"tags,omitempty"`
	TagKind      int64   `json:"tagKind,omitempty"`
	Person       int64   `json:"person,omitempty"`
	Organization int64   `json:"organization,omitempty"`
}

// Pointer IDs distinguish omitted fields from explicit zero or negative values.
type smartRuleInput struct {
	Q            string   `json:"q"`
	Favorite     bool     `json:"favorite"`
	Tags         *[]int64 `json:"tags"`
	TagKind      *int64   `json:"tagKind"`
	Person       *int64   `json:"person"`
	Organization *int64   `json:"organization"`
}

// ParseSmartRule rejects nonobjects, unknown fields, wrong types, and out-of-range values with ErrInvalidSmartRule.
func ParseSmartRule(raw []byte) (SmartRule, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return SmartRule{}, fmt.Errorf("%w: the rule is not an object", ErrInvalidSmartRule)
	}
	// Unmarshal rejects trailing data, including unmatched closing delimiters.
	var members map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &members); err != nil {
		return SmartRule{}, fmt.Errorf("%w: %w", ErrInvalidSmartRule, err)
	}
	// Reject null explicitly because encoding/json otherwise treats it as omitted.
	for name, value := range members {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return SmartRule{}, fmt.Errorf("%w: %s is null", ErrInvalidSmartRule, name)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var input smartRuleInput
	if err := decoder.Decode(&input); err != nil {
		return SmartRule{}, fmt.Errorf("%w: %w", ErrInvalidSmartRule, err)
	}
	rule := SmartRule{Q: input.Q, Favorite: input.Favorite}
	if input.Tags != nil {
		if len(*input.Tags) == 0 {
			return SmartRule{}, fmt.Errorf("%w: tags is empty", ErrInvalidSmartRule)
		}
		rule.Tags = *input.Tags
	}
	for _, member := range []struct {
		name string
		in   *int64
		out  *int64
	}{
		{"tagKind", input.TagKind, &rule.TagKind},
		{"person", input.Person, &rule.Person},
		{"organization", input.Organization, &rule.Organization},
	} {
		if member.in == nil {
			continue
		}
		if *member.in <= 0 {
			return SmartRule{}, fmt.Errorf("%w: %s is not a positive id", ErrInvalidSmartRule, member.name)
		}
		*member.out = *member.in
	}
	if err := rule.Validate(); err != nil {
		return SmartRule{}, err
	}
	return rule, nil
}

func (r SmartRule) Validate() error {
	if utf8.RuneCountInString(r.Q) > SmartRuleMaxQueryLength {
		return fmt.Errorf("%w: q is longer than %d characters", ErrInvalidSmartRule, SmartRuleMaxQueryLength)
	}
	if err := validateSmartRuleTags(r.Tags); err != nil {
		return err
	}
	for _, id := range []int64{r.TagKind, r.Person, r.Organization} {
		if id < 0 {
			return fmt.Errorf("%w: an id is negative", ErrInvalidSmartRule)
		}
	}
	return nil
}

func validateSmartRuleTags(tags []int64) error {
	if tags == nil {
		return nil
	}
	if len(tags) == 0 || len(tags) > SmartRuleMaxTags {
		return fmt.Errorf("%w: tags holds %d ids, not 1 to %d", ErrInvalidSmartRule, len(tags), SmartRuleMaxTags)
	}
	seen := make(map[int64]bool, len(tags))
	for _, id := range tags {
		if id <= 0 {
			return fmt.Errorf("%w: tags holds a non-positive id", ErrInvalidSmartRule)
		}
		if seen[id] {
			return fmt.Errorf("%w: tags holds %d twice", ErrInvalidSmartRule, id)
		}
		seen[id] = true
	}
	return nil
}

func (r SmartRule) Encode() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("cannot encode the smart rule: %w", err)
	}
	return string(encoded), nil
}

// Query leaves sorting, cursor, and limit to the caller.
func (r SmartRule) Query() VideoQuery {
	return VideoQuery{
		Query:        r.Q,
		FavoriteOnly: r.Favorite,
		TagIDs:       slices.Clone(r.Tags),
		TagKindID:    r.TagKind,
		Person:       r.Person,
		Organization: r.Organization,
	}
}

type SmartRuleRefKind string

const (
	SmartRefTag          SmartRuleRefKind = "tag"
	SmartRefTagKind      SmartRuleRefKind = "tagKind"
	SmartRefPerson       SmartRuleRefKind = "person"
	SmartRefOrganization SmartRuleRefKind = "organization"
)

type SmartRuleRef struct {
	Kind SmartRuleRefKind
	ID   int64
}

// Refs orders tags first in rule order, then tag kind, person, and organization for validation and merge rewrites.
func (r SmartRule) Refs() []SmartRuleRef {
	refs := make([]SmartRuleRef, 0, len(r.Tags)+3)
	for _, id := range r.Tags {
		refs = append(refs, SmartRuleRef{Kind: SmartRefTag, ID: id})
	}
	for _, ref := range []SmartRuleRef{
		{Kind: SmartRefTagKind, ID: r.TagKind},
		{Kind: SmartRefPerson, ID: r.Person},
		{Kind: SmartRefOrganization, ID: r.Organization},
	} {
		if ref.ID != 0 {
			refs = append(refs, ref)
		}
	}
	return refs
}

// ReplaceRefs preserves the receiver and deduplicates replacement tags in first-appearance order.
func (r SmartRule) ReplaceRefs(kind SmartRuleRefKind, sources []int64, target int64) (SmartRule, bool) {
	out := r
	changed := false
	switch kind {
	case SmartRefTag:
		out.Tags, changed = replaceSmartRuleTags(r.Tags, sources, target)
	case SmartRefTagKind:
		out.TagKind, changed = replaceSmartRuleID(r.TagKind, sources, target)
	case SmartRefPerson:
		out.Person, changed = replaceSmartRuleID(r.Person, sources, target)
	case SmartRefOrganization:
		out.Organization, changed = replaceSmartRuleID(r.Organization, sources, target)
	}
	return out, changed
}

func replaceSmartRuleTags(tags, sources []int64, target int64) ([]int64, bool) {
	if !slices.ContainsFunc(tags, func(id int64) bool { return slices.Contains(sources, id) }) {
		return tags, false
	}
	out := make([]int64, 0, len(tags))
	for _, id := range tags {
		if slices.Contains(sources, id) {
			id = target
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, true
}

func replaceSmartRuleID(id int64, sources []int64, target int64) (int64, bool) {
	if id != 0 && slices.Contains(sources, id) {
		return target, true
	}
	return id, false
}

// Smart folders support known sort orders except SortRandom.
func ValidateSmartSort(sort VideoSort) error {
	if !sort.Valid() || sort == SortRandom {
		return fmt.Errorf("%w: sort %q", ErrInvalidSmartRule, sort)
	}
	return nil
}

// ValidateSmartFolderParent allows root placement and rejects self/descendant cycles; new folders have ID zero and no descendants.
func ValidateSmartFolderParent(id, parent int64, parents map[int64]int64) error {
	if parent == 0 || id == 0 {
		return nil
	}
	seen := map[int64]bool{}
	for at := parent; at != 0 && !seen[at]; at = parents[at] {
		if at == id {
			return ErrSmartFolderCycle
		}
		seen[at] = true
	}
	return nil
}

type SmartFolder struct {
	ID       int64
	ParentID int64
	Name     string
	Rule     SmartRule
	Sort     VideoSort
	// CreatedAt and UpdatedAt use Unix milliseconds.
	CreatedAt int64
	UpdatedAt int64
}

// Empty Sort defaults to SortAddedDesc; ParentID zero creates a root.
type SmartFolderCreate struct {
	Name     string
	Rule     SmartRule
	Sort     VideoSort
	ParentID int64
}

// Nil fields are preserved; ParentSet controls parent changes, with zero selecting the root.
type SmartFolderChange struct {
	Name      *string
	Rule      *SmartRule
	Sort      *VideoSort
	ParentSet bool
	ParentID  int64
}

// LibrarySmartFolder embeds folder ID/name and favorite state, has no Path, and derives card values from playback-order members.
type LibrarySmartFolder struct {
	LibraryGroup
	ParentID int64
}

// SmartFolderSummary omits card data, which the UI fetches separately.
type SmartFolderSummary struct {
	ID       int64
	ParentID int64
	Name     string
	Favorite bool
}

const MaxSmartFolderCards = 60

// SmartFolderCard aggregates matching visible videos individually, without grouping them first.
type SmartFolderCard struct {
	SmartFolderSummary
	VideoCount   int
	WatchState   WatchState
	WatchedCount int
	// OpenVideoID follows GroupOpenIndex in folder sort order, or zero when empty.
	OpenVideoID int64
	// DurationMs is nil when no duration is known.
	DurationMs   *int64
	SizeBytes    int64
	AddedAt      time.Time
	LastPlayedAt *time.Time
	// Previews contains up to MaxFolderPreviews ready thumbnails in folder order.
	Previews []Video
	// Tags is the union of matching-video tags, empty for guests.
	Tags []VideoTag
}

type SmartFolderPage struct {
	SmartFolder LibrarySmartFolder
	// Settings are returned only to the owner.
	Folder SmartFolder
	// Missing references are ordered outermost-ancestor first; any missing reference limits contents to child folders.
	Missing []SmartRuleRef
	Trail   []GroupRef
	// The first page places name-sorted child-folder summaries before library items.
	Items      []LibraryItem
	NextCursor string
	// Roots shares the contents' read snapshot.
	Roots []MediaFolder
}

// SmartFoldersChanged is emitted after commit with sorted unique nonempty IDs; folder edits affect descendants and parent, while matching-data changes affect all folders.
type SmartFoldersChanged struct {
	IDs []int64
}

func (SmartFoldersChanged) event() {}

// SameSmartRule compares rules independently of tag order.
func SameSmartRule(a, b SmartRule) bool {
	a.Tags = slices.Clone(a.Tags)
	b.Tags = slices.Clone(b.Tags)
	slices.Sort(a.Tags)
	slices.Sort(b.Tags)
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(left) == string(right)
}
