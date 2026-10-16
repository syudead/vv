package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// 重複の候補の判定の記録（specs/055-activity/data-model.md「Rules」、research.md R-4・R-5）。組ごとに 1 件の
// version_judgement の記録を保ち、候補の行の activity_id がそれを指す。候補が作り直されたら、組の開いている
// 記録（subject の鍵で引く）に付き直す。記録の状態は候補の判定に従い（domain.VersionJudgementActivityState）、
// 書き込みは候補を変える取引の中で行う。

// activityUpdate は処理の結果で変わる記録の欄である。decider が空なら rule。
type activityUpdate struct {
	state       domain.ActivityState
	decider     domain.ActivityDecider
	probability *float64
	detail      string
	undoKeys    string
}

// updateActivity は記録 id の状態・判断したもの・確率・失敗の文・取り消しの鍵を update にする。記録を
// 変えたことを c に残す。
func updateActivity(ctx context.Context, tx *sql.Tx, c *changes, id int64, update activityUpdate, now time.Time) error {
	decider := update.decider
	if decider == "" {
		decider = domain.ActivityByRule
	}
	if _, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = ?, outcome = ?, decider = ?,
		probability = ?, detail = ?, undo_keys = ?, updated_at = ? where id = ?`,
		string(update.state.Status), nullString(string(update.state.Reason)), nullString(string(update.state.Outcome)),
		string(decider), update.probability, nullString(update.detail), nullString(update.undoKeys), now.UnixMilli(),
		id); err != nil {
		return fmt.Errorf("cannot update the activity entry (id=%d): %w", id, err)
	}
	c.activityChanged()
	return nil
}

// waitingUpdate は待ちに戻した記録の欄である。
var waitingUpdate = activityUpdate{state: domain.VersionJudgementActivityState("")}

// contentLabel は内容 key のいまライブラリにある動画の、所有者から見た題名を返す。無ければ鍵そのもの。
func contentLabel(ctx context.Context, tx *sql.Tx, key string) (string, error) {
	id, err := videoIDForContent(ctx, tx, key)
	if errors.Is(err, domain.ErrNotFound) {
		return key, nil
	}
	if err != nil {
		return "", err
	}
	video, err := getVideo(ctx, tx, domain.AudienceOwner, id)
	if err != nil {
		return "", err
	}
	return video.Title, nil
}

// newPairActivity は組 keys（key_a < key_b）の version_judgement の記録を state で開き、その id を返す。
// 判断したものは rule で、表示名はいまの題名である。
func newPairActivity(
	ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, state domain.ActivityState, now time.Time,
) (int64, error) {
	subject := domain.ActivitySubject{Kind: domain.ActivitySubjectPair, Key: keys[0], KeyB: keys[1]}
	var err error
	if subject.Label, err = contentLabel(ctx, tx, keys[0]); err != nil {
		return 0, err
	}
	if subject.LabelB, err = contentLabel(ctx, tx, keys[1]); err != nil {
		return 0, err
	}
	return recordActivity(ctx, tx, c, domain.ActivityEntry{
		Kind: domain.ActivityVersionJudgement, Subject: subject, Status: state.Status, Reason: state.Reason,
		Decider: domain.ActivityByRule,
	}, now)
}

// openPairActivityID は組 keys の、いちばん新しい開いている（done でない）version_judgement の記録の id を返す。
// 無ければ 0。
func openPairActivityID(ctx context.Context, tx *sql.Tx, keys [2]string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `select id from activity_entries
		where kind = ? and subject_key = ? and subject_key_b = ? and subject_kind = ? and status <> ?
		order by id desc limit 1`, string(domain.ActivityVersionJudgement), keys[0], keys[1],
		string(domain.ActivitySubjectPair), string(domain.ActivityDone)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("cannot read the pair's activity entry: %w", err)
	}
	return id, nil
}

// attachPairActivity は組 keys の候補の行を記録 id に付ける。
func attachPairActivity(ctx context.Context, tx *sql.Tx, keys [2]string, id int64) error {
	if _, err := tx.ExecContext(ctx, `update video_version_candidates set activity_id = ? where key_a = ? and key_b = ?`,
		id, keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot attach the activity entry to the version candidate: %w", err)
	}
	return nil
}

// pairActivityFor は組 keys の候補が付いている開いた記録の id を返す。attached（候補の activity_id、無ければ 0）
// が開いていなければ、組の開いている記録に、それも無ければ state で開いた新しい記録に候補を付け直す。
func pairActivityFor(
	ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, attached int64, state domain.ActivityState, now time.Time,
) (int64, error) {
	if attached != 0 {
		var status string
		err := tx.QueryRowContext(ctx, `select status from activity_entries where id = ? and kind = ?`,
			attached, string(domain.ActivityVersionJudgement)).Scan(&status)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("cannot read the pair's activity entry: %w", err)
		}
		if err == nil && domain.ActivityStatus(status) != domain.ActivityDone {
			return attached, nil
		}
	}
	id, err := openPairActivityID(ctx, tx, keys)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		if id, err = newPairActivity(ctx, tx, c, keys, state, now); err != nil {
			return 0, err
		}
	}
	return id, attachPairActivity(ctx, tx, keys, id)
}

// keysJSON は鍵の一覧を json_each に渡す JSON にする。nil は null で、keysCondition はそれを全件とみなす。
func keysJSON(keys []string) (any, error) {
	if keys == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	return string(encoded), nil
}

// keysCondition は候補の組 c が鍵の一覧 ?1（null なら全件）のどれかを含む条件である。
const keysCondition = `(?1 is null or c.key_a in (select value from json_each(?1)) or c.key_b in (select value from json_each(?1)))`

// syncPairActivities は記録の付いていない候補の行（鍵 keys のどれかを含むもの、nil なら全件）を、組の開いて
// いる記録に付け直す（data-model.md「Rules」）。判定できる待ちの候補なら、付け直した記録を待ちに戻し、
// 開いている記録が無ければ、判定が入のときだけ新しい記録を待ちで開く（research.md R-4）。
func syncPairActivities(ctx context.Context, tx *sql.Tx, c *changes, keys []string, now time.Time) error {
	filter, err := keysJSON(keys)
	if err != nil {
		return err
	}
	pairs, err := queryKeyPairs(ctx, tx, `select c.key_a, c.key_b from video_version_candidates c
		where c.activity_id is null and `+keysCondition+` order by c.key_a, c.key_b`, filter)
	if err != nil || len(pairs) == 0 {
		return err
	}
	waiting, err := queryKeyPairs(ctx, tx, `select c.key_a, c.key_b `+judgeablePairs+`
		and c.activity_id is null and c.judgement is null and `+keysCondition, filter)
	if err != nil {
		return err
	}
	judgeable := make(map[[2]string]bool, len(waiting))
	for _, pair := range waiting {
		judgeable[pair] = true
	}
	enabled, err := versionJudgingEnabled(ctx, tx)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if err := syncPairActivity(ctx, tx, c, pair, judgeable[pair], enabled, now); err != nil {
			return err
		}
	}
	return nil
}

// syncPairActivity は記録の付いていない候補の組 pair を syncPairActivities の規則で記録に付ける。
func syncPairActivity(
	ctx context.Context, tx *sql.Tx, c *changes, pair [2]string, judgeable, enabled bool, now time.Time,
) error {
	id, err := openPairActivityID(ctx, tx, pair)
	if err != nil {
		return err
	}
	switch {
	case id != 0:
		if err := attachPairActivity(ctx, tx, pair, id); err != nil {
			return err
		}
		if judgeable {
			return updateActivity(ctx, tx, c, id, waitingUpdate, now)
		}
		return nil
	case judgeable && enabled:
		_, err := pairActivityFor(ctx, tx, c, pair, 0, waitingUpdate.state, now)
		return err
	default:
		return nil
	}
}

// queryKeyPairs は (key_a, key_b) の結果を読み切る。
func queryKeyPairs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([][2]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	pairs, err := appendKeyPairs(rows, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot read the version candidates: %w", err)
	}
	return pairs, nil
}

// closePairActivities は、両方の鍵が members にある組の開いている version_judgement の記録（except を除く）を
// done/outcome で閉じる。decider が空なら判断したものは変えない。束ねる操作が呼ぶ（data-model.md「Rules」）。
func closePairActivities(
	ctx context.Context, tx *sql.Tx, c *changes, members []string, outcome domain.ActivityOutcome,
	decider domain.ActivityDecider, except int64, now time.Time,
) error {
	encoded, err := keysJSON(members)
	if err != nil {
		return err
	}
	return closeActivities(ctx, tx, c, `subject_key in (select value from json_each(?1))
		and subject_key_b in (select value from json_each(?1)) and id <> ?2`, outcome, decider, now, encoded, except)
}

// leftLibraryCondition は内容の鍵 %[1]s がライブラリから外れた条件である。その鍵の動画の行が無く、同じパスの
// 中身の引き継ぎ（video_successions）の前の中身としても待っていない。引き継ぎを待つ鍵は、引き継げば新しい
// 鍵へ付け替わり（movePairActivityKey）、引き継がないと決まったときに外れる（data-model.md「Rules」）。
const leftLibraryCondition = `(not exists (select 1 from videos v where v.content_key = %[1]s)
	and not exists (select 1 from video_successions s where s.old_key = %[1]s))`

// closeGonePairActivities は、どちらかの鍵がライブラリから外れた組（leftLibraryCondition）の開いている
// version_judgement の記録を done/gone で閉じる。鍵の動画の行を消した取引（closeDeletedPairActivities）と、
// 引き継ぎを待っていた前の中身を引き継がないと決めた取引（applySuccession）が呼ぶ。
func closeGonePairActivities(ctx context.Context, tx *sql.Tx, c *changes, now time.Time) error {
	return closeActivities(ctx, tx, c, `subject_kind = ?1 and (`+fmt.Sprintf(leftLibraryCondition, "subject_key")+`
		or `+fmt.Sprintf(leftLibraryCondition, "subject_key_b")+`)`, domain.ActivityGone, "", now,
		string(domain.ActivitySubjectPair))
}

// closeDeletedPairActivities は、取引 c が動画の行を消した（videosDeleted）とき、鍵がライブラリから外れた組の
// 開いている version_judgement の記録を done/gone で閉じる。動画の行を消すと候補も消え（releaseContentIndex）、
// その動画を後継に待っていた前の中身の引き継ぎも消える。同じパスの中身が変わって消えた前の中身は、引き継ぎを
// 待つあいだ閉じない。確定の前に commit が呼ぶ。
func closeDeletedPairActivities(ctx context.Context, tx *sql.Tx, c *changes) error {
	if c == nil || len(c.deleted) == 0 {
		return nil
	}
	return closeGonePairActivities(ctx, tx, c, time.Now())
}

// closeActivities は condition（引数は ?1 から）を満たす開いている version_judgement の記録を done/outcome で
// 閉じる。閉じた記録があれば c に残す。
func closeActivities(
	ctx context.Context, tx *sql.Tx, c *changes, condition string, outcome domain.ActivityOutcome,
	decider domain.ActivityDecider, now time.Time, args ...any,
) error {
	base := len(args)
	args = append(args, string(outcome), string(decider), now.UnixMilli(), string(domain.ActivityVersionJudgement),
		string(domain.ActivityDone))
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`update activity_entries
		set status = ?%[5]d, reason = null, outcome = ?%[1]d, decider = coalesce(nullif(?%[2]d, ''), decider),
		    updated_at = ?%[3]d
		where kind = ?%[4]d and status <> ?%[5]d and `, base+1, base+2, base+3, base+4, base+5)+condition, args...)
	if err != nil {
		return fmt.Errorf("cannot close the pair activity entries: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("cannot close the pair activity entries: %w", err)
	} else if affected > 0 {
		c.activityChanged()
	}
	return nil
}

// returnPairActivitiesToWaiting は記録 ids のうち状態が from のものを待ちに戻す。候補の判定を待ちに戻す操作
// （判定中の行の回復、接続の保存）が、その行の記録に呼ぶ。
func returnPairActivitiesToWaiting(
	ctx context.Context, tx *sql.Tx, c *changes, ids []int64, from domain.ActivityState, now time.Time,
) error {
	if len(ids) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(ids) // 整数の一覧の Marshal は失敗しない。
	state := waitingUpdate.state
	result, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = ?, outcome = null,
		decider = ?, probability = null, detail = null, updated_at = ?
		where id in (select value from json_each(?)) and status = ? and reason = ?`,
		string(state.Status), string(state.Reason), string(domain.ActivityByRule), now.UnixMilli(), string(encoded),
		string(from.Status), string(from.Reason))
	if err != nil {
		return fmt.Errorf("cannot return the pair activity entries to waiting: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("cannot return the pair activity entries to waiting: %w", err)
	} else if affected > 0 {
		c.activityChanged()
	}
	return nil
}

