package sddguard

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/syudead/vv/scripts/internal/mdslug"
)

// The rules below come from docs/design-docs/writing-style.md. They keep the
// documents in one language, so the translation pipeline has a single source,
// and keep prose from growing into walls of text.

const (
	maxParagraphWords  = 90
	maxItemWords       = 70
	maxProseParagraphs = 4
)

// templatesDir holds the document templates. Their links and placeholders are
// written for the file's destination, so the checks below skip them.
const templatesDir = "docs/templates/"

var (
	headingRe     = regexp.MustCompile(`^(#{1,6})\s+`)
	itemRe        = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s+`)
	linkTargetRe  = regexp.MustCompile(`\]\([^)]*\)`)
	placeholderRe = regexp.MustCompile(`\{\{[^}]*\}\}`)
)

// proseLine is one line of a document outside fenced blocks and HTML
// comments, with its code spans removed.
type proseLine struct {
	number int
	raw    string
	text   string
}

func proseLines(body string) []proseLine {
	var out []proseLine
	inFence, inComment := false, false
	for i, line := range strings.Split(body, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
			out = append(out, proseLine{number: i + 1, raw: line})
			continue
		}
		if inFence {
			continue
		}
		if strings.Contains(line, "<!--") {
			inComment = true
		}
		if inComment {
			if strings.Contains(line, "-->") {
				inComment = false
			}
			continue
		}
		out = append(out, proseLine{number: i + 1, raw: line, text: codeSpan.ReplaceAllString(line, "")})
	}
	return out
}

func isJapanese(r rune) bool {
	return unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) ||
		(r >= 0xFF01 && r <= 0xFF60) // full-width punctuation and letters
}

func styledMarkdown(t *testing.T) []string {
	var files []string
	for _, path := range guardedMarkdown(t) {
		if !strings.HasPrefix(relativeTo(t, path), templatesDir) {
			files = append(files, path)
		}
	}
	return files
}

// TestDocumentsAreEnglish fails on Japanese prose. Japanese inside a code span
// or a fenced block is data (a fixture name, a legacy heading being matched)
// and stays allowed.
func TestDocumentsAreEnglish(t *testing.T) {
	for _, path := range guardedMarkdown(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, line := range proseLines(string(body)) {
			if strings.IndexFunc(line.text, isJapanese) >= 0 {
				t.Errorf("%s:%d: Japanese outside a code span. Documents are written in English; the Japanese site is generated (docs/design-docs/writing-style.md)",
					relativeTo(t, path), line.number)
			}
		}
	}
}

// TestProseStaysScannable enforces the paragraph limits of writing-style.md.
func TestProseStaysScannable(t *testing.T) {
	for _, path := range styledMarkdown(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		rel := relativeTo(t, path)
		for _, problem := range proseProblems(string(body)) {
			t.Errorf("%s:%s. Split it, or turn it into a list or a table (docs/design-docs/writing-style.md)", rel, problem)
		}
	}
}

type block struct {
	kind  string // "para", "item" or "other"
	start int
	words int
}

func proseProblems(body string) []string {
	var blocks []block
	var cur *block
	for _, line := range proseLines(body) {
		trimmed := strings.TrimSpace(line.raw)
		switch {
		case fence.MatchString(line.raw):
			blocks = append(blocks, block{kind: "other", start: line.number})
			cur = nil
		case trimmed == "":
			cur = nil
		case headingRe.MatchString(line.raw), strings.HasPrefix(trimmed, "|"),
			strings.HasPrefix(trimmed, ">"), strings.HasPrefix(trimmed, "<"):
			blocks = append(blocks, block{kind: "other", start: line.number})
			cur = nil
		case itemRe.MatchString(line.raw):
			blocks = append(blocks, block{kind: "item", start: line.number, words: words(line.text)})
			cur = &blocks[len(blocks)-1]
		case cur != nil:
			cur.words += words(line.text)
		default:
			blocks = append(blocks, block{kind: "para", start: line.number, words: words(line.text)})
			cur = &blocks[len(blocks)-1]
		}
	}

	var problems []string
	run := 0
	for _, b := range blocks {
		if b.kind != "para" {
			run = 0
		}
		switch b.kind {
		case "para":
			run++
			if b.words > maxParagraphWords {
				problems = append(problems, lineMsg(b.start, "paragraph of %d words (max %d)", b.words, maxParagraphWords))
			}
			if run > maxProseParagraphs {
				problems = append(problems, lineMsg(b.start, "%d prose paragraphs in a row (max %d)", run, maxProseParagraphs))
			}
		case "item":
			if b.words > maxItemWords {
				problems = append(problems, lineMsg(b.start, "list item of %d words (max %d)", b.words, maxItemWords))
			}
		}
	}
	return problems
}

func words(text string) int {
	return len(strings.Fields(linkTargetRe.ReplaceAllString(text, "]")))
}

func lineMsg(line int, format string, args ...any) string {
	return fmt.Sprintf("%d: ", line) + fmt.Sprintf(format, args...)
}

// TestMarkdownAnchorsResolve fails on a link whose #fragment names no heading
// of the target document. Renaming a heading changes its anchor, and the
// translation pipeline keeps the English anchors, so a stale one breaks both
// editions.
func TestMarkdownAnchorsResolve(t *testing.T) {
	anchors := map[string]map[string]bool{}
	anchorsOf := func(path string) map[string]bool {
		if a, ok := anchors[path]; ok {
			return a
		}
		body, err := os.ReadFile(path)
		if err != nil {
			anchors[path] = nil
			return nil
		}
		a := map[string]bool{}
		for _, h := range mdslug.Headings(string(body)) {
			a[h.Slug] = true
		}
		anchors[path] = a
		return a
	}
	for _, path := range styledMarkdown(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var prose strings.Builder
		for _, line := range proseLines(string(body)) {
			prose.WriteString(line.text + "\n")
		}
		for _, match := range markdownLink.FindAllStringSubmatch(prose.String(), -1) {
			target, fragment, ok := strings.Cut(match[1], "#")
			if !ok || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			file := path
			if target != "" {
				file = filepath.Join(filepath.Dir(path), target)
			}
			if !strings.HasSuffix(file, ".md") {
				continue
			}
			if decoded, err := url.PathUnescape(fragment); err == nil {
				fragment = decoded
			}
			a := anchorsOf(file)
			if a != nil && !a[fragment] {
				t.Errorf("%s: %s has no heading with the anchor #%s. Update the link to the heading's current anchor",
					relativeTo(t, path), relativeTo(t, file), fragment)
			}
		}
	}
}

// TestTemplatePlaceholdersAreFilled fails on a {{...}} placeholder copied from
// docs/templates/ and left in a delivered document.
func TestTemplatePlaceholdersAreFilled(t *testing.T) {
	for _, path := range styledMarkdown(t) {
		rel := relativeTo(t, path)
		if !strings.HasPrefix(rel, "docs/") && !strings.HasPrefix(rel, "specs/") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, line := range proseLines(string(body)) {
			if m := placeholderRe.FindString(line.text); m != "" {
				t.Errorf("%s:%d: template placeholder %s was left unfilled. Fill it, or delete the section", rel, line.number, m)
			}
		}
	}
}

func TestProseProblemsCountsBlocks(t *testing.T) {
	long := strings.Repeat("word ", maxParagraphWords+1)
	cases := map[string]int{
		"# Title\n\n" + long + "\n":                    1, // long paragraph
		"- " + strings.Repeat("word ", maxItemWords+1): 1, // long item
		"a\n\nb\n\nc\n\nd\n\ne\n":                      1, // five paragraphs in a row
		"a\n\nb\n\n- item\n\nc\n\nd\n\ne\n":            0, // a list breaks the run
		"```\n" + long + "\n```\n":                     0, // code is not prose
		"a `" + long + "` b\n":                         0, // neither is a code span
	}
	for body, want := range cases {
		if got := proseProblems(body); len(got) != want {
			t.Errorf("proseProblems(%.40q) = %v, want %d problems", body, got, want)
		}
	}
}
