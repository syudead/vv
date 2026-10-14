package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeOrganizeStore は操作ごとに "image-<n>" の像を返し、取り消しで受け取った像を覚える。
type fakeOrganizeStore struct {
	reorders int
	undone   [][]byte
	undoErr  error
}

func (f *fakeOrganizeStore) Reorder(_ context.Context, _ int64, _ []int64, _ *int64) ([]byte, error) {
	f.reorders++
	return fmt.Appendf(nil, "image-%d", f.reorders), nil
}

func (f *fakeOrganizeStore) Undo(_ context.Context, image []byte) error {
	f.undone = append(f.undone, image)
	return f.undoErr
}

func newTestOrganizer(store *fakeOrganizeStore, clock *fakeClock) *Organizer {
	return NewOrganizer(OrganizerOptions{Store: store, Now: clock.Now})
}

func reorderToken(t *testing.T, o *Organizer) string {
	t.Helper()
	result, err := o.Reorder(context.Background(), 1, []int64{2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.UndoToken == "" || result.GroupID != 0 {
		t.Fatalf("result = %+v", result)
	}
	return result.UndoToken
}

// トークンは操作の像を保存先へ返し、2 回目の使用は ErrUndoExpired である。
func TestOrganizerTokenIsUsedOnce(t *testing.T) {
	store := &fakeOrganizeStore{}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	first, second := reorderToken(t, o), reorderToken(t, o)
	if first == second {
		t.Fatalf("tokens are equal: %q", first)
	}
	if err := o.Undo(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if len(store.undone) != 1 || !bytes.Equal(store.undone[0], []byte("image-1")) {
		t.Fatalf("undone = %q", store.undone)
	}
	if err := o.Undo(context.Background(), first); !errors.Is(err, domain.ErrUndoExpired) {
		t.Fatalf("second Undo error = %v", err)
	}
	if err := o.Undo(context.Background(), "unknown"); !errors.Is(err, domain.ErrUndoExpired) {
		t.Fatalf("unknown Undo error = %v", err)
	}
	if len(store.undone) != 1 {
		t.Fatalf("refused undos reached the store: %q", store.undone)
	}
}

// 保存先が取り消しを拒んでも、トークンは使い済みになる。
func TestOrganizerConflictUsesTheToken(t *testing.T) {
	store := &fakeOrganizeStore{undoErr: domain.ErrUndoConflict}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	token := reorderToken(t, o)
	if err := o.Undo(context.Background(), token); !errors.Is(err, domain.ErrUndoConflict) {
		t.Fatalf("Undo error = %v", err)
	}
	if err := o.Undo(context.Background(), token); !errors.Is(err, domain.ErrUndoExpired) {
		t.Fatalf("second Undo error = %v", err)
	}
}

// トークンは 10 分で切れる。10 分に満たなければ使える。
func TestOrganizerTokenExpiresAfterTenMinutes(t *testing.T) {
	store := &fakeOrganizeStore{}
	clock := &fakeClock{now: time.Unix(1000, 0)}
	o := newTestOrganizer(store, clock)
	early, late := reorderToken(t, o), reorderToken(t, o)

	clock.Advance(10*time.Minute - time.Millisecond)
	if err := o.Undo(context.Background(), early); err != nil {
		t.Fatalf("Undo before 10 minutes error = %v", err)
	}
	clock.Advance(time.Millisecond)
	if err := o.Undo(context.Background(), late); !errors.Is(err, domain.ErrUndoExpired) {
		t.Fatalf("Undo after 10 minutes error = %v", err)
	}
}

// より新しいトークンが 100 件たまると、いちばん古いトークンは拒まれ、その次は使える。
func TestOrganizerKeepsAtMostOneHundredTokens(t *testing.T) {
	store := &fakeOrganizeStore{}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	oldest, next := reorderToken(t, o), reorderToken(t, o)
	for range 99 {
		reorderToken(t, o)
	}
	if err := o.Undo(context.Background(), oldest); !errors.Is(err, domain.ErrUndoExpired) {
		t.Fatalf("Undo of the oldest error = %v", err)
	}
	if err := o.Undo(context.Background(), next); err != nil {
		t.Fatalf("Undo of the 100th newest error = %v", err)
	}
}

// 保存先の誤りはそのまま返り、トークンは作らない。
func TestOrganizerReturnsStoreErrors(t *testing.T) {
	o := NewOrganizer(OrganizerOptions{Store: failingOrganizeStore{}})
	if _, err := o.Reorder(context.Background(), 1, []int64{2}, nil); !errors.Is(err, domain.ErrGroupTargetNotFound) {
		t.Fatalf("Reorder error = %v", err)
	}
	if len(o.entries) != 0 {
		t.Fatalf("entries = %d", len(o.entries))
	}
}

type failingOrganizeStore struct{}

func (failingOrganizeStore) Reorder(context.Context, int64, []int64, *int64) ([]byte, error) {
	return nil, domain.ErrGroupTargetNotFound
}

func (failingOrganizeStore) Undo(context.Context, []byte) error { return nil }
