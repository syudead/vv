package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"modernc.org/sqlite"

	"github.com/syudead/vv/internal/domain"
)

// アクティビティの記録（specs/055-activity/data-model.md）。記録は変える内容と同じ取引で、記録する各役割が
// recordActivity・setActivityState を呼んで書き、確定後に domain.ActivityChanged を発行する（research.md
// R-2）。ActivityStore は記録の一覧・1 件の読み出しと、起動時の回復だけを持つ（plan.md の Structure
// decision）。

// foldCaseFunction は表示名の検索で大文字と小文字をそろえる関数を SQLite から呼ぶ名前である。SQLite の
// lower は ASCII だけを変えるので、Go の strings.ToLower を使う。select の中でしか使わず、スキーマには
// 残らない（vv_natural_sort_key と同じ）。
const foldCaseFunction = "vv_fold_case"

func init() {
	err := sqlite.RegisterDeterministicScalarFunction(foldCaseFunction, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch value := args[0].(type) {
			case nil:
				return "", nil
			case string:
				return strings.ToLower(value), nil
			case []byte:
				return strings.ToLower(string(value)), nil
			default:
				return nil, fmt.Errorf("%s: the label is not text: %T", foldCaseFunction, args[0])
			}
		})
	if err != nil {
		panic(fmt.Sprintf("cannot register %s: %v", foldCaseFunction, err))
	}
}

// activityWriter は記録を書く取引である。*sql.Tx と、begin immediate で取引を始めた *sql.Conn
// （仕事の専有）が満たす。
type activityWriter interface {
	queryExecer
	rowQueryer
}

// nullID は 0 の id を列の null にする。
func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// recordActivity は記録 entry を書き、その id を返す。entry.ID・CreatedAt・UpdatedAt は見ず、now を使う。
// Feature は種類の機能で、undo では取り消した記録（UndoesID）の機能を同じ取引で読んで写す。同じ取引で、同じ
// 種類と対象の古い attention の記録を done/superseded で閉じる（domain.ActivityEntry.Supersedes）。c に
// 記録を書いたことを残し、確定後に domain.ActivityChanged を発行させる。
func recordActivity(ctx context.Context, tx activityWriter, c *changes, entry domain.ActivityEntry, now time.Time) (int64, error) {
	feature, err := activityFeatureOf(ctx, tx, entry)
	if err != nil {
		return 0, err
	}
	entry.Feature = feature
	at := now.UnixMilli()
	result, err := tx.ExecContext(ctx, `insert into activity_entries (kind, feature, subject_kind, subject_key,
		subject_key_b, subject_label, subject_label_b, status, reason, outcome, detail, stage, undoes_id, decider,
		probability, undo_keys, undone_by, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(entry.Kind), string(entry.Feature), string(entry.Subject.Kind), entry.Subject.Key,
		nullString(entry.Subject.KeyB), entry.Subject.Label, nullString(entry.Subject.LabelB),
		string(entry.Status), nullString(string(entry.Reason)), nullString(string(entry.Outcome)),
		nullString(entry.Detail), nullString(string(entry.Stage)), nullID(entry.UndoesID), string(entry.Decider),
		entry.Probability, nullString(entry.UndoKeys), nullID(entry.UndoneBy), at, at)
	if err != nil {
		return 0, fmt.Errorf("cannot record the activity: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("cannot record the activity: %w", err)
	}
	entry.ID = id
	if err := supersedeActivity(ctx, tx, entry, at); err != nil {
		return 0, err
	}
	c.activityChanged()
	return id, nil
}

// activityFeatureOf は記録 entry の機能を返す。undo は取り消した記録の機能である。
func activityFeatureOf(ctx context.Context, tx activityWriter, entry domain.ActivityEntry) (domain.ActivityFeature, error) {
	if feature, ok := entry.Kind.Feature(); ok {
		return feature, nil
	}
	if entry.Kind != domain.ActivityUndo || entry.UndoesID == 0 {
		return "", fmt.Errorf("cannot record the activity: unknown kind %q", entry.Kind)
	}
	var feature string
	err := tx.QueryRowContext(ctx, `select feature from activity_entries where id = ?`, entry.UndoesID).Scan(&feature)
	if err != nil {
		return "", fmt.Errorf("cannot read the undone activity entry (id=%d): %w", entry.UndoesID, err)
	}
	return domain.ActivityFeature(feature), nil
}

// supersedeActivity は新しい記録 entry が閉じる古い記録を done/superseded にする。閉じるかの判断は
// domain.ActivityEntry.Supersedes が持ち、ここは候補を種類と対象の索引で引いてそれに渡す。
func supersedeActivity(ctx context.Context, tx activityWriter, entry domain.ActivityEntry, at int64) error {
	rows, err := tx.QueryContext(ctx, `select id, status from activity_entries
		where kind = ? and subject_key = ? and subject_key_b is ? and subject_kind = ? and id < ? and status = ?`,
		string(entry.Kind), entry.Subject.Key, nullString(entry.Subject.KeyB), string(entry.Subject.Kind), entry.ID,
		string(domain.ActivityAttention))
	if err != nil {
		return fmt.Errorf("cannot read the superseded activity: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var closing []int64
	for rows.Next() {
		older := domain.ActivityEntry{Kind: entry.Kind, Subject: entry.Subject}
		var status string
		if err := rows.Scan(&older.ID, &status); err != nil {
			return fmt.Errorf("cannot read the superseded activity: %w", err)
		}
		older.Status = domain.ActivityStatus(status)
		if entry.Supersedes(older) {
			closing = append(closing, older.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the superseded activity: %w", err)
	}
	_ = rows.Close()
	for _, id := range closing {
		if err := writeActivityState(ctx, tx, id,
			domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySuperseded}, at); err != nil {
			return err
		}
	}
	return nil
}

// setActivityState は記録 id の状態と理由か結果を state にする。記録を変えたことを c に残す。
func setActivityState(ctx context.Context, tx activityWriter, c *changes, id int64, state domain.ActivityState, now time.Time) error {
	if err := writeActivityState(ctx, tx, id, state, now.UnixMilli()); err != nil {
		return err
	}
	c.activityChanged()
	return nil
}

func writeActivityState(ctx context.Context, tx activityWriter, id int64, state domain.ActivityState, at int64) error {
	_, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = ?, outcome = ?, updated_at = ?
		where id = ?`, string(state.Status), nullString(string(state.Reason)), nullString(string(state.Outcome)), at, id)
	if err != nil {
		return fmt.Errorf("cannot update the activity entry (id=%d): %w", id, err)
	}
	return nil
}

