package domain

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Bump SearchKeyVersion when folding or sort-key rules change to rebuild persisted keys.
const SearchKeyVersion = 1

// Terms after the sixteenth and their operators are ignored.
const MaxSearchTerms = 16

// Text is folded and nonempty.
type SearchTerm struct {
	Text    string
	Negated bool
}

// SearchClause joins terms with OR.
type SearchClause struct {
	Terms []SearchTerm
}

// SearchExpr joins clauses with AND; empty clauses mean no filter.
type SearchExpr struct {
	Clauses []SearchClause
}

func (e SearchExpr) Empty() bool { return len(e.Clauses) == 0 }

type searchToken struct {
	kind    searchTokenKind
	text    string
	negated bool
}

type searchTokenKind int

const (
	tokenTerm searchTokenKind = iota
	// An OR token that cannot act as an operator becomes literal text.
	tokenOR
	// A pipe that cannot act as an operator is discarded.
	tokenPipe
)

// ParseSearchQuery never rejects input; quotes open only at term boundaries, text after a closing quote starts another term, and empty phrases disappear before operator parsing (docs/design-docs/catalog-organization.md).
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
				continue
			}
			token = searchToken{kind: tokenTerm, text: token.text}
		}
		if count == MaxSearchTerms {
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

// Skip pipes after an operator; OR in that position counts as a literal term.
func hasFollowingTerm(rest []searchToken) bool {
	for _, token := range rest {
		if token.kind != tokenPipe {
			return true
		}
	}
	return false
}

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
				if strings.TrimFunc(phrase, isSearchSpace) != "" {
					tokens = append(tokens, searchToken{kind: tokenTerm, text: phrase, negated: negated})
				}
				continue
			}
			// An unmatched quote is literal text.
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
			// A standalone minus is a literal search term.
			tokens = append(tokens, searchToken{kind: tokenTerm, text: word})
		}
	}
}

func isSearchSpace(r rune) bool { return unicode.Is(unicode.White_Space, r) }

// FoldForMatch applies NFKC, per-code-point lowercase, then hiragana-to-katakana conversion to both query terms and persisted search keys.
func FoldForMatch(s string) string {
	return strings.Map(func(r rune) rune {
		r = unicode.ToLower(r)
		switch {
		case r >= 'ぁ' && r <= 'ゖ', r == 'ゝ', r == 'ゞ':
			// Adding 0x60 maps U+3041–U+3096 and U+309D–U+309E to their katakana equivalents.
			return r + 0x60
		}
		return r
	}, norm.NFKC.String(s))
}

// NaturalSortKey encodes folded digit runs as a four-digit significant-length prefix plus digits, making byte order match natural order; callers break equal keys by ID, and runs beyond 9,999 significant digits are unsupported.
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
