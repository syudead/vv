package store

// Publish collection changes only after their stored information commits.

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

func smartFoldersChangedEvents(ids []int64) []domain.Event {
	if len(ids) == 0 {
		return nil
	}
	return []domain.Event{domain.SmartFoldersChanged{IDs: normalizeIDs(ids)}}
}

func allSmartFolderIDs(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	ids, err := queryInt64s(ctx, tx, `select id from smart_folders order by id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	return ids, nil
}

func changesMatches(events []domain.Event) bool {
	for _, event := range events {
		switch event.(type) {
		case domain.GroupsChanged, domain.VideoBundleChanged, domain.AutoTagApplied, domain.VideoWorkInfoChanged:
			return true
		}
	}
	return false
}

// Merge partial folder notices into one all-folder notice, keeping folders deleted in the same write named; preserve events unchanged when no folders exist.
func withEveryFolderNotice(ctx context.Context, tx *sql.Tx, events []domain.Event) ([]domain.Event, error) {
	ids, err := allSmartFolderIDs(ctx, tx)
	if err != nil || len(ids) == 0 {
		return events, err
	}
	kept := make([]domain.Event, 0, len(events)+1)
	for _, event := range events {
		if notice, ok := event.(domain.SmartFoldersChanged); ok {
			ids = append(ids, notice.IDs...)
		} else {
			kept = append(kept, event)
		}
	}
	return append(kept, smartFoldersChangedEvents(ids)...), nil
}

func withMatchNotice(ctx context.Context, tx *sql.Tx, events []domain.Event) ([]domain.Event, error) {
	if !changesMatches(events) {
		return events, nil
	}
	return withEveryFolderNotice(ctx, tx, events)
}

// Read folder IDs inside the write transaction and publish only after a successful commit.
func (db *DB) commitMatchChange(ctx context.Context, tx *sql.Tx, events ...domain.Event) error {
	events, err := withEveryFolderNotice(ctx, tx, events)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.publishEvents(events...)
	return nil
}

// Only edits whose actual values change matches need an all-folder notice.
func (db *DB) commitMatchChangeIf(ctx context.Context, tx *sql.Tx, matched bool, events ...domain.Event) error {
	if matched {
		return db.commitMatchChange(ctx, tx, events...)
	}
	return db.commitEvents(ctx, tx, events...)
}

// Publish only after commit, adding an all-folder notice for match-changing events.
func (db *DB) commitEvents(ctx context.Context, tx *sql.Tx, events ...domain.Event) error {
	events, err := withMatchNotice(ctx, tx, events)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.publishEvents(events...)
	return nil
}

func smartFolderEditNotice(ctx context.Context, tx *sql.Tx, ids []int64, parents ...int64) ([]domain.Event, error) {
	named, err := smartFolderNoticeIDs(ctx, tx, ids, parents...)
	if err != nil {
		return nil, err
	}
	return smartFoldersChangedEvents(named), nil
}

// Read descendants before deletion and include affected parents, excluding the root sentinel.
func smartFolderNoticeIDs(ctx context.Context, tx *sql.Tx, ids []int64, parents ...int64) ([]int64, error) {
	named, err := smartFolderSubtrees(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for _, parent := range parents {
		if parent != 0 {
			named = append(named, parent)
		}
	}
	return named, nil
}

// Keep requested IDs even if their folders are already absent.
func smartFolderSubtrees(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	named := append([]int64(nil), ids...)
	for _, id := range ids {
		descendants, err := queryInt64s(ctx, tx, `with recursive down (id) as (
				select id from smart_folders where parent_id = ?
				union
				select f.id from smart_folders f join down on f.parent_id = down.id)
			select id from down`, id)
		if err != nil {
			return nil, fmt.Errorf("cannot read the child smart folders: %w", err)
		}
		named = append(named, descendants...)
	}
	return named, nil
}
