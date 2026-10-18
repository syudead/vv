package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Candidate routes are owner-only and excluded from accessRoutes. Fingerprint updates use scan events; bundling clears matching candidates in the store.

// Return newest pairs first, with video IDs ascending within each pair.
func (s *server) ListVersionCandidates(w http.ResponseWriter, r *http.Request) {
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	page, err := s.versions.Candidates(r.Context())
	if err != nil {
		s.internalError(w, "Could not load the possible duplicates.", err)
		return
	}

	videos := make([]domain.Video, 0, len(page.Items)*2)
	for _, item := range page.Items {
		videos = append(videos, item.Videos[0], item.Videos[1])
	}
	items := s.versionItems(r, videos)
	payload := gen.VersionCandidatePage{Items: make([]gen.VersionCandidate, 0, len(page.Items)), Total: page.Total}
	for i, item := range page.Items {
		payload.Items = append(payload.Items, gen.VersionCandidate{
			Videos:   []gen.Video{items[2*i], items[2*i+1]},
			Distance: item.Distance,
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

func (s *server) DismissVersionCandidate(w http.ResponseWriter, r *http.Request) {
	var body gen.VersionCandidateDismissRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.VideoIds) != 2 || body.VideoIds[0] == body.VideoIds[1] {
		s.invalidRequest(w, "videoIds must be two different video ids.")
		return
	}
	if s.versions == nil {
		s.internalError(w, "Version storage is not configured.", nil)
		return
	}
	err := s.versions.Dismiss(r.Context(), [2]int64{body.VideoIds[0], body.VideoIds[1]})
	if !s.checkVersionsError(w, err, "Could not record the videos as different.") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
