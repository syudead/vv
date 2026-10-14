package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// goTestDecl は足された行のうち、トップレベルのテスト関数の宣言に一致する。
var goTestDecl = regexp.MustCompile(`^func (Test\w+)\(\w+ \*testing\.T\)`)

// 呼び出しの名前から、テストが用意する重い資源を見分ける。名前はパッケージの
// テストファイルにある補助関数をたどった先まで含める（newGuestFixture →
// newAuthEnvWith → store.Open のように）。
var (
	ffmpegCalls   = []string{"requireFFmpeg", "requireLibx264", "requireEncoder", "encodeScene", "exec.LookPath"}
	databaseCalls = []string{"migratedDB", "store.Open", "store.OpenContext", "store.Migrate"}
	// internal/store の中では Open・Migrate は修飾なしで呼ばれる。
	storePackageCalls = []string{"Open", "OpenContext", "Migrate"}
	httpCalls         = []string{"newTestServer", "NewRouter", "httptest.NewRecorder", "httptest.NewServer"}
)

// goTests は 1 つの _test.go で足されたテスト関数を、パッケージの補助関数を
// たどって分かる資源からレベルを付けて返す。
func goTests(file string, lines []string) ([]*addedTest, error) {
	var names []string
	for _, line := range lines {
		if m := goTestDecl.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	dir := filepath.Dir(file)
	calls, err := packageCalls(dir)
	if err != nil {
		return nil, err
	}
	tests := make([]*addedTest, 0, len(names))
	for _, name := range names {
		// 文字列の中の "func Test..." の行は宣言ではない。
		if _, declared := calls[name]; !declared {
			continue
		}
		reached := reach(calls, name)
		tests = append(tests, &addedTest{
			File: file, Name: name, Package: dir,
			Level: goLevel(dir, reached),
		})
	}
	return tests, nil
}

// goLevel は、テストが届く呼び出しからレベルを決める。重い資源を優先する。
func goLevel(dir string, reached map[string]bool) level {
	usesDB := hasAny(reached, databaseCalls) ||
		(filepath.ToSlash(dir) == "internal/store" && hasAny(reached, storePackageCalls))
	switch {
	case hasAny(reached, ffmpegCalls):
		return levelMedia
	case usesDB && hasAny(reached, httpCalls):
		return levelHTTP
	case usesDB:
		return levelStore
	default:
		return levelLogic
	}
}

// packageCalls は、dir のテストファイルにある関数ごとに、その本体が呼ぶ名前を返す。
// 修飾付きの呼び出しは "pkg.Name"、修飾なしは "Name" として記録する。
func packageCalls(dir string) (map[string][]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	calls := map[string][]string{}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				calls[funcKey(fn)] = append(calls[funcKey(fn)], calledNames(fn.Body)...)
			}
		}
	}
	return calls, nil
}

// funcKey は関数の名前を返す。メソッドも名前だけで引く。
func funcKey(fn *ast.FuncDecl) string {
	return fn.Name.Name
}

// calledNames は node の中の呼び出しの名前を集める。x.Name の形の呼び出しは
// "x.Name" と、補助のメソッドをたどるための ".Name" の 2 つで記録する。
// 修飾なしの "Name" と分けるのは、internal/store で os.Open を Open と取り違えないため。
func calledNames(node ast.Node) []string {
	var names []string
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			names = append(names, fun.Name)
		case *ast.SelectorExpr:
			if x, ok := fun.X.(*ast.Ident); ok {
				names = append(names, x.Name+"."+fun.Sel.Name)
			}
			names = append(names, "."+fun.Sel.Name)
		}
		return true
	})
	return names
}

// reach は start から、パッケージのテストファイルにある関数をたどって届く
// 呼び出しの名前をすべて返す。
func reach(calls map[string][]string, start string) map[string]bool {
	reached := map[string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, called := range calls[name] {
			if reached[called] {
				continue
			}
			reached[called] = true
			if _, ok := calls[strings.TrimPrefix(called, ".")]; ok {
				queue = append(queue, strings.TrimPrefix(called, "."))
			}
		}
	}
	return reached
}

func hasAny(set map[string]bool, names []string) bool {
	for _, name := range names {
		if set[name] {
			return true
		}
	}
	return false
}
