package domain

import (
	"slices"
)

// Fetched merging replaces scalar/date fields and appends new credits, URLs, and IDs; invalid, excessive, or ambiguous values are reported and skipped while valid values still apply.

type SkippedWorkInfoReason string

const (
	SkippedWorkInfoInvalid   SkippedWorkInfoReason = "invalid"
	SkippedWorkInfoTooMany   SkippedWorkInfoReason = "too_many"
	SkippedWorkInfoAmbiguous SkippedWorkInfoReason = "ambiguous"
)

const SkippedDescriptionMaxRunes = 100

// Value preserves the comparison text, truncating descriptions to SkippedDescriptionMaxRunes.
type SkippedWorkInfoValue struct {
	Field  WorkInfoField
	Value  string
	Reason SkippedWorkInfoReason
}

// NewCredit.Links identifies result positions whose IDs storage fills after creating the normalized entity.
type NewCredit struct {
	Input CreditInput
	Links []int
}

// Match arrays must align with fetched credit entries.
type WorkInfoMergeInput struct {
	Stored              WorkInfo
	Fetched             FetchedWorkInfo
	PersonMatches       []CreditMatch
	OrganizationMatches []CreditMatch
	Fields              []WorkInfoField
}

// New entities initially have ID zero; Links identifies their positions, and Skipped follows field order then fetch order.
type WorkInfoMerge struct {
	Info             WorkInfo
	NewPeople        []NewCredit
	NewOrganizations []NewCredit
	Skipped          []SkippedWorkInfoValue
}

// MergeFetchedWorkInfo ignores absent selected values and title/tags, validating individual values with NormalizeWorkInfo.
func MergeFetchedWorkInfo(in WorkInfoMergeInput) WorkInfoMerge {
	m := workInfoMerger{
		selected: func(field WorkInfoField) bool { return slices.Contains(in.Fields, field) },
		out:      WorkInfoMerge{Info: cloneWorkInfo(in.Stored)},
	}
	m.replaceText(WorkInfoFieldCode, in.Fetched.Code, func(w *WorkInfo) *string { return &w.Code })
	m.replaceText(WorkInfoFieldDescription, in.Fetched.Description, func(w *WorkInfo) *string { return &w.Description })
	m.replaceDates(in.Fetched.Dates)

	people := mergeCreditRoles(m.selected, creditRoleInput[PersonRole]{
		roles: PersonRoles, field: PersonField, limit: WorkInfoLimits.PeoplePerRoleMaxCount,
		stored:  workLinks(in.Stored.People, func(p WorkPerson) (PersonRole, CreditRef) { return p.Role, p.Person }),
		fetched: fetchedLinks(in.Fetched.People, func(p FetchedPerson) (PersonRole, string, []ExternalID) { return p.Role, p.Name, p.ExternalIDs }),
		matches: in.PersonMatches,
	})
	m.out.Info.People = make([]WorkPerson, len(people.links))
	for i, link := range people.links {
		m.out.Info.People[i] = WorkPerson{Role: link.role, Person: link.ref}
	}
	m.out.NewPeople = people.created
	m.out.Skipped = append(m.out.Skipped, people.skipped...)

	organizations := mergeCreditRoles(m.selected, creditRoleInput[OrganizationRole]{
		roles: OrganizationRoles, field: OrganizationField, limit: WorkInfoLimits.OrganizationsPerRoleMaxCount,
		stored: workLinks(in.Stored.Organizations, func(o WorkOrganization) (OrganizationRole, CreditRef) {
			return o.Role, o.Organization
		}),
		fetched: fetchedLinks(in.Fetched.Organizations, func(o FetchedOrganization) (OrganizationRole, string, []ExternalID) {
			return o.Role, o.Name, o.ExternalIDs
		}),
		matches: in.OrganizationMatches,
	})
	m.out.Info.Organizations = make([]WorkOrganization, len(organizations.links))
	for i, link := range organizations.links {
		m.out.Info.Organizations[i] = WorkOrganization{Role: link.role, Organization: link.ref}
	}
	m.out.NewOrganizations = organizations.created
	m.out.Skipped = append(m.out.Skipped, organizations.skipped...)

	m.appendURLs(in.Fetched.URLs)
	m.appendExternalIDs(in.Fetched.ExternalIDs)
	return m.out
}

type workInfoMerger struct {
	selected func(WorkInfoField) bool
	out      WorkInfoMerge
}

func (m *workInfoMerger) skip(field WorkInfoField, value string, reason SkippedWorkInfoReason) {
	m.out.Skipped = append(m.out.Skipped, SkippedWorkInfoValue{Field: field, Value: value, Reason: reason})
}

