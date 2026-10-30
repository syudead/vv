package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// AssistantProposals interprets requests, approves and reads the assistant's write proposals, and reads and
// decides its review runs; app.Assistant implements it.
type AssistantProposals interface {
	// Turn returns domain.ErrAssistantTooManyVideos for a target over the selection limit, and the generator's
	// domain.ErrOllama* causes when the model cannot answer.
	Turn(ctx context.Context, in domain.AssistantTurnInput) (domain.AssistantTurnResult, error)
	// Approve keeps the steps at the given indexes. It returns domain.ErrNotFound for a missing, discarded or
	// expired proposal, the proposal with domain.ErrAssistantProposalNotPending when it is no longer proposed,
	// domain.ErrAssistantStepsInvalid for an index it does not have, and *domain.AssistantStepNeedsSkippedError for
	// a kept step that uses what an unticked one creates.
	Approve(ctx context.Context, id int64, kept []int) (domain.AssistantProposal, error)
	Proposal(ctx context.Context, id int64) (domain.AssistantProposal, error)
	ActiveProposals(ctx context.Context) ([]domain.AssistantProposal, error)
	// ReviewPage returns domain.ErrNotFound for a missing run and domain.ErrInvalidCursor for a cursor it did not
	// issue.
	ReviewPage(ctx context.Context, runID int64, cursor string, limit int) (domain.AssistantReviewPage, error)
	// AdoptReviewCandidates and RejectReviewCandidates return domain.ErrNotFound for a missing run.
	AdoptReviewCandidates(ctx context.Context, runID int64, ids []int64) ([]domain.AssistantProposal, []int64, error)
	RejectReviewCandidates(ctx context.Context, runID int64, ids []int64) ([]int64, []int64, error)
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
	var body gen.AssistantExecution
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.Steps) == 0 {
		s.invalidRequest(w, "Keep at least one step.")
		return
	}
	p, err := s.assistant.Approve(r.Context(), id, body.Steps)
	var needs *domain.AssistantStepNeedsSkippedError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The proposal does not exist or has expired.")
	case errors.Is(err, domain.ErrAssistantStepsInvalid):
		s.invalidRequest(w, "Keep only steps the proposal has.")
	case errors.As(err, &needs):
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: gen.ErrorCodeStepNeedsSkipped, Message: "A kept step uses what an unticked step creates.", Step: &needs.Step,
		})
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
	body := gen.AssistantProposal{
		Id: p.ID, Status: gen.AssistantProposalStatus(p.Status), Request: p.Request,
		Steps: make([]gen.AssistantProposalStep, len(p.Steps)),
	}
	for i, step := range p.Steps {
		body.Steps[i] = toAPIAssistantStep(i, step)
	}
	if p.TurnID != 0 {
		body.TurnId = &p.TurnID
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

func toAPIAssistantStep(index int, step domain.AssistantStep) gen.AssistantProposalStep {
	plan := step.Plan
	count := len(plan.Targets.Labels)
	if len(plan.Targets.VideoKeys) > 0 {
		count = len(plan.Targets.VideoKeys)
	}
	body := gen.AssistantProposalStep{
		Index: index, Operation: plan.ActionID, Irreversible: step.Irreversible(),
		Targets: gen.AssistantProposalTargets{
			Kind: string(plan.Targets.Kind), Count: count,
			Labels: toAPIAssistantLabels(plan.Targets.Labels[:min(len(plan.Targets.Labels), assistantCardLabels)]),
		},
		Change:    make([]gen.AssistantChange, len(plan.Change)),
		Created:   append([]string{}, plan.Created...),
		CreatedBy: make([]gen.AssistantCreatedBy, len(plan.CreatedBy)),
		Status:    gen.AssistantStepStatus(step.Status),
		Counts:    gen.AssistantProposalCounts{Total: step.Total, Applied: step.Applied, Failed: step.Failed},
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
	for i, created := range plan.CreatedBy {
		body.CreatedBy[i] = gen.AssistantCreatedBy{Name: created.Name, Step: created.Step}
	}
	if plan.Impact != nil {
		body.Impact = &gen.AssistantImpact{Videos: plan.Impact.Videos}
	}
	if step.Error != nil {
		body.Error = &gen.AssistantProposalError{Code: step.Error.Code}
		if step.Error.Reason != "" {
			body.Error.Reason = &step.Error.Reason
		}
		if step.Error.Message != "" {
			body.Error.Message = &step.Error.Message
		}
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
