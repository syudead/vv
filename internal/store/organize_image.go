package store

// ドラッグの操作の取り消しの像（specs/050-drag-organize/data-model.md「Undo image」、research.md R-1）。
// 操作は自分の取引の中で、変える行を範囲（表と列の値）ごとに変更の前と後に読む。像はこのファイル
// だけがバイト列にし、読み戻すのもこのファイルだけである。取り消しは、今の行が後の像と等しい
// ときだけ範囲の行を前の像に置き換え、等しくなければ domain.ErrUndoConflict で何も書かない。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// imageScopeColumns は像に読んでよい範囲である。表ごとに、範囲を選ぶ列を挙げる。表と列の名前は
// SQL に埋め込むので、像を読み戻すときもこの表に無い名前は受け付けない。親の表（行を消すと子の
// 行が外部キーで消える表）を足すときは、範囲の並びで子より前に置く（restoreUndoImage）。
var imageScopeColumns = map[string][]string{
	"groups":                 {"id"},
	"group_items":            {"group_id", "child_id"},
	"group_exclusions":       {"group_id"},
	"group_favorites":        {"group_id"},
	"playlist_items":         {"playlist_id", "group_id"},
	"folder_group_overrides": {"path"},
}

// imageCell は行の 1 つの値である。どれも nil なら NULL。SQLite の 4 つの型をそのまま持ち、
// 読み戻しで型を取り違えない。
type imageCell struct {
	Int  *int64   `json:"i,omitempty"`
	Real *float64 `json:"r,omitempty"`
	Text *string  `json:"t,omitempty"`
	Blob *[]byte  `json:"b,omitempty"`
}

// value は cell を SQL の引数にする。
func (c imageCell) value() any {
	switch {
	case c.Int != nil:
		return *c.Int
	case c.Real != nil:
		return *c.Real
	case c.Text != nil:
		return *c.Text
	case c.Blob != nil:
		return *c.Blob
	}
	return nil
}

// newImageCell は SQLite のドライバーが返した値を cell にする。
func newImageCell(v any) (imageCell, error) {
	switch v := v.(type) {
	case nil:
		return imageCell{}, nil
	case int64:
		return imageCell{Int: &v}, nil
	case float64:
		return imageCell{Real: &v}, nil
	case string:
		return imageCell{Text: &v}, nil
	case []byte:
		b := slices.Clone(v)
		return imageCell{Blob: &b}, nil
	}
	return imageCell{}, fmt.Errorf("cannot keep a value of type %T for undo", v)
}

// imageScope は像の 1 つの範囲で、表 Table の列 Column が Key の行すべてである。Columns は行の
// 列の名前、Before と After は変更の前と後の行で、どちらも符号にした行の順に並べる（等しいかを
// 並びに左右されずに比べるため）。
type imageScope struct {
	Table   string        `json:"table"`
	Column  string        `json:"column"`
	Key     imageCell     `json:"key"`
	Columns []string      `json:"columns,omitempty"`
	Before  [][]imageCell `json:"before,omitempty"`
	After   [][]imageCell `json:"after,omitempty"`
}

// undoImage は操作 1 つの像である。Groups は取り消しの後で更新日時を進めて知らせるグループ、
// Playlists は詰め直して更新日時を進めるプレイリスト、RebuildFolders はフォルダの索引を作り直すか
// で、どれも操作が同じことをしたものである。
type undoImage struct {
	Scopes         []imageScope `json:"scopes"`
	Groups         []int64      `json:"groups,omitempty"`
	Playlists      []int64      `json:"playlists,omitempty"`
	RebuildFolders bool         `json:"rebuildFolders,omitempty"`
}

// newImageScope は表 table の列 column が key の範囲を作る。imageScopeColumns に無い組は誤りである。
func newImageScope(table, column string, key any) (imageScope, error) {
	if !slices.Contains(imageScopeColumns[table], column) {
		return imageScope{}, fmt.Errorf("cannot keep %s.%s for undo", table, column)
	}
	cell, err := newImageCell(key)
	if err != nil {
		return imageScope{}, err
	}
	return imageScope{Table: table, Column: column, Key: cell}, nil
}

