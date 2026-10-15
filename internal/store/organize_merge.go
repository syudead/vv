package store

// グループのマージ（specs/050-drag-organize/data-model.md「Operations」の Merge、research.md R-5）。
// ドラッグしたカードを画面の順にたどり、グループはマージ先へマージし、動画はマージ先の末尾へ足す。
// 動画とグループが混ざった選択も 1 つの取引で、取り消しの像は 1 つである。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// mergeStep はマージのカード 1 枚である。動画なら key（利用者データの鍵）を、グループなら group を持つ。
type mergeStep struct {
	key   string
	group int64
}

// mergeOutcome はマージの書き込みの結果である。
type mergeOutcome struct {
	// deleted はマージで消したグループ、holders は X の項目を置き換えたか消した親グループ、removedFrom
	// はそのうち X の項目を消した親グループ、playlists は項目を置き換えたか消したプレイリストである。
	deleted, holders, removedFrom, playlists []int64
	// automatic は自動グループを 1 つでも消したかで、そのときはフォルダの索引を作り直す。
	automatic bool
}

// Merge はドラッグしたカード items を画面の順にたどり、グループはグループ into へマージし（R-5）、
// 動画は into の末尾へ足す（既にあれば飛ばす）。グループ X のマージは、X の項目を順に into の末尾へ
// 足し、X がお気に入りなら into をお気に入りにし、X を持つプレイリストと親グループの項目を
// domain.PlanGroupMerge のとおり X の位置で into に置き換えるか消して、X を消す。自動グループの X は
// そのフォルダに ungroup を書く。X の項目を消して空になった親グループは解散する（DissolveGroup と
// 同じ道筋）。from（0 なら無し）は開いているグループで、あればドラッグしたカードはマージの前に
// Move の取り外しと同じく from を離れる（要件 9）。from の中の動画の項目は消え、グループの項目は
// into に置き換えずに消える。自動グループの from からフォルダのメンバーが抜けると除外を書く。
// 取り消しの像を返す。
//
// into・from か、ドラッグしたグループ・動画が無いか、カードが from に無ければ
// domain.ErrGroupTargetNotFound、ドラッグしたグループが into そのものか into の祖先なら
// domain.ErrGroupCycle で、どれも何も書かない。
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

// resolveMergeSteps は items を重複なく画面の順のマージのカードにし、ドラッグしたグループの id も
// 返す。into・グループ・動画が無ければ domain.ErrGroupTargetNotFound、グループが into か into の
// 祖先なら domain.ErrGroupCycle である。
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

// mergeSource はドラッグしたカードが離れる開いているグループ group（0 なら無し）と、その中の
// カードの項目の id items（カードの順）である。
type mergeSource struct {
	group int64
	items []int64
}

// mergeFromItems は from（0 なら無し）の中の steps の項目の id を steps の順に返す。from が無いか、
// カードが from に無ければ domain.ErrGroupTargetNotFound。
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

// resolveMergeStep はカード item を mergeStep にする。keys は動画の id ごとの利用者データの鍵、
// edges は今の入れ子である。
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

// mergeImage はマージの取り消しの像の範囲を作る（data-model.md「Undo image」の Merge の行で、Move と
// 同じ行）。groups はドラッグしたグループと、あれば開いているグループ from である。消えるかもしれない
// グループ（groups と、解散が届きうるその祖先すべて）と
// into は、グループの行、項目、それを指す項目、除外、お気に入り、それを持つプレイリストの全項目と、
// 自動グループのフォルダの例外を持つ。祖先は親をすべて含むので、親の項目もこの範囲に入る。into の
// グループの行は更新日時を進めるので、後で into を改名すれば取り消しは domain.ErrUndoConflict に
// なる。取り消しは into の行も消して入れ直すので、外部キーで消える into を指す行も範囲に入れる。
// 親の表（groups）の範囲を子の表より前に置く（restoreUndoImage）。
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

// appendScopeSpecs は表 table の列 column が ids の各値の範囲を specs に足す。
func appendScopeSpecs(specs []imageScopeSpec, table, column string, ids []int64) []imageScopeSpec {
	for _, id := range ids {
		specs = append(specs, imageScopeSpec{table: table, column: column, key: id})
	}
	return specs
}

// mergeImageHolders はグループ ids のどれかを持つプレイリストの id と、ids のうち自動グループの
// フォルダの鍵（folder_group_overrides.path の形）を返す。
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

// applyMerge はカードを source から外してから steps を順に into へ書き、空になった親を解散し、自動
// グループを消したならフォルダの索引を作り直し、残るグループの更新日時を進めて、変えたグループと
// その祖先の id を返す。先に外すので、ドラッグしたグループの親としての source は
// domain.PlanGroupMerge に渡らず、その項目は into に置き換わらずに消えたままになる。
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

// leaveMergeSource は source.group の項目 source.items を removeGroupItem で消して（フォルダ由来の
// 項目は除外を書く）詰め直し、source.group を outcome の項目を消した親に足す。空になれば finishMerge
// が解散する。source.group が 0 なら何もしない。
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

// finishMerge はマージで空になった親を解散し、自動グループを消したならフォルダの索引を作り直し、
// 今もある into と親の更新日時を進めて、変えたグループとその祖先の id を返す。into はマージ先として
// 残す（要件 8）ので、空になっても、解散が祖先へ届いても解散しない。
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
	automatic := outcome.automatic
	for _, id := range emptied {
		dissolved, wasAutomatic, err := dissolveGroupKeeping(ctx, tx, id, into)
		if err != nil {
			return nil, err
		}
		changed = append(changed, dissolved...)
		automatic = automatic || wasAutomatic
	}
	if automatic {
		rebuilt, err := rebuildFolderIndex(ctx, tx)
		if err != nil {
			return nil, err
		}
		changed = append(changed, rebuilt...)
	}
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

// existingGroups はグループ ids のうち、今もあるものの id を返す。
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

// mergeGroup はグループ x を into へマージし（R-5）、結果を outcome に足す。x の子グループのうち、
// into そのものか into の祖先になったもの（前のカードのマージで入れ子が変わったとき）は、into に
// 足さずに捨てる。入れ子に循環は作らない。
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
	// x の項目・除外・お気に入りは外部キーの on delete cascade で消える。x を指す項目はもう無い。
	if _, err := tx.ExecContext(ctx, `delete from groups where id = ?`, x); err != nil {
		return fmt.Errorf("cannot merge the group: %w", err)
	}
	outcome.deleted = append(outcome.deleted, x)
	return nil
}

// appendMergedItems はグループ x の項目を位置の順に into の末尾へ足し（既にあれば飛ばす）、足した
// 後の入れ子を返す。
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

// mergedItem はマージで into へ渡すグループの項目 1 つで、動画の鍵か子グループの id のどちらかを持つ。
type mergedItem struct {
	key   sql.NullString
	child sql.NullInt64
}

// readMergedItems はグループ x の項目を位置の順に読む。
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

// handOverGroupHolders は x を持つプレイリストと親グループの項目を domain.PlanGroupMerge のとおり
// x の位置で into に置き換えるか消し、項目を消したものを詰め直して、outcome に足す。edges は今の
// 入れ子である。
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

// mergeHolders は query（?1 が x、?2 が into）が返す id と、それが into を持つかの組を読む。
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
