package sddguard

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// numberedHeading matches a heading text that starts with a section number
// ("4. Rules", "2.1. Reads", "4) Rules"). A bare version ("2.1 API") or year
// ("2026 release notes") is a name, not a number.
var numberedHeading = regexp.MustCompile(`^\d+(\.\d+)*[.)]\s`)

// TestHeadingsAreNotNumbered fails on a heading that starts with a section
// number. The number becomes part of the anchor, so adding or removing a
// section breaks every link and "§N" reference to the sections after it
// (W-9 in docs/design-docs/writing-quality.md).
func TestHeadingsAreNotNumbered(t *testing.T) {
	for _, path := range guardedMarkdown(t) {
		rel := relativeTo(t, path)
		if isTranslation(rel) {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
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
			m := atxHeading.FindStringSubmatch(strings.TrimRight(line, "\r"))
			if m != nil && numberedHeading.MatchString(m[2]) {
				t.Errorf("%s:%d: numbered heading %q. Drop the number and refer to the section by its name", rel, i+1, m[2])
			}
		}
	}
}

func TestNumberedHeadingDetection(t *testing.T) {
	cases := map[string]bool{
		"4. Rules":               true,
		"2.1. Reads":             true,
		"4) Rules":               true,
		"2.1 API":                false,
		"0. Migration":           true,
		"Rules":                  false,
		"R-3: Glossary":          false,
		"2026 release notes":     false,
		"`GET /api/videos/{id}`": false,
		"Step 1 — set up":        false,
	}
	for in, want := range cases {
		if got := numberedHeading.MatchString(in); got != want {
			t.Errorf("numberedHeading(%q) = %v, want %v", in, got, want)
		}
	}
}
