package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type ComfyUIConnection interface {
	Settings(ctx context.Context) (domain.ComfyUISettings, error)
	SaveSettings(ctx context.Context, update domain.ComfyUISettingsUpdate) (domain.ComfyUISettings, error)
	Check(ctx context.Context) error
}

// Expose only API-key presence, never its value.
func (s *server) GetComfyUISettings(w http.ResponseWriter, r *http.Request) {
	if s.comfyUI == nil {
		s.internalError(w, "ComfyUI settings are not configured.", nil)
		return
	}
	settings, err := s.comfyUI.Settings(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the ComfyUI settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, comfyUISettingsBody(settings), s.logger)
}

// Omitted apiKey/model preserves saved values; an empty apiKey deletes it.
func (s *server) UpdateComfyUISettings(w http.ResponseWriter, r *http.Request) {
	if s.comfyUI == nil {
		s.internalError(w, "ComfyUI settings are not configured.", nil)
		return
	}
	var body struct {
		Provider *string `json:"provider"`
		URL      *string `json:"url"`
		APIKey   *string `json:"apiKey"`
		Model    *string `json:"model"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Provider == nil {
		s.invalidRequest(w, "provider is required.")
		return
	}
	update := domain.ComfyUISettingsUpdate{Provider: domain.ComfyUIProvider(*body.Provider), APIKey: body.APIKey}
	if body.URL != nil {
		update.URL = *body.URL
	}
	if body.Model != nil {
		model := domain.ComfyUIModel(*body.Model)
		update.Model = &model
	}
	saved, err := s.comfyUI.SaveSettings(r.Context(), update)
	switch {
	case errors.Is(err, domain.ErrComfyUIURLInvalid):
		s.invalidRequestReason(w, reasonComfyUIURLInvalid, "A self-hosted ComfyUI needs an http or https URL.")
		return
	case errors.Is(err, domain.ErrComfyUIAPIKeyMissing):
		s.invalidRequestReason(w, reasonComfyUIAPIKeyMissing, "Comfy Cloud needs an API key.")
		return
	case errors.Is(err, domain.ErrInvalidComfyUISettings):
		s.invalidRequest(w, err.Error())
		return
	case err != nil:
		s.internalError(w, "Could not save the ComfyUI settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, comfyUISettingsBody(saved), s.logger)
}

// Checks use saved settings only and report unavailability as 200/available=false with missing resources.
func (s *server) CheckComfyUI(w http.ResponseWriter, r *http.Request) {
	if s.comfyUI == nil {
		s.internalError(w, "ComfyUI settings are not configured.", nil)
		return
	}
	err := s.comfyUI.Check(r.Context())
	var missing *domain.ComfyUIMissingError
	if !errors.As(err, &missing) {
		s.writeCheckResult(w, err)
		return
	}
	message := err.Error()
	items := make([]gen.ComfyUIMissingItem, 0, len(missing.Items))
	for _, item := range missing.Items {
		body := gen.ComfyUIMissingItem{Kind: gen.ComfyUIMissingItemKind(item.Kind), Name: item.Name}
		if item.Folder != "" {
			folder := item.Folder
			body.Folder = &folder
		}
		if item.Source != "" {
			source := item.Source
			body.Source = &source
		}
		items = append(items, body)
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.CheckResult{Available: false, Message: &message, Missing: &items}, s.logger)
}

// Readiness requires a saved connection, not a successful probe (docs/design-docs/thumbnail-editing.md).
func (s *server) GetThumbnailEditReadiness(w http.ResponseWriter, r *http.Request) {
	if s.comfyUI == nil {
		s.internalError(w, "ComfyUI settings are not configured.", nil)
		return
	}
	settings, err := s.comfyUI.Settings(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the ComfyUI settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.ThumbnailEditReadiness{Connection: settings.Saved()}, s.logger)
}

func comfyUISettingsBody(settings domain.ComfyUISettings) gen.ComfyUISettings {
	body := gen.ComfyUISettings{ApiKeySet: settings.APIKey != "", Model: gen.ComfyUIModel(settings.EffectiveModel())}
	if settings.Saved() {
		provider := gen.ComfyUIProvider(settings.Provider)
		body.Provider = &provider
	}
	if settings.URL != "" {
		url := settings.URL
		body.Url = &url
	}
	return body
}
