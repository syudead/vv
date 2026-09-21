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
	outputPath = "bin/mdm"
)

// preserveDist は web/dist を退避してから fn を実行し、必ず元へ戻す。
//
// web/dist は Go の埋め込み先で、版管理には .gitkeep と index.html の
// 置き場所だけを入れている（.gitignore）。ビルドはそこへ本番の資材を書くので、
// 戻さないと作業ツリーが生成物で汚れたままになる。
func preserveDist(root string, fn func() error) (err error) {
	dist := filepath.Join(root, distPath)
	backupRoot, err := os.MkdirTemp(filepath.Join(root, ".local"), "build-dist-backup-")
	if err != nil {
		return fmt.Errorf("退避先を作れません: %w", err)
	}
	backupDist := filepath.Join(backupRoot, "dist")

	hadDist := true
	if _, statErr := os.Stat(dist); errors.Is(statErr, os.ErrNotExist) {
		hadDist = false
	} else if statErr != nil {
		return statErr
	}
	if hadDist {
		if err := os.Rename(dist, backupDist); err != nil {
			return fmt.Errorf("%s を退避できません: %w", distPath, err)
		}
	}

	defer func() {
		restoreErr := restoreDist(dist, backupDist, backupRoot, hadDist)
		if err == nil {
			err = restoreErr
		}
	}()

	return fn()
}

func restoreDist(dist, backupDist, backupRoot string, hadDist bool) error {
	if err := os.RemoveAll(dist); err != nil {
		return fmt.Errorf("%s を片付けられません: %w", distPath, err)
	}
	if hadDist {
		if err := os.Rename(backupDist, dist); err != nil {
			return fmt.Errorf("%s を戻せません: %w", distPath, err)
		}
	}
	return os.RemoveAll(backupRoot)
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
	if err := os.MkdirAll(filepath.Join(root, ".local"), 0o755); err != nil {
		devtools.Fail(err)
	}

	err = preserveDist(root, func() error {
		if err := devtools.Run(root, "npm", "--prefix", "web", "run", "build"); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
			return err
		}
		// CGO を切るのは、配布するバイナリを実行環境の libc から独立させるため
		// （Dockerfile の実行段は alpine）。
		return devtools.RunEnv(root, []string{"CGO_ENABLED=0"}, "go", "build",
			"-trimpath", "-ldflags", "-s -w -X main.version="+version,
			"-o", outputPath, "./cmd/mdm")
	})
	if err != nil {
		devtools.Fail(err)
	}

	fmt.Printf("Built %s (version=%s)\n", outputPath, version)
}