// movePairActivityKey は、同じパスの中身が from から to に変わったとき、開いている version_judgement の記録の
// 対象の鍵 from を to に付け替える（data-model.md「Rules」、moveKeyPairs と同じ規則）。組は並べ直し、表示名は
// 鍵と一緒に動かす。to 自身との組になる記録は変えない。付け替えたかを返す。
func movePairActivityKey(ctx context.Context, tx *sql.Tx, from, to string, now time.Time) (bool, error) {
	subjects, err := openPairSubjects(ctx, tx, from)
	if err != nil {
		return false, err
	}
	moved := false
	for _, entry := range subjects {
		subject, ok := rekeyedPairSubject(entry.Subject, from, to)
		if !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `update activity_entries set subject_key = ?, subject_key_b = ?,
			subject_label = ?, subject_label_b = ?, updated_at = ? where id = ?`, subject.Key, subject.KeyB,
			subject.Label, subject.LabelB, now.UnixMilli(), entry.ID); err != nil {
			return false, fmt.Errorf("cannot move the pair activity entry: %w", err)
		}
		moved = true
	}
	return moved, nil
}

// openPairSubjects は鍵 key を含む組の開いている version_judgement の記録の id と対象を読み切る。
func openPairSubjects(ctx context.Context, tx *sql.Tx, key string) ([]domain.ActivityEntry, error) {
	rows, err := tx.QueryContext(ctx, `select id, subject_key, subject_key_b, subject_label,
		coalesce(subject_label_b, '') from activity_entries
		where kind = ? and subject_kind = ? and status <> ? and (subject_key = ?4 or subject_key_b = ?4)`,
		string(domain.ActivityVersionJudgement), string(domain.ActivitySubjectPair), string(domain.ActivityDone), key)
	if err != nil {
		return nil, fmt.Errorf("cannot read the pair activity entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var entries []domain.ActivityEntry
	for rows.Next() {
		var entry domain.ActivityEntry
		if err := rows.Scan(&entry.ID, &entry.Subject.Key, &entry.Subject.KeyB, &entry.Subject.Label,
			&entry.Subject.LabelB); err != nil {
			return nil, fmt.Errorf("cannot read the pair activity entries: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the pair activity entries: %w", err)
	}
	return entries, rows.Close()
}

// rekeyedPairSubject は組の対象 subject の鍵 from を to にし、鍵の順に並べ直したものを返す。to 自身との組に
// なるなら ok は偽。
func rekeyedPairSubject(subject domain.ActivitySubject, from, to string) (domain.ActivitySubject, bool) {
	keys := [2]string{subject.Key, subject.KeyB}
	labels := [2]string{subject.Label, subject.LabelB}
	for i := range keys {
		if keys[i] == from {
			keys[i] = to
		}
	}
	if keys[0] == keys[1] {
		return domain.ActivitySubject{}, false
	}
	if keys[0] > keys[1] {
		keys[0], keys[1] = keys[1], keys[0]
		labels[0], labels[1] = labels[1], labels[0]
	}
	subject.Key, subject.KeyB, subject.Label, subject.LabelB = keys[0], keys[1], labels[0], labels[1]
	return subject, true
}

// recordJudgementActivity は判定の結果 result を組 keys の記録 id に書く（research.md R-5）。束ねたなら
// done/same、却下したなら done/different で、判断したものと確率を残し、束ねたときは加わった側の鍵を残して、
// 同じまとまりになった他の組の開いている記録を superseded で閉じる。それ以外は候補のいまの判定に従い、
// モデルが決めなかった組は rule、失敗はその文を残す。
func recordJudgementActivity(
	ctx context.Context, tx *sql.Tx, c *changes, id int64, keys [2]string, result domain.VersionJudgeResult,
	applied appliedVerdict, now time.Time,
) error {
	switch {
	case applied.bundled:
		if err := closePairActivities(ctx, tx, c, applied.members, domain.ActivitySuperseded, "", id, now); err != nil {
			return err
		}
		// 取り消しは、まとまりがこの時刻より後に作られていないことで束ねがまだあると見る（undoJudgedBundle）ので、
		// 束ねた後の時刻を書く。
		return updateActivity(ctx, tx, c, id, activityUpdate{
			state:   domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySame},
			decider: result.ResultDecider(), probability: result.Probability, undoKeys: applied.undoKeys.Encode(),
		}, time.Now())
	case result.Verdict == domain.VersionDifferent:
		return updateActivity(ctx, tx, c, id, activityUpdate{
			state:   domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityDifferent},
			decider: result.ResultDecider(), probability: result.Probability,
		}, now)
	}
	var candidate candidateVerdict
	err := tx.QueryRowContext(ctx, `select judgement, probability, judgement_error from video_version_candidates
		where key_a = ? and key_b = ?`, keys[0], keys[1]).
		Scan(&candidate.judgement, &candidate.probability, &candidate.err)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("cannot read the version candidate: %w", err)
	}
	return updateActivity(ctx, tx, c, id, candidate.activityUpdate(), now)
}

