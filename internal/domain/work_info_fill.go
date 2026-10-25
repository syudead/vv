package domain

import (
	"errors"
	"strings"
	"unicode"
)

// ErrWorkInfoFillQueueFull reports a fill request larger than the room left in the fill queue; nothing is queued.
var ErrWorkInfoFillQueueFull = errors.New("too many work-info fills are queued")

// WorkInfoFillTarget is the video a fill runs for on behalf of its user-data key, with the key's work code, empty
// when it has none.
type WorkInfoFillTarget struct {
	VideoID    int64
	ContentKey string
	Code       string
}

// MatchWorkInfoCandidate picks the first candidate whose title or ID equals query ignoring case, width, spaces and
// punctuation, or the only candidate; a fill applies nothing else, since no owner looks at the choice.
func MatchWorkInfoCandidate(candidates []WorkInfoCandidate, query string) (WorkInfoCandidate, bool) {
	key := candidateMatchKey(query)
	for _, c := range candidates {
		if key != "" && (candidateMatchKey(c.Title) == key || candidateMatchKey(c.ID) == key) {
			return c, true
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return WorkInfoCandidate{}, false
}

func candidateMatchKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, FoldForMatch(s))
}
