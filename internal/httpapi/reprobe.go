package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Storage resets state and queues reprobe atomically; repeat/nonfailed requests return probe_not_failed so the UI can treat them as already retrying.
func (s *server) ReprobeVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.catalog == nil {
		s.internalError(w, "Probe retry is not configured.", nil)
		return
	}

	err := s.catalog.RetryProbe(r.Context(), video)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
		return
	case errors.Is(err, domain.ErrProbeNotFailed):
		s.writeError(w, http.StatusConflict, codeProbeNotFailed, "This video is being probed or has already been probed.")
		return
	case err != nil:
		s.internalError(w, "Could not retry the probe.", err)
		return
	}

	updated, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	progress := s.progressFor(r.Context(), []domain.Video{updated})
	tags := s.tagsFor(r.Context(), []domain.Video{updated})
	payload := withTags(withProgress(s.apiVideo(r.Context(), updated), progress, updated.UserKey), tags, updated.UserKey)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, payload, s.logger)
}
