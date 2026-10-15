package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// runScoped は dirs だけを読む走査を1回実行する。
func runScoped(t *testing.T, root string, index *fakeIndex, dirs ...domain.DirtyDirectory) domain.ScanResult {
	t.Helper()

	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	result, err := scanner.ScanScoped(context.Background(), dirs)
	if err != nil {
		t.Fatalf("範囲を絞った走査に失敗した: %v", err)
	}
	return result
}

func dirOf(root, name string, recursive bool) domain.DirtyDirectory {
	return domain.DirtyDirectory{Path: filepath.Join(root, name), Recursive: recursive}
}

func rowFor(t *testing.T, index *fakeIndex, path string) domain.IndexedVideo {
	t.Helper()
	row, ok := index.rows[path]
	if !ok {
		t.Fatalf("索引に %s が無い: %v", path, index.rows)
	}
	return row
}

// 1つのディレクトリの走査は、その外の所在を、ファイルが消えていても変えない。
func TestScanScopedLeavesLocationsOutsideTheScope(t *testing.T) {
	root := mediaTree(t, map[string]string{"a/1.mp4": "1", "b/2.mp4": "2", "c/3.mp4": "3"})
	index := newFakeIndex()
	runScan(t, root, index)
	for _, name := range []string{"b/2.mp4", "c/3.mp4"} {
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}

	result := runScoped(t, root, index, dirOf(root, "a", false))
	if result.Removed != 0 || len(index.deleted) != 0 {
		t.Fatalf("範囲の外を消した: %+v, deleted %v", result, index.deleted)
	}
	if len(index.rows) != 3 {
		t.Fatalf("行が %d 件になった, want 3", len(index.rows))
	}

	result = runScoped(t, root, index, dirOf(root, "b", false))
	if result.Removed != 1 {
		t.Fatalf("Removed = %d, want 1", result.Removed)
	}
	if _, ok := index.rows[filepath.Join(root, "b", "2.mp4")]; ok {
		t.Fatal("範囲の中の消えたファイルが残った")
	}
	rowFor(t, index, filepath.Join(root, "c", "3.mp4"))
}

