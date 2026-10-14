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
		if err := image.captureAfter(ctx, tx); err != nil {
			return nil, err
		}
		if encoded, err = image.encode(); err != nil {
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
	itemIDs := make([]int64, 0, len(members))
	for _, member := range members {
		itemID, err := groupMemberItemID(ctx, tx, move.From, member)
		if err != nil {
			return nil, nil, err
		}
		itemIDs = append(itemIDs, itemID)
	}
	return members, itemIDs, nil
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
