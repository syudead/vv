package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインの行と、その設定の値。どちらも利用者データで、イベントを発行しない
//

// workInfoPluginColumns は行を読む列である。モジュールのバイト列は読まない。
// 最後の列は残っている直近の実行の終わった時刻で、無ければ null である。
const workInfoPluginColumns = `id, name, file_name, version, description, contract, operations, settings,
	digest, problem, approved_digest, added_at, updated_at, generation,
	(select ran_at from work_info_plugin_runs where plugin_id = work_info_plugins.id)`

// ListWorkInfoPlugins はすべてのプラグインを、名前、次にファイル名の順で返す。名前を読めなかった
// プラグインは名前のあるものの後に、ファイル名の順で並ぶ。
func (s *WorkInfoPluginStore) ListWorkInfoPlugins(ctx context.Context) ([]domain.WorkInfoPlugin, error) {
	tx, err := s.read.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot list the work-info plugins: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `select `+workInfoPluginColumns+` from work_info_plugins
		order by name is null, name, file_name, id`)
	if err != nil {
		return nil, fmt.Errorf("cannot list the work-info plugins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var plugins []domain.WorkInfoPlugin
	for rows.Next() {
		plugin, err := scanWorkInfoPlugin(rows)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, plugin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot list the work-info plugins: %w", err)
	}
	values, err := workInfoPluginValues(ctx, tx, 0)
	if err != nil {
		return nil, err
	}
	for i := range plugins {
		plugins[i].Values = valuesOrEmpty(values[plugins[i].ID])
	}
	return plugins, nil
}

// WorkInfoPlugin はプラグイン id を返す。無ければ domain.ErrNotFound を返す。
func (s *WorkInfoPluginStore) WorkInfoPlugin(ctx context.Context, id int64) (domain.WorkInfoPlugin, error) {
	return readWorkInfoPlugin(ctx, s.sql, id)
}

// WorkInfoPluginModule はプラグイン id の行と、そのモジュールのバイト列を同じスナップショットから返す。
// 問題のある行のモジュールは nil である。無ければ domain.ErrNotFound を返す。
func (s *WorkInfoPluginStore) WorkInfoPluginModule(ctx context.Context, id int64) (domain.WorkInfoPlugin, []byte, error) {
	tx, err := s.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.WorkInfoPlugin{}, nil, fmt.Errorf("cannot read the work-info plugin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	plugin, err := readWorkInfoPlugin(ctx, tx, id)
	if err != nil {
		return domain.WorkInfoPlugin{}, nil, err
	}
	var module []byte
	if err := tx.QueryRowContext(ctx, `select module from work_info_plugins where id = ?`, id).Scan(&module); err != nil {
		return domain.WorkInfoPlugin{}, nil, fmt.Errorf("cannot read the work-info plugin module: %w", err)
	}
	return plugin, module, nil
}

// SaveWorkInfoPlugin はアップロード upload を保存し、保存した行と、既存の行を差し替えたかを返す。
//
// マニフェストの名前が既存の行と同じなら、1 つの取引でその行のモジュール・メタデータ・ダイジェスト・
// 問題を置き換え、approved_digest を消して承認前に戻す。同じバイト列の再アップロードや、別の中身を
// 挟んで以前のバイト列へ戻した差し替えでも、ダイジェストの一致で古い承認が戻らないようにする（親 Issue の
// 「同じ名前のプラグインを追加したときは差し替えとして扱い、承認前に戻る」）。
// 差し替えでは、新しいマニフェストが宣言するキーの値を残してそれ以外を消す。問題のある差し替えは
// 宣言を読めていないので、値を全部残す。名前を読めなかったアップロードは新しい行になる
func (s *WorkInfoPluginStore) SaveWorkInfoPlugin(
	ctx context.Context, upload domain.WorkInfoPluginUpload, now time.Time,
) (domain.WorkInfoPlugin, bool, error) {
	cols, err := workInfoPluginUploadColumns(upload)
	if err != nil {
		return domain.WorkInfoPlugin{}, false, err
	}
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.WorkInfoPlugin{}, false, fmt.Errorf("cannot save the work-info plugin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, replaced, err := existingWorkInfoPlugin(ctx, tx, upload.Manifest.Name)
	if err != nil {
		return domain.WorkInfoPlugin{}, false, err
	}
	if replaced {
		err = replaceWorkInfoPlugin(ctx, tx, id, cols, upload, now)
	} else {
		id, err = insertWorkInfoPlugin(ctx, tx, cols, upload, now)
	}
	if err != nil {
		return domain.WorkInfoPlugin{}, false, err
	}
	plugin, err := readWorkInfoPlugin(ctx, tx, id)
	if err != nil {
		return domain.WorkInfoPlugin{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.WorkInfoPlugin{}, false, fmt.Errorf("cannot save the work-info plugin: %w", err)
	}
	return plugin, replaced, nil
}

// uploadColumns はアップロードから書く列の値である。問題のある行は宣言とモジュールを持たない。
type uploadColumns struct {
	name                 sql.NullString
	version, description sql.NullString
	contract             sql.NullInt64
	operations, settings sql.NullString
	module               []byte
	problem              sql.NullString
	declared             []string
	keepAllValues        bool
}

func workInfoPluginUploadColumns(upload domain.WorkInfoPluginUpload) (uploadColumns, error) {
	m := upload.Manifest
	cols := uploadColumns{
		name:        nullString(m.Name),
		version:     nullString(m.Version),
		description: nullString(m.Description),
	}
	if m.Contract != 0 {
		cols.contract = sql.NullInt64{Int64: int64(m.Contract), Valid: true}
	}
	if upload.Problem != "" {
		cols.problem = nullString(string(upload.Problem))
		cols.keepAllValues = true
		return cols, nil
	}
	if len(upload.Module) == 0 {
		return uploadColumns{}, errors.New("cannot save a work-info plugin without a module")
	}
	operations := make([]string, len(m.Operations))
	for i, op := range m.Operations {
		operations[i] = string(op)
	}
	settings := m.Settings
	if settings == nil {
		settings = []domain.WorkInfoPluginSetting{}
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return uploadColumns{}, fmt.Errorf("cannot encode the work-info plugin settings: %w", err)
	}
	cols.operations = nullString(strings.Join(operations, ","))
	cols.settings = sql.NullString{String: string(encoded), Valid: true}
	cols.module = upload.Module
	for _, s := range settings {
		cols.declared = append(cols.declared, s.Key)
	}
	return cols, nil
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// existingWorkInfoPlugin は名前 name の行の id を返す。名前が空か、その名前の行が無ければ偽を返す。
func existingWorkInfoPlugin(ctx context.Context, tx *sql.Tx, name string) (int64, bool, error) {
	if name == "" {
		return 0, false, nil
	}
	var id int64
	err := tx.QueryRowContext(ctx, `select id from work_info_plugins where name = ?`, name).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("cannot read the work-info plugin %q: %w", name, err)
	}
	return id, true, nil
}

func insertWorkInfoPlugin(
	ctx context.Context, tx *sql.Tx, cols uploadColumns, upload domain.WorkInfoPluginUpload, now time.Time,
) (int64, error) {
	result, err := tx.ExecContext(ctx, `insert into work_info_plugins
		(name, file_name, version, description, contract, operations, settings, module, digest, problem,
		 approved_digest, added_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, null, ?, ?)`,
		cols.name, upload.FileName, cols.version, cols.description, cols.contract, cols.operations,
		cols.settings, cols.module, upload.Digest, cols.problem, now.Unix(), now.Unix())
	if err != nil {
		return 0, fmt.Errorf("cannot add the work-info plugin: %w", err)
	}
	return result.LastInsertId()
}

func replaceWorkInfoPlugin(
	ctx context.Context, tx *sql.Tx, id int64, cols uploadColumns, upload domain.WorkInfoPluginUpload, now time.Time,
) error {
	if _, err := tx.ExecContext(ctx, `update work_info_plugins set
		file_name = ?, version = ?, description = ?, contract = ?, operations = ?, settings = ?, module = ?,
		digest = ?, problem = ?, approved_digest = null, updated_at = ?, generation = generation + 1
		where id = ?`,
		upload.FileName, cols.version, cols.description, cols.contract, cols.operations, cols.settings,
		cols.module, upload.Digest, cols.problem, now.Unix(), id); err != nil {
		return fmt.Errorf("cannot replace the work-info plugin: %w", err)
	}
	// 前のモジュールの実行は残さない。
	if _, err := tx.ExecContext(ctx, `delete from work_info_plugin_runs where plugin_id = ?`, id); err != nil {
		return fmt.Errorf("cannot delete the replaced work-info plugin's last run: %w", err)
	}
	if cols.keepAllValues {
		return nil
	}
	declared, err := json.Marshal(append([]string{}, cols.declared...))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from work_info_plugin_settings
		where plugin_id = ? and key not in (select value from json_each(?))`, id, string(declared)); err != nil {
		return fmt.Errorf("cannot delete the undeclared work-info plugin settings: %w", err)
	}
	return nil
}

// ApproveWorkInfoPlugin はプラグイン id のダイジェスト digest を承認し、承認後の行を返す。取引の中で行を
// 読み直し、無ければ domain.ErrNotFound、問題があれば domain.ErrWorkInfoPluginUnusable、ダイジェストが
// 違えば domain.ErrWorkInfoPluginChanged を返して何も書かない。
func (s *WorkInfoPluginStore) ApproveWorkInfoPlugin(
	ctx context.Context, id int64, digest string, now time.Time,
) (domain.WorkInfoPlugin, error) {
	return s.changeWorkInfoPlugin(ctx, id, func(tx *sql.Tx, plugin domain.WorkInfoPlugin) error {
		switch {
		case plugin.Problem != "":
			return domain.ErrWorkInfoPluginUnusable
		case plugin.Digest != digest:
			return domain.ErrWorkInfoPluginChanged
		}
		_, err := tx.ExecContext(ctx,
			`update work_info_plugins set approved_digest = ?, updated_at = ? where id = ?`, digest, now.Unix(), id)
		return err
	})
}

// RevokeWorkInfoPlugin はプラグイン id の承認を取り消し、取り消し後の行を返す。無ければ
// domain.ErrNotFound を返す。
func (s *WorkInfoPluginStore) RevokeWorkInfoPlugin(ctx context.Context, id int64, now time.Time) (domain.WorkInfoPlugin, error) {
	return s.changeWorkInfoPlugin(ctx, id, func(tx *sql.Tx, _ domain.WorkInfoPlugin) error {
		_, err := tx.ExecContext(ctx,
			`update work_info_plugins set approved_digest = null, updated_at = ?, generation = generation + 1
				where id = ?`, now.Unix(), id)
		return err
	})
}

// SetWorkInfoPluginValues はプラグイン id の設定の値を書き、書いた後の行を返す。nil か空の値はその
// キーの値を消し、values に無いキーの値はそのまま残す。承認と updated_at には触れない。取引の中で行を
// 読み直し、無ければ domain.ErrNotFound、問題があれば domain.ErrWorkInfoPluginUnusable、今の宣言に
// 無いキーか長すぎる値なら domain.ErrWorkInfoPluginSettingInvalid を返して何も書かない。
func (s *WorkInfoPluginStore) SetWorkInfoPluginValues(
	ctx context.Context, id int64, values map[string]*string, now time.Time,
) (domain.WorkInfoPlugin, error) {
	return s.changeWorkInfoPlugin(ctx, id, func(tx *sql.Tx, plugin domain.WorkInfoPlugin) error {
		if plugin.Problem != "" {
			return domain.ErrWorkInfoPluginUnusable
		}
		if err := domain.CheckWorkInfoPluginValues(plugin.Manifest.Settings, values); err != nil {
			return err
		}
		for key, value := range values {
			if err := writeWorkInfoPluginValue(ctx, tx, id, key, value, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeWorkInfoPluginValue(ctx context.Context, tx *sql.Tx, id int64, key string, value *string, now time.Time) error {
	if value == nil || *value == "" {
		_, err := tx.ExecContext(ctx, `delete from work_info_plugin_settings where plugin_id = ? and key = ?`, id, key)
		return err
	}
	_, err := tx.ExecContext(ctx, `insert into work_info_plugin_settings (plugin_id, key, value, updated_at)
		values (?, ?, ?, ?)
		on conflict (plugin_id, key) do update set value = excluded.value, updated_at = excluded.updated_at`,
		id, key, *value, now.Unix())
	return err
}

// changeWorkInfoPlugin は取引の中でプラグイン id を読み直して change に渡し、確定後の行を返す。
func (s *WorkInfoPluginStore) changeWorkInfoPlugin(
	ctx context.Context, id int64, change func(tx *sql.Tx, plugin domain.WorkInfoPlugin) error,
) (domain.WorkInfoPlugin, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.WorkInfoPlugin{}, fmt.Errorf("cannot change the work-info plugin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	plugin, err := readWorkInfoPlugin(ctx, tx, id)
	if err != nil {
		return domain.WorkInfoPlugin{}, err
	}
	if err := change(tx, plugin); err != nil {
		return domain.WorkInfoPlugin{}, err
	}
	changed, err := readWorkInfoPlugin(ctx, tx, id)
	if err != nil {
		return domain.WorkInfoPlugin{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.WorkInfoPlugin{}, fmt.Errorf("cannot change the work-info plugin: %w", err)
	}
	return changed, nil
}

// DeleteWorkInfoPlugin はプラグイン id を消す。設定の値は外部キーの on delete cascade で一緒に消える。
// 無ければ domain.ErrNotFound を返す。
func (s *WorkInfoPluginStore) DeleteWorkInfoPlugin(ctx context.Context, id int64) error {
	result, err := s.sql.ExecContext(ctx, `delete from work_info_plugins where id = ?`, id)
	if err != nil {
		return fmt.Errorf("cannot delete the work-info plugin: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot delete the work-info plugin: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// pluginQueryer は *sql.DB と *sql.Tx の共通部分のうち、プラグインの行と値を読むものである。
type pluginQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// readWorkInfoPlugin はプラグイン id を設定の値とともに読む。無ければ domain.ErrNotFound を返す。
func readWorkInfoPlugin(ctx context.Context, q pluginQueryer, id int64) (domain.WorkInfoPlugin, error) {
	plugin, err := scanWorkInfoPlugin(q.QueryRowContext(ctx,
		`select `+workInfoPluginColumns+` from work_info_plugins where id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.WorkInfoPlugin{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.WorkInfoPlugin{}, err
	}
	values, err := workInfoPluginValues(ctx, q, id)
	if err != nil {
		return domain.WorkInfoPlugin{}, err
	}
	plugin.Values = valuesOrEmpty(values[id])
	return plugin, nil
}

// workInfoPluginValues はプラグイン id（0 ならすべて）の設定の値を、プラグインごとに返す。
func workInfoPluginValues(ctx context.Context, q pluginQueryer, id int64) (map[int64]map[string]string, error) {
	rows, err := q.QueryContext(ctx, `select plugin_id, key, value from work_info_plugin_settings
		where ? = 0 or plugin_id = ?`, id, id)
	if err != nil {
		return nil, fmt.Errorf("cannot read the work-info plugin settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	values := map[int64]map[string]string{}
	for rows.Next() {
		var pluginID int64
		var key, value string
		if err := rows.Scan(&pluginID, &key, &value); err != nil {
			return nil, fmt.Errorf("cannot read the work-info plugin settings: %w", err)
		}
		if values[pluginID] == nil {
			values[pluginID] = map[string]string{}
		}
		values[pluginID][key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the work-info plugin settings: %w", err)
	}
	return values, nil
}

func valuesOrEmpty(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

func scanWorkInfoPlugin(row rowScanner) (domain.WorkInfoPlugin, error) {
	var (
		plugin                                           domain.WorkInfoPlugin
		name, version, description, operations, settings sql.NullString
		problem, approved                                sql.NullString
		contract, lastRunAt                              sql.NullInt64
		addedAt, updatedAt                               int64
	)
	err := row.Scan(&plugin.ID, &name, &plugin.FileName, &version, &description, &contract, &operations,
		&settings, &plugin.Digest, &problem, &approved, &addedAt, &updatedAt, &plugin.Generation, &lastRunAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.WorkInfoPlugin{}, err
	}
	if err != nil {
		return domain.WorkInfoPlugin{}, fmt.Errorf("cannot read the work-info plugin: %w", err)
	}
	plugin.Manifest = domain.WorkInfoPluginManifest{
		Contract:    int(contract.Int64),
		Name:        name.String,
		Version:     version.String,
		Description: description.String,
	}
	if operations.String != "" {
		for op := range strings.SplitSeq(operations.String, ",") {
			plugin.Manifest.Operations = append(plugin.Manifest.Operations, domain.WorkInfoOperation(op))
		}
	}
	if settings.Valid {
		if err := json.Unmarshal([]byte(settings.String), &plugin.Manifest.Settings); err != nil {
			return domain.WorkInfoPlugin{}, fmt.Errorf("cannot read the work-info plugin settings declaration: %w", err)
		}
	}
	plugin.Problem = domain.WorkInfoPluginProblem(problem.String)
	plugin.ApprovedDigest = approved.String
	plugin.AddedAt = time.Unix(addedAt, 0)
	plugin.UpdatedAt = time.Unix(updatedAt, 0)
	if lastRunAt.Valid {
		plugin.LastRunAt = time.Unix(lastRunAt.Int64, 0)
	}
	return plugin, nil
}
