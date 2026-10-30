package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// AssistantStore persists the assistant's proposals; each transition is guarded by the previous status and writes
// the proposal's assistant_run entry in the same transaction.
type AssistantStore interface {
	// CreateProposal stores the plans as the proposal's pending steps, in card order; turnID is 0 without a turn.
	CreateProposal(ctx context.Context, turnID int64, request string, plans []domain.AssistantPlan, now time.Time) (domain.AssistantProposal, error)
	// Proposal returns domain.ErrNotFound for a missing proposal.
	Proposal(ctx context.Context, id int64) (domain.AssistantProposal, error)
	ActiveProposals(ctx context.Context, since time.Time) ([]domain.AssistantProposal, error)
	// DiscardProposal returns domain.ErrNotFound for a missing proposal and domain.ErrAssistantProposalNotPending
	// for any other status than proposed.
	DiscardProposal(ctx context.Context, id int64, now time.Time) error
	// ApproveProposal keeps the steps at the given indexes and skips the others. It returns domain.ErrNotFound for
	// a missing or discarded proposal, the proposal with domain.ErrAssistantProposalNotPending for any other status
	// than proposed, and the errors of domain.AssistantProposal.Select.
	ApproveProposal(ctx context.Context, id int64, kept []int, now time.Time) (domain.AssistantProposal, error)
	// ClaimProposal returns domain.ErrNoJob when no approved proposal waits.
	ClaimProposal(ctx context.Context, now time.Time) (domain.AssistantProposal, error)
	// RecordProgress writes a running proposal's steps as they stand.
	RecordProgress(ctx context.Context, id int64, steps []domain.AssistantStep, now time.Time) error
	FinishProposal(ctx context.Context, id int64, result domain.AssistantRunResult, now time.Time) error
	ExpireProposals(ctx context.Context, before time.Time) (int, error)
	RecoverProposals(ctx context.Context, now time.Time) (int, error)
	CurrentLabels(ctx context.Context, plan domain.AssistantPlan) (domain.AssistantCurrent, error)
	VideoIDs(ctx context.Context, keys []string) (map[string]int64, error)
	MergeImpact(ctx context.Context, target domain.AssistantTarget, sourceIDs []int64) (int, error)
}

type AssistantOptions struct {
	Store AssistantStore
	// Turns, Chat and Vocabulary are required by turns; Videos by names that resolve to videos, SmartFolders by
	// targets that expand smart folders and Groups by favorites on selected groups.
	Turns        AssistantTurnStore
	Chat         ToolChat
	Vocabulary   AssistantVocabulary
	Videos       AssistantVideos
	SmartFolders AssistantSmartFolders
	Groups       AssistantGroups
	// Reviews and Classifier are required by review runs; without Reviews an organize request is unsupported.
	Reviews    AssistantReviewStore
	Classifier Classifier
	// Nil uses slog.Default.
	Logger *slog.Logger
	// Nil uses time.Now.
	Now func() time.Time
}

// Assistant runs requests as server-side tool loops over the generation model, stores write proposals and executes
// the approved ones, one at a time, through the screen API's own operations, in batches of user keys.
type Assistant struct {
	store        AssistantStore
	turns        AssistantTurnStore
	chat         ToolChat
	vocabulary   AssistantVocabulary
	videos       AssistantVideos
	smartFolders AssistantSmartFolders
	groups       AssistantGroups
	reviews      AssistantReviewStore
	classifier   Classifier
	logger       *slog.Logger
	now          func() time.Time
	wake         chan struct{}
	reviewWake   chan struct{}
	turnWake     chan struct{}
	// editPoll is how often a thumbnail edit step reads its session while ComfyUI runs.
	editPoll time.Duration

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
		store: opts.Store, turns: opts.Turns, chat: opts.Chat, vocabulary: opts.Vocabulary, videos: opts.Videos,
		smartFolders: opts.SmartFolders, groups: opts.Groups, reviews: opts.Reviews, classifier: opts.Classifier,
		logger: logger.With(slog.String("component", "assistant")), now: now,
		// Coalesce wakeups in a one-element buffer.
		wake: make(chan struct{}, 1), reviewWake: make(chan struct{}, 1), turnWake: make(chan struct{}, 1),
		editPoll: 2 * time.Second,
	}
}

