package app

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	fetchPerson  = int64(3)
	otherPerson  = int64(4)
	linkedPerson = int64(5)
)

// fakePluginPeople rechecks the plugin like the store's transaction, unless the peer fetched, and adds the applied external IDs; a person absent
// from people does not exist.
type fakePluginPeople struct {
	plugins *runPluginStore
	people  map[int64]domain.Credit
	applies []domain.PersonPluginApply
}

func (f *fakePluginPeople) Get(_ context.Context, _ domain.CreditKind, id int64) (domain.Credit, error) {
	person, ok := f.people[id]
	if !ok {
		return domain.Credit{}, domain.ErrNotFound
	}
	return person, nil
}

func (f *fakePluginPeople) ApplyPersonPluginFetch(
	ctx context.Context, apply domain.PersonPluginApply,
) (domain.PersonPluginApplied, error) {
	f.applies = append(f.applies, apply)
	if !apply.FetchedByPeer {
		plugin, err := f.plugins.WorkInfoPlugin(ctx, apply.PluginID)
		if err != nil || plugin.State() != domain.WorkInfoPluginApproved || plugin.Digest != apply.Digest ||
			plugin.Generation != apply.Generation {
			return domain.PersonPluginApplied{}, domain.ErrWorkInfoFetchStale
		}
	}
	person, ok := f.people[apply.PersonID]
	if !ok {
		return domain.PersonPluginApplied{}, domain.ErrNotFound
	}
	person.ExternalIDs = append(slices.Clone(person.ExternalIDs), apply.Fetched.ExternalIDs...)
	f.people[apply.PersonID] = person
	return domain.PersonPluginApplied{Person: person, Skipped: []domain.SkippedPersonValue{}}, nil
}

type filledFace struct {
	id   int64
	data []byte
}

// recordingFaces stores every face it is given at revision 9, keeping fills and replacements apart.
type recordingFaces struct{ filled, replaced []filledFace }

func (r *recordingFaces) FillPersonImage(_ context.Context, id int64, data []byte) (int64, error) {
	r.filled = append(r.filled, filledFace{id: id, data: data})
	return 9, nil
}

func (r *recordingFaces) ReplacePersonImage(_ context.Context, id int64, data []byte) (int64, error) {
	r.replaced = append(r.replaced, filledFace{id: id, data: data})
	return 9, nil
}

type personFetchEnv struct {
	*fetchEnv
	people *fakePluginPeople
	faces  *recordingFaces
}

// Plugin 1 is a person plugin accepting site and wiki and handling example.com links; fetchPerson stores an unaccepted
// ID before a wiki and a site ID and a handled link, linkedPerson only links.
func newPersonFetchEnv() *personFetchEnv {
	e := &personFetchEnv{fetchEnv: newFetchEnv(), faces: &recordingFaces{}}
	e.usePersonPlugin()
	e.store.plugins[1].Manifest.Domains = []string{"example.com"}
	e.people = &fakePluginPeople{plugins: e.store, people: map[int64]domain.Credit{
		fetchPerson: {ID: fetchPerson, Name: "Hanako", ExternalIDs: []domain.ExternalID{
			{Source: "other", Value: "o-1"}, {Source: "wiki", Value: "w-1"}, {Source: "wiki", Value: "w-2"},
			{Source: "site", Value: "s-1"},
		}, Profile: domain.PersonProfile{Links: []string{"https://example.com/p/1"}}},
		otherPerson: {ID: otherPerson, Name: "Taro", ExternalIDs: []domain.ExternalID{{Source: "other", Value: "o-2"}},
			Profile: domain.PersonProfile{Links: []string{"https://example.org/p/2"}}},
		linkedPerson: {ID: linkedPerson, Name: "Jiro", ExternalIDs: []domain.ExternalID{{Source: "other", Value: "o-3"}},
			Profile: domain.PersonProfile{Links: []string{"https://example.org/p/3", "https://www.example.com/p/3"}}},
	}}
	e.plugins = NewWorkInfoPlugins(WorkInfoPluginsOptions{
		Store: e.store, Runtime: e.runtime, People: e.people, PersonImages: e.faces, Now: func() time.Time { return e.now },
	})
	return e
}

