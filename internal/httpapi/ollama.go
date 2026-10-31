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
	// Models lists the models at the saved endpoint; errors wrap domain.ErrOllamaUnreachable or domain.ErrNotOllama.
	Models(ctx context.Context) ([]domain.OllamaModel, error)
	AssistantReadiness(ctx context.Context) (domain.AssistantStatus, error)
}

func (s *server) ollamaConfigured(w http.ResponseWriter) bool {
	if s.ollama == nil {
		s.internalError(w, "Ollama settings are not configured.", nil)
		return false
	}
	return true
}

func (s *server) GetOllamaSettings(w http.ResponseWriter, r *http.Request) {
	if !s.ollamaConfigured(w) {
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
	if !s.ollamaConfigured(w) {
		return
	}
	var body struct {
		Endpoint        *string `json:"endpoint"`
		Model           *string `json:"model"`
		GenerationModel *string `json:"generationModel"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Endpoint == nil || body.Model == nil {
		s.invalidRequest(w, "endpoint and model are required.")
		return
	}
	settings := domain.OllamaSettings{Endpoint: *body.Endpoint, Model: *body.Model}
	if body.GenerationModel != nil {
		settings.GenerationModel = *body.GenerationModel
	} else {
		// An omitted generation model keeps the saved one, so older callers do not clear it.
		current, _, err := s.ollama.Settings(r.Context())
		if err != nil {
			s.internalError(w, "Could not read the Ollama settings.", err)
			return
		}
		settings.GenerationModel = current.GenerationModel
	}
	saved, err := s.ollama.SaveSettings(r.Context(), settings)
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
	return gen.OllamaSettings{
		Endpoint: settings.Endpoint, Model: settings.Model, GenerationModel: settings.GenerationModel, Saved: saved,
	}
}

// Check saved settings only without classification; unavailable endpoints/models return 200 with available=false.
func (s *server) CheckOllama(w http.ResponseWriter, r *http.Request) {
	if !s.ollamaConfigured(w) {
		return
	}
	err := s.ollama.Check(r.Context())
	result := checkResult(err)
	var reason gen.CheckResultReason
	switch {
	case errors.Is(err, domain.ErrGenerationModelMissing):
		reason = gen.CheckResultReasonGenerationModelMissing
	case errors.Is(err, domain.ErrGenerationModelWithoutTools):
		reason = gen.CheckResultReasonGenerationModelWithoutTools
	}
	if reason != "" {
		result.Reason = &reason
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, result, s.logger)
}

func (s *server) ListOllamaModels(w http.ResponseWriter, r *http.Request) {
	if !s.ollamaConfigured(w) {
		return
	}
	models, err := s.ollama.Models(r.Context())
	switch {
	case errors.Is(err, domain.ErrOllamaUnreachable), errors.Is(err, domain.ErrOllamaTimeout):
		s.writeReasonError(w, http.StatusConflict, codeOllamaUnreachable, gen.ErrorReasonConnection, err.Error())
		return
	case errors.Is(err, domain.ErrNotOllama):
		s.writeReasonError(w, http.StatusConflict, codeOllamaUnreachable, gen.ErrorReasonNotOllama, err.Error())
		return
	case err != nil:
		s.internalError(w, "Could not list the Ollama models.", err)
		return
	}
	items := make([]gen.OllamaModel, len(models))
	for i, m := range models {
		items[i] = gen.OllamaModel{Name: m.Name, Tools: m.Capabilities.Tools, Vision: m.Capabilities.Vision}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.OllamaModelList{Items: items}, s.logger)
}

// GetAssistantReadiness asks Ollama only on the first read for a model, so opening the panel stays quick.
func (s *server) GetAssistantReadiness(w http.ResponseWriter, r *http.Request) {
	if !s.ollamaConfigured(w) {
		return
	}
	status, err := s.ollama.AssistantReadiness(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the assistant readiness.", err)
		return
	}
	body := gen.AssistantReadiness{Ready: status.NotReady == domain.AssistantReady, Vision: status.Vision}
	if !body.Ready {
		reason := gen.AssistantReadinessReason(status.NotReady)
		body.Reason = &reason
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, s.logger)
}

func (s *server) writeCheckResult(w http.ResponseWriter, err error) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, checkResult(err), s.logger)
}

func checkResult(err error) gen.CheckResult {
	result := gen.CheckResult{Available: err == nil}
	if err != nil {
		message := err.Error()
		result.Message = &message
	}
	return result
}
