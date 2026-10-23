package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func personPluginUpload(name, digest string, sources ...string) domain.WorkInfoPluginUpload {
	upload := pluginUpload(name, digest)
	upload.Manifest.Kind, upload.Manifest.Sources = domain.WorkInfoPluginKindPerson, sources
	return upload
}

func TestPersonPluginUploadStoresKindSourcesAndDomains(t *testing.T) {
	ctx := context.Background()
	s := migratedDB(t).WorkInfoPlugins()

	savePlugin(t, s, personPluginUpload("people", "d1", "site", "wiki"))
	listed, err := s.ListWorkInfoPlugins(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("List = %+v, %v", listed, err)
	}
	if got := listed[0]; got.Kind() != domain.WorkInfoPluginKindPerson ||
		!slices.Equal(got.Manifest.Sources, []string{"site", "wiki"}) || got.Manifest.Domains != nil {
		t.Fatalf("listed = %+v; want a person plugin with its sources and no domains", got)
	}

	work, replaced := savePlugin(t, s, pluginUpload("people", "d2"))
	if !replaced || work.Kind() != domain.WorkInfoPluginKindWork || work.Manifest.Sources != nil {
		t.Fatalf("work replacement = %+v; want the new kind and no sources", work)
	}
	withDomains := personPluginUpload("people", "d3", "wiki")
	withDomains.Manifest.Domains = []string{"example.com", "example.org"}
	savePlugin(t, s, withDomains)
	listed, err = s.ListWorkInfoPlugins(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("List = %+v, %v", listed, err)
	}
	if person := listed[0]; person.Kind() != domain.WorkInfoPluginKindPerson ||
		!slices.Equal(person.Manifest.Sources, []string{"wiki"}) ||
		!slices.Equal(person.Manifest.Domains, []string{"example.com", "example.org"}) {
		t.Fatalf("person replacement = %+v; want the new sources and domains", person)
	}
}

func approvedPersonPlugin(t *testing.T, db *DB) domain.WorkInfoPlugin {
	t.Helper()
	added, _ := savePlugin(t, db.WorkInfoPlugins(), personPluginUpload("people", "d1", "wiki"))
	plugin, err := db.WorkInfoPlugins().ApproveWorkInfoPlugin(context.Background(), added.ID, "d1", pluginNow)
	if err != nil {
		t.Fatalf("ApproveWorkInfoPlugin: %v", err)
	}
	return plugin
}

func TestApplyPersonPluginFetchSkipsAnIDAnotherPersonHolds(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	plugin := approvedPersonPlugin(t, db)
	taken := domain.ExternalID{Source: "wiki", Value: "taken"}
	holder := createCredit(t, db, domain.CreditPerson,
		domain.CreditInput{Name: "Holder", ExternalIDs: []domain.ExternalID{taken}})
	target := createPerson(t, db, "Target")

	applied, err := db.Credits().ApplyPersonPluginFetch(ctx, domain.PersonPluginApply{
		PersonID: target.ID, PluginID: plugin.ID, Digest: plugin.Digest, Generation: plugin.Generation,
		Fetched: domain.FetchedPluginPerson{
			Name: "Target", ExternalIDs: []domain.ExternalID{taken, {Source: "wiki", Value: "new"}},
			Profile: domain.PersonProfile{Birthplace: "Tokyo"},
		},
		Items: []domain.PersonPluginItem{domain.PersonItemExternalIDs, domain.PersonProfileItem(domain.ProfileBirthplace)},
	})
	if err != nil {
		t.Fatalf("ApplyPersonPluginFetch: %v", err)
	}
	if applied.Person.ID != target.ID || applied.Person.Profile.Birthplace != "Tokyo" ||
		!slices.Equal(applied.Person.ExternalIDs, []domain.ExternalID{{Source: "wiki", Value: "new"}}) {
		t.Fatalf("person = %+v; want the target with only the free ID and the birthplace", applied.Person)
	}
	want := []domain.SkippedPersonValue{{
		Item: domain.PersonItemExternalIDs, Value: "wiki: taken", Reason: domain.SkippedWorkInfoTaken, HolderName: "Holder",
	}}
	if !slices.Equal(applied.Skipped, want) {
		t.Fatalf("skipped = %+v; want %+v", applied.Skipped, want)
	}
	kept, err := db.Credits().Get(ctx, domain.CreditPerson, holder.ID)
	if err != nil || !slices.Equal(kept.ExternalIDs, []domain.ExternalID{taken}) {
		t.Fatalf("holder = %+v, %v; want it kept, unmerged, with its ID", kept, err)
	}
}

func TestApplyPersonPluginFetchReplacesAStoredProfileField(t *testing.T) {
	db := migratedDB(t)
	plugin := approvedPersonPlugin(t, db)
	target := createCredit(t, db, domain.CreditPerson,
		domain.CreditInput{Name: "Target", Profile: domain.PersonProfile{Birthplace: "Osaka"}})

	applied, err := db.Credits().ApplyPersonPluginFetch(context.Background(), domain.PersonPluginApply{
		PersonID: target.ID, PluginID: plugin.ID, Digest: plugin.Digest, Generation: plugin.Generation,
		Fetched: domain.FetchedPluginPerson{Name: "Target", Profile: domain.PersonProfile{Birthplace: "Tokyo"}},
		Items:   []domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileBirthplace)},
	})
	if err != nil {
		t.Fatalf("ApplyPersonPluginFetch: %v", err)
	}
	if applied.Person.Profile.Birthplace != "Tokyo" {
		t.Fatalf("birthplace = %q; want the fetched value over the stored one", applied.Person.Profile.Birthplace)
	}
}

func TestApplyPersonPluginFetchWritesNothingForAChangedPlugin(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	plugin := approvedPersonPlugin(t, db)
	target := createPerson(t, db, "Target")
	if _, err := db.WorkInfoPlugins().RevokeWorkInfoPlugin(ctx, plugin.ID, pluginNow); err != nil {
		t.Fatal(err)
	}

	_, err := db.Credits().ApplyPersonPluginFetch(ctx, domain.PersonPluginApply{
		PersonID: target.ID, PluginID: plugin.ID, Digest: plugin.Digest, Generation: plugin.Generation,
		Fetched: domain.FetchedPluginPerson{Name: "Target", Profile: domain.PersonProfile{Birthplace: "Tokyo"}},
		Items:   []domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileBirthplace)},
	})
	if !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("err = %v; want ErrWorkInfoFetchStale", err)
	}
	if stored, _ := db.Credits().Get(ctx, domain.CreditPerson, target.ID); stored.Profile.Birthplace != "" {
		t.Fatalf("birthplace = %q; want nothing written", stored.Profile.Birthplace)
	}
}
