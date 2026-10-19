package domain

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Work-info and credit validation share InvalidWorkInfoError with the failing input property's path.

const (
	ExternalIDsMaxCount       = 20
	ExternalIDSourceMaxLength = 50
	ExternalIDValueMaxLength  = 200
)

var ErrInvalidWorkInfo = errors.New("invalid work information")

type WorkInfoProblem int

const (
	// Description permits line feeds; other control characters are rejected.
	WorkInfoControlCharacters WorkInfoProblem = iota + 1
	WorkInfoTooLong
	WorkInfoTooMany
	WorkInfoInvalidDate
	WorkInfoInvalidURL
	WorkInfoInvalidLanguage
	WorkInfoDuplicate
	WorkInfoEmpty
	// OutOfRange covers numbers outside their range, values outside their set, future dates and an end before its start.
	WorkInfoOutOfRange
)

// Field uses JSON paths with original input indices, including dropped entries; Limit applies only to TooLong/TooMany.
type InvalidWorkInfoError struct {
	Field   string
	Problem WorkInfoProblem
	Limit   int
}

func (e *InvalidWorkInfoError) Error() string {
	switch e.Problem {
	case WorkInfoControlCharacters:
		return fmt.Sprintf("%s contains control characters", e.Field)
	case WorkInfoTooLong:
		return fmt.Sprintf("%s must be at most %d characters", e.Field, e.Limit)
	case WorkInfoTooMany:
		return fmt.Sprintf("%s must have at most %d entries", e.Field, e.Limit)
	case WorkInfoInvalidDate:
		return fmt.Sprintf("%s is not a valid date", e.Field)
	case WorkInfoInvalidURL:
		return fmt.Sprintf("%s is not a valid URL", e.Field)
	case WorkInfoInvalidLanguage:
		return fmt.Sprintf("%s is not a valid language tag", e.Field)
	case WorkInfoDuplicate:
		return fmt.Sprintf("%s is a duplicate", e.Field)
	case WorkInfoEmpty:
		return fmt.Sprintf("%s is empty", e.Field)
	case WorkInfoOutOfRange:
		return fmt.Sprintf("%s is out of range", e.Field)
	default:
		return ErrInvalidWorkInfo.Error()
	}
}

func (e *InvalidWorkInfoError) Unwrap() error { return ErrInvalidWorkInfo }

func invalidWorkInfo(field string, problem WorkInfoProblem, limit int) error {
	return &InvalidWorkInfoError{Field: field, Problem: problem, Limit: limit}
}

type ExternalID struct {
	Source string
	Value  string
}

// CreditRef reads carry current names; writes use only ID.
type CreditRef struct {
	ID   int64
	Name string
	// Provisional marks a person without a canonical name; Name is then empty. Writes ignore it.
	Provisional bool
}

// normalizeWorkInfoText rejects controls before trimming and counts code points; empty results are left to callers to clear, drop, or reject.
func normalizeWorkInfoText(field, input string, maxLength int, allowLineFeed bool) (string, error) {
	for _, r := range input {
		if unicode.Is(unicode.Cc, r) && (!allowLineFeed || r != '\n') {
			return "", invalidWorkInfo(field, WorkInfoControlCharacters, 0)
		}
	}
	trimmed := trimWorkInfoText(input)
	if utf8.RuneCountInString(trimmed) > maxLength {
		return "", invalidWorkInfo(field, WorkInfoTooLong, maxLength)
	}
	return trimmed, nil
}

func trimWorkInfoText(input string) string {
	return strings.TrimFunc(input, func(r rune) bool {
		return unicode.Is(unicode.White_Space, r)
	})
}

// Drop fully blank external IDs; reject partially blank IDs on the missing side.
func normalizeExternalID(field string, id ExternalID) (ExternalID, bool, error) {
	source, err := normalizeWorkInfoText(field+".source", id.Source, ExternalIDSourceMaxLength, false)
	if err != nil {
		return ExternalID{}, false, err
	}
	value, err := normalizeWorkInfoText(field+".value", id.Value, ExternalIDValueMaxLength, false)
	if err != nil {
		return ExternalID{}, false, err
	}
	switch {
	case source == "" && value == "":
		return ExternalID{}, false, nil
	case source == "":
		return ExternalID{}, false, invalidWorkInfo(field+".source", WorkInfoEmpty, 0)
	case value == "":
		return ExternalID{}, false, invalidWorkInfo(field+".value", WorkInfoEmpty, 0)
	}
	return ExternalID{Source: source, Value: value}, true, nil
}

