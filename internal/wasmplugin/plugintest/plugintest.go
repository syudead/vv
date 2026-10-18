// Package plugintest supplies small prebuilt WASI fixtures and builds Go plugins
// for toolchain compatibility tests. Callers add vv.plugin with WithManifest.
package plugintest

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	mu    sync.Mutex
	built = map[string][]byte{}
)

// Build each Go WASI fixture directory once per process.
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
