package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// These tests cover HTTP status and field mapping with scripted plugin results; application tests cover decisions.

type scriptedPlugins struct {
	WorkInfoPlugins
	err        error
	candidates []domain.WorkInfoCandidate
	people     []domain.PersonCandidate
	fetch      domain.WorkInfoFetchResult
	applied    domain.WorkInfoApplied
	calls      int
	selection  domain.WorkInfoSelection
	pageCover  bool
	ids        []string
	// covers and personImages map an index to its kept image.
	covers       map[int]domain.WorkInfoCover
	personImages map[int]domain.WorkInfoCover
}

func (p *scriptedPlugins) WorkInfoFetchPersonImage(_ int64, fetchID string, index int) (domain.WorkInfoCover, error) {
	if fetchID != "abc" {
		return domain.WorkInfoCover{}, domain.ErrNotFound
	}
	image, ok := p.personImages[index]
	if !ok {
		return domain.WorkInfoCover{}, domain.ErrPersonImageNotFound
	}
	return image, nil
}

func (p *scriptedPlugins) SearchWorkInfo(context.Context, int64, string) ([]domain.WorkInfoCandidate, error) {
	p.calls++
	return p.candidates, p.err
}

func (p *scriptedPlugins) SearchPeople(context.Context, int64, string) ([]domain.PersonCandidate, error) {
	p.calls++
	return p.people, p.err
}

func (p *scriptedPlugins) FetchWorkInfo(_ context.Context, _, _ int64, id string) (domain.WorkInfoFetchResult, error) {
	p.calls++
	p.ids = append(p.ids, id)
	return p.fetch, p.err
}

func (p *scriptedPlugins) WorkInfoFetchCover(_ int64, fetchID string, index int) (domain.WorkInfoCover, error) {
	cover, ok := p.covers[index]
	if fetchID != "abc" || !ok {
		return domain.WorkInfoCover{}, domain.ErrNotFound
	}
	return cover, nil
}

func (p *scriptedPlugins) ApplyWorkInfoFetch(
	_ context.Context, _ int64, _ string, selection domain.WorkInfoSelection, coverByPage bool,
) (domain.WorkInfoApplied, error) {
	p.calls++
	p.selection, p.pageCover = selection, coverByPage
	return p.applied, p.err
}

type fetchRouteEnv struct {
	*authEnv
	owner   *http.Cookie
	plugins *scriptedPlugins
}

func newFetchRouteEnv(t *testing.T) *fetchRouteEnv {
	t.Helper()
	plugins := &scriptedPlugins{}
	env := newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options { return Options{WorkInfoPlugins: plugins} })
	return &fetchRouteEnv{authEnv: env, owner: env.setup(), plugins: plugins}
}

func (e *fetchRouteEnv) post(target, body string, cookies ...*http.Cookie) (int, []byte) {
	e.t.Helper()
	rec := e.serve(authRequest{method: http.MethodPost, target: target, body: body, cookies: cookies})
	return rec.Code, rec.Body.Bytes()
}

const (
	searchPath       = "/api/work-info-plugins/1/search"
	personSearchPath = "/api/work-info-plugins/1/person-search"
	fetchPath        = "/api/videos/7/work-info-fetches"
	applyPath        = "/api/videos/7/work-info-fetches/abc/apply"
)

func decodeErrorBody(t *testing.T, body []byte) gen.Error {
	t.Helper()
	var got gen.Error
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return got
}

