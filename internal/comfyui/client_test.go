package comfyui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fixedSettings struct {
	settings domain.ComfyUISettings
	err      error
}

func (f fixedSettings) ComfyUISettings(context.Context) (domain.ComfyUISettings, error) {
	return f.settings, f.err
}

func selfHosted(url string) fixedSettings {
	return fixedSettings{settings: domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: url}}
}

func cloud(key string) fixedSettings {
	return fixedSettings{settings: domain.ComfyUISettings{Provider: domain.ComfyUICloud, APIKey: key}}
}

// serve は 1 つの経路だけに答える偽の提供元を立てる。ほかの経路は失敗にする。
func serve(t *testing.T, path string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != path {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

// 自前の ComfyUI は /system_stats に答えれば使える。キーは送らない。
func TestCheckSelfHostedSystemStats(t *testing.T) {
	server := serve(t, "/system_stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "" {
			t.Errorf("self-hosted request carried a key")
		}
		_, _ = w.Write([]byte(`{"system":{"os":"posix","comfyui_version":"0.3.60"},"devices":[]}`))
	})
	settings := selfHosted(server.URL)
	settings.settings.APIKey = "kept-for-cloud"
	if err := New(settings).Check(context.Background()); err != nil {
		t.Fatalf("Check = %v", err)
	}
}

// Comfy Cloud は /jobs?limit=1 に X-API-Key を付けて問い合わせ、200 なら使える。
func TestCheckCloudSendsKey(t *testing.T) {
	server := serve(t, "/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" || r.URL.Query().Get("limit") != "1" {
			t.Errorf("key = %q, query = %q", r.Header.Get("X-API-Key"), r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"jobs":[]}`))
	})
	client := NewWithOptions(cloud("secret"), Options{CloudBaseURL: server.URL + "/api"})
	if err := client.Check(context.Background()); err != nil {
		t.Fatalf("Check = %v", err)
	}
}

// 使えない理由（つながらない、ComfyUI でない、キーが拒まれた、何も保存されていない）を
// それぞれ別の文で伝え、相手の本文は写さない。
func TestCheckReportsWhyItFailed(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	html := serve(t, "/system_stats", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>other service secret</html>`))
	})
	notFound := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(notFound.Close)
	noSystem := serve(t, "/system_stats", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"1"}`))
	})
	rejected := serve(t, "/api/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	cases := []struct {
		name   string
		client *Client
		want   string
	}{
		{"届かない", New(selfHosted(closedURL)), "cannot reach ComfyUI"},
		{"JSON でない", New(selfHosted(html.URL)), "is not ComfyUI (/system_stats is not JSON)"},
		{"経路が無い", New(selfHosted(notFound.URL)), "is not ComfyUI (/system_stats answered 404)"},
		{"system が無い", New(selfHosted(noSystem.URL)), "is not ComfyUI (/system_stats has no system information)"},
		{"キーが拒まれた", NewWithOptions(cloud("bad"), Options{CloudBaseURL: rejected.URL + "/api"}), "API key was rejected by Comfy Cloud"},
		{"何も保存されていない", New(fixedSettings{}), "no ComfyUI connection is saved"},
		{"Cloud のキーが無い", New(cloud("")), "Comfy Cloud needs an API key"},
	}
	seen := map[string]string{}
	for _, c := range cases {
		err := c.client.Check(context.Background())
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
			continue
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: the reply body leaked: %v", c.name, err)
		}
		if other, ok := seen[err.Error()]; ok {
			t.Errorf("%s and %s share the message %q", c.name, other, err)
		}
		seen[err.Error()] = c.name
	}
}

// 確かめは転送に従わず、転送先へキーを持ち出さない。
func TestCheckDoesNotFollowRedirects(t *testing.T) {
	var followed bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed = true }))
	t.Cleanup(target.Close)
	server := serve(t, "/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/jobs", http.StatusFound)
	})
	err := NewWithOptions(cloud("secret"), Options{CloudBaseURL: server.URL + "/api"}).Check(context.Background())
	if err == nil || followed {
		t.Fatalf("err = %v, followed = %v", err, followed)
	}
}

func TestCheckReportsUnreadableSettings(t *testing.T) {
	err := New(fixedSettings{err: errors.New("disk")}).Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "cannot read the ComfyUI settings") {
		t.Fatalf("err = %v", err)
	}
}
