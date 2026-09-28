// nolintguard は Go のソース（テストを含む）に //nolint が無いことを確かめる。
// task lint-go の一部として動く。
//
// golangci-lint は //nolint を書いた箇所の指摘を黙らせる。検査が全体に例外なく
// 効くように、指摘はコードの書き換えで解消し、無効化の記述そのものを認めない
// （syudead/vv#443）。golangci-lint には無効化を拒む設定が無いので、ここで拒む。
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// directive は golangci-lint が無効化として読むコメントの本文に一致する。
// 行頭の空白や大文字小文字の違いも拒む側に倒す。
var directive = regexp.MustCompile(`(?i)^\s*nolint\b`)

// skippedDirs は Go のソースを持たない、または手元の依存物を置く場所である。
var skippedDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	found, err := findDirectives(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nolintguard: %v\n", err)
		os.Exit(2)
	}
	for _, f := range found {
		fmt.Fprintln(os.Stderr, f)
	}
	if len(found) > 0 {
		fmt.Fprintf(os.Stderr, "nolintguard: %d //nolint directive(s) found. Fix the code instead of disabling the linter.\n", len(found))
		os.Exit(1)
	}
}

// findDirectives は root 配下の .go ファイルを読み、//nolint の位置を返す。
// 文字列の中の "//nolint" は無効化にならないので、コメントだけを見る。
func findDirectives(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, group := range file.Comments {
			for _, c := range group.List {
				text := strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), "/*")
				if directive.MatchString(text) {
					found = append(found, fmt.Sprintf("%s: %s", fset.Position(c.Slash), c.Text))
				}
			}
		}
		return nil
	})
	return found, err
}
