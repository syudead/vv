package store

import (
	"context"
	"database/sql"
	"fmt"
)

// positionTable は、親ごとに 0 から詰めた位置（position）を持つ項目の表である。プレイリストの
// 項目（specs/044-playlists/research.md R-3）とグループの項目（specs/048-groups/data-model.md）が
// 同じ詰め直しを使う。
type positionTable struct {
	// table は項目の表、parent は親を指す列の名前である。どちらも定数で、利用者の入力ではない。
	table  string
	parent string
	// noun は誤りの文に使う親の呼び名である。
	noun string
}

var (
	playlistPositions = positionTable{table: "playlist_items", parent: "playlist_id", noun: "playlist"}
	groupPositions    = positionTable{table: "group_items", parent: "group_id", noun: "group"}
)

// renumberPositions は parentIDs の親の項目の位置を、今の並び（位置、同じなら id）のまま 0 から
// 詰め直す。並びを先に読み切ってから書く。位置を相関副問い合わせで数える 1 文の update は、同じ
// 文で既に書き換えた行の位置を数えてしまい、位置が重なる。
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

// positionOrder は親 id の全項目の id を位置の順に返す。
func positionOrder(ctx context.Context, tx *sql.Tx, t positionTable, id int64) ([]int64, error) {
	ids, err := queryInt64s(ctx, tx, `select id from `+t.table+` where `+t.parent+` = ? order by position, id`, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the %s items: %w", t.noun, err)
	}
	return ids, nil
}

// writePositionOrder は order の項目の位置を、その並びの 0 からの番号にする。
func writePositionOrder(ctx context.Context, tx *sql.Tx, t positionTable, order []int64) error {
	for position, itemID := range order {
		if _, err := tx.ExecContext(ctx, `update `+t.table+` set position = ? where id = ?`, position, itemID); err != nil {
			return fmt.Errorf("cannot reorder the %s: %w", t.noun, err)
		}
	}
	return nil
}

// deleteRekeyDuplicates は、鍵 from を to へ付け替える前に、1 つの親に鍵 from と to の項目が
// 両方あるとき位置の大きい方（同じなら id の大きい方）を消す。付け替えの後に同じ親で鍵が
// 重ならず、前の位置が残る（specs/044-playlists/research.md R-2）。
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
