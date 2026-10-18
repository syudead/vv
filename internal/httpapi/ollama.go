package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// OllamaConnection is separate from classifier-consuming feature interfaces (docs/design-docs/ai-layer.md).
type OllamaConnection interface {
	// Settings returns connection data and saved status from one snapshot.
	Settings(ctx context.Context) (settings domain.OllamaSettings, saved bool, err error)
	SaveSettings(ctx context.Context, settings domain.OllamaSettings) (domain.OllamaSettings, error)
	Check(ctx context.Context) error
}

func (s *server) GetOllamaSettings(w http.ResponseWriter, r *http.Request) {
	if s.ollama == nil {
		s.internalError(w, "Ollama settings are not configured.", nil)
		return
	}
	settings, saved, err := s.ollama.Settings(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the Ollama settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, ollamaSettingsBody(settings, saved), s.logger)
}

func (s *server) UpdateOllamaSettings(w http.ResponseWriter, r *http.Request) {
	if s.ollama == nil {
		s.internalError(w, "Ollama settings are not configured.", nil)
		return
	}
	var body struct {
		Endpoint *string `json:"endpoint"`
		Model    *string `json:"model"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Endpoint == nil || body.Model == nil {
		s.invalidRequest(w, "endpoint and model are required.")
		return
	}
	saved, err := s.ollama.SaveSettings(r.Context(), domain.OllamaSettings{
		Endpoint: *body.Endpoint, Model: *body.Model,
	})
	if errors.Is(err, domain.ErrInvalidOllamaSettings) {
		s.invalidRequest(w, err.Error())
		return
	}
	if err != nil {
		s.internalError(w, "Could not save the Ollama settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, ollamaSettingsBody(saved, true), s.logger)
}

func ollamaSettingsBody(settings domain.OllamaSettings, saved bool) gen.OllamaSettings {
	return gen.OllamaSettings{Endpoint: settings.Endpoint, Model: settings.Model, Saved: saved}
}

// Check saved settings only without classification; unavailable endpoints/models return 200 with available=false.
func (s *server) CheckOllama(w http.ResponseWriter, r *http.Request) {
	if s.ollama == nil {
		s.internalError(w, "Ollama settings are not configured.", nil)
		return
	}
	s.writeCheckResult(w, s.ollama.Check(r.Context()))
}

func (s *server) writeCheckResult(w http.ResponseWriter, err error) {
	result := gen.CheckResult{Available: err == nil}
	if err != nil {
		message := err.Error()
		result.Message = &message
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, result, s.logger)
}
