package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Defer same-path succession until a successful full scan and known replacement duration, since old content may reappear elsewhere during the scan.

// Record only when the deleted old content has a known duration; replace any existing successor record.
func recordSuccession(ctx context.Context, tx *sql.Tx, newKey, oldKey string, oldDurationMs sql.NullInt64, now int64) error {
	if newKey == "" || oldKey == "" || !oldDurationMs.Valid || oldDurationMs.Int64 <= 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `insert or replace into video_successions
		(new_key, old_key, old_duration_ms, ready, created_at) values (?, ?, ?, 0, ?)`,
		newKey, oldKey, oldDurationMs.Int64, now); err != nil {
		return fmt.Errorf("cannot record the succession: %w", err)
	}
	return nil
}

// Old content reappearing elsewhere means a file swap, not succession.
func cancelSuccessionsFrom(ctx context.Context, tx *sql.Tx, key string) error {
	if _, err := tx.ExecContext(ctx, `delete from video_successions where old_key = ?`, key); err != nil {
		return fmt.Errorf("cannot cancel the succession: %w", err)
	}
	return nil
}

func releaseContentIndex(ctx context.Context, tx *sql.Tx, released []domain.DeletedVideo) error {
	if len(released) == 0 {
		return nil
	}
	keys := make([]string, 0, len(released))
	for _, video := range released {
		keys = append(keys, video.ContentKey)
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("cannot build content keys: %w", err)
	}
	for _, statement := range []string{
		`delete from video_successions where new_key in (select value from json_each(?))`,
		`delete from video_fingerprints where content_key in (select value from json_each(?))`,
		`delete from video_version_candidates where key_a in (select value from json_each(?1))
		    or key_b in (select value from json_each(?1))`,
	} {
		if _, err := tx.ExecContext(ctx, statement, string(encoded)); err != nil {
			return fmt.Errorf("cannot release the content index: %w", err)
		}
	}
	return nil
}

// A successful scan marks candidates ready, then resolves those with known replacement duration.
func applyReadySuccessions(ctx context.Context, tx *sql.Tx) (succession, error) {
	if _, err := tx.ExecContext(ctx, `update video_successions set ready = 1`); err != nil {
		return succession{}, fmt.Errorf("cannot mark the successions ready: %w", err)
	}
	ready, err := judgeableSuccessions(ctx, tx)
	if err != nil {
		return succession{}, err
	}
	var changed succession
	for _, p := range ready {
		one, err := applySuccession(ctx, tx, p.newKey, p.durationMs)
		if err != nil {
			return succession{}, err
		}
		changed.videoIDs = append(changed.videoIDs, one.videoIDs...)
		changed.groupIDs = append(changed.groupIDs, one.groupIDs...)
		changed.activity = changed.activity || one.activity
	}
	return succession{
		videoIDs: normalizeIDs(changed.videoIDs), groupIDs: normalizeIDs(changed.groupIDs), activity: changed.activity,
	}, nil
}

type succession struct {
	videoIDs []int64
	groupIDs []int64
	activity bool
}

type judgeableSuccession struct {
	newKey     string
	durationMs int64
}

