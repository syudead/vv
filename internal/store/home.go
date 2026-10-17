package store

// Home shelves are smart folders ordered by home_position, then id.
// Only owner actions change shelf membership and order.

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// Shelves はホームの棚のフォルダを並び（home_position、次に id）の順に返す。
func (s *HomeStore) Shelves(ctx context.Context) ([]domain.SmartFolder, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot read the home shelves: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	shelves, err := homeShelves(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot read the home shelves: %w", err)
	}
	return shelves, nil
}

// SetShelves はホームの棚を ids のフォルダにし、上から順に位置を 1, 2, 3… と振り直す（PUT
// /api/home/shelves、research.md R-7）。棚から外したフォルダと、足したか動かしたフォルダ
// （domain.MovedShelves）の owner_kept を 1 にする。ids が規則に合わなければ
// domain.ErrInvalidHomeShelves、名指すフォルダが無ければ domain.ErrNotFound で、どちらも何も書かない。
// 並びが変わらなければ何も書かず、何も発行しない。
func (s *HomeStore) SetShelves(ctx context.Context, ids []int64) error {
	if err := domain.ValidateHomeShelfOrder(ids); err != nil {
		return err
	}
	return writeWithEvents(ctx, s.db, "set the home shelves", func(tx *sql.Tx) ([]domain.Event, error) {
		for _, id := range ids {
			if _, err := readSmartFolder(ctx, tx, id); err != nil {
				return nil, err
			}
		}
		current, err := homeShelves(ctx, tx)
		if err != nil {
			return nil, err
		}
		before := shelfIDs(current)
		if slices.Equal(before, ids) {
			return nil, nil
		}
		for _, id := range before {
			if !slices.Contains(ids, id) {
				if _, err := tx.ExecContext(ctx, `update smart_folders set home_position = null, owner_kept = 1 where id = ?`,
					id); err != nil {
					return nil, fmt.Errorf("cannot remove the home shelf: %w", err)
				}
			}
		}
		if err := writeShelfPositions(ctx, tx, ids); err != nil {
			return nil, err
		}
		for _, id := range domain.MovedShelves(before, ids) {
			if _, err := tx.ExecContext(ctx, `update smart_folders set owner_kept = 1 where id = ?`, id); err != nil {
				return nil, fmt.Errorf("cannot keep the home shelf: %w", err)
			}
		}
		return []domain.Event{domain.HomeChanged{}}, nil
	})
}

// homeShelves は取引 tx で棚のフォルダを並びの順に読む。
func homeShelves(ctx context.Context, tx *sql.Tx) ([]domain.SmartFolder, error) {
	ids, err := queryInt64s(ctx, tx, `select id from smart_folders where home_position is not null
		order by home_position, id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the home shelves: %w", err)
	}
	return readSmartFolders(ctx, tx, ids)
}

// readSmartFolders は取引 tx でフォルダ ids の行を ids の順に読む。
func readSmartFolders(ctx context.Context, tx *sql.Tx, ids []int64) ([]domain.SmartFolder, error) {
	folders := make([]domain.SmartFolder, 0, len(ids))
	for _, id := range ids {
		folder, err := readSmartFolder(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, nil
}

// shelfIDs はフォルダの id を順に返す。
func shelfIDs(folders []domain.SmartFolder) []int64 {
	ids := make([]int64, len(folders))
	for i, folder := range folders {
		ids[i] = folder.ID
	}
	return ids
}

// writeShelfPositions はフォルダ ids の位置を上から 1, 2, 3… にする。
func writeShelfPositions(ctx context.Context, tx *sql.Tx, ids []int64) error {
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `update smart_folders set home_position = ? where id = ?`, i+1, id); err != nil {
			return fmt.Errorf("cannot set the home shelf position: %w", err)
		}
	}
	return nil
}

// sameRuleAs は folders のどれかが rule と同じ条件かを返す。
func sameRuleAs(folders []domain.SmartFolder, rule domain.SmartRule) bool {
	return slices.ContainsFunc(folders, func(folder domain.SmartFolder) bool {
		return domain.SameSmartRule(folder.Rule, rule)
	})
}
