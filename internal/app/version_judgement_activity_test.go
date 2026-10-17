package app

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 重複の候補の判定の記録のやり直しと取り消し（specs/055-activity/contracts/screen-api.md）。

// fakeJudgementActivityStore はやり直しと取り消しを頼まれた記録を覚える。err は両方が返す誤りである。
type fakeJudgementActivityStore struct {
	retried, undone []int64
	err             error
}

func (s *fakeJudgementActivityStore) RetryJudgement(_ context.Context, id int64) error {
	s.retried = append(s.retried, id)
	return s.err
}

func (s *fakeJudgementActivityStore) UndoVersionJudgement(_ context.Context, id int64) (domain.ActivityEntry, error) {
	s.undone = append(s.undone, id)
	if s.err != nil {
		return domain.ActivityEntry{}, s.err
	}
	return domain.ActivityEntry{ID: 50, Kind: domain.ActivityUndo, UndoesID: id, Outcome: domain.ActivityCompleted}, nil
}

// countingWaker は起こされた回数を数える。
type countingWaker struct{ woken int }

func (w *countingWaker) Wake() { w.woken++ }

// 失敗した組のやり直しと自動の束ねの取り消しは、Activity の振り分けから保存層を呼び、判定のワーカーを起こす。
// 保存層が断ればワーカーを起こさない。
func TestVersionJudgementActivityRetriesAndUndoes(t *testing.T) {
	log := activityLogFixture()
	log.entries[5] = domain.ActivityEntry{ID: 5, Kind: domain.ActivityVersionJudgement, Status: domain.ActivityAttention,
		Reason: domain.ActivityFailed, Decider: domain.ActivityByRule}
	store := &fakeJudgementActivityStore{}
	waker := &countingWaker{}
	handler := NewVersionJudgementActivity(store, store, waker)
	activity := NewActivity(ActivityOptions{
		Store:    log,
		Retriers: map[domain.ActivityKind]ActivityRetrier{domain.ActivityVersionJudgement: handler},
		Undoers:  map[domain.ActivityKind]ActivityUndoer{domain.ActivityVersionJudgement: handler},
	})
	ctx := context.Background()

	if err := activity.RetryActivity(ctx, 5); err != nil {
		t.Fatalf("RetryActivity: %v", err)
	}
	undo, err := activity.UndoActivity(ctx, 3, 0)
	if err != nil {
		t.Fatalf("UndoActivity: %v", err)
	}
	if undo.UndoesID != 3 || len(store.retried) != 1 || store.retried[0] != 5 || len(store.undone) != 1 || waker.woken != 2 {
		t.Errorf("やり直し %v、取り消し %v（undo %+v）、起こした %d 回, want [5]、[3]、2 回", store.retried, store.undone,
			undo, waker.woken)
	}

	store.err = domain.ErrActivityNotRetryable
	if err := activity.RetryActivity(ctx, 5); !errors.Is(err, domain.ErrActivityNotRetryable) || waker.woken != 2 {
		t.Errorf("断られたやり直し = %v（起こした %d 回）, want ErrActivityNotRetryable で起こさない", err, waker.woken)
	}
	if _, err := activity.UndoActivity(ctx, 3, 0); !errors.Is(err, domain.ErrActivityNotRetryable) || waker.woken != 2 {
		t.Errorf("断られた取り消し = %v（起こした %d 回）, want 保存層の誤りで起こさない", err, waker.woken)
	}
}
