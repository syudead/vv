package domain

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// 取得した作品の詳細を保存済みの作品情報に重ねる（specs/052-plugin-work-info-fields/research.md R-4、#1158）。

var (
	matchNew       = CreditMatch{State: CreditMatchNew}
	matchAmbiguous = CreditMatch{State: CreditMatchAmbiguous}
)

func matchExisting(id int64, name string) CreditMatch {
	return CreditMatch{State: CreditMatchExisting, Credit: &CreditRef{ID: id, Name: name}}
}

func performers(n int) []WorkPerson {
	out := make([]WorkPerson, n)
	for i := range out {
		out[i] = WorkPerson{Role: PersonPerformer, Person: CreditRef{ID: int64(i + 1)}}
	}
	return out
}

func TestMergeReplacesCodeAndTheSelectedDateKind(t *testing.T) {
	got := MergeFetchedWorkInfo(WorkInfoMergeInput{
		Stored: WorkInfo{Code: "OLD-1", Description: "kept", Dates: []WorkDate{
			{Kind: DateRelease, Date: "2020"}, {Kind: DateRecorded, Date: "2019-05"},
		}},
		Fetched: FetchedWorkInfo{Code: "NEW-2", Description: "not selected", Dates: []WorkDate{
			{Kind: DateRelease, Date: "2024-03-01"}, {Kind: DateRecorded, Date: "2023"},
		}},
		Fields: []WorkInfoField{WorkInfoFieldCode, DateField(DateRelease)},
	})
	want := WorkInfo{Code: "NEW-2", Description: "kept", Dates: []WorkDate{
		{Kind: DateRelease, Date: "2024-03-01"}, {Kind: DateRecorded, Date: "2019-05"},
	}, People: []WorkPerson{}, Organizations: []WorkOrganization{}}
	if !reflect.DeepEqual(got.Info, want) || len(got.Skipped) != 0 {
		t.Fatalf("merge = %+v / %+v, want %+v and nothing skipped", got.Info, got.Skipped, want)
	}
}

func TestMergeAppendsPerformersAndURLsWithoutRepeats(t *testing.T) {
	got := MergeFetchedWorkInfo(WorkInfoMergeInput{
		Stored: WorkInfo{
			People: []WorkPerson{{Role: PersonPerformer, Person: CreditRef{ID: 1, Name: "Aoi"}}},
			URLs:   []string{"https://example.com/a"},
		},
		Fetched: FetchedWorkInfo{
			People: []FetchedPerson{
				{Role: PersonPerformer, Name: "Aoi"},
				{Role: PersonPerformer, Name: "Mika"},
				{Role: PersonPerformer, Name: "Mika again"},
				{Role: PersonDirector, Name: "Not selected"},
			},
			URLs: []string{"https://example.com/a", "https://example.com/b"},
		},
		PersonMatches: []CreditMatch{matchExisting(1, "Aoi"), matchExisting(2, "Mika"), matchExisting(2, "Mika"), matchNew},
		Fields:        []WorkInfoField{PersonField(PersonPerformer), WorkInfoFieldURLs},
	})
	wantPeople := []WorkPerson{
		{Role: PersonPerformer, Person: CreditRef{ID: 1, Name: "Aoi"}},
		{Role: PersonPerformer, Person: CreditRef{ID: 2, Name: "Mika"}},
	}
	if !reflect.DeepEqual(got.Info.People, wantPeople) {
		t.Errorf("people = %+v, want %+v", got.Info.People, wantPeople)
	}
	if want := []string{"https://example.com/a", "https://example.com/b"}; !reflect.DeepEqual(got.Info.URLs, want) {
		t.Errorf("urls = %v, want %v", got.Info.URLs, want)
	}
	if len(got.NewPeople) != 0 || len(got.Skipped) != 0 {
		t.Errorf("new = %+v, skipped = %+v; want none", got.NewPeople, got.Skipped)
	}
}

