package main

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/syudead/vv/scripts/internal/mdslug"
)

// A document is a sequence of pieces. A verbatim piece is copied to the
// output unchanged. A translatable piece is written as prefix + translation +
// suffix, where the translation replaces the segment's placeholders with the
// Markdown they protect.
type piece struct {
	verbatim string
	prefix   string
	seg      *segment
	suffix   string
}

// segment is the unit sent to the translator: one paragraph, list item,
// table cell, heading or front-matter value. Inline Markdown the translator
// must not change (code spans, link targets, HTML) is replaced by numbered
// tags, so identical prose shares one translation-memory entry.
type segment struct {
	text   string            // the text sent to the translator, with tags
	tokens map[string]string // tag -> the Markdown it stands for
	order  []string          // tags in source order
}

var (
	fenceLine   = regexp.MustCompile("^\\s*(```+|~~~+)")
	headingLine = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	itemLine    = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])(\s+)(\[[ xX]\]\s+)?(.*)$`)
	quoteLine   = regexp.MustCompile(`^(\s*>\s?)(.*)$`)
	alertLine   = regexp.MustCompile(`^\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`)
	tableSep    = regexp.MustCompile(`^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$`)
	frontValue  = regexp.MustCompile(`^(\s*(?:-\s+)?(?:text|tagline|title|details):\s+)(.+)$`)
	frontLink   = regexp.MustCompile(`^(\s*(?:-\s+)?link:\s+)(/\S*)\s*$`)

	imageRe       = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	linkRe        = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)(\s+"[^"]*")?\)`)
	autoLinkRe    = regexp.MustCompile(`<https?://[^>]+>`)
	autoLinkStart = regexp.MustCompile(`^<(https?://|mailto:)[^>\s]+>`)
	htmlRe        = regexp.MustCompile(`</?[A-Za-z][^>]*>`)
	urlRe         = regexp.MustCompile(`https?://[^\s)>\]]+`)
	tagRe         = regexp.MustCompile(`</?[ax]\d+/?>`)
	letterRe      = regexp.MustCompile(`\p{L}`)
)

// linkFunc rewrites a link target found in the source for the translated
// page, which lives at a different path.
type linkFunc func(target string) string

// parse splits a Markdown document into pieces.
func parse(src string, rewrite linkFunc) []piece {
	lines := strings.Split(src, "\n")
	var out []piece
	verb := func(s string) { out = append(out, piece{verbatim: s}) }
	text := func(prefix, body, suffix string) {
		out = append(out, piece{prefix: prefix, seg: protect(body, rewrite), suffix: suffix})
	}

	i := 0
	// Front matter: translate the display strings, point site links at the
	// translated pages, and keep everything else.
	if len(lines) > 0 && lines[0] == "---" {
		verb(lines[0])
		for i = 1; i < len(lines); i++ {
			l := lines[i]
			if l == "---" {
				verb(l)
				i++
				break
			}
			if m := frontLink.FindStringSubmatch(l); m != nil {
				verb(m[1] + "/ja" + m[2])
			} else if m := frontValue.FindStringSubmatch(l); m != nil {
				text(m[1], m[2], "")
			} else {
				verb(l)
			}
		}
	}

	var slugs mdslug.Slugger
	for i < len(lines) {
		l := lines[i]
		switch {
		case fenceLine.MatchString(l):
			marker := fenceLine.FindStringSubmatch(l)[1]
			verb(l)
			for i++; i < len(lines); i++ {
				verb(lines[i])
				if m := fenceLine.FindStringSubmatch(lines[i]); m != nil && strings.HasPrefix(m[1], marker[:1]) && len(m[1]) >= len(marker) {
					break
				}
			}
			i++
		case strings.HasPrefix(strings.TrimSpace(l), "<!--"):
			for ; i < len(lines); i++ {
				verb(lines[i])
				if strings.Contains(lines[i], "-->") {
					break
				}
			}
			i++
		case strings.TrimSpace(l) == "":
			verb(l)
			i++
		case headingLine.MatchString(l):
			m := headingLine.FindStringSubmatch(l)
			// Keep the English anchor, so links into the page keep working
			// after the heading text is translated.
			slug := slugs.Slug(m[2])
			body := mdslug.CustomID.ReplaceAllString(m[2], "")
			text(m[1]+" ", body, " {#"+slug+"}")
			i++
		case strings.HasPrefix(strings.TrimSpace(l), "|"):
			for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				out = append(out, tableRow(lines[i], rewrite)...)
			}
		case quoteLine.MatchString(l):
			var body []string
			prefix := quoteLine.FindStringSubmatch(l)[1]
			flush := func() {
				if len(body) > 0 {
					text(prefix, strings.Join(body, " "), "")
					body = nil
				}
			}
			for ; i < len(lines) && quoteLine.MatchString(lines[i]); i++ {
				m := quoteLine.FindStringSubmatch(lines[i])
				switch {
				case alertLine.MatchString(m[2]):
					flush()
					verb(lines[i])
				case strings.TrimSpace(m[2]) == "":
					flush()
					verb(lines[i])
				default:
					body = append(body, strings.TrimSpace(m[2]))
				}
			}
			flush()
		case htmlBlock(l):
			verb(l)
			i++
		case itemLine.MatchString(l):
			m := itemLine.FindStringSubmatch(l)
			body := []string{m[5]}
			indent := len(m[1]) + len(m[2]) + len(m[3])
			for i++; i < len(lines) && continuation(lines[i], indent); i++ {
				body = append(body, strings.TrimSpace(lines[i]))
			}
			text(m[1]+m[2]+m[3]+m[4], strings.Join(body, " "), "")
		default:
			indent := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			body := []string{strings.TrimSpace(l)}
			for i++; i < len(lines) && paragraphContinues(lines[i]); i++ {
				body = append(body, strings.TrimSpace(lines[i]))
			}
			text(indent, strings.Join(body, " "), "")
		}
	}
	return out
}

