package domain

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func and(terms ...SearchTerm) SearchExpr {
	var expr SearchExpr
	for _, term := range terms {
		expr.Clauses = append(expr.Clauses, SearchClause{Terms: []SearchTerm{term}})
	}
	return expr
}

// has/not build positive/negative terms; clauses combines OR groups with AND.
func has(text string) SearchTerm { return SearchTerm{Text: text} }
func not(text string) SearchTerm { return SearchTerm{Text: text, Negated: true} }

func clauses(groups ...[]SearchTerm) SearchExpr {
	var expr SearchExpr
	for _, group := range groups {
		expr.Clauses = append(expr.Clauses, SearchClause{Terms: group})
	}
	return expr
}

func anyOf(terms ...SearchTerm) []SearchTerm { return terms }

func TestParseSearchQuery(t *testing.T) {
	tests := []struct {
		input string
		want  SearchExpr
	}{
		{`京都 2024`, and(has("京都"), has("2024"))},
		{"😀‍👩 x", and(has("😀‍👩"), has("x"))},
		{`"   "`, SearchExpr{}},
		{`"京都旅行 2024"`, and(has("京都旅行 2024"))},
		{`"旅行 20"`, and(has("旅行 20"))},
		{`京都 -2023`, and(has("京都"), not("2023"))},
		{`-京都`, and(not("京都"))},
		{`京都 OR 奈良`, clauses(anyOf(has("京都"), has("奈良")))},
		{`京都 | 奈良`, clauses(anyOf(has("京都"), has("奈良")))},
		{`2024 京都 OR 奈良`, clauses(anyOf(has("2024")), anyOf(has("京都"), has("奈良")))},
		{`"京都`, and(has(`"京都`))},
		{`OR`, and(has("or"))},
		{`-`, and(has("-"))},
		{`100%`, and(has("100%"))},
		{`a_b`, and(has("a_b"))},
		{`(test)`, and(has("(test)"))},
		{`title:x`, and(has("title:x"))},

		{``, SearchExpr{}},
		{`|`, SearchExpr{}},
		{`| |`, SearchExpr{}},
		{`""`, SearchExpr{}},
		{`-""`, SearchExpr{}},
		{"  \t　  ", SearchExpr{}},

		{"京都　ＯＲ　奈良", clauses(anyOf(has("京都"), has("奈良")))},
		{"京都｜奈良", and(has("京都|奈良"))},
		{"京都 ｜ 奈良", clauses(anyOf(has("京都"), has("奈良")))},
		{"－京都", and(not("京都"))},
		{"＂京都 旅行＂", and(has("京都 旅行"))},

		{`OR 京都`, and(has("or"), has("京都"))},
		{`京都 OR`, and(has("京都"), has("or"))},
		{`| 京都 |`, and(has("京都"))},
		{`a OR OR b`, clauses(anyOf(has("a"), has("or")), anyOf(has("b")))},
		{`a | | b`, clauses(anyOf(has("a"), has("b")))},
		{`a OR | b`, clauses(anyOf(has("a"), has("b")))},
		{`a OR |`, and(has("a"), has("or"))},
		{`a|b`, and(has("a|b"))},
		{`a or b`, and(has("a"), has("or"), has("b"))},
		{`"OR"`, and(has("or"))},
		{`-OR`, and(not("or"))},

		{`a OR b OR c d`, clauses(anyOf(has("a"), has("b"), has("c")), anyOf(has("d")))},
		{`a OR -b`, clauses(anyOf(has("a"), not("b")))},
		{`-"京都 旅行" OR 奈良`, clauses(anyOf(not("京都 旅行"), has("奈良")))},

		{`- a`, and(has("-"), has("a"))},
		{`--a`, and(not("-a"))},
		{`-"京都`, and(not(`"京都`))},
		{`"a b"c`, and(has("a b"), has("c"))},
		{`ab"c d"`, and(has(`ab"c`), has(`d"`))},
		{`a OR "" b`, clauses(anyOf(has("a"), has("b")))},

		{`ＡＢＣ たび ｶﾀｶﾅ`, and(has("abc"), has("タビ"), has("カタカナ"))},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseSearchQuery(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSearchQuery(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
			if got.Empty() != (len(tt.want.Clauses) == 0) {
				t.Errorf("Empty() = %v", got.Empty())
			}
		})
	}
}

