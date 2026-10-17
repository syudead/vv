package store

// ホームの読み出しが引く表の維持（specs/057-home/data-model.md「Group closure」、research.md R-12）。
// 書き込みの引き金が消した根の値（group_root_values）と印を付けた動画の棚への一致（home_shelved_videos）
// を、書き込みの確定のあと（publishEvents）とマイグレーションの直後に埋め直す。グループの構造の表に印が
// あれば（作り直しを通らない書き手）作り直す。読み出しは書かない。

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
	if err := refillShelvedVideos(ctx, tx); err != nil {
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

// refillShelvedVideos は印の付いた動画（all_dirty ならすべて）について、どのフォルダの棚の実効の規則にも
// 動画として一致するか（shelvedVideos と同じ決め方）と、視聴状態（video_watch_states。ClassifyWatch と同じ
// 決め方）を求め直す。鍵は利用者データの鍵で、所有者に見せる動画が対象である。追加日からの日数の規則を持つ
// 棚は時刻で一致が変わるので表に入れず、読み出しが行ごとに掛ける（homeShelfReader.ruleOf）。
func refillShelvedVideos(ctx context.Context, tx *sql.Tx) error {
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
	reader, err := newHomeShelfReader(ctx, tx, domain.AudienceOwner, listNow().Unix())
	if err != nil {
		return err
	}
	shelves, err := homeShelfSpecs(ctx, tx, domain.AudienceOwner, reader.now)
	if err != nil {
		return err
	}
	var conditions []string
	var args []any
	for _, shelf := range shelves {
		if timeRelative(shelf) {
			continue
		}
		condition, shelfArgs, err := reader.perVideoCondition(shelf)
		if err != nil {
			return err
		}
		conditions = append(conditions, `(`+condition+`)`)
		args = append(args, shelfArgs...)
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
		{dirtyKeys + ` delete from home_shelved_videos` + marked, nil},
		{dirtyKeys + ` delete from video_watch_states` + marked, nil},
		{dirtyKeys + ` insert or replace into video_watch_states (content_key, state) select distinct ` + userKey + `, ` + videoWatchStateSQL(userKey) + `
			from videos where videos.content_key <> ''` + scope, nil},
	}
	if len(conditions) > 0 {
		statements = append(statements, struct {
			sql  string
			args []any
		}{dirtyKeys + ` insert or ignore into home_shelved_videos (content_key) select ` + userKey + ` from videos
			where videos.content_key <> '' and ` + reader.shownCondition() + scope + ` and (` + strings.Join(conditions, " or ") + `)`, args})
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
			return fmt.Errorf("cannot refill the shelved videos: %w", err)
		}
	}
	return nil
}

// timeRelative は棚 spec の規則に、時刻で一致が変わる段（追加日からの日数）があるかを返す。
func timeRelative(spec listSpec) bool {
	return spec.anyLevel(func(level listSpec) bool { return level.addedWithinDays > 0 })
}

// shelvedMarksPending は、棚への一致を求め直していない動画があるかを返す。読み出しは、あれば規則を行ごとに
// 掛ける（維持が済んでいれば表を引くだけでよい）。
func shelvedMarksPending(ctx context.Context, q queryRower) (bool, error) {
	var pending bool
	if err := q.QueryRowContext(ctx, `select (select all_dirty from home_shelved_state where id = 1) = 1
		or exists (select 1 from home_shelved_dirty)`).Scan(&pending); err != nil {
		return false, fmt.Errorf("cannot read the shelved marks: %w", err)
	}
	return pending, nil
}
