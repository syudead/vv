package sddguard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

var (
	atxHeading     = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)[ \t#]*$`)
	explicitAnchor = regexp.MustCompile(`[ \t]*\{#([^}\s]+)\}$`)
	htmlAnchor     = regexp.MustCompile(`<a\s+(?:id|name)="([^"]+)"`)
	inlineLink     = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	emphasis       = regexp.MustCompile("[*`~]")
	htmlTag        = regexp.MustCompile(`<[^>]+>`)
)

// slugify follows GitHub's heading anchors (github-slugger): lowercase, drop
// everything but letters, marks, numbers, spaces, hyphens and underscores, and
// turn spaces into hyphens.
func slugify(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

func headingText(raw string) string {
	text := inlineLink.ReplaceAllString(raw, "$1")
	text = htmlTag.ReplaceAllString(text, "")
	return emphasis.ReplaceAllString(text, "")
}

// anchorsOf returns every fragment a Markdown document defines: GitHub slugs of
// its headings (with -1, -2 for repeats), explicit {#id} anchors, and HTML
// anchors.
func anchorsOf(body string) map[string]bool {
	anchors := map[string]bool{}
	counts := map[string]int{}
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if fenceLine.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range htmlAnchor.FindAllStringSubmatch(line, -1) {
			anchors[m[1]] = true
		}
		m := atxHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := m[2]
		if e := explicitAnchor.FindStringSubmatch(text); e != nil {
			anchors[e[1]] = true
			continue
		}
		slug := slugify(headingText(text))
		if n := counts[slug]; n > 0 {
			anchors[fmt.Sprintf("%s-%d", slug, n)] = true
		} else {
			anchors[slug] = true
		}
		counts[slug]++
	}
	return anchors
}

var fragmentLink = regexp.MustCompile(`\]\(([^)\s]*#[^)\s]+)\)`)

// TestMarkdownLinkAnchorsResolve fails on a link whose #fragment matches no
// heading in the target document. Rewriting a heading changes its anchor, and
// the inbound links have to move with it.
func TestMarkdownLinkAnchorsResolve(t *testing.T) {
	cache := map[string]map[string]bool{}
	for _, path := range guardedMarkdown(t) {
		if isTranslation(relativeTo(t, path)) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		inFence := false
		for i, line := range strings.Split(string(body), "\n") {
			if fenceLine.MatchString(line) {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			for _, m := range fragmentLink.FindAllStringSubmatch(codeSpan.ReplaceAllString(line, ""), -1) {
				target := m[1]
				if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				hash := strings.Index(target, "#")
				file, fragment := target[:hash], target[hash+1:]
				targetPath := path
				if file != "" {
					targetPath = filepath.Join(filepath.Dir(path), file)
				}
				if !strings.HasSuffix(targetPath, ".md") {
					continue
				}
				anchors, ok := cache[targetPath]
				if !ok {
					content, err := os.ReadFile(targetPath)
					if err != nil {
						continue // TestMarkdownRelativeLinksResolve reports the missing file
					}
					anchors = anchorsOf(string(content))
					cache[targetPath] = anchors
				}
				if !anchors[strings.ToLower(fragment)] && !anchors[fragment] {
					t.Errorf("%s:%d: anchor #%s matches no heading in %s. Point the link at the current heading",
						relativeTo(t, path), i+1, fragment, relativeTo(t, targetPath))
				}
			}
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"R-4: Where translations live, and how they follow the English source": "r-4-where-translations-live-and-how-they-follow-the-english-source",
		"The site's published set":  "the-sites-published-set",
		"`ui-design.md` の文言":        "ui-designmd-の文言",
		"Step 1 — set up":           "step-1--set-up",
		"[Link](x.md) and **bold**": "link-and-bold",
	}
	for in, want := range cases {
		if got := slugify(headingText(in)); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
