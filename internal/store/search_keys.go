package store

import (
	"context"
	"database/sql"
	"encoding/json"
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
//
// 表示名があれば3つ目の部分として足す（specs/029-video-overrides/data-model.md §4）。
// 表示名の無い所在の鍵は足す前と同じ値なので、SearchKeyVersion は上げない。
func locationSearchKey(roots []string, path, title, displayName string) string {
	for _, root := range roots {
		rel, ok := registeredRelativePath(root, path)
		if !ok {
			continue
		}
		key := searchKeyPart(title) + "\n" + searchKeyPart(rel)
		if displayName != "" {
			key += "\n" + searchKeyPart(displayName)
		}
		return key
	}
	return ""
}

// searchKeyPart は search_key の部分（題名・相対パス・表示名）を照合形にする。
// 各部分の中の改行は空白にそろえる。search_key は部分を改行でつなぐので、
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
	folders, err := listMediaFolders(ctx, q)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(folders))
	for _, folder := range folders {
		roots = append(roots, folder.Path)
	}
	return roots, nil
}

// searchKeyTarget は鍵を作り直す所在1件である。displayName はその所在の動画の
// 表示名（video_overrides）で、未設定は空である。
type searchKeyTarget struct {
	id                       int64
	path, title, displayName string
}

// searchKeyTargetsSelect は所在と、その動画の表示名を読む選択句である。where 句は
// 呼び出し側が足す。所在の別名は l で、表示名は内容の識別子が空でない動画だけに結ぶ
// （specs/029-video-overrides/data-model.md §4）。
const searchKeyTargetsSelect = `select l.id, l.path, l.title, coalesce(ov.display_name, '')
	from video_locations l join videos v on v.id = l.video_id
	left join video_overrides ov on ov.content_key = v.content_key and v.content_key <> ''`

// sortTitle は所在の title_key の元にする題名で、表示名があればそれである。
func (t searchKeyTarget) sortTitle() string {
	if t.displayName != "" {
		return t.displayName
	}
	return t.title
}

// writeSearchKeys は所在ごとに search_key・title_key を作り、現在の版を書く。
func writeSearchKeys(ctx context.Context, q queryExecer, roots []string, targets []searchKeyTarget) error {
	for _, target := range targets {
		if _, err := q.ExecContext(ctx,
			`update video_locations set search_key = ?, title_key = ?, search_version = ? where id = ?`,
			locationSearchKey(roots, target.path, target.title, target.displayName),
			domain.NaturalSortKey(target.sortTitle()),
			domain.SearchKeyVersion, target.id,
		); err != nil {
			return fmt.Errorf("cannot save the search key (%s): %w", target.path, err)
		}
	}
	return nil
}

// refreshSearchKeysByPath は path の所在の鍵を作り直す。取り込み（UpsertVideo）で
// 所在を追加・更新したときに、同じトランザクションの中で呼ぶ。
func refreshSearchKeysByPath(ctx context.Context, q queryExecer, path string) error {
	targets, err := searchKeyTargets(ctx, q, searchKeyTargetsSelect+` where l.path = ?`, path)
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
	all, err := searchKeyTargets(ctx, q, searchKeyTargetsSelect)
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

// refreshSearchKeysForContentKeys は内容の識別子が keys のいずれかである動画の、すべての
// 所在の鍵を作り直す。表示名を書き換える取引の中で呼ぶ（specs/029-video-overrides/data-model.md §4）。
func refreshSearchKeysForContentKeys(ctx context.Context, q queryExecer, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	encoded, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("cannot build content keys: %w", err)
	}
	targets, err := searchKeyTargets(ctx, q, searchKeyTargetsSelect+
		` where v.content_key in (select value from json_each(?)) and v.content_key <> ''`, string(encoded))
	if err != nil {
		return err
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
		return nil, fmt.Errorf("cannot read locations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var targets []searchKeyTarget
	for rows.Next() {
		var target searchKeyTarget
		if err := rows.Scan(&target.id, &target.path, &target.title, &target.displayName); err != nil {
			return nil, fmt.Errorf("cannot read locations: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot read locations: %w", err)
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
func (s *LibraryStore) RefreshSearchKeys(ctx context.Context) (int, error) {
	refreshed := 0
	for {
		count, err := s.refreshSearchKeyBatch(ctx)
		if err != nil {
			return refreshed, err
		}
		refreshed += count
		if count < searchKeyBatchSize {
			return refreshed, nil
		}
	}
}

func (s *LibraryStore) refreshSearchKeyBatch(ctx context.Context) (int, error) {
	s.db.folderMu.Lock()
	defer s.db.folderMu.Unlock()
	tx, err := s.db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	targets, err := searchKeyTargets(ctx, tx,
		searchKeyTargetsSelect+` where l.search_version < ? order by l.id limit ?`,
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
		return 0, fmt.Errorf("cannot save search keys: %w", err)
	}
	return len(targets), nil
}
