// Package sddguard holds repository guards for the SDD artifacts under specs/
// and docs/. It contains no runtime code: the rules in
// docs/design-docs/plan-quality.md and docs/product-specs/spec-quality.md are
// prose, and these tests turn the mechanically checkable part of them into a
// failure instead of a convention.
package sddguard

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve sddguard source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("verify repository root %s: %v", root, err)
	}
	return root
}

// removedArtifacts are files the Tasks stage used to own. The workflow replaced
// that stage with the Implementation Work section of plan.md
// (docs/exec-plans/active/011-agent-agnostic-issue-handoff.md), so their
// reappearance means the stage is creeping back in.
var removedArtifacts = []string{
	".specify/templates/tasks-template.md",
	".specify/scripts/bash/setup-tasks.sh",
	"specs/003-sdd-loop-harness/tasks-retired-routine.md",
}

func TestTasksStageArtifactsAreAbsent(t *testing.T) {
	repoRoot := repositoryRoot(t)
	for _, rel := range removedArtifacts {
		if _, err := os.Stat(filepath.Join(repoRoot, rel)); err == nil {
			t.Errorf("%s exists. The Tasks stage was removed; implementation work belongs in the Implementation Work section of plan.md", rel)
		} else if !os.IsNotExist(err) {
			t.Errorf("cannot verify that %s is absent: %v", rel, err)
		}
	}

	prerequisitesPath := filepath.Join(repoRoot, ".specify", "scripts", "bash", "check-prerequisites.sh")
	prerequisites, err := os.ReadFile(prerequisitesPath)
	if err != nil {
		t.Fatalf("read check-prerequisites.sh: %v", err)
	}
	for _, obsolete := range []string{"--require-tasks", "--include-tasks", "/speckit-tasks"} {
		if strings.Contains(string(prerequisites), obsolete) {
			t.Errorf("check-prerequisites.sh contains removed Tasks-stage reference %q", obsolete)
		}
	}

	specsDir := filepath.Join(repoRoot, "specs")
	err = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		if d.Name() == "tasks.md" {
			t.Errorf("%s exists. Implementation work belongs in the Implementation Work section of plan.md, which /speckit-plan-to-issues turns into child Issues", filepath.ToSlash(rel))
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), "/speckit-tasks") {
			t.Errorf("%s contains the removed /speckit-tasks workflow", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect specs for removed Tasks workflow: %v", err)
	}
}

// placeholders are fragments the templates carry for the author to replace.
// Finding one in a delivered artifact means a section was shipped unfilled.
//
// `[NEEDS CLARIFICATION: ...]` is deliberately not on this list. It is a
// legitimate state, not residue: /speckit-clarify records a product ambiguity
// it could not ask this round as such a marker, so the spec carries it between
// rounds. Whether a marker may still be open is a question about the stage the
// feature is in, which this repository-wide check cannot see.
var placeholders = []string{
	"[REMOVE IF UNUSED]",
	"[e.g.,",
	"[Assumption about",
	"[Measurable metric",
	"[Child Issue title]",
	"[FEATURE]",
	"[###-feature-name]",
	"[DATE]",
	"[PRINCIPLE_",
}

var (
	fence    = regexp.MustCompile("^\\s*```")
	codeSpan = regexp.MustCompile("`[^`]*`")
	skipDirs = map[string]bool{"assets": true}
)

// TestSpecArtifactsHaveNoTemplatePlaceholders reads every markdown file under
// specs/ and fails on a placeholder left outside a code span or fenced block,
// where an artifact quotes a marker to discuss it rather than to leave one.
func TestSpecArtifactsHaveNoTemplatePlaceholders(t *testing.T) {
	repoRoot := repositoryRoot(t)
	specsDir := filepath.Join(repoRoot, "specs")
	err := filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		inFence := false
		for i, line := range strings.Split(string(body), "\n") {
			if fence.MatchString(line) {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			stripped := codeSpan.ReplaceAllString(line, "")
			for _, marker := range placeholders {
				if strings.Contains(stripped, marker) {
					t.Errorf("%s:%d: template placeholder %q was delivered unfilled. Fill the section, or remove it when it does not apply", filepath.ToSlash(path), i+1, marker)
				}
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk specs/: %v", err)
	}
}

// indexedDirs must each link every sibling document from their index, which is
// what AGENTS.md asks for when a document is added.
var indexedDirs = []string{"docs/design-docs", "docs/product-specs"}

func TestDocumentsAreLinkedFromTheirIndex(t *testing.T) {
	repoRoot := repositoryRoot(t)
	for _, dir := range indexedDirs {
		indexPath := filepath.Join(repoRoot, dir, "index.md")
		index, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatalf("read %s: %v", dir+"/index.md", err)
		}
		entries, err := os.ReadDir(filepath.Join(repoRoot, dir))
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || name == "index.md" || !strings.HasSuffix(name, ".md") {
				continue
			}
			if !strings.Contains(string(index), "]("+name+")") {
				t.Errorf("%s/%s is not linked from %s/index.md. Add it to the index so the document can be found", dir, name, dir)
			}
		}
	}
}