func TestWorkInfoRunFailuresFollowTheContract(t *testing.T) {
	e := newFetchRouteEnv(t)
	for _, tc := range []struct {
		name  string
		err   error
		want  wantError
		check func(gen.Error) bool
	}{
		{"no plugin", domain.ErrNotFound, wantError{status: 404, code: gen.ErrorCodeNotFound}, nil},
		{"not approved", domain.ErrWorkInfoPluginNotApproved, wantError{status: 409, code: gen.ErrorCodePluginNotApproved}, nil},
		{"other kind", domain.ErrWorkInfoPluginKindMismatch, wantError{status: 409, code: gen.ErrorCodePluginKindMismatch}, nil},
		{"unsupported", domain.ErrWorkInfoOperationUnsupported, wantError{status: 409, code: gen.ErrorCodeOperationUnsupported}, nil},
		{"invalid", domain.ErrWorkInfoRunInvalid, wantError{status: 400, code: gen.ErrorCodeInvalidRequest}, nil},
		{"settings missing", &domain.WorkInfoSettingsMissingError{Keys: []string{"apiKey", "region"}},
			wantError{status: 409, code: gen.ErrorCodePluginSettingsMissing},
			func(got gen.Error) bool {
				return got.SettingKeys != nil && slices.Equal(*got.SettingKeys, []string{"apiKey", "region"})
			}},
		{"plugin error", &domain.WorkInfoRunError{Failure: domain.WorkInfoRunFailed, PluginMessage: "quota exceeded"},
			wantError{status: 502, code: gen.ErrorCodePluginFailed},
			func(got gen.Error) bool {
				return got.PluginMessage != nil && *got.PluginMessage == "quota exceeded" && got.Message != "quota exceeded"
			}},
		{"blocked address", &domain.WorkInfoRunError{Failure: domain.WorkInfoRunBlockedAddress, Host: "192.168.1.2"},
			wantError{status: 502, code: gen.ErrorCodePluginFailed, reason: gen.ErrorReasonBlockedAddress},
			func(got gen.Error) bool { return got.Host != nil && *got.Host == "192.168.1.2" }},
		{"network error", &domain.WorkInfoRunError{Failure: domain.WorkInfoRunNetworkError, Host: "example.com"},
			wantError{status: 502, code: gen.ErrorCodePluginFailed, reason: gen.ErrorReasonNetworkError},
			func(got gen.Error) bool { return got.Host != nil && *got.Host == "example.com" }},
		{"timed out", &domain.WorkInfoRunError{Failure: domain.WorkInfoRunTimedOut},
			wantError{status: 504, code: gen.ErrorCodePluginTimedOut}, nil},
		{"memory", &domain.WorkInfoRunError{Failure: domain.WorkInfoRunMemoryExceeded},
			wantError{status: 502, code: gen.ErrorCodePluginFailed, reason: gen.ErrorReasonMemoryExceeded}, nil},
		{"output", &domain.WorkInfoRunError{Failure: domain.WorkInfoRunOutputTooLarge},
			wantError{status: 502, code: gen.ErrorCodePluginFailed, reason: gen.ErrorReasonOutputTooLarge}, nil},
		{"busy", &domain.WorkInfoRunError{Failure: domain.WorkInfoRunBusy},
			wantError{status: 503, code: gen.ErrorCodePluginBusy}, nil},
	} {
		e.plugins.err = tc.err
		for _, route := range []struct{ path, body string }{
			{searchPath, `{"query":"work"}`},
			{personSearchPath, `{"query":"person"}`},
			{fetchPath, `{"pluginId":1,"id":"ABC-123"}`},
		} {
			label := tc.name + " " + route.path
			status, body := e.post(route.path, route.body, e.owner)
			assertErrorBody(t, label, status, body, tc.want)
			if tc.check != nil && !tc.check(decodeErrorBody(t, body)) {
				t.Errorf("%s: body = %s", label, body)
			}
		}
	}
}

