package sddguard

import (
	"bufio"
	"os"
	"path/filepath"
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

// pendingList names the documents that still have to be rewritten in English.
const pendingList = "scripts/sddguard/japanese-pending.txt"

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

func readPending(t *testing.T) map[string]bool {
	t.Helper()
	pending := map[string]bool{}
	f, err := os.Open(filepath.Join(repositoryRoot(t), pendingList))
	if os.IsNotExist(err) {
		return pending
	}
	if err != nil {
		t.Fatalf("read %s: %v", pendingList, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Errorf("close %s: %v", pendingList, err)
		}
	}()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			pending[line] = true
		}
	}
	return pending
}

// TestDocumentsAreWrittenInEnglish fails on Japanese prose in a repository
// document. Issue and PR bodies stay Japanese; documents do not
// (docs/design-docs/writing-quality.md).
func TestDocumentsAreWrittenInEnglish(t *testing.T) {
	pending := readPending(t)
	seen := map[string]bool{}
	for _, path := range guardedMarkdown(t) {
		rel := relativeTo(t, path)
		if japaneseExempt[rel] {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		lines := japaneseProse(string(body))
		if pending[rel] {
			seen[rel] = true
			if len(lines) == 0 {
				t.Errorf("%s has no Japanese prose left. Remove it from %s", rel, pendingList)
			}
			continue
		}
		for _, n := range lines {
			t.Errorf("%s:%d: Japanese prose. Write documents in English; put quoted screen text or user data in inline code", rel, n)
		}
	}
	for rel := range pending {
		if !seen[rel] {
			t.Errorf("%s is listed in %s but is not a tracked Markdown file. Remove the entry", rel, pendingList)
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
