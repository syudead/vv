package httpapi

import (
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
	person, err := s.credits.ConfirmFaceCandidate(r.Context(), id, otherID)
	if err != nil {
		s.writeFaceCandidateError(w, err)
		return
	}
	s.writeCreditBody(w, http.StatusOK, toAPIPerson(person))
}

func (s *server) DismissFaceCandidate(w http.ResponseWriter, r *http.Request, id gen.PersonId, otherID gen.FaceCandidateOtherId) {
	if !s.creditsConfigured(w) {
		return
	}
	if err := s.credits.DismissFaceCandidate(r.Context(), id, otherID); err != nil {
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
