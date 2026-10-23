package wasmplugin

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestPersonFetchReturnsThePersonAndItsImages(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	// The probe picks its mode from the fetched ID and prints the person setting as the person.
	req := domain.WorkInfoRunRequest{
		Operation: domain.WorkInfoFetch, Kind: domain.WorkInfoPluginKindPerson, Source: "s", ID: "person",
		Sources: []string{"s"},
		Settings: map[string]string{"person": `{"name":"Someone",` +
			`"images":["http://site.test/face/1","http://site.test/missing","http://site.test/face/2"]}`},
	}

	got, _, err := rt.Run(context.Background(), probeDigest, probeModule(t), req, domain.WorkInfoRunNetwork{}, nil)
	images := got.PersonPluginImages
	if err != nil || got.Person.Name != "Someone" || len(images) != 3 || string(images[0]) != "face 1" ||
		images[1] != nil || string(images[2]) != "face 2" {
		t.Fatalf("Run(person) = %+v, images %q, %v; want Someone with faces 1 and 2 around a dropped one",
			got.Person, images, err)
	}
	if ids := got.Person.ExternalIDs; len(ids) != 1 || ids[0] != (domain.ExternalID{Source: "s", Value: "person"}) {
		t.Fatalf("external IDs = %v, want the requested s:person", ids)
	}
}

func TestPersonFetchByLinkSendsTheURLAndPrependsNoID(t *testing.T) {
	rt := newNetTestRuntime(t, newTestSite(t))
	link := "https://www.example.com/p/1?tab=info#top"
	// The probe names the person by the url it received and returns one ID of its own.
	req := domain.WorkInfoRunRequest{
		Operation: domain.WorkInfoFetch, Kind: domain.WorkInfoPluginKindPerson, URL: link, Sources: []string{"s"},
	}

	got, _, err := rt.Run(context.Background(), probeDigest, probeModule(t), req, domain.WorkInfoRunNetwork{}, nil)
	if err != nil || got.Person.Name != link {
		t.Fatalf("Run(person by link) = %+v, %v; want the person named by the link sent as url", got.Person, err)
	}
	if ids := got.Person.ExternalIDs; len(ids) != 1 || ids[0] != (domain.ExternalID{Source: "s", Value: "from-link"}) {
		t.Fatalf("external IDs = %v, want only the plugin's s:from-link", ids)
	}
}
