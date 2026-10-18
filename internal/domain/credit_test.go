package domain

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeCreditRefusesEachRule(t *testing.T) {
	manyAliases := make([]Alias, CreditAliasesMaxCount+1)
	for i := range manyAliases {
		manyAliases[i] = Alias{Name: fmt.Sprintf("別名%d", i)}
	}
	manyIDs := make([]ExternalID, ExternalIDsMaxCount+1)
	for i := range manyIDs {
		manyIDs[i] = ExternalID{Source: "example", Value: fmt.Sprint(i)}
	}
	cases := []struct {
		name    string
		input   CreditInput
		field   string
		problem WorkInfoProblem
		limit   int
	}{
		{name: "名前の制御文字", input: CreditInput{Name: "山田\n"}, field: "name", problem: WorkInfoControlCharacters},
		{name: "空の名前", input: CreditInput{Name: ""}, field: "name", problem: WorkInfoEmpty},
		{name: "空白だけの名前", input: CreditInput{Name: " 　 "}, field: "name", problem: WorkInfoEmpty},
		{name: "長すぎる名前", input: CreditInput{Name: strings.Repeat("あ", CreditNameMaxLength+1)}, field: "name", problem: WorkInfoTooLong, limit: CreditNameMaxLength},
		{name: "別名の制御文字", input: CreditInput{Name: "A", Aliases: []Alias{{Name: "B"}, {Name: "C\t"}}}, field: "aliases[1].name", problem: WorkInfoControlCharacters},
		{name: "長すぎる別名", input: CreditInput{Name: "A", Aliases: []Alias{{Name: strings.Repeat("b", CreditNameMaxLength+1)}}}, field: "aliases[0].name", problem: WorkInfoTooLong, limit: CreditNameMaxLength},
		{name: "名前と同じ別名", input: CreditInput{Name: "山田花子", Aliases: []Alias{{Name: " 山田花子 "}}}, field: "aliases[0].name", problem: WorkInfoDuplicate},
		{name: "別名どうしが同じ", input: CreditInput{Name: "A", Aliases: []Alias{{Name: "B"}, {Name: "C"}, {Name: "B", Language: "en"}}}, field: "aliases[2].name", problem: WorkInfoDuplicate},
		{name: "別名が多すぎる", input: CreditInput{Name: "A", Aliases: manyAliases}, field: "aliases", problem: WorkInfoTooMany, limit: CreditAliasesMaxCount},
		{name: "1 文字の言語", input: CreditInput{Name: "A", Aliases: []Alias{{Name: "B", Language: "x"}}}, field: "aliases[0].language", problem: WorkInfoInvalidLanguage},
		{name: "英字 4 文字で始まる言語", input: CreditInput{Name: "A", Aliases: []Alias{{Name: "B", Language: "engl"}}}, field: "aliases[0].language", problem: WorkInfoInvalidLanguage},
		{name: "数字で始まる言語", input: CreditInput{Name: "A", Aliases: []Alias{{Name: "B", Language: "12-ab"}}}, field: "aliases[0].language", problem: WorkInfoInvalidLanguage},
		{name: "使えない文字の言語", input: CreditInput{Name: "A", Aliases: []Alias{{Name: "B", Language: "ja_JP"}}}, field: "aliases[0].language", problem: WorkInfoInvalidLanguage},
		{name: "長すぎる言語", input: CreditInput{Name: "A", Aliases: []Alias{{Name: "B", Language: "en-" + strings.Repeat("a", CreditLanguageMaxLength)}}}, field: "aliases[0].language", problem: WorkInfoTooLong, limit: CreditLanguageMaxLength},
		{name: "外部 ID の source の制御文字", input: CreditInput{Name: "A", ExternalIDs: []ExternalID{{Source: "ex\u0007", Value: "1"}}}, field: "externalIds[0].source", problem: WorkInfoControlCharacters},
		{name: "長すぎる source", input: CreditInput{Name: "A", ExternalIDs: []ExternalID{{Source: strings.Repeat("s", ExternalIDSourceMaxLength+1), Value: "1"}}}, field: "externalIds[0].source", problem: WorkInfoTooLong, limit: ExternalIDSourceMaxLength},
		{name: "長すぎる value", input: CreditInput{Name: "A", ExternalIDs: []ExternalID{{Source: "s", Value: strings.Repeat("v", ExternalIDValueMaxLength+1)}}}, field: "externalIds[0].value", problem: WorkInfoTooLong, limit: ExternalIDValueMaxLength},
		{name: "source だけ空", input: CreditInput{Name: "A", ExternalIDs: []ExternalID{{Source: " ", Value: "1"}}}, field: "externalIds[0].source", problem: WorkInfoEmpty},
		{name: "value だけ空", input: CreditInput{Name: "A", ExternalIDs: []ExternalID{{Source: "", Value: ""}, {Source: "s", Value: ""}}}, field: "externalIds[1].value", problem: WorkInfoEmpty},
		{name: "同じ外部 ID", input: CreditInput{Name: "A", ExternalIDs: []ExternalID{{Source: "s", Value: "1"}, {Source: "s ", Value: " 1"}}}, field: "externalIds[1]", problem: WorkInfoDuplicate},
		{name: "外部 ID が多すぎる", input: CreditInput{Name: "A", ExternalIDs: manyIDs}, field: "externalIds", problem: WorkInfoTooMany, limit: ExternalIDsMaxCount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeCredit(tc.input)
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

func TestNormalizeCreditTrimsAndDrops(t *testing.T) {
	parent := int64(7)
	got, err := NormalizeCredit(CreditInput{
		Name: "  山田 花子 ",
		Aliases: []Alias{
			{Name: " Hanako ", Language: " en "},
			{Name: "  ", Language: "ja"},
			{Name: "", Language: "x"},
			{Name: " ", Language: strings.Repeat("a", CreditLanguageMaxLength+1)},
			{Name: "", Language: "ja\x00"},
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
		Name:        "山田 花子",
		Aliases:     []Alias{{Name: "Hanako", Language: "en"}, {Name: "hanako"}, {Name: "やまだ", Language: "ja-Hira"}},
		ExternalIDs: []ExternalID{{Source: "example", Value: "42"}},
		ParentID:    &parent,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeCredit = %+v, want %+v", got, want)
	}

	empty, err := NormalizeCredit(CreditInput{Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Aliases == nil || empty.ExternalIDs == nil {
		t.Errorf("空の一覧は空の配列で返す: %+v", empty)
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
