package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// EnqueueJob はジョブを積む。同じ (kind, video_id) の未完了ジョブが既に
// あれば何もしない（部分ユニーク索引がその状態を保証する）。
//
// 一度諦めた行は消してから積み直す。内容が変わった動画を解析し直せないと、
// 差し替えたファイルが永久に未解析のままになる。諦めた行を残さないのは、
// 再スキャンのたびに履歴が積み上がるのを避けるためである。
func (s *IngestStore) EnqueueJob(ctx context.Context, kind domain.JobKind, videoID int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start queueing the job (%s, video=%d): %w", kind, videoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := requeueJob(ctx, tx, kind, videoID, time.Now().Unix()); err != nil {
		return err
	}
	var c changes
	c.jobsQueued(kind)
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit queueing the job (%s, video=%d): %w", kind, videoID, err)
	}
	return nil
}

// jobStateColumns は仕事の種類ごとに、その結果を持つ動画の列である。
var jobStateColumns = map[domain.JobKind]string{
	domain.JobProbe:         "probe_state",
	domain.JobThumbnail:     "thumbnail_state",
	domain.JobSeekThumbnail: "seek_thumbnail_state",
	domain.JobPreview:       "preview_state",
}

// EnsureJob は、状態が pending の動画に欠けている仕事を積み直す。走査が
// 見つけた pending の動画に対して呼ぶ。
//
// 終端の失敗は動画側の状態へ同じ取引で記録する（recordTerminalFailure）ので、
// 動画が failed なら積まない。動画が pending のまま failed の行だけが残って
// いるのは、失敗を行にだけ記録していた旧版の名残である。その行は捨てて積み
// 直す。残すと、次の手動の取り込みでも直らない。
//
// 指紋は videos に状態の列を持たないので、pending の代わりに「シーク用スプライトが
// 完成しているのに今の版の指紋が無い」を条件にする（fingerprintMissingCondition）。
// 上限まで失敗した failed の行も、完了した done の行も捨てて積み直す（queued・running の
// 行があれば積まない）。失敗した指紋は次の走査で作り直され、版を上げたときも前の版の
// done の行に妨げられずに追いつく（specs/030-video-versions/data-model.md §6）。
func (s *IngestStore) EnsureJob(ctx context.Context, kind domain.JobKind, videoID int64) error {
	var pending string
	discarded := `state = 'failed'`
	if kind == domain.JobFingerprint {
		discarded = `state in ('done', 'failed')`
		pending = `exists (select 1 from videos v where v.id = ? and ` + fingerprintMissingCondition("v") + `)`
	} else {
		column, ok := jobStateColumns[kind]
		if !ok {
			return fmt.Errorf("unknown job kind: %s", kind)
		}
		pending = `exists (select 1 from videos where id = ? and ` + column + ` = 'pending')`
	}

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start restoring missing jobs (%s, video=%d): %w", kind, videoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and video_id = ? and `+discarded+` and `+pending,
		string(kind), videoID, videoID); err != nil {
		return fmt.Errorf("cannot discard finished job rows (%s, video=%d): %w", kind, videoID, err)
	}
	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `
		insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		select ?, ?, 'queued', 0, ?, ?
		where not exists (select 1 from jobs where kind = ? and video_id = ?) and `+pending,
		string(kind), videoID, now, now, string(kind), videoID, videoID)
	if err != nil {
		return fmt.Errorf("cannot restore missing jobs (%s, video=%d): %w", kind, videoID, err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot count restored jobs (%s, video=%d): %w", kind, videoID, err)
	}
	var c changes
	if inserted > 0 {
		if err := addScanVideos(ctx, tx, videoID); err != nil {
			return err
		}
		c.jobsQueued(kind)
	}
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit restoring missing jobs (%s, video=%d): %w", kind, videoID, err)
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
func (s *IngestStore) ClaimJob(ctx context.Context, kind domain.JobKind) (domain.Job, error) {
	conn, err := s.db.sql.Conn(ctx)
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a job: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `begin immediate`); err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a job: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `rollback`)
		}
	}()

	var job domain.Job
	var kindName string
	var previousPath sql.NullString
	// 取り出してよい条件は domain が決める（domain.ClaimConditionFor）。ここでは
	// それを SQL の条件へ写すだけにする。
	queuedJobSQL := `select j.id, j.kind, j.video_id, j.attempts, j.location_path from jobs j
		where j.state = 'queued' and j.kind = ?` + claimConditionSQL(domain.ClaimConditionFor(kind), "j") + `
		order by j.id limit 1`
	err = conn.QueryRowContext(ctx, queuedJobSQL, string(kind)).Scan(&job.ID, &kindName, &job.VideoID, &job.Attempts, &previousPath)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot claim a job: %w", err)
	}
	job.Kind = domain.JobKind(kindName)
	var locationID, locationVersion int64
	var locationPath, contentKey string
	// 直前に試した所在があれば、その続き（path の順で後ろ）から探す。後ろに
	// 無ければ先頭へ戻り、新しい巡回として試行回数を数える。
	selectLocation := `select l.id, l.version, l.path, v.content_key
		from video_locations l join videos v on v.id = l.video_id
		where l.video_id = ? and ` + registeredLocationCondition("l")
	startsRound := !previousPath.Valid
	if previousPath.Valid {
		err = conn.QueryRowContext(ctx, selectLocation+` and l.path > ? order by l.path limit 1`, job.VideoID, previousPath.String).
			Scan(&locationID, &locationVersion, &locationPath, &contentKey)
		startsRound = errors.Is(err, sql.ErrNoRows)
	}
	if startsRound {
		err = conn.QueryRowContext(ctx, selectLocation+` order by l.path limit 1`, job.VideoID).
			Scan(&locationID, &locationVersion, &locationPath, &contentKey)
	}
	job.Attempts = domain.ClaimAttempts(job.Attempts, startsRound)
	if errors.Is(err, sql.ErrNoRows) {
		if _, updateErr := conn.ExecContext(ctx, `delete from jobs where id = ?`, job.ID); updateErr != nil {
			return domain.Job{}, updateErr
		}
		if settleErr := refreshScanSettled(ctx, conn, time.Now().Unix()); settleErr != nil {
			return domain.Job{}, settleErr
		}
		if _, commitErr := conn.ExecContext(ctx, `commit`); commitErr != nil {
			return domain.Job{}, commitErr
		}
		committed = true
		return domain.Job{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("cannot choose a location for the job: %w", err)
	}
	job.ContentKey = contentKey
	job.LocationID = locationID
	job.LocationVersion = locationVersion
	job.LocationPath = locationPath
	var hasLaterLocation int
	if err := conn.QueryRowContext(ctx, `select exists (
		select 1 from video_locations l where l.video_id = ? and l.path > ? and `+registeredLocationCondition("l")+`)`,
		job.VideoID, job.LocationPath).Scan(&hasLaterLocation); err != nil {
		return domain.Job{}, fmt.Errorf("cannot check the job location's final state: %w", err)
	}
	job.LastLocation = hasLaterLocation == 0
	if err := conn.QueryRowContext(ctx, `select location_generation from videos where id = ?`, job.VideoID).
		Scan(&job.LocationGeneration); err != nil {
		return domain.Job{}, fmt.Errorf("cannot check the job's location generation: %w", err)
	}

	if _, err := conn.ExecContext(ctx, `
		update jobs set state = 'running', attempts = ?, location_id = ?, location_version = ?,
		location_path = ?, updated_at = ? where id = ?`,
		job.Attempts, locationID, locationVersion, locationPath, time.Now().Unix(), job.ID,
	); err != nil {
		return domain.Job{}, fmt.Errorf("cannot take ownership of the job (id=%d): %w", job.ID, err)
	}
	if err := refreshScanSettled(ctx, conn, time.Now().Unix()); err != nil {
		return domain.Job{}, err
	}

	if _, err := conn.ExecContext(ctx, `commit`); err != nil {
		return domain.Job{}, fmt.Errorf("cannot take ownership of the job (id=%d): %w", job.ID, err)
	}
	committed = true

	return job, nil
}

