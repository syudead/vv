package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
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
	fetch      domain.WorkInfoFetchResult
	applied    domain.WorkInfoApplied
	calls      int
	fields     []domain.WorkInfoField
	tags       []string
	ids        []string
	cover      *domain.WorkInfoCover
}

func (p *scriptedPlugins) SearchWorkInfo(context.Context, int64, string) ([]domain.WorkInfoCandidate, error) {
	p.calls++
	return p.candidates, p.err
}

func (p *scriptedPlugins) FetchWorkInfo(_ context.Context, _, _ int64, id string) (domain.WorkInfoFetchResult, error) {
	p.calls++
	p.ids = append(p.ids, id)
	return p.fetch, p.err
}

func (p *scriptedPlugins) WorkInfoFetchCover(int64, string) (domain.WorkInfoCover, error) {
	if p.cover == nil {
		return domain.WorkInfoCover{}, domain.ErrNotFound
	}
	return *p.cover, nil
}

func (p *scriptedPlugins) ApplyWorkInfoFetch(_ context.Context, _ int64, _ string, fields []domain.WorkInfoField, tags []string) (domain.WorkInfoApplied, error) {
	p.calls++
	p.fields, p.tags = fields, tags
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
	searchPath = "/api/work-info-plugins/1/search"
	fetchPath  = "/api/videos/7/work-info-fetches"
	applyPath  = "/api/videos/7/work-info-fetches/abc/apply"
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

func TestWorkInfoApplyAnswers(t *testing.T) {
	e := newFetchRouteEnv(t)
	e.plugins.applied = domain.WorkInfoApplied{
		Video:       domain.Video{ID: 7, Title: "Fetched", DisplayName: "Fetched"},
		SkippedTags: []domain.SkippedTag{{Name: "spoiler", Reason: domain.SkippedTagRejected}, {Name: "bad", Reason: domain.SkippedTagInvalid}},
	}
	status, body := e.post(applyPath, `{"fields":["title","tags"]}`, e.owner)
	var applied gen.WorkInfoFetchApplied
	if status != http.StatusOK || json.Unmarshal(body, &applied) != nil || applied.Video.Id != 7 || applied.Video.Title != "Fetched" ||
		len(applied.SkippedTags) != 2 || applied.SkippedTags[1].Reason != gen.SkippedTagReasonInvalid {
		t.Fatalf("apply = %d %s", status, body)
	}
	if !slices.Equal(e.plugins.fields, []domain.WorkInfoField{domain.WorkInfoFieldTitle, domain.WorkInfoFieldTags}) {
		t.Fatalf("fields = %v", e.plugins.fields)
	}

	calls := e.plugins.calls
	for _, body := range []string{`{"fields":[]}`, `{}`, `{"fields":["dates"]}`, `{"fields":["people.actor"]}`} {
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
	if status != http.StatusOK || !slices.Equal(e.plugins.tags, []string{"drama", "comedy"}) {
		t.Fatalf("apply = %d %s, tags %v", status, body, e.plugins.tags)
	}
	status, body = e.post(applyPath, `{"fields":["title","tags"]}`, e.owner)
	if status != http.StatusOK || e.plugins.tags != nil {
		t.Fatalf("apply without tags = %d %s, tags %v; want nil", status, body, e.plugins.tags)
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
		ID: "abc", Tags: []string{}, Cover: &domain.WorkInfoCover{Width: 3, Height: 2},
	}
	status, body := e.post(fetchPath, `{"pluginId":1,"id":"ABC-123"}`, e.owner)
	var fetch gen.WorkInfoFetch
	if status != http.StatusCreated || json.Unmarshal(body, &fetch) != nil || fetch.Cover == nil ||
		fetch.Cover.Width != 3 || fetch.Cover.Height != 2 {
		t.Fatalf("fetch = %d %s; want the cover's size", status, body)
	}

	coverPath := "/api/videos/7/work-info-fetches/abc/cover"
	rec := e.serve(authRequest{method: http.MethodGet, target: coverPath, cookies: []*http.Cookie{e.owner}})
	assertErrorBody(t, "no cover", rec.Code, rec.Body.Bytes(), wantError{status: 404, code: gen.ErrorCodeNotFound})

	e.plugins.cover = &domain.WorkInfoCover{Data: []byte("png bytes"), ContentType: "image/png"}
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
