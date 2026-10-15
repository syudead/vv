// Package plugintest はテストの作品情報プラグインを Go のソースから組み立てる。.wasm のファイルは
// リポジトリに置かず、テストのたびに GOOS=wasip1 GOARCH=wasm でビルドし、vv.plugin のセクションを
// 足す（specs/049-work-info-plugins/research.md R-12）。テストだけが使う。
package plugintest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/wasmplugin/wasmsection"
)

var (
	mu    sync.Mutex
	built = map[string][]byte{}
)

// Build は dir の Go の main パッケージを wasip1 向けにビルドし、モジュールのバイト列を返す。
// 同じ dir のビルドはプロセスの中で 1 回だけ行う。
func Build(tb testing.TB, dir string) []byte {
	tb.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		tb.Fatalf("resolve plugin source %s: %v", dir, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if module, ok := built[abs]; ok {
		return module
	}
	out := filepath.Join(tb.TempDir(), "plugin.wasm")
	cmd := exec.Command("go", "build", "-trimpath", "-o", out, ".")
	cmd.Dir = abs
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("build plugin %s: %v\n%s", dir, err, output)
	}
	module, err := os.ReadFile(out)
	if err != nil {
		tb.Fatalf("read built plugin: %v", err)
	}
	built[abs] = module
	return module
}

// WithManifest は module の末尾に、manifest を中身とする vv.plugin のカスタムセクションを足した
// 新しいバイト列を返す。
func WithManifest(module []byte, manifest string) []byte {
	return append(bytes.Clone(module), CustomSection("vv.plugin", []byte(manifest))...)
}

// CustomSection は名前 name、中身 data のカスタムセクション（id 0）を符号化する。
func CustomSection(name string, data []byte) []byte {
	return wasmsection.Custom(name, data)
}
