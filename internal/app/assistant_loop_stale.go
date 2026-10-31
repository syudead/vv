package app

import (
	"crypto/sha256"

	"github.com/syudead/vv/internal/domain"
)

// staleRoundLimit is how many rounds in a row may learn nothing new before the turn stops as step_limit. One is
// allowed because a model may rightly call again for a result fitContext dropped.
const staleRoundLimit = 2

// staleTracker tells rounds that learned nothing: every call repeats an earlier call of the turn, or answers what an
// earlier call already answered. A model that cycles through such calls, such as listVideos and listTags in turn,
// otherwise never repeats the previous reply exactly and runs until the owner stops it.
type staleTracker struct {
	calls   map[string]bool
	results map[[sha256.Size]byte]bool
	// round is whether every call of the running round was stale so far; streak counts the stale rounds in a row.
	round  bool
	streak int
}

func (s *staleTracker) startRound() {
	if s.calls == nil {
		s.calls, s.results = map[string]bool{}, map[[sha256.Size]byte]bool{}
	}
	s.round = true
}

// add records one call and its result; byResult says whether an equal result marks the call stale, which does not
// hold for image and write tools, whose results do not depend on what they read.
func (s *staleTracker) add(call domain.ToolCall, content string, byResult bool) {
	key := callsKey([]domain.ToolCall{call})
	sum := sha256.Sum256([]byte(content))
	stale := s.calls[key] || (byResult && s.results[sum])
	s.calls[key] = true
	if byResult {
		s.results[sum] = true
	}
	s.round = s.round && stale
}

// endRound reports whether the stale rounds in a row reached staleRoundLimit.
func (s *staleTracker) endRound() bool {
	if !s.round {
		s.streak = 0
		return false
	}
	s.streak++
	return s.streak >= staleRoundLimit
}

// comparesResult says whether an equal result of the call means the call learned nothing new.
func (l *turnLoop) comparesResult(call domain.ToolCall) bool {
	name := domain.AssistantToolNameFromModel(call.Name)
	if l.isImageTool(name) {
		return false
	}
	tool, known := l.tools.byName[name]
	return !known || tool.Kind != domain.AssistantToolWrite
}