func (e *personFetchEnv) fetch(t *testing.T, images ...[]byte) domain.PersonPluginFetchResult {
	t.Helper()
	e.runtime.result = domain.WorkInfoRunResult{
		Person:             domain.FetchedPluginPerson{Name: "Hanako", Profile: domain.PersonProfile{Birthplace: "Tokyo"}},
		PersonPluginImages: images,
	}
	result, err := e.plugins.FetchPersonWithPlugin(context.Background(), fetchPerson, 1, domain.PersonFetchKey{})
	if err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	return result
}

func TestFetchPersonWithPluginSendsTheFirstAcceptedStoredIDBeforeALink(t *testing.T) {
	e := newPersonFetchEnv()
	result := e.fetch(t)
	req := e.runtime.requests[0]
	if req.Operation != domain.WorkInfoFetch || req.Kind != domain.WorkInfoPluginKindPerson || req.Source != "wiki" ||
		req.ID != "w-1" || req.URL != "" || result.Link != "" {
		t.Fatalf("request = %+v, link %q; want a person fetch of the first stored ID the plugin accepts, wiki w-1, "+
			"over the handled link", req, result.Link)
	}
}

func TestFetchPersonWithPluginSendsTheOnlyHandledLink(t *testing.T) {
	e := newPersonFetchEnv()
	result, err := e.plugins.FetchPersonWithPlugin(context.Background(), linkedPerson, 1, domain.PersonFetchKey{})
	if err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	want := "https://www.example.com/p/3"
	if len(e.runtime.requests) != 1 || e.runtime.requests[0].URL != want || e.runtime.requests[0].Source != "" ||
		result.Link != want {
		t.Fatalf("requests = %+v, link %q; want one fetch by the handled link", e.runtime.requests, result.Link)
	}
}

func TestFetchPersonWithPluginAsksForAChoiceAmongHandledLinks(t *testing.T) {
	e := newPersonFetchEnv()
	person := e.people.people[linkedPerson]
	person.Profile.Links = []string{
		"http://Example.com/p/3/", "https://example.org/p/3", "https://example.com/p/3", "https://m.example.com/p/4",
	}
	e.people.people[linkedPerson] = person

	_, err := e.plugins.FetchPersonWithPlugin(context.Background(), linkedPerson, 1, domain.PersonFetchKey{})
	var ambiguous *domain.PersonLinkAmbiguousError
	if !errors.As(err, &ambiguous) || !errors.Is(err, domain.ErrPersonLinkAmbiguous) ||
		!slices.Equal(ambiguous.Links, []string{"http://Example.com/p/3/", "https://m.example.com/p/4"}) {
		t.Fatalf("err = %v; want ErrPersonLinkAmbiguous with the folded links in stored order", err)
	}
	if len(e.runtime.requests) != 0 {
		t.Fatal("the module ran before the choice")
	}
}

func TestFetchPersonWithPluginSendsTheGivenLink(t *testing.T) {
	e := newPersonFetchEnv()
	link := "  https://example.com/p/9#bio  "
	result, err := e.plugins.FetchPersonWithPlugin(context.Background(), fetchPerson, 1, domain.PersonFetchKey{Link: &link})
	if err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	if req := e.runtime.requests[0]; req.URL != "https://example.com/p/9#bio" || req.Source != "" ||
		result.Link != req.URL {
		t.Fatalf("request = %+v, link %q; want the trimmed given link over the stored ID", req, result.Link)
	}
}

func TestFetchPersonWithPluginRefusesALinkTheReplacedPluginDropped(t *testing.T) {
	e := newPersonFetchEnv()
	e.store.beforeModule = func() { e.store.plugins[1].Manifest.Domains = nil }

	_, err := e.plugins.FetchPersonWithPlugin(context.Background(), linkedPerson, 1, domain.PersonFetchKey{})
	if !errors.Is(err, domain.ErrWorkInfoRunInvalid) {
		t.Fatalf("err = %v; want ErrWorkInfoRunInvalid for the link the replaced plugin no longer handles", err)
	}
	if len(e.runtime.requests) != 0 {
		t.Fatal("the module ran with a link outside its domains")
	}
}

