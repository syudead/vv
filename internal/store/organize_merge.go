package store

// Mixed video/group selections merge in screen order within one transaction and one undo image.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

type mergeStep struct {
	key   string
	group int64
}

type mergeOutcome struct {
	// Track deleted groups, changed holders, holders losing items, and affected playlists.
	deleted, holders, removedFrom, playlists []int64
	// Removing any automatic group requires rebuilding the folder index.
	automatic bool
}

// Merge preserves card order, transfers group contents and references, and dissolves empty sources; source items leave before holder replacement.
func (s *OrganizeStore) Merge(ctx context.Context, items []domain.OrganizeRef, into, from int64) ([]byte, error) {
	var encoded []byte
	err := writeGroups(ctx, s.db, "merge the groups", func(tx *sql.Tx) ([]int64, error) {
		steps, groups, err := resolveMergeSteps(ctx, tx, items, into)
		if err != nil {
			return nil, err
		}
		fromItems, err := mergeFromItems(ctx, tx, from, steps)
		if err != nil {
			return nil, err
		}
		image, err := mergeImage(ctx, tx, append(groups, nonZeroIDs(from)...), into)
		if err != nil {
			return nil, err
		}
		if err := image.captureBefore(ctx, tx); err != nil {
			return nil, err
		}
		changed, err := applyMerge(ctx, tx, steps, into, mergeSource{group: from, items: fromItems})
		if err != nil {
			return nil, err
		}
		if err := image.captureAfter(ctx, tx); err != nil {
			return nil, err
		}
		if encoded, err = image.encode(); err != nil {
			return nil, err
		}
		return changed, nil
	})
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// Deduplicate cards in order and reject missing targets or cycles before applying merge steps.
func resolveMergeSteps(ctx context.Context, tx *sql.Tx, items []domain.OrganizeRef, into int64) ([]mergeStep, []int64, error) {
	if err := ensureGroupTarget(ctx, tx, into); err != nil {
		return nil, nil, err
	}
	var videoIDs []int64
	for _, item := range items {
		if item.Kind == domain.OrganizeVideo {
			videoIDs = append(videoIDs, item.ID)
		}
	}
	keys, err := userKeysByVideoID(ctx, tx, videoIDs, true)
	if err != nil {
		return nil, nil, err
	}
	edges, err := groupEdges(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	var steps []mergeStep
	var groups []int64
	seen := make(map[mergeStep]bool, len(items))
	for _, item := range items {
		step, err := resolveMergeStep(ctx, tx, item, into, keys, edges)
		if err != nil {
			return nil, nil, err
		}
		if seen[step] {
			continue
		}
		seen[step] = true
		steps = append(steps, step)
		if step.group != 0 {
			groups = append(groups, step.group)
		}
	}
	return steps, groups, nil
}

// Zero means no source; item IDs follow card order.
type mergeSource struct {
	group int64
	items []int64
}

func mergeFromItems(ctx context.Context, tx *sql.Tx, from int64, steps []mergeStep) ([]int64, error) {
	if from == 0 {
		return nil, nil
	}
	if err := ensureGroupTarget(ctx, tx, from); err != nil {
		return nil, err
	}
	members := make([]groupMember, 0, len(steps))
	for _, step := range steps {
		members = append(members, groupMember{Key: step.key, Child: step.group})
	}
	return groupMemberItemIDs(ctx, tx, from, members)
}

func resolveMergeStep(ctx context.Context, tx *sql.Tx, item domain.OrganizeRef, into int64, keys map[int64]string, edges []domain.GroupEdge) (mergeStep, error) {
	switch item.Kind {
	case domain.OrganizeVideo:
		key, ok := keys[item.ID]
		if !ok {
			return mergeStep{}, domain.ErrGroupTargetNotFound
		}
		return mergeStep{key: key}, nil
	case domain.OrganizeGroup:
		if err := ensureGroupTarget(ctx, tx, item.ID); err != nil {
			return mergeStep{}, err
		}
		if err := domain.ValidateNesting(into, item.ID, edges); err != nil {
			return mergeStep{}, err
		}
		return mergeStep{group: item.ID}, nil
	}
	return mergeStep{}, fmt.Errorf("unknown card kind %q", item.Kind)
}

// Capture potentially dissolved ancestors and all cascading dependents, with parents first; later target edits must invalidate undo.
func mergeImage(ctx context.Context, tx *sql.Tx, groups []int64, into int64) (undoImage, error) {
	deletable, err := groupsWithAncestors(ctx, tx, groups)
	if err != nil {
		return undoImage{}, err
	}
	kept := deletable
	if !slices.Contains(deletable, into) {
		kept = append(slices.Clone(deletable), into)
	}
	playlists, folders, err := mergeImageHolders(ctx, tx, kept)
	if err != nil {
		return undoImage{}, err
	}
	var specs []imageScopeSpec
	specs = appendScopeSpecs(specs, "groups", "id", kept)
	specs = appendScopeSpecs(specs, "group_items", "group_id", kept)
	specs = appendScopeSpecs(specs, "group_items", "child_id", kept)
	specs = appendScopeSpecs(specs, "group_exclusions", "group_id", kept)
	specs = appendScopeSpecs(specs, "group_favorites", "group_id", kept)
	specs = appendScopeSpecs(specs, "playlist_items", "group_id", kept)
	specs = appendScopeSpecs(specs, "playlist_items", "playlist_id", playlists)
	for _, folder := range folders {
		specs = append(specs, imageScopeSpec{table: "folder_group_overrides", column: "path", key: folder})
	}
	image := undoImage{Groups: normalizeIDs(slices.Clone(kept)), Playlists: playlists, RebuildFolders: len(folders) > 0}
	for _, spec := range specs {
		scope, err := newImageScope(spec.table, spec.column, spec.key)
		if err != nil {
			return undoImage{}, err
		}
		image.Scopes = append(image.Scopes, scope)
	}
	return image, nil
}

func appendScopeSpecs(specs []imageScopeSpec, table, column string, ids []int64) []imageScopeSpec {
	for _, id := range ids {
		specs = append(specs, imageScopeSpec{table: table, column: column, key: id})
	}
	return specs
}

func mergeImageHolders(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, []string, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot build group ids: %w", err)
	}
	playlists, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items
		where group_id in (select value from json_each(?)) order by playlist_id`, string(encoded))
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the playlists: %w", err)
	}
	origins, err := queryStrings(ctx, tx, `select origin_folder from groups
		where id in (select value from json_each(?)) and origin_folder is not null order by origin_folder`, string(encoded))
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the folder groups: %w", err)
	}
	folders := make([]string, 0, len(origins))
	for _, origin := range origins {
		folders = append(folders, domain.FolderKey(origin))
	}
	return playlists, folders, nil
}

// Remove source cards first so their references stay removed rather than being replaced by the merge target.
func applyMerge(ctx context.Context, tx *sql.Tx, steps []mergeStep, into int64, source mergeSource) ([]int64, error) {
	var outcome mergeOutcome
	if err := leaveMergeSource(ctx, tx, source, &outcome); err != nil {
		return nil, err
	}
	next, err := countGroupItems(ctx, tx, into)
	if err != nil {
		return nil, err
	}
	for _, step := range steps {
		if step.group == 0 {
			if next, err = appendGroupContent(ctx, tx, into, step.key, next); err != nil {
				return nil, err
			}
			continue
		}
		if err := mergeGroup(ctx, tx, step.group, into, &outcome); err != nil {
			return nil, err
		}
		if next, err = countGroupItems(ctx, tx, into); err != nil {
			return nil, err
		}
	}
	changed, err := finishMerge(ctx, tx, into, outcome)
	if err != nil {
		return nil, err
	}
	for _, id := range outcome.playlists {
		if err := touchPlaylist(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	return changed, nil
}

func leaveMergeSource(ctx context.Context, tx *sql.Tx, source mergeSource, outcome *mergeOutcome) error {
	if source.group == 0 {
		return nil
	}
	for _, itemID := range source.items {
		if err := removeGroupItem(ctx, tx, source.group, itemID); err != nil {
			return err
		}
	}
	if err := renumberPositions(ctx, tx, groupPositions, []int64{source.group}); err != nil {
		return err
	}
	outcome.holders = append(outcome.holders, source.group)
	outcome.removedFrom = append(outcome.removedFrom, source.group)
	return nil
}

// Keep the merge target even if empty or reached by ancestor dissolution.
func finishMerge(ctx context.Context, tx *sql.Tx, into int64, outcome mergeOutcome) ([]int64, error) {
	changed := slices.Clone(outcome.deleted)
	present, err := existingGroups(ctx, tx, normalizeIDs(slices.Clone(outcome.removedFrom)))
	if err != nil {
		return nil, err
	}
	emptied, err := emptyGroups(ctx, tx, slices.DeleteFunc(present, func(id int64) bool { return id == into }))
	if err != nil {
		return nil, err
	}
	dissolved, err := dissolveGroupsKeeping(ctx, tx, emptied, into, outcome.automatic)
	if err != nil {
		return nil, err
	}
	changed = append(changed, dissolved...)
	kept, err := existingGroups(ctx, tx, normalizeIDs(append([]int64{into}, outcome.holders...)))
	if err != nil {
		return nil, err
	}
	touched, err := touchedGroupsWithAncestors(ctx, tx, kept)
	if err != nil {
		return nil, err
	}
	return normalizeIDs(append(changed, touched...)), nil
}

func existingGroups(ctx context.Context, tx *sql.Tx, ids []int64) ([]int64, error) {
	var out []int64
	for _, id := range ids {
		if _, err := groupSummary(ctx, tx, id); errors.Is(err, domain.ErrGroupNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// Earlier merge steps can change nesting; discard newly cyclic child references instead of adding them to the target.
func mergeGroup(ctx context.Context, tx *sql.Tx, x, into int64, outcome *mergeOutcome) error {
	group, err := groupSummary(ctx, tx, x)
	if err != nil {
		return err
	}
	edges, err := appendMergedItems(ctx, tx, x, into)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `insert or ignore into group_favorites (group_id, favorited_at)
		select ?, favorited_at from group_favorites where group_id = ?`, into, x); err != nil {
		return fmt.Errorf("cannot merge the group: %w", err)
	}
	if err := handOverGroupHolders(ctx, tx, x, into, edges, outcome); err != nil {
		return err
	}
	if group.Automatic {
		if err := writeOverride(ctx, tx, group.OriginFolder, domain.FolderGroupUngroup); err != nil {
			return err
		}
		outcome.automatic = true
	}
	// Items, exclusions and favorites cascade; all references to the source have already been handled.
	if _, err := tx.ExecContext(ctx, `delete from groups where id = ?`, x); err != nil {
		return fmt.Errorf("cannot merge the group: %w", err)
	}
	outcome.deleted = append(outcome.deleted, x)
	return nil
}

func appendMergedItems(ctx context.Context, tx *sql.Tx, x, into int64) ([]domain.GroupEdge, error) {
	items, err := readMergedItems(ctx, tx, x)
	if err != nil {
		return nil, err
	}
	edges, err := groupEdges(ctx, tx)
	if err != nil {
		return nil, err
	}
	next, err := countGroupItems(ctx, tx, into)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		switch {
		case item.key.Valid:
			next, err = appendGroupContent(ctx, tx, into, item.key.String, next)
		case domain.ValidateNesting(into, item.child.Int64, edges) == nil:
			next, err = insertGroupItem(ctx, tx, into, "child_id", item.child.Int64, next)
			edges = append(edges, domain.GroupEdge{Parent: into, Child: item.child.Int64})
		}
		if err != nil {
			return nil, err
		}
	}
	return edges, nil
}

type mergedItem struct {
	key   sql.NullString
	child sql.NullInt64
}

func readMergedItems(ctx context.Context, tx *sql.Tx, x int64) ([]mergedItem, error) {
	rows, err := tx.QueryContext(ctx, `select content_key, child_id from group_items where group_id = ? order by position, id`, x)
	if err != nil {
		return nil, fmt.Errorf("cannot read the group items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []mergedItem
	for rows.Next() {
		var item mergedItem
		if err := rows.Scan(&item.key, &item.child); err != nil {
			return nil, fmt.Errorf("cannot read the group items: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the group items: %w", err)
	}
	return items, nil
}

// Apply the domain holder plan at the source position, then compact holders that lost items.
func handOverGroupHolders(ctx context.Context, tx *sql.Tx, x, into int64, edges []domain.GroupEdge, outcome *mergeOutcome) error {
	parents, err := mergeHolders(ctx, tx, `select i.group_id, exists (select 1 from group_items t
		where t.group_id = i.group_id and t.child_id = ?2) from group_items i where i.child_id = ?1 order by i.group_id`, x, into)
	if err != nil {
		return err
	}
	playlists, err := mergeHolders(ctx, tx, `select i.playlist_id, exists (select 1 from playlist_items t
		where t.playlist_id = i.playlist_id and t.group_id = ?2) from playlist_items i where i.group_id = ?1 order by i.playlist_id`, x, into)
	if err != nil {
		return err
	}
	plan := domain.PlanGroupMerge(into, playlists, parents, edges)
	for _, edit := range []struct {
		statement string
		ids       []int64
		args      []any
	}{
		{`update group_items set child_id = ? where group_id = ? and child_id = ?`, plan.ReplaceParents, []any{into}},
		{`delete from group_items where group_id = ? and child_id = ?`, plan.RemoveParents, nil},
		{`update playlist_items set group_id = ? where playlist_id = ? and group_id = ?`, plan.ReplacePlaylists, []any{into}},
		{`delete from playlist_items where playlist_id = ? and group_id = ?`, plan.RemovePlaylists, nil},
	} {
		for _, id := range edit.ids {
			if _, err := tx.ExecContext(ctx, edit.statement, append(slices.Clone(edit.args), id, x)...); err != nil {
				return fmt.Errorf("cannot merge the group: %w", err)
			}
		}
	}
	if err := renumberPositions(ctx, tx, groupPositions, plan.RemoveParents); err != nil {
		return err
	}
	if err := renumberPlaylists(ctx, tx, plan.RemovePlaylists); err != nil {
		return err
	}
	outcome.holders = append(outcome.holders, plan.ReplaceParents...)
	outcome.holders = append(outcome.holders, plan.RemoveParents...)
	outcome.removedFrom = append(outcome.removedFrom, plan.RemoveParents...)
	outcome.playlists = normalizeIDs(append(append(outcome.playlists, plan.ReplacePlaylists...), plan.RemovePlaylists...))
	return nil
}

func mergeHolders(ctx context.Context, tx *sql.Tx, query string, x, into int64) ([]domain.GroupMergeHolder, error) {
	rows, err := tx.QueryContext(ctx, query, x, into)
	if err != nil {
		return nil, fmt.Errorf("cannot read what holds the group: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.GroupMergeHolder
	for rows.Next() {
		var holder domain.GroupMergeHolder
		if err := rows.Scan(&holder.ID, &holder.HoldsTarget); err != nil {
			return nil, fmt.Errorf("cannot read what holds the group: %w", err)
		}
		out = append(out, holder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read what holds the group: %w", err)
	}
	return out, nil
}
