package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// EditVideoCredits takes the same video targets as UpdateVideoTags and reports work-info errors at edits[i].
func (s *server) EditVideoCredits(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoCreditsRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	edits := make([]domain.CreditEdit, 0, len(body.Edits))
	for _, edit := range body.Edits {
		edits = append(edits, domain.CreditEdit{
			Kind: domain.CreditKind(edit.Kind), Role: edit.Role,
			Action: domain.CreditEditAction(edit.Action), IDs: edit.Ids,
		})
	}
	// Reject a malformed request before resolving smart folders.
	if err := domain.ValidateCreditEdits(edits); err != nil {
		s.writeVideoWorkInfoError(w, err)
		return
	}
	if !s.workInfoConfigured(w) {
		return
	}
	videoIDs, ok := s.resolveVideoTargets(w, r, body.VideoIds, body.SmartFolderIds)
	if !ok {
		return
	}
	applied, err := s.workInfo.EditCredits(r.Context(), videoIDs, edits)
	if err != nil {
		s.writeVideoWorkInfoError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VideoCreditsResponse{Applied: applied}, s.logger)
}
