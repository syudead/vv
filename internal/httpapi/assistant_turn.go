package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (s *server) CreateAssistantTurn(w http.ResponseWriter, r *http.Request) {
	if !s.assistantConfigured(w, 1) || !s.ollamaConfigured(w) {
		return
	}
	var body gen.AssistantTurnRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	in, ok := s.assistantTurnInput(w, body)
	if !ok {
		return
	}
	readiness, err := s.ollama.AssistantReadiness(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the assistant readiness.", err)
		return
	}
	if readiness.NotReady != domain.AssistantReady {
		s.writeReasonError(w, http.StatusConflict, gen.ErrorCodeAssistantNotReady, gen.ErrorReason(readiness.NotReady),
			"Save the Ollama connection and a generation model that can call tools first.")
		return
	}
	turn, err := s.assistant.StartTurn(r.Context(), in)
	switch {
	case errors.Is(err, domain.ErrAssistantBusy):
		s.writeErrorBody(w, http.StatusConflict, gen.Error{
			Code: gen.ErrorCodeAssistantBusy, Message: "Another request is still running.", TurnId: &turn.ID,
		})
	case err != nil:
		s.internalError(w, "Could not start the request.", err)
	default:
		s.writeAssistantTurn(w, r.Context(), http.StatusAccepted, turn)
	}
}

func (s *server) GetAssistantTurn(w http.ResponseWriter, r *http.Request, id gen.AssistantTurnId) {
	if !s.assistantConfigured(w, id) {
		return
	}
	turn, err := s.assistant.Turn(r.Context(), id)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "The request does not exist or is no longer kept.")
	case err != nil:
		s.internalError(w, "Could not read the request.", err)
	default:
		s.writeAssistantTurn(w, r.Context(), http.StatusOK, turn)
	}
}

