package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// ComfyUIConnection は画像編集の提供元 ComfyUI への接続先の設定と確かめである。
// internal/app の *ComfyUIConnection が満たす（docs/design-docs/ai-layer.md）。
type ComfyUIConnection interface {
	Settings(ctx context.Context) (domain.ComfyUISettings, error)
	SaveSettings(ctx context.Context, update domain.ComfyUISettingsUpdate) (domain.ComfyUISettings, error)
	Check(ctx context.Context) error
}

// GetComfyUISettings は ComfyUI の接続先を返す。API キーは返さず、保存されているかだけを
// 返す（specs/045-comfyui-thumbnails/research.md R-9）。
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

// UpdateComfyUISettings は ComfyUI の接続先を確かめて保存する。apiKey が無ければ保存済みの
// キーを残し、空文字列なら消す。
func (s *server) UpdateComfyUISettings(w http.ResponseWriter, r *http.Request) {
	if s.comfyUI == nil {
		s.internalError(w, "ComfyUI settings are not configured.", nil)
		return
	}
	var body struct {
		Provider *string `json:"provider"`
		URL      *string `json:"url"`
		APIKey   *string `json:"apiKey"`
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

// CheckComfyUI は保存した接続先に ComfyUI が応えるかを確かめる。使えないことは 200 の
// available=false で返す。接続先は要求から取らず、保存した設定だけを使う。
func (s *server) CheckComfyUI(w http.ResponseWriter, r *http.Request) {
	if s.comfyUI == nil {
		s.internalError(w, "ComfyUI settings are not configured.", nil)
		return
	}
	s.writeCheckResult(w, s.comfyUI.Check(r.Context()))
}

// comfyUISettingsBody は接続先を応答の形にする。API キーは入れない。
func comfyUISettingsBody(settings domain.ComfyUISettings) gen.ComfyUISettings {
	body := gen.ComfyUISettings{ApiKeySet: settings.APIKey != ""}
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