func TestWorkInfoSearchAndFetchAnswer(t *testing.T) {
	e := newFetchRouteEnv(t)
	e.plugins.candidates = []domain.WorkInfoCandidate{
		{ID: "w-1", URL: "https://e.test/1", Title: "One"},
		{ID: "w-2", URL: "https://e.test/2", Title: "Two", Detail: "2024"},
	}
	status, body := e.post(searchPath, `{"query":"w"}`, e.owner)
	var search gen.WorkInfoSearchResult
	if status != http.StatusOK || json.Unmarshal(body, &search) != nil || len(search.Candidates) != 2 ||
		search.Candidates[0].Id != "w-1" || search.Candidates[0].Url != "https://e.test/1" || search.Candidates[0].Detail != nil || *search.Candidates[1].Detail != "2024" {
		t.Fatalf("search = %d %s", status, body)
	}

	e.plugins.fetch = domain.WorkInfoFetchResult{ID: "abc", ExpiresAt: time.Unix(1_800_000_600, 0), Tags: []string{}}
	status, body = e.post(fetchPath, `{"pluginId":1,"id":"ABC-123"}`, e.owner)
	var fetch gen.WorkInfoFetch
	if status != http.StatusCreated || json.Unmarshal(body, &fetch) != nil || fetch.FetchId != "abc" ||
		fetch.ExpiresAt != 1_800_000_600 || fetch.Title != nil || fetch.Tags == nil {
		t.Fatalf("fetch = %d %s", status, body)
	}

	status, body = e.post(fetchPath, `{"pluginId":1,"id":"https://example.com/w"}`, e.owner)
	if status != http.StatusCreated || json.Unmarshal(body, &fetch) != nil || fetch.FetchId != "abc" ||
		!slices.Equal(e.plugins.ids, []string{"ABC-123", "https://example.com/w"}) {
		t.Fatalf("fetch by a URL-shaped id = %d %s (ids %v)", status, body, e.plugins.ids)
	}

	calls := e.plugins.calls
	for _, req := range []struct{ path, body string }{
		{searchPath, `{}`},
		{fetchPath, `{"id":"ABC-123"}`},
		{fetchPath, `{"pluginId":1}`},
		{fetchPath, `{"pluginId":1,"url":"https://example.com/w"}`},
	} {
		status, body := e.post(req.path, req.body, e.owner)
		assertErrorBody(t, req.path+" "+req.body, status, body, wantError{status: 400, code: gen.ErrorCodeInvalidRequest})
	}
	if e.plugins.calls != calls {
		t.Fatal("an incomplete request reached the registry")
	}
}

func TestPersonSearchAnswersCandidates(t *testing.T) {
	e := newFetchRouteEnv(t)
	e.plugins.people = []domain.PersonCandidate{
		{Source: "wiki", ID: "p-1", Name: "One", URL: "https://e.test/p/1", Detail: "actor"},
		{Source: "site", ID: "p-2", Name: "Two", URL: "https://e.test/p/2"},
	}
	status, body := e.post(personSearchPath, `{"query":"o"}`, e.owner)
	var got gen.PersonPluginSearchResult
	if status != http.StatusOK || json.Unmarshal(body, &got) != nil || len(got.Candidates) != 2 {
		t.Fatalf("person search = %d %s", status, body)
	}
	want := gen.PersonPluginCandidate{Source: "wiki", Id: "p-1", Name: "One", Url: "https://e.test/p/1", Detail: new("actor")}
	if c := got.Candidates[0]; c.Source != want.Source || c.Id != want.Id || c.Name != want.Name || c.Url != want.Url ||
		c.Detail == nil || *c.Detail != *want.Detail || got.Candidates[1].Detail != nil {
		t.Fatalf("candidates = %s", body)
	}
	calls := e.plugins.calls
	status, body = e.post(personSearchPath, `{}`, e.owner)
	assertErrorBody(t, "no query", status, body, wantError{status: 400, code: gen.ErrorCodeInvalidRequest})
	if e.plugins.calls != calls {
		t.Fatal("a request without a query reached the registry")
	}
}

