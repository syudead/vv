package domain

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if len(m.Settings) != 1 || m.Settings[0] != want {
		t.Fatalf("settings = %+v, want [%+v]", m.Settings, want)
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
