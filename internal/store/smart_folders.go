package store

// Validate parent structure inside writes and mark owner_kept; missing rule references are reported on reads rather than rejected on writes.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

var smartFolderNow = groupNow

var smartRuleRefTables = map[domain.SmartRuleRefKind]string{
	domain.SmartRefTag:          "tags",
	domain.SmartRefTagKind:      "tag_kinds",
	domain.SmartRefPerson:       "people",
	domain.SmartRefOrganization: "organizations",
}

// CreateSmartFolder defaults to added-descending order and rejects a missing parent atomically.
func (s *SmartFolderStore) CreateSmartFolder(ctx context.Context, create domain.SmartFolderCreate) (domain.SmartFolder, error) {
	if create.Sort == "" {
		create.Sort = domain.SortAddedDesc
	}
	folder := domain.SmartFolder{ParentID: create.ParentID, Name: create.Name, Rule: create.Rule, Sort: create.Sort}
	rule, err := checkSmartFolder(&folder)
	if err != nil {
		return domain.SmartFolder{}, err
	}
	var id int64
	err = writeWithEvents(ctx, s.db, "create the smart folder", func(tx *sql.Tx) ([]domain.Event, error) {
		if err := ensureSmartFolderParent(ctx, tx, 0, folder.ParentID); err != nil {
			return nil, err
		}
		var err error
		id, err = insertSmartFolder(ctx, tx, folder, rule, true, 0)
		if err != nil {
			return nil, err
		}
		return smartFolderEditNotice(ctx, tx, []int64{id}, folder.ParentID)
	})
	if err != nil {
		return domain.SmartFolder{}, err
	}
	return s.SmartFolder(ctx, id)
}