func TestWorkInfoApplyAnswers(t *testing.T) {
	e := newFetchRouteEnv(t)
	e.plugins.applied = domain.WorkInfoApplied{
		Video:       domain.Video{ID: 7, Title: "Fetched", DisplayName: "Fetched"},
		SkippedTags: []domain.SkippedTag{{Name: "spoiler", Reason: domain.SkippedTagRejected}, {Name: "bad", Reason: domain.SkippedTagInvalid}},
	}
	status, body := e.post(applyPath, `{"fields":["title","tags"]}`, e.owner)
	var applied gen.WorkInfoFetchApplied
	if status != http.StatusOK || json.Unmarshal(body, &applied) != nil || applied.Video.Id != 7 || applied.Video.Title != "Fetched" ||
		len(applied.SkippedTags) != 2 || applied.SkippedTags[1].Reason != gen.SkippedTagReasonInvalid ||
		applied.LinkedPeople == nil || len(applied.LinkedPeople) != 0 {
		t.Fatalf("apply = %d %s; want linkedPeople []", status, body)
	}
	// Omitted lists choose nothing and an omitted coverIndex no cover.
	got := e.plugins.selection
	if !slices.Equal(got.Fields, []domain.WorkInfoField{domain.WorkInfoFieldTitle, domain.WorkInfoFieldTags}) ||
		got.Tags != nil || got.CoverIndex != -1 || len(got.People)+len(got.Organizations)+len(got.URLs)+len(got.ExternalIDs) != 0 {
		t.Fatalf("selection = %+v; want the two fields alone", got)
	}

	calls := e.plugins.calls
	for _, body := range []string{`{"fields":["dates"]}`, `{"fields":["people.actor"]}`} {
		status, got := e.post(applyPath, body, e.owner)
		assertErrorBody(t, body, status, got, wantError{status: 400, code: gen.ErrorCodeInvalidRequest})
	}
	if e.plugins.calls != calls {
		t.Fatal("an invalid apply reached the registry")
	}
	for _, tc := range []struct {
		err  error
		want wantError
	}{
		{domain.ErrWorkInfoApplyInvalid, wantError{status: 400, code: gen.ErrorCodeInvalidRequest}},
		{domain.ErrNotFound, wantError{status: 404, code: gen.ErrorCodeNotFound}},
		{domain.ErrWorkInfoFetchStale, wantError{status: 409, code: gen.ErrorCodeWorkInfoFetchStale}},
	} {
		e.plugins.err = tc.err
		status, body := e.post(applyPath, `{"fields":["title"]}`, e.owner)
		assertErrorBody(t, tc.err.Error(), status, body, tc.want)
	}
}

func TestWorkInfoApplyPassesTheSelectedTags(t *testing.T) {
	e := newFetchRouteEnv(t)
	status, body := e.post(applyPath, `{"fields":["tags"],"tags":["drama","comedy"]}`, e.owner)
	if status != http.StatusOK || !slices.Equal(e.plugins.selection.Tags, []string{"drama", "comedy"}) {
		t.Fatalf("apply = %d %s, tags %v", status, body, e.plugins.selection.Tags)
	}
	status, body = e.post(applyPath, `{"fields":["title","tags"]}`, e.owner)
	if status != http.StatusOK || e.plugins.selection.Tags != nil {
		t.Fatalf("apply without tags = %d %s, tags %v; want nil", status, body, e.plugins.selection.Tags)
	}

	calls := e.plugins.calls
	for _, body := range []string{
		`{"fields":["tags"],"tags":[]}`, `{"fields":["title"],"tags":["drama"]}`, `{"fields":["tags"],"tags":null}`,
	} {
		status, got := e.post(applyPath, body, e.owner)
		assertErrorBody(t, body, status, got, wantError{status: 400, code: gen.ErrorCodeInvalidRequest})
	}
	if e.plugins.calls != calls {
		t.Fatal("an invalid tag selection reached the registry")
	}
	e.plugins.err = domain.ErrWorkInfoApplyInvalid
	status, body = e.post(applyPath, `{"fields":["tags"],"tags":["horror"]}`, e.owner)
	assertErrorBody(t, "a name the result does not have", status, body, wantError{status: 400, code: gen.ErrorCodeInvalidRequest})
}

func TestWorkInfoApplyPassesCoverByPage(t *testing.T) {
	e := newFetchRouteEnv(t)
	for body, want := range map[string]bool{
		`{"fields":["cover"],"coverByPage":true}`: true,
		`{"fields":["cover"]}`:                    false,
	} {
		status, got := e.post(applyPath, body, e.owner)
		if status != http.StatusOK || e.plugins.pageCover != want {
			t.Fatalf("%s: apply = %d %s, coverByPage %v; want %v", body, status, got, e.plugins.pageCover, want)
		}
	}
}

