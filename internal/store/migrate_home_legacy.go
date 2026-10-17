package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pressly/goose/v3/database"
)

// The home feature originally used versions 56–58 before main assigned 56 and
// 57 to keyframe spacing and location timestamps. Goose records numbers, not
// filenames. Reconcile that specific development schema before Up skips main's
// changes and tries to create the home tables a second time.
// All schema and history changes commit together; no library rows are removed.
func repairLegacyHomeMigrations(ctx context.Context, db *sql.DB, current int64) (int, error) {
	if current < 56 || current > 58 {
		return 0, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	legacy, err := legacyHomeSchema(ctx, tx, current)
	if err != nil || !legacy {
		return 0, err
	}
	if err := validateLegacyHomeVersions(ctx, tx, current); err != nil {
		return 0, err
	}
	// Descending updates avoid colliding with the old version 58 row.
	for version := current; version >= 56; version-- {
		if _, err := tx.ExecContext(ctx, `update goose_db_version set version_id = ? where version_id = ?`, version+2, version); err != nil {
			return 0, err
		}
	}
	store, err := database.NewStore(database.DialectSQLite3, versionTableName)
	if err != nil {
		return 0, err
	}
	for _, migration := range []struct {
		version int64
		name    string
	}{
		{56, "00056_keyframe_spacing.sql"},
		{57, "00057_location_mtime_ns.sql"},
	} {
		if err := applyLegacyHomePrerequisite(ctx, tx, migration.name); err != nil {
			return 0, err
		}
		if err := store.Insert(ctx, tx, database.InsertRequest{Version: migration.version}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return 2, nil
}

func legacyHomeSchema(ctx context.Context, tx *sql.Tx, current int64) (bool, error) {
	var home, keyframes, timestamps, closure, watch, search bool
	err := tx.QueryRowContext(ctx, `select
		exists(select 1 from pragma_table_info('smart_folders') where name = 'home_position'),
		exists(select 1 from sqlite_schema where type = 'table' and name = 'video_keyframe_spacing'),
		exists(select 1 from pragma_table_info('video_locations') where name = 'mtime_ns'),
		exists(select 1 from sqlite_schema where type = 'table' and name = 'group_closure'),
		exists(select 1 from sqlite_schema where type = 'table' and name = 'video_watch_states'),
		exists(select 1 from sqlite_schema where type = 'table' and name = 'home_search_hits')`).
		Scan(&home, &keyframes, &timestamps, &closure, &watch, &search)
	if err != nil {
		return false, err
	}
	if !home || (keyframes && timestamps) {
		return false, nil
	}
	if keyframes || timestamps || closure != (current >= 57) || watch != (current >= 58) || search != watch {
		return false, errors.New("unrecognized legacy home schema; database left unchanged")
	}
	return true, nil
}

func validateLegacyHomeVersions(ctx context.Context, tx *sql.Tx, current int64) error {
	var rows, versions, applied int64
	err := tx.QueryRowContext(ctx, `select count(*), count(distinct version_id), coalesce(sum(is_applied), 0)
		from goose_db_version where version_id >= 56`).Scan(&rows, &versions, &applied)
	if err != nil {
		return err
	}
	if rows != current-55 || versions != rows || applied != rows {
		return errors.New("unrecognized legacy home migration history; database left unchanged")
	}
	return nil
}

// These two immutable main migrations are plain SQL with one Up and Down
// section. Execute their embedded Up text rather than maintaining a second
// copy of their schema definitions. This helper is specific to versions 56/57,
// not a replacement for Goose's general SQL parser.
func applyLegacyHomePrerequisite(ctx context.Context, tx *sql.Tx, name string) error {
	body, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return err
	}
	up, _, ok := strings.Cut(string(body), "-- +goose Down")
	if !ok {
		return fmt.Errorf("missing Down section in legacy home prerequisite %s", name)
	}
	if _, err := tx.ExecContext(ctx, up); err != nil {
		return fmt.Errorf("cannot apply legacy home prerequisite %s: %w", name, err)
	}
	return nil
}
