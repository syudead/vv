package store

// グループの編集（specs/048-groups/data-model.md「Edits」）。GroupStore は書き込みの取引の中で
// 入力を読み、domain の規則（ValidateGroupName・ValidateNesting）で決める。要求が名指した動画・
// グループ・項目のどれかが無ければ domain.ErrGroupTargetNotFound で、取引を巻き戻すので何も
// 書かない（research.md R-12）。どの操作も変えたグループの更新日時を進め、それらとその祖先の id を
// 確定後に domain.GroupsChanged で知らせる（R-13）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// groupNow は編集の時刻の元である。
var groupNow = time.Now

// CreateGroup は名前 name（domain.ValidateGroupName で整える）の手で作るグループを作り、add の
// 動画、次にグループを要求の順に項目にする。origin_folder は持たない。
func (s *GroupStore) CreateGroup(ctx context.Context, name string, add domain.GroupAdd) (domain.Group, error) {
	name, err := domain.ValidateGroupName(name)
	if err != nil {
		return domain.Group{}, err
	}
	var id int64
	err = s.write(ctx, "create the group", func(tx *sql.Tx) ([]int64, error) {
		now := groupNow().UnixMilli()
		result, err := tx.ExecContext(ctx, `insert into groups (name, title_key, created_at, updated_at) values (?, ?, ?, ?)`,
			name, domain.NaturalSortKey(name), now, now)
		if err != nil {
			return nil, fmt.Errorf("cannot create the group: %w", err)
		}
		if id, err = result.LastInsertId(); err != nil {
			return nil, fmt.Errorf("cannot create the group: %w", err)
		}
		if _, err := appendGroupMembers(ctx, tx, id, add); err != nil {
			return nil, err
		}
		return []int64{id}, nil
	})
	if err != nil {
		return domain.Group{}, err
	}
	return s.summary(ctx, id)
}

// RenameGroup はグループ id の名前を name（domain.ValidateGroupName で整える）にし、title_key も
// 合わせる。無ければ domain.ErrGroupNotFound。
func (s *GroupStore) RenameGroup(ctx context.Context, id int64, name string) (domain.Group, error) {
	name, err := domain.ValidateGroupName(name)
	if err != nil {
		return domain.Group{}, err
	}
	err = s.write(ctx, "rename the group", func(tx *sql.Tx) ([]int64, error) {
		if _, err := groupSummary(ctx, tx, id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `update groups set name = ?, title_key = ? where id = ?`,
			name, domain.NaturalSortKey(name), id); err != nil {
			return nil, fmt.Errorf("cannot rename the group: %w", err)
		}
		return touchedGroupsWithAncestors(ctx, tx, []int64{id})
	})
	if err != nil {
		return domain.Group{}, err
	}
	return s.summary(ctx, id)
}

// AddGroupItems は add の動画、次にグループを、グループ id の末尾に要求の順で足す。既に項目に
// あるものは飛ばす。所有者が外したフォルダ由来のメンバーは除外を消し、手で足した項目
// （from_folder = 0）として戻す。グループ id が無ければ domain.ErrGroupNotFound、add の動画か
// グループが無ければ domain.ErrGroupTargetNotFound、add のグループが id 自身か id の祖先なら
// domain.ErrGroupCycle で、どれも何も書かない。何も足さなければ更新日時も進めず、知らせない。
func (s *GroupStore) AddGroupItems(ctx context.Context, id int64, add domain.GroupAdd) (domain.Group, error) {
	err := s.write(ctx, "add to the group", func(tx *sql.Tx) ([]int64, error) {
		if _, err := groupSummary(ctx, tx, id); err != nil {
			return nil, err
		}
		added, err := appendGroupMembers(ctx, tx, id, add)
		if err != nil || added == 0 {
			return nil, err
		}
		return touchedGroupsWithAncestors(ctx, tx, []int64{id})
	})
	if err != nil {
		return domain.Group{}, err
	}
	return s.summary(ctx, id)
}

