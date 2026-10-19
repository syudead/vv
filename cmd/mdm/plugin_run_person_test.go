package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/wasmplugin"
)

const personRunManifest = `{"contract":1,"kind":"person","name":"person-test","version":"0.1.0",` +
	`"operations":["search","fetch"],"sources":["s"]}`

func TestPluginRunFetchesAPersonFromTheSource(t *testing.T) {
	module := runModulePath(t, personRunManifest)

	got := runPluginCommand(t, wasmplugin.Options{}, module, "fetch", "1", "--source", "s")
	wantRun(t, got, exitAccountOK)
	var body map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &body); err != nil {
		t.Fatalf("stdout %q: %v", got.stdout, err)
	}
	// The fixture names the person by the ID and lists another ID before the requested one.
	want := map[string]any{"person": map[string]any{"name": "1", "externalIds": []any{
		map[string]any{"source": "s", "value": "1"}, map[string]any{"source": "other", "value": "x"},
	}}}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("stdout = %s, want %v", got.stdout, want)
	}
}

func TestPluginPackPrintsThePersonKindAndSources(t *testing.T) {
	modulePath, manifestPath := writePackFiles(t, testModule(""), personRunManifest)
	run := runPack(t, modulePath, manifestPath)
	if run.code != exitAccountOK || !strings.Contains(run.stderr, "Kind: person") || !strings.Contains(run.stderr, "Sources: s") {
		t.Fatalf("exit = %d, stderr %q; want the kind and the sources", run.code, run.stderr)
	}
}

func TestPluginRunRefusesASourceTheRunCannotTake(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		args     []string
	}{
		{"work-info plugin", runManifest, []string{"fetch", "1", "--source", "s", "--set", "token=t"}},
		{"person plugin without --source", personRunManifest, []string{"fetch", "1"}},
		{"undeclared source", personRunManifest, []string{"fetch", "1", "--source", "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			module := runModulePath(t, tc.manifest)
			wantRun(t, runPluginCommand(t, wasmplugin.Options{}, append([]string{module}, tc.args...)...), exitAccountUsage, "source")
		})
	}
}

func TestPluginRunPrintsAPersonImageOnlyWhenFetched(t *testing.T) {
	req := domain.WorkInfoRunRequest{Operation: domain.WorkInfoFetch, Kind: domain.WorkInfoPluginKindPerson}
	const url = "https://site.test/p.jpg"
	cases := []struct {
		name  string
		image []byte
		want  string
	}{
		{"fetched", []byte{0xff}, url},
		{"not fetched", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := domain.WorkInfoRunResult{
				Person: domain.FetchedPluginPerson{Name: "n", ImageURL: url}, PersonImage: tc.image,
			}
			var out bytes.Buffer
			if failure := writeKeptResult(&out, req, result); failure != nil {
				t.Fatalf("writeKeptResult: %v", failure)
			}
			var body struct {
				Person struct {
					Image string `json:"image"`
				} `json:"person"`
			}
			if err := json.Unmarshal(out.Bytes(), &body); err != nil || body.Person.Image != tc.want {
				t.Fatalf("stdout = %s, %v; want image %q", out.String(), err, tc.want)
			}
		})
	}
}