func TestParseSearchQueryKeepsFirstSixteenTerms(t *testing.T) {
	words := make([]string, 20)
	var want SearchExpr
	for i := range words {
		words[i] = "w" + strconv.Itoa(i+1)
		if i < MaxSearchTerms {
			want.Clauses = append(want.Clauses, SearchClause{Terms: []SearchTerm{has(words[i])}})
		}
	}
	if got := ParseSearchQuery(strings.Join(words, " ")); !reflect.DeepEqual(got, want) {
		t.Errorf("20 terms = %+v, want the first 16", got)
	}

	withOr := strings.Join(words[:MaxSearchTerms], " ") + " OR w17 OR w18"
	if got := ParseSearchQuery(withOr); !reflect.DeepEqual(got, want) {
		t.Errorf("16 terms + OR = %+v, want the first 16", got)
	}

	orChain := strings.Join(words, " OR ")
	got := ParseSearchQuery(orChain)
	if len(got.Clauses) != 1 || len(got.Clauses[0].Terms) != MaxSearchTerms {
		t.Fatalf("OR chain = %+v, want one clause of 16 terms", got)
	}
	if got.Clauses[0].Terms[MaxSearchTerms-1].Text != "w16" {
		t.Errorf("last term = %q, want w16", got.Clauses[0].Terms[MaxSearchTerms-1].Text)
	}
}

func TestFoldForMatch(t *testing.T) {
	tests := []struct{ a, b string }{
		{"ＡＢＣ１２３", "abc123"},
		{"ｶﾀｶﾅ", "カタカナ"},
		{"たび", "タビ"},
		{norm.NFD.String("が"), norm.NFC.String("が")},
		{"ゝゞ", "ヽヾ"},
		{"ぁゖ", "ァヶ"},
		{"Ｔｉｔｌｅ", "TITLE"},
		{"①", "1"},
	}
	for _, tt := range tests {
		if got, want := FoldForMatch(tt.a), FoldForMatch(tt.b); got != want {
			t.Errorf("FoldForMatch(%q) = %q, want FoldForMatch(%q) = %q", tt.a, got, tt.b, want)
		}
	}
	if got := FoldForMatch("ＡＢＣ１２３"); got != "abc123" {
		t.Errorf("FoldForMatch(ＡＢＣ１２３) = %q", got)
	}
	if got := FoldForMatch("たび"); got != "タビ" {
		t.Errorf("FoldForMatch(たび) = %q", got)
	}
}

func TestNaturalSortKey(t *testing.T) {
	keys := []struct{ title, want string }{
		{"2", "00012"},
		{"10", "000210"},
		{"0", "0000"},
		{"007", "00017"},
		{"第２話", "第00012話"},
	}
	for _, tt := range keys {
		if got := NaturalSortKey(tt.title); got != tt.want {
			t.Errorf("NaturalSortKey(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}

	before := []struct{ a, b string }{
		{"2話", "10話"},
		{"２話", "10話"},
		{"ア", "い"},
	}
	for _, tt := range before {
		if NaturalSortKey(tt.a) >= NaturalSortKey(tt.b) {
			t.Errorf("key(%q) = %q must sort before key(%q) = %q", tt.a, NaturalSortKey(tt.a), tt.b, NaturalSortKey(tt.b))
		}
	}
}

func TestNaturalSortKeyAgreesWithCompareNatural(t *testing.T) {
	names := []string{
		"10", "b", "2", "A", "1", "100% #1 日本語", "a2", "a10", "file01", "file1",
		"ア", "い", "あ", "カ", "か", "が", "ｶﾞ", "ヽ", "ゝ",
		"２話", "10話", "2話", "第１０話", "第9話", "①", "⑫",
		"0", "00", "000a", "a0", "a00b", "a0b", "a01b", "x9", "x09", "x10", "x9y", "x9 ",
		"file", "file ", "file-1", "file.1", "file 1", "Ｆｉｌｅ２", "FILE10",
		"99999999999999999999999", "100000000000000000000000",
		"", " ", "!", "~", "Ω", "ω", "漢字", "漢字2",
	}
	for _, a := range names {
		for _, b := range names {
			want := sign(CompareNatural(FoldForMatch(a), FoldForMatch(b)))
			got := sign(strings.Compare(NaturalSortKey(a), NaturalSortKey(b)))
			if got != want {
				t.Errorf("compare(key(%q), key(%q)) = %d, CompareNatural(fold) = %d", a, b, got, want)
			}
		}
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
