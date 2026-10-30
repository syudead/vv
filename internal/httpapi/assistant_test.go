package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

func TestExecuteAssistantProposalApprovesTheKeptStepsOnce(t *testing.T) {
	var db *store.DB
	env := newAuthEnvWith(t, t.TempDir(), func(opened *store.DB) Options {
		db = opened
		return Options{Assistant: app.NewAssistant(app.AssistantOptions{Store: opened.Assistant(), Logger: discardLogger()})}
	})
	owner := env.setup()
	merge := domain.AssistantPlan{
		ActionID: "mergeTag",
		Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetTag, Labels: []domain.AssistantLabel{
			{Kind: domain.AssistantLabelTag, ID: 1, Name: "Anime"},
		}},
	}
	p, err := db.Assistant().CreateProposal(context.Background(), 0, "アニメのタグをまとめて",
		[]domain.AssistantPlan{merge, {ActionID: "deleteTag"}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	execute := func(body string) *httptest.ResponseRecorder {
		return env.serve(authRequest{
			method: http.MethodPost, target: "/api/assistant/proposals/" + strconv.FormatInt(p.ID, 10) + "/execute",
			body: body, cookies: []*http.Cookie{owner},
		})
	}

	if res := execute(`{"steps":[2]}`); res.Code != http.StatusBadRequest {
		t.Fatalf("out of range: status = %d, want 400", res.Code)
	}
	first := execute(`{"steps":[0]}`)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d: %s", first.Code, first.Body)
	}
	approved := decode[gen.AssistantProposal](t, first)
	if approved.Id != p.ID || approved.Status != gen.AssistantProposalStatusApproved || approved.ActivityId == nil ||
		len(approved.Steps) != 2 {
		t.Fatalf("approved = %+v", approved)
	}
	if step := approved.Steps[0]; !step.Irreversible || step.Operation != "mergeTag" || step.Targets.Count != 1 ||
		step.Status != gen.AssistantStepStatusPending || approved.Steps[1].Status != gen.AssistantStepStatusSkipped {
		t.Errorf("steps = %+v", approved.Steps)
	}

	second := execute(`{"steps":[0]}`)
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d", second.Code)
	}
	conflict := decode[gen.Error](t, second)
	if conflict.Code != gen.ErrorCodeProposalNotPending || conflict.Proposal == nil ||
		conflict.Proposal.Status != gen.AssistantProposalStatusApproved {
		t.Errorf("conflict = %+v", conflict)
	}
}

func TestAssistantEventsAreOwnerOnly(t *testing.T) {
	events := NewEvents()
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	server := &server{}
	for _, guest := range []bool{false, true} {
		// Two changes before delivery coalesce into one event.
		events.Handle(domain.AssistantChanged{})
		events.Handle(domain.AssistantChanged{})
		response := httptest.NewRecorder()
		if err := server.writePendingEvents(context.Background(), response, sub, guest); err != nil {
			t.Fatal(err)
		}
		want := "event: assistant\ndata: {}\n\n"
		if guest {
			want = ""
		}
		if response.Body.String() != want {
			t.Fatalf("guest %t: %q, want %q", guest, response.Body.String(), want)
		}
	}
}