func TestMergeSkipsInvalidValuesAndAppliesTheRestOfTheRow(t *testing.T) {
	long := strings.Repeat("あ", WorkInfoLimits.DescriptionMaxLength+1)
	got := MergeFetchedWorkInfo(WorkInfoMergeInput{
		Fetched: FetchedWorkInfo{
			Code: "ABC-1", Description: long,
			Dates:       []WorkDate{{Kind: DateRelease, Date: "2024/03/01"}, {Kind: DatePublished, Date: "2024-04"}},
			URLs:        []string{"example.com/no-scheme", "https://example.com/ok"},
			ExternalIDs: []ExternalID{{Source: "fanza", Value: strings.Repeat("x", ExternalIDValueMaxLength+1)}, {Source: "fanza", Value: "ok"}},
		},
		Fields: []WorkInfoField{
			WorkInfoFieldExternalIDs, WorkInfoFieldURLs, DateField(DateRelease), DateField(DatePublished),
			WorkInfoFieldDescription, WorkInfoFieldCode,
		},
	})
	if got.Info.Code != "ABC-1" || got.Info.Description != "" {
		t.Errorf("code, description = %q, %q; want the code only", got.Info.Code, got.Info.Description)
	}
	if want := []WorkDate{{Kind: DatePublished, Date: "2024-04"}}; !reflect.DeepEqual(got.Info.Dates, want) {
		t.Errorf("dates = %+v, want %+v", got.Info.Dates, want)
	}
	if want := []string{"https://example.com/ok"}; !reflect.DeepEqual(got.Info.URLs, want) {
		t.Errorf("urls = %v, want %v", got.Info.URLs, want)
	}
	if want := []ExternalID{{Source: "fanza", Value: "ok"}}; !reflect.DeepEqual(got.Info.ExternalIDs, want) {
		t.Errorf("externalIds = %v, want %v", got.Info.ExternalIDs, want)
	}
	want := []SkippedWorkInfoValue{
		{Field: WorkInfoFieldDescription, Value: strings.Repeat("あ", SkippedDescriptionMaxRunes), Reason: SkippedWorkInfoInvalid},
		{Field: DateField(DateRelease), Value: "2024/03/01", Reason: SkippedWorkInfoInvalid},
		{Field: WorkInfoFieldURLs, Value: "example.com/no-scheme", Reason: SkippedWorkInfoInvalid},
		{Field: WorkInfoFieldExternalIDs, Value: "fanza: " + strings.Repeat("x", ExternalIDValueMaxLength+1), Reason: SkippedWorkInfoInvalid},
	}
	if !reflect.DeepEqual(got.Skipped, want) {
		t.Errorf("skipped = %+v, want %+v", got.Skipped, want)
	}
}

func TestMergeSkipsThePerformerOverTheLimitAsTooMany(t *testing.T) {
	limit := WorkInfoLimits.PeoplePerRoleMaxCount
	got := MergeFetchedWorkInfo(WorkInfoMergeInput{
		Stored: WorkInfo{People: performers(limit - 1)},
		Fetched: FetchedWorkInfo{People: []FetchedPerson{
			{Role: PersonPerformer, Name: "Hundredth"}, {Role: PersonPerformer, Name: "Over"},
		}},
		PersonMatches: []CreditMatch{matchExisting(500, "Hundredth"), matchNew},
		Fields:        []WorkInfoField{PersonField(PersonPerformer)},
	})
	if len(got.Info.People) != limit || got.Info.People[limit-1].Person.ID != 500 {
		t.Errorf("people = %d, last %+v; want %d ending with 500", len(got.Info.People), got.Info.People[len(got.Info.People)-1], limit)
	}
	want := []SkippedWorkInfoValue{{Field: PersonField(PersonPerformer), Value: "Over", Reason: SkippedWorkInfoTooMany}}
	if !reflect.DeepEqual(got.Skipped, want) || len(got.NewPeople) != 0 {
		t.Errorf("skipped = %+v, new = %+v; want %+v and nothing created", got.Skipped, got.NewPeople, want)
	}
}

