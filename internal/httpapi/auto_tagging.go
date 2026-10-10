package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// AutoTagging は自動タグ付けの設定・確かめ・判定の始め方である。internal/app の *AutoTagger が
// これを満たす（docs/design-docs/auto-tagging.md）。
type AutoTagging interface {
	Status(ctx context.Context) (domain.AutoTagStatus, error)
	SaveSettings(ctx context.Context, settings domain.AutoTagSettings) (domain.AutoTagStatus, error)
	Check(ctx context.Context) error
	QueueLibrary(ctx context.Context, scope domain.AutoTagScope) (int, error)
	QueueVideo(ctx context.Context, videoID int64) (bool, error)
}

// GetAutoTaggingSettings は自動タグ付けの設定と待ち行列の件数を返す。
func (s *server) GetAutoTaggingSettings(w http.ResponseWriter, r *http.Request) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	status, err := s.autoTagging.Status(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the auto-tagging settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIAutoTaggingSettings(status), s.logger)
}

// UpdateAutoTaggingSettings は設定を確かめて保存する。判定は始めない。
func (s *server) UpdateAutoTaggingSettings(w http.ResponseWriter, r *http.Request) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	// すべての項目が必須なので、無いことを零値と区別して読む。
	var body struct {
		Enabled   *bool    `json:"enabled"`
		Endpoint  *string  `json:"endpoint"`
		Model     *string  `json:"model"`
		Threshold *float64 `json:"threshold"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Enabled == nil || body.Endpoint == nil || body.Model == nil || body.Threshold == nil {
		s.invalidRequest(w, "enabled, endpoint, model and threshold are required.")
		return
	}
	status, err := s.autoTagging.SaveSettings(r.Context(), domain.AutoTagSettings{
		Enabled:   *body.Enabled,
		Endpoint:  *body.Endpoint,
		Model:     *body.Model,
		Threshold: *body.Threshold,
	})
	if errors.Is(err, domain.ErrInvalidAutoTagSettings) {
		s.invalidRequest(w, err.Error())
		return
	}
	if err != nil {
		s.internalError(w, "Could not save the auto-tagging settings.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIAutoTaggingSettings(status), s.logger)
}

// CheckAutoTagging は保存した問い合わせ先と模型で判定できるかを確かめる。使えないことは 200 の
// available=false で返す。問い合わせ先は要求から取らず、保存した設定だけを使う。
func (s *server) CheckAutoTagging(w http.ResponseWriter, r *http.Request) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	err := s.autoTagging.Check(r.Context())
	result := gen.AutoTaggingCheck{Available: err == nil}
	if err != nil {
		message := err.Error()
		result.Message = &message
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, result, s.logger)
}

// StartAutoTagging はライブラリの動画を判定に回す。
func (s *server) StartAutoTagging(w http.ResponseWriter, r *http.Request) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	var body struct {
		Scope *string `json:"scope"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Scope == nil || !domain.AutoTagScope(*body.Scope).Valid() {
		s.invalidRequest(w, "scope must be missing or all.")
		return
	}
	queued, err := s.autoTagging.QueueLibrary(r.Context(), domain.AutoTagScope(*body.Scope))
	if err != nil {
		s.internalError(w, "Could not start auto-tagging.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, gen.AutoTaggingQueued{Queued: queued}, s.logger)
}

// AutoTagVideo は動画 1 本を判定に回す。
func (s *server) AutoTagVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	if _, ok := s.lookupVideo(w, r, id); !ok {
		return
	}
	queued, err := s.autoTagging.QueueVideo(r.Context(), id)
	if err != nil {
		s.internalError(w, "Could not start auto-tagging.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, gen.AutoTagVideoResult{Queued: queued}, s.logger)
}

func toAPIAutoTaggingSettings(status domain.AutoTagStatus) gen.AutoTaggingSettings {
	queue := gen.AutoTaggingQueue{
		Queued:  status.Counts.Queued,
		Running: status.Counts.Running,
		Done:    status.Counts.Done,
		Failed:  status.Counts.Failed,
	}
	if status.Counts.LastError != "" {
		lastError := status.Counts.LastError
		queue.LastError = &lastError
	}
	return gen.AutoTaggingSettings{
		Enabled:   status.Settings.Enabled,
		Endpoint:  status.Settings.Endpoint,
		Model:     status.Settings.Model,
		Threshold: status.Settings.Threshold,
		Queue:     queue,
	}
}
