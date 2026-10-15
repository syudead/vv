// Command pluginpack は plugins/ の作品情報プラグインを wasip1 向けにビルドし、同じディレクトリの
// manifest.json を vv.plugin のカスタムセクションとして足したモジュールを書く
// （docs/how-to/work-info-plugins.md の手順 3 から 6）。
//
// 使い方: go run ./scripts/pluginpack [-o bin/plugins] plugins/wikipedia ...
// 出力は <出力先>/<マニフェストの name>.wasm である。
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/syudead/vv/internal/wasmplugin/wasmsection"
)

func main() {
	out := flag.String("o", filepath.Join("bin", "plugins"), "output directory")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: pluginpack [-o dir] plugin-dir ...")
		os.Exit(2)
	}
	for _, dir := range flag.Args() {
		path, err := pack(dir, *out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pluginpack %s: %v\n", dir, err)
			os.Exit(1)
		}
		fmt.Println(path)
	}
}

// pack は dir のプラグインをビルドしてマニフェストを足し、書いたファイルの道筋を返す。
func pack(dir, outDir string) (string, error) {
	manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return "", err
	}
	var head struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(manifest, &head); err != nil {
		return "", fmt.Errorf("read manifest.json: %w", err)
	}
	if head.Name == "" {
		return "", errors.New("manifest.json has no name")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	output, err := filepath.Abs(filepath.Join(outDir, head.Name+".wasm"))
	if err != nil {
		return "", err
	}
	cmd := exec.Command("go", "build", "-trimpath", "-o", output, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build: %w", err)
	}
	module, err := os.ReadFile(output)
	if err != nil {
		return "", err
	}
	return output, os.WriteFile(output, append(module, wasmsection.Custom("vv.plugin", manifest)...), 0o644)
}
