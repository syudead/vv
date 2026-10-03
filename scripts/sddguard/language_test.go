package sddguard

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// japaneseExempt are Markdown files whose Japanese prose is intended. The pull
// request template is the skeleton of a PR body, and PR bodies are written in
// Japanese.
var japaneseExempt = map[string]bool{
	".github/pull_request_template.md": true,
}

var fenceLine = regexp.MustCompile("^\\s*(```|~~~)")

// japaneseProse returns the line numbers of a Markdown document whose prose
// holds Japanese. Fenced code and inline code are not prose: quoted screen text
// and user-data examples are written there.
func japaneseProse(body string) []int {
	var lines []int
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		if fenceLine.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if hasJapanese(codeSpan.ReplaceAllString(line, "")) {
			lines = append(lines, i+1)
		}
	}
	return lines
}

func hasJapanese(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) ||
			(r >= 0xFF01 && r <= 0xFF60) || (r >= 0x3000 && r <= 0x303F) {
			return true
		}
	}
	return false
}

// translationRoot holds the Japanese translations of the published documents.
// They are checked by docs-site/translate/ja.mjs, not by these guards: their
// prose is Japanese, and their links mirror their English sources.
const translationRoot = "translations/"

func isTranslation(rel string) bool {
	return strings.HasPrefix(rel, translationRoot)
}

// TestDocumentsAreWrittenInEnglish fails on Japanese prose in a repository
// document. Issue and PR bodies stay Japanese; documents do not
// (docs/design-docs/writing-quality.md).
func TestDocumentsAreWrittenInEnglish(t *testing.T) {
	for _, path := range guardedMarkdown(t) {
		rel := relativeTo(t, path)
		if japaneseExempt[rel] || isTranslation(rel) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for _, n := range japaneseProse(string(body)) {
			t.Errorf("%s:%d: Japanese prose. Write documents in English; put quoted screen text or user data in inline code", rel, n)
		}
	}
}

func TestJapaneseProseDetection(t *testing.T) {
	cases := []struct {
		body string
		want int
	}{
		{"Plain English.", 0},
		{"Quote `検索` as inline code.", 0},
		{"```text\n日本語のログ\n```", 0},
		{"これは日本語です。", 1},
		{"Mixed `ok` and 日本語.", 1},
	}
	for _, c := range cases {
		if got := len(japaneseProse(c.body)); got != c.want {
			t.Errorf("japaneseProse(%q) found %d lines, want %d", c.body, got, c.want)
		}
	}
}
