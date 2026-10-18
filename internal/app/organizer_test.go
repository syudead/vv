package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Use deterministic image-n snapshots, optionally padded to exercise byte limits.
type fakeOrganizeStore struct {
	reorders  int
	undone    [][]byte
	undoErr   error
	imageSize int

	groups        int
	renames       []renameCall
	renameErr     error
	staleRename   bool
	renameEntered chan struct{}
	renameRelease chan struct{}
}

func (f *fakeOrganizeStore) Reorder(_ context.Context, _ int64, _ []int64, _ *int64) ([]byte, error) {
	f.reorders++
	image := fmt.Appendf(nil, "image-%d", f.reorders)
	if f.imageSize > len(image) {
		image = append(image, make([]byte, f.imageSize-len(image))...)
	}
	return image, nil
}

func (f *fakeOrganizeStore) Move(ctx context.Context, _ domain.OrganizeMove) ([]byte, error) {
	return f.Reorder(ctx, 0, nil, nil)
}

func (f *fakeOrganizeStore) Bundle(ctx context.Context, _ []int64, _ int64) ([]byte, error) {
	return f.Reorder(ctx, 0, nil, nil)
}

func (f *fakeOrganizeStore) Merge(_ context.Context, items []domain.OrganizeRef, into, from int64) ([]byte, error) {
	return fmt.Appendf(nil, "merge-%d-%d-%d", len(items), into, from), nil
}

func (f *fakeOrganizeStore) Undo(_ context.Context, image []byte) error {
	f.undone = append(f.undone, image)
	return f.undoErr
}

func (f *fakeOrganizeStore) Group(_ context.Context, _ []int64, _ int64, _ *int64) (int64, []byte, error) {
	f.groups++
	return 99 + int64(f.groups), fmt.Appendf(nil, "group-%d", f.groups), nil
}

// Optional gates pause rename; staleRename prevents snapshot refresh.
func (f *fakeOrganizeStore) RenameGroup(_ context.Context, id int64, name string, image []byte) (domain.Group, []byte, error) {
	f.renames = append(f.renames, renameCall{id: id, name: name, image: image})
	if f.renameEntered != nil {
		f.renameEntered <- struct{}{}
		<-f.renameRelease
	}
	if f.renameErr != nil {
		return domain.Group{}, nil, f.renameErr
	}
	group := domain.Group{ID: id, Name: name}
	if image == nil || f.staleRename {
		return group, nil, nil
	}
	return group, fmt.Appendf(nil, "renamed-%d", len(f.renames)), nil
}

