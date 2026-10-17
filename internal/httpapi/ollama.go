package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// OllamaConnection は判定モデルの提供元 Ollama への接続先の設定と確かめである。
// internal/app の *OllamaConnection が満たす（docs/design-docs/ai-layer.md）。判定モデルを
// 使う機能（AutoTagging）とは別の口にする。
type OllamaConnection interface {
	// Settings は接続先と、所有者が接続先を保存したかを同じ時点の値として返す
	//
	Settings(ctx context.Context) (settings domain.OllamaSettings, saved bool, err error)
	SaveSettings(ctx context.Context, settings domain.OllamaSettings) (domain.OllamaSettings, error)
	Check(ctx context.Context) error
}

// GetOllamaSettings は Ollama の接続先を返す。
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

// UpdateOllamaSettings は Ollama の接続先を確かめて保存する。
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

// ollamaSettingsBody は接続先と保存したかを応答の本文にする。
func ollamaSettingsBody(settings domain.OllamaSettings, saved bool) gen.OllamaSettings {
	return gen.OllamaSettings{Endpoint: settings.Endpoint, Model: settings.Model, Saved: saved}
}

// CheckOllama は保存した接続先に Ollama が動いていて、保存した模型があるかを確かめる。判定は
// させない。使えないことは 200 の available=false で返す。接続先は要求から取らず、保存した
// 設定だけを使う。
func (s *server) CheckOllama(w http.ResponseWriter, r *http.Request) {
	if s.ollama == nil {
		s.internalError(w, "Ollama settings are not configured.", nil)
		return
	}
	s.writeCheckResult(w, s.ollama.Check(r.Context()))
}

// writeCheckResult は確かめの結果を 200 で返す。失敗は available=false と理由にする。
func (s *server) writeCheckResult(w http.ResponseWriter, err error) {
	result := gen.CheckResult{Available: err == nil}
	if err != nil {
		message := err.Error()
		result.Message = &message
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, result, s.logger)
}
