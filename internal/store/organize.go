package store

// ドラッグの操作と取り消し（specs/050-drag-organize/data-model.md「Operations」）。OrganizeStore の
// 操作はどれも 1 つの取引で、変える行の前後の像（organize_image.go）を同じ取引で読んで返し、確定後に
// domain.GroupsChanged を発行する。要求が名指したグループや項目が無ければ
// domain.ErrGroupTargetNotFound で、何も書かない。

import (
	"context"
	"database/sql"
	"errors"

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

// Undo は操作が返した像 image の前の状態へ戻す（research.md R-1）。今の行が後の像と等しくなければ
// domain.ErrUndoConflict で、何も書かない。戻したグループとその祖先を確定後に知らせる。
func (s *OrganizeStore) Undo(ctx context.Context, image []byte) error {
	decoded, err := decodeUndoImage(image)
	if err != nil {
		return err
	}
	return writeGroups(ctx, s.db, "undo", func(tx *sql.Tx) ([]int64, error) {
		return restoreUndoImage(ctx, tx, decoded)
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
