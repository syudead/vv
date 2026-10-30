package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// fakeTurns answers every turn with result or err and keeps the input.
type fakeTurns struct {
	AssistantProposals
	result domain.AssistantTurnResult
	err    error
	got    []domain.AssistantTurnInput
}

func (f *fakeTurns) Turn(_ context.Context, in domain.AssistantTurnInput) (domain.AssistantTurnResult, error) {
	f.got = append(f.got, in)
	return f.result, f.err
}

type readinessOnly struct {
	OllamaConnection
	notReady domain.AssistantNotReady
}

func (r readinessOnly) AssistantReadiness(context.Context) (domain.AssistantNotReady, error) {
	return r.notReady, nil
}

func turnEnv(t *testing.T, turns *fakeTurns, notReady domain.AssistantNotReady) func(body string) *httptest.ResponseRecorder {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options {
		return Options{Assistant: turns, Ollama: readinessOnly{notReady: notReady}}
	})
	owner := env.setup()
	return func(body string) *httptest.ResponseRecorder {
		return env.serve(authRequest{
			method: http.MethodPost, target: "/api/assistant/turns", body: body, cookies: []*http.Cookie{owner},
		})
	}
}

func TestCreateAssistantTurnAnswersTheOutcomeShapes(t *testing.T) {
	tag := domain.AssistantLabel{Kind: domain.AssistantLabelTag, ID: 3, Name: "Cat"}
	results := map[gen.AssistantTurnOutcomeKind]domain.AssistantTurnResult{
		gen.AssistantTurnOutcomeKindFilter: {Kind: domain.AssistantTurnFilter, Criteria: &domain.AssistantCriteria{
			Rule: domain.SmartRule{Tags: []int64{3}}, Sort: domain.SortAddedDesc, Labels: []domain.AssistantLabel{tag},
		}},
		gen.AssistantTurnOutcomeKindProposal: {Kind: domain.AssistantTurnProposal, Proposal: &domain.AssistantProposal{
			ID: 9, Status: domain.AssistantProposed, Request: "tag", Steps: domain.NewAssistantSteps([]domain.AssistantPlan{{
				ActionID: "updateVideoTags.add", Created: []string{"Sunset"},
				Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetVideos, VideoKeys: []string{"k1", "k2"}},
			}}),
		}},
		gen.AssistantTurnOutcomeKindClarify: {Kind: domain.AssistantTurnClarify, Clarify: &domain.AssistantClarification{
			About: "value", Candidates: []domain.AssistantCandidate{{Kind: domain.AssistantLabelTag, ID: 3, Name: "Cat", Count: 4}},
		}},
	}
	for kind, result := range results {
		turns := &fakeTurns{result: result}
		send := turnEnv(t, turns, domain.AssistantReady)
		res := send(`{"text":"これに猫のタグを付けて","context":{"route":{"kind":"library"},"selectedVideoIds":[7,8],` +
			`"filter":{"favorite":true}},"previous":{"kind":"proposal","proposal":{"id":5,"status":"proposed",` +
			`"request":"x","steps":[{"index":0,"operation":"updateVideoTags.add","irreversible":false,` +
			`"targets":{"kind":"videos","count":1,"labels":[]},"change":[],"created":[],"createdBy":[],` +
			`"status":"pending","counts":{"total":1,"applied":0,"failed":0}}]}}}`)
		if res.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", kind, res.Code, res.Body)
		}
		got := decode[gen.AssistantTurnResponse](t, res).Outcome
		if got.Kind != kind {
			t.Fatalf("kind = %q, want %q", got.Kind, kind)
		}
		in := turns.got[0]
		if !slices.Equal(in.Context.SelectedVideoIDs, []int64{7, 8}) || in.Context.Filter == nil || !in.Context.Filter.Favorite ||
			in.PreviousProposalID != 5 || len(in.Previous) == 0 {
			t.Fatalf("%s: input = %+v", kind, in)
		}
		switch kind {
		case gen.AssistantTurnOutcomeKindFilter:
			if got.Criteria == nil || got.Criteria.Rule.Tags == nil || !slices.Equal(*got.Criteria.Rule.Tags, []int64{3}) ||
				len(got.Criteria.Labels) != 1 || got.Criteria.Labels[0].Name != "Cat" {
				t.Errorf("filter = %+v", got.Criteria)
			}
		case gen.AssistantTurnOutcomeKindProposal:
			if got.Proposal == nil || got.Proposal.Id != 9 || len(got.Proposal.Steps) != 1 ||
				got.Proposal.Steps[0].Targets.Count != 2 || !slices.Equal(got.Proposal.Steps[0].Created, []string{"Sunset"}) {
				t.Errorf("proposal = %+v", got.Proposal)
			}
		case gen.AssistantTurnOutcomeKindClarify:
			if got.About == nil || *got.About != gen.Value || got.Candidates == nil ||
				(*got.Candidates)[0].Count != 4 {
				t.Errorf("clarify = %+v", got)
			}
		}
	}
}

func TestCreateAssistantTurnRefusesBeforeInterpreting(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		notReady domain.AssistantNotReady
		err      error
		status   int
		code     gen.ErrorCode
		reason   gen.ErrorReason
		turns    int
	}{
		{name: "text over 500 characters", text: strings.Repeat("猫", 501), status: http.StatusBadRequest,
			code: gen.ErrorCodeInvalidRequest},
		{name: "not ready", text: "猫", notReady: domain.AssistantGenerationModelUnset, status: http.StatusConflict,
			code: gen.ErrorCodeAssistantNotReady, reason: gen.ErrorReasonGenerationModelUnset},
		{name: "timeout", text: "猫", err: fmt.Errorf("chat: %w", domain.ErrOllamaTimeout),
			status: http.StatusServiceUnavailable, code: gen.ErrorCodeAssistantUnavailable, reason: gen.ErrorReasonTimeout,
			turns: 1},
		{name: "too many videos", text: "猫", err: domain.ErrAssistantTooManyVideos, status: http.StatusBadRequest,
			code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonTooManyVideos, turns: 1},
	}
	for _, c := range cases {
		turns := &fakeTurns{err: c.err}
		res := turnEnv(t, turns, c.notReady)(fmt.Sprintf(`{"text":%q,"context":{"route":{"kind":"other"}}}`, c.text))
		body := decode[gen.Error](t, res)
		if res.Code != c.status || body.Code != c.code || (c.reason != "" && (body.Reason == nil || *body.Reason != c.reason)) {
			t.Errorf("%s: %d %+v", c.name, res.Code, body)
		}
		if len(turns.got) != c.turns {
			t.Errorf("%s: %d turns, want %d", c.name, len(turns.got), c.turns)
		}
	}
}
