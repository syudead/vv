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
	JobPreview   = domain.JobPreview
	// MaxJobAttempts は諦めるまでの試行回数である。
	MaxJobAttempts = domain.MaxJobAttempts
)

// ErrNoJob は待ち行列が空であることを表す。
var ErrNoJob = domain.ErrNoJob

// JobRetention は完了したジョブを残す期間である。取り込み直後に最大 2万行に
// なるため、放置せず掃除する。
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

// EnsureJob recovers a missing pending job without reviving a terminal failure.
func (db *DB) EnsureJob(ctx context.Context, kind JobKind, videoID int64) error {
	now := time.Now().Unix()
	_, err := db.sql.ExecContext(ctx, `
		insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		select ?, ?, 'queued', 0, ?, ?
		where not exists (select 1 from jobs where kind = ? and video_id = ?)`,
		string(kind), videoID, now, now, string(kind), videoID)
	if err != nil {
		return fmt.Errorf("欠落ジョブを復旧できません (%s, video=%d): %w", kind, videoID, err)
	}
	return nil
}

// ClaimJob は待ち行列から1件を専有する。
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
	// 登録前の移行locationは保持するが処理しない。folder登録後に同じqueued
	// jobをそのまま再開でき、登録外pathをworkerへ渡すこともない。
	queuedJobSQL := `select j.id, j.kind, j.video_id, j.attempts, j.location_path from jobs j
		where j.state = 'queued' and exists (
			select 1 from video_locations l where l.video_id = j.video_id and ` + registeredLocationCondition("l") + `)
		order by j.id limit 1`
	//nolint:gosec // registeredLocationCondition は定型SQLだけを返す。
	err = conn.QueryRowContext(ctx, queuedJobSQL).Scan(&job.ID, &kind, &job.VideoID, &job.Attempts, &previousPath)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
	}
	job.Kind = JobKind(kind)
	if !previousPath.Valid {
		job.Attempts++
	}
	var locationID, locationVersion int64
	var locationPath, contentKey string
	selectLocation := `select l.id, l.version, l.path, v.content_key
		from video_locations l join videos v on v.id = l.video_id
		where l.video_id = ? and ` + registeredLocationCondition("l")
	args := []any{job.VideoID}
	if previousPath.Valid {
		selectLocation += ` and l.path > ?`
		args = append(args, previousPath.String)
	}
	selectLocation += ` order by l.path limit 1`
	err = conn.QueryRowContext(ctx, selectLocation, args...).Scan(&locationID, &locationVersion, &locationPath, &contentKey)
	if errors.Is(err, sql.ErrNoRows) && previousPath.Valid {
		job.Attempts++
		err = conn.QueryRowContext(ctx, `select l.id, l.version, l.path, v.content_key
			from video_locations l join videos v on v.id = l.video_id
			where l.video_id = ? and `+registeredLocationCondition("l")+` order by l.path limit 1`, job.VideoID).
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
	var hasLaterLocation int
	if err := conn.QueryRowContext(ctx, `select exists (
		select 1 from video_locations l where l.video_id = ? and l.path > ? and `+registeredLocationCondition("l")+`)`,
		job.VideoID, job.LocationPath).Scan(&hasLaterLocation); err != nil {
		return Job{}, fmt.Errorf("ジョブの処理場所の終端を確認できません: %w", err)
	}
	job.LastLocation = hasLaterLocation == 0
	if err := conn.QueryRowContext(ctx, `select location_generation from videos where id = ?`, job.VideoID).
		Scan(&job.LocationGeneration); err != nil {
		return Job{}, fmt.Errorf("ジョブのlocation世代を確認できません: %w", err)
	}

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
			and v.location_generation = ?
		) then 'done' else 'queued' end,
		last_error = null,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		updated_at = ? where id = ? and state = 'running'`,
		job.VideoID, job.ContentKey, job.LocationID, job.LocationVersion, job.LocationPath, job.LocationGeneration,
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
// 達していれば failed で止める。
func (db *DB) FailClaimedJob(ctx context.Context, job Job, reason string) error {
	_, err := db.sql.ExecContext(ctx, `
		update jobs
		set state = case
			when not exists (select 1 from videos v join video_locations l on l.video_id = v.id
				where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?)
			then 'queued'
			when exists (select 1 from videos where id = ? and location_generation <> ?) then 'queued'
			when attempts >= ? and ? then 'failed' else 'queued' end,
		last_error = ?,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		updated_at = ? where id = ?`,
		job.VideoID, job.ContentKey, job.LocationID, job.LocationVersion, job.LocationPath,
		job.VideoID, job.LocationGeneration, MaxJobAttempts, job.LastLocation, reason,
		job.LocationID, job.LocationVersion, job.LocationPath,
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
		where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?
		and v.location_generation = ?)`, job.VideoID, job.ContentKey, job.LocationID,
		job.LocationVersion, job.LocationPath, job.LocationGeneration).Scan(&current)
	return current == 1, err
}

// RequeueRunningJobs は running のまま残っている行を queued へ戻し、その数を
// 返す。起動時に1度だけ呼ぶ。
//
// これがあるので、取り込みの途中でプロセスを止めても次の起動で再開でき、
// 同じ処理を二重に行うこともない。
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
// 返す。location固有の終端失敗は論理stateがpendingの間は再試行抑止記録として残す。
func (db *DB) DeleteFinishedJobsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.sql.ExecContext(ctx,
		`delete from jobs where updated_at < ? and (
			state = 'done' or (state = 'failed' and exists (
				select 1 from videos v where v.id = jobs.video_id and (
					(jobs.kind = 'probe' and v.probe_state <> 'pending') or
					(jobs.kind = 'thumbnail' and v.thumbnail_state <> 'pending') or
					(jobs.kind = 'preview' and v.preview_state <> 'pending')
				)
			))
		)`,
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
