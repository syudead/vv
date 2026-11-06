package domain

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeCreditRefusesEachRule(t *testing.T) {
	manyNames := make([]CreditName, CreditNamesMaxCount+1)
	for i := range manyNames {
		manyNames[i] = CreditName{Name: fmt.Sprintf("名前%d", i)}
	}
	manyIDs := make([]ExternalID, ExternalIDsMaxCount+1)
	for i := range manyIDs {
		manyIDs[i] = ExternalID{Source: "example", Value: fmt.Sprint(i)}
	}
	one := func(name, language string) []CreditName { return []CreditName{{Name: name, Language: language}} }
	cases := []struct {
		name    string
		kind    CreditKind
		input   CreditInput
		field   string
		problem WorkInfoProblem
		limit   int
	}{
		{name: "代表名の制御文字", input: CreditInput{Names: one("山田\n", "")}, field: "names[0].name", problem: WorkInfoControlCharacters},
		{name: "空白だけの代表名", input: CreditInput{Names: one(" 　 ", "")}, field: "names[0].name", problem: WorkInfoEmpty},
		{name: "空の後の名前", input: CreditInput{Names: []CreditName{{Name: "A"}, {Name: ""}}}, field: "names[1].name", problem: WorkInfoEmpty},
		{name: "長すぎる名前", input: CreditInput{Names: []CreditName{{Name: "A"}, {Name: strings.Repeat("あ", CreditNameMaxLength+1)}}}, field: "names[1].name", problem: WorkInfoTooLong, limit: CreditNameMaxLength},
		{name: "前の名前と同じ綴り", input: CreditInput{Names: []CreditName{{Name: "山田花子"}, {Name: "B"}, {Name: " 山田花子 ", Language: "ja"}}}, field: "names[2].name", problem: WorkInfoDuplicate},
		{name: "名前が多すぎる", input: CreditInput{Names: manyNames}, field: "names", problem: WorkInfoTooMany, limit: CreditNamesMaxCount},
		{name: "空の一覧の組織", kind: CreditOrganization, input: CreditInput{Names: []CreditName{}}, field: "names", problem: WorkInfoEmpty},
		{name: "代表名の不正な言語", input: CreditInput{Names: one("A", "x")}, field: "names[0].language", problem: WorkInfoInvalidLanguage},
		{name: "英字 4 文字で始まる言語", input: CreditInput{Names: one("A", "engl")}, field: "names[0].language", problem: WorkInfoInvalidLanguage},
		{name: "数字で始まる言語", input: CreditInput{Names: one("A", "12-ab")}, field: "names[0].language", problem: WorkInfoInvalidLanguage},
		{name: "使えない文字の言語", input: CreditInput{Names: one("A", "ja_JP")}, field: "names[0].language", problem: WorkInfoInvalidLanguage},
		{name: "長すぎる言語", input: CreditInput{Names: one("A", "en-"+strings.Repeat("a", CreditLanguageMaxLength))}, field: "names[0].language", problem: WorkInfoTooLong, limit: CreditLanguageMaxLength},
		{name: "外部 ID の source の制御文字", input: CreditInput{Names: one("A", ""), ExternalIDs: []ExternalID{{Source: "ex\u0007", Value: "1"}}}, field: "externalIds[0].source", problem: WorkInfoControlCharacters},
		{name: "長すぎる source", input: CreditInput{Names: one("A", ""), ExternalIDs: []ExternalID{{Source: strings.Repeat("s", ExternalIDSourceMaxLength+1), Value: "1"}}}, field: "externalIds[0].source", problem: WorkInfoTooLong, limit: ExternalIDSourceMaxLength},
		{name: "長すぎる value", input: CreditInput{Names: one("A", ""), ExternalIDs: []ExternalID{{Source: "s", Value: strings.Repeat("v", ExternalIDValueMaxLength+1)}}}, field: "externalIds[0].value", problem: WorkInfoTooLong, limit: ExternalIDValueMaxLength},
		{name: "source だけ空", input: CreditInput{Names: one("A", ""), ExternalIDs: []ExternalID{{Source: " ", Value: "1"}}}, field: "externalIds[0].source", problem: WorkInfoEmpty},
		{name: "value だけ空", input: CreditInput{Names: one("A", ""), ExternalIDs: []ExternalID{{Source: "", Value: ""}, {Source: "s", Value: ""}}}, field: "externalIds[1].value", problem: WorkInfoEmpty},
		{name: "同じ外部 ID", input: CreditInput{Names: one("A", ""), ExternalIDs: []ExternalID{{Source: "s", Value: "1"}, {Source: "s ", Value: " 1"}}}, field: "externalIds[1]", problem: WorkInfoDuplicate},
		{name: "外部 ID が多すぎる", input: CreditInput{Names: one("A", ""), ExternalIDs: manyIDs}, field: "externalIds", problem: WorkInfoTooMany, limit: ExternalIDsMaxCount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind := tc.kind
			if kind == "" {
				kind = CreditPerson
			}
			_, err := NormalizeCredit(kind, tc.input)
			assertInvalidWorkInfo(t, err, InvalidWorkInfoError{Field: tc.field, Problem: tc.problem, Limit: tc.limit})
		})
	}
}

