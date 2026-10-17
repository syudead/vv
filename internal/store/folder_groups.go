package store

// フォルダの索引（動画ごとの祖先フォルダ名）と、フォルダごとの例外。グループそのものは groups と group_items が持ち、作り直しの同期（syncFolderGroups）が
// フォルダとそろえる。
// 例外の書き換えは FolderGroupStore、スキャンの後と起動時の作り直しは ScanIndexStore、
// メディアフォルダの変更に伴う作り直しは SettingsStore、束ねの変更に伴う作り直しは
// VersionStoreが受け持ち、どれも
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
		return fmt.Errorf("invalid folder grouping: %q", mode)
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
// docs/design-docs/catalog-organization.md）。mode が空なら例外を
// 外す（自動）。同じ値の再設定も誤りにしない。
func (s *FolderGroupStore) SetFolderGrouping(ctx context.Context, folderPath string, mode domain.FolderGroupMode) (domain.FolderGrouping, error) {
	if mode != "" && !mode.Valid() {
		return domain.FolderGrouping{}, fmt.Errorf("invalid folder grouping: %q", mode)
	}
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.FolderGrouping{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := writeOverride(ctx, tx, folderPath, mode); err != nil {
		return domain.FolderGrouping{}, err
	}
	changedGroups, err := dissolveUngroupedAndRebuild(ctx, tx, folderPath, mode)
	if err != nil {
		return domain.FolderGrouping{}, err
	}
	groupings, err := folderGroupings(ctx, tx, []string{folderPath})
	if err != nil {
		return domain.FolderGrouping{}, err
	}
	if err := s.db.commitEvents(ctx, tx, groupsChangedEvents(changedGroups)...); err != nil {
		return domain.FolderGrouping{}, fmt.Errorf("cannot save the folder grouping: %w", err)
	}
	return groupings[0], nil
}

// FolderGroupings はフォルダ folderPaths（絶対パス）それぞれのまとめ方を、同じ
// 読み取りのスナップショットから folderPaths と同じ順で返す（FolderSummary.grouping）。
func (s *FolderGroupStore) FolderGroupings(ctx context.Context, folderPaths []string) ([]domain.FolderGrouping, error) {
	if len(folderPaths) == 0 {
		return []domain.FolderGrouping{}, nil
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("cannot start reading folder groupings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	groupings, err := folderGroupings(ctx, tx, folderPaths)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot finish reading folder groupings: %w", err)
	}
	return groupings, nil
}

// folderGroupings はフォルダそれぞれの例外と、そのフォルダの自動グループ（groups.origin_folder）の
// id を引く。
func folderGroupings(ctx context.Context, tx *sql.Tx, folderPaths []string) ([]domain.FolderGrouping, error) {
	keys := make([]string, len(folderPaths))
	for i, path := range folderPaths {
		keys[i] = domain.FolderKey(path)
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build folder keys: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		with wanted(folder_key) as (select distinct value from json_each(?))
		select w.folder_key, o.mode, g.id
		  from wanted w
		  left join folder_group_overrides o on o.path = w.folder_key
		  left join groups g on g.origin_folder = w.folder_key`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot read folder groupings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byKey := map[string]domain.FolderGrouping{}
	for rows.Next() {
		var key string
		var mode sql.NullString
		var groupID sql.NullInt64
		if err := rows.Scan(&key, &mode, &groupID); err != nil {
			return nil, fmt.Errorf("cannot read folder groupings: %w", err)
		}
		byKey[key] = domain.FolderGrouping{Mode: domain.FolderGroupMode(mode.String), GroupID: groupID.Int64}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read folder groupings: %w", err)
	}
	out := make([]domain.FolderGrouping, len(keys))
	for i, key := range keys {
		out[i] = byKey[key]
	}
	return out, nil
}

// dissolveUngroupedAndRebuild は、例外 mode が ungroup ならフォルダ folderPath の自動グループを
// 解散と同じ経路（dissolveFolderGroup）で消し、索引を作り直して、変わったグループとその祖先の id を
// 返す。auto と groupDirect は作り直しの同期が、規則が成り立てば
// 新しいグループを作る。
func dissolveUngroupedAndRebuild(ctx context.Context, tx *sql.Tx, folderPath string, mode domain.FolderGroupMode) ([]int64, error) {
	var dissolved []int64
	if mode == domain.FolderGroupUngroup {
		var err error
		if dissolved, err = dissolveFolderGroup(ctx, tx, folderPath); err != nil {
			return nil, err
		}
	}
	rebuilt, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return nil, err
	}
	return normalizeIDs(append(dissolved, rebuilt...)), nil
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
		return fmt.Errorf("cannot save the folder grouping: %w", err)
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
		return false, fmt.Errorf("cannot read the folder index state: %w", err)
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
	changedGroups, err := rebuildFolderIndex(ctx, tx)
	if err != nil {
		return err
	}
	return s.db.commitEvents(ctx, tx, groupsChangedEvents(changedGroups)...)
}

// markFolderIndexStale は索引が古いことを記録する。行が無ければ版 0 で作るので、
// 次の起動でも作り直す。
func markFolderIndexStale(ctx context.Context, q queryExecer) error {
	if _, err := q.ExecContext(ctx, `insert into folder_index_state (id, version, search_version, stale) values (1, 0, 0, 1)
		on conflict (id) do update set stale = 1`); err != nil {
		return fmt.Errorf("cannot save the folder index state: %w", err)
	}
	return nil
}

// rebuildFolderIndex は、登録フォルダの下の全所在と例外を読み、
// domain.BuildFolderIndex を通して、フォルダ名の表を丸ごと書き直す。同じ取引で自動グループをフォルダとそろえ
// （syncFolderGroups、docs/design-docs/catalog-organization.md「Automatic group sync」）、変わった
// グループとその祖先の id を返す。呼び出し側は確定の後に groupsChangedEvents で知らせる。
// 呼び出し側の書き込み取引の中で行うので、読み出しは作り直しの前か後のどちらかだけを見る。
// ファイルシステムは読まない。
func rebuildFolderIndexOnly(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	roots, err := listMediaFolders(ctx, tx)
	if err != nil {
		return nil, err
	}
	locations, err := folderIndexLocations(ctx, tx)
	if err != nil {
		return nil, err
	}
	overrides, err := folderGroupOverrides(ctx, tx)
	if err != nil {
		return nil, err
	}
	index := domain.BuildFolderIndex(roots, locations, overrides)
	if err := writeFolderIndex(ctx, tx, index); err != nil {
		return nil, fmt.Errorf("cannot save the folder index: %w", err)
	}
	changed, err := syncFolderGroups(ctx, tx, index, overrides)
	if err != nil {
		return nil, fmt.Errorf("cannot sync the folder groups: %w", err)
	}
	return changed, nil
}

// folderIndexLocations は索引の入力にする所在を返す。所有者から見た見せる動画
// （shownVideoCondition、docs/design-docs/catalog-organization.md）の所在
// だけで、集まりの代表以外のバージョンはグループにもフォルダ名の表にも入らない。
func folderIndexLocations(ctx context.Context, tx *sql.Tx) ([]domain.FolderIndexLocation, error) {
	rows, err := tx.QueryContext(ctx, `select l.video_id, l.path from video_locations l
		join videos on videos.id = l.video_id
		where `+shownVideoCondition("videos", domain.AudienceOwner))
	if err != nil {
		return nil, fmt.Errorf("cannot read locations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var locations []domain.FolderIndexLocation
	for rows.Next() {
		var location domain.FolderIndexLocation
		if err := rows.Scan(&location.VideoID, &location.Path); err != nil {
			return nil, fmt.Errorf("cannot read locations: %w", err)
		}
		locations = append(locations, location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read locations: %w", err)
	}
	return locations, nil
}

func folderGroupOverrides(ctx context.Context, tx *sql.Tx) (map[string]domain.FolderGroupMode, error) {
	rows, err := tx.QueryContext(ctx, `select path, mode from folder_group_overrides`)
	if err != nil {
		return nil, fmt.Errorf("cannot read folder groupings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	overrides := map[string]domain.FolderGroupMode{}
	for rows.Next() {
		var path, mode string
		if err := rows.Scan(&path, &mode); err != nil {
			return nil, fmt.Errorf("cannot read folder groupings: %w", err)
		}
		overrides[path] = domain.FolderGroupMode(mode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read folder groupings: %w", err)
	}
	return overrides, nil
}

func writeFolderIndex(ctx context.Context, tx *sql.Tx, index domain.FolderIndex) error {
	if _, err := tx.ExecContext(ctx, `delete from video_folder_names`); err != nil {
		return err
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