// RemoveGroupItems はグループ id の項目 itemIDs を消して詰め直す。自動グループのフォルダ由来の
// 項目（from_folder = 1）は除外を書き、作り直しで戻らないようにする。動画・子グループそのものには
// 触れない。グループが無ければ domain.ErrGroupNotFound、項目のどれかがそのグループに無ければ
// domain.ErrGroupTargetNotFound で、何も消さない。itemIDs が空なら更新日時も進めず、知らせない。
func (s *GroupStore) RemoveGroupItems(ctx context.Context, id int64, itemIDs []int64) (domain.Group, error) {
	err := s.write(ctx, "remove the group items", func(tx *sql.Tx) ([]int64, error) {
		if _, err := groupSummary(ctx, tx, id); err != nil {
			return nil, err
		}
		ids := normalizeIDs(slices.Clone(itemIDs))
		if len(ids) == 0 {
			return nil, nil
		}
		for _, itemID := range ids {
			if err := removeGroupItem(ctx, tx, id, itemID); err != nil {
				return nil, err
			}
		}
		if err := renumberPositions(ctx, tx, groupPositions, []int64{id}); err != nil {
			return nil, err
		}
		return touchedGroupsWithAncestors(ctx, tx, []int64{id})
	})
	if err != nil {
		return domain.Group{}, err
	}
	return s.summary(ctx, id)
}

// MoveGroupItem はグループ id の項目 itemID を、項目 afterItemID のすぐ後ろへ移し（nil なら
// 先頭）、詰め直す（specs/044-playlists/research.md R-3 の規則を共有する）。グループが無ければ
// domain.ErrGroupNotFound、どちらかの項目がそのグループに無ければ domain.ErrGroupTargetNotFound。
// itemID の後ろへ自分を移すのは何も変えない（API はその要求を 400 にする）。
func (s *GroupStore) MoveGroupItem(ctx context.Context, id, itemID int64, afterItemID *int64) error {
	return s.write(ctx, "move the group item", func(tx *sql.Tx) ([]int64, error) {
		if _, err := groupSummary(ctx, tx, id); err != nil {
			return nil, err
		}
		moved, err := moveItemAfter(ctx, tx, groupPositions, id, itemID, afterItemID)
		if errors.Is(err, errPositionItemNotFound) {
			return nil, domain.ErrGroupTargetNotFound
		}
		if err != nil || !moved {
			return nil, err
		}
		return touchedGroupsWithAncestors(ctx, tx, []int64{id})
	})
}

// DissolveGroup はグループ id を解散する（research.md R-4）。グループの行と項目を消し、子グループと
// 動画は残し、親からその項目を消して親を詰め直す。自動グループならそのフォルダに ungroup を書き、
// 同じ取引で索引を作り直すので、作り直しでもう作られない。無ければ domain.ErrGroupNotFound。
func (s *GroupStore) DissolveGroup(ctx context.Context, id int64) error {
	return s.write(ctx, "dissolve the group", func(tx *sql.Tx) ([]int64, error) {
		return dissolveGroupAndRebuild(ctx, tx, id)
	})
}

// TagGroup はグループ id をタグにする（research.md R-11）。グループの名前を
// domain.NormalizeTagName に通し、名前かシノニムで引けたタグを使うか新しく作り、子孫の全動画の
// 項目の鍵に手で付け（video_tags）、グループを解散する。1 つの取引で行う。名前がタグ名の規則に
// 合わなければ domain.ErrInvalidTagName、無ければ domain.ErrGroupNotFound で、何も書かない。
func (s *GroupStore) TagGroup(ctx context.Context, id int64) (domain.GroupTag, error) {
	var out domain.GroupTag
	err := s.write(ctx, "turn the group into a tag", func(tx *sql.Tx) ([]int64, error) {
		group, err := groupSummary(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		normalized, err := domain.NormalizeTagName(group.Name)
		if err != nil {
			return nil, err
		}
		if out.Tag, out.Created, err = findOrCreateTag(ctx, tx, normalized); err != nil {
			return nil, err
		}
		keys, err := descendantContentKeys(ctx, tx, id, false)
		if err != nil {
			return nil, err
		}
		if err := attachTagToUserKeys(ctx, tx, keys, out.Tag.ID, groupNow()); err != nil {
			return nil, err
		}
		return dissolveGroupAndRebuild(ctx, tx, id)
	})
	if err != nil {
		return domain.GroupTag{}, err
	}
	return out, nil
}

// write は fn を 1 つの書き込み取引で行い、確定できたときだけ fn が返したグループの id を
// domain.GroupsChanged で知らせる。fn が誤りを返せば巻き戻し、何も書かない。
func (s *GroupStore) write(ctx context.Context, verb string, fn func(*sql.Tx) ([]int64, error)) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot %s: %w", verb, err)
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := fn(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot %s: %w", verb, err)
	}
	s.db.publishEvents(groupsChangedEvents(changed)...)
	return nil
}

