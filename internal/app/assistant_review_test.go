package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeAssistantReviews holds one run, the sets its finder returns and the candidates of the run, and records the writes.
type fakeAssistantReviews struct {
	AssistantReviewStore
	run        domain.AssistantReviewRun
	started    []domain.AssistantReviewRun
	sets       []domain.AssistantReviewSet
	total      int
	judgements []bool
	finished   domain.AssistantReviewStatus
	candidates map[int64]domain.AssistantReviewCandidate
	adopted    []domain.AssistantPlan
}

func (f *fakeAssistantReviews) StartReviewRun(
	_ context.Context, kind domain.AssistantReviewKind, request string, now time.Time,
) (domain.AssistantReviewRun, bool, error) {
	run := domain.AssistantReviewRun{ID: 5, Kind: kind, Request: request, Status: domain.AssistantReviewRunning, StartedAt: now}
	f.started = append(f.started, run)
	return run, true, nil
}

func (f *fakeAssistantReviews) ReviewRun(context.Context, int64) (domain.AssistantReviewRun, error) {
	return f.run, nil
}

func (f *fakeAssistantReviews) FindReviewSets(context.Context, domain.AssistantReviewKind) ([]domain.AssistantReviewSet, error) {
	return f.sets, nil
}

func (f *fakeAssistantReviews) SetReviewTotal(_ context.Context, _ int64, total int) error {
	f.total = total
	return nil
}

func (f *fakeAssistantReviews) RecordReviewJudgement(
	_ context.Context, _ int64, _ domain.AssistantReviewSet, _ float64, listed bool,
) error {
	f.judgements = append(f.judgements, listed)
	return nil
}

func (f *fakeAssistantReviews) FinishReviewRun(
	_ context.Context, _ int64, status domain.AssistantReviewStatus, _ string, _ time.Time,
) error {
	f.finished = status
	return nil
}

func (f *fakeAssistantReviews) ReviewCandidatesByID(
	context.Context, int64, []int64,
) (map[int64]domain.AssistantReviewCandidate, error) {
	return f.candidates, nil
}

func (f *fakeAssistantReviews) AdoptReviewCandidate(
	_ context.Context, id int64, request string, plan domain.AssistantPlan, _ time.Time,
) (domain.AssistantProposal, error) {
	f.adopted = append(f.adopted, plan)
	return domain.AssistantProposal{ID: id * 10, Status: domain.AssistantApproved, Request: request, Steps: domain.NewAssistantSteps([]domain.AssistantPlan{plan})}, nil
}

// factClassifier answers each request's question from the probability of its tag_a fact, and keeps the requests.
type factClassifier struct {
	answers map[string]float64
	got     []domain.ClassifierRequest
}

func (c *factClassifier) Classify(_ context.Context, req domain.ClassifierRequest) (map[string]float64, error) {
	c.got = append(c.got, req)
	return map[string]float64{req.Questions[0].Key: c.answers[req.Input.Facts["tag_a"]]}, nil
}

func tagPair(a, b int64, names ...string) domain.AssistantReviewSet {
	return domain.AssistantReviewSet{Members: []domain.AssistantReviewMember{
		{Kind: domain.AssistantLabelTag, ID: a, Name: names[0]}, {Kind: domain.AssistantLabelTag, ID: b, Name: names[1]},
	}}
}

func TestReviewRunJudgesEverySetAndListsFromHalf(t *testing.T) {
	reviews := &fakeAssistantReviews{sets: []domain.AssistantReviewSet{
		tagPair(1, 2, "selfie", "selfy"), tagPair(3, 4, "cats", "cat5"),
	}}
	classifier := &factClassifier{answers: map[string]float64{"selfie": 0.5, "cats": 0.49}}
	a := NewAssistant(AssistantOptions{Store: &fakeAssistantStore{}, Reviews: reviews, Classifier: classifier})

	a.review(context.Background(), domain.AssistantReviewRun{ID: 5, Kind: domain.AssistantReviewTags})

	if len(classifier.got) != 2 || len(classifier.got[0].Questions) != 1 || classifier.got[1].Input.Facts["tag_b"] != "cat5" {
		t.Errorf("classifier requests = %+v, want one question per set", classifier.got)
	}
	if reviews.total != 2 || !slices.Equal(reviews.judgements, []bool{true, false}) {
		t.Errorf("total = %d, judgements = %v; want 2 and 0.5 listed, 0.49 dropped", reviews.total, reviews.judgements)
	}
	if reviews.finished != domain.AssistantReviewDone {
		t.Errorf("finished = %q, want done", reviews.finished)
	}
}

func TestResolveOrganizeAnswersTheRunTheStoreStarted(t *testing.T) {
	f := newTurnFixture(`{"kind":"organize","organize":{"kind":"tags"}}`, nil)
	reviews := &fakeAssistantReviews{}
	f.a.reviews = reviews

	result, err := f.resolve(context.Background(), domain.AssistantTurnInput{Text: "タグを整理して"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != domain.AssistantResolvedReview || result.Review == nil || result.Review.ID != 5 {
		t.Fatalf("result = %+v, want the review run 5", result)
	}
	if len(reviews.started) != 1 || reviews.started[0].Kind != domain.AssistantReviewTags ||
		reviews.started[0].Request != "タグを整理して" {
		t.Errorf("started = %+v", reviews.started)
	}
}

func TestAdoptingTwoTagPairsStoresTwoMerges(t *testing.T) {
	member := func(id int64, name string, count int) domain.AssistantReviewMember {
		return domain.AssistantReviewMember{Kind: domain.AssistantLabelTag, ID: id, Name: name, Count: count}
	}
	listed := func(id int64, members ...domain.AssistantReviewMember) domain.AssistantReviewCandidate {
		return domain.AssistantReviewCandidate{ID: id, Members: members, State: domain.AssistantCandidateListed}
	}
	reviews := &fakeAssistantReviews{
		run: domain.AssistantReviewRun{ID: 5, Kind: domain.AssistantReviewTags, Request: "タグを整理して"},
		candidates: map[int64]domain.AssistantReviewCandidate{
			1: listed(1, member(11, "selfie", 9), member(12, "selfy", 2)),
			2: listed(2, member(21, "cats", 1), member(22, "Cats", 4)),
			3: {ID: 3, Members: []domain.AssistantReviewMember{member(31, "x", 1), member(32, "y", 1)}, State: domain.AssistantCandidateRejected},
		},
	}
	store := &fakeAssistantStore{}
	a := NewAssistant(AssistantOptions{Store: store, Reviews: reviews})

	proposals, skipped, err := a.AdoptReviewCandidates(context.Background(), 5, []int64{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(skipped, []int64{3, 4}) || len(proposals) != 2 {
		t.Fatalf("proposals = %d, skipped = %v; want 2 and the rejected and unknown ids", len(proposals), skipped)
	}
	for i, want := range []struct {
		destination string
		source      int64
	}{{"11", 12}, {"22", 21}} {
		plan := reviews.adopted[i]
		if plan.ActionID != "mergeTag" || plan.Call.Path["id"] != want.destination ||
			!slices.Equal(plan.Call.Body["sourceIds"].([]int64), []int64{want.source}) ||
			plan.Impact == nil || plan.Impact.Videos != 42 || proposals[i].Request != "タグを整理して" {
			t.Errorf("adoption %d = %+v, want mergeTag into %s from %d with its impact", i, plan, want.destination, want.source)
		}
	}
	if !slices.Equal(store.impactIDs, []int64{21}) {
		t.Errorf("last impact read the sources %v, want [21]", store.impactIDs)
	}
}
