package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 取り込みと失敗した走査の記録（specs/055-activity/data-model.md「Rules」、research.md R-3）。1 本の動画の
// 取り込みは 5 つの段階をまとめて 1 件の import の記録にし、仕事の行（jobs.activity_id）がその記録を指す。
// 仕事を積む・取り出す・終える取引が、同じ取引でここを呼んで記録を開き、状態をまとめ直す。

// pendingJobCondition は待っているか走っている仕事の行（別名 alias）の条件である。
func pendingJobCondition(alias string) string {
	return alias + `.state in ('queued', 'running')`
}

// videoActivitySubject は動画 videoID の記録の対象を、今の内容の鍵と表示名で返す。表示名は上書きの表示名、
// 無ければパスの順で最初の所在の題名である（research.md R-10）。
func videoActivitySubject(ctx context.Context, q rowQueryer, videoID int64) (domain.ActivitySubject, error) {
	subject := domain.ActivitySubject{Kind: domain.ActivitySubjectVideo}
	err := q.QueryRowContext(ctx, `select v.content_key, coalesce(
			(select ov.display_name from video_overrides ov where ov.content_key = v.content_key and v.content_key <> ''),
			(select l.title from video_locations l where l.video_id = v.id order by l.path limit 1), '')
		from videos v where v.id = ?`, videoID).Scan(&subject.Key, &subject.Label)
	if err != nil {
		return domain.ActivitySubject{}, fmt.Errorf("cannot read the activity subject (video=%d): %w", videoID, err)
	}
	return subject, nil
}

// joinImportActivity は動画 videoID の待っているか走っている仕事を、1 件の running の import の記録に
// まとめる。そうした記録が無ければ、decider が始めた記録を running/waiting で開く。終わった記録は
// 生き返らせない（再スキャン・読み取りのやり直し・再起動のあとの仕事は新しい記録になる）。待つか走る仕事が
// 無ければ何もしない。最後に記録の状態をまとめ直す（settleImportActivity）。
func joinImportActivity(
	ctx context.Context, q activityWriter, c *changes, videoID int64, decider domain.ActivityDecider, now time.Time,
) error {
	var id int64
	err := q.QueryRowContext(ctx, `select j.activity_id from jobs j join activity_entries e on e.id = j.activity_id
		where j.video_id = ? and `+pendingJobCondition("j")+` and e.status = ? order by j.id limit 1`,
		videoID, string(domain.ActivityRunning)).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		var pending bool
		if err := q.QueryRowContext(ctx, `select exists (select 1 from jobs j where j.video_id = ? and `+
			pendingJobCondition("j")+`)`, videoID).Scan(&pending); err != nil {
			return fmt.Errorf("cannot read the pending jobs (video=%d): %w", videoID, err)
		}
		if !pending {
			return nil
		}
		subject, err := videoActivitySubject(ctx, q, videoID)
		if err != nil {
			return err
		}
		id, err = recordActivity(ctx, q, c, domain.ActivityEntry{
			Kind: domain.ActivityImport, Subject: subject, Status: domain.ActivityRunning,
			Reason: domain.ActivityWaiting, Decider: decider,
		}, now)
		if err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("cannot read the running import activity (video=%d): %w", videoID, err)
	}
	if _, err := q.ExecContext(ctx, `update jobs set activity_id = ? where video_id = ? and `+
		pendingJobCondition("jobs")+` and activity_id is not ?`, id, videoID, id); err != nil {
		return fmt.Errorf("cannot attach the jobs to the import activity (video=%d): %w", videoID, err)
	}
	return settleImportActivity(ctx, q, c, id, now)
}

// settleJobActivity は仕事 jobID が属する import の記録の状態をまとめ直す。記録を持たない仕事（この機能より
// 前に積んだもの）では何もしない。
func settleJobActivity(ctx context.Context, q activityWriter, c *changes, jobID int64, now time.Time) error {
	var id sql.NullInt64
	err := q.QueryRowContext(ctx, `select activity_id from jobs where id = ?`, jobID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !id.Valid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the job's activity (job=%d): %w", jobID, err)
	}
	return settleImportActivity(ctx, q, c, id.Int64, now)
}