// summary はグループ id の要約を、確定した後の状態から読む。
func (s *GroupStore) summary(ctx context.Context, id int64) (domain.Group, error) {
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Group{}, fmt.Errorf("cannot read the group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return groupSummary(ctx, tx, id)
}

// groupSummary はグループ id の要約を読む。無ければ domain.ErrGroupNotFound。
func groupSummary(ctx context.Context, tx *sql.Tx, id int64) (domain.Group, error) {
	var group domain.Group
	err := tx.QueryRowContext(ctx, `select id, name, coalesce(origin_folder, ''), updated_at from groups where id = ?`, id).
		Scan(&group.ID, &group.Name, &group.OriginFolder, &group.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Group{}, domain.ErrGroupNotFound
	}
	if err != nil {
		return domain.Group{}, fmt.Errorf("cannot read the group: %w", err)
	}
	return group, nil
}

// touchedGroupsWithAncestors はグループ ids の更新日時を進め、それらとその祖先の id を返す。
func touchedGroupsWithAncestors(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	if err := touchGroups(ctx, tx, ids, groupNow().UnixMilli()); err != nil {
		return nil, err
	}
	return groupsWithAncestors(ctx, tx, ids)
}

// appendGroupMembers は add の動画、次にグループを、グループ id の末尾に要求の順で足し、足した
// 項目の数を返す。動画の id は利用者データの鍵へ引き直すので、1 つの集まりの 2 つのバージョンは
// 1 つの項目になる。引けない動画か無いグループは domain.ErrGroupTargetNotFound、入れ子の循環は
// domain.ErrGroupCycle で、呼び出し側が取引を巻き戻す。
func appendGroupMembers(ctx context.Context, tx *sql.Tx, id int64, add domain.GroupAdd) (int, error) {
	keys, err := groupAddKeys(ctx, tx, add.VideoIDs)
	if err != nil {
		return 0, err
	}
	edges, err := groupEdges(ctx, tx)
	if err != nil {
		return 0, err
	}
	var next int
	if err := tx.QueryRowContext(ctx, `select count(*) from group_items where group_id = ?`, id).Scan(&next); err != nil {
		return 0, fmt.Errorf("cannot read the group items: %w", err)
	}
	start := next
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `delete from group_exclusions where group_id = ? and content_key = ?`, id, key); err != nil {
			return 0, fmt.Errorf("cannot add to the group: %w", err)
		}
		if next, err = insertGroupItem(ctx, tx, id, "content_key", key, next); err != nil {
			return 0, err
		}
	}
	for _, child := range add.GroupIDs {
		if _, err := groupSummary(ctx, tx, child); errors.Is(err, domain.ErrGroupNotFound) {
			return 0, domain.ErrGroupTargetNotFound
		} else if err != nil {
			return 0, err
		}
		if err := domain.ValidateNesting(id, child, edges); err != nil {
			return 0, err
		}
		edges = append(edges, domain.GroupEdge{Parent: id, Child: child})
		if next, err = insertGroupItem(ctx, tx, id, "child_id", child, next); err != nil {
			return 0, err
		}
	}
	return next - start, nil
}

