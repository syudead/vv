package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Batch IDs through json_each to keep statement count fixed; notify smart folders after commit.

// BatchTags is atomic and preserves input order in each result category; kind edits retain tentative state and require an existing kind.
func (s *TagStore) BatchTags(ctx context.Context, req domain.TagBatchRequest, ids []int64) (domain.TagBatchOutcome, error) {
	action := req.Action
	if !action.Valid() {
		return domain.TagBatchOutcome{}, fmt.Errorf("unknown tag batch action %q", action)
	}
	if (action == domain.TagBatchSetKind) != (req.KindID != 0) {
		return domain.TagBatchOutcome{}, fmt.Errorf("tag batch action %q with kind id %d", action, req.KindID)
	}
	outcome := domain.TagBatchOutcome{AppliedIDs: []int64{}, NotFoundIDs: []int64{}, NotApplicableIDs: []int64{}}
	unique := uniqueTagIDs(ids)
	if len(unique) == 0 {
		return outcome, nil
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagBatchOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if action == domain.TagBatchSetKind {
		if err := requireTagKind(ctx, tx, req.KindID); err != nil {
			return domain.TagBatchOutcome{}, err
		}
	}
	targets, err := tagBatchTargets(ctx, tx, unique)
	if err != nil {
		return domain.TagBatchOutcome{}, err
	}
	for _, id := range unique {
		target, found := targets[id]
		switch {
		case !found:
			outcome.NotFoundIDs = append(outcome.NotFoundIDs, id)
		case !domain.TagBatchApplies(req, target):
			outcome.NotApplicableIDs = append(outcome.NotApplicableIDs, id)
		default:
			outcome.AppliedIDs = append(outcome.AppliedIDs, id)
		}
	}
	if len(outcome.AppliedIDs) == 0 {
		return outcome, nil
	}

	encoded, err := json.Marshal(outcome.AppliedIDs)
	if err != nil {
		return domain.TagBatchOutcome{}, fmt.Errorf("cannot build tag ids: %w", err)
	}
	if err := applyTagBatch(ctx, tx, req, string(encoded)); err != nil {
		return domain.TagBatchOutcome{}, err
	}

	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return domain.TagBatchOutcome{}, fmt.Errorf("cannot apply the tag batch: %w", err)
	}
	return outcome, nil
}

func applyTagBatch(ctx context.Context, tx *sql.Tx, req domain.TagBatchRequest, encodedIDs string) error {
	switch req.Action {
	case domain.TagBatchConfirm:
		if _, err := tx.ExecContext(ctx,
			`update tags set tentative = 0 where id in (select value from json_each(?))`, encodedIDs,
		); err != nil {
			return fmt.Errorf("cannot confirm the tags: %w", err)
		}
	case domain.TagBatchReject:
		// Copy names before deletion cascades through tag_names.
		if err := rememberRejectedNames(ctx, tx, encodedIDs); err != nil {
			return err
		}
		return deleteTagsByIDs(ctx, tx, encodedIDs)
	case domain.TagBatchDelete:
		return deleteTagsByIDs(ctx, tx, encodedIDs)
	case domain.TagBatchSetKind, domain.TagBatchClearKind:
		var kindID any
		if req.KindID != 0 {
			kindID = req.KindID
		}
		if _, err := tx.ExecContext(ctx,
			`update tags set kind_id = ? where id in (select value from json_each(?))`, kindID, encodedIDs,
		); err != nil {
			return fmt.Errorf("cannot set the kind of the tags: %w", err)
		}
	}
	return nil
}

func requireTagKind(ctx context.Context, tx *sql.Tx, id int64) error {
	var found int
	err := tx.QueryRowContext(ctx, `select 1 from tag_kinds where id = ?`, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrTagKindNotFound
	}
	if err != nil {
		return fmt.Errorf("cannot read the tag kind: %w", err)
	}
	return nil
}

// TagImpact counts eligible tags and distinct visible videos across both manual and folder-derived sources without writing.
func (s *TagStore) TagImpact(ctx context.Context, action domain.TagImpactAction, ids []int64) (domain.TagImpact, error) {
	if !action.Valid() {
		return domain.TagImpact{}, fmt.Errorf("unknown tag impact action %q", action)
	}
	// Read eligible tags and video counts from one snapshot.
	tx, err := s.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.TagImpact{}, err
	}
	defer func() { _ = tx.Rollback() }()
	return tagImpact(ctx, tx, action, ids)
}

