package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

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

// goTests は 1 つの _test.go で、足された行を含むトップレベルのテスト関数を、
// パッケージの補助関数をたどって分かる資源からレベルを付けて返す。既存のテストへ
// subtest や表の行を足しただけの変更も、そのテストとして数える。
func goTests(file string, lines []addedLine) ([]*addedTest, error) {
	touched, err := touchedTests(file, lines)
	if err != nil || len(touched) == 0 {
		return nil, err
	}
	dir := filepath.Dir(file)
	calls, err := packageCalls(dir)
	if err != nil {
		return nil, err
	}
	tests := make([]*addedTest, 0, len(touched))
	for _, t := range touched {
		t.Package = dir
		t.Level = goLevel(dir, reach(calls, t.Name))
		tests = append(tests, t)
	}
	return tests, nil
}

// touchedTests は file のトップレベルのテスト関数のうち、足された行を含むものを
// 宣言の順に返す。宣言の行が足されていなければ、既存のテストへの追加（Changed）とする。
func touchedTests(file string, lines []addedLine) ([]*addedTest, error) {
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	added := map[int]bool{}
	for _, line := range lines {
		added[line.N] = true
	}
	var tests []*addedTest
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		first, last := fset.Position(fn.Pos()).Line, fset.Position(fn.End()).Line
		if anyIn(added, first, last) {
			tests = append(tests, &addedTest{File: file, Name: fn.Name.Name, Changed: !added[first]})
		}
	}
	return tests, nil
}

func anyIn(lines map[int]bool, first, last int) bool {
	for n := first; n <= last; n++ {
		if lines[n] {
			return true
		}
	}
	return false
}

// goLevel は、テストが届く呼び出しからレベルを決める。重い資源を優先する。
func goLevel(dir string, reached map[string]bool) level {
	usesDB := hasAny(reached, databaseCalls) ||
		(filepath.ToSlash(dir) == "internal/store" && hasAny(reached, storePackageCalls))
	switch {
	case hasAny(reached, ffmpegCalls):
		return levelMedia
	case hasAny(reached, httpCalls):
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
