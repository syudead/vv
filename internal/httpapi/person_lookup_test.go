package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const lookupPath = "/api/videos/7/work-info-fetches/abc/person-lookups/0"

func (p *scriptedPlugins) RunPersonLookup(context.Context, int64, string, int) (domain.PersonLookupResult, error) {
	p.calls++
	return p.lookup, p.err
}

func (p *scriptedPlugins) PersonLookupImage(_ int64, _ string, index int) (domain.WorkInfoCover, error) {
	if index != 0 {
		return domain.WorkInfoCover{}, domain.ErrNotFound
	}
	if p.cover == nil {
		return domain.WorkInfoCover{}, domain.ErrPersonImageNotFound
	}
	return *p.cover, nil
}

func TestWorkInfoFetchCarriesThePersonLookupPlan(t *testing.T) {
	e := newFetchRouteEnv(t)
	e.plugins.fetch = domain.WorkInfoFetchResult{
		ID: "abc", ExpiresAt: time.Unix(1_800_000_600, 0), Tags: []string{},
		Details: domain.FetchedWorkInfo{People: []domain.FetchedPerson{
			{Role: domain.PersonPerformer, Name: "A"}, {Role: domain.PersonPerformer, Name: "B"},
			{Role: domain.PersonPerformer, Name: "C"},
		}},
		Lookups: domain.PersonLookupPlan{
			Lookups:      []domain.PersonLookup{{PluginID: 2, PluginName: "People", Source: "wiki", ID: "1"}},
			EntryLookups: []int{0, -1, -1}, Limited: []bool{false, true, false},
		},
	}
	status, body := e.post(fetchPath, `{"pluginId":1,"id":"ABC-123"}`, e.owner)
	var fetch gen.WorkInfoFetch
	if status != http.StatusCreated || json.Unmarshal(body, &fetch) != nil {
		t.Fatalf("fetch = %d %s", status, body)
	}
	want := []gen.PersonLookup{{PluginId: 2, PluginName: "People", Source: "wiki", Id: "1"}}
	if !slices.Equal(fetch.PersonLookups, want) {
		t.Fatalf("personLookups = %+v; want %+v", fetch.PersonLookups, want)
	}
	people := fetch.People
	if people[0].Lookup == nil || *people[0].Lookup != 0 || people[0].LookupLimited != nil ||
		people[1].Lookup != nil || people[1].LookupLimited == nil || people[2].Lookup != nil || people[2].LookupLimited != nil {
		t.Fatalf("people = %s; want the first with lookup 0 and the second limited", body)
	}
}

func TestPersonLookupAnswers(t *testing.T) {
	e := newFetchRouteEnv(t)
	e.plugins.lookup = domain.PersonLookupResult{
		Person: domain.FetchedPluginPerson{Name: "Hanako", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "1"}}},
	}
	status, body := e.post(lookupPath, "", e.owner)
	if status != http.StatusOK || !json.Valid(body) || !jsonHas(t, body, "current", nil) {
		t.Fatalf("lookup without a match = %d %s; want current null", status, body)
	}
	e.plugins.lookup.Current, e.plugins.lookup.HasImage = &domain.Credit{ID: 5, Name: "Hanako"}, true
	status, body = e.post(lookupPath, "", e.owner)
	var got gen.PersonPluginResult
	if status != http.StatusOK || json.Unmarshal(body, &got) != nil || got.Current == nil || got.Current.Id != 5 ||
		got.Person.Name != "Hanako" || got.Person.ImageCount == nil || *got.Person.ImageCount != 1 {
		t.Fatalf("lookup = %d %s", status, body)
	}
	for _, tc := range []struct {
		err  error
		want wantError
	}{
		{domain.ErrNotFound, wantError{status: 404, code: gen.ErrorCodeNotFound}},
		{domain.ErrWorkInfoFetchStale, wantError{status: 409, code: gen.ErrorCodeWorkInfoFetchStale}},
		{domain.ErrPersonLookupPluginChanged, wantError{status: 409, code: gen.ErrorCodePluginChanged}},
		{&domain.WorkInfoRunError{Failure: domain.WorkInfoRunTimedOut}, wantError{status: 504, code: gen.ErrorCodePluginTimedOut}},
	} {
		e.plugins.err = tc.err
		status, body := e.post(lookupPath, "", e.owner)
		assertErrorBody(t, tc.err.Error(), status, body, tc.want)
	}
}

