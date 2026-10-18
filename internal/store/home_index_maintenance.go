package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// homeIndexMaintainer は維持を 1 つずつ行う。
type homeIndexMaintainer struct {
	mu sync.Mutex
}

// maintainHomeIndexes は欠けた根の値と印の付いた動画の一致を埋め直す。
func (db *DB) maintainHomeIndexes(ctx context.Context) error {
	db.homeIndexes.mu.Lock()
	defer db.homeIndexes.mu.Unlock()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start maintaining the home indexes: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	dirty, err := groupClosureDirty(ctx, tx)
	if err != nil {
		return err
	}
	if dirty {
		if err := rebuildGroupClosure(ctx, tx); err != nil {
			return err
		}
	}
	for _, audience := range []domain.Audience{domain.AudienceOwner, domain.AudienceGuest} {
		if err := refillGroupRootValues(ctx, tx, audience); err != nil {
			return err
		}
	}
	if err := refillSearchHits(ctx, tx); err != nil {
		return err
	}
	if err := refillWatchStates(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot finish maintaining the home indexes: %w", err)
	}
	return nil
}

// refillSearchHits は、検索に関わる書き込みがあれば（search_dirty）、フォルダの棚の検索式の段ごとに当たる
// 動画の id を全文索引から読み直して home_search_hits に入れる。所有者の範囲で読み、ゲストには読み出しが
// 見せる動画の条件を重ねる。
func refillSearchHits(ctx context.Context, tx *sql.Tx) error {
	pending, err := searchHitsPending(ctx, tx)
	if err != nil || !pending {
		return err
	}
	shelves, err := homeShelfSpecs(ctx, tx, domain.AudienceOwner, listNow().Unix())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from home_search_hits`); err != nil {
		return fmt.Errorf("cannot clear the search hits: %w", err)
	}
	done := map[string]bool{}
	for _, shelf := range shelves {
		if !shelf.hasSearch() {
			continue
		}
		identity := searchIdentity(shelf)
		if done[identity] {
			continue
		}
		done[identity] = true
		query, args := searchVideosSelect(shelf.scope, shelf.searchExprs())
		if _, err := tx.ExecContext(ctx, `insert or ignore into home_search_hits (search, video_id) select ?, video_id from (`+query+`)`,
			append([]any{identity}, args...)...); err != nil {
			return fmt.Errorf("cannot refill the search hits: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `update home_shelved_state set search_dirty = 0`); err != nil {
		return fmt.Errorf("cannot finish the search hits: %w", err)
	}
	return nil
}

// searchHitsPending は、検索の当たりを求め直していないかを返す。
func searchHitsPending(ctx context.Context, q queryRower) (bool, error) {
	var pending bool
	if err := q.QueryRowContext(ctx, `select search_dirty = 1 from home_shelved_state where id = 1`).Scan(&pending); err != nil {
		return false, fmt.Errorf("cannot read the search state: %w", err)
	}
	return pending, nil
}

// refillWatchStates updates watch states for marked content keys, including bundle aliases.
func refillWatchStates(ctx context.Context, tx *sql.Tx) error {
	var all bool
	if err := tx.QueryRowContext(ctx, `select all_dirty from home_shelved_state where id = 1`).Scan(&all); err != nil {
		return fmt.Errorf("cannot read the shelved state: %w", err)
	}
	var some bool
	if err := tx.QueryRowContext(ctx, `select exists (select 1 from home_shelved_dirty)`).Scan(&some); err != nil {
		return fmt.Errorf("cannot read the shelved marks: %w", err)
	}
	if !all && !some {
		return nil
	}
	// 印は動画の内容の鍵か集まりの鍵なので、どちらで付いても集まりの全部を求め直す。鍵の表は索引で引く。
	const dirtyKeys = `with dirty_keys(content_key) as (select content_key from home_shelved_dirty
		union select b.user_key from video_bundle_members m join video_bundles b on b.id = m.bundle_id
			where m.content_key in (select content_key from home_shelved_dirty)
		union select m.content_key from video_bundle_members m join video_bundles b on b.id = m.bundle_id
			where b.user_key in (select content_key from home_shelved_dirty))`
	userKey := userKeyExpr("videos")
	scope, marked := ``, ``
	if !all {
		scope = ` and videos.content_key in (select content_key from dirty_keys)`
		marked = ` where content_key in (select content_key from dirty_keys)`
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{dirtyKeys + ` delete from video_watch_states` + marked, nil},
		{dirtyKeys + ` insert or replace into video_watch_states (content_key, state) select distinct ` + userKey + `, ` + videoWatchStateSQL(userKey) + `
			from videos where videos.content_key <> ''` + scope, nil},
	}
	statements = append(statements, struct {
		sql  string
		args []any
	}{`delete from home_shelved_dirty`, nil}, struct {
		sql  string
		args []any
	}{`update home_shelved_state set all_dirty = 0`, nil})
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			return fmt.Errorf("cannot refill watch states: %w", err)
		}
	}
	return nil
}

// watchMarksPending reports pending changes to the watch-state index.
func watchMarksPending(ctx context.Context, q queryRower) (bool, error) {
	var pending bool
	if err := q.QueryRowContext(ctx, `select (select all_dirty from home_shelved_state where id = 1) = 1
		or exists (select 1 from home_shelved_dirty)`).Scan(&pending); err != nil {
		return false, fmt.Errorf("cannot read the shelved marks: %w", err)
	}
	return pending, nil
}
