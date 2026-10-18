package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

// Embed migrations so startup needs no goose CLI.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// Goose owns this history, except for the atomic legacy Home repair in migrate_home_legacy.go.
const versionTableName = "goose_db_version"

// ErrFutureSchema prevents startup writes when the database is newer than this application.
var ErrFutureSchema = errors.New("the database schema is newer than this application")

type MigrateResult struct {
	Applied int
	Version int64
}

// Migrate applies pending embedded migrations and refuses future schema versions before writing.
func Migrate(ctx context.Context, db *DB) (MigrateResult, error) {
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return MigrateResult{}, fmt.Errorf("cannot read migrations: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsys)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("cannot prepare migrations: %w", err)
	}

	known, err := latestKnownVersion(provider)
	if err != nil {
		return MigrateResult{}, err
	}

	current, err := recordedVersion(ctx, db.sql)
	if err != nil {
		return MigrateResult{}, err
	}
	if current > known {
		return MigrateResult{}, fmt.Errorf(
			"%w: the database is at version %d, but this application knows versions up to %d. "+
				"Start a newer version of the application or recreate the database (%s)",
			ErrFutureSchema, current, known, db.Path(),
		)
	}

	repaired, err := repairLegacyHomeMigrations(ctx, db.sql, current)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("cannot reconcile legacy home migrations: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("cannot apply migrations: %w", err)
	}
	// ホームの読み出しが引く表（根の値、棚への一致）を、マイグレーションの直後に埋める。
	if err := db.maintainHomeIndexes(ctx); err != nil {
		return MigrateResult{}, err
	}

	applied, err := recordedVersion(ctx, db.sql)
	if err != nil {
		return MigrateResult{}, err
	}

	return MigrateResult{Applied: repaired + len(results), Version: applied}, nil
}

func latestKnownVersion(provider *goose.Provider) (int64, error) {
	sources := provider.ListSources()
	if len(sources) == 0 {
		return 0, errors.New("no embedded migrations")
	}

	var latest int64
	for _, source := range sources {
		if source.Version > latest {
			latest = source.Version
		}
	}
	return latest, nil
}

// A missing version table means version zero.
func recordedVersion(ctx context.Context, db *sql.DB) (int64, error) {
	var exists int
	err := db.QueryRowContext(ctx,
		`select count(*) from sqlite_master where type = 'table' and name = ?`,
		versionTableName,
	).Scan(&exists)
	if err != nil {
		return 0, fmt.Errorf("cannot read the schema version: %w", err)
	}
	if exists == 0 {
		return 0, nil
	}

	var version sql.NullInt64
	err = db.QueryRowContext(ctx,
		`select max(version_id) from `+versionTableName,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("cannot read the schema version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return version.Int64, nil
}

// Down reverts one migration for verification; startup never calls it.
func Down(ctx context.Context, db *DB) error {
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("cannot read migrations: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsys)
	if err != nil {
		return fmt.Errorf("cannot prepare migrations: %w", err)
	}

	if _, err := provider.Down(ctx); err != nil {
		return fmt.Errorf("cannot roll back migrations: %w", err)
	}
	return nil
}
