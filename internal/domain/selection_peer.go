package domain

import (
	"cmp"
	"slices"
	"strings"
)

// The summaries of an operation over a selection of both nodes merge by entity. The peer's entries arrive with
// the local id of the same-named entity, or the offset when this node has none, so equal ids are equal names;
// a provisional person keeps the offset and stays apart.

// MergeTagSummaries adds the peer's tag counts to the local ones in natural-name order. A video belongs to one
// node, so the sums count each video once.
func MergeTagSummaries(local, peer TagSummary) TagSummary {
	return TagSummary{Total: local.Total + peer.Total, Items: mergeTagSummaryItems(local.Items, peer.Items)}
}

func mergeTagSummaryItems(local, peer []TagSummaryItem) []TagSummaryItem {
	out := slices.Clone(local)
	index := make(map[int64]int, len(out))
	for i, item := range out {
		index[item.Tag.ID] = i
	}
	for _, item := range peer {
		if i, ok := index[item.Tag.ID]; ok {
			out[i].Count += item.Count
			out[i].ManualCount += item.ManualCount
			continue
		}
		index[item.Tag.ID] = len(out)
		out = append(out, item)
	}
	slices.SortFunc(out, func(a, b TagSummaryItem) int {
		return cmp.Or(CompareNatural(a.Tag.Name, b.Tag.Name), strings.Compare(a.Tag.Name, b.Tag.Name), cmp.Compare(a.Tag.ID, b.Tag.ID))
	})
	if out == nil {
		out = []TagSummaryItem{}
	}
	return out
}

// MergeVideoEditSummaries adds the peer's summary to the local one, keeping the orders SummarizeWorkInfos gives;
// a date kind keeps its date only when every counted video of both nodes has it.
func MergeVideoEditSummaries(local, peer VideoEditSummary) VideoEditSummary {
	out := VideoEditSummary{
		Total: local.Total + peer.Total, Public: local.Public + peer.Public,
		Tags:          mergeTagSummaryItems(local.Tags, peer.Tags),
		People:        mergeCounts(local.People, peer.People, personCountKey, addPersonCounts),
		Organizations: mergeCounts(local.Organizations, peer.Organizations, organizationCountKey, addOrganizationCounts),
		Dates:         mergeDateCounts(local.Dates, peer.Dates),
	}
	slices.SortFunc(out.People, func(a, b VideoEditPersonCount) int {
		return cmp.Or(slices.Index(PersonRoles, a.Role)-slices.Index(PersonRoles, b.Role), CompareCreditRefs(a.Person, b.Person))
	})
	slices.SortFunc(out.Organizations, func(a, b VideoEditOrganizationCount) int {
		return cmp.Or(slices.Index(OrganizationRoles, a.Role)-slices.Index(OrganizationRoles, b.Role),
			CompareCreditRefs(a.Organization, b.Organization))
	})
	return out
}

type creditCountKey struct {
	role string
	id   int64
}

func personCountKey(c VideoEditPersonCount) creditCountKey {
	return creditCountKey{role: string(c.Role), id: c.Person.ID}
}

func organizationCountKey(c VideoEditOrganizationCount) creditCountKey {
	return creditCountKey{role: string(c.Role), id: c.Organization.ID}
}

func addPersonCounts(into *VideoEditPersonCount, from VideoEditPersonCount) { into.Count += from.Count }

func addOrganizationCounts(into *VideoEditOrganizationCount, from VideoEditOrganizationCount) {
	into.Count += from.Count
}

// mergeCounts appends the peer's entries to the local ones, adding an entry with the same key into the local one.
func mergeCounts[T any](local, peer []T, key func(T) creditCountKey, add func(*T, T)) []T {
	out := slices.Clone(local)
	index := make(map[creditCountKey]int, len(out))
	for i, item := range out {
		index[key(item)] = i
	}
	for _, item := range peer {
		if i, ok := index[key(item)]; ok {
			add(&out[i], item)
			continue
		}
		index[key(item)] = len(out)
		out = append(out, item)
	}
	if out == nil {
		out = []T{}
	}
	return out
}

func mergeDateCounts(local, peer []VideoEditDateCount) []VideoEditDateCount {
	out := []VideoEditDateCount{}
	for _, kind := range DateKinds {
		merged := VideoEditDateCount{Kind: kind}
		for _, side := range [][]VideoEditDateCount{local, peer} {
			i := slices.IndexFunc(side, func(d VideoEditDateCount) bool { return d.Kind == kind })
			if i < 0 || side[i].Count == 0 {
				continue
			}
			if merged.Count > 0 && merged.Date != side[i].Date {
				merged.Date = ""
			} else if merged.Count == 0 {
				merged.Date = side[i].Date
			}
			merged.Count += side[i].Count
		}
		if merged.Count > 0 {
			out = append(out, merged)
		}
	}
	return out
}
