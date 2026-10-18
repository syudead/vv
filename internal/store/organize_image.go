package store

// Undo replaces scoped rows only when they match the captured after image; images are encoded and decoded exclusively here.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Whitelist interpolated table/column names and order parent scopes before children; include references that would otherwise cascade silently during undo.
var imageScopeColumns = buildImageScopeColumns()

func buildImageScopeColumns() map[string][]string {
	columns := map[string][]string{
		"groups":                   {"id"},
		"group_items":              {"group_id", "child_id", "content_key"},
		"group_exclusions":         {"group_id", "content_key"},
		"group_favorites":          {"group_id"},
		"playlist_items":           {"playlist_id", "group_id", "content_key"},
		"folder_group_overrides":   {"path"},
		"video_bundles":            {"id"},
		"video_bundle_members":     {"bundle_id"},
		"video_version_candidates": {"key_a"},
	}
	for _, t := range userDataTables {
		columns[t.table] = append(columns[t.table], "content_key")
	}
	return columns
}

// Preserve SQLite value types when round-tripping images; all nil fields represent NULL.
type imageCell struct {
	Int  *int64   `json:"i,omitempty"`
	Real *float64 `json:"r,omitempty"`
	Text *string  `json:"t,omitempty"`
	Blob *[]byte  `json:"b,omitempty"`
}

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

// Sort encoded rows before comparing images so query order cannot create false conflicts.
type imageScope struct {
	Table   string        `json:"table"`
	Column  string        `json:"column"`
	Key     imageCell     `json:"key"`
	Columns []string      `json:"columns,omitempty"`
	Before  [][]imageCell `json:"before,omitempty"`
	After   [][]imageCell `json:"after,omitempty"`
}

// Track the same timestamp, compaction, rebuild and notification work performed by the original operation.
type undoImage struct {
	Scopes         []imageScope `json:"scopes"`
	Groups         []int64      `json:"groups,omitempty"`
	Playlists      []int64      `json:"playlists,omitempty"`
	RebuildFolders bool         `json:"rebuildFolders,omitempty"`
	Videos         []int64      `json:"videos,omitempty"`
}

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

// Capture parents before dependents, including references added later, so undo reports conflicts instead of cascading away new references.
func groupReferenceScopes(id int64) ([]imageScope, error) {
	var scopes []imageScope
	for _, pair := range [][2]string{
		{"groups", "id"}, {"group_items", "group_id"}, {"group_items", "child_id"},
		{"group_favorites", "group_id"}, {"playlist_items", "group_id"},
	} {
		scope, err := newImageScope(pair[0], pair[1], id)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

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

// Capture in the same transaction after writing; newly created scopes also acquire column names here.
func (image *undoImage) captureAfter(ctx context.Context, tx *sql.Tx) error {
	for i := range image.Scopes {
		columns, rows, err := readImageRows(ctx, tx, image.Scopes[i])
		if err != nil {
			return err
		}
		if image.Scopes[i].Columns == nil {
			image.Scopes[i].Columns = columns
		}
		image.Scopes[i].After = rows
	}
	return nil
}

func (image *undoImage) captureAfterAndEncode(ctx context.Context, tx *sql.Tx) ([]byte, error) {
	if err := image.captureAfter(ctx, tx); err != nil {
		return nil, err
	}
	return image.encode()
}

func (image *undoImage) encode() ([]byte, error) {
	out, err := json.Marshal(image)
	if err != nil {
		return nil, fmt.Errorf("cannot keep the rows for undo: %w", err)
	}
	return out, nil
}

// Validate decoded scope names against the whitelist and require identifier-shaped column names.
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

func isIdentifier(name string) bool {
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return false
	}
	return strings.IndexFunc(name, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_'
	}) < 0
}

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

// Validate every after image before restoring; delete scopes in reverse order and insert forward so parent-first scopes preserve foreign-key dependents.
func restoreUndoImage(ctx context.Context, tx *sql.Tx, image undoImage) ([]int64, error) {
	if err := image.ensureCurrent(ctx, tx); err != nil {
		return nil, err
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

func (image *undoImage) ensureCurrent(ctx context.Context, tx *sql.Tx) error {
	for _, scope := range image.Scopes {
		if err := ensureImageUnchanged(ctx, tx, scope); err != nil {
			return err
		}
	}
	return nil
}

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

// Restore original IDs and replace rows already restored through overlapping scopes.
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

// Capture every cascading dependent and whole affected playlists; later group references must cause undo conflicts rather than silent deletion.
func groupsImage(ctx context.Context, tx *sql.Tx, ids []int64) (undoImage, error) {
	ids = normalizeIDs(slices.Clone(ids))
	encoded, err := json.Marshal(ids)
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot build group ids: %w", err)
	}
	playlists, err := queryInt64s(ctx, tx, `select distinct playlist_id from playlist_items
		where group_id in (select value from json_each(?)) order by playlist_id`, string(encoded))
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot read the playlists: %w", err)
	}
	folders, err := queryStrings(ctx, tx, `select origin_folder from groups
		where origin_folder is not null and id in (select value from json_each(?)) order by origin_folder`, string(encoded))
	if err != nil {
		return undoImage{}, fmt.Errorf("cannot read the folder groups: %w", err)
	}
	var specs []imageScopeSpec
	for _, spec := range []imageScopeSpec{{"groups", "id", nil}, {"group_items", "group_id", nil},
		{"group_items", "child_id", nil}, {"group_exclusions", "group_id", nil}, {"group_favorites", "group_id", nil},
		{"playlist_items", "group_id", nil}} {
		for _, id := range ids {
			specs = append(specs, imageScopeSpec{spec.table, spec.column, id})
		}
	}
	for _, id := range playlists {
		specs = append(specs, imageScopeSpec{"playlist_items", "playlist_id", id})
	}
	for _, folder := range folders {
		specs = append(specs, imageScopeSpec{"folder_group_overrides", "path", domain.FolderKey(folder)})
	}
	image := undoImage{Scopes: make([]imageScope, 0, len(specs))}
	if err := image.addScopes(specs); err != nil {
		return undoImage{}, err
	}
	return image, nil
}

type imageScopeSpec struct {
	table  string
	column string
	key    any
}

func (image *undoImage) finishGroupsImage(groups []int64) {
	image.Groups = normalizeIDs(slices.Clone(groups))
	for _, scope := range image.Scopes {
		if reflect.DeepEqual(scope.Before, scope.After) {
			continue
		}
		switch scope.Table {
		case "playlist_items":
			if scope.Column == "playlist_id" && scope.Key.Int != nil {
				image.Playlists = append(image.Playlists, *scope.Key.Int)
			}
		case "folder_group_overrides":
			image.RebuildFolders = true
		}
	}
}
