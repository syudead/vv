package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
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

// addedLine は足された 1 行と、変更後のファイルでの行番号である。
type addedLine struct {
	N    int
	Text string
}

// hunkHeader は unified diff の範囲の行から、変更後の開始行を取り出す。
var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)`)

// addedLines は、base とのマージベースから作業ツリーまでに足された行を、
// テストのファイルごとに返す。コミット前のファイルも、まだ追跡されていない
// ファイルも含める（実装中のエージェントがコミット前に見るため）。
func addedLines(base string) (map[string][]addedLine, error) {
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
		added[path] = wholeFile(string(body))
	}
	return added, nil
}

// wholeFile は追跡されていないファイルの全行を、足された行として返す。
func wholeFile(body string) []addedLine {
	var lines []addedLine
	for i, text := range strings.Split(body, "\n") {
		lines = append(lines, addedLine{N: i + 1, Text: text})
	}
	return lines
}

// parseDiff は unified diff（--unified=0）から、テストのファイルごとに足された行と
// その行番号を取り出す。
func parseDiff(diff string) map[string][]addedLine {
	added := map[string][]addedLine{}
	file := ""
	next := 0
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
		case hunkHeader.MatchString(line):
			next, _ = strconv.Atoi(hunkHeader.FindStringSubmatch(line)[1])
		case file != "" && strings.HasPrefix(line, "+"):
			added[file] = append(added[file], addedLine{N: next, Text: strings.TrimPrefix(line, "+")})
			next++
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

func sortedKeys(m map[string][]addedLine) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