func TestFetchPersonWithPluginGoesByTheIDAppliedFromALinkFetch(t *testing.T) {
	e := newPersonFetchEnv()
	gained := domain.ExternalID{Source: "site", Value: "s-3"}
	e.runtime.result = domain.WorkInfoRunResult{Person: domain.FetchedPluginPerson{
		Name: "Jiro", ExternalIDs: []domain.ExternalID{gained},
	}}
	result, err := e.plugins.FetchPersonWithPlugin(context.Background(), linkedPerson, 1, domain.PersonFetchKey{})
	if err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	if _, err := e.plugins.ApplyPersonPluginFetch(context.Background(), linkedPerson, result.ID,
		domain.PersonPluginSelection{ExternalIDs: []domain.ExternalID{gained}}, -1); err != nil {
		t.Fatalf("ApplyPersonPluginFetch: %v", err)
	}

	if _, err := e.plugins.FetchPersonWithPlugin(context.Background(), linkedPerson, 1, domain.PersonFetchKey{}); err != nil {
		t.Fatalf("second FetchPersonWithPlugin: %v", err)
	}
	if req := e.runtime.requests[1]; req.Source != "site" || req.ID != "s-3" || req.URL != "" {
		t.Fatalf("second request = %+v; want the fetch by the applied site ID", req)
	}
}

func TestFetchPersonWithPluginSendsTheCandidatesID(t *testing.T) {
	e := newPersonFetchEnv()
	source, id := "site", "  s-9  "
	if _, err := e.plugins.FetchPersonWithPlugin(context.Background(), otherPerson, 1, domain.PersonFetchKey{Source: &source, ID: &id}); err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	if req := e.runtime.requests[0]; req.Source != "site" || req.ID != "s-9" {
		t.Fatalf("request = %+v; want the candidate's trimmed ID", req)
	}
}

func TestFetchPersonWithPluginRefusesBeforeTheRun(t *testing.T) {
	site, wrong, id := "site", "other", "s-1"
	outside, name, link := "https://example.org/p/1", "Hanako", "https://example.com/p/1"
	for _, tc := range []struct {
		name     string
		personID int64
		key      domain.PersonFetchKey
		want     error
	}{
		{"no accepted stored ID or link", otherPerson, domain.PersonFetchKey{}, domain.ErrPersonExternalIDMissing},
		{"missing person", 99, domain.PersonFetchKey{}, domain.ErrPersonNotFound},
		{"undeclared source", fetchPerson, domain.PersonFetchKey{Source: &wrong, ID: &id}, domain.ErrWorkInfoRunInvalid},
		{"source without id", fetchPerson, domain.PersonFetchKey{Source: &site}, domain.ErrWorkInfoRunInvalid},
		{"link outside the domains", otherPerson, domain.PersonFetchKey{Link: &outside}, domain.ErrPersonLinkUnsupported},
		{"link that is not a URL", otherPerson, domain.PersonFetchKey{Link: &name}, domain.ErrWorkInfoRunInvalid},
		{"link with source and id", otherPerson, domain.PersonFetchKey{Source: &site, ID: &id, Link: &link},
			domain.ErrWorkInfoRunInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPersonFetchEnv()
			_, err := e.plugins.FetchPersonWithPlugin(context.Background(), tc.personID, 1, tc.key)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v; want %v", err, tc.want)
			}
			if len(e.runtime.requests) != 0 {
				t.Fatal("the module ran")
			}
		})
	}
}

func TestFetchPersonWithPluginRefusesASourceTheReplacedPluginDropped(t *testing.T) {
	e := newPersonFetchEnv()
	e.store.beforeModule = func() { e.store.plugins[1].Manifest.Sources = []string{"site"} }

	_, err := e.plugins.FetchPersonWithPlugin(context.Background(), fetchPerson, 1, domain.PersonFetchKey{})
	if !errors.Is(err, domain.ErrWorkInfoRunInvalid) {
		t.Fatalf("err = %v; want ErrWorkInfoRunInvalid for the wiki ID the replaced plugin no longer declares", err)
	}
	if len(e.runtime.requests) != 0 {
		t.Fatal("the module ran with an undeclared source")
	}
}

func TestApplyPersonPluginFetchRefusesAProfileValueTheResultHid(t *testing.T) {
	e := newPersonFetchEnv()
	e.runtime.result = domain.WorkInfoRunResult{Person: domain.FetchedPluginPerson{
		Name: "Hanako", Profile: domain.PersonProfile{BirthDate: "2000", DeathDate: "1990"},
	}}
	result, err := e.plugins.FetchPersonWithPlugin(context.Background(), fetchPerson, 1, domain.PersonFetchKey{})
	if err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	if result.Person.Profile.BirthDate != "2000" || result.Person.Profile.DeathDate != "" {
		t.Fatalf("profile = %+v; want the birth date shown and the death date before it hidden", result.Person.Profile)
	}

	_, err = e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID,
		domain.PersonPluginSelection{Items: []domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileDeathDate)}}, -1)
	if !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
		t.Fatalf("err = %v; want ErrWorkInfoApplyInvalid for a value the result did not show", err)
	}
	if len(e.people.applies) != 0 {
		t.Fatal("a hidden value reached the store")
	}
}

