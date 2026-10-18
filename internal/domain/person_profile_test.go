package domain

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

var profileToday = time.Date(2026, time.October, 19, 8, 0, 0, 0, time.FixedZone("JST", 9*60*60))

func TestNormalizePersonProfileRefusesEachRule(t *testing.T) {
	manyLinks := make([]string, PersonLinksMaxCount+1)
	for i := range manyLinks {
		manyLinks[i] = fmt.Sprintf("https://example.com/%d", i)
	}
	cases := []struct {
		name    string
		input   PersonProfile
		field   string
		problem WorkInfoProblem
		limit   int
	}{
		{name: "知らない性別", input: PersonProfile{Gender: "unknown"}, field: "profile.gender", problem: WorkInfoOutOfRange},
		{name: "形式の違う生年月日", input: PersonProfile{BirthDate: "1990/05/17"}, field: "profile.birthDate", problem: WorkInfoInvalidDate},
		{name: "ありえない日付", input: PersonProfile{BirthDate: "1990-02-30"}, field: "profile.birthDate", problem: WorkInfoInvalidDate},
		// Today is 2026-10-18 in UTC although the clock reads 2026-10-19 in JST.
		{name: "UTC の今日より後の生年月日", input: PersonProfile{BirthDate: "2026-10-19"}, field: "profile.birthDate", problem: WorkInfoOutOfRange},
		{name: "今日より後の年だけの没年", input: PersonProfile{DeathDate: "2027"}, field: "profile.deathDate", problem: WorkInfoOutOfRange},
		{name: "生年月日より前の没年月日", input: PersonProfile{BirthDate: "1990-05-17", DeathDate: "1990-05-16"}, field: "profile.deathDate", problem: WorkInfoOutOfRange},
		{name: "精度の混ざった没年月日が前", input: PersonProfile{BirthDate: "1990-05", DeathDate: "1990-04-30"}, field: "profile.deathDate", problem: WorkInfoOutOfRange},
		{name: "年だけの没年が前", input: PersonProfile{BirthDate: "1990-05-17", DeathDate: "1989"}, field: "profile.deathDate", problem: WorkInfoOutOfRange},
		{name: "出身地の制御文字", input: PersonProfile{Birthplace: "東京\n"}, field: "profile.birthplace", problem: WorkInfoControlCharacters},
		{name: "長すぎる出身地", input: PersonProfile{Birthplace: strings.Repeat("あ", PersonBirthplaceMaxLength+1)}, field: "profile.birthplace", problem: WorkInfoTooLong, limit: PersonBirthplaceMaxLength},
		{name: "低すぎる身長", input: PersonProfile{HeightCm: ptr(PersonHeightMinCm - 1)}, field: "profile.heightCm", problem: WorkInfoOutOfRange},
		{name: "高すぎる身長", input: PersonProfile{HeightCm: ptr(PersonHeightMaxCm + 1)}, field: "profile.heightCm", problem: WorkInfoOutOfRange},
		{name: "小さすぎるバスト", input: PersonProfile{BustCm: ptr(PersonMeasurementMinCm - 1)}, field: "profile.bustCm", problem: WorkInfoOutOfRange},
		{name: "大きすぎるウエスト", input: PersonProfile{WaistCm: ptr(PersonMeasurementMaxCm + 1)}, field: "profile.waistCm", problem: WorkInfoOutOfRange},
		{name: "0 のヒップ", input: PersonProfile{HipsCm: ptr(0)}, field: "profile.hipsCm", problem: WorkInfoOutOfRange},
		{name: "英字でないカップ", input: PersonProfile{Cup: "D1"}, field: "profile.cup", problem: WorkInfoOutOfRange},
		{name: "4 文字のカップ", input: PersonProfile{Cup: "ABCD"}, field: "profile.cup", problem: WorkInfoTooLong, limit: PersonCupMaxLength},
		{name: "知らない血液型", input: PersonProfile{BloodType: "C"}, field: "profile.bloodType", problem: WorkInfoOutOfRange},
		{name: "1900 年より前の活動開始", input: PersonProfile{CareerStartYear: ptr(PersonCareerMinYear - 1)}, field: "profile.careerStartYear", problem: WorkInfoOutOfRange},
		{name: "10000 年の活動終了", input: PersonProfile{CareerEndYear: ptr(PersonCareerMaxYear + 1)}, field: "profile.careerEndYear", problem: WorkInfoOutOfRange},
		{name: "開始より前の終了", input: PersonProfile{CareerStartYear: ptr(2010), CareerEndYear: ptr(2009)}, field: "profile.careerEndYear", problem: WorkInfoOutOfRange},
		{name: "長すぎる紹介文", input: PersonProfile{Bio: strings.Repeat("a", PersonBioMaxLength+1)}, field: "profile.bio", problem: WorkInfoTooLong, limit: PersonBioMaxLength},
		{name: "紹介文のタブ", input: PersonProfile{Bio: "a\tb"}, field: "profile.bio", problem: WorkInfoControlCharacters},
		{name: "http でないリンク", input: PersonProfile{Links: []string{"", "ftp://example.com"}}, field: "profile.links[1]", problem: WorkInfoInvalidURL},
		{name: "同じリンク", input: PersonProfile{Links: []string{"https://example.com", " https://example.com"}}, field: "profile.links[1]", problem: WorkInfoDuplicate},
		{name: "リンクが多すぎる", input: PersonProfile{Links: manyLinks}, field: "profile.links", problem: WorkInfoTooMany, limit: PersonLinksMaxCount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizePersonProfile(tc.input, profileToday)
			var invalid *InvalidWorkInfoError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want *InvalidWorkInfoError", err)
			}
			want := InvalidWorkInfoError{Field: tc.field, Problem: tc.problem, Limit: tc.limit}
			if *invalid != want {
				t.Errorf("err = %+v, want %+v", *invalid, want)
			}
		})
	}
}

