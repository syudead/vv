package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Automatic version judgements write the same bundles/different-video records as owner actions.

// Waiting candidates have a null judgement.
type VersionJudgement string

const (
	// Only one candidate may be running at a time.
	VersionJudgementRunning VersionJudgement = "running"
	// Undecided candidates remain visible but are not claimed again.
	VersionJudgementUndecided VersionJudgement = "undecided"
	// Failed candidates keep this state while their version_judge job waits out its retry delay.
	VersionJudgementFailed VersionJudgement = "failed"
	// NoConnection candidates send no request and wait for saved Ollama settings.
	VersionJudgementNoConnection VersionJudgement = "no_connection"
	// Held is never stored: the candidate list reports it for a waiting pair the claim skips because the owner
	// unbundled it or undid its automatic dismissal.
	VersionJudgementHeld VersionJudgement = "held"
)

// VersionJudgeJob is what a judgement reads of a claimed pair; Keys are ordered key_a < key_b, and Fingerprints and
// Properties align with them.
type VersionJudgeJob struct {
	Keys         [2]string
	Fingerprints [2]Fingerprint
	Properties   [2]VersionProperties
}

type VersionVerdict int

const (
	// Bundle conflicts turn a same verdict into undecided.
	VersionSame VersionVerdict = iota + 1
	VersionDifferent
	VersionUndecided
	VersionFailed
	VersionNoConnection
)

func (v VersionVerdict) String() string {
	switch v {
	case VersionSame:
		return "same"
	case VersionDifferent:
		return "different"
	case VersionUndecided:
		return "undecided"
	case VersionFailed:
		return "failed"
	case VersionNoConnection:
		return "no_connection"
	default:
		return "unknown"
	}
}

// Probability is present only for model responses; Decider defaults to rule and records AI only for model decisions in the activity log.
// Evidence is nil when the judgement stopped before the frame rule ran.
type VersionJudgeResult struct {
	Verdict     VersionVerdict
	Probability *float64
	Error       string
	Decider     ActivityDecider
	Evidence    *VersionJudgeEvidence
}

// VersionJudgeEvidence is what the activity log keeps of the frame rule, so the owner can see why a pair got its
// verdict. Times are on A's timeline except the B side of each frame.
type VersionJudgeEvidence struct {
	Rule       string `json:"rule"`
	Compared   int    `json:"compared"`
	Differing  int    `json:"differing"`
	LongestRun int    `json:"longestRun,omitempty"`
	RunStartMs int64  `json:"runStartMs,omitempty"`
	RunEndMs   int64  `json:"runEndMs,omitempty"`
	// VersionMismatch marks fingerprints made by different fingerprint versions, which cannot be compared.
	VersionMismatch bool                   `json:"versionMismatch,omitempty"`
	Frames          []VersionEvidenceFrame `json:"frames,omitempty"`
	// Conflict marks a same verdict left undecided because a side is known to differ from the other side.
	Conflict bool `json:"conflict,omitempty"`
	// Properties compare what both files have; ByProperties marks a verdict they decided or held for the owner.
	Properties   []VersionPropertySignal `json:"properties,omitempty"`
	ByProperties bool                    `json:"byProperties,omitempty"`
}

// VersionEvidenceFrame is one frame pair shown to the classifier; A and B are frame indexes in each seek preview.
type VersionEvidenceFrame struct {
	A        int   `json:"a"`
	B        int   `json:"b"`
	AMs      int64 `json:"aMs"`
	BMs      int64 `json:"bMs"`
	Distance int   `json:"distance"`
}

func NewVersionJudgeEvidence(a, b Fingerprint, rule VersionFrameRule) *VersionJudgeEvidence {
	evidence := &VersionJudgeEvidence{
		Rule: rule.Outcome.String(), Compared: rule.Compared, Differing: rule.Differing, LongestRun: rule.LongestRun,
		VersionMismatch: a.Version != b.Version,
	}
	if rule.LongestRun > 0 {
		evidence.RunStartMs = int64(rule.Run[0].A) * a.IntervalMs
		evidence.RunEndMs = int64(rule.Run[1].A+1) * a.IntervalMs
	}
	for _, pair := range rule.Shown {
		evidence.Frames = append(evidence.Frames, VersionEvidenceFrame{
			A: pair.A, B: pair.B, AMs: int64(pair.A) * a.IntervalMs, BMs: int64(pair.B) * b.IntervalMs,
			Distance: pair.Distance,
		})
	}
	return evidence
}

func (e VersionJudgeEvidence) Encode() string {
	body, _ := json.Marshal(e) // Marshaling plain numbers, strings and booleans cannot fail.
	return string(body)
}

func DecodeVersionJudgeEvidence(value string) (*VersionJudgeEvidence, error) {
	if value == "" {
		return nil, nil
	}
	var evidence VersionJudgeEvidence
	if err := json.Unmarshal([]byte(value), &evidence); err != nil {
		return nil, fmt.Errorf("cannot read the judgement evidence: %w", err)
	}
	return &evidence, nil
}

// InferConflict sets Conflict on evidence recomputed for an undecided pair judged before evidence was stored: a rule
// match or a same answer left undecided can only have met a known difference across the two sides.
func (e *VersionJudgeEvidence) InferConflict(probability *float64) {
	e.Conflict = e.Rule == VersionFramesSame.String() ||
		(probability != nil && VersionVerdictFor(*probability) == VersionSame)
}

// FramePairs returns the shown frames as the pairs the frame source cuts.
func (e VersionJudgeEvidence) FramePairs() []FramePair {
	pairs := make([]FramePair, 0, len(e.Frames))
	for _, frame := range e.Frames {
		pairs = append(pairs, FramePair{A: frame.A, B: frame.B, Distance: frame.Distance})
	}
	return pairs
}

var (
	ErrVersionJudgingOff     = errors.New("duplicate judging is turned off")
	ErrVersionNotRejudgeable = errors.New("the pair is waiting or being judged")
	ErrNotVersionCandidate   = errors.New("the pair is not a judgeable possible duplicate")
)

// VersionRejudgeable lists the states the owner can send back to the classifier: every result that left the pair on
// Duplicates.
func VersionRejudgeable(state VersionJudgement) bool {
	switch state {
	case VersionJudgementUndecided, VersionJudgementFailed, VersionJudgementNoConnection:
		return true
	default:
		return false
	}
}

// VersionCandidatesChanged is published after judgement application or detachment commits.
type VersionCandidatesChanged struct{}

func (VersionCandidatesChanged) event() {}

var versionJudgeRetryDelays = []time.Duration{10 * time.Minute, time.Hour, 6 * time.Hour}

const versionJudgeRetryDaily = 24 * time.Hour

// VersionJudgeRetryDelay is the wait after failed attempt round: ten minutes, one hour, six hours, then daily; rounds
// below one use the first delay.
func VersionJudgeRetryDelay(round int) time.Duration {
	if index := max(round, 1) - 1; index < len(versionJudgeRetryDelays) {
		return versionJudgeRetryDelays[index]
	}
	return versionJudgeRetryDaily
}

// VersionSidesConflict checks known-different pairs crossing sides, ignoring within-side pairs and key order.
func VersionSidesConflict(a, b []string, differing [][2]string) bool {
	side := make(map[string]int, len(a)+len(b))
	for _, key := range a {
		side[key] |= 1
	}
	for _, key := range b {
		side[key] |= 2
	}
	for _, pair := range differing {
		x, y := side[pair[0]], side[pair[1]]
		if (x == 1 && y == 2) || (x == 2 && y == 1) {
			return true
		}
	}
	return false
}