func (s *server) ListAssistantTurns(w http.ResponseWriter, r *http.Request, params gen.ListAssistantTurnsParams) {
	if !s.assistantConfigured(w, 1) {
		return
	}
	if !params.Active {
		s.invalidRequest(w, "Only active=true is supported.")
		return
	}
	turns, err := s.assistant.ActiveTurns(r.Context())
	if err != nil {
		s.internalError(w, "Could not list the requests.", err)
		return
	}
	items := make([]gen.AssistantTurn, len(turns))
	for i, turn := range turns {
		if items[i], err = s.toAPIAssistantTurn(r.Context(), turn); err != nil {
			s.internalError(w, "Could not encode the request.", err)
			return
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.AssistantTurnList{Items: items}, s.logger)
}

func (s *server) writeAssistantTurn(w http.ResponseWriter, ctx context.Context, status int, turn domain.AssistantTurn) {
	body, err := s.toAPIAssistantTurn(ctx, turn)
	if err != nil {
		s.internalError(w, "Could not encode the request.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, body, s.logger)
}

// assistantTurnInput checks the limits the schema states and converts the context; false has written the error.
func (s *server) assistantTurnInput(w http.ResponseWriter, body gen.AssistantTurnRequest) (domain.AssistantTurnInput, bool) {
	if body.Text == "" || utf8.RuneCountInString(body.Text) > domain.AssistantTextMax {
		s.invalidRequest(w, "text must be 1 to 500 characters.")
		return domain.AssistantTurnInput{}, false
	}
	c := body.Context
	in := domain.AssistantTurnInput{Text: body.Text, PreviousTurnID: valueOrZero(body.PreviousTurnId), Context: domain.AssistantTurnContext{
		Route: domain.AssistantRouteKind(c.Route.Kind), RouteID: valueOrZero(c.Route.Id), SelectedVideoIDs: deref(c.SelectedVideoIds),
	}}
	if c.SelectedCollectionIds != nil {
		in.Context.SelectedGroupIDs = deref(c.SelectedCollectionIds.Groups)
		in.Context.SelectedSmartFolderIDs = deref(c.SelectedCollectionIds.SmartFolders)
	}
	if len(in.Context.SelectedVideoIDs) > domain.AssistantSelectionMax {
		s.invalidRequestLimit(w, reasonTooManyVideos, domain.AssistantSelectionMax, "Select at most 20,000 videos.")
		return domain.AssistantTurnInput{}, false
	}
	if c.Sort != nil {
		in.Context.Sort = domain.VideoSort(*c.Sort)
	}
	for _, pin := range deref(c.Pinned) {
		in.Context.Pinned = append(in.Context.Pinned,
			domain.AssistantPin{Kind: domain.AssistantLabelKind(pin.Kind), Name: pin.Name, ID: pin.Id})
	}
	if c.Filter != nil {
		raw, _ := json.Marshal(c.Filter) // A decoded rule always encodes.
		rule, err := domain.ParseSmartRule(raw)
		if err != nil {
			s.invalidRequest(w, "context.filter is not a valid library filter.")
			return domain.AssistantTurnInput{}, false
		}
		in.Context.Filter = &rule
	}
	return in, true
}

// toAPIAssistantTurn embeds the turn's proposal as it stands; an expired proposal is left out.
func (s *server) toAPIAssistantTurn(ctx context.Context, t domain.AssistantTurn) (gen.AssistantTurn, error) {
	out := gen.AssistantTurn{
		Id: t.ID, Status: gen.AssistantTurnStatus(t.Status), Request: t.Request, StartedAt: t.StartedAt,
		Judged: t.Judged, Progress: make([]gen.AssistantProgressItem, len(t.Progress)),
	}
	if !t.FinishedAt.IsZero() {
		out.FinishedAt = &t.FinishedAt
	}
	if t.Answer != "" {
		out.Answer = &t.Answer
	}
	for i, item := range t.Progress {
		out.Progress[i] = toAPIAssistantProgressItem(item)
	}
	if t.Directive != nil {
		directive, err := toAPIAssistantDirective(*t.Directive)
		if err != nil {
			return out, err
		}
		out.Directive = &directive
	}
	if c := t.Clarify; c != nil {
		out.Clarify = &gen.AssistantTurnClarification{
			Name: c.Name, Kind: gen.AssistantLabelKind(c.Kind), Candidates: toAPIAssistantCandidates(c.Candidates),
		}
	}
	if t.Stop != nil {
		out.Stop = &gen.AssistantTurnStop{Reason: gen.AssistantTurnStopReason(t.Stop.Reason)}
		if t.Stop.Detail != "" {
			out.Stop.Detail = &t.Stop.Detail
		}
		if e := t.Stop.Excluded; e != nil {
			out.Stop.Excluded = &gen.AssistantExcludedOperation{Operation: e.Operation, Reason: e.Reason, Page: e.Page}
		}
	}
	if t.ProposalID > 0 {
		p, err := s.assistant.Proposal(ctx, t.ProposalID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return out, err
		}
		if err == nil {
			proposal := toAPIAssistantProposal(p)
			out.Proposal = &proposal
		}
	}
	return out, nil
}

func toAPIAssistantProgressItem(item domain.AssistantProgressItem) gen.AssistantProgressItem {
	out := gen.AssistantProgressItem{
		Tool: item.Tool, Kind: gen.AssistantProgressItemKind(item.Kind), Labels: toAPIAssistantLabels(item.Labels),
		At: item.At, Images: positive(item.Images), N: positive(item.N), M: positive(item.M),
	}
	if item.Error != "" {
		out.Error = &item.Error
	}
	if item.ReviewRunID > 0 {
		out.ReviewRunId = &item.ReviewRunID
	}
	return out
}

// positive answers nil for zero, which the schema leaves out.
func positive(n int) *int {
	if n <= 0 {
		return nil
	}
	return &n
}

func toAPIAssistantCandidates(candidates []domain.AssistantCandidate) []gen.AssistantCandidate {
	out := make([]gen.AssistantCandidate, len(candidates))
	for i, c := range candidates {
		out[i] = gen.AssistantCandidate{Kind: gen.AssistantLabelKind(c.Kind), Id: c.ID, Name: c.Name, Count: c.Count}
	}
	return out
}

func toAPIAssistantDirective(d domain.AssistantDirective) (gen.AssistantDirective, error) {
	out := gen.AssistantDirective{Kind: gen.AssistantDirectiveKind(d.Kind)}
	if d.Criteria != nil {
		criteria, err := toAPIAssistantCriteria(*d.Criteria)
		if err != nil {
			return out, err
		}
		out.Criteria = criteria
	}
	if d.Destination != nil {
		out.Destination = toAPIAssistantDestination(*d.Destination)
	}
	return out, nil
}

func toAPIAssistantCriteria(c domain.AssistantCriteria) (*gen.AssistantCriteria, error) {
	out := &gen.AssistantCriteria{Labels: toAPIAssistantLabels(c.Labels)}
	raw, err := json.Marshal(c.Rule)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &out.Rule); err != nil {
		return nil, err
	}
	if c.Sort != "" {
		sort := gen.VideoSort(c.Sort)
		out.Sort = &sort
	}
	return out, nil
}

func toAPIAssistantDestination(d domain.AssistantDestination) *gen.AssistantDestination {
	out := &gen.AssistantDestination{Kind: gen.AssistantDestinationKind(d.Kind)}
	if d.ID != 0 {
		out.Id = &d.ID
	}
	if d.SettingsPage != "" {
		page := gen.AssistantDestinationSettingsPage(d.SettingsPage)
		out.SettingsPage = &page
	}
	return out
}
