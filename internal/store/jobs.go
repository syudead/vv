package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ジョブの語彙は internal/domain が持つ。ここでは別名を置いて、store を使う側が
// domain を直接 import しなくても読めるようにする。
type (
	// JobKind はジョブの種類である。
	JobKind = domain.JobKind
	// Job は専有したジョブである。
	Job = domain.Job
)

const (
	// JobProbe は ffprobe によるメタデータの取得。
	JobProbe = domain.JobProbe
	// JobThumbnail は ffmpeg による静止画の抽出。
	JobThumbnail = domain.JobThumbnail
	// MaxJobAttempts は諦めるまでの試行回数である。
	MaxJobAttempts = domain.MaxJobAttempts
)

// ErrNoJob は待ち行列が空であることを表す。
var ErrNoJob = domain.ErrNoJob

// JobRetention は完了したジョブを残す期間である。取り込み直後に最大 2万行に
// なるため、放置せず掃除する（data-model.md 4 節）。
const JobRetention = 7 * 24 * time.Hour

// EnqueueJob はジョブを積む。同じ (kind, video_id) の未完了ジョブが既に
// あれば何もしない（部分ユニーク索引がその状態を保証する）。
//
// 一度諦めた行は消してから積み直す。内容が変わった動画を解析し直せないと、
// 差し替えたファイルが永久に未解析のままになる。諦めた行を残さないのは、
// 再スキャンのたびに履歴が積み上がるのを避けるためである。
func (db *DB) EnqueueJob(ctx context.Context, kind JobKind, videoID int64) error {
	if _, err := db.sql.ExecContext(ctx,
		`delete from jobs where kind = ? and video_id = ? and state in ('done', 'failed')`,
		string(kind), videoID,
	); err != nil {
		return fmt.Errorf("古いジョブを掃除できません: %w", err)
	}

	now := time.Now().Unix()
	_, err := db.sql.ExecContext(ctx, `
		insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		values (?, ?, 'queued', 0, ?, ?)
		on conflict (kind, video_id) where state in ('queued', 'running') do nothing`,
		string(kind), videoID, now, now,
	)
	if err != nil {
		return fmt.Errorf("ジョブを積めません (%s, video=%d): %w", kind, videoID, err)
	}
	return nil
}

// ClaimJob は待ち行列から1件を専有する（R-106）。
//
// 取り出しと状態の書き換えを begin immediate のトランザクションで囲む。
// select と update を分けると、同じ行を二重に処理する余地が残る。ワーカーは
// 1本だけだが、将来増やしたときにここが壊れないようにしておく。
//
// 待ち行列が空なら ErrNoJob を返す。
func (db *DB) ClaimJob(ctx context.Context) (Job, error) {
	conn, err := db.sql.Conn(ctx)
	if err != nil {
		return Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `begin immediate`); err != nil {
		return Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `rollback`)
		}
	}()

	var job Job
	var kind string
	err = conn.QueryRowContext(ctx, `
		select id, kind, video_id, attempts from jobs
		 where state = 'queued'
		 order by id
		 limit 1`).Scan(&job.ID, &kind, &job.VideoID, &job.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
	}
	job.Kind = JobKind(kind)
	job.Attempts++

	if _, err := conn.ExecContext(ctx, `
		update jobs set state = 'running', attempts = ?, updated_at = ? where id = ?`,
		job.Attempts, time.Now().Unix(), job.ID,
	); err != nil {
		return Job{}, fmt.Errorf("ジョブを専有できません (id=%d): %w", job.ID, err)
	}

	if _, err := conn.ExecContext(ctx, `commit`); err != nil {
		return Job{}, fmt.Errorf("ジョブを専有できません (id=%d): %w", job.ID, err)
	}
	committed = true

	return job, nil
}

// CompleteJob はジョブを完了にする。
func (db *DB) CompleteJob(ctx context.Context, id int64) error {
	_, err := db.sql.ExecContext(ctx,
		`update jobs set state = 'done', last_error = null, updated_at = ? where id = ?`,
		time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("ジョブの完了を記録できません (id=%d): %w", id, err)
	}
	return nil
}

// FailJob は失敗を記録する。試行回数が上限に達していなければ queued へ戻し、
// 達していれば failed で止める（R-106）。
func (db *DB) FailJob(ctx context.Context, id int64, reason string) error {
	_, err := db.sql.ExecContext(ctx, `
		update jobs
		   set state = case when attempts >= ? then 'failed' else 'queued' end,
		       last_error = ?, updated_at = ?
		 where id = ?`,
		MaxJobAttempts, reason, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("ジョブの失敗を記録できません (id=%d): %w", id, err)
	}
	return nil
}

// RequeueRunningJobs は running のまま残っている行を queued へ戻し、その数を
// 返す。起動時に1度だけ呼ぶ。
//
// これがあるので、取り込みの途中でプロセスを止めても次の起動で再開でき、
// 同じ処理を二重に行うこともない（R-106）。
func (db *DB) RequeueRunningJobs(ctx context.Context) (int64, error) {
	res, err := db.sql.ExecContext(ctx,
		`update jobs set state = 'queued', updated_at = ? where state = 'running'`,
		time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("中断したジョブを戻せません: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("中断したジョブを戻せません: %w", err)
	}
	return affected, nil
}

// DeleteFinishedJobsBefore は指定時刻より前に完了・失敗した行を消し、その数を
// 返す。未完了の行は対象にしない。
func (db *DB) DeleteFinishedJobsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.sql.ExecContext(ctx,
		`delete from jobs where state in ('done', 'failed') and updated_at < ?`,
		cutoff.Unix())
	if err != nil {
		return 0, fmt.Errorf("完了したジョブを掃除できません: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("完了したジョブを掃除できません: %w", err)
	}
	return affected, nil
}
