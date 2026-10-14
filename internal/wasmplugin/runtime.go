package wasmplugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/syudead/vv/internal/domain"
)

const (
	wasiModule = wasi_snapshot_preview1.ModuleName
	vvModule   = "vv"

	manifestSection = "vv.plugin"
	startExport     = "_start"
	memoryExport    = "memory"

	// compiledCacheSize はダイジェストごとにコンパイル済みのまま持つモジュールの数である。
	compiledCacheSize = 8
)

// Options は Runtime の設定である。
type Options struct {
	// Logger はプラグインの標準エラーを書く先である。nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Runtime は作品情報プラグインを検査し、動かす。app.WorkInfoPluginRuntime を満たす。
// 検査はいつでも並べて行え、実行は同時に 1 本だけ動く。
type Runtime struct {
	logger *slog.Logger

	// inspect は検査でコンパイルだけを行う wazero のランタイムで、何も実行しない。
	inspect wazero.Runtime
	// allowed は import してよい関数と、その引数と戻り値の型である。
	allowed map[importKey]signature

	// run は実行の wazero のランタイムで、WASI と vv のホストモジュールを持つ。
	run *runner
	// slot は同時に動く実行の枠である。
	slot chan struct{}
}

type importKey struct{ module, name string }

type signature struct{ params, results []api.ValueType }

// New は Runtime を作る。使い終えたら Close を呼ぶ。
func New(ctx context.Context, opts Options) (*Runtime, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	inspect := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCustomSections(true).
		WithMemoryLimitPages(domain.WorkInfoPluginMemoryLimitPages))
	allowed, err := allowedImports(ctx, inspect)
	if err != nil {
		_ = inspect.Close(ctx)
		return nil, err
	}
	run, err := newRunner(ctx, logger)
	if err != nil {
		_ = inspect.Close(ctx)
		return nil, err
	}
	return &Runtime{
		logger:  logger,
		inspect: inspect,
		allowed: allowed,
		run:     run,
		slot:    make(chan struct{}, 1),
	}, nil
}

// Close は wazero のランタイムを閉じる。動いている実行があれば止まる。
func (r *Runtime) Close(ctx context.Context) error {
	return errors.Join(r.inspect.Close(ctx), r.run.close(ctx))
}

// allowedImports は WASI と vv のホストモジュールをコンパイルし、それが export する関数を
// import してよい関数の一覧にする。
func allowedImports(ctx context.Context, rt wazero.Runtime) (map[importKey]signature, error) {
	allowed := map[importKey]signature{}
	builders := []interface {
		Compile(context.Context) (wazero.CompiledModule, error)
	}{wasi_snapshot_preview1.NewBuilder(rt), hostModule(rt)}
	for _, b := range builders {
		compiled, err := b.Compile(ctx)
		if err != nil {
			return nil, fmt.Errorf("compile host module: %w", err)
		}
		for name, def := range compiled.ExportedFunctions() {
			allowed[importKey{compiled.Name(), name}] = signature{def.ParamTypes(), def.ResultTypes()}
		}
	}
	return allowed, nil
}

// Inspect はモジュールを実行せずに検査する。最大値を宣言しないテーブルに最大値を書き足してから
// コンパイルし、順に、コンパイル、vv.plugin のマニフェスト、import、export、テーブルを確かめ、
// 最初に当たった問題を返す（data-model.md の work_info_plugins、research.md R-4）。
func (r *Runtime) Inspect(ctx context.Context, module []byte) (domain.WorkInfoPluginManifest, domain.WorkInfoPluginProblem, error) {
	if len(module) > domain.WorkInfoPluginModuleMaxBytes {
		return domain.WorkInfoPluginManifest{}, "", domain.ErrWorkInfoPluginTooLarge
	}
	layout, err := readLayout(module)
	if err != nil {
		return domain.WorkInfoPluginManifest{}, domain.WorkInfoPluginNotWasm, nil
	}
	compiled, err := r.inspect.CompileModule(ctx, capTables(module, layout, domain.WorkInfoPluginTableMaxElements))
	if err != nil {
		if ctx.Err() != nil {
			return domain.WorkInfoPluginManifest{}, "", ctx.Err()
		}
		return domain.WorkInfoPluginManifest{}, domain.WorkInfoPluginNotWasm, nil
	}
	defer func() { _ = compiled.Close(ctx) }()
	manifest, problem := readManifest(compiled)
	if problem == "" {
		problem = r.checkImports(compiled, layout)
	}
	if problem == "" {
		problem = checkExports(compiled)
	}
	if problem == "" {
		problem = checkTables(layout)
	}
	if problem != "" {
		return domain.WorkInfoPluginManifest{}, problem, nil
	}
	return manifest, "", nil
}

func readManifest(compiled wazero.CompiledModule) (domain.WorkInfoPluginManifest, domain.WorkInfoPluginProblem) {
	for _, s := range compiled.CustomSections() {
		if s.Name() == manifestSection {
			return domain.ParseWorkInfoPluginManifest(s.Data())
		}
	}
	return domain.WorkInfoPluginManifest{}, domain.WorkInfoPluginManifestMissing
}

// checkImports は、どの import も WASI か vv のホストモジュールが export する関数で、型も同じかを確かめる。
func (r *Runtime) checkImports(compiled wazero.CompiledModule, layout moduleLayout) domain.WorkInfoPluginProblem {
	for _, imp := range layout.imports {
		if imp.kind != importKindFunc {
			return domain.WorkInfoPluginImportUnsupported
		}
	}
	for _, def := range compiled.ImportedFunctions() {
		module, name, _ := def.Import()
		want, ok := r.allowed[importKey{module, name}]
		if !ok || !slices.Equal(def.ParamTypes(), want.params) || !slices.Equal(def.ResultTypes(), want.results) {
			return domain.WorkInfoPluginImportUnsupported
		}
	}
	return ""
}

func checkExports(compiled wazero.CompiledModule) domain.WorkInfoPluginProblem {
	start, ok := compiled.ExportedFunctions()[startExport]
	if !ok || len(start.ParamTypes()) != 0 || len(start.ResultTypes()) != 0 {
		return domain.WorkInfoPluginExportInvalid
	}
	if _, ok := compiled.ExportedMemories()[memoryExport]; !ok {
		return domain.WorkInfoPluginExportInvalid
	}
	return ""
}

// checkTables は、どのテーブルも最小値と、宣言するなら最大値が上限以下かを確かめる。
// 最大値を宣言しないテーブルは、コンパイルの前に capTables が上限を最大値にしている。
func checkTables(layout moduleLayout) domain.WorkInfoPluginProblem {
	for _, t := range layout.tables {
		if t.min > domain.WorkInfoPluginTableMaxElements || t.hasMax && t.max > domain.WorkInfoPluginTableMaxElements {
			return domain.WorkInfoPluginTableTooLarge
		}
	}
	return ""
}
