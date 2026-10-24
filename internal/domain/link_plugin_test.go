package domain

import (
	"errors"
	"slices"
	"testing"
)

func linkPlugin(id int64, rank int, domains ...string) WorkInfoPlugin {
	return WorkInfoPlugin{
		ID: id, Digest: "d", ApprovedDigest: "d", LinkRank: rank,
		Manifest: WorkInfoPluginManifest{Kind: WorkInfoPluginKindWork, Operations: []WorkInfoOperation{WorkInfoFetch}, Domains: domains},
	}
}

func pluginIDs(plugins []WorkInfoPlugin) []int64 {
	var ids []int64
	for _, p := range plugins {
		ids = append(ids, p.ID)
	}
	return ids
}

// The owner's rank orders the matching plugins; an unapproved one, one missing a required setting, a person plugin
// and a plugin for another site never run.
func TestLinkPluginsForKeepsUsableMatchesInRankOrder(t *testing.T) {
	builtin := linkPlugin(1, 3, AnyLinkDomain)
	site := linkPlugin(2, 1, "example.com")
	later := linkPlugin(3, 2, "example.com")
	unapproved := linkPlugin(4, 0, "example.com")
	unapproved.ApprovedDigest = ""
	unset := linkPlugin(5, 0, "example.com")
	unset.Manifest.Settings = []WorkInfoPluginSetting{{Key: "token", Required: true}}
	person := linkPlugin(6, 0, "example.com")
	person.Manifest.Kind = WorkInfoPluginKindPerson
	other := linkPlugin(7, 0, "other.test")
	all := []WorkInfoPlugin{builtin, site, later, unapproved, unset, person, other}

	if got := pluginIDs(LinkPluginsFor(all, "https://www.example.com/v/1")); !slices.Equal(got, []int64{2, 3, 1}) {
		t.Fatalf("plugins for an example.com link = %v; want [2 3 1]", got)
	}
	if got := pluginIDs(LinkPluginsFor(all, "https://another.test/v")); !slices.Equal(got, []int64{1}) {
		t.Fatalf("plugins for another site = %v; want only the bundled one", got)
	}
}

func TestCheckLinkOrderNeedsEachLinkPluginOnce(t *testing.T) {
	plugins := []WorkInfoPlugin{linkPlugin(1, 1, AnyLinkDomain), linkPlugin(2, 2, "example.com"), linkPlugin(3, 0)}
	if err := CheckLinkOrder(plugins, []int64{2, 1}); err != nil {
		t.Fatalf("CheckLinkOrder(2, 1) = %v; want accepted", err)
	}
	for _, order := range [][]int64{{1}, {2, 1, 3}, {2, 2}} {
		if err := CheckLinkOrder(plugins, order); !errors.Is(err, ErrLinkOrderChanged) {
			t.Errorf("CheckLinkOrder(%v) = %v; want ErrLinkOrderChanged", order, err)
		}
	}
}

func TestEmptyWorkInfoFieldsDropsFieldsWithAStoredValue(t *testing.T) {
	stored := WorkInfo{Description: "owner's", URLs: []string{"https://e.test"}, Dates: []WorkDate{{Kind: DateRelease, Date: "2020-01-01"}}}
	fields := []WorkInfoField{WorkInfoFieldCode, WorkInfoFieldDescription, DateField(DateRelease), DateField(DatePublished), WorkInfoFieldURLs}
	got := EmptyWorkInfoFields(stored, fields)
	if want := []WorkInfoField{WorkInfoFieldCode, DateField(DatePublished)}; !slices.Equal(got, want) {
		t.Fatalf("EmptyWorkInfoFields = %v; want %v", got, want)
	}
}
