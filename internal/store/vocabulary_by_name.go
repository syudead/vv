package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// LookupTagName is the tag holding the normalized name as its canonical name or a synonym.
func (s *TagStore) LookupTagName(ctx context.Context, normalized string) (domain.TagRef, bool, error) {
	lookup, found, err := lookupTagName(ctx, s.read, normalized)
	return lookup.ref(), found, err
}

// TagNames are the canonical names of the existing tags among ids.
func (s *TagStore) TagNames(ctx context.Context, ids []int64) (map[int64]string, error) {
	return canonicalNames(ctx, s.read, `select tag_id, name from tag_names
		 where canonical = 1 and tag_id in (select value from json_each(?))`, ids)
}

// Names are the canonical names of the existing entries of kind among ids; a provisional person has none.
func (s *CreditStore) Names(ctx context.Context, kind domain.CreditKind, ids []int64) (map[int64]string, error) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return nil, err
	}
	return canonicalNames(ctx, s.read, t.q(`select {id}, name from {names}
		 where canonical = 1 and {id} in (select value from json_each(?))`), ids)
}

func canonicalNames(ctx context.Context, db *sql.DB, query string, ids []int64) (map[int64]string, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build ids: %w", err)
	}
	rows, err := db.QueryContext(ctx, query, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the names: %w", err)
	}
	defer func() { _ = rows.Close() }()
	names := make(map[int64]string, len(ids))
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("cannot read the names: %w", err)
		}
		names[id] = name
	}
	return names, rows.Err()
}

// ApplyTagByName applies change to the tag of each canonical name in one transaction. A missing name changes
// nothing; a new name any tag holds is a *domain.TagNameConflict.
func (s *TagStore) ApplyTagByName(ctx context.Context, change domain.VocabularyChange) (domain.VocabularyOutcome, error) {
	return applyByName(ctx, s.sql, s.db, change, map[domain.VocabularyAction]byNameAction{
		domain.VocabularyRename: func(ctx context.Context, tx *sql.Tx, change domain.VocabularyChange) (domain.VocabularyOutcome, []domain.Event, error) {
			outcome, err := renameTagByName(ctx, tx, change.Name, change.NewName)
			return outcome, nil, err
		},
		domain.VocabularyDelete: func(ctx context.Context, tx *sql.Tx, change domain.VocabularyChange) (domain.VocabularyOutcome, []domain.Event, error) {
			outcome, err := deleteTagByName(ctx, tx, change.Name)
			return outcome, nil, err
		},
		domain.VocabularyMerge: func(ctx context.Context, tx *sql.Tx, change domain.VocabularyChange) (domain.VocabularyOutcome, []domain.Event, error) {
			outcome, err := mergeTagsByName(ctx, tx, change.Into, change.Sources)
			return outcome, nil, err
		},
	})
}

// byNameAction applies one action by name inside tx and returns the events its commit publishes.
type byNameAction func(ctx context.Context, tx *sql.Tx, change domain.VocabularyChange) (domain.VocabularyOutcome, []domain.Event, error)

// applyByName runs the action of change in one transaction, committed as a match change only when it changed
// anything.
func applyByName(ctx context.Context, writer *sql.DB, db *DB, change domain.VocabularyChange,
	actions map[domain.VocabularyAction]byNameAction,
) (domain.VocabularyOutcome, error) {
	action, ok := actions[change.Action]
	if !ok {
		return domain.VocabularyOutcome{}, fmt.Errorf("unknown vocabulary action: %q", change.Action)
	}
	tx, err := writer.BeginTx(ctx, nil)
	if err != nil {
		return domain.VocabularyOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()
	outcome, events, err := action(ctx, tx, change)
	if err != nil || !outcome.Changed {
		return outcome, err
	}
	if err := db.commitMatchChange(ctx, tx, events...); err != nil {
		return domain.VocabularyOutcome{}, fmt.Errorf("cannot apply the %s change: %w", change.Kind, err)
	}
	return outcome, nil
}

// canonicalTagIDByName is the tag whose canonical name is name; a synonym or a name breaking the rule names none.
func canonicalTagIDByName(ctx context.Context, tx *sql.Tx, name string) (int64, bool, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return 0, false, nil
	}
	lookup, found, err := lookupTagName(ctx, tx, normalized)
	if err != nil || !found || !lookup.isCanonical {
		return 0, false, err
	}
	return lookup.tagID, true, nil
}

