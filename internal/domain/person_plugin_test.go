package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const personManifestHead = `{"contract":1,"name":"p","version":"1",`

func TestCheckWorkInfoPluginManifestKind(t *testing.T) {
	eleven := make([]string, 11)
	for i := range eleven {
		eleven[i] = fmt.Sprintf("%q", fmt.Sprintf("s%d", i))
	}
	elevenDomains := make([]string, 11)
	for i := range elevenDomains {
		elevenDomains[i] = fmt.Sprintf(`"d%d.example"`, i)
	}
	const personOps = `"kind":"person","operations":["search","fetch"],"sources":["a"],`
	cases := []struct {
		name  string
		rest  string
		field string
	}{
		{"person without sources", `"kind":"person","operations":["search","fetch"]}`, "sources"},
		{"person with 11 sources", `"kind":"person","operations":["search","fetch"],"sources":[` + strings.Join(eleven, ",") + `]}`, "sources"},
		{"person with a repeated source", `"kind":"person","operations":["search","fetch"],"sources":["a","b","a"]}`, "sources[2]"},
		{"person with only fetch", `"kind":"person","operations":["fetch"],"sources":["a"]}`, "operations"},
		{"work with sources", `"operations":["fetch"],"sources":["a"]}`, "sources"},
		{"work with domains", `"operations":["fetch"],"domains":["example.com"]}`, "domains"},
		{"person with empty domains", personOps + `"domains":[]}`, "domains"},
		{"person with 11 domains", personOps + `"domains":[` + strings.Join(elevenDomains, ",") + `]}`, "domains"},
		{"person with an uppercase domain", personOps + `"domains":["Example.com"]}`, "domains[0]"},
		{"person with a scheme", personOps + `"domains":["example.com","https://example.org"]}`, "domains[1]"},
		{"person with a port", personOps + `"domains":["example.com:8080"]}`, "domains[0]"},
		{"person with a path", personOps + `"domains":["example.com/people"]}`, "domains[0]"},
		{"person with a repeated domain", personOps + `"domains":["a.example","b.example","a.example"]}`, "domains[2]"},
		{"unknown kind", `"kind":"studio","operations":["fetch"]}`, "kind"},
		{"null kind", `"kind":null,"operations":["fetch"]}`, "kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, rejection := CheckWorkInfoPluginManifest([]byte(personManifestHead + tc.rest))
			if rejection == nil || rejection.Problem != WorkInfoPluginManifestInvalid || rejection.Field != tc.field {
				t.Fatalf("rejection = %+v, want manifest_invalid on %s", rejection, tc.field)
			}
		})
	}
}

func TestCheckWorkInfoPluginManifestReadsKindSourcesAndDomains(t *testing.T) {
	work, rejection := CheckWorkInfoPluginManifest([]byte(personManifestHead + `"operations":["fetch"]}`))
	if rejection != nil || work.Kind != WorkInfoPluginKindWork || work.Sources != nil {
		t.Fatalf("manifest without kind = %+v, %v; want kind work without sources", work, rejection)
	}
	person, rejection := CheckWorkInfoPluginManifest([]byte(personManifestHead +
		`"kind":"person","operations":["fetch","search"],"sources":["fanza","wiki"]}`))
	if rejection != nil || person.Kind != WorkInfoPluginKindPerson || !reflect.DeepEqual(person.Sources, []string{"fanza", "wiki"}) {
		t.Fatalf("person manifest = %+v, %v", person, rejection)
	}
	tenDomains := make([]string, 10)
	for i := range tenDomains {
		tenDomains[i] = fmt.Sprintf("d-%d.example.com", i)
	}
	tenDomains[0] = strings.Repeat("a", PersonPluginDomainMaxLength-len(".com")) + ".com"
	encoded, _ := json.Marshal(tenDomains)
	withDomains, rejection := CheckWorkInfoPluginManifest([]byte(personManifestHead +
		`"kind":"person","operations":["fetch","search"],"sources":["wiki"],"domains":` + string(encoded) + `}`))
	if rejection != nil || !slices.Equal(withDomains.Domains, tenDomains) {
		t.Fatalf("person manifest with domains = %+v, %v; want the domains in order", withDomains, rejection)
	}
}

