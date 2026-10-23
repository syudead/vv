package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	WorkInfoSearchQueryMaxRunes = 200
	WorkInfoFetchLifetime       = 10 * time.Minute
)

var (
	ErrWorkInfoPluginNotApproved    = errors.New("the work-info plugin is not approved")
	ErrWorkInfoOperationUnsupported = errors.New("the work-info plugin does not support the operation")
	ErrWorkInfoRunInvalid           = errors.New("the work-info plugin request is invalid")
	// Stale results include expiry, unknown IDs, replacement, revocation, and deletion of the plugin.
	ErrWorkInfoFetchStale   = errors.New("the fetched work information is out of date")
	ErrWorkInfoApplyInvalid = errors.New("the selected fields cannot be applied")
)

// Keys lists missing required settings in declaration order; execution never started.
type WorkInfoSettingsMissingError struct {
	Keys []string
}

func (e *WorkInfoSettingsMissingError) Error() string {
	return "the work-info plugin needs settings: " + strings.Join(e.Keys, ", ")
}

// EffectiveWorkInfoPluginValue uses a nonempty allowed saved value, otherwise the default; stale marks rejected saved choices, and empty effective values are omitted.
func EffectiveWorkInfoPluginValue(s WorkInfoPluginSetting, values map[string]string) (value string, stale bool) {
	stored := values[s.Key]
	if stored == "" {
		return s.Default, false
	}
	if s.Options != nil && !slices.Contains(s.Options, stored) {
		return s.Default, true
	}
	return stored, false
}

// Required settings with effective defaults are not missing; missing keys follow declaration order.
func MissingWorkInfoPluginSettings(settings []WorkInfoPluginSetting, values map[string]string) []string {
	var missing []string
	for _, s := range settings {
		if v, _ := EffectiveWorkInfoPluginValue(s, values); s.Required && v == "" {
			missing = append(missing, s.Key)
		}
	}
	return missing
}

// Only nonempty effective values for declared keys enter the execution request.
func DeclaredWorkInfoPluginValues(settings []WorkInfoPluginSetting, values map[string]string) map[string]string {
	out := make(map[string]string, len(settings))
	for _, s := range settings {
		if v, _ := EffectiveWorkInfoPluginValue(s, values); v != "" {
			out[s.Key] = v
		}
	}
	return out
}

// WorkInfoRunSettings is shared by server execution and mdm plugin run; missing required effective values prevent execution.
func WorkInfoRunSettings(settings []WorkInfoPluginSetting, values map[string]string) (map[string]string, error) {
	if missing := MissingWorkInfoPluginSettings(settings, values); len(missing) > 0 {
		return nil, &WorkInfoSettingsMissingError{Keys: missing}
	}
	return DeclaredWorkInfoPluginValues(settings, values), nil
}

// KeptWorkInfoTitle treats invalid display names as absent.
func KeptWorkInfoTitle(raw string) string {
	title, _, err := NormalizeDisplayName(raw)
	if err != nil {
		return ""
	}
	return title
}

// NormalizeWorkInfoSearchQuery trims and validates UTF-8/code-point length; even URL-shaped owner input remains an opaque work ID.
func NormalizeWorkInfoSearchQuery(query string) (string, error) {
	trimmed := strings.TrimFunc(query, unicode.IsSpace)
	if !runeCountBetween(trimmed, 1, WorkInfoSearchQueryMaxRunes) {
		return "", ErrWorkInfoRunInvalid
	}
	return trimmed, nil
}

type WorkInfoField string

const (
	WorkInfoFieldTitle WorkInfoField = "title"
	// Fetched tags are applied tentatively.
	WorkInfoFieldTags        WorkInfoField = "tags"
	WorkInfoFieldCode        WorkInfoField = "code"
	WorkInfoFieldDescription WorkInfoField = "description"
	// Fetched URLs append only previously absent values.
	WorkInfoFieldURLs WorkInfoField = "urls"
	// Fetched external IDs append only previously absent pairs.
	WorkInfoFieldExternalIDs WorkInfoField = "externalIds"
)