// RecoverInterruptedActivity は、前回の停止で processing の running のまま残った記録をすべて
// attention/interrupted にし、件数を返す。waiting の記録は取り出される前なので変えない（research.md R-4）。
// 起動時に、ワーカーを動かす前に 1 度だけ呼ぶ。変えた記録があれば確定後に domain.ActivityChanged を発行する。
func (s *ActivityStore) RecoverInterruptedActivity(ctx context.Context) (int, error) {
	result, err := s.db.sql.ExecContext(ctx, `update activity_entries set status = ?, reason = ?, updated_at = ?
		where status = ? and reason = ?`,
		string(domain.ActivityAttention), string(domain.ActivityInterrupted), time.Now().UnixMilli(),
		string(domain.ActivityRunning), string(domain.ActivityProcessing))
	if err != nil {
		return 0, fmt.Errorf("cannot recover the interrupted activity: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cannot recover the interrupted activity: %w", err)
	}
	if affected > 0 {
		s.db.publishEvents(domain.ActivityChanged{})
	}
	return int(affected), nil
}

// activityColumns は一覧と 1 件の読み出しが読む列である。e は記録、c は組の候補、u は取り消した記録。
// 組が候補の画面にまだあるかは、候補の行があるだけでなく、重複の候補の一覧と同じ条件
// （candidatePairsCondition: 両方に登録の所在があり、却下も同じ集まりでもない）で決める。
var activityColumns = `e.id, e.kind, e.feature, e.subject_kind, e.subject_key, coalesce(e.subject_key_b, ''),
	e.subject_label, coalesce(e.subject_label_b, ''), e.status, coalesce(e.reason, ''), coalesce(e.outcome, ''),
	coalesce(e.detail, ''), coalesce(e.stage, ''), coalesce(e.undoes_id, 0), e.decider, e.probability,
	coalesce(e.undo_keys, ''), coalesce(e.undone_by, 0), e.created_at, e.updated_at,
	c.key_a is not null and exists (select 1 ` + candidatePairsFrom + `c.key_a = e.subject_key
		and c.key_b = e.subject_key_b and ` + candidatePairsCondition() + `),
	coalesce(c.judgement, ''), c.retry_at, u.kind, u.outcome,
	(select t.tag_name from activity_entry_tags t where t.entry_id = e.undoes_id and t.undone_by = e.id limit 1)`