type renameCall struct {
	id    int64
	name  string
	image []byte
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

func TestOrganizerKeepsTheTokenWhenTheUndoFails(t *testing.T) {
	store := &fakeOrganizeStore{undoErr: context.Canceled}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	token := reorderToken(t, o)
	if err := o.Undo(context.Background(), token); !errors.Is(err, context.Canceled) {
		t.Fatalf("Undo error = %v", err)
	}
	store.undoErr = nil
	if err := o.Undo(context.Background(), token); err != nil {
		t.Fatalf("retried Undo error = %v", err)
	}
	if err := o.Undo(context.Background(), token); !errors.Is(err, domain.ErrUndoExpired) {
		t.Fatalf("third Undo error = %v", err)
	}
}

func TestOrganizerCreatesTheTokenBeforeTheStoreWrites(t *testing.T) {
	store := &fakeOrganizeStore{}
	o := NewOrganizer(OrganizerOptions{Store: store, Random: bytes.NewReader(nil)})
	if _, err := o.Reorder(context.Background(), 1, []int64{2}, nil); err == nil {
		t.Fatal("Reorder succeeded without randomness")
	}
	if store.reorders != 0 || len(o.entries) != 0 {
		t.Fatalf("reorders = %d, entries = %d", store.reorders, len(o.entries))
	}
}

func TestOrganizerBoundsTheJournalInBytes(t *testing.T) {
	store := &fakeOrganizeStore{imageSize: undoMaxBytes/2 + 1}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	oldest, newest := reorderToken(t, o), reorderToken(t, o)
	if err := o.Undo(context.Background(), oldest); !errors.Is(err, domain.ErrUndoExpired) {
		t.Fatalf("Undo of the oldest error = %v", err)
	}
	if o.bytes != undoMaxBytes/2+1 {
		t.Fatalf("bytes = %d", o.bytes)
	}
	if err := o.Undo(context.Background(), newest); err != nil {
		t.Fatalf("Undo of the newest error = %v", err)
	}
	if o.bytes != 0 {
		t.Fatalf("bytes after the undo = %d", o.bytes)
	}

	store.imageSize = undoMaxBytes + 1
	if err := o.Undo(context.Background(), reorderToken(t, o)); err != nil {
		t.Fatalf("Undo of an image over the limit error = %v", err)
	}
}

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

func TestOrganizerReturnsStoreErrors(t *testing.T) {
	o := NewOrganizer(OrganizerOptions{Store: failingOrganizeStore{}})
	if _, err := o.Reorder(context.Background(), 1, []int64{2}, nil); !errors.Is(err, domain.ErrGroupTargetNotFound) {
		t.Fatalf("Reorder error = %v", err)
	}
	if _, err := o.Group(context.Background(), []int64{2}, 1, nil); !errors.Is(err, domain.ErrGroupTargetNotFound) {
		t.Fatalf("Group error = %v", err)
	}
	if _, err := o.Move(context.Background(), domain.OrganizeMove{To: 1}); !errors.Is(err, domain.ErrGroupCycle) {
		t.Fatalf("Move error = %v", err)
	}
	if _, err := o.Bundle(context.Background(), []int64{1}, 2); !errors.Is(err, domain.ErrTooManyVersions) {
		t.Fatalf("Bundle error = %v", err)
	}
	if _, err := o.Merge(context.Background(), nil, 1, 0); !errors.Is(err, domain.ErrGroupCycle) {
		t.Fatalf("Merge error = %v", err)
	}
	if len(o.entries) != 0 {
		t.Fatalf("entries = %d", len(o.entries))
	}
}

type failingOrganizeStore struct{}

func (failingOrganizeStore) Reorder(context.Context, int64, []int64, *int64) ([]byte, error) {
	return nil, domain.ErrGroupTargetNotFound
}

func (failingOrganizeStore) Move(context.Context, domain.OrganizeMove) ([]byte, error) {
	return nil, domain.ErrGroupCycle
}

func (failingOrganizeStore) Bundle(context.Context, []int64, int64) ([]byte, error) {
	return nil, domain.ErrTooManyVersions
}

func (failingOrganizeStore) Merge(context.Context, []domain.OrganizeRef, int64, int64) ([]byte, error) {
	return nil, domain.ErrGroupCycle
}

func (failingOrganizeStore) Undo(context.Context, []byte) error { return nil }

func TestOrganizerMergeKeepsItsImage(t *testing.T) {
	store := &fakeOrganizeStore{}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	items := []domain.OrganizeRef{{Kind: domain.OrganizeVideo, ID: 3}, {Kind: domain.OrganizeGroup, ID: 4}}
	result, err := o.Merge(context.Background(), items, 9, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Undo(context.Background(), result.UndoToken); err != nil {
		t.Fatal(err)
	}
	if len(store.undone) != 1 || !bytes.Equal(store.undone[0], []byte("merge-2-9-7")) {
		t.Fatalf("undone = %q", store.undone)
	}
}

func (failingOrganizeStore) Group(context.Context, []int64, int64, *int64) (int64, []byte, error) {
	return 0, nil, domain.ErrGroupTargetNotFound
}

func (failingOrganizeStore) RenameGroup(context.Context, int64, string, []byte) (domain.Group, []byte, error) {
	return domain.Group{}, nil, domain.ErrGroupNotFound
}

func groupToken(t *testing.T, o *Organizer) domain.OrganizeResult {
	t.Helper()
	result, err := o.Group(context.Background(), []int64{2}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.UndoToken == "" || result.GroupID == 0 {
		t.Fatalf("result = %+v", result)
	}
	return result
}

func renameGroup(t *testing.T, o *Organizer, id int64, token string) {
	t.Helper()
	group, err := o.RenameGroup(context.Background(), id, "Renamed", token)
	if err != nil {
		t.Fatal(err)
	}
	if group.ID != id || group.Name != "Renamed" {
		t.Fatalf("group = %+v", group)
	}
}

func TestOrganizerGroupReturnsTheNewGroup(t *testing.T) {
	store := &fakeOrganizeStore{}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	result := groupToken(t, o)
	if result.GroupID != 100 {
		t.Fatalf("GroupID = %d", result.GroupID)
	}
	if err := o.Undo(context.Background(), result.UndoToken); err != nil {
		t.Fatal(err)
	}
	if len(store.undone) != 1 || string(store.undone[0]) != "group-1" {
		t.Fatalf("undone = %q", store.undone)
	}
}

// Keep the old snapshot when storage cannot refresh it; either snapshot remains undoable.
func TestOrganizerRenameWithTheTokenReplacesTheImage(t *testing.T) {
	store := &fakeOrganizeStore{}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	result := groupToken(t, o)
	renameGroup(t, o, result.GroupID, result.UndoToken)
	if len(store.renames) != 1 || string(store.renames[0].image) != "group-1" {
		t.Fatalf("renames = %+v", store.renames)
	}
	if err := o.Undo(context.Background(), result.UndoToken); err != nil {
		t.Fatal(err)
	}
	if string(store.undone[0]) != "renamed-1" {
		t.Fatalf("undone = %q", store.undone)
	}

	store.staleRename = true
	second := groupToken(t, o)
	renameGroup(t, o, second.GroupID, second.UndoToken)
	if err := o.Undo(context.Background(), second.UndoToken); err != nil {
		t.Fatal(err)
	}
	if string(store.undone[1]) != "group-2" {
		t.Fatalf("undone = %q", store.undone)
	}
}

func TestOrganizerRenameFailureKeepsTheToken(t *testing.T) {
	store := &fakeOrganizeStore{renameErr: domain.ErrGroupNotFound}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	result := groupToken(t, o)
	if _, err := o.RenameGroup(context.Background(), result.GroupID, "Renamed", result.UndoToken); !errors.Is(err, domain.ErrGroupNotFound) {
		t.Fatalf("RenameGroup error = %v", err)
	}
	if err := o.Undo(context.Background(), result.UndoToken); err != nil {
		t.Fatal(err)
	}
	if string(store.undone[0]) != "group-1" {
		t.Fatalf("undone = %q", store.undone)
	}
}

func TestOrganizerRenameWithOtherTokensKeepsTheImage(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	store := &fakeOrganizeStore{}
	o := newTestOrganizer(store, clock)
	expired := groupToken(t, o)
	clock.Advance(10 * time.Minute)
	used := groupToken(t, o)
	if err := o.Undo(context.Background(), used.UndoToken); err != nil {
		t.Fatal(err)
	}
	reorder := reorderToken(t, o)
	other := groupToken(t, o)
	target := groupToken(t, o)

	for _, c := range []struct {
		name  string
		id    int64
		token string
	}{
		{"知らないトークン", target.GroupID, "unknown"},
		{"使い済みのトークン", used.GroupID, used.UndoToken},
		{"切れたトークン", expired.GroupID, expired.UndoToken},
		{"並べ替えのトークン", target.GroupID, reorder},
		{"別のグループのトークン", target.GroupID, other.UndoToken},
	} {
		store.renames = nil
		renameGroup(t, o, c.id, c.token)
		if len(store.renames) != 1 || store.renames[0].image != nil {
			t.Errorf("%s: renames = %+v", c.name, store.renames)
		}
	}
	store.undone = nil
	for _, token := range []string{reorder, other.UndoToken} {
		if err := o.Undo(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	if len(store.undone) != 2 || string(store.undone[0]) != "image-1" || string(store.undone[1]) != "group-3" {
		t.Fatalf("undone = %q", store.undone)
	}
}

func TestOrganizerUndoWaitsForTheRename(t *testing.T) {
	store := &fakeOrganizeStore{renameEntered: make(chan struct{}), renameRelease: make(chan struct{})}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	result := groupToken(t, o)

	renamed := make(chan error, 1)
	go func() {
		_, err := o.RenameGroup(context.Background(), result.GroupID, "Renamed", result.UndoToken)
		renamed <- err
	}()
	<-store.renameEntered
	undone := make(chan error, 1)
	go func() { undone <- o.Undo(context.Background(), result.UndoToken) }()
	waitForWaiters(t, o, 1)
	close(store.renameRelease)
	if err := <-renamed; err != nil {
		t.Fatal(err)
	}
	if err := <-undone; err != nil {
		t.Fatal(err)
	}
	if len(store.undone) != 1 || string(store.undone[0]) != "renamed-1" {
		t.Fatalf("undone = %q", store.undone)
	}
}

func TestOrganizerUndoStopsWaitingWhenCanceled(t *testing.T) {
	store := &fakeOrganizeStore{renameEntered: make(chan struct{}), renameRelease: make(chan struct{})}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	result := groupToken(t, o)
	renamed := make(chan error, 1)
	go func() {
		_, err := o.RenameGroup(context.Background(), result.GroupID, "Renamed", result.UndoToken)
		renamed <- err
	}()
	<-store.renameEntered
	ctx, cancel := context.WithCancel(context.Background())
	undone := make(chan error, 1)
	go func() { undone <- o.Undo(ctx, result.UndoToken) }()
	waitForWaiters(t, o, 1)
	cancel()
	if err := <-undone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Undo error = %v", err)
	}
	close(store.renameRelease)
	if err := <-renamed; err != nil {
		t.Fatal(err)
	}
	if err := o.Undo(context.Background(), result.UndoToken); err != nil {
		t.Fatal(err)
	}
}

func waitForWaiters(t *testing.T, o *Organizer, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		o.mu.Lock()
		waiting := o.waiters
		o.mu.Unlock()
		if waiting >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiters = %d, want %d", waiting, n)
		}
		runtime.Gosched()
	}
}

func TestOrganizerConcurrentRenamesBothAdvanceTheImage(t *testing.T) {
	store := &fakeOrganizeStore{renameEntered: make(chan struct{}), renameRelease: make(chan struct{})}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	result := groupToken(t, o)

	first := make(chan error, 1)
	go func() {
		_, err := o.RenameGroup(context.Background(), result.GroupID, "Renamed", result.UndoToken)
		first <- err
	}()
	<-store.renameEntered
	second := make(chan error, 1)
	go func() {
		_, err := o.RenameGroup(context.Background(), result.GroupID, "Renamed", result.UndoToken)
		second <- err
	}()
	waitForWaiters(t, o, 1)
	close(store.renameRelease)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	<-store.renameEntered
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if len(store.renames) != 2 || string(store.renames[1].image) != "renamed-1" {
		t.Fatalf("renames = %+v", store.renames)
	}
	if err := o.Undo(context.Background(), result.UndoToken); err != nil {
		t.Fatal(err)
	}
	if len(store.undone) != 1 || string(store.undone[0]) != "renamed-2" {
		t.Fatalf("undone = %q", store.undone)
	}
}

// Advance once by step on the next clock read.
type steppingClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func (c *steppingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now
	c.now = c.now.Add(c.step)
	c.step = 0
	return now
}

// Tokens expiring during rename are discarded on release, without leaving undo blocked.
func TestOrganizerRenameAtTheExpiryHoldsTheEntry(t *testing.T) {
	clock := &steppingClock{now: time.Unix(1000, 0)}
	store := &fakeOrganizeStore{}
	o := NewOrganizer(OrganizerOptions{Store: store, Now: clock.Now})
	result := groupToken(t, o)
	clock.mu.Lock()
	clock.now = clock.now.Add(undoLifetime - time.Nanosecond)
	clock.step = time.Nanosecond
	clock.mu.Unlock()
	renameGroup(t, o, result.GroupID, result.UndoToken)
	if len(store.renames) != 1 || string(store.renames[0].image) != "group-1" {
		t.Fatalf("renames = %+v", store.renames)
	}
	if len(o.held) != 0 {
		t.Fatalf("held = %v", o.held)
	}
	if err := o.Undo(context.Background(), result.UndoToken); !errors.Is(err, domain.ErrUndoExpired) {
		t.Fatalf("Undo error = %v", err)
	}
}

func TestOrganizerMoveKeepsItsImage(t *testing.T) {
	store := &fakeOrganizeStore{}
	o := newTestOrganizer(store, &fakeClock{now: time.Unix(1000, 0)})
	result, err := o.Move(context.Background(), domain.OrganizeMove{Items: []domain.OrganizeRef{{Kind: domain.OrganizeVideo, ID: 1}}, To: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.UndoToken == "" || result.GroupID != 0 {
		t.Fatalf("result = %+v", result)
	}
	if err := o.Undo(context.Background(), result.UndoToken); err != nil {
		t.Fatal(err)
	}
	if len(store.undone) != 1 || !bytes.Equal(store.undone[0], []byte("image-1")) {
		t.Fatalf("undone = %q", store.undone)
	}
}
