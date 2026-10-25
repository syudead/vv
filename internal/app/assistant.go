package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// AssistantStore persists the assistant's proposals; each transition is guarded by the previous status and writes
// the proposal's assistant_run entry in the same transaction.
type AssistantStore interface {
	CreateProposal(ctx context.Context, request string, plan domain.AssistantPlan, now time.Time) (domain.AssistantProposal, error)
	// Proposal returns domain.ErrNotFound for a missing proposal.
	Proposal(ctx context.Context, id int64) (domain.AssistantProposal, error)
	ActiveProposals(ctx context.Context, since time.Time) ([]domain.AssistantProposal, error)
	// DiscardProposal returns domain.ErrNotFound for a missing proposal and domain.ErrAssistantProposalNotPending
	// for any other status than proposed.
	DiscardProposal(ctx context.Context, id int64, now time.Time) error
	// ApproveProposal returns domain.ErrNotFound for a missing or discarded proposal, and the proposal with
	// domain.ErrAssistantProposalNotPending for any other status than proposed.
	ApproveProposal(ctx context.Context, id int64, now time.Time) (domain.AssistantProposal, error)
	// ClaimProposal returns domain.ErrNoJob when no approved proposal waits.
	ClaimProposal(ctx context.Context, now time.Time) (domain.AssistantProposal, error)
	RecordProgress(ctx context.Context, id int64, counts domain.AssistantCounts, now time.Time) error
	FinishProposal(ctx context.Context, id int64, result domain.AssistantRunResult, now time.Time) error
	ExpireProposals(ctx context.Context, before time.Time) (int, error)
	RecoverProposals(ctx context.Context, now time.Time) (int, error)
	CurrentLabels(ctx context.Context, plan domain.AssistantPlan) (domain.AssistantCurrent, error)
	VideoIDs(ctx context.Context, keys []string) (map[string]int64, error)
	MergeImpact(ctx context.Context, target domain.AssistantTarget, sourceIDs []int64) (int, error)
}

type AssistantOptions struct {
	Store AssistantStore
	// Generator, Vocabulary and Videos are required by Turn; History by the recent question and SmartFolders by
	// targets that expand smart folders.
	Generator    StructuredGenerator
	Vocabulary   AssistantVocabulary
	Videos       AssistantVideos
	SmartFolders AssistantSmartFolders
	History      AssistantHistory
	// Nil uses slog.Default.
	Logger *slog.Logger
	// Nil uses time.Now.
	Now func() time.Time
}

// Assistant interprets requests into typed outcomes, stores write proposals and executes the approved ones, one at a time, through the screen API's own
// operations, in batches of user keys.
type Assistant struct {
	store        AssistantStore
	generator    StructuredGenerator
	vocabulary   AssistantVocabulary
	videos       AssistantVideos
	smartFolders AssistantSmartFolders
	history      AssistantHistory
	logger       *slog.Logger
	now          func() time.Time
	wake         chan struct{}

	runnerMu sync.RWMutex
	runner   ActionRunner
}

// NewAssistant does not execute proposals until Run, nor before UseRunner registers the runner.
func NewAssistant(opts AssistantOptions) *Assistant {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Assistant{
		store: opts.Store, generator: opts.Generator, vocabulary: opts.Vocabulary, videos: opts.Videos,
		smartFolders: opts.SmartFolders, history: opts.History,
		logger: logger.With(slog.String("component", "assistant")), now: now,
		// Coalesce wakeups in a one-element buffer.
		wake: make(chan struct{}, 1),
	}
}

// UseRunner registers the runner once the HTTP server that implements it exists, and wakes the worker.
func (a *Assistant) UseRunner(runner ActionRunner) {
	a.runnerMu.Lock()
	a.runner = runner
	a.runnerMu.Unlock()
	a.Wake()
}

func (a *Assistant) currentRunner() ActionRunner {
	a.runnerMu.RLock()
	defer a.runnerMu.RUnlock()
	return a.runner
}

// Wake is nonblocking.
func (a *Assistant) Wake() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// Propose stores a resolved plan for the card. A merge also stores the number of videos it moves; no other
// operation carries an impact.
func (a *Assistant) Propose(ctx context.Context, request string, plan domain.AssistantPlan) (domain.AssistantProposal, error) {
	if _, ok := plan.Action(); !ok {
		return domain.AssistantProposal{}, fmt.Errorf("%w: %q", domain.ErrAssistantActionUnknown, plan.ActionID)
	}
	plan.Impact = nil
	if target, sources, ok := plan.MergeSources(); ok {
		videos, err := a.store.MergeImpact(ctx, target, sources)
		if err != nil {
			return domain.AssistantProposal{}, err
		}
		plan.Impact = &domain.AssistantImpact{Videos: videos}
	}
	return a.store.CreateProposal(ctx, request, plan, a.now())
}

// Approve moves the proposal to approved and wakes the worker; see AssistantStore.ApproveProposal for the errors.
func (a *Assistant) Approve(ctx context.Context, id int64) (domain.AssistantProposal, error) {
	p, err := a.store.ApproveProposal(ctx, id, a.now())
	if err != nil {
		return p, err
	}
	a.Wake()
	return p, nil
}

func (a *Assistant) Proposal(ctx context.Context, id int64) (domain.AssistantProposal, error) {
	return a.store.Proposal(ctx, id)
}

// ActiveProposals lists the approved and running proposals and those finished within the last hour.
func (a *Assistant) ActiveProposals(ctx context.Context) ([]domain.AssistantProposal, error) {
	return a.store.ActiveProposals(ctx, a.now().Add(-domain.AssistantProposalTTL))
}

