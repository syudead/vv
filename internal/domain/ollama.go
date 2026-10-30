package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Ollama connection settings belong to the provider adapter, independently of feature settings (docs/design-docs/ai-layer.md).

const (
	DefaultOllamaEndpoint = "http://127.0.0.1:11434"
	// DefaultOllamaModel favors the smaller, faster Clef variant.
	DefaultOllamaModel = "clef-flash"
)

const ollamaMaxModelLength = 200

var ErrInvalidOllamaSettings = errors.New("invalid Ollama settings")

// Ollama failures the adapter tells apart, so callers can name the cause without reading messages.
var (
	ErrOllamaUnreachable = errors.New("cannot reach Ollama")
	ErrNotOllama         = errors.New("the server is not Ollama")
	// ErrOllamaModel covers a missing model and a model that refused the request.
	ErrOllamaModel   = errors.New("the Ollama model is missing or refused the request")
	ErrOllamaTimeout = errors.New("the Ollama request timed out")
	// ErrGenerationUnreadable means the model's answer is not JSON; callers do not retry it.
	ErrGenerationUnreadable = errors.New("the model's answer is not JSON")
	// ErrGenerationModelMissing means the check found the decision model but not the generation model.
	ErrGenerationModelMissing = errors.New("the generation model is not at the Ollama endpoint")
	// ErrGenerationModelWithoutTools means the generation model is there but cannot call tools.
	ErrGenerationModelWithoutTools = errors.New("the generation model cannot call tools")
)

type OllamaSettings struct {
	Endpoint string
	// Model is the decision model the classifier uses.
	Model string
	// GenerationModel is the chat model the assistant uses; empty turns the assistant off.
	GenerationModel string
}

func DefaultOllamaSettings() OllamaSettings {
	return OllamaSettings{Endpoint: DefaultOllamaEndpoint, Model: DefaultOllamaModel}
}

func NormalizeOllamaSettings(s OllamaSettings) (OllamaSettings, error) {
	endpoint, ok := normalizeBaseURL(s.Endpoint)
	if !ok {
		return OllamaSettings{}, fmt.Errorf("%w: the endpoint must be an http or https URL", ErrInvalidOllamaSettings)
	}
	s.Endpoint = endpoint
	s.Model = strings.TrimSpace(s.Model)
	if s.Model == "" || !isModelName(s.Model) {
		return OllamaSettings{}, fmt.Errorf("%w: the model must be a model name", ErrInvalidOllamaSettings)
	}
	s.GenerationModel = strings.TrimSpace(s.GenerationModel)
	if s.GenerationModel != "" && !isModelName(s.GenerationModel) {
		return OllamaSettings{}, fmt.Errorf("%w: the generation model must be a model name", ErrInvalidOllamaSettings)
	}
	return s, nil
}

func isModelName(name string) bool {
	return len(name) <= ollamaMaxModelLength && !strings.ContainsFunc(name, isControlOrSpace)
}

// AssistantNotReady names why the assistant cannot take a request; empty means it can.
type AssistantNotReady string

const (
	AssistantReady                AssistantNotReady = ""
	AssistantOllamaNotSaved       AssistantNotReady = "ollama_not_saved"
	AssistantGenerationModelUnset AssistantNotReady = "generation_model_unset"
	// AssistantGenerationModelWithoutTools means Ollama reports no tools capability for the generation model.
	AssistantGenerationModelWithoutTools AssistantNotReady = "generation_model_without_tools"
)

// AssistantStatus is the readiness the panel reads; Vision tells whether the generation model sees images.
type AssistantStatus struct {
	NotReady AssistantNotReady
	Vision   bool
}

// AssistantStatusOf decides from the generation model's capabilities once the saved connection is ready.
func AssistantStatusOf(capabilities ModelCapabilities) AssistantStatus {
	if !capabilities.Tools {
		return AssistantStatus{NotReady: AssistantGenerationModelWithoutTools, Vision: capabilities.Vision}
	}
	return AssistantStatus{Vision: capabilities.Vision}
}

// AssistantReadiness decides from the saved connection alone, without asking Ollama.
func AssistantReadiness(settings OllamaSettings, saved bool) AssistantNotReady {
	switch {
	case !saved:
		return AssistantOllamaNotSaved
	case settings.GenerationModel == "":
		return AssistantGenerationModelUnset
	default:
		return AssistantReady
	}
}

// normalizeBaseURL rejects credentials, queries, and fragments, including trailing ? or #, because endpoint paths are appended.
func normalizeBaseURL(raw string) (string, bool) {
	normalized := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(normalized)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || strings.ContainsAny(normalized, "?#") {
		return "", false
	}
	return normalized, true
}

func isControlOrSpace(r rune) bool {
	return r <= ' ' || r == 0x7f
}