// continuation reports whether a line continues the list item above it: a
// lazy or indented line that does not start another block.
func continuation(l string, indent int) bool {
	if !paragraphContinues(l) {
		return false
	}
	lead := len(l) - len(strings.TrimLeft(l, " "))
	return !itemLine.MatchString(l) || lead >= indent+2
}

func paragraphContinues(l string) bool {
	t := strings.TrimSpace(l)
	return t != "" && !fenceLine.MatchString(l) && !headingLine.MatchString(l) &&
		!strings.HasPrefix(t, "|") && !quoteLine.MatchString(l) && !itemLine.MatchString(l) &&
		!htmlBlock(l)
}

// htmlBlock reports whether a line starts an HTML block. A line that opens
// with an autolink (<https://...>) is an ordinary paragraph.
func htmlBlock(l string) bool {
	t := strings.TrimSpace(l)
	return strings.HasPrefix(t, "<") && !autoLinkStart.MatchString(t)
}

// tableRow translates each non-empty cell of a table row.
func tableRow(l string, rewrite linkFunc) []piece {
	if tableSep.MatchString(l) {
		return []piece{{verbatim: l}}
	}
	cells := splitRow(strings.TrimSpace(l))
	var out []piece
	lead := l[:len(l)-len(strings.TrimLeft(l, " "))]
	sep := lead + "|"
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			sep += " |"
			continue
		}
		out = append(out, piece{prefix: sep + " ", seg: protect(c, rewrite)})
		sep = " |"
	}
	out = append(out, piece{verbatim: sep})
	// Join the row's pieces onto one line: only the last piece ends it.
	for k := range out[:len(out)-1] {
		out[k].suffix += "\x00"
	}
	return out
}

