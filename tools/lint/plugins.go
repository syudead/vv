// Package lint registers this repository's analyzers as golangci-lint module plugins; the golangci-lint
// command in tools/cmd imports it (docs/how-to/development.md#lint-rules).
package lint

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/syudead/vv/tools/lint/englishcomments"
	"github.com/syudead/vv/tools/lint/nosleepintests"
)

func init() {
	register.Plugin(englishcomments.Analyzer.Name, newPlugin(englishcomments.Analyzer, register.LoadModeSyntax))
	register.Plugin(nosleepintests.Analyzer.Name, newPlugin(nosleepintests.Analyzer, register.LoadModeTypesInfo))
}

type plugin struct {
	analyzer *analysis.Analyzer
	loadMode string
}

// The analyzers take no settings.
func newPlugin(analyzer *analysis.Analyzer, loadMode string) register.NewPlugin {
	return func(any) (register.LinterPlugin, error) {
		return plugin{analyzer: analyzer, loadMode: loadMode}, nil
	}
}

func (p plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{p.analyzer}, nil
}

func (p plugin) GetLoadMode() string {
	return p.loadMode
}
