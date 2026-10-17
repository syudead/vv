package wasmplugin

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/wasmplugin/plugintest"
)

// 検査のテストは手で組んだ小さなモジュールを使う。どれも実行しない。

const goodManifest = `{"contract":1,"name":"example.works","version":"1.0.0","description":"Example",` +
	`"operations":["search","fetch"],"settings":[{"key":"apiKey","label":"API key","secret":true,"required":true}]}`

// 型の番号。
const (
	typeVoid     = 0 // () -> ()
	typeI32      = 1 // (i32) -> ()
	typeHTTPSend = 2 // (i32, i32) -> i32
	typeFdWrite  = 3 // (i32, i32, i32, i32) -> i32
)

type testImport struct {
	module, name string
	kind         byte
	desc         []byte
}

func funcImport(module, name string, typeIndex byte) testImport {
	return testImport{module: module, name: name, kind: 0x00, desc: []byte{typeIndex}}
}

// moduleSpec は手で組むモジュールの形である。既定は検査を通る形になる。
type moduleSpec struct {
	manifest     *string
	imports      []testImport
	startType    byte
	noStart      bool
	noMemory     bool
	memoryExport string
	tables       [][]byte // 各テーブルの宣言（要素の型と要素数）
	code         []byte   // _start の命令（end を除く）。空なら何もしない
	data         []byte   // メモリの 0 番地から置くデータ
}

func goodSpec() moduleSpec {
	m := goodManifest
	return moduleSpec{
		manifest: &m,
		imports: []testImport{
			funcImport("wasi_snapshot_preview1", "proc_exit", typeI32),
			funcImport("vv", "http_send", typeHTTPSend),
		},
		tables: [][]byte{{0x70, 0x01, 0x01, 0x01}},
	}
}

func vec(items ...[]byte) []byte {
	out := binary.AppendUvarint(nil, uint64(len(items)))
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

func name(s string) []byte {
	return append(binary.AppendUvarint(nil, uint64(len(s))), s...)
}

func section(id byte, payload []byte) []byte {
	return append(binary.AppendUvarint([]byte{id}, uint64(len(payload))), payload...)
}

func (s moduleSpec) bytes() []byte {
	out := []byte("\x00asm\x01\x00\x00\x00")
	out = append(out, section(1, vec(
		[]byte{0x60, 0x00, 0x00},
		[]byte{0x60, 0x01, 0x7f, 0x00},
		[]byte{0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f},
		[]byte{0x60, 0x04, 0x7f, 0x7f, 0x7f, 0x7f, 0x01, 0x7f},
	))...)
	var imports [][]byte
	for _, imp := range s.imports {
		entry := append(append(name(imp.module), name(imp.name)...), imp.kind)
		imports = append(imports, append(entry, imp.desc...))
	}
	if len(imports) > 0 {
		out = append(out, section(2, vec(imports...))...)
	}
	out = append(out, section(3, vec([]byte{s.startType}))...)
	if len(s.tables) > 0 {
		out = append(out, section(4, vec(s.tables...))...)
	}
	out = append(out, section(5, vec([]byte{0x00, 0x01}))...)
	funcImports := 0
	for _, imp := range s.imports {
		if imp.kind == 0x00 {
			funcImports++
		}
	}
	out = append(out, section(7, vec(s.exports(funcImports)...))...)
	body := append(append([]byte{0x00}, s.code...), 0x0b)
	out = append(out, section(10, vec(append(binary.AppendUvarint(nil, uint64(len(body))), body...)))...)
	if len(s.data) > 0 {
		segment := append([]byte{0x00, 0x41, 0x00, 0x0b}, binary.AppendUvarint(nil, uint64(len(s.data)))...)
		out = append(out, section(11, vec(append(segment, s.data...)))...)
	}
	if s.manifest != nil {
		out = appendCustomSection(out, manifestSection, []byte(*s.manifest))
	}
	return out
}

func (s moduleSpec) exports(funcIndex int) [][]byte {
	var exports [][]byte
	if !s.noStart {
		exports = append(exports, append(name("_start"), 0x00, byte(funcIndex)))
	}
	if !s.noMemory {
		memoryName := s.memoryExport
		if memoryName == "" {
			memoryName = "memory"
		}
		exports = append(exports, append(name(memoryName), 0x02, 0x00))
	}
	return exports
}

func newTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	rt, err := New(context.Background(), Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(context.Background()) })
	return rt
}

