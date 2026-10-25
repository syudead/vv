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

func TestExecuteAssistantProposalApprovesOnce(t *testing.T) {
	var db *store.DB
	env := newAuthEnvWith(t, t.TempDir(), func(opened *store.DB) Options {
		db = opened
		return Options{Assistant: app.NewAssistant(app.AssistantOptions{Store: opened.Assistant(), Logger: discardLogger()})}
	})
	owner := env.setup()
	p, err := db.Assistant().CreateProposal(context.Background(), "アニメのタグをまとめて", domain.AssistantPlan{
		ActionID: "mergeTag",
		Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetTag, Labels: []domain.AssistantLabel{
			{Kind: domain.AssistantLabelTag, ID: 1, Name: "Anime"},
		}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	execute := func() *httptest.ResponseRecorder {
		return env.serve(authRequest{
			method: http.MethodPost, target: "/api/assistant/proposals/" + strconv.FormatInt(p.ID, 10) + "/execute",
			cookies: []*http.Cookie{owner},
		})
	}

	first := execute()
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d", first.Code)
	}
	approved := decode[gen.AssistantProposal](t, first)
	if approved.Id != p.ID || approved.Status != gen.AssistantProposalStatusApproved || !approved.Irreversible ||
		approved.Operation != "mergeTag" || approved.Targets.Count != 1 || approved.ActivityId == nil {
		t.Errorf("approved = %+v", approved)
	}

	second := execute()
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
