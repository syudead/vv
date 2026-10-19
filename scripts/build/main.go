// build embeds the SPA in a Go binary; -windows-app creates the Windows ZIP instead.
package main

import (
	"errors"
	"flag"
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

// cleanDist preserves the tracked .gitkeep so interrupted builds and builds without the SPA retain a valid embed target. Vite emptyOutDir would remove it.
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
	windowsApp := flag.Bool("windows-app", false, "build the Windows app zip into dist/")
	faceRuntime := flag.String("face-runtime", "", "build the face runtime archive for a platform (linux-amd64, linux-arm64, windows-amd64) into dist/")
	faceSmoke := flag.String("face-runtime-smoke", "", "run the helper of a built Linux face runtime archive in Alpine on the fixture photo")
	static := flag.Bool("check-static", false, "fail when the Linux mdm has an INTERP header")
	flag.Parse()

	root, err := devtools.RepositoryRoot()
	if err != nil {
		devtools.Fail(err)
	}
	// These builds do not embed the SPA.
	if *faceRuntime != "" || *faceSmoke != "" || *static {
		if err := runWithoutSPA(root, *faceRuntime, *faceSmoke, *static); err != nil {
			devtools.Fail(err)
		}
		return
	}
	if err := cleanDist(root); err != nil {
		devtools.Fail(err)
	}
	if err := devtools.Run(root, "npm", "--prefix", "web", "run", "build"); err != nil {
		devtools.Fail(err)
	}
	if *windowsApp {
		out, err := buildWindowsApp(root, os.Getenv("VERSION"))
		if err != nil {
			devtools.Fail(err)
		}
		fmt.Printf("Built %s\n", out)
		return
	}

	version := os.Getenv("VERSION")
	if version == "" {
		version = "dev"
	}
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		devtools.Fail(err)
	}
	// Disable CGO so distributed binaries do not depend on the host libc.
	if err := devtools.RunEnv(root, []string{"CGO_ENABLED=0"}, "go", "build",
		"-trimpath", "-ldflags", "-s -w -X main.version="+version,
		"-o", outputPath, "./cmd/mdm"); err != nil {
		devtools.Fail(err)
	}

	fmt.Printf("Built %s (version=%s)\n", outputPath, version)
}

func runWithoutSPA(root, faceRuntime, faceSmoke string, static bool) error {
	switch {
	case static:
		return checkStatic(root)
	case faceSmoke != "":
		return smokeFaceRuntime(root, faceSmoke)
	}
	out, err := buildFaceRuntime(root, faceRuntime)
	if err != nil {
		return err
	}
	sum, err := sha256File(out)
	if err != nil {
		return err
	}
	fmt.Printf("Built %s (sha256 %s)\n", out, sum)
	return nil
}
