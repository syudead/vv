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

// Keep one activity per candidate pair, reattaching rebuilt candidates and updating it within the candidate transaction.

// An empty decider defaults to rule.
type activityUpdate struct {
	state       domain.ActivityState
	decider     domain.ActivityDecider
	probability *float64
	detail      string
	undoKeys    string
}

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

func decideActivity(ctx context.Context, tx *sql.Tx, c *changes, id int64, update activityUpdate, now time.Time) error {
	if err := updateActivity(ctx, tx, c, id, update, now); err != nil {
		return err
	}
	return markDecided(ctx, tx, id)
}

var waitingUpdate = activityUpdate{state: domain.VersionJudgementActivityState("")}

// Return the owner's current title, falling back to the content key.
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

// Pair keys must be sorted; new entries use the current titles and the rule decider.
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

// Return zero when the pair has no open activity.
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

func attachPairActivity(ctx context.Context, tx *sql.Tx, keys [2]string, id int64) error {
	if _, err := tx.ExecContext(ctx, `update video_version_candidates set activity_id = ? where key_a = ? and key_b = ?`,
		id, keys[0], keys[1]); err != nil {
		return fmt.Errorf("cannot attach the activity entry to the version candidate: %w", err)
	}
	return nil
}

// Reuse the attached open activity, then the pair's open activity, otherwise create one in the requested state.
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

// Nil keys encode as null, which keysCondition treats as all keys.
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

const keysCondition = `(?1 is null or c.key_a in (select value from json_each(?1)) or c.key_b in (select value from json_each(?1)))`

// Reattach orphan candidates to open pair activities; create missing activities only while judging is enabled.
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

// Close open activities within the bundled members, except the supplied entry; an empty decider preserves its value.
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

// A missing video remains in the library while succession is pending; its activity follows the replacement key if accepted.
const leftLibraryCondition = `(not exists (select 1 from videos v where v.content_key = %[1]s)
	and not exists (select 1 from video_successions s where s.old_key = %[1]s))`

// Close pair activities as gone when either key has left the library, including rejected successions.
func closeGonePairActivities(ctx context.Context, tx *sql.Tx, c *changes, now time.Time) error {
	return closeActivities(ctx, tx, c, `subject_kind = ?1 and (`+fmt.Sprintf(leftLibraryCondition, "subject_key")+`
		or `+fmt.Sprintf(leftLibraryCondition, "subject_key_b")+`)`, domain.ActivityGone, "", now,
		string(domain.ActivitySubjectPair))
}

// A rebuilt fingerprint can supersede a pair while both videos remain; missing keys are handled by closeGonePairActivities.
func closeDroppedPairActivities(ctx context.Context, tx *sql.Tx, c *changes, key string, now time.Time) error {
	return closeActivities(ctx, tx, c, `subject_kind = ?1 and (subject_key = ?2 or subject_key_b = ?2)
		and not exists (select 1 from video_version_candidates vc
		                 where vc.key_a = activity_entries.subject_key and vc.key_b = activity_entries.subject_key_b)
		and exists (select 1 from videos v where v.content_key = activity_entries.subject_key)
		and exists (select 1 from videos v where v.content_key = activity_entries.subject_key_b)`,
		domain.ActivitySuperseded, "", now, string(domain.ActivitySubjectPair), key)
}

// Run before commit after video deletion; keep activities for old content while succession is pending.
func closeDeletedPairActivities(ctx context.Context, tx *sql.Tx, c *changes) error {
	if c == nil || len(c.deleted) == 0 {
		return nil
	}
	return closeGonePairActivities(ctx, tx, c, time.Now())
}

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

