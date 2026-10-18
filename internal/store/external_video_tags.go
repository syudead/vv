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

// External bulk tag edits fail atomically on unresolved videos and affect manual attachments only.

// ApplyVideoTags resolves normalized names and synonyms atomically, preserves input video order, and skips rejected tentative names.
func (s *TagStore) ApplyVideoTags(ctx context.Context, videos []domain.VideoRef, action domain.VideoTagsAction, names []string, tentative bool) (domain.VideoTagsOutcome, error) {
	if !action.Valid() {
		return domain.VideoTagsOutcome{}, fmt.Errorf("unknown video tags action %q", action)
	}
	normalized := make([]string, 0, len(names))
	for i, name := range names {
		n, err := domain.NormalizeTagName(name)
		if err != nil {
			return domain.VideoTagsOutcome{}, &domain.TagNameAtError{Index: i, Err: err}
		}
		normalized = append(normalized, n)
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}
	defer func() { _ = tx.Rollback() }()

	targets, err := resolveTaggableVideos(ctx, tx, videos)
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}
	keys := uniqueUserKeys(targets)
	c := &changes{}
	skipped, err := writeVideoTagsByName(ctx, tx, keys, action, normalized, tagWrite{tentative: tentative, activity: c},
		time.Now())
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}

	tags, err := tagsByContentKeys(ctx, tx, keys)
	if err != nil {
		return domain.VideoTagsOutcome{}, err
	}
	c.matchesChanged()
	if err := s.db.commit(ctx, tx, c); err != nil {
		return domain.VideoTagsOutcome{}, fmt.Errorf("cannot update the video tags: %w", err)
	}

	out := make([]domain.VideoTagsResult, 0, len(targets))
	for _, target := range targets {
		result := domain.VideoTagsResult{VideoID: target.id, ContentKey: target.contentKey, Tags: []domain.VideoTag{}}
		if got := tags[target.userKey]; got != nil {
			result.Tags = got
		}
		out = append(out, result)
	}
	return domain.VideoTagsOutcome{Items: out, SkippedNames: skipped}, nil
}

// keepSuggested preserves tentative attachments; activity records owner changes when supplied.
type tagWrite struct {
	tentative     bool
	keepSuggested bool
	activity      *changes
}

// Resolve names and write tags in the caller's transaction; advance edit times only for keys whose attachments change.
func writeVideoTagsByName(
	ctx context.Context, tx *sql.Tx, keys []string, action domain.VideoTagsAction, names []string, mode tagWrite, now time.Time,
) ([]string, error) {
	tagIDs, skipped, err := resolveTagNames(ctx, tx, action, names, mode.tentative)
	if err != nil {
		return nil, err
	}
	changed, err := applyManualTags(ctx, tx, keys, action, tagIDs, mode, now)
	if err != nil {
		return nil, err
	}
	if err := touchEditedAtForUserKeys(ctx, tx, changed, now); err != nil {
		return nil, err
	}
	return skipped, nil
}

// Read and write tags through the resolved user-data key.
type taggableVideo struct {
	id         int64
	contentKey string
	userKey    string
}

// Videos without content keys cannot hold tags and are reported as unresolved references.
func resolveTaggableVideos(ctx context.Context, tx *sql.Tx, videos []domain.VideoRef) ([]taggableVideo, error) {
	out := make([]taggableVideo, 0, len(videos))
	for i, ref := range videos {
		if !ref.Valid() {
			return nil, &domain.VideoRefNotFoundError{Index: i}
		}
		id, err := resolveVideoRef(ctx, tx, ref)
		if errors.Is(err, domain.ErrNotFound) {
			return nil, &domain.VideoRefNotFoundError{Index: i}
		}
		if err != nil {
			return nil, err
		}
		var key, userKey string
		if err := tx.QueryRowContext(ctx, `select v.content_key, `+userKeyExpr("v")+` from videos v where v.id = ?`, id).
			Scan(&key, &userKey); err != nil {
			return nil, fmt.Errorf("cannot read the content key (video=%d): %w", id, err)
		}
		if key == "" {
			return nil, &domain.VideoRefNotFoundError{Index: i}
		}
		out = append(out, taggableVideo{id: id, contentKey: key, userKey: userKey})
	}
	return out, nil
}