// settleImportActivity は running の import の記録 id の状態を、その記録に属する仕事の状態から決め直す
// （domain.ImportActivityState）。諦めた仕事があれば、段階の順ではなく id の順で最初のものの種類を stage に、
// その文を detail に書く。running でない記録は変えない。状態が変わらなければ書かない。
func settleImportActivity(ctx context.Context, q activityWriter, c *changes, id int64, now time.Time) error {
	var current domain.ActivityState
	var status, reason string
	err := q.QueryRowContext(ctx, `select status, coalesce(reason, '') from activity_entries where id = ?`, id).
		Scan(&status, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the import activity (id=%d): %w", id, err)
	}
	current.Status, current.Reason = domain.ActivityStatus(status), domain.ActivityReason(reason)
	if current.Status != domain.ActivityRunning {
		return nil
	}
	jobs, failure, err := importActivityJobs(ctx, q, id)
	if err != nil {
		return err
	}
	states := make([]domain.JobState, 0, len(jobs))
	for _, job := range jobs {
		states = append(states, job.State)
	}
	next := domain.ImportActivityState(states)
	if next == current {
		return nil
	}
	if err := setActivityState(ctx, q, c, id, next, now); err != nil {
		return err
	}
	if next.Reason != domain.ActivityFailed {
		return nil
	}
	if _, err := q.ExecContext(ctx, `update activity_entries set stage = ?, detail = ? where id = ?`,
		string(failure.kind), nullString(failure.message), id); err != nil {
		return fmt.Errorf("cannot record the failed import stage (id=%d): %w", id, err)
	}
	return nil
}

// goneImportCondition は running の import の記録（別名 e）のうち、待つか走る仕事がもう無いものの条件である。
const goneImportCondition = `e.kind = 'import' and e.status = 'running' and not exists (
	select 1 from jobs j where j.activity_id = e.id and j.state in ('queued', 'running'))`

// closeGoneImportActivity は import の記録 id を、待つか走る仕事がもう無い running なら done/gone で閉じる。
// 登録の所在を失った動画の最後の仕事を捨てたときに呼ぶ（data-model.md「Outcomes」の gone）。閉じたら
// true を返す。
func closeGoneImportActivity(ctx context.Context, q activityWriter, c *changes, id int64, now time.Time) (bool, error) {
	return closeGoneImports(ctx, q, c, `e.id = ?`, now, id)
}

// closeDeletedVideoImports は消した動画 deleted の running の import の記録のうち、待つか走る仕事が無くなった
// ものを done/gone で閉じる。仕事は動画の行と連鎖して消えているので、記録は対象の内容の鍵で引く。動画の行を
// 消す取引の確定（DB.commit）から呼ぶ。
func closeDeletedVideoImports(ctx context.Context, q activityWriter, c *changes, deleted []domain.DeletedVideo) error {
	if len(deleted) == 0 {
		return nil
	}
	keys := make([]string, 0, len(deleted))
	for _, video := range deleted {
		keys = append(keys, video.ContentKey)
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("cannot build the deleted content keys: %w", err)
	}
	_, err = closeGoneImports(ctx, q, c, `e.subject_key in (select value from json_each(?))`, time.Now(),
		string(encoded))
	return err
}

// closeGoneImports は条件 where（別名 e）を満たし goneImportCondition に当たる記録を done/gone にする。
func closeGoneImports(
	ctx context.Context, q activityWriter, c *changes, where string, now time.Time, args ...any,
) (bool, error) {
	res, err := q.ExecContext(ctx, `update activity_entries as e set status = ?, reason = null, outcome = ?,
		updated_at = ? where `+where+` and `+goneImportCondition,
		append([]any{string(domain.ActivityDone), string(domain.ActivityGone), now.UnixMilli()}, args...)...)
	if err != nil {
		return false, fmt.Errorf("cannot close the gone import activity: %w", err)
	}
	closed, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cannot close the gone import activity: %w", err)
	}
	if closed > 0 {
		c.activityChanged()
	}
	return closed > 0, nil
}

// importFailure は import の記録で諦めた最初の仕事の段階と文である。
type importFailure struct {
	kind    domain.JobKind
	message string
}

