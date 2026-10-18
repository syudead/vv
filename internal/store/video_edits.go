package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Track actual owner edits by content key, falling back to added time when absent; each role updates it within its own transaction.

const editedAtColumn = `coalesce((select e.edited_at from video_edits e
		where e.content_key = videos.content_key and videos.content_key <> ''), videos.added_at) as edited_at`

// Use one captured transaction time and one set-based statement, excluding empty keys.
func touchEditedAt(ctx context.Context, tx *sql.Tx, contentKeys []string, now time.Time) error {
	if len(contentKeys) == 0 {
		return nil
	}
	encoded, err := json.Marshal(contentKeys)
	if err != nil {
		return fmt.Errorf("cannot build content keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`insert into video_edits (content_key, edited_at)
		 select distinct value, ? from json_each(?) where value <> ''
		 on conflict (content_key) do update set edited_at = excluded.edited_at`,
		now.Unix(), string(encoded),
	); err != nil {
		return fmt.Errorf("cannot record the edited time: %w", err)
	}
	return nil
}

// Expand shared bundle keys to all member content keys before touching edit times.
func touchEditedAtForUserKeys(ctx context.Context, tx *sql.Tx, userKeys []string, now time.Time) error {
	if len(userKeys) == 0 {
		return nil
	}
	keys, err := contentKeysForUserKeys(ctx, tx, userKeys)
	if err != nil {
		return err
	}
	return touchEditedAt(ctx, tx, keys, now)
}
