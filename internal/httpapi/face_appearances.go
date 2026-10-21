package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
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
	if !s.readJSONBody(w, r, &body) || !s.acceptAppearanceIDs(w, body.AppearanceIds) {
		return
	}
	s.splitFaceAppearances(w, r, id, body.AppearanceIds)
}

// acceptAppearanceIDs answers 400 unless the list holds 1 to 200 distinct positive ids.
func (s *server) acceptAppearanceIDs(w http.ResponseWriter, ids []int64) bool {
	if n := len(ids); n == 0 || n > 200 {
		s.invalidRequest(w, "appearanceIds must list 1 to 200 appearances.")
		return false
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id < 1 || seen[id] {
			s.invalidRequest(w, "appearanceIds must be distinct positive integers.")
			return false
		}
		seen[id] = true
	}
	return true
}

func (s *server) MoveFaceAppearances(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	var body gen.FaceAppearanceMoveRequest
	if !s.readJSONBody(w, r, &body) || !s.acceptAppearanceIDs(w, body.AppearanceIds) || !s.creditsConfigured(w) {
		return
	}
	// The store reads a zero target as a new person, which only a split may ask for.
	if body.TargetId < 1 {
		s.invalidRequest(w, "targetId must be a positive integer.")
		return
	}
	var restore []domain.FaceMoveCredit
	if body.RestoreCredits != nil {
		for _, credit := range *body.RestoreCredits {
			if credit.Position < 0 {
				s.invalidRequest(w, "restoreCredits positions must not be negative.")
				return
			}
			restore = append(restore, domain.FaceMoveCredit{UserKey: credit.WorkKey, Position: credit.Position})
		}
	}
	moved, err := s.credits.MoveFaceAppearances(r.Context(), id, body.AppearanceIds, body.TargetId, restore)
	switch {
	case errors.Is(err, domain.ErrFaceAppearanceNotFound):
		s.notFoundReason(w, gen.ErrorReasonFaceAppearanceNotFound, "The person has no such face appearance.")
	case errors.Is(err, domain.ErrMergeSameCredit):
		s.invalidRequest(w, "targetId must be another person.")
	case err != nil:
		s.writeCreditError(w, peopleAPI, err)
	default:
		body := gen.FaceAppearanceMove{Person: toAPIPerson(moved.Person), RestoreCredits: []gen.FaceMoveCredit{}}
		for _, credit := range moved.Restore {
			body.RestoreCredits = append(body.RestoreCredits, gen.FaceMoveCredit{WorkKey: credit.UserKey, Position: credit.Position})
		}
		s.writeCreditBody(w, http.StatusOK, body)
	}
}

func (s *server) ListSimilarPeople(w http.ResponseWriter, r *http.Request, id gen.PersonId, params gen.ListSimilarPeopleParams) {
	limit := 8
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > 50 {
			s.invalidRequest(w, "limit must be from 1 to 50.")
			return
		}
		limit = *params.Limit
	}
	var appearanceIDs []int64
	if params.AppearanceId != nil {
		appearanceIDs = *params.AppearanceId
		if len(appearanceIDs) > 200 {
			s.invalidRequest(w, "appearanceId may be repeated at most 200 times.")
			return
		}
		if slices.ContainsFunc(appearanceIDs, func(id int64) bool { return id < 1 }) {
			s.invalidRequest(w, "appearanceId must be a positive integer.")
			return
		}
	}
	if !s.creditsConfigured(w) {
		return
	}
	people, err := s.credits.SimilarPeople(r.Context(), id, appearanceIDs, limit)
	if err != nil {
		s.writeCreditError(w, peopleAPI, err)
		return
	}
	body := gen.SimilarPersonList{Items: make([]gen.SimilarPerson, 0, len(people))}
	for _, p := range people {
		item := gen.SimilarPerson{
			Person: toAPIPerson(p.Person), Similarity: p.Similarity, FaceCount: p.FaceCount,
			SampleAppearanceIds: p.SampleAppearanceIDs,
		}
		if p.FirstSeenTitle != "" {
			item.FirstSeenTitle = &p.FirstSeenTitle
		}
		body.Items = append(body.Items, item)
	}
	s.writeCreditBody(w, http.StatusOK, body)
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
