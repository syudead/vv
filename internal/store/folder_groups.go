package store

// フォルダのグループの索引と、フォルダごとの例外（specs/017-folder-groups/data-model.md §1〜§3）。
// 例外の書き換えは FolderGroupStore、スキャンの後と起動時の作り直しは ScanIndexStore、
// メディアフォルダの変更に伴う作り直しは SettingsStore が受け持ち、どれも
// rebuildFolderIndex を自分の取引の中で呼ぶ。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// SetOverride はフォルダ folderPath（絶対パス）に例外 mode を付け、同じ取引で
// 索引を作り直す。既に例外があれば置き換える。1つのフォルダに付く例外は1つだけ
// である。folderPath に一致するフォルダが今無くても例外は保存する。
func (s *FolderGroupStore) SetOverride(ctx context.Context, folderPath string, mode domain.FolderGroupMode) error {
	if !mode.Valid() {
		return fmt.Errorf("フォルダのまとめ方が正しくありません: %q", mode)
	}
	_, err := s.SetFolderGrouping(ctx, folderPath, mode)
	return err
}

// ClearOverride はフォルダ folderPath（絶対パス）の例外を外し、同じ取引で索引を
// 作り直す。例外が無ければ何も変えずに作り直す。
func (s *FolderGroupStore) ClearOverride(ctx context.Context, folderPath string) error {
	_, err := s.SetFolderGrouping(ctx, folderPath, "")
	return err
}

