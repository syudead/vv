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

// migrationsFS はマイグレーション SQL をバイナリへ同梱する。外部ツール（goose CLI）を
// 導入しなくても起動時に適用できるようにするため（research.md R-006）。
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// versionTableName は goose が適用済みの版を記録する表である。
// アプリケーションは読み取り専用で扱い、直接書き換えない。
const versionTableName = "goose_db_version"

// ErrFutureSchema はデータベースがアプリケーションの知らない将来の版を
// 持っていた場合に返る。この場合は何も書き換えずに起動を中止する
// （ダウングレードによる破壊を防ぐため）。
var ErrFutureSchema = errors.New("データベースの構造がアプリケーションより新しい")

// MigrateResult はマイグレーションの結果である。記録に出す用途を想定している。
type MigrateResult struct {
	// Applied はこの呼び出しで適用したマイグレーションの数。
	Applied int
	// Version は適用後のスキーマ版。
	Version int64
}

// Migrate は未適用のマイグレーションを適用する。
//
//  1. 未適用があれば適用する。
//  2. データベースが埋め込み済みより新しい版を持っていた場合は、何も書き換えずに
//     ErrFutureSchema を返す。
//  3. データベースファイルが無い場合は Open が作成しているので、そのまま適用する。
func Migrate(ctx context.Context, db *DB) (MigrateResult, error) {
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return MigrateResult{}, fmt.Errorf("マイグレーションを読み出せません: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, db.SQL(), fsys)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("マイグレーションを準備できません: %w", err)
	}

	known, err := latestKnownVersion(provider)
	if err != nil {
		return MigrateResult{}, err
	}

	// 適用の前に将来の版を検出する。ここで止めれば何も書き換えずに済む。
	current, err := recordedVersion(ctx, db.SQL())
	if err != nil {
		return MigrateResult{}, err
	}
	if current > known {
		return MigrateResult{}, fmt.Errorf(
			"%w: データベースは版 %d、このアプリケーションが知っているのは版 %d までです。"+
				"新しい版のアプリケーションで起動するか、データベース (%s) を作り直してください",
			ErrFutureSchema, current, known, db.Path(),
		)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("マイグレーションを適用できません: %w", err)
	}

	applied, err := recordedVersion(ctx, db.SQL())
	if err != nil {
		return MigrateResult{}, err
	}

	return MigrateResult{Applied: len(results), Version: applied}, nil
}

// latestKnownVersion は埋め込み済みマイグレーションの最大の版を返す。
func latestKnownVersion(provider *goose.Provider) (int64, error) {
	sources := provider.ListSources()
	if len(sources) == 0 {
		return 0, errors.New("埋め込まれたマイグレーションがありません")
	}

	var latest int64
	for _, source := range sources {
		if source.Version > latest {
			latest = source.Version
		}
	}
	return latest, nil
}

// recordedVersion はデータベースが記録しているスキーマ版を返す。
// 記録表がまだ無い場合は 0 を返す（未適用とみなす）。
func recordedVersion(ctx context.Context, db *sql.DB) (int64, error) {
	var exists int
	err := db.QueryRowContext(ctx,
		`select count(*) from sqlite_master where type = 'table' and name = ?`,
		versionTableName,
	).Scan(&exists)
	if err != nil {
		return 0, fmt.Errorf("スキーマ版を読み取れません: %w", err)
	}
	if exists == 0 {
		return 0, nil
	}

	var version sql.NullInt64
	//nolint:gosec // 表名は定数で、利用者の入力は混ざらない。
	err = db.QueryRowContext(ctx,
		`select max(version_id) from `+versionTableName,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("スキーマ版を読み取れません: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return version.Int64, nil
}
