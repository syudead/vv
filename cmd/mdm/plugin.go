package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/wasmplugin"
)

// Plugin-author commands need no MDM_DATA_DIR, database, or ffmpeg; domain owns manifest rules and
// wasmplugin owns module validation and rewriting.

var inspectionProblemHints = map[domain.WorkInfoPluginProblem]string{
	domain.WorkInfoPluginNotWasm:           "the module is not a WebAssembly module that compiles",
	domain.WorkInfoPluginImportUnsupported: "the module imports something other than WASI and vv functions, or uses another signature",
	domain.WorkInfoPluginExportInvalid:     "_start or memory is missing or has another type",
	domain.WorkInfoPluginTableTooLarge:     fmt.Sprintf("the module's tables exceed %d elements", domain.WorkInfoPluginTableMaxElements),
	// Only plugin run emits these errors; pack validates manifests first.
	domain.WorkInfoPluginManifestMissing:     "the module has no vv.plugin section; write the manifest into it with mdm plugin pack",
	domain.WorkInfoPluginManifestInvalid:     "the module's vv.plugin section breaks a manifest rule; mdm plugin pack names the field and the rule",
	domain.WorkInfoPluginContractUnsupported: "the manifest's contract is not 1",
}

var operationNames = map[domain.WorkInfoOperation]string{
	domain.WorkInfoSearch: "Search",
	domain.WorkInfoFetch:  "Fetch by ID",
}

// args excludes the leading "plugin" argument.
func runPlugin(ctx context.Context, args []string, env accountEnv) *accountFailure {
	if len(args) == 0 {
		return usageFailure("Specify a plugin subcommand.")
	}
	switch args[0] {
	case "pack":
		return runPluginPack(ctx, args[1:], env.Stderr)
	case "run":
		return runPluginRun(ctx, args[1:], env)
	default:
		return usageFailure("Unknown plugin subcommand: %s", args[0])
	}
}

// Input-content failures use exit code 2 without usage text.
func refusedFailure(format string, args ...any) *accountFailure {
	return &accountFailure{code: exitAccountUsage, message: fmt.Sprintf(format, args...)}
}

// runPluginPack validates with the upload rules and atomically replaces the resolved module target;
// every failure preserves the original, including when given a symlink.
func runPluginPack(ctx context.Context, args []string, stderr io.Writer) *accountFailure {
	if len(args) != 2 {
		return usageFailure("pack takes exactly two arguments: the module and the manifest.")
	}
	modulePath := args[0]
	unchanged := fmt.Sprintf(" %s was not changed.", modulePath)
	target, module, manifestJSON, failure := readPackInputs(modulePath, args[1], unchanged)
	if failure != nil {
		return failure
	}
	if _, rejection := domain.CheckWorkInfoPluginManifest(manifestJSON); rejection != nil {
		return refusedFailure("%v.%s", rejection, unchanged)
	}
	packed, err := wasmplugin.WithManifest(module, manifestJSON)
	if err != nil {
		return refusedFailure("%s: %s.%s", domain.WorkInfoPluginNotWasm,
			inspectionProblemHints[domain.WorkInfoPluginNotWasm], unchanged)
	}
	manifest, failure := inspectPacked(ctx, packed, unchanged)
	if failure != nil {
		return failure
	}
	if err := replaceFile(target, packed); err != nil {
		return operationFailure("Cannot write the module: %v", err)
	}
	writePackedManifest(stderr, modulePath, manifest)
	return nil
}

// Reject oversized inputs before reading them fully; no packing can make them pass the module
// limit.
func readPackInputs(modulePath, manifestPath, unchanged string) (target string, module, manifest []byte, failure *accountFailure) {
	target, err := filepath.EvalSymlinks(modulePath)
	if err != nil {
		return "", nil, nil, operationFailure("Cannot read the module: %v", err)
	}
	module, failure = readPackInput(target, "module", unchanged)
	if failure != nil {
		return "", nil, nil, failure
	}
	manifest, failure = readPackInput(manifestPath, "manifest", unchanged)
	if failure != nil {
		return "", nil, nil, failure
	}
	return target, module, manifest, nil
}

