package store

// Capture before/after images in each drag transaction; missing targets roll back and events publish only after commit.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Reorder preserves selection order and rejects missing targets or insertion after a selected item.
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

// Move appends before removing source items, records folder exclusions, and dissolves empty source ancestors atomically.
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

// Append before removing so dissolving the source cannot remove the destination.
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

func nonZeroIDs(ids ...int64) []int64 {
	return slices.DeleteFunc(ids, func(id int64) bool { return id == 0 })
}

// Undo restores only if current rows match the after image; otherwise return domain.ErrUndoConflict without changes.
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
		c := &changes{}
		now := time.Now()
		if err := reopenRestoredPairActivities(ctx, tx, c, now); err != nil {
			return nil, err
		}
		if err := recordOrganizeUndo(ctx, tx, c, decoded.Videos, now); err != nil {
			return nil, err
		}
		events := c.events()
		if len(decoded.Videos) > 0 {
			events = append(events, domain.VideoBundleChanged{VideoIDs: decoded.Videos})
		}
		return append(events, groupsChangedEvents(changed)...), nil
	})
}

// Record owner undo only for changed bundles; group-only undo changes neither tags nor bundles.
func recordOrganizeUndo(ctx context.Context, tx *sql.Tx, c *changes, videoIDs []int64, now time.Time) error {
	keys, err := registeredContentKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return err
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	return recordOwnerVideoActivity(ctx, tx, c, domain.ActivityOrganizeUndo, ownerVideoChanges(keys, ""), now)
}

func ensureGroupTarget(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := groupSummary(ctx, tx, id)
	if errors.Is(err, domain.ErrGroupNotFound) {
		return domain.ErrGroupTargetNotFound
	}
	return err
}

// Group creates a named group with target first and deduplicated user keys; within a source group it replaces the target's position.
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
		// Advance the new group's timestamp before capturing its after image.
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

// No source yields an empty image; every selected key must exist in a supplied source.
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

// Replace the target's position with the new child group, recording exclusions for removed folder members.
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

// RenameGroup refreshes only a still-current undo after image; preserve the before image and drop stale images.
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
