package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Storage applies edits atomically and returns opaque before/after snapshots; Organizer retains
// them under random in-memory tokens without interpreting them.

const (
	undoLifetime = 10 * time.Minute
	// Evict oldest tokens above capacity.
	undoCapacity = 100
	// Bound snapshot bytes as well as count because large groups contain two full images; retain the
	// newest snapshot even when it alone exceeds the cap so its operation remains undoable.
	undoMaxBytes   = 64 << 20
	undoTokenBytes = 16
)

type OrganizeStore interface {
	// Preserve item order and use nil afterItemID for the beginning; missing targets return
	// ErrGroupTargetNotFound.
	Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) ([]byte, error)
	// Place a new group at targetID's position within fromID when supplied; missing targets/membership
	// return ErrGroupTargetNotFound.
	Group(ctx context.Context, videoIDs []int64, targetID int64, fromID *int64) (int64, []byte, error)
	// Update a grouping undo image only if the pre-rename rows still match its after-image; otherwise
	// return no image.
	RenameGroup(ctx context.Context, id int64, name string, image []byte) (domain.Group, []byte, error)
	// Move validates source membership and rejects cyclic group nesting.
	Move(ctx context.Context, move domain.OrganizeMove) ([]byte, error)
	// Bundle uses targetID as representative and rejects missing videos or too many expanded versions.
	Bundle(ctx context.Context, videoIDs []int64, targetID int64) ([]byte, error)
	// Merge follows screen order, appending videos and merging groups while atomically removing source
	// cards; reject missing membership and merges into self/descendants.
	Merge(ctx context.Context, items []domain.OrganizeRef, into, from int64) ([]byte, error)

	Undo(ctx context.Context, image []byte) error
}

type OrganizerOptions struct {
	Store OrganizeStore
	// Nil uses time.Now.
	Now func() time.Time
	// Nil uses crypto/rand.Reader.
	Random io.Reader
}

// Organizer stores single-use undo tokens for ten minutes, bounded by count and snapshot bytes.
type Organizer struct {
	store  OrganizeStore
	now    func() time.Time
	random io.Reader

	mu sync.Mutex
	// Tokens remain in creation order.
	entries []undoEntry
	// Total bytes retained across snapshots.
	bytes int
	// Rename temporarily holds tokens outside entries; closing the held channel wakes undo waiters
	// when restored.
	held map[string]chan struct{}
	// Expose waiter counts for synchronization in tests.
	waiters int
}

type undoEntry struct {
	token   string
	image   []byte
	created time.Time
	// Nonzero only for tokens creating a group.
	groupID int64
}

func NewOrganizer(opts OrganizerOptions) *Organizer {
	o := &Organizer{store: opts.Store, now: opts.Now, random: opts.Random, held: map[string]chan struct{}{}}
	if o.now == nil {
		o.now = time.Now
	}
	if o.random == nil {
		o.random = rand.Reader
	}
	return o
}

// Create tokens before mutations so random-source failure cannot leave a committed operation
// without undo.
func (o *Organizer) Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) (domain.OrganizeResult, error) {
	return o.run(func() ([]byte, error) { return o.store.Reorder(ctx, groupID, itemIDs, afterItemID) })
}

func (o *Organizer) Move(ctx context.Context, move domain.OrganizeMove) (domain.OrganizeResult, error) {
	return o.run(func() ([]byte, error) { return o.store.Move(ctx, move) })
}

func (o *Organizer) Bundle(ctx context.Context, videoIDs []int64, targetID int64) (domain.OrganizeResult, error) {
	return o.run(func() ([]byte, error) { return o.store.Bundle(ctx, videoIDs, targetID) })
}

// Create the token before storage mutation, as Reorder does.
func (o *Organizer) Merge(ctx context.Context, items []domain.OrganizeRef, into, from int64) (domain.OrganizeResult, error) {
	return o.run(func() ([]byte, error) { return o.store.Merge(ctx, items, into, from) })
}

func (o *Organizer) run(operation func() ([]byte, error)) (domain.OrganizeResult, error) {
	token, err := o.newToken()
	if err != nil {
		return domain.OrganizeResult{}, err
	}
	image, err := operation()
	if err != nil {
		return domain.OrganizeResult{}, err
	}
	o.keep(undoEntry{token: token, image: image, created: o.now()})
	return domain.OrganizeResult{UndoToken: token}, nil
}

// Create the token before storage mutation, as Reorder does.
func (o *Organizer) Group(ctx context.Context, videoIDs []int64, targetID int64, fromID *int64) (domain.OrganizeResult, error) {
	token, err := o.newToken()
	if err != nil {
		return domain.OrganizeResult{}, err
	}
	groupID, image, err := o.store.Group(ctx, videoIDs, targetID, fromID)
	if err != nil {
		return domain.OrganizeResult{}, err
	}
	o.keep(undoEntry{token: token, image: image, created: o.now(), groupID: groupID})
	return domain.OrganizeResult{UndoToken: token, GroupID: groupID}, nil
}

