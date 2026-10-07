package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/scanner"
)

// 変わったディレクトリだけを読む監視の走査を、本物の保存層と走査でつなぐ
// （specs/042-folder-watch-import/data-model.md の Rules）。

// 1回の走査の範囲の中で、2つのディレクトリの間を移した動画は、id・タグ・再生位置を保つ。
// 範囲の外の所在は、ファイルが消えていても変わらない。
func TestWatchScanKeepsVideoMovedBetweenDirtyDirectories(t *testing.T) {
	ctx, db, mediaDir := openTestDB(t)
	for name, content := range map[string]string{
		"a/movie.mp4": "movie content", "b/other.mp4": "other content", "c/outside.mp4": "outside content",
	} {
		path := filepath.Join(mediaDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.DiscardHandler)
	scans := app.NewScans(app.ScansOptions{
		Store: db.Scans(), Jobs: db.Ingest(), FolderIndex: db.ScanIndex(),
		NewScanner: func(reporter app.ScanReporter) app.Scanner {
			return scanner.New(scanner.Options{
				Index: db.ScanIndex(), Queue: db.Ingest(), Reporter: reporter, Logger: logger,
			})
		},
		Logger: logger,
	})
	if _, _, err := scans.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	scans.Wait()

	moviePath := filepath.Join(mediaDir, "a", "movie.mp4")
	before, err := db.ScanIndex().IndexedVideosByPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	movie := before[moviePath]
	if movie.ID == 0 || len(before) != 3 {
		t.Fatalf("準備: 索引 = %+v", before)
	}
	if _, _, err := db.Tags().AttachTagByName(ctx, []int64{movie.ID}, "好き"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, movie.ContentKey,
		domain.Progress{PositionMs: 40_000, DurationMs: 100_000}); err != nil {
		t.Fatal(err)
	}

	// a から b へ移し、範囲の外 c のファイルは消す。
	if err := os.Rename(moviePath, filepath.Join(mediaDir, "b", "movie.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(mediaDir, "c", "outside.mp4")); err != nil {
		t.Fatal(err)
	}

	scan, started, err := scans.StartWatchScan(ctx, []domain.DirtyDirectory{
		{Path: filepath.Join(mediaDir, "a")}, {Path: filepath.Join(mediaDir, "b")},
	})
	if err != nil || !started || scan.Origin != domain.ScanOriginWatch {
		t.Fatalf("StartWatchScan = %+v started %v err %v", scan, started, err)
	}
	scans.Wait()

	current, err := db.Scans().CurrentScan(ctx)
	if err != nil || current.State != domain.ScanDone || current.Origin != domain.ScanOriginWatch {
		t.Fatalf("走査 = %+v, err %v, want done の watch", current, err)
	}
	after, err := db.ScanIndex().IndexedVideosByPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	moved := after[filepath.Join(mediaDir, "b", "movie.mp4")]
	if moved.ID != movie.ID {
		t.Fatalf("動画の id が変わった: %d -> %d", movie.ID, moved.ID)
	}
	if _, ok := after[filepath.Join(mediaDir, "c", "outside.mp4")]; !ok {
		t.Fatal("範囲の外の所在が消えた")
	}
	if _, ok := after[moviePath]; ok {
		t.Fatal("移す前の所在が残った")
	}
	tags, err := db.Tags().TagsByContentKeys(ctx, []string{movie.ContentKey})
	if err != nil {
		t.Fatal(err)
	}
	if got := tags[movie.ContentKey]; len(got) != 1 || got[0].Name != "好き" {
		t.Fatalf("タグ = %+v", tags)
	}
	progress, err := db.Playback().Progress(ctx, movie.ContentKey)
	if err != nil || progress.PositionMs != 40_000 {
		t.Fatalf("再生位置 = %+v, err %v", progress, err)
	}
	if paths := slices.Sorted(func(yield func(string) bool) {
		for path := range after {
			if !yield(filepath.Base(path)) {
				return
			}
		}
	}); !slices.Equal(paths, []string{"movie.mp4", "other.mp4", "outside.mp4"}) {
		t.Fatalf("索引 = %v", paths)
	}
}