// Read at most limit+1 bytes, even if the file grows during reading.
func readPackInput(path, what, unchanged string) ([]byte, *accountFailure) {
	f, err := os.Open(path)
	if err != nil {
		return nil, operationFailure("Cannot read the %s: %v", what, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, domain.WorkInfoPluginModuleMaxBytes+1))
	if err != nil {
		return nil, operationFailure("Cannot read the %s: %v", what, err)
	}
	if len(data) > domain.WorkInfoPluginModuleMaxBytes {
		return nil, refusedFailure("The %s is larger than %d MiB.%s", what, domain.WorkInfoPluginModuleMaxBytes>>20, unchanged)
	}
	return data, nil
}

func inspectPacked(ctx context.Context, packed []byte, unchanged string) (domain.WorkInfoPluginManifest, *accountFailure) {
	runtime, failure := newPluginRuntime(ctx, wasmplugin.Options{})
	if failure != nil {
		return domain.WorkInfoPluginManifest{}, failure
	}
	defer func() { _ = runtime.Close(context.WithoutCancel(ctx)) }()
	return inspectModule(ctx, runtime, packed, "packed module", unchanged, refusedFailure)
}

// Use server runtime limits but leave plugin stderr to the caller's execution log.
func newPluginRuntime(ctx context.Context, opts wasmplugin.Options) (*wasmplugin.Runtime, *accountFailure) {
	opts.Logger = slog.New(slog.DiscardHandler)
	runtime, err := wasmplugin.New(ctx, opts)
	if err != nil {
		return nil, operationFailure("Cannot start the plugin runtime: %v", err)
	}
	return runtime, nil
}

func inspectModule(
	ctx context.Context, runtime *wasmplugin.Runtime, module []byte, what, suffix string,
	refuse func(string, ...any) *accountFailure,
) (domain.WorkInfoPluginManifest, *accountFailure) {
	manifest, problem, err := runtime.Inspect(ctx, module)
	switch {
	case errors.Is(err, domain.ErrWorkInfoPluginTooLarge):
		return manifest, refuse("The %s is larger than %d MiB.%s", what, domain.WorkInfoPluginModuleMaxBytes>>20, suffix)
	case err != nil:
		return manifest, operationFailure("Cannot inspect the module: %v", err)
	case problem != "":
		return manifest, refuse("%s: %s.%s", problem, inspectionProblemHints[problem], suffix)
	}
	return manifest, nil
}

// Write and rename a same-directory temporary file, preserving permissions; failures before rename
// leave the target unchanged.
func replaceFile(path string, data []byte) (err error) {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = writeAndClose(tmp, data); err != nil {
		return err
	}
	if err = os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func writeAndClose(f *os.File, data []byte) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

func writePackedManifest(w io.Writer, modulePath string, m domain.WorkInfoPluginManifest) {
	ops := make([]string, 0, len(m.Operations))
	for _, op := range m.Operations {
		ops = append(ops, operationNames[op])
	}
	lines := []string{
		fmt.Sprintf("Wrote the manifest into %s.", modulePath),
		fmt.Sprintf("%s %s", m.Name, m.Version),
	}
	if m.Description != "" {
		lines = append(lines, m.Description)
	}
	lines = append(lines, "Does: "+strings.Join(ops, " · "))
	if len(m.Settings) == 0 {
		lines = append(lines, "Settings: none")
	} else {
		lines = append(lines, "Settings:")
	}
	for _, s := range m.Settings {
		lines = append(lines, "  "+describeSetting(s))
	}
	_, _ = fmt.Fprintln(w, strings.Join(lines, "\n"))
}

// Match the approval screen's field order: label, Secret, Required, default.
func describeSetting(s domain.WorkInfoPluginSetting) string {
	facts := []string{}
	if s.Secret {
		facts = append(facts, "Secret")
	}
	if s.Required {
		facts = append(facts, "Required")
	}
	if s.Default != "" {
		facts = append(facts, "Default: "+s.Default)
	}
	if s.Options != nil {
		facts = append(facts, "Options: "+strings.Join(s.Options, ", "))
	}
	line := fmt.Sprintf("%s (%s)", s.Label, s.Key)
	if len(facts) > 0 {
		line += ": " + strings.Join(facts, "; ")
	}
	return line
}
