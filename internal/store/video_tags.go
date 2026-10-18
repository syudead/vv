package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// AttachTagByID atomically skips unresolved videos and counts resolved targets even when already attached; a missing tag fails.
func (s *TagStore) AttachTagByID(ctx context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	ref, err := tagRefByID(ctx, tx, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	applied, err := s.attachTagToVideoIDs(ctx, tx, videoIDs, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	return ref, applied, nil
}

// AttachTagByName resolves synonyms and creates missing tags in the same transaction, otherwise matching AttachTagByID.
func (s *TagStore) AttachTagByName(ctx context.Context, videoIDs []int64, name string) (domain.TagRef, int, error) {
	normalized, err := domain.NormalizeTagName(name)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	ref, _, err := findOrCreateTag(ctx, tx, normalized)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	applied, err := s.attachTagToVideoIDs(ctx, tx, videoIDs, ref.ID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	return ref, applied, nil
}

// DetachTag tolerates absent attachments but rejects a missing tag.
func (s *TagStore) DetachTag(ctx context.Context, videoIDs []int64, tagID int64) (domain.TagRef, int, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	ref, err := tagRefByID(ctx, tx, tagID)
	if err != nil {
		return domain.TagRef{}, 0, err
	}

	now := time.Now()
	applied, changed, err := detachTagFromVideoIDs(ctx, tx, videoIDs, tagID, now)
	if err != nil {
		return domain.TagRef{}, 0, err
	}
	if err := s.commitTagChanges(ctx, tx, domain.ActivityTagDetach, changed, tagID, now); err != nil {
		return domain.TagRef{}, 0, fmt.Errorf("cannot remove the tag: %w", err)
	}
	return ref, applied, nil
}

// Write owner activity and edit times only for changed shared user keys, then commit atomically.
func (s *TagStore) attachTagToVideoIDs(ctx context.Context, tx *sql.Tx, videoIDs []int64, tagID int64) (int, error) {
	keys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	changed, err := attachTagToUserKeys(ctx, tx, keys, tagID, now)
	if err != nil {
		return 0, err
	}
	if err := s.commitTagChanges(ctx, tx, domain.ActivityTagAttach, changed, tagID, now); err != nil {
		return 0, fmt.Errorf("cannot add the tag: %w", err)
	}
	return len(keys), nil
}

func (s *TagStore) commitTagChanges(
	ctx context.Context, tx *sql.Tx, kind domain.ActivityKind, changed []string, tagID int64, now time.Time,
) error {
	c := &changes{}
	if err := recordOwnerVideoActivity(ctx, tx, c, kind, ownerTagChanges(changed, tagID), now); err != nil {
		return err
	}
	c.matchesChanged()
	return s.db.commit(ctx, tx, c)
}

// Manual attachment promotes tentative suggestions and advances edit times for all affected bundle members.
func attachTagToUserKeys(ctx context.Context, tx *sql.Tx, keys []string, tagID int64, now time.Time) ([]string, error) {
	changed := make([]string, 0, len(keys))
	for _, key := range keys {
		result, err := tx.ExecContext(ctx,
			`insert into video_tags (content_key, tag_id, created_at) values (?, ?, ?)
			 on conflict (content_key, tag_id) do update set tentative = 0 where video_tags.tentative = 1`,
			key, tagID, now.Unix(),
		)
		if err != nil {
			return nil, fmt.Errorf("cannot add the tag (tag=%d): %w", tagID, err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return nil, fmt.Errorf("cannot add the tag (tag=%d): %w", tagID, err)
		} else if affected > 0 {
			changed = append(changed, key)
		}
	}
	return changed, touchEditedAtForUserKeys(ctx, tx, changed, now)
}

func detachTagFromVideoIDs(ctx context.Context, tx *sql.Tx, videoIDs []int64, tagID int64, now time.Time) (int, []string, error) {
	keys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return 0, nil, err
	}
	changed := make([]string, 0, len(keys))
	for _, key := range keys {
		result, err := tx.ExecContext(ctx,
			`delete from video_tags where content_key = ? and tag_id = ?`, key, tagID,
		)
		if err != nil {
			return 0, nil, fmt.Errorf("cannot remove the tag (tag=%d): %w", tagID, err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return 0, nil, fmt.Errorf("cannot remove the tag (tag=%d): %w", tagID, err)
		} else if affected > 0 {
			changed = append(changed, key)
		}
	}
	if err := touchEditedAtForUserKeys(ctx, tx, changed, now); err != nil {
		return 0, nil, err
	}
	return len(keys), changed, nil
}

// TagsByContentKeys uses user-data keys and unions manual/folder sources, retaining both source flags and omitting untagged keys.
func (s *TagStore) TagsByContentKeys(ctx context.Context, contentKeys []string) (map[string][]domain.VideoTag, error) {
	return tagsByContentKeys(ctx, s.sql, contentKeys)
}

// Use the caller's snapshot; folder-derived tags resolve through every video sharing the key.
func tagsByContentKeys(ctx context.Context, q queryExecer, contentKeys []string) (map[string][]domain.VideoTag, error) {
	if len(contentKeys) == 0 {
		return map[string][]domain.VideoTag{}, nil
	}
	encoded, err := json.Marshal(contentKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content_key values: %w", err)
	}

	rows, err := q.QueryContext(ctx, `
		with selected(content_key) as (select value from json_each(?)),
		keyed_videos(user_key, video_id) as (
			select v.content_key, v.id from videos v
			 where v.content_key in (select content_key from selected) and v.content_key <> ''
			   and not exists (select 1 from video_bundle_members m where m.content_key = v.content_key)
			union all
			select b.user_key, v.id from video_bundles b
			  join video_bundle_members m on m.bundle_id = b.id
			  join videos v on v.content_key = m.content_key and v.content_key <> ''
			 where b.user_key in (select content_key from selected)
		),
		tagged(content_key, tag_id, manual, from_folder, unconfirmed) as (
			select vt.content_key, vt.tag_id, 1, 0, vt.tentative from video_tags vt
			 where vt.content_key in (select content_key from selected)
			union all
			select k.user_key, folder_tn.tag_id, 0, 1, 0 from keyed_videos k
			  join video_folder_names vfn on vfn.video_id = k.video_id
			  join tag_names folder_tn on folder_tn.name = vfn.name
		)
		select t.content_key, t.tag_id, tn.name, tg.tentative, max(t.manual), max(t.from_folder), min(t.unconfirmed),
		       `+tagKindColumns+`
		  from tagged t
		  join tag_names tn on tn.tag_id = t.tag_id and tn.canonical = 1
		  join tags tg on tg.id = t.tag_id`+tagKindJoin+`
		 group by t.content_key, t.tag_id`, string(encoded),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot read item tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string][]domain.VideoTag, len(contentKeys))
	for rows.Next() {
		var key string
		var tag domain.VideoTag
		var kind scannedTagKind
		if err := rows.Scan(append([]any{&key, &tag.ID, &tag.Name, &tag.Tentative, &tag.Manual, &tag.FromFolder, &tag.Unconfirmed},
			kind.targets()...)...); err != nil {
			return nil, fmt.Errorf("cannot read item tags: %w", err)
		}
		tag.Kind = kind.ref()
		out[key] = append(out[key], tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read item tags: %w", err)
	}
	for key := range out {
		domain.SortVideoTags(out[key])
	}
	return out, nil
}
