package domain

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

var personFillToday = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestFillPersonItemsFillsOnlyEmptyProfileFields(t *testing.T) {
	stored, fetchedHeight, invalidHeight := 160, 170, 1000
	current := Credit{Name: "Hanako", Profile: PersonProfile{BirthDate: "1990", HeightCm: &stored}}
	fetched := FetchedPluginPerson{Name: "Hanako", Profile: PersonProfile{
		BirthDate: "1985", HeightCm: &fetchedHeight, Birthplace: "Tokyo", BustCm: &invalidHeight,
	}}
	items := []PersonPluginItem{
		PersonProfileItem(ProfileBirthDate), PersonProfileItem(ProfileHeightCm), PersonProfileItem(ProfileBirthplace),
		PersonProfileItem(ProfileBustCm),
	}

	got := FillPersonItems(PersonItemFill{Current: current, Fetched: fetched, Items: items, Today: personFillToday})

	if got.Profile.BirthDate != "1990" || *got.Profile.HeightCm != 160 || got.Profile.Birthplace != "Tokyo" ||
		got.Profile.BustCm != nil {
		t.Fatalf("profile = %+v; want stored values kept, the empty birthplace filled and the invalid bust left out",
			got.Profile)
	}
	want := []SkippedPersonValue{{Item: PersonProfileItem(ProfileBustCm), Value: "1000", Reason: SkippedWorkInfoInvalid}}
	if !slices.Equal(got.Skipped, want) {
		t.Fatalf("skipped = %+v; want %+v", got.Skipped, want)
	}
}

func TestFillPersonItemsReplacesSelectedProfileFields(t *testing.T) {
	stored, fetchedHeight, invalidBust, storedBust := 160, 170, 1000, 80
	current := Credit{Name: "Hanako", Profile: PersonProfile{
		BirthDate: "1990", HeightCm: &stored, BustCm: &storedBust, Birthplace: "Osaka",
	}}
	fetched := FetchedPluginPerson{Name: "Hanako", Profile: PersonProfile{
		BirthDate: "1985", HeightCm: &fetchedHeight, BustCm: &invalidBust, Birthplace: "Tokyo",
	}}
	items := []PersonPluginItem{
		PersonProfileItem(ProfileHeightCm), PersonProfileItem(ProfileBustCm), PersonProfileItem(ProfileBirthplace),
	}

	got := FillPersonItems(PersonItemFill{
		Current: current, Fetched: fetched, Items: items, Today: personFillToday, Replace: true,
	})

	if got.Profile.BirthDate != "1990" || *got.Profile.HeightCm != 170 || *got.Profile.BustCm != 80 ||
		got.Profile.Birthplace != "Tokyo" {
		t.Fatalf("profile = %+v; want the selected fields replaced, the unselected and the invalid one kept",
			got.Profile)
	}
	want := []SkippedPersonValue{{Item: PersonProfileItem(ProfileBustCm), Value: "1000", Reason: SkippedWorkInfoInvalid}}
	if !slices.Equal(got.Skipped, want) {
		t.Fatalf("skipped = %+v; want %+v", got.Skipped, want)
	}
}

func TestFillPersonItemsReplacesDependentFieldsTogether(t *testing.T) {
	storedStart, storedEnd, fetchedStart, fetchedEnd := 1990, 2000, 2010, 2020
	current := Credit{Name: "Hanako", Profile: PersonProfile{CareerStartYear: &storedStart, CareerEndYear: &storedEnd}}
	fetched := FetchedPluginPerson{Name: "Hanako", Profile: PersonProfile{
		CareerStartYear: &fetchedStart, CareerEndYear: &fetchedEnd,
	}}

	got := FillPersonItems(PersonItemFill{
		Current: current, Fetched: fetched, Today: personFillToday, Replace: true,
		Items: []PersonPluginItem{PersonProfileItem(ProfileCareerStartYear), PersonProfileItem(ProfileCareerEndYear)},
	})

	if *got.Profile.CareerStartYear != 2010 || *got.Profile.CareerEndYear != 2020 || len(got.Skipped) != 0 {
		t.Fatalf("profile = %+v, skipped %+v; want both career years replaced", got.Profile, got.Skipped)
	}
}

