package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestParseWorkInfoPluginManifest(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		problem WorkInfoPluginProblem
	}{
		{"minimal", `{"contract":1,"name":"a.b-1","version":"1","operations":["fetch"]}`, ""},
		{"unknown fields ignored", `{"contract":1,"name":"p","version":"1","operations":["search"],"extra":true}`, ""},
		{"not an object", `[1]`, WorkInfoPluginManifestInvalid},
		{"no contract", `{"name":"p","version":"1","operations":["fetch"]}`, WorkInfoPluginManifestInvalid},
		{"contract 2 with other fields broken", `{"contract":2}`, WorkInfoPluginContractUnsupported},
		{"contract 2 with fields of other types", `{"contract":2,"name":7,"operations":{"fetch":true}}`, WorkInfoPluginContractUnsupported},
		{"contract not a number", `{"contract":"1","name":"p","version":"1","operations":["fetch"]}`, WorkInfoPluginManifestInvalid},
		{"contract 1 with a field of another type", `{"contract":1,"name":"p","version":"1","operations":{"fetch":true}}`, WorkInfoPluginManifestInvalid},
		{"null", `null`, WorkInfoPluginManifestInvalid},
		{"name with upper case", `{"contract":1,"name":"P","version":"1","operations":["fetch"]}`, WorkInfoPluginManifestInvalid},
		{"name of 65 characters", `{"contract":1,"name":"` + strings.Repeat("a", 65) + `","version":"1","operations":["fetch"]}`, WorkInfoPluginManifestInvalid},
		{"empty version", `{"contract":1,"name":"p","version":"","operations":["fetch"]}`, WorkInfoPluginManifestInvalid},
		{"version of 33 characters", `{"contract":1,"name":"p","version":"` + strings.Repeat("1", 33) + `","operations":["fetch"]}`, WorkInfoPluginManifestInvalid},
		{"description of 501 characters", `{"contract":1,"name":"p","version":"1","description":"` + strings.Repeat("d", 501) + `","operations":["fetch"]}`, WorkInfoPluginManifestInvalid},
		{"no operations", `{"contract":1,"name":"p","version":"1","operations":[]}`, WorkInfoPluginManifestInvalid},
		{"unknown operation", `{"contract":1,"name":"p","version":"1","operations":["delete"]}`, WorkInfoPluginManifestInvalid},
		{"repeated operation", `{"contract":1,"name":"p","version":"1","operations":["fetch","fetch"]}`, WorkInfoPluginManifestInvalid},
		{"setting key starting with a digit", `{"contract":1,"name":"p","version":"1","operations":["fetch"],"settings":[{"key":"1a","label":"L"}]}`, WorkInfoPluginManifestInvalid},
		{"setting without a label", `{"contract":1,"name":"p","version":"1","operations":["fetch"],"settings":[{"key":"a"}]}`, WorkInfoPluginManifestInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, problem := ParseWorkInfoPluginManifest([]byte(tc.data))
			if problem != tc.problem {
				t.Fatalf("problem = %q, want %q", problem, tc.problem)
			}
		})
	}
}

func TestParseWorkInfoPluginManifestDefaultsSettingFlags(t *testing.T) {
	m, problem := ParseWorkInfoPluginManifest([]byte(
		`{"contract":1,"name":"p","version":"1","operations":["fetch"],"settings":[{"key":"lang","label":"Language"}]}`))
	if problem != "" {
		t.Fatalf("problem = %q", problem)
	}
	want := WorkInfoPluginSetting{Key: "lang", Label: "Language"}
	if !reflect.DeepEqual(m.Settings, []WorkInfoPluginSetting{want}) {
		t.Fatalf("settings = %+v, want [%+v]", m.Settings, want)
	}
}

