package store

// Group edits revalidate targets atomically, update changed groups, and notify them and their ancestors only after commit.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

var groupNow = time.Now

// CreateGroup adds videos before child groups, preserving request order and leaving origin_folder unset.
func (s *GroupStore) CreateGroup(ctx context.Context, name string, add domain.GroupAdd) (domain.Group, error) {
	name, err := domain.ValidateGroupName(name)
	if err != nil {
		return domain.Group{}, err
	}
	var id int64
	err = s.write(ctx, "create the group", func(tx *sql.Tx) ([]int64, error) {
		if id, err = insertManualGroup(ctx, tx, name); err != nil {
			return nil, err
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

func insertManualGroup(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	now := groupNow().UnixMilli()
	result, err := tx.ExecContext(ctx, `insert into groups (name, title_key, created_at, updated_at) values (?, ?, ?, ?)`,
		name, domain.NaturalSortKey(name), now, now)
	if err != nil {
		return 0, fmt.Errorf("cannot create the group: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot create the group: %w", err)
	}
	return id, nil
}

func (s *GroupStore) RenameGroup(ctx context.Context, id int64, name string) (domain.Group, error) {
	name, err := domain.ValidateGroupName(name)
	if err != nil {
		return domain.Group{}, err
	}
	err = s.write(ctx, "rename the group", func(tx *sql.Tx) ([]int64, error) {
		return renameGroup(ctx, tx, id, name)
	})
	if err != nil {
		return domain.Group{}, err
	}
	return s.summary(ctx, id)
}

func renameGroup(ctx context.Context, tx *sql.Tx, id int64, name string) ([]int64, error) {
	if _, err := groupSummary(ctx, tx, id); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `update groups set name = ?, title_key = ? where id = ?`,
		name, domain.NaturalSortKey(name), id); err != nil {
		return nil, fmt.Errorf("cannot rename the group: %w", err)
	}
	return touchedGroupsWithAncestors(ctx, tx, []int64{id})
}

// AddGroupItems skips existing members and restores excluded folder members as manual items; no additions means no timestamp or event.
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

// RemoveGroupItems records folder exclusions and dissolves empty groups and ancestors atomically, preserving the videos and child groups.
func (s *GroupStore) RemoveGroupItems(ctx context.Context, id int64, itemIDs []int64) (group domain.Group, dissolved bool, err error) {
	err = s.write(ctx, "remove the group items", func(tx *sql.Tx) ([]int64, error) {
		if _, err := groupSummary(ctx, tx, id); err != nil {
			return nil, err
		}
		ids := normalizeIDs(slices.Clone(itemIDs))
		if len(ids) == 0 {
			return nil, nil
		}
		var changed []int64
		changed, dissolved, err = removeGroupItems(ctx, tx, id, ids)
		return changed, err
	})
	if err != nil || dissolved {
		return domain.Group{}, dissolved, err
	}
	group, err = s.summary(ctx, id)
	return group, false, err
}

// MoveGroupItem places an item after the target, or first for nil; moving after itself is a no-op.
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

// DissolveGroup preserves children and videos, dissolves newly empty ancestors, and prevents automatic folder groups from reappearing.
func (s *GroupStore) DissolveGroup(ctx context.Context, id int64) error {
	return s.write(ctx, "dissolve the group", func(tx *sql.Tx) ([]int64, error) {
		return dissolveGroupAndRebuild(ctx, tx, id)
	})
}

// TagGroup atomically tags all descendant videos with its normalized name, records owner edits, and dissolves the group and empty ancestors.
func (s *GroupStore) TagGroup(ctx context.Context, id int64) (domain.GroupTag, error) {
	var out domain.GroupTag
	err := writeWithEvents(ctx, s.db, "turn the group into a tag", func(tx *sql.Tx) ([]domain.Event, error) {
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
		keys, err := descendantContentKeys(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		now := groupNow()
		attached, err := attachTagToUserKeys(ctx, tx, keys, out.Tag.ID, now)
		if err != nil {
			return nil, err
		}
		c := &changes{}
		if err := recordOwnerVideoActivity(ctx, tx, c, domain.ActivityTagAttach, ownerTagChanges(attached, out.Tag.ID),
			now); err != nil {
			return nil, err
		}
		changed, err := dissolveGroupAndRebuild(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		return append(groupsChangedEvents(changed), c.events()...), nil
	})
	if err != nil {
		return domain.GroupTag{}, err
	}
	return out, nil
}

// Run the edit atomically and notify changed groups only after commit.
func (s *GroupStore) write(ctx context.Context, verb string, fn func(*sql.Tx) ([]int64, error)) error {
	return writeGroups(ctx, s.db, verb, fn)
}

func writeGroups(ctx context.Context, db *DB, verb string, fn func(*sql.Tx) ([]int64, error)) error {
	return writeWithEvents(ctx, db, verb, func(tx *sql.Tx) ([]domain.Event, error) {
		changed, err := fn(tx)
		if err != nil {
			return nil, err
		}
		// グループの構造が変わったので、その表を同じ取引で作り直す（group_closure.go）。
		return groupsChangedEvents(changed), rebuildGroupClosure(ctx, tx)
	})
}

// Publish events only after commit; match-changing edits also invalidate smart folders.
func writeWithEvents(ctx context.Context, db *DB, verb string, fn func(*sql.Tx) ([]domain.Event, error)) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot %s: %w", verb, err)
	}
	defer func() { _ = tx.Rollback() }()
	events, err := fn(tx)
	if err != nil {
		return err
	}
	if err := db.commitEvents(ctx, tx, events...); err != nil {
		return fmt.Errorf("cannot %s: %w", verb, err)
	}
	return nil
}

func (s *GroupStore) summary(ctx context.Context, id int64) (domain.Group, error) {
	return committedGroupSummary(ctx, s.db, id)
}

func committedGroupSummary(ctx context.Context, db *DB, id int64) (domain.Group, error) {
	tx, err := db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.Group{}, fmt.Errorf("cannot read the group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return groupSummary(ctx, tx, id)
}

func groupSummary(ctx context.Context, tx *sql.Tx, id int64) (domain.Group, error) {
	var group domain.Group
	// The root folder's automatic key is empty, so distinguish automatic groups by NULL rather than an empty string.
	err := tx.QueryRowContext(ctx, `select g.id, g.name, coalesce(g.origin_folder, ''), g.origin_folder is not null, `+
		`g.updated_at, `+groupFavoritedAtExpr("g")+` is not null from groups g where g.id = ?`, id).
		Scan(&group.ID, &group.Name, &group.OriginFolder, &group.Automatic, &group.UpdatedAt, &group.Favorite)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Group{}, domain.ErrGroupNotFound
	}
	if err != nil {
		return domain.Group{}, fmt.Errorf("cannot read the group: %w", err)
	}
	return group, nil
}

func touchedGroupsWithAncestors(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	if err := touchGroups(ctx, tx, ids, groupNow().UnixMilli()); err != nil {
		return nil, err
	}
	return groupsWithAncestors(ctx, tx, ids)
}

// Resolve and append videos before groups in request order; missing targets or cycles roll back the transaction.
func appendGroupMembers(ctx context.Context, tx *sql.Tx, id int64, add domain.GroupAdd) (int, error) {
	refs := make([]domain.OrganizeRef, 0, len(add.VideoIDs)+len(add.GroupIDs))
	for _, videoID := range add.VideoIDs {
		refs = append(refs, domain.OrganizeRef{Kind: domain.OrganizeVideo, ID: videoID})
	}
	for _, groupID := range add.GroupIDs {
		refs = append(refs, domain.OrganizeRef{Kind: domain.OrganizeGroup, ID: groupID})
	}
	members, err := resolveGroupMembers(ctx, tx, refs)
	if err != nil {
		return 0, err
	}
	return appendGroupMemberList(ctx, tx, id, members)
}

type groupMember struct {
	Key   string
	Child int64
}

// Resolve and deduplicate in card order; bundle versions collapse to one user-data key, and any missing target fails the operation.
func resolveGroupMembers(ctx context.Context, tx *sql.Tx, refs []domain.OrganizeRef) ([]groupMember, error) {
	var videoIDs []int64
	for _, ref := range refs {
		if ref.Kind == domain.OrganizeVideo {
			videoIDs = append(videoIDs, ref.ID)
		}
	}
	keys, err := userKeysByVideoID(ctx, tx, videoIDs, true)
	if err != nil {
		return nil, err
	}
	members := make([]groupMember, 0, len(refs))
	seen := make(map[groupMember]bool, len(refs))
	for _, ref := range refs {
		member, err := resolveGroupMember(ctx, tx, ref, keys)
		if err != nil {
			return nil, err
		}
		if !seen[member] {
			seen[member] = true
			members = append(members, member)
		}
	}
	return members, nil
}

func resolveGroupMember(ctx context.Context, tx *sql.Tx, ref domain.OrganizeRef, keys map[int64]string) (groupMember, error) {
	switch ref.Kind {
	case domain.OrganizeVideo:
		key, ok := keys[ref.ID]
		if !ok {
			return groupMember{}, domain.ErrGroupTargetNotFound
		}
		return groupMember{Key: key}, nil
	case domain.OrganizeGroup:
		if err := ensureGroupTarget(ctx, tx, ref.ID); err != nil {
			return groupMember{}, err
		}
		return groupMember{Child: ref.ID}, nil
	}
	return groupMember{}, fmt.Errorf("unknown card kind %q", ref.Kind)
}

// Restore excluded folder members as manual items; skip duplicates and reject nesting cycles.
func appendGroupMemberList(ctx context.Context, tx *sql.Tx, id int64, members []groupMember) (int, error) {
	edges, err := groupEdges(ctx, tx)
	if err != nil {
		return 0, err
	}
	next, err := countGroupItems(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	start := next
	for _, member := range members {
		if member.Child == 0 {
			if next, err = appendGroupContent(ctx, tx, id, member.Key, next); err != nil {
				return 0, err
			}
			continue
		}
		if err := domain.ValidateNesting(id, member.Child, edges); err != nil {
			return 0, err
		}
		edges = append(edges, domain.GroupEdge{Parent: id, Child: member.Child})
		if next, err = insertGroupItem(ctx, tx, id, "child_id", member.Child, next); err != nil {
			return 0, err
		}
	}
	return next - start, nil
}

func appendGroupContent(ctx context.Context, tx *sql.Tx, id int64, key string, position int) (int, error) {
	if _, err := tx.ExecContext(ctx, `delete from group_exclusions where group_id = ? and content_key = ?`, id, key); err != nil {
		return 0, fmt.Errorf("cannot add to the group: %w", err)
	}
	return insertGroupItem(ctx, tx, id, "content_key", key, position)
}

// The column is a trusted constant, never user input; duplicates do not consume a position.
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

// Record exclusions for folder members; the caller compacts positions after removal.
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

// Dissolve empty groups atomically, otherwise compact positions; missing items roll back the operation.
func removeGroupItems(ctx context.Context, tx *sql.Tx, id int64, itemIDs []int64) (changed []int64, dissolved bool, err error) {
	for _, itemID := range itemIDs {
		if err := removeGroupItem(ctx, tx, id, itemID); err != nil {
			return nil, false, err
		}
	}
	left, err := countGroupItems(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	if left == 0 {
		changed, err = dissolveGroupAndRebuild(ctx, tx, id)
		return changed, err == nil, err
	}
	if err := renumberPositions(ctx, tx, groupPositions, []int64{id}); err != nil {
		return nil, false, err
	}
	changed, err = touchedGroupsWithAncestors(ctx, tx, []int64{id})
	return changed, false, err
}

func countGroupItems(ctx context.Context, tx *sql.Tx, id int64) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `select count(*) from group_items where group_id = ?`, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("cannot read the group items: %w", err)
	}
	return count, nil
}

// Dissolving automatic groups writes ungroup overrides that must reach the index in the same transaction.
func dissolveGroupAndRebuild(ctx context.Context, tx *sql.Tx, id int64) ([]int64, error) {
	changed, automatic, err := dissolveGroup(ctx, tx, id)
	if err != nil || !automatic {
		return changed, err
	}
	rebuilt, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return nil, err
	}
	return normalizeIDs(append(changed, rebuilt...)), nil
}

// Dissolve newly empty ancestors recursively; the caller rebuilds the folder index when automatic groups were removed.
func dissolveGroup(ctx context.Context, tx *sql.Tx, id int64) ([]int64, bool, error) {
	return dissolveGroupKeeping(ctx, tx, id, 0)
}

// Preserve keep even when empty so a merge target survives dissolution.
func dissolveGroupKeeping(ctx context.Context, tx *sql.Tx, id, keep int64) ([]int64, bool, error) {
	var changed []int64
	automatic := false
	for queue := []int64{id}; len(queue) > 0; queue = queue[1:] {
		result, err := dissolveOneGroup(ctx, tx, queue[0])
		if err != nil {
			return nil, false, err
		}
		changed = append(changed, result.changed...)
		automatic = automatic || result.automatic
		emptied, err := emptyGroups(ctx, tx, slices.DeleteFunc(result.parents, func(parent int64) bool { return parent == keep }))
		if err != nil {
			return nil, false, err
		}
		queue = append(queue, emptied...)
	}
	return normalizeIDs(changed), automatic, nil
}

func emptyGroups(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	var out []int64
	for _, id := range ids {
		count, err := countGroupItems(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			out = append(out, id)
		}
	}
	return out, nil
}

type dissolvedGroup struct {
	changed   []int64
	parents   []int64
	automatic bool
}

// Preserve child groups and videos, compact parent and playlist positions, and suppress recreation of automatic groups.
func dissolveOneGroup(ctx context.Context, tx *sql.Tx, id int64) (dissolvedGroup, error) {
	group, err := groupSummary(ctx, tx, id)
	if err != nil {
		return dissolvedGroup{}, err
	}
	parents, err := queryInt64s(ctx, tx, `select distinct group_id from group_items where child_id = ? order by group_id`, id)
	if err != nil {
		return dissolvedGroup{}, fmt.Errorf("cannot read the parent groups: %w", err)
	}
	changed, err := groupsWithAncestors(ctx, tx, parents)
	if err != nil {
		return dissolvedGroup{}, err
	}
	playlists, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items where group_id = ? order by playlist_id`, id)
	if err != nil {
		return dissolvedGroup{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	// Foreign-key cascades remove child references, items, favorites and playlist entries.
	if _, err := tx.ExecContext(ctx, `delete from groups where id = ?`, id); err != nil {
		return dissolvedGroup{}, fmt.Errorf("cannot dissolve the group: %w", err)
	}
	if err := renumberPositions(ctx, tx, groupPositions, parents); err != nil {
		return dissolvedGroup{}, err
	}
	if err := renumberPlaylists(ctx, tx, playlists); err != nil {
		return dissolvedGroup{}, err
	}
	for _, playlistID := range playlists {
		if err := touchPlaylist(ctx, tx, playlistID); err != nil {
			return dissolvedGroup{}, err
		}
	}
	if err := touchGroups(ctx, tx, parents, groupNow().UnixMilli()); err != nil {
		return dissolvedGroup{}, err
	}
	if group.Automatic {
		if err := writeOverride(ctx, tx, group.OriginFolder, domain.FolderGroupUngroup); err != nil {
			return dissolvedGroup{}, err
		}
	}
	return dissolvedGroup{changed: normalizeIDs(append(changed, id)), parents: parents, automatic: group.Automatic}, nil
}

// Dissolve the automatic folder group if present; the caller rebuilds the folder index.
func dissolveFolderGroup(ctx context.Context, tx *sql.Tx, folderPath string) ([]int64, error) {
	id, err := folderGroupID(ctx, tx, folderPath)
	if err != nil || id == 0 {
		return nil, err
	}
	changed, _, err := dissolveGroup(ctx, tx, id)
	return changed, err
}

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

func descendantContentKeys(ctx context.Context, tx *sql.Tx, id int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `with recursive down(id) as (
			select ?1
			union
			select i.child_id from group_items i join down on i.group_id = down.id where i.child_id is not null)
		select distinct content_key from group_items
		where content_key is not null and group_id in (select id from down)
		order by content_key`, id)
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
