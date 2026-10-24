package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// Derive shared bundle user keys in one place for tags, progress, visibility and favorites; overrides and artifacts remain content-specific.

// Unidentified content yields an empty key; callers must exclude it from user-data joins.
func userKeyExpr(alias string) string {
	return `coalesce((select ukb.user_key from video_bundle_members ukm ` +
		`join video_bundles ukb on ukb.id = ukm.bundle_id ` +
		`where ukm.content_key = ` + alias + `.content_key), ` + alias + `.content_key)`
}

// Resolve a key set to videos without per-video key lookups; keys appears twice, so bind its arguments twice.
func userKeyVideosSQL(keys string) string {
	return `select v.id from videos v where v.content_key <> '' and v.content_key in (` + keys + `) ` +
		`and not exists (select 1 from video_bundle_members ukm where ukm.content_key = v.content_key) ` +
		`union all select v.id from video_bundles ukb join video_bundle_members ukm on ukm.bundle_id = ukb.id ` +
		`join videos v on v.content_key = ukm.content_key and v.content_key <> '' where ukb.user_key in (` + keys + `)`
}

func bundleUserKey(id int64) string {
	return fmt.Sprintf("bundle:%d", id)
}

// Resolve registered identified or virtual videos to unique user keys; bundle members collapse to one key.
func userKeysForVideoIDs(ctx context.Context, q queryExecer, videoIDs []int64) ([]string, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(videoIDs)
	if err != nil {
		return nil, fmt.Errorf("cannot build video ids: %w", err)
	}

	query := `select distinct ` + userKeyExpr("v") + ` from videos v
		where v.id in (select value from json_each(?)) and v.content_key <> '' and ` +
		libraryVideoCondition("v")
	rows, err := q.QueryContext(ctx, query, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot map video ids to user keys: %w", err)
	}
	return keys, nil
}

// Expand bundle keys to every content key so visibility changes can terminate guest streams for all versions.
func contentKeysForUserKeys(ctx context.Context, q queryExecer, userKeys []string) ([]string, error) {
	if len(userKeys) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(userKeys)
	if err != nil {
		return nil, fmt.Errorf("cannot build user keys: %w", err)
	}
	rows, err := q.QueryContext(ctx, `
		with selected(user_key) as (select value from json_each(?))
		select s.user_key from selected s
		 where not exists (select 1 from video_bundles b where b.user_key = s.user_key)
		union
		select m.content_key from selected s
		  join video_bundles b on b.user_key = s.user_key
		  join video_bundle_members m on m.bundle_id = b.id`, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot map user keys to content keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot map user keys to content keys: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot map user keys to content keys: %w", err)
	}
	return keys, nil
}

// Lists use one audience-visible effective representative per bundle, falling back to the lowest video ID; details and streaming still allow other versions.

func effectiveRepresentativeExpr(bundleID string, audience domain.Audience) string {
	return `(select rv.id from video_bundle_members rm
		join video_bundles rb on rb.id = rm.bundle_id
		join videos rv on rv.content_key = rm.content_key and rv.content_key <> ''
		where rm.bundle_id = ` + bundleID + ` and ` + visibleVideoCondition("rv", audience) + `
		order by rv.content_key = rb.representative_key desc, rv.id limit 1)`
}

// This predicate checks bundle representation only; callers must add location or video visibility.
func shownVideoCondition(alias string, audience domain.Audience) string {
	return `(not exists (select 1 from video_bundle_members sm where sm.content_key = ` + alias + `.content_key and ` +
		alias + `.content_key <> '') or ` + alias + `.id = ` +
		effectiveRepresentativeExpr(`(select sm.bundle_id from video_bundle_members sm where sm.content_key = `+alias+`.content_key)`, audience) + `)`
}
