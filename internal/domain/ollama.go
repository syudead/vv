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

type OllamaSettings struct {
	Endpoint string
	Model    string
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
	if s.Model == "" || len(s.Model) > ollamaMaxModelLength || strings.ContainsFunc(s.Model, isControlOrSpace) {
		return OllamaSettings{}, fmt.Errorf("%w: the model must be a model name", ErrInvalidOllamaSettings)
	}
	return s, nil
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
