package main

import (
	"context"
	"log/slog"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/store"
	"github.com/syudead/vv/internal/wasmplugin"
)

// newWorkInfoPlugins は作品情報プラグインのランタイム（wazero）と登録・検索・取得・反映のユースケースを組み立て、
// 画面の API に渡す登録と、ランタイムを閉じる関数を返す。
// ランタイムを作れなければ記録に残して nil を返し、プラグインの経路は 500 を返す。プラグインが
// 使えないだけで、ほかの機能の起動は止めない。各実行は network が返すプロキシを通す
func newWorkInfoPlugins(
	ctx context.Context, db *store.DB, network app.WorkInfoRunNetworkSource, covers app.WorkInfoCoverApplier,
	logger *slog.Logger,
) (httpapi.WorkInfoPlugins, func()) {
	runtime, err := wasmplugin.New(ctx, wasmplugin.Options{Logger: logger})
	if err != nil {
		logger.Error("could not start the work-info plugin runtime", slog.Any("error", err))
		return nil, func() {}
	}
	registry := app.NewWorkInfoPlugins(app.WorkInfoPluginsOptions{
		Store: db.WorkInfoPlugins(), Runtime: runtime, Videos: db.Library(), Applier: db.Overrides(),
		Matcher: db.Credits(), Network: network, Covers: covers,
	})
	return registry, func() { _ = runtime.Close(context.WithoutCancel(ctx)) }
}