func (m *workInfoMerger) replaceText(field WorkInfoField, value string, at func(*WorkInfo) *string) {
	if value == "" || !m.selected(field) {
		return
	}
	var probe WorkInfo
	*at(&probe) = value
	normalized, _, err := NormalizeWorkInfo(probe)
	if err != nil {
		shown := value
		if field == WorkInfoFieldDescription {
			shown = truncateRunes(value, SkippedDescriptionMaxRunes)
		}
		m.skip(field, shown, SkippedWorkInfoInvalid)
		return
	}
	*at(&m.out.Info) = *at(&normalized)
}

func (m *workInfoMerger) replaceDates(dates []WorkDate) {
	for _, kind := range DateKinds {
		i := slices.IndexFunc(dates, func(d WorkDate) bool { return d.Kind == kind })
		if i < 0 || !m.selected(DateField(kind)) {
			continue
		}
		normalized, _, err := NormalizeWorkInfo(WorkInfo{Dates: []WorkDate{dates[i]}})
		if err != nil || len(normalized.Dates) != 1 {
			m.skip(DateField(kind), dates[i].Date, SkippedWorkInfoInvalid)
			continue
		}
		m.out.Info.Dates = slices.DeleteFunc(m.out.Info.Dates, func(d WorkDate) bool { return d.Kind == kind })
		m.out.Info.Dates = append(m.out.Info.Dates, normalized.Dates[0])
	}
	slices.SortStableFunc(m.out.Info.Dates, func(a, b WorkDate) int {
		return slices.Index(DateKinds, a.Kind) - slices.Index(DateKinds, b.Kind)
	})
}

func (m *workInfoMerger) appendURLs(urls []string) {
	if !m.selected(WorkInfoFieldURLs) {
		return
	}
	for _, u := range urls {
		normalized, _, err := NormalizeWorkInfo(WorkInfo{URLs: []string{u}})
		switch {
		case err != nil || len(normalized.URLs) != 1:
			m.skip(WorkInfoFieldURLs, u, SkippedWorkInfoInvalid)
		case slices.Contains(m.out.Info.URLs, normalized.URLs[0]):
		case len(m.out.Info.URLs) >= WorkInfoLimits.URLsMaxCount:
			m.skip(WorkInfoFieldURLs, u, SkippedWorkInfoTooMany)
		default:
			m.out.Info.URLs = append(m.out.Info.URLs, normalized.URLs[0])
		}
	}
}

func (m *workInfoMerger) appendExternalIDs(ids []ExternalID) {
	if !m.selected(WorkInfoFieldExternalIDs) {
		return
	}
	for _, id := range ids {
		shown := id.Source + ": " + id.Value
		normalized, _, err := NormalizeWorkInfo(WorkInfo{ExternalIDs: []ExternalID{id}})
		switch {
		case err != nil || len(normalized.ExternalIDs) != 1:
			m.skip(WorkInfoFieldExternalIDs, shown, SkippedWorkInfoInvalid)
		case slices.Contains(m.out.Info.ExternalIDs, normalized.ExternalIDs[0]):
		case len(m.out.Info.ExternalIDs) >= WorkInfoLimits.ExternalIDsMaxCount:
			m.skip(WorkInfoFieldExternalIDs, shown, SkippedWorkInfoTooMany)
		default:
			m.out.Info.ExternalIDs = append(m.out.Info.ExternalIDs, normalized.ExternalIDs[0])
		}
	}
}

// Clone lists to avoid mutating persisted input arrays during merge.
func cloneWorkInfo(w WorkInfo) WorkInfo {
	w.Dates = slices.Clone(w.Dates)
	w.People = slices.Clone(w.People)
	w.Organizations = slices.Clone(w.Organizations)
	w.URLs = slices.Clone(w.URLs)
	w.ExternalIDs = slices.Clone(w.ExternalIDs)
	return w
}

// created is a NewCredit index or -1 for existing entities; pending creates on append, while absorb collects IDs for an already-planned entity.
type creditLink[R ~string] struct {
	role    R
	ref     CreditRef
	created int
	pending *CreditInput
	absorb  []ExternalID
}

type fetchedCredit[R ~string] struct {
	role R
	name string
	ids  []ExternalID
}

func workLinks[R ~string, W any](stored []W, split func(W) (R, CreditRef)) []creditLink[R] {
	out := make([]creditLink[R], len(stored))
	for i, w := range stored {
		role, ref := split(w)
		out[i] = creditLink[R]{role: role, ref: ref, created: -1}
	}
	return out
}

func fetchedLinks[R ~string, F any](fetched []F, split func(F) (R, string, []ExternalID)) []fetchedCredit[R] {
	out := make([]fetchedCredit[R], len(fetched))
	for i, f := range fetched {
		role, name, ids := split(f)
		out[i] = fetchedCredit[R]{role: role, name: name, ids: ids}
	}
	return out
}

type creditRoleInput[R ~string] struct {
	roles   []R
	field   func(R) WorkInfoField
	limit   int
	stored  []creditLink[R]
	fetched []fetchedCredit[R]
	matches []CreditMatch
}

