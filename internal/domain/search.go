package domain

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// SearchKeyVersion は FoldForMatch と NaturalSortKey の規則の版である。
// 規則を変えるときは版を上げ、保存済みの鍵を作り直させる
// （specs/013-library-search/data-model.md §5）。
const SearchKeyVersion = 1

// MaxSearchTerms は1つの検索式で使う語の上限である。17 個目以降の語と、
// それに掛かる演算子は無視する。
const MaxSearchTerms = 16

// SearchTerm は検索語1つである。Text は FoldForMatch を掛けた照合形で、
// 空にはならない。
type SearchTerm struct {
	Text string
	// Negated は「Text を含まない」を満たす除外語であることを表す。
	Negated bool
}

// SearchClause は OR で結ばれた語の連なりで、どれか1つを満たせば満たす。
type SearchClause struct {
	Terms []SearchTerm
}

// SearchExpr は解釈済みの検索式である。Clauses をすべて満たせば当たる（AND）。
// Clauses が空なら絞り込まない。
type SearchExpr struct {
	Clauses []SearchClause
}

// Empty は式が絞り込まない（語が1つも無い）ことを返す。
func (e SearchExpr) Empty() bool { return len(e.Clauses) == 0 }

// searchToken は区切った直後の字句である。
type searchToken struct {
	kind    searchTokenKind
	text    string
	negated bool
}

type searchTokenKind int

const (
	tokenTerm searchTokenKind = iota
	// tokenOR は単独の `OR`。演算子になれなければ字面の語になる。
	tokenOR
	// tokenPipe は単独の `|`。演算子になれなければ捨てる。
	tokenPipe
)

// ParseSearchQuery は検索欄の入力を検索式に解釈する。規則は
// specs/013-library-search/contracts/list-api.md §1 のとおりで、どの入力でも
// 誤りを返さない。語が残らなければ空の式（絞り込まない）を返す。
//
// 契約に書かれていない細部は次のように決めている。
//   - フレーズを開く `"` は語の先頭（除外の `-` の直後を含む）にあるものだけで、
//     語の途中の `"` は字面の文字である（`ab"c` は1語）。
//   - 閉じた `"` の直後に空白を挟まず続く文字は、次の語として扱う。
//   - 空のフレーズと空白だけのフレーズは、演算子の判定より前に捨てる（`a OR "" b` は `a OR b`）。
func ParseSearchQuery(input string) SearchExpr {
	tokens := tokenizeSearchQuery(norm.NFKC.String(input))

	var expr SearchExpr
	count := 0
	joinNext := false
	prevIsTerm := false
	for i, token := range tokens {
		switch token.kind {
		case tokenPipe, tokenOR:
			if prevIsTerm && hasFollowingTerm(tokens[i+1:]) {
				joinNext = true
				prevIsTerm = false
				continue
			}
			if token.kind == tokenPipe {
				// 演算子になれない `|` は捨てる。直前の演算子もそのまま残す。
				continue
			}
			// 演算子になれない `OR` は字面の語である。
			token = searchToken{kind: tokenTerm, text: token.text}
		}
		if count == MaxSearchTerms {
			// 17 個目以降の語と、それに掛かる演算子は無視する。
			break
		}
		count++
		term := SearchTerm{Text: FoldForMatch(token.text), Negated: token.negated}
		if joinNext && len(expr.Clauses) > 0 {
			last := &expr.Clauses[len(expr.Clauses)-1]
			last.Terms = append(last.Terms, term)
		} else {
			expr.Clauses = append(expr.Clauses, SearchClause{Terms: []SearchTerm{term}})
		}
		joinNext = false
		prevIsTerm = true
	}
	return expr
}

// hasFollowingTerm は、演算子の候補の後ろに語が続くかどうかを返す。演算子の
// 直後の `|` は捨てられるので読み飛ばし、`OR` は演算子の直後では字面の語に
// なるので語として数える。
func hasFollowingTerm(rest []searchToken) bool {
	for _, token := range rest {
		if token.kind != tokenPipe {
			return true
		}
	}
	return false
}

