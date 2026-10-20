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
	"github.com/syudead/vv/internal/store"
)

// scriptedPersonFetches answers person page fetches with scripted results; application tests cover the decisions.
type scriptedPersonFetches struct {
	WorkInfoPlugins
	err        error
	fetch      domain.PersonPluginFetchResult
	applied    domain.PersonPluginApplied
	image      *domain.WorkInfoCover
	calls      int
	source, id *string
	items      []domain.PersonPluginItem
	imageIndex int
}

func (p *scriptedPersonFetches) FetchPersonWithPlugin(
	_ context.Context, _, _ int64, source, id *string,
) (domain.PersonPluginFetchResult, error) {
	p.calls++
	p.source, p.id = source, id
	return p.fetch, p.err
}

// PersonPluginFetchImage serves image as the result's second image.
func (p *scriptedPersonFetches) PersonPluginFetchImage(_ int64, fetchID string, index int) (domain.WorkInfoCover, error) {
	switch {
	case fetchID != "abc":
		return domain.WorkInfoCover{}, domain.ErrNotFound
	case p.image == nil || index != 1:
		return domain.WorkInfoCover{}, domain.ErrPersonImageNotFound
	}
	return *p.image, nil
}

func (p *scriptedPersonFetches) ApplyPersonPluginFetch(
	_ context.Context, _ int64, _ string, items []domain.PersonPluginItem, imageIndex int,
) (domain.PersonPluginApplied, error) {
	p.calls++
	p.items, p.imageIndex = items, imageIndex
	return p.applied, p.err
}

const (
	personFetchPath = "/api/people/3/plugin-fetches"
	personApplyPath = "/api/people/3/plugin-fetches/abc/apply"
)

func newPersonFetchRouteEnv(t *testing.T) (*authEnv, *http.Cookie, *scriptedPersonFetches) {
	t.Helper()
	plugins := &scriptedPersonFetches{}
	env := newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options { return Options{WorkInfoPlugins: plugins} })
	return env, env.setup(), plugins
}

func TestFetchPersonWithPluginAnswersTheResultOrItsError(t *testing.T) {
	e, owner, plugins := newPersonFetchRouteEnv(t)
	post := func(body string) (int, []byte) {
		rec := e.serve(authRequest{method: http.MethodPost, target: personFetchPath, body: body, cookies: []*http.Cookie{owner}})
		return rec.Code, rec.Body.Bytes()
	}
	plugins.fetch = domain.PersonPluginFetchResult{
		ID: "abc", ExpiresAt: time.Unix(1_800_000_600, 0), ImageCount: 2,
		Person: domain.FetchedPluginPerson{Name: "Hanako", ExternalIDs: []domain.ExternalID{{Source: "wiki", Value: "w-1"}}},
	}
	status, body := post(`{"pluginId":1,"source":"wiki","id":"w-1"}`)
	var fetch gen.PersonPluginFetch
	if status != http.StatusCreated || json.Unmarshal(body, &fetch) != nil || fetch.FetchId != "abc" ||
		fetch.ExpiresAt != 1_800_000_600 || fetch.Person.Name != "Hanako" || fetch.Person.ImageCount == nil || *fetch.Person.ImageCount != 2 ||
		len(fetch.Person.ExternalIds) != 1 || fetch.Person.Aliases == nil || fetch.Person.Profile != nil {
		t.Fatalf("fetch = %d %s", status, body)
	}
	if plugins.source == nil || *plugins.source != "wiki" || plugins.id == nil || *plugins.id != "w-1" {
		t.Fatalf("source, id = %v, %v; want the candidate passed on", plugins.source, plugins.id)
	}

	for _, tc := range []struct {
		name string
		err  error
		want wantError
	}{
		{"missing person", domain.ErrPersonNotFound,
			wantError{status: 404, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}},
		{"missing plugin", domain.ErrNotFound, wantError{status: 404, code: gen.ErrorCodeNotFound}},
		{"no accepted ID", domain.ErrPersonExternalIDMissing,
			wantError{status: 409, code: gen.ErrorCodePersonExternalIdMissing}},
		{"work-info plugin", domain.ErrWorkInfoPluginKindMismatch, wantError{status: 409, code: gen.ErrorCodePluginKindMismatch}},
		{"undeclared source", domain.ErrWorkInfoRunInvalid, wantError{status: 400, code: gen.ErrorCodeInvalidRequest}},
		{"run failure", &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed},
			wantError{status: 502, code: gen.ErrorCodePluginFailed}},
	} {
		plugins.err = tc.err
		status, body := post(`{"pluginId":1}`)
		assertErrorBody(t, tc.name, status, body, tc.want)
	}
	calls := plugins.calls
	status, body = post(`{"source":"wiki"}`)
	assertErrorBody(t, "missing pluginId", status, body, wantError{status: 400, code: gen.ErrorCodeInvalidRequest})
	if plugins.calls != calls {
		t.Fatal("a request without pluginId reached the registry")
	}
	assertUnauthenticated(t, "guest fetch",
		e.serve(authRequest{method: http.MethodPost, target: personFetchPath, body: `{"pluginId":1}`}))
}