// 設定の既定値と選択肢の規則に外れるマニフェストは manifest_invalid である
// （specs/056-plugin-tooling/contracts/plugin-contract.md の Manifest）。
func TestParseWorkInfoPluginManifestSettingChoices(t *testing.T) {
	manifest := func(setting string) string {
		return `{"contract":1,"name":"p","version":"1","operations":["fetch"],"settings":[` + setting + `]}`
	}
	quoted := func(values ...string) string {
		b, _ := json.Marshal(values)
		return string(b)
	}
	many := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("o%d", i)
		}
		return out
	}
	cases := []struct {
		name    string
		setting string
		problem WorkInfoPluginProblem
	}{
		{"default without options", `{"key":"k","label":"L","default":"ja"}`, ""},
		{"default among the options", `{"key":"k","label":"L","default":"en","options":["ja","en"]}`, ""},
		{"options without a default", `{"key":"k","label":"L","required":true,"options":` + quoted(many(50)...) + `}`, ""},
		{"option of 200 characters", `{"key":"k","label":"L","options":` + quoted("a", strings.Repeat("b", 200)) + `}`, ""},
		{"default of 2000 characters", `{"key":"k","label":"L","default":"` + strings.Repeat("d", 2000) + `"}`, ""},
		{"default outside the options", `{"key":"k","label":"L","default":"fr","options":["ja","en"]}`, WorkInfoPluginManifestInvalid},
		{"one option", `{"key":"k","label":"L","options":["ja"]}`, WorkInfoPluginManifestInvalid},
		{"no options", `{"key":"k","label":"L","options":[]}`, WorkInfoPluginManifestInvalid},
		{"51 options", `{"key":"k","label":"L","options":` + quoted(many(51)...) + `}`, WorkInfoPluginManifestInvalid},
		{"option of 201 characters", `{"key":"k","label":"L","options":` + quoted("a", strings.Repeat("b", 201)) + `}`, WorkInfoPluginManifestInvalid},
		{"empty option", `{"key":"k","label":"L","options":["a",""]}`, WorkInfoPluginManifestInvalid},
		{"repeated option", `{"key":"k","label":"L","options":["ja","en","ja"]}`, WorkInfoPluginManifestInvalid},
		{"empty default", `{"key":"k","label":"L","default":""}`, WorkInfoPluginManifestInvalid},
		{"default of 2001 characters", `{"key":"k","label":"L","default":"` + strings.Repeat("d", 2001) + `"}`, WorkInfoPluginManifestInvalid},
		{"secret with a default", `{"key":"k","label":"L","secret":true,"default":"x"}`, WorkInfoPluginManifestInvalid},
		{"secret with options", `{"key":"k","label":"L","secret":true,"options":["a","b"]}`, WorkInfoPluginManifestInvalid},
		{"null default", `{"key":"k","label":"L","default":null}`, WorkInfoPluginManifestInvalid},
		{"null options", `{"key":"k","label":"L","options":null}`, WorkInfoPluginManifestInvalid},
		{"numeric default", `{"key":"k","label":"L","default":1}`, WorkInfoPluginManifestInvalid},
		{"null option", `{"key":"k","label":"L","options":["a",null]}`, WorkInfoPluginManifestInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, problem := ParseWorkInfoPluginManifest([]byte(manifest(tc.setting)))
			if problem != tc.problem {
				t.Fatalf("problem = %q, want %q", problem, tc.problem)
			}
		})
	}
}

func TestParseWorkInfoPluginManifestKeepsDefaultAndOptions(t *testing.T) {
	m, problem := ParseWorkInfoPluginManifest([]byte(`{"contract":1,"name":"p","version":"1","operations":["fetch"],` +
		`"settings":[{"key":"lang","label":"Language","default":"en","options":["ja","en"]}]}`))
	want := []WorkInfoPluginSetting{{Key: "lang", Label: "Language", Default: "en", Options: []string{"ja", "en"}}}
	if problem != "" || !reflect.DeepEqual(m.Settings, want) {
		t.Fatalf("settings = %+v, problem %q; want %+v", m.Settings, problem, want)
	}
}