// SetFolderGrouping はフォルダ folderPath（絶対パス）の例外を mode にし、同じ取引で
// 索引を作り直して、変更後のまとめ方を返す（PUT /api/folders/{rootId}/grouping、
// specs/017-folder-groups/contracts/folder-groups-api.md §1）。mode が空なら例外を
// 外す（自動）。同じ値の再設定も誤りにしない。
func (s *FolderGroupStore) SetFolderGrouping(ctx context.Context, folderPath string, mode domain.FolderGroupMode) (domain.FolderGrouping, error) {
	if mode != "" && !mode.Valid() {
		return domain.FolderGrouping{}, fmt.Errorf("フォルダのまとめ方が正しくありません: %q", mode)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FolderGrouping{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := writeOverride(ctx, tx, folderPath, mode); err != nil {
		return domain.FolderGrouping{}, err
	}
	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return domain.FolderGrouping{}, err
	}
	groupings, err := folderGroupings(ctx, tx, []string{folderPath})
	if err != nil {
		return domain.FolderGrouping{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FolderGrouping{}, fmt.Errorf("フォルダのまとめ方を保存できません: %w", err)
	}
	return groupings[0], nil
}

// TagFolderGroup はフォルダ folderPath（絶対パス）のグループをタグに変える
// （POST /api/folders/{rootId}/grouping/tag、specs/017-folder-groups/data-model.md §4）。
// 1つの取引で、フォルダ名を domain.NormalizeTagName に通し、名前かシノニムで引けた
// タグを使うか新しく作り、そのフォルダに ungroup を書き、索引を作り直す。
//
// そのフォルダが今グループでなければ domain.ErrNotFolderGroup、フォルダ名がタグ名の
// 規則に合わなければ domain.ErrInvalidTagName を返し、どちらも何も書かない。
// 登録フォルダそのものかどうかは呼び出し側が確かめる。
func (s *FolderGroupStore) TagFolderGroup(ctx context.Context, folderPath string) (domain.FolderGroupTag, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FolderGroupTag{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var name string
	err = tx.QueryRowContext(ctx, `select name from folder_groups where path_key = ?`, domain.FolderKey(folderPath)).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.FolderGroupTag{}, domain.ErrNotFolderGroup
	}
	if err != nil {
		return domain.FolderGroupTag{}, fmt.Errorf("グループを読み出せません: %w", err)
	}
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.FolderGroupTag{}, err
	}
	tag, created, err := findOrCreateTag(ctx, tx, normalized)
	if err != nil {
		return domain.FolderGroupTag{}, err
	}
	if err := writeOverride(ctx, tx, folderPath, domain.FolderGroupUngroup); err != nil {
		return domain.FolderGroupTag{}, err
	}
	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return domain.FolderGroupTag{}, err
	}
	groupings, err := folderGroupings(ctx, tx, []string{folderPath})
	if err != nil {
		return domain.FolderGroupTag{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.FolderGroupTag{}, fmt.Errorf("グループをタグに変えられません: %w", err)
	}
	return domain.FolderGroupTag{Tag: tag, Created: created, Grouping: groupings[0]}, nil
}

// FolderGroupings はフォルダ folderPaths（絶対パス）それぞれのまとめ方を、同じ
// 読み取りのスナップショットから folderPaths と同じ順で返す（FolderSummary.grouping）。
func (s *FolderGroupStore) FolderGroupings(ctx context.Context, folderPaths []string) ([]domain.FolderGrouping, error) {
	if len(folderPaths) == 0 {
		return []domain.FolderGrouping{}, nil
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("フォルダのまとめ方の読み取りを始められません: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	groupings, err := folderGroupings(ctx, tx, folderPaths)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("フォルダのまとめ方の読み取りを終えられません: %w", err)
	}
	return groupings, nil
}

// folderGroupings はフォルダそれぞれの例外と、いまグループかどうかを引く。
func folderGroupings(ctx context.Context, tx *sql.Tx, folderPaths []string) ([]domain.FolderGrouping, error) {
	keys := make([]string, len(folderPaths))
	for i, path := range folderPaths {
		keys[i] = domain.FolderKey(path)
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("フォルダの鍵を組み立てられません: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		with wanted(folder_key) as (select distinct value from json_each(?))
		select w.folder_key, o.mode, g.id is not null
		  from wanted w
		  left join folder_group_overrides o on o.path = w.folder_key
		  left join folder_groups g on g.path_key = w.folder_key`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("フォルダのまとめ方を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byKey := map[string]domain.FolderGrouping{}
	for rows.Next() {
		var key string
		var mode sql.NullString
		var grouping domain.FolderGrouping
		if err := rows.Scan(&key, &mode, &grouping.Grouped); err != nil {
			return nil, fmt.Errorf("フォルダのまとめ方を読み出せません: %w", err)
		}
		grouping.Mode = domain.FolderGroupMode(mode.String)
		byKey[key] = grouping
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("フォルダのまとめ方を読み出せません: %w", err)
	}
	out := make([]domain.FolderGrouping, len(keys))
	for i, key := range keys {
		out[i] = byKey[key]
	}
	return out, nil
}

// writeOverride はフォルダ folderPath の例外を mode にする。mode が空なら外す。
func writeOverride(ctx context.Context, tx *sql.Tx, folderPath string, mode domain.FolderGroupMode) error {
	var err error
	if mode == "" {
		_, err = tx.ExecContext(ctx, `delete from folder_group_overrides where path = ?`, domain.FolderKey(folderPath))
	} else {
		_, err = tx.ExecContext(ctx, `insert into folder_group_overrides (path, mode, updated_at) values (?, ?, ?)
			on conflict (path) do update set mode = excluded.mode, updated_at = excluded.updated_at`,
			domain.FolderKey(folderPath), string(mode), time.Now().Unix())
	}
	if err != nil {
		return fmt.Errorf("フォルダのまとめ方を保存できません: %w", err)
	}
	return nil
}

// RebuildFolderIndex はフォルダの索引を作り直す。スキャンを閉じる直前と、起動時に
// 中断したスキャンを閉じたときに呼ぶ。
//
// 作り直しに失敗したら、その取引は巻き戻して前の索引を残し、別の取引で索引を
// 古い（stale）と記録してから失敗を返す。次の起動で RefreshFolderIndex が作り直す。
func (s *ScanIndexStore) RebuildFolderIndex(ctx context.Context) error {
	err := s.rebuildFolderIndexTx(ctx)
	if err == nil {
		return nil
	}
	if markErr := markFolderIndexStale(ctx, s.db.sql); markErr != nil {
		return errors.Join(err, markErr)
	}
	return err
}

// RefreshFolderIndex は、索引を作った規則の版が今の値と違うとき、前回の作り直しが
// 失敗していたとき、まだ一度も作っていないときだけ索引を作り直し、作り直したか
// どうかを返す。起動時、RefreshSearchKeys の後で、HTTP とワーカーの開始より前に呼ぶ。
func (s *ScanIndexStore) RefreshFolderIndex(ctx context.Context) (bool, error) {
	var version, searchVersion, stale int
	err := s.db.sql.QueryRowContext(ctx,
		`select version, search_version, stale from folder_index_state where id = 1`).
		Scan(&version, &searchVersion, &stale)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return false, fmt.Errorf("フォルダの索引の状態を読み出せません: %w", err)
	case version == domain.FolderIndexVersion && searchVersion == domain.SearchKeyVersion && stale == 0:
		return false, nil
	}
	if err := s.RebuildFolderIndex(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *ScanIndexStore) rebuildFolderIndexTx(ctx context.Context) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := rebuildFolderIndex(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// markFolderIndexStale は索引が古いことを記録する。行が無ければ版 0 で作るので、
// 次の起動でも作り直す。
func markFolderIndexStale(ctx context.Context, q queryExecer) error {
	if _, err := q.ExecContext(ctx, `insert into folder_index_state (id, version, search_version, stale) values (1, 0, 0, 1)
		on conflict (id) do update set stale = 1`); err != nil {
		return fmt.Errorf("フォルダの索引の状態を保存できません: %w", err)
	}
	return nil
}

// rebuildFolderIndex は、登録フォルダの下の全所在と例外を読み、
// domain.BuildFolderIndex を通して、グループとフォルダ名の表を丸ごと書き直す
// （specs/017-folder-groups/data-model.md §3）。呼び出し側の書き込み取引の中で
// 行うので、読み出しは作り直しの前か後のどちらかだけを見る。ファイルシステムは読まない。
func rebuildFolderIndex(ctx context.Context, tx *sql.Tx) error {
	roots, err := listMediaFolders(ctx, tx)
	if err != nil {
		return err
	}
	locations, err := folderIndexLocations(ctx, tx)
	if err != nil {
		return err
	}
	overrides, err := folderGroupOverrides(ctx, tx)
	if err != nil {
		return err
	}
	index := domain.BuildFolderIndex(roots, locations, overrides)
	if err := writeFolderIndex(ctx, tx, index); err != nil {
		return fmt.Errorf("フォルダの索引を保存できません: %w", err)
	}
	return nil
}

func folderIndexLocations(ctx context.Context, tx *sql.Tx) ([]domain.FolderIndexLocation, error) {
	rows, err := tx.QueryContext(ctx, `select video_id, path from video_locations`)
	if err != nil {
		return nil, fmt.Errorf("所在を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var locations []domain.FolderIndexLocation
	for rows.Next() {
		var location domain.FolderIndexLocation
		if err := rows.Scan(&location.VideoID, &location.Path); err != nil {
			return nil, fmt.Errorf("所在を読み出せません: %w", err)
		}
		locations = append(locations, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("所在を読み出せません: %w", err)
	}
	return locations, nil
}

func folderGroupOverrides(ctx context.Context, tx *sql.Tx) (map[string]domain.FolderGroupMode, error) {
	rows, err := tx.QueryContext(ctx, `select path, mode from folder_group_overrides`)
	if err != nil {
		return nil, fmt.Errorf("フォルダのまとめ方を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	overrides := map[string]domain.FolderGroupMode{}
	for rows.Next() {
		var path, mode string
		if err := rows.Scan(&path, &mode); err != nil {
			return nil, fmt.Errorf("フォルダのまとめ方を読み出せません: %w", err)
		}
		overrides[path] = domain.FolderGroupMode(mode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("フォルダのまとめ方を読み出せません: %w", err)
	}
	return overrides, nil
}

func writeFolderIndex(ctx context.Context, tx *sql.Tx, index domain.FolderIndex) error {
	for _, statement := range []string{
		`delete from folder_group_members`,
		`delete from folder_groups`,
		`delete from video_folder_names`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}

	insertGroup, err := tx.PrepareContext(ctx, `insert into folder_groups (path_key, path, name, title_key) values (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = insertGroup.Close() }()
	insertMember, err := tx.PrepareContext(ctx, `insert into folder_group_members (video_id, group_id, position) values (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = insertMember.Close() }()
	for _, group := range index.Groups {
		res, err := insertGroup.ExecContext(ctx, group.Key, group.Path, group.Name, group.TitleKey)
		if err != nil {
			return err
		}
		groupID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for position, videoID := range group.VideoIDs {
			if _, err := insertMember.ExecContext(ctx, videoID, groupID, position); err != nil {
				return err
			}
		}
	}

	insertName, err := tx.PrepareContext(ctx, `insert into video_folder_names (video_id, name) values (?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = insertName.Close() }()
	for videoID, names := range index.FolderNames {
		for _, name := range names {
			if _, err := insertName.ExecContext(ctx, videoID, name); err != nil {
				return err
			}
		}
	}

	_, err = tx.ExecContext(ctx, `insert into folder_index_state (id, version, search_version, stale) values (1, ?, ?, 0)
		on conflict (id) do update set version = excluded.version, search_version = excluded.search_version, stale = 0`,
		domain.FolderIndexVersion, domain.SearchKeyVersion)
	return err
}