func TestNormalizePersonProfileTrimsFoldsAndKeepsBoundaries(t *testing.T) {
	input := PersonProfile{
		Gender: " female ", BirthDate: " 1990 ", DeathDate: "1990-01", Birthplace: " 　", HeightCm: ptr(PersonHeightMinCm),
		BustCm: ptr(PersonMeasurementMaxCm), Cup: " e ", BloodType: "ab", CareerStartYear: ptr(2010),
		CareerEndYear: ptr(2010), Bio: " 一行目\n二行目 ", Links: []string{" ", " https://example.com/a "},
	}
	got, err := NormalizePersonProfile(input, profileToday)
	if err != nil {
		t.Fatal(err)
	}
	want := PersonProfile{
		Gender: "female", BirthDate: "1990", DeathDate: "1990-01", HeightCm: ptr(PersonHeightMinCm),
		BustCm: ptr(PersonMeasurementMaxCm), Cup: "E", BloodType: "AB", CareerStartYear: ptr(2010),
		CareerEndYear: ptr(2010), Bio: "一行目\n二行目", Links: []string{"https://example.com/a"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizePersonProfile = %+v, want %+v", got, want)
	}
	if today, err := NormalizePersonProfile(PersonProfile{BirthDate: "2026-10-18"}, profileToday); err != nil || today.BirthDate != "2026-10-18" {
		t.Errorf("UTC の今日の生年月日 = %+v, %v", today, err)
	}
}

func TestPersonProfilePatchReplacesOnlyItsFields(t *testing.T) {
	current := PersonProfile{Gender: "male", HeightCm: ptr(170), Links: []string{"https://example.com"}}
	patch := PersonProfilePatch{Fields: []PersonProfileField{ProfileHeightCm, ProfileLinks}, Values: PersonProfile{Gender: "female"}}
	got := patch.Apply(current)
	if want := (PersonProfile{Gender: "male"}); !reflect.DeepEqual(got, want) {
		t.Errorf("Apply = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(current.Links, []string{"https://example.com"}) {
		t.Errorf("Apply changed the current links: %v", current.Links)
	}
}

func ptr[T any](v T) *T { return &v }