// 範囲の中の新しいファイルは取り込み、変わらないファイルは読み直さない。
func TestScanScopedImportsOnlyTheScope(t *testing.T) {
	root := mediaTree(t, map[string]string{"a/1.mp4": "1", "b/2.mp4": "2"})
	index := newFakeIndex()
	runScan(t, root, index)
	index.upserts = nil
	for name, content := range map[string]string{"a/new.mp4": "new", "b/other.mp4": "other"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result := runScoped(t, root, index, dirOf(root, "a", false))
	if result.Added != 1 || result.Total != 1 {
		t.Fatalf("結果 = %+v, want 追加 1 件", result)
	}
	if got := index.upsertedPaths(); len(got) != 1 || got[0] != filepath.Join(root, "a", "new.mp4") {
		t.Fatalf("取り込んだもの = %v", got)
	}
}

// 再帰でない項目は直下だけを読む。再帰の項目はその下をすべて読む。
func TestScanScopedRecursiveFlag(t *testing.T) {
	root := mediaTree(t, map[string]string{"a/1.mp4": "1", "a/sub/2.mp4": "2"})
	index := newFakeIndex()

	runScoped(t, root, index, dirOf(root, "a", false))
	if got := index.upsertedPaths(); len(got) != 1 || got[0] != filepath.Join(root, "a", "1.mp4") {
		t.Fatalf("再帰でない項目が下へ入った: %v", got)
	}

	runScoped(t, root, index, dirOf(root, "a", true))
	rowFor(t, index, filepath.Join(root, "a", "sub", "2.mp4"))
}

// 1回の範囲の中で2つのディレクトリの間を移した動画は、同じ動画のまま所在が付け替わる。
// 追加を先に処理するので、移した先の取り込みが前の所在の消去より先に済む。
func TestScanScopedMoveBetweenDirtyDirectoriesKeepsVideo(t *testing.T) {
	root := mediaTree(t, map[string]string{"a/movie.mp4": "same content", "b/other.mp4": "other"})
	index := newFakeIndex()
	runScan(t, root, index)
	before := rowFor(t, index, filepath.Join(root, "a", "movie.mp4"))

	if err := os.Rename(filepath.Join(root, "a", "movie.mp4"), filepath.Join(root, "b", "movie.mp4")); err != nil {
		t.Fatal(err)
	}

	result := runScoped(t, root, index, dirOf(root, "a", false), dirOf(root, "b", false))
	if result.Moved != 1 || result.Removed != 0 || len(index.deleted) != 0 {
		t.Fatalf("結果 = %+v, deleted %v, want 移動 1 件で削除なし", result, index.deleted)
	}
	after := rowFor(t, index, filepath.Join(root, "b", "movie.mp4"))
	if after.ID != before.ID {
		t.Fatalf("動画の id が変わった: %d -> %d", before.ID, after.ID)
	}
}

// 消えたディレクトリの下は、再帰の項目なら所在を失い、再帰でない項目なら直下だけが失う。
func TestScanScopedRemovedSubtree(t *testing.T) {
	root := mediaTree(t, map[string]string{"d/1.mp4": "1", "d/sub/2.mp4": "2", "keep/3.mp4": "3"})
	index := newFakeIndex()
	runScan(t, root, index)
	if err := os.RemoveAll(filepath.Join(root, "d")); err != nil {
		t.Fatal(err)
	}

	result := runScoped(t, root, index, dirOf(root, "d", false))
	if result.Removed != 1 {
		t.Fatalf("再帰でない項目の Removed = %d, want 1", result.Removed)
	}
	rowFor(t, index, filepath.Join(root, "d", "sub", "2.mp4"))

	result = runScoped(t, root, index, dirOf(root, "d", true))
	if result.Removed != 1 {
		t.Fatalf("再帰の項目の Removed = %d, want 1", result.Removed)
	}
	if len(index.rows) != 1 {
		t.Fatalf("行が %d 件残った, want 1 (keep)", len(index.rows))
	}
	rowFor(t, index, filepath.Join(root, "keep", "3.mp4"))
}

// 取って代わられた走査は、足したものを残して何も消さず、ErrScanSuperseded を返す。
func TestScanScopedSupersededRemovesNothing(t *testing.T) {
	root := mediaTree(t, map[string]string{"a/gone.mp4": "gone"})
	index := newFakeIndex()
	runScan(t, root, index)
	if err := os.Remove(filepath.Join(root, "a", "gone.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "new.mp4"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	scanner.contentKey = func(path string) (string, error) {
		cancel(domain.ErrScanSuperseded)
		return ContentKey(path)
	}

	result, err := scanner.ScanScoped(ctx, []domain.DirtyDirectory{dirOf(root, "a", false)})
	if !errors.Is(err, domain.ErrScanSuperseded) {
		t.Fatalf("error = %v, want ErrScanSuperseded", err)
	}
	if result.Removed != 0 || len(index.deleted) != 0 {
		t.Fatalf("取って代わられたのに消した: %+v, deleted %v", result, index.deleted)
	}
	rowFor(t, index, filepath.Join(root, "a", "gone.mp4"))
}

// 全体の走査も、取って代わられれば何も消さずに止まる。
func TestScanSupersededRemovesNothing(t *testing.T) {
	root := mediaTree(t, map[string]string{"gone.mp4": "gone"})
	index := newFakeIndex()
	runScan(t, root, index)
	if err := os.Remove(filepath.Join(root, "gone.mp4")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(domain.ErrScanSuperseded)
	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	if _, err := scanner.Scan(ctx); !errors.Is(err, domain.ErrScanSuperseded) {
		t.Fatalf("error = %v, want ErrScanSuperseded", err)
	}
	if len(index.deleted) != 0 {
		t.Fatalf("消した: %v", index.deleted)
	}
}

// メディアフォルダが読めなければ、範囲の中の所在を消さずに失敗する。
func TestScanScopedFailsWhenMediaFolderIsUnreachable(t *testing.T) {
	root := mediaTree(t, map[string]string{"a/1.mp4": "1"})
	index := newFakeIndex()
	runScan(t, root, index)
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}

	scanner := New(Options{Index: index, Queue: index, Reporter: index})
	_, err := scanner.ScanScoped(context.Background(), []domain.DirtyDirectory{dirOf(root, "a", true)})
	code, _ := domain.ScanFailureOf(err)
	if code != domain.ScanErrorMediaFolderUnreadable {
		t.Fatalf("error = %v (%q), want media folder unreadable", err, code)
	}
	if len(index.deleted) != 0 {
		t.Fatalf("消した: %v", index.deleted)
	}
}

// 除外するディレクトリと、メディアフォルダの外のディレクトリは読まない。
func TestScanScopedSkipsExcludedAndForeignDirectories(t *testing.T) {
	root := mediaTree(t, map[string]string{"@eaDir/1.mp4": "1", ".hidden/2.mp4": "2", "ok/3.mp4": "3"})
	foreign := mediaTree(t, map[string]string{"4.mp4": "4"})
	index := newFakeIndex()

	runScoped(t, root, index,
		dirOf(root, "@eaDir", true), dirOf(root, ".hidden", true), dirOf(root, "ok", true),
		domain.DirtyDirectory{Path: foreign, Recursive: true})
	if got := index.upsertedPaths(); len(got) != 1 || got[0] != filepath.Join(root, "ok", "3.mp4") {
		t.Fatalf("取り込んだもの = %v", got)
	}
}

// メディアフォルダの祖先の項目は歩かないので、その下のメディアフォルダの所在を消さない。
func TestScanScopedAncestorOfMediaFolderRemovesNothing(t *testing.T) {
	parent := mediaTree(t, map[string]string{"library/a/1.mp4": "1"})
	root := filepath.Join(parent, "library")
	index := newFakeIndex()
	runScan(t, root, index)
	if err := os.Remove(filepath.Join(root, "a", "1.mp4")); err != nil {
		t.Fatal(err)
	}

	result := runScoped(t, root, index,
		domain.DirtyDirectory{Path: parent, Recursive: true},
		dirOf(root, "b", false))
	if result.Removed != 0 || len(index.deleted) != 0 {
		t.Fatalf("歩いていない所在を消した: %+v, deleted %v", result, index.deleted)
	}
	rowFor(t, index, filepath.Join(root, "a", "1.mp4"))
}

// scopedScanner は root だけを登録した、範囲を絞った走査の組み立てである。
func scopedScanner(root string, index *fakeIndex) *Scanner {
	index.folders = []domain.MediaFolder{{ID: 1, Path: root, Version: 1}}
	return New(Options{Index: index, Queue: index, Reporter: index})
}

// 監視の走査では、取り込みの途中で変わった・消えたファイルを失敗にせず、対象から外す。
// その変更の知らせが、ディレクトリを次の監視の走査へ回す（research.md R-3）。
func TestScanScopedSkipsFileThatChangesOrVanishesDuringImport(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(path string) error
	}{
		{name: "書き足された", change: func(path string) error {
			return os.WriteFile(path, []byte("copy still running"), 0o600)
		}},
		{name: "消えた", change: os.Remove},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mediaTree(t, map[string]string{"a/copying.mp4": "part", "a/done.mp4": "done"})
			copying := filepath.Join(root, "a", "copying.mp4")
			index := newFakeIndex()
			scanner := scopedScanner(root, index)
			scanner.contentKey = func(path string) (string, error) {
				if path == copying {
					if err := tc.change(path); err != nil {
						return "", err
					}
				}
				return ContentKey(path)
			}

			result, err := scanner.ScanScoped(context.Background(), []domain.DirtyDirectory{dirOf(root, "a", false)})
			if err != nil {
				t.Fatal(err)
			}
			if result.Failed != 0 || len(index.issues) != 0 {
				t.Fatalf("result = %+v, issues = %+v, want 失敗なし", result, index.issues)
			}
			if result.Total != 1 || result.Added != 1 || result.Completed() != 1 {
				t.Fatalf("result = %+v, want done.mp4 の 1 / 1", result)
			}
			if got := index.upsertedPaths(); len(got) != 1 || got[0] != filepath.Join(root, "a", "done.mp4") {
				t.Fatalf("取り込んだもの = %v", got)
			}
		})
	}
}

