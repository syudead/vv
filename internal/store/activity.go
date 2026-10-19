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

// Write activity in the same transaction as its effects; publish ActivityChanged only after commit.

// SQLite lower() is ASCII-only; use Go's Unicode case folding for display-name searches.
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

// Implemented by *sql.Tx and the *sql.Conn holding a BEGIN IMMEDIATE job claim.
type activityWriter interface {
	queryExecer
	rowQueryer
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// recordActivity ignores entry timestamps and ID, supersedes matching attention entries, and schedules an event after commit.
func recordActivity(ctx context.Context, tx activityWriter, c *changes, entry domain.ActivityEntry, now time.Time) (int64, error) {
	feature, err := activityFeatureOf(ctx, tx, entry)
	if err != nil {
		return 0, err
	}
	entry.Feature = feature
	at := now.UnixMilli()
	result, err := tx.ExecContext(ctx, `insert into activity_entries (kind, feature, subject_kind, subject_key,
		subject_key_b, subject_label, subject_label_b, status, reason, outcome, detail, stage, undoes_id, decider,
		probability, undo_keys, undone_by, scan_id, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(entry.Kind), string(entry.Feature), string(entry.Subject.Kind), entry.Subject.Key,
		nullString(entry.Subject.KeyB), entry.Subject.Label, nullString(entry.Subject.LabelB),
		string(entry.Status), nullString(string(entry.Reason)), nullString(string(entry.Outcome)),
		nullString(entry.Detail), nullString(string(entry.Stage)), nullID(entry.UndoesID), string(entry.Decider),
		entry.Probability, nullString(entry.UndoKeys), nullID(entry.UndoneBy), nullID(entry.ScanID), at, at)
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

// recordContinuedActivity records an entry that continues earlier ones, such as a retry from the activity log or a
// claim that reopens an interrupted run. It joins every pipeline of the entries it continues instead of opening one,
// so the continuation stays inside the run it belongs to.
func recordContinuedActivity(
	ctx context.Context, tx activityWriter, c *changes, entry domain.ActivityEntry, continued []int64, now time.Time,
) (int64, error) {
	id, err := recordActivity(ctx, tx, c, entry, now)
	if err != nil {
		return 0, err
	}
	return id, joinPipelines(ctx, tx, id, continued, now)
}

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

// The domain decides which entries to supersede; SQL narrows candidates by kind and subject.
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
		if _, err := tx.ExecContext(ctx, `update activity_entries set superseded_by = ? where id = ?`,
			entry.ID, id); err != nil {
			return fmt.Errorf("cannot record the superseding activity (id=%d): %w", id, err)
		}
		if err := writeActivityState(ctx, tx, id,
			domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySuperseded}, at); err != nil {
			return err
		}
	}
	return nil
}

// Recheck retry eligibility inside the write transaction so concurrent retries cannot repeat an already closed entry.
func activityRetryOffered(ctx context.Context, tx activityWriter, id int64) (bool, error) {
	var entry domain.ActivityEntry
	var kind, status, reason string
	err := tx.QueryRowContext(ctx, `select kind, status, coalesce(reason, '') from activity_entries where id = ?`, id).
		Scan(&kind, &status, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot read the activity entry to retry (id=%d): %w", id, err)
	}
	entry.Kind, entry.Status, entry.Reason = domain.ActivityKind(kind), domain.ActivityStatus(status),
		domain.ActivityReason(reason)
	return entry.RetryOffered(), nil
}

func setActivityState(ctx context.Context, tx activityWriter, c *changes, id int64, state domain.ActivityState, now time.Time) error {
	if err := writeActivityState(ctx, tx, id, state, now.UnixMilli()); err != nil {
		return err
	}
	c.activityChanged()
	return nil
}

// writeActivityState updates a member's state and re-rolls its pipelines in the same transaction.
func writeActivityState(ctx context.Context, tx activityWriter, id int64, state domain.ActivityState, at int64) error {
	if err := storeActivityState(ctx, tx, id, state, at); err != nil {
		return err
	}
	return rollUpPipelines(ctx, tx, []int64{id}, at)
}

func storeActivityState(ctx context.Context, tx activityWriter, id int64, state domain.ActivityState, at int64) error {
	_, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = ?, outcome = ?, updated_at = ?
		where id = ?`, string(state.Status), nullString(string(state.Reason)), nullString(string(state.Outcome)), at, id)
	if err != nil {
		return fmt.Errorf("cannot update the activity entry (id=%d): %w", id, err)
	}
	return nil
}

// collectActivityIDs reads the ids an update's returning clause yields.
func collectActivityIDs(rows *sql.Rows, err error) ([]int64, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// RecoverInterruptedActivity marks processing entries interrupted before workers start; waiting entries remain
// untouched, and pipelines and stage rows follow their members.
func (s *ActivityStore) RecoverInterruptedActivity(ctx context.Context) (int, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot recover the interrupted activity: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	at := time.Now().UnixMilli()
	ids, err := collectActivityIDs(tx.QueryContext(ctx, `update activity_entries set status = ?, reason = ?,
		updated_at = ? where status = ? and reason = ? and kind <> ? returning id`,
		string(domain.ActivityAttention), string(domain.ActivityInterrupted), at,
		string(domain.ActivityRunning), string(domain.ActivityProcessing), string(domain.ActivityPipeline)))
	if err != nil {
		return 0, fmt.Errorf("cannot recover the interrupted activity: %w", err)
	}
	if err := resetInterruptedStages(ctx, tx, ids, at); err != nil {
		return 0, err
	}
	if err := rollUpPipelines(ctx, tx, ids, at); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cannot recover the interrupted activity: %w", err)
	}
	if len(ids) > 0 {
		s.db.publishEvents(domain.ActivityChanged{})
	}
	return len(ids), nil
}

// resetInterruptedStages returns an interrupted import's processing stage to waiting, as its job is queued again, so
// the detail shows it as not run instead of in progress.
func resetInterruptedStages(ctx context.Context, tx *sql.Tx, ids []int64, at int64) error {
	if len(ids) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(ids) // A slice of integers always encodes.
	if _, err := tx.ExecContext(ctx, `update activity_entry_stages set state = ?, updated_at = ?
		where entry_id in (select value from json_each(?)) and state = ?`,
		string(domain.ActivityStageWaiting), at, string(encoded), string(domain.ActivityStageProcessing)); err != nil {
		return fmt.Errorf("cannot reset the interrupted import stages: %w", err)
	}
	return nil
}

// A pair is a visible candidate only when it satisfies the same predicate as the candidate list.
var activityColumns = `e.id, e.kind, e.feature, e.subject_kind, e.subject_key, coalesce(e.subject_key_b, ''),
	e.subject_label, coalesce(e.subject_label_b, ''), e.status, coalesce(e.reason, ''), coalesce(e.outcome, ''),
	coalesce(e.detail, ''), coalesce(e.stage, ''), coalesce(e.undoes_id, 0), e.decider, e.probability,
	coalesce(e.undo_keys, ''), coalesce(e.undone_by, 0), coalesce(e.scan_id, 0), e.created_at, e.updated_at,
	c.key_a is not null and exists (select 1 ` + candidatePairsFrom + `c.key_a = e.subject_key
		and c.key_b = e.subject_key_b and ` + candidatePairsCondition() + `),
	coalesce(c.judgement, ''), c.retry_at, u.kind, u.outcome,
	(select t.tag_name from activity_entry_tags t where t.entry_id = e.undoes_id and t.undone_by = e.id limit 1)`

const activityFrom = ` from activity_entries e` + activityJoins

const activityJoins = `
	left join video_version_candidates c on e.subject_kind = 'pair' and c.key_a = e.subject_key
		and c.key_b = e.subject_key_b
	left join activity_entries u on u.id = e.undoes_id`

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
		&entry.UndoesID, &decider, &probability, &entry.UndoKeys, &entry.UndoneBy, &entry.ScanID, &createdAt,
		&updatedAt,
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

// ActivityEntry returns an owner-visible entry or domain.ErrNotFound.
func (s *ActivityStore) ActivityEntry(ctx context.Context, id int64) (domain.ActivityEntry, error) {
	return readActivityEntry(ctx, s.db.sql, id)
}

// ListActivity pages top-level entries by descending ID, each pipeline with its members, in a snapshot shared with its
// counts; counts omit their own facet filter.
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
	if err := attachPipelineMembers(ctx, tx, audience, page.Items, activityMemberMatch(query)); err != nil {
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

// activityFilter selects the top-level entries, pipelines and entries of no pipeline, that match the query. A pipeline
// matches the entry filters when it or any one member satisfies all of them; status applies to the top-level entry
// only. Disable status or feature filtering when counting that facet.
func activityFilter(query domain.ActivityQuery, withStatus, withFeatures bool) ([]string, []any) {
	conditions := []string{`(e.kind = ? or not exists (select 1 from activity_pipeline_members pm where pm.entry_id = e.id))`}
	args := []any{string(domain.ActivityPipeline)}
	if withStatus && query.Status != "" {
		conditions = append(conditions, `e.status = ?`)
		args = append(args, string(query.Status))
	}
	own, ownArgs := activityEntryFilter(query, "e", withFeatures)
	if len(own) == 0 {
		return conditions, args
	}
	member, memberArgs := activityEntryFilter(query, "m", withFeatures)
	conditions = append(conditions, `(`+strings.Join(own, ` and `)+` or (e.kind = ? and exists (select 1 `+
		pipelineMembersFrom+` where pm.pipeline_id = e.id and `+strings.Join(member, ` and `)+`)))`)
	args = append(append(append(args, ownArgs...), string(domain.ActivityPipeline)), memberArgs...)
	return conditions, args
}

// pipelineMembersFrom joins a pipeline's membership rows (pm) to the member entries (m).
const pipelineMembersFrom = `from activity_pipeline_members pm join activity_entries m on m.id = pm.entry_id`

// activityEntryFilter is the query's per-entry filters, except status, on the entry aliased alias.
func activityEntryFilter(query domain.ActivityQuery, alias string, withFeatures bool) ([]string, []any) {
	var conditions []string
	var args []any
	if withFeatures && len(query.Features) > 0 {
		encoded, _ := json.Marshal(query.Features)
		conditions = append(conditions, alias+`.feature in (select value from json_each(?))`)
		args = append(args, string(encoded))
	}
	if query.Decider != "" {
		conditions = append(conditions, alias+`.decider = ?`)
		args = append(args, string(query.Decider))
	}
	if !query.Since.IsZero() {
		conditions = append(conditions, alias+`.created_at >= ?`)
		args = append(args, query.Since.UnixMilli())
	}
	if query.Search != "" {
		// Search only subject labels; other filters bound the scan without a text index.
		needle := strings.ToLower(query.Search)
		conditions = append(conditions, `(instr(`+foldCaseFunction+`(`+alias+`.subject_label), ?) > 0 or instr(`+
			foldCaseFunction+`(`+alias+`.subject_label_b), ?) > 0)`)
		args = append(args, needle, needle)
	}
	return conditions, args
}

func activityItems(
	ctx context.Context, tx *sql.Tx, query domain.ActivityQuery, before int64, limit int,
) ([]domain.ActivityEntry, string, error) {
	conditions, args := activityFilter(query, true, true)
	if before > 0 {
		conditions = append(conditions, `e.id < ?`)
		args = append(args, before)
	}
	// Read one extra entry to detect another page.
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

func activityCounts(ctx context.Context, tx *sql.Tx, query domain.ActivityQuery) (domain.ActivityCounts, error) {
	conditions, args := activityFilter(query, false, true)
	var counts domain.ActivityCounts
	err := groupedCounts(ctx, tx, `select e.status, count(*) from activity_entries e`+whereClause(conditions)+
		` group by e.status`, args, func(status string, count int) {
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

// Feature counts include zero-valued features and ignore the feature filter. A top-level entry counts under its own
// feature when it matches, and a pipeline also under the feature of each matching member, once per feature: the count
// under a feature is the number of entries the list shows with only that feature selected.
func activityFeatureCounts(ctx context.Context, tx *sql.Tx, query domain.ActivityQuery) (map[domain.ActivityFeature]int, error) {
	top, topArgs := activityFilter(domain.ActivityQuery{Status: query.Status}, true, false)
	own, ownArgs := activityEntryFilter(query, "e", false)
	member, memberArgs := activityEntryFilter(query, "m", false)
	pipeline := []string{`e.kind = ?`}
	pipelineArgs := []any{string(domain.ActivityPipeline)}
	if query.Status != "" {
		pipeline = append(pipeline, `e.status = ?`)
		pipelineArgs = append(pipelineArgs, string(query.Status))
	}
	args := append(append(append(topArgs, ownArgs...), pipelineArgs...), memberArgs...)
	counts := make(map[domain.ActivityFeature]int, len(domain.ActivityFeatures))
	for _, feature := range domain.ActivityFeatures {
		counts[feature] = 0
	}
	err := groupedCounts(ctx, tx, `select feature, count(distinct id) from (
		select e.id, e.feature from activity_entries e`+whereClause(append(top, own...))+`
		union all
		select e.id, m.feature from activity_entries e join activity_pipeline_members pm on pm.pipeline_id = e.id
			join activity_entries m on m.id = pm.entry_id`+whereClause(append(pipeline, member...))+`
	) group by feature`, args, func(feature string, count int) {
		counts[domain.ActivityFeature(feature)] = count
	})
	return counts, err
}

func groupedCounts(ctx context.Context, tx *sql.Tx, query string, args []any, add func(string, int)) error {
	rows, err := tx.QueryContext(ctx, query, args...)
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

func attachActivityDetails(ctx context.Context, q rowsQueryer, audience domain.Audience, entries []domain.ActivityEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := attachActivityTags(ctx, q, entries); err != nil {
		return err
	}
	if err := attachActivityFaceLinks(ctx, q, entries); err != nil {
		return err
	}
	if err := attachImportProgress(ctx, q, entries); err != nil {
		return err
	}
	return attachActivityVideos(ctx, q, audience, entries)
}

func activityEntryIDs(entries []domain.ActivityEntry, keep func(domain.ActivityEntry) bool) (map[int64]int, string) {
	index := map[int64]int{}
	ids := []int64{}
	for i, entry := range entries {
		if keep(entry) {
			index[entry.ID] = i
			ids = append(ids, entry.ID)
		}
	}
	encoded, _ := json.Marshal(ids)
	return index, string(encoded)
}

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

// Missing or inaccessible videos retain ID zero.
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
