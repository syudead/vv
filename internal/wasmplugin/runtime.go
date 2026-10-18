package wasmplugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
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

	compiledCacheSize = 8
)

type Options struct {
	// Nil uses slog.Default.
	Logger *slog.Logger
	// Nil uses the system resolver; tests may replace it without internet access.
	Resolve func(ctx context.Context, host string) ([]netip.Addr, error)
	// Connect receives only rule-approved resolved addresses for direct requests; nil uses net.Dialer.
	Connect func(ctx context.Context, network, address string) (net.Conn, error)
}

// Runtime allows concurrent inspection but serial execution.
type Runtime struct {
	logger *slog.Logger

	// Inspection compiles modules without running them.
	inspect wazero.Runtime
	allowed map[importKey]signature

	run  *runner
	slot chan struct{}
}

type importKey struct{ module, name string }

type signature struct{ params, results []api.ValueType }

// Call Close when the runtime is no longer needed.
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
	run, err := newRunner(ctx, logger, newNetClient(opts))
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

// Close stops active executions.
func (r *Runtime) Close(ctx context.Context) error {
	return errors.Join(r.inspect.Close(ctx), r.run.close(ctx))
}

// Allowed imports come from compiled WASI and application host exports.
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

// Inspect caps unbounded tables before compilation and returns the first validation problem without
// running code. Preserve a readable manifest even on later failure so storage can identify invalid
// replacements by name.
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
	if problem != "" {
		return domain.WorkInfoPluginManifest{}, problem, nil
	}
	problem = r.checkImports(compiled, layout)
	if problem == "" {
		problem = checkExports(compiled)
	}
	if problem == "" {
		problem = checkTables(layout)
	}
	return manifest, problem, nil
}

func readManifest(compiled wazero.CompiledModule) (domain.WorkInfoPluginManifest, domain.WorkInfoPluginProblem) {
	for _, s := range compiled.CustomSections() {
		if s.Name() == manifestSection {
			return domain.ParseWorkInfoPluginManifest(s.Data())
		}
	}
	return domain.WorkInfoPluginManifest{}, domain.WorkInfoPluginManifestMissing
}

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

// Bound both individual tables and their aggregate declared maxima, including inserted caps; tables
// consume host memory outside linear-memory limits.
func checkTables(layout moduleLayout) domain.WorkInfoPluginProblem {
	var total uint64
	for _, t := range layout.tables {
		if t.min > domain.WorkInfoPluginTableMaxElements || t.hasMax && t.max > domain.WorkInfoPluginTableMaxElements {
			return domain.WorkInfoPluginTableTooLarge
		}
		total += t.elements(domain.WorkInfoPluginTableMaxElements)
	}
	if total > domain.WorkInfoPluginTableMaxElements {
		return domain.WorkInfoPluginTableTooLarge
	}
	return ""
}
