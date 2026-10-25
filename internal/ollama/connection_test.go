package ollama

import (
	"context"
	"errors"
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

func TestCheckNamesMissingGenerationModel(t *testing.T) {
	server := fakeOllama(t, `{"models":[{"name":"clef-flash:latest","model":"clef-flash:latest"},{"name":"qwen3:8b","model":"qwen3:8b"}]}`)
	check := func(decision, generation string) error {
		return New(fixedSettings{settings: domain.OllamaSettings{Endpoint: server.URL, Model: decision, GenerationModel: generation}}).
			Check(context.Background())
	}
	if err := check("clef-flash", "qwen3:8b"); err != nil {
		t.Errorf("both listed: %v", err)
	}
	if err := check("clef-flash", "gemma3"); !errors.Is(err, domain.ErrGenerationModelMissing) || !strings.Contains(err.Error(), `"gemma3"`) {
		t.Errorf("generation missing: err = %v", err)
	}
	// The decision model is reported first, since auto-tagging depends on it.
	if err := check("clef", "gemma3"); !errors.Is(err, domain.ErrOllamaModel) || errors.Is(err, domain.ErrGenerationModelMissing) {
		t.Errorf("both missing: err = %v", err)
	}
}

func TestModelsListsNamesInOllamaOrder(t *testing.T) {
	server := fakeOllama(t, `{"models":[{"name":"qwen3:8b","model":"qwen3:8b"},{"name":"","model":"gemma3:4b"},{"name":"clef-flash:latest"}]}`)
	names, err := New(withModel(server.URL, "clef")).Models(context.Background())
	if err != nil || strings.Join(names, ",") != "qwen3:8b,gemma3:4b,clef-flash:latest" {
		t.Fatalf("names = %v, %v", names, err)
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	if _, err := New(withModel(closed.URL, "clef")).Models(context.Background()); !errors.Is(err, domain.ErrOllamaUnreachable) {
		t.Errorf("closed: err = %v", err)
	}
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	if _, err := New(withModel(other.URL, "clef")).Models(context.Background()); !errors.Is(err, domain.ErrNotOllama) {
		t.Errorf("not Ollama: err = %v", err)
	}
}
