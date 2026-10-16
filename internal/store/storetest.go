package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// このファイルの関数はテスト専用である。パッケージの外のテスト（cmd/mdm の結合
// テストなど）が、役割の型の操作では作れない保存層の状態を用意し、確かめるために
// 置く。_test.go は別パッケージから読み込めないので通常のファイルに置くが、本番の
// コードからは呼ばない。名前の末尾の ForTest がその印である。

// SetJobAttemptsForTest は、動画 videoID の kind のジョブの試行回数を attempts に
// 書き換える。再試行の上限に達したジョブを用意するのに使う。テスト専用。
func SetJobAttemptsForTest(ctx context.Context, db *DB, kind domain.JobKind, videoID int64, attempts int) error {
	_, err := db.sql.ExecContext(ctx,
		`update jobs set attempts = ? where kind = ? and video_id = ?`, attempts, string(kind), videoID)
	return err
}

// DeleteJobsForTest は待ち行列のジョブをすべて消す。ジョブの残っていない
// 既存の MDM_DATA_DIR を用意するのに使う。テスト専用。
func DeleteJobsForTest(ctx context.Context, db *DB) error {
	_, err := db.sql.ExecContext(ctx, `delete from jobs`)
	return err
}

// CountJobsForTest は待ち行列のジョブの件数を、状態を問わず返す。テスト専用。
func CountJobsForTest(ctx context.Context, db *DB) (int, error) {
	var count int
	err := db.sql.QueryRowContext(ctx, `select count(*) from jobs`).Scan(&count)
	return count, err
}

// CountVideoJobsForTest は動画 videoID の kind のジョブの件数を、状態を問わず
// 返す。テスト専用。
func CountVideoJobsForTest(ctx context.Context, db *DB, kind domain.JobKind, videoID int64) (int, error) {
	var count int
	err := db.sql.QueryRowContext(ctx,
		`select count(*) from jobs where kind = ? and video_id = ?`, string(kind), videoID).Scan(&count)
	return count, err
}

// migratedImage は、すべてのマイグレーションを適用したデータベースファイルの中身を
// 1 プロセスに 1 回だけ作って持つ。マイグレーションの適用は 1 回に 70ms ほどかかり、
// テストごとに適用すると保存層や HTTP のテストの時間の半分ほどを占める。
var migratedImage struct {
	once  sync.Once
	bytes []byte
	err   error
}

// OpenMigratedForTest は、すべてのマイグレーションを適用したデータベースを dataDir に
// 置いて開く。Open と Migrate を続けて呼んだのと同じ状態になるが、適用はプロセスで
// 最初の 1 回だけ行い、以降はその結果のファイルを写す。dataDir にデータベースが
// すでにあれば（再起動を模すテスト）、写さずに開いて未適用の分を適用する。
// マイグレーションそのものを確かめるテストは Open と Migrate を使う。呼び出し側は
// 必ず Close を呼ぶこと。テスト専用。
func OpenMigratedForTest(dataDir string) (*DB, error) {
	if _, err := os.Stat(DatabasePath(dataDir)); err == nil {
		return openAndMigrate(dataDir)
	}
	migratedImage.once.Do(func() {
		migratedImage.bytes, migratedImage.err = buildMigratedImage()
	})
	if migratedImage.err != nil {
		return nil, migratedImage.err
	}
	if err := os.WriteFile(DatabasePath(dataDir), migratedImage.bytes, 0o600); err != nil {
		return nil, fmt.Errorf("cannot write the migrated database: %w", err)
	}
	return Open(dataDir)
}

// buildMigratedImage は一時ディレクトリでマイグレーションを適用し、WAL を本体へ
// 書き戻してから、データベースファイルの中身を返す。
func buildMigratedImage() ([]byte, error) {
	dir, err := os.MkdirTemp("", "mdm-migrated-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	db, err := openAndMigrate(dir)
	if err != nil {
		return nil, err
	}
	if _, err := db.sql.Exec(`pragma wal_checkpoint(truncate)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(DatabasePath(dir))
}

// openAndMigrate は dataDir のデータベースを開き、未適用のマイグレーションを適用する。
func openAndMigrate(dataDir string) (*DB, error) {
	db, err := Open(dataDir)
	if err != nil {
		return nil, err
	}
	if _, err := Migrate(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// RecordActivityForTest は記録 entry を、記録する役割と同じ関数（recordActivity）で 1 つの取引に書き、その id を
// 返す。確定後に domain.ActivityChanged を発行する。まだどの処理も記録を書かない段階で、一覧と知らせを
// 確かめるのに使う。テスト専用。
func RecordActivityForTest(ctx context.Context, db *DB, entry domain.ActivityEntry, now time.Time) (int64, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	id, err := recordActivity(ctx, tx, c, entry, now)
	if err != nil {
		return 0, err
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return 0, err
	}
	return id, nil
}