func TestWorkInfoApplyPassesTheChosenEntriesAndValues(t *testing.T) {
	e := newFetchRouteEnv(t)
	e.plugins.applied = domain.WorkInfoApplied{Video: domain.Video{ID: 7}, LinkedPeople: []int64{4, 2}}
	status, body := e.post(applyPath, `{"fields":["cover","code"],"coverIndex":2,"people":[3,0],"organizations":[1],
		"urls":["https://e.test/w"],"externalIds":[{"source":"s","value":"1"}],"samePeople":[{"at":3,"personId":9}]}`,
		e.owner)
	var applied gen.WorkInfoFetchApplied
	if status != http.StatusOK || json.Unmarshal(body, &applied) != nil || !slices.Equal(applied.LinkedPeople, []int64{4, 2}) {
		t.Fatalf("apply = %d %s; want the linked people", status, body)
	}
	want := domain.WorkInfoSelection{
		Fields: []domain.WorkInfoField{domain.WorkInfoFieldCover, domain.WorkInfoFieldCode}, CoverIndex: 2,
		People: []int{3, 0}, Organizations: []int{1}, URLs: []string{"https://e.test/w"},
		ExternalIDs: []domain.ExternalID{{Source: "s", Value: "1"}}, SamePeople: []domain.SamePerson{{At: 3, PersonID: 9}},
	}
	if !reflect.DeepEqual(e.plugins.selection, want) {
		t.Fatalf("selection = %+v; want %+v", e.plugins.selection, want)
	}
}

func TestWorkInfoApplySucceedsWhenTheDetailIsPartlyUnreadable(t *testing.T) {
	plugins := &scriptedPlugins{applied: domain.WorkInfoApplied{
		Video:       domain.Video{ID: 7, Title: "Fetched", DisplayName: "Fetched"},
		SkippedTags: []domain.SkippedTag{{Name: "spoiler", Reason: domain.SkippedTagRejected}},
	}}
	catalog := &fakeCatalog{groupErr: errors.New("database is locked")}
	env := newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options {
		return Options{WorkInfoPlugins: plugins, Catalog: catalog}
	})
	owner := env.setup()
	rec := env.serve(authRequest{method: http.MethodPost, target: applyPath, body: `{"fields":["title"]}`, cookies: []*http.Cookie{owner}})
	var applied gen.WorkInfoFetchApplied
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &applied) != nil || applied.Video.Id != 7 ||
		applied.Video.Group != nil || len(applied.SkippedTags) != 1 {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(env.logs.String(), "database is locked") {
		t.Fatalf("the unreadable detail was not logged: %s", env.logs.String())
	}
}

func TestWorkInfoFetchRoutesAreOwnerOnly(t *testing.T) {
	e := newFetchRouteEnv(t)
	for _, req := range []struct{ path, body string }{
		{searchPath, `{"query":"work"}`},
		{fetchPath, `{"pluginId":1,"id":"ABC-123"}`},
		{applyPath, `{"fields":["title"]}`},
	} {
		assertUnauthenticated(t, req.path, e.serve(authRequest{method: http.MethodPost, target: req.path, body: req.body}))
	}
	if e.plugins.calls != 0 {
		t.Fatal("a guest request reached the registry")
	}
}

