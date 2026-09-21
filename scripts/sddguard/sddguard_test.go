// Package sddguard holds repository guards for the SDD artifacts under specs/
// and docs/. It contains no runtime code: the rules in
// docs/design-docs/plan-quality.md and docs/product-specs/spec-quality.md are
// prose, and these tests turn the mechanically checkable part of them into a
// failure instead of a convention.
package sddguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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

// guardedMarkdown は web/ の外にある、版管理された Markdown を返す。prettier は
// `npm --prefix web run format:check` で web/ だけを整形検査するので、
// specs/ と docs/ とリポジトリ直下はどの検査にもかかっていない。
//
// 走査ではなく git の追跡一覧を使う。ディレクトリを辿ると、無視されている作業用の
// チェックアウト（エージェントが作る .claude/worktrees/ など）まで拾い、そこにある
// 古い内容を現在の違反として報告してしまう。
func guardedMarkdown(t *testing.T) []string {
	t.Helper()
	repoRoot := repositoryRoot(t)

	out, err := exec.Command("git", "-C", repoRoot, "ls-files", "-z", "--", "*.md").Output()
	if err != nil {
		t.Fatalf("版管理された Markdown を一覧できない: %v", err)
	}

	var files []string
	for _, rel := range strings.Split(string(out), "\x00") {
		if rel == "" || strings.HasPrefix(rel, "web/") {
			continue
		}
		files = append(files, filepath.Join(repoRoot, rel))
	}
	if len(files) == 0 {
		t.Fatal("Markdown を1つも拾えていない。一覧の条件を確認すること")
	}
	return files
}

// TestMarkdownHasNoTrailingWhitespace は行末の空白を落とす。PR #72 では
// PR 本文に `git diff --check` 済みと書いてあったが実際には違反しており、
// レビューが指摘して1往復かかった。
func TestMarkdownHasNoTrailingWhitespace(t *testing.T) {
	for _, path := range guardedMarkdown(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s を読めない: %v", path, err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			if line != strings.TrimRight(line, " \t") {
				t.Errorf("%s:%d: 行末に空白がある。取り除くこと", relativeTo(t, path), i+1)
			}
		}
	}
}

// markdownLink は Markdown の相対リンクを拾う。外部 URL と純粋な anchor は除く。
var markdownLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)

// TestMarkdownRelativeLinksResolve は解決できない相対リンクを落とす。PR #76 は
// 実行計画を active/ から completed/ へ移した結果、参照元のリンクが切れていた。
func TestMarkdownRelativeLinksResolve(t *testing.T) {
	for _, path := range guardedMarkdown(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s を読めない: %v", path, err)
		}
		for _, match := range markdownLink.FindAllStringSubmatch(string(body), -1) {
			target := match[1]
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
				strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			if i := strings.Index(target, "#"); i >= 0 {
				target = target[:i]
			}
			if target == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), target)); err != nil {
				t.Errorf("%s: リンク先 %s が見つからない。移動したなら参照元も更新すること",
					relativeTo(t, path), target)
			}
		}
	}
}

func relativeTo(t *testing.T, path string) string {
	t.Helper()
	rel, err := filepath.Rel(repositoryRoot(t), path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// TestCheckAllCoversEveryCIMakeTarget は、CI が呼ぶ make 目標がすべて
// check-all でも実行されることを確かめる。
//
// Makefile の冒頭は「CI でしか動かない検査を作らない」と定めているが、
// 守っているかを見るものが無かった。実際、migrations-check を CI へ足した
// ときに check へ繋ぎ忘れ、手元では push するまで気づけない状態にした。
// 目標名を人が2箇所へ書く限り同じことが起きるので、突き合わせる。
//
// 目標の依存関係は make 自身に解決させる。Makefile を読み直すと、書き方の
// 違い（前提条件とレシピ内の再帰呼び出し）で取りこぼす。
func TestCheckAllCoversEveryCIMakeTarget(t *testing.T) {
	repoRoot := repositoryRoot(t)

	workflow, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("CI の定義を読めない: %v", err)
	}
	calls := regexp.MustCompile(`\bmake ([a-z0-9-]+)`).FindAllStringSubmatch(string(workflow), -1)
	if len(calls) == 0 {
		t.Fatal("CI から make の呼び出しを1つも読み取れていない。走査の条件を確認すること")
	}

	covered := map[string]bool{}
	for _, cmd := range dryRun(t, repoRoot, "check-all") {
		covered[cmd] = true
	}
	if len(covered) == 0 {
		t.Fatal("check-all が実行する命令を1つも読み取れていない")
	}

	// 検査ではない目標は対象外にする。check-all 自身も含める必要はない。
	notAChecked := map[string]bool{"check-all": true, "generate": true, "up": true, "setup": true}

	seen := map[string]bool{}
	for _, call := range calls {
		target := call[1]
		if notAChecked[target] || seen[target] {
			continue
		}
		seen[target] = true

		for _, cmd := range dryRun(t, repoRoot, target) {
			if covered[cmd] {
				continue
			}
			t.Errorf("CI は make %s を呼ぶが、check-all はその命令を実行しない:\n  %s\n"+
				"手元で同じ判定ができないので、check か check-all へ繋ぐこと", target, cmd)
			break
		}
	}
}

// dryRun は目標が実行する命令を make に列挙させる。報告を安定させるため
// 並びを固定して返す。
func dryRun(t *testing.T, repoRoot, target string) []string {
	t.Helper()

	out, err := exec.Command("make", "-C", repoRoot, "--dry-run", target).Output()
	if err != nil {
		t.Fatalf("make --dry-run %s を実行できない: %v", target, err)
	}

	var cmds []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// make 自身の進捗表示は命令ではない。
		if line == "" || strings.HasPrefix(line, "make[") {
			continue
		}
		cmds = append(cmds, line)
	}
	sort.Strings(cmds)
	return cmds
}