// claimConditionSQL は domain.JobClaimCondition を、jobs の行（別名 alias）に
// 対する SQL の条件へ写す。条件が無ければ空文字、あれば先頭に " and " を付けて
// 返す。どの条件も domain.JobClaimCondition.Allows と同じ判断になるように書く。
func claimConditionSQL(c domain.JobClaimCondition, alias string) string {
	var cond string
	if c.RegisteredLocation {
		cond += ` and exists (select 1 from video_locations l where l.video_id = ` + alias + `.video_id and ` +
			registeredLocationCondition("l") + `)`
	}
	if c.ProbeFinished {
		cond += ` and exists (select 1 from videos v where v.id = ` + alias + `.video_id and v.probe_state <> '` +
			string(domain.ProbeStatePending) + `')`
	}
	if c.SeekThumbnailFinished {
		cond += ` and exists (select 1 from videos v where v.id = ` + alias + `.video_id and v.seek_thumbnail_state = '` +
			string(domain.SeekThumbnailDone) + `')`
	}
	if c.NoClaimableThumbnail {
		// 取り出せる thumbnail の仕事は残りの仕事（remainingJobCondition）と同じ範囲で、解析待ちで
		// 今は取り出せないものも含める。
		cond += ` and not exists (select 1 from jobs t where t.kind = '` + string(domain.JobThumbnail) +
			`' and t.state in ('queued', 'running') and exists (
				select 1 from video_locations tl where tl.video_id = t.video_id and ` +
			registeredLocationCondition("tl") + `))`
	}
	return cond
}

