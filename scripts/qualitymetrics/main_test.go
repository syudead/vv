package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"
)

func TestCyclomaticCountsBranchesAndBooleanOperators(t *testing.T) {
	src := `package p
func f(a, b bool, xs []int, c chan int) int {
	if a && b || !a {
		return 1
	}
	for _, x := range xs {
		switch x {
		case 1, 2:
		case 3:
		default:
		}
	}
	select {
	case <-c:
	default:
	}
	go func() {
		if a {
		}
	}()
	return 0
}`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 1 + if + && + || + range + case 1,2 + case 3 + case <-c + 関数リテラルの if
	if got := cyclomatic(file.Decls[0]); got != 9 {
		t.Errorf("cyclomatic = %d, want 9", got)
	}
}

func TestPercentileUsesNearestRank(t *testing.T) {
	values := []float64{5, 1, 4, 2, 3, 6, 7, 8, 9, 10}
	if got := percentile(values, 0.5); got != 5 {
		t.Errorf("p50 = %v, want 5", got)
	}
	if got := percentile(values, 0.9); got != 9 {
		t.Errorf("p90 = %v, want 9", got)
	}
	if got := percentile(nil, 0.9); got != 0 {
		t.Errorf("p90 of nothing = %v, want 0", got)
	}
}

func TestParseESLintKeepsComplexityOfSourceFunctionsOnly(t *testing.T) {
	data := []byte(`[
	 {"filePath":"/w/web/src/a.ts","messages":[
	  {"ruleId":"complexity","message":"Function 'load' has a complexity of 12. Maximum allowed is 0.","line":3},
	  {"ruleId":"no-unused-vars","message":"x","line":4}]},
	 {"filePath":"/w/web/src/a.test.ts","messages":[
	  {"ruleId":"complexity","message":"Arrow function has a complexity of 2. Maximum allowed is 0.","line":1}]}]`)
	funcs, err := parseESLint(data, "/w/web")
	if err != nil {
		t.Fatal(err)
	}
	if len(funcs) != 1 || funcs[0].Name != "Function 'load'" || funcs[0].Complexity != 12 || funcs[0].Location != "web/src/a.ts:3" {
		t.Errorf("funcs = %+v", funcs)
	}
}

func TestCIDurationsIgnoreJobsThatDidNotSucceed(t *testing.T) {
	at := func(s int) time.Time { return time.Date(2026, 10, 11, 0, 0, s, 0, time.UTC) }
	runs := []ciRun{{Jobs: []ciJob{
		{Name: "Checks (go test)", Conclusion: "success", StartedAt: at(0), CompletedAt: at(80)},
		{Name: "Checks (web lint)", Conclusion: "success", StartedAt: at(10), CompletedAt: at(50)},
		{Name: "Browser E2E (tags)", Conclusion: "skipped", StartedAt: at(0), CompletedAt: at(1)},
	}}}
	d := ciDurations(runs)
	if d.Runs != 1 || d.Wall.P50 != 80 {
		t.Errorf("wall = %+v (runs %d), want p50 80 over 1 run", d.Wall, d.Runs)
	}
	if _, found := d.Jobs["Browser E2E (tags)"]; found {
		t.Error("a skipped job is counted")
	}
	if d.Jobs["Checks (go test)"].P90 != 80 {
		t.Errorf("go test = %+v", d.Jobs["Checks (go test)"])
	}
}

func TestMarkdownShowsChangeFromThePreviousEntry(t *testing.T) {
	prev := &Entry{Date: "2026-10-04", Commit: "aaaa", Complexity: map[string]Complexity{"go": {Functions: 10, Mean: 3, P90: 7}}}
	now := Entry{Date: "2026-10-11", Commit: "bbbb", Complexity: map[string]Complexity{"go": {Functions: 12, Mean: 3.5, P90: 7}}}
	md := markdown(now, prev)
	for _, want := range []string{"Compared with 2026-10-04 (aaaa)", "| go functions | 12 | +2 |", "| go complexity mean | 3.5 | +0.5 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("summary lacks %q:\n%s", want, md)
		}
	}
}

func TestRegressionsFlagWorseMetricsOnly(t *testing.T) {
	prev := Entry{
		Complexity:  map[string]Complexity{"go": {Functions: 100, Mean: 4.3, OverLimit: 46}, "ts": {Functions: 50, Mean: 2.5, OverLimit: 40}},
		Duplication: &Duplication{Percentage: 2.64},
		CI:          &CIDurations{Runs: 20, Wall: Percentiles{P90: 300}},
	}
	now := Entry{
		Complexity:  map[string]Complexity{"go": {Functions: 101, Mean: 4.4, OverLimit: 47}, "ts": {Functions: 50, Mean: 2.4, OverLimit: 39}},
		Duplication: &Duplication{Percentage: 2.70},
		CI:          &CIDurations{Runs: 20, Wall: Percentiles{P90: 330}},
	}
	got := regressions(now, prev)
	want := []string{
		"go: functions over complexity 15 went from 46 to 47",
		"go: mean complexity went from 4.3 to 4.4",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("regressions = %q, want %q", got, want)
	}

	now.Duplication.Percentage = 2.90
	now.CI.Wall.P90 = 400
	got = regressions(now, prev)
	if len(got) != 4 || got[2] != "duplicated lines went from 2.64% to 2.90%" || got[3] != "CI wall p90 went from 300s to 400s" {
		t.Errorf("regressions = %q", got)
	}
}
