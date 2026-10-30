package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// fakeTurns starts every turn as turn, or refuses it with err, keeps the input and answers one stored proposal.
type fakeTurns struct {
	AssistantProposals
	turn     domain.AssistantTurn
	err      error
	got      []domain.AssistantTurnInput
	proposal domain.AssistantProposal
}

func (f *fakeTurns) StartTurn(_ context.Context, in domain.AssistantTurnInput) (domain.AssistantTurn, error) {
	f.got = append(f.got, in)
	return f.turn, f.err
}

func (f *fakeTurns) Turn(_ context.Context, id int64) (domain.AssistantTurn, error) {
	if id != f.turn.ID {
		return domain.AssistantTurn{}, domain.ErrNotFound
	}
	return f.turn, nil
}

func (f *fakeTurns) Proposal(_ context.Context, id int64) (domain.AssistantProposal, error) {
	if id != f.proposal.ID {
		return domain.AssistantProposal{}, domain.ErrNotFound
	}
	return f.proposal, nil
}

type readinessOnly struct {
	OllamaConnection
	notReady domain.AssistantNotReady
}

func (r readinessOnly) AssistantReadiness(context.Context) (domain.AssistantStatus, error) {
	return domain.AssistantStatus{NotReady: r.notReady}, nil
}

func turnEnv(t *testing.T, turns *fakeTurns, notReady domain.AssistantNotReady) func(method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options {
		return Options{Assistant: turns, Ollama: readinessOnly{notReady: notReady}}
	})
	owner := env.setup()
	return func(method, target, body string) *httptest.ResponseRecorder {
		return env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{owner}})
	}
}

var runningTurn = domain.AssistantTurn{
	ID: 7, Status: domain.AssistantTurnRunning, Request: "猫の動画は何本?",
	StartedAt: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC),
}

func TestCreateAssistantTurnAnswersTheRunningTurn(t *testing.T) {
	turns := &fakeTurns{turn: runningTurn}
	send := turnEnv(t, turns, domain.AssistantReady)

	res := send(http.MethodPost, "/api/assistant/turns", `{"text":"猫の動画は何本?","previousTurnId":4,`+
		`"context":{"route":{"kind":"person","id":3},"selectedVideoIds":[7,8],"filter":{"favorite":true}}}`)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", res.Code, res.Body)
	}
	if got := decode[gen.AssistantTurn](t, res); got.Id != 7 || got.Status != gen.AssistantTurnStatusRunning ||
		len(got.Progress) != 0 {
		t.Fatalf("turn = %+v", got)
	}
	in := turns.got[0]
	if in.PreviousTurnID != 4 || in.Context.RouteID != 3 || !slices.Equal(in.Context.SelectedVideoIDs, []int64{7, 8}) ||
		in.Context.Filter == nil || !in.Context.Filter.Favorite {
		t.Fatalf("input = %+v", in)
	}
}

func TestCreateAssistantTurnRefusesASecondWhileOneRuns(t *testing.T) {
	turns := &fakeTurns{turn: runningTurn, err: domain.ErrAssistantBusy}
	res := turnEnv(t, turns, domain.AssistantReady)(http.MethodPost, "/api/assistant/turns",
		`{"text":"もう一つ","context":{"route":{"kind":"other"}}}`)

	body := decode[gen.Error](t, res)
	if res.Code != http.StatusConflict || body.Code != gen.ErrorCodeAssistantBusy || body.TurnId == nil || *body.TurnId != 7 {
		t.Fatalf("%d %+v", res.Code, body)
	}
}

func TestCreateAssistantTurnRefusesBeforeStarting(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		notReady domain.AssistantNotReady
		status   int
		code     gen.ErrorCode
		reason   gen.ErrorReason
	}{
		{name: "text over 500 characters", text: strings.Repeat("猫", 501), status: http.StatusBadRequest,
			code: gen.ErrorCodeInvalidRequest},
		{name: "not ready", text: "猫", notReady: domain.AssistantGenerationModelWithoutTools, status: http.StatusConflict,
			code: gen.ErrorCodeAssistantNotReady, reason: gen.ErrorReasonGenerationModelWithoutTools},
	}
	for _, c := range cases {
		turns := &fakeTurns{}
		res := turnEnv(t, turns, c.notReady)(http.MethodPost, "/api/assistant/turns",
			fmt.Sprintf(`{"text":%q,"context":{"route":{"kind":"other"}}}`, c.text))
		body := decode[gen.Error](t, res)
		if res.Code != c.status || body.Code != c.code || (c.reason != "" && (body.Reason == nil || *body.Reason != c.reason)) {
			t.Errorf("%s: %d %+v", c.name, res.Code, body)
		}
		if len(turns.got) != 0 {
			t.Errorf("%s: a turn started", c.name)
		}
	}
}

func TestGetAssistantTurnEmbedsItsProposalAndStop(t *testing.T) {
	turn := runningTurn
	turn.Status, turn.ProposalID = domain.AssistantTurnStopped, 9
	turn.Stop = &domain.AssistantTurnStop{Reason: domain.AssistantStopUngrounded}
	turn.Progress = []domain.AssistantProgressItem{{
		Tool: "getVideo", Kind: domain.AssistantProgressRead, At: turn.StartedAt, Error: "the operation answered 404",
		Labels: []domain.AssistantLabel{{Kind: domain.AssistantLabelVideo, ID: 7, Name: "Clip 7"}},
	}, {Tool: "start_review", Kind: domain.AssistantProgressBuiltin, At: turn.StartedAt, ReviewRunID: 5}}
	turns := &fakeTurns{turn: turn, proposal: domain.AssistantProposal{ID: 9, Status: domain.AssistantProposed}}
	send := turnEnv(t, turns, domain.AssistantReady)

	res := send(http.MethodGet, "/api/assistant/turns/7", "")

	got := decode[gen.AssistantTurn](t, res)
	if res.Code != http.StatusOK || got.Proposal == nil || got.Proposal.Id != 9 || got.Stop == nil ||
		got.Stop.Reason != gen.AssistantTurnStopReasonUngrounded {
		t.Fatalf("%d %+v", res.Code, got)
	}
	if item := got.Progress[0]; item.Tool != "getVideo" || item.Error == nil || len(item.Labels) != 1 ||
		item.Labels[0].Name != "Clip 7" {
		t.Fatalf("progress = %+v", got.Progress)
	}
	if item := got.Progress[1]; item.ReviewRunId == nil || *item.ReviewRunId != 5 || got.Progress[0].ReviewRunId != nil {
		t.Fatalf("review run ids = %+v", got.Progress)
	}
	if res := send(http.MethodGet, "/api/assistant/turns/8", ""); res.Code != http.StatusNotFound {
		t.Fatalf("expired turn: status = %d", res.Code)
	}
}