// CompleteClaimedJob はジョブを完了にする。専有した時点の所在と内容が今も同じなら
// done に、変わっていれば queued へ戻す。
func (s *IngestStore) CompleteClaimedJob(ctx context.Context, job domain.Job) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start recording job completion (id=%d): %w", job.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
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
		return fmt.Errorf("cannot record job completion (id=%d): %w", job.ID, err)
	}
	var c changes
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit job completion (id=%d): %w", job.ID, err)
	}
	return nil
}

// CompleteJob は専有時点の所在を確かめずに完了を記録する。専有の控えを
// 持たない呼び出し（テストの準備など）のために残している。
func (s *IngestStore) CompleteJob(ctx context.Context, id int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `update jobs set state = 'done', last_error = null, updated_at = ? where id = ?`,
		time.Now().Unix(), id); err != nil {
		return err
	}
	var c changes
	c.remainingChanged()
	return s.db.commit(ctx, tx, &c)
}

// FailJob は専有時点の所在を確かめずに失敗を記録する。専有の控えを持たない
// 呼び出し（テストの準備など）のために残している。
func (s *IngestStore) FailJob(ctx context.Context, id int64, reason string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var attempts int
	err = tx.QueryRowContext(ctx, `select attempts from jobs where id = ?`, id).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	state := domain.JobStateAfterFailure(attempts, true, true)
	if _, err := tx.ExecContext(ctx, `update jobs set state = ?, last_error = ?, updated_at = ? where id = ?`,
		string(state), reason, time.Now().Unix(), id); err != nil {
		return err
	}
	var c changes
	c.remainingChanged()
	return s.db.commit(ctx, tx, &c)
}

