package app

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ListWorkInfoPlugins lists every plugin in name order, as the registry does.
func (s *runPluginStore) ListWorkInfoPlugins(context.Context) ([]domain.WorkInfoPlugin, error) {
	var out []domain.WorkInfoPlugin
	for _, plugin := range s.plugins {
		out = append(out, *plugin)
	}
	slices.SortFunc(out, func(a, b domain.WorkInfoPlugin) int { return strings.Compare(a.Manifest.Name, b.Manifest.Name) })
	return out, nil
}

const lookupPlugin = int64(3)

type lookupEnv struct {
	*fetchEnv
	people *fakePluginPeople
	faces  *recordingFaces
}

// Plugin 3 is an approved person plugin for wiki with automatic lookups; "Hanako" matches fetchPerson.
func newLookupEnv() *lookupEnv {
	e := &lookupEnv{fetchEnv: newFetchEnv(), faces: &recordingFaces{}}
	e.store.plugins[lookupPlugin] = &domain.WorkInfoPlugin{
		ID: lookupPlugin, Digest: "d3", ApprovedDigest: "d3", Generation: 1, AutoLookup: true,
		Manifest: domain.WorkInfoPluginManifest{
			Contract: 1, Name: "people", Kind: domain.WorkInfoPluginKindPerson, Sources: []string{"wiki"},
			Operations: []domain.WorkInfoOperation{domain.WorkInfoSearch, domain.WorkInfoFetch},
		},
	}
	e.people = &fakePluginPeople{plugins: e.store, people: map[int64]domain.Credit{
		fetchPerson: {ID: fetchPerson, Name: "Hanako"},
	}}
	e.matcher.answers = map[string]domain.CreditMatch{
		"Hanako": {State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: fetchPerson, Name: "Hanako"}},
	}
	e.plugins = NewWorkInfoPlugins(WorkInfoPluginsOptions{
		Store: e.store, Runtime: e.runtime, Videos: fakeVideos{fetchVideo: true}, Applier: e.applier,
		Matcher: e.matcher, People: e.people, PersonImages: e.faces, Now: func() time.Time { return e.now },
	})
	return e
}

// fetchTwo fetches Hanako (wiki 1) and Taro (wiki 2) as performers, planning lookups 0 and 1.
func (e *lookupEnv) fetchTwo(t *testing.T, image []byte) domain.WorkInfoFetchResult {
	t.Helper()
	e.runtime.result = domain.WorkInfoRunResult{
		WorkInfo: domain.FetchedWorkInfo{People: []domain.FetchedPerson{
			{Role: domain.PersonPerformer, Name: "Hanako", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "1"}}},
			{Role: domain.PersonPerformer, Name: "Taro", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "2"}}},
		}},
		PersonImages: [][]byte{image},
	}
	result, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "W-1")
	if err != nil {
		t.Fatalf("FetchWorkInfo: %v", err)
	}
	if len(result.Lookups.Lookups) != 2 {
		t.Fatalf("lookups = %+v; want one per person", result.Lookups)
	}
	e.runtime.requests = nil
	return result
}

func (e *lookupEnv) lookupReturns(person domain.FetchedPluginPerson, image []byte, err error) {
	e.runtime.result = domain.WorkInfoRunResult{Person: person, PersonPluginImages: [][]byte{image}}
	e.runtime.err = err
}

