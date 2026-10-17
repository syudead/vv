package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/store"
	"github.com/syudead/vv/internal/wasmplugin"
)

// mdm plugin pack のテスト。モジュールは手で組んだ
// 小さなもので、実行はしない。

const packManifest = `{"contract":1,"name":"example-title","version":"0.1.0","description":"Reads titles.",` +
	`"operations":["search","fetch"],"settings":[{"key":"token","label":"API token","secret":true,"required":true},` +
	`{"key":"region","label":"Region","default":"jp","options":["jp","us"]}]}`

// testModule は _start と memory を export する最小のモジュールである。importName が空でなければ
// env.<importName> の関数を import する。
func testModule(importName string) []byte {
	section := func(id byte, payload ...byte) []byte {
		return append(binary.AppendUvarint([]byte{id}, uint64(len(payload))), payload...)
	}
	name := func(s string) []byte { return append([]byte{byte(len(s))}, s...) }
	out := []byte("\x00asm\x01\x00\x00\x00")
	out = append(out, section(1, 0x01, 0x60, 0x00, 0x00)...) // type: () -> ()
	startIndex := byte(0)
	if importName != "" {
		entry := append(append(name("env"), name(importName)...), 0x00, 0x00)
		out = append(out, section(2, append([]byte{0x01}, entry...)...)...)
		startIndex = 1
	}
	out = append(out, section(3, 0x01, 0x00)...)       // function
	out = append(out, section(5, 0x01, 0x00, 0x01)...) // memory: 1 page
	exports := append(append([]byte{0x02}, name("_start")...), 0x00, startIndex)
	exports = append(append(exports, name("memory")...), 0x02, 0x00)
	out = append(out, section(7, exports...)...)
	return append(out, section(10, 0x01, 0x02, 0x00, 0x0b)...) // code: empty body
}

type packRun struct {
	code   int
	stderr string
}

// runPack は mdm plugin pack を args で実行する。MDM_DATA_DIR を読めばテストを止める。
func runPack(t *testing.T, args ...string) packRun {
	t.Helper()
	var stderr bytes.Buffer
	env := accountEnv{
		Getenv: func(key string) string {
			t.Errorf("plugin pack read the environment variable %s", key)
			return ""
		},
		Stdin:  strings.NewReader(""),
		Stderr: &stderr,
	}
	code := runCommand(context.Background(), append([]string{"plugin", "pack"}, args...), env)
	return packRun{code: code, stderr: stderr.String()}
}

// writePackFiles はモジュールとマニフェストを一時ディレクトリに書き、そのパスを返す。
func writePackFiles(t *testing.T, module []byte, manifest string) (modulePath, manifestPath string) {
	t.Helper()
	dir := t.TempDir()
	modulePath = filepath.Join(dir, "plugin.wasm")
	manifestPath = filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(modulePath, module, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return modulePath, manifestPath
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPluginPackWritesAModuleThatInspectAndUploadAccept(t *testing.T) {
	modulePath, manifestPath := writePackFiles(t, testModule(""), packManifest)
	run := runPack(t, modulePath, manifestPath)
	if run.code != exitAccountOK {
		t.Fatalf("exit = %d, stderr %q", run.code, run.stderr)
	}
	for _, want := range []string{
		"example-title 0.1.0", "Does: Search · Fetch by ID",
		"API token (token): Secret; Required", "Region (region): Default: jp; Options: jp, us",
	} {
		if !strings.Contains(run.stderr, want) {
			t.Errorf("stderr %q does not contain %q", run.stderr, want)
		}
	}
	info, err := os.Stat(modulePath)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("module mode = %v, %v; want 0640", info, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(modulePath), ".*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temporary files left: %v", leftovers)
	}

	ctx := context.Background()
	module := readFile(t, modulePath)
	runtime, err := wasmplugin.New(ctx, wasmplugin.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(ctx) })
	if manifest, problem, err := runtime.Inspect(ctx, module); err != nil || problem != "" || manifest.Name != "example-title" {
		t.Fatalf("Inspect = %+v, %q, %v", manifest, problem, err)
	}

	db, err := store.OpenMigratedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	plugins := app.NewWorkInfoPlugins(app.WorkInfoPluginsOptions{Store: db.WorkInfoPlugins(), Runtime: runtime})
	entry, created, err := plugins.UploadWorkInfoPlugin(ctx, "plugin.wasm", module)
	if err != nil || !created || entry.Problem != "" || entry.Name != "example-title" {
		t.Fatalf("Upload = %+v, created %v, err %v; want a usable example-title", entry, created, err)
	}
}

func TestPluginPackRunTwiceLeavesOneSection(t *testing.T) {
	modulePath, manifestPath := writePackFiles(t, testModule(""), packManifest)
	if run := runPack(t, modulePath, manifestPath); run.code != exitAccountOK {
		t.Fatalf("first pack exit = %d, stderr %q", run.code, run.stderr)
	}
	once := readFile(t, modulePath)
	if run := runPack(t, modulePath, manifestPath); run.code != exitAccountOK {
		t.Fatalf("second pack exit = %d, stderr %q", run.code, run.stderr)
	}
	twice := readFile(t, modulePath)
	if !bytes.Equal(once, twice) {
		t.Fatalf("a second pack changed the module")
	}
	if n := bytes.Count(twice, []byte("vv.plugin")); n != 1 {
		t.Fatalf("vv.plugin sections = %d, want 1", n)
	}
}