func judgeableSuccessions(ctx context.Context, tx *sql.Tx) ([]judgeableSuccession, error) {
	rows, err := tx.QueryContext(ctx, `select s.new_key, v.duration_ms from video_successions s
		join videos v on v.content_key = s.new_key and v.content_key <> ''
		where v.duration_ms is not null order by s.new_key`)
	if err != nil {
		return nil, fmt.Errorf("cannot read the successions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []judgeableSuccession
	for rows.Next() {
		var j judgeableSuccession
		if err := rows.Scan(&j.newKey, &j.durationMs); err != nil {
			return nil, fmt.Errorf("cannot read the successions: %w", err)
		}
		out = append(out, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the successions: %w", err)
	}
	return out, nil
}

// Consume ready succession only; transfer user data and bundle position when durations match and old content has no remaining video reference.
func applySuccession(ctx context.Context, tx *sql.Tx, contentKey string, durationMs int64) (succession, error) {
	if contentKey == "" {
		return succession{}, nil
	}
	var oldKey string
	var oldDurationMs int64
	err := tx.QueryRowContext(ctx, `select old_key, old_duration_ms from video_successions
		where new_key = ? and ready = 1`, contentKey).Scan(&oldKey, &oldDurationMs)
	if errors.Is(err, sql.ErrNoRows) {
		return succession{}, nil
	}
	if err != nil {
		return succession{}, fmt.Errorf("cannot read the succession: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `delete from video_successions where new_key = ?`, contentKey); err != nil {
		return succession{}, fmt.Errorf("cannot remove the succession: %w", err)
	}
	if !domain.DurationsMatch(oldDurationMs, durationMs) {
		return abandonSuccession(ctx, tx)
	}
	var referenced int
	if err := tx.QueryRowContext(ctx, `select exists(select 1 from videos where content_key = ?)`, oldKey).Scan(&referenced); err != nil {
		return succession{}, fmt.Errorf("cannot read the previous content: %w", err)
	}
	if referenced != 0 {
		return abandonSuccession(ctx, tx)
	}
	var newID int64
	err = tx.QueryRowContext(ctx, `select id from videos where content_key = ?`, contentKey).Scan(&newID)
	if errors.Is(err, sql.ErrNoRows) {
		return abandonSuccession(ctx, tx)
	}
	if err != nil {
		return succession{}, fmt.Errorf("cannot read the succeeding video: %w", err)
	}

	// Advance playlist timestamps after rekeying only when their visible shape changes.
	shapes, err := snapshotPlaylistShapes(ctx, tx, []string{oldKey, contentKey})
	if err != nil {
		return succession{}, err
	}
	bundle, bundled, err := bundleOf(ctx, tx, oldKey)
	if err != nil {
		return succession{}, err
	}
	if bundled {
		if err := moveBundleMember(ctx, tx, bundle, oldKey, contentKey); err != nil {
			return succession{}, err
		}
	}
	// Also move the old member's hidden pre-bundle user data.
	rekeyedGroups, err := moveUserData(ctx, tx, oldKey, contentKey)
	if err != nil {
		return succession{}, err
	}
	for _, table := range []string{"video_version_dismissals", "video_version_unbundles", "video_version_judging_holds"} {
		if err := moveKeyPairs(ctx, tx, table, oldKey, contentKey); err != nil {
			return succession{}, err
		}
	}
	// Rekey completed activities too, so later undo finds the new content's rows.
	movedActivity, err := moveActivityKey(ctx, tx, oldKey, contentKey, time.Now())
	if err != nil {
		return succession{}, err
	}
	// Updated bundles and dismissals can invalidate the replacement content's candidates.
	if err := pruneVersionCandidates(ctx, tx); err != nil {
		return succession{}, err
	}

	changed := []int64{newID}
	var changedGroups []int64
	if bundled {
		keys, err := bundleMemberKeys(ctx, tx, bundle.id)
		if err != nil {
			return succession{}, err
		}
		ids, err := videoIDsForContentKeys(ctx, tx, keys)
		if err != nil {
			return succession{}, err
		}
		changed = append(changed, ids...)
		if changedGroups, err = rebuildFolderIndex(ctx, tx); err != nil {
			return succession{}, err
		}
	}
	if err := touchReshapedPlaylists(ctx, tx, shapes); err != nil {
		return succession{}, err
	}
	return succession{
		videoIDs: normalizeIDs(changed), groupIDs: normalizeIDs(append(rekeyedGroups, changedGroups...)),
		activity: movedActivity,
	}, nil
}

// Rejected succession closes open pair activities as gone once the old content leaves the library.
func abandonSuccession(ctx context.Context, tx *sql.Tx) (succession, error) {
	c := &changes{}
	if err := closeGonePairActivities(ctx, tx, c, time.Now()); err != nil {
		return succession{}, err
	}
	return succession{activity: c.activity}, nil
}

// Preserve bundle values and update its representative; do not replace membership if the new key already belongs to a bundle.
func moveBundleMember(ctx context.Context, tx *sql.Tx, bundle bundleRef, oldKey, newKey string) error {
	if _, already, err := bundleOf(ctx, tx, newKey); err != nil {
		return err
	} else if already {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `update video_bundle_members set content_key = ? where content_key = ?`, newKey, oldKey); err != nil {
		return fmt.Errorf("cannot move the bundle member: %w", err)
	}
	if bundle.representativeKey == oldKey {
		if _, err := tx.ExecContext(ctx, `update video_bundles set representative_key = ?, updated_at = ? where id = ?`,
			newKey, time.Now().Unix(), bundle.id); err != nil {
			return fmt.Errorf("cannot move the bundle representative: %w", err)
		}
	}
	return nil
}

// Move scalar user data with source precedence and union tags/history; move all work-info tables as one unit.
func moveUserData(ctx context.Context, tx *sql.Tx, from, to string) ([]int64, error) {
	if err := moveWorkInfo(ctx, tx, from, to); err != nil {
		return nil, err
	}
	for _, table := range []string{"playback_progress", "public_videos", "video_favorites", "video_edits"} {
		if _, err := tx.ExecContext(ctx, `delete from `+table+` where content_key = ?
			and exists (select 1 from `+table+` where content_key = ?)`, to, from); err != nil {
			return nil, fmt.Errorf("cannot move the %s values: %w", table, err)
		}
		if _, err := tx.ExecContext(ctx, `update `+table+` set content_key = ? where content_key = ?`, to, from); err != nil {
			return nil, fmt.Errorf("cannot move the %s values: %w", table, err)
		}
	}
	if err := unionTagsAndHistory(ctx, tx, from, to); err != nil {
		return nil, err
	}
	return rekeyUserItems(ctx, tx, from, to)
}

// Union tags and auto-tag rejections and move watch history; succession and a virtual-video merge share it.
func unionTagsAndHistory(ctx context.Context, tx *sql.Tx, from, to string) error {
	if _, err := tx.ExecContext(ctx, `insert or ignore into video_tags (content_key, tag_id, created_at, tentative)
		select ?, tag_id, created_at, tentative from video_tags where content_key = ?`, to, from); err != nil {
		return fmt.Errorf("cannot move the video_tags values: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `delete from video_tags where content_key = ?`, from); err != nil {
		return fmt.Errorf("cannot move the video_tags values: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `insert or ignore into auto_tag_rejections (content_key, tag_id, rejected_at)
		select ?, tag_id, rejected_at from auto_tag_rejections where content_key = ?`, to, from); err != nil {
		return fmt.Errorf("cannot move the auto_tag_rejections values: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `delete from auto_tag_rejections where content_key = ?`, from); err != nil {
		return fmt.Errorf("cannot move the auto_tag_rejections values: %w", err)
	}
	// Each history row is a separate viewing, so retain existing destination rows.
	if _, err := tx.ExecContext(ctx, `update watch_history set content_key = ? where content_key = ?`, to, from); err != nil {
		return fmt.Errorf("cannot move the watch_history values: %w", err)
	}
	return nil
}

// Rekey sorted pairs, deduplicate collisions and discard self-pairs.
func moveKeyPairs(ctx context.Context, tx *sql.Tx, table, from, to string) error {
	pairs, err := keyPairsOf(ctx, tx, table, from)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from `+table+` where key_a = ? or key_b = ?`, from, from); err != nil {
		return fmt.Errorf("cannot move the %s pairs: %w", table, err)
	}
	for _, p := range pairs {
		if p.other == to {
			continue
		}
		if _, err := tx.ExecContext(ctx, `insert or ignore into `+table+` (key_a, key_b, created_at)
			values (min(?, ?), max(?, ?), ?)`, to, p.other, to, p.other, p.createdAt); err != nil {
			return fmt.Errorf("cannot move the %s pairs: %w", table, err)
		}
	}
	return nil
}

type keyPair struct {
	other     string
	createdAt int64
}

func keyPairsOf(ctx context.Context, tx *sql.Tx, table, key string) ([]keyPair, error) {
	rows, err := tx.QueryContext(ctx, `select key_a, key_b, created_at from `+table+`
		where key_a = ? or key_b = ?`, key, key)
	if err != nil {
		return nil, fmt.Errorf("cannot read the %s pairs: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var out []keyPair
	for rows.Next() {
		var a, b string
		var p keyPair
		if err := rows.Scan(&a, &b, &p.createdAt); err != nil {
			return nil, fmt.Errorf("cannot read the %s pairs: %w", table, err)
		}
		p.other = a
		if a == key {
			p.other = b
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the %s pairs: %w", table, err)
	}
	return out, nil
}

func successionEvents(changed succession) []domain.Event {
	var events []domain.Event
	if len(changed.videoIDs) > 0 {
		events = append(events, domain.VideoBundleChanged{VideoIDs: changed.videoIDs})
	}
	if changed.activity {
		events = append(events, domain.ActivityChanged{})
	}
	return append(events, groupsChangedEvents(changed.groupIDs)...)
}

func normalizeIDs(ids []int64) []int64 {
	slices.Sort(ids)
	return slices.Compact(ids)
}
