package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

// Bound compound SELECT size while updating folders; list requests never enumerate these targets.
const collectionValueBatch = 100

func refreshSmartFolderValues(ctx context.Context, tx *sql.Tx, audience domain.Audience, now int64) error {
	ids, err := queryInt64s(ctx, tx, changedCollections("smart", "smart_folders"))
	if err != nil {
		return err
	}
	for start := 0; start < len(ids); start += collectionValueBatch {
		if err := refreshSmartFolderValueBatch(ctx, tx, audience, ids[start:min(start+collectionValueBatch, len(ids))], now); err != nil {
			return err
		}
	}
	return nil
}

func refreshSmartFolderValueBatch(ctx context.Context, tx *sql.Tx, audience domain.Audience, ids []int64, now int64) error {
	targets := make([]smartFolderTarget, 0, len(ids))
	visible := make([]bool, 0, len(ids))
	for _, id := range ids {
		target, shown, err := smartFolderTargetByID(ctx, tx, audience, id, now)
		if err != nil {
			return err
		}
		targets = append(targets, target)
		visible = append(visible, shown)
	}
	values := make([]librarySmartFolder, len(targets))
	if err := readSmartFolderSortValues(ctx, tx, audience, targets, values); err != nil {
		return err
	}
	for i, id := range ids {
		v := values[i]
		if _, err := tx.ExecContext(ctx, `insert into smart_folder_values
			(folder_id, audience, visible, added_at, mtime, created_at, duration_ms, size_bytes, played_at, expires_at)
			values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) on conflict (folder_id, audience) do update set
			visible = excluded.visible, added_at = excluded.added_at, mtime = excluded.mtime, created_at = excluded.created_at,
			duration_ms = excluded.duration_ms, size_bytes = excluded.size_bytes, played_at = excluded.played_at, expires_at = excluded.expires_at`,
			id, audienceValue(audience), visible[i], v.values[0], v.values[1], v.values[2], v.values[3], v.values[4], v.values[5], v.expiresAt); err != nil {
			return fmt.Errorf("cannot update smart folder information: %w", err)
		}
	}
	return nil
}

// Schedule age boundaries independently of watch filters: a group may match a watch state
// even when no individual member does. The cutoff is inclusive to the whole second.
func smartFolderExpiry(spec listSpec) string {
	var days int
	for _, level := range spec.levels() {
		if level.addedWithinDays > 0 && (days == 0 || level.addedWithinDays < days) {
			days = level.addedWithinDays
		}
	}
	if days == 0 {
		return `null`
	}
	seconds := int64(days) * secondsPerDay
	return `(select min(added_at) + ` + strconv.FormatInt(seconds+1, 10) +
		` from smart_base where added_at >= ` + strconv.FormatInt(spec.now-seconds, 10) + `)`
}

// The database mixes stored folder values with video and group keys before ordering and LIMIT.
func storedSmartFolderItemsSQL(spec listSpec) string {
	if !spec.includeSmartFolders {
		return ""
	}
	condition := `f.parent_id is null`
	if spec.favoriteOnly {
		condition = `exists (select 1 from smart_folder_favorites ff where ff.smart_folder_id = f.id)`
	}
	return ` union all select null, -f.id, null, null,
		v.added_at, v.mtime, v.created_at, f.title_key, v.duration_ms, v.size_bytes, v.played_at,
		(select ff.favorited_at from smart_folder_favorites ff where ff.smart_folder_id = f.id), null
		from smart_folder_values v join smart_folders f on f.id = v.folder_id
		where v.audience = ` + strconv.Itoa(audienceValue(spec.scope.audience)) + ` and v.visible = 1 and ` + condition
}