func TestPluginPackRefusesAndLeavesTheModuleUnchanged(t *testing.T) {
	cases := []struct {
		name     string
		module   []byte
		manifest string
		want     []string
	}{
		{
			name:   "default outside the options",
			module: testModule(""),
			manifest: `{"contract":1,"name":"p","version":"1","operations":["fetch"],` +
				`"settings":[{"key":"region","label":"Region","default":"fr","options":["jp","us"]}]}`,
			want: []string{"manifest_invalid", "settings[0].default", "must be one of options"},
		},
		{name: "not a JSON object", module: testModule(""), manifest: `["fetch"]`,
			want: []string{"manifest_invalid", "must be a JSON object"}},
		{name: "contract 2", module: testModule(""), manifest: `{"contract":2}`,
			want: []string{"contract_unsupported", "contract must be 1"}},
		{name: "unsupported import", module: testModule("open"), manifest: packManifest,
			want: []string{"import_unsupported"}},
		{name: "not wasm", module: []byte("not a module"), manifest: packManifest,
			want: []string{"not_wasm"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modulePath, manifestPath := writePackFiles(t, tc.module, tc.manifest)
			run := runPack(t, modulePath, manifestPath)
			if run.code != exitAccountUsage {
				t.Fatalf("exit = %d, want %d; stderr %q", run.code, exitAccountUsage, run.stderr)
			}
			for _, want := range append(tc.want, "was not changed") {
				if !strings.Contains(run.stderr, want) {
					t.Errorf("stderr %q does not contain %q", run.stderr, want)
				}
			}
			if strings.Contains(run.stderr, "Usage:") {
				t.Errorf("stderr %q shows usage for a refused file", run.stderr)
			}
			if got := readFile(t, modulePath); !bytes.Equal(got, tc.module) {
				t.Fatalf("the module changed")
			}
		})
	}
}

func TestPluginPackUsageAndFileErrors(t *testing.T) {
	modulePath, manifestPath := writePackFiles(t, testModule(""), packManifest)
	missing := filepath.Join(t.TempDir(), "missing.json")
	cases := []struct {
		name string
		args []string
		code int
		want []string
	}{
		{"no arguments", nil, exitAccountUsage, []string{"two arguments", "Usage:", "mdm plugin pack"}},
		{"one argument", []string{modulePath}, exitAccountUsage, []string{"two arguments", "Usage:"}},
		{"missing manifest", []string{modulePath, missing}, exitAccountError, []string{"Cannot read the manifest"}},
		{"missing module", []string{missing, manifestPath}, exitAccountError, []string{"Cannot read the module"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := runPack(t, tc.args...)
			if run.code != tc.code {
				t.Fatalf("exit = %d, want %d; stderr %q", run.code, tc.code, run.stderr)
			}
			for _, want := range tc.want {
				if !strings.Contains(run.stderr, want) {
					t.Errorf("stderr %q does not contain %q", run.stderr, want)
				}
			}
		})
	}
	if got := readFile(t, modulePath); !bytes.Equal(got, testModule("")) {
		t.Fatalf("the module changed")
	}

	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{"plugin", "unpack"}, accountEnv{Stderr: &stderr})
	if code != exitAccountUsage || !strings.Contains(stderr.String(), "Unknown plugin subcommand") {
		t.Fatalf("unknown subcommand: exit %d, stderr %q", code, stderr.String())
	}
}

func TestPluginPackRefusesAnInputOverTheModuleLimit(t *testing.T) {
	for _, what := range []string{"module", "manifest"} {
		t.Run(what, func(t *testing.T) {
			modulePath, manifestPath := writePackFiles(t, testModule(""), packManifest)
			large := modulePath
			if what == "manifest" {
				large = manifestPath
			}
			// 疎なファイルなので、上限を大きく超えてもディスクは使わない。コマンドは上限の 1 バイト先までしか読まない。
			if err := os.Truncate(large, 1<<40); err != nil {
				t.Skipf("cannot make a sparse file: %v", err)
			}
			before := readFileInfo(t, modulePath)
			run := runPack(t, modulePath, manifestPath)
			if run.code != exitAccountUsage {
				t.Fatalf("exit = %d, want %d; stderr %q", run.code, exitAccountUsage, run.stderr)
			}
			for _, want := range []string{"The " + what + " is larger than 16 MiB", "was not changed"} {
				if !strings.Contains(run.stderr, want) {
					t.Errorf("stderr %q does not contain %q", run.stderr, want)
				}
			}
			if after := readFileInfo(t, modulePath); after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
				t.Fatalf("the module changed")
			}
		})
	}
}

func readFileInfo(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestPluginPackWritesThroughASymbolicLink(t *testing.T) {
	modulePath, manifestPath := writePackFiles(t, testModule(""), packManifest)
	link := filepath.Join(t.TempDir(), "plugin.wasm")
	if err := os.Symlink(modulePath, link); err != nil {
		t.Skipf("cannot make a symbolic link: %v", err)
	}
	if run := runPack(t, link, manifestPath); run.code != exitAccountOK {
		t.Fatalf("exit = %d, stderr %q", run.code, run.stderr)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced: %v, %v", info, err)
	}
	if n := bytes.Count(readFile(t, modulePath), []byte("vv.plugin")); n != 1 {
		t.Fatalf("vv.plugin sections in the link's target = %d, want 1", n)
	}
}