// DateField replaces the selected date kind.
func DateField(kind DateKind) WorkInfoField { return WorkInfoField("dates." + string(kind)) }

// PersonField adds previously unlinked people for the role.
func PersonField(role PersonRole) WorkInfoField { return WorkInfoField("people." + string(role)) }

func OrganizationField(role OrganizationRole) WorkInfoField {
	return WorkInfoField("organizations." + string(role))
}

// WorkInfoFields defines comparison-row order.
var WorkInfoFields = func() []WorkInfoField {
	fields := []WorkInfoField{
		WorkInfoFieldTitle, WorkInfoFieldTags, WorkInfoFieldCover, WorkInfoFieldCode, WorkInfoFieldDescription,
	}
	for _, kind := range DateKinds {
		fields = append(fields, DateField(kind))
	}
	for _, role := range PersonRoles {
		fields = append(fields, PersonField(role))
	}
	for _, role := range OrganizationRoles {
		fields = append(fields, OrganizationField(role))
	}
	return append(fields, WorkInfoFieldURLs, WorkInfoFieldExternalIDs)
}()

type WorkInfoFetchResult struct {
	ID        string
	ExpiresAt time.Time
	Title     string
	Tags      []string
	// Details excludes title and tags and follows OrderFetchedWorkDetails.
	Details FetchedWorkInfo
	// Match arrays align with Details entries and reflect fetch-time matching.
	PersonMatches       []CreditMatch
	OrganizationMatches []CreditMatch
	// Covers are the readable fetched covers in result order.
	Covers []WorkInfoCover
}

// WorkInfoSelection is what a work-info apply takes from a fetch: whole Fields (title, tags, code, description, dates
// and cover), the cover at CoverIndex, and the people, organizations, URLs and external IDs one by one.
type WorkInfoSelection struct {
	Fields []WorkInfoField
	// Tags names the chosen fetched tags; nil chooses all of them.
	Tags       []string
	CoverIndex int
	// People and Organizations are positions in the fetched details.
	People        []int
	Organizations []int
	URLs          []string
	ExternalIDs   []ExternalID
}

// isListField reports a field the selection chooses value by value instead of whole.
func isListField(field WorkInfoField) bool {
	if field == WorkInfoFieldURLs || field == WorkInfoFieldExternalIDs {
		return true
	}
	return slices.ContainsFunc(PersonRoles, func(r PersonRole) bool { return field == PersonField(r) }) ||
		slices.ContainsFunc(OrganizationRoles, func(r OrganizationRole) bool { return field == OrganizationField(r) })
}

// Select narrows f to the chosen people, organizations, URLs and external IDs, in f's order, and returns the detail
// fields to merge: the code, description and dates among sel.Fields, then each role with a chosen entry, then URLs and
// external IDs when chosen. It reports false for a list field among sel.Fields, a position outside f or named twice,
// or a value f does not carry; the whole fields' values are the caller's to check.
func (f FetchedWorkInfo) Select(sel WorkInfoSelection) (FetchedWorkInfo, []WorkInfoField, bool) {
	if slices.ContainsFunc(sel.Fields, isListField) {
		return FetchedWorkInfo{}, nil, false
	}
	people, okPeople := pickAt(f.People, sel.People)
	organizations, okOrganizations := pickAt(f.Organizations, sel.Organizations)
	urls, okURLs := pick(f.URLs, sel.URLs, func(url string) string { return url })
	ids, okIDs := pick(f.ExternalIDs, sel.ExternalIDs, func(id ExternalID) ExternalID { return id })
	if !okPeople || !okOrganizations || !okURLs || !okIDs {
		return FetchedWorkInfo{}, nil, false
	}
	out := f
	out.People, out.Organizations, out.URLs, out.ExternalIDs = people, organizations, urls, ids
	var fields []WorkInfoField
	for _, field := range WorkInfoFields {
		if slices.Contains(sel.Fields, field) && field != WorkInfoFieldTitle && field != WorkInfoFieldTags &&
			field != WorkInfoFieldCover || isListField(field) && out.HasDetail(field) {
			fields = append(fields, field)
		}
	}
	return out, fields, true
}

