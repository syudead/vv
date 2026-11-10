package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (s *server) GetBackgroundWork(w http.ResponseWriter, r *http.Request) {
	if s.background == nil {
		s.internalError(w, "Background work is not configured.", nil)
		return
	}
	status, err := s.background.Status(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the background work status.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIBackgroundWork(status), s.logger)
}

func (s *server) SetBackgroundPause(w http.ResponseWriter, r *http.Request) {
	if s.background == nil {
		s.internalError(w, "Background work is not configured.", nil)
		return
	}
	// Distinguish a missing paused from false.
	var body struct {
		Paused *bool `json:"paused"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Paused == nil {
		s.invalidRequest(w, "paused must be true or false.")
		return
	}
	status, err := s.background.SetPaused(r.Context(), *body.Paused)
	if err != nil {
		s.internalError(w, "Could not change the background work pause.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIBackgroundWork(status), s.logger)
}

func toAPIBackgroundWork(status domain.BackgroundWork) gen.BackgroundWork {
	return gen.BackgroundWork{Paused: status.Paused}
}
