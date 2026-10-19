package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fakeActivityLog struct {
	entries   map[int64]domain.ActivityEntry
	recovered int
}

func (f *fakeActivityLog) ListActivity(
	context.Context, domain.Audience, domain.ActivityQuery, string, int,
) (domain.ActivityPage, error) {
	return domain.ActivityPage{}, nil
}

func (f *fakeActivityLog) ActivityEntry(_ context.Context, id int64) (domain.ActivityEntry, error) {
	entry, ok := f.entries[id]
	if !ok {
		return domain.ActivityEntry{}, domain.ErrNotFound
	}
	return entry, nil
}

func (f *fakeActivityLog) ActivityDetail(context.Context, int64) (domain.ActivityDetail, error) {
	return domain.ActivityDetail{}, nil
}

func (f *fakeActivityLog) RecoverInterruptedActivity(context.Context) (int, error) {
	return f.recovered, nil
}

type recordingRetrier struct{ retried []int64 }

func (r *recordingRetrier) RetryActivity(_ context.Context, entry domain.ActivityEntry) error {
	r.retried = append(r.retried, entry.ID)
	return nil
}

type recordingUndoer struct{ undone [][2]int64 }

func (u *recordingUndoer) UndoActivity(_ context.Context, entry domain.ActivityEntry, tagID int64) (domain.ActivityEntry, error) {
	u.undone = append(u.undone, [2]int64{entry.ID, tagID})
	return domain.ActivityEntry{ID: 100, Kind: domain.ActivityUndo, UndoesID: entry.ID}, nil
}

func activityLogFixture() *fakeActivityLog {
	failed := domain.ActivityEntry{ID: 1, Kind: domain.ActivityImport, Status: domain.ActivityAttention, Reason: domain.ActivityFailed}
	done := domain.ActivityEntry{ID: 2, Kind: domain.ActivityImport, Status: domain.ActivityDone, Outcome: domain.ActivityCompleted}
	same := domain.ActivityEntry{ID: 3, Kind: domain.ActivityVersionJudgement, Status: domain.ActivityDone,
		Outcome: domain.ActivitySame, Decider: domain.ActivityByAI}
	tags := domain.ActivityEntry{ID: 4, Kind: domain.ActivityAutoTag, Status: domain.ActivityDone, Decider: domain.ActivityByAI,
		Tags: []domain.ActivityEntryTag{{TagID: 7}, {TagID: 8, UndoneBy: 9}}}
	faces := domain.ActivityEntry{ID: 5, Kind: domain.ActivityFaceAnalysis, Status: domain.ActivityDone,
		Decider: domain.ActivityByRule, FaceLinks: []domain.ActivityFaceLink{{AppearanceID: 11}, {AppearanceID: 12, UndoneBy: 13}}}
	facesUndone := domain.ActivityEntry{ID: 6, Kind: domain.ActivityFaceAnalysis, Status: domain.ActivityDone,
		Decider: domain.ActivityByRule, FaceLinks: []domain.ActivityFaceLink{{AppearanceID: 14, UndoneBy: 15}}}
	return &fakeActivityLog{entries: map[int64]domain.ActivityEntry{
		1: failed, 2: done, 3: same, 4: tags, 5: faces, 6: facesUndone,
	}}
}

func TestActivityRetryWithoutRetriers(t *testing.T) {
	activity := NewActivity(ActivityOptions{Store: activityLogFixture()})
	if err := activity.RetryActivity(context.Background(), 1); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Errorf("振り分け先の無い import = %v, want ErrActivityNotRetryable", err)
	}
	if err := activity.RetryActivity(context.Background(), 99); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い記録 = %v, want ErrNotFound", err)
	}
	if _, err := activity.UndoActivity(context.Background(), 3, domain.ActivityUndoTarget{}); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Errorf("振り分け先の無い version_judgement = %v, want ErrActivityNotUndoable", err)
	}
}

func TestActivityDispatchesByKind(t *testing.T) {
	retrier := &recordingRetrier{}
	undoer := &recordingUndoer{}
	activity := NewActivity(ActivityOptions{
		Store:    activityLogFixture(),
		Retriers: map[domain.ActivityKind]ActivityRetrier{domain.ActivityImport: retrier},
		Undoers: map[domain.ActivityKind]ActivityUndoer{
			domain.ActivityVersionJudgement: undoer, domain.ActivityAutoTag: undoer, domain.ActivityFaceAnalysis: undoer,
		},
	})
	ctx := context.Background()
	if err := activity.RetryActivity(ctx, 1); err != nil {
		t.Fatalf("失敗した import のやり直し: %v", err)
	}
	if err := activity.RetryActivity(ctx, 2); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Errorf("done の import = %v, want ErrActivityNotRetryable", err)
	}
	if len(retrier.retried) != 1 || retrier.retried[0] != 1 {
		t.Errorf("やり直した記録 = %v, want [1]", retrier.retried)
	}

	if undo, err := activity.UndoActivity(ctx, 3, domain.ActivityUndoTarget{}); err != nil || undo.UndoesID != 3 {
		t.Fatalf("自動の束ねの取り消し = %+v, %v", undo, err)
	}
	if _, err := activity.UndoActivity(ctx, 4, domain.ActivityUndoTarget{TagID: 7}); err != nil {
		t.Fatalf("付けたタグの取り消し: %v", err)
	}
	if _, err := activity.UndoActivity(ctx, 4, domain.ActivityUndoTarget{TagID: 8}); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Errorf("取り消し済みのタグ = %v, want ErrActivityNotUndoable", err)
	}
	if _, err := activity.UndoActivity(ctx, 4, domain.ActivityUndoTarget{TagID: 5}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("付けていないタグ = %v, want ErrNotFound", err)
	}
	if _, err := activity.UndoActivity(ctx, 2, domain.ActivityUndoTarget{}); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Errorf("import の取り消し = %v, want ErrActivityNotUndoable", err)
	}
	if _, err := activity.UndoActivity(ctx, 5, domain.ActivityUndoTarget{AppearanceID: 11}); err != nil {
		t.Fatalf("顔の紐付けの取り消し: %v", err)
	}
	// A tagId on a face_analysis entry is not an appearance: the request undoes every link.
	if _, err := activity.UndoActivity(ctx, 5, domain.ActivityUndoTarget{TagID: 11}); err != nil {
		t.Fatalf("顔の紐付けをすべて取り消す: %v", err)
	}
	if _, err := activity.UndoActivity(ctx, 5, domain.ActivityUndoTarget{AppearanceID: 12}); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Errorf("取り消し済みの紐付け = %v, want ErrActivityNotUndoable", err)
	}
	if _, err := activity.UndoActivity(ctx, 5, domain.ActivityUndoTarget{AppearanceID: 99}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い紐付け = %v, want ErrNotFound", err)
	}
	if _, err := activity.UndoActivity(ctx, 6, domain.ActivityUndoTarget{}); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Errorf("すべて取り消し済みの顔の解析 = %v, want ErrActivityNotUndoable", err)
	}
	if want := [][2]int64{{3, 0}, {4, 7}, {5, 11}, {5, 0}}; !slices.Equal(undoer.undone, want) {
		t.Errorf("取り消した記録 = %v, want %v", undoer.undone, want)
	}
}

func TestActivityRecoverInterrupted(t *testing.T) {
	store := activityLogFixture()
	store.recovered = 2
	if recovered, err := NewActivity(ActivityOptions{Store: store}).RecoverInterrupted(context.Background()); err != nil || recovered != 2 {
		t.Errorf("RecoverInterrupted = %d, %v; want 2", recovered, err)
	}
}
