// qualitymetrics はコードとテストと CI の品質の指標を測り、Markdown のまとめを書く。
// task quality-metrics の実体で、CI は main への push ごとに流して履歴を quality-metrics ブランチに積む。
//
// 測るもの:
//   - 循環複雑度: Go は関数ごとに go/ast で数え、TS は eslint の complexity ルールで出す
//   - 重複: jscpd が数える重複行の割合
//   - CI の所要時間: main への push で成功した CI の、job ごとと実行全体の p50・p90
//
// カバレッジとテストごとの所要時間は Codecov が追うので、ここでは測らない。
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// complexityLimit を超える関数を「複雑すぎる関数」として数える（gocyclo の慣例の 15）。
const complexityLimit = 15

// skippedDirs は数えないディレクトリ。生成コードは手で書いたコードの複雑さではない。
var skippedDirs = map[string]bool{
	".git": true, ".claude": true, ".local": true, "node_modules": true,
	"worktrees": true, "reports": true, "gen": true, "extgen": true,
}

// Entry は 1 回の計測で、履歴の 1 行になる。
type Entry struct {
	Date        string                `json:"date"`
	Commit      string                `json:"commit,omitempty"`
	Complexity  map[string]Complexity `json:"complexity"`
	Duplication *Duplication          `json:"duplication,omitempty"`
	CI          *CIDurations          `json:"ci,omitempty"`
}

// Complexity は 1 つの言語の関数ごとの循環複雑度のまとめ。
type Complexity struct {
	Functions int        `json:"functions"`
	Mean      float64    `json:"mean"`
	P90       float64    `json:"p90"`
	Max       int        `json:"max"`
	OverLimit int        `json:"overLimit"`
	Top       []Function `json:"top"`
}

// Function は複雑度の高い関数の 1 つ。
type Function struct {
	Name       string `json:"name"`
	Location   string `json:"location"`
	Complexity int    `json:"complexity"`
}

// Duplication は jscpd の数えた重複。
type Duplication struct {
	Percentage float64            `json:"percentage"`
	Lines      int                `json:"lines"`
	Clones     int                `json:"clones"`
	ByFormat   map[string]float64 `json:"byFormat"`
}

// CIDurations は CI の所要時間（秒）。
type CIDurations struct {
	Runs int                    `json:"runs"`
	Wall Percentiles            `json:"wall"`
	Jobs map[string]Percentiles `json:"jobs"`
}

// Percentiles は所要時間の分布。
type Percentiles struct {
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
}

func main() {
	root := flag.String("root", ".", "repository root")
	history := flag.String("history", "", "JSON Lines file to compare with and append to (optional)")
	summary := flag.String("summary", "", "Markdown summary output (default: stdout)")
	ciRuns := flag.Int("ci-runs", 20, "recent successful CI runs on main to read with gh (0 skips)")
	skipWeb := flag.Bool("skip-web", false, "skip the eslint and jscpd measurements")
	flag.Parse()

	if err := run(*root, *history, *summary, *ciRuns, *skipWeb); err != nil {
		fmt.Fprintf(os.Stderr, "qualitymetrics: %v\n", err)
		os.Exit(1)
	}
}

