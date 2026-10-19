package main

// Check that mdm stays a static binary that starts on Alpine; see docs/design-docs/face-recognition.md.

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"

	"github.com/syudead/vv/scripts/devtools"
)

// staticArchitectures are the Docker image's platforms.
var staticArchitectures = []string{"amd64", "arm64"}

// checkStatic builds the Linux mdm for each image architecture and fails when one asks for a dynamic loader.
func checkStatic(root string) error {
	dir, err := os.MkdirTemp("", "mdm-static-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for _, arch := range staticArchitectures {
		out := filepath.Join(dir, "mdm-"+arch)
		if err := devtools.RunEnv(root, []string{"GOOS=linux", "GOARCH=" + arch, "CGO_ENABLED=0"},
			"go", "build", "-trimpath", "-o", out, "./cmd/mdm"); err != nil {
			return err
		}
		if err := requireNoInterp(out); err != nil {
			return err
		}
		fmt.Printf("linux/%s mdm has no INTERP header\n", arch)
	}
	return nil
}

// requireNoInterp fails for an ELF file with an INTERP program header: such a binary does not start
// where that loader is missing, as glibc's is on Alpine.
func requireNoInterp(name string) error {
	f, err := elf.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			return fmt.Errorf("%s has an INTERP header and needs a dynamic loader; something made mdm link libc (cgo, or purego outside cmd/mdm-face)", name)
		}
	}
	return nil
}
