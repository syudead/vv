package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// 作品情報の項目の規則（specs/047-video-work-info/data-model.md「Fields」）。

func TestNormalizeWorkInfoTrimsAndClears(t *testing.T) {
	got, empty, err := NormalizeWorkInfo(WorkInfo{
		Code:        "  ABC-123 ",
		ReleaseDate: " 2024-02-29 ",
		Maker:       "　メーカー　",
		Performers:  []string{" 甲 ", "", "  ", "乙", "甲", " 乙 ", "丙"},
		Description: "\n 1 行目\n2 行目 \n",
		SourceURL:   " https://example.com/works/1?x=y ",
	})
	if err != nil || empty {
		t.Fatalf("NormalizeWorkInfo = %v, empty %v", err, empty)
	}
	want := WorkInfo{
		Code:        "ABC-123",
		ReleaseDate: "2024-02-29",
		Maker:       "メーカー",
		Performers:  []string{"甲", "乙", "丙"},
		Description: "1 行目\n2 行目",
		SourceURL:   "https://example.com/works/1?x=y",
	}
	if !got.Equal(want) || !slices.Equal(got.Performers, want.Performers) {
		t.Errorf("NormalizeWorkInfo = %+v, want %+v", got, want)
	}
}

func TestNormalizeWorkInfoEmpty(t *testing.T) {
	for name, input := range map[string]WorkInfo{
		"ゼロ値":     {},
		"空白だけ":    {Code: " ", ReleaseDate: "　", Maker: " ", Description: "\n \n", SourceURL: " "},
		"空の出演者だけ": {Performers: []string{"", "  "}},
	} {
		t.Run(name, func(t *testing.T) {
			got, empty, err := NormalizeWorkInfo(input)
			if err != nil || !empty || !got.IsEmpty() || got.Performers != nil {
				t.Errorf("NormalizeWorkInfo = %+v, empty %v, err %v; want 空", got, empty, err)
			}
		})
	}
}

func TestNormalizeWorkInfoRejects(t *testing.T) {
	limits := WorkInfoLimits
	tooManyPerformers := make([]string, limits.PerformersMaxCount+1)
	for i := range tooManyPerformers {
		tooManyPerformers[i] = strings.Repeat("名", i+1)
	}
	for name, c := range map[string]struct {
		input WorkInfo
		want  InvalidWorkInfoError
	}{
		"品番の制御文字": {WorkInfo{Code: "A\tB"}, InvalidWorkInfoError{Field: WorkInfoFieldCode, Problem: WorkInfoControlCharacters}},
		"品番の改行":   {WorkInfo{Code: "A\nB"}, InvalidWorkInfoError{Field: WorkInfoFieldCode, Problem: WorkInfoControlCharacters}},
		"品番の長さ": {
			WorkInfo{Code: strings.Repeat("あ", limits.CodeMaxLength+1)},
			InvalidWorkInfoError{Field: WorkInfoFieldCode, Problem: WorkInfoTooLong, Limit: 100},
		},
		"発売日の制御文字": {WorkInfo{ReleaseDate: "2024-01-01\x00"}, InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoControlCharacters}},
		"発売日の形":    {WorkInfo{ReleaseDate: "2024/01/01"}, InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoInvalidDate}},
		"発売日の桁":    {WorkInfo{ReleaseDate: "2024-1-01"}, InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoInvalidDate}},
		"発売日の全角数字": {WorkInfo{ReleaseDate: "２０２４-01-01"}, InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoInvalidDate}},
		"暦に無い日":    {WorkInfo{ReleaseDate: "2023-02-29"}, InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoInvalidDate}},
		"13 月":     {WorkInfo{ReleaseDate: "2023-13-01"}, InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoInvalidDate}},
		"年 0":      {WorkInfo{ReleaseDate: "0000-01-01"}, InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoInvalidDate}},
		"メーカーの長さ": {
			WorkInfo{Maker: strings.Repeat("x", limits.MakerMaxLength+1)},
			InvalidWorkInfoError{Field: WorkInfoFieldMaker, Problem: WorkInfoTooLong, Limit: 200},
		},
		"出演者の改行": {WorkInfo{Performers: []string{"甲", "乙\n丙"}}, InvalidWorkInfoError{Field: WorkInfoFieldPerformers, Problem: WorkInfoControlCharacters}},
		"出演者の長さ": {
			WorkInfo{Performers: []string{strings.Repeat("x", limits.PerformerMaxLength+1)}},
			InvalidWorkInfoError{Field: WorkInfoFieldPerformers, Problem: WorkInfoTooLong, Limit: 200},
		},
		"出演者の数": {
			WorkInfo{Performers: tooManyPerformers},
			InvalidWorkInfoError{Field: WorkInfoFieldPerformers, Problem: WorkInfoTooMany, Limit: 100},
		},
		"説明の CR": {WorkInfo{Description: "a\r\nb"}, InvalidWorkInfoError{Field: WorkInfoFieldDescription, Problem: WorkInfoControlCharacters}},
		"説明のタブ":  {WorkInfo{Description: "a\tb"}, InvalidWorkInfoError{Field: WorkInfoFieldDescription, Problem: WorkInfoControlCharacters}},
		"説明の長さ": {
			WorkInfo{Description: strings.Repeat("あ", limits.DescriptionMaxLength+1)},
			InvalidWorkInfoError{Field: WorkInfoFieldDescription, Problem: WorkInfoTooLong, Limit: 10_000},
		},
		"出典の制御文字": {WorkInfo{SourceURL: "https://example.com/\n"}, InvalidWorkInfoError{Field: WorkInfoFieldSourceURL, Problem: WorkInfoControlCharacters}},
		"出典の長さ": {
			WorkInfo{SourceURL: "https://example.com/" + strings.Repeat("a", limits.SourceURLMaxLength)},
			InvalidWorkInfoError{Field: WorkInfoFieldSourceURL, Problem: WorkInfoTooLong, Limit: 2_048},
		},
		"相対 URL":     {WorkInfo{SourceURL: "/works/1"}, InvalidWorkInfoError{Field: WorkInfoFieldSourceURL, Problem: WorkInfoInvalidURL}},
		"http 以外":    {WorkInfo{SourceURL: "ftp://example.com/"}, InvalidWorkInfoError{Field: WorkInfoFieldSourceURL, Problem: WorkInfoInvalidURL}},
		"ホストが無い":     {WorkInfo{SourceURL: "https:///path"}, InvalidWorkInfoError{Field: WorkInfoFieldSourceURL, Problem: WorkInfoInvalidURL}},
		"スキームだけ":     {WorkInfo{SourceURL: "https:example.com"}, InvalidWorkInfoError{Field: WorkInfoFieldSourceURL, Problem: WorkInfoInvalidURL}},
		"URL でない文字列": {WorkInfo{SourceURL: "example"}, InvalidWorkInfoError{Field: WorkInfoFieldSourceURL, Problem: WorkInfoInvalidURL}},
		// 2 つの項目が規則に合わなければ、WorkInfo の項目の順で最初のものを返す。
		"最初の項目": {
			WorkInfo{Code: "a\tb", SourceURL: "nope", ReleaseDate: "x"},
			InvalidWorkInfoError{Field: WorkInfoFieldCode, Problem: WorkInfoControlCharacters},
		},
		"発売日が出典より先": {
			WorkInfo{SourceURL: "nope", ReleaseDate: "x"},
			InvalidWorkInfoError{Field: WorkInfoFieldReleaseDate, Problem: WorkInfoInvalidDate},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := NormalizeWorkInfo(c.input)
			if !errors.Is(err, ErrInvalidWorkInfo) {
				t.Fatalf("err = %v, want ErrInvalidWorkInfo", err)
			}
			var invalid *InvalidWorkInfoError
			if !errors.As(err, &invalid) || *invalid != c.want {
				t.Errorf("err = %+v, want %+v", invalid, c.want)
			}
		})
	}
}