// 拒んだマニフェストの理由は、反した項目と規則を名指す（specs/056-plugin-tooling/contracts/plugin-cli.md）。
func TestCheckWorkInfoPluginManifestNamesTheFieldAndTheRule(t *testing.T) {
	const ok = `"contract":1,"name":"p","version":"1","operations":["fetch"]`
	cases := []struct {
		name string
		data string
		want WorkInfoPluginManifestError
	}{
		{"not an object", `[1]`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "", "must be a JSON object"}},
		{"no contract", `{"name":"p"}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "contract", "must be an integer"}},
		{"contract 2", `{"contract":2}`,
			WorkInfoPluginManifestError{WorkInfoPluginContractUnsupported, "contract", "must be 1"}},
		{"operations of another type", `{"contract":1,"name":"p","version":"1","operations":{"fetch":true}}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "operations", "must be an array"}},
		{"name with upper case", `{"contract":1,"name":"P","version":"1","operations":["fetch"]}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "name", "must be 1 to 64 characters of a-z, 0-9, - and ."}},
		{"repeated operation", `{"contract":1,"name":"p","version":"1","operations":["fetch","fetch"]}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "operations", "must list search, fetch or both, each at most once"}},
		{"second setting's default outside the options",
			`{` + ok + `,"settings":[{"key":"a","label":"A"},{"key":"k","label":"L","default":"fr","options":["ja","en"]}]}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "settings[1].default", "must be one of options"}},
		{"secret with options", `{` + ok + `,"settings":[{"key":"k","label":"L","secret":true,"options":["a","b"]}]}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "settings[0].options", "is not allowed on a secret setting"}},
		{"repeated key", `{` + ok + `,"settings":[{"key":"k","label":"L"},{"key":"k","label":"M"}]}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "settings[1].key", "must be unique within the manifest"}},
		{"second setting's label of another type", `{` + ok + `,"settings":[{"key":"a","label":"A"},{"key":"b","label":42}]}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "settings[1].label", "must be a string"}},
		{"first setting's secret of another type", `{` + ok + `,"settings":[{"key":"a","label":"A","secret":"yes"}]}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "settings[0].secret", "must be a boolean"}},
		{"setting that is not an object", `{` + ok + `,"settings":[{"key":"a","label":"A"},42]}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "settings[1]", "must be an object"}},
		{"settings of another type", `{` + ok + `,"settings":{"key":"a"}}`,
			WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "settings", "must be an array"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := CheckWorkInfoPluginManifest([]byte(tc.data))
			if got == nil || *got != tc.want {
				t.Fatalf("rejection = %+v, want %+v", got, tc.want)
			}
		})
	}
	if _, got := CheckWorkInfoPluginManifest([]byte(`{` + ok + `}`)); got != nil {
		t.Fatalf("valid manifest rejected: %v", got)
	}
	err := &WorkInfoPluginManifestError{WorkInfoPluginManifestInvalid, "settings[0].default", "must be one of options"}
	if got, want := err.Error(), "manifest_invalid: settings[0].default must be one of options"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestWorkInfoPluginStateOf(t *testing.T) {
	if got := WorkInfoPluginStateOf(WorkInfoPluginNotWasm, "d", "d"); got != WorkInfoPluginInvalid {
		t.Fatalf("state with a problem = %q", got)
	}
	if got := WorkInfoPluginStateOf("", "d", "d"); got != WorkInfoPluginApproved {
		t.Fatalf("state with the approved digest = %q", got)
	}
	if got := WorkInfoPluginStateOf("", "d2", "d"); got != WorkInfoPluginUnapproved {
		t.Fatalf("state with another digest = %q", got)
	}
}

