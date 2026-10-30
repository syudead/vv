package httpapi

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// fakeReviews answers run 5 only and keeps the arguments.
type fakeReviews struct {
	AssistantProposals
	limit   int
	adopted []int64
}

func (f *fakeReviews) ReviewPage(_ context.Context, runID int64, _ string, limit int) (domain.AssistantReviewPage, error) {
	if runID != 5 {
		return domain.AssistantReviewPage{}, domain.ErrNotFound
	}
	f.limit = limit
	return domain.AssistantReviewPage{
		Run: domain.AssistantReviewRun{ID: 5, Kind: domain.AssistantReviewTags, Status: domain.AssistantReviewRunning,
			Total: 3, Judged: 1, Listed: 1, StartedAt: time.UnixMilli(1)},
		Items: []domain.AssistantReviewCandidate{{ID: 9, Probability: 0.8, State: domain.AssistantCandidateListed,
			Members: []domain.AssistantReviewMember{
				{Kind: domain.AssistantLabelTag, ID: 1, Name: "selfie", Count: 4},
				{Kind: domain.AssistantLabelTag, ID: 2, Name: "selfy", Count: 1},
			}}},
		NextCursor: "next",
	}, nil
}

func (f *fakeReviews) AdoptReviewCandidates(_ context.Context, _ int64, ids []int64) ([]domain.AssistantProposal, []int64, error) {
	f.adopted = ids
	return []domain.AssistantProposal{{ID: 30, Status: domain.AssistantApproved, Steps: domain.NewAssistantSteps([]domain.AssistantPlan{{ActionID: "mergeTag"}})}},
		[]int64{8}, nil
}

func TestAssistantReviewRunPageAndAdoption(t *testing.T) {
	reviews := &fakeReviews{}
	env := newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options { return Options{Assistant: reviews} })
	owner := env.setup()
	serve := func(method, target, body string) (int, []byte) {
		res := env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{owner}})
		return res.Code, res.Body.Bytes()
	}

	res := env.serve(authRequest{method: http.MethodGet, target: "/api/assistant/review-runs/5", cookies: []*http.Cookie{owner}})
	if res.Code != http.StatusOK {
		t.Fatalf("page status = %d: %s", res.Code, res.Body)
	}
	page := decode[gen.AssistantReviewRunPage](t, res)
	if reviews.limit != assistantReviewDefaultLimit || page.Run.Judged != 1 || len(page.Items) != 1 ||
		page.Items[0].Members[0].Count != 4 || page.NextCursor == nil || *page.NextCursor != "next" {
		t.Errorf("page = %+v (limit %d)", page, reviews.limit)
	}
	if code, _ := serve(http.MethodGet, "/api/assistant/review-runs/6", ""); code != http.StatusNotFound {
		t.Errorf("missing run status = %d", code)
	}
	if code, _ := serve(http.MethodGet, "/api/assistant/review-runs/5?limit=201", ""); code != http.StatusBadRequest {
		t.Errorf("limit 201 status = %d", code)
	}

	if code, body := serve(http.MethodPost, "/api/assistant/review-runs/5/candidates/adopt", `{"ids":[9,8]}`); code != http.StatusAccepted {
		t.Fatalf("adopt status = %d: %s", code, body)
	}
	if !slices.Equal(reviews.adopted, []int64{9, 8}) {
		t.Errorf("adopted ids = %v", reviews.adopted)
	}
	if code, _ := serve(http.MethodPost, "/api/assistant/review-runs/5/candidates/adopt", `{"ids":[]}`); code != http.StatusBadRequest {
		t.Errorf("empty ids status = %d", code)
	}
}
