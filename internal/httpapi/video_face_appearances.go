package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (s *server) ListVideoFaceAppearances(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if !s.creditsConfigured(w) {
		return
	}
	found, err := s.credits.VideoFaceAppearances(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not load the video's face appearances.", err)
		return
	}
	body := gen.VideoFaceAppearanceList{
		Analysed: found.Analysed, Items: make([]gen.VideoFaceAppearance, 0, len(found.Items)),
	}
	for _, a := range found.Items {
		body.Items = append(body.Items, gen.VideoFaceAppearance{
			AppearanceId: a.ID, Person: toAPIPersonRef(a.Person), ImageRevision: optionalPositive(a.ImageRevision),
			HasCrop: a.HasCrop, Spans: toAPIFaceTimeSpans(a.Spans),
		})
	}
	s.writeCreditBody(w, http.StatusOK, body)
}
