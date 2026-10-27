package main

import (
	"log/slog"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/watcher"
)

// Adapt watcher events here so internal/app does not depend on the watcher. recheckRoots runs on a scan start, a
// change and an unreachable folder, the events after which an unavailable media folder may be readable again.
func newAutoImport(
	store app.AutoImportStore, folders app.ActivityFolderStore, scans *app.Scans, recheckRoots func(),
	logger *slog.Logger, tune ...func(*app.AutoImportOptions),
) *app.AutoImport {
	var auto *app.AutoImport
	watch := watcher.New(watcher.Handler{
		Change: func(c watcher.Change) {
			recheckRoots()
			auto.Changed(domain.DirtyDirectory{Path: c.Dir, Recursive: c.Recursive})
		},
		Problem: func(p watcher.Problem) {
			logger.Warn("the folder watcher reports a problem",
				slog.String("kind", string(p.Kind)), slog.String("path", p.Path), slog.Any("error", p.Err))
			problem := watchProblemOf(p)
			if problem.Kind == domain.FolderWatchProblemFolderUnreachable {
				recheckRoots()
			}
			auto.WatchProblem(problem)
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
	scans.OnStart(func(notice app.ScanNotice) {
		recheckRoots()
		auto.ScanStarted(notice)
	})
	scans.OnFinish(auto.ScanFinished)
	return auto
}

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
