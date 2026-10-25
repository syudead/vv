package main

import (
	"context"
	"log/slog"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/store"
	"github.com/syudead/vv/internal/wasmplugin"
)

// Runtime creation failure is logged and leaves plugin routes returning 500 without blocking other
// startup; each execution uses the configured proxy.
func newWorkInfoPlugins(
	ctx context.Context, db *store.DB, network app.WorkInfoRunNetworkSource, covers app.WorkInfoCoverApplier,
	personImages app.WorkInfoPersonImageFiller, logger *slog.Logger,
) (httpapi.WorkInfoPlugins, func()) {
	runtime, err := wasmplugin.New(ctx, wasmplugin.Options{Logger: logger})
	if err != nil {
		logger.Error("could not start the work-info plugin runtime", slog.Any("error", err))
		return nil, func() {}
	}
	registry := app.NewWorkInfoPlugins(app.WorkInfoPluginsOptions{
		Store: db.WorkInfoPlugins(), Runtime: runtime, Videos: db.Library(), Applier: db.Overrides(),
		Matcher: db.Credits(), Network: network, Covers: covers, PersonImages: personImages,
		People: db.Credits(), FillTargets: db.Overrides(),
	})
	registry.StartLinkFetches(ctx, db.VirtualVideos(), logger)
	return registry, func() {
		registry.Close()
		_ = runtime.Close(context.WithoutCancel(ctx))
	}
}
