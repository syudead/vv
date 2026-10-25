package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func generating(url string) fixedSettings {
	return fixedSettings{settings: domain.OllamaSettings{Endpoint: url, Model: "clef", GenerationModel: "qwen3:8b"}}
}

func TestGenerateSendsSchemaAsFormat(t *testing.T) {
	var got map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"model":"qwen3:8b","message":{"role":"assistant","content":" {\"kind\":\"filter\"} "},"done":true}`))
	}))
	defer server.Close()

	schema := json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}}}`)
	answer, err := NewGenerator(generating(server.URL)).Generate(context.Background(),
		domain.GenerationRequest{Prompt: "猫の動画", Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	if string(answer) != `{"kind":"filter"}` {
		t.Errorf("answer = %s", answer)
	}
	want := map[string]string{
		"model":      `"qwen3:8b"`,
		"messages":   `[{"role":"user","content":"猫の動画"}]`,
		"format":     string(schema),
		"stream":     `false`,
		"keep_alive": `"10m"`,
	}
	for key, value := range want {
		if string(got[key]) != value {
			t.Errorf("%s = %s, want %s", key, got[key], value)
		}
	}
}

func TestGenerateTellsFailuresApart(t *testing.T) {
	reply := func(status int, body string) string {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(server.Close)
		return server.URL
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	for _, tc := range []struct {
		name string
		url  string
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{"content is not JSON", reply(http.StatusOK, `{"message":{"role":"assistant","content":"はい、どうぞ"}}`), nil, domain.ErrGenerationUnreadable},
		{"error body", reply(http.StatusNotFound, `{"error":"model \"qwen3:8b\" not found, try pulling it first"}`), nil, domain.ErrOllamaModel},
		{"not Ollama", reply(http.StatusOK, `<html>secret</html>`), nil, domain.ErrNotOllama},
		{"unreachable", closed.URL, nil, domain.ErrOllamaUnreachable},
		{"timeout", reply(http.StatusOK, `{}`), func() (context.Context, context.CancelFunc) {
			return context.WithDeadline(context.Background(), time.Unix(0, 0))
		}, domain.ErrOllamaTimeout},
	} {
		ctx, cancel := context.Background(), func() {}
		if tc.ctx != nil {
			ctx, cancel = tc.ctx()
		}
		_, err := NewGenerator(generating(tc.url)).Generate(ctx, domain.GenerationRequest{Prompt: "?", Schema: json.RawMessage(`{}`)})
		cancel()
		for _, kind := range []error{domain.ErrGenerationUnreadable, domain.ErrOllamaModel, domain.ErrNotOllama,
			domain.ErrOllamaUnreachable, domain.ErrOllamaTimeout} {
			if want := errors.Is(tc.want, kind); errors.Is(err, kind) != want {
				t.Errorf("%s: err = %v, Is(%v) = %v", tc.name, err, kind, !want)
			}
		}
	}
}

func TestGenerateWithoutGenerationModelSendsNothing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	}))
	defer server.Close()
	_, err := NewGenerator(at(server.URL)).Generate(context.Background(), domain.GenerationRequest{Prompt: "?"})
	if !errors.Is(err, domain.ErrOllamaModel) {
		t.Fatalf("err = %v", err)
	}
}
