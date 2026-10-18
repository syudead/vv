package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func fakeOllama(t *testing.T, tags string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.40.2"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(tags))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func withModel(url, model string) fixedSettings {
	return fixedSettings{settings: domain.OllamaSettings{Endpoint: url, Model: model}}
}

func TestCheckFindsModelWithoutClassifying(t *testing.T) {
	server := fakeOllama(t, `{"models":[{"name":"clef-flash:latest","model":"clef-flash:latest"},{"name":"qwen3:8b","model":"qwen3:8b"}]}`)
	for _, model := range []string{"clef-flash", "clef-flash:latest", "qwen3:8b"} {
		if err := New(withModel(server.URL, model)).Check(context.Background()); err != nil {
			t.Errorf("%s: %v", model, err)
		}
	}
	for _, model := range []string{"clef", "qwen3", "clef-flash:q4"} {
		err := New(withModel(server.URL, model)).Check(context.Background())
		if err == nil || !strings.Contains(err.Error(), "has no model named") {
			t.Errorf("%s: err = %v", model, err)
		}
	}
}

func TestCheckReportsWhyItFailed(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	if err := New(withModel(url, "clef")).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "cannot reach Ollama") {
		t.Errorf("closed: err = %v", err)
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>secret internal page</html>"))
	}))
	defer other.Close()
	err := New(withModel(other.URL, "clef")).Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "is not Ollama") || strings.Contains(err.Error(), "secret") {
		t.Errorf("not Ollama: err = %v", err)
	}

	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	if err := New(withModel(missing.URL, "clef")).Check(context.Background()); err == nil || !strings.Contains(err.Error(), "answered 404") {
		t.Errorf("404: err = %v", err)
	}
}
