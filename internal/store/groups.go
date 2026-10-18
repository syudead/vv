package store

// Folder synchronization and succession/bundle rekeying run inside the caller's write transaction.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"modernc.org/sqlite"

	"github.com/syudead/vv/internal/domain"
)

// Migration 00043 uses the Go natural-sort function for truncated folder names; it appears only in SELECTs, never persisted schema.
const naturalSortKeyFunction = "vv_natural_sort_key"

func init() {
	err := sqlite.RegisterDeterministicScalarFunction(naturalSortKeyFunction, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch value := args[0].(type) {
			case string:
				return domain.NaturalSortKey(value), nil
			case []byte:
				return domain.NaturalSortKey(string(value)), nil
			default:
				return nil, fmt.Errorf("%s: the title is not text: %T", naturalSortKeyFunction, args[0])
			}
		})
	if err != nil {
		panic(fmt.Sprintf("cannot register %s: %v", naturalSortKeyFunction, err))
	}
}

// Use the domain sync plan and return changed groups with ancestors; the caller publishes after commit.
func syncFolderGroups(ctx context.Context, tx *sql.Tx, index domain.FolderIndex, overrides map[string]domain.FolderGroupMode) ([]int64, error) {
	folders, err := folderSyncFolders(ctx, tx, index)
	if err != nil {
		return nil, err
	}
	groups, err := autoGroups(ctx, tx)
	if err != nil {
		return nil, err
	}
	plan := domain.PlanFolderGroupSync(folders, groups, overrides)
	if len(plan.Create) == 0 && len(plan.Change) == 0 {
		return nil, nil
	}
	now := time.Now().UnixMilli()
	changed := make([]int64, 0, len(plan.Create)+len(plan.Change))
	for _, create := range plan.Create {
		id, err := createFolderGroup(ctx, tx, create, now)
		if err != nil {
			return nil, err
		}
		changed = append(changed, id)
	}
	for _, change := range plan.Change {
		if err := applyFolderGroupChange(ctx, tx, change, now); err != nil {
			return nil, err
		}
		changed = append(changed, change.GroupID)
	}
	return groupsWithAncestors(ctx, tx, changed)
}

// Exclude unidentified content and deduplicate user-data keys in first occurrence order.
func folderSyncFolders(ctx context.Context, tx *sql.Tx, index domain.FolderIndex) ([]domain.FolderSyncFolder, error) {
	var ids []int64
	for _, folder := range index.Folders {
		ids = append(ids, folder.VideoIDs...)
	}
	keys, err := userKeysByVideoID(ctx, tx, ids, false)
	if err != nil {
		return nil, err
	}
	folders := make([]domain.FolderSyncFolder, 0, len(index.Folders))
	for _, folder := range index.Folders {
		synced := domain.FolderSyncFolder{Key: folder.Key, Name: folder.Name, Grouped: folder.Grouped, ContentKeys: []string{}}
		seen := map[string]struct{}{}
		for _, id := range folder.VideoIDs {
			key, ok := keys[id]
			if _, dup := seen[key]; !ok || dup {
				continue
			}
			seen[key] = struct{}{}
			synced.ContentKeys = append(synced.ContentKeys, key)
		}
		folders = append(folders, synced)
	}
	return folders, nil
}

// Exclude missing IDs and unidentified content; registeredOnly also requires a registered location.
func userKeysByVideoID(ctx context.Context, tx *sql.Tx, ids []int64, registeredOnly bool) (map[int64]string, error) {
	keys := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return keys, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}
	condition := "1 = 1"
	if registeredOnly {
		condition = registeredVideoCondition("v")
	}
	rows, err := tx.QueryContext(ctx, `select v.id, `+userKeyExpr("v")+` from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and `+condition, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
		}
		keys[id] = key
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	return keys, nil
}

