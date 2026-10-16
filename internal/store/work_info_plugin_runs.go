package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報プラグインごとの直近の実行の標準エラー（specs/056-plugin-tooling/data-model.md の
// work_info_plugin_runs・Rules、research.md R-6）。利用者データで、イベントを発行しない。

// SaveWorkInfoPluginRun はプラグイン run.PluginID の直近の実行を run に置き換える。プラグインの行が無いか、
// 世代が run.Generation でなければ（実行の後に差し替えられたか、取り消されたか、消された）何も書かない。
// 確かめと書き込みは 1 つの文で行う。
func (s *WorkInfoPluginStore) SaveWorkInfoPluginRun(ctx context.Context, run domain.WorkInfoPluginRun) error {
	failure := nullString(string(run.Failure))
	if _, err := s.sql.ExecContext(ctx, `insert into work_info_plugin_runs
		(plugin_id, generation, operation, failure, ran_at, stderr, truncated)
		select id, generation, ?, ?, ?, ?, ? from work_info_plugins where id = ? and generation = ?
		on conflict (plugin_id) do update set generation = excluded.generation, operation = excluded.operation,
			failure = excluded.failure, ran_at = excluded.ran_at, stderr = excluded.stderr,
			truncated = excluded.truncated`,
		string(run.Operation), failure, run.RanAt.Unix(), run.Stderr, run.Truncated,
		run.PluginID, run.Generation); err != nil {
		return fmt.Errorf("cannot save the work-info plugin's last run: %w", err)
	}
	return nil
}

// WorkInfoPluginRun はプラグイン id の直近の実行と、それが残っているかを返す。プラグインが無ければ
// domain.ErrNotFound を返す。
func (s *WorkInfoPluginStore) WorkInfoPluginRun(ctx context.Context, id int64) (domain.WorkInfoPluginRun, bool, error) {
	var (
		generation, ranAt  sql.NullInt64
		operation, failure sql.NullString
		stderr             sql.NullString
		truncated          sql.NullBool
	)
	err := s.read.QueryRowContext(ctx, `select r.generation, r.operation, r.failure, r.ran_at, r.stderr, r.truncated
		from work_info_plugins p left join work_info_plugin_runs r on r.plugin_id = p.id
		where p.id = ?`, id).Scan(&generation, &operation, &failure, &ranAt, &stderr, &truncated)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.WorkInfoPluginRun{}, false, domain.ErrNotFound
	case err != nil:
		return domain.WorkInfoPluginRun{}, false, fmt.Errorf("cannot read the work-info plugin's last run: %w", err)
	case !ranAt.Valid:
		return domain.WorkInfoPluginRun{}, false, nil
	}
	return domain.WorkInfoPluginRun{
		PluginID: id, Generation: generation.Int64, RanAt: time.Unix(ranAt.Int64, 0),
		WorkInfoRunLog: domain.WorkInfoRunLog{
			Operation: domain.WorkInfoOperation(operation.String), Failure: domain.WorkInfoRunFailure(failure.String),
			Stderr: stderr.String, Truncated: truncated.Bool,
		},
	}, true, nil
}
