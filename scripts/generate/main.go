// generate builds API bindings and the shadcn registry. The -check mode uses the same generator invocations to detect stale output.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/syudead/vv/scripts/devtools"
)

// Tracked generated files must not be edited by hand; see AGENTS.md.
var generated = []string{
	"internal/httpapi/gen/api.gen.go",
	"internal/httpapi/extgen/api.gen.go",
	"web/src/api/gen/openapi.ts",
	registryOut,
}

// Directory of registry items emitted by shadcn build.
const registryOut = "web/registry/r"

// snapshot hashes paths and contents, including directory entries, so additions and deletions count as changes; absent files have an empty hash.
func snapshot(root string, paths []string) (map[string]string, error) {
	sums := make(map[string]string, len(paths))
	for _, path := range paths {
		full := filepath.Join(root, path)
		info, err := os.Stat(full)
		if os.IsNotExist(err) {
			sums[path] = ""
			continue
		}
		if err != nil {
			return nil, err
		}
		digest := sha256.New()
		if info.IsDir() {
			err = filepath.WalkDir(full, func(file string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil || entry.IsDir() {
					return walkErr
				}
				rel, relErr := filepath.Rel(full, file)
				if relErr != nil {
					return relErr
				}
				if _, err := io.WriteString(digest, filepath.ToSlash(rel)+"\x00"); err != nil {
					return err
				}
				return hashFile(digest, file)
			})
		} else {
			err = hashFile(digest, full)
		}
		if err != nil {
			return nil, err
		}
		sums[path] = hex.EncodeToString(digest.Sum(nil))
	}
	return sums, nil
}

func hashFile(w io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(w, file)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// changed preserves the order of paths.
func changed(before, after map[string]string, paths []string) []string {
	stale := []string{}
	for _, path := range paths {
		if before[path] != after[path] {
			stale = append(stale, path)
		}
	}
	return stale
}

// Replaceable command runner for verifying that generation stops on failure.
type runner func(dir string, name string, args ...string) error

func generate(root, oapiCodegen, openapiTypescript, shadcn string, run runner) error {
	if err := run(root, oapiCodegen,
		"-config", "api/oapi-codegen.yaml", "api/openapi.yaml"); err != nil {
		return err
	}

	if err := run(root, oapiCodegen,
		"-config", "api/oapi-codegen-external.yaml", "api/external-v1.yaml"); err != nil {
		return err
	}
	if err := run(root, openapiTypescript,
		"api/openapi.yaml", "-o", "web/src/api/gen/openapi.ts"); err != nil {
		return err
	}
	// Clear stale registry entries because shadcn build does not delete removed items.
	if err := os.RemoveAll(filepath.Join(root, registryOut)); err != nil {
		return err
	}
	return run(filepath.Join(root, "web"), shadcn,
		"build", "registry.json", "--output", "registry/r")
}

func main() {
	check := flag.Bool("check", false, "生成し直しても版管理の生成物が変わらないことを確かめる")
	flag.Parse()

	root, err := devtools.RepositoryRoot()
	if err != nil {
		devtools.Fail(err)
	}
	oapiCodegen, err := devtools.GoToolPath(root, "oapi-codegen")
	if err != nil {
		devtools.Fail(err)
	}
	openapiTypescript, err := devtools.NodeToolPath(root, "openapi-typescript")
	if err != nil {
		devtools.Fail(err)
	}

	shadcn, err := webToolPath(root, "shadcn")
	if err != nil {
		devtools.Fail(err)
	}

	var before map[string]string
	if *check {
		if before, err = snapshot(root, generated); err != nil {
			devtools.Fail(err)
		}
	}

	if err := generate(root, oapiCodegen, openapiTypescript, shadcn, devtools.Run); err != nil {
		devtools.Fail(err)
	}

	if !*check {
		return
	}

	after, err := snapshot(root, generated)
	if err != nil {
		devtools.Fail(err)
	}
	stale := changed(before, after, generated)
	if len(stale) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, "生成物が古くなっていました。task generate を実行し、その結果を版管理に入れること:")
	for _, path := range stale {
		fmt.Fprintln(os.Stderr, "  "+path)
	}
	os.Exit(1)
}

// Resolve shadcn from web/package.json so its version matches the SPA components.
func webToolPath(root, name string) (string, error) {
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	path := filepath.Join(root, "web", "node_modules", ".bin", name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("web の %s を解決できません。task setup を実行してください: %w", name, err)
	}
	return path, nil
}
