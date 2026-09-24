package domain

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

// and は語1つずつの項を AND で並べた式を作る。
func and(terms ...SearchTerm) SearchExpr {
	var expr SearchExpr
	for _, term := range terms {
		expr.Clauses = append(expr.Clauses, SearchClause{Terms: []SearchTerm{term}})
	}
	return expr
}

// has は含む語、not は除外語、clauses は OR の組を AND で並べた式、anyOf は OR の組を表す。
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
		// 親 Issue #195 の受け入れ条件 1〜5 の入力。
		{`京都 2024`, and(has("京都"), has("2024"))},
		// 絵文字や ZWJ 連結を含む語でも失敗しない（親 Issue の Edge Case）。
		{"😀‍👩 x", and(has("😀‍👩"), has("x"))},
		// 空白だけのフレーズは空のフレーズと同じく捨てる。
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

		// 絞り込まない入力。
		{``, SearchExpr{}},
		{`|`, SearchExpr{}},
		{`| |`, SearchExpr{}},
		{`""`, SearchExpr{}},
		{`-""`, SearchExpr{}},
		{"  \t　  ", SearchExpr{}},

		// 全角の演算子・空白・引用符は NFKC で半角になる。
		{"京都　ＯＲ　奈良", clauses(anyOf(has("京都"), has("奈良")))},
		{"京都｜奈良", and(has("京都|奈良"))},
		{"京都 ｜ 奈良", clauses(anyOf(has("京都"), has("奈良")))},
		{"－京都", and(not("京都"))},
		{"＂京都 旅行＂", and(has("京都 旅行"))},

		// 演算子になれない OR と |。
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

		// OR は AND より強く結び付き、除外語も OR の項になる。
		{`a OR b OR c d`, clauses(anyOf(has("a"), has("b"), has("c")), anyOf(has("d")))},
		{`a OR -b`, clauses(anyOf(has("a"), not("b")))},
		{`-"京都 旅行" OR 奈良`, clauses(anyOf(not("京都 旅行"), has("奈良")))},

		// 除外、フレーズ、引用符の細部。
		{`- a`, and(has("-"), has("a"))},
		{`--a`, and(not("-a"))},
		{`-"京都`, and(not(`"京都`))},
		{`"a b"c`, and(has("a b"), has("c"))},
		{`ab"c d"`, and(has(`ab"c`), has(`d"`))},
		{`a OR "" b`, clauses(anyOf(has("a"), has("b")))},

		// 各語に照合形への変換を掛ける。
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

	// 17 個目の語に掛かる演算子も無視し、16 個目までの項は変えない。
	withOr := strings.Join(words[:MaxSearchTerms], " ") + " OR w17 OR w18"
	if got := ParseSearchQuery(withOr); !reflect.DeepEqual(got, want) {
		t.Errorf("16 terms + OR = %+v, want the first 16", got)
	}

	// 演算子は語として数えない。
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

// TestNaturalSortKeyAgreesWithCompareNatural は、鍵のバイト順が
// CompareNatural(fold(a), fold(b)) と同じ向きになることを、組をすべて比べて確かめる。
func TestNaturalSortKeyAgreesWithCompareNatural(t *testing.T) {
	names := []string{
		// TestCompareNatural の名前。
		"10", "b", "2", "A", "1", "100% #1 日本語", "a2", "a10", "file01", "file1",
		// かな・全角数字・先頭の 0・数字とほかの文字の境目。
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
