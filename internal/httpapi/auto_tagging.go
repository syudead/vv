package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Classifier connection settings belong to OllamaConnection, not AutoTagging (docs/design-docs/auto-tagging.md).
type AutoTagging interface {
	Status(ctx context.Context) (domain.AutoTagStatus, error)
	SaveSettings(ctx context.Context, settings domain.AutoTagSettings) (domain.AutoTagStatus, error)
	CheckClassifier(ctx context.Context) error
	QueueLibrary(ctx context.Context, scope domain.AutoTagScope) (int, error)
	QueueVideo(ctx context.Context, videoID int64) (bool, error)
	ConfirmTag(ctx context.Context, videoID, tagID int64) error
	RejectTag(ctx context.Context, videoID, tagID int64) error
}

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

func (s *server) UpdateAutoTaggingSettings(w http.ResponseWriter, r *http.Request) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	// Distinguish missing required fields from zero values.
	var body struct {
		Enabled            *bool    `json:"enabled"`
		AfterImport        *bool    `json:"afterImport"`
		Threshold          *float64 `json:"threshold"`
		TentativeThreshold *float64 `json:"tentativeThreshold"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Enabled == nil || body.AfterImport == nil || body.Threshold == nil || body.TentativeThreshold == nil {
		s.invalidRequest(w, "enabled, afterImport, threshold and tentativeThreshold are required.")
		return
	}
	status, err := s.autoTagging.SaveSettings(r.Context(), domain.AutoTagSettings{
		Enabled:            *body.Enabled,
		AfterImport:        *body.AfterImport,
		Threshold:          *body.Threshold,
		TentativeThreshold: *body.TentativeThreshold,
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

// Classifier checks run even when tagging is disabled; unavailable results return 200 with available=false.
func (s *server) CheckAutoTaggingClassifier(w http.ResponseWriter, r *http.Request) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	s.writeCheckResult(w, s.autoTagging.CheckClassifier(r.Context()))
}

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
	if errors.Is(err, domain.ErrAutoTaggingOff) {
		s.conflictReason(w, gen.ErrorReasonAutoTaggingOff, "Auto-tagging is turned off.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not start auto-tagging.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, gen.AutoTaggingQueued{Queued: queued}, s.logger)
}

func (s *server) AutoTagVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	if _, ok := s.lookupVideo(w, r, id); !ok {
		return
	}
	queued, err := s.autoTagging.QueueVideo(r.Context(), id)
	if errors.Is(err, domain.ErrAutoTaggingOff) {
		s.conflictReason(w, gen.ErrorReasonAutoTaggingOff, "Auto-tagging is turned off.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not start auto-tagging.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, gen.AutoTagVideoResult{Queued: queued}, s.logger)
}

func (s *server) ConfirmVideoTag(w http.ResponseWriter, r *http.Request, id gen.VideoId, tagID gen.VideoTagId) {
	s.settleVideoTag(w, r, id, tagID, false)
}

// Rejected assignments are not automatically reapplied to the video.
func (s *server) RejectVideoTag(w http.ResponseWriter, r *http.Request, id gen.VideoId, tagID gen.VideoTagId) {
	s.settleVideoTag(w, r, id, tagID, true)
}

func (s *server) settleVideoTag(w http.ResponseWriter, r *http.Request, id, tagID int64, reject bool) {
	if s.autoTagging == nil {
		s.internalError(w, "Auto-tagging is not configured.", nil)
		return
	}
	if _, ok := s.lookupVideo(w, r, id); !ok {
		return
	}
	settle := s.autoTagging.ConfirmTag
	if reject {
		settle = s.autoTagging.RejectTag
	}
	err := settle(r.Context(), id, tagID)
	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, "This tag is not tentatively attached to the video.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not update the tag.", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		Enabled:            status.Settings.Enabled,
		AfterImport:        status.Settings.AfterImport,
		Threshold:          status.Settings.Threshold,
		TentativeThreshold: status.Settings.TentativeThreshold,
		Queue:              queue,
	}
}
