package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// assistantUnavailable maps a generator failure to the reason the panel shows; nothing was stored.
var assistantUnavailable = []struct {
	cause  error
	reason gen.ErrorReason
}{
	{domain.ErrOllamaUnreachable, gen.ErrorReasonConnection},
	{domain.ErrNotOllama, gen.ErrorReasonConnection},
	{domain.ErrOllamaModel, gen.ErrorReasonModel},
	{domain.ErrGenerationModelMissing, gen.ErrorReasonModel},
	{domain.ErrOllamaTimeout, gen.ErrorReasonTimeout},
}

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
	notReady, err := s.ollama.AssistantReadiness(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the assistant readiness.", err)
		return
	}
	if notReady != domain.AssistantReady {
		s.writeReasonError(w, http.StatusConflict, gen.ErrorCodeAssistantNotReady, gen.ErrorReason(notReady),
			"Save the Ollama connection and a generation model first.")
		return
	}
	result, err := s.assistant.Turn(r.Context(), in)
	if err != nil {
		s.writeAssistantTurnError(w, err)
		return
	}
	outcome, err := toAPIAssistantOutcome(result)
	if err != nil {
		s.internalError(w, "Could not encode the assistant outcome.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.AssistantTurnResponse{Outcome: outcome}, s.logger)
}

func (s *server) writeAssistantTurnError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrAssistantTooManyVideos) {
		s.invalidRequestLimit(w, reasonTooManyVideos, domain.AssistantSelectionMax,
			"The request names more videos than one change can take. Narrow it down.")
		return
	}
	for _, failure := range assistantUnavailable {
		if errors.Is(err, failure.cause) {
			s.writeReasonError(w, http.StatusServiceUnavailable, gen.ErrorCodeAssistantUnavailable, failure.reason,
				"The generation model could not answer. Nothing was changed.")
			return
		}
	}
	s.internalError(w, "Could not interpret the request.", err)
}

// assistantTurnInput checks the limits the schema states and converts the context; false has written the error.
func (s *server) assistantTurnInput(w http.ResponseWriter, body gen.AssistantTurnRequest) (domain.AssistantTurnInput, bool) {
	if body.Text == "" || utf8.RuneCountInString(body.Text) > domain.AssistantTextMax {
		s.invalidRequest(w, "text must be 1 to 500 characters.")
		return domain.AssistantTurnInput{}, false
	}
	c := body.Context
	in := domain.AssistantTurnInput{Text: body.Text, Context: domain.AssistantTurnContext{
		Route: domain.AssistantRouteKind(c.Route.Kind), SelectedVideoIDs: deref(c.SelectedVideoIds),
	}}
	if c.Route.Id != nil {
		in.Context.RouteID = *c.Route.Id
	}
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
	if body.Previous != nil {
		in.Previous, _ = json.Marshal(body.Previous) // A decoded outcome always encodes.
		if body.Previous.Proposal != nil {
			in.PreviousProposalID = body.Previous.Proposal.Id
		}
	}
	return in, true
}

func toAPIAssistantOutcome(r domain.AssistantTurnResult) (gen.AssistantTurnOutcome, error) {
	out := gen.AssistantTurnOutcome{Kind: gen.AssistantTurnOutcomeKind(r.Kind)}
	var err error
	if r.Criteria != nil {
		if out.Criteria, err = toAPIAssistantCriteria(*r.Criteria); err != nil {
			return out, err
		}
	}
	if r.Destination != nil {
		out.Destination = toAPIAssistantDestination(*r.Destination)
	}
	if r.Answer != nil {
		if err := setAPIAssistantAnswer(&out, *r.Answer); err != nil {
			return out, err
		}
	}
	if r.Proposal != nil {
		proposal := toAPIAssistantProposal(*r.Proposal)
		out.Proposal = &proposal
	}
	if r.Review != nil {
		run := toAPIAssistantReviewRun(*r.Review)
		out.Run = &run
	}
	if r.Clarify != nil {
		setAPIAssistantClarify(&out, *r.Clarify)
	}
	if r.Unsupported != "" {
		out.Operation = &r.Unsupported
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

func setAPIAssistantAnswer(out *gen.AssistantTurnOutcome, a domain.AssistantAnswer) error {
	question := gen.AssistantTurnOutcomeQuestion(a.Question)
	out.Question, out.Total = &question, a.Total
	items := make([]gen.AssistantAnswerItem, len(a.Items))
	for i, item := range a.Items {
		items[i] = gen.AssistantAnswerItem{Id: item.ID, Title: item.Title}
	}
	out.Items = &items
	if a.Criteria == nil && a.Page == nil {
		return nil
	}
	out.Link = &gen.AssistantAnswerLink{}
	if a.Page != nil {
		out.Link.Destination = toAPIAssistantDestination(*a.Page)
	}
	if a.Criteria != nil {
		criteria, err := toAPIAssistantCriteria(*a.Criteria)
		if err != nil {
			return err
		}
		out.Link.Criteria = criteria
	}
	return nil
}

func setAPIAssistantClarify(out *gen.AssistantTurnOutcome, c domain.AssistantClarification) {
	about := gen.AssistantTurnOutcomeAbout(c.About)
	out.About = &about
	if len(c.Candidates) > 0 {
		candidates := make([]gen.AssistantCandidate, len(c.Candidates))
		for i, candidate := range c.Candidates {
			candidates[i] = gen.AssistantCandidate{
				Kind: gen.AssistantLabelKind(candidate.Kind), Id: candidate.ID, Name: candidate.Name, Count: candidate.Count,
			}
		}
		out.Candidates = &candidates
	}
	if len(c.Names) > 0 {
		out.Names = &c.Names
	}
}
