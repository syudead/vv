package domain

import (
	"regexp"
	"slices"
	"strings"
)

// assistantAnswerNumbers finds numbers in an answer, with thousands separators, so "1,234" is checked as 1234.
var assistantAnswerNumbers = regexp.MustCompile(`\d[\d,]*`)

// AssistantNameMinRunes is the shortest library name the grounding check looks for in an answer.
const AssistantNameMinRunes = 3

// AssistantUngrounded lists what an answer states without support: each number of two or more digits that is not
// a number of the evidence, and each folded library name (names, found in the answer by the caller) that the
// evidence does not contain. The evidence is the turn's tool results, the request, the screen and today's date.
func AssistantUngrounded(answer, evidence string, names []string) []string {
	known := map[string]bool{}
	for _, number := range assistantAnswerNumbers.FindAllString(evidence, -1) {
		known[plainNumber(number)] = true
	}
	var out []string
	for _, number := range assistantAnswerNumbers.FindAllString(answer, -1) {
		plain := plainNumber(number)
		if len(plain) >= 2 && !known[plain] && !slices.Contains(out, plain) {
			out = append(out, plain)
		}
	}
	folded := FoldForMatch(evidence)
	for _, name := range names {
		if len([]rune(name)) >= AssistantNameMinRunes && !strings.Contains(folded, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// plainNumber drops thousands separators and a trailing comma that ends a clause.
func plainNumber(number string) string {
	return strings.ReplaceAll(number, ",", "")
}
