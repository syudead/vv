package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Owner activity is written atomically with the edit; batch video records in one statement even for large tag selections.

// Bundle keys target their representative content; a nonzero tag ID supplies the detail label.
type ownerVideoChange struct {
	Key    string `json:"k"`
	TagID  int64  `json:"t,omitempty"`
	Detail string `json:"d,omitempty"`
}

func ownerVideoChanges(keys []string, detail string) []ownerVideoChange {
	out := make([]ownerVideoChange, 0, len(keys))
	for _, key := range keys {
		out = append(out, ownerVideoChange{Key: key, Detail: detail})
	}
	return out
}

func ownerTagChanges(keys []string, tagID int64) []ownerVideoChange {
	out := make([]ownerVideoChange, 0, len(keys))
	for _, key := range keys {
		out = append(out, ownerVideoChange{Key: key, TagID: tagID})
	}
	return out
}

func recordOwnerVideoActivity(
	ctx context.Context, tx activityWriter, c *changes, kind domain.ActivityKind, changes []ownerVideoChange, now time.Time,
) error {
	if len(changes) == 0 {
		return nil
	}
	feature, ok := kind.Feature()
	if !ok {
		return fmt.Errorf("cannot record the owner's activity: unknown kind %q", kind)
	}
	encoded, err := json.Marshal(changes)
	if err != nil {
		return fmt.Errorf("cannot record the owner's activity: %w", err)
	}
	return insertOwnerVideoEntries(ctx, tx, c, feature, `select j.key, json_extract(j.value, '$.k'),
		json_extract(j.value, '$.t'), nullif(json_extract(j.value, '$.d'), ''), ? from json_each(?) j`,
		now, string(kind), string(encoded))
}

// Resolve bundle user keys to representative content keys for activity subjects.
func ownerSubjectKeyExpr(keyExpr string) string {
	return `coalesce((select b.representative_key from video_bundles b where b.user_key = ` + keyExpr + `), ` + keyExpr + `)`
}

// Insert ordered owner entries in one statement and retain affected content keys for undo provenance; notify after commit.
func insertOwnerVideoEntries(
	ctx context.Context, tx activityWriter, c *changes, feature domain.ActivityFeature, source string, now time.Time,
	args ...any,
) error {
	var before int64
	if err := tx.QueryRowContext(ctx, `select coalesce(max(id), 0) from activity_entries`).Scan(&before); err != nil {
		return fmt.Errorf("cannot record the owner's activity: %w", err)
	}
	at := now.UnixMilli()
	// Use named parameters here to avoid collisions with the source query's positional parameters.
	args = append(args, sql.Named("feature", string(feature)), sql.Named("subject_kind", string(domain.ActivitySubjectVideo)),
		sql.Named("status", string(domain.ActivityDone)), sql.Named("outcome", string(domain.ActivityCompleted)),
		sql.Named("decider", string(domain.ActivityByOwner)), sql.Named("at", at), sql.Named("before", before))
	result, err := tx.ExecContext(ctx, `with o (position, user_key, tag_id, detail, kind) as (`+source+`)
		insert into activity_entries (kind, feature, subject_kind, subject_key, subject_label, status, outcome, detail,
			tag_id, decider, created_at, updated_at)
		select s.kind, :feature, :subject_kind, s.subject_key, `+contentLabelExpr("s.subject_key")+`, :status, :outcome,
			s.detail, s.tag_id, :decider, :at, :at
		from (select o.position, o.tag_id, o.kind, `+ownerSubjectKeyExpr("o.user_key")+` as subject_key,
				case when o.tag_id is not null
					then (select tn.name from tag_names tn where tn.tag_id = o.tag_id and tn.canonical = 1)
					else o.detail end as detail
			from o) s
		order by s.position, s.tag_id`, args...)
	if err != nil {
		return fmt.Errorf("cannot record the owner's activity: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot record the owner's activity: %w", err)
	}
	if affected == 0 {
		return nil
	}
	c.activityChanged()
	return insertOwnerEntryKeys(ctx, tx, source, args...)
}

// Record every current bundle member's content key, or the single content key, for each owner tag edit.
func insertOwnerEntryKeys(ctx context.Context, tx activityWriter, source string, args ...any) error {
	if _, err := tx.ExecContext(ctx, `with o (position, user_key, tag_id, detail, kind) as (`+source+`)
		insert or ignore into activity_entry_keys (entry_id, content_key)
		select e.id, k.content_key
		from o join activity_entries e on e.id > :before and e.created_at = :at and e.kind = o.kind
			and e.tag_id = o.tag_id and e.subject_key = `+ownerSubjectKeyExpr("o.user_key")+`
		join (select m.content_key, b.user_key from video_bundle_members m join video_bundles b on b.id = m.bundle_id
			union all select v.content_key, v.content_key from videos v where v.content_key <> ''
				and not exists (select 1 from video_bundle_members vm where vm.content_key = v.content_key)) k
			on k.user_key = o.user_key
		where o.tag_id is not null`, args...); err != nil {
		return fmt.Errorf("cannot record the keys of the owner's activity: %w", err)
	}
	return nil
}