func TestInspectGoodModuleReturnsManifest(t *testing.T) {
	rt := newTestRuntime(t)
	manifest, problem, err := rt.Inspect(context.Background(), goodSpec().bytes())
	if err != nil || problem != "" {
		t.Fatalf("Inspect = problem %q, err %v; want a manifest", problem, err)
	}
	if manifest.Name != "example.works" || manifest.Version != "1.0.0" || manifest.Description != "Example" ||
		!manifest.Supports(domain.WorkInfoSearch) || !manifest.Supports(domain.WorkInfoFetch) {
		t.Fatalf("manifest = %+v", manifest)
	}
	want := domain.WorkInfoPluginSetting{Key: "apiKey", Label: "API key", Secret: true, Required: true}
	if !reflect.DeepEqual(manifest.Settings, []domain.WorkInfoPluginSetting{want}) {
		t.Fatalf("settings = %+v, want [%+v]", manifest.Settings, want)
	}
}

func TestInspectBuiltPluginReturnsManifest(t *testing.T) {
	rt := newTestRuntime(t)
	module := mustWithManifest(t, plugintest.Build(t, "testdata/probe"), goodManifest)
	if _, problem, err := rt.Inspect(context.Background(), module); err != nil || problem != "" {
		t.Fatalf("Inspect(built probe) = problem %q, err %v; want a manifest", problem, err)
	}
}

func manifestSpec(manifest string) moduleSpec {
	s := goodSpec()
	s.manifest = &manifest
	return s
}

func settingsManifest(n int, key func(i int) string) string {
	settings := make([]string, n)
	for i := range settings {
		settings[i] = fmt.Sprintf(`{"key":%q,"label":"Label"}`, key(i))
	}
	return `{"contract":1,"name":"p","version":"1","operations":["fetch"],"settings":[` + strings.Join(settings, ",") + `]}`
}

func TestInspectReportsEachProblem(t *testing.T) {
	rt := newTestRuntime(t)
	cases := []struct {
		name   string
		module func() []byte
		want   domain.WorkInfoPluginProblem
	}{
		{"not WebAssembly", func() []byte { return []byte("not a module") }, domain.WorkInfoPluginNotWasm},
		{"truncated module", func() []byte { b := goodSpec().bytes(); return b[:len(b)/2] }, domain.WorkInfoPluginNotWasm},
		{"no manifest section", func() []byte { s := goodSpec(); s.manifest = nil; return s.bytes() }, domain.WorkInfoPluginManifestMissing},
		{"manifest not JSON", func() []byte { return manifestSpec("name: p").bytes() }, domain.WorkInfoPluginManifestInvalid},
		{"manifest bad name", func() []byte {
			return manifestSpec(`{"contract":1,"name":"Bad Name","version":"1","operations":["fetch"]}`).bytes()
		}, domain.WorkInfoPluginManifestInvalid},
		{"duplicate setting key", func() []byte {
			return manifestSpec(settingsManifest(2, func(int) string { return "token" })).bytes()
		}, domain.WorkInfoPluginManifestInvalid},
		{"eleventh setting", func() []byte {
			return manifestSpec(settingsManifest(11, func(i int) string { return fmt.Sprintf("key%d", i) })).bytes()
		}, domain.WorkInfoPluginManifestInvalid},
		{"contract 2", func() []byte {
			return manifestSpec(`{"contract":2,"name":"p","version":"1","operations":["fetch"]}`).bytes()
		}, domain.WorkInfoPluginContractUnsupported},
		{"import from another module", func() []byte {
			s := goodSpec()
			s.imports = append(s.imports, funcImport("env", "now", typeVoid))
			return s.bytes()
		}, domain.WorkInfoPluginImportUnsupported},
		{"unknown vv function", func() []byte {
			s := goodSpec()
			s.imports = append(s.imports, funcImport("vv", "exec", typeVoid))
			return s.bytes()
		}, domain.WorkInfoPluginImportUnsupported},
		{"vv function with another signature", func() []byte {
			s := goodSpec()
			s.imports = []testImport{funcImport("vv", "http_send", typeVoid)}
			return s.bytes()
		}, domain.WorkInfoPluginImportUnsupported},
		{"imported global", func() []byte {
			s := goodSpec()
			s.imports = []testImport{{module: "vv", name: "g", kind: 0x03, desc: []byte{0x7f, 0x00}}}
			return s.bytes()
		}, domain.WorkInfoPluginImportUnsupported},
		{"_start with a parameter", func() []byte { s := goodSpec(); s.startType = typeI32; return s.bytes() }, domain.WorkInfoPluginExportInvalid},
		{"no _start", func() []byte { s := goodSpec(); s.noStart = true; return s.bytes() }, domain.WorkInfoPluginExportInvalid},
		{"no exported memory", func() []byte { s := goodSpec(); s.noMemory = true; return s.bytes() }, domain.WorkInfoPluginExportInvalid},
		{"memory exported under another name", func() []byte { s := goodSpec(); s.memoryExport = "mem"; return s.bytes() }, domain.WorkInfoPluginExportInvalid},
		{"table without a maximum and a minimum over 100,000", func() []byte {
			s := goodSpec()
			s.tables = [][]byte{binary.AppendUvarint([]byte{0x70, 0x00}, domain.WorkInfoPluginTableMaxElements+1)}
			return s.bytes()
		}, domain.WorkInfoPluginTableTooLarge},
		{"table with a minimum over 100,000", func() []byte {
			s := goodSpec()
			limits := binary.AppendUvarint([]byte{0x70, 0x01}, domain.WorkInfoPluginTableMaxElements+1)
			s.tables = [][]byte{binary.AppendUvarint(limits, 2_000_000)}
			return s.bytes()
		}, domain.WorkInfoPluginTableTooLarge},
		{"table of 1,000,000 elements", func() []byte {
			s := goodSpec()
			s.tables = [][]byte{binary.AppendUvarint([]byte{0x70, 0x01, 0x00}, 1_000_000)}
			return s.bytes()
		}, domain.WorkInfoPluginTableTooLarge},
		{"tables each within 100,000 whose maximums sum over it", func() []byte {
			s := goodSpec()
			s.tables = [][]byte{maxTable(60_000), maxTable(60_000)}
			return s.bytes()
		}, domain.WorkInfoPluginTableTooLarge},
		{"many small tables whose maximums sum over 100,000", func() []byte {
			s := goodSpec()
			s.tables = nil
			for range 11 {
				s.tables = append(s.tables, maxTable(10_000))
			}
			return s.bytes()
		}, domain.WorkInfoPluginTableTooLarge},
		{"table without a maximum beside another table", func() []byte {
			s := goodSpec()
			// 最大値の無いテーブルは vv が書き足す 100,000 で数えるので、もう 1 要素で合計を超える。
			s.tables = [][]byte{binary.AppendUvarint([]byte{0x70, 0x00}, 1), maxTable(1)}
			return s.bytes()
		}, domain.WorkInfoPluginTableTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest, problem, err := rt.Inspect(context.Background(), tc.module())
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if problem != tc.want {
				t.Fatalf("problem = %q, want %q", problem, tc.want)
			}
			// マニフェストを読めたあとの問題（import・export・テーブル）は、読めた名前も返す。
			wantName := ""
			if readsManifest(tc.want) {
				wantName = "example.works"
			}
			if manifest.Name != wantName {
				t.Fatalf("manifest name = %q, want %q with problem %q", manifest.Name, wantName, tc.want)
			}
		})
	}
}