func TestPersonLookupRunsOnceAndAFailureCanBeRetried(t *testing.T) {
	e := newLookupEnv()
	result := e.fetchTwo(t, nil)
	ctx := context.Background()

	e.lookupReturns(domain.FetchedPluginPerson{}, nil, &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed})
	var failed *domain.WorkInfoRunError
	if _, err := e.plugins.RunPersonLookup(ctx, fetchVideo, result.ID, 0); !errors.As(err, &failed) {
		t.Fatalf("failed lookup: err = %v; want the run failure", err)
	}
	// The lookup's result names another accepted ID, which plans nothing more.
	e.lookupReturns(domain.FetchedPluginPerson{Name: "Taro", ExternalIDs: []domain.ExternalID{
		{Source: "wiki", Value: "2"}, {Source: "wiki", Value: "9"},
	}}, nil, nil)
	if got, err := e.plugins.RunPersonLookup(ctx, fetchVideo, result.ID, 1); err != nil || got.Person.Name != "Taro" ||
		got.Current != nil {
		t.Fatalf("other lookup = %+v, %v; want Taro, who matched no one", got, err)
	}
	if _, err := e.plugins.RunPersonLookup(ctx, fetchVideo, result.ID, 2); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lookup outside the plan: err = %v; want ErrNotFound", err)
	}

	e.lookupReturns(domain.FetchedPluginPerson{Name: "Hanako"}, nil, nil)
	got, err := e.plugins.RunPersonLookup(ctx, fetchVideo, result.ID, 0)
	if err != nil || got.Person.Name != "Hanako" || got.Current == nil || got.Current.ID != fetchPerson {
		t.Fatalf("retried lookup = %+v, %v; want Hanako with the matched person", got, err)
	}
	e.lookupReturns(domain.FetchedPluginPerson{Name: "changed"}, nil, nil)
	if got, err := e.plugins.RunPersonLookup(ctx, fetchVideo, result.ID, 0); err != nil || got.Person.Name != "Hanako" {
		t.Fatalf("repeated lookup = %+v, %v; want the kept result", got, err)
	}
	want := []domain.WorkInfoRunRequest{
		{Operation: domain.WorkInfoFetch, Kind: domain.WorkInfoPluginKindPerson, Source: "wiki", ID: "1"},
		{Operation: domain.WorkInfoFetch, Kind: domain.WorkInfoPluginKindPerson, Source: "wiki", ID: "2"},
		{Operation: domain.WorkInfoFetch, Kind: domain.WorkInfoPluginKindPerson, Source: "wiki", ID: "1"},
	}
	if len(e.runtime.requests) != len(want) {
		t.Fatalf("runs = %+v; want %d", e.runtime.requests, len(want))
	}
	for i, req := range e.runtime.requests {
		if req.Operation != want[i].Operation || req.Kind != want[i].Kind || req.Source != want[i].Source ||
			req.ID != want[i].ID {
			t.Fatalf("run %d = %+v; want %+v", i, req, want[i])
		}
	}
	if _, err := e.plugins.ApplyWorkInfoFetch(ctx, fetchVideo, result.ID,
		[]domain.WorkInfoField{domain.PersonField(domain.PersonPerformer)}, nil, false, nil); err != nil {
		t.Fatalf("apply after a failed lookup: %v", err)
	}
}

func TestPersonLookupRefusesAPluginChangedSinceTheFetch(t *testing.T) {
	e := newLookupEnv()
	result := e.fetchTwo(t, nil)
	e.lookupReturns(domain.FetchedPluginPerson{Name: "Hanako"}, nil, nil)
	// Turning automatic lookups off after the fetch does not stop a planned lookup.
	e.store.plugins[lookupPlugin].AutoLookup = false
	if _, err := e.plugins.RunPersonLookup(context.Background(), fetchVideo, result.ID, 0); err != nil {
		t.Fatalf("lookup after automatic lookups were turned off: %v", err)
	}
	e.store.plugins[lookupPlugin].Digest, e.store.plugins[lookupPlugin].ApprovedDigest = "d4", "d4"
	e.store.plugins[lookupPlugin].Generation++
	_, err := e.plugins.RunPersonLookup(context.Background(), fetchVideo, result.ID, 1)
	if !errors.Is(err, domain.ErrPersonLookupPluginChanged) || len(e.runtime.requests) != 1 {
		t.Fatalf("lookup on a replaced plugin: err = %v after %d runs; want ErrPersonLookupPluginChanged without a run",
			err, len(e.runtime.requests))
	}
}

func TestPersonLookupKeepsNothingFromAPluginChangedWhileItRuns(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*lookupEnv)
	}{
		{"revoked", func(e *lookupEnv) {
			e.store.plugins[lookupPlugin].ApprovedDigest = ""
			e.store.plugins[lookupPlugin].Generation++
		}},
		{"deleted", func(e *lookupEnv) { delete(e.store.plugins, lookupPlugin) }},
	} {
		e := newLookupEnv()
		result := e.fetchTwo(t, nil)
		e.lookupReturns(domain.FetchedPluginPerson{Name: "Hanako"}, pngOf(t, 2, 2), nil)
		e.runtime.during = func() { tc.change(e) }
		if _, err := e.plugins.RunPersonLookup(context.Background(), fetchVideo, result.ID, 0); !errors.Is(err, domain.ErrPersonLookupPluginChanged) {
			t.Fatalf("%s: err = %v; want ErrPersonLookupPluginChanged", tc.name, err)
		}
		if _, err := e.plugins.PersonLookupImage(fetchVideo, result.ID, 0); !errors.Is(err, domain.ErrPersonImageNotFound) {
			t.Fatalf("%s: image err = %v; want nothing kept", tc.name, err)
		}
	}
}

