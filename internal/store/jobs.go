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
	if _, err := s.db.sql.ExecContext(ctx,
		`delete from jobs where kind = ? and video_id = ? and state in ('done', 'failed')`,
		string(kind), videoID,
	); err != nil {
		return fmt.Errorf("古いジョブを掃除できません: %w", err)
	}

	now := time.Now().Unix()
	_, err := s.db.sql.ExecContext(ctx, `
		insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		values (?, ?, 'queued', 0, ?, ?)
		on conflict (kind, video_id) where state in ('queued', 'running') do nothing`,
		string(kind), videoID, now, now,
	)
	if err != nil {
		return fmt.Errorf("ジョブを積めません (%s, video=%d): %w", kind, videoID, err)
	}
	var c changes
	c.jobsQueued(kind)
	s.db.publish(&c)
	return nil
}

// jobStateColumns は仕事の種類ごとに、その結果を持つ動画の列である。
var jobStateColumns = map[domain.JobKind]string{
	domain.JobProbe:     "probe_state",
	domain.JobThumbnail: "thumbnail_state",
	domain.JobPreview:   "preview_state",
}

// EnsureJob は、状態が pending の動画に欠けている仕事を積み直す。走査が
// 見つけた pending の動画に対して呼ぶ。
//
// 終端の失敗は動画側の状態へ同じ取引で記録する（recordTerminalFailure）ので、
// 動画が failed なら積まない。動画が pending のまま failed の行だけが残って
// いるのは、失敗を行にだけ記録していた旧版の名残である。その行は捨てて積み
// 直す。残すと、次の手動の取り込みでも直らない。
func (s *IngestStore) EnsureJob(ctx context.Context, kind domain.JobKind, videoID int64) error {
	column, ok := jobStateColumns[kind]
	if !ok {
		return fmt.Errorf("未知の仕事の種類です: %s", kind)
	}
	pending := `exists (select 1 from videos where id = ? and ` + column + ` = 'pending')`

	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("欠落ジョブの復旧を開始できません (%s, video=%d): %w", kind, videoID, err)
	}
	defer func() { _ = tx.Rollback() }()
	//nolint:gosec // 組み立てるのは定型の列名だけで、値はすべて引数で渡す。
	if _, err := tx.ExecContext(ctx, `delete from jobs where kind = ? and video_id = ? and state = 'failed' and `+pending,
		string(kind), videoID, videoID); err != nil {
		return fmt.Errorf("旧版の失敗の行を捨てられません (%s, video=%d): %w", kind, videoID, err)
	}
	now := time.Now().Unix()
	//nolint:gosec // 組み立てるのは定型の列名だけで、値はすべて引数で渡す。
	res, err := tx.ExecContext(ctx, `
		insert into jobs (kind, video_id, state, attempts, created_at, updated_at)
		select ?, ?, 'queued', 0, ?, ?
		where not exists (select 1 from jobs where kind = ? and video_id = ?) and `+pending,
		string(kind), videoID, now, now, string(kind), videoID, videoID)
	if err != nil {
		return fmt.Errorf("欠落ジョブを復旧できません (%s, video=%d): %w", kind, videoID, err)
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("復旧したジョブを数えられません (%s, video=%d): %w", kind, videoID, err)
	}
	var c changes
	if inserted > 0 {
		c.jobsQueued(kind)
	}
	if err := s.db.commit(tx, &c); err != nil {
		return fmt.Errorf("欠落ジョブの復旧を確定できません (%s, video=%d): %w", kind, videoID, err)
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
		return domain.Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `begin immediate`); err != nil {
		return domain.Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
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
	//nolint:gosec // claimConditionSQL は定型SQLだけを返す。
	queuedJobSQL := `select j.id, j.kind, j.video_id, j.attempts, j.location_path from jobs j
		where j.state = 'queued' and j.kind = ?` + claimConditionSQL(domain.ClaimConditionFor(kind), "j") + `
		order by j.id limit 1`
	err = conn.QueryRowContext(ctx, queuedJobSQL, string(kind)).Scan(&job.ID, &kindName, &job.VideoID, &job.Attempts, &previousPath)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("ジョブを取り出せません: %w", err)
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
		if _, commitErr := conn.ExecContext(ctx, `commit`); commitErr != nil {
			return domain.Job{}, commitErr
		}
		committed = true
		return domain.Job{}, domain.ErrNoJob
	}
	if err != nil {
		return domain.Job{}, fmt.Errorf("ジョブの処理場所を選べません: %w", err)
	}
	job.ContentKey = contentKey
	job.LocationID = locationID
	job.LocationVersion = locationVersion
	job.LocationPath = locationPath
	var hasLaterLocation int
	if err := conn.QueryRowContext(ctx, `select exists (
		select 1 from video_locations l where l.video_id = ? and l.path > ? and `+registeredLocationCondition("l")+`)`,
		job.VideoID, job.LocationPath).Scan(&hasLaterLocation); err != nil {
		return domain.Job{}, fmt.Errorf("ジョブの処理場所の終端を確認できません: %w", err)
	}
	job.LastLocation = hasLaterLocation == 0
	if err := conn.QueryRowContext(ctx, `select location_generation from videos where id = ?`, job.VideoID).
		Scan(&job.LocationGeneration); err != nil {
		return domain.Job{}, fmt.Errorf("ジョブのlocation世代を確認できません: %w", err)
	}

	if _, err := conn.ExecContext(ctx, `
		update jobs set state = 'running', attempts = ?, location_id = ?, location_version = ?,
		location_path = ?, updated_at = ? where id = ?`,
		job.Attempts, locationID, locationVersion, locationPath, time.Now().Unix(), job.ID,
	); err != nil {
		return domain.Job{}, fmt.Errorf("ジョブを専有できません (id=%d): %w", job.ID, err)
	}

	if _, err := conn.ExecContext(ctx, `commit`); err != nil {
		return domain.Job{}, fmt.Errorf("ジョブを専有できません (id=%d): %w", job.ID, err)
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
	return cond
}

