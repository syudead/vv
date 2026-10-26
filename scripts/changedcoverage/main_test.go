package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDiff(t *testing.T) {
	diff := `diff --git a/internal/a.go b/internal/a.go
--- a/internal/a.go
+++ b/internal/a.go
@@ -3 +3,2 @@ func a() {
+x
+y
@@ -9,2 +11,0 @@
@@ -20 +21 @@
+z
diff --git a/gone.go b/gone.go
--- a/gone.go
+++ /dev/null
@@ -1 +0,0 @@
`
	got := parseDiff(strings.NewReader(diff))
	want := map[string][]int{"internal/a.go": {3, 4, 21}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDiff = %v, want %v", got, want)
	}
}

func TestParseGoProfileKeepsHighestCountPerLine(t *testing.T) {
	profile := `mode: set
github.com/syudead/vv/internal/a.go:10.2,12.3 2 0
github.com/syudead/vv/internal/a.go:12.3,13.4 1 1
`
	counts := lineCounts{}
	if err := parseGoProfile(strings.NewReader(profile), counts); err != nil {
		t.Fatal(err)
	}
	want := lineCounts{"internal/a.go": {10: 0, 11: 0, 12: 1, 13: 1}}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("counts = %v, want %v", counts, want)
	}
}

func TestParseLCOVMapsVitestPaths(t *testing.T) {
	report := "SF:src/a.ts\nDA:4,0\nDA:5,3\nend_of_record\n"
	counts := lineCounts{}
	if err := parseLCOV(strings.NewReader(report), counts); err != nil {
		t.Fatal(err)
	}
	want := lineCounts{"web/src/a.ts": {4: 0, 5: 3}}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("counts = %v, want %v", counts, want)
	}
}

func TestCompareAndPrint(t *testing.T) {
	changed := map[string][]int{
		"internal/a.go":      {1, 2, 3, 4, 7},
		"internal/a_test.go": {1},
		"web/src/new.ts":     {1},
		"docs/x.md":          {1},
	}
	counts := lineCounts{"internal/a.go": {2: 0, 3: 0, 4: 5, 7: 0}}
	out, code := formatReports(compare(changed, counts))
	want := "internal/a.go: 2-3, 7\nweb/src/new.ts: not in the coverage reports\n" +
		"changed statement lines: 4, uncovered: 3, files not measured: 1\n"
	if code != 1 || out != want {
		t.Fatalf("code %d, output:\n%s\nwant:\n%s", code, out, want)
	}
}

func TestFormatReportsSucceedsWhenEveryChangedLineRuns(t *testing.T) {
	if out, code := formatReports(compare(map[string][]int{"cmd/m.go": {1}}, lineCounts{"cmd/m.go": {1: 1}})); code != 0 {
		t.Fatalf("code = %d, output %s", code, out)
	}
}
