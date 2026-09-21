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

	// 退避先を作った直後に後始末を登録する。途中で返っても空の退避先を
	// 残さないため。戻す側の失敗も握り潰さない —— 版管理された
	// web/dist/index.html が退避先にしか無い状態で黙って終わると、
	// 利用者は作業ツリーが欠けたことに気付けない。
	moved, existed := false, true
	defer func() {
		err = errors.Join(err, restoreDist(dist, backupDist, backupRoot, moved, existed))
	}()

	if _, statErr := os.Stat(dist); errors.Is(statErr, os.ErrNotExist) {
		existed = false
	} else if statErr != nil {
		return statErr
	}
	if existed {
		if err := os.Rename(dist, backupDist); err != nil {
			return fmt.Errorf("%s を退避できません: %w", distPath, err)
		}
		moved = true
	}

	return fn()
}

// restoreDist は退避した web/dist を戻し、退避先を片付ける。退避できなかった
// ときは元の web/dist がそのまま残っているので、触らない。
//
// 戻せなかったときは退避先を消さない。版管理された web/dist/index.html と
// .gitkeep はその時点で退避先にしか無く、消すと原状回復の手段が無くなる。
func restoreDist(dist, backupDist, backupRoot string, moved, existed bool) error {
	switch {
	case moved:
		if err := os.RemoveAll(dist); err != nil {
			return fmt.Errorf("%s を片付けられません。退避したものは %s にある: %w", distPath, backupDist, err)
		}
		if err := os.Rename(backupDist, dist); err != nil {
			return fmt.Errorf("%s を戻せません。退避したものは %s にある: %w", distPath, backupDist, err)
		}
	case !existed:
		// 退避するものが無かった場合。fn が作った出力だけを片付ける。
		if err := os.RemoveAll(dist); err != nil {
			return err
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
