package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Update auto-tag activity in the queue transaction; retain applied tags for atomic per-tag undo.

// Prefer the override label, then a registered representative location, then the content key.
func contentLabelExpr(keyExpr string) string {
	return `coalesce((select nullif(ov.display_name, '') from video_overrides ov where ov.content_key = ` + keyExpr + `),
		(select l.title from videos lv join video_locations l on l.video_id = lv.id
			where lv.content_key = ` + keyExpr + ` and ` + registeredLocationCondition("l") + ` order by l.path limit 1),
		` + keyExpr + `)`
}

func autoTagActivityEntry(key, label string, reason domain.ActivityReason, decider domain.ActivityDecider) domain.ActivityEntry {
	return domain.ActivityEntry{
		Kind:    domain.ActivityAutoTag,
		Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectVideo, Key: key, Label: label},
		Status:  domain.ActivityRunning,
		Reason:  reason,
		Decider: decider,
	}
}

// Create a new processing activity if the previous entry is no longer running, preserving its decider and pipelines
// when available.
func claimAutoTagActivity(ctx context.Context, tx *sql.Tx, c *changes, key string, activityID int64, now time.Time) error {
	decider := domain.ActivityByRule
	var continued []int64
	if activityID != 0 {
		var status, stored string
		err := tx.QueryRowContext(ctx, `select status, decider from activity_entries where id = ?`, activityID).
			Scan(&status, &stored)
		switch {
		case err == nil && domain.ActivityStatus(status) == domain.ActivityRunning:
			return setActivityState(ctx, tx, c, activityID,
				domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityProcessing}, now)
		case err == nil:
			decider = domain.ActivityDecider(stored)
			continued = []int64{activityID}
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("cannot read the auto-tagging activity (id=%d): %w", activityID, err)
		}
	}
	var label string
	if err := tx.QueryRowContext(ctx, `select `+contentLabelExpr("?1"), key).Scan(&label); err != nil {
		return fmt.Errorf("cannot read the label of the auto-tagging subject: %w", err)
	}
	id, err := recordContinuedActivity(ctx, tx, c,
		autoTagActivityEntry(key, label, domain.ActivityProcessing, decider), continued, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update auto_tag_queue set activity_id = ? where content_key = ?`, id, key); err != nil {
		return fmt.Errorf("cannot link the auto-tagging job to its activity: %w", err)
	}
	return nil
}

// Ignore legacy zero activity IDs; completed entries retain a decision mark for undo.
func closeAutoTagActivity(
	ctx context.Context, tx *sql.Tx, c *changes, id int64, state domain.ActivityState, decider domain.ActivityDecider,
	detail string, now time.Time,
) error {
	if id == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `update activity_entries set status = ?, reason = ?, outcome = ?,
		decider = coalesce(nullif(?, ''), decider), detail = coalesce(nullif(?, ''), detail), updated_at = ?
		where id = ?`, string(state.Status), nullString(string(state.Reason)), nullString(string(state.Outcome)),
		string(decider), detail, now.UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("cannot update the auto-tagging activity (id=%d): %w", id, err)
	}
	if state.Status == domain.ActivityDone {
		if err := markDecided(ctx, tx, id); err != nil {
			return err
		}
	}
	c.activityChanged()
	return rollUpPipelines(ctx, tx, []int64{id}, now.UnixMilli())
}

type autoTagWrite struct {
	tagID     int64
	suggested bool
}

// Confirmed tags promote tentative attachments; tentative tags preserve existing attachments, including owner edits.
func writeAutoTags(
	ctx context.Context, tx *sql.Tx, userKey string, selection domain.AutoTagSelection, now time.Time,
) ([]autoTagWrite, error) {
	groups := []struct {
		ids       []int64
		suggested bool
		statement string
	}{
		{selection.Confirmed, false, `insert into video_tags (content_key, tag_id, created_at, tentative) values (?, ?, ?, 0)
			on conflict (content_key, tag_id) do update set tentative = 0, created_at = excluded.created_at
			where video_tags.tentative = 1`},
		{selection.Tentative, true, `insert or ignore into video_tags (content_key, tag_id, created_at, tentative)
			values (?, ?, ?, 1)`},
	}
	var written []autoTagWrite
	for _, group := range groups {
		ids, err := attachableAutoTags(ctx, tx, userKey, group.ids)
		if err != nil {
			return nil, err
		}
		for _, tagID := range ids {
			result, err := tx.ExecContext(ctx, group.statement, userKey, tagID, now.Unix())
			if err != nil {
				return nil, fmt.Errorf("cannot add an auto-tagging tag (tag=%d): %w", tagID, err)
			}
			if affected, err := result.RowsAffected(); err != nil {
				return nil, fmt.Errorf("cannot add an auto-tagging tag (tag=%d): %w", tagID, err)
			} else if affected > 0 {
				written = append(written, autoTagWrite{tagID: tagID, suggested: group.suggested})
			}
		}
	}
	return written, nil
}

func recordAutoTagActivityTags(
	ctx context.Context, tx *sql.Tx, entryID int64, userKey string, written []autoTagWrite,
	probabilities map[int64]float64,
) error {
	for _, write := range written {
		ref, err := tagRefByID(ctx, tx, write.tagID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `insert into activity_entry_tags (entry_id, tag_id, tag_name, suggested,
			user_key, probability) values (?, ?, ?, ?, ?, ?)`,
			entryID, write.tagID, ref.Name, boolToInt(write.suggested), userKey, probabilities[write.tagID]); err != nil {
			return fmt.Errorf("cannot record the auto-tagging tag (tag=%d): %w", write.tagID, err)
		}
		if _, err := tx.ExecContext(ctx, `insert into activity_entry_tag_keys (entry_id, tag_id, user_key) values (?, ?, ?)`,
			entryID, write.tagID, userKey); err != nil {
			return fmt.Errorf("cannot record the key of the auto-tagging tag (tag=%d): %w", write.tagID, err)
		}
	}
	return nil
}

// Track copied auto-tag rows so undo removes only rows derived from that decision.
func copyAutoTagKeys(ctx context.Context, tx *sql.Tx, from, to string) error {
	if _, err := tx.ExecContext(ctx, `insert or ignore into activity_entry_tag_keys (entry_id, tag_id, user_key)
		select k.entry_id, k.tag_id, ?2 from activity_entry_tag_keys k
		where k.user_key = ?1 and exists (select 1 from video_tags vt where vt.content_key = ?1 and vt.tag_id = k.tag_id)`,
		from, to); err != nil {
		return fmt.Errorf("cannot copy the keys of the auto-tagging tags: %w", err)
	}
	return nil
}

// Drop provenance when replacement removes all tag rows for a key.
func dropAutoTagKeys(ctx context.Context, tx *sql.Tx, key string) error {
	if _, err := tx.ExecContext(ctx, `delete from activity_entry_tag_keys where user_key = ?`, key); err != nil {
		return fmt.Errorf("cannot drop the keys of the auto-tagging tags: %w", err)
	}
	return nil
}

type autoTagUndoTarget struct {
	entry   domain.ActivityEntry
	tag     domain.ActivityEntryTag
	userKey string
	// The highest activity ID when the decision was written.
	mark int64
}

// Recheck the activity and attached tag in the transaction; missing targets return domain.ErrNotFound.
func readAutoTagUndoTarget(ctx context.Context, tx *sql.Tx, entryID, tagID int64) (autoTagUndoTarget, error) {
	var (
		target                     autoTagUndoTarget
		kind, subjectKind, decider string
	)
	err := tx.QueryRowContext(ctx, `select e.kind, e.subject_kind, e.subject_key, e.subject_label, e.decider,
		coalesce(e.decided_mark, e.id), t.tag_name, t.suggested, t.user_key, t.probability, coalesce(t.undone_by, 0)
		from activity_entries e join activity_entry_tags t on t.entry_id = e.id
		where e.id = ? and t.tag_id = ?`, entryID, tagID).Scan(&kind, &subjectKind, &target.entry.Subject.Key,
		&target.entry.Subject.Label, &decider, &target.mark, &target.tag.Name, &target.tag.Suggested, &target.userKey,
		&target.tag.Probability, &target.tag.UndoneBy)
	if errors.Is(err, sql.ErrNoRows) {
		return autoTagUndoTarget{}, fmt.Errorf("tag %d of activity entry %d: %w", tagID, entryID, domain.ErrNotFound)
	}
	if err != nil {
		return autoTagUndoTarget{}, fmt.Errorf("cannot read the activity entry to undo (id=%d): %w", entryID, err)
	}
	target.entry.ID = entryID
	target.entry.Kind = domain.ActivityKind(kind)
	target.entry.Subject.Kind = domain.ActivitySubjectKind(subjectKind)
	target.entry.Decider = domain.ActivityDecider(decider)
	target.tag.TagID = tagID
	return target, nil
}

// Return an empty key when the video no longer exists.
func currentUserKey(ctx context.Context, tx *sql.Tx, contentKey string) (string, error) {
	var key string
	err := tx.QueryRowContext(ctx, `select `+userKeyExpr("v")+` from videos v
		where v.content_key = ? and v.content_key <> '' limit 1`, contentKey).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cannot read the user key of the video: %w", err)
	}
	return key, nil
}

// Track removed keys, whether the primary attachment was reverted, and where rejection must be written.
type autoTagRemoval struct {
	removed  []string
	reverted bool
	rejectAt []string
}

// Undo follows tracked copies, including hidden original rows; an untracked primary key belongs to another attachment and is preserved.
func autoTagUndoKeys(ctx context.Context, tx *sql.Tx, target autoTagUndoTarget) (string, []string, error) {
	primary, err := currentUserKey(ctx, tx, target.entry.Subject.Key)
	if err != nil {
		return "", nil, err
	}
	if primary == "" {
		primary = target.userKey
	}
	keys, err := queryStrings(ctx, tx, `select user_key from activity_entry_tag_keys where entry_id = ? and tag_id = ?
		order by case when user_key = ?3 then 0 else 1 end, user_key`, target.entry.ID, target.tag.TagID, primary)
	if err != nil {
		return "", nil, fmt.Errorf("cannot read the keys of the auto-tagging tag (tag=%d): %w", target.tag.TagID, err)
	}
	return primary, keys, nil
}

// Find later owner edits by tag ID, content key and decision mark, independent of renames or representative changes.
func ownerChangedAutoTag(ctx context.Context, tx *sql.Tx, target autoTagUndoTarget) (bool, error) {
	kinds := []domain.ActivityKind{domain.ActivityTagAttach, domain.ActivityTagDetach, domain.ActivityTagKeep,
		domain.ActivityTagReject}
	return ownerEntryAfter(ctx, tx, target.mark, kinds,
		`e.tag_id = ? and exists (select 1 from activity_entry_keys k where k.entry_id = e.id and k.content_key = ?)`,
		target.tag.TagID, target.entry.Subject.Key)
}

// Remove only tracked copies, and only still-tentative rows for suggestions; an unchanged primary attachment yields already_reverted.
func removeAutoTagAttachment(
	ctx context.Context, tx *sql.Tx, target autoTagUndoTarget, primary string, keys []string,
) (autoTagRemoval, error) {
	removal := autoTagRemoval{rejectAt: []string{primary}}
	for _, key := range keys {
		result, err := tx.ExecContext(ctx, `delete from video_tags where content_key = ? and tag_id = ?
			and (? = 0 or tentative = 1)`, key, target.tag.TagID, boolToInt(target.tag.Suggested))
		if err != nil {
			return autoTagRemoval{}, fmt.Errorf("cannot remove the auto-tagging tag (tag=%d): %w", target.tag.TagID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return autoTagRemoval{}, fmt.Errorf("cannot remove the auto-tagging tag (tag=%d): %w", target.tag.TagID, err)
		}
		if affected == 0 {
			continue
		}
		removal.removed = append(removal.removed, key)
		if key == primary {
			removal.reverted = true
		} else {
			removal.rejectAt = append(removal.rejectAt, key)
		}
	}
	return removal, nil
}

// Preserve later owner edits while still recording rejection on the current key.
func revertAutoTag(ctx context.Context, tx *sql.Tx, target autoTagUndoTarget) (autoTagRemoval, error) {
	primary, keys, err := autoTagUndoKeys(ctx, tx, target)
	if err != nil {
		return autoTagRemoval{}, err
	}
	changed, err := ownerChangedAutoTag(ctx, tx, target)
	if err != nil {
		return autoTagRemoval{}, err
	}
	if changed {
		return autoTagRemoval{rejectAt: []string{primary}}, nil
	}
	return removeAutoTagAttachment(ctx, tx, target, primary, keys)
}

// UndoAutoTag atomically removes unchanged automatic attachments and records rejection; later owner edits yield already_reverted.
func (s *ActivityStore) UndoAutoTag(ctx context.Context, entryID, tagID int64) (domain.ActivityEntry, error) {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	defer func() { _ = tx.Rollback() }()
	target, err := readAutoTagUndoTarget(ctx, tx, entryID, tagID)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if !target.entry.TagUndoOffered(target.tag) {
		return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
	}
	removal, err := revertAutoTag(ctx, tx, target)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	now := time.Now()
	for _, key := range removal.rejectAt {
		if _, err := tx.ExecContext(ctx, `insert into auto_tag_rejections (content_key, tag_id, rejected_at)
			values (?, ?, ?) on conflict (content_key, tag_id) do update set rejected_at = excluded.rejected_at`,
			key, tagID, now.Unix()); err != nil {
			return domain.ActivityEntry{}, fmt.Errorf("cannot remember the rejected tag (tag=%d): %w", tagID, err)
		}
	}
	outcome := domain.ActivityAlreadyReverted
	if removal.reverted {
		outcome = domain.ActivityCompleted
	}
	c := &changes{}
	undoID, err := recordActivity(ctx, tx, c, domain.ActivityEntry{
		Kind: domain.ActivityUndo, Subject: target.entry.Subject, Status: domain.ActivityDone, Outcome: outcome,
		Detail: target.tag.Name, UndoesID: entryID, Decider: domain.ActivityByOwner,
	}, now)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	if _, err := tx.ExecContext(ctx, `update activity_entry_tags set undone_by = ? where entry_id = ? and tag_id = ?`,
		undoID, entryID, tagID); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot mark the auto-tagging tag undone (tag=%d): %w", tagID, err)
	}
	affected, err := touchAutoTagUndoneKeys(ctx, tx, removal.removed, now)
	if err != nil {
		return domain.ActivityEntry{}, err
	}
	// Removing a tag may change smart-folder matches; prepare their notifications in the same transaction.
	var events []domain.Event
	if len(affected) > 0 {
		events = append(events, domain.AutoTagApplied{VideoIDs: affected})
	}
	if err := s.db.commitWith(ctx, tx, c, events...); err != nil {
		return domain.ActivityEntry{}, fmt.Errorf("cannot undo the auto-tagging tag (tag=%d): %w", tagID, err)
	}
	return s.ActivityEntry(ctx, undoID)
}

func touchAutoTagUndoneKeys(ctx context.Context, tx *sql.Tx, keys []string, now time.Time) ([]int64, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if err := touchEditedAtForUserKeys(ctx, tx, keys, now); err != nil {
		return nil, err
	}
	var affected []int64
	for _, key := range keys {
		ids, err := videoIDsForUserKey(ctx, tx, key)
		if err != nil {
			return nil, err
		}
		affected = append(affected, ids...)
	}
	slices.Sort(affected)
	return slices.Compact(affected), nil
}
