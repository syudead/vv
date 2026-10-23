package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// Use real auth, storage, and app.WorkInfoPlugins; only plugin execution is faked.

type fetchedRuntime struct {
	manifestRuntime
	work *domain.FetchedWorkInfo
}

func (r fetchedRuntime) Run(
	context.Context, string, []byte, domain.WorkInfoRunRequest, domain.WorkInfoRunNetwork, func(domain.WorkInfoRunLog) error,
) (domain.WorkInfoRunResult, domain.WorkInfoRunLog, error) {
	return domain.WorkInfoRunResult{WorkInfo: *r.work}, domain.WorkInfoRunLog{}, nil
}

type detailsEnv struct {
	*authEnv
	owner    *http.Cookie
	work     *domain.FetchedWorkInfo
	videoID  int64
	pluginID int64
}

func newDetailsEnv(t *testing.T) *detailsEnv {
	t.Helper()
	ctx := context.Background()
	work := &domain.FetchedWorkInfo{}
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		library := db.Library()
		return Options{Videos: library, Overrides: db.Overrides(), WorkInfo: db.WorkInfo(), Credits: db.Credits(),
			WorkInfoPlugins: app.NewWorkInfoPlugins(app.WorkInfoPluginsOptions{
				Store: db.WorkInfoPlugins(), Runtime: fetchedRuntime{work: work}, Videos: library,
				Applier: db.Overrides(), Matcher: db.Credits(),
			})}
	})
	e := &detailsEnv{authEnv: env, owner: env.setup(), work: work}
	mediaDir := t.TempDir()
	if _, err := env.db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	video, err := env.db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(mediaDir, "work.mp4"), Title: "work", ContentKey: "content-key-work",
		SizeBytes: 1, MTime: time.Unix(0, 0), AddedAt: time.Unix(0, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	e.videoID = video.ID
	plugins := env.db.WorkInfoPlugins()
	saved, _, err := plugins.SaveWorkInfoPlugin(ctx, domain.WorkInfoPluginUpload{
		FileName: "works.wasm", Module: []byte("module"), Digest: "digest",
		Manifest: domain.WorkInfoPluginManifest{
			Contract: 1, Name: "example.works", Version: "1.0.0",
			Operations: []domain.WorkInfoOperation{domain.WorkInfoFetch},
		},
	}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plugins.ApproveWorkInfoPlugin(ctx, saved.ID, "digest", time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	e.pluginID = saved.ID
	return e
}

func (e *detailsEnv) videoPath() string {
	return "/api/videos/" + strconv.FormatInt(e.videoID, 10) + "/work-info-fetches"
}

func (e *detailsEnv) fetch(work domain.FetchedWorkInfo) []byte {
	e.t.Helper()
	*e.work = work
	body := `{"pluginId":` + strconv.FormatInt(e.pluginID, 10) + `,"id":"W-1"}`
	rec := e.serve(authRequest{method: http.MethodPost, target: e.videoPath(), body: body, cookies: []*http.Cookie{e.owner}})
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("fetch = %d %s", rec.Code, rec.Body)
	}
	return rec.Body.Bytes()
}

func (e *detailsEnv) apply(fetchID, body string) gen.WorkInfoFetchApplied {
	e.t.Helper()
	rec := e.serve(authRequest{
		method: http.MethodPost, target: e.videoPath() + "/" + fetchID + "/apply", body: body,
		cookies: []*http.Cookie{e.owner},
	})
	var applied gen.WorkInfoFetchApplied
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &applied) != nil {
		e.t.Fatalf("apply %s = %d %s", body, rec.Code, rec.Body)
	}
	return applied
}

func fetchIDOf(t *testing.T, body []byte) string {
	t.Helper()
	var fetch gen.WorkInfoFetch
	if err := json.Unmarshal(body, &fetch); err != nil {
		t.Fatal(err)
	}
	return fetch.FetchId
}