func TestMergeChecksNewEntitiesBeforeKeepingThem(t *testing.T) {
	tooLong := strings.Repeat("名", CreditNameMaxLength+1)
	got := MergeFetchedWorkInfo(WorkInfoMergeInput{
		Fetched: FetchedWorkInfo{People: []FetchedPerson{
			{Role: PersonPerformer, Name: ""},
			{Role: PersonPerformer, Name: tooLong},
			{Role: PersonPerformer, Name: "Rin", ExternalIDs: []ExternalID{{Source: "fanza", Value: "r-1"}}},
			{Role: PersonPerformer, Name: "Two"},
			{Role: PersonDirector, Name: "ＲＩＮ"},
		}},
		PersonMatches: []CreditMatch{matchNew, matchNew, matchNew, matchAmbiguous, matchNew},
		Fields:        []WorkInfoField{PersonField(PersonPerformer), PersonField(PersonDirector)},
	})
	wantSkipped := []SkippedWorkInfoValue{
		{Field: PersonField(PersonPerformer), Value: "", Reason: SkippedWorkInfoInvalid},
		{Field: PersonField(PersonPerformer), Value: tooLong, Reason: SkippedWorkInfoInvalid},
		{Field: PersonField(PersonPerformer), Value: "Two", Reason: SkippedWorkInfoAmbiguous},
	}
	if !reflect.DeepEqual(got.Skipped, wantSkipped) {
		t.Errorf("skipped = %+v, want %+v", got.Skipped, wantSkipped)
	}
	// 同じ照合形の名前は役割が違っても 1 つの実体にまとめ、両方の位置に結ぶ。
	wantNew := []NewCredit{{
		Input: CreditInput{Name: "Rin", Aliases: []Alias{}, ExternalIDs: []ExternalID{{Source: "fanza", Value: "r-1"}}},
		Links: []int{0, 1},
	}}
	if !reflect.DeepEqual(got.NewPeople, wantNew) {
		t.Errorf("new = %+v, want %+v", got.NewPeople, wantNew)
	}
	wantPeople := []WorkPerson{
		{Role: PersonPerformer, Person: CreditRef{Name: "Rin"}},
		{Role: PersonDirector, Person: CreditRef{Name: "Rin"}},
	}
	if !reflect.DeepEqual(got.Info.People, wantPeople) {
		t.Errorf("people = %+v, want %+v", got.Info.People, wantPeople)
	}
}

func TestMergeLeavesUnselectedAndMissingFieldsAlone(t *testing.T) {
	stored := WorkInfo{Code: "KEEP", URLs: []string{"https://example.com/a"}}
	got := MergeFetchedWorkInfo(WorkInfoMergeInput{
		Stored:  stored,
		Fetched: FetchedWorkInfo{URLs: []string{"https://example.com/b"}},
		Fields:  []WorkInfoField{WorkInfoFieldTitle, WorkInfoFieldCode},
	})
	if got.Info.Code != "KEEP" || fmt.Sprint(got.Info.URLs) != "[https://example.com/a]" || len(got.Skipped) != 0 {
		t.Errorf("merge = %+v / %+v; want the stored value unchanged", got.Info, got.Skipped)
	}
}

func TestWorkInfoFieldsFollowTheContractOrder(t *testing.T) {
	want := []WorkInfoField{
		"title", "tags", "code", "description", "dates.release", "dates.recorded", "dates.published",
		"people.performer", "people.director", "people.photographer", "people.streamer",
		"organizations.maker", "organizations.label", "urls", "externalIds",
	}
	if !reflect.DeepEqual(WorkInfoFields, want) {
		t.Errorf("WorkInfoFields = %v, want %v", WorkInfoFields, want)
	}
}
