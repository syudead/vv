// changedcoverage lists the changed lines that no test executes, from the Go and Vitest coverage reports under reports/.
// It reads the same reports CI sends to Codecov, so an agent sees the uncovered changed lines before it pushes.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const modulePrefix = "github.com/syudead/vv/"

// Paths Codecov does not measure (.github/codecov.yml ignore and web/vite.config.ts coverage.exclude).
var ignoredPaths = regexp.MustCompile(`(^|/)(gen|extgen|testdata)/|^internal/store/storetest\.go$|^web/src/(designSystem|testing)/|_test\.go$|\.test\.tsx?$`)

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// lineCounts maps a file to the execution count of each statement line.
type lineCounts map[string]map[int]int

// fileReport is the result for one changed file in coverage scope.
type fileReport struct {
	path      string
	measured  bool
	statement int
	uncovered []int
}

func main() {
	base := flag.String("base", "origin/main", "ref whose merge base with HEAD the working tree is compared against")
	goProfile := flag.String("go", "reports/go-coverage.out", "Go coverprofile")
	lcov := flag.String("web", "reports/web-coverage/lcov.info", "Vitest lcov report")
	flag.Parse()

	changed, err := changedLines(*base)
	if err != nil {
		fmt.Fprintf(os.Stderr, "changedcoverage: %v\n", err)
		os.Exit(2)
	}
	counts := lineCounts{}
	for _, r := range []struct {
		path  string
		parse func(io.Reader, lineCounts) error
	}{{*goProfile, parseGoProfile}, {*lcov, parseLCOV}} {
		if err := readReport(r.path, r.parse, counts); err != nil {
			fmt.Fprintf(os.Stderr, "changedcoverage: %v\n", err)
		}
	}
	out, code := formatReports(compare(changed, counts))
	fmt.Print(out)
	os.Exit(code)
}

func readReport(path string, parse func(io.Reader, lineCounts) error, counts lineCounts) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w (run task test-go-report or task test-web-report first)", err)
	}
	return parse(bytes.NewReader(data), counts)
}

// changedLines returns the added lines of tracked changes since the merge base and of untracked files.
func changedLines(base string) (map[string][]int, error) {
	mergeBase, err := gitOutput("merge-base", base, "HEAD")
	if err != nil {
		return nil, err
	}
	diff, err := gitOutput("diff", "-U0", "--no-color", "--no-ext-diff", "--diff-filter=AMR", strings.TrimSpace(mergeBase), "--")
	if err != nil {
		return nil, err
	}
	changed := parseDiff(strings.NewReader(diff))
	untracked, err := gitOutput("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, path := range strings.Split(strings.TrimSpace(untracked), "\n") {
		if !inScope(path) {
			continue
		}
		n, err := countLines(path)
		if err != nil {
			return nil, err
		}
		for line := 1; line <= n; line++ {
			changed[path] = append(changed[path], line)
		}
	}
	return changed, nil
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	n := strings.Count(string(data), "\n")
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n, nil
}

// parseDiff reads a zero-context unified diff and returns the added line numbers of each new file path.
func parseDiff(r io.Reader) map[string][]int {
	changed := map[string][]int{}
	path := ""
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		text := scanner.Text()
		if name, ok := strings.CutPrefix(text, "+++ "); ok {
			path = strings.TrimPrefix(name, "b/")
			if name == "/dev/null" {
				path = ""
			}
			continue
		}
		m := hunkHeader.FindStringSubmatch(text)
		if m == nil || path == "" {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		n := 1
		if m[2] != "" {
			n, _ = strconv.Atoi(m[2])
		}
		for line := start; line < start+n; line++ {
			changed[path] = append(changed[path], line)
		}
	}
	return changed
}

// parseGoProfile adds the statement lines of each coverprofile block; a line takes the highest count of the blocks spanning it.
func parseGoProfile(r io.Reader, counts lineCounts) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "mode:") || text == "" {
			continue
		}
		var start, startCol, end, endCol, statements, count int
		file, rest, ok := strings.Cut(text, ":")
		if !ok {
			return fmt.Errorf("go profile: malformed line %q", text)
		}
		if _, err := fmt.Sscanf(rest, "%d.%d,%d.%d %d %d", &start, &startCol, &end, &endCol, &statements, &count); err != nil {
			return fmt.Errorf("go profile: malformed line %q: %w", text, err)
		}
		path := strings.TrimPrefix(file, modulePrefix)
		for line := start; line <= end; line++ {
			counts.add(path, line, count)
		}
	}
	return scanner.Err()
}

