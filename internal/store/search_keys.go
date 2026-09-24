package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// searchKeyBatchSize は起動時の埋め直しで1つのトランザクションに書く所在の数である。
// 版は行ごとに書くので、途中で止まっても次の起動で続きから埋まる。
const searchKeyBatchSize = 500

// queryExecer は *sql.DB と *sql.Tx の両方で鍵を読み書きするための共通部分である。
type queryExecer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// locationSearchKey は所在1件の search_key を作る（data-model.md §3）。
//
// 相対パスは、所在を含む登録メディアフォルダより下の `/` 区切りのパスで、拡張子を
// 含む。登録フォルダ自身のパスは入れない。どの登録フォルダにも含まれない所在は
// 空文字列を返す。登録フォルダは互いに入れ子にならない（ensureFolderMutationAllowed）
// ので、含むフォルダは高々1つである。
func locationSearchKey(roots []string, path, title string) string {
	for _, root := range roots {
		if !domain.PathWithinRoot(root, path) {
			continue
		}
		return domain.FoldForMatch(title) + "\n" + domain.FoldForMatch(relativeLocationPath(root, path))
	}
	return ""
}

// relativeLocationPath は root の下にある path の相対パスを `/` 区切りで返す。
// 呼び出し側が domain.PathWithinRoot で含まれることを確かめてから呼ぶ。
//
// Windows の filepath.Rel は段を strings.EqualFold で比べるので、綴りの大小が
// 違う登録でもふつうは1回目で取れる。strings.ToLower と EqualFold の結果が
// 食い違う文字（U+0130 など）のときだけ、小文字にそろえて取り直す。鍵には
// FoldForMatch を掛けるので、小文字にそろえても照合の結果は変わらない。
func relativeLocationPath(root, path string) string {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if (err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) && runtime.GOOS == "windows" {
		rel, err = filepath.Rel(strings.ToLower(cleanRoot), strings.ToLower(cleanPath))
	}
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

// mediaFolderRoots は登録メディアフォルダのパスをすべて返す。
func mediaFolderRoots(ctx context.Context, q queryExecer) ([]string, error) {
	rows, err := q.QueryContext(ctx, `select path from media_folders order by id`)
	if err != nil {
		return nil, fmt.Errorf("メディアフォルダを読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var roots []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			return nil, fmt.Errorf("メディアフォルダを読み出せません: %w", err)
		}
		roots = append(roots, root)
	}
	return roots, rows.Err()
}

// searchKeyTarget は鍵を作り直す所在1件である。
type searchKeyTarget struct {
	id          int64
	path, title string
}

// writeSearchKeys は所在ごとに search_key・title_key を作り、現在の版を書く。
func writeSearchKeys(ctx context.Context, q queryExecer, roots []string, targets []searchKeyTarget) error {
	for _, target := range targets {
		if _, err := q.ExecContext(ctx,
			`update video_locations set search_key = ?, title_key = ?, search_version = ? where id = ?`,
			locationSearchKey(roots, target.path, target.title), domain.NaturalSortKey(target.title),
			domain.SearchKeyVersion, target.id,
		); err != nil {
			return fmt.Errorf("照合用の鍵を保存できません (%s): %w", target.path, err)
		}
	}
	return nil
}

// refreshSearchKeysByPath は path の所在の鍵を作り直す。取り込み（UpsertVideo）で
// 所在を追加・更新したときに、同じトランザクションの中で呼ぶ。
func refreshSearchKeysByPath(ctx context.Context, q queryExecer, path string) error {
	targets, err := searchKeyTargets(ctx, q, `select id, path, title from video_locations where path = ?`, path)
	if err != nil {
		return err
	}
	roots, err := mediaFolderRoots(ctx, q)
	if err != nil {
		return err
	}
	return writeSearchKeys(ctx, q, roots, targets)
}

// refreshSearchKeysUnder は root の下にある所在の鍵を作り直す。メディアフォルダの
// 追加・変更と同じトランザクションの中で呼ぶ。
func refreshSearchKeysUnder(ctx context.Context, q queryExecer, root string) error {
	all, err := searchKeyTargets(ctx, q, `select id, path, title from video_locations`)
	if err != nil {
		return err
	}
	var targets []searchKeyTarget
	for _, target := range all {
		if domain.PathWithinRoot(root, target.path) {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	roots, err := mediaFolderRoots(ctx, q)
	if err != nil {
		return err
	}
	return writeSearchKeys(ctx, q, roots, targets)
}

func searchKeyTargets(ctx context.Context, q queryExecer, query string, args ...any) ([]searchKeyTarget, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("所在を読み出せません: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var targets []searchKeyTarget
	for rows.Next() {
		var target searchKeyTarget
		if err := rows.Scan(&target.id, &target.path, &target.title); err != nil {
			return nil, fmt.Errorf("所在を読み出せません: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("所在を読み出せません: %w", err)
	}
	return targets, nil
}

// RefreshSearchKeys は search_version が現在の版より小さい所在の鍵をすべて
// 作り直し、作り直した件数を返す（data-model.md §5）。
//
// 起動時、Migrate の直後で、中断したジョブの戻し・ジョブワーカーの起動・HTTP の
// 受け付けより前に呼ぶ。searchKeyBatchSize 件ずつのトランザクションで書き、
// 版は行ごとに書くので、途中で失敗しても次の呼び出しで続きから埋まる。失敗を
// 返したら、呼び出し側は起動を止める（古い鍵のまま検索を出さない）。
func (db *DB) RefreshSearchKeys(ctx context.Context) (int, error) {
	refreshed := 0
	for {
		count, err := db.refreshSearchKeyBatch(ctx)
		if err != nil {
			return refreshed, err
		}
		refreshed += count
		if count < searchKeyBatchSize {
			return refreshed, nil
		}
	}
}

func (db *DB) refreshSearchKeyBatch(ctx context.Context) (int, error) {
	db.folderMu.Lock()
	defer db.folderMu.Unlock()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	targets, err := searchKeyTargets(ctx, tx,
		`select id, path, title from video_locations where search_version < ? order by id limit ?`,
		domain.SearchKeyVersion, searchKeyBatchSize)
	if err != nil {
		return 0, err
	}
	if len(targets) == 0 {
		return 0, nil
	}
	roots, err := mediaFolderRoots(ctx, tx)
	if err != nil {
		return 0, err
	}
	if err := writeSearchKeys(ctx, tx, roots, targets); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("照合用の鍵を保存できません: %w", err)
	}
	return len(targets), nil
}
