package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (s *server) ListFaceCandidates(w http.ResponseWriter, r *http.Request, id gen.PersonId) {
	if !s.creditsConfigured(w) {
		return
	}
	pairs, err := s.credits.FaceCandidates(r.Context(), id)
	if err != nil {
		s.writeCreditError(w, peopleAPI, err)
		return
	}
	items := make([]gen.FaceCandidate, 0, len(pairs))
	for _, p := range pairs {
		items = append(items, gen.FaceCandidate{
			Person: gen.FaceCandidatePerson{
				Id: p.Person.ID, Name: p.Person.Name, Provisional: p.Person.Provisional,
				ImageRevision: optionalPositive(p.ImageRevision),
			},
			Similarity: p.Similarity,
		})
	}
	s.writeCreditBody(w, http.StatusOK, gen.FaceCandidateList{Items: items})
}

func (s *server) ConfirmFaceCandidate(w http.ResponseWriter, r *http.Request, id gen.PersonId, otherID gen.FaceCandidateOtherId) {
	if !s.creditsConfigured(w) {
		return
	}
	confirmed, err := s.credits.ConfirmFaceCandidate(r.Context(), id, otherID)
	if err != nil {
		s.writeFaceCandidateError(w, err)
		return
	}
	body := gen.FaceCandidateConfirmation{Person: toAPIPerson(confirmed.Person)}
	if confirmed.UndoActivityID != 0 {
		body.UndoActivityId = &confirmed.UndoActivityID
	}
	s.writeCreditBody(w, http.StatusOK, body)
}

func (s *server) DismissFaceCandidate(w http.ResponseWriter, r *http.Request, id gen.PersonId, otherID gen.FaceCandidateOtherId) {
	s.setFaceCandidateDismissed(w, r, func(ctx context.Context) error { return s.credits.DismissFaceCandidate(ctx, id, otherID) })
}

func (s *server) ReopenFaceCandidate(w http.ResponseWriter, r *http.Request, id gen.PersonId, otherID gen.FaceCandidateOtherId) {
	s.setFaceCandidateDismissed(w, r, func(ctx context.Context) error { return s.credits.ReopenFaceCandidate(ctx, id, otherID) })
}

func (s *server) setFaceCandidateDismissed(w http.ResponseWriter, r *http.Request, set func(context.Context) error) {
	if !s.creditsConfigured(w) {
		return
	}
	if err := set(r.Context()); err != nil {
		s.writeFaceCandidateError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) writeFaceCandidateError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrFaceCandidateNotFound) {
		s.notFoundReason(w, gen.ErrorReasonFaceCandidateNotFound, "These two people have no open face candidate pair.")
		return
	}
	s.writeCreditError(w, peopleAPI, err)
}