// CompleteJob はジョブを完了にする。
func (s *IngestStore) CompleteClaimedJob(ctx context.Context, job domain.Job) error {
	_, err := s.db.sql.ExecContext(ctx, `
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
func (s *IngestStore) CompleteJob(ctx context.Context, id int64) error {
	_, err := s.db.sql.ExecContext(ctx, `update jobs set state = 'done', last_error = null, updated_at = ? where id = ?`, time.Now().Unix(), id)
	return err
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
	return tx.Commit()
}

// FailClaimedJob は失敗を記録する。queued へ戻すか failed で止めるかは
// domain.JobStateAfterFailure が決め、ここではその結果を書く。failed なら、
// 動画側の状態へも同じ取引で記録する（recordTerminalFailure）。
func (s *IngestStore) FailClaimedJob(ctx context.Context, job domain.Job, reason string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ジョブの失敗記録を開始できません (id=%d): %w", job.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	var attempts int
	err = tx.QueryRowContext(ctx, `select attempts from jobs where id = ? and state = 'running'`, job.ID).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("実行中のジョブへ失敗を記録できません (id=%d, affected=0)", job.ID)
	}
	if err != nil {
		return fmt.Errorf("ジョブの試行回数を確認できません (id=%d): %w", job.ID, err)
	}
	current, err := jobIdentityCurrent(ctx, tx, job)
	if err != nil {
		return fmt.Errorf("ジョブの処理場所を確認できません (id=%d): %w", job.ID, err)
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
		return fmt.Errorf("ジョブの失敗を記録できません (id=%d): %w", job.ID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("ジョブの更新件数を確認できません (id=%d): %w", job.ID, err)
	}
	if affected != 1 {
		return fmt.Errorf("実行中のジョブへ失敗を記録できません (id=%d, affected=%d)", job.ID, affected)
	}

	if state == domain.JobFailed {
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
func recordTerminalFailure(ctx context.Context, tx *sql.Tx, job domain.Job, reason string, now int64) error {
	const identity = `id = ? and content_key = ? and location_generation = ? and exists (
			select 1 from video_locations where id = ? and video_id = ? and version = ? and path = ?
		)`
	identityArgs := []any{job.VideoID, job.ContentKey, job.LocationGeneration,
		job.LocationID, job.VideoID, job.LocationVersion, job.LocationPath}

	switch job.Kind {
	case domain.JobProbe:
		// pending のときだけ書く。app.Ingest.Probe は結果を保存して done にしたあとで
		// プレビューのジョブを積み、そこで失敗してもエラーを返す。保存済みの結果を
		// 失敗で上書きしないためである。欠けたプレビューのジョブは、次の手動の
		// 取り込みで走査が積み直す（Scanner.ensurePendingJobs）。
		if _, err := tx.ExecContext(ctx, `update videos set probe_state = 'failed', probe_error = ?, playable = 0, updated_at = ?
			where probe_state = 'pending' and `+identity,
			append([]any{reason, now}, identityArgs...)...); err != nil {
			return fmt.Errorf("読み取りの終端失敗を記録できません (job=%d): %w", job.ID, err)
		}
	case domain.JobThumbnail:
		// 代表サムネイルの後でシーク用プレビューだけが失敗した動画は done のまま残す。
		if _, err := tx.ExecContext(ctx, `update videos set thumbnail_state = 'failed', updated_at = ?
			where thumbnail_state <> 'done' and `+identity,
			append([]any{now}, identityArgs...)...); err != nil {
			return fmt.Errorf("サムネイルの終端失敗を記録できません (job=%d): %w", job.ID, err)
		}
	case domain.JobPreview:
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

// RequeueRunningJobs は running のまま残っている行を queued へ戻し、その数を
// 返す。起動時に1度だけ呼ぶ。
//
// これがあるので、取り込みの途中でプロセスを止めても次の起動で再開でき、
// 同じ処理を二重に行うこともない。
func (s *IngestStore) RequeueRunningJobs(ctx context.Context) (int64, error) {
	res, err := s.db.sql.ExecContext(ctx,
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
		var c changes
		c.jobsQueued(domain.JobKinds...)
		s.db.publish(&c)
	}
	return affected, nil
}

// ThumbnailJobActive はその動画のサムネイルのジョブが queued か running かを
// 返す。シーク用プレビューの状態（pending か failed か）を導くのに使う。
func (s *IngestStore) ThumbnailJobActive(ctx context.Context, videoID int64) (bool, error) {
	var active int
	err := s.db.sql.QueryRowContext(ctx, `select exists (select 1 from jobs
		where kind = 'thumbnail' and video_id = ? and state in ('queued', 'running'))`, videoID).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("サムネイルのジョブを確かめられません (video=%d): %w", videoID, err)
	}
	return active == 1, nil
}

// Processing は段階ごとに残っている仕事の数を返す。数えるのは queued と
// running で、ClaimJob と同じく登録済みの所在がある動画に限る。登録外の
// 所在しかない仕事はワーカーが取り出さないので、数えると準備が終わらない。
func (s *IngestStore) Processing(ctx context.Context) (domain.Processing, error) {
	//nolint:gosec // registeredLocationCondition は定型SQLだけを返す。
	query := `select j.kind, count(*) from jobs j
		where j.state in ('queued', 'running') and exists (
			select 1 from video_locations l where l.video_id = j.video_id and ` + registeredLocationCondition("l") + `)
		group by j.kind`
	rows, err := s.db.sql.QueryContext(ctx, query)
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
		switch domain.JobKind(kind) {
		case domain.JobProbe:
			out.Probe = count
		case domain.JobThumbnail:
			out.Thumbnail = count
		case domain.JobPreview:
			out.Preview = count
		}
	}
	if err := rows.Err(); err != nil {
		return domain.Processing{}, fmt.Errorf("残りの仕事を数えられません: %w", err)
	}
	return out, nil
}