// activityFrom は一覧と 1 件の読み出しの from 句である。組の候補は判定と再試行の時刻を運ぶ。
const activityFrom = ` from activity_entries e
	left join video_version_candidates c on e.subject_kind = 'pair' and c.key_a = e.subject_key
		and c.key_b = e.subject_key_b
	left join activity_entries u on u.id = e.undoes_id`

// scanActivityEntry は activityColumns の 1 行を読む。
func scanActivityEntry(row rowScanner) (domain.ActivityEntry, error) {
	var (
		entry                                    domain.ActivityEntry
		kind, feature, subjectKind, status       string
		reason, outcome, stage, decider          string
		judgement                                string
		probability                              sql.NullFloat64
		createdAt, updatedAt                     int64
		retryAt                                  sql.NullInt64
		undoneKind, undoneOutcome, undoneTagName sql.NullString
	)
	err := row.Scan(&entry.ID, &kind, &feature, &subjectKind, &entry.Subject.Key, &entry.Subject.KeyB,
		&entry.Subject.Label, &entry.Subject.LabelB, &status, &reason, &outcome, &entry.Detail, &stage,
		&entry.UndoesID, &decider, &probability, &entry.UndoKeys, &entry.UndoneBy, &createdAt, &updatedAt,
		&entry.Candidate, &judgement, &retryAt, &undoneKind, &undoneOutcome, &undoneTagName)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	entry.Kind = domain.ActivityKind(kind)
	entry.Feature = domain.ActivityFeature(feature)
	entry.Subject.Kind = domain.ActivitySubjectKind(subjectKind)
	entry.Status = domain.ActivityStatus(status)
	entry.Reason = domain.ActivityReason(reason)
	entry.Outcome = domain.ActivityOutcome(outcome)
	entry.Stage = domain.JobKind(stage)
	entry.Decider = domain.ActivityDecider(decider)
	if probability.Valid {
		value := probability.Float64
		entry.Probability = &value
	}
	entry.CreatedAt = time.UnixMilli(createdAt)
	entry.UpdatedAt = time.UnixMilli(updatedAt)
	if judgement == string(domain.VersionJudgementFailed) && retryAt.Valid && entry.Kind == domain.ActivityVersionJudgement {
		entry.RetryAt = time.Unix(retryAt.Int64, 0)
	}
	if undoneKind.Valid {
		entry.Undoes = &domain.ActivityUndone{
			Kind: domain.ActivityKind(undoneKind.String), Outcome: domain.ActivityOutcome(undoneOutcome.String),
			TagName: undoneTagName.String,
		}
	}
	return entry, nil
}

