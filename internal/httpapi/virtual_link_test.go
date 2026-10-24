package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// linkPlugins records the fetches the create route starts.
type linkPlugins struct {
	WorkInfoPlugins
	started []string
}

func (p *linkPlugins) StartLinkFetch(video domain.Video, link string) {
	p.started = append(p.started, video.ContentKey+" "+link)
}

// Creating from a link stores the link with an optional title, starts its fetch after the store call, and answers
// with the running fetch; a link that is not http or https, or a body with neither field, is refused.
func TestCreateVirtualVideoFromLink(t *testing.T) {
	video := virtualVideo(5, "https://example.com/v/1")
	store := &fakeVirtualVideos{video: video, linkFetch: &domain.LinkFetch{State: domain.LinkFetchRunning}}
	plugins := &linkPlugins{}
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{}}, VirtualVideos: store,
		WorkInfoPlugins: plugins})

	rec := jsonRequest(t, handler, http.MethodPost, "/api/virtual-videos", `{"url":" https://example.com/v/1 "}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body)
	}
	got := decode[gen.Video](t, rec)
	if got.LinkFetch == nil || got.LinkFetch.State != gen.VirtualLinkFetchStateRunning {
		t.Errorf("linkFetch = %+v; want running", got.LinkFetch)
	}
	want := []virtualCall{{op: "create-link", info: domain.WorkInfo{URLs: []string{" https://example.com/v/1 "}}}}
	if !reflect.DeepEqual(store.calls, want) || !reflect.DeepEqual(plugins.started, []string{"virtual:5 https://example.com/v/1"}) {
		t.Errorf("store calls %+v, started %v", store.calls, plugins.started)
	}

	store.err = domain.ErrInvalidVirtualLink
	rec = jsonRequest(t, handler, http.MethodPost, "/api/virtual-videos", `{"url":"example.com/v/1","title":"t"}`)
	assertErrorBody(t, "a link without a scheme", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	rec = jsonRequest(t, handler, http.MethodPost, "/api/virtual-videos", `{}`)
	assertErrorBody(t, "no title and no url", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	if len(plugins.started) != 1 {
		t.Errorf("a refused create started a fetch: %v", plugins.started)
	}
}

// Without plugins the created video's fetch ends at once as failed, so the page never waits on it.
func TestCreateVirtualVideoFromLinkWithoutPlugins(t *testing.T) {
	store := &fakeVirtualVideos{video: virtualVideo(5, "t")}
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{}}, VirtualVideos: store})
	rec := jsonRequest(t, handler, http.MethodPost, "/api/virtual-videos", `{"url":"https://example.com/v/1"}`)
	if rec.Code != http.StatusCreated || len(store.calls) != 2 || store.calls[1].op != "finish" ||
		store.calls[1].title == "" {
		t.Fatalf("status %d, calls %+v; want the fetch finished as failed", rec.Code, store.calls)
	}
}

type linkFetchPlugins struct {
	scriptedPlugins
	links []string
}

func (p *linkFetchPlugins) FetchWorkInfoByLink(_ context.Context, _, _ int64, link string) (domain.WorkInfoFetchResult, error) {
	p.links = append(p.links, link)
	return p.fetch, p.err
}

// A fetch sends exactly one of id and link, and a link outside the plugin's domains has its own code.
func TestFetchWorkInfoByLinkRoute(t *testing.T) {
	plugins := &linkFetchPlugins{}
	plugins.fetch = domain.WorkInfoFetchResult{ID: "abc"}
	handler := newTestServer(t, Options{WorkInfoPlugins: plugins})
	rec := jsonRequest(t, handler, http.MethodPost, "/api/videos/1/work-info-fetches", `{"pluginId":2,"link":"https://e.test/v"}`)
	if rec.Code != http.StatusCreated || !reflect.DeepEqual(plugins.links, []string{"https://e.test/v"}) || plugins.calls != 0 {
		t.Fatalf("status %d, links %v, id fetches %d: %s", rec.Code, plugins.links, plugins.calls, rec.Body)
	}
	rec = jsonRequest(t, handler, http.MethodPost, "/api/videos/1/work-info-fetches", `{"pluginId":2,"link":"x","id":"y"}`)
	assertErrorBody(t, "both id and link", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	plugins.err = domain.ErrWorkInfoLinkUnsupported
	rec = jsonRequest(t, handler, http.MethodPost, "/api/videos/1/work-info-fetches", `{"pluginId":2,"link":"https://other.test/v"}`)
	assertErrorBody(t, "a link of another site", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeWorkInfoLinkUnsupported})
}
