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

// plugin の下位コマンドは作品情報プラグインの作者のためのホスト側のコマンドである
// （specs/056-plugin-tooling/contracts/plugin-cli.md、research.md R-1）。MDM_DATA_DIR を読まず、
// データベースも ffmpeg も使わない。マニフェストの規則は internal/domain、モジュールの検査と
// セクションの書き換えは internal/wasmplugin が持ち、ここはファイルの読み書きだけを行う。

// inspectionProblemHints は、詰めたモジュールが検査に通らないときに問題に添える説明である。
var inspectionProblemHints = map[domain.WorkInfoPluginProblem]string{
	domain.WorkInfoPluginNotWasm:           "the module is not a WebAssembly module that compiles",
	domain.WorkInfoPluginImportUnsupported: "the module imports something other than WASI and vv functions, or uses another signature",
	domain.WorkInfoPluginExportInvalid:     "_start or memory is missing or has another type",
	domain.WorkInfoPluginTableTooLarge:     fmt.Sprintf("the module's tables exceed %d elements", domain.WorkInfoPluginTableMaxElements),
}

// operationNames は操作を設定画面と同じ言葉で表す。
var operationNames = map[domain.WorkInfoOperation]string{
	domain.WorkInfoSearch: "Search",
	domain.WorkInfoFetch:  "Fetch by URL",
}

// runPlugin は plugin の下位コマンドを実行する。args は "plugin" の後ろの引数である。
func runPlugin(ctx context.Context, args []string, env accountEnv) *accountFailure {
	if len(args) == 0 {
		return usageFailure("Specify a plugin subcommand.")
	}
	switch args[0] {
	case "pack":
		return runPluginPack(ctx, args[1:], env.Stderr)
	default:
		return usageFailure("Unknown plugin subcommand: %s", args[0])
	}
}

// refusedFailure は終了コード 2 で、使い方を添えない失敗である。入力のファイルの中身が規則に
// 反したときに使う。
func refusedFailure(format string, args ...any) *accountFailure {
	return &accountFailure{code: exitAccountUsage, message: fmt.Sprintf(format, args...)}
}

// runPluginPack は mdm plugin pack <module.wasm> <manifest.json> を実行する。マニフェストを確かめ、
// モジュールの vv.plugin のセクションを置き換え、詰めたモジュールをアップロードと同じ検査にかけてから、
// 一時ファイルと rename でモジュールに書く（research.md R-2）。どの失敗でもモジュールは元のままである。
func runPluginPack(ctx context.Context, args []string, stderr io.Writer) *accountFailure {
	if len(args) != 2 {
		return usageFailure("pack takes exactly two arguments: the module and the manifest.")
	}
	modulePath, manifestPath := args[0], args[1]
	module, err := os.ReadFile(modulePath)
	if err != nil {
		return operationFailure("Cannot read the module: %v", err)
	}
	manifestJSON, err := os.ReadFile(manifestPath)
	if err != nil {
		return operationFailure("Cannot read the manifest: %v", err)
	}
	unchanged := fmt.Sprintf(" %s was not changed.", modulePath)
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
	if err := replaceFile(modulePath, packed); err != nil {
		return operationFailure("Cannot write the module: %v", err)
	}
	writePackedManifest(stderr, modulePath, manifest)
	return nil
}

// inspectPacked は詰めたモジュールをサーバーと同じランタイムで検査する。
func inspectPacked(ctx context.Context, packed []byte, unchanged string) (domain.WorkInfoPluginManifest, *accountFailure) {
	runtime, err := wasmplugin.New(ctx, wasmplugin.Options{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		return domain.WorkInfoPluginManifest{}, operationFailure("Cannot start the plugin runtime: %v", err)
	}
	defer func() { _ = runtime.Close(context.WithoutCancel(ctx)) }()
	manifest, problem, err := runtime.Inspect(ctx, packed)
	switch {
	case errors.Is(err, domain.ErrWorkInfoPluginTooLarge):
		return manifest, refusedFailure("The packed module is larger than %d MiB.%s",
			domain.WorkInfoPluginModuleMaxBytes>>20, unchanged)
	case err != nil:
		return manifest, operationFailure("Cannot inspect the module: %v", err)
	case problem != "":
		return manifest, refusedFailure("%s: %s.%s", problem, inspectionProblemHints[problem], unchanged)
	}
	return manifest, nil
}

// replaceFile は path を data で置き換える。同じディレクトリの一時ファイルに書いてから rename するので、
// rename の前に失敗すれば path は元のままである。元のファイルの権限を引き継ぐ。
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

// writePackedManifest は書いたプラグインを、設定画面が一覧に出すとおりに標準エラーへ出す。
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

// describeSetting は設定 1 つを、承認の確認の行と同じ順（ラベル、Secret、Required、既定値）で表す。
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