// readsManifest は、問題 problem がマニフェストを読めたあとの検査で見つかるものかを返す。
func readsManifest(problem domain.WorkInfoPluginProblem) bool {
	switch problem {
	case domain.WorkInfoPluginImportUnsupported, domain.WorkInfoPluginExportInvalid, domain.WorkInfoPluginTableTooLarge:
		return true
	}
	return false
}

func TestInspectAcceptsTableWithoutAMaximum(t *testing.T) {
	rt := newTestRuntime(t)
	s := goodSpec()
	// Go の wasip1 のリンカは関数のテーブルを最大値なしで書く。
	s.tables = [][]byte{binary.AppendUvarint([]byte{0x70, 0x00}, domain.WorkInfoPluginTableMaxElements)}
	if _, problem, err := rt.Inspect(context.Background(), s.bytes()); err != nil || problem != "" {
		t.Fatalf("Inspect = problem %q, err %v; want none", problem, err)
	}
}

func TestInspectAcceptsTableAtTheLimit(t *testing.T) {
	rt := newTestRuntime(t)
	s := goodSpec()
	s.tables = [][]byte{binary.AppendUvarint([]byte{0x70, 0x01, 0x00}, domain.WorkInfoPluginTableMaxElements)}
	if _, problem, err := rt.Inspect(context.Background(), s.bytes()); err != nil || problem != "" {
		t.Fatalf("Inspect = problem %q, err %v; want none", problem, err)
	}
}

func TestInspectAcceptsTablesSummingToTheLimit(t *testing.T) {
	rt := newTestRuntime(t)
	s := goodSpec()
	s.tables = [][]byte{maxTable(60_000), maxTable(domain.WorkInfoPluginTableMaxElements - 60_000)}
	if _, problem, err := rt.Inspect(context.Background(), s.bytes()); err != nil || problem != "" {
		t.Fatalf("Inspect = problem %q, err %v; want none", problem, err)
	}
}

// maxTable は最小値 0、最大値 n の funcref のテーブルの宣言である。
func maxTable(n uint64) []byte {
	return binary.AppendUvarint([]byte{0x70, 0x01, 0x00}, n)
}

func TestInspectRefusesModuleOver16MiB(t *testing.T) {
	rt := newTestRuntime(t)
	module := mustWithManifest(t, goodSpec().bytes(), strings.Repeat(" ", domain.WorkInfoPluginModuleMaxBytes))
	if _, _, err := rt.Inspect(context.Background(), module); !errors.Is(err, domain.ErrWorkInfoPluginTooLarge) {
		t.Fatalf("Inspect err = %v, want ErrWorkInfoPluginTooLarge", err)
	}
}