func run(root, historyPath, summaryPath string, ciRuns int, skipWeb bool) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	reports := filepath.Join(root, "reports")
	if err := os.MkdirAll(reports, 0o755); err != nil {
		return err
	}

	entry := Entry{
		Date:       time.Now().UTC().Format("2006-01-02"),
		Commit:     gitHead(root),
		Complexity: map[string]Complexity{},
	}

	goFuncs, err := goComplexity(root)
	if err != nil {
		return fmt.Errorf("go complexity: %w", err)
	}
	entry.Complexity["go"] = summarize(goFuncs)

	if !skipWeb {
		tsFuncs, err := tsComplexity(root, reports)
		if err != nil {
			return fmt.Errorf("ts complexity: %w", err)
		}
		entry.Complexity["ts"] = summarize(tsFuncs)

		dup, err := duplication(root, reports)
		if err != nil {
			return fmt.Errorf("duplication: %w", err)
		}
		entry.Duplication = dup
	}

	if ciRuns > 0 {
		runs, err := fetchCIRuns(root, ciRuns)
		if err != nil {
			return fmt.Errorf("ci durations: %w", err)
		}
		entry.CI = ciDurations(runs)
	}

	var previous *Entry
	if historyPath != "" {
		previous, err = lastEntry(historyPath)
		if err != nil {
			return err
		}
	}

	out := os.Stdout
	if summaryPath != "" {
		f, err := os.OpenFile(summaryPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		out = f
	}
	if _, err := out.WriteString(markdown(entry, previous)); err != nil {
		return err
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(reports, "quality-metrics.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if historyPath != "" {
		f, err := os.OpenFile(historyPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		if _, err := f.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// goComplexity は root 配下のテストでない Go の関数の循環複雑度を返す。
func goComplexity(root string) ([]Function, error) {
	var funcs []Function
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			funcs = append(funcs, Function{
				Name:       funcName(fn),
				Location:   fmt.Sprintf("%s:%d", filepath.ToSlash(rel), fset.Position(fn.Pos()).Line),
				Complexity: cyclomatic(fn.Body),
			})
		}
		return nil
	})
	return funcs, err
}

// cyclomatic は gocyclo と同じ数え方で、1 に分岐の数を足す。関数リテラルの分岐は
// それを含む関数に数える。
func cyclomatic(body ast.Node) int {
	n := 1
	ast.Inspect(body, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			n++
		case *ast.CaseClause:
			if x.List != nil {
				n++
			}
		case *ast.CommClause:
			if x.Comm != nil {
				n++
			}
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				n++
			}
		}
		return true
	})
	return n
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if idx, ok := recv.(*ast.IndexExpr); ok {
		recv = idx.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// complexityMessage は eslint の complexity ルールの指摘から名前と値を取り出す。
var complexityMessage = regexp.MustCompile(`^(.*) has a complexity of (\d+)`)

// tsComplexity は eslint の complexity ルールを最大 0 で流し、web/src のすべての関数の
// 複雑度を集める。ほかのルールの設定は web/eslint.config.js のまま使う。
func tsComplexity(root, reports string) ([]Function, error) {
	out := filepath.Join(reports, "eslint-complexity.json")
	cmd := exec.Command("npx", "eslint", "--rule", "complexity: [warn, 0]", "--format", "json", "--output-file", out, "src")
	cmd.Dir = filepath.Join(root, "web")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// ほかのルールの指摘でも 1 で終わる。結果のファイルがあれば複雑度は読める。
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return nil, err
		}
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	return parseESLint(data, filepath.Join(root, "web"))
}

