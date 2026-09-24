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

// queryExecer は *sql.DB と *sql.Tx の両方で行を読み書きするための共通部分である。
type queryExecer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// locationSearchKey は所在1件の search_key を作る（data-model.md §3）。
//
// 相対パスは、所在を含む登録メディアフォルダより下のパスで、拡張子を含む。
// 登録フォルダ自身のパスは入れない。どの登録フォルダにも含まれない所在は
// 空文字列を返す。登録フォルダは互いに入れ子にならない（ensureFolderPlacementAllowed）
// ので、含むフォルダは高々1つである。
func locationSearchKey(roots []string, path, title string) string {
	for _, root := range roots {
		rel, ok := registeredRelativePath(root, path)
		if !ok {
			continue
		}
		return searchKeyPart(title) + "\n" + searchKeyPart(rel)
	}
	return ""
}

// searchKeyPart は search_key の片側（題名か相対パス）を照合形にする。
// 題名や相対パスの中の改行は空白にそろえる。search_key は2つを改行でつなぐので、
// 改行を境目のためだけに残し、検索語の側も改行を空白として扱う（search.go）。
// 改行を含む題名も、改行を空白に読み替えた語で見つかる。
func searchKeyPart(s string) string {
	return strings.ReplaceAll(domain.FoldForMatch(s), "\n", " ")
}

// registeredRelativePath は、path が root の下にあるかを一覧の判定
// （registeredLocationCondition）と同じ規則（registrationSeparators）で調べ、
// 下にあれば root より下の相対パスを返す。一覧に出る所在が鍵を持たずに検索で
// 見つからない、というずれを作らないためである。Windows では大文字小文字を
// 区別しない。
//
// 相対パスの区切りは `/` にそろえる。Windows では小文字にそろえた綴りから
// 取るが、鍵には FoldForMatch を掛けるので照合の結果は変わらない。
func registeredRelativePath(root, path string) (string, bool) {
	p, r := path, root
	if runtime.GOOS == "windows" {
		// SQL の lower() と同じく ASCII だけを小文字にする（registeredLocationCondition）。
		p, r = domain.LowerASCII(p), domain.LowerASCII(r)
	}
	if p == r {
		return "", true
	}
	separators := registrationSeparators()
	trimmed := strings.TrimRight(r, string(separators))
	for _, separator := range separators {
		if rest, ok := strings.CutPrefix(p, trimmed+string(separator)); ok {
			return filepath.ToSlash(rest), true
		}
	}
	return "", false
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
		if _, ok := registeredRelativePath(root, target.path); ok {
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
