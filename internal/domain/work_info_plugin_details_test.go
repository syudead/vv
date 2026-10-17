package domain

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func parseFetched(t *testing.T, workInfo string) FetchedWorkInfo {
	t.Helper()
	got, err := ParseWorkInfoResponse(WorkInfoFetch, []byte(`{"workInfo":`+workInfo+`}`))
	if err != nil {
		t.Fatalf("ParseWorkInfoResponse(%s) = %v", workInfo, err)
	}
	return got.WorkInfo
}

func TestParseWorkInfoResponseReadsEveryWorkDetailInOrder(t *testing.T) {
	got := parseFetched(t, `{
		"title": "T", "tags": ["a"],
		"code": " ABC-123 ",
		"description": "  line 1\nline 2\n ",
		"dates": [{"kind": "recorded", "date": "2024-02"}, {"kind": " release ", "date": " 2024-03-01 "}],
		"people": [
			{"role": "performer", "name": " Hanako ", "externalIds": [{"source": " site ", "value": " p1 "}]},
			{"role": "director", "name": "Taro"}
		],
		"organizations": [{"role": "maker", "name": "Studio", "externalIds": []}, {"role": "label", "name": "Label"}],
		"urls": [" https://e.test/b ", "https://e.test/a"],
		"externalIds": [{"source": "site", "value": "w1"}, {"source": "other", "value": "w2"}]
	}`)
	want := FetchedWorkInfo{
		Title:       "T",
		Tags:        []string{"a"},
		Code:        "ABC-123",
		Description: "line 1\nline 2",
		Dates:       []WorkDate{{DateRecorded, "2024-02"}, {DateRelease, "2024-03-01"}},
		People: []FetchedPerson{
			{PersonPerformer, "Hanako", []ExternalID{{"site", "p1"}}},
			{PersonDirector, "Taro", nil},
		},
		Organizations: []FetchedOrganization{
			{OrganizationMaker, "Studio", nil},
			{OrganizationLabel, "Label", nil},
		},
		URLs:        []string{"https://e.test/b", "https://e.test/a"},
		ExternalIDs: []ExternalID{{"site", "w1"}, {"other", "w2"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fetched = %+v\nwant      %+v", got, want)
	}
}

func TestParseWorkInfoResponseDropsUnknownAndEmptyEntries(t *testing.T) {
	got := parseFetched(t, `{
		"code": "  ",
		"dates": [{"kind": "birthday", "date": "2024"}, {"date": "2024"}, {"kind": "release", "date": " "},
			{"kind": "release", "date": "2024-03-01"}, {"kind": "release", "date": "2025-01-01"}],
		"people": [{"role": "singer", "name": "A"}, {"name": "B"}, {"role": "performer", "name": " "},
			{"role": "performer", "name": "C",
			 "externalIds": [{"source": "s"}, {"value": "v"}, {"source": " ", "value": "v"}]}],
		"organizations": [{"role": "publisher", "name": "O"}],
		"urls": ["", "  "],
		"externalIds": [{"source": "s", "value": ""}]
	}`)
	want := FetchedWorkInfo{
		Tags:   []string{},
		Dates:  []WorkDate{{DateRelease, "2024-03-01"}},
		People: []FetchedPerson{{PersonPerformer, "C", nil}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fetched = %+v\nwant      %+v", got, want)
	}
}

func TestParseWorkInfoResponseKeepsRuleBreakingValues(t *testing.T) {
	long := strings.Repeat("x", WorkInfoLimits.CodeMaxLength+1)
	got := parseFetched(t,
		`{"code": "`+long+`", "dates": [{"kind": "release", "date": "March 2024"}], "urls": ["/relative"]}`)
	if got.Code != long || got.Dates[0].Date != "March 2024" || got.URLs[0] != "/relative" {
		t.Fatalf("fetched = %+v", got)
	}
}

func TestParseWorkInfoResponseKeepsTheFirstEntriesUpToEachLimit(t *testing.T) {
	var people, orgs, urls, ids, personIDs []string
	for i := range WorkInfoLimits.PeoplePerRoleMaxCount + 1 {
		people = append(people, fmt.Sprintf(`{"role":"performer","name":"p%d"}`, i))
	}
	people = append(people, `{"role":"director","name":"d"}`)
	for i := range ExternalIDsMaxCount + 1 {
		personIDs = append(personIDs, fmt.Sprintf(`{"source":"s","value":"%d"}`, i))
	}
	people = append(people, `{"role":"streamer","name":"s","externalIds":[`+strings.Join(personIDs, ",")+`]}`)
	for i := range WorkInfoLimits.OrganizationsPerRoleMaxCount + 1 {
		orgs = append(orgs, fmt.Sprintf(`{"role":"maker","name":"o%d"}`, i))
	}
	urls = append(urls, `"https://e.test/0"`, `" https://e.test/0 "`)
	for i := range WorkInfoLimits.URLsMaxCount + 1 {
		urls = append(urls, fmt.Sprintf(`"https://e.test/%d"`, i))
	}
	ids = append(ids, `{"source":"s","value":"0"}`, `{"source":" s ","value":"0"}`)
	for i := range ExternalIDsMaxCount + 1 {
		ids = append(ids, fmt.Sprintf(`{"source":"s","value":"%d"}`, i))
	}
	got := parseFetched(t, `{"people":[`+strings.Join(people, ",")+`],"organizations":[`+strings.Join(orgs, ",")+
		`],"urls":[`+strings.Join(urls, ",")+`],"externalIds":[`+strings.Join(ids, ",")+`]}`)

	limit := WorkInfoLimits.PeoplePerRoleMaxCount
	if len(got.People) != limit+2 || got.People[limit-1].Name != fmt.Sprintf("p%d", limit-1) ||
		got.People[limit].Role != PersonDirector {
		t.Fatalf("people = %d, last performer %+v", len(got.People), got.People[limit-1])
	}
	streamer := got.People[limit+1]
	if len(streamer.ExternalIDs) != ExternalIDsMaxCount || streamer.ExternalIDs[ExternalIDsMaxCount-1].Value != "19" {
		t.Fatalf("streamer external IDs = %+v", streamer.ExternalIDs)
	}
	if len(got.Organizations) != WorkInfoLimits.OrganizationsPerRoleMaxCount {
		t.Fatalf("organizations = %d", len(got.Organizations))
	}
	if len(got.URLs) != WorkInfoLimits.URLsMaxCount || got.URLs[19] != "https://e.test/19" {
		t.Fatalf("urls = %v", got.URLs)
	}
	if len(got.ExternalIDs) != ExternalIDsMaxCount || got.ExternalIDs[0] != (ExternalID{"s", "0"}) ||
		got.ExternalIDs[1] != (ExternalID{"s", "1"}) {
		t.Fatalf("external IDs = %v", got.ExternalIDs)
	}
}

func TestParseWorkInfoResponseKeepsTheSamePersonInOneRoleOnce(t *testing.T) {
	got := parseFetched(t, `{"people": [
		{"role": "performer", "name": "A", "externalIds": [{"source": "s", "value": "1"}]},
		{"role": "performer", "name": " A ", "externalIds": [{"source": "s", "value": "1"}]},
		{"role": "director", "name": "A", "externalIds": [{"source": "s", "value": "1"}]},
		{"role": "performer", "name": "A", "externalIds": [{"source": "s", "value": "2"}]}
	]}`)
	if len(got.People) != 3 || got.People[1].Role != PersonDirector || got.People[2].ExternalIDs[0].Value != "2" {
		t.Fatalf("people = %+v", got.People)
	}
}

func TestParseWorkInfoResponseKeepsTheSamePersonOnceWhateverTheExternalIDOrder(t *testing.T) {
	got := parseFetched(t, `{"people": [
		{"role": "performer", "name": "A", "externalIds": [{"source": "t", "value": "2"}, {"source": "s", "value": "1"}]},
		{"role": "performer", "name": "A", "externalIds": [{"source": "s", "value": "1"}, {"source": "t", "value": "2"}]}
	], "organizations": [
		{"role": "maker", "name": "M", "externalIds": [{"source": "t", "value": "2"}, {"source": "s", "value": "1"}]},
		{"role": "maker", "name": "M", "externalIds": [{"source": "s", "value": "1"}, {"source": "t", "value": "2"}]}
	]}`)
	if len(got.People) != 1 || got.People[0].ExternalIDs[0] != (ExternalID{"t", "2"}) {
		t.Fatalf("people = %+v", got.People)
	}
	if len(got.Organizations) != 1 || got.Organizations[0].ExternalIDs[0] != (ExternalID{"t", "2"}) {
		t.Fatalf("organizations = %+v", got.Organizations)
	}
}

func TestParseWorkInfoResponseReadsTitleAndTagsAsBefore(t *testing.T) {
	got := parseFetched(t, `{"title": " T ", "tags": [" a ", "a", ""]}`)
	want := FetchedWorkInfo{Title: " T ", Tags: []string{" a ", "a", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fetched = %#v, want %#v", got, want)
	}
}

func TestParseWorkInfoResponseIgnoresWorkDetailsOfAnotherShape(t *testing.T) {
	got := parseFetched(t, `{"title": "T", "code": 5, "description": ["d"], "dates": {}, "urls": "https://e.test/",
		"people": [{"role": "performer", "name": 1}, "B", {"role": "performer", "name": "C", "externalIds": "x"}],
		"externalIds": [{"source": "s", "value": 1}, {"source": "s", "value": "1"}]}`)
	want := FetchedWorkInfo{
		Title:       "T",
		Tags:        []string{},
		People:      []FetchedPerson{{PersonPerformer, "C", nil}},
		ExternalIDs: []ExternalID{{"s", "1"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fetched = %+v\nwant      %+v", got, want)
	}
}

func TestParseWorkInfoResponseKeepsOnlyAWebCoverURL(t *testing.T) {
	for raw, want := range map[string]string{
		`"https://example.com/cover.jpg"`: "https://example.com/cover.jpg",
		`"  http://example.com/c.png  "`:  "http://example.com/c.png",
		`"ftp://example.com/cover.jpg"`:   "",
		`"/cover.jpg"`:                    "",
		`{"url":"https://example.com/c"}`: "",
		`"https://example.com/` + strings.Repeat("a", WorkInfoCoverURLMaxRunes) + `"`: "",
	} {
		got, err := ParseWorkInfoResponse(WorkInfoFetch, []byte(`{"workInfo":{"title":"T","cover":`+raw+`}}`))
		if err != nil || got.WorkInfo.CoverURL != want || got.WorkInfo.Title != "T" {
			t.Fatalf("cover %.40s = %q, %v; want %q", raw, got.WorkInfo.CoverURL, err, want)
		}
	}
}
