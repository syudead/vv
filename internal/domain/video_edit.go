package domain

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Bulk video edits share InvalidWorkInfoError with field paths into the request, such as tags.remove[0].

const VideoEditTagsMaxCount = 100

// ErrNoVideoEdit rejects a bulk edit without any part that could change a video.
var ErrNoVideoEdit = errors.New("the edit has no changes")

// TagInput names a tag by exactly one of ID or Name.
type TagInput struct {
	ID   *int64
	Name *string
}

// DateEdit sets one date kind; a nil Date clears it.
type DateEdit struct {
	Kind DateKind
	Date *string
}

// VideoEdit is one bulk edit; absent and empty parts leave their values as they are, and a nil Public keeps visibility.
type VideoEdit struct {
	TagsAdd    []TagInput
	TagsRemove []int64
	Credits    []CreditEdit
	Dates      []DateEdit
	Public     *bool
}

// ChangesWorkInfo reports whether the edit has credit or date parts.
func (e VideoEdit) ChangesWorkInfo() bool { return len(e.Credits) > 0 || len(e.Dates) > 0 }

// ChangesMatches reports whether the edit has tag or visibility parts, which smart-folder rules can read.
func (e VideoEdit) ChangesMatches() bool {
	return len(e.TagsAdd) > 0 || len(e.TagsRemove) > 0 || e.Public != nil
}

// NormalizeVideoEdit validates the whole edit before any video is read, normalizing tag names and dates.
func NormalizeVideoEdit(edit VideoEdit) (VideoEdit, error) {
	if !edit.ChangesWorkInfo() && !edit.ChangesMatches() {
		return VideoEdit{}, ErrNoVideoEdit
	}
	out := edit
	var err error
	if out.TagsAdd, err = normalizeTagInputs(edit.TagsAdd); err != nil {
		return VideoEdit{}, err
	}
	if err := checkTagRemovals(edit.TagsAdd, edit.TagsRemove); err != nil {
		return VideoEdit{}, err
	}
	if err := ValidateCreditEdits(edit.Credits); err != nil {
		return VideoEdit{}, err
	}
	if out.Dates, err = normalizeDateEdits(edit.Dates); err != nil {
		return VideoEdit{}, err
	}
	return out, nil
}

func normalizeTagInputs(inputs []TagInput) ([]TagInput, error) {
	if len(inputs) > VideoEditTagsMaxCount {
		return nil, invalidWorkInfo("tags.add", WorkInfoTooMany, VideoEditTagsMaxCount)
	}
	out := make([]TagInput, 0, len(inputs))
	for i, input := range inputs {
		field := fmt.Sprintf("tags.add[%d]", i)
		if (input.ID == nil) == (input.Name == nil) {
			return nil, invalidWorkInfo(field, WorkInfoOutOfRange, 0)
		}
		if input.Name != nil {
			name, err := NormalizeTagName(*input.Name)
			if err != nil {
				return nil, &TagNameAtError{Index: i, Err: err}
			}
			input.Name = &name
		}
		out = append(out, input)
	}
	return out, nil
}

// Removals are distinct and may not name a tag that the edit adds by ID.
func checkTagRemovals(add []TagInput, remove []int64) error {
	if len(remove) > VideoEditTagsMaxCount {
		return invalidWorkInfo("tags.remove", WorkInfoTooMany, VideoEditTagsMaxCount)
	}
	added := make(map[int64]bool, len(add))
	for _, input := range add {
		if input.ID != nil {
			added[*input.ID] = true
		}
	}
	return CheckTagRemovals(added, remove)
}

// CheckTagRemovals reports a removal that repeats another or names an added tag at tags.remove[i].
func CheckTagRemovals(added map[int64]bool, remove []int64) error {
	seen := make(map[int64]bool, len(remove))
	for i, id := range remove {
		if seen[id] || added[id] {
			return invalidWorkInfo(fmt.Sprintf("tags.remove[%d]", i), WorkInfoDuplicate, 0)
		}
		seen[id] = true
	}
	return nil
}

func normalizeDateEdits(dates []DateEdit) ([]DateEdit, error) {
	if len(dates) > len(DateKinds) {
		return nil, invalidWorkInfo("dates", WorkInfoTooMany, len(DateKinds))
	}
	seen := make(map[DateKind]bool, len(dates))
	out := make([]DateEdit, 0, len(dates))
	for i, edit := range dates {
		field := fmt.Sprintf("dates[%d]", i)
		if !edit.Kind.Valid() {
			return nil, invalidWorkInfo(field+".kind", WorkInfoOutOfRange, 0)
		}
		if seen[edit.Kind] {
			return nil, invalidWorkInfo(field+".kind", WorkInfoDuplicate, 0)
		}
		seen[edit.Kind] = true
		if edit.Date != nil {
			date, err := normalizeWorkDate(field+".date", *edit.Date)
			if err != nil {
				return nil, err
			}
			// A blank date would silently clear the kind; null is the explicit way to do that.
			if date == "" {
				return nil, invalidWorkInfo(field+".date", WorkInfoInvalidDate, 0)
			}
			edit.Date = &date
		}
		out = append(out, edit)
	}
	return out, nil
}

