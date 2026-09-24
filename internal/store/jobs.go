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
	db.notifyJobsQueued(kind)
	return nil
}

// EnsureJob recovers a missing pending job without reviving a terminal failure.
func (db *DB) EnsureJob(ctx context.Context, kind JobKind, videoID int64) error {
	now := time.Now().Unix()
	res, err := db.sql.ExecContext(ctx, `
		insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		select ?, ?, 'queued', 0, ?, ?
		where not exists (select 1 from jobs where kind = ? and video_id = ?)`,
		string(kind), videoID, now, now, string(kind), videoID)
	if err != nil {
		return fmt.Errorf("欠落ジョブを復旧できません (%s, video=%d): %w", kind, videoID, err)
	}
	if inserted, err := res.RowsAffected(); err == nil && inserted > 0 {
		db.notifyJobsQueued(kind)
	}
	return nil
}

// ClaimJob は待ち行列から kind の仕事を1件専有する。
//
// 取り出しと状態の書き換えを begin immediate のトランザクションで囲む。
// select と update を分けると、同じ行を二重に処理する余地が残る。段階ごとに
// ワーカーを置くので、種類の違う仕事は別々のワーカーが同時に取り出す。
//
// その種類の待ち行列が空なら ErrNoJob を返す。
func (db *DB) ClaimJob(ctx context.Context, kind JobKind) (Job, error) {
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
	var kindName string
	var previousPath sql.NullString
	// 登録前の移行locationは保持するが処理しない。folder登録後に同じqueued
	// jobをそのまま再開でき、登録外pathをworkerへ渡すこともない。
	queuedJobSQL := `select j.id, j.kind, j.video_id, j.attempts, j.location_path from jobs j
		where j.state = 'queued' and j.kind = ? and exists (
			select 1 from video_locations l where l.video_id = j.video_id and ` + registeredLocationCondition("l") + `)
		order by j.id limit 1`
	//nolint:gosec // registeredLocationCondition は定型SQLだけを返す。
	err = conn.QueryRowContext(ctx, queuedJobSQL, string(kind)).Scan(&job.ID, &kindName, &job.VideoID, &job.Attempts, &previousPath)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
	}
	job.Kind = JobKind(kindName)
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

// CompleteJob は専有時点の所在を確かめずに完了を記録する。専有の控えを
// 持たない呼び出し（テストの準備など）のために残している。
func (db *DB) CompleteJob(ctx context.Context, id int64) error {
	_, err := db.sql.ExecContext(ctx, `update jobs set state = 'done', last_error = null, updated_at = ? where id = ?`, time.Now().Unix(), id)
	return err
}

// FailJob は専有時点の所在を確かめずに失敗を記録する。専有の控えを持たない
// 呼び出し（テストの準備など）のために残している。
func (db *DB) FailJob(ctx context.Context, id int64, reason string) error {
	_, err := db.sql.ExecContext(ctx, `update jobs set state = case when attempts >= ? then 'failed' else 'queued' end,
		last_error = ?, updated_at = ? where id = ?`, MaxJobAttempts, reason, time.Now().Unix(), id)
	return err
}

