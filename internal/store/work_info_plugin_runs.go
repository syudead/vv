package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Latest plugin stderr is persistent user data and emits no events.

// SaveWorkInfoPluginRun atomically ignores missing plugins or changed generations so stale runs cannot overwrite current state.
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

// WorkInfoPluginRun distinguishes no saved run from a missing plugin, which returns domain.ErrNotFound.
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
