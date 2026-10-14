package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// fileKind はテストを読むファイルの種類である。
type fileKind int

const (
	otherFile fileKind = iota
	goTestFile
	webUnitFile
	e2eFile
)

// kindOf はパスからファイルの種類を決める。
func kindOf(path string) fileKind {
	switch {
	case strings.HasSuffix(path, "_test.go"):
		return goTestFile
	case strings.HasPrefix(path, "web/e2e/") && strings.HasSuffix(path, ".e2e.ts"):
		return e2eFile
	case strings.HasPrefix(path, "web/src/") && (strings.HasSuffix(path, ".test.ts") || strings.HasSuffix(path, ".test.tsx")):
		return webUnitFile
	default:
		return otherFile
	}
}

// addedLines は、base とのマージベースから作業ツリーまでに足された行を、
// テストのファイルごとに返す。コミット前のファイルも、まだ追跡されていない
// ファイルも含める（実装中のエージェントがコミット前に見るため）。
func addedLines(base string) (map[string][]string, error) {
	mergeBase, err := git("merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	diff, err := git("diff", "--unified=0", "--no-color", "--no-renames", strings.TrimSpace(mergeBase), "--")
	if err != nil {
		return nil, err
	}
	added := parseDiff(diff)

	untracked, err := git("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, path := range strings.Split(strings.TrimSpace(untracked), "\n") {
		if kindOf(path) == otherFile {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		added[path] = strings.Split(string(body), "\n")
	}
	return added, nil
}

// parseDiff は unified diff から、テストのファイルごとに足された行を取り出す。
func parseDiff(diff string) map[string][]string {
	added := map[string][]string{}
	file := ""
	scanner := bufio.NewScanner(strings.NewReader(diff))
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
			if kindOf(file) == otherFile {
				file = ""
			}
		case file != "" && strings.HasPrefix(line, "+"):
			added[file] = append(added[file], strings.TrimPrefix(line, "+"))
		}
	}
	return added
}

func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