// captureBefore は image の各範囲の今の行を前の像として読む。操作が書く前に呼ぶ。
func (image *undoImage) captureBefore(ctx context.Context, tx *sql.Tx) error {
	for i := range image.Scopes {
		columns, rows, err := readImageRows(ctx, tx, image.Scopes[i])
		if err != nil {
			return err
		}
		image.Scopes[i].Columns, image.Scopes[i].Before = columns, rows
	}
	return nil
}

// captureAfter は image の各範囲の今の行を後の像として読む。操作が書いた後、同じ取引で呼ぶ。
func (image *undoImage) captureAfter(ctx context.Context, tx *sql.Tx) error {
	for i := range image.Scopes {
		_, rows, err := readImageRows(ctx, tx, image.Scopes[i])
		if err != nil {
			return err
		}
		image.Scopes[i].After = rows
	}
	return nil
}

// encode は image をバイト列にする。
func (image *undoImage) encode() ([]byte, error) {
	out, err := json.Marshal(image)
	if err != nil {
		return nil, fmt.Errorf("cannot keep the rows for undo: %w", err)
	}
	return out, nil
}

// decodeUndoImage は encode のバイト列を読み戻す。表と列の名前は imageScopeColumns にあるか、
// 列の名前は識別子の形かを確かめる。
func decodeUndoImage(data []byte) (undoImage, error) {
	var image undoImage
	if err := json.Unmarshal(data, &image); err != nil {
		return undoImage{}, fmt.Errorf("cannot read the rows kept for undo: %w", err)
	}
	for _, scope := range image.Scopes {
		if !slices.Contains(imageScopeColumns[scope.Table], scope.Column) {
			return undoImage{}, fmt.Errorf("cannot restore %s.%s", scope.Table, scope.Column)
		}
		for _, column := range scope.Columns {
			if !isIdentifier(column) {
				return undoImage{}, fmt.Errorf("cannot restore the column %q", column)
			}
		}
	}
	return image, nil
}

// isIdentifier は name が英小文字・数字・_ だけで、数字で始まらないかを返す。
func isIdentifier(name string) bool {
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return false
	}
	return strings.IndexFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_'
	}) < 0
}

// readImageRows は範囲 scope の今の行すべてを、列の名前と、符号にした行の順に並べた値で返す。
func readImageRows(ctx context.Context, tx *sql.Tx, scope imageScope) ([]string, [][]imageCell, error) {
	rows, err := tx.QueryContext(ctx, `select * from `+scope.Table+` where `+scope.Column+` = ?`, scope.Key.value())
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the rows for undo: %w", err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the rows for undo: %w", err)
	}
	var out [][]imageCell
	for rows.Next() {
		row, err := scanImageRow(rows, len(columns))
		if err != nil {
			return nil, nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("cannot read the rows for undo: %w", err)
	}
	if err := sortImageRows(out); err != nil {
		return nil, nil, err
	}
	return columns, out, nil
}

// scanImageRow は rows の今の行の n 個の値を cell にする。
func scanImageRow(rows *sql.Rows, n int) ([]imageCell, error) {
	values := make([]any, n)
	targets := make([]any, n)
	for i := range values {
		targets[i] = &values[i]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, fmt.Errorf("cannot read the rows for undo: %w", err)
	}
	row := make([]imageCell, n)
	for i, v := range values {
		cell, err := newImageCell(v)
		if err != nil {
			return nil, err
		}
		row[i] = cell
	}
	return row, nil
}

// sortImageRows は rows を符号にした行の順に並べる。
func sortImageRows(rows [][]imageCell) error {
	type keyedRow struct {
		key string
		row []imageCell
	}
	keyed := make([]keyedRow, len(rows))
	for i, row := range rows {
		encoded, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("cannot keep the rows for undo: %w", err)
		}
		keyed[i] = keyedRow{key: string(encoded), row: row}
	}
	slices.SortFunc(keyed, func(a, b keyedRow) int { return strings.Compare(a.key, b.key) })
	for i := range keyed {
		rows[i] = keyed[i].row
	}
	return nil
}

