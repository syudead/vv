package app

import (
	"context"

	"github.com/syudead/vv/internal/domain"
)

// WorkInfoPluginRuntime は作品情報プラグイン（WebAssembly モジュール）を検査し、動かすポートである。
// internal/wasmplugin の *Runtime が満たす（specs/049-work-info-plugins/plan.md の Constitution Check、
// research.md R-1〜R-4）。
type WorkInfoPluginRuntime interface {
	// Inspect はモジュールを実行せずに検査し、マニフェストか、承認できない理由を返す。マニフェストを
	// 読めたあとの検査で問題が見つかったときは、読めたマニフェストも問題と一緒に返す。
	// モジュールが domain.WorkInfoPluginModuleMaxBytes を超えるときは domain.ErrWorkInfoPluginTooLarge を返す。
	Inspect(ctx context.Context, module []byte) (domain.WorkInfoPluginManifest, domain.WorkInfoPluginProblem, error)
	// Run はダイジェスト digest のモジュールを新しいインスタンスで 1 回動かし、要求への応答を返す。
	// プラグインの要求は network のとおりに送る（specs/056-plugin-tooling/research.md R-4）。
	// 実行の失敗は *domain.WorkInfoRunError で返す。成功でも失敗でも、残した標準エラーを実行のログとして返し、
	// モジュールが成功か実行の失敗で終わったときは、同じログを次の実行が始まれる前に record に渡す
	// （specs/056-plugin-tooling/research.md R-6）。record は nil でもよい。
	Run(
		ctx context.Context, digest string, module []byte, req domain.WorkInfoRunRequest, network domain.WorkInfoRunNetwork,
		record func(domain.WorkInfoRunLog) error,
	) (domain.WorkInfoRunResult, domain.WorkInfoRunLog, error)
}
