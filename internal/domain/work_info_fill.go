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

// WorkInfoFillQuery is a fill target as the owner reviews it: the video's title and the words its search uses.
type WorkInfoFillQuery struct {
	VideoID int64
	Title   string
	Query   string
}

// WorkInfoFillItem is one fill the owner confirmed: the video and the ID of the work chosen for it.
type WorkInfoFillItem struct {
	VideoID int64
	WorkID  string
}

// MatchWorkInfoCandidate returns the index of the candidate a fill preselects: the first whose title or ID equals
// query ignoring case, width, spaces and punctuation, or the only candidate.
func MatchWorkInfoCandidate(candidates []WorkInfoCandidate, query string) (int, bool) {
	key := candidateMatchKey(query)
	for i, c := range candidates {
		if key != "" && (candidateMatchKey(c.Title) == key || candidateMatchKey(c.ID) == key) {
			return i, true
		}
	}
	if len(candidates) == 1 {
		return 0, true
	}
	return 0, false
}

func candidateMatchKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, FoldForMatch(s))
}