// parseLCOV adds the DA lines of an lcov report, mapping Vitest's web-relative paths to repository paths.
func parseLCOV(r io.Reader, counts lineCounts) error {
	scanner := bufio.NewScanner(r)
	path := ""
	for scanner.Scan() {
		text := scanner.Text()
		if name, ok := strings.CutPrefix(text, "SF:"); ok {
			path = lcovPath(name)
			continue
		}
		da, ok := strings.CutPrefix(text, "DA:")
		if !ok || path == "" {
			continue
		}
		lineText, countText, _ := strings.Cut(da, ",")
		countText, _, _ = strings.Cut(countText, ",")
		line, err1 := strconv.Atoi(lineText)
		count, err2 := strconv.Atoi(countText)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("lcov: malformed line %q", text)
		}
		counts.add(path, line, count)
	}
	return scanner.Err()
}

func lcovPath(name string) string {
	name = filepath.ToSlash(name)
	if filepath.IsAbs(name) {
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, name); err == nil {
				return filepath.ToSlash(rel)
			}
		}
	}
	if strings.HasPrefix(name, "src/") {
		return "web/" + name
	}
	return name
}

func (c lineCounts) add(path string, line, count int) {
	lines := c[path]
	if lines == nil {
		lines = map[int]int{}
		c[path] = lines
	}
	if prev, ok := lines[line]; !ok || count > prev {
		lines[line] = count
	}
}

// inScope reports whether Codecov measures the path, matching the go and web flags of .github/codecov.yml.
func inScope(path string) bool {
	if ignoredPaths.MatchString(path) {
		return false
	}
	switch {
	case strings.HasSuffix(path, ".go"):
		return strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/")
	case strings.HasSuffix(path, ".ts"), strings.HasSuffix(path, ".tsx"):
		return strings.HasPrefix(path, "web/src/") && !strings.HasSuffix(path, ".d.ts")
	}
	return false
}

// compare returns, for each changed file in scope, its changed statement lines that ran zero times.
func compare(changed map[string][]int, counts lineCounts) []fileReport {
	var reports []fileReport
	for path, lines := range changed {
		if !inScope(path) {
			continue
		}
		report := fileReport{path: path}
		fileCounts, measured := counts[path]
		if !measured && strings.HasSuffix(path, ".go") && !hasGoStatements(path) {
			continue
		}
		report.measured = measured
		for _, line := range lines {
			count, statement := fileCounts[line]
			if !statement {
				continue
			}
			report.statement++
			if count == 0 {
				report.uncovered = append(report.uncovered, line)
			}
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].path < reports[j].path })
	return reports
}

// hasGoStatements reports whether a Go file has a statement go test can count; a file of declarations only never appears in a coverprofile.
func hasGoStatements(path string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return true
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if block, ok := n.(*ast.BlockStmt); ok && len(block.List) > 0 {
			found = true
		}
		return !found
	})
	return found
}

// formatReports lists the uncovered changed lines and returns the exit code: 1 when any changed line is unmeasured or uncovered.
func formatReports(reports []fileReport) (string, int) {
	var w strings.Builder
	statements, uncovered := 0, 0
	var unmeasured []string
	for _, r := range reports {
		if !r.measured {
			unmeasured = append(unmeasured, r.path)
			continue
		}
		statements += r.statement
		uncovered += len(r.uncovered)
		if len(r.uncovered) > 0 {
			fmt.Fprintf(&w, "%s: %s\n", r.path, lineRanges(r.uncovered))
		}
	}
	for _, path := range unmeasured {
		fmt.Fprintf(&w, "%s: not in the coverage reports\n", path)
	}
	fmt.Fprintf(&w, "changed statement lines: %d, uncovered: %d, files not measured: %d\n", statements, uncovered, len(unmeasured))
	if uncovered > 0 || len(unmeasured) > 0 {
		return w.String(), 1
	}
	return w.String(), 0
}

// lineRanges formats sorted line numbers as "3-5, 9".
func lineRanges(lines []int) string {
	sort.Ints(lines)
	var parts []string
	for i := 0; i < len(lines); {
		j := i
		for j+1 < len(lines) && lines[j+1] == lines[j]+1 {
			j++
		}
		if i == j {
			parts = append(parts, strconv.Itoa(lines[i]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", lines[i], lines[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}
