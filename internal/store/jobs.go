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
	var previousPath sql.NullString
	err = conn.QueryRowContext(ctx, `
		select id, kind, video_id, attempts, location_path from jobs
		 where state = 'queued'
		 order by id
		 limit 1`).Scan(&job.ID, &kind, &job.VideoID, &job.Attempts, &previousPath)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
	}
	job.Kind = JobKind(kind)
	job.Attempts++
	var locationID, locationVersion int64
	var locationPath, contentKey string
	selectLocation := `select l.id, l.version, l.path, v.content_key
		from video_locations l join videos v on v.id = l.video_id
		where l.video_id = ?`
	args := []any{job.VideoID}
	if previousPath.Valid {
		selectLocation += ` and l.path > ?`
		args = append(args, previousPath.String)
	}
	selectLocation += ` order by l.path limit 1`
	err = conn.QueryRowContext(ctx, selectLocation, args...).Scan(&locationID, &locationVersion, &locationPath, &contentKey)
	if errors.Is(err, sql.ErrNoRows) && previousPath.Valid {
		err = conn.QueryRowContext(ctx, `select l.id, l.version, l.path, v.content_key
			from video_locations l join videos v on v.id = l.video_id
			where l.video_id = ? order by l.path limit 1`, job.VideoID).
			Scan(&locationID, &locationVersion, &locationPath, &contentKey)
	}
	if errors.Is(err, sql.ErrNoRows) {
		if _, updateErr := conn.ExecContext(ctx, `delete from jobs where id = ?`, job.ID); updateErr != nil {
			return Job{}, updateErr
		}
		if _, commitErr := conn.ExecContext(ctx, `commit`); commitErr != nil {
			return Job{}, commitErr
		}
		committed = true
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("ジョブの処理場所を選べません: %w", err)
	}
	job.ContentKey = contentKey
	job.LocationID = locationID
	job.LocationVersion = locationVersion
	job.LocationPath = locationPath

	if _, err := conn.ExecContext(ctx, `
		update jobs set state = 'running', attempts = ?, location_id = ?, location_version = ?,
		location_path = ?, updated_at = ? where id = ?`,
		job.Attempts, locationID, locationVersion, locationPath, time.Now().Unix(), job.ID,
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
func (db *DB) CompleteClaimedJob(ctx context.Context, job Job) error {
	_, err := db.sql.ExecContext(ctx, `
		update jobs set
		state = case when exists (
			select 1 from video_locations l join videos v on v.id = l.video_id
			where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?
		) then 'done' else 'queued' end,
		last_error = null,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		updated_at = ? where id = ?`,
		job.VideoID, job.ContentKey, job.LocationID, job.LocationVersion, job.LocationPath,
		job.LocationID, job.LocationVersion, job.LocationPath, time.Now().Unix(), job.ID)
	if err != nil {
		return fmt.Errorf("ジョブの完了を記録できません (id=%d): %w", job.ID, err)
	}
	return nil
}

// CompleteJob is retained for administrative callers that do not own a claim snapshot.
func (db *DB) CompleteJob(ctx context.Context, id int64) error {
	_, err := db.sql.ExecContext(ctx, `update jobs set state = 'done', last_error = null, updated_at = ? where id = ?`, time.Now().Unix(), id)
	return err
}

// FailJob は失敗を記録する。試行回数が上限に達していなければ queued へ戻し、
// 達していれば failed で止める（R-106）。
func (db *DB) FailClaimedJob(ctx context.Context, job Job, reason string) error {
	_, err := db.sql.ExecContext(ctx, `
		update jobs
		set state = case
			when not exists (select 1 from videos v join video_locations l on l.video_id = v.id
				where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?)
			then 'queued'
			when attempts >= ? then 'failed' else 'queued' end,
		last_error = ?,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		updated_at = ? where id = ?`,
		job.VideoID, job.ContentKey, job.LocationID, job.LocationVersion, job.LocationPath,
		MaxJobAttempts, reason, job.LocationID, job.LocationVersion, job.LocationPath,
		time.Now().Unix(), job.ID)
	if err != nil {
		return fmt.Errorf("ジョブの失敗を記録できません (id=%d): %w", job.ID, err)
	}
	return nil
}

// FailJob is retained for administrative callers that do not own a claim snapshot.
func (db *DB) FailJob(ctx context.Context, id int64, reason string) error {
	_, err := db.sql.ExecContext(ctx, `update jobs set state = case when attempts >= ? then 'failed' else 'queued' end,
		last_error = ?, updated_at = ? where id = ?`, MaxJobAttempts, reason, time.Now().Unix(), id)
	return err
}

func (db *DB) JobIdentityCurrent(ctx context.Context, job Job) (bool, error) {
	var current int
	err := db.sql.QueryRowContext(ctx, `select exists (
		select 1 from videos v join video_locations l on l.video_id = v.id
		where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?)`,
		job.VideoID, job.ContentKey, job.LocationID, job.LocationVersion, job.LocationPath).Scan(&current)
	return current == 1, err
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
