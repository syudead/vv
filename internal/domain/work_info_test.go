package domain

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeWorkInfoRefusesEachRule(t *testing.T) {
	performers := func(n int) []WorkPerson {
		out := make([]WorkPerson, n)
		for i := range out {
			out[i] = WorkPerson{Role: PersonPerformer, Person: CreditRef{ID: int64(i + 1)}}
		}
		return out
	}
	makers := make([]WorkOrganization, WorkInfoLimits.OrganizationsPerRoleMaxCount+1)
	for i := range makers {
		makers[i] = WorkOrganization{Role: OrganizationMaker, Organization: CreditRef{ID: int64(i + 1)}}
	}
	urls := make([]string, WorkInfoLimits.URLsMaxCount+1)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/%d", i)
	}
	externalIDs := make([]ExternalID, ExternalIDsMaxCount+1)
	for i := range externalIDs {
		externalIDs[i] = ExternalID{Source: "example", Value: fmt.Sprint(i)}
	}
	cases := []struct {
		name    string
		input   WorkInfo
		field   string
		problem WorkInfoProblem
		limit   int
	}{
		{name: "品番の制御文字", input: WorkInfo{Code: "ABC\n-123"}, field: "code", problem: WorkInfoControlCharacters},
		{name: "長すぎる品番", input: WorkInfo{Code: strings.Repeat("A", 101)}, field: "code", problem: WorkInfoTooLong, limit: 100},
		{name: "説明のタブ", input: WorkInfo{Description: "一行目\n\t二行目"}, field: "description", problem: WorkInfoControlCharacters},
		{name: "長すぎる説明", input: WorkInfo{Description: strings.Repeat("あ", 10_001)}, field: "description", problem: WorkInfoTooLong, limit: 10_000},
		{name: "形の違う日付", input: WorkInfo{Dates: []WorkDate{{Kind: DateRelease, Date: "2024"}, {Kind: DateRecorded, Date: "2024/01/02"}}}, field: "dates[1].date", problem: WorkInfoInvalidDate},
		{name: "1 桁の月", input: WorkInfo{Dates: []WorkDate{{Kind: DateRelease, Date: "2024-1"}}}, field: "dates[0].date", problem: WorkInfoInvalidDate},
		{name: "13 月", input: WorkInfo{Dates: []WorkDate{{Kind: DateRelease, Date: "2024-13"}}}, field: "dates[0].date", problem: WorkInfoInvalidDate},
		{name: "暦に無い日", input: WorkInfo{Dates: []WorkDate{{Kind: DateRelease, Date: "2023-02-29"}}}, field: "dates[0].date", problem: WorkInfoInvalidDate},
		{name: "0 年", input: WorkInfo{Dates: []WorkDate{{Kind: DateRelease, Date: "0000"}}}, field: "dates[0].date", problem: WorkInfoInvalidDate},
		{name: "日付の制御文字", input: WorkInfo{Dates: []WorkDate{{Kind: DateRelease, Date: "2024\n"}}}, field: "dates[0].date", problem: WorkInfoControlCharacters},
		{name: "同じ種類の日付", input: WorkInfo{Dates: []WorkDate{{Kind: DatePublished, Date: "2024"}, {Kind: DatePublished, Date: "2025"}}}, field: "dates[1].kind", problem: WorkInfoDuplicate},
		{name: "同じ役割に同じ人物", input: WorkInfo{People: []WorkPerson{
			{Role: PersonPerformer, Person: CreditRef{ID: 1}}, {Role: PersonDirector, Person: CreditRef{ID: 1}},
			{Role: PersonPerformer, Person: CreditRef{ID: 1}},
		}}, field: "people[2].person.id", problem: WorkInfoDuplicate},
		{name: "役割の人物が多すぎる", input: WorkInfo{People: performers(WorkInfoLimits.PeoplePerRoleMaxCount + 1)}, field: "people", problem: WorkInfoTooMany, limit: 100},
		{name: "同じ役割に同じ団体", input: WorkInfo{Organizations: []WorkOrganization{
			{Role: OrganizationMaker, Organization: CreditRef{ID: 7}}, {Role: OrganizationMaker, Organization: CreditRef{ID: 7}},
		}}, field: "organizations[1].organization.id", problem: WorkInfoDuplicate},
		{name: "役割の団体が多すぎる", input: WorkInfo{Organizations: makers}, field: "organizations", problem: WorkInfoTooMany, limit: 20},
		{name: "相対 URL", input: WorkInfo{URLs: []string{"/works/1"}}, field: "urls[0]", problem: WorkInfoInvalidURL},
		{name: "http でない URL", input: WorkInfo{URLs: []string{"https://example.com", "ftp://example.com/a"}}, field: "urls[1]", problem: WorkInfoInvalidURL},
		{name: "host の無い URL", input: WorkInfo{URLs: []string{"https:///a"}}, field: "urls[0]", problem: WorkInfoInvalidURL},
		{name: "長すぎる URL", input: WorkInfo{URLs: []string{"https://example.com/" + strings.Repeat("a", 2_048)}}, field: "urls[0]", problem: WorkInfoTooLong, limit: 2_048},
		{name: "同じ URL", input: WorkInfo{URLs: []string{"https://example.com/a", "", " https://example.com/a "}}, field: "urls[2]", problem: WorkInfoDuplicate},
		{name: "URL が多すぎる", input: WorkInfo{URLs: urls}, field: "urls", problem: WorkInfoTooMany, limit: 20},
		{name: "source だけ空", input: WorkInfo{ExternalIDs: []ExternalID{{Source: "", Value: "1"}}}, field: "externalIds[0].source", problem: WorkInfoEmpty},
		{name: "同じ外部 ID", input: WorkInfo{ExternalIDs: []ExternalID{{Source: "s", Value: "1"}, {Source: "s", Value: "1"}}}, field: "externalIds[1]", problem: WorkInfoDuplicate},
		{name: "外部 ID が多すぎる", input: WorkInfo{ExternalIDs: externalIDs}, field: "externalIds", problem: WorkInfoTooMany, limit: 20},
		{name: "最初の誤りは項目の順", input: WorkInfo{Code: strings.Repeat("A", 101), URLs: []string{"x"}}, field: "code", problem: WorkInfoTooLong, limit: 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := NormalizeWorkInfo(tc.input)
			var invalid *InvalidWorkInfoError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want *InvalidWorkInfoError", err)
			}
			want := InvalidWorkInfoError{Field: tc.field, Problem: tc.problem, Limit: tc.limit}
			if *invalid != want {
				t.Errorf("err = %+v, want %+v", *invalid, want)
			}
			if !errors.Is(err, ErrInvalidWorkInfo) {
				t.Error("errors.Is(err, ErrInvalidWorkInfo) = false")
			}
		})
	}
}