// A lookup image that pushes its own fetch out of the budget answers the fetch as stale instead of a result nobody
// can apply.
func TestKeepLookupReportsItsOwnFetchEvicted(t *testing.T) {
	fetches := newWorkInfoFetches()
	now := time.Unix(1_800_000_000, 0)
	full := &domain.WorkInfoCover{Data: make([]byte, domain.WorkInfoCoverMaxBytes)}
	people := make([]domain.FetchedPerson, domain.WorkInfoPersonImagesMaxCount)
	for i := range people {
		people[i].Image = full
	}
	lookups := make([]*keptLookup, domain.PersonLookupsMaxCount)
	for i := range lookups[1:] {
		lookups[i+1] = &keptLookup{image: full}
	}
	id := fetches.add(&workInfoFetch{
		videoID: fetchVideo, cover: full, details: domain.FetchedWorkInfo{People: people}, lookupRuns: lookups,
		expiresAt: now.Add(domain.WorkInfoFetchLifetime),
	}, now)
	fetches.addPerson(&personPluginFetch{images: []domain.WorkInfoCover{*full}, expiresAt: now.Add(domain.WorkInfoFetchLifetime + 1)}, now)
	if _, ok := fetches.entries[id]; !ok {
		t.Fatal("a fetch within the budget was dropped")
	}
	if kept, err := fetches.keepLookup(id, 0, &keptLookup{image: full}, now); !errors.Is(err, domain.ErrWorkInfoFetchStale) {
		t.Fatalf("keepLookup = %+v, %v; want ErrWorkInfoFetchStale", kept, err)
	}
}

func TestApplyWorkInfoFetchCarriesLookupItemsAndPutsTheLookupFaceFirst(t *testing.T) {
	e := newLookupEnv()
	workFace, lookupFace := pngOf(t, 2, 2), pngOf(t, 3, 3)
	result := e.fetchTwo(t, workFace)
	ctx := context.Background()
	e.lookupReturns(domain.FetchedPluginPerson{Name: "Hanako", Profile: domain.PersonProfile{Birthplace: "Tokyo"}},
		lookupFace, nil)
	if _, err := e.plugins.RunPersonLookup(ctx, fetchVideo, result.ID, 0); err != nil {
		t.Fatalf("RunPersonLookup: %v", err)
	}
	performers := []domain.WorkInfoField{domain.PersonField(domain.PersonPerformer)}
	birthplace := domain.PersonProfileItem(domain.ProfileBirthplace)
	selected := []domain.PersonLookupSelection{{Index: 0, Items: []domain.PersonPluginItem{domain.PersonItemImage, birthplace}}}
	for _, tc := range []struct {
		name    string
		fields  []domain.WorkInfoField
		lookups []domain.PersonLookupSelection
	}{
		{"without the role", []domain.WorkInfoField{domain.PersonField(domain.PersonDirector)}, selected},
		{"a lookup that did not run", performers, []domain.PersonLookupSelection{{Index: 1, Items: []domain.PersonPluginItem{birthplace}}}},
		{"an item without a value", performers, []domain.PersonLookupSelection{{Index: 0, Items: []domain.PersonPluginItem{domain.PersonItemAliases}}}},
	} {
		if _, err := e.plugins.ApplyWorkInfoFetch(ctx, fetchVideo, result.ID, tc.fields, nil, false, tc.lookups); !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
			t.Fatalf("%s: err = %v; want ErrWorkInfoApplyInvalid", tc.name, err)
		}
	}

	e.applier.personIDs, e.applier.lookupPersonIDs = []int64{fetchPerson, 0}, []int64{fetchPerson}
	if _, err := e.plugins.ApplyWorkInfoFetch(ctx, fetchVideo, result.ID, performers, nil, false, selected); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := e.applier.calls[len(e.applier.calls)-1].PersonLookups
	if len(got) != 1 || got[0].PluginID != lookupPlugin || got[0].Digest != "d3" || got[0].MatchedID != fetchPerson ||
		!slices.Equal(got[0].Entries, []int{0}) || got[0].Fetched.Profile.Birthplace != "Tokyo" ||
		!slices.Equal(got[0].Items, selected[0].Items) {
		t.Fatalf("lookups sent to the apply = %+v", got)
	}
	if len(e.faces.filled) != 2 || e.faces.filled[0].id != fetchPerson || !bytes.Equal(e.faces.filled[0].data, lookupFace) {
		t.Fatalf("faces = %+v; want the lookup's face offered to the person before the work-info one", e.faces.filled)
	}
}
