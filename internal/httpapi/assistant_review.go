package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Review page and adoption limits the schema states.
const (
	assistantReviewDefaultLimit = 50
	assistantReviewMaxLimit     = 200
	assistantReviewMaxIDs       = 1000
)

func (s *server) GetAssistantReviewRun(
	w http.ResponseWriter, r *http.Request, id gen.AssistantReviewRunId, params gen.GetAssistantReviewRunParams,
) {
	if !s.assistantReviewConfigured(w, id) {
		return
	}
	limit := assistantReviewDefaultLimit
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < 1 || limit > assistantReviewMaxLimit {
		s.invalidRequest(w, "limit must be 1 to 200.")
		return
	}
	page, err := s.assistant.ReviewPage(r.Context(), id, valueOrZero(params.Cursor), limit)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The review run does not exist.")
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequest(w, "The cursor is not valid.")
	case err != nil:
		s.internalError(w, "Could not read the review run.", err)
	default:
		body := gen.AssistantReviewRunPage{
			Run: toAPIAssistantReviewRun(page.Run), Items: make([]gen.AssistantReviewCandidate, len(page.Items)),
		}
		for i, c := range page.Items {
			body.Items[i] = toAPIAssistantReviewCandidate(c)
		}
		if page.NextCursor != "" {
			body.NextCursor = &page.NextCursor
		}
		w.Header().Set("Cache-Control", cacheNoStore)
		writeJSON(w, http.StatusOK, body, s.logger)
	}
}

func (s *server) AdoptAssistantReviewCandidates(w http.ResponseWriter, r *http.Request, id gen.AssistantReviewRunId) {
	ids, ok := s.assistantReviewIDs(w, r, id)
	if !ok {
		return
	}
	proposals, skipped, err := s.assistant.AdoptReviewCandidates(r.Context(), id, ids)
	if s.assistantReviewFailed(w, err, "Could not adopt the review candidates.") {
		return
	}
	body := gen.AssistantReviewAdoption{Proposals: make([]gen.AssistantProposal, len(proposals)), SkippedIds: skipped}
	for i, p := range proposals {
		body.Proposals[i] = toAPIAssistantProposal(p)
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, body, s.logger)
}

func (s *server) RejectAssistantReviewCandidates(w http.ResponseWriter, r *http.Request, id gen.AssistantReviewRunId) {
	ids, ok := s.assistantReviewIDs(w, r, id)
	if !ok {
		return
	}
	rejected, skipped, err := s.assistant.RejectReviewCandidates(r.Context(), id, ids)
	if s.assistantReviewFailed(w, err, "Could not reject the review candidates.") {
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.AssistantReviewRejection{RejectedIds: rejected, SkippedIds: skipped}, s.logger)
}

func (s *server) assistantReviewConfigured(w http.ResponseWriter, id int64) bool {
	if s.assistant == nil {
		s.internalError(w, "The assistant is not configured.", nil)
		return false
	}
	if id < 1 {
		s.invalidRequest(w, "Specify a valid review run id.")
		return false
	}
	return true
}

// assistantReviewIDs reads the candidate ids of an adopt or reject request; false has written the error.
func (s *server) assistantReviewIDs(w http.ResponseWriter, r *http.Request, id int64) ([]int64, bool) {
	if !s.assistantReviewConfigured(w, id) {
		return nil, false
	}
	var body gen.AssistantReviewCandidateIds
	if !s.readJSONBody(w, r, &body) {
		return nil, false
	}
	if len(body.Ids) == 0 || len(body.Ids) > assistantReviewMaxIDs {
		s.invalidRequest(w, "ids must hold 1 to 1,000 candidate ids.")
		return nil, false
	}
	return body.Ids, true
}

// assistantReviewFailed writes the error of an adopt or reject request; false means there was none.
func (s *server) assistantReviewFailed(w http.ResponseWriter, err error, message string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The review run does not exist.")
	default:
		s.internalError(w, message, err)
	}
	return true
}

func toAPIAssistantReviewRun(run domain.AssistantReviewRun) gen.AssistantReviewRun {
	out := gen.AssistantReviewRun{
		Id: run.ID, Kind: gen.AssistantReviewRunKind(run.Kind), Status: gen.AssistantReviewRunStatus(run.Status),
		Total: run.Total, Judged: run.Judged, Listed: run.Listed, StartedAt: run.StartedAt,
	}
	if !run.FinishedAt.IsZero() {
		out.FinishedAt = &run.FinishedAt
	}
	return out
}

func toAPIAssistantReviewCandidate(c domain.AssistantReviewCandidate) gen.AssistantReviewCandidate {
	out := gen.AssistantReviewCandidate{
		Id: c.ID, Probability: c.Probability, State: gen.AssistantReviewCandidateState(c.State),
		Members: make([]gen.AssistantReviewMember, len(c.Members)),
	}
	for i, m := range c.Members {
		out.Members[i] = gen.AssistantReviewMember{
			Kind: gen.AssistantLabelKind(m.Kind), Id: m.ID, Name: m.Name, Count: m.Count,
		}
	}
	return out
}
