package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

type positionTable struct {
	// Table and parent-column names are trusted constants, never user input.
	table  string
	parent string
	noun   string
}

var (
	playlistPositions = positionTable{table: "playlist_items", parent: "playlist_id", noun: "playlist"}
	groupPositions    = positionTable{table: "group_items", parent: "group_id", noun: "group"}
)

// Read the complete order before writing; a correlated UPDATE can count already-renumbered rows and create duplicate positions.
func renumberPositions(ctx context.Context, tx *sql.Tx, t positionTable, parentIDs []int64) error {
	for _, id := range parentIDs {
		order, err := positionOrder(ctx, tx, t, id)
		if err != nil {
			return err
		}
		if err := writePositionOrder(ctx, tx, t, order); err != nil {
			return err
		}
	}
	return nil
}

func positionOrder(ctx context.Context, tx *sql.Tx, t positionTable, id int64) ([]int64, error) {
	ids, err := queryInt64s(ctx, tx, `select id from `+t.table+` where `+t.parent+` = ? order by position, id`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the %s items: %w", t.noun, err)
	}
	return ids, nil
}

func writePositionOrder(ctx context.Context, tx *sql.Tx, t positionTable, order []int64) error {
	for position, itemID := range order {
		if _, err := tx.ExecContext(ctx, `update `+t.table+` set position = ? where id = ?`, position, itemID); err != nil {
			return fmt.Errorf("cannot reorder the %s: %w", t.noun, err)
		}
	}
	return nil
}

// Delete the later duplicate before rekeying so each parent retains its earliest position.
func deleteRekeyDuplicates(ctx context.Context, tx *sql.Tx, t positionTable, from, to string) error {
	if _, err := tx.ExecContext(ctx, `delete from `+t.table+` where content_key in (?1, ?2)
		and exists (select 1 from `+t.table+` o where o.`+t.parent+` = `+t.table+`.`+t.parent+`
			and o.content_key in (?1, ?2) and o.id <> `+t.table+`.id
			and (o.position < `+t.table+`.position or (o.position = `+t.table+`.position and o.id < `+t.table+`.id)))`,
		from, to); err != nil {
		return fmt.Errorf("cannot move the %s items: %w", t.noun, err)
	}
	return nil
}

// Callers translate missing-position targets into their own domain errors.
var errPositionItemNotFound = errors.New("item not in its parent")

// Nil means first; moving after itself returns false without changes.
func moveItemAfter(ctx context.Context, tx *sql.Tx, t positionTable, parentID, itemID int64, afterItemID *int64) (bool, error) {
	order, err := positionOrder(ctx, tx, t, parentID)
	if err != nil {
		return false, err
	}
	if afterItemID != nil && *afterItemID == itemID {
		if !slices.Contains(order, itemID) {
			return false, errPositionItemNotFound
		}
		return false, nil
	}
	placed, err := placeAfter(order, []int64{itemID}, afterItemID)
	if err != nil {
		return false, err
	}
	return true, writePositionOrder(ctx, tx, t, placed)
}

func placeItemsAfter(ctx context.Context, tx *sql.Tx, t positionTable, parentID int64, itemIDs []int64, afterItemID *int64) error {
	order, err := positionOrder(ctx, tx, t, parentID)
	if err != nil {
		return err
	}
	placed, err := placeAfter(order, itemIDs, afterItemID)
	if err != nil {
		return err
	}
	return writePositionOrder(ctx, tx, t, placed)
}

// Deduplicate selected IDs in order; reject missing IDs or insertion after a selected item.
func placeAfter(order, itemIDs []int64, afterItemID *int64) ([]int64, error) {
	present := make(map[int64]bool, len(order))
	for _, id := range order {
		present[id] = true
	}
	movedSet := make(map[int64]bool, len(itemIDs))
	moved := make([]int64, 0, len(itemIDs))
	for _, id := range itemIDs {
		if !present[id] {
			return nil, errPositionItemNotFound
		}
		if !movedSet[id] {
			movedSet[id] = true
			moved = append(moved, id)
		}
	}
	rest := slices.DeleteFunc(slices.Clone(order), func(id int64) bool { return movedSet[id] })
	to := 0
	if afterItemID != nil {
		at := slices.Index(rest, *afterItemID)
		if at < 0 {
			return nil, errPositionItemNotFound
		}
		to = at + 1
	}
	return slices.Insert(rest, to, moved...), nil
}