// FailClaimedJob は失敗を記録する。試行回数が上限に達していなければ queued へ戻し、
// 達していれば failed で止める。
func (db *DB) FailClaimedJob(ctx context.Context, job Job, reason string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ジョブの失敗記録を開始できません (id=%d): %w", job.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `
		update jobs
		set state = case
			when not exists (select 1 from videos v join video_locations l on l.video_id = v.id
				where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?)
			then 'queued'
			when exists (select 1 from videos where id = ? and location_generation <> ?) then 'queued'
			when attempts >= ? and ? then 'failed' else 'queued' end,
		last_error = ?,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		updated_at = ? where id = ? and state = 'running'`,
		job.VideoID, job.ContentKey, job.LocationID, job.LocationVersion, job.LocationPath,
		job.VideoID, job.LocationGeneration, MaxJobAttempts, job.LastLocation, reason,
		job.LocationID, job.LocationVersion, job.LocationPath,
		now, job.ID)
	if err != nil {
		return fmt.Errorf("ジョブの失敗を記録できません (id=%d): %w", job.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("ジョブの更新件数を確認できません (id=%d): %w", job.ID, err)
	}
	if affected != 1 {
		return fmt.Errorf("実行中のジョブへ失敗を記録できません (id=%d, affected=%d)", job.ID, affected)
	}

	var state string
	if err := tx.QueryRowContext(ctx, `select state from jobs where id = ?`, job.ID).Scan(&state); err != nil {
		return fmt.Errorf("ジョブの失敗状態を確認できません (id=%d): %w", job.ID, err)
	}
	if state == "failed" {
		if err := recordTerminalFailure(ctx, tx, job, reason, now); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ジョブの失敗を確定できません (id=%d): %w", job.ID, err)
	}
	return nil
}

// recordTerminalFailure はジョブの終端失敗を、同じ取引の中で動画側の状態へ
// 記録する。ハンドラが自分で書くと、前段（ファイルの確認など）で上限まで
// 失敗したときに状態が pending のまま残り、また動画が failed になってから
// ジョブが failed になるまでの間に読み取りのやり直しが来ると、新しいジョブの
// 挿入が running の古いジョブとの重複防止で省かれる。ここで記録すれば、
// 「状態が failed なら、その種類のジョブは終わっている」が成り立つ。
//
// どの種類も、claim 時点の内容鍵・所在の世代・所在が今も一致するときに限る。
func recordTerminalFailure(ctx context.Context, tx *sql.Tx, job Job, reason string, now int64) error {
	const identity = `id = ? and content_key = ? and location_generation = ? and exists (
			select 1 from video_locations where id = ? and video_id = ? and version = ? and path = ?
		)`
	identityArgs := []any{job.VideoID, job.ContentKey, job.LocationGeneration,
		job.LocationID, job.VideoID, job.LocationVersion, job.LocationPath}

	switch job.Kind {
	case JobProbe:
		// pending のときだけ書く。probeHandler は結果を保存して done にしたあとで
		// プレビューのジョブを積み、そこで失敗してもエラーを返す。保存済みの結果を
		// 失敗で上書きしないためである。欠けたプレビューのジョブは、次の手動の
		// 取り込みで走査が積み直す（Scanner.ensurePendingJobs）。
		if _, err := tx.ExecContext(ctx, `update videos set probe_state = 'failed', probe_error = ?, playable = 0, updated_at = ?
			where probe_state = 'pending' and `+identity,
			append([]any{reason, now}, identityArgs...)...); err != nil {
			return fmt.Errorf("読み取りの終端失敗を記録できません (job=%d): %w", job.ID, err)
		}
	case JobThumbnail:
		// 代表サムネイルの後でシーク用プレビューだけが失敗した動画は done のまま残す。
		if _, err := tx.ExecContext(ctx, `update videos set thumbnail_state = 'failed', updated_at = ?
			where thumbnail_state <> 'done' and `+identity,
			append([]any{now}, identityArgs...)...); err != nil {
			return fmt.Errorf("サムネイルの終端失敗を記録できません (job=%d): %w", job.ID, err)
		}
	case JobPreview:
		res, err := tx.ExecContext(ctx, `update videos set preview_state = 'failed', updated_at = ?
			where `+identity, append([]any{now}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("プレビューの終端失敗を記録できません (job=%d): %w", job.ID, err)
		}
		updated, rowsErr := res.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("preview 状態の更新件数を確認できません (job=%d): %w", job.ID, rowsErr)
		}
		if updated != 1 {
			return fmt.Errorf("current preview へ終端失敗を記録できません (job=%d, affected=%d)", job.ID, updated)
		}
	}
	return nil
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
	if affected > 0 {
		db.notifyJobsQueued(domain.JobKinds...)
	}
	return affected, nil
}

// ThumbnailJobActive はその動画のサムネイルのジョブが queued か running かを
// 返す。シーク用プレビューの状態（pending か failed か）を導くのに使う。
func (db *DB) ThumbnailJobActive(ctx context.Context, videoID int64) (bool, error) {
	var active int
	err := db.sql.QueryRowContext(ctx, `select exists (select 1 from jobs
		where kind = 'thumbnail' and video_id = ? and state in ('queued', 'running'))`, videoID).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("サムネイルのジョブを確かめられません (video=%d): %w", videoID, err)
	}
	return active == 1, nil
}

// Processing は段階ごとに残っている仕事の数を返す。数えるのは queued と
// running で、ClaimJob と同じく登録済みの所在がある動画に限る。登録外の
// 所在しかない仕事はワーカーが取り出さないので、数えると準備が終わらない。
func (db *DB) Processing(ctx context.Context) (domain.Processing, error) {
	//nolint:gosec // registeredLocationCondition は定型SQLだけを返す。
	query := `select j.kind, count(*) from jobs j
		where j.state in ('queued', 'running') and exists (
			select 1 from video_locations l where l.video_id = j.video_id and ` + registeredLocationCondition("l") + `)
		group by j.kind`
	rows, err := db.sql.QueryContext(ctx, query)
	if err != nil {
		return domain.Processing{}, fmt.Errorf("残りの仕事を数えられません: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out domain.Processing
	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			return domain.Processing{}, fmt.Errorf("残りの仕事を数えられません: %w", err)
		}
		switch JobKind(kind) {
		case JobProbe:
			out.Probe = count
		case JobThumbnail:
			out.Thumbnail = count
		case JobPreview:
			out.Preview = count
		}
	}
	if err := rows.Err(); err != nil {
		return domain.Processing{}, fmt.Errorf("残りの仕事を数えられません: %w", err)
	}
	return out, nil
}