func TestWorkInfoFetchAnswersTheDetailsWithMatches(t *testing.T) {
	e := newDetailsEnv(t)
	aoi, err := e.db.Credits().Create(context.Background(), domain.CreditPerson, domain.CreditInput{
		Name: "Aoi Sora", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := e.fetch(domain.FetchedWorkInfo{
		Title: "Title", Tags: []string{"drama"}, Code: "ABC-123",
		Dates: []domain.WorkDate{{Kind: domain.DatePublished, Date: "2024"}, {Kind: domain.DateRelease, Date: "March 2024"}},
		People: []domain.FetchedPerson{
			{Role: domain.PersonDirector, Name: "Kenji"},
			{Role: domain.PersonPerformer, Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-1"}}},
		},
		Organizations: []domain.FetchedOrganization{{Role: domain.OrganizationMaker, Name: "Maker"}},
		URLs:          []string{"https://example.com/works/1"},
		ExternalIDs:   []domain.ExternalID{{Source: "fanza", Value: "abc123"}},
	})
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	delete(got, "fetchId")
	delete(got, "expiresAt")
	want := map[string]any{
		"title": "Title", "tags": []any{"drama"}, "code": "ABC-123",
		"dates": []any{
			map[string]any{"kind": "release", "date": "March 2024"},
			map[string]any{"kind": "published", "date": "2024"},
		},
		"people": []any{
			map[string]any{"role": "performer", "name": "Aoi",
				"externalIds": []any{map[string]any{"source": "fanza", "value": "a-1"}},
				"match":       map[string]any{"state": "existing", "id": float64(aoi.ID), "name": "Aoi Sora"}},
			map[string]any{"role": "director", "name": "Kenji", "externalIds": []any{},
				"match": map[string]any{"state": "new"}},
		},
		"organizations": []any{map[string]any{"role": "maker", "name": "Maker", "externalIds": []any{},
			"match": map[string]any{"state": "new"}}},
		"urls":        []any{"https://example.com/works/1"},
		"externalIds": []any{map[string]any{"source": "fanza", "value": "abc123"}},
		"covers":      []any{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fetch body = %s;\nwant %v", body, want)
	}

	plain := map[string]any{}
	if err := json.Unmarshal(e.fetch(domain.FetchedWorkInfo{Title: "Title", Tags: []string{"drama"}}), &plain); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"dates", "people", "organizations", "urls", "externalIds"} {
		if list, ok := plain[key].([]any); !ok || len(list) != 0 {
			t.Fatalf("%s = %v; want []", key, plain[key])
		}
	}
	if _, ok := plain["code"]; ok {
		t.Fatalf("a title-and-tags result has a code: %v", plain)
	}
	if _, ok := plain["description"]; ok {
		t.Fatalf("a title-and-tags result has a description: %v", plain)
	}
}

func TestWorkInfoApplyWritesTheSelectedDetails(t *testing.T) {
	e := newDetailsEnv(t)
	fetchID := fetchIDOf(t, e.fetch(domain.FetchedWorkInfo{
		Code: "ABC-123", Description: "Not selected",
		Dates:  []domain.WorkDate{{Kind: domain.DateRelease, Date: "2024-03-01"}},
		People: []domain.FetchedPerson{{Role: domain.PersonPerformer, Name: "Mika"}},
	}))
	applied := e.apply(fetchID, `{"fields":["code","dates.release"],"people":[0]}`)
	info := applied.Video.WorkInfo
	if info == nil || info.Code == nil || *info.Code != "ABC-123" || info.Description != nil {
		t.Fatalf("workInfo = %+v; want the code and no description", info)
	}
	if want := []gen.WorkDate{{Kind: gen.Release, Date: "2024-03-01"}}; !reflect.DeepEqual(info.Dates, want) {
		t.Fatalf("dates = %+v; want %+v", info.Dates, want)
	}
	if len(info.People) != 1 || info.People[0].Role != gen.Performer || info.People[0].Person.Name != "Mika" ||
		len(applied.LinkedPeople) != 1 || applied.LinkedPeople[0] != info.People[0].Person.Id {
		t.Fatalf("people = %+v, linked %v; want the new performer Mika linked", info.People, applied.LinkedPeople)
	}
	if applied.Skipped == nil || len(applied.Skipped) != 0 || applied.SkippedTags == nil {
		t.Fatalf("skipped = %+v %+v; want empty lists", applied.Skipped, applied.SkippedTags)
	}
}

func TestWorkInfoApplySkipsAMalformedDate(t *testing.T) {
	e := newDetailsEnv(t)
	fetchID := fetchIDOf(t, e.fetch(domain.FetchedWorkInfo{
		Code:  "ABC-123",
		Dates: []domain.WorkDate{{Kind: domain.DateRelease, Date: "March 2024"}},
	}))
	applied := e.apply(fetchID, `{"fields":["code","dates.release"]}`)
	want := []gen.SkippedWorkInfoValue{{Field: gen.WorkInfoFieldDatesRelease, Value: "March 2024", Reason: gen.SkippedWorkInfoReasonInvalid}}
	if !reflect.DeepEqual(applied.Skipped, want) {
		t.Fatalf("skipped = %+v; want %+v", applied.Skipped, want)
	}
	info := applied.Video.WorkInfo
	if info == nil || info.Code == nil || *info.Code != "ABC-123" || len(info.Dates) != 0 {
		t.Fatalf("workInfo = %+v; want the code without a date", info)
	}
}

// A virtual video has no registered location, yet the apply reaches it and answers its new title.
func TestWorkInfoApplyAnswersAVirtualVideo(t *testing.T) {
	e := newDetailsEnv(t)
	virtual, err := e.db.VirtualVideos().Create(context.Background(), "Before")
	if err != nil {
		t.Fatal(err)
	}
	e.videoID = virtual.ID
	fetchID := fetchIDOf(t, e.fetch(domain.FetchedWorkInfo{Title: "Fetched", Description: "Notes"}))
	applied := e.apply(fetchID, `{"fields":["title","description"]}`)
	if applied.Video.Id != virtual.ID || applied.Video.Title != "Fetched" || applied.Video.WorkInfo == nil ||
		applied.Video.WorkInfo.Description == nil || *applied.Video.WorkInfo.Description != "Notes" {
		t.Fatalf("applied video = %+v; want the virtual video with the fetched title and description", applied.Video)
	}
}

func TestWorkInfoDetailRoutesRefuseGuests(t *testing.T) {
	e := newDetailsEnv(t)
	fetchID := fetchIDOf(t, e.fetch(domain.FetchedWorkInfo{Code: "ABC-123"}))
	fetchBody := `{"pluginId":` + strconv.FormatInt(e.pluginID, 10) + `,"id":"W-1"}`
	for _, req := range []struct{ path, body string }{
		{e.videoPath(), fetchBody},
		{e.videoPath() + "/" + fetchID + "/apply", `{"fields":["code"]}`},
	} {
		assertUnauthenticated(t, req.path, e.serve(authRequest{method: http.MethodPost, target: req.path, body: req.body}))
	}
	if applied := e.apply(fetchID, `{"fields":["code"]}`); applied.Video.WorkInfo == nil || *applied.Video.WorkInfo.Code != "ABC-123" {
		t.Fatalf("the owner could not apply after a guest was refused: %+v", applied.Video.WorkInfo)
	}
}
