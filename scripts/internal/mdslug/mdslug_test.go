package mdslug

import "testing"

func TestSlugMatchesGitHub(t *testing.T) {
	cases := map[string]string{
		"R-3: The domain layer picks the encoder; the app layer caches it": "r-3-the-domain-layer-picks-the-encoder-the-app-layer-caches-it",
		"`GET /api/videos`":                     "get-apivideos",
		"6. List layout":                        "6-list-layout",
		"[Link](x.md) and **bold** text":        "link-and-bold-text",
		"snake_case & kebab-case":               "snake_case--kebab-case",
		"Heading with explicit id {#custom-id}": "custom-id",
	}
	for in, want := range cases {
		var s Slugger
		if got := s.Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSluggerSuffixesRepeats(t *testing.T) {
	var s Slugger
	got := []string{s.Slug("Notes"), s.Slug("Notes"), s.Slug("Notes")}
	want := []string{"notes", "notes-1", "notes-2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("repeats = %v, want %v", got, want)
		}
	}
}

func TestHeadingsSkipFencedBlocks(t *testing.T) {
	doc := "# Title\n\n```md\n## Not a heading\n```\n\n## Section\n"
	hs := Headings(doc)
	if len(hs) != 2 || hs[1].Slug != "section" || hs[1].Line != 7 {
		t.Fatalf("Headings = %+v", hs)
	}
}