func TestNormalizeWorkInfoRefusesUnknownKindsAndRoles(t *testing.T) {
	for name, input := range map[string]WorkInfo{
		"日付の種類": {Dates: []WorkDate{{Kind: "birthday", Date: "2024"}}},
		"人物の役割": {People: []WorkPerson{{Role: "maker", Person: CreditRef{ID: 1}}}},
		"団体の役割": {Organizations: []WorkOrganization{{Role: "performer", Organization: CreditRef{ID: 1}}}},
	} {
		if _, _, err := NormalizeWorkInfo(input); !errors.Is(err, ErrInvalidWorkInfo) {
			t.Errorf("%s: err = %v, want ErrInvalidWorkInfo", name, err)
		}
	}
}

func TestNormalizeWorkInfoTrimsClearsAndOrders(t *testing.T) {
	got, empty, err := NormalizeWorkInfo(WorkInfo{
		Code:        "  ABC-123 ",
		Description: " 一行目\n二行目\n ",
		Dates: []WorkDate{
			{Kind: DatePublished, Date: " 2024-02-29 "}, {Kind: DateRelease, Date: "2024"},
			{Kind: DateRecorded, Date: "  "}, {Kind: DateRecorded, Date: "2023-12"},
		},
		People: []WorkPerson{
			{Role: PersonDirector, Person: CreditRef{ID: 3, Name: "無視する"}},
			{Role: PersonPerformer, Person: CreditRef{ID: 2}},
			{Role: PersonPerformer, Person: CreditRef{ID: 1}},
			{Role: PersonStreamer, Person: CreditRef{ID: 2}},
		},
		Organizations: []WorkOrganization{
			{Role: OrganizationLabel, Organization: CreditRef{ID: 5}},
			{Role: OrganizationMaker, Organization: CreditRef{ID: 4}},
		},
		URLs:        []string{" https://example.com/b ", "", "http://example.com/a"},
		ExternalIDs: []ExternalID{{Source: "z", Value: "1"}, {Source: " ", Value: " "}, {Source: "a", Value: "2"}},
	})
	if err != nil || empty {
		t.Fatalf("NormalizeWorkInfo = empty %v, err %v", empty, err)
	}
	want := WorkInfo{
		Code:        "ABC-123",
		Description: "一行目\n二行目",
		Dates: []WorkDate{
			{Kind: DateRelease, Date: "2024"}, {Kind: DateRecorded, Date: "2023-12"}, {Kind: DatePublished, Date: "2024-02-29"},
		},
		People: []WorkPerson{
			{Role: PersonPerformer, Person: CreditRef{ID: 2}}, {Role: PersonPerformer, Person: CreditRef{ID: 1}},
			{Role: PersonDirector, Person: CreditRef{ID: 3}}, {Role: PersonStreamer, Person: CreditRef{ID: 2}},
		},
		Organizations: []WorkOrganization{
			{Role: OrganizationMaker, Organization: CreditRef{ID: 4}}, {Role: OrganizationLabel, Organization: CreditRef{ID: 5}},
		},
		URLs:        []string{"https://example.com/b", "http://example.com/a"},
		ExternalIDs: []ExternalID{{Source: "a", Value: "2"}, {Source: "z", Value: "1"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeWorkInfo =\n%+v\nwant\n%+v", got, want)
	}

	cleared, empty, err := NormalizeWorkInfo(WorkInfo{
		Code: " ", Description: "\n", Dates: []WorkDate{{Kind: DateRelease, Date: ""}}, URLs: []string{" "},
		ExternalIDs: []ExternalID{{}},
	})
	if err != nil || !empty || !cleared.IsEmpty() {
		t.Errorf("空の値 = %+v, empty %v, err %v", cleared, empty, err)
	}
	if _, _, err := NormalizeWorkInfo(WorkInfo{Code: strings.Repeat("A", 100), Description: strings.Repeat("あ", 10_000)}); err != nil {
		t.Errorf("上限ちょうど: %v", err)
	}
}

func TestWorkInfoEqualComparesIDs(t *testing.T) {
	stored := WorkInfo{Code: "A", People: []WorkPerson{{Role: PersonPerformer, Person: CreditRef{ID: 1, Name: "山田"}}}, URLs: []string{}}
	written := WorkInfo{Code: "A", People: []WorkPerson{{Role: PersonPerformer, Person: CreditRef{ID: 1}}}}
	if !stored.Equal(written) {
		t.Error("名前の有無で違う値になった")
	}
	written.People[0].Person.ID = 2
	if stored.Equal(written) {
		t.Error("違う人物が同じ値になった")
	}
}
