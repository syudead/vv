package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/syudead/vv/internal/domain"
)

func audienceValue(audience domain.Audience) int {
	if audience.IsOwner() {
		return 1
	}
	return 0
}

// Writes refresh collection information before commit. Startup, expired age rules and writes
// made outside the store use this recovery path; unchanged pages perform only the pending check.
func (db *DB) RefreshCollectionValues(ctx context.Context) error {
	var pending bool
	now := listNow().Unix()
	if err := db.read.QueryRowContext(ctx, `select exists (select 1 from collection_changes)
		or exists (select 1 from smart_folder_values where expires_at <= ?)`, now).Scan(&pending); err != nil {
		return fmt.Errorf("cannot read collection changes: %w", err)
	}
	if !pending {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := refreshCollectionValues(ctx, tx, now); err != nil {
		return err
	}
	return tx.Commit()
}

func refreshCollectionValues(ctx context.Context, tx *sql.Tx, now int64) error {
	if _, err := tx.ExecContext(ctx, `insert into collection_changes (kind, id)
		select 'smart', folder_id from smart_folder_values where expires_at <= ?`, now); err != nil {
		return err
	}
	for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
		if err := refreshGroupValues(ctx, tx, audience); err != nil {
			return err
		}
		if err := refreshSmartFolderValues(ctx, tx, audience, now); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `delete from collection_changes`)
	return err
}

func changedCollections(kind, table string) string {
	return `select id from ` + table + ` where exists (select 1 from collection_changes
		where kind = '` + kind + `' and id = 0) or id in (select id from collection_changes where kind = '` + kind + `')`
}

func refreshGroupValues(ctx context.Context, tx *sql.Tx, audience domain.Audience) error {
	var pending bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from collection_changes where kind = 'group')`).Scan(&pending); err != nil || !pending {
		return err
	}
	roots := changedCollections("group", "groups")
	a := strconv.Itoa(audienceValue(audience))
	statements := []string{
		`delete from group_members where audience = ` + a + ` and group_id in (` + roots + `)`,
		`insert into group_members (group_id, audience, video_id, position) with ` + groupDescendantsCTE(roots, audience) +
			` select root_id, ` + a + `, video_id, sort_key from gdesc`,
		`delete from group_values where audience = ` + a + ` and group_id in (` + roots + `)`,
		`insert into group_values ` + groupValuesSelect(roots, audience),
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("cannot update group information: %w", err)
		}
	}
	return nil
}

// Aggregate narrow member rows in SQLite; no member details cross into Go.
func groupValuesSelect(roots string, audience domain.Audience) string {
	a := strconv.Itoa(audienceValue(audience))
	progress, join := `0 as completed, 0 as position_ms, null as played_at`, ""
	if audience.IsOwner() {
		progress = `coalesce(p.completed, 0) as completed, coalesce(p.position_ms, 0) as position_ms, p.updated_at as played_at`
		join = ` left join playback_progress p on p.content_key = ` + userKeyExpr("videos") + ` and videos.content_key <> ''`
	}
	return `with m as materialized (
		select gm.group_id, gm.video_id, gm.position, videos.added_at, videos.duration_ms,
			rl.mtime, ` + fileCreatedAtExpr("rl") + ` as created_at, rl.size_bytes,
			` + playableCondition + ` as playable, ` + progress + `
		from group_members gm join videos on videos.id = gm.video_id` + representativeLocationJoin("gm", audience) + join + `
		where gm.audience = ` + a + ` and gm.group_id in (` + roots + `))
		select g.id, ` + a + `, count(m.video_id), max(m.added_at), max(m.mtime), max(m.created_at), max(m.played_at),
			sum(m.duration_ms), coalesce(sum(m.size_bytes), 0), coalesce(sum(m.completed = 1), 0),
			coalesce(sum(m.completed = 1 or m.position_ms > 0), 0), coalesce(max(m.playable), 0),
			(select video_id from m chosen where chosen.group_id = g.id
			 order by case when completed = 1 then 2 when position_ms > 0 then 0 else 1 end, position limit 1)
		from groups g left join m on m.group_id = g.id where g.id in (` + roots + `) group by g.id`
}
