package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSameSiteOrigin(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		headers map[string]string
		want    bool
	}{
		{"Origin なし", "localhost:8080", nil, false},
		{"ブラウザが same-origin と言う", "localhost:8080",
			map[string]string{"Origin": "https://x-8080.app.github.dev", "Sec-Fetch-Site": "same-origin"}, true},
		{"cross-site は Host が一致しても通さない", "evil.example",
			map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}, false},
		{"same-site は別 origin なので通さない", "x-8080.app.github.dev",
			map[string]string{"Origin": "https://y-8080.app.github.dev", "Sec-Fetch-Site": "same-site"}, false},
		{"Sec-Fetch-Site なしで Host と一致", "x-8080.app.github.dev",
			map[string]string{"Origin": "https://x-8080.app.github.dev"}, true},
		{"Sec-Fetch-Site なしで X-Forwarded-Host と一致", "localhost:8080",
			map[string]string{"Origin": "https://x-8080.app.github.dev", "X-Forwarded-Host": "x-8080.app.github.dev"}, true},
		{"Sec-Fetch-Site なしで一致しない", "localhost:8080",
			map[string]string{"Origin": "https://evil.example"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/scans", nil)
			r.Host = c.host
			for k, v := range c.headers {
				r.Header.Set(k, v)
			}
			if got := sameSiteOrigin(r); got != c.want {
				t.Errorf("sameSiteOrigin = %v, want %v", got, c.want)
			}
		})
	}
}

func TestProxyRewritesOnlySameSiteOrigin(t *testing.T) {
	var gotOrigin, gotHost string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrigin = r.Header.Get("Origin")
		gotHost = r.Host
	}))
	defer backend.Close()
	target, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newProxy(target)

	// 公開側と同じ Origin は、vv から見た origin（http と受けた Host）に書き換わる。
	// Host はそのまま渡す。ループバックに書き換えると、vv がローカル限定の操作を
	// 外からの要求にも許してしまう。
	r := httptest.NewRequest(http.MethodPost, "/api/scans", nil)
	r.Host = "x-8080.app.github.dev"
	r.Header.Set("Origin", "https://x-8080.app.github.dev")
	proxy.ServeHTTP(httptest.NewRecorder(), r)
	if gotHost != "x-8080.app.github.dev" {
		t.Errorf("Host を書き換えた: %q", gotHost)
	}
	if gotOrigin != "http://x-8080.app.github.dev" {
		t.Errorf("Origin = %q, want http://x-8080.app.github.dev", gotOrigin)
	}

	// 別サイトからの書き込みと Origin の無い書き込みは、vv へ渡さずに断る。
	for _, origin := range []string{"https://evil.example", ""} {
		gotOrigin = "not reached"
		r = httptest.NewRequest(http.MethodDelete, "/api/tags/1", nil)
		r.Host = "x-8080.app.github.dev"
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		recorder := httptest.NewRecorder()
		proxy.ServeHTTP(recorder, r)
		if recorder.Code != http.StatusForbidden || gotOrigin != "not reached" {
			t.Errorf("Origin=%q の書き込みを通した: status %d", origin, recorder.Code)
		}
	}

	// 読み取りは Origin が無くても通す。
	gotOrigin = "not reached"
	r = httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	proxy.ServeHTTP(httptest.NewRecorder(), r)
	if gotOrigin == "not reached" {
		t.Error("読み取りを vv へ渡さなかった")
	}
}

func TestRunningPreviewTellsPreviewFromOtherServers(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer other.Close()
	if _, err := runningPreview(other.Listener.Addr().String()); err == nil {
		t.Error("preview 以外のサーバーをエラーにしなかった")
	}

	preview := httptest.NewServer(newProxy(&url.URL{Scheme: "http", Host: "127.0.0.1:1"}))
	defer preview.Close()
	if running, err := runningPreview(preview.Listener.Addr().String()); err != nil || running == "" {
		t.Errorf("動いている preview を見つけなかった: %q, %v", running, err)
	}
}

func TestBackendEnvironDropsDisplay(t *testing.T) {
	got := backendEnviron([]string{"PATH=/bin", "DISPLAY=:0", "WAYLAND_DISPLAY=wayland-0", "HOME=/root"})
	want := []string{"PATH=/bin", "HOME=/root"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("backendEnviron = %q, want %q", got, want)
	}
}
