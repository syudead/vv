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

func visibleVideoCondition(alias string, audience domain.Audience) string {
	return `exists (select 1 from video_locations l where l.video_id = ` + alias + `.id and ` +
		visibleLocationCondition("l", audience) + `)`
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
