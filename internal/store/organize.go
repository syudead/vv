package store

// ドラッグの操作と取り消し（specs/050-drag-organize/data-model.md「Operations」）。OrganizeStore の
// 操作はどれも 1 つの取引で、変える行の前後の像（organize_image.go）を同じ取引で読んで返し、確定後に
// domain.GroupsChanged を発行する。要求が名指したグループや項目が無ければ
// domain.ErrGroupTargetNotFound で、何も書かない。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/syudead/vv/internal/domain"
)

// Reorder はグループ groupID の項目 itemIDs を、その順のまままとめて項目 afterItemID のすぐ後ろへ
// 移し（nil なら先頭）、位置を 0 から詰め直す。取り消しの像（そのグループの group_items）を返す。
// グループが無いか、項目か afterItemID がそのグループに無いか、afterItemID が itemIDs の中にあれば
// domain.ErrGroupTargetNotFound で、何も書かない。
func (s *OrganizeStore) Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) ([]byte, error) {
	var encoded []byte
	err := writeGroups(ctx, s.db, "reorder the group", func(tx *sql.Tx) ([]int64, error) {
		if err := ensureGroupTarget(ctx, tx, groupID); err != nil {
			return nil, err
		}
		scope, err := newImageScope("group_items", "group_id", groupID)
		if err != nil {
			return nil, err
		}
		image := undoImage{Scopes: []imageScope{scope}, Groups: []int64{groupID}}
		if err := image.captureBefore(ctx, tx); err != nil {
			return nil, err
		}
		err = placeItemsAfter(ctx, tx, groupPositions, groupID, itemIDs, afterItemID)
		if errors.Is(err, errPositionItemNotFound) {
			return nil, domain.ErrGroupTargetNotFound
		}
		if err != nil {
			return nil, err
		}
		if encoded, err = image.captureAfterAndEncode(ctx, tx); err != nil {
			return nil, err
		}
		return touchedGroupsWithAncestors(ctx, tx, []int64{groupID})
	})
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// Move はカード move.Items を画面の順にグループ move.To の末尾へ足し（既にあるものは飛ばす）、
// move.From があればそこから外す（data-model.md「Operations」の Move）。To が無ければ外すだけである。
// 外すのは RemoveGroupItems と同じで、フォルダ由来のメンバーには除外を書き、最後の項目を外した From は
// 解散する（空になった祖先も）。取り消しの像（groupsImage。From・To と From の祖先）を返す。From か To
// が無い、カードの動画やグループが無い、カードが From に無ければ domain.ErrGroupTargetNotFound、
// ドラッグしたグループが To か To の祖先なら domain.ErrGroupCycle で、どれも何も書かない。From と To が
// どちらも無いか同じなら誤りである（API はその要求を 400 にする）。
func (s *OrganizeStore) Move(ctx context.Context, move domain.OrganizeMove) ([]byte, error) {
	switch {
	case move.From == 0 && move.To == 0:
		return nil, errors.New("cannot move the cards: there is no group to leave or join")
	case move.From == move.To:
		return nil, errors.New("cannot move the cards into the group they leave")
	}
	var encoded []byte
	err := writeGroups(ctx, s.db, "move the cards", func(tx *sql.Tx) ([]int64, error) {
		members, fromItems, err := resolveMove(ctx, tx, move)
		if err != nil {
			return nil, err
		}
		scope, err := groupsWithAncestors(ctx, tx, nonZeroIDs(move.From))
		if err != nil {
			return nil, err
		}
		image, err := groupsImage(ctx, tx, append(scope, nonZeroIDs(move.To)...))
		if err != nil {
			return nil, err
		}
		if err := image.captureBefore(ctx, tx); err != nil {
			return nil, err
		}
		changed, err := applyMove(ctx, tx, move, members, fromItems)
		if err != nil {
			return nil, err
		}
		if err := image.captureAfter(ctx, tx); err != nil {
			return nil, err
		}
		image.finishGroupsImage(append(changed, nonZeroIDs(move.From, move.To)...))
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

// resolveMove は Move の From と To があるかを確かめ、カードを項目にするもの（resolveGroupMembers）と、
// From があればそれぞれの From の項目の id を返す。
func resolveMove(ctx context.Context, tx *sql.Tx, move domain.OrganizeMove) ([]groupMember, []int64, error) {
	for _, id := range nonZeroIDs(move.From, move.To) {
		if err := ensureGroupTarget(ctx, tx, id); err != nil {
			return nil, nil, err
		}
	}
	members, err := resolveGroupMembers(ctx, tx, move.Items)
	if err != nil || move.From == 0 {
		return members, nil, err
	}
	itemIDs, err := groupMemberItemIDs(ctx, tx, move.From, members)
	if err != nil {
		return nil, nil, err
	}
	return members, itemIDs, nil
}

// groupMemberItemIDs はグループ id の中の members の項目の id を members の順に返す。どれかが
// 無ければ domain.ErrGroupTargetNotFound。
func groupMemberItemIDs(ctx context.Context, tx *sql.Tx, id int64, members []groupMember) ([]int64, error) {
	itemIDs := make([]int64, 0, len(members))
	for _, member := range members {
		itemID, err := groupMemberItemID(ctx, tx, id, member)
		if err != nil {
			return nil, err
		}
		itemIDs = append(itemIDs, itemID)
	}
	return itemIDs, nil
}

// groupMemberItemID はグループ id の中の member の項目の id を返す。無ければ
// domain.ErrGroupTargetNotFound。
func groupMemberItemID(ctx context.Context, tx *sql.Tx, id int64, member groupMember) (int64, error) {
	column, value := "content_key", any(member.Key)
	if member.Child != 0 {
		column, value = "child_id", member.Child
	}
	var itemID int64
	err := tx.QueryRowContext(ctx, `select id from group_items where group_id = ? and `+column+` = ?`, id, value).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrGroupTargetNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("cannot read the group item: %w", err)
	}
	return itemID, nil
}

// applyMove は Move の書き込みで、To へ足してから From の項目 fromItems を外す。先に足すので、From を
// 解散して To の項目が尽きても To は残る。変えたグループとその祖先の id を返す。
func applyMove(ctx context.Context, tx *sql.Tx, move domain.OrganizeMove, members []groupMember, fromItems []int64) ([]int64, error) {
	var changed []int64
	if move.To != 0 {
		added, err := appendGroupMemberList(ctx, tx, move.To, members)
		if err != nil {
			return nil, err
		}
		if added > 0 {
			if changed, err = touchedGroupsWithAncestors(ctx, tx, []int64{move.To}); err != nil {
				return nil, err
			}
		}
	}
	if move.From != 0 {
		removed, _, err := removeGroupItems(ctx, tx, move.From, fromItems)
		if err != nil {
			return nil, err
		}
		changed = append(changed, removed...)
	}
	return normalizeIDs(changed), nil
}

// nonZeroIDs は ids のうち 0 でないものを返す。
func nonZeroIDs(ids ...int64) []int64 {
	return slices.DeleteFunc(ids, func(id int64) bool { return id == 0 })
}

// Undo は操作が返した像 image の前の状態へ戻す（research.md R-1）。今の行が後の像と等しくなければ
// domain.ErrUndoConflict で、何も書かない。確定後に、集まりを戻した動画を domain.VideoBundleChanged
// で、戻したグループとその祖先を domain.GroupsChanged で知らせる。
func (s *OrganizeStore) Undo(ctx context.Context, image []byte) error {
	decoded, err := decodeUndoImage(image)
	if err != nil {
		return err
	}
	return writeWithEvents(ctx, s.db, "undo", func(tx *sql.Tx) ([]domain.Event, error) {
		changed, err := restoreUndoImage(ctx, tx, decoded)
		if err != nil {
			return nil, err
		}
		var events []domain.Event
		if len(decoded.Videos) > 0 {
			events = append(events, domain.VideoBundleChanged{VideoIDs: decoded.Videos})
		}
		return append(events, groupsChangedEvents(changed)...), nil
	})
}

// ensureGroupTarget は要求が名指したグループ id があるかを確かめる。無ければ
// domain.ErrGroupTargetNotFound（contracts/screen-api.md「Errors」）。
func ensureGroupTarget(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := groupSummary(ctx, tx, id)
	if errors.Is(err, domain.ErrGroupNotFound) {
		return domain.ErrGroupTargetNotFound
	}
	return err
}

// Group は動画 targetID に動画 videoIDs を落とした「グループにする」を行う（data-model.md
// 「Operations」の Group、research.md R-6）。targetID の題名を domain.GroupNameFromTitle で名前に
// した手で作るグループを作り、targetID、続いて videoIDs を要求の順に項目にする（同じ利用者データの
// 鍵は 1 つにする）。fromID があれば、新しいグループがグループ fromID の中で targetID の項目の位置を
// 取り、targetID と videoIDs の項目は fromID から抜ける（フォルダ由来の項目は除外を書く）。新しい
// グループの id と取り消しの像を返す。像は新しいグループとそれを指す行（groupReferenceScopes）、
// fromID の項目と除外である。動画か fromID が無いか、どれかの動画が fromID に無ければ
// domain.ErrGroupTargetNotFound で、何も書かない。
func (s *OrganizeStore) Group(ctx context.Context, videoIDs []int64, targetID int64, fromID *int64) (int64, []byte, error) {
	var id int64
	var encoded []byte
	err := writeGroups(ctx, s.db, "make the group", func(tx *sql.Tx) ([]int64, error) {
		name, keys, err := groupOperationMembers(ctx, tx, videoIDs, targetID)
		if err != nil {
			return nil, err
		}
		image, leaving, err := captureGroupSource(ctx, tx, fromID, keys)
		if err != nil {
			return nil, err
		}
		if id, err = insertManualGroup(ctx, tx, name); err != nil {
			return nil, err
		}
		for position, key := range keys {
			if _, err := insertGroupItem(ctx, tx, id, "content_key", key, position); err != nil {
				return nil, err
			}
		}
		image.Groups = []int64{id}
		if fromID != nil {
			if err := placeGroupAt(ctx, tx, *fromID, id, leaving); err != nil {
				return nil, err
			}
			image.Groups = append(image.Groups, *fromID)
		}
		created, err := groupReferenceScopes(id)
		if err != nil {
			return nil, err
		}
		image.Scopes = append(created, image.Scopes...)
		// 新しいグループの行は像にあるので、更新日時を進めてから後の像を読む。
		changed, err := touchedGroupsWithAncestors(ctx, tx, image.Groups)
		if err != nil {
			return nil, err
		}
		if encoded, err = image.captureAfterAndEncode(ctx, tx); err != nil {
			return nil, err
		}
		return changed, nil
	})
	if err != nil {
		return 0, nil, err
	}
	return id, encoded, nil
}

// groupOperationMembers は「グループにする」の新しいグループの名前と、項目にする利用者データの鍵を
// targetID、続いて videoIDs の順に重複なく返す（resolveGroupMembers）。動画が無ければ
// domain.ErrGroupTargetNotFound。
func groupOperationMembers(ctx context.Context, tx *sql.Tx, videoIDs []int64, targetID int64) (string, []string, error) {
	refs := make([]domain.OrganizeRef, 0, len(videoIDs)+1)
	for _, videoID := range append([]int64{targetID}, videoIDs...) {
		refs = append(refs, domain.OrganizeRef{Kind: domain.OrganizeVideo, ID: videoID})
	}
	members, err := resolveGroupMembers(ctx, tx, refs)
	if err != nil {
		return "", nil, err
	}
	keys := make([]string, 0, len(members))
	for _, member := range members {
		keys = append(keys, member.Key)
	}
	target, err := getVideo(ctx, tx, domain.AudienceOwner, targetID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil, domain.ErrGroupTargetNotFound
	}
	if err != nil {
		return "", nil, err
	}
	name, err := domain.GroupNameFromTitle(target.Title)
	if err != nil {
		return "", nil, err
	}
	return name, keys, nil
}

// captureGroupSource は「グループにする」の fromID の項目と除外の前の像を読み、keys の項目の id を
// keys の順に返す（先頭は落とした先の動画の項目）。fromID が nil なら空の像を返す。fromID が無いか、
// keys のどれかの項目が fromID に無ければ domain.ErrGroupTargetNotFound。
func captureGroupSource(ctx context.Context, tx *sql.Tx, fromID *int64, keys []string) (undoImage, []int64, error) {
	if fromID == nil {
		return undoImage{}, nil, nil
	}
	if err := ensureGroupTarget(ctx, tx, *fromID); err != nil {
		return undoImage{}, nil, err
	}
	leaving := make([]int64, 0, len(keys))
	for _, key := range keys {
		var itemID int64
		err := tx.QueryRowContext(ctx, `select id from group_items where group_id = ? and content_key = ?`, *fromID, key).Scan(&itemID)
		if errors.Is(err, sql.ErrNoRows) {
			return undoImage{}, nil, domain.ErrGroupTargetNotFound
		}
		if err != nil {
			return undoImage{}, nil, fmt.Errorf("cannot read the group item: %w", err)
		}
		leaving = append(leaving, itemID)
	}
	items, err := newImageScope("group_items", "group_id", *fromID)
	if err != nil {
		return undoImage{}, nil, err
	}
	exclusions, err := newImageScope("group_exclusions", "group_id", *fromID)
	if err != nil {
		return undoImage{}, nil, err
	}
	image := undoImage{Scopes: []imageScope{items, exclusions}}
	if err := image.captureBefore(ctx, tx); err != nil {
		return undoImage{}, nil, err
	}
	return image, leaving, nil
}

// placeGroupAt はグループ fromID の項目 leaving（先頭は落とした先の動画の項目）を外し、先頭の項目が
// あった位置にグループ childID の項目を入れて、fromID を 0 から詰め直す。フォルダ由来の項目を外すと
// 除外を書く（removeGroupItem）。
func placeGroupAt(ctx context.Context, tx *sql.Tx, fromID, childID int64, leaving []int64) error {
	order, err := positionOrder(ctx, tx, groupPositions, fromID)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `insert into group_items (group_id, child_id, position, from_folder) values (?, ?, ?, 0)`,
		fromID, childID, len(order))
	if err != nil {
		return fmt.Errorf("cannot add to the group: %w", err)
	}
	childItem, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("cannot add to the group: %w", err)
	}
	leavingSet := make(map[int64]struct{}, len(leaving))
	for _, itemID := range leaving {
		leavingSet[itemID] = struct{}{}
	}
	placed := make([]int64, 0, len(order)+1-len(leaving))
	for _, itemID := range order {
		_, left := leavingSet[itemID]
		switch {
		case itemID == leaving[0]:
			placed = append(placed, childItem)
		case left:
		default:
			placed = append(placed, itemID)
		}
	}
	for _, itemID := range leaving {
		if err := removeGroupItem(ctx, tx, fromID, itemID); err != nil {
			return err
		}
	}
	return writePositionOrder(ctx, tx, groupPositions, placed)
}

// RenameGroup はグループ id の名前を name にする。名前の規則と誤りは GroupStore.RenameGroup と
// 同じである。image は取り消しのトークンが持つ「グループにする」の像で（nil なら持たない）、名前を
// 変える直前の行がその後の像と等しければ、名前を変えた後の行で後の像を読み直した像を返す
// （data-model.md「Rename under an undo token」）。等しくないか image が nil なら像を返さない。
// 前の像は変えない。
func (s *OrganizeStore) RenameGroup(ctx context.Context, id int64, name string, image []byte) (domain.Group, []byte, error) {
	name, err := domain.ValidateGroupName(name)
	if err != nil {
		return domain.Group{}, nil, err
	}
	var kept *undoImage
	if image != nil {
		decoded, err := decodeUndoImage(image)
		if err != nil {
			return domain.Group{}, nil, err
		}
		kept = &decoded
	}
	var encoded []byte
	err = writeGroups(ctx, s.db, "rename the group", func(tx *sql.Tx) ([]int64, error) {
		current, err := imageIsCurrent(ctx, tx, kept)
		if err != nil {
			return nil, err
		}
		changed, err := renameGroup(ctx, tx, id, name)
		if err != nil || !current {
			return changed, err
		}
		if encoded, err = kept.captureAfterAndEncode(ctx, tx); err != nil {
			return nil, err
		}
		return changed, nil
	})
	if err != nil {
		return domain.Group{}, nil, err
	}
	group, err := committedGroupSummary(ctx, s.db, id)
	if err != nil {
		return domain.Group{}, nil, err
	}
	return group, encoded, nil
}

// imageIsCurrent は image（nil なら false）のどの範囲も今の行が後の像と等しいかを返す。
func imageIsCurrent(ctx context.Context, tx *sql.Tx, image *undoImage) (bool, error) {
	if image == nil {
		return false, nil
	}
	err := image.ensureCurrent(ctx, tx)
	if errors.Is(err, domain.ErrUndoConflict) {
		return false, nil
	}
	return err == nil, err
}
