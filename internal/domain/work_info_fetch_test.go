package domain

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"
)

func TestWorkInfoPluginEffectiveValues(t *testing.T) {
	settings := []WorkInfoPluginSetting{
		{Key: "unset", Label: "L", Required: true, Default: "d1"},
		{Key: "stored", Label: "L", Default: "d2"},
		{Key: "stale", Label: "L", Default: "en", Options: []string{"ja", "en"}},
		{Key: "chosen", Label: "L", Default: "en", Options: []string{"ja", "en"}},
		{Key: "free", Label: "L"},
		{Key: "needed", Label: "L", Required: true},
		{Key: "staleNoDefault", Label: "L", Options: []string{"a", "b"}},
	}
	values := map[string]string{"stored": "mine", "stale": "fr", "chosen": "ja", "staleNoDefault": "c", "other": "x"}

	got := DeclaredWorkInfoPluginValues(settings, values)
	want := map[string]string{"unset": "d1", "stored": "mine", "stale": "en", "chosen": "ja"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request settings = %v, want %v", got, want)
	}
	if missing := MissingWorkInfoPluginSettings(settings, values); !reflect.DeepEqual(missing, []string{"needed"}) {
		t.Fatalf("missing = %v, want [needed]", missing)
	}
	for _, tc := range []struct {
		key   string
		value string
		stale bool
	}{{"stale", "en", true}, {"chosen", "ja", false}, {"staleNoDefault", "", true}, {"unset", "d1", false}} {
		var s WorkInfoPluginSetting
		for _, d := range settings {
			if d.Key == tc.key {
				s = d
			}
		}
		if value, stale := EffectiveWorkInfoPluginValue(s, values); value != tc.value || stale != tc.stale {
			t.Fatalf("%s = %q, stale %v; want %q, %v", tc.key, value, stale, tc.value, tc.stale)
		}
	}
}

func TestWorkInfoPluginEntryReportsAStaleValue(t *testing.T) {
	plugin := WorkInfoPlugin{
		Manifest: WorkInfoPluginManifest{Settings: []WorkInfoPluginSetting{
			{Key: "lang", Label: "L", Default: "en", Options: []string{"ja", "en"}},
			{Key: "other", Label: "L", Options: []string{"ja", "en"}},
		}},
		Values: map[string]string{"lang": "fr", "other": "ja"},
	}
	settings := plugin.Entry().Settings
	if s := settings[0]; !s.Set || s.Value != "fr" || s.StaleValue != "fr" || s.Default != "en" {
		t.Fatalf("stale entry = %+v", s)
	}
	if s := settings[1]; !s.Set || s.Value != "ja" || s.StaleValue != "" {
		t.Fatalf("entry inside the options = %+v", s)
	}
}

func TestWorkInfoRunSettingsCarriesEffectiveValuesOrReportsTheMissingKeys(t *testing.T) {
	settings := []WorkInfoPluginSetting{
		{Key: "token", Required: true, Secret: true},
		{Key: "region", Required: true, Default: "jp", Options: []string{"jp", "us"}},
		{Key: "note"},
	}
	got, err := WorkInfoRunSettings(settings, map[string]string{"token": "t", "other": "x"})
	if err != nil || !maps.Equal(got, map[string]string{"token": "t", "region": "jp"}) {
		t.Fatalf("WorkInfoRunSettings = %v, %v; want the token and the default region only", got, err)
	}
	_, err = WorkInfoRunSettings(settings, map[string]string{"region": "us"})
	var missing *WorkInfoSettingsMissingError
	if !errors.As(err, &missing) || !slices.Equal(missing.Keys, []string{"token"}) {
		t.Fatalf("err = %v, want token missing", err)
	}
}

func TestKeptWorkInfoTitleDropsATitleThatBreaksTheDisplayNameRule(t *testing.T) {
	if got := KeptWorkInfoTitle("  Title  "); got != "Title" {
		t.Fatalf("KeptWorkInfoTitle = %q, want the trimmed title", got)
	}
	if got := KeptWorkInfoTitle("   "); got != "" {
		t.Fatalf("KeptWorkInfoTitle(blank) = %q, want none", got)
	}
}

func selectableWorkInfo() FetchedWorkInfo {
	return FetchedWorkInfo{
		Code: "ABC-1", Description: "d",
		People: []FetchedPerson{
			{Role: PersonPerformer, Name: "A"}, {Role: PersonDirector, Name: "B"}, {Role: PersonPerformer, Name: "C"},
		},
		Organizations: []FetchedOrganization{{Role: OrganizationMaker, Name: "M"}, {Role: OrganizationLabel, Name: "L"}},
		URLs:          []string{"https://e.test/1", "https://e.test/2"},
		ExternalIDs:   []ExternalID{{Source: "s", Value: "1"}, {Source: "s", Value: "2"}},
	}
}

func TestFetchedWorkInfoSelectNarrowsToTheChosenEntriesAndValues(t *testing.T) {
	got, fields, ok := selectableWorkInfo().Select(WorkInfoSelection{
		Fields: []WorkInfoField{WorkInfoFieldCover, WorkInfoFieldCode, WorkInfoFieldTitle},
		People: []int{2, 0}, Organizations: []int{1},
		URLs: []string{"https://e.test/2"}, ExternalIDs: []ExternalID{{Source: "s", Value: "1"}},
	})

	// Whole fields other than the detail ones are the caller's; list fields follow the chosen entries' roles.
	wantFields := []WorkInfoField{
		WorkInfoFieldCode, PersonField(PersonPerformer), OrganizationField(OrganizationLabel),
		WorkInfoFieldURLs, WorkInfoFieldExternalIDs,
	}
	if !ok || !slices.Equal(fields, wantFields) {
		t.Fatalf("fields = %v, %v; want %v", fields, ok, wantFields)
	}
	want := selectableWorkInfo()
	want.People = []FetchedPerson{want.People[0], want.People[2]}
	want.Organizations = want.Organizations[1:]
	want.URLs, want.ExternalIDs = want.URLs[1:], want.ExternalIDs[:1]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("narrowed = %+v\nwant       %+v", got, want)
	}
}

func TestFetchedWorkInfoSelectRefusesASelectionTheResultCannotGive(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection WorkInfoSelection
	}{
		{"a role as a whole field", WorkInfoSelection{Fields: []WorkInfoField{PersonField(PersonPerformer)}}},
		{"urls as a whole field", WorkInfoSelection{Fields: []WorkInfoField{WorkInfoFieldURLs}}},
		{"a position past the end", WorkInfoSelection{People: []int{3}}},
		{"a negative position", WorkInfoSelection{Organizations: []int{-1}}},
		{"a position twice", WorkInfoSelection{People: []int{0, 0}}},
		{"a URL not fetched", WorkInfoSelection{URLs: []string{"https://e.test/3"}}},
		{"an ID not fetched", WorkInfoSelection{ExternalIDs: []ExternalID{{Source: "s", Value: "3"}}}},
	} {
		if _, _, ok := selectableWorkInfo().Select(tc.selection); ok {
			t.Errorf("%s: Select accepted %+v", tc.name, tc.selection)
		}
	}
}
