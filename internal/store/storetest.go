package store

import (
	"context"

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
