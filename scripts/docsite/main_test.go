package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugifyFollowsGitHub(t *testing.T) {
	cases := map[string]string{
		"Selecting the stage":          "selecting-the-stage",
		"R-3: 切り替えの順序と時間の予算":           "r-3-切り替えの順序と時間の予算",
		"Review of the integration PR": "review-of-the-integration-pr",
		"`task check` と (docs)":        "task-check-と-docs",
		"！？":                           "section",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeadingIDsAreNumberedWhenRepeated(t *testing.T) {
	ids := newGitHubIDs()
	for _, want := range []string{"notes", "notes-1", "notes-2"} {
		if got := ids.generate("Notes"); got != want {
			t.Errorf("generate = %q, want %q", got, want)
		}
	}
}

func testRenderer() renderer {
	return renderer{
		files: newRepoFiles([]string{
			"README.md",
			"docs/how-to/README.md",
			"docs/how-to/running-vv.md",
			"docs/design-docs/index.md",
			"docs/screenshots/a.png",
			"internal/store/migrations/0001.sql",
			"api/openapi.yaml",
		}),
		github: "https://github.com/syudead/vv",
		ref:    "main",
	}
}

func TestRewriteLinks(t *testing.T) {
	r := testRenderer()
	assets := map[string]bool{}
	cases := []struct{ doc, dest, want string }{
		{"docs/how-to/README.md", "running-vv.md#network-exposure", "../../docs/how-to/running-vv.html#network-exposure"},
		{"docs/design-docs/index.md", "../how-to/", "../../docs/how-to/README.html"},
		{"README.md", "docs/screenshots/a.png", "docs/screenshots/a.png"},
		{"docs/how-to/README.md", "../../api/openapi.yaml", "https://github.com/syudead/vv/blob/main/api/openapi.yaml"},
		{"docs/how-to/README.md", "../../internal/store/migrations/", "https://github.com/syudead/vv/tree/main/internal/store/migrations"},
		{"README.md", "https://taskfile.dev/", "https://taskfile.dev/"},
		{"README.md", "#heading", "#heading"},
		{"README.md", "missing.md", "missing.md"},
	}
	for _, c := range cases {
		if got := r.rewrite(c.doc, c.dest, assets); got != c.want {
			t.Errorf("rewrite(%s, %s) = %q, want %q", c.doc, c.dest, got, c.want)
		}
	}
	if !assets["docs/screenshots/a.png"] {
		t.Errorf("image link was not recorded as an asset: %v", assets)
	}
}

func TestRenderCollectsTitleAndTOC(t *testing.T) {
	src := []byte("# Running vv\n\n## Network exposure\n\n### [Link](README.md) in heading\n\nSee [home](../../README.md).\n")
	got, err := testRenderer().render("docs/how-to/running-vv.md", src)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Running vv" {
		t.Errorf("title = %q", got.Title)
	}
	if len(got.TOC) != 2 || got.TOC[0].ID != "network-exposure" || got.TOC[1].ID != "link-in-heading" {
		t.Errorf("toc = %+v", got.TOC)
	}
	if !strings.Contains(got.HTML, `href="../../README.html"`) {
		t.Errorf("relative doc link not rewritten: %s", got.HTML)
	}
}

func TestBuildNavOpensTheCurrentBranch(t *testing.T) {
	files := testRenderer().files
	nav := buildNav(files, map[string]string{}, "docs/how-to/running-vv.md")
	if nav[0].URL != "../../README.html" {
		t.Fatalf("first entry = %+v, want the top README", nav[0])
	}
	var docs *navNode
	for _, n := range nav {
		if n.Name == "docs" {
			docs = n
		}
	}
	if docs == nil || !docs.Open {
		t.Fatalf("docs/ is not open: %+v", docs)
	}
}

// TestHandlerServesOnlyListedFiles は、一覧に無いファイル（.env など無視された
// もの）と、画像以外のソースを出さないことを確かめる。
func TestHandlerServesOnlyListedFiles(t *testing.T) {
	root := t.TempDir()
	write := func(p, body string) {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "# Home\n\n[Guide](docs/guide.md)\n")
	write("docs/guide.md", "# Guide\n")
	write("docs/shot.png", "png")
	write("main.go", "package main")
	write(".env", "SECRET=1")
	s := site{root: root, github: "https://example.com/r", ref: "main", list: func() ([]string, error) {
		return []string{"README.md", "docs/guide.md", "docs/shot.png", "main.go"}, nil
	}}
	handler := s.handler()

	cases := map[string]int{
		"/":                         http.StatusFound,
		"/docs/":                    http.StatusNotFound,
		"/README.html":              http.StatusOK,
		"/docs/guide.html":          http.StatusOK,
		"/docs/shot.png":            http.StatusOK,
		"/_docsite/style.css":       http.StatusOK,
		"/_docsite/search-index.js": http.StatusOK,
		"/.env":                     http.StatusNotFound,
		"/main.go":                  http.StatusNotFound,
		"/README.md":                http.StatusNotFound,
		"/../etc/passwd":            http.StatusNotFound,
	}
	for p, want := range cases {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", p, rec.Code, want)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/README.html", nil))
	if body := rec.Body.String(); !strings.Contains(body, `href="docs/guide.html"`) || !strings.Contains(body, "<title>Home · vv docs</title>") {
		t.Errorf("README page is missing the rewritten link or title:\n%s", body)
	}
}

func TestExportWritesPagesAndAssets(t *testing.T) {
	root := t.TempDir()
	for p, body := range map[string]string{
		"README.md":     "# Home\n\n![shot](docs/shot.png)\n",
		"docs/shot.png": "png",
	} {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := site{root: root, github: "https://example.com/r", ref: "main", list: func() ([]string, error) {
		return []string{"README.md", "docs/shot.png"}, nil
	}}
	out := filepath.Join(t.TempDir(), "site")
	if err := s.export(out); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"index.html", "README.html", "docs/shot.png", "_docsite/style.css", "_docsite/app.js", "_docsite/search-index.js"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(p))); err != nil {
			t.Errorf("%s was not written: %v", p, err)
		}
	}
}