// Only the exact keys carry sources and domains; a key that differs in case must not bypass their checks.
func TestCheckWorkInfoPluginManifestIgnoresMiscasedSourcesAndDomains(t *testing.T) {
	person, rejection := CheckWorkInfoPluginManifest([]byte(personManifestHead +
		`"kind":"person","operations":["search","fetch"],"sources":["s"],"Domains":["HTTPS://example.com/path"]}`))
	if rejection != nil || person.Domains != nil {
		t.Fatalf("person manifest with Domains = %+v, %v; want accepted without domains", person, rejection)
	}
	work, rejection := CheckWorkInfoPluginManifest([]byte(personManifestHead +
		`"operations":["fetch"],"Sources":["a"],"DOMAINS":["example.com"]}`))
	if rejection != nil || work.Sources != nil || work.Domains != nil {
		t.Fatalf("work manifest with Sources and DOMAINS = %+v, %v; want accepted without either", work, rejection)
	}
}

func TestPersonFetchRequestJSONCarriesTheSource(t *testing.T) {
	req := WorkInfoRunRequest{Operation: WorkInfoFetch, Kind: WorkInfoPluginKindPerson, Source: "s", ID: "1", Sources: []string{"s"}}
	b, err := json.Marshal(req)
	if err != nil || string(b) != `{"contract":1,"operation":"fetch","source":"s","id":"1","settings":{}}` {
		t.Fatalf("person fetch request = %s, %v", b, err)
	}
}

func personFetch(t *testing.T, person string) (FetchedPluginPerson, error) {
	t.Helper()
	req := WorkInfoRunRequest{Operation: WorkInfoFetch, Kind: WorkInfoPluginKindPerson, Source: "s", ID: "1"}
	got, err := ParseRunResponse(req, []byte(`{"person":`+person+`}`))
	return got.Person, err
}

// idsJSON returns n external IDs of source o with values 0 to n-1.
func idsJSON(n int) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf(`{"source":"o","value":"%d"}`, i)
	}
	return "[" + strings.Join(ids, ",") + "]"
}

func TestParsePersonResponseLimits(t *testing.T) {
	aliases := make([]string, 51)
	for i := range aliases {
		aliases[i] = fmt.Sprintf(`{"name":"a%d","language":"ja"}`, i)
	}
	got, err := personFetch(t, `{"name":" N ","aliases":[`+strings.Join(aliases, ",")+`],"externalIds":`+idsJSON(ExternalIDsMaxCount)+`}`)
	if err != nil || got.Name != "N" || len(got.Aliases) != CreditAliasesMaxCount || got.Aliases[49].Name != "a49" {
		t.Fatalf("person = %+v, %v; want N with aliases a0 to a49", got, err)
	}
	if len(got.ExternalIDs) != ExternalIDsMaxCount || got.ExternalIDs[0] != (ExternalID{Source: "s", Value: "1"}) ||
		got.ExternalIDs[19] != (ExternalID{Source: "o", Value: "18"}) {
		t.Fatalf("external IDs = %v; want s:1 then o:0 to o:18", got.ExternalIDs)
	}
}

func TestParsePersonResponsePutsTheRequestedIDFirst(t *testing.T) {
	got, err := personFetch(t, `{"name":"N","externalIds":[{"source":"o","value":"x"},{"source":"s","value":"1"}]}`)
	want := []ExternalID{{Source: "s", Value: "1"}, {Source: "o", Value: "x"}}
	if err != nil || !reflect.DeepEqual(got.ExternalIDs, want) {
		t.Fatalf("external IDs = %v, %v; want %v", got.ExternalIDs, err, want)
	}
	got, err = personFetch(t, `{"name":"N"}`)
	if err != nil || !reflect.DeepEqual(got.ExternalIDs, want[:1]) {
		t.Fatalf("external IDs without the plugin's = %v, %v; want the requested one", got.ExternalIDs, err)
	}
}