// Resolve names and synonyms once; remove skips unknown names, while tentative creation skips rejected names.
func resolveTagNames(ctx context.Context, tx *sql.Tx, action domain.VideoTagsAction, names []string, tentative bool) ([]int64, []string, error) {
	seen := make(map[int64]bool, len(names))
	ids := make([]int64, 0, len(names))
	skipped := []string{}
	skippedSeen := make(map[string]bool)
	for _, name := range names {
		lookup, found, err := lookupTagName(ctx, tx, name)
		if err != nil {
			return nil, nil, err
		}
		var id int64
		switch {
		case found:
			id = lookup.tagID
		case action == domain.VideoTagsRemove:
			continue
		case tentative:
			rejected, err := isRejectedTagName(ctx, tx, name)
			if err != nil {
				return nil, nil, err
			}
			if rejected {
				if !skippedSeen[name] {
					skippedSeen[name] = true
					skipped = append(skipped, name)
				}
				continue
			}
			if id, err = insertTag(ctx, tx, name, true); err != nil {
				return nil, nil, err
			}
		default:
			if id, err = insertTag(ctx, tx, name, false); err != nil {
				return nil, nil, err
			}
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, skipped, nil
}

func isRejectedTagName(ctx context.Context, q rowQueryer, name string) (bool, error) {
	var n int
	if err := q.QueryRowContext(ctx, `select count(*) from rejected_tag_names where name = ?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("cannot read rejected tag names (%s): %w", name, err)
	}
	return n > 0, nil
}

// Preserve first occurrence order while deduplicating content keys.
func uniqueContentKeys(targets []taggableVideo) []string {
	seen := make(map[string]bool, len(targets))
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		if !seen[target.contentKey] {
			seen[target.contentKey] = true
			keys = append(keys, target.contentKey)
		}
	}
	return keys
}

// Deduplicate user-data keys in input order; bundle members share one key.
func uniqueUserKeys(targets []taggableVideo) []string {
	seen := make(map[string]bool, len(targets))
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		if !seen[target.userKey] {
			seen[target.userKey] = true
			keys = append(keys, target.userKey)
		}
	}
	return keys
}

// Use set-based SQL to bound statement count for large batches; identify changed keys before writing in the same transaction.
func applyManualTags(
	ctx context.Context, tx *sql.Tx, keys []string, action domain.VideoTagsAction, tagIDs []int64, mode tagWrite, now time.Time,
) ([]string, error) {
	encodedKeys, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("cannot build content keys: %w", err)
	}
	encodedTags, err := json.Marshal(tagIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build tag ids: %w", err)
	}
	changed, err := changedManualTagKeys(ctx, tx, string(encodedKeys), action, string(encodedTags), mode.keepSuggested)
	if err != nil {
		return nil, err
	}
	if mode.activity != nil {
		if err := recordManualTagActivity(ctx, tx, mode.activity, string(encodedKeys), action, string(encodedTags),
			mode.keepSuggested, now); err != nil {
			return nil, err
		}
	}
	switch action {
	case domain.VideoTagsReplace:
		if _, err := tx.ExecContext(ctx,
			`delete from video_tags
			 where content_key in (select value from json_each(?))
			   and tag_id not in (select value from json_each(?))`,
			string(encodedKeys), string(encodedTags),
		); err != nil {
			return nil, fmt.Errorf("cannot replace the video tags: %w", err)
		}
		fallthrough
	case domain.VideoTagsAdd:
		onConflict := `do update set tentative = 0 where video_tags.tentative = 1`
		if mode.keepSuggested {
			onConflict = `do nothing`
		}
		if _, err := tx.ExecContext(ctx,
			`insert into video_tags (content_key, tag_id, created_at)
			 select k.value, t.value, ? from json_each(?) as k cross join json_each(?) as t where true
			 on conflict (content_key, tag_id) `+onConflict,
			now.Unix(), string(encodedKeys), string(encodedTags),
		); err != nil {
			return nil, fmt.Errorf("cannot add the video tags: %w", err)
		}
	case domain.VideoTagsRemove:
		if _, err := tx.ExecContext(ctx,
			`delete from video_tags
			 where content_key in (select value from json_each(?))
			   and tag_id in (select value from json_each(?))`,
			string(encodedKeys), string(encodedTags),
		); err != nil {
			return nil, fmt.Errorf("cannot remove the video tags: %w", err)
		}
	}
	return changed, nil
}

// Return at most one row per changed key; promoting a suggestion also counts as a change.
func changedManualTagKeys(
	ctx context.Context, tx *sql.Tx, encodedKeys string, action domain.VideoTagsAction, encodedTags string, keepSuggested bool,
) ([]string, error) {
	missing := `exists (select 1 from json_each(:tags) t where ` + unheldManualTag(keepSuggested) + `)`
	present := `exists (select 1 from video_tags vt where vt.content_key = k.value
		and vt.tag_id in (select value from json_each(:tags)))`
	extra := `exists (select 1 from video_tags vt where vt.content_key = k.value
		and vt.tag_id not in (select value from json_each(:tags)))`
	var condition string
	switch action {
	case domain.VideoTagsAdd:
		condition = missing
	case domain.VideoTagsRemove:
		condition = present
	case domain.VideoTagsReplace:
		condition = missing + ` or ` + extra
	default:
		return nil, fmt.Errorf("unknown video tags action %q", action)
	}
	rows, err := tx.QueryContext(ctx,
		`select k.value from json_each(:keys) k where `+condition+` order by k.key`,
		sql.Named("keys", encodedKeys), sql.Named("tags", encodedTags))
	if err != nil {
		return nil, fmt.Errorf("cannot read the changed video tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot read the changed video tags: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the changed video tags: %w", err)
	}
	return keys, nil
}

// An existing tentative attachment changes when promotion is requested.
func unheldManualTag(keepSuggested bool) string {
	held := `vt.content_key = k.value and vt.tag_id = t.value`
	if !keepSuggested {
		held += ` and vt.tentative = 0`
	}
	return `not exists (select 1 from video_tags vt where ` + held + `)`
}
