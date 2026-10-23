package domain

import (
	"errors"
	"slices"
	"unicode/utf8"
)

// Merging a virtual video into a file video combines their work information; a result over the work-information
// limits is rejected so the owner shortens one side instead of losing text
// (docs/design-docs/catalog-organization.md).

var (
	ErrMergedDescriptionTooLong = errors.New("the combined description is too long")
	ErrMergedTooManyURLs        = errors.New("the combined links are too many")
)

// ContentVersionLength is the length of a file video's contentVersion, a prefix of its content key.
const ContentVersionLength = 12

// ContentVersion is the version a screen reads for a video: a virtual video's whole user key, or a file video's
// content key prefix.
func ContentVersion(contentKey string) string {
	if IsVirtualKey(contentKey) || len(contentKey) <= ContentVersionLength {
		return contentKey
	}
	return contentKey[:ContentVersionLength]
}

// MergeWorkInfo keeps the file video's scalar values, filling only empty ones from the virtual video, appends the
// virtual description after a blank line, and unions the lists with the file video's entries first.
func MergeWorkInfo(file, virtual WorkInfo) (WorkInfo, error) {
	out := WorkInfo{
		Code:          firstNonEmpty(file.Code, virtual.Code),
		Description:   joinDescriptions(file.Description, virtual.Description),
		Dates:         mergeDates(file.Dates, virtual.Dates),
		People:        appendMissing(file.People, virtual.People, personCredit),
		Organizations: appendMissing(file.Organizations, virtual.Organizations, organizationCredit),
		URLs:          appendMissing(file.URLs, virtual.URLs, itself[string]),
		ExternalIDs:   appendMissing(file.ExternalIDs, virtual.ExternalIDs, itself[ExternalID]),
	}
	SortExternalIDs(out.ExternalIDs)
	if utf8.RuneCountInString(out.Description) > WorkInfoLimits.DescriptionMaxLength {
		return WorkInfo{}, ErrMergedDescriptionTooLong
	}
	if len(out.URLs) > WorkInfoLimits.URLsMaxCount {
		return WorkInfo{}, ErrMergedTooManyURLs
	}
	return out, nil
}

type roleCredit struct {
	role string
	id   int64
}

func personCredit(p WorkPerson) roleCredit { return roleCredit{string(p.Role), p.Person.ID} }

func organizationCredit(o WorkOrganization) roleCredit {
	return roleCredit{string(o.Role), o.Organization.ID}
}

func itself[T any](value T) T { return value }

func firstNonEmpty(first, second string) string {
	if first != "" {
		return first
	}
	return second
}

func joinDescriptions(file, virtual string) string {
	if file == "" || virtual == "" {
		return file + virtual
	}
	return file + "\n\n" + virtual
}

func mergeDates(file, virtual []WorkDate) []WorkDate {
	out := appendMissing(file, virtual, func(d WorkDate) DateKind { return d.Kind })
	slices.SortStableFunc(out, func(a, b WorkDate) int {
		return slices.Index(DateKinds, a.Kind) - slices.Index(DateKinds, b.Kind)
	})
	return out
}

// appendMissing returns a new slice of first followed by the entries of second whose key first lacks.
func appendMissing[T any, K comparable](first, second []T, key func(T) K) []T {
	seen := make(map[K]bool, len(first))
	out := make([]T, 0, len(first)+len(second))
	for _, item := range first {
		seen[key(item)] = true
		out = append(out, item)
	}
	for _, item := range second {
		if !seen[key(item)] {
			seen[key(item)] = true
			out = append(out, item)
		}
	}
	return out
}
