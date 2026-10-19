package domain

import (
	"fmt"
	"slices"
	"testing"
)

func lookupPlugin(id int64, name string, sources ...string) WorkInfoPlugin {
	return WorkInfoPlugin{
		ID: id, Manifest: WorkInfoPluginManifest{Name: name, Kind: WorkInfoPluginKindPerson, Sources: sources},
		Digest: "d", ApprovedDigest: "d", Generation: 1, AutoLookup: true,
	}
}

func lookupPerson(name string, ids ...ExternalID) FetchedPerson {
	return FetchedPerson{Role: PersonPerformer, Name: name, ExternalIDs: ids}
}

func existingMatch(id int64) CreditMatch {
	return CreditMatch{State: CreditMatchExisting, Credit: &CreditRef{ID: id}}
}

// planned summarizes a plan as "entry: plugin source:id matched" lines, "-" for no lookup and "limit" over the limit.
func planned(plan PersonLookupPlan) []string {
	out := make([]string, len(plan.EntryLookups))
	for i, at := range plan.EntryLookups {
		switch {
		case plan.Limited[i]:
			out[i] = "limit"
		case at < 0:
			out[i] = "-"
		default:
			l := plan.Lookups[at]
			out[i] = fmt.Sprintf("#%d %d %s:%s m%d", at, l.PluginID, l.Source, l.ID, l.MatchedID)
		}
	}
	return out
}

func TestPlanPersonLookups(t *testing.T) {
	wiki, fanza := ExternalID{Source: "wiki", Value: "1"}, ExternalID{Source: "fanza", Value: "f"}
	disabled := lookupPlugin(3, "a disabled", "wiki")
	disabled.AutoLookup = false
	unapproved := lookupPlugin(4, "a unapproved", "wiki")
	unapproved.ApprovedDigest = ""
	plugins := []WorkInfoPlugin{disabled, unapproved, lookupPlugin(1, "b first", "wiki"), lookupPlugin(2, "c second", "wiki", "fanza")}
	tests := []struct {
		name    string
		people  []FetchedPerson
		matches []CreditMatch
		want    []string
	}{
		{
			name:   "an unaccepted ID or only a name gets no lookup",
			people: []FetchedPerson{lookupPerson("A", ExternalID{Source: "other", Value: "1"}), lookupPerson("B")},
			want:   []string{"-", "-"},
		},
		{
			name:   "the first accepted ID and the first enabled plugin in list order",
			people: []FetchedPerson{lookupPerson("A", ExternalID{Source: "other", Value: "1"}, fanza, wiki)},
			want:   []string{"#0 2 fanza:f m0"},
		},
		{
			name:   "two plugins on one source pick the first, skipping disabled and unapproved ones",
			people: []FetchedPerson{lookupPerson("A", wiki)},
			want:   []string{"#0 1 wiki:1 m0"},
		},
		{
			name:    "a shared ID matched to different people gives two lookups, and the same match shares one",
			people:  []FetchedPerson{lookupPerson("A", wiki), lookupPerson("B", wiki), lookupPerson("C", wiki)},
			matches: []CreditMatch{existingMatch(7), existingMatch(8), existingMatch(7)},
			want:    []string{"#0 1 wiki:1 m7", "#1 1 wiki:1 m8", "#0 1 wiki:1 m7"},
		},
		{
			name:    "an ambiguous entry gets none",
			people:  []FetchedPerson{lookupPerson("A", wiki)},
			matches: []CreditMatch{{State: CreditMatchAmbiguous}},
			want:    []string{"-"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planned(PlanPersonLookups(tt.people, tt.matches, plugins))
			if !slices.Equal(got, tt.want) {
				t.Fatalf("plan = %q; want %q", got, tt.want)
			}
		})
	}
}

func TestPlanPersonLookupsLimitsDistinctLookups(t *testing.T) {
	var people []FetchedPerson
	for i := range PersonLookupsMaxCount + 1 {
		people = append(people, lookupPerson("P", ExternalID{Source: "wiki", Value: fmt.Sprint(i)}))
	}
	people = append(people, lookupPerson("again", ExternalID{Source: "wiki", Value: "0"}))

	plan := PlanPersonLookups(people, nil, []WorkInfoPlugin{lookupPlugin(1, "p", "wiki")})

	if len(plan.Lookups) != PersonLookupsMaxCount {
		t.Fatalf("lookups = %d; want %d", len(plan.Lookups), PersonLookupsMaxCount)
	}
	if !plan.Limited[PersonLookupsMaxCount] || plan.EntryLookups[PersonLookupsMaxCount] != -1 {
		t.Fatalf("sixth entry = lookup %d, limited %v; want limited", plan.EntryLookups[PersonLookupsMaxCount],
			plan.Limited[PersonLookupsMaxCount])
	}
	if last := len(people) - 1; plan.Limited[last] || plan.EntryLookups[last] != 0 {
		t.Fatalf("entry sharing the first lookup = lookup %d, limited %v; want lookup 0", plan.EntryLookups[last],
			plan.Limited[last])
	}
	if got := plan.Lookups[0].Entries; !slices.Equal(got, []int{0, len(people) - 1}) {
		t.Fatalf("first lookup entries = %v; want both entries sharing it", got)
	}
}
