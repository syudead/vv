package wasmplugin

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestPersonFetchReturnsThePersonAndItsImage(t *testing.T) {
	site := newTestSite(t)
	rt := newNetTestRuntime(t, site)
	// The probe picks its mode from the fetched ID and prints the person setting as the person.
	req := domain.WorkInfoRunRequest{
		Operation: domain.WorkInfoFetch, Kind: domain.WorkInfoPluginKindPerson, Source: "s", ID: "person",
		Sources:  []string{"s"},
		Settings: map[string]string{"person": `{"name":"Someone","image":"http://site.test/face/1"}`},
	}

	got, _, err := rt.Run(context.Background(), probeDigest, probeModule(t), req, domain.WorkInfoRunNetwork{}, nil)
	if err != nil || got.Person.Name != "Someone" || string(got.PersonImage) != "face 1" {
		t.Fatalf("Run(person) = %+v, image %q, %v; want Someone with face 1", got.Person, got.PersonImage, err)
	}
	if ids := got.Person.ExternalIDs; len(ids) != 1 || ids[0] != (domain.ExternalID{Source: "s", Value: "person"}) {
		t.Fatalf("external IDs = %v, want the requested s:person", ids)
	}
}
