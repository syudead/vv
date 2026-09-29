// Package mdslug computes heading anchors the way GitHub and the documentation
// site (github-slugger in docs-site/.vitepress/config.mts) do, so that the
// link checks and the translation pipeline agree with both renderers.
package mdslug

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	codeSpan = regexp.MustCompile("`+([^`]*)`+")
	link     = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	emphasis = regexp.MustCompile(`(\*\*|__|\*)`)
	htmlTag  = regexp.MustCompile(`<[^>]+>`)
	// CustomID is an explicit `{#id}` anchor at the end of a heading.
	CustomID = regexp.MustCompile(`\s*\{#([^}\s]+)\}\s*$`)
)

// Text returns the rendered text of a heading's Markdown source: code spans,
// links and emphasis reduced to their visible text.
func Text(heading string) string {
	s := CustomID.ReplaceAllString(heading, "")
	s = codeSpan.ReplaceAllString(s, "$1")
	s = link.ReplaceAllString(s, "$1")
	s = htmlTag.ReplaceAllString(s, "")
	s = emphasis.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// Slug converts rendered heading text to an anchor: lower case, punctuation
// and symbols dropped, spaces turned into hyphens.
func Slug(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.M, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Slugger hands out unique anchors within one document, suffixing repeats
// with -1, -2, ... as github-slugger does.
type Slugger struct{ seen map[string]int }

func (s *Slugger) Slug(heading string) string {
	if s.seen == nil {
		s.seen = map[string]int{}
	}
	if m := CustomID.FindStringSubmatch(heading); m != nil {
		s.seen[m[1]]++
		return m[1]
	}
	base := Slug(Text(heading))
	slug := base
	for s.seen[slug] > 0 {
		slug = base + "-" + strconv.Itoa(s.seen[base])
		s.seen[base]++
	}
	s.seen[slug]++
	return slug
}

var (
	fence   = regexp.MustCompile("^\\s*(```+|~~~+)")
	heading = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
)

// Heading is one ATX heading of a document.
type Heading struct {
	Line  int // 1-based
	Level int
	Text  string // Markdown source of the heading text
	Slug  string
}

// Headings lists the ATX headings outside fenced code blocks, with the anchor
// each one gets.
func Headings(doc string) []Heading {
	var out []Heading
	var s Slugger
	open := ""
	for i, line := range strings.Split(doc, "\n") {
		if m := fence.FindStringSubmatch(line); m != nil {
			switch {
			case open == "":
				open = m[1][:1]
			case strings.HasPrefix(m[1], open):
				open = ""
			}
			continue
		}
		if open != "" {
			continue
		}
		if m := heading.FindStringSubmatch(line); m != nil {
			out = append(out, Heading{Line: i + 1, Level: len(m[1]), Text: m[2], Slug: s.Slug(m[2])})
		}
	}
	return out
}
