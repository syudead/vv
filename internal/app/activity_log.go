package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// Storage writes activity logs atomically with mutations; this layer lists records and dispatches
// retries/undo. activity.go separately tracks current ingestion work.

type ActivityLogStore interface {
	// Return descending IDs after cursor; malformed cursors return ErrInvalidCursor.
	ListActivity(
		ctx context.Context, audience domain.Audience, query domain.ActivityQuery, cursor string, limit int,
	) (domain.ActivityPage, error)
	// Missing IDs return domain.ErrNotFound.
	ActivityEntry(ctx context.Context, id int64) (domain.ActivityEntry, error)
	// ActivityDetail reads an entry with its members, pipelines and stage rows; missing IDs return domain.ErrNotFound.
	ActivityDetail(ctx context.Context, id int64) (domain.ActivityDetail, error)
	// Change processing/running records to attention/interrupted.
	RecoverInterruptedActivity(ctx context.Context) (int, error)
}

type ActivityRetrier interface {
	RetryActivity(ctx context.Context, entry domain.ActivityEntry) error
}

// item is the tag ID for auto_tag and the appearance ID for face_analysis, zero for every link; other kinds use zero.
type ActivityUndoer interface {
	UndoActivity(ctx context.Context, entry domain.ActivityEntry, item int64) (domain.ActivityEntry, error)
}

type ActivityOptions struct {
	Store ActivityLogStore
	// Missing kinds do not offer retries.
	Retriers map[domain.ActivityKind]ActivityRetrier
	// Missing kinds do not offer undo.
	Undoers map[domain.ActivityKind]ActivityUndoer
}

type Activity struct {
	store    ActivityLogStore
	retriers map[domain.ActivityKind]ActivityRetrier
	undoers  map[domain.ActivityKind]ActivityUndoer
}

// NewActivity copies the dispatch maps.
func NewActivity(opts ActivityOptions) *Activity {
	a := &Activity{
		store:    opts.Store,
		retriers: map[domain.ActivityKind]ActivityRetrier{},
		undoers:  map[domain.ActivityKind]ActivityUndoer{},
	}
	for kind, retrier := range opts.Retriers {
		if retrier != nil {
			a.retriers[kind] = retrier
		}
	}
	for kind, undoer := range opts.Undoers {
		if undoer != nil {
			a.undoers[kind] = undoer
		}
	}
	return a
}

func (a *Activity) ListActivity(
	ctx context.Context, audience domain.Audience, query domain.ActivityQuery, cursor string, limit int,
) (domain.ActivityPage, error) {
	return a.store.ListActivity(ctx, audience, query, cursor, limit)
}

func (a *Activity) ActivityDetail(ctx context.Context, id int64) (domain.ActivityDetail, error) {
	return a.store.ActivityDetail(ctx, id)
}

// RecoverInterrupted must run before workers start; see docs/design-docs/process-lifecycle.md.
func (a *Activity) RecoverInterrupted(ctx context.Context) (int, error) {
	recovered, err := a.store.RecoverInterruptedActivity(ctx)
	if err != nil {
		return 0, fmt.Errorf("cannot recover the interrupted activity: %w", err)
	}
	return recovered, nil
}

// RetryActivity rejects missing, ineligible, or undispatched records; dispatcher errors such as an
// active scan pass through unchanged.
func (a *Activity) RetryActivity(ctx context.Context, id int64) error {
	entry, err := a.store.ActivityEntry(ctx, id)
	if err != nil {
		return err
	}
	retrier, ok := a.retriers[entry.Kind]
	if !ok || !entry.RetryOffered() {
		return domain.ErrActivityNotRetryable
	}
	return retrier.RetryActivity(ctx, entry)
}

// UndoActivity rejects absent records, tags and links, and ineligible or undispatched targets, returning the
// owner's undo activity on success.
func (a *Activity) UndoActivity(
	ctx context.Context, id int64, target domain.ActivityUndoTarget,
) (domain.ActivityEntry, error) {
	entry, err := a.store.ActivityEntry(ctx, id)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	item, err := undoItem(entry, target)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	undoer, ok := a.undoers[entry.Kind]
	if !ok {
		return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
	}
	return undoer.UndoActivity(ctx, entry, item)
}

// undoItem picks the target's field for the entry's kind and checks it is offered: a tag or link the entry does
// not have is domain.ErrNotFound.
func undoItem(entry domain.ActivityEntry, target domain.ActivityUndoTarget) (int64, error) {
	switch {
	case entry.Kind == domain.ActivityAutoTag:
		index := slices.IndexFunc(entry.Tags, func(tag domain.ActivityEntryTag) bool { return tag.TagID == target.TagID })
		if index < 0 {
			return 0, fmt.Errorf("tag %d of activity entry %d: %w", target.TagID, entry.ID, domain.ErrNotFound)
		}
		if !entry.TagUndoOffered(entry.Tags[index]) {
			return 0, domain.ErrActivityNotUndoable
		}
		return target.TagID, nil
	case entry.Kind == domain.ActivityFaceAnalysis && target.AppearanceID != 0:
		index := slices.IndexFunc(entry.FaceLinks, func(link domain.ActivityFaceLink) bool {
			return link.AppearanceID == target.AppearanceID
		})
		if index < 0 {
			return 0, fmt.Errorf("face link %d of activity entry %d: %w", target.AppearanceID, entry.ID, domain.ErrNotFound)
		}
		if !entry.FaceLinkUndoOffered(entry.FaceLinks[index]) {
			return 0, domain.ErrActivityNotUndoable
		}
		return target.AppearanceID, nil
	case !entry.UndoOffered():
		return 0, domain.ErrActivityNotUndoable
	default:
		return 0, nil
	}
}