// ChangeSmartFolder applies supplied fields, advances edit time and marks owner_kept; root parent is zero and cycles are rejected.
func (s *SmartFolderStore) ChangeSmartFolder(ctx context.Context, id int64, change domain.SmartFolderChange) (domain.SmartFolder, error) {
	err := writeWithEvents(ctx, s.db, "change the smart folder", func(tx *sql.Tx) ([]domain.Event, error) {
		folder, err := readSmartFolder(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		oldParent := folder.ParentID
		applySmartFolderChange(&folder, change)
		rule, err := checkSmartFolder(&folder)
		if err != nil {
			return nil, err
		}
		if change.ParentSet {
			if err := ensureSmartFolderParent(ctx, tx, id, folder.ParentID); err != nil {
				return nil, err
			}
		}
		if _, err := tx.ExecContext(ctx, `update smart_folders
			set parent_id = ?, name = ?, title_key = ?, rule = ?, sort = ?, owner_kept = 1, updated_at = ?
			where id = ?`,
			nullableID(folder.ParentID), folder.Name, domain.NaturalSortKey(folder.Name), rule, string(folder.Sort),
			smartFolderNow().UnixMilli(), id); err != nil {
			return nil, fmt.Errorf("cannot change the smart folder: %w", err)
		}
		return smartFolderEditNotice(ctx, tx, []int64{id}, oldParent, folder.ParentID)
	})
	if err != nil {
		return domain.SmartFolder{}, err
	}
	return s.SmartFolder(ctx, id)
}

// DeleteSmartFolder cascades descendants, favorites and playlist entries, then compacts affected playlists.
func (s *SmartFolderStore) DeleteSmartFolder(ctx context.Context, id int64) error {
	return writeWithEvents(ctx, s.db, "delete the smart folder", func(tx *sql.Tx) ([]domain.Event, error) {
		return deleteSmartFolder(ctx, tx, id)
	})
}
func insertSmartFolder(ctx context.Context, tx *sql.Tx, folder domain.SmartFolder, rule string, ownerKept bool, homePosition int) (int64, error) {
	now := smartFolderNow().UnixMilli()
	var position any
	if homePosition > 0 {
		position = homePosition
	}
	result, err := tx.ExecContext(ctx, `insert into smart_folders
		(parent_id, name, title_key, rule, sort, owner_kept, home_position, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullableID(folder.ParentID), folder.Name, domain.NaturalSortKey(folder.Name), rule, string(folder.Sort),
		ownerKept, position, now, now)
	if err != nil {
		return 0, fmt.Errorf("cannot create the smart folder: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot create the smart folder: %w", err)
	}
	return id, nil
}
func deleteSmartFolder(ctx context.Context, tx *sql.Tx, id int64) ([]domain.Event, error) {
	const subtree = `with recursive down (id) as (
			select ?
			union
			select f.id from smart_folders f join down on f.parent_id = down.id) `
	playlists, err := queryInt64s(ctx, tx, subtree+`select distinct playlist_id from playlist_items
		where smart_folder_id in (select id from down) order by playlist_id`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the playlists: %w", err)
	}
	shelves, err := queryInt64s(ctx, tx, subtree+`select f.id from smart_folders f
		where f.id in (select id from down) and f.home_position is not null`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the home shelves: %w", err)
	}
	folder, err := readSmartFolder(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	events, err := smartFolderEditNotice(ctx, tx, []int64{id}, folder.ParentID)
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `delete from smart_folders where id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot delete the smart folder: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("cannot delete the smart folder: %w", err)
	}
	if deleted == 0 {
		return nil, smartFolderMissing()
	}
	if err := renumberPlaylists(ctx, tx, playlists); err != nil {
		return nil, err
	}
	for _, playlistID := range playlists {
		if err := touchPlaylist(ctx, tx, playlistID); err != nil {
			return nil, err
		}
	}
	if len(shelves) > 0 {
		events = append(events, domain.HomeChanged{})
	}
	return events, nil
}
func (s *SmartFolderStore) SmartFolder(ctx context.Context, id int64) (domain.SmartFolder, error) {
	return readSmartFolder(ctx, s.db.read, id)
}

// MissingRefs deduplicates missing references in outermost-ancestor order.
func (s *SmartFolderStore) MissingRefs(ctx context.Context, id int64) ([]domain.SmartRuleRef, error) {
	tx, err := s.db.read.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folder: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rules, err := smartFolderPathRules(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return missingSmartRuleRefs(ctx, tx, rules)
}

func missingSmartRuleRefs(ctx context.Context, tx *sql.Tx, rules []domain.SmartRule) ([]domain.SmartRuleRef, error) {
	missing := []domain.SmartRuleRef{}
	seen := map[domain.SmartRuleRef]bool{}
	for _, rule := range rules {
		for _, ref := range rule.Refs() {
			table, ok := smartRuleRefTables[ref.Kind]
			if !ok || seen[ref] {
				continue
			}
			seen[ref] = true
			var exists bool
			if err := tx.QueryRowContext(ctx, `select exists (select 1 from `+table+` where id = ?)`, ref.ID).
				Scan(&exists); err != nil {
				return nil, fmt.Errorf("cannot read the smart folder references: %w", err)
			}
			if !exists {
				missing = append(missing, ref)
			}
		}
	}
	return missing, nil
}

// Rewrite merged references atomically without changing owner_kept or edit timestamps, since this is not an owner edit.
func rewriteSmartRuleRefs(ctx context.Context, tx queryExecer, kind domain.SmartRuleRefKind, targetID int64, sourceIDs []int64) error {
	rules, err := smartFolderRules(ctx, tx)
	if err != nil {
		return err
	}
	for _, folder := range rules {
		next, changed := folder.rule.ReplaceRefs(kind, sourceIDs, targetID)
		if !changed {
			continue
		}
		encoded, err := next.Encode()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `update smart_folders set rule = ? where id = ?`, encoded, folder.id); err != nil {
			return fmt.Errorf("cannot rewrite the smart folder rule %d: %w", folder.id, err)
		}
	}
	return nil
}

type smartFolderRule struct {
	id   int64
	rule domain.SmartRule
}

func smartFolderRules(ctx context.Context, tx queryExecer) ([]smartFolderRule, error) {
	rows, err := tx.QueryContext(ctx, `select id, rule from smart_folders order by id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folder rules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var rules []smartFolderRule
	for rows.Next() {
		var folder smartFolderRule
		var raw string
		if err := rows.Scan(&folder.id, &raw); err != nil {
			return nil, fmt.Errorf("cannot read the smart folder rules: %w", err)
		}
		if folder.rule, err = domain.ParseSmartRule([]byte(raw)); err != nil {
			return nil, fmt.Errorf("cannot read the smart folder rule %d: %w", folder.id, err)
		}
		rules = append(rules, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the smart folder rules: %w", err)
	}
	return rules, nil
}

func smartFolderPathRules(ctx context.Context, tx *sql.Tx, id int64) ([]domain.SmartRule, error) {
	rows, err := tx.QueryContext(ctx, `with recursive up (id, depth) as (
			select id, 0 from smart_folders where id = ?
			union
			select f.parent_id, up.depth + 1 from up join smart_folders f on f.id = up.id where f.parent_id is not null)
		select f.rule from up join smart_folders f on f.id = up.id order by up.depth desc`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folder path: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var rules []domain.SmartRule
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("cannot read the smart folder path: %w", err)
		}
		rule, err := domain.ParseSmartRule([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("cannot read the smart folder rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the smart folder path: %w", err)
	}
	if len(rules) == 0 {
		return nil, smartFolderMissing()
	}
	return rules, nil
}

func applySmartFolderChange(folder *domain.SmartFolder, change domain.SmartFolderChange) {
	if change.Name != nil {
		folder.Name = *change.Name
	}
	if change.Rule != nil {
		folder.Rule = *change.Rule
	}
	if change.Sort != nil {
		folder.Sort = *change.Sort
	}
	if change.ParentSet {
		folder.ParentID = change.ParentID
	}
}

func checkSmartFolder(folder *domain.SmartFolder) (string, error) {
	name, err := domain.ValidateGroupName(folder.Name)
	if err != nil {
		return "", err
	}
	folder.Name = name
	if err := domain.ValidateSmartSort(folder.Sort); err != nil {
		return "", err
	}
	return folder.Rule.Encode()
}

// Zero denotes root or a new folder; reject absent parents and self/descendant cycles.
func ensureSmartFolderParent(ctx context.Context, tx *sql.Tx, id, parent int64) error {
	if parent == 0 {
		return nil
	}
	parents, err := smartFolderParents(ctx, tx)
	if err != nil {
		return err
	}
	if _, ok := parents[parent]; !ok {
		return domain.ErrSmartFolderNotFound
	}
	return domain.ValidateSmartFolderParent(id, parent, parents)
}

func smartFolderParents(ctx context.Context, tx *sql.Tx) (map[int64]int64, error) {
	rows, err := tx.QueryContext(ctx, `select id, coalesce(parent_id, 0) from smart_folders`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	parents := map[int64]int64{}
	for rows.Next() {
		var id, parent int64
		if err := rows.Scan(&id, &parent); err != nil {
			return nil, fmt.Errorf("cannot read the smart folders: %w", err)
		}
		parents[id] = parent
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the smart folders: %w", err)
	}
	return parents, nil
}

func readSmartFolder(ctx context.Context, q queryRower, id int64) (domain.SmartFolder, error) {
	var folder domain.SmartFolder
	var rule, sort string
	err := q.QueryRowContext(ctx, `select id, coalesce(parent_id, 0), name, rule, sort, owner_kept,
			coalesce(home_position, 0), created_at, updated_at
		from smart_folders where id = ?`, id).
		Scan(&folder.ID, &folder.ParentID, &folder.Name, &rule, &sort, &folder.OwnerKept, &folder.HomePosition,
			&folder.CreatedAt, &folder.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SmartFolder{}, smartFolderMissing()
	}
	if err != nil {
		return domain.SmartFolder{}, fmt.Errorf("cannot read the smart folder: %w", err)
	}
	if folder.Rule, err = domain.ParseSmartRule([]byte(rule)); err != nil {
		return domain.SmartFolder{}, fmt.Errorf("cannot read the smart folder rule: %w", err)
	}
	folder.Sort = domain.VideoSort(sort)
	return folder, nil
}

func smartFolderMissing() error {
	return fmt.Errorf("smart folder %w", domain.ErrNotFound)
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
