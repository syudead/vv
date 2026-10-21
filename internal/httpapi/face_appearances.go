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
	sort := domain.PersonAppearancesNewest
	if params.Sort != nil {
		sort = domain.PersonAppearanceSort(*params.Sort)
		if sort != domain.PersonAppearancesNewest && sort != domain.PersonAppearancesLeastSimilar {
			s.invalidRequest(w, "sort must be newest or leastSimilar.")
			return
		}
	}
	page, err := s.credits.PersonAppearances(r.Context(), id,
		domain.PersonAppearanceQuery{Sort: sort, Cursor: query.Cursor, Limit: query.Limit})
	if errors.Is(err, domain.ErrInvalidCursor) {
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	}
	if err != nil {
		s.writeCreditError(w, peopleAPI, err)
		return
	}
	body := gen.FaceAppearanceList{Items: make([]gen.FaceAppearance, 0, len(page.Items)), Total: page.Total}
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
			Similarity: a.Similarity,
		})
		if a.Unlike {
			body.Items[len(body.Items)-1].Unlike = &a.Unlike
		}
	}
	s.writeCreditBody(w, http.StatusOK, body)
}

func (s *server) SplitFaceAppearance(w http.ResponseWriter, r *http.Request, id gen.PersonId, appearanceID gen.FaceAppearanceId) {
	s.splitFaceAppearances(w, r, id, []int64{appearanceID})
}

func (s *server) SplitFaceAppearances(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	var body gen.FaceAppearanceSplitRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if n := len(body.AppearanceIds); n == 0 || n > 200 {
		s.invalidRequest(w, "appearanceIds must list 1 to 200 appearances.")
		return
	}
	seen := map[int64]bool{}
	for _, appearanceID := range body.AppearanceIds {
		if appearanceID < 1 || seen[appearanceID] {
			s.invalidRequest(w, "appearanceIds must be distinct positive integers.")
			return
		}
		seen[appearanceID] = true
	}
	s.splitFaceAppearances(w, r, id, body.AppearanceIds)
}

func (s *server) splitFaceAppearances(w http.ResponseWriter, r *http.Request, id int64, appearanceIDs []int64) {
	if !s.creditsConfigured(w) {
		return
	}
	person, err := s.credits.SplitFaceAppearances(r.Context(), id, appearanceIDs)
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
