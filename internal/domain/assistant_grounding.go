package domain

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// assistantAnswerNumbers finds numbers in an answer, with thousands separators, so "1,234" is checked as 1234.
var assistantAnswerNumbers = regexp.MustCompile(`\d[\d,]*`)

// AssistantNameMinRunes is the shortest library name the grounding check looks for in an answer.
const AssistantNameMinRunes = 3

// AssistantUngrounded lists what an answer states without support: each number of two or more digits that is not
// a number of the evidence, and each folded library name that the answer mentions as a token and the evidence does
// not contain. names are the caller's candidates, found as substrings of the folded answer. The evidence is the
// turn's tool results, the request, the screen and today's date.
func AssistantUngrounded(answer, evidence string, names []string) []string {
	folded := FoldForMatch(evidence)
	known := map[string]bool{}
	for _, number := range assistantAnswerNumbers.FindAllString(folded, -1) {
		known[plainNumber(number)] = true
	}
	var out []string
	for _, number := range assistantAnswerNumbers.FindAllString(FoldForMatch(answer), -1) {
		plain := plainNumber(number)
		if len(plain) >= 2 && !known[plain] && !slices.Contains(out, plain) {
			out = append(out, plain)
		}
	}
	text := newScriptText(answer)
	for _, name := range names {
		if len([]rune(name)) >= AssistantNameMinRunes && text.mentions(name) && !strings.Contains(folded, name) &&
			!slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// plainNumber drops thousands separators and a trailing comma that ends a clause.
func plainNumber(number string) string {
	return strings.ReplaceAll(number, ",", "")
}

// script is the run a character belongs to for the token rule; scriptOther never joins a run.
type script int

const (
	scriptOther script = iota
	scriptWord
	scriptHiragana
	scriptKatakana
	scriptHan
)

// scriptText is a text folded like FoldForMatch, rune for rune, with each rune's script taken before kana folding.
type scriptText struct {
	folded  []rune
	scripts []script
}

func newScriptText(text string) scriptText {
	runes := []rune(norm.NFKC.String(text))
	t := scriptText{folded: make([]rune, len(runes)), scripts: make([]script, len(runes))}
	for i, r := range runes {
		t.folded[i], t.scripts[i] = foldRune(r), scriptOf(r)
	}
	return t
}

// mentions reports whether name occurs with no character of its first rune's script just before it and none of its
// last rune's script just after it, so 「ライブ」 is not found in 「ライブラリ」 nor "live" in "lively".
func (t scriptText) mentions(name string) bool {
	target := []rune(name)
	for i := 0; len(target) > 0 && i+len(target) <= len(t.folded); i++ {
		end := i + len(target)
		if slices.Equal(t.folded[i:end], target) &&
			(i == 0 || !t.sameScript(i-1, i)) && (end == len(t.folded) || !t.sameScript(end, end-1)) {
			return true
		}
	}
	return false
}

func (t scriptText) sameScript(a, b int) bool {
	return t.scripts[a] != scriptOther && t.scripts[a] == t.scripts[b]
}

// scriptOf treats the long-vowel mark as katakana and Latin and other letters and digits as one word script.
func scriptOf(r rune) script {
	switch {
	case r == 'ー' || unicode.Is(unicode.Katakana, r):
		return scriptKatakana
	case unicode.Is(unicode.Hiragana, r):
		return scriptHiragana
	case unicode.Is(unicode.Han, r) || r == '〆':
		return scriptHan
	case unicode.IsLetter(r) || unicode.IsDigit(r):
		return scriptWord
	}
	return scriptOther
}
