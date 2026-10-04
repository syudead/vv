// generate は api/openapi.yaml から Go と TypeScript の型を、api/external-v1.yaml から
// 外部連携 API の Go の型を生成し、web/registry.json から shadcn の registry を組み立てる。
// task generate の実体で、-check を付けると task generate-check になる。
//
// 生成と差分確認を1つのコマンドに収めているのは、生成器の呼び出し方を
// 1か所に保つためである。別々に書くと、片方だけ版や引数を直して
// 「生成し直すと差分が出る」状態に気付けなくなる。
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

// generated は版管理に含める生成物である。手編集しない（AGENTS.md）。
var generated = []string{
	"internal/httpapi/gen/api.gen.go",
	"internal/httpapi/extgen/api.gen.go",
	"web/src/api/gen/openapi.ts",
	registryOut,
}

// registryOut は shadcn build が書く vv registry の項目である
// （specs/038-design-system/research.md R-3）。ディレクトリなので中のファイルごと要約する。
const registryOut = "web/registry/r"

// snapshot は生成物の内容を要約する。存在しないファイルは空の要約にして、
// 新しい生成物が増えたときも差分として扱えるようにする。ディレクトリは
// 中のファイルの相対パスと内容をまとめて要約するので、増減も差分になる。
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

// changed は生成の前後で内容が変わったものを、paths の順で返す。
func changed(before, after map[string]string, paths []string) []string {
	stale := []string{}
	for _, path := range paths {
		if before[path] != after[path] {
			stale = append(stale, path)
		}
	}
	return stale
}

// runner は外部コマンドを実行する。テストから差し替えて、失敗したときに
// 後続の生成器を呼ばないことを確かめる。
type runner func(dir string, name string, args ...string) error

func generate(root, oapiCodegen, openapiTypescript, shadcn string, run runner) error {
	if err := run(root, oapiCodegen,
		"-config", "api/oapi-codegen.yaml", "api/openapi.yaml"); err != nil {
		return err
	}
	// 外部連携 API は Go だけを生成する（specs/026-external-api/research.md R-5）。
	if err := run(root, oapiCodegen,
		"-config", "api/oapi-codegen-external.yaml", "api/external-v1.yaml"); err != nil {
		return err
	}
	if err := run(root, openapiTypescript,
		"api/openapi.yaml", "-o", "web/src/api/gen/openapi.ts"); err != nil {
		return err
	}
	// shadcn build は消えた項目のファイルを残すので、組み立て直す前に出力を空にする。
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

// webToolPath は web/package.json の devDependency の実行ファイルを解決する。
// shadcn は SPA の部品と同じ版で動かしたいので、tools/ ではなく web/ に置いている。
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