func TestParsePersonResponseKeepsImagesAndProfileUnderTheCreditRules(t *testing.T) {
	got, err := personFetch(t, `{"name":"N","images":["https://e.test/1.jpg","ftp://e.test/x",7,"https://e.test/1.jpg",`+
		`"https://e.test/2.jpg"],"profile":{"heightCm":160,"cup":7}}`)
	if err != nil || !slices.Equal(got.ImageURLs, []string{"https://e.test/1.jpg", "https://e.test/2.jpg"}) ||
		got.Profile.HeightCm == nil || *got.Profile.HeightCm != 160 || got.Profile.Cup != "" {
		t.Fatalf("person = %+v, %v; want the valid distinct images and the height, without the mistyped cup", got, err)
	}
	urls := make([]string, PersonPluginImagesMaxCount+1)
	for i := range urls {
		urls[i] = fmt.Sprintf("%q", fmt.Sprintf("https://e.test/%d.jpg", i))
	}
	got, err = personFetch(t, `{"name":"N","images":[`+strings.Join(urls, ",")+`]}`)
	if err != nil || len(got.ImageURLs) != PersonPluginImagesMaxCount || got.ImageURLs[0] != "https://e.test/0.jpg" {
		t.Fatalf("images = %v, %v; want the first %d", got.ImageURLs, err, PersonPluginImagesMaxCount)
	}
}

func TestParsePersonSearchCandidates(t *testing.T) {
	req := WorkInfoRunRequest{Operation: WorkInfoSearch, Kind: WorkInfoPluginKindPerson, Sources: []string{"s", "t"}}
	candidates := make([]string, 51)
	for i := range candidates {
		candidates[i] = fmt.Sprintf(`{"source":"t","id":" %d ","name":"n%d","url":"https://e.test/%d"}`, i, i, i)
	}
	// The 51st candidate's source is undeclared, but it lies beyond the kept limit.
	candidates[50] = `{"source":"x","id":"z","name":"z","url":"https://e.test/z"}`
	got, err := ParseRunResponse(req, []byte(`{"candidates":[`+strings.Join(candidates, ",")+`]}`))
	if err != nil || len(got.PersonCandidates) != WorkInfoPluginMaxCandidates ||
		got.PersonCandidates[0] != (PersonCandidate{Source: "t", ID: "0", Name: "n0", URL: "https://e.test/0"}) {
		t.Fatalf("candidates = %d, %v; want 50 starting with t:0", len(got.PersonCandidates), err)
	}
}

func TestParsePersonResponseFailures(t *testing.T) {
	search := WorkInfoRunRequest{Operation: WorkInfoSearch, Kind: WorkInfoPluginKindPerson, Sources: []string{"s"}}
	fetch := WorkInfoRunRequest{Operation: WorkInfoFetch, Kind: WorkInfoPluginKindPerson, Source: "s", ID: "1"}
	cases := []struct {
		name   string
		req    WorkInfoRunRequest
		output string
	}{
		{"candidate with an undeclared source", search, `{"candidates":[{"source":"x","id":"1","name":"n","url":"https://e.test/1"}]}`},
		{"candidate without a name", search, `{"candidates":[{"source":"s","id":"1","url":"https://e.test/1"}]}`},
		{"person without a name", fetch, `{"person":{"name":" "}}`},
		{"work-info result", fetch, `{"workInfo":{"title":"t"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRunResponse(tc.req, []byte(tc.output))
			var runErr *WorkInfoRunError
			if !errors.As(err, &runErr) || runErr.Failure != WorkInfoRunFailed {
				t.Fatalf("err = %v, want plugin_failed", err)
			}
		})
	}
}