// candidateVerdict は候補の行の判定の欄である。
type candidateVerdict struct {
	judgement   sql.NullString
	probability sql.NullFloat64
	err         sql.NullString
}

// activityUpdate は候補の判定に従う記録の欄を返す。モデルの確率を残した undecided は ai とその確率（規則の衝突で
// 決まらなかった組は確率を残さないので rule）、failed はその文である（research.md R-5）。
func (j candidateVerdict) activityUpdate() activityUpdate {
	update := activityUpdate{state: domain.VersionJudgementActivityState(domain.VersionJudgement(j.judgement.String))}
	switch update.state.Reason {
	case domain.ActivityUndecided:
		if j.probability.Valid {
			probability := j.probability.Float64
			update.decider, update.probability = domain.ActivityByAI, &probability
		}
	case domain.ActivityFailed:
		update.detail = j.err.String
	}
	return update
}

// reopenRestoredPairActivities は、ドラッグの束ねの取り消し（OrganizeStore.Undo）が戻した候補の行のうち、
// 束ねで所有者が決めたとして閉じた（decided_by_owner）記録を指すものの記録を、その候補の判定に従う状態に開き
// 直す。束ねる前の記録に戻すので、組は 1 つの記録を保つ（data-model.md「Rules」）。
func reopenRestoredPairActivities(ctx context.Context, tx *sql.Tx, c *changes, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `select c.activity_id, c.judgement, c.probability, c.judgement_error
		from video_version_candidates c join activity_entries e on e.id = c.activity_id
		where e.kind = ? and e.status = ? and e.outcome = ?`, string(domain.ActivityVersionJudgement),
		string(domain.ActivityDone), string(domain.ActivityDecidedByOwner))
	if err != nil {
		return fmt.Errorf("cannot read the restored version candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	updates := map[int64]activityUpdate{}
	for rows.Next() {
		var id int64
		var candidate candidateVerdict
		if err := rows.Scan(&id, &candidate.judgement, &candidate.probability, &candidate.err); err != nil {
			return fmt.Errorf("cannot read the restored version candidates: %w", err)
		}
		updates[id] = candidate.activityUpdate()
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the restored version candidates: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("cannot read the restored version candidates: %w", err)
	}
	for id, update := range updates {
		if err := updateActivity(ctx, tx, c, id, update, now); err != nil {
			return err
		}
	}
	return nil
}

// RetryJudgement は失敗か中断で attention の version_judgement の記録 id の組を、すぐ取り出せるようにし
// （失敗なら retry_at を今にする）、記録を待ちに戻す（contracts/screen-api.md「POST /api/activity/{id}/retry」）。
// 判定のワーカーを起こすのは呼び出し側である。記録が無ければ domain.ErrNotFound、やり直しを出さないか、組が
// もう判定できる候補でなければ domain.ErrActivityNotRetryable。
func (s *VersionStore) RetryJudgement(ctx context.Context, id int64) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot retry the version judgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entry := domain.ActivityEntry{ID: id, Kind: domain.ActivityVersionJudgement}
	var status, reason string
	err = tx.QueryRowContext(ctx, `select subject_key, coalesce(subject_key_b, ''), status, coalesce(reason, '')
		from activity_entries where id = ? and kind = ?`, id, string(entry.Kind)).
		Scan(&entry.Subject.Key, &entry.Subject.KeyB, &status, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("activity entry %d: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("cannot read the activity entry (id=%d): %w", id, err)
	}
	entry.Status, entry.Reason = domain.ActivityStatus(status), domain.ActivityReason(reason)
	if !entry.RetryOffered() {
		return domain.ErrActivityNotRetryable
	}
	keys := [2]string{entry.Subject.Key, entry.Subject.KeyB}
	var judgement sql.NullString
	err = tx.QueryRowContext(ctx, `select c.judgement `+judgeablePairs+` and c.key_a = ? and c.key_b = ?`,
		keys[0], keys[1]).Scan(&judgement)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrActivityNotRetryable
	}
	if err != nil {
		return fmt.Errorf("cannot read the version candidate: %w", err)
	}
	switch domain.VersionJudgement(judgement.String) {
	case "":
	case domain.VersionJudgementFailed:
		if _, err := tx.ExecContext(ctx, `update video_version_candidates set retry_at = ? where key_a = ? and key_b = ?`,
			time.Now().Unix(), keys[0], keys[1]); err != nil {
			return fmt.Errorf("cannot retry the version judgement: %w", err)
		}
	default:
		return domain.ErrActivityNotRetryable
	}
	c := &changes{}
	if err := attachPairActivity(ctx, tx, keys, id); err != nil {
		return err
	}
	if err := updateActivity(ctx, tx, c, id, waitingUpdate, time.Now()); err != nil {
		return err
	}
	return s.db.commit(ctx, tx, c)
}

