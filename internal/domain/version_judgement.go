package domain

import (
	"cmp"
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
	// Failed candidates become claimable after RetryAt.
	VersionJudgementFailed VersionJudgement = "failed"
	// NoConnection candidates send no request and wait for saved Ollama settings.
	VersionJudgementNoConnection VersionJudgement = "no_connection"
	// Held is never stored: the candidate list reports it for a waiting pair the claim skips because the owner
	// unbundled it or undid its automatic dismissal.
	VersionJudgementHeld VersionJudgement = "held"
)

// Keys are ordered key_a < key_b, Fingerprints aligns with them, and Attempts counts prior failures.
type VersionJudgeJob struct {
	Keys         [2]string
	Fingerprints [2]Fingerprint
	Attempts     int
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

// Retry delays are ten minutes, one hour, six hours, then daily; attempts below one use the first delay.
func VersionJudgeRetryAt(attempts int, now time.Time) time.Time {
	delay := versionJudgeRetryDaily
	if index := max(attempts, 1) - 1; index < len(versionJudgeRetryDelays) {
		delay = versionJudgeRetryDelays[index]
	}
	return now.Add(delay)
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

// OwnerData includes tags, progress, visibility, display name, applied thumbnail, or work info on the side's user-data key.
type VersionSide struct {
	Video     Video
	OwnerData bool
	Bundled   bool
}

// ChooseVersionRepresentative prefers a bundled side, then owner data, browser playback, audio, pixels, bitrate, and lower ID.
func ChooseVersionRepresentative(sides [2]VersionSide) int {
	if sides[0].Bundled != sides[1].Bundled {
		return pickSide(sides[0].Bundled)
	}
	a, b := sides[0].Video, sides[1].Video
	for _, order := range []int{
		compareTrue(sides[0].OwnerData, sides[1].OwnerData),
		compareTrue(a.Playable, b.Playable),
		compareTrue(a.AudioCodec != "", b.AudioCodec != ""),
		cmp.Compare(videoPixels(b), videoPixels(a)),
		compareBitrate(a, b),
		cmp.Compare(a.ID, b.ID),
	} {
		if order != 0 {
			return pickSide(order < 0)
		}
	}
	return 0
}

func pickSide(first bool) int {
	if first {
		return 0
	}
	return 1
}

func compareTrue(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

func videoPixels(v Video) int64 {
	if v.Width == nil || v.Height == nil {
		return 0
	}
	return int64(*v.Width) * int64(*v.Height)
}

// Compare bitrate by cross multiplication; unknown duration ranks lower.
func compareBitrate(a, b Video) int {
	aKnown := a.DurationMs != nil && *a.DurationMs > 0
	bKnown := b.DurationMs != nil && *b.DurationMs > 0
	if !aKnown || !bKnown {
		return compareTrue(aKnown, bKnown)
	}
	// a.Size / a.Duration > b.Size / b.Duration ⇔ a.Size * b.Duration > b.Size * a.Duration。
	return cmp.Compare(float64(b.SizeBytes)*float64(*a.DurationMs), float64(a.SizeBytes)*float64(*b.DurationMs))
}
