package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/scanner"
	"github.com/syudead/vv/internal/watcher"
)

// 本物の watcher・走査・保存層をつないで、メディアフォルダへ置いたファイルが走査を始めずに
// 索引へ入り、消すと索引から消えることを確かめる
// （specs/042-folder-watch-import/quickstart.md の最初と 3 つ目の行）。
func TestAutoImportImportsAndRemovesFilesWithoutManualScan(t *testing.T) {
	ctx, db, mediaDir := openTestDB(t)
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
	auto := newAutoImport(db.Settings(), db.ScanIndex(), scans, logger, func(o *app.AutoImportOptions) {
		o.Quiet = 50 * time.Millisecond
		o.Settle = 100 * time.Millisecond
	})
	if err := auto.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		auto.Stop()
		scans.Wait()
	})
	waitFor(t, "監視が張られる", func() bool { return auto.Status().State == domain.FolderWatchActive })

	path := filepath.Join(mediaDir, "new", "movie.mp4")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("movie content"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "ファイルが索引に入る", func() bool {
		indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
		return err == nil && indexed[path].ID != 0
	})
	current, err := db.Scans().CurrentScan(ctx)
	if err != nil || current.Origin != domain.ScanOriginWatch {
		t.Fatalf("走査 = %+v, err %v; 監視の走査のはず", current, err)
	}

	scans.Wait()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "ファイルが索引から消える", func() bool {
		indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
		return err == nil && indexed[path].ID == 0
	})
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("待ち切れなかった: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWatchProblemOf(t *testing.T) {
	for kind, want := range map[watcher.ProblemKind]domain.FolderWatchProblemKind{
		watcher.ProblemOverflow:    domain.FolderWatchProblemEventsLost,
		watcher.ProblemWatchLimit:  domain.FolderWatchProblemLimit,
		watcher.ProblemPermission:  domain.FolderWatchProblemPermissionDenied,
		watcher.ProblemUnreachable: domain.FolderWatchProblemFolderUnreachable,
	} {
		if got := watchProblemOf(watcher.Problem{Kind: kind, Path: "/media"}); got.Kind != want || got.Path != "/media" {
			t.Errorf("%s -> %+v, want %s", kind, got, want)
		}
	}
}
