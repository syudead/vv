package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// applyLookups applies the performers role of fetched with lookups run on the person plugin.
func applyLookups(
	t *testing.T, db *DB, videoID int64, people []domain.FetchedPerson, person domain.WorkInfoPlugin,
	lookups ...domain.FetchedPersonLookupApply,
) domain.FetchedWorkInfoApplied {
	t.Helper()
	work := approvedPlugin(t, db, "details", "dd")
	for i := range lookups {
		lookups[i].Index, lookups[i].PluginID = i, person.ID
		lookups[i].Digest, lookups[i].Generation = person.Digest, person.Generation
	}
	applied, err := db.Overrides().ApplyFetchedWorkInfo(context.Background(), domain.FetchedWorkInfoApply{
		VideoID: videoID, PluginID: work.ID, Digest: "dd", Generation: work.Generation,
		Details: domain.FetchedWorkInfo{People: people}, DetailFields: []domain.WorkInfoField{performerField},
		PersonLookups: lookups,
	})
	if err != nil {
		t.Fatalf("ApplyFetchedWorkInfo: %v", err)
	}
	return applied
}

func birthplaceLookup(entry int, matched int64, birthplace string) domain.FetchedPersonLookupApply {
	return domain.FetchedPersonLookupApply{
		Entries: []int{entry}, MatchedID: matched,
		Fetched: domain.FetchedPluginPerson{Name: "fetched", Profile: domain.PersonProfile{Birthplace: birthplace}},
		Items:   []domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileBirthplace)},
	}
}

func birthplaceOf(t *testing.T, db *DB, id int64) string {
	t.Helper()
	person, err := db.Credits().Get(context.Background(), domain.CreditPerson, id)
	if err != nil {
		t.Fatal(err)
	}
	return person.Profile.Birthplace
}

func TestApplyFetchedLookupsFillTheMatchedAndTheCreatedPerson(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	plugin := approvedPersonPlugin(t, db)
	aoi := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "a"}},
	})
	fetchedAoi := fetchedPerformer("Aoi", domain.ExternalID{Source: "wiki", Value: "a"})
	// The work-info profile fills what the lookup left empty, after it.
	fetchedAoi.Profile = domain.PersonProfile{Birthplace: "Work", Cup: "C"}
	aoiLookup := birthplaceLookup(0, aoi.ID, "Lookup")
	aoiLookup.Fetched.Aliases = []domain.Alias{{Name: "Aoichan"}}
	aoiLookup.Items = append(aoiLookup.Items, domain.PersonItemAliases)

	applied := applyLookups(t, db, alpha, []domain.FetchedPerson{
		fetchedAoi, fetchedPerformer("Mika", domain.ExternalID{Source: "wiki", Value: "m"}),
	}, plugin, aoiLookup, birthplaceLookup(1, 0, "Osaka"))

	mika := applied.PersonIDs[1]
	if want := []int64{aoi.ID, mika}; !reflect.DeepEqual(applied.LookupPersonIDs, want) || mika == 0 {
		t.Fatalf("lookup people = %v; want %v", applied.LookupPersonIDs, want)
	}
	stored, err := db.Credits().Get(context.Background(), domain.CreditPerson, aoi.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Profile.Birthplace != "Lookup" || stored.Profile.Cup != "C" || len(stored.Aliases) != 1 ||
		stored.Aliases[0].Name != "Aoichan" {
		t.Fatalf("Aoi = %+v; want the lookup's birthplace and alias, and the work-info cup", stored)
	}
	if got := birthplaceOf(t, db, mika); got != "Osaka" {
		t.Fatalf("created person's birthplace = %q; want the lookup's", got)
	}
	if len(applied.Skipped) != 0 {
		t.Fatalf("skipped = %+v; want none", applied.Skipped)
	}
}

func TestApplyFetchedLookupsSkipAPersonMergedOrDeletedSinceTheFetch(t *testing.T) {
	ctx := context.Background()
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	plugin := approvedPersonPlugin(t, db)
	target := createPerson(t, db, "Target")
	merged := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Merged", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "merged"}},
	})
	deleted := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Deleted", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "deleted"}},
	})
	kept := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Kept", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "kept"}},
	})
	if _, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{merged.ID}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if err := db.Credits().Delete(ctx, domain.CreditPerson, deleted.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	applied := applyLookups(t, db, alpha, []domain.FetchedPerson{
		fetchedPerformer("Merged", domain.ExternalID{Source: "wiki", Value: "merged"}),
		fetchedPerformer("Deleted", domain.ExternalID{Source: "wiki", Value: "deleted"}),
		fetchedPerformer("Kept", domain.ExternalID{Source: "wiki", Value: "kept"}),
	}, plugin, birthplaceLookup(0, merged.ID, "A"), birthplaceLookup(1, deleted.ID, "B"),
		birthplaceLookup(2, kept.ID, "C"))

	if applied.PersonIDs[0] != target.ID || applied.PersonIDs[1] == 0 {
		t.Fatalf("linked people = %v; want the merge target and a new person", applied.PersonIDs)
	}
	if got := birthplaceOf(t, db, target.ID) + birthplaceOf(t, db, applied.PersonIDs[1]); got != "" {
		t.Fatalf("birthplaces of the merge target and the new person = %q; want nothing written", got)
	}
	if got := birthplaceOf(t, db, kept.ID); got != "C" {
		t.Fatalf("kept person's birthplace = %q; want the rest applied", got)
	}
	first, second := 0, 1
	want := []domain.SkippedWorkInfoValue{
		{Field: performerField, Value: "fetched", Reason: domain.SkippedWorkInfoPersonChanged, Lookup: &first},
		{Field: performerField, Value: "fetched", Reason: domain.SkippedWorkInfoPersonChanged, Lookup: &second},
	}
	if !reflect.DeepEqual(applied.Skipped, want) || !reflect.DeepEqual(applied.LookupPersonIDs, []int64{0, 0, kept.ID}) {
		t.Fatalf("skipped = %+v, lookup people = %v; want both changed people skipped", applied.Skipped,
			applied.LookupPersonIDs)
	}
}

func TestApplyFetchedLookupsSkipALookupWhosePluginWasRevoked(t *testing.T) {
	db, ids := overrideFixture(t)
	alpha := ids[fixturePath("/media/alpha.mp4")]
	plugin := approvedPersonPlugin(t, db)
	aoi := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "a"}},
	})
	if _, err := db.WorkInfoPlugins().RevokeWorkInfoPlugin(context.Background(), plugin.ID, pluginNow); err != nil {
		t.Fatal(err)
	}

	applied := applyLookups(t, db, alpha, []domain.FetchedPerson{
		fetchedPerformer("Aoi", domain.ExternalID{Source: "wiki", Value: "a"}),
	}, plugin, birthplaceLookup(0, aoi.ID, "Tokyo"))

	if got := performerIDs(applied.Video.WorkInfo); len(got) != 1 || got[0] != aoi.ID {
		t.Fatalf("performers = %v; want the role applied", got)
	}
	first := 0
	want := []domain.SkippedWorkInfoValue{
		{Field: performerField, Value: "fetched", Reason: domain.SkippedWorkInfoStale, Lookup: &first},
	}
	if !reflect.DeepEqual(applied.Skipped, want) || birthplaceOf(t, db, aoi.ID) != "" {
		t.Fatalf("skipped = %+v; want the lookup skipped as stale and nothing written", applied.Skipped)
	}
}