// splitRow splits "| a | b |" into cells, keeping pipes that are escaped or
// inside a code span.
func splitRow(row string) []string {
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	var cells []string
	var cur strings.Builder
	ticks := 0
	for k := 0; k < len(row); k++ {
		c := row[k]
		switch {
		case c == '\\' && k+1 < len(row) && row[k+1] == '|':
			cur.WriteString(`\|`)
			k++
		case c == '`':
			n := 1
			for k+n < len(row) && row[k+n] == '`' {
				n++
			}
			switch ticks {
			case 0:
				ticks = n
			case n:
				ticks = 0
			}
			cur.WriteString(row[k : k+n])
			k += n - 1
		case c == '|' && ticks == 0:
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	return append(cells, cur.String())
}

// protect replaces the inline Markdown a translator must not touch with tags.
// Link text stays translatable between <aN> and </aN>.
func protect(text string, rewrite linkFunc) *segment {
	s := &segment{tokens: map[string]string{}}
	n := 0
	hold := func(md string) string {
		tag := "<x" + strconv.Itoa(n) + "/>"
		n++
		s.tokens[tag] = md
		return tag
	}
	// Code spans first, so that brackets and URLs inside them stay code.
	text = replaceCodeSpans(text, hold)
	text = imageRe.ReplaceAllStringFunc(text, func(md string) string {
		m := regexp.MustCompile(`^(!\[[^\]]*\]\()([^)\s]*)(.*)$`).FindStringSubmatch(md)
		return hold(m[1] + rewrite(m[2]) + m[3])
	})
	text = linkRe.ReplaceAllStringFunc(text, func(md string) string {
		m := linkRe.FindStringSubmatch(md)
		id := strconv.Itoa(n)
		n++
		open, closing := "<a"+id+">", "</a"+id+">"
		s.tokens[open] = "["
		s.tokens[closing] = "](" + rewrite(m[2]) + m[3] + ")"
		return open + m[1] + closing
	})
	text = autoLinkRe.ReplaceAllStringFunc(text, hold)
	text = htmlRe.ReplaceAllStringFunc(text, func(md string) string {
		if tagRe.MatchString(md) && s.tokens[md] != "" {
			return md
		}
		return hold(md)
	})
	text = urlRe.ReplaceAllStringFunc(text, hold)
	s.text = text
	s.order = tagRe.FindAllString(text, -1)
	return s
}

// replaceCodeSpans replaces each code span (a run of backticks up to the next
// run of the same length) with f's result.
func replaceCodeSpans(text string, f func(string) string) string {
	var b strings.Builder
	for k := 0; k < len(text); {
		if text[k] != '`' {
			b.WriteByte(text[k])
			k++
			continue
		}
		n := 1
		for k+n < len(text) && text[k+n] == '`' {
			n++
		}
		ticks := text[k : k+n]
		end := -1
		for j := k + n; j < len(text); {
			if text[j] != '`' {
				j++
				continue
			}
			m := 1
			for j+m < len(text) && text[j+m] == '`' {
				m++
			}
			if m == n {
				end = j + m
				break
			}
			j += m
		}
		if end < 0 {
			b.WriteString(ticks)
			k += n
			continue
		}
		b.WriteString(f(text[k:end]))
		k = end
	}
	return b.String()
}

// needsTranslation is false for a segment with no words, such as a table cell
// holding only a code span or a number.
func (s *segment) needsTranslation() bool {
	return letterRe.MatchString(tagRe.ReplaceAllString(s.text, ""))
}

// restore puts the protected Markdown back into a translation. It fails when
// the translator dropped, duplicated or invented a tag, or closed a link
// before opening it.
func (s *segment) restore(translated string) (string, error) {
	found := tagRe.FindAllString(translated, -1)
	if len(found) != len(s.order) {
		return "", fmt.Errorf("expected %d tags, got %d", len(s.order), len(found))
	}
	seen := map[string]bool{}
	var open []string // link tags opened and not yet closed
	for _, tag := range found {
		if _, ok := s.tokens[tag]; !ok || seen[tag] {
			return "", fmt.Errorf("unexpected or repeated tag %s", tag)
		}
		seen[tag] = true
		switch {
		case strings.HasPrefix(tag, "<a"):
			open = append(open, tag)
		case strings.HasPrefix(tag, "</a"):
			// Links must nest: a closing tag ends the latest open link, so
			// interleaved links (<a0> <a1> </a0> </a1>) are rejected.
			if len(open) == 0 || open[len(open)-1] != "<a"+tag[3:] {
				return "", fmt.Errorf("tag %s does not close the innermost open link", tag)
			}
			open = open[:len(open)-1]
		}
	}
	// Normalise whitespace while the protected Markdown is still a tag, so
	// the spaces inside a code span or a link target are kept byte for byte.
	out := strings.Join(strings.Fields(translated), " ")
	out = tagRe.ReplaceAllStringFunc(out, func(tag string) string { return s.tokens[tag] })
	return out, nil
}

// render writes the document, taking each segment's translation from tr. A
// segment tr cannot translate keeps its English source.
func render(pieces []piece, tr func(*segment) (string, bool)) string {
	var b strings.Builder
	for k, p := range pieces {
		if p.seg == nil {
			b.WriteString(p.verbatim)
		} else {
			b.WriteString(p.prefix)
			text, ok := tr(p.seg)
			if !ok {
				text, _ = p.seg.restore(p.seg.text)
			}
			b.WriteString(text)
			b.WriteString(p.suffix)
		}
		if k < len(pieces)-1 {
			b.WriteString("\n")
		}
	}
	// Table rows were split into pieces; rejoin them onto their line.
	return strings.ReplaceAll(b.String(), "\x00\n", "")
}

// linkRewriter returns the rewrite for a document at srcPath (relative to the
// repository root) whose translation is written to outPath. Links between
// translated documents keep their relative form, because the translated tree
// mirrors the source tree. Every other relative link is re-anchored to the
// source location, so images and code links still resolve.
func linkRewriter(srcPath, outPath string, translated func(string) bool) linkFunc {
	return func(target string) string {
		if target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "//") ||
			regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`).MatchString(target) {
			return target
		}
		file, hash, _ := strings.Cut(target, "#")
		if hash != "" {
			hash = "#" + hash
		}
		if strings.HasPrefix(file, "/") {
			return "/ja" + file + hash
		}
		resolved := path.Clean(path.Join(path.Dir(srcPath), file))
		if translated(resolved) && path.Dir(srcPath) != "docs-site" {
			return target
		}
		rel, err := relPath(path.Dir(outPath), resolved)
		if err != nil {
			return target
		}
		if strings.HasSuffix(file, "/") {
			rel += "/"
		}
		return rel + hash
	}
}

// relPath is filepath.Rel for slash-separated repository paths.
func relPath(from, to string) (string, error) {
	f := strings.Split(path.Clean(from), "/")
	t := strings.Split(path.Clean(to), "/")
	k := 0
	for k < len(f) && k < len(t) && f[k] == t[k] {
		k++
	}
	if f[0] == ".." || t[0] == ".." {
		return "", fmt.Errorf("path %s or %s leaves the repository", from, to)
	}
	up := strings.Repeat("../", len(f)-k)
	return up + strings.Join(t[k:], "/"), nil
}
