package store

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Writes only mark collections; list and group reads, and startup, refresh the marks here. Unchanged
// pages perform only the pending check.
func (db *DB) RefreshCollectionValues(ctx context.Context) error {
	pending, err := db.collectionChangesPending(ctx)
	if err != nil || !pending {
		return err
	}
	db.refreshMu.Lock()
	defer db.refreshMu.Unlock()
	// A reader that waited finds the marks cleared by the one before it.
	if pending, err := db.collectionChangesPending(ctx); err != nil || !pending {
		return err
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := refreshCollectionValues(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) collectionChangesPending(ctx context.Context) (bool, error) {
	var pending bool
	if err := db.read.QueryRowContext(ctx, `select exists (select 1 from collection_changes)`).Scan(&pending); err != nil {
		return false, fmt.Errorf("cannot read collection changes: %w", err)
	}
	return pending, nil
}

func refreshCollectionValues(ctx context.Context, tx *sql.Tx) error {
	ids, err := queryInt64s(ctx, tx, changedGroupsSQL)
	if err != nil {
		return fmt.Errorf("cannot read the changed groups: %w", err)
	}
	if len(ids) > 0 {
		encoded, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		// Integer IDs are safe to inline, which keeps the statements free of repeated arguments.
		roots := `select value as id from json_each('` + string(encoded) + `')`
		for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
			if err := refreshGroupValues(ctx, tx, audience, roots); err != nil {
				return err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `delete from collection_changes`)
	return err
}

// changedGroupsSQL selects the marked groups and their ancestors, whose members include a marked group's videos;
// mark 0 selects every group.
const changedGroupsSQL = `with recursive marked(id) as (
		select id from groups where exists (select 1 from collection_changes where kind = 'group' and id = 0)
		union
		select id from collection_changes where kind = 'group' and id <> 0
		union
		select gi.group_id from group_items gi join marked on gi.child_id = marked.id)
	select g.id from groups g join marked on marked.id = g.id`

func refreshGroupValues(ctx context.Context, tx *sql.Tx, audience domain.Audience, roots string) error {
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
			` + progress + `
		from group_members gm join videos on videos.id = gm.video_id` + representativeLocationJoin("gm", audience) + join + `
		where gm.audience = ` + a + ` and gm.group_id in (` + roots + `))
		select g.id, ` + a + `, count(m.video_id), max(m.added_at), max(m.mtime), max(m.created_at), max(m.played_at),
			sum(m.duration_ms), coalesce(sum(m.size_bytes), 0), coalesce(sum(m.completed = 1), 0),
			coalesce(sum(m.completed = 1 or m.position_ms > 0), 0),
			(select video_id from m chosen where chosen.group_id = g.id
			 order by case when completed = 1 then 2 when position_ms > 0 then 0 else 1 end, position limit 1)
		from groups g left join m on m.group_id = g.id where g.id in (` + roots + `) group by g.id`
}
