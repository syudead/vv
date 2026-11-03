package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// AssistantReviewStore persists review runs, their listed candidates and the rejected sets; every change publishes
// AssistantChanged after commit.
type AssistantReviewStore interface {
	// StartReviewRun returns the run already running for the kind with started false.
	StartReviewRun(ctx context.Context, kind domain.AssistantReviewKind, request string, now time.Time) (domain.AssistantReviewRun, bool, error)
	// NextReviewRun returns domain.ErrNoJob when no run is running.
	NextReviewRun(ctx context.Context) (domain.AssistantReviewRun, error)
	// ReviewRun returns domain.ErrNotFound for a missing run.
	ReviewRun(ctx context.Context, id int64) (domain.AssistantReviewRun, error)
	FindReviewSets(ctx context.Context, kind domain.AssistantReviewKind) ([]domain.AssistantReviewSet, error)
	SetReviewTotal(ctx context.Context, runID int64, total int) error
	RecordReviewJudgement(ctx context.Context, runID int64, set domain.AssistantReviewSet, probability float64, listed bool) error
	FinishReviewRun(ctx context.Context, runID int64, status domain.AssistantReviewStatus, failure string, now time.Time) error
	RecoverReviewRuns(ctx context.Context, now time.Time) (int, error)
	// ReviewCandidates returns domain.ErrInvalidCursor for a cursor it did not issue.
	ReviewCandidates(ctx context.Context, runID int64, cursor string, limit int) ([]domain.AssistantReviewCandidate, string, error)
	ReviewCandidatesByID(ctx context.Context, runID int64, ids []int64) (map[int64]domain.AssistantReviewCandidate, error)
	// AdoptReviewCandidate returns domain.ErrAssistantCandidateNotListed when the candidate is no longer listed.
	AdoptReviewCandidate(ctx context.Context, candidateID int64, request string, plan domain.AssistantPlan, now time.Time) (domain.AssistantProposal, error)
	RejectReviewCandidates(ctx context.Context, runID int64, ids []int64, now time.Time) ([]int64, error)
}

// organize starts a review run of the kind, or answers the one already running.
func (t *assistantTurn) organize(ctx context.Context, req *domain.AssistantOrganizeRequest) (domain.AssistantResolution, error) {
	kind := domain.AssistantReviewKind(req.Kind)
	if t.a.reviews == nil || !kind.Valid() {
		return domain.AssistantResolution{Kind: domain.AssistantResolvedUnsupported}, nil
	}
	run, started, err := t.a.reviews.StartReviewRun(ctx, kind, t.input.Text, t.a.now())
	if err != nil {
		return domain.AssistantResolution{}, err
	}
	if started {
		t.a.wakeReviews()
	}
	return domain.AssistantResolution{Kind: domain.AssistantResolvedReview, Review: &run}, nil
}

func (a *Assistant) wakeReviews() {
	select {
	case a.reviewWake <- struct{}{}:
	default:
	}
}

// RunReviews judges the running review runs one at a time; a run interrupted by shutdown stays running for Recover
// to mark interrupted at the next start.
func (a *Assistant) RunReviews(ctx context.Context) {
	if a.reviews == nil {
		return
	}
	for ctx.Err() == nil {
		run, err := a.reviews.NextReviewRun(ctx)
		switch {
		case errors.Is(err, domain.ErrNoJob):
			a.waitReviews(ctx, nil)
		case err != nil:
			if ctx.Err() == nil {
				a.logger.Error("cannot read the next review run", slog.Any("error", err))
			}
			a.waitReviews(ctx, time.After(assistantRetryDelay))
		default:
			a.review(ctx, run)
		}
	}
}

func (a *Assistant) waitReviews(ctx context.Context, timer <-chan time.Time) {
	select {
	case <-ctx.Done():
	case <-a.reviewWake:
	case <-timer:
	}
}

// review finds the sets, judges each with one question and lists those at or above the threshold; the first
// classifier or storage failure ends the run failed.
func (a *Assistant) review(ctx context.Context, run domain.AssistantReviewRun) {
	err := a.judge(ctx, run)
	if ctx.Err() != nil {
		return
	}
	status, failure := domain.AssistantReviewDone, ""
	if err != nil {
		a.logger.Error("the review run failed", slog.Int64("run", run.ID), slog.Any("error", err))
		status, failure = domain.AssistantReviewFailed, err.Error()
	}
	if err := a.reviews.FinishReviewRun(ctx, run.ID, status, failure, a.now()); err != nil && ctx.Err() == nil {
		a.logger.Error("cannot finish the review run", slog.Int64("run", run.ID), slog.Any("error", err))
		// A run that cannot be finished would be picked again at once.
		a.waitReviews(ctx, time.After(assistantRetryDelay))
	}
}