// Recover must run before Run: proposals whose thread is gone are deleted and runs a restart stopped fail.
func (a *Assistant) Recover(ctx context.Context) error {
	recovered, err := a.store.RecoverProposals(ctx, a.now())
	if err != nil {
		return err
	}
	if recovered > 0 {
		a.logger.Info("recovered the assistant proposals of the previous run", slog.Int("count", recovered))
	}
	return nil
}

// assistantRetryDelay paces the loop after a storage error.
const assistantRetryDelay = 5 * time.Second

// assistantExpiryInterval is how often an idle worker deletes expired proposals.
const assistantExpiryInterval = 5 * time.Minute

// Run executes approved proposals one at a time and deletes expired ones; a run interrupted by shutdown stays
// running for Recover to fail at the next start.
func (a *Assistant) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if _, err := a.store.ExpireProposals(ctx, a.now().Add(-domain.AssistantProposalTTL)); err != nil && ctx.Err() == nil {
			a.logger.Error("cannot delete the expired assistant proposals", slog.Any("error", err))
		}
		runner := a.currentRunner()
		if runner == nil {
			a.wait(ctx, nil)
			continue
		}
		p, err := a.store.ClaimProposal(ctx, a.now())
		switch {
		case errors.Is(err, domain.ErrNoJob):
			a.wait(ctx, time.After(assistantExpiryInterval))
		case err != nil:
			if ctx.Err() == nil {
				a.logger.Error("cannot claim an assistant proposal", slog.Any("error", err))
			}
			a.wait(ctx, time.After(assistantRetryDelay))
		default:
			a.execute(ctx, runner, p)
		}
	}
}

func (a *Assistant) wait(ctx context.Context, timer <-chan time.Time) {
	select {
	case <-ctx.Done():
	case <-a.wake:
	case <-timer:
	}
}

// execute re-reads the targets before the first call, then runs the batches in order and stops at the first
// failed one; a video gone between batches counts as failed.
func (a *Assistant) execute(ctx context.Context, runner ActionRunner, p domain.AssistantProposal) {
	result, err := a.run(ctx, runner, p)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		a.logger.Error("the assistant proposal failed", slog.Int64("proposal", p.ID), slog.Any("error", err))
		result.Status = domain.AssistantFailed
		if result.Counts.Applied > 0 {
			result.Status = domain.AssistantPartial
		}
		result.Error = &domain.AssistantProposalError{Code: "internal"}
	}
	if err := a.store.FinishProposal(ctx, p.ID, result, a.now()); err != nil && ctx.Err() == nil {
		a.logger.Error("cannot finish the assistant proposal", slog.Int64("proposal", p.ID), slog.Any("error", err))
	}
}

// run returns an error only for a storage failure; a failed operation call is part of the result.
func (a *Assistant) run(ctx context.Context, runner ActionRunner, p domain.AssistantProposal) (domain.AssistantRunResult, error) {
	plan := p.Plan
	result := domain.AssistantRunResult{Counts: domain.AssistantCounts{Total: plan.Total()}}
	current, err := a.store.CurrentLabels(ctx, plan)
	if err != nil {
		return result, err
	}
	if plan.Stale(current) {
		result.Status = domain.AssistantStale
		return result, nil
	}
	for i, keys := range plan.Batches() {
		ids, missing, err := a.batchVideos(ctx, i, keys, current)
		if err != nil {
			return result, err
		}
		result.Counts.Failed += missing
		if len(keys) == 0 || len(ids) > 0 {
			if _, err := runner.Run(ctx, plan.CallFor(ids)); err != nil {
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				result.Counts.Failed += batchCount(plan, ids)
				result.Error = callFailure(err)
				result.Status = domain.AssistantPartial
				if i == 0 && result.Counts.Applied == 0 {
					result.Status = domain.AssistantFailed
				}
				return result, nil
			}
			result.Counts.Applied += batchCount(plan, ids)
		}
		if err := a.store.RecordProgress(ctx, p.ID, result.Counts, a.now()); err != nil {
			return result, err
		}
	}
	result.Status = domain.AssistantDone
	if result.Counts.Failed > 0 {
		result.Status = domain.AssistantPartial
	}
	return result, nil
}

// batchVideos maps a batch's user keys to current video ids in plan order; the first batch uses the stale check's
// read, later ones read again, and missing counts the keys that are gone.
func (a *Assistant) batchVideos(
	ctx context.Context, batch int, keys []string, current domain.AssistantCurrent,
) ([]int64, int, error) {
	found := current.Videos
	if batch > 0 {
		var err error
		if found, err = a.store.VideoIDs(ctx, keys); err != nil {
			return nil, 0, err
		}
	}
	ids := make([]int64, 0, len(keys))
	for _, key := range keys {
		if id, ok := found[key]; ok {
			ids = append(ids, id)
		}
	}
	return ids, len(keys) - len(ids), nil
}

// batchCount counts a call in user keys for a batched operation, otherwise as the one call.
func batchCount(plan domain.AssistantPlan, ids []int64) int {
	if plan.Batched() {
		return len(ids)
	}
	return 1
}

func callFailure(err error) *domain.AssistantProposalError {
	var failure *domain.AssistantCallError
	if errors.As(err, &failure) {
		return &domain.AssistantProposalError{Code: failure.Code, Reason: failure.Reason}
	}
	if errors.Is(err, domain.ErrAssistantActionUnknown) {
		return &domain.AssistantProposalError{Code: "operation_unsupported"}
	}
	return &domain.AssistantProposalError{Code: "internal"}
}