func TestPersonPluginFetchImageServesTheKeptImageByIndex(t *testing.T) {
	e, owner, plugins := newPersonFetchRouteEnv(t)
	get := func(path string) (int, []byte, http.Header) {
		rec := e.serve(authRequest{
			method: http.MethodGet, target: "/api/people/3/plugin-fetches/" + path, cookies: []*http.Cookie{owner},
		})
		return rec.Code, rec.Body.Bytes(), rec.Header()
	}
	plugins.image = &domain.WorkInfoCover{Data: []byte("jpeg bytes"), ContentType: "image/jpeg"}
	status, body, _ := get("abc/images/0")
	assertErrorBody(t, "index without an image", status, body,
		wantError{status: 404, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonImageNotFound})
	status, body, _ = get("gone/images/1")
	assertErrorBody(t, "unknown result", status, body, wantError{status: 404, code: gen.ErrorCodeNotFound})

	status, body, header := get("abc/images/1")
	if status != http.StatusOK || string(body) != "jpeg bytes" || header.Get("Content-Type") != "image/jpeg" ||
		header.Get("Cache-Control") != cacheNoStore {
		t.Fatalf("image = %d %q %v", status, body, header)
	}
}

func TestApplyPersonPluginFetchAnswersThePersonOrItsError(t *testing.T) {
	e, owner, plugins := newPersonFetchRouteEnv(t)
	post := func(body string) (int, []byte) {
		rec := e.serve(authRequest{method: http.MethodPost, target: personApplyPath, body: body, cookies: []*http.Cookie{owner}})
		return rec.Code, rec.Body.Bytes()
	}
	plugins.applied = domain.PersonPluginApplied{
		Person: domain.Credit{Kind: domain.CreditPerson, ID: 3, Name: "Hanako", ImageRevision: 9},
		Skipped: []domain.SkippedPersonValue{{
			Item: domain.PersonItemExternalIDs, Value: "wiki: w-1", Reason: domain.SkippedWorkInfoTaken, HolderName: "Taro",
		}},
	}
	status, body := post(`{"items":["image","profile.heightCm","externalIds"],"imageIndex":1}`)
	var applied gen.PersonPluginFetchApplied
	if status != http.StatusOK || json.Unmarshal(body, &applied) != nil || applied.Person.Id != 3 ||
		applied.Person.ImageRevision == nil || len(applied.Skipped) != 1 {
		t.Fatalf("apply = %d %s", status, body)
	}
	if got := applied.Skipped[0]; got.Item != gen.PersonPluginItemExternalIds || got.Reason != gen.SkippedPersonReasonTaken ||
		got.HolderName == nil || *got.HolderName != "Taro" {
		t.Fatalf("skipped = %+v; want the taken ID with its holder", got)
	}
	want := []domain.PersonPluginItem{domain.PersonItemImage, domain.PersonProfileItem(domain.ProfileHeightCm), domain.PersonItemExternalIDs}
	if !slices.Equal(plugins.items, want) || plugins.imageIndex != 1 {
		t.Fatalf("items = %v, image %d; want %v and image 1", plugins.items, plugins.imageIndex, want)
	}
	if post(`{"items":["aliases"]}`); plugins.imageIndex != -1 {
		t.Fatalf("image index without one = %d; want -1, no image", plugins.imageIndex)
	}

	for _, tc := range []struct {
		name string
		err  error
		want wantError
	}{
		{"item without a value", domain.ErrWorkInfoApplyInvalid, wantError{status: 400, code: gen.ErrorCodeInvalidRequest}},
		{"person merged away", domain.ErrPersonNotFound,
			wantError{status: 404, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}},
		{"plugin replaced", domain.ErrWorkInfoFetchStale, wantError{status: 409, code: gen.ErrorCodeWorkInfoFetchStale}},
	} {
		plugins.err = tc.err
		status, body := post(`{"items":["aliases"]}`)
		assertErrorBody(t, tc.name, status, body, tc.want)
	}
	calls := plugins.calls
	status, body = post(`{"items":["name"]}`)
	assertErrorBody(t, "unknown item", status, body, wantError{status: 400, code: gen.ErrorCodeInvalidRequest})
	if plugins.calls != calls {
		t.Fatal("an unknown item reached the registry")
	}
}