// RenameGroup serializes use of a live matching grouping token and refreshes its snapshot without
// extending expiry; absent or inapplicable tokens never prevent ordinary renaming.
func (o *Organizer) RenameGroup(ctx context.Context, id int64, name, token string) (domain.Group, error) {
	entry, ok := o.hold(ctx, token, id)
	if !ok {
		group, _, err := o.store.RenameGroup(ctx, id, name, nil)
		return group, err
	}
	group, image, err := o.store.RenameGroup(ctx, id, name, entry.image)
	if err == nil && image != nil {
		entry.image = image
	}
	o.release(entry)
	return group, err
}

// Undo consumes successful/conflicting tokens, restores tokens after other storage failures, and
// waits cancelably for rename-held tokens. Missing, expired, or concurrently consumed tokens return
// ErrUndoExpired.
func (o *Organizer) Undo(ctx context.Context, token string) error {
	entry, ok, err := o.takeWhenReleased(ctx, token)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrUndoExpired
	}
	err = o.store.Undo(ctx, entry.image)
	if err != nil && !errors.Is(err, domain.ErrUndoConflict) {
		o.keep(entry)
	}
	return err
}

func (o *Organizer) newToken() (string, error) {
	raw := make([]byte, undoTokenBytes)
	if _, err := io.ReadFull(o.random, raw); err != nil {
		return "", fmt.Errorf("cannot create an undo token: %w", err)
	}
	return sessionTokenEncoding.EncodeToString(raw), nil
}

// Evict expired or oldest over-budget entries while preserving the newest snapshot.
func (o *Organizer) keep(entry undoEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.keepLocked(entry)
}

// Caller holds mu.
func (o *Organizer) keepLocked(entry undoEntry) {
	o.forgetExpired()
	i := slices.IndexFunc(o.entries, func(e undoEntry) bool { return e.created.After(entry.created) })
	if i < 0 {
		i = len(o.entries)
	}
	o.entries = slices.Insert(o.entries, i, entry)
	o.bytes += len(entry.image)
	for len(o.entries) > 1 && (len(o.entries) > undoCapacity || o.bytes > undoMaxBytes) {
		o.bytes -= len(o.entries[0].image)
		o.entries = slices.Delete(o.entries, 0, 1)
	}
}

// Wait cancelably for rename-held tokens; missing/expired tokens return false.
func (o *Organizer) takeWhenReleased(ctx context.Context, token string) (undoEntry, bool, error) {
	if err := o.lockReleased(ctx, token); err != nil {
		return undoEntry{}, false, err
	}
	defer o.mu.Unlock()
	entry, ok := o.takeLocked(token)
	return entry, ok, nil
}

// Success returns with mu held for the caller to release; cancellation returns without the lock.
func (o *Organizer) lockReleased(ctx context.Context, token string) error {
	for {
		o.mu.Lock()
		released, held := o.held[token]
		if !held {
			return nil
		}
		o.waiters++
		o.mu.Unlock()
		select {
		case <-released:
		case <-ctx.Done():
		}
		o.mu.Lock()
		o.waiters--
		o.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// Hold only a matching grouping token, waiting for prior renames; invalid tokens or canceled waits
// fall back to ordinary rename, with storage handling context errors.
func (o *Organizer) hold(ctx context.Context, token string, groupID int64) (undoEntry, bool) {
	if err := o.lockReleased(ctx, token); err != nil {
		return undoEntry{}, false
	}
	defer o.mu.Unlock()
	o.forgetExpired()
	i := slices.IndexFunc(o.entries, func(e undoEntry) bool { return e.token == token })
	if i < 0 || o.entries[i].groupID != groupID {
		return undoEntry{}, false
	}
	entry := o.removeAt(i)
	o.held[token] = make(chan struct{})
	return entry, true
}

// Restore unexpired held entries and wake undo waiters.
func (o *Organizer) release(entry undoEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.keepLocked(entry)
	close(o.held[entry.token])
	delete(o.held, entry.token)
}

// Caller holds mu; missing or expired tokens return false.
func (o *Organizer) takeLocked(token string) (undoEntry, bool) {
	o.forgetExpired()
	i := slices.IndexFunc(o.entries, func(e undoEntry) bool { return e.token == token })
	if i < 0 {
		return undoEntry{}, false
	}
	return o.removeAt(i), true
}

// Caller holds mu; expiration is not checked.
func (o *Organizer) removeAt(i int) undoEntry {
	entry := o.entries[i]
	o.bytes -= len(entry.image)
	o.entries = slices.Delete(o.entries, i, i+1)
	return entry
}

// Caller holds mu.
func (o *Organizer) forgetExpired() {
	now := o.now()
	o.entries = slices.DeleteFunc(o.entries, func(e undoEntry) bool {
		expired := !now.Before(e.created.Add(undoLifetime))
		if expired {
			o.bytes -= len(e.image)
		}
		return expired
	})
}