func autoGroups(ctx context.Context, tx *sql.Tx) ([]domain.AutoGroup, error) {
	rows, err := tx.QueryContext(ctx, `select id, origin_folder from groups where origin_folder is not null order by id`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var groups []domain.AutoGroup
	byID := map[int64]int{}
	for rows.Next() {
		var group domain.AutoGroup
		if err := rows.Scan(&group.ID, &group.OriginFolder); err != nil {
			return nil, fmt.Errorf("cannot read the groups: %w", err)
		}
		byID[group.ID] = len(groups)
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the groups: %w", err)
	}
	_ = rows.Close()
	if err := readAutoGroupItems(ctx, tx, groups, byID); err != nil {
		return nil, err
	}
	if err := readAutoGroupExclusions(ctx, tx, groups, byID); err != nil {
		return nil, err
	}
	return groups, nil
}

const autoGroupCondition = `group_id in (select id from groups where origin_folder is not null)`

func readAutoGroupItems(ctx context.Context, tx *sql.Tx, groups []domain.AutoGroup, byID map[int64]int) error {
	rows, err := tx.QueryContext(ctx, `select group_id, content_key, from_folder from group_items
		where content_key is not null and `+autoGroupCondition+` order by group_id, position, id`)
	if err != nil {
		return fmt.Errorf("cannot read the group items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var groupID int64
		var item domain.AutoGroupItem
		if err := rows.Scan(&groupID, &item.ContentKey, &item.FromFolder); err != nil {
			return fmt.Errorf("cannot read the group items: %w", err)
		}
		groups[byID[groupID]].Items = append(groups[byID[groupID]].Items, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the group items: %w", err)
	}
	return nil
}

func readAutoGroupExclusions(ctx context.Context, tx *sql.Tx, groups []domain.AutoGroup, byID map[int64]int) error {
	rows, err := tx.QueryContext(ctx, `select group_id, content_key from group_exclusions where `+autoGroupCondition)
	if err != nil {
		return fmt.Errorf("cannot read the group exclusions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var groupID int64
		var key string
		if err := rows.Scan(&groupID, &key); err != nil {
			return fmt.Errorf("cannot read the group exclusions: %w", err)
		}
		groups[byID[groupID]].Exclusions = append(groups[byID[groupID]].Exclusions, key)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the group exclusions: %w", err)
	}
	return nil
}

func createFolderGroup(ctx context.Context, tx *sql.Tx, create domain.FolderGroupCreate, now int64) (int64, error) {
	result, err := tx.ExecContext(ctx, `insert into groups (name, title_key, origin_folder, created_at, updated_at)
		values (?, ?, ?, ?, ?)`, create.Name, domain.NaturalSortKey(create.Name), create.OriginFolder, now, now)
	if err != nil {
		return 0, fmt.Errorf("cannot create the group: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot create the group: %w", err)
	}
	return id, appendFolderItems(ctx, tx, id, 0, create.ContentKeys)
}

func applyFolderGroupChange(ctx context.Context, tx *sql.Tx, change domain.FolderGroupChange, now int64) error {
	if len(change.Remove) > 0 {
		encoded, err := json.Marshal(change.Remove)
		if err != nil {
			return fmt.Errorf("cannot build the group items: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `delete from group_items where group_id = ? and from_folder = 1
			and content_key in (select value from json_each(?))`, change.GroupID, string(encoded)); err != nil {
			return fmt.Errorf("cannot remove the group items: %w", err)
		}
		if err := renumberPositions(ctx, tx, groupPositions, []int64{change.GroupID}); err != nil {
			return err
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from group_items where group_id = ?`, change.GroupID).Scan(&count); err != nil {
		return fmt.Errorf("cannot read the group items: %w", err)
	}
	if err := appendFolderItems(ctx, tx, change.GroupID, count, change.Append); err != nil {
		return err
	}
	return touchGroups(ctx, tx, []int64{change.GroupID}, now)
}

func appendFolderItems(ctx context.Context, tx *sql.Tx, groupID int64, start int, keys []string) error {
	for i, key := range keys {
		if _, err := tx.ExecContext(ctx, `insert into group_items (group_id, content_key, position, from_folder)
			values (?, ?, ?, 1)`, groupID, key, start+i); err != nil {
			return fmt.Errorf("cannot add the group item: %w", err)
		}
	}
	return nil
}

// Group edit timestamps always advance beyond their previous value.
func touchGroups(ctx context.Context, tx *sql.Tx, ids []int64, now int64) error {
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `update groups set updated_at = max(?, updated_at + 1) where id = ?`, now, id); err != nil {
			return fmt.Errorf("cannot update the group: %w", err)
		}
	}
	return nil
}

// Include ancestors because they display descendant names and counts.
func groupsWithAncestors(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("cannot build group ids: %w", err)
	}
	out, err := queryInt64s(ctx, tx, `with recursive up(id) as (
			select value from json_each(?)
			union
			select i.group_id from group_items i join up on i.child_id = up.id)
		select id from up order by id`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read the parent groups: %w", err)
	}
	return out, nil
}

func groupsChangedEvents(ids []int64) []domain.Event {
	if len(ids) == 0 {
		return nil
	}
	return []domain.Event{domain.GroupsChanged{GroupIDs: ids}}
}

func rekeyGroupItemsOnly(ctx context.Context, tx *sql.Tx, from, to string) ([]int64, error) {
	if from == "" || to == "" || from == to {
		return nil, nil
	}
	affected, err := queryInt64s(ctx, tx, `select group_id from group_items where content_key = ?1
		union select group_id from group_exclusions where content_key = ?1`, from)
	if err != nil {
		return nil, fmt.Errorf("cannot move the group items: %w", err)
	}
	if len(affected) == 0 {
		return nil, nil
	}
	if _, err := tx.ExecContext(ctx, `update group_items set from_folder = 0
		where content_key in (?1, ?2) and from_folder = 1
		and exists (select 1 from group_items o where o.group_id = group_items.group_id
			and o.content_key in (?1, ?2) and o.id <> group_items.id and o.from_folder = 0)`, from, to); err != nil {
		return nil, fmt.Errorf("cannot move the group items: %w", err)
	}
	if err := deleteRekeyDuplicates(ctx, tx, groupPositions, from, to); err != nil {
		return nil, err
	}
	for _, statement := range []string{
		`update group_items set content_key = ?2 where content_key = ?1`,
		`insert or ignore into group_exclusions (group_id, content_key)
			select group_id, ?2 from group_exclusions where content_key = ?1`,
		`delete from group_exclusions where content_key = ?1`,
		`delete from group_exclusions where content_key = ?2
			and exists (select 1 from group_items i where i.group_id = group_exclusions.group_id and i.content_key = ?2)`,
	} {
		if _, err := tx.ExecContext(ctx, statement, from, to); err != nil {
			return nil, fmt.Errorf("cannot move the group items: %w", err)
		}
	}
	if err := renumberPositions(ctx, tx, groupPositions, affected); err != nil {
		return nil, err
	}
	if err := touchGroups(ctx, tx, affected, time.Now().UnixMilli()); err != nil {
		return nil, err
	}
	return groupsWithAncestors(ctx, tx, affected)
}

// Rekey playlists and groups alongside other user data in the caller's transaction.
func rekeyUserItems(ctx context.Context, tx *sql.Tx, from, to string) ([]int64, error) {
	if err := rekeyPlaylistItems(ctx, tx, from, to); err != nil {
		return nil, err
	}
	return rekeyGroupItems(ctx, tx, from, to)
}