func renameTagByName(ctx context.Context, tx *sql.Tx, name, newName string) (domain.VocabularyOutcome, error) {
	normalized, err := domain.NormalizeTagName(newName)
	if err != nil {
		return domain.VocabularyOutcome{}, err
	}
	id, found, err := canonicalTagIDByName(ctx, tx, name)
	if err != nil {
		return domain.VocabularyOutcome{}, err
	}
	if !found {
		// The caller renames its own tag next, so a new name held here is refused even when name is absent.
		holder, taken, err := lookupTagName(ctx, tx, normalized)
		if err == nil && taken {
			err = &domain.TagNameConflict{Tag: holder.ref()}
		}
		return domain.VocabularyOutcome{}, err
	}
	changed, err := renameTagInTx(ctx, tx, id, normalized)
	return domain.VocabularyOutcome{Changed: changed}, err
}

func deleteTagByName(ctx context.Context, tx *sql.Tx, name string) (domain.VocabularyOutcome, error) {
	id, found, err := canonicalTagIDByName(ctx, tx, name)
	if err != nil || !found {
		return domain.VocabularyOutcome{}, err
	}
	if _, err := tx.ExecContext(ctx, `delete from tags where id = ?`, id); err != nil {
		return domain.VocabularyOutcome{}, fmt.Errorf("cannot delete the tag (id=%d): %w", id, err)
	}
	return domain.VocabularyOutcome{Changed: true}, nil
}

// mergeTagsByName creates into, or takes the tag holding it, only when one of sources exists.
func mergeTagsByName(ctx context.Context, tx *sql.Tx, into string, sources []string) (domain.VocabularyOutcome, error) {
	normalized, err := domain.NormalizeTagName(into)
	if err != nil {
		return domain.VocabularyOutcome{}, err
	}
	ids, skipped, err := idsByName(sources, func(name string) (int64, bool, error) { return canonicalTagIDByName(ctx, tx, name) })
	if err != nil || len(ids) == 0 {
		return domain.VocabularyOutcome{Skipped: skipped}, err
	}
	target, created, err := findOrCreateTag(ctx, tx, normalized)
	if err != nil {
		return domain.VocabularyOutcome{}, err
	}
	ids = slices.DeleteFunc(ids, func(id int64) bool { return id == target.ID })
	if len(ids) > 0 {
		if err := mergeTagsInto(ctx, tx, target.ID, ids); err != nil {
			return domain.VocabularyOutcome{}, err
		}
	}
	return domain.VocabularyOutcome{Changed: created || len(ids) > 0, Skipped: skipped}, nil
}

// idsByName resolves names in order, leaving out repeated ids and returning the names that name nothing.
func idsByName(names []string, resolve func(string) (int64, bool, error)) (ids []int64, skipped []string, err error) {
	skipped = []string{}
	for _, name := range names {
		id, found, err := resolve(name)
		switch {
		case err != nil:
			return nil, nil, err
		case !found:
			skipped = append(skipped, name)
		case !slices.Contains(ids, id):
			ids = append(ids, id)
		}
	}
	return ids, skipped, nil
}

// ApplyByName applies change to the entry of kind with each canonical name in one transaction; a name several
// entries hold names the lowest id, as the join of the paired lists does. A new name another entry holds is a
// *domain.CreditNameConflict.
func (s *CreditStore) ApplyByName(ctx context.Context, kind domain.CreditKind, change domain.VocabularyChange) (
	domain.VocabularyOutcome, error,
) {
	t, err := creditTablesFor(kind)
	if err != nil {
		return domain.VocabularyOutcome{}, err
	}
	return applyByName(ctx, s.sql, s.db, change, map[domain.VocabularyAction]byNameAction{
		domain.VocabularyRename: func(ctx context.Context, tx *sql.Tx, change domain.VocabularyChange) (domain.VocabularyOutcome, []domain.Event, error) {
			return renameCreditByName(ctx, tx, t, change.Name, change.NewName)
		},
		domain.VocabularyDelete: func(ctx context.Context, tx *sql.Tx, change domain.VocabularyChange) (domain.VocabularyOutcome, []domain.Event, error) {
			outcome, err := deleteCreditByName(ctx, tx, t, change.Name)
			return outcome, nil, err
		},
		domain.VocabularyMerge: func(ctx context.Context, tx *sql.Tx, change domain.VocabularyChange) (domain.VocabularyOutcome, []domain.Event, error) {
			return mergeCreditsByName(ctx, tx, t, change.Into, change.Sources)
		},
	})
}

