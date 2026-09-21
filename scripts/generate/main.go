// generate は api/openapi.yaml から Go と TypeScript の型を生成する。
// task generate の実体で、-check を付けると task generate-check になる。
//
// 生成と差分確認を1つのコマンドに収めているのは、生成器の呼び出し方を
// 1か所に保つためである。別々に書くと、片方だけ版や引数を直して
// 「生成し直すと差分が出る」状態に気付けなくなる。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/syudead/vv/scripts/devtools"
)

// generated は版管理に含める生成物である。手編集しない（AGENTS.md）。
var generated = []string{
	"internal/httpapi/gen/api.gen.go",
	"web/src/api/gen/openapi.ts",
}

// toolVersions は scripts/tool-versions.json の必要な部分である。
type toolVersions struct {
	OapiCodegen       string `json:"oapiCodegen"`
	OpenapiTypescript string `json:"openapiTypescript"`
}

func readToolVersions(root string) (toolVersions, error) {
	var versions toolVersions
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "tool-versions.json"))
	if err != nil {
		return versions, fmt.Errorf("scripts/tool-versions.json を読めません: %w", err)
	}
	if err := json.Unmarshal(raw, &versions); err != nil {
		return versions, fmt.Errorf("scripts/tool-versions.json を解釈できません: %w", err)
	}
	if versions.OapiCodegen == "" || versions.OpenapiTypescript == "" {
		return versions, fmt.Errorf("scripts/tool-versions.json に生成器の版がありません")
	}
	return versions, nil
}

// snapshot は生成物の内容を要約する。存在しないファイルは空の要約にして、
// 新しい生成物が増えたときも差分として扱えるようにする。
func snapshot(root string, paths []string) (map[string]string, error) {
	sums := make(map[string]string, len(paths))
	for _, path := range paths {
		file, err := os.Open(filepath.Join(root, path))
		if os.IsNotExist(err) {
			sums[path] = ""
			continue
		}
		if err != nil {
			return nil, err
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		sums[path] = hex.EncodeToString(digest.Sum(nil))
	}
	return sums, nil
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

func generate(root string, versions toolVersions, run runner) error {
	if err := run(root, "go", "run",
		"github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@"+versions.OapiCodegen,
		"-config", "api/oapi-codegen.yaml", "api/openapi.yaml"); err != nil {
		return err
	}
	return run(root, "npx", "--yes",
		"openapi-typescript@"+versions.OpenapiTypescript,
		"api/openapi.yaml", "-o", "web/src/api/gen/openapi.ts")
}

func main() {
	check := flag.Bool("check", false, "生成し直しても版管理の生成物が変わらないことを確かめる")
	flag.Parse()

	root, err := devtools.RepositoryRoot()
	if err != nil {
		devtools.Fail(err)
	}
	versions, err := readToolVersions(root)
	if err != nil {
		devtools.Fail(err)
	}

	var before map[string]string
	if *check {
		if before, err = snapshot(root, generated); err != nil {
			devtools.Fail(err)
		}
	}

	if err := generate(root, versions, devtools.Run); err != nil {
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