// Validate external IDs in input order, reject duplicate pairs, and enforce the count after dropping empty entries; return empty slices, not nil.
func normalizeExternalIDs(field string, ids []ExternalID) ([]ExternalID, error) {
	normalized := make([]ExternalID, 0, len(ids))
	seen := make(map[ExternalID]bool, len(ids))
	for i, raw := range ids {
		entryField := fmt.Sprintf("%s[%d]", field, i)
		id, keep, err := normalizeExternalID(entryField, raw)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		if seen[id] {
			return nil, invalidWorkInfo(entryField, WorkInfoDuplicate, 0)
		}
		seen[id] = true
		normalized = append(normalized, id)
	}
	if len(normalized) > ExternalIDsMaxCount {
		return nil, invalidWorkInfo(field, WorkInfoTooMany, ExternalIDsMaxCount)
	}
	return normalized, nil
}

type DateKind string

const (
	DateRelease   DateKind = "release"
	DateRecorded  DateKind = "recorded"
	DatePublished DateKind = "published"
)

// DateKinds defines read order.
var DateKinds = []DateKind{DateRelease, DateRecorded, DatePublished}

func (k DateKind) Valid() bool { return slices.Contains(DateKinds, k) }

type PersonRole string

const (
	PersonPerformer    PersonRole = "performer"
	PersonDirector     PersonRole = "director"
	PersonPhotographer PersonRole = "photographer"
	PersonStreamer     PersonRole = "streamer"
)

// PersonRoles defines read order.
var PersonRoles = []PersonRole{PersonPerformer, PersonDirector, PersonPhotographer, PersonStreamer}

func (r PersonRole) Valid() bool { return slices.Contains(PersonRoles, r) }

type OrganizationRole string

const (
	OrganizationMaker OrganizationRole = "maker"
	OrganizationLabel OrganizationRole = "label"
)

// OrganizationRoles defines read order.
var OrganizationRoles = []OrganizationRole{OrganizationMaker, OrganizationLabel}

func (r OrganizationRole) Valid() bool { return slices.Contains(OrganizationRoles, r) }

// WorkDate preserves YYYY, YYYY-MM, or YYYY-MM-DD precision as submitted.
type WorkDate struct {
	Kind DateKind
	Date string
}

// Writes use Person.ID only.
type WorkPerson struct {
	Role   PersonRole
	Person CreditRef
	// ImageRevision is zero unless the person has a face; writes ignore it.
	ImageRevision int64
}

// Writes use Organization.ID only.
type WorkOrganization struct {
	Role         OrganizationRole
	Organization CreditRef
}

// WorkInfo field order matches the API and determines the first validation error; empty strings/lists mean unset.
type WorkInfo struct {
	Code          string
	Description   string
	Dates         []WorkDate
	People        []WorkPerson
	Organizations []WorkOrganization
	URLs          []string
	ExternalIDs   []ExternalID
}

func (w WorkInfo) IsEmpty() bool {
	return w.Code == "" && w.Description == "" && len(w.Dates) == 0 && len(w.People) == 0 &&
		len(w.Organizations) == 0 && len(w.URLs) == 0 && len(w.ExternalIDs) == 0
}

// Equal compares list order, treats nil and empty lists equally, and compares credits by ID only.
func (w WorkInfo) Equal(other WorkInfo) bool {
	return w.Code == other.Code && w.Description == other.Description &&
		slices.Equal(w.Dates, other.Dates) && slices.Equal(w.URLs, other.URLs) &&
		slices.Equal(w.ExternalIDs, other.ExternalIDs) &&
		slices.EqualFunc(w.People, other.People, func(a, b WorkPerson) bool {
			return a.Role == b.Role && a.Person.ID == b.Person.ID
		}) &&
		slices.EqualFunc(w.Organizations, other.Organizations, func(a, b WorkOrganization) bool {
			return a.Role == b.Role && a.Organization.ID == b.Organization.ID
		})
}

// Limits count code points and retained entries after normalization.
type WorkInfoLimitSet struct {
	CodeMaxLength                int
	DescriptionMaxLength         int
	URLsMaxCount                 int
	URLMaxLength                 int
	ExternalIDsMaxCount          int
	ExternalIDSourceMaxLength    int
	ExternalIDValueMaxLength     int
	PeoplePerRoleMaxCount        int
	OrganizationsPerRoleMaxCount int
}