// groupAddKeys は videoIDs を要求の順に利用者データの鍵へ引き直し、重複を除いて返す。いまライブラリ
// に無い id が 1 つでもあれば domain.ErrGroupTargetNotFound である。
func groupAddKeys(ctx context.Context, tx *sql.Tx, videoIDs []int64) ([]string, error) {
	byID, err := userKeysByVideoID(ctx, tx, videoIDs, true)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(videoIDs))
	for _, videoID := range videoIDs {
		key, ok := byID[videoID]
		if !ok {
			return nil, domain.ErrGroupTargetNotFound
		}
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// insertGroupItem はグループ id の位置 position に、列 column（content_key か child_id。定数で
// 利用者の入力ではない）が value の手で足す項目を入れ、次の位置を返す。既に項目にあれば何も
// 変えず、position をそのまま返す。
func insertGroupItem(ctx context.Context, tx *sql.Tx, id int64, column string, value any, position int) (int, error) {
	result, err := tx.ExecContext(ctx, `insert or ignore into group_items (group_id, `+column+`, position, from_folder)
		values (?, ?, ?, 0)`, id, value, position)
	if err != nil {
		return 0, fmt.Errorf("cannot add to the group: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot add to the group: %w", err)
	}
	return position + int(affected), nil
}

// groupEdges は今の入れ子（子グループの項目）をすべて読む。
func groupEdges(ctx context.Context, tx *sql.Tx) ([]domain.GroupEdge, error) {
	rows, err := tx.QueryContext(ctx, `select group_id, child_id from group_items where child_id is not null`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the group nesting: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var edges []domain.GroupEdge
	for rows.Next() {
		var edge domain.GroupEdge
		if err := rows.Scan(&edge.Parent, &edge.Child); err != nil {
			return nil, fmt.Errorf("cannot read the group nesting: %w", err)
		}
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the group nesting: %w", err)
	}
	return edges, nil
}

// removeGroupItem はグループ id の項目 itemID を消す。フォルダ由来の項目なら除外を書く。その
// グループに無ければ domain.ErrGroupTargetNotFound。詰め直しは呼び出し側が行う。
func removeGroupItem(ctx context.Context, tx *sql.Tx, id, itemID int64) error {
	var key sql.NullString
	var fromFolder bool
	err := tx.QueryRowContext(ctx, `select content_key, from_folder from group_items where id = ? and group_id = ?`, itemID, id).
		Scan(&key, &fromFolder)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrGroupTargetNotFound
	}
	if err != nil {
		return fmt.Errorf("cannot read the group item: %w", err)
	}
	if fromFolder && key.Valid {
		if _, err := tx.ExecContext(ctx, `insert or ignore into group_exclusions (group_id, content_key) values (?, ?)`,
			id, key.String); err != nil {
			return fmt.Errorf("cannot remove the group item: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `delete from group_items where id = ?`, itemID); err != nil {
		return fmt.Errorf("cannot remove the group item: %w", err)
	}
	return nil
}

// dissolveGroupAndRebuild はグループ id を解散し（dissolveGroup）、自動グループだったなら書いた
// ungroup をフォルダの索引に映すため同じ取引で作り直す。変えたグループとその祖先の id を返す。
func dissolveGroupAndRebuild(ctx context.Context, tx *sql.Tx, id int64) ([]int64, error) {
	changed, origin, err := dissolveGroup(ctx, tx, id)
	if err != nil || origin == "" {
		return changed, err
	}
	rebuilt, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return nil, err
	}
	return normalizeIDs(append(changed, rebuilt...)), nil
}

// dissolveGroup はグループ id の行と項目を消し（R-4）、子グループと動画は残す。親からその項目を
// 消して親を詰め直し、親の更新日時を進める。グループのお気に入りとプレイリストの項目も外部キーで
// 消え（specs/048-groups/research.md R-10）、項目を失ったプレイリストを詰め直して更新日時を進める。
// 自動グループなら同じ取引でそのフォルダに ungroup を
// 書く。id と、元の親とその祖先の id と、自動グループのフォルダの鍵（手で作ったグループなら空）を
// 返す。無ければ domain.ErrGroupNotFound。フォルダの索引の作り直しは呼び出し側が行う。
func dissolveGroup(ctx context.Context, tx *sql.Tx, id int64) ([]int64, string, error) {
	group, err := groupSummary(ctx, tx, id)
	if err != nil {
		return nil, "", err
	}
	parents, err := queryInt64s(ctx, tx, `select distinct group_id from group_items where child_id = ? order by group_id`, id)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the parent groups: %w", err)
	}
	changed, err := groupsWithAncestors(ctx, tx, parents)
	if err != nil {
		return nil, "", err
	}
	playlists, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items where group_id = ? order by playlist_id`, id)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the playlists: %w", err)
	}
	// 子グループの項目（child_id）、このグループの項目、お気に入り、プレイリストの項目は外部キーの
	// on delete cascade で消える。
	if _, err := tx.ExecContext(ctx, `delete from groups where id = ?`, id); err != nil {
		return nil, "", fmt.Errorf("cannot dissolve the group: %w", err)
	}
	if err := renumberPositions(ctx, tx, groupPositions, parents); err != nil {
		return nil, "", err
	}
	if err := renumberPlaylists(ctx, tx, playlists); err != nil {
		return nil, "", err
	}
	for _, playlistID := range playlists {
		if err := touchPlaylist(ctx, tx, playlistID); err != nil {
			return nil, "", err
		}
	}
	if err := touchGroups(ctx, tx, parents, groupNow().UnixMilli()); err != nil {
		return nil, "", err
	}
	if group.OriginFolder != "" {
		if err := writeOverride(ctx, tx, group.OriginFolder, domain.FolderGroupUngroup); err != nil {
			return nil, "", err
		}
	}
	return normalizeIDs(append(changed, id)), group.OriginFolder, nil
}

// dissolveFolderGroup はフォルダ folderPath の自動グループがあれば dissolveGroup で解散し、変えた
// グループとその祖先の id を返す。無ければ何もしない。フォルダの索引の作り直しは呼び出し側が行う。
func dissolveFolderGroup(ctx context.Context, tx *sql.Tx, folderPath string) ([]int64, error) {
	id, err := folderGroupID(ctx, tx, folderPath)
	if err != nil || id == 0 {
		return nil, err
	}
	changed, _, err := dissolveGroup(ctx, tx, id)
	return changed, err
}

// attachTagToFolderGroup はフォルダ folderPath の自動グループがあれば、手で足したメンバーと子孫の
// グループの動画の項目の鍵に tagID を手で付ける（research.md R-11）。それらはフォルダ由来のタグを
// 得ないので、解散の前に付けておかないとタグもグループも失う。フォルダ由来の項目は従来どおり
// フォルダ名のタグを得るので手では付けない（specs/017-folder-groups/data-model.md §4）。無ければ
// 何もしない。
func attachTagToFolderGroup(ctx context.Context, tx *sql.Tx, folderPath string, tagID int64) error {
	id, err := folderGroupID(ctx, tx, folderPath)
	if err != nil || id == 0 {
		return err
	}
	keys, err := descendantContentKeys(ctx, tx, id, true)
	if err != nil {
		return err
	}
	return attachTagToUserKeys(ctx, tx, keys, tagID, groupNow())
}

// folderGroupID はフォルダ folderPath の自動グループの id を返す。無ければ 0。
func folderGroupID(ctx context.Context, tx *sql.Tx, folderPath string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `select id from groups where origin_folder = ?`, domain.FolderKey(folderPath)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("cannot read the folder group: %w", err)
	}
	return id, nil
}

// descendantContentKeys はグループ id とその子孫のグループの動画の項目の鍵を、重複なく返す。
// exceptFolderItems なら、グループ id 自身のフォルダ由来の項目（from_folder = 1）の鍵を除く。
func descendantContentKeys(ctx context.Context, tx *sql.Tx, id int64, exceptFolderItems bool) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `with recursive down(id) as (
			select ?1
			union
			select i.child_id from group_items i join down on i.group_id = down.id where i.child_id is not null)
		select distinct content_key from group_items
		where content_key is not null and group_id in (select id from down)
		  and content_key not in (select content_key from group_items
		    where ?2 and group_id = ?1 and from_folder = 1 and content_key is not null)
		order by content_key`, id, exceptFolderItems)
	if err != nil {
		return nil, fmt.Errorf("cannot read the group videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot read the group videos: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the group videos: %w", err)
	}
	return keys, nil
}
