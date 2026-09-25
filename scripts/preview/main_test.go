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

	// 公開側と同じ Origin は、vv から見た origin に書き換わる。
	r := httptest.NewRequest(http.MethodPost, "/api/scans", nil)
	r.Host = "x-8080.app.github.dev"
	r.Header.Set("Origin", "https://x-8080.app.github.dev")
	proxy.ServeHTTP(httptest.NewRecorder(), r)
	if want := "http://" + target.Host; gotOrigin != want || gotHost != target.Host {
		t.Errorf("Origin=%q Host=%q, want both to match %q", gotOrigin, gotHost, want)
	}

	// 別サイトの Origin はそのまま渡し、vv 自身に断らせる。
	r = httptest.NewRequest(http.MethodPost, "/api/scans", nil)
	r.Host = "x-8080.app.github.dev"
	r.Header.Set("Origin", "https://evil.example")
	proxy.ServeHTTP(httptest.NewRecorder(), r)
	if gotOrigin != "https://evil.example" {
		t.Errorf("別サイトの Origin を書き換えた: %q", gotOrigin)
	}
}