var WorkInfoLimits = WorkInfoLimitSet{
	CodeMaxLength:                100,
	DescriptionMaxLength:         10_000,
	URLsMaxCount:                 20,
	URLMaxLength:                 2_048,
	ExternalIDsMaxCount:          ExternalIDsMaxCount,
	ExternalIDSourceMaxLength:    ExternalIDSourceMaxLength,
	ExternalIDValueMaxLength:     ExternalIDValueMaxLength,
	PeoplePerRoleMaxCount:        100,
	OrganizationsPerRoleMaxCount: 20,
}

// NormalizeWorkInfo validates in schema/input order with original indices, then orders dates/roles/external IDs for reading while preserving URL order; storage checks referenced credit IDs.
func NormalizeWorkInfo(input WorkInfo) (WorkInfo, bool, error) {
	var out WorkInfo
	var err error
	limits := WorkInfoLimits
	if out.Code, err = normalizeWorkInfoText("code", input.Code, limits.CodeMaxLength, false); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Description, err = normalizeWorkInfoText("description", input.Description, limits.DescriptionMaxLength, true); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Dates, err = normalizeWorkDates(input.Dates); err != nil {
		return WorkInfo{}, false, err
	}
	if out.People, err = normalizeWorkPeople(input.People); err != nil {
		return WorkInfo{}, false, err
	}
	if out.Organizations, err = normalizeWorkOrganizations(input.Organizations); err != nil {
		return WorkInfo{}, false, err
	}
	if out.URLs, err = normalizeWorkURLs(input.URLs); err != nil {
		return WorkInfo{}, false, err
	}
	if out.ExternalIDs, err = normalizeExternalIDs("externalIds", input.ExternalIDs); err != nil {
		return WorkInfo{}, false, err
	}
	SortExternalIDs(out.ExternalIDs)
	return out, out.IsEmpty(), nil
}

func normalizeWorkDates(dates []WorkDate) ([]WorkDate, error) {
	byKind := make(map[DateKind]string, len(DateKinds))
	for i, raw := range dates {
		field := fmt.Sprintf("dates[%d]", i)
		if !raw.Kind.Valid() {
			return nil, fmt.Errorf("%s.kind: unknown date kind %q: %w", field, raw.Kind, ErrInvalidWorkInfo)
		}
		date, err := normalizeWorkDate(field+".date", raw.Date)
		if err != nil {
			return nil, err
		}
		if date == "" {
			continue
		}
		if _, ok := byKind[raw.Kind]; ok {
			return nil, invalidWorkInfo(field+".kind", WorkInfoDuplicate, 0)
		}
		byKind[raw.Kind] = date
	}
	out := make([]WorkDate, 0, len(byKind))
	for _, kind := range DateKinds {
		if date, ok := byKind[kind]; ok {
			out = append(out, WorkDate{Kind: kind, Date: date})
		}
	}
	return out, nil
}

var workDateLayouts = map[int]string{len("2006"): "2006", len("2006-01"): "2006-01", len(time.DateOnly): time.DateOnly}

// Dates must use YYYY, YYYY-MM, or real YYYY-MM-DD within years 1–9999; overlong values are format errors.
func normalizeWorkDate(field, input string) (string, error) {
	date, err := normalizeWorkInfoText(field, input, utf8.RuneCountInString(input), false)
	if err != nil || date == "" {
		return date, err
	}
	if !isWorkDate(date) {
		return "", invalidWorkInfo(field, WorkInfoInvalidDate, 0)
	}
	return date, nil
}

// Check digits and separators first because time.Parse accepts unpadded months/days.
func isWorkDate(date string) bool {
	layout, ok := workDateLayouts[len(date)]
	if !ok {
		return false
	}
	for i := 0; i < len(date); i++ {
		if i == 4 || i == 7 {
			if date[i] != '-' {
				return false
			}
		} else if date[i] < '0' || date[i] > '9' {
			return false
		}
	}
	if date[:4] == "0000" {
		return false
	}
	_, err := time.Parse(layout, date)
	return err == nil
}

// People are unique per role, bounded per role, and returned in role order with input order preserved within roles; names are dropped.
func normalizeWorkPeople(people []WorkPerson) ([]WorkPerson, error) {
	refs := make([]workRef[PersonRole], 0, len(people))
	for _, p := range people {
		refs = append(refs, workRef[PersonRole]{role: p.Role, id: p.Person.ID})
	}
	sorted, err := normalizeWorkRefs("people", "person", PersonRoles, refs, WorkInfoLimits.PeoplePerRoleMaxCount)
	if err != nil {
		return nil, err
	}
	out := make([]WorkPerson, 0, len(sorted))
	for _, ref := range sorted {
		out = append(out, WorkPerson{Role: ref.role, Person: CreditRef{ID: ref.id}})
	}
	return out, nil
}