func TestPersonLookupImageAnswers(t *testing.T) {
	e := newFetchRouteEnv(t)
	get := func(target string) (int, []byte) {
		rec := e.serve(authRequest{method: http.MethodGet, target: target, cookies: []*http.Cookie{e.owner}})
		return rec.Code, rec.Body.Bytes()
	}
	status, body := get(lookupPath + "/image")
	assertErrorBody(t, "no image", status, body,
		wantError{status: 404, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonImageNotFound})
	status, body = get("/api/videos/7/work-info-fetches/abc/person-lookups/1/image")
	assertErrorBody(t, "no lookup", status, body, wantError{status: 404, code: gen.ErrorCodeNotFound})
	e.plugins.cover = &domain.WorkInfoCover{Data: []byte("img"), ContentType: "image/png"}
	if status, body := get(lookupPath + "/image"); status != http.StatusOK || string(body) != "img" {
		t.Fatalf("image = %d %q", status, body)
	}
}

func TestWorkInfoApplyPassesAndReportsLookups(t *testing.T) {
	e := newFetchRouteEnv(t)
	lookup := 0
	e.plugins.applied = domain.WorkInfoApplied{Video: domain.Video{ID: 7}, Skipped: []domain.SkippedWorkInfoValue{{
		Field: domain.PersonField(domain.PersonPerformer), Value: "wiki: 2", Reason: domain.SkippedWorkInfoTaken,
		Lookup: &lookup, Item: domain.PersonItemExternalIDs, HolderName: "Other",
	}}}
	status, body := e.post(applyPath,
		`{"fields":["people.performer"],"personLookups":[{"index":0,"items":["image","profile.birthDate"]}]}`, e.owner)
	var applied gen.WorkInfoFetchApplied
	if status != http.StatusOK || json.Unmarshal(body, &applied) != nil {
		t.Fatalf("apply = %d %s", status, body)
	}
	want := []domain.PersonLookupSelection{{Index: 0, Items: []domain.PersonPluginItem{
		domain.PersonItemImage, domain.PersonProfileItem(domain.ProfileBirthDate),
	}}}
	if len(e.plugins.lookups) != 1 || e.plugins.lookups[0].Index != 0 ||
		!slices.Equal(e.plugins.lookups[0].Items, want[0].Items) {
		t.Fatalf("lookups = %+v; want %+v", e.plugins.lookups, want)
	}
	skipped := applied.Skipped[0]
	if skipped.Reason != gen.SkippedWorkInfoReasonTaken || skipped.Lookup == nil || *skipped.Lookup != 0 ||
		skipped.Item == nil || *skipped.Item != gen.PersonPluginItemExternalIds || skipped.HolderName == nil ||
		*skipped.HolderName != "Other" {
		t.Fatalf("skipped = %s", body)
	}
	calls := e.plugins.calls
	for _, tc := range []struct{ name, body string }{
		{"unknown lookup item", `{"fields":["people.performer"],"personLookups":[{"index":0,"items":["name"]}]}`},
		{"null lookups", `{"fields":["people.performer"],"personLookups":null}`},
	} {
		status, body = e.post(applyPath, tc.body, e.owner)
		assertErrorBody(t, tc.name, status, body, wantError{status: 400, code: gen.ErrorCodeInvalidRequest})
		if e.plugins.calls != calls {
			t.Fatalf("%s reached the registry", tc.name)
		}
	}
}

// jsonHas reports whether the top-level property key of body equals want.
func jsonHas(t *testing.T, body []byte, key string, want any) bool {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	value, ok := got[key]
	return ok && value == want
}