// UseRunner registers the runner once the HTTP server that implements it exists, and wakes the workers.
func (a *Assistant) UseRunner(runner ActionRunner) {
	a.runnerMu.Lock()
	a.runner = runner
	a.runnerMu.Unlock()
	a.Wake()
	a.wakeTurns()
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

// Propose stores resolved plans as the steps of one card, in order. A merge also stores the number of videos it
// moves; no other operation carries an impact.
func (a *Assistant) Propose(
	ctx context.Context, turnID int64, request string, plans []domain.AssistantPlan,
) (domain.AssistantProposal, error) {
	steps := make([]domain.AssistantPlan, len(plans))
	for i, plan := range plans {
		var err error
		if steps[i], err = a.withImpact(ctx, plan); err != nil {
			return domain.AssistantProposal{}, err
		}
	}
	return a.store.CreateProposal(ctx, turnID, request, steps, a.now())
}

// withImpact checks the plan's operation and sets the number of videos a merge moves.
func (a *Assistant) withImpact(ctx context.Context, plan domain.AssistantPlan) (domain.AssistantPlan, error) {
	if _, ok := plan.Action(); !ok {
		return plan, fmt.Errorf("%w: %q", domain.ErrAssistantActionUnknown, plan.ActionID)
	}
	plan.Impact = nil
	if target, sources, ok := plan.MergeSources(); ok {
		videos, err := a.store.MergeImpact(ctx, target, sources)
		if err != nil {
			return plan, err
		}
		plan.Impact = &domain.AssistantImpact{Videos: videos}
	}
	return plan, nil
}

// Approve moves the proposal to approved with the kept steps and wakes the worker; see
// AssistantStore.ApproveProposal for the errors.
func (a *Assistant) Approve(ctx context.Context, id int64, kept []int) (domain.AssistantProposal, error) {
	p, err := a.store.ApproveProposal(ctx, id, kept, a.now())
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

// Recover must run before Run, RunReviews and RunTurns: proposals whose thread is gone are deleted, proposals a
// restart stopped fail, and review runs and turns it stopped are interrupted.
func (a *Assistant) Recover(ctx context.Context) error {
	if err := a.recoverReviews(ctx); err != nil {
		return err
	}
	if err := a.recoverTurns(ctx); err != nil {
		return err
	}
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

// execute runs the kept steps in card order and stops at the first step that ends stale or with a failed batch;
// the kept steps after it do not run.
func (a *Assistant) execute(ctx context.Context, runner ActionRunner, p domain.AssistantProposal) {
	steps, err := a.run(ctx, runner, p)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		a.logger.Error("the assistant proposal failed", slog.Int64("proposal", p.ID), slog.Any("error", err))
		steps = failRunning(steps, &domain.AssistantProposalError{Code: "internal"})
	}
	result := domain.NewAssistantRunResult(steps)
	if err := a.store.FinishProposal(ctx, p.ID, result, a.now()); err != nil && ctx.Err() == nil {
		a.logger.Error("cannot finish the assistant proposal", slog.Int64("proposal", p.ID), slog.Any("error", err))
	}
}

// failRunning ends the running step with the error after a storage failure; a step that applied part of its
// batches is partial.
func failRunning(steps []domain.AssistantStep, failure *domain.AssistantProposalError) []domain.AssistantStep {
	for i := range steps {
		if steps[i].Status != domain.AssistantStepRunning {
			continue
		}
		steps[i].Status, steps[i].Error = domain.AssistantStepFailed, failure
		if steps[i].Applied > 0 {
			steps[i].Status = domain.AssistantStepPartial
		}
		domain.SkipAfter(steps, i)
	}
	return steps
}

// run returns an error only for a storage failure; a failed operation call is part of the steps. The ids that
// steps create are kept for the later steps that name them.
func (a *Assistant) run(ctx context.Context, runner ActionRunner, p domain.AssistantProposal) ([]domain.AssistantStep, error) {
	steps := slices.Clone(p.Steps)
	created := map[int]int64{}
	for i := range steps {
		if steps[i].Status != domain.AssistantStepPending {
			continue
		}
		if err := steps[i].Move(domain.AssistantStepRunning); err != nil {
			return steps, err
		}
		if err := a.store.RecordProgress(ctx, p.ID, steps, a.now()); err != nil {
			return steps, err
		}
		id, err := a.runStep(ctx, runner, p.ID, steps, i, created)
		if err != nil {
			return steps, err
		}
		if id != 0 {
			created[i] = id
		}
		if steps[i].Stops() {
			domain.SkipAfter(steps, i)
			break
		}
	}
	return steps, nil
}

// runStep re-reads step i's labels before its first call, except those an earlier kept step acts on, then runs
// its batches in order and stops at the first failed one; a video gone between batches counts as failed. It
// returns the id the step's response carries, for a step that creates something.
func (a *Assistant) runStep(
	ctx context.Context, runner ActionRunner, proposalID int64, steps []domain.AssistantStep, i int,
	created map[int]int64,
) (int64, error) {
	step := &steps[i]
	plan := step.Plan
	current, err := a.store.CurrentLabels(ctx, plan)
	if err != nil {
		return 0, err
	}
	if plan.Stale(current, domain.TouchedBefore(steps, i)) {
		step.Status = domain.AssistantStepStale
		return 0, nil
	}
	var id int64
	for batch, keys := range plan.Batches() {
		ids, missing, err := a.batchVideos(ctx, batch, keys, current)
		if err != nil {
			return 0, err
		}
		step.Failed += missing
		if len(keys) == 0 || len(ids) > 0 {
			call, ok := plan.WithStepIDs(plan.CallFor(ids), created)
			if !ok {
				endStep(step, batch, batchCount(plan, ids), &domain.AssistantProposalError{Code: "internal"})
				return 0, nil
			}
			response, err := a.runCall(ctx, runner, call)
			if err != nil {
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				endStep(step, batch, batchCount(plan, ids), callFailure(err))
				return 0, nil
			}
			step.Applied += batchCount(plan, ids)
			id = createdID(response.Body)
		}
		if err := a.store.RecordProgress(ctx, proposalID, steps, a.now()); err != nil {
			return 0, err
		}
	}
	step.Status = domain.AssistantStepDone
	if step.Failed > 0 {
		step.Status = domain.AssistantStepPartial
	}
	return id, nil
}

// endStep ends a step at a failed batch: partial when an earlier batch applied, otherwise failed.
func endStep(step *domain.AssistantStep, batch, count int, failure *domain.AssistantProposalError) {
	step.Failed += count
	step.Error = failure
	step.Status = domain.AssistantStepPartial
	if batch == 0 && step.Applied == 0 {
		step.Status = domain.AssistantStepFailed
	}
}

// createdID reads the id of what a call created from its response body; 0 when the body carries none.
func createdID(body []byte) int64 {
	var created struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(body, &created) != nil {
		return 0
	}
	return created.ID
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
		return &domain.AssistantProposalError{Code: failure.Code, Reason: failure.Reason, Message: failure.Message}
	}
	if errors.Is(err, domain.ErrAssistantActionUnknown) {
		return &domain.AssistantProposalError{Code: "operation_unsupported"}
	}
	return &domain.AssistantProposalError{Code: "internal"}
}
