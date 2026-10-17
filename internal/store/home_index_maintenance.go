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
	if err := refillShelvedVideos(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot finish maintaining the home indexes: %w", err)
	}
	return nil
}

// refillShelvedVideos は印の付いた動画（all_dirty ならすべて）について、どのフォルダの棚の実効の規則にも
// 動画として一致するかを求め直す（shelvedVideos と同じ決め方）。鍵は利用者データの鍵で、所有者に見せる
// 動画が対象である。
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
		condition, shelfArgs, err := reader.perVideoCondition(shelf)
		if err != nil {
			return err
		}
		conditions = append(conditions, `(`+condition+`)`)
		args = append(args, shelfArgs...)
	}
	userKey := userKeyExpr("videos")
	scope := ``
	if !all {
		scope = ` and ` + userKey + ` in (select content_key from home_shelved_dirty)`
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{`delete from home_shelved_videos` + strings.Replace(scope, ` and `, ` where `, 1), nil},
	}
	if !all {
		statements[0].sql = `delete from home_shelved_videos where content_key in (select content_key from home_shelved_dirty)`
	}
	if len(conditions) > 0 {
		statements = append(statements, struct {
			sql  string
			args []any
		}{`insert or ignore into home_shelved_videos (content_key) select ` + userKey + ` from videos
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