func TestFillPersonItemsAppliesOnlySelectedItems(t *testing.T) {
	fetched := FetchedPluginPerson{
		Name: "Hanako", Aliases: []Alias{{Name: "Hana"}}, ExternalIDs: []ExternalID{{Source: "wiki", Value: "1"}},
		Profile: PersonProfile{Birthplace: "Tokyo"},
	}
	got := FillPersonItems(PersonItemFill{
		Current: Credit{Name: "Hanako"}, Fetched: fetched, Items: []PersonPluginItem{PersonItemAliases},
		Today: personFillToday,
	})
	if len(got.Aliases) != 1 || len(got.ExternalIDs) != 0 || got.Profile.Birthplace != "" {
		t.Fatalf("filled = %+v; want only the aliases", got)
	}
}

func TestFillPersonItemsAppendsAliasesUpToTheLimit(t *testing.T) {
	current := Credit{Name: "Hanako"}
	for i := range CreditAliasesMaxCount - 1 {
		current.Aliases = append(current.Aliases, Alias{Name: fmt.Sprintf("stored %d", i)})
	}
	fetched := FetchedPluginPerson{Name: "Hanako", Aliases: []Alias{
		{Name: "Hanako"}, {Name: "stored 0"}, {Name: "bad", Language: "not a tag"}, {Name: "Hana", Language: "ja"},
		{Name: "Hanachan"},
	}}

	got := FillPersonItems(PersonItemFill{
		Current: current, Fetched: fetched, Items: []PersonPluginItem{PersonItemAliases}, Today: personFillToday,
	})

	if len(got.Aliases) != CreditAliasesMaxCount || got.Aliases[CreditAliasesMaxCount-1] != (Alias{Name: "Hana", Language: "ja"}) {
		t.Fatalf("aliases end with %+v (%d); want Hana appended as the 50th", got.Aliases[len(got.Aliases)-1], len(got.Aliases))
	}
	want := []SkippedPersonValue{
		{Item: PersonItemAliases, Value: "bad", Reason: SkippedWorkInfoInvalid},
		{Item: PersonItemAliases, Value: "Hanachan", Reason: SkippedWorkInfoTooMany},
	}
	if !slices.Equal(got.Skipped, want) {
		t.Fatalf("skipped = %+v; want %+v and nothing for the name or a stored alias", got.Skipped, want)
	}
}

func TestFillPersonItemsAppendsFreeExternalIDsUpToTheLimit(t *testing.T) {
	current := Credit{Name: "Hanako"}
	for i := range ExternalIDsMaxCount - 1 {
		current.ExternalIDs = append(current.ExternalIDs, ExternalID{Source: "stored", Value: fmt.Sprint(i)})
	}
	taken := ExternalID{Source: "wiki", Value: "taken"}
	fetched := FetchedPluginPerson{Name: "Hanako", ExternalIDs: []ExternalID{
		{Source: "stored", Value: "0"}, taken, {Source: "wiki", Value: " "}, {Source: "wiki", Value: "new"},
		{Source: "wiki", Value: "over"},
	}}

	got := FillPersonItems(PersonItemFill{
		Current: current, Fetched: fetched, Items: []PersonPluginItem{PersonItemExternalIDs},
		Holders: map[ExternalID]string{taken: "Other"}, Today: personFillToday,
	})

	if len(got.ExternalIDs) != ExternalIDsMaxCount || got.ExternalIDs[ExternalIDsMaxCount-1] != (ExternalID{Source: "wiki", Value: "new"}) {
		t.Fatalf("external IDs = %+v; want wiki new appended as the 20th", got.ExternalIDs)
	}
	want := []SkippedPersonValue{
		{Item: PersonItemExternalIDs, Value: "wiki: taken", Reason: SkippedWorkInfoTaken, HolderName: "Other"},
		{Item: PersonItemExternalIDs, Value: "wiki:  ", Reason: SkippedWorkInfoInvalid},
		{Item: PersonItemExternalIDs, Value: "wiki: over", Reason: SkippedWorkInfoTooMany},
	}
	if !slices.Equal(got.Skipped, want) {
		t.Fatalf("skipped = %+v; want %+v", got.Skipped, want)
	}
}

func TestPersonFetchIDPicksTheFirstAcceptedStoredID(t *testing.T) {
	stored := []ExternalID{{Source: "a", Value: "1"}, {Source: "site", Value: "2"}, {Source: "wiki", Value: "3"}}
	if got, ok := PersonFetchID(stored, []string{"wiki", "site"}); !ok || got != stored[1] {
		t.Fatalf("PersonFetchID = %+v, %v; want the first stored ID in stored order, not in source order", got, ok)
	}
	if _, ok := PersonFetchID(stored, []string{"other"}); ok {
		t.Fatal("PersonFetchID found an ID for an undeclared source")
	}
}

