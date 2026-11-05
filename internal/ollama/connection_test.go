package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func fakeOllama(t *testing.T, tags string) *httptest.Server {
	t.Helper()
	return fakeOllamaShowing(t, tags, nil)
}

// fakeOllamaShowing answers /api/show from shows by model name, and Ollama's not-found error for any other name.
func fakeOllamaShowing(t *testing.T, tags string, shows map[string]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.40.2"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(tags))
		case "/api/show":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			show, ok := shows[body.Model]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				show = `{"error":"model '` + body.Model + `' not found"}`
			}
			_, _ = w.Write([]byte(show))
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

func TestModelsListsNamesInOllamaOrderWithCapabilities(t *testing.T) {
	server := fakeOllamaShowing(t, `{"models":[{"name":"qwen3:8b","model":"qwen3:8b"},{"name":"","model":"gemma3:4b"},{"name":"clef-flash:latest"}]}`,
		map[string]string{
			"qwen3:8b":  `{"capabilities":["completion","tools","thinking"]}`,
			"gemma3:4b": `{"capabilities":["completion","vision"]}`,
		})
	models, err := New(withModel(server.URL, "clef")).Models(context.Background())
	want := []domain.OllamaModel{
		{Name: "qwen3:8b", Capabilities: domain.ModelCapabilities{Tools: true}},
		{Name: "gemma3:4b", Capabilities: domain.ModelCapabilities{Vision: true}},
		// /api/show refuses this one, so it is listed with neither capability.
		{Name: "clef-flash:latest"},
	}
	if err != nil || !slices.Equal(models, want) {
		t.Fatalf("models = %+v, %v", models, err)
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

func TestCapabilitiesReadsToolsAndVision(t *testing.T) {
	server := fakeOllamaShowing(t, `{"models":[]}`, map[string]string{
		"gemma3:27b": `{"capabilities":["completion","vision","tools"]}`,
		// An Ollama too old to report capabilities.
		"llama3:8b": `{"modelfile":"FROM llama3"}`,
	})
	classifier := New(at(server.URL))
	for model, want := range map[string]domain.ModelCapabilities{
		"gemma3:27b": {Tools: true, Vision: true},
		"llama3:8b":  {},
	} {
		if got, err := classifier.Capabilities(context.Background(), server.URL, model); err != nil || got != want {
			t.Errorf("%s = %+v, %v; want %+v", model, got, err, want)
		}
	}
	if _, err := classifier.Capabilities(context.Background(), server.URL, "missing"); !errors.Is(err, domain.ErrOllamaModel) {
		t.Errorf("missing: err = %v", err)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>secret</html>"))
	}))
	defer other.Close()
	if _, err := classifier.Capabilities(context.Background(), other.URL, "gemma3:27b"); !errors.Is(err, domain.ErrNotOllama) {
		t.Errorf("not Ollama: err = %v", err)
	}
}