// Appended link positions match the resulting work-info list positions.
type creditRoleMerge[R ~string] struct {
	links   []creditLink[R]
	created []NewCredit
	skipped []SkippedWorkInfoValue
}

// mergeCreditRoles skips invalid/ambiguous/over-limit entries and coalesces planned entities sharing external IDs or folded names, merging their IDs across roles.
func mergeCreditRoles[R ~string](selected func(WorkInfoField) bool, in creditRoleInput[R]) creditRoleMerge[R] {
	out := creditRoleMerge[R]{links: slices.Clone(in.stored)}
	for _, role := range in.roles {
		field := in.field(role)
		if !selected(field) {
			continue
		}
		count := 0
		for _, link := range out.links {
			if link.role == role {
				count++
			}
		}
		for i, entry := range in.fetched {
			if entry.role != role {
				continue
			}
			match := CreditMatch{State: CreditMatchNew}
			if i < len(in.matches) {
				match = in.matches[i]
			}
			link, reason, keep := out.resolve(entry, match)
			switch {
			case reason != "":
				out.skipped = append(out.skipped, SkippedWorkInfoValue{Field: field, Value: entry.name, Reason: reason})
			case keep && count >= in.limit:
				out.skipped = append(out.skipped, SkippedWorkInfoValue{Field: field, Value: entry.name, Reason: SkippedWorkInfoTooMany})
			default:
				out.mergeIDs(field, link)
				if keep {
					out.add(link)
					count++
				}
			}
		}
	}
	return out
}

func (m *creditRoleMerge[R]) resolve(entry fetchedCredit[R], match CreditMatch) (creditLink[R], SkippedWorkInfoReason, bool) {
	link := creditLink[R]{role: entry.role, created: -1}
	switch match.State {
	case CreditMatchAmbiguous:
		return link, SkippedWorkInfoAmbiguous, false
	case CreditMatchExisting:
		if match.Credit == nil {
			return link, SkippedWorkInfoAmbiguous, false
		}
		link.ref = *match.Credit
	default:
		input, err := NormalizeCredit(CreditInput{Name: entry.name, ExternalIDs: entry.ids})
		if err != nil {
			return link, SkippedWorkInfoInvalid, false
		}
		link.ref = CreditRef{Name: input.Name}
		link.created = m.createdIndex(input)
		if link.created >= 0 {
			link.ref.Name = m.created[link.created].Input.Name
			link.absorb = input.ExternalIDs
		} else {
			// Create only when appending so skipped entries leave no orphan entity.
			link.created, link.pending = len(m.created), &input
		}
	}
	return link, "", !m.linked(link)
}

// createdIndex prefers shared external IDs, then folded-name equality, or -1.
func (m *creditRoleMerge[R]) createdIndex(input CreditInput) int {
	for i, created := range m.created {
		for _, id := range input.ExternalIDs {
			if slices.Contains(created.Input.ExternalIDs, id) {
				return i
			}
		}
	}
	key := FoldForMatch(input.Name)
	return slices.IndexFunc(m.created, func(c NewCredit) bool { return FoldForMatch(c.Input.Name) == key })
}

// mergeIDs skips IDs owned by another planned entity as ambiguous and excess IDs as too_many.
func (m *creditRoleMerge[R]) mergeIDs(field WorkInfoField, link creditLink[R]) {
	if link.pending != nil || link.created < 0 {
		return
	}
	target := &m.created[link.created].Input
	for _, id := range link.absorb {
		shown := id.Source + ": " + id.Value
		switch {
		case slices.Contains(target.ExternalIDs, id):
		case slices.ContainsFunc(m.created, func(c NewCredit) bool { return slices.Contains(c.Input.ExternalIDs, id) }):
			m.skipped = append(m.skipped, SkippedWorkInfoValue{Field: field, Value: shown, Reason: SkippedWorkInfoAmbiguous})
		case len(target.ExternalIDs) >= ExternalIDsMaxCount:
			m.skipped = append(m.skipped, SkippedWorkInfoValue{Field: field, Value: shown, Reason: SkippedWorkInfoTooMany})
		default:
			target.ExternalIDs = append(target.ExternalIDs, id)
		}
	}
}

func (m *creditRoleMerge[R]) linked(link creditLink[R]) bool {
	return slices.ContainsFunc(m.links, func(l creditLink[R]) bool {
		if l.role != link.role {
			return false
		}
		if link.created >= 0 {
			return l.created == link.created
		}
		return l.created < 0 && l.ref.ID == link.ref.ID
	})
}

func (m *creditRoleMerge[R]) add(link creditLink[R]) {
	if link.pending != nil {
		m.created = append(m.created, NewCredit{Input: *link.pending})
		link.pending = nil
	}
	if link.created >= 0 {
		m.created[link.created].Links = append(m.created[link.created].Links, len(m.links))
	}
	m.links = append(m.links, link)
}