// ActivityEntry は記録 id を、一覧の 1 件と同じ形で返す。見る人は所有者である。無ければ domain.ErrNotFound。
func (s *ActivityStore) ActivityEntry(ctx context.Context, id int64) (domain.ActivityEntry, error) {
	entry, err := scanActivityEntry(s.db.sql.QueryRowContext(ctx,
		`select `+activityColumns+activityFrom+` where e.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ActivityEntry{}, fmt.Errorf("activity entry %d: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot read the activity entry (id=%d): %w", id, err)
	}
	entries := []domain.ActivityEntry{entry}
	if err := attachActivityDetails(ctx, s.db.sql, domain.AudienceOwner, entries); err != nil {
		return domain.ActivityEntry{}, err
	}
	return entries[0], nil
}

// ListActivity は query を満たす記録を id の降順に、cursor の次から limit 件返す（research.md R-9）。
// cursor が空なら先頭から、読めなければ domain.ErrInvalidCursor。Counts は Status 以外の、Features は
// Features 以外の絞り込みに従う。ページと件数は同じスナップショットから読む（読み取り用の接続）。
// 各件の VideoIDs は audience が開ける今の動画である。
func (s *ActivityStore) ListActivity(
	ctx context.Context, audience domain.Audience, query domain.ActivityQuery, cursor string, limit int,
) (domain.ActivityPage, error) {
	limit = max(1, limit)
	var before int64
	if cursor != "" {
		id, err := domain.DecodeActivityCursor(cursor)
		if err != nil {
			return domain.ActivityPage{}, err
		}
		before = id
	}
	tx, err := s.db.read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ActivityPage{}, fmt.Errorf("cannot read the activity: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	page := domain.ActivityPage{}
	if page.Items, page.NextCursor, err = activityItems(ctx, tx, query, before, limit); err != nil {
		return domain.ActivityPage{}, err
	}
	if err := attachActivityDetails(ctx, tx, audience, page.Items); err != nil {
		return domain.ActivityPage{}, err
	}
	if page.Counts, err = activityCounts(ctx, tx, query); err != nil {
		return domain.ActivityPage{}, err
	}
	if page.Features, err = activityFeatureCounts(ctx, tx, query); err != nil {
		return domain.ActivityPage{}, err
	}
	return page, nil
}

// activityFilter は絞り込みの条件句と引数である。withStatus・withFeatures が偽ならその絞り込みを外す
// （件数の読み出し）。
func activityFilter(query domain.ActivityQuery, withStatus, withFeatures bool) ([]string, []any) {
	var conditions []string
	var args []any
	if withStatus && query.Status != "" {
		conditions = append(conditions, `e.status = ?`)
		args = append(args, string(query.Status))
	}
	if withFeatures && len(query.Features) > 0 {
		encoded, _ := json.Marshal(query.Features) // 文字列の一覧の Marshal は失敗しない。
		conditions = append(conditions, `e.feature in (select value from json_each(?))`)
		args = append(args, string(encoded))
	}
	if query.Decider != "" {
		conditions = append(conditions, `e.decider = ?`)
		args = append(args, string(query.Decider))
	}
	if !query.Since.IsZero() {
		conditions = append(conditions, `e.created_at >= ?`)
		args = append(args, query.Since.UnixMilli())
	}
	if query.Search != "" {
		// 対象の表示名だけを見る。ほかの絞り込みが残す行を読むので、文字の索引は要らない（data-model.md）。
		needle := strings.ToLower(query.Search)
		conditions = append(conditions, `(instr(`+foldCaseFunction+`(e.subject_label), ?) > 0 or instr(`+
			foldCaseFunction+`(e.subject_label_b), ?) > 0)`)
		args = append(args, needle, needle)
	}
	return conditions, args
}

// activityItems は 1 ページの記録を読み、続きがあればカーソルを返す。before が 0 でなければその id より
// 小さい記録から読む。
func activityItems(
	ctx context.Context, tx *sql.Tx, query domain.ActivityQuery, before int64, limit int,
) ([]domain.ActivityEntry, string, error) {
	conditions, args := activityFilter(query, true, true)
	if before > 0 {
		conditions = append(conditions, `e.id < ?`)
		args = append(args, before)
	}
	// 1 件多く読んで、続きがあるかを知る。
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, `select `+activityColumns+activityFrom+whereClause(conditions)+
		` order by e.id desc limit ?`, args...)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the activity: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := []domain.ActivityEntry{}
	for rows.Next() {
		entry, err := scanActivityEntry(rows)
		if err != nil {
			return nil, "", fmt.Errorf("cannot read the activity: %w", err)
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("cannot read the activity: %w", err)
	}
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	return items, domain.EncodeActivityCursor(items[len(items)-1].ID), nil
}

// activityCounts は Status 以外の絞り込みを満たす記録を状態ごとに数える。
func activityCounts(ctx context.Context, tx *sql.Tx, query domain.ActivityQuery) (domain.ActivityCounts, error) {
	conditions, args := activityFilter(query, false, true)
	var counts domain.ActivityCounts
	err := groupedCounts(ctx, tx, `e.status`, conditions, args, func(status string, count int) {
		switch domain.ActivityStatus(status) {
		case domain.ActivityRunning:
			counts.Running = count
		case domain.ActivityAttention:
			counts.Attention = count
		case domain.ActivityDone:
			counts.Done = count
		}
	})
	return counts, err
}

// activityFeatureCounts は Features 以外の絞り込みを満たす記録を機能ごとに数える。どの機能も 0 から入る。
func activityFeatureCounts(ctx context.Context, tx *sql.Tx, query domain.ActivityQuery) (map[domain.ActivityFeature]int, error) {
	conditions, args := activityFilter(query, true, false)
	counts := make(map[domain.ActivityFeature]int, len(domain.ActivityFeatures))
	for _, feature := range domain.ActivityFeatures {
		counts[feature] = 0
	}
	err := groupedCounts(ctx, tx, `e.feature`, conditions, args, func(feature string, count int) {
		counts[domain.ActivityFeature(feature)] = count
	})
	return counts, err
}

// groupedCounts は条件を満たす記録を column ごとに数え、1 組ずつ add に渡す。
func groupedCounts(
	ctx context.Context, tx *sql.Tx, column string, conditions []string, args []any, add func(string, int),
) error {
	rows, err := tx.QueryContext(ctx, `select `+column+`, count(*) from activity_entries e`+whereClause(conditions)+
		` group by `+column, args...)
	if err != nil {
		return fmt.Errorf("cannot count the activity: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var value string
		var count int
		if err := rows.Scan(&value, &count); err != nil {
			return fmt.Errorf("cannot count the activity: %w", err)
		}
		add(value, count)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot count the activity: %w", err)
	}
	return nil
}

// attachActivityDetails は記録に、auto_tag の付けたタグ、running の import の進み具合と、対象の鍵ごとの
// audience が開ける今の動画を添える。
func attachActivityDetails(ctx context.Context, q rowsQueryer, audience domain.Audience, entries []domain.ActivityEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := attachActivityTags(ctx, q, entries); err != nil {
		return err
	}
	if err := attachImportProgress(ctx, q, entries); err != nil {
		return err
	}
	return attachActivityVideos(ctx, q, audience, entries)
}

// activityEntryIDs は keep を満たす記録の id から entries の添字を引く表と、それらの id の JSON の配列を返す。
func activityEntryIDs(entries []domain.ActivityEntry, keep func(domain.ActivityEntry) bool) (map[int64]int, string) {
	index := map[int64]int{}
	ids := []int64{}
	for i, entry := range entries {
		if keep(entry) {
			index[entry.ID] = i
			ids = append(ids, entry.ID)
		}
	}
	encoded, _ := json.Marshal(ids) // 整数の一覧の Marshal は失敗しない。
	return index, string(encoded)
}

// attachActivityTags は auto_tag の記録に、付けたタグを名前の順で添える。
func attachActivityTags(ctx context.Context, q rowsQueryer, entries []domain.ActivityEntry) error {
	index, encoded := activityEntryIDs(entries, func(entry domain.ActivityEntry) bool {
		return entry.Kind == domain.ActivityAutoTag
	})
	if len(index) == 0 {
		return nil
	}
	rows, err := q.QueryContext(ctx, `select entry_id, tag_id, tag_name, suggested, probability,
		coalesce(undone_by, 0) from activity_entry_tags where entry_id in (select value from json_each(?))
		order by entry_id, tag_name, tag_id`, encoded)
	if err != nil {
		return fmt.Errorf("cannot read the activity tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var entryID int64
		var tag domain.ActivityEntryTag
		if err := rows.Scan(&entryID, &tag.TagID, &tag.Name, &tag.Suggested, &tag.Probability, &tag.UndoneBy); err != nil {
			return fmt.Errorf("cannot read the activity tags: %w", err)
		}
		entry := &entries[index[entryID]]
		entry.Tags = append(entry.Tags, tag)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the activity tags: %w", err)
	}
	return nil
}

// attachActivityVideos は動画と組の記録の各鍵に、audience が開ける今の動画の id を添える。鍵の動画が無いか
// 開けなければ 0 のままにする（research.md R-10）。
func attachActivityVideos(ctx context.Context, q rowsQueryer, audience domain.Audience, entries []domain.ActivityEntry) error {
	var keys []string
	for _, entry := range entries {
		if entry.Subject.Kind != domain.ActivitySubjectVideo && entry.Subject.Kind != domain.ActivitySubjectPair {
			continue
		}
		keys = append(keys, entry.Subject.Key)
		if entry.Subject.KeyB != "" {
			keys = append(keys, entry.Subject.KeyB)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("cannot build content keys: %w", err)
	}
	rows, err := q.QueryContext(ctx, `select videos.content_key, videos.id from videos
		where videos.content_key in (select value from json_each(?)) and videos.content_key <> '' and `+
		visibleVideoCondition("videos", audience), string(encoded))
	if err != nil {
		return fmt.Errorf("cannot read the activity videos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := map[string]int64{}
	for rows.Next() {
		var key string
		var id int64
		if err := rows.Scan(&key, &id); err != nil {
			return fmt.Errorf("cannot read the activity videos: %w", err)
		}
		ids[key] = id
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("cannot read the activity videos: %w", err)
	}
	for i := range entries {
		subject := entries[i].Subject
		if subject.Kind != domain.ActivitySubjectVideo && subject.Kind != domain.ActivitySubjectPair {
			continue
		}
		entries[i].VideoIDs = [2]int64{ids[subject.Key], ids[subject.KeyB]}
	}
	return nil
}