func TestPersonPluginItemValid(t *testing.T) {
	for item, want := range map[PersonPluginItem]bool{
		"image": true, "aliases": true, "externalIds": true, "profile.heightCm": true, "profile.name": false, "name": false,
	} {
		if got := item.Valid(); got != want {
			t.Errorf("%q.Valid() = %v, want %v", item, got, want)
		}
	}
}

func TestSelectNarrowsTheFetchedPersonToTheChosenValues(t *testing.T) {
	fetched := FetchedPluginPerson{
		Name: "Hanako Yamada", Aliases: []Alias{{Name: "Hana"}, {Name: "Hanachan"}},
		ExternalIDs: []ExternalID{{Source: "wiki", Value: "1"}, {Source: "site", Value: "2"}},
		Profile:     PersonProfile{Birthplace: "Tokyo", Links: []string{"https://a.test", "https://b.test"}},
	}
	got, items, ok := fetched.Select(PersonPluginSelection{
		Items:   []PersonPluginItem{PersonProfileItem(ProfileBirthplace)},
		Aliases: []string{"Hanachan", "Hanako Yamada"}, ExternalIDs: []ExternalID{{Source: "site", Value: "2"}},
		Links: []string{"https://b.test"},
	}, false)

	wantItems := []PersonPluginItem{
		PersonProfileItem(ProfileBirthplace), PersonProfileItem(ProfileLinks), PersonItemAliases, PersonItemExternalIDs,
	}
	if !ok || !slices.Equal(items, wantItems) {
		t.Fatalf("items = %v, %v; want %v", items, ok, wantItems)
	}
	// The fetched name is an alias choice, and chosen values keep the fetched order.
	if !slices.Equal(got.Aliases, []Alias{{Name: "Hanako Yamada"}, {Name: "Hanachan"}}) ||
		!slices.Equal(got.ExternalIDs, []ExternalID{{Source: "site", Value: "2"}}) ||
		!slices.Equal(got.Profile.Links, []string{"https://b.test"}) || got.Profile.Birthplace != "Tokyo" {
		t.Fatalf("narrowed = %+v", got)
	}
}

func TestSelectRefusesASelectionTheResultCannotGive(t *testing.T) {
	fetched := FetchedPluginPerson{Name: "Hanako", Aliases: []Alias{{Name: "Hana"}}}
	for _, tc := range []struct {
		name      string
		selection PersonPluginSelection
		hasImage  bool
	}{
		{"nothing", PersonPluginSelection{}, true},
		{"a list as an item", PersonPluginSelection{Items: []PersonPluginItem{PersonItemAliases}}, true},
		{"a value not fetched", PersonPluginSelection{Aliases: []string{"Hana", "Other"}}, true},
		{"a field without a value", PersonPluginSelection{Items: []PersonPluginItem{PersonProfileItem(ProfileCup)}}, true},
		{"the image without one", PersonPluginSelection{Items: []PersonPluginItem{PersonItemImage}}, false},
	} {
		if _, _, ok := fetched.Select(tc.selection, tc.hasImage); ok {
			t.Errorf("%s: Select accepted %+v", tc.name, tc.selection)
		}
	}
}

func TestFillPersonItemsAppendsLinksEvenWhenReplacing(t *testing.T) {
	stored := make([]string, PersonLinksMaxCount-1)
	for i := range stored {
		stored[i] = fmt.Sprintf("https://s.test/%d", i)
	}
	fetched := FetchedPluginPerson{Name: "Hanako", Profile: PersonProfile{
		Links: []string{stored[0], "https://new.test/1", "https://new.test/2"},
	}}

	got := FillPersonItems(PersonItemFill{
		Current: Credit{Name: "Hanako", Profile: PersonProfile{Links: stored}}, Fetched: fetched,
		Items: []PersonPluginItem{PersonProfileItem(ProfileLinks)}, Today: personFillToday, Replace: true,
	})

	if want := append(slices.Clone(stored), "https://new.test/1"); !slices.Equal(got.Profile.Links, want) {
		t.Fatalf("links = %v; want the stored ones and the first new one", got.Profile.Links)
	}
	want := []SkippedPersonValue{{
		Item: PersonProfileItem(ProfileLinks), Value: "https://new.test/2", Reason: SkippedWorkInfoTooMany,
	}}
	if !slices.Equal(got.Skipped, want) {
		t.Fatalf("skipped = %+v; want %+v", got.Skipped, want)
	}
}
