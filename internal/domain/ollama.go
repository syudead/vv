package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Ollama connection settings belong to the provider adapter, independently of feature settings; the adapter owns
// their default values (docs/design-docs/ai-layer.md).

const ollamaMaxModelLength = 200

var ErrInvalidOllamaSettings = errors.New("invalid Ollama settings")

// Model provider failures an adapter tells apart, so callers can name the cause without reading messages.
var (
	ErrProviderUnreachable = errors.New("cannot reach the model provider")
	// ErrUnexpectedProvider means a server answered, but not as the configured provider does.
	ErrUnexpectedProvider = errors.New("the server is not the configured model provider")
	// ErrModelUnavailable covers a missing model and a model that refused the request.
	ErrModelUnavailable = errors.New("the model is missing or refused the request")
	ErrProviderTimeout  = errors.New("the model provider did not answer in time")
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

// WithDefaults fills an absent endpoint or decision model from defaults, which the provider adapter supplies.
func (s OllamaSettings) WithDefaults(defaults OllamaSettings) OllamaSettings {
	if s.Endpoint == "" {
		s.Endpoint = defaults.Endpoint
	}
	if s.Model == "" {
		s.Model = defaults.Model
	}
	return s
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
