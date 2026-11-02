package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Public flags use shared user-data keys; all guest library reads enforce them through visibleLocationCondition.

// Unidentified content is never public, even if an empty public key exists.
func publicVideoCondition(alias string) string {
	return `exists (select 1 from videos pub_v join public_videos pub on pub.content_key = ` + userKeyExpr("pub_v") + ` ` +
		`where pub_v.id = ` + alias + `.video_id and pub_v.content_key <> '')`
}

// All guest-facing reads require registered public locations; the zero audience is guest, while ingest and job predicates retain owner semantics.
func visibleLocationCondition(alias string, audience domain.Audience) string {
	registered := registeredLocationCondition(alias)
	if audience.IsOwner() {
		return registered
	}
	return registered + ` and ` + publicVideoCondition(alias)
}

// Lists, searches, counts and folder reads add the audience's folder scope to visibleLocationCondition; reads by id
// keep visibleLocationCondition so an open video and writes to it still find it
// (docs/design-docs/media-folder-switches.md).
func listedLocationCondition(alias string, audience domain.Audience) string {
	listed := scopedLocationCondition(alias, audience)
	if audience.IsOwner() {
		return listed
	}
	return listed + ` and ` + publicVideoCondition(alias)
}

// scopedLocationCondition is registeredLocationCondition under the audience's folder scope, without the role's
// public rule; folders never overlap, so the scoped folder replaces the registered one.
func scopedLocationCondition(alias string, audience domain.Audience) string {
	scope := `mf.enabled = 1`
	if ids, explicit := audience.ExcludedFolders(); explicit {
		scope = `mf.id not in (` + domain.JoinIDs(ids) + `)`
	}
	return `exists (select 1 from media_folders mf where ` + inMediaFolderCondition(alias+`.path`) + ` and ` + scope + `)`
}

// A file video needs a visible location; a projected virtual video has none and follows visibleVirtualCondition.
func visibleVideoCondition(alias string, audience domain.Audience) string {
	return videoConditionWith(alias, audience, visibleLocationCondition)
}

// listedVideoCondition is visibleVideoCondition under the folder scope; a virtual video has no folder.
func listedVideoCondition(alias string, audience domain.Audience) string {
	return videoConditionWith(alias, audience, listedLocationCondition)
}

// listedFileVideoCondition requires a listed location, leaving virtual videos out as registeredVideoCondition does.
func listedFileVideoCondition(alias string, audience domain.Audience) string {
	return `exists (select 1 from video_locations l where l.video_id = ` + alias + `.id and ` +
		listedLocationCondition("l", audience) + `)`
}

func videoConditionWith(alias string, audience domain.Audience, location func(string, domain.Audience) string) string {
	return `(exists (select 1 from video_locations l where l.video_id = ` + alias + `.id and ` +
		location("l", audience) + `) or ` + visibleVirtualCondition(alias+`.content_key`, audience) + `)`
}

// folderOffExpr is true for a file video visible to the audience with no location under its folder scope.
func folderOffExpr(alias string, audience domain.Audience) string {
	return `(not ` + listedVideoCondition(alias, audience) + `)`
}

// The owner sees every virtual video; a guest sees one only while its user key is public.
func visibleVirtualCondition(key string, audience domain.Audience) string {
	condition := `exists (select 1 from virtual_videos vis_vv where vis_vv.user_key = ` + key + `)`
	if audience.IsOwner() {
		return condition
	}
	return condition + ` and exists (select 1 from public_videos vis_pub where vis_pub.content_key = ` + key + `)`
}

// User data attaches to a video with a registered location or to a virtual video.
func libraryVideoCondition(alias string) string {
	return `(` + registeredVideoCondition(alias) + ` or ` +
		visibleVirtualCondition(alias+`.content_key`, domain.AudienceOwner) + `)`
}

var publicColumn = `exists (select 1 from public_videos pub where pub.content_key = ` + userKeyExpr("videos") +
	` and videos.content_key <> '')`

// SetVideosPublic atomically writes shared user keys and returns all affected content keys so callers can stop guest streams for every bundle version.
func (s *VisibilityStore) SetVideosPublic(ctx context.Context, videoIDs []int64, public bool) ([]string, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot update the public flag: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	userKeys, err := userKeysForVideoIDs(ctx, tx, videoIDs)
	if err != nil {
		return nil, err
	}
	keys, err := contentKeysForUserKeys(ctx, tx, userKeys)
	if err != nil {
		return nil, err
	}
	if _, err := setPublicForUserKeys(ctx, tx, userKeys, public, time.Now()); err != nil {
		return nil, err
	}
	if err := s.db.commitMatchChange(ctx, tx); err != nil {
		return nil, fmt.Errorf("cannot update the public flag: %w", err)
	}
	return keys, nil
}

// Write the flag for each shared user key and advance edit time, including every bundle member, only for the
// keys whose visibility changed; the caller commits with a match change.
func setPublicForUserKeys(ctx context.Context, tx *sql.Tx, userKeys []string, public bool, now time.Time) ([]string, error) {
	changed := make([]string, 0, len(userKeys))
	for _, key := range userKeys {
		var result sql.Result
		var err error
		if public {
			result, err = tx.ExecContext(ctx,
				`insert or ignore into public_videos (content_key, published_at) values (?, ?)`, key, now.Unix())
		} else {
			result, err = tx.ExecContext(ctx, `delete from public_videos where content_key = ?`, key)
		}
		if err != nil {
			return nil, fmt.Errorf("cannot update the public flag: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("cannot update the public flag: %w", err)
		}
		if affected > 0 {
			changed = append(changed, key)
		}
	}
	return changed, touchEditedAtForUserKeys(ctx, tx, changed, now)
}
