package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const (
	fetchPerson = int64(3)
	otherPerson = int64(4)
)

// fakePluginPeople rechecks the plugin like the store's transaction; a person absent from people does not exist.
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
	plugin, err := f.plugins.WorkInfoPlugin(ctx, apply.PluginID)
	if err != nil || plugin.State() != domain.WorkInfoPluginApproved || plugin.Digest != apply.Digest ||
		plugin.Generation != apply.Generation {
		return domain.PersonPluginApplied{}, domain.ErrWorkInfoFetchStale
	}
	person, ok := f.people[apply.PersonID]
	if !ok {
		return domain.PersonPluginApplied{}, domain.ErrNotFound
	}
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

// Plugin 1 is a person plugin accepting site and wiki; fetchPerson stores an unaccepted ID before a wiki and a site ID.
func newPersonFetchEnv() *personFetchEnv {
	e := &personFetchEnv{fetchEnv: newFetchEnv(), faces: &recordingFaces{}}
	e.usePersonPlugin()
	e.people = &fakePluginPeople{plugins: e.store, people: map[int64]domain.Credit{
		fetchPerson: {ID: fetchPerson, Name: "Hanako", ExternalIDs: []domain.ExternalID{
			{Source: "other", Value: "o-1"}, {Source: "wiki", Value: "w-1"}, {Source: "wiki", Value: "w-2"},
			{Source: "site", Value: "s-1"},
		}},
		otherPerson: {ID: otherPerson, Name: "Taro", ExternalIDs: []domain.ExternalID{{Source: "other", Value: "o-2"}}},
	}}
	e.plugins = NewWorkInfoPlugins(WorkInfoPluginsOptions{
		Store: e.store, Runtime: e.runtime, People: e.people, PersonImages: e.faces, Now: func() time.Time { return e.now },
	})
	return e
}

func (e *personFetchEnv) fetch(t *testing.T, image []byte) domain.PersonPluginFetchResult {
	t.Helper()
	e.runtime.result = domain.WorkInfoRunResult{
		Person:      domain.FetchedPluginPerson{Name: "Hanako", Profile: domain.PersonProfile{Birthplace: "Tokyo"}},
		PersonImage: image,
	}
	result, err := e.plugins.FetchPersonWithPlugin(context.Background(), fetchPerson, 1, nil, nil)
	if err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	return result
}

func TestFetchPersonWithPluginSendsTheFirstAcceptedStoredID(t *testing.T) {
	e := newPersonFetchEnv()
	e.fetch(t, nil)
	req := e.runtime.requests[0]
	if req.Operation != domain.WorkInfoFetch || req.Kind != domain.WorkInfoPluginKindPerson || req.Source != "wiki" ||
		req.ID != "w-1" {
		t.Fatalf("request = %+v; want a person fetch of the first stored ID the plugin accepts, wiki w-1", req)
	}
}

func TestFetchPersonWithPluginSendsTheCandidatesID(t *testing.T) {
	e := newPersonFetchEnv()
	source, id := "site", "  s-9  "
	if _, err := e.plugins.FetchPersonWithPlugin(context.Background(), otherPerson, 1, &source, &id); err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	if req := e.runtime.requests[0]; req.Source != "site" || req.ID != "s-9" {
		t.Fatalf("request = %+v; want the candidate's trimmed ID", req)
	}
}

