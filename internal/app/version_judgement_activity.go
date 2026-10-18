package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// Activity dispatch writes retry/undo atomically before waking the judgement worker.

type VersionJudgementRetryStore interface {
	// Set retry_at to now and return the activity to waiting, or ErrActivityNotRetryable if no longer
	// eligible.
	RetryJudgement(ctx context.Context, id int64) error
}

type VersionJudgementUndoStore interface {
	UndoVersionJudgement(ctx context.Context, id int64) (domain.ActivityEntry, error)
}

type judgeWaker interface {
	Wake()
}

type VersionJudgementActivity struct {
	retry VersionJudgementRetryStore
	undo  VersionJudgementUndoStore
	judge judgeWaker
}

// Wake only after persistence.
func NewVersionJudgementActivity(
	retry VersionJudgementRetryStore, undo VersionJudgementUndoStore, judge judgeWaker,
) *VersionJudgementActivity {
	return &VersionJudgementActivity{retry: retry, undo: undo, judge: judge}
}

// RetryActivity reuses the activity record for the new result and wakes the worker immediately.
func (a *VersionJudgementActivity) RetryActivity(ctx context.Context, entry domain.ActivityEntry) error {
	if err := a.retry.RetryJudgement(ctx, entry.ID); err != nil {
		return err
	}
	a.judge.Wake()
	return nil
}

// Undo may restore candidate pairs, so wake the worker afterward; held/excluded pairs remain
// ineligible and tagID is unused.
func (a *VersionJudgementActivity) UndoActivity(
	ctx context.Context, entry domain.ActivityEntry, _ int64,
) (domain.ActivityEntry, error) {
	undo, err := a.undo.UndoVersionJudgement(ctx, entry.ID)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	a.judge.Wake()
	return undo, nil
}
