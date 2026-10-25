package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// AssistantProposals interprets requests and approves and reads the assistant's write proposals; app.Assistant
// implements it.
type AssistantProposals interface {
	// Turn returns domain.ErrAssistantTooManyVideos for a target over the selection limit, and the generator's
	// domain.ErrOllama* causes when the model cannot answer.
	Turn(ctx context.Context, in domain.AssistantTurnInput) (domain.AssistantTurnResult, error)
	// Approve returns domain.ErrNotFound for a missing, discarded or expired proposal, and the proposal with
	// domain.ErrAssistantProposalNotPending when it is no longer proposed.
	Approve(ctx context.Context, id int64) (domain.AssistantProposal, error)
	Proposal(ctx context.Context, id int64) (domain.AssistantProposal, error)
	ActiveProposals(ctx context.Context) ([]domain.AssistantProposal, error)
}

// assistantCardLabels is how many targets the card names.
const assistantCardLabels = 10

func (s *server) assistantConfigured(w http.ResponseWriter, id int64) bool {
	if s.assistant == nil {
		s.internalError(w, "The assistant is not configured.", nil)
		return false
	}
	if id < 1 {
		s.invalidRequest(w, "Specify a valid proposal id.")
		return false
	}
	return true
}

func (s *server) ExecuteAssistantProposal(w http.ResponseWriter, r *http.Request, id gen.AssistantProposalId) {
	if !s.assistantConfigured(w, id) {
		return
	}
	p, err := s.assistant.Approve(r.Context(), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The proposal does not exist or has expired.")
	case errors.Is(err, domain.ErrAssistantProposalNotPending):
		body := toAPIAssistantProposal(p)
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: gen.ErrorCodeProposalNotPending, Message: "The proposal is no longer waiting for approval.", Proposal: &body,
		})
	case err != nil:
		s.internalError(w, "Could not approve the proposal.", err)
	default:
		w.Header().Set("Cache-Control", cacheNoStore)
		writeJSON(w, http.StatusAccepted, toAPIAssistantProposal(p), s.logger)
	}
}

func (s *server) GetAssistantProposal(w http.ResponseWriter, r *http.Request, id gen.AssistantProposalId) {
	if !s.assistantConfigured(w, id) {
		return
	}
	p, err := s.assistant.Proposal(r.Context(), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The proposal does not exist or has expired.")
	case err != nil:
		s.internalError(w, "Could not read the proposal.", err)
	default:
		w.Header().Set("Cache-Control", cacheNoStore)
		writeJSON(w, http.StatusOK, toAPIAssistantProposal(p), s.logger)
	}
}

func (s *server) ListAssistantProposals(w http.ResponseWriter, r *http.Request, params gen.ListAssistantProposalsParams) {
	if !s.assistantConfigured(w, 1) {
		return
	}
	if !params.Active {
		s.invalidRequest(w, "Only active=true is supported.")
		return
	}
	proposals, err := s.assistant.ActiveProposals(r.Context())
	if err != nil {
		s.internalError(w, "Could not list the proposals.", err)
		return
	}
	items := make([]gen.AssistantProposal, len(proposals))
	for i, p := range proposals {
		items[i] = toAPIAssistantProposal(p)
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.AssistantProposalList{Items: items}, s.logger)
}

func toAPIAssistantProposal(p domain.AssistantProposal) gen.AssistantProposal {
	plan := p.Plan
	count := len(plan.Targets.Labels)
	if len(plan.Targets.VideoKeys) > 0 {
		count = len(plan.Targets.VideoKeys)
	}
	body := gen.AssistantProposal{
		Id: p.ID, Status: gen.AssistantProposalStatus(p.Status), Request: p.Request, Operation: plan.ActionID,
		Irreversible: p.Irreversible(),
		Targets: gen.AssistantProposalTargets{
			Kind: string(plan.Targets.Kind), Count: count,
			Labels: toAPIAssistantLabels(plan.Targets.Labels[:min(len(plan.Targets.Labels), assistantCardLabels)]),
		},
		Change:  make([]gen.AssistantChange, len(plan.Change)),
		Created: append([]string{}, plan.Created...),
		Counts: gen.AssistantProposalCounts{
			Total: p.Counts.Total, Applied: p.Counts.Applied, Failed: p.Counts.Failed,
		},
	}
	for i, change := range plan.Change {
		body.Change[i] = gen.AssistantChange{Argument: string(change.Argument)}
		if len(change.Labels) > 0 {
			labels := toAPIAssistantLabels(change.Labels)
			body.Change[i].Labels = &labels
		}
		if len(change.Value) > 0 {
			var value any
			// The value was encoded by the plan, so it always decodes.
			_ = json.Unmarshal(change.Value, &value)
			body.Change[i].Value = value
		}
	}
	if plan.Impact != nil {
		body.Impact = &gen.AssistantImpact{Videos: plan.Impact.Videos}
	}
	if p.Error != nil {
		body.Error = &gen.AssistantProposalError{Code: p.Error.Code}
		if p.Error.Reason != "" {
			body.Error.Reason = &p.Error.Reason
		}
	}
	if p.ActivityID != 0 {
		body.ActivityId = &p.ActivityID
	}
	if !p.StartedAt.IsZero() {
		body.StartedAt = &p.StartedAt
	}
	if !p.FinishedAt.IsZero() {
		body.FinishedAt = &p.FinishedAt
	}
	return body
}

func toAPIAssistantLabels(labels []domain.AssistantLabel) []gen.AssistantLabel {
	out := make([]gen.AssistantLabel, len(labels))
	for i, label := range labels {
		out[i] = gen.AssistantLabel{Kind: gen.AssistantLabelKind(label.Kind), Name: label.Name}
		if label.ID != 0 {
			out[i].Id = &label.ID
		}
	}
	return out
}
