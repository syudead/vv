package domain

import (
	"errors"
	"slices"
)

// A work-info fetch plans one person plugin lookup per fetched person whose external ID an enabled person plugin
// accepts; the page runs each one and the apply fills the linked person (docs/design-docs/person-plugins.md).

// PersonLookupsMaxCount is how many distinct lookups one work-info fetch plans, the first in result order.
const PersonLookupsMaxCount = 5

// ErrPersonLookupPluginChanged reports a lookup whose plugin was replaced, revoked or deleted since the fetch.
var ErrPersonLookupPluginChanged = errors.New("the person plugin changed since the work information was fetched")

const (
	// SkippedWorkInfoStale skips a lookup whose plugin changed since it ran.
	SkippedWorkInfoStale SkippedWorkInfoReason = "stale"
	// SkippedWorkInfoPersonChanged skips a lookup whose entry now links another person than it matched at fetch time.
	SkippedWorkInfoPersonChanged SkippedWorkInfoReason = "person_changed"
)

// PersonLookup is one planned run of a person plugin's fetch; every entry in Entries matched MatchedID at fetch
// time, 0 for none.
type PersonLookup struct {
	PluginID   int64
	PluginName string
	Digest     string
	Generation int64
	Source     string
	ID         string
	Entries    []int
	MatchedID  int64
}

// PersonLookupPlan aligns EntryLookups and Limited with the fetched people: the index into Lookups or -1, and
// whether an accepted ID was over PersonLookupsMaxCount.
type PersonLookupPlan struct {
	Lookups      []PersonLookup
	EntryLookups []int
	Limited      []bool
}

// PlanPersonLookups picks, for each person in result order, the first fetched ID that an approved person plugin with
// automatic lookups accepts, taking plugins in the order given (name, then file name). Entries with the same plugin,
// ID and fetch-time match share a lookup; an ambiguous entry gets none. matches aligns with people.
func PlanPersonLookups(people []FetchedPerson, matches []CreditMatch, plugins []WorkInfoPlugin) PersonLookupPlan {
	enabled := slices.DeleteFunc(slices.Clone(plugins), func(p WorkInfoPlugin) bool {
		return p.Kind() != WorkInfoPluginKindPerson || p.State() != WorkInfoPluginApproved || !p.AutoLookup
	})
	plan := PersonLookupPlan{EntryLookups: make([]int, len(people)), Limited: make([]bool, len(people))}
	for i, person := range people {
		plan.EntryLookups[i] = -1
		matched, ok := lookupMatch(matches, i)
		if !ok {
			continue
		}
		lookup, ok := firstPersonLookup(person.ExternalIDs, enabled)
		if !ok {
			continue
		}
		lookup.MatchedID = matched
		at := slices.IndexFunc(plan.Lookups, func(l PersonLookup) bool {
			return l.PluginID == lookup.PluginID && l.Source == lookup.Source && l.ID == lookup.ID &&
				l.MatchedID == lookup.MatchedID
		})
		switch {
		case at >= 0:
			plan.Lookups[at].Entries = append(plan.Lookups[at].Entries, i)
			plan.EntryLookups[i] = at
		case len(plan.Lookups) >= PersonLookupsMaxCount:
			plan.Limited[i] = true
		default:
			lookup.Entries = []int{i}
			plan.EntryLookups[i] = len(plan.Lookups)
			plan.Lookups = append(plan.Lookups, lookup)
		}
	}
	return plan
}

// lookupMatch returns the person an entry matched at fetch time, 0 for a new one; an ambiguous entry links no one.
func lookupMatch(matches []CreditMatch, i int) (int64, bool) {
	if i >= len(matches) {
		return 0, true
	}
	switch match := matches[i]; {
	case match.State == CreditMatchAmbiguous:
		return 0, false
	case match.State == CreditMatchExisting && match.Credit != nil:
		return match.Credit.ID, true
	}
	return 0, true
}

func firstPersonLookup(ids []ExternalID, plugins []WorkInfoPlugin) (PersonLookup, bool) {
	for _, raw := range ids {
		id, ok := NormalizeExternalID(raw)
		if !ok {
			continue
		}
		for _, plugin := range plugins {
			if plugin.Manifest.DeclaresSource(id.Source) {
				return PersonLookup{
					PluginID: plugin.ID, PluginName: plugin.Manifest.Name, Digest: plugin.Digest,
					Generation: plugin.Generation, Source: id.Source, ID: id.Value,
				}, true
			}
		}
	}
	return PersonLookup{}, false
}

// PersonLookupResult is a run lookup as the comparison sees it; Current is the person every entry matched at fetch
// time as stored now, or nil when they matched none or that person is gone.
type PersonLookupResult struct {
	Person   FetchedPluginPerson
	HasImage bool
	Current  *Credit
}

// PersonLookupSelection names the items the owner selected from planned lookup Index.
type PersonLookupSelection struct {
	Index int
	PersonPluginSelection
}

// FetchedPersonLookupApply carries a run lookup's selected items to the apply, which fills the person its entries
// link only when that is MatchedID, or a person the apply creates when MatchedID is 0.
type FetchedPersonLookupApply struct {
	Index      int
	PluginID   int64
	Digest     string
	Generation int64
	Entries    []int
	MatchedID  int64
	Fetched    FetchedPluginPerson
	Items      []PersonPluginItem
}
