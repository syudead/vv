package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// EditCredits applies validated credit edits to every listed video's shared work information in one
// transaction and returns how many of the listed videos changed. Missing videos are skipped; a missing credit
// or a role over its limit writes nothing.
func (s *WorkInfoStore) EditCredits(ctx context.Context, videoIDs []int64, edits []domain.CreditEdit) (int, error) {
	if err := domain.ValidateCreditEdits(edits); err != nil {
		return 0, err
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot edit the credits: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := checkCreditEditRefs(ctx, tx, edits); err != nil {
		return 0, err
	}
	keyByVideo, err := userKeyByVideoID(ctx, tx, videoIDs)
	if err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(keyByVideo))
	seen := make(map[string]bool, len(keyByVideo))
	for _, id := range videoIDs {
		if key, ok := keyByVideo[id]; ok && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	before, err := workInfoByUserKeys(ctx, tx, keys)
	if err != nil {
		return 0, err
	}
	final := make(map[string]domain.WorkInfo, len(keys))
	for _, key := range keys {
		edited, err := domain.ApplyCreditEdits(before[key], edits)
		if err != nil {
			return 0, err
		}
		final[key] = edited
	}
	edited, err := applyWorkInfo(ctx, tx, keys, final)
	if err != nil {
		return 0, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, edited, time.Now()); err != nil {
		return 0, err
	}
	events, err := workInfoChangedEvents(ctx, tx, edited)
	if err != nil {
		return 0, err
	}
	if err := s.db.commitEvents(ctx, tx, events...); err != nil {
		return 0, fmt.Errorf("cannot edit the credits: %w", err)
	}
	changed := make(map[string]bool, len(edited))
	for _, key := range edited {
		changed[key] = true
	}
	applied := 0
	for _, key := range keyByVideo {
		if changed[key] {
			applied++
		}
	}
	return applied, nil
}

// Report the first missing credit as edits[i].ids[j], in request order.
func checkCreditEditRefs(ctx context.Context, tx *sql.Tx, edits []domain.CreditEdit) error {
	byKind := map[domain.CreditKind][]int64{}
	for _, edit := range edits {
		byKind[edit.Kind] = append(byKind[edit.Kind], edit.IDs...)
	}
	existing := make(map[domain.CreditKind]map[int64]bool, len(byKind))
	for kind, ids := range byKind {
		found, err := existingCreditIDs(ctx, tx, creditTablesByKind[kind], ids)
		if err != nil {
			return err
		}
		existing[kind] = found
	}
	for i, edit := range edits {
		for j, id := range edit.IDs {
			if !existing[edit.Kind][id] {
				return &domain.CreditNotFoundError{Kind: edit.Kind, ID: id, Field: fmt.Sprintf("edits[%d].ids[%d]", i, j)}
			}
		}
	}
	return nil
}

func userKeyByVideoID(ctx context.Context, tx *sql.Tx, videoIDs []int64) (map[int64]string, error) {
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `select v.id, `+userKeyExpr("v")+` from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and `+registeredVideoCondition("v"),
		string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int64]string, len(videoIDs))
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
		}
		out[id] = key
	}
	return out, rows.Err()
}