func TestWorkInfoRunRequestJSON(t *testing.T) {
	b, err := json.Marshal(WorkInfoRunRequest{Operation: WorkInfoSearch, Query: "q", URL: "ignored"})
	if err != nil || string(b) != `{"contract":1,"operation":"search","query":"q","settings":{}}` {
		t.Fatalf("search request = %s, %v", b, err)
	}
	b, err = json.Marshal(WorkInfoRunRequest{Operation: WorkInfoFetch, URL: "https://e.test/", Settings: map[string]string{"k": "v"}})
	if err != nil || string(b) != `{"contract":1,"operation":"fetch","url":"https://e.test/","settings":{"k":"v"}}` {
		t.Fatalf("fetch request = %s, %v", b, err)
	}
}

func TestParseWorkInfoResponseKeepsTheFirstCandidatesAndUniqueTags(t *testing.T) {
	candidates := make([]string, 60)
	for i := range candidates {
		candidates[i] = fmt.Sprintf(`{"url":"https://e.test/%d","title":"t%d"}`, i, i)
	}
	got, err := ParseWorkInfoResponse(WorkInfoSearch, []byte(`{"candidates":[`+strings.Join(candidates, ",")+`]}`))
	if err != nil || len(got.Candidates) != WorkInfoPluginMaxCandidates || got.Candidates[49].Title != "t49" {
		t.Fatalf("search result = %d candidates, %v", len(got.Candidates), err)
	}

	// 保持しない 51 件目以降は確かめないので、不正でも検索を失敗させない。
	candidates = append(candidates[:WorkInfoPluginMaxCandidates], `{"url":"ftp://e.test/","title":""}`)
	got, err = ParseWorkInfoResponse(WorkInfoSearch, []byte(`{"candidates":[`+strings.Join(candidates, ",")+`]}`))
	if err != nil || len(got.Candidates) != WorkInfoPluginMaxCandidates {
		t.Fatalf("search result with an invalid 51st candidate = %d candidates, %v", len(got.Candidates), err)
	}

	tags := []string{`"a"`, `"a"`}
	for i := range 120 {
		tags = append(tags, fmt.Sprintf(`"t%d"`, i))
	}
	got, err = ParseWorkInfoResponse(WorkInfoFetch, []byte(`{"workInfo":{"title":"T","tags":[`+strings.Join(tags, ",")+`]}}`))
	if err != nil || got.WorkInfo.Title != "T" || len(got.WorkInfo.Tags) != WorkInfoPluginMaxTags ||
		got.WorkInfo.Tags[0] != "a" || got.WorkInfo.Tags[1] != "t0" {
		t.Fatalf("fetch result = %+v, %v", got.WorkInfo, err)
	}
}

func TestParseWorkInfoResponseFailures(t *testing.T) {
	cases := []struct {
		name   string
		op     WorkInfoOperation
		output string
	}{
		{"not JSON", WorkInfoFetch, `nope`},
		{"empty", WorkInfoFetch, ``},
		{"fetch shape for search", WorkInfoSearch, `{"workInfo":{}}`},
		{"search shape for fetch", WorkInfoFetch, `{"candidates":[]}`},
		{"candidate without a title", WorkInfoSearch, `{"candidates":[{"url":"https://e.test/"}]}`},
		{"candidate with another scheme", WorkInfoSearch, `{"candidates":[{"url":"ftp://e.test/","title":"t"}]}`},
		{"candidate without a host", WorkInfoSearch, `{"candidates":[{"url":"https:///work","title":"t"}]}`},
		{"candidate with an opaque URL", WorkInfoSearch, `{"candidates":[{"url":"https:e.test/work","title":"t"}]}`},
		{"error", WorkInfoFetch, `{"error":{"message":"not found"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseWorkInfoResponse(tc.op, []byte(tc.output))
			var runErr *WorkInfoRunError
			if !errors.As(err, &runErr) || runErr.Failure != WorkInfoRunFailed {
				t.Fatalf("err = %v, want plugin_failed", err)
			}
		})
	}
}