// tokenizeSearchQuery は NFKC 済みの入力を空白とフレーズで区切る。空のフレーズは
// ここで捨てる。
func tokenizeSearchQuery(s string) []searchToken {
	var tokens []searchToken
	for {
		s = strings.TrimLeftFunc(s, isSearchSpace)
		if s == "" {
			return tokens
		}
		negated := false
		body := s
		if body[0] == '-' && len(body) > 1 {
			if r, _ := utf8.DecodeRuneInString(body[1:]); !isSearchSpace(r) {
				negated = true
				body = body[1:]
			}
		}
		if body[0] == '"' {
			if end := strings.IndexByte(body[1:], '"'); end >= 0 {
				phrase := body[1 : 1+end]
				s = body[end+2:]
				// 空のフレーズと空白だけのフレーズは捨てる。
				if strings.TrimFunc(phrase, isSearchSpace) != "" {
					tokens = append(tokens, searchToken{kind: tokenTerm, text: phrase, negated: negated})
				}
				continue
			}
			// 閉じていない `"` は字面の文字として語に含める。
		}
		end := strings.IndexFunc(body, isSearchSpace)
		if end < 0 {
			end = len(body)
		}
		word := body[:end]
		s = body[end:]
		switch {
		case negated:
			tokens = append(tokens, searchToken{kind: tokenTerm, text: word, negated: true})
		case word == "OR":
			tokens = append(tokens, searchToken{kind: tokenOR, text: word})
		case word == "|":
			tokens = append(tokens, searchToken{kind: tokenPipe, text: word})
		default:
			// `-` だけの語もここに来て、字面の `-` を探す語になる。
			tokens = append(tokens, searchToken{kind: tokenTerm, text: word})
		}
	}
}

func isSearchSpace(r rune) bool { return unicode.Is(unicode.White_Space, r) }

// FoldForMatch は文字列を照合形にする。NFKC 正規化、1文字ずつの
// unicode.ToLower（CompareNatural と同じ小文字化）、ひらがなからカタカナへの
// 置き換えの順に掛ける。全角半角・大文字小文字・かなの違いと、NFC・NFD の
// 違いが同じ照合形に畳まれる。検索語の各語と、所在の search_key の両方に
// 掛ける（specs/013-library-search/data-model.md §3）。
func FoldForMatch(s string) string {
	return strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		switch {
		case r >= 'ぁ' && r <= 'ゖ', r == 'ゝ', r == 'ゞ':
			// U+3041–U+3096 と ゝゞ（U+309D・U+309E）は、0x60 足すと対応する
			// カタカナ（ァ–ヶ、ヽヾ）になる。
			return r + 0x60
		}
		return r
	}, norm.NFKC.String(s))
}

// NaturalSortKey は題名から、バイト順に比べると自然順になる鍵を作る
// （specs/013-library-search/data-model.md §4）。FoldForMatch を掛けたうえで、
// ASCII の数字の連続を「先頭の 0 を除いた桁数を10進4桁で表した接頭辞 + 先頭の
// 0 を除いた数字」に置き換える（`2` → `00012`、`10` → `000210`、`0` や `00` は
// 数値 0 として `0000`）。
//
// 異なる2つの題名 a・b について、鍵のバイト順は CompareNatural(FoldForMatch(a),
// FoldForMatch(b)) と同じ向きを返す。CompareNatural が同順位とする組（`01` と
// `1` のように先頭の 0 だけが違う組や、照合形が同じ組）は同じ鍵になり、呼び出し
// 側が id で決着させる。接頭辞は `0` から始まるので、数字の連続とほかの文字との
// 前後は元の文字の前後と変わらない。
//
// 先頭の 0 を除いて 9999 桁を超える数字の連続は4桁の接頭辞に収まらず、順序を
// 保証しない。題名はファイル名から作るので、その長さには届かない。
func NaturalSortKey(title string) string {
	folded := FoldForMatch(title)
	var b strings.Builder
	b.Grow(len(folded) + 8)
	for folded != "" {
		if !isDigit(rune(folded[0])) {
			_, size := utf8.DecodeRuneInString(folded)
			b.WriteString(folded[:size])
			folded = folded[size:]
			continue
		}
		digits, rest := splitDigits(folded)
		digits = strings.TrimLeft(digits, "0")
		length := strconv.Itoa(len(digits))
		b.WriteString(strings.Repeat("0", max(0, 4-len(length))))
		b.WriteString(length)
		b.WriteString(digits)
		folded = rest
	}
	return b.String()
}