// importActivityJobs は import の記録 id に属する仕事の段階と状態を id の順に返し、諦めた最初の仕事を添える。
func importActivityJobs(ctx context.Context, q queryExecer, id int64) ([]domain.ImportJobState, importFailure, error) {
	rows, err := q.QueryContext(ctx, `select kind, state, coalesce(last_error, '') from jobs where activity_id = ?
		order by id`, id)
	if err != nil {
		return nil, importFailure{}, fmt.Errorf("cannot read the import activity's jobs (id=%d): %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []domain.ImportJobState
	var failure importFailure
	for rows.Next() {
		var kind, state, message string
		if err := rows.Scan(&kind, &state, &message); err != nil {
			return nil, importFailure{}, fmt.Errorf("cannot read the import activity's jobs (id=%d): %w", id, err)
		}
		job := domain.ImportJobState{Kind: domain.JobKind(kind), State: domain.JobState(state)}
		if job.State == domain.JobFailed && failure.kind == "" {
			failure = importFailure{kind: job.Kind, message: message}
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, importFailure{}, fmt.Errorf("cannot read the import activity's jobs (id=%d): %w", id, err)
	}
	return jobs, failure, nil
}

// attachImportProgress は waiting を過ぎた running の import の記録に、今の段階と、その動画の済んだ段階の数を
// 添える。
func attachImportProgress(ctx context.Context, q rowsQueryer, entries []domain.ActivityEntry) error {
	index, encoded := activityEntryIDs(entries, func(entry domain.ActivityEntry) bool {
		return entry.Kind == domain.ActivityImport && entry.Status == domain.ActivityRunning &&
			entry.Reason == domain.ActivityProcessing
	})
	if len(index) == 0 {
		return nil
	}
	// 済んだ段階は動画のすべての仕事から数える。やり直しの記録に属するのは積み直した仕事だけで、前に済んだ
	// 段階の仕事は前の記録を指したまま残る。
	rows, err := q.QueryContext(ctx, `select a.activity_id, j.kind, j.state from jobs j join (
			select distinct activity_id, video_id from jobs where activity_id in (select value from json_each(?))
		) a on a.video_id = j.video_id order by j.id`, encoded)
	if err != nil {
		return fmt.Errorf("cannot read the import progress: %w", err)
	}
	defer func() { _ = rows.Close() }()
	jobs := map[int64][]domain.ImportJobState{}
	for rows.Next() {
		var id int64
		var kind, state string
		if err := rows.Scan(&id, &kind, &state); err != nil {
			return fmt.Errorf("cannot read the import progress: %w", err)
		}
		jobs[id] = append(jobs[id], domain.ImportJobState{Kind: domain.JobKind(kind), State: domain.JobState(state)})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the import progress: %w", err)
	}
	for id, i := range index {
		entries[i].Progress = domain.ImportActivityProgress(jobs[id])
	}
	return nil
}

// recordScanActivity は走査 id の終わりを記録する。interrupted 以外の理由で failed に終わった走査は、対象を
// その走査、表示名を始めた主体にした attention/failed の scan の記録を書く。done に終わった走査は、開いている
// scan の記録をすべて done/superseded で閉じる。起動時に interrupted で閉じた走査は書かない（research.md R-3）。
func recordScanActivity(
	ctx context.Context, tx activityWriter, c *changes, id int64, origin domain.ScanOrigin, state domain.ScanState,
	cause error, decider domain.ActivityDecider, now time.Time,
) error {
	if state == domain.ScanDone {
		return closeScanActivity(ctx, tx, c, now)
	}
	var detail string
	if cause != nil {
		if code, _ := domain.ScanFailureOf(cause); code == domain.ScanErrorInterrupted {
			return nil
		}
		detail = cause.Error()
	}
	_, err := recordActivity(ctx, tx, c, domain.ActivityEntry{
		Kind: domain.ActivityScan,
		Subject: domain.ActivitySubject{
			Kind: domain.ActivitySubjectScan, Key: strconv.FormatInt(id, 10), Label: string(origin),
		},
		Status: domain.ActivityAttention, Reason: domain.ActivityFailed, Detail: detail, Decider: decider,
	}, now)
	return err
}

// closeScanActivity は開いている scan の記録をすべて done/superseded で閉じる。
func closeScanActivity(ctx context.Context, tx activityWriter, c *changes, now time.Time) error {
	res, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = null, outcome = ?, updated_at = ?
		where kind = ? and status <> ?`, string(domain.ActivityDone), string(domain.ActivitySuperseded),
		now.UnixMilli(), string(domain.ActivityScan), string(domain.ActivityDone))
	if err != nil {
		return fmt.Errorf("cannot close the open scan activity: %w", err)
	}
	closed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot close the open scan activity: %w", err)
	}
	if closed > 0 {
		c.activityChanged()
	}
	return nil
}