// Record changed key/tag pairs before writing in the same transaction; keep the cross product inside SQLite rather than returning it to Go.
func recordManualTagActivity(
	ctx context.Context, tx *sql.Tx, c *changes, encodedKeys string, action domain.VideoTagsAction, encodedTags string,
	keepSuggested bool, now time.Time,
) error {
	missing := `select k.key, k.value, t.value, null, ?3 from json_each(?1) k cross join json_each(?2) t
		where ` + unheldManualTag(keepSuggested)
	const present = `select k.key, k.value, vt.tag_id, null, ?4 from json_each(?1) k
		join video_tags vt on vt.content_key = k.value where vt.tag_id in (select value from json_each(?2))`
	const extra = `select k.key, k.value, vt.tag_id, null, ?4 from json_each(?1) k
		join video_tags vt on vt.content_key = k.value where vt.tag_id not in (select value from json_each(?2))`
	var source string
	switch action {
	case domain.VideoTagsAdd:
		source = missing
	case domain.VideoTagsRemove:
		source = present
	case domain.VideoTagsReplace:
		source = missing + ` union all ` + extra
	default:
		return fmt.Errorf("unknown video tags action %q", action)
	}
	feature, _ := domain.ActivityTagAttach.Feature()
	return insertOwnerVideoEntries(ctx, tx, c, feature, source, now, encodedKeys, encodedTags,
		string(domain.ActivityTagAttach), string(domain.ActivityTagDetach))
}

// Record only members whose bundle changed; untouched representative-side members remain eligible for automatic undo.
func recordOwnerBundle(ctx context.Context, tx *sql.Tx, c *changes, members, joined []string) error {
	now := time.Now()
	if err := closePairActivities(ctx, tx, c, members, domain.ActivityDecidedByOwner, domain.ActivityByOwner, 0,
		now); err != nil {
		return err
	}
	return recordOwnerVideoActivity(ctx, tx, c, domain.ActivityBundle, ownerVideoChanges(joined, ""), now)
}

// Capture the maximum activity ID when writing the result, not when queuing it, so only later owner edits block undo.
func markDecided(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `update activity_entries set decided_mark = (select max(id) from activity_entries)
		where id = ?`, id); err != nil {
		return fmt.Errorf("cannot mark the decided activity entry (id=%d): %w", id, err)
	}
	return nil
}

// Legacy completed entries without a decision mark fall back to their own ID.
func decidedMark(ctx context.Context, tx *sql.Tx, id int64) (int64, error) {
	var mark int64
	if err := tx.QueryRowContext(ctx, `select coalesce(decided_mark, id) from activity_entries where id = ?`, id).
		Scan(&mark); err != nil {
		return 0, fmt.Errorf("cannot read the decision mark of the activity entry (id=%d): %w", id, err)
	}
	return mark, nil
}

// Use monotonically increasing activity IDs rather than timestamps to identify later owner edits.
func ownerEntryAfter(
	ctx context.Context, tx *sql.Tx, mark int64, kinds []domain.ActivityKind, condition string, args ...any,
) (bool, error) {
	encodedKinds, err := json.Marshal(kinds)
	if err != nil {
		return false, fmt.Errorf("cannot look for the owner's later activity: %w", err)
	}
	var found bool
	err = tx.QueryRowContext(ctx, `select exists (select 1 from activity_entries e
		where e.id > ? and e.decider = ? and e.subject_kind = ?
		and e.kind in (select value from json_each(?)) and `+condition+`)`,
		append([]any{mark, string(domain.ActivityByOwner), string(domain.ActivitySubjectVideo), string(encodedKinds)},
			args...)...).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("cannot look for the owner's later activity: %w", err)
	}
	return found, nil
}

func recordOwnerDismissal(ctx context.Context, tx *sql.Tx, c *changes, keys [2]string, now time.Time) error {
	subject := domain.ActivitySubject{Kind: domain.ActivitySubjectPair, Key: keys[0], KeyB: keys[1]}
	var err error
	if subject.Label, err = contentLabel(ctx, tx, keys[0]); err != nil {
		return err
	}
	if subject.LabelB, err = contentLabel(ctx, tx, keys[1]); err != nil {
		return err
	}
	_, err = recordActivity(ctx, tx, c, ownerDoneEntry(domain.ActivityDismiss, subject, ""), now)
	return err
}

func recordMediaFolderRemoval(ctx context.Context, tx *sql.Tx, c *changes, path string, removed int, now time.Time) error {
	subject := domain.ActivitySubject{Kind: domain.ActivitySubjectFolder, Key: path, Label: path}
	_, err := recordActivity(ctx, tx, c,
		ownerDoneEntry(domain.ActivityMediaFolderRemove, subject, strconv.Itoa(removed)), now)
	return err
}

func ownerDoneEntry(kind domain.ActivityKind, subject domain.ActivitySubject, detail string) domain.ActivityEntry {
	return domain.ActivityEntry{
		Kind: kind, Subject: subject, Status: domain.ActivityDone, Outcome: domain.ActivityCompleted,
		Detail: detail, Decider: domain.ActivityByOwner,
	}
}

// RecordWorkInfoFetch records completed or failed fetches without offering retry; missing videos return domain.ErrNotFound.
func (s *OverrideStore) RecordWorkInfoFetch(ctx context.Context, videoID int64, failure string) error {
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot record the work-information fetch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	keys, err := registeredContentKeysForVideoIDs(ctx, tx, []int64{videoID})
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return domain.ErrNotFound
	}
	subject, err := videoActivitySubject(ctx, tx, videoID)
	if err != nil {
		return err
	}
	entry := ownerDoneEntry(domain.ActivityWorkInfoFetch, subject, failure)
	if failure != "" {
		entry.Outcome = domain.ActivityOutcomeFailed
	}
	c := &changes{}
	if _, err := recordActivity(ctx, tx, c, entry, time.Now()); err != nil {
		return err
	}
	if err := s.db.commit(ctx, tx, c); err != nil {
		return fmt.Errorf("cannot record the work-information fetch: %w", err)
	}
	return nil
}
