package sddguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var featureArtifactReference = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_-])specs/[0-9]{3}-[a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+`)

// Comments, API descriptions and generated registry files also outlive the
// feature that introduced them. Check raw paths as well as Markdown links.
func TestPermanentSourcesDoNotCiteFeatureArtifacts(t *testing.T) {
	root := repositoryRoot(t)
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" || strings.HasPrefix(rel, "specs/") || strings.HasPrefix(rel, "translations/") || strings.HasPrefix(rel, "scripts/sddguard/") {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if match := featureArtifactReference.Find(body); match != nil {
			t.Errorf("%s cites temporary SDD artifact %s; use permanent design docs or remove the historical citation", rel, strings.TrimSpace(string(match)))
		}
	}
}

// Permanent documents must remain usable after a feature's working directory
// is deleted. Generic descriptions of the SDD workflow are allowed; links to
// its working artifacts are not, even while the linked file still exists.
func TestPermanentDocumentsDoNotLinkToSDDArtifacts(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range guardedMarkdown(t) {
		rel := relativeTo(t, path)
		if rel != "ARCHITECTURE.md" && rel != "README.md" && !strings.HasPrefix(rel, "docs/") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range markdownLink.FindAllStringSubmatch(string(body), -1) {
			if linksToSDDArtifact(root, path, match[1]) {
				t.Errorf("%s links to temporary SDD artifact %s; retain the lasting decision in docs/design-docs and cite that instead", rel, match[1])
			}
		}
	}
}

var remoteArtifact = regexp.MustCompile(`/specs/[^/#]+/`)

func linksToSDDArtifact(root, source, target string) bool {
	if strings.Contains(target, "://") {
		return strings.HasPrefix(target, "https://github.com/syudead/vv/") && remoteArtifact.MatchString(target)
	}
	file, _, _ := strings.Cut(target, "#")
	if file == "" {
		return false
	}
	resolved := filepath.Join(filepath.Dir(source), filepath.FromSlash(file))
	if strings.HasPrefix(file, "/") {
		resolved = filepath.Join(root, filepath.FromSlash(file))
	}
	rel, err := filepath.Rel(filepath.Join(root, "specs"), resolved)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestSDDArtifactLinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "docs", "design-docs", "example.md")
	for _, tc := range []struct {
		target string
		want   bool
	}{
		{"../../specs/active-feature/plan.md#decisions", true},
		{"/specs/active-feature/research.md", true},
		{"https://github.com/syudead/vv/blob/main/specs/active-feature/plan.md", true},
		{"https://example.org/specs/protocol/index.html", false},
		{"catalog-organization.md", false},
		{"../product-specs/spec-quality.md", false},
		{"#specs", false},
	} {
		t.Run(tc.target, func(t *testing.T) {
			if got := linksToSDDArtifact(root, source, tc.target); got != tc.want {
				t.Errorf("linksToSDDArtifact(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}