func TestWorkInfoFetchCoverServesTheImageWithItsType(t *testing.T) {
	e := newFetchRouteEnv(t)
	e.plugins.fetch = domain.WorkInfoFetchResult{
		ID: "abc", Tags: []string{}, Covers: []domain.WorkInfoCover{{Width: 3, Height: 2}, {Width: 5, Height: 4}},
	}
	status, body := e.post(fetchPath, `{"pluginId":1,"id":"ABC-123"}`, e.owner)
	var fetch gen.WorkInfoFetch
	want := []gen.WorkInfoFetchedCover{{Width: 3, Height: 2}, {Width: 5, Height: 4}}
	if status != http.StatusCreated || json.Unmarshal(body, &fetch) != nil || !slices.Equal(fetch.Covers, want) {
		t.Fatalf("fetch = %d %s; want the covers' sizes in order", status, body)
	}

	e.plugins.covers = map[int]domain.WorkInfoCover{1: {Data: []byte("png bytes"), ContentType: "image/png"}}
	coverPath := "/api/videos/7/work-info-fetches/abc/covers/1"
	rec := e.serve(authRequest{method: http.MethodGet, target: "/api/videos/7/work-info-fetches/abc/covers/0",
		cookies: []*http.Cookie{e.owner}})
	assertErrorBody(t, "no cover at 0", rec.Code, rec.Body.Bytes(), wantError{status: 404, code: gen.ErrorCodeNotFound})

	rec = e.serve(authRequest{method: http.MethodGet, target: coverPath, cookies: []*http.Cookie{e.owner}})
	if rec.Code != http.StatusOK || rec.Body.String() != "png bytes" || rec.Header().Get("Content-Type") != "image/png" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("cover = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	rec = e.serve(authRequest{method: http.MethodGet, target: coverPath})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("guest cover = %d, want 401", rec.Code)
	}
}

func TestWorkInfoFetchPersonImageServesTheKeptImageByIndex(t *testing.T) {
	e := newFetchRouteEnv(t)
	height := 160
	e.plugins.fetch = domain.WorkInfoFetchResult{ID: "abc", Tags: []string{}, Details: domain.FetchedWorkInfo{People: []domain.FetchedPerson{
		{Role: domain.PersonPerformer, Name: "A"},
		{Role: domain.PersonPerformer, Name: "B", Image: &domain.WorkInfoCover{Data: []byte("jpeg bytes")},
			Profile: domain.PersonProfile{HeightCm: &height, BloodType: "unknown"}},
	}}}
	status, body := e.post(fetchPath, `{"pluginId":1,"id":"ABC-123"}`, e.owner)
	var fetch gen.WorkInfoFetch
	if status != http.StatusCreated || json.Unmarshal(body, &fetch) != nil || len(fetch.People) != 2 {
		t.Fatalf("fetch = %d %s", status, body)
	}
	if fetch.People[0].HasImage != nil || fetch.People[0].Profile != nil {
		t.Errorf("person A = %+v; want neither an image nor a profile", fetch.People[0])
	}
	b := fetch.People[1]
	if b.HasImage == nil || !*b.HasImage || b.Profile == nil || b.Profile.HeightCm == nil || *b.Profile.HeightCm != 160 ||
		b.Profile.BloodType != nil {
		t.Errorf("person B = %+v; want the image flag and the valid height alone", b)
	}

	e.plugins.personImages = map[int]domain.WorkInfoCover{1: {Data: []byte("jpeg bytes"), ContentType: "image/jpeg"}}
	path := func(fetchID string, index int) string {
		return "/api/videos/7/work-info-fetches/" + fetchID + "/people/" + strconv.Itoa(index) + "/image"
	}
	rec := e.serve(authRequest{method: http.MethodGet, target: path("abc", 1), cookies: []*http.Cookie{e.owner}})
	if rec.Code != http.StatusOK || rec.Body.String() != "jpeg bytes" || rec.Header().Get("Content-Type") != "image/jpeg" ||
		rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("image = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	rec = e.serve(authRequest{method: http.MethodGet, target: path("abc", 0), cookies: []*http.Cookie{e.owner}})
	assertErrorBody(t, "no image", rec.Code, rec.Body.Bytes(),
		wantError{status: 404, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonImageNotFound})
	rec = e.serve(authRequest{method: http.MethodGet, target: path("gone", 1), cookies: []*http.Cookie{e.owner}})
	assertErrorBody(t, "expired fetch", rec.Code, rec.Body.Bytes(), wantError{status: 404, code: gen.ErrorCodeNotFound})
	rec = e.serve(authRequest{method: http.MethodGet, target: path("abc", 1)})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("guest image = %d, want 401", rec.Code)
	}
}