func normalizeWorkOrganizations(organizations []WorkOrganization) ([]WorkOrganization, error) {
	refs := make([]workRef[OrganizationRole], 0, len(organizations))
	for _, o := range organizations {
		refs = append(refs, workRef[OrganizationRole]{role: o.Role, id: o.Organization.ID})
	}
	sorted, err := normalizeWorkRefs("organizations", "organization", OrganizationRoles, refs,
		WorkInfoLimits.OrganizationsPerRoleMaxCount)
	if err != nil {
		return nil, err
	}
	out := make([]WorkOrganization, 0, len(sorted))
	for _, ref := range sorted {
		out = append(out, WorkOrganization{Role: ref.role, Organization: CreditRef{ID: ref.id}})
	}
	return out, nil
}

type workRef[R ~string] struct {
	role R
	id   int64
}

// normalizeWorkRefs shares person/organization validation using per-role limits and read order.
func normalizeWorkRefs[R ~string](list, entity string, roles []R, refs []workRef[R], limit int) ([]workRef[R], error) {
	byRole := make(map[R][]workRef[R], len(roles))
	seen := make(map[workRef[R]]bool, len(refs))
	for i, ref := range refs {
		field := fmt.Sprintf("%s[%d]", list, i)
		if !slices.Contains(roles, ref.role) {
			return nil, fmt.Errorf("%s.role: unknown role %q: %w", field, ref.role, ErrInvalidWorkInfo)
		}
		if seen[ref] {
			return nil, invalidWorkInfo(field+"."+entity+".id", WorkInfoDuplicate, 0)
		}
		seen[ref] = true
		byRole[ref.role] = append(byRole[ref.role], ref)
		if len(byRole[ref.role]) > limit {
			return nil, invalidWorkInfo(list, WorkInfoTooMany, limit)
		}
	}
	out := make([]workRef[R], 0, len(refs))
	for _, role := range roles {
		out = append(out, byRole[role]...)
	}
	return out, nil
}

// URLs must be absolute HTTP(S) with a host; drop blanks, reject duplicates, enforce retained count, and preserve input order.
func normalizeWorkURLs(urls []string) ([]string, error) {
	return normalizeURLList("urls", urls, WorkInfoLimits.URLsMaxCount)
}

// normalizeURLList reports entries as field[i] and the retained count against field.
func normalizeURLList(field string, urls []string, maxCount int) ([]string, error) {
	out := make([]string, 0, len(urls))
	seen := make(map[string]bool, len(urls))
	for i, raw := range urls {
		at := fmt.Sprintf("%s[%d]", field, i)
		value, err := normalizeWorkInfoText(at, raw, WorkInfoLimits.URLMaxLength, false)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		if !isWorkURL(value) {
			return nil, invalidWorkInfo(at, WorkInfoInvalidURL, 0)
		}
		if seen[value] {
			return nil, invalidWorkInfo(at, WorkInfoDuplicate, 0)
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) > maxCount {
		return nil, invalidWorkInfo(field, WorkInfoTooMany, maxCount)
	}
	return out, nil
}

func isWorkURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Opaque == "" &&
		parsed.Hostname() != ""
}

// WorkInfoChange replaces the whole value; normalized empty input clears it.
type WorkInfoChange struct {
	Video    VideoRef
	WorkInfo WorkInfo
}

// WorkInfoAtError carries a zero-based input index and wraps validation or missing-credit errors.
type WorkInfoAtError struct {
	Index int
	Err   error
}

func (e *WorkInfoAtError) Error() string {
	return fmt.Sprintf("work information %d: %v", e.Index, e.Err)
}

func (e *WorkInfoAtError) Unwrap() error { return e.Err }

// VideoWorkInfoChanged includes all videos sharing changed user-data keys and notifies the UI without waking workers.
type VideoWorkInfoChanged struct {
	VideoIDs []int64
}

func (VideoWorkInfoChanged) event() {}

// ExternalIDIndex recovers the first original input position for errors on normalized IDs, or -1 if absent.
func ExternalIDIndex(raw []ExternalID, id ExternalID) int {
	for i, entry := range raw {
		if normalized, ok := NormalizeExternalID(entry); ok && normalized == id {
			return i
		}
	}
	return -1
}

// NormalizeExternalID shares storage normalization with lookup; invalid IDs cannot belong to any entity and therefore match nothing.
func NormalizeExternalID(id ExternalID) (ExternalID, bool) {
	normalized, keep, err := normalizeExternalID("", id)
	return normalized, keep && err == nil
}