// UndoVersionJudgement は rule か ai が決めた version_judgement の記録 id の判断を取り消し、所有者の undo の記録を
// 返す（research.md R-7）。1 つの取引で対象の状態を読み直し、「同じ」なら束ねがまだあるときだけ加わった側を外し、
// 「違う」なら却下がまだあるときだけ消して組を候補に戻し、判定の保留を書く。状態がもう違えば何も変えず、undo の
// 記録の結果は already_reverted。記録が無ければ domain.ErrNotFound、取り消しを出さなければ
// domain.ErrActivityNotUndoable。
func (s *ActivityStore) UndoVersionJudgement(ctx context.Context, id int64) (domain.ActivityEntry, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot undo the version judgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entry, err := scanActivityEntry(tx.QueryRowContext(ctx, `select `+activityColumns+activityFrom+` where e.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ActivityEntry{}, fmt.Errorf("activity entry %d: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot read the activity entry (id=%d): %w", id, err)
	}
	if entry.Kind != domain.ActivityVersionJudgement || !entry.UndoOffered() {
		return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
	}
	c := &changes{}
	keys := [2]string{entry.Subject.Key, entry.Subject.KeyB}
	var restored bool
	var events []domain.Event
	if entry.Outcome == domain.ActivitySame {
		restored, events, err = undoJudgedBundle(ctx, tx, c, keys, entry.UndoKeys, entry.UpdatedAt)
	} else {
		restored, err = undoJudgedDismissal(ctx, tx, c, keys)
		events = []domain.Event{domain.VersionCandidatesChanged{}}
	}
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	undoID, err := recordUndo(ctx, tx, c, entry, restored, time.Now())
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot undo the version judgement: %w", err)
	}
	s.db.publishEvents(events...)
	return s.ActivityEntry(ctx, undoID)
}

// recordUndo は記録 undone を取り消した所有者の undo の記録を書き、undone をその記録で取り消したことにする。
// restored が偽なら結果は already_reverted。
func recordUndo(
	ctx context.Context, tx *sql.Tx, c *changes, undone domain.ActivityEntry, restored bool, now time.Time,
) (int64, error) {
	outcome := domain.ActivityCompleted
	if !restored {
		outcome = domain.ActivityAlreadyReverted
	}
	subject := undone.Subject
	id, err := recordActivity(ctx, tx, c, domain.ActivityEntry{
		Kind: domain.ActivityUndo, Subject: subject, Status: domain.ActivityDone, Outcome: outcome,
		UndoesID: undone.ID, Decider: domain.ActivityByOwner,
	}, now)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `update activity_entries set undone_by = ?, updated_at = ? where id = ?`,
		id, now.UnixMilli(), undone.ID); err != nil {
		return 0, fmt.Errorf("cannot mark the activity entry undone (id=%d): %w", undone.ID, err)
	}
	return id, nil
}

// undoJudgedDismissal は自動の「違う」の組 keys の却下がまだあれば消し、両方の鍵の候補を作り直して組を候補の
// 一覧に戻し、判定の保留（video_version_judging_holds）を書いて、戻したかを返す。却下がもう無ければ何も
// 変えない。
func undoJudgedDismissal(ctx context.Context, tx *sql.Tx, c *changes, keys [2]string) (bool, error) {
	result, err := tx.ExecContext(ctx, `delete from video_version_dismissals where key_a = ? and key_b = ?`,
		keys[0], keys[1])
	if err != nil {
		return false, fmt.Errorf("cannot remove the dismissal: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return false, fmt.Errorf("cannot remove the dismissal: %w", err)
	} else if affected == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `insert or ignore into video_version_judging_holds (key_a, key_b, created_at)
		values (?, ?, ?)`, keys[0], keys[1], time.Now().Unix()); err != nil {
		return false, fmt.Errorf("cannot hold the version judgement: %w", err)
	}
	for _, key := range keys {
		if err := rebuildCandidatesOf(ctx, tx, c, key); err != nil {
			return false, err
		}
	}
	return true, nil
}

// undoJudgedBundle は自動の「同じ」の組 keys の束ねがまだあれば（2 つの鍵が同じまとまりにあれば）、加わった側
// （leavingMembers）を元の代表のまとまりとして外し（splitBundle）、外したかと確定後の知らせを返す。judgedAt は
// 判定で束ねた時刻（記録を done/same にした時刻）である。束ねがもう無いか、外すメンバーが無ければ何も変えない。
func undoJudgedBundle(
	ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, undoKeys string, judgedAt time.Time,
) (bool, []domain.Event, error) {
	joined, err := domain.DecodeVersionBundleUndoKeys(undoKeys)
	if err != nil {
		return false, nil, err
	}
	bundles := [2]bundleRef{}
	for i, key := range keys {
		bundle, bundled, err := bundleOf(ctx, tx, key)
		if err != nil || !bundled {
			return false, nil, err
		}
		bundles[i] = bundle
	}
	if bundles[0].id != bundles[1].id {
		return false, nil, nil
	}
	members, err := bundleMemberKeys(ctx, tx, bundles[0].id)
	if err != nil {
		return false, nil, err
	}
	leaving, err := leavingMembers(ctx, tx, joined.Members, bundles[0].id, judgedAt)
	if err != nil {
		return false, nil, err
	}
	if len(leaving) == 0 || len(leaving) == len(members) {
		return false, nil, nil
	}
	split, err := splitBundle(ctx, tx, c, bundles[0], leaving, joined.Representative)
	if err != nil {
		return false, nil, err
	}
	return true, split.events(), nil
}

// leavingMembers は加わった側の鍵 joined のうち、判定で束ねた時刻 judgedAt から合わさったまとまり bundleID に
// いるメンバーで、いまライブラリにあるものを返す。メンバーになった時刻（video_bundle_members.added_at）が
// judgedAt より後の鍵は、所有者が一度外して束ね直したので、合わさったまとまりにいたとはみなさない。まとまりを
// 束ねる操作は、すでにメンバーの鍵の時刻を変えない（moveBundleMembers）。
func leavingMembers(
	ctx context.Context, tx *sql.Tx, joined []string, bundleID int64, judgedAt time.Time,
) ([]string, error) {
	var leaving []string
	for _, key := range joined {
		var addedAt int64
		err := tx.QueryRowContext(ctx, `select added_at from video_bundle_members where content_key = ? and bundle_id = ?`,
			key, bundleID).Scan(&addedAt)
		if errors.Is(err, sql.ErrNoRows) || err == nil && addedAt > judgedAt.Unix() {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read the bundle member: %w", err)
		}
		if _, err := videoIDForContent(ctx, tx, key); errors.Is(err, domain.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		leaving = append(leaving, key)
	}
	return leaving, nil
}
