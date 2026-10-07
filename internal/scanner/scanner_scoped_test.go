package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
