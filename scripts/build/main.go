// build は SPA を web/dist へ出して Go の単一バイナリに埋め込む。
// task build の実体。
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/syudead/vv/scripts/devtools"
)

const (
	distPath   = "web/dist"
	keepFile   = ".gitkeep"
	outputPath = "bin/mdm"
)

// cleanDist はビルドの出力先から前回の生成物だけを消す。
//
// web/dist は Go の埋め込み先で、版管理に入れているのは .gitkeep だけである
// （.gitignore）。Vite の emptyOutDir は .git 以外を区別せず消すので切ってあり、
// 代わりにここで掃除する。.gitkeep を残したまま横へ書くので、途中で中断されても
// 作業ツリーから版管理のファイルが消えることはない。
func cleanDist(root string) error {
	dist := filepath.Join(root, distPath)
	entries, err := os.ReadDir(dist)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dist, 0o755)
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == keepFile {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dist, entry.Name())); err != nil {
			return fmt.Errorf("%s の前回の生成物を消せません: %w", distPath, err)
		}
	}
	return nil
}

func main() {
	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}

	root, err := devtools.RepositoryRoot()
	if err != nil {
		devtools.Fail(err)
	}
	if err := cleanDist(root); err != nil {
		devtools.Fail(err)
	}
	if err := devtools.Run(root, "npm", "--prefix", "web", "run", "build"); err != nil {
		devtools.Fail(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		devtools.Fail(err)
	}
	// CGO を切るのは、配布するバイナリを実行環境の libc から独立させるため
	// （Dockerfile の実行段は alpine）。
	if err := devtools.RunEnv(root, []string{"CGO_ENABLED=0"}, "go", "build",
		"-trimpath", "-ldflags", "-s -w -X main.version="+version,
		"-o", outputPath, "./cmd/mdm"); err != nil {
		devtools.Fail(err)
	}

	fmt.Printf("Built %s (version=%s)\n", outputPath, version)
}