// FailClaimedJob は失敗を記録する。queued へ戻すか failed で止めるかは
// domain.JobStateAfterFailure が決め、ここではその結果を書く。failed なら、
// 動画側の状態へも同じ取引で記録する（recordTerminalFailure）。
//
// cause の文を jobs.last_error に書く。解析の終端失敗では、domain.ProbeFailure で
// 包まれた理由のコードも動画側へ書く（specs/023-english-i18n/data-model.md §1）。
func (s *IngestStore) FailClaimedJob(ctx context.Context, job domain.Job, cause error) error {
	reason := cause.Error()
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start recording the job failure (id=%d): %w", job.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	var attempts int
	err = tx.QueryRowContext(ctx, `select attempts from jobs where id = ? and state = 'running'`, job.ID).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("cannot record a failure on the running job (id=%d, affected=0)", job.ID)
	}
	if err != nil {
		return fmt.Errorf("cannot check the job attempt count (id=%d): %w", job.ID, err)
	}
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil {
		return fmt.Errorf("cannot check the job location (id=%d): %w", job.ID, err)
	}
	state := domain.JobStateAfterFailure(attempts, job.LastLocation, current)

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx, `
		update jobs
		set state = ?,
		last_error = ?,
		location_id = case when exists (select 1 from video_locations where id = ? and version = ? and path = ?) then location_id else null end,
		updated_at = ? where id = ? and state = 'running'`,
		string(state), reason,
		job.LocationID, job.LocationVersion, job.LocationPath,
		now, job.ID)
	if err != nil {
		return fmt.Errorf("cannot record the job failure (id=%d): %w", job.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot check the job update count (id=%d): %w", job.ID, err)
	}
	if affected != 1 {
		return fmt.Errorf("cannot record a failure on the running job (id=%d, affected=%d)", job.ID, affected)
	}

	if state == domain.JobFailed {
		if err := recordTerminalFailure(ctx, tx, job, cause, now); err != nil {
			return err
		}
	}
	var c changes
	c.remainingChanged()
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return fmt.Errorf("cannot commit the job failure (id=%d): %w", job.ID, err)
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
func recordTerminalFailure(ctx context.Context, tx *sql.Tx, job domain.Job, cause error, now int64) error {
	const identity = `id = ? and content_key = ? and location_generation = ? and exists (
			select 1 from video_locations where id = ? and video_id = ? and version = ? and path = ?
		)`
	identityArgs := []any{job.VideoID, job.ContentKey, job.LocationGeneration,
		job.LocationID, job.VideoID, job.LocationVersion, job.LocationPath}

	// updated は動画の段階を failed にした文の結果である。書いたときだけ問題を記録する。
	var updated sql.Result
	switch job.Kind {
	case domain.JobProbe:
		// pending のときだけ書く。解析の結果を保存して done にしたあとの失敗
		// （仕事の完了の記録など）で、保存済みの結果を失敗で上書きしないためである。
		res, err := tx.ExecContext(ctx, `update videos set probe_state = 'failed', probe_error = ?, probe_error_code = ?,
			playable = 0, updated_at = ?
			where probe_state = 'pending' and `+identity,
			append([]any{cause.Error(), string(domain.ProbeErrorCodeOf(cause)), now}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("cannot record the final probe failure (job=%d): %w", job.ID, err)
		}
		updated = res
	case domain.JobThumbnail:
		// 代表サムネイルの後でシーク用プレビューだけが失敗した動画は done のまま残す。
		// seek_thumbnail_state はシーク用の仕事が自分で記録するので、ここでは変えない。
		res, err := tx.ExecContext(ctx, `update videos set thumbnail_state = 'failed', updated_at = ?
			where thumbnail_state <> 'done' and `+identity,
			append([]any{now}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("cannot record the final thumbnail failure (job=%d): %w", job.ID, err)
		}
		updated = res
	case domain.JobSeekThumbnail:
		// seek_thumbnail_state だけを failed にする。代表サムネイルは別の仕事の結果である。
		res, err := tx.ExecContext(ctx, `update videos set seek_thumbnail_state = 'failed', updated_at = ?
			where seek_thumbnail_state <> 'done' and `+identity,
			append([]any{now}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("cannot record the final seek thumbnail failure (job=%d): %w", job.ID, err)
		}
		updated = res
	case domain.JobFingerprint:
		// 指紋は動画に状態の列を持たない。専有した時点の内容と所在が今も同じときだけ、
		// 問題として記録する。積み直しは次の走査が行う（EnsureJob）。
		var current int
		if err := tx.QueryRowContext(ctx, `select exists (select 1 from videos where `+identity+`)`,
			identityArgs...).Scan(&current); err != nil {
			return fmt.Errorf("cannot check the fingerprint job identity (job=%d): %w", job.ID, err)
		}
		if current != 1 {
			return nil
		}
		return recordFailedIssue(ctx, tx, job, now)
	case domain.JobPreview:
		res, err := tx.ExecContext(ctx, `update videos set preview_state = 'failed', updated_at = ?
			where `+identity, append([]any{now}, identityArgs...)...)
		if err != nil {
			return fmt.Errorf("cannot record the final preview failure (job=%d): %w", job.ID, err)
		}
		affected, rowsErr := res.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("cannot check the preview state update count (job=%d): %w", job.ID, rowsErr)
		}
		if affected != 1 {
			return fmt.Errorf("cannot record the final failure on the current preview (job=%d, affected=%d)", job.ID, affected)
		}
		return recordFailedIssue(ctx, tx, job, now)
	}
	if updated == nil {
		return nil
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot check the final failure update count (job=%d): %w", job.ID, err)
	}
	if count != 1 {
		return nil
	}
	return recordFailedIssue(ctx, tx, job, now)
}

// recordFailedIssue は、動画の段階を failed にした取引で、その段階の *_failed を直近の
// 取り込みの問題として記録する（specs/024-import-progress/data-model.md §3）。上限の
// 手前の失敗は記録しない。
func recordFailedIssue(ctx context.Context, tx *sql.Tx, job domain.Job, now int64) error {
	issue, ok := domain.FailedIssueKind(job.Kind)
	if !ok {
		return nil
	}
	return recordScanIssue(ctx, tx, job.VideoID, job.LocationPath, issue, now)
}

// JobIdentityCurrent は、専有したときの内容鍵・所在・所在の世代が今も
// 一致するかを返す。
func (s *IngestStore) JobIdentityCurrent(ctx context.Context, job domain.Job) (bool, error) {
	return jobIdentityCurrent(ctx, s.db.sql, job)
}

// rowQueryer は *sql.DB と *sql.Tx の共通部分のうち、1行を読むものである。
type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func jobIdentityCurrent(ctx context.Context, q rowQueryer, job domain.Job) (bool, error) {
	var current int
	err := q.QueryRowContext(ctx, `select exists (
		select 1 from videos v join video_locations l on l.video_id = v.id
		where v.id = ? and v.content_key = ? and l.id = ? and l.version = ? and l.path = ?
		and v.location_generation = ?)`, job.VideoID, job.ContentKey, job.LocationID,
		job.LocationVersion, job.LocationPath, job.LocationGeneration).Scan(&current)
	return current == 1, err
}

// HasUnfinishedJobs は queued か running の仕事が 1 件以上あるかを返す。デスクトップ版の
// 閉じる確認が、取り込みの途中かを判断するのに使う（specs/037-windows-app/research.md R-7）。
func (s *IngestStore) HasUnfinishedJobs(ctx context.Context) (bool, error) {
	var found bool
	err := s.db.sql.QueryRowContext(ctx,
		`select exists (select 1 from jobs where state in ('queued', 'running'))`).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("cannot look up unfinished jobs: %w", err)
	}
	return found, nil
}

// RequeueRunningJobs は running のまま残っている行を queued へ戻し、その数を
// 返す。起動時に1度だけ呼ぶ。
//
// これがあるので、取り込みの途中でプロセスを止めても次の起動で再開でき、
// 同じ処理を二重に行うこともない。
//
// 戻す行が無くても、直近の取り込みの完了の時刻は実行時の条件で決め直す。移行
// （00018_scan_import.sql）は着手できるかを OS に依らない条件で判定するので、
// その後に書き込みが無くても、起動のたびにここで揃う。
func (s *IngestStore) RequeueRunningJobs(ctx context.Context) (int64, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`update jobs set state = 'queued', updated_at = ? where state = 'running'`,
		time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}
	var c changes
	c.remainingChanged()
	if affected > 0 {
		c.jobsQueued(domain.JobKinds...)
	}
	if err := s.db.commit(ctx, tx, &c); err != nil {
		return 0, fmt.Errorf("cannot requeue interrupted jobs: %w", err)
	}
	return affected, nil
}