func TestApplyPersonPluginFetchIsStaleAfterThePluginIsReplaced(t *testing.T) {
	e := newPersonFetchEnv()
	result := e.fetch(t)
	e.store.plugins[1].Digest, e.store.plugins[1].ApprovedDigest = "d1b", "d1b"

	_, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID,
		domain.PersonPluginSelection{Items: []domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileBirthplace)}}, -1)
	if !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("err = %v; want ErrWorkInfoFetchStale", err)
	}
	if len(e.people.applies) != 1 || e.people.applies[0].Digest != "d1" {
		t.Fatalf("applies = %+v; want one apply carrying the fetch-time digest", e.people.applies)
	}
}

func TestApplyPersonPluginFetchOfADeletedPersonIsPersonNotFound(t *testing.T) {
	e := newPersonFetchEnv()
	result := e.fetch(t)
	delete(e.people.people, fetchPerson)
	items := domain.PersonPluginSelection{Items: []domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileBirthplace)}}

	_, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID, items, -1)
	if !errors.Is(err, domain.ErrPersonNotFound) {
		t.Fatalf("err = %v; want ErrPersonNotFound", err)
	}
	if _, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID, items, -1); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("second apply err = %v; want the result consumed", err)
	}
}

func TestApplyPersonPluginFetchReplacesTheFaceWithTheChosenImage(t *testing.T) {
	e := newPersonFetchEnv()
	chosen := testPNG(t, 2, 2)
	// An image that cannot be read is dropped, so the chosen one is the second kept image.
	result := e.fetch(t, testPNG(t, 4, 4), []byte("not an image"), nil, chosen)
	if result.ImageCount != 2 {
		t.Fatalf("image count = %d; want the two readable images", result.ImageCount)
	}
	if got, err := e.plugins.PersonPluginFetchImage(fetchPerson, result.ID, 1); err != nil || string(got.Data) != string(chosen) {
		t.Fatalf("image 1 = %d bytes, %v; want the chosen image", len(got.Data), err)
	}
	if _, err := e.plugins.PersonPluginFetchImage(otherPerson, result.ID, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("image for another person err = %v; want ErrNotFound", err)
	}
	if _, err := e.plugins.ApplyPersonPluginFetch(context.Background(), otherPerson, result.ID,
		domain.PersonPluginSelection{Items: []domain.PersonPluginItem{domain.PersonItemImage}}, 1); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("apply for another person err = %v; want ErrWorkInfoFetchStale", err)
	}

	applied, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID,
		domain.PersonPluginSelection{Items: []domain.PersonPluginItem{domain.PersonItemImage}}, 1)
	if err != nil {
		t.Fatalf("ApplyPersonPluginFetch: %v", err)
	}
	if len(e.faces.filled) != 0 || len(e.faces.replaced) != 1 || e.faces.replaced[0].id != fetchPerson ||
		string(e.faces.replaced[0].data) != string(chosen) || applied.Person.ImageRevision != 9 {
		t.Fatalf("faces = %+v, revision %d; want the chosen image to replace the person's face", e.faces,
			applied.Person.ImageRevision)
	}
}

func TestApplyPersonPluginFetchRefusesAnItemWithoutAValue(t *testing.T) {
	e := newPersonFetchEnv()
	result := e.fetch(t, testPNG(t, 2, 2))
	image := []domain.PersonPluginItem{domain.PersonItemImage}
	for _, tc := range []struct {
		selection domain.PersonPluginSelection
		image     int
	}{
		{domain.PersonPluginSelection{}, -1},
		{domain.PersonPluginSelection{Aliases: []string{"Not fetched"}}, -1},
		{domain.PersonPluginSelection{Items: image}, -1}, {domain.PersonPluginSelection{Items: image}, 1},
	} {
		if _, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID, tc.selection, tc.image); !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
			t.Fatalf("selection %+v, image %d err = %v; want ErrWorkInfoApplyInvalid", tc.selection, tc.image, err)
		}
	}
	if len(e.people.applies) != 0 {
		t.Fatal("an invalid selection reached the store")
	}
}