func assertInvalidWorkInfo(t *testing.T, err error, want InvalidWorkInfoError) {
	t.Helper()
	var invalid *InvalidWorkInfoError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want *InvalidWorkInfoError", err)
	}
	if *invalid != want {
		t.Errorf("err = %+v, want %+v", *invalid, want)
	}
	if !errors.Is(err, ErrInvalidWorkInfo) {
		t.Error("errors.Is(err, ErrInvalidWorkInfo) = false")
	}
}

func TestNormalizeCreditKeepsTheOrderAndTrims(t *testing.T) {
	parent := int64(7)
	got, err := NormalizeCredit(CreditOrganization, CreditInput{
		Names: []CreditName{
			{Name: " Hanako ", Language: " en "},
			{Name: "  山田 花子 "},
			{Name: "hanako"},
			{Name: "やまだ", Language: "ja-Hira"},
		},
		ExternalIDs: []ExternalID{{Source: " ", Value: ""}, {Source: " example ", Value: " 42 "}},
		ParentID:    &parent,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := CreditInput{
		Names: []CreditName{
			{Name: "Hanako", Language: "en"}, {Name: "山田 花子"}, {Name: "hanako"}, {Name: "やまだ", Language: "ja-Hira"},
		},
		ExternalIDs: []ExternalID{{Source: "example", Value: "42"}},
		ParentID:    &parent,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeCredit = %+v, want %+v", got, want)
	}
}

// A person with no names is provisional, and its lists come back as empty slices.
func TestNormalizeCreditAcceptsAnEmptyPerson(t *testing.T) {
	got, err := NormalizeCredit(CreditPerson, CreditInput{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Names == nil || len(got.Names) != 0 || got.ExternalIDs == nil {
		t.Errorf("NormalizeCredit = %+v, want empty lists", got)
	}
}

func TestPatchCredit(t *testing.T) {
	stored := []CreditName{{Name: "A", Language: "ja"}, {Name: "B"}, {Name: "C"}}
	manyStored := make([]CreditName, CreditNamesMaxCount+1)
	for i := range manyStored {
		manyStored[i] = CreditName{Name: fmt.Sprintf("名前%d", i)}
	}
	named := Credit{Kind: CreditPerson, Name: "A", Names: stored}
	ptr := func(s string) *string { return &s }
	list := func(names ...CreditName) *[]CreditName {
		if names == nil {
			names = []CreditName{}
		}
		return &names
	}
	cases := []struct {
		name    string
		current Credit
		patch   CreditPatch
		want    []CreditName
		err     *InvalidWorkInfoError
	}{
		{name: "2 つの名前の入れ替えは重複にならない", current: named,
			patch: CreditPatch{Names: list(CreditName{Name: "B"}, CreditName{Name: "A", Language: "ja"}, CreditName{Name: "C"})},
			want:  []CreditName{{Name: "B"}, {Name: "A", Language: "ja"}, {Name: "C"}}},
		{name: "name は代表名の綴りだけを変え言語を保つ", current: named, patch: CreditPatch{Name: ptr(" Z ")},
			want: []CreditName{{Name: "Z", Language: "ja"}, {Name: "B"}, {Name: "C"}}},
		{name: "仮の人物への name は 1 件の一覧", current: Credit{Kind: CreditPerson, Provisional: true, Names: []CreditName{}},
			patch: CreditPatch{Name: ptr("新")}, want: []CreditName{{Name: "新"}}},
		{name: "一覧の別の行と同じ name", current: named, patch: CreditPatch{Name: ptr("C")},
			err: &InvalidWorkInfoError{Field: "name", Problem: WorkInfoDuplicate}},
		{name: "空の name", current: named, patch: CreditPatch{Name: ptr(" ")},
			err: &InvalidWorkInfoError{Field: "name", Problem: WorkInfoEmpty}},
		{name: "51 件の人物へのプロフィールだけの更新", current: Credit{Kind: CreditPerson, Names: manyStored},
			patch: CreditPatch{Profile: &PersonProfilePatch{}}, want: manyStored},
		{name: "51 件の人物への name の更新", current: Credit{Kind: CreditPerson, Names: manyStored},
			patch: CreditPatch{Name: ptr("名前0")}, want: manyStored},
		{name: "51 件の names", current: named, patch: CreditPatch{Names: &manyStored},
			err: &InvalidWorkInfoError{Field: "names", Problem: WorkInfoTooMany, Limit: CreditNamesMaxCount}},
		{name: "人物の一覧を空にすると仮の人物", current: named, patch: CreditPatch{Names: list()}, want: []CreditName{}},
		{name: "組織の一覧を空にする", current: Credit{Kind: CreditOrganization, Names: stored}, patch: CreditPatch{Names: list()},
			err: &InvalidWorkInfoError{Field: "names", Problem: WorkInfoEmpty}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PatchCredit(tc.current, tc.patch)
			if tc.err != nil {
				assertInvalidWorkInfo(t, err, *tc.err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Names, tc.want) {
				t.Errorf("Names = %+v, want %+v", got.Names, tc.want)
			}
		})
	}
}

func TestLanguageTagSyntax(t *testing.T) {
	for _, ok := range []string{"ja", "en", "fil", "ja-Hira", "zh-Hant-TW", "en-US-x-private", "EN"} {
		if !isLanguageTagSyntax(ok) {
			t.Errorf("isLanguageTagSyntax(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "j", "jpan", "j1", "1a", "-en", "en_US", "日本", "en US"} {
		if isLanguageTagSyntax(bad) {
			t.Errorf("isLanguageTagSyntax(%q) = true", bad)
		}
	}
}

func TestExternalIDIndexFindsTheSentPosition(t *testing.T) {
	raw := []ExternalID{{Source: "", Value: ""}, {Source: "a", Value: "1"}, {Source: " b ", Value: "2"}}
	if got := ExternalIDIndex(raw, ExternalID{Source: "b", Value: "2"}); got != 2 {
		t.Errorf("ExternalIDIndex = %d, want 2", got)
	}
	if got := ExternalIDIndex(raw, ExternalID{Source: "c", Value: "3"}); got != -1 {
		t.Errorf("ExternalIDIndex = %d, want -1", got)
	}
}

func TestNormalizeExternalIDForLookup(t *testing.T) {
	if got, ok := NormalizeExternalID(ExternalID{Source: " example ", Value: "　 42 "}); !ok || got != (ExternalID{Source: "example", Value: "42"}) {
		t.Errorf("NormalizeExternalID = %+v, %v", got, ok)
	}
	for _, id := range []ExternalID{{Source: "example", Value: " "}, {Source: "", Value: ""}, {Source: "ex\u0007", Value: "1"}} {
		if _, ok := NormalizeExternalID(id); ok {
			t.Errorf("NormalizeExternalID(%+v) が true", id)
		}
	}
}