func TestFetchPersonWithPluginRefusesBeforeTheRun(t *testing.T) {
	site, wrong, id := "site", "other", "s-1"
	for _, tc := range []struct {
		name       string
		personID   int64
		source, id *string
		want       error
	}{
		{"no accepted stored ID", otherPerson, nil, nil, domain.ErrPersonExternalIDMissing},
		{"missing person", 99, nil, nil, domain.ErrPersonNotFound},
		{"undeclared source", fetchPerson, &wrong, &id, domain.ErrWorkInfoRunInvalid},
		{"source without id", fetchPerson, &site, nil, domain.ErrWorkInfoRunInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPersonFetchEnv()
			_, err := e.plugins.FetchPersonWithPlugin(context.Background(), tc.personID, 1, tc.source, tc.id)
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

	_, err := e.plugins.FetchPersonWithPlugin(context.Background(), fetchPerson, 1, nil, nil)
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
	result, err := e.plugins.FetchPersonWithPlugin(context.Background(), fetchPerson, 1, nil, nil)
	if err != nil {
		t.Fatalf("FetchPersonWithPlugin: %v", err)
	}
	if result.Person.Profile.BirthDate != "2000" || result.Person.Profile.DeathDate != "" {
		t.Fatalf("profile = %+v; want the birth date shown and the death date before it hidden", result.Person.Profile)
	}

	_, err = e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID,
		[]domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileDeathDate)})
	if !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
		t.Fatalf("err = %v; want ErrWorkInfoApplyInvalid for a value the result did not show", err)
	}
	if len(e.people.applies) != 0 {
		t.Fatal("a hidden value reached the store")
	}
}

func TestApplyPersonPluginFetchIsStaleAfterThePluginIsReplaced(t *testing.T) {
	e := newPersonFetchEnv()
	result := e.fetch(t, nil)
	e.store.plugins[1].Digest, e.store.plugins[1].ApprovedDigest = "d1b", "d1b"

	_, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID,
		[]domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileBirthplace)})
	if !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("err = %v; want ErrWorkInfoFetchStale", err)
	}
	if len(e.people.applies) != 1 || e.people.applies[0].Digest != "d1" {
		t.Fatalf("applies = %+v; want one apply carrying the fetch-time digest", e.people.applies)
	}
}

func TestApplyPersonPluginFetchOfADeletedPersonIsPersonNotFound(t *testing.T) {
	e := newPersonFetchEnv()
	result := e.fetch(t, nil)
	delete(e.people.people, fetchPerson)
	items := []domain.PersonPluginItem{domain.PersonProfileItem(domain.ProfileBirthplace)}

	_, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID, items)
	if !errors.Is(err, domain.ErrPersonNotFound) {
		t.Fatalf("err = %v; want ErrPersonNotFound", err)
	}
	if _, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID, items); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("second apply err = %v; want the result consumed", err)
	}
}

func TestApplyPersonPluginFetchReplacesTheFaceAfterTheCommit(t *testing.T) {
	e := newPersonFetchEnv()
	image := testPNG(t, 4, 4)
	result := e.fetch(t, image)
	if !result.HasImage {
		t.Fatal("the fetched image was not kept")
	}
	if _, err := e.plugins.PersonPluginFetchImage(otherPerson, result.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("image for another person err = %v; want ErrNotFound", err)
	}
	if _, err := e.plugins.ApplyPersonPluginFetch(context.Background(), otherPerson, result.ID,
		[]domain.PersonPluginItem{domain.PersonItemImage}); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("apply for another person err = %v; want ErrWorkInfoFetchStale", err)
	}

	applied, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID,
		[]domain.PersonPluginItem{domain.PersonItemImage})
	if err != nil {
		t.Fatalf("ApplyPersonPluginFetch: %v", err)
	}
	if len(e.faces.filled) != 0 || len(e.faces.replaced) != 1 || e.faces.replaced[0].id != fetchPerson ||
		string(e.faces.replaced[0].data) != string(image) || applied.Person.ImageRevision != 9 {
		t.Fatalf("faces = %+v, revision %d; want the kept image to replace the person's face", e.faces,
			applied.Person.ImageRevision)
	}
}

func TestApplyPersonPluginFetchRefusesAnItemWithoutAValue(t *testing.T) {
	e := newPersonFetchEnv()
	result := e.fetch(t, nil)
	for _, items := range [][]domain.PersonPluginItem{nil, {domain.PersonItemImage}, {domain.PersonItemAliases}} {
		if _, err := e.plugins.ApplyPersonPluginFetch(context.Background(), fetchPerson, result.ID, items); !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
			t.Fatalf("items %v err = %v; want ErrWorkInfoApplyInvalid", items, err)
		}
	}
	if len(e.people.applies) != 0 {
		t.Fatal("an invalid selection reached the store")
	}
}