// restoreUndoImage は image の各範囲の今の行が後の像と等しいことを確かめ、範囲の行を前の像に
// 置き換える（R-1）。等しくない範囲が 1 つでもあれば domain.ErrUndoConflict で、何も書かない
// （呼び出し側が取引を巻き戻す）。範囲は後ろから消し、前から入れるので、親の表の範囲を先に
// 置けば、外部キーで消える子の行も前の像から戻る。そのあと操作と同じく、プレイリストを詰め直して
// 更新日時を進め、フォルダの索引を作り直し、グループの更新日時を進めて、変えたグループと祖先の
// id を返す。
func restoreUndoImage(ctx context.Context, tx *sql.Tx, image undoImage) ([]int64, error) {
	for _, scope := range image.Scopes {
		if err := ensureImageUnchanged(ctx, tx, scope); err != nil {
			return nil, err
		}
	}
	for _, scope := range slices.Backward(image.Scopes) {
		if _, err := tx.ExecContext(ctx, `delete from `+scope.Table+` where `+scope.Column+` = ?`, scope.Key.value()); err != nil {
			return nil, fmt.Errorf("cannot undo: %w", err)
		}
	}
	for _, scope := range image.Scopes {
		if err := insertImageRows(ctx, tx, scope); err != nil {
			return nil, err
		}
	}
	return finishUndo(ctx, tx, image)
}

// ensureImageUnchanged は範囲 scope の今の行が後の像と等しいかを確かめ、違えば domain.ErrUndoConflict。
func ensureImageUnchanged(ctx context.Context, tx *sql.Tx, scope imageScope) error {
	columns, rows, err := readImageRows(ctx, tx, scope)
	if err != nil {
		return err
	}
	if !slices.Equal(columns, scope.Columns) {
		return domain.ErrUndoConflict
	}
	now, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("cannot undo: %w", err)
	}
	want, err := json.Marshal(scope.After)
	if err != nil {
		return fmt.Errorf("cannot undo: %w", err)
	}
	if string(now) != string(want) {
		return domain.ErrUndoConflict
	}
	return nil
}

// insertImageRows は範囲 scope の前の像の行を、元の id のまま入れる。範囲が重なって既に入れた行は
// 置き換える。
func insertImageRows(ctx context.Context, tx *sql.Tx, scope imageScope) error {
	if len(scope.Before) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(scope.Columns)), ", ")
	statement := `insert or replace into ` + scope.Table + ` (` + strings.Join(scope.Columns, ", ") + `) values (` + placeholders + `)`
	for _, row := range scope.Before {
		if len(row) != len(scope.Columns) {
			return errors.New("cannot undo: the kept row does not match its columns")
		}
		args := make([]any, len(row))
		for i, cell := range row {
			args[i] = cell.value()
		}
		if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
			return fmt.Errorf("cannot undo: %w", err)
		}
	}
	return nil
}

// finishUndo は行を戻した後で、操作と同じくプレイリストを詰め直して更新日時を進め、フォルダの
// 索引を作り直し、今もあるグループの更新日時を進めて、変えたグループと祖先の id を返す。
func finishUndo(ctx context.Context, tx *sql.Tx, image undoImage) ([]int64, error) {
	if err := renumberPlaylists(ctx, tx, image.Playlists); err != nil {
		return nil, err
	}
	for _, id := range image.Playlists {
		if err := touchPlaylist(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	var changed []int64
	if image.RebuildFolders {
		rebuilt, err := rebuildFolderIndex(ctx, tx)
		if err != nil {
			return nil, err
		}
		changed = rebuilt
	}
	var present []int64
	for _, id := range image.Groups {
		if _, err := groupSummary(ctx, tx, id); errors.Is(err, domain.ErrGroupNotFound) {
			changed = append(changed, id)
			continue
		} else if err != nil {
			return nil, err
		}
		present = append(present, id)
	}
	touched, err := touchedGroupsWithAncestors(ctx, tx, present)
	if err != nil {
		return nil, err
	}
	return normalizeIDs(append(changed, touched...)), nil
}