// 上限ちょうどの値と、許す日付・URL は通る。
func TestNormalizeWorkInfoAcceptsLimits(t *testing.T) {
	limits := WorkInfoLimits
	performers := make([]string, limits.PerformersMaxCount)
	for i := range performers {
		performers[i] = strings.Repeat("名", i+1)
	}
	// 100 を超えて渡しても、重複と空を除いて 100 以下なら通る。
	performers = append(performers, performers[0], " ")
	input := WorkInfo{
		Code:        strings.Repeat("あ", limits.CodeMaxLength),
		ReleaseDate: "9999-12-31",
		Maker:       strings.Repeat("x", limits.MakerMaxLength),
		Performers:  performers,
		Description: strings.Repeat("あ", limits.DescriptionMaxLength),
		SourceURL:   "HTTP://Example.com",
	}
	got, _, err := NormalizeWorkInfo(input)
	if err != nil {
		t.Fatalf("上限ちょうどを拒んだ: %v", err)
	}
	if len(got.Performers) != limits.PerformersMaxCount {
		t.Errorf("出演者 = %d 人, want %d", len(got.Performers), limits.PerformersMaxCount)
	}
	for _, date := range []string{"0001-01-01", "2000-02-29"} {
		if _, _, err := NormalizeWorkInfo(WorkInfo{ReleaseDate: date}); err != nil {
			t.Errorf("%s を拒んだ: %v", date, err)
		}
	}
}

func TestWorkInfoEqual(t *testing.T) {
	a := WorkInfo{Code: "A", Performers: []string{"甲", "乙"}}
	if !a.Equal(WorkInfo{Code: "A", Performers: []string{"甲", "乙"}}) {
		t.Error("同じ値が違うとされた")
	}
	if a.Equal(WorkInfo{Code: "A", Performers: []string{"乙", "甲"}}) {
		t.Error("出演者の順番の違いを見落とした")
	}
	if !(WorkInfo{}).Equal(WorkInfo{Performers: []string{}}) {
		t.Error("nil と空の出演者が違うとされた")
	}
}

// 一覧の performer・maker の値は前後の空白を除き、空か 200 符号位置を超えれば誤りにする。
func TestNormalizeWorkInfoFilter(t *testing.T) {
	limit := strings.Repeat("あ", WorkInfoFilterMaxLength)
	for input, want := range map[string]string{"  山田\u3000": "山田", limit: limit, " " + limit + "\t": limit} {
		got, err := NormalizeWorkInfoFilter(input)
		if err != nil || got != want {
			t.Errorf("NormalizeWorkInfoFilter(%q) = %q, %v, want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", " \u3000\t", limit + "い"} {
		if _, err := NormalizeWorkInfoFilter(input); !errors.Is(err, ErrInvalidWorkInfoFilter) {
			t.Errorf("NormalizeWorkInfoFilter(%q) = %v, want ErrInvalidWorkInfoFilter", input, err)
		}
	}
}