func (a *Assistant) judge(ctx context.Context, run domain.AssistantReviewRun) error {
	if a.classifier == nil {
		return errors.New("the assistant cannot judge review sets: no classifier is configured")
	}
	sets, err := a.reviews.FindReviewSets(ctx, run.Kind)
	if err != nil {
		return err
	}
	if err := a.reviews.SetReviewTotal(ctx, run.ID, len(sets)); err != nil {
		return err
	}
	for _, set := range sets {
		answers, err := a.classifier.Classify(ctx, set.ClassifierRequest(run.Kind))
		if err != nil {
			return err
		}
		// An unanswered question is dropped like a no.
		probability, answered := answers[domain.AssistantReviewQuestionKey]
		listed := answered && probability >= domain.AssistantReviewThreshold
		if err := a.reviews.RecordReviewJudgement(ctx, run.ID, set, probability, listed); err != nil {
			return err
		}
	}
	return nil
}

// ReviewPage reads a run and one page of its candidates; see AssistantReviewStore for the errors.
func (a *Assistant) ReviewPage(ctx context.Context, runID int64, cursor string, limit int) (domain.AssistantReviewPage, error) {
	if a.reviews == nil {
		return domain.AssistantReviewPage{}, errors.New("the assistant has no review runs: it is not configured")
	}
	run, err := a.reviews.ReviewRun(ctx, runID)
	if err != nil {
		return domain.AssistantReviewPage{}, err
	}
	items, next, err := a.reviews.ReviewCandidates(ctx, runID, cursor, limit)
	if err != nil {
		return domain.AssistantReviewPage{}, err
	}
	return domain.AssistantReviewPage{Run: run, Items: items, NextCursor: next}, nil
}

// AdoptReviewCandidates stores one approved proposal per chosen candidate, in the order of ids, each running the
// set's one operation; skipped are the ids that are not listed in the run or whose member is gone. A missing run is
// domain.ErrNotFound.
func (a *Assistant) AdoptReviewCandidates(
	ctx context.Context, runID int64, ids []int64,
) (proposals []domain.AssistantProposal, skipped []int64, err error) {
	run, candidates, err := a.reviewCandidates(ctx, runID, ids)
	if err != nil {
		return nil, nil, err
	}
	proposals, skipped = []domain.AssistantProposal{}, []int64{}
	for _, id := range ids {
		c, ok := candidates[id]
		if !ok || c.State != domain.AssistantCandidateListed || c.Gone {
			skipped = append(skipped, id)
			continue
		}
		p, err := a.adopt(ctx, run, c)
		if errors.Is(err, domain.ErrAssistantCandidateNotListed) {
			skipped = append(skipped, id)
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		proposals = append(proposals, p)
		// A repeated id is skipped the second time.
		c.State = domain.AssistantCandidateAdopted
		candidates[id] = c
	}
	if len(proposals) > 0 {
		a.Wake()
	}
	return proposals, skipped, nil
}

func (a *Assistant) adopt(
	ctx context.Context, run domain.AssistantReviewRun, c domain.AssistantReviewCandidate,
) (domain.AssistantProposal, error) {
	plan, err := domain.AssistantReviewPlan(run.Kind, c.Members)
	if err != nil {
		return domain.AssistantProposal{}, err
	}
	if plan, err = a.withImpact(ctx, plan); err != nil {
		return domain.AssistantProposal{}, err
	}
	return a.reviews.AdoptReviewCandidate(ctx, c.ID, run.Request, plan, a.now())
}

// RejectReviewCandidates rejects the listed candidates among ids and remembers their sets; skipped are the others,
// in the order of ids. A missing run is domain.ErrNotFound.
func (a *Assistant) RejectReviewCandidates(
	ctx context.Context, runID int64, ids []int64,
) (rejected, skipped []int64, err error) {
	if _, _, err := a.reviewCandidates(ctx, runID, nil); err != nil {
		return nil, nil, err
	}
	rejected, err = a.reviews.RejectReviewCandidates(ctx, runID, ids, a.now())
	if err != nil {
		return nil, nil, err
	}
	skipped = []int64{}
	for _, id := range ids {
		if !slices.Contains(rejected, id) {
			skipped = append(skipped, id)
		}
	}
	return rejected, skipped, nil
}

func (a *Assistant) reviewCandidates(
	ctx context.Context, runID int64, ids []int64,
) (domain.AssistantReviewRun, map[int64]domain.AssistantReviewCandidate, error) {
	if a.reviews == nil {
		return domain.AssistantReviewRun{}, nil, errors.New("the assistant has no review runs: it is not configured")
	}
	run, err := a.reviews.ReviewRun(ctx, runID)
	if err != nil {
		return domain.AssistantReviewRun{}, nil, err
	}
	if len(ids) == 0 {
		return run, nil, nil
	}
	candidates, err := a.reviews.ReviewCandidatesByID(ctx, runID, ids)
	if err != nil {
		return domain.AssistantReviewRun{}, nil, fmt.Errorf("cannot read the review candidates: %w", err)
	}
	return run, candidates, nil
}

// recoverReviews marks the runs a restart stopped interrupted.
func (a *Assistant) recoverReviews(ctx context.Context) error {
	if a.reviews == nil {
		return nil
	}
	recovered, err := a.reviews.RecoverReviewRuns(ctx, a.now())
	if err != nil {
		return err
	}
	if recovered > 0 {
		a.logger.Info("marked the review runs of the previous run interrupted", slog.Int("count", recovered))
	}
	return nil
}
