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

// Plugin rows and settings are persistent user data without domain events.

// Metadata reads omit module bytes and include the optional latest-run completion time.
const workInfoPluginColumns = `id, name, file_name, version, description, contract, operations, settings,
	digest, problem, approved_digest, added_at, updated_at, generation, kind, sources, domains, builtin,
	(select count(*) from work_info_plugins r where r.link_rank is not null and work_info_plugins.link_rank is not null
		and (r.link_rank < work_info_plugins.link_rank or r.link_rank = work_info_plugins.link_rank and r.id <= work_info_plugins.id)),
	(select ran_at from work_info_plugin_runs where plugin_id = work_info_plugins.id)`

// ListWorkInfoPlugins sorts by name then filename, placing unreadable names last.
func (s *WorkInfoPluginStore) ListWorkInfoPlugins(ctx context.Context) ([]domain.WorkInfoPlugin, error) {
	tx, err := s.read.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot list the work-info plugins: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	return listWorkInfoPlugins(ctx, tx)
}

func listWorkInfoPlugins(ctx context.Context, tx *sql.Tx) ([]domain.WorkInfoPlugin, error) {
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

func (s *WorkInfoPluginStore) WorkInfoPlugin(ctx context.Context, id int64) (domain.WorkInfoPlugin, error) {
	return readWorkInfoPlugin(ctx, s.sql, id)
}

// WorkInfoPluginModule reads metadata and bytes from one snapshot; unusable rows have nil modules.
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

// SaveWorkInfoPlugin replaces same-name uploads and always revokes approval, retaining only declared settings unless the
// new manifest is unreadable; a replacement takes the new kind, sources and domains.
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
	handlesLinks := upload.Problem == "" && upload.Manifest.Kind != domain.WorkInfoPluginKindPerson &&
		len(upload.Manifest.Domains) > 0
	if err := placeLinkRank(ctx, tx, id, handlesLinks); err != nil {
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

// Unusable uploads retain neither module bytes nor declarations.
type uploadColumns struct {
	name                 sql.NullString
	version, description sql.NullString
	contract             sql.NullInt64
	operations, settings sql.NullString
	kind                 domain.WorkInfoPluginKind
	sources, domains     sql.NullString
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
		kind:        domain.WorkInfoPluginKindWork,
	}
	if m.Contract != 0 {
		cols.contract = sql.NullInt64{Int64: int64(m.Contract), Valid: true}
	}
	if m.Kind == domain.WorkInfoPluginKindPerson {
		encoded, err := json.Marshal(m.Sources)
		if err != nil {
			return uploadColumns{}, fmt.Errorf("cannot encode the person plugin sources: %w", err)
		}
		cols.kind, cols.sources = m.Kind, sql.NullString{String: string(encoded), Valid: true}
	}
	var err error
	if cols.domains, err = encodeDomains(m.Domains); err != nil {
		return uploadColumns{}, err
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

// encodeDomains stores nil, a manifest without domains, as null.
func encodeDomains(domains []string) (sql.NullString, error) {
	if domains == nil {
		return sql.NullString{}, nil
	}
	encoded, err := json.Marshal(domains)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("cannot encode the plugin domains: %w", err)
	}
	return sql.NullString{String: string(encoded), Valid: true}, nil
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func existingWorkInfoPlugin(ctx context.Context, tx *sql.Tx, name string) (int64, bool, error) {
	if name == "" {
		return 0, false, nil
	}
	var id int64
	var builtin bool
	err := tx.QueryRowContext(ctx, `select id, builtin from work_info_plugins where name = ?`, name).Scan(&id, &builtin)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("cannot read the work-info plugin %q: %w", name, err)
	case builtin:
		return 0, false, domain.ErrWorkInfoPluginBuiltin
	}
	return id, true, nil
}

// placeLinkRank keeps a plugin's place in the link order, places a plugin new to the order just before the bundled
// one, so a site's own plugin wins over it by default, and drops a plugin that no longer declares domains.
func placeLinkRank(ctx context.Context, tx *sql.Tx, id int64, handlesLinks bool) error {
	if !handlesLinks {
		_, err := tx.ExecContext(ctx, `update work_info_plugins set link_rank = null where id = ?`, id)
		return err
	}
	var rank, builtinRank sql.NullInt64
	if err := tx.QueryRowContext(ctx, `select link_rank,
		(select min(link_rank) from work_info_plugins where builtin = 1) from work_info_plugins where id = ?`,
		id).Scan(&rank, &builtinRank); err != nil {
		return fmt.Errorf("cannot read the plugin's link order: %w", err)
	}
	if rank.Valid {
		return nil
	}
	if !builtinRank.Valid {
		_, err := tx.ExecContext(ctx, `update work_info_plugins
			set link_rank = coalesce((select max(link_rank) from work_info_plugins), 0) + 1 where id = ?`, id)
		return err
	}
	if _, err := tx.ExecContext(ctx, `update work_info_plugins set link_rank = link_rank + 1 where link_rank >= ?`,
		builtinRank.Int64); err != nil {
		return fmt.Errorf("cannot move the plugins in the link order: %w", err)
	}
	_, err := tx.ExecContext(ctx, `update work_info_plugins set link_rank = ? where id = ?`, builtinRank.Int64, id)
	return err
}

// SetLinkOrder ranks the plugins that declare domains in order, which must name each of them once, and returns the
// plugins as ListWorkInfoPlugins does.
func (s *WorkInfoPluginStore) SetLinkOrder(ctx context.Context, order []int64) ([]domain.WorkInfoPlugin, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot change the link order: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	plugins, err := listWorkInfoPlugins(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := domain.CheckLinkOrder(plugins, order); err != nil {
		return nil, err
	}
	for i, id := range order {
		if _, err := tx.ExecContext(ctx, `update work_info_plugins set link_rank = ? where id = ?`, i+1, id); err != nil {
			return nil, fmt.Errorf("cannot change the link order: %w", err)
		}
	}
	if plugins, err = listWorkInfoPlugins(ctx, tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot change the link order: %w", err)
	}
	return plugins, nil
}

func insertWorkInfoPlugin(
	ctx context.Context, tx *sql.Tx, cols uploadColumns, upload domain.WorkInfoPluginUpload, now time.Time,
) (int64, error) {
	result, err := tx.ExecContext(ctx, `insert into work_info_plugins
		(name, file_name, version, description, contract, operations, settings, module, digest, problem,
		 approved_digest, added_at, updated_at, kind, sources, domains)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, null, ?, ?, ?, ?, ?)`,
		cols.name, upload.FileName, cols.version, cols.description, cols.contract, cols.operations,
		cols.settings, cols.module, upload.Digest, cols.problem, now.Unix(), now.Unix(), cols.kind, cols.sources, cols.domains)
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
		digest = ?, problem = ?, approved_digest = null, updated_at = ?, generation = generation + 1,
		kind = ?, sources = ?, domains = ?
		where id = ?`,
		upload.FileName, cols.version, cols.description, cols.contract, cols.operations, cols.settings,
		cols.module, upload.Digest, cols.problem, now.Unix(), cols.kind, cols.sources, cols.domains, id); err != nil {
		return fmt.Errorf("cannot replace the work-info plugin: %w", err)
	}
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

// ApproveWorkInfoPlugin rechecks usability and digest inside the transaction before approving.
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

func (s *WorkInfoPluginStore) RevokeWorkInfoPlugin(ctx context.Context, id int64, now time.Time) (domain.WorkInfoPlugin, error) {
	return s.changeWorkInfoPlugin(ctx, id, func(tx *sql.Tx, _ domain.WorkInfoPlugin) error {
		_, err := tx.ExecContext(ctx,
			`update work_info_plugins set approved_digest = null, updated_at = ?, generation = generation + 1
				where id = ?`, now.Unix(), id)
		return err
	})
}

// SetWorkInfoPluginValues revalidates declarations, clears nil/empty values and preserves unspecified keys, approval and edit time.
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

// DeleteWorkInfoPlugin cascades settings and rejects a missing plugin and the bundled one.
func (s *WorkInfoPluginStore) DeleteWorkInfoPlugin(ctx context.Context, id int64) error {
	var builtin bool
	err := s.sql.QueryRowContext(ctx, `select builtin from work_info_plugins where id = ?`, id).Scan(&builtin)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.ErrNotFound
	case err != nil:
		return fmt.Errorf("cannot delete the work-info plugin: %w", err)
	case builtin:
		return domain.ErrWorkInfoPluginBuiltin
	}
	result, err := s.sql.ExecContext(ctx, `delete from work_info_plugins where id = ? and builtin = 0`, id)
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

type pluginQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

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

// Plugin ID zero reads values for all plugins.
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
		problem, approved, sources, domains              sql.NullString
		contract, lastRunAt                              sql.NullInt64
		addedAt, updatedAt                               int64
		kind                                             string
	)
	err := row.Scan(&plugin.ID, &name, &plugin.FileName, &version, &description, &contract, &operations,
		&settings, &plugin.Digest, &problem, &approved, &addedAt, &updatedAt, &plugin.Generation,
		&kind, &sources, &domains, &plugin.Builtin, &plugin.LinkRank, &lastRunAt)
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
		Kind:        domain.WorkInfoPluginKind(kind),
	}
	if sources.Valid {
		if err := json.Unmarshal([]byte(sources.String), &plugin.Manifest.Sources); err != nil {
			return domain.WorkInfoPlugin{}, fmt.Errorf("cannot read the person plugin sources: %w", err)
		}
	}
	if domains.Valid {
		if err := json.Unmarshal([]byte(domains.String), &plugin.Manifest.Domains); err != nil {
			return domain.WorkInfoPlugin{}, fmt.Errorf("cannot read the plugin domains: %w", err)
		}
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
	if plugin.Builtin {
		// The bundled plugin's manifest is the code's, so a newer VVMDM describes it without a migration.
		plugin.Manifest = domain.LinkPluginManifest
	}
	return plugin, nil
}
