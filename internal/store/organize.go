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
// targetID、続いて videoIDs の順に重複なく返す。動画が無ければ domain.ErrGroupTargetNotFound。
func groupOperationMembers(ctx context.Context, tx *sql.Tx, videoIDs []int64, targetID int64) (string, []string, error) {
	keys, err := groupAddKeys(ctx, tx, append([]int64{targetID}, videoIDs...))
	if err != nil {
		return "", nil, err
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