// creditIDByName is the lowest id of kind whose canonical name is name after normalization.
func creditIDByName(ctx context.Context, tx *sql.Tx, t creditTables, name string) (int64, bool, error) {
	normalized, err := domain.NormalizeCredit(domain.CreditInput{Name: name})
	if err != nil {
		return 0, false, nil
	}
	var id int64
	err = tx.QueryRowContext(ctx, t.q(`select min({id}) from {names} where name = ? and canonical = 1 having count(*) > 0`),
		normalized.Name).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("cannot look up the %s by name: %w", t.kind, err)
	}
	return id, true, nil
}

func renameCreditByName(ctx context.Context, tx *sql.Tx, t creditTables, name, newName string) (
	domain.VocabularyOutcome, []domain.Event, error,
) {
	normalized, err := domain.NormalizeCredit(domain.CreditInput{Name: newName})
	if err != nil {
		return domain.VocabularyOutcome{}, nil, err
	}
	id, found, err := creditIDByName(ctx, tx, t, name)
	if err != nil {
		return domain.VocabularyOutcome{}, nil, err
	}
	// The caller renames its own entry next, so a new name held here is refused even when name is absent.
	holder, taken, err := creditIDByName(ctx, tx, t, normalized.Name)
	switch {
	case err != nil:
		return domain.VocabularyOutcome{}, nil, err
	case taken && (!found || holder != id):
		return domain.VocabularyOutcome{}, nil, &domain.CreditNameConflict{Kind: t.kind, Name: normalized.Name}
	case !found:
		return domain.VocabularyOutcome{}, nil, nil
	}
	current, updated, changes, err := updateCreditInTx(ctx, tx, t, id, domain.CreditPatch{Name: &normalized.Name})
	if err != nil {
		return domain.VocabularyOutcome{}, nil, err
	}
	events, err := queuePersonFace(ctx, tx, current, updated)
	if err != nil {
		return domain.VocabularyOutcome{}, nil, err
	}
	return domain.VocabularyOutcome{Changed: changes.any()}, events, nil
}

func deleteCreditByName(ctx context.Context, tx *sql.Tx, t creditTables, name string) (domain.VocabularyOutcome, error) {
	id, found, err := creditIDByName(ctx, tx, t, name)
	if err != nil || !found {
		return domain.VocabularyOutcome{}, err
	}
	return domain.VocabularyOutcome{Changed: true}, deleteCreditInTx(ctx, tx, t, id)
}

// mergeCreditsByName creates into, or takes the entry holding it, only when one of sources exists.
func mergeCreditsByName(ctx context.Context, tx *sql.Tx, t creditTables, into string, sources []string) (
	domain.VocabularyOutcome, []domain.Event, error,
) {
	normalized, err := domain.NormalizeCredit(domain.CreditInput{Name: into})
	if err != nil {
		return domain.VocabularyOutcome{}, nil, err
	}
	ids, skipped, err := idsByName(sources, func(name string) (int64, bool, error) { return creditIDByName(ctx, tx, t, name) })
	if err != nil || len(ids) == 0 {
		return domain.VocabularyOutcome{Skipped: skipped}, nil, err
	}
	target, found, err := creditIDByName(ctx, tx, t, normalized.Name)
	if err != nil {
		return domain.VocabularyOutcome{}, nil, err
	}
	if !found {
		if target, err = insertCredit(ctx, tx, t, normalized, nil); err != nil {
			return domain.VocabularyOutcome{}, nil, err
		}
	}
	ids = slices.DeleteFunc(ids, func(id int64) bool { return id == target })
	if len(ids) == 0 {
		return domain.VocabularyOutcome{Changed: !found, Skipped: skipped}, nil, nil
	}
	_, events, err := mergeCreditsInTx(ctx, tx, t, target, ids)
	return domain.VocabularyOutcome{Changed: true, Skipped: skipped}, events, err
}