// Return activities to waiting when recovery or connection changes reset their candidates.
func returnPairActivitiesToWaiting(
	ctx context.Context, tx *sql.Tx, c *changes, ids []int64, from domain.ActivityState, now time.Time,
) error {
	if len(ids) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(ids)
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

// Move active and completed activity keys with inherited user data so later undo finds the replacement rows.
func moveActivityKey(ctx context.Context, tx *sql.Tx, from, to string, now time.Time) (bool, error) {
	moved, err := movePairActivityKey(ctx, tx, from, to, now)
	if err != nil {
		return false, err
	}
	for _, statement := range []string{
		`update activity_entries set subject_key = ?2, updated_at = ?3 where subject_kind = 'video' and subject_key = ?1`,
		`update activity_entry_tags set user_key = ?2 where user_key = ?1`,
		`update or ignore activity_entry_tag_keys set user_key = ?2 where user_key = ?1`,
		`update or ignore activity_entry_keys set content_key = ?2 where content_key = ?1`,
	} {
		result, err := tx.ExecContext(ctx, statement, from, to, now.UnixMilli())
		if err != nil {
			return false, fmt.Errorf("cannot move the activity entries: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return false, fmt.Errorf("cannot move the activity entries: %w", err)
		} else if affected > 0 {
			moved = true
		}
	}
	movedUndoKeys, err := moveBundleUndoKeys(ctx, tx, from, to)
	if err != nil {
		return false, err
	}
	return moved || movedUndoKeys, nil
}

func moveBundleUndoKeys(ctx context.Context, tx *sql.Tx, from, to string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `select id, undo_keys from activity_entries
		where undo_keys is not null and instr(undo_keys, json_quote(?)) > 0`, from)
	if err != nil {
		return false, fmt.Errorf("cannot read the bundle undo keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	updates := map[int64]string{}
	for rows.Next() {
		var id int64
		var encoded string
		if err := rows.Scan(&id, &encoded); err != nil {
			return false, fmt.Errorf("cannot read the bundle undo keys: %w", err)
		}
		keys, err := domain.DecodeVersionBundleUndoKeys(encoded)
		if err != nil {
			return false, err
		}
		for i, key := range keys.Members {
			if key == from {
				keys.Members[i] = to
			}
		}
		if keys.Representative == from {
			keys.Representative = to
		}
		updates[id] = keys.Encode()
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("cannot read the bundle undo keys: %w", err)
	}
	if err := rows.Close(); err != nil {
		return false, fmt.Errorf("cannot read the bundle undo keys: %w", err)
	}
	for id, encoded := range updates {
		if _, err := tx.ExecContext(ctx, `update activity_entries set undo_keys = ? where id = ?`, encoded, id); err != nil {
			return false, fmt.Errorf("cannot move the bundle undo keys: %w", err)
		}
	}
	return len(updates) > 0, nil
}

// Rekey and sort pair subjects with their labels; skip pairs that would become self-pairs.
func movePairActivityKey(ctx context.Context, tx *sql.Tx, from, to string, now time.Time) (bool, error) {
	subjects, err := pairSubjects(ctx, tx, from)
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

func pairSubjects(ctx context.Context, tx *sql.Tx, key string) ([]domain.ActivityEntry, error) {
	rows, err := tx.QueryContext(ctx, `select id, subject_key, subject_key_b, subject_label,
		coalesce(subject_label_b, '') from activity_entries
		where subject_kind = ? and (subject_key = ?2 or subject_key_b = ?2)`,
		string(domain.ActivitySubjectPair), key)
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

// Return false if replacing the key would produce a self-pair.
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

// Record the verdict and decider; bundles retain joined keys for undo and supersede other activities within the bundle.
func recordJudgementActivity(
	ctx context.Context, tx *sql.Tx, c *changes, id int64, keys [2]string, result domain.VersionJudgeResult,
	applied appliedVerdict, now time.Time,
) error {
	switch {
	case applied.bundled:
		if err := closePairActivities(ctx, tx, c, applied.members, domain.ActivitySuperseded, "", id, now); err != nil {
			return err
		}
		return decideActivity(ctx, tx, c, id, activityUpdate{
			state:   domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySame},
			decider: result.ResultDecider(), probability: result.Probability, undoKeys: applied.undoKeys.Encode(),
		}, now)
	case result.Verdict == domain.VersionDifferent:
		return decideActivity(ctx, tx, c, id, activityUpdate{
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

type candidateVerdict struct {
	judgement   sql.NullString
	probability sql.NullFloat64
	err         sql.NullString
}

// Undecided model results retain AI confidence; rule conflicts have no confidence, and failures retain their detail.
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

// Undoing a drag bundle reopens the same owner-closed activity using the restored candidate state.
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

// RetryJudgement makes failed or interrupted candidates immediately claimable; the caller wakes the worker.
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

// UndoVersionJudgement rechecks current state atomically and holds the pair from judging; changed state yields already_reverted.
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
	mark, err := decidedMark(ctx, tx, id)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	c := &changes{}
	keys := [2]string{entry.Subject.Key, entry.Subject.KeyB}
	var restored bool
	var events []domain.Event
	if entry.Outcome == domain.ActivitySame {
		restored, events, err = undoJudgedBundle(ctx, tx, c, keys, entry.UndoKeys, mark)
	} else {
		restored, err = undoJudgedDismissal(ctx, tx, c, keys, mark)
		events = []domain.Event{domain.VersionCandidatesChanged{}}
	}
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	undoID, err := recordUndo(ctx, tx, c, entry, restored, time.Now())
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	// Unbundling may change smart-folder matches, so commitWith prepares folder notifications in the same transaction.
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot undo the version judgement: %w", err)
	}
	return s.ActivityEntry(ctx, undoID)
}

// A no-op undo records already_reverted.
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

// Remove an automatic dismissal only if no later owner dismissal exists; compare activity IDs with the decision mark.
func undoJudgedDismissal(ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, mark int64) (bool, error) {
	var redecided bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from activity_entries
		where kind = ? and subject_key = ? and subject_key_b = ? and subject_kind = ? and decider = ?
		and id > ?)`, string(domain.ActivityDismiss), keys[0], keys[1], string(domain.ActivitySubjectPair),
		string(domain.ActivityByOwner), mark).Scan(&redecided); err != nil {
		return false, fmt.Errorf("cannot read the owner's dismissal: %w", err)
	}
	if redecided {
		return false, nil
	}
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

// Undo an automatic bundle only while its keys still share a bundle, restoring the joined side's original representative.
func undoJudgedBundle(
	ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, undoKeys string, mark int64,
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
	leaving, err := leavingMembers(ctx, tx, joined.Members, bundles[0].id, mark)
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

// Exclude missing members and keys changed by later owner actions, using the decision mark rather than timestamps.
func leavingMembers(ctx context.Context, tx *sql.Tx, joined []string, bundleID int64, mark int64) ([]string, error) {
	kinds := []domain.ActivityKind{domain.ActivityBundle, domain.ActivityUnbundle, domain.ActivityOrganizeUndo}
	var leaving []string
	for _, key := range joined {
		var member bool
		if err := tx.QueryRowContext(ctx, `select exists (select 1 from video_bundle_members where content_key = ? and bundle_id = ?)`,
			key, bundleID).Scan(&member); err != nil {
			return nil, fmt.Errorf("cannot read the bundle member: %w", err)
		}
		if !member {
			continue
		}
		if _, err := videoIDForContent(ctx, tx, key); errors.Is(err, domain.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		if changed, err := ownerEntryAfter(ctx, tx, mark, kinds, `e.subject_key = ?`, key); err != nil {
			return nil, err
		} else if changed {
			continue
		}
		leaving = append(leaving, key)
	}
	return leaving, nil
}
