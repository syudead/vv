// Package englishcomments reports Go comments written in Japanese prose, because AGENTS.md
// ("Code comments") keeps code comments in English.
package englishcomments

import (
	"go/ast"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"
)

const message = "write this comment in English and quote Japanese values in backticks (AGENTS.md \"Code comments\")"

var Analyzer = &analysis.Analyzer{
	Name: "englishcomments",
	Doc:  "reports comments with Japanese prose outside backticks",
	Run:  run,
}

// A directive such as //go:generate or //nolint:all is tool input, not prose.
var directive = regexp.MustCompile(`^//[a-z0-9]+:[a-z0-9]`)

// Inline code may quote Japanese values such as screen text or test data.
var inlineCode = regexp.MustCompile("`[^`]*`")

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}
		for _, group := range file.Comments {
			if comment := firstJapanese(group); comment != nil {
				pass.Reportf(comment.Pos(), "%s", message)
			}
		}
	}
	return nil, nil
}

func firstJapanese(group *ast.CommentGroup) *ast.Comment {
	for _, comment := range group.List {
		if directive.MatchString(comment.Text) {
			continue
		}
		if isJapanese(inlineCode.ReplaceAllString(comment.Text, "")) {
			return comment
		}
	}
	return nil
}

// isJapanese reports Japanese prose: an ideographic comma or full stop, or two Hiragana in a row.
// Kanji or Katakana alone may be a quoted name, so they do not count.
func isJapanese(text string) bool {
	if strings.ContainsAny(text, "、。") {
		return true
	}
	previous := false
	for _, r := range text {
		hiragana := unicode.Is(unicode.Hiragana, r)
		if hiragana && previous {
			return true
		}
		previous = hiragana
	}
	return false
}