// pickAt keeps the values of all at the selected positions, in all's order, and reports false for a position outside
// all or named twice.
func pickAt[T any](all []T, selected []int) ([]T, bool) {
	var out []T
	for i, value := range all {
		if slices.Contains(selected, i) {
			out = append(out, value)
		}
	}
	return out, len(out) == len(selected)
}

// OrderFetchedWorkDetails returns a copy ordered by date kind and credit role, preserving within-kind/role order and excluding title/tags.
func OrderFetchedWorkDetails(fetched FetchedWorkInfo) FetchedWorkInfo {
	out := fetched
	out.Title, out.Tags = "", nil
	out.Dates = stableByOrder(fetched.Dates, DateKinds, func(d WorkDate) DateKind { return d.Kind })
	out.People = stableByOrder(fetched.People, PersonRoles, func(p FetchedPerson) PersonRole { return p.Role })
	out.Organizations = stableByOrder(fetched.Organizations, OrganizationRoles,
		func(o FetchedOrganization) OrganizationRole { return o.Role })
	return out
}

// stableByOrder copies input and preserves nil.
func stableByOrder[T any, K comparable](items []T, order []K, key func(T) K) []T {
	if items == nil {
		return nil
	}
	out := slices.Clone(items)
	slices.SortStableFunc(out, func(a, b T) int {
		return slices.Index(order, key(a)) - slices.Index(order, key(b))
	})
	return out
}

func (f FetchedWorkInfo) HasDetail(field WorkInfoField) bool {
	switch field {
	case WorkInfoFieldCode:
		return f.Code != ""
	case WorkInfoFieldDescription:
		return f.Description != ""
	case WorkInfoFieldURLs:
		return len(f.URLs) > 0
	case WorkInfoFieldExternalIDs:
		return len(f.ExternalIDs) > 0
	}
	for _, kind := range DateKinds {
		if field == DateField(kind) {
			return slices.ContainsFunc(f.Dates, func(d WorkDate) bool { return d.Kind == kind })
		}
	}
	for _, role := range PersonRoles {
		if field == PersonField(role) {
			return slices.ContainsFunc(f.People, func(p FetchedPerson) bool { return p.Role == role })
		}
	}
	for _, role := range OrganizationRoles {
		if field == OrganizationField(role) {
			return slices.ContainsFunc(f.Organizations, func(o FetchedOrganization) bool { return o.Role == role })
		}
	}
	return false
}

type SkippedTagReason string

const (
	SkippedTagRejected SkippedTagReason = "rejected"
	SkippedTagInvalid  SkippedTagReason = "invalid"
)

type SkippedTag struct {
	Name   string
	Reason SkippedTagReason
}

// Apply rechecks plugin existence, approval, digest, and generation inside the same write transaction.
type FetchedWorkInfoApply struct {
	VideoID int64
	// ContentKey is the video's key when it was fetched; a reused VideoID with another key is not the fetched video.
	ContentKey string
	PluginID   int64
	Digest     string
	// Generation detects replacement/revocation, including reapproval of an identical digest.
	Generation int64
	// Nil Title preserves the display name.
	Title *string
	// Empty Tags preserves current tags; supplied names are normalized tentative assignments.
	Tags    []string
	Details FetchedWorkInfo
	// Empty DetailFields preserves work info.
	DetailFields []WorkInfoField
}

// RejectedTags is deduplicated in Tags order; Skipped reports omitted detail values.
type FetchedWorkInfoApplied struct {
	Video        Video
	RejectedTags []string
	Skipped      []SkippedWorkInfoValue
	// PersonIDs aligns with Details.People: the person each entry was linked to, or 0.
	PersonIDs []int64
}

type WorkInfoApplied struct {
	Video       Video
	SkippedTags []SkippedTag
	Skipped     []SkippedWorkInfoValue
	// LinkedPeople are the distinct people the chosen entries were linked to, in fetched order.
	LinkedPeople []int64
}
