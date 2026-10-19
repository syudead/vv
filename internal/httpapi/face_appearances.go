package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A crop is never rewritten and a re-analysis gives a new appearance a new id, so a day's private caching is safe.
const cacheFaceCrop = "private, max-age=86400"

// ListPersonAppearances pages with the admin lists' limit and cursor rules.
func (s *server) ListPersonAppearances(w http.ResponseWriter, r *http.Request, id gen.PersonId, params gen.ListPersonAppearancesParams) {
	query, problem := buildTagListQuery(nil, nil, nil, nil, params.Cursor, params.Limit)
	if !s.acceptTagListQuery(w, problem) || !s.creditsConfigured(w) {
		return
	}
	page, err := s.credits.PersonAppearances(r.Context(), id, query.Cursor, query.Limit)
	if errors.Is(err, domain.ErrInvalidCursor) {
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	}
	if err != nil {
		s.writeCreditError(w, peopleAPI, err)
		return
	}
	body := gen.FaceAppearanceList{Items: make([]gen.FaceAppearance, 0, len(page.Items))}
	if page.NextCursor != "" {
		body.NextCursor = &page.NextCursor
	}
	for _, a := range page.Items {
		spans := make([]gen.FaceAppearanceSpan, 0, len(a.Spans))
		for _, span := range a.Spans {
			spans = append(spans, gen.FaceAppearanceSpan{Start: span.Start, End: span.End})
		}
		body.Items = append(body.Items, gen.FaceAppearance{
			AppearanceId: a.ID, VideoId: optionalPositive(a.VideoID), Title: a.Title, Spans: spans, HasCrop: a.HasCrop,
		})
	}
	s.writeCreditBody(w, http.StatusOK, body)
}

func (s *server) SplitFaceAppearance(w http.ResponseWriter, r *http.Request, id gen.PersonId, appearanceID gen.FaceAppearanceId) {
	if !s.creditsConfigured(w) {
		return
	}
	person, err := s.credits.SplitFaceAppearance(r.Context(), id, appearanceID)
	if errors.Is(err, domain.ErrFaceAppearanceNotFound) {
		s.notFoundReason(w, gen.ErrorReasonFaceAppearanceNotFound, "The person has no such face appearance.")
		return
	}
	if err != nil {
		s.writeCreditError(w, peopleAPI, err)
		return
	}
	s.writeCreditBody(w, http.StatusCreated, toAPIPerson(person))
}

func (s *server) GetFaceAppearanceCrop(w http.ResponseWriter, r *http.Request, id int64) {
	if !s.creditsConfigured(w) {
		return
	}
	crop, err := s.credits.FaceAppearanceCrop(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, "The face appearance was not found.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not load the face crop.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheFaceCrop)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(crop)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(crop); err != nil {
		s.logger.Debug("could not write the face crop", slog.Int64("appearance", id), slog.Any("error", err))
	}
}
