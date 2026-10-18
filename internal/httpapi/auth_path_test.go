package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ServeMux decodes individual escaped segments, so classifying only fully decoded paths could expose protected routes.
func TestClassifyRequestFailsClosedOnEscapedSeparators(t *testing.T) {
	cases := []struct {
		method, target string
		want           access
		api            bool
	}{
		// The decoded path is /index.html, but ServeMux selects GET /api/videos/{id}.
		{http.MethodGet, "/api/videos/x%2F..%2F..%2F..%2Findex.html", accessOwner, true},
		// The decoded path is /a/stream, but ServeMux selects GET /api/videos/{id}/stream.
		{http.MethodGet, "/api/videos/1%2F..%2F..%2F..%2Fa/stream", accessOwner, true},
		// ServeMux selects the wildcard video route, not decoded /api/videos/ids.
		{http.MethodGet, "/api/videos%2Fids", accessOwner, true},
		{http.MethodPost, "/api/tags/1%2F..%2F..%2Fauth%2Flogin/merge", accessOwner, true},
		{http.MethodDelete, "/api/tags/1%2F..%2F..%2F..%2Fx", accessOwner, true},
		{http.MethodGet, "/%61pi/videos", accessGuest, true},
		{http.MethodGet, "/api/../api/health", accessPublic, true},
		{http.MethodGet, "/api%2Fvideos", accessOwner, true},
		{http.MethodGet, "/%2E%2E/api/videos", accessOwner, true},
		{http.MethodGet, "/videos/1", accessPublic, false},
		{http.MethodGet, "/videos/a%2Fb", accessPublic, false},
		{http.MethodGet, "/api%2F..%2Findex.html", accessPublic, false},
	}
	for _, c := range cases {
		got, api := classifyRequest(httptest.NewRequest(c.method, c.target, nil))
		if got != c.want || api != c.api {
			t.Errorf("%s %s: 扱い = %s, api = %t; want %s, %t", c.method, c.target, got, api, c.want, c.api)
		}
	}
}

func TestAuthEscapedSeparatorsDoNotBypassBoundary(t *testing.T) {
	env := newAuthEnv(t, t.TempDir(), Options{Videos: sampleLibrary()})
	env.setup()

	for _, target := range []string{
		"/api/videos/x%2F..%2F..%2F..%2Findex.html",
		"/api/videos/1%2F..%2F..%2F..%2Fa/stream",
		"/api/folders/1%2F..%2F..%2F..%2Fa/videos",
		"/api/videos%2Fids",
	} {
		rec := env.get(target)
		assertUnauthenticated(t, target, rec)
		assertAudience(t, target, rec, "guest")
	}
}