// vanishedEntry は列挙のあとに消えた項目である。
type vanishedEntry struct {
	fs.DirEntry
}

func (e vanishedEntry) Info() (fs.FileInfo, error) {
	return nil, &fs.PathError{Op: "lstat", Path: e.Name(), Err: fs.ErrNotExist}
}

// 監視の走査で列挙のあとにファイルの情報を読む前に消えたファイルは、失敗にしない。
func TestScanScopedSkipsFileThatVanishesBeforeItsInformationIsRead(t *testing.T) {
	root := mediaTree(t, map[string]string{"a/gone.mp4": "gone"})
	index := newFakeIndex()
	scanner := scopedScanner(root, index)
	gone := filepath.Join(root, "a", "gone.mp4")
	scanner.walkDir = func(dir string, fn fs.WalkDirFunc) error {
		return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if path == gone {
				return fn(path, vanishedEntry{DirEntry: entry}, err)
			}
			return fn(path, entry, err)
		})
	}

	result, err := scanner.ScanScoped(context.Background(), []domain.DirtyDirectory{dirOf(root, "a", false)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 0 || result.Failed != 0 || len(index.issues) != 0 {
		t.Fatalf("result = %+v, issues = %+v, want 対象なし", result, index.issues)
	}
}

// 監視の走査の途中でディレクトリごと移された・消されたときは、走査を失敗にせず、その所在を
// 残す。移動の知らせで次の監視の走査が、移した先を加えてから消す（research.md R-3）。
func TestScanScopedKeepsLocationsWhenParentVanishesDuringImport(t *testing.T) {
	root := mediaTree(t, map[string]string{"a/gone/1.mp4": "1", "a/z.mp4": "z"})
	index := newFakeIndex()
	runScan(t, root, index)
	gone := filepath.Join(root, "a", "gone", "1.mp4")
	before := rowFor(t, index, gone)

	z := filepath.Join(root, "a", "z.mp4")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(z, later, later); err != nil {
		t.Fatal(err)
	}
	scanner := scopedScanner(root, index)
	scanner.contentKey = func(path string) (string, error) {
		if path == z {
			if err := os.Rename(filepath.Join(root, "a", "gone"), filepath.Join(root, "moved")); err != nil {
				return "", err
			}
		}
		return ContentKey(path)
	}

	result, err := scanner.ScanScoped(context.Background(), []domain.DirtyDirectory{dirOf(root, "a", true)})
	if err != nil {
		t.Fatalf("ディレクトリが消えた監視の走査が失敗した: %v", err)
	}
	if result.Removed != 0 || len(index.deleted) != 0 {
		t.Fatalf("result = %+v, deleted %v, want 削除なし", result, index.deleted)
	}
	if after := rowFor(t, index, gone); after.ID != before.ID {
		t.Fatalf("動画の id が変わった: %d -> %d", before.ID, after.ID)
	}
}

// 監視の走査の歩みの途中でディレクトリが移されたときは、走査を失敗にせず、その下の所在を
// 残す。歩いた項目の情報を読む前に消えた場合（vanishedAt が項目）も、ディレクトリの中身を
// 読む前に消えた場合（vanishedAt がディレクトリ）も同じである。
func TestScanScopedKeepsLocationsWhenDirectoryVanishesWhileWalking(t *testing.T) {
	for _, tc := range []struct {
		name       string
		vanishedAt string
	}{
		{name: "項目の情報を読む前", vanishedAt: "a/gone/1.mp4"},
		{name: "ディレクトリの中身を読む前", vanishedAt: "a/gone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := mediaTree(t, map[string]string{"a/gone/1.mp4": "1", "a/z.mp4": "z"})
			index := newFakeIndex()
			runScan(t, root, index)
			gone := filepath.Join(root, "a", "gone", "1.mp4")
			before := rowFor(t, index, gone)

			scanner := scopedScanner(root, index)
			at := filepath.Join(root, filepath.FromSlash(tc.vanishedAt))
			scanner.walkDir = func(dir string, fn fs.WalkDirFunc) error {
				return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
					if path == at && err == nil {
						if err := os.Rename(filepath.Join(root, "a", "gone"), filepath.Join(root, "moved")); err != nil {
							return err
						}
					}
					return fn(path, entry, err)
				})
			}

			result, err := scanner.ScanScoped(context.Background(), []domain.DirtyDirectory{dirOf(root, "a", true)})
			if err != nil {
				t.Fatalf("ディレクトリが消えた監視の走査が失敗した: %v", err)
			}
			if result.Removed != 0 || result.Failed != 0 || len(index.deleted) != 0 || len(index.issues) != 0 {
				t.Fatalf("result = %+v, deleted %v, issues %v, want 削除も失敗もなし", result, index.deleted, index.issues)
			}
			if after := rowFor(t, index, gone); after.ID != before.ID {
				t.Fatalf("動画の id が変わった: %d -> %d", before.ID, after.ID)
			}
		})
	}
}
