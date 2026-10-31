package httpapi

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestOllamaGenerationModelSaveKeepAndClear(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})
	put := func(body string) gen.OllamaSettings {
		t.Helper()
		rec := f.env.serve(authRequest{method: http.MethodPut, target: "/api/settings/ollama", body: body, cookies: []*http.Cookie{f.owner}})
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s = %d %s", body, rec.Code, rec.Body)
		}
		return decode[gen.OllamaSettings](t, rec)
	}
	if got := put(`{"endpoint":"http://gpu:11434","model":"clef","generationModel":" qwen3:8b "}`); got.GenerationModel != "qwen3:8b" {
		t.Fatalf("saved = %+v", got)
	}
	// A caller that omits the field keeps the saved generation model.
	if got := put(`{"endpoint":"http://gpu:11434","model":"clef-flash"}`); got.GenerationModel != "qwen3:8b" || got.Model != "clef-flash" {
		t.Fatalf("omitted = %+v", got)
	}
	if got := put(`{"endpoint":"http://gpu:11434","model":"clef","generationModel":""}`); got.GenerationModel != "" || !got.Saved {
		t.Fatalf("cleared = %+v", got)
	}
	if res := f.send(http.MethodPut, "/api/settings/ollama", `{"endpoint":"http://gpu:11434","model":"clef","generationModel":"qwen 3"}`); res.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid generation model: status = %d", res.StatusCode)
	}
}

func TestCheckOllamaNamesMissingGenerationModel(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{err: fmt.Errorf("no qwen3: %w", domain.ErrGenerationModelMissing)})
	rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/ollama/check", cookies: []*http.Cookie{f.owner}})
	got := decode[gen.CheckResult](t, rec)
	if rec.Code != http.StatusOK || got.Available || got.Reason == nil || *got.Reason != gen.CheckResultReasonGenerationModelMissing {
		t.Fatalf("check = %d %+v", rec.Code, got)
	}
}

func TestCheckOllamaNamesGenerationModelWithoutTools(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{caps: domain.ModelCapabilities{Vision: true}})
	f.send(http.MethodPut, "/api/settings/ollama", `{"endpoint":"http://gpu:11434","model":"clef","generationModel":"llava:7b"}`)
	rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/ollama/check", cookies: []*http.Cookie{f.owner}})
	got := decode[gen.CheckResult](t, rec)
	if rec.Code != http.StatusOK || got.Available || got.Reason == nil || *got.Reason != gen.CheckResultReasonGenerationModelWithoutTools {
		t.Fatalf("check = %d %+v", rec.Code, got)
	}
}

func TestListOllamaModels(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{models: []domain.OllamaModel{
		{Name: "qwen3:8b", Capabilities: domain.ModelCapabilities{Tools: true}},
		{Name: "clef-flash:latest", Capabilities: domain.ModelCapabilities{Vision: true}},
	}})
	rec := f.env.get("/api/settings/ollama/models", f.owner)
	got := decode[gen.OllamaModelList](t, rec)
	want := []gen.OllamaModel{{Name: "qwen3:8b", Tools: true}, {Name: "clef-flash:latest", Vision: true}}
	if rec.Code != http.StatusOK || !slices.Equal(got.Items, want) {
		t.Fatalf("models = %d %+v", rec.Code, got)
	}

	for kind, want := range map[error]gen.ErrorReason{
		domain.ErrOllamaUnreachable: gen.ErrorReasonConnection,
		domain.ErrNotOllama:         gen.ErrorReasonNotOllama,
	} {
		down := newAutoTaggingFixture(t, ollamaProbe{err: fmt.Errorf("at http://h: %w", kind)})
		rec := down.env.get("/api/settings/ollama/models", down.owner)
		body := decode[gen.Error](t, rec)
		if rec.Code != http.StatusConflict || body.Code != gen.ErrorCodeOllamaUnreachable || body.Reason == nil || *body.Reason != want {
			t.Errorf("%v: %d %+v", kind, rec.Code, body)
		}
	}
}

func TestAssistantReadinessNamesModelWithoutTools(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{caps: domain.ModelCapabilities{Vision: true}})
	f.send(http.MethodPut, "/api/settings/ollama", `{"endpoint":"http://gpu:11434","model":"clef","generationModel":"llava:7b"}`)
	got := decode[gen.AssistantReadiness](t, f.env.get("/api/assistant/readiness", f.owner))
	if got.Ready || got.Reason == nil || *got.Reason != gen.AssistantReadinessReasonGenerationModelWithoutTools || !got.Vision {
		t.Fatalf("readiness = %+v", got)
	}
}

func TestAssistantReadiness(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{caps: domain.ModelCapabilities{Tools: true}})
	readiness := func() gen.AssistantReadiness {
		t.Helper()
		rec := f.env.get("/api/assistant/readiness", f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d %s", rec.Code, rec.Body)
		}
		return decode[gen.AssistantReadiness](t, rec)
	}
	if got := readiness(); got.Ready || got.Reason == nil || *got.Reason != gen.AssistantReadinessReasonOllamaNotSaved {
		t.Fatalf("unsaved = %+v", got)
	}
	f.send(http.MethodPut, "/api/settings/ollama", `{"endpoint":"http://gpu:11434","model":"clef","generationModel":""}`)
	if got := readiness(); got.Ready || got.Reason == nil || *got.Reason != gen.AssistantReadinessReasonGenerationModelUnset {
		t.Fatalf("no generation model = %+v", got)
	}
	f.send(http.MethodPut, "/api/settings/ollama", `{"endpoint":"http://gpu:11434","model":"clef","generationModel":"qwen3:8b"}`)
	if got := readiness(); !got.Ready || got.Reason != nil {
		t.Fatalf("ready = %+v", got)
	}
}
