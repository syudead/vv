package store

// 任意のグループ（specs/048-groups/data-model.md）。この段では、フォルダの索引を作り直すたびに
// 自動グループをフォルダとそろえる同期（syncFolderGroups）と、同じパスの引き継ぎと束ねで項目と
// 除外を付け替える rekeyGroupItems を持つ。どちらも呼び出し側の書き込み取引の中で行う。

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

// naturalSortKeyFunction は domain.NaturalSortKey を SQLite から呼ぶ名前である。移行
// 00043_groups.sql が、100 文字に切ったフォルダグループの名前の title_key を作るのに使う
// （SQL では照合形を作れない）。登録の仕方は vv_shuffle_key（listing.go）と同じで、select の
// 中でしか使わず、スキーマには残らない。
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

// syncFolderGroups は、作り直したフォルダの索引 index と例外 overrides から
// domain.PlanFolderGroupSync で変更を決め、自動グループに書く（data-model.md「Automatic group
// sync」）。変わったグループとその祖先の id を小さい順に返す。呼び出し側は確定の後に
// domain.GroupsChanged で知らせる（groupsChangedEvents）。
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

// folderSyncFolders は索引のフォルダの直下の動画を、利用者データの鍵（userKeyExpr）へ引き直す。
// 空の content_key の動画は除き、同じ鍵は最初の 1 つだけ残す。
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

// userKeysByVideoID は動画の id ごとの利用者データの鍵を返す。無い id と空の content_key の
// 動画は入らない。registeredOnly なら、いまライブラリにある動画（registeredVideoCondition、
// userKeysForVideoIDs と同じ条件）だけを引く。
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

// autoGroups は自動グループ（origin_folder を持つグループ）を、その動画の項目と除外とともに
// id の順に読む。
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

// createFolderGroup はフォルダの自動グループを作り、その id を返す。
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

// applyFolderGroupChange は自動グループ1つから直下でなくなったフォルダ由来の項目を外して詰め
// 直し、新しい直下の動画を末尾に足して、更新日時を進める。
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

// appendFolderItems は鍵 keys の動画を、位置 start から順にフォルダ由来の項目として足す。
func appendFolderItems(ctx context.Context, tx *sql.Tx, groupID int64, start int, keys []string) error {
	for i, key := range keys {
		if _, err := tx.ExecContext(ctx, `insert into group_items (group_id, content_key, position, from_folder)
			values (?, ?, ?, 1)`, groupID, key, start+i); err != nil {
			return fmt.Errorf("cannot add the group item: %w", err)
		}
	}
	return nil
}

// touchGroups はグループ ids の更新日時を now に、前の値より必ず後へ進める。
func touchGroups(ctx context.Context, tx *sql.Tx, ids []int64, now int64) error {
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `update groups set updated_at = max(?, updated_at + 1) where id = ?`, now, id); err != nil {
			return fmt.Errorf("cannot update the group: %w", err)
		}
	}
	return nil
}

// groupsWithAncestors はグループ ids とその祖先（子グループの項目をたどった親）の id を、小さい
// 順に重複なく返す。親はその子の名前と子孫の本数を見せるので、子が変われば親も変わる（R-13）。
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

// groupsChangedEvents は変わったグループの id を、確定後に発行する変化にする。
func groupsChangedEvents(ids []int64) []domain.Event {
	if len(ids) == 0 {
		return nil
	}
	return []domain.Event{domain.GroupsChanged{GroupIDs: ids}}
}

// rekeyGroupItems は利用者データの鍵 from の動画の項目と除外を鍵 to へ付け替える
// （data-model.md「Succession and bundling」、R-2）。行は写さずに移す。1 つのグループに鍵 to の
// 項目が 2 つになるときは位置の大きい方を消してそのグループを詰め直し、どちらかが手で足した
// 項目（from_folder = 0）なら残す項目も 0 にする。付け替えの後で鍵 to の項目を持つグループの
// 鍵 to の除外は、項目が見えているので消す。変えたグループとその祖先の id を返し、呼び出し側が
// 確定後に GroupsChanged で知らせる（R-13）。
func rekeyGroupItems(ctx context.Context, tx *sql.Tx, from, to string) ([]int64, error) {
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

// rekeyUserItems はプレイリストとグループの項目を、利用者データの鍵 from から to へ付け替え、
// 変えたグループとその祖先の id を返す。同じパスの引き継ぎと束ねの各経路が、ほかの利用者データを
// 付け替える取引の中で呼ぶ。
func rekeyUserItems(ctx context.Context, tx *sql.Tx, from, to string) ([]int64, error) {
	if err := rekeyPlaylistItems(ctx, tx, from, to); err != nil {
		return nil, err
	}
	return rekeyGroupItems(ctx, tx, from, to)
}