// ApplyToWorkInfo applies the normalized credit and date parts to one video's work information.
func (e VideoEdit) ApplyToWorkInfo(info WorkInfo) (WorkInfo, error) {
	out, err := ApplyCreditEdits(info, e.Credits)
	if err != nil {
		return WorkInfo{}, err
	}
	if len(e.Dates) == 0 {
		return out, nil
	}
	byKind := make(map[DateKind]string, len(DateKinds))
	for _, d := range info.Dates {
		byKind[d.Kind] = d.Date
	}
	for _, edit := range e.Dates {
		if edit.Date == nil {
			delete(byKind, edit.Kind)
		} else {
			byKind[edit.Kind] = *edit.Date
		}
	}
	out.Dates = make([]WorkDate, 0, len(byKind))
	for _, kind := range DateKinds {
		if date, ok := byKind[kind]; ok {
			out.Dates = append(out.Dates, WorkDate{Kind: kind, Date: date})
		}
	}
	return out, nil
}

// VideoEditResult counts the target videos any part changed; ContentKeys lists every content key of the targets
// when the edit made them private, so callers can end their guest streams.
type VideoEditResult struct {
	Applied     int
	Tags        []TagRef
	ContentKeys []string
}

type VideoEditPersonCount struct {
	Role          PersonRole
	Person        CreditRef
	ImageRevision int64
	Count         int
}

type VideoEditOrganizationCount struct {
	Role         OrganizationRole
	Organization CreditRef
	Count        int
}

// VideoEditDateCount leaves Date empty unless every counted video shares one date.
type VideoEditDateCount struct {
	Kind  DateKind
	Count int
	Date  string
}

// VideoEditSummary counts videos by user-data key; every count is out of Total.
type VideoEditSummary struct {
	Total         int
	Public        int
	Tags          []TagSummaryItem
	People        []VideoEditPersonCount
	Organizations []VideoEditOrganizationCount
	Dates         []VideoEditDateCount
}

// SummarizeWorkInfos counts credits per role and dates per kind across one work-info value per video, in role
// order then natural name order, and date-kind order omitting kinds no video has.
func SummarizeWorkInfos(infos []WorkInfo) ([]VideoEditPersonCount, []VideoEditOrganizationCount, []VideoEditDateCount) {
	people := []VideoEditPersonCount{}
	personIndex := map[WorkPerson]int{}
	organizations := []VideoEditOrganizationCount{}
	organizationIndex := map[WorkOrganization]int{}
	dates := make([]VideoEditDateCount, len(DateKinds))
	mixed := make([]bool, len(DateKinds))
	for _, info := range infos {
		for _, p := range info.People {
			key := WorkPerson{Role: p.Role, Person: CreditRef{ID: p.Person.ID}}
			if i, ok := personIndex[key]; ok {
				people[i].Count++
				continue
			}
			personIndex[key] = len(people)
			people = append(people, VideoEditPersonCount{Role: p.Role, Person: p.Person, ImageRevision: p.ImageRevision, Count: 1})
		}
		for _, o := range info.Organizations {
			key := WorkOrganization{Role: o.Role, Organization: CreditRef{ID: o.Organization.ID}}
			if i, ok := organizationIndex[key]; ok {
				organizations[i].Count++
				continue
			}
			organizationIndex[key] = len(organizations)
			organizations = append(organizations, VideoEditOrganizationCount{Role: o.Role, Organization: o.Organization, Count: 1})
		}
		countDates(dates, mixed, info.Dates)
	}
	for i := range dates {
		if mixed[i] {
			dates[i].Date = ""
		}
	}
	slices.SortFunc(people, func(a, b VideoEditPersonCount) int {
		return cmp.Or(slices.Index(PersonRoles, a.Role)-slices.Index(PersonRoles, b.Role), CompareCreditRefs(a.Person, b.Person))
	})
	slices.SortFunc(organizations, func(a, b VideoEditOrganizationCount) int {
		return cmp.Or(slices.Index(OrganizationRoles, a.Role)-slices.Index(OrganizationRoles, b.Role),
			CompareCreditRefs(a.Organization, b.Organization))
	})
	return people, organizations, slices.DeleteFunc(dates, func(d VideoEditDateCount) bool { return d.Count == 0 })
}

// mixed marks the kinds whose videos disagree on the date.
func countDates(counts []VideoEditDateCount, mixed []bool, dates []WorkDate) {
	for _, d := range dates {
		i := slices.Index(DateKinds, d.Kind)
		if i < 0 {
			continue
		}
		c := &counts[i]
		if c.Count == 0 {
			c.Kind, c.Date = d.Kind, d.Date
		} else if c.Date != d.Date {
			mixed[i] = true
		}
		c.Count++
	}
}

// CompareCreditRefs orders named credits by natural name before provisional ones, with ties by ID.
func CompareCreditRefs(a, b CreditRef) int {
	if a.Provisional != b.Provisional {
		if a.Provisional {
			return 1
		}
		return -1
	}
	return cmp.Or(CompareNatural(a.Name, b.Name), strings.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
}
