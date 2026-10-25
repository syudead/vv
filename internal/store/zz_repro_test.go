package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestZZRepro(t *testing.T) {
	db := migratedDB(t)
	plugin := approvedPersonPlugin(t, db)
	var ids []domain.ExternalID
	var links []string
	for i := range 9 {
		ids = append(ids, domain.ExternalID{Source: "wiki", Value: fmt.Sprint("v", i)})
		links = append(links, fmt.Sprintf("https://e.test/%d", i))
	}
	target := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "T", ExternalIDs: ids, Profile: domain.PersonProfile{Links: links}})
	got, _ := db.Credits().Get(context.Background(), domain.CreditPerson, target.ID)
	t.Logf("stored ids=%d links=%d", len(got.ExternalIDs), len(got.Profile.Links))
	fetched := append(append([]domain.ExternalID{}, ids...), domain.ExternalID{Source: "wiki", Value: "new"})
	applied, err := db.Credits().ApplyPersonPluginFetch(context.Background(), domain.PersonPluginApply{
		PersonID: target.ID, PluginID: plugin.ID, Digest: plugin.Digest, Generation: plugin.Generation,
		Fetched: domain.FetchedPluginPerson{Name: "T", ExternalIDs: fetched, Profile: domain.PersonProfile{Links: append(links, "https://e.test/new")}},
		Items:   []domain.PersonPluginItem{domain.PersonItemExternalIDs, domain.PersonProfileItem(domain.ProfileLinks)},
	})
	t.Logf("err=%v skipped=%+v ids=%d links=%d", err, applied.Skipped, len(applied.Person.ExternalIDs), len(applied.Person.Profile.Links))
}
