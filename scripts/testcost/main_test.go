package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseDiffKeepsAddedLinesOfTestFiles(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/internal/store/a_test.go b/internal/store/a_test.go",
		"--- a/internal/store/a_test.go",
		"+++ b/internal/store/a_test.go",
		"@@ -1,0 +2,1 @@",
		"+func TestA(t *testing.T) {",
		"@@ -9 +10,2 @@",
		"-func TestOld(t *testing.T) {",
		"+	{name: \"case\"},",
		"+	{name: \"another\"},",
		"diff --git a/internal/store/a.go b/internal/store/a.go",
		"+++ b/internal/store/a.go",
		"+func TestNotATest(t *testing.T) {",
		"+++ b/web/src/x.test.ts",
		"@@ -0,0 +1 @@",
		"+it(\"x\", () => {",
	}, "\n")

	got := parseDiff(diff)
	want := map[string][]addedLine{
		"internal/store/a_test.go": {
			{N: 2, Text: "func TestA(t *testing.T) {"},
			{N: 10, Text: "	{name: \"case\"},"},
			{N: 11, Text: "	{name: \"another\"},"},
		},
		"web/src/x.test.ts": {{N: 1, Text: "it(\"x\", () => {"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseDiff = %v, want %v", got, want)
	}
}

func TestKindOf(t *testing.T) {
	for path, want := range map[string]fileKind{
		"internal/store/a_test.go":   goTestFile,
		"web/src/tags/Tags.test.tsx": webUnitFile,
		"web/src/tags/rows.test.ts":  webUnitFile,
		"web/e2e/tags.e2e.ts":        e2eFile,
		"web/e2e/clock.ts":           otherFile,
		"internal/store/a.go":        otherFile,
		"docs/test.test.ts":          otherFile,
	} {
		if got := kindOf(path); got != want {
			t.Errorf("kindOf(%q) = %d, want %d", path, got, want)
		}
	}
}

func TestWebTestNames(t *testing.T) {
	lines := []string{
		`  it("plain", () => {`,
		`  test('single quotes', async () => {`,
		"  it(`template`, () => {",
		`  it.each([[1], [2]])("each %s", (n) => {`,
		`  it.skip("skipped", () => {`,
		`  expect(it).toBe(1);`,
		`  describe("not a test", () => {`,
	}
	want := []string{"plain", "single quotes", "template", "each %s", "skipped"}
	if got := webTestNames(wholeFile(strings.Join(lines, "\n"))); !reflect.DeepEqual(got, want) {
		t.Fatalf("webTestNames = %q, want %q", got, want)
	}
}

func TestWebLevel(t *testing.T) {
	for body, want := range map[string]level{
		`import LibraryPage from "./LibraryPage";`:                                levelPage,
		`import { renderTagsPage } from "../testing/tagsPage";`:                   levelPage,
		"import { render } from \"@testing-library/react\";\nrender(<TagRow />);": levelComponent,
		"const { result } = renderHook(() => useVideos());":                       levelLogic,
		"expect(sortRows(rows)).toEqual([]);":                                     levelLogic,
	} {
		if got := webLevel(body); got != want {
			t.Errorf("webLevel(%q) = %s, want %s", body, got, want)
		}
	}
}

func TestGoTestsFollowHelpersToTheResourceTheyOpen(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("helpers_test.go", `package p

func fixture(t *testing.T) *env { return newEnv(t) }
func newEnv(t *testing.T) *env {
	db, _ := store.Open(t.TempDir())
	return &env{db: db}
}
func (e *env) serve() { newTestServer(e.t, Options{}) }
`)
	write("a_test.go", `package p

func TestPure(t *testing.T) { _ = os.Open("x") }
func TestStore(t *testing.T) { fixture(t) }
func TestHTTP(t *testing.T) { e := fixture(t); e.serve() }
func TestHTTPOverFakes(t *testing.T) { newTestServer(t, Options{}) }
func TestMedia(t *testing.T) { requireFFmpeg(t); fixture(t) }
func TestUntouched(t *testing.T) { fixture(t) }
func TestCasesAdded(t *testing.T) {
	for _, tc := range []string{
		"old",
		"new",
	} {
		fixture(t)
	}
}

var s = "func TestInString(t *testing.T) {}"
`)
	// 宣言の行（3〜7 行目）、既存のテストに足した表の行（12 行目）、文字列の中の行（18 行目）。
	lines := []addedLine{{N: 3}, {N: 4}, {N: 5}, {N: 6}, {N: 7}, {N: 12}, {N: 18}}
	tests, err := goTests(filepath.Join(dir, "a_test.go"), lines)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, test := range tests {
		got[displayName(test)] = test.Level.String()
	}
	want := map[string]string{
		"TestPure": "logic", "TestStore": "store", "TestHTTP": "http", "TestHTTPOverFakes": "http",
		"TestMedia": "media", "TestCasesAdded (cases added)": "store",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("levels = %v, want %v", got, want)
	}
}

func TestGoLevelReadsUnqualifiedOpenOnlyInsideTheStorePackage(t *testing.T) {
	reached := map[string]bool{"Open": true}
	if got := goLevel("internal/store", reached); got != levelStore {
		t.Errorf("internal/store: %s, want store", got)
	}
	if got := goLevel("internal/app", reached); got != levelLogic {
		t.Errorf("internal/app: %s, want logic", got)
	}
}

func TestRenderOrdersByCostAndAsksForReasons(t *testing.T) {
	tests := []*addedTest{
		{File: "web/src/a.test.ts", Name: "a|b", Level: levelLogic, Seconds: 0.01},
		{File: "web/src/P.test.tsx", Name: "page", Level: levelPage, Seconds: 0.5},
		{File: "web/src/Q.test.tsx", Name: "gone", Level: levelPage, Missing: true},
	}
	got := render(tests, true)
	want := "| Level | Test | File | Seconds |\n| --- | --- | --- | --- |\n" +
		"| page | page | `web/src/P.test.tsx` | 0.50 |\n" +
		"| page | gone | `web/src/Q.test.tsx` | not found |\n" +
		"| logic | a\\|b | `web/src/a.test.ts` | 0.01 |\n\n" +
		"Added: page 2 (0.50 s), logic 1 (0.01 s).\n" +
		"2 test(s) at store, http, page, media or e2e level: state in the pull request why the level below would not fail (docs/design-docs/go-testing.md, docs/design-docs/web-testing.md).\n"
	if got != want {
		t.Fatalf("render =\n%s\nwant\n%s", got, want)
	}
	if got := render(nil, false); got != "No tests added.\n" {
		t.Errorf("render(nil) = %q", got)
	}
}

func TestDurationsFromToolReports(t *testing.T) {
	goOut := []byte(`{"Action":"run","Test":"TestA"}
{"Action":"pass","Test":"TestA/case","Elapsed":0.1}
{"Action":"pass","Test":"TestA","Elapsed":0.25}
{"Action":"fail","Test":"TestB","Elapsed":1.5}
{"Action":"pass","Elapsed":2}
not json`)
	if got, want := goDurations(goOut), map[string]float64{"TestA": 0.25, "TestB": 1.5}; !reflect.DeepEqual(got, want) {
		t.Errorf("goDurations = %v, want %v", got, want)
	}

	vitest := []byte(`{"testResults":[{"name":"a","assertionResults":[{"title":"x","duration":250},{"title":"x","duration":250},{"title":"y","duration":10}]}]}`)
	got, err := vitestDurations(vitest)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]float64{"x": 0.5, "y": 0.01}; !reflect.DeepEqual(got, want) {
		t.Errorf("vitestDurations = %v, want %v", got, want)
	}
}
