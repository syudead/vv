package main

import (
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/watcher"
)

// newAutoImport は自動の取り込みを組み立て、fsnotify の watcher とつなぐ。watcher の変更と
// 問題の報告を AutoImport へ渡し、走査が閉じる知らせも AutoImport へ渡す。internal/app は
// watcher を知らず、変更の型の変換はここだけが持つ
// （specs/042-folder-watch-import/plan.md の Constitution Check）。
func newAutoImport(
	store app.AutoImportStore, folders app.ActivityFolderStore, scans *app.Scans, logger *slog.Logger,
	tune ...func(*app.AutoImportOptions),
) *app.AutoImport {
	var auto *app.AutoImport
	watch := watcher.New(watcher.Handler{
		Change: func(c watcher.Change) {
			auto.Changed(domain.DirtyDirectory{Path: c.Dir, Recursive: c.Recursive})
		},
		Problem: func(p watcher.Problem) {
			logger.Warn("the folder watcher reports a problem",
				slog.String("kind", string(p.Kind)), slog.String("path", p.Path), slog.Any("error", p.Err))
			auto.WatchProblem(watchProblemOf(p))
		},
	})
	opts := app.AutoImportOptions{
		Store:   store,
		Folders: folders,
		Watcher: watch,
		Scans:   scans,
		Probe: func(dir domain.DirtyDirectory) (time.Time, error) {
			return watcher.NewestFileChange(dir.Path, dir.Recursive)
		},
		Logger: logger,
	}
	for _, f := range tune {
		f(&opts)
	}
	auto = app.NewAutoImport(opts)
	scans.OnStart(auto.ScanStarted)
	scans.OnFinish(auto.ScanFinished)
	return auto
}

// watchProblemOf は watcher の問題を、設定画面に見せる種類へ写す。
func watchProblemOf(p watcher.Problem) domain.FolderWatchProblem {
	kind := domain.FolderWatchProblemFolderUnreachable
	switch p.Kind {
	case watcher.ProblemOverflow:
		kind = domain.FolderWatchProblemEventsLost
	case watcher.ProblemWatchLimit:
		kind = domain.FolderWatchProblemLimit
	case watcher.ProblemPermission:
		kind = domain.FolderWatchProblemPermissionDenied
	case watcher.ProblemUnreachable:
		kind = domain.FolderWatchProblemFolderUnreachable
	}
	return domain.FolderWatchProblem{Kind: kind, Path: p.Path}
}
