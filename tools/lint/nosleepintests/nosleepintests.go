// Package nosleepintests reports time.Sleep in tests outside a testing/synctest bubble, because a
// wall-clock wait does not prove another goroutine reached a state (docs/design-docs/testing-time.md).
package nosleepintests

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const message = "time.Sleep in a test waits on the wall clock: synchronize on the state under test, or sleep " +
	"inside a synctest.Test callback, where time is virtual (docs/design-docs/testing-time.md)"

var Analyzer = &analysis.Analyzer{
	Name:     "nosleepintests",
	Doc:      "reports time.Sleep in test files outside a testing/synctest Test callback",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	walk := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	walk.WithStack([]ast.Node{(*ast.Ident)(nil)}, func(node ast.Node, push bool, stack []ast.Node) bool {
		if !push || !isFunc(pass, node.(*ast.Ident), "time", "Sleep") {
			return true
		}
		if strings.HasSuffix(pass.Fset.Position(node.Pos()).Filename, "_test.go") && !inBubble(pass, stack) {
			pass.Reportf(node.Pos(), "%s", message)
		}
		return true
	})
	return nil, nil
}

// A function literal passed to synctest.Test runs on the bubble's virtual clock.
func inBubble(pass *analysis.Pass, stack []ast.Node) bool {
	for i := len(stack) - 1; i > 0; i-- {
		literal, ok := stack[i].(*ast.FuncLit)
		if !ok {
			continue
		}
		call, ok := stack[i-1].(*ast.CallExpr)
		if !ok || !passesArgument(call, literal) {
			continue
		}
		if name := calledIdent(call.Fun); name != nil && isFunc(pass, name, "testing/synctest", "Test") {
			return true
		}
	}
	return false
}

func passesArgument(call *ast.CallExpr, literal *ast.FuncLit) bool {
	for _, arg := range call.Args {
		if arg == literal {
			return true
		}
	}
	return false
}

func calledIdent(fun ast.Expr) *ast.Ident {
	switch fun := fun.(type) {
	case *ast.Ident:
		return fun
	case *ast.SelectorExpr:
		return fun.Sel
	}
	return nil
}

func isFunc(pass *analysis.Pass, ident *ast.Ident, pkg, name string) bool {
	fn, ok := pass.TypesInfo.Uses[ident].(*types.Func)
	return ok && fn.Name() == name && fn.Pkg() != nil && fn.Pkg().Path() == pkg
}