func parseESLint(data []byte, webDir string) ([]Function, error) {
	var files []struct {
		FilePath string `json:"filePath"`
		Messages []struct {
			RuleID  string `json:"ruleId"`
			Message string `json:"message"`
			Line    int    `json:"line"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, err
	}
	var funcs []Function
	for _, f := range files {
		rel, err := filepath.Rel(webDir, f.FilePath)
		if err != nil {
			rel = f.FilePath
		}
		if slash := filepath.ToSlash(rel); strings.Contains(slash, ".test.") || strings.HasPrefix(slash, "src/api/gen/") || strings.HasPrefix(slash, "src/testing/") {
			continue
		}
		for _, m := range f.Messages {
			if m.RuleID != "complexity" {
				continue
			}
			match := complexityMessage.FindStringSubmatch(m.Message)
			if match == nil {
				continue
			}
			var c int
			_, _ = fmt.Sscan(match[2], &c)
			funcs = append(funcs, Function{
				Name:       match[1],
				Location:   fmt.Sprintf("web/%s:%d", filepath.ToSlash(rel), m.Line),
				Complexity: c,
			})
		}
	}
	return funcs, nil
}

func summarize(funcs []Function) Complexity {
	if len(funcs) == 0 {
		return Complexity{}
	}
	values := make([]float64, len(funcs))
	total, over := 0, 0
	for i, f := range funcs {
		values[i] = float64(f.Complexity)
		total += f.Complexity
		if f.Complexity > complexityLimit {
			over++
		}
	}
	sorted := slices.Clone(funcs)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Complexity > sorted[j].Complexity })
	top := sorted[:min(5, len(sorted))]
	return Complexity{
		Functions: len(funcs),
		Mean:      round1(float64(total) / float64(len(funcs))),
		P90:       percentile(values, 0.9),
		Max:       sorted[0].Complexity,
		OverLimit: over,
		Top:       top,
	}
}

// duplication は tools/ の jscpd で Go と TS の重複を数える。除外のパターンは
// 根からの相対パスに当てるので、根で "." を渡す（根自体が .claude/worktrees/ の
// 下にあっても全部が除外されないように）。
func duplication(root, reports string) (*Duplication, error) {
	outDir := filepath.Join(reports, "jscpd")
	cmd := exec.Command("node", filepath.Join(root, "tools", "node_modules", "jscpd", "bin", "jscpd"), ".",
		"--format", "go,typescript,tsx,javascript",
		"--ignore", "**/node_modules/**,**/dist/**,**/gen/**,**/extgen/**,translations/**,docs-site/**,worktrees/**,.claude/**,web/registry/**,media/**,**/testdata/**,**/*.d.ts,reports/**,tools/**",
		"--gitignore", "--reporters", "json", "--output", outDir, "--silent")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(outDir, "jscpd-report.json"))
	if err != nil {
		return nil, err
	}
	return parseJscpd(data)
}

func parseJscpd(data []byte) (*Duplication, error) {
	type stats struct {
		Percentage      float64 `json:"percentage"`
		DuplicatedLines int     `json:"duplicatedLines"`
		Clones          int     `json:"clones"`
	}
	var report struct {
		Statistics struct {
			Total   stats `json:"total"`
			Formats map[string]struct {
				Total stats `json:"total"`
			} `json:"formats"`
		} `json:"statistics"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	d := &Duplication{
		Percentage: report.Statistics.Total.Percentage,
		Lines:      report.Statistics.Total.DuplicatedLines,
		Clones:     report.Statistics.Total.Clones,
		ByFormat:   map[string]float64{},
	}
	for name, f := range report.Statistics.Formats {
		d.ByFormat[name] = f.Total.Percentage
	}
	return d, nil
}

// ciRun は 1 回の CI の、成功した job の開始と終了。
type ciRun struct {
	Jobs []ciJob `json:"jobs"`
}

type ciJob struct {
	Name        string    `json:"name"`
	Conclusion  string    `json:"conclusion"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// fetchCIRuns は gh で main への push の成功した CI を新しい順に n 回分読む。
func fetchCIRuns(root string, n int) ([]ciRun, error) {
	ids, err := gh(root, fmt.Sprintf("repos/{owner}/{repo}/actions/workflows/ci.yml/runs?branch=main&event=push&status=success&per_page=%d", n), ".workflow_runs[].id")
	if err != nil {
		return nil, err
	}
	var runs []ciRun
	for _, id := range strings.Fields(string(ids)) {
		data, err := gh(root, "repos/{owner}/{repo}/actions/runs/"+id+"/jobs?per_page=100", "")
		if err != nil {
			return nil, err
		}
		var run ciRun
		if err := json.Unmarshal(data, &run); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func gh(root, path, jq string) ([]byte, error) {
	args := []string{"api", path}
	if jq != "" {
		args = append(args, "--jq", jq)
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api %s: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// ciDurations は成功した job だけで分布を出す。飛ばした job（文書だけの変更の
// 検査など）は所要時間が 0 に近く、分布を歪めるので数えない。
func ciDurations(runs []ciRun) *CIDurations {
	jobs := map[string][]float64{}
	var walls []float64
	for _, run := range runs {
		var first, last time.Time
		for _, j := range run.Jobs {
			if j.Conclusion != "success" || j.StartedAt.IsZero() || j.CompletedAt.IsZero() {
				continue
			}
			jobs[j.Name] = append(jobs[j.Name], j.CompletedAt.Sub(j.StartedAt).Seconds())
			if first.IsZero() || j.StartedAt.Before(first) {
				first = j.StartedAt
			}
			if j.CompletedAt.After(last) {
				last = j.CompletedAt
			}
		}
		if !first.IsZero() {
			walls = append(walls, last.Sub(first).Seconds())
		}
	}
	d := &CIDurations{Runs: len(walls), Wall: dist(walls), Jobs: map[string]Percentiles{}}
	for name, v := range jobs {
		d.Jobs[name] = dist(v)
	}
	return d
}

func dist(v []float64) Percentiles {
	return Percentiles{P50: percentile(v, 0.5), P90: percentile(v, 0.9)}
}

// percentile は最近傍順位法で p 分位点を返す（値が無ければ 0）。
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := slices.Clone(values)
	slices.Sort(s)
	i := int(math.Ceil(p*float64(len(s)))) - 1
	return round1(s[max(0, min(i, len(s)-1))])
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func lastEntry(path string) (*Entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var last *Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		last = &e
	}
	return last, scanner.Err()
}

func gitHead(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--short=8", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// markdown は今回の値と、履歴の前回からの差を表にする。
func markdown(e Entry, prev *Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Quality metrics (%s, %s)\n\n", e.Date, e.Commit)
	if prev != nil {
		fmt.Fprintf(&b, "Compared with %s (%s).\n\n", prev.Date, prev.Commit)
	}
	b.WriteString("| Metric | Value | Change |\n| --- | --- | --- |\n")
	row := func(name string, now, before float64, ok bool, unit string) {
		change := "—"
		if ok {
			change = fmt.Sprintf("%+.1f%s", now-before, unit)
		}
		fmt.Fprintf(&b, "| %s | %.1f%s | %s |\n", name, now, unit, change)
	}
	count := func(name string, now, before int, ok bool) {
		change := "—"
		if ok {
			change = fmt.Sprintf("%+d", now-before)
		}
		fmt.Fprintf(&b, "| %s | %d | %s |\n", name, now, change)
	}
	for _, lang := range []string{"go", "ts"} {
		c, found := e.Complexity[lang]
		if !found {
			continue
		}
		p, ok := Complexity{}, false
		if prev != nil {
			p, ok = prev.Complexity[lang]
		}
		count(lang+" functions", c.Functions, p.Functions, ok)
		row(lang+" complexity mean", c.Mean, p.Mean, ok, "")
		row(lang+" complexity p90", c.P90, p.P90, ok, "")
		count(fmt.Sprintf("%s functions over %d", lang, complexityLimit), c.OverLimit, p.OverLimit, ok)
	}
	if d := e.Duplication; d != nil {
		ok := prev != nil && prev.Duplication != nil
		var p Duplication
		if ok {
			p = *prev.Duplication
		}
		row("duplicated lines", d.Percentage, p.Percentage, ok, "%")
	}
	if c := e.CI; c != nil && c.Runs > 0 {
		ok := prev != nil && prev.CI != nil
		var p CIDurations
		if ok {
			p = *prev.CI
		}
		row("CI wall p50", c.Wall.P50, p.Wall.P50, ok, "s")
		row("CI wall p90", c.Wall.P90, p.Wall.P90, ok, "s")
		names := make([]string, 0, len(c.Jobs))
		for name := range c.Jobs {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			pj, found := p.Jobs[name]
			row(name+" p90", c.Jobs[name].P90, pj.P90, ok && found, "s")
		}
	}
	for _, lang := range []string{"go", "ts"} {
		c, found := e.Complexity[lang]
		if !found || len(c.Top) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\nMost complex %s functions:\n\n| Function | Location | Complexity |\n| --- | --- | --- |\n", lang)
		for _, f := range c.Top {
			fmt.Fprintf(&b, "| %s | %s | %d |\n", f.Name, f.Location, f.Complexity)
		}
	}
	b.WriteString("\n")
	return b.String()
}