// tagImpact is TagImpact inside the caller's snapshot; the assistant counts a tag merge's videos with it.
func tagImpact(ctx context.Context, tx *sql.Tx, action domain.TagImpactAction, ids []int64) (domain.TagImpact, error) {
	unique := uniqueTagIDs(ids)
	if len(unique) == 0 {
		return domain.TagImpact{}, nil
	}
	targets, err := tagBatchTargets(ctx, tx, unique)
	if err != nil {
		return domain.TagImpact{}, err
	}
	counted := make([]int64, 0, len(unique))
	for _, id := range unique {
		if target, found := targets[id]; found && domain.TagImpactApplies(action, target.Tentative) {
			counted = append(counted, id)
		}
	}
	if len(counted) == 0 {
		return domain.TagImpact{}, nil
	}

	encoded, err := json.Marshal(counted)
	if err != nil {
		return domain.TagImpact{}, fmt.Errorf("cannot build tag ids: %w", err)
	}
	query := `select count(distinct video_id) from (` +
		taggedVideosSQL(` and tag_id in (select value from json_each(?))`) + `)`
	var videoCount int
	if err := tx.QueryRowContext(ctx, query, string(encoded), string(encoded)).Scan(&videoCount); err != nil {
		return domain.TagImpact{}, fmt.Errorf("cannot count videos with the tags: %w", err)
	}
	return domain.TagImpact{TagCount: len(counted), VideoCount: videoCount}, nil
}

func uniqueTagIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	return unique
}

func tagBatchTargets(ctx context.Context, q queryExecer, ids []int64) (map[int64]domain.TagBatchTarget, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build tag ids: %w", err)
	}
	rows, err := q.QueryContext(ctx,
		`select id, tentative, coalesce(kind_id, 0) from tags where id in (select value from json_each(?))`,
		string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	targets := make(map[int64]domain.TagBatchTarget, len(ids))
	for rows.Next() {
		var id int64
		var target domain.TagBatchTarget
		if err := rows.Scan(&id, &target.Tentative, &target.KindID); err != nil {
			return nil, fmt.Errorf("cannot read the tags: %w", err)
		}
		targets[id] = target
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the tags: %w", err)
	}
	return targets, nil
}

func deleteTagsByIDs(ctx context.Context, tx *sql.Tx, encodedIDs string) error {
	if _, err := tx.ExecContext(ctx,
		`delete from tags where id in (select value from json_each(?))`, encodedIDs,
	); err != nil {
		return fmt.Errorf("cannot delete the tags: %w", err)
	}
	return nil
}

// Compute natural-sort keys in Go, then insert rejected names in one set-based statement.
func rememberRejectedNames(ctx context.Context, tx *sql.Tx, encodedIDs string) error {
	pairs, err := rejectedNamePairs(ctx, tx, encodedIDs)
	if err != nil {
		return fmt.Errorf("cannot read the rejected tag names: %w", err)
	}
	if len(pairs) == 0 {
		return nil
	}

	encoded, err := json.Marshal(pairs)
	if err != nil {
		return fmt.Errorf("cannot build the rejected tag names: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		insert or ignore into rejected_tag_names (name, sort_key, search_version, created_at)
		select json_extract(value, '$[0]'), json_extract(value, '$[1]'), ?, ?
		  from json_each(?)`,
		domain.SearchKeyVersion, time.Now().Unix(), string(encoded),
	); err != nil {
		return fmt.Errorf("cannot remember the rejected tag names: %w", err)
	}
	return nil
}

func rejectedNamePairs(ctx context.Context, tx *sql.Tx, encodedIDs string) ([][2]string, error) {
	rows, err := tx.QueryContext(ctx, `
		select name from tag_names
		 where canonical = 1 and tag_id in (select value from json_each(?))`, encodedIDs)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var pairs [][2]string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		pairs = append(pairs, [2]string{name, domain.NaturalSortKey(name)})
	}
	return pairs, rows.Err()
}
